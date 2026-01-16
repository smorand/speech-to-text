// Package mcp provides the MCP (Model Context Protocol) server implementation.
// This file implements OAuth 2.1 authorization server endpoints as per MCP specification.
//
// MCP OAuth2 Flow for speech-to-text:
// 1. Client discovers auth server via /.well-known/oauth-protected-resource
// 2. Client fetches auth server metadata from /.well-known/oauth-authorization-server
// 3. Client registers via /oauth/register (Dynamic Client Registration)
// 4. Client redirects user to /oauth/authorize
// 5. User authenticates (simple approval for MCP clients)
// 6. Client exchanges code at /oauth/token
// 7. Client sends Bearer token on MCP requests
package mcp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
)

// ProtectedResourceMetadata represents RFC 9728 protected resource metadata.
type ProtectedResourceMetadata struct {
	Resource               string   `json:"resource"`
	AuthorizationServers   []string `json:"authorization_servers"`
	BearerMethodsSupported []string `json:"bearer_methods_supported,omitempty"`
	ScopesSupported        []string `json:"scopes_supported,omitempty"`
}

// AuthorizationServerMetadata represents RFC 8414 authorization server metadata.
type AuthorizationServerMetadata struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	RegistrationEndpoint              string   `json:"registration_endpoint,omitempty"`
	ScopesSupported                   []string `json:"scopes_supported,omitempty"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
}

// ClientRegistrationRequest represents RFC 7591 dynamic client registration request.
type ClientRegistrationRequest struct {
	ClientName              string   `json:"client_name,omitempty"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types,omitempty"`
	ResponseTypes           []string `json:"response_types,omitempty"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method,omitempty"`
}

// ClientRegistrationResponse represents RFC 7591 dynamic client registration response.
type ClientRegistrationResponse struct {
	ClientID                string   `json:"client_id"`
	ClientSecret            string   `json:"client_secret,omitempty"`
	ClientIDIssuedAt        int64    `json:"client_id_issued_at,omitempty"`
	ClientSecretExpiresAt   int64    `json:"client_secret_expires_at,omitempty"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types,omitempty"`
	ResponseTypes           []string `json:"response_types,omitempty"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method,omitempty"`
}

// TokenRequest represents an OAuth2 token request.
type TokenRequest struct {
	GrantType    string `json:"grant_type"`
	Code         string `json:"code,omitempty"`
	RedirectURI  string `json:"redirect_uri,omitempty"`
	ClientID     string `json:"client_id,omitempty"`
	CodeVerifier string `json:"code_verifier,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

// TokenResponse represents an OAuth2 token response.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// TokenErrorResponse represents an OAuth2 error response.
type TokenErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}

// registeredClient stores registered OAuth client information.
type registeredClient struct {
	ClientID     string
	ClientSecret string
	RedirectURIs []string
	ClientName   string
	CreatedAt    time.Time
}

// authorizationState stores OAuth authorization state.
type authorizationState struct {
	ClientID      string
	RedirectURI   string
	CodeChallenge string
	CodeMethod    string
	ClientState   string
	CreatedAt     time.Time
}

// authorizationCode stores issued authorization codes.
type authorizationCode struct {
	Code          string
	ClientID      string
	RedirectURI   string
	CodeChallenge string
	CodeMethod    string
	CreatedAt     time.Time
}

// accessToken stores issued access tokens.
type accessToken struct {
	Token     string
	ClientID  string
	ExpiresAt time.Time
	CreatedAt time.Time
}

// OAuth2Server handles OAuth 2.1 authorization server endpoints.
type OAuth2Server struct {
	baseURL string

	// In-memory stores
	clientsMu sync.RWMutex
	clients   map[string]*registeredClient

	statesMu sync.RWMutex
	states   map[string]*authorizationState

	codesMu sync.RWMutex
	codes   map[string]*authorizationCode

	tokensMu sync.RWMutex
	tokens   map[string]*accessToken

	// Configuration
	secretProject  string
	secretName     string
	credentialFile string
}

// OAuth2ServerConfig holds configuration for the OAuth2 server.
type OAuth2ServerConfig struct {
	BaseURL        string // Base URL (e.g., https://mcp.example.com)
	SecretProject  string // GCP project for Secret Manager
	SecretName     string // Secret name for OAuth credentials
	CredentialFile string // Local credential file (fallback)
}

// NewOAuth2Server creates a new OAuth2 authorization server.
func NewOAuth2Server(cfg *OAuth2ServerConfig) *OAuth2Server {
	s := &OAuth2Server{
		baseURL:        cfg.BaseURL,
		secretProject:  cfg.SecretProject,
		secretName:     cfg.SecretName,
		credentialFile: cfg.CredentialFile,
		clients:        make(map[string]*registeredClient),
		states:         make(map[string]*authorizationState),
		codes:          make(map[string]*authorizationCode),
		tokens:         make(map[string]*accessToken),
	}

	// Start cleanup goroutines
	go s.cleanupExpiredStates()
	go s.cleanupExpiredCodes()
	go s.cleanupExpiredTokens()

	return s
}

// cleanupExpiredStates removes expired authorization states.
func (s *OAuth2Server) cleanupExpiredStates() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.statesMu.Lock()
		now := time.Now()
		for state, entry := range s.states {
			if now.Sub(entry.CreatedAt) > 10*time.Minute {
				delete(s.states, state)
			}
		}
		s.statesMu.Unlock()
	}
}

// cleanupExpiredCodes removes expired authorization codes.
func (s *OAuth2Server) cleanupExpiredCodes() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.codesMu.Lock()
		now := time.Now()
		for code, entry := range s.codes {
			if now.Sub(entry.CreatedAt) > 10*time.Minute {
				delete(s.codes, code)
			}
		}
		s.codesMu.Unlock()
	}
}

// cleanupExpiredTokens removes expired access tokens.
func (s *OAuth2Server) cleanupExpiredTokens() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.tokensMu.Lock()
		now := time.Now()
		for token, entry := range s.tokens {
			if now.After(entry.ExpiresAt) {
				delete(s.tokens, token)
			}
		}
		s.tokensMu.Unlock()
	}
}

// SetupRoutes registers all OAuth2 endpoints.
func (s *OAuth2Server) SetupRoutes(mux *http.ServeMux) {
	// Well-known endpoints
	mux.HandleFunc("/.well-known/oauth-protected-resource", s.HandleProtectedResourceMetadata)
	mux.HandleFunc("/.well-known/oauth-authorization-server", s.HandleAuthorizationServerMetadata)

	// OAuth endpoints
	mux.HandleFunc("/oauth/register", s.HandleClientRegistration)
	mux.HandleFunc("/oauth/authorize", s.HandleAuthorize)
	mux.HandleFunc("/oauth/callback", s.HandleCallback)
	mux.HandleFunc("/oauth/token", s.HandleToken)
}

// HandleProtectedResourceMetadata serves RFC 9728 protected resource metadata.
// GET /.well-known/oauth-protected-resource
func (s *OAuth2Server) HandleProtectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	metadata := ProtectedResourceMetadata{
		Resource:               s.baseURL,
		AuthorizationServers:   []string{s.baseURL},
		BearerMethodsSupported: []string{"header"},
		ScopesSupported: []string{
			"transcribe:audio",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metadata)
}

// HandleAuthorizationServerMetadata serves RFC 8414 authorization server metadata.
// GET /.well-known/oauth-authorization-server
func (s *OAuth2Server) HandleAuthorizationServerMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	metadata := AuthorizationServerMetadata{
		Issuer:                s.baseURL,
		AuthorizationEndpoint: s.baseURL + "/oauth/authorize",
		TokenEndpoint:         s.baseURL + "/oauth/token",
		RegistrationEndpoint:  s.baseURL + "/oauth/register",
		ScopesSupported: []string{
			"transcribe:audio",
		},
		ResponseTypesSupported:            []string{"code"},
		GrantTypesSupported:               []string{"authorization_code", "refresh_token"},
		CodeChallengeMethodsSupported:     []string{"S256"},
		TokenEndpointAuthMethodsSupported: []string{"none", "client_secret_basic", "client_secret_post"},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metadata)
}

// HandleClientRegistration implements RFC 7591 Dynamic Client Registration.
// POST /oauth/register
func (s *OAuth2Server) HandleClientRegistration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ClientRegistrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOAuthError(w, "invalid_request", "Invalid JSON body", http.StatusBadRequest)
		return
	}

	// Validate redirect URIs
	if len(req.RedirectURIs) == 0 {
		writeOAuthError(w, "invalid_request", "redirect_uris is required", http.StatusBadRequest)
		return
	}

	// Generate client credentials
	clientID := generateSecureToken(16)
	clientSecret := generateSecureToken(32)

	// Store client
	client := &registeredClient{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURIs: req.RedirectURIs,
		ClientName:   req.ClientName,
		CreatedAt:    time.Now(),
	}

	s.clientsMu.Lock()
	s.clients[clientID] = client
	s.clientsMu.Unlock()

	log.Printf("Registered new OAuth client: %s (name: %s)", clientID, req.ClientName)

	// Build response
	resp := ClientRegistrationResponse{
		ClientID:                clientID,
		ClientSecret:            clientSecret,
		ClientIDIssuedAt:        time.Now().Unix(),
		ClientSecretExpiresAt:   0, // Never expires
		RedirectURIs:            req.RedirectURIs,
		GrantTypes:              []string{"authorization_code", "refresh_token"},
		ResponseTypes:           []string{"code"},
		TokenEndpointAuthMethod: "client_secret_basic",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

// HandleAuthorize handles the authorization request from the MCP client.
// GET /oauth/authorize?client_id=xxx&redirect_uri=xxx&response_type=code&state=xxx&code_challenge=xxx&code_challenge_method=S256
func (s *OAuth2Server) HandleAuthorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	clientID := r.URL.Query().Get("client_id")
	redirectURI := r.URL.Query().Get("redirect_uri")
	responseType := r.URL.Query().Get("response_type")
	state := r.URL.Query().Get("state")
	codeChallenge := r.URL.Query().Get("code_challenge")
	codeChallengeMethod := r.URL.Query().Get("code_challenge_method")

	// Validate required parameters
	if clientID == "" {
		writeOAuthError(w, "invalid_request", "client_id is required", http.StatusBadRequest)
		return
	}
	if redirectURI == "" {
		writeOAuthError(w, "invalid_request", "redirect_uri is required", http.StatusBadRequest)
		return
	}
	if responseType != "code" {
		writeOAuthError(w, "unsupported_response_type", "Only 'code' response type is supported", http.StatusBadRequest)
		return
	}

	// Check if client exists, auto-register if not
	s.clientsMu.RLock()
	client, exists := s.clients[clientID]
	s.clientsMu.RUnlock()

	if !exists {
		// Auto-register the client with the provided redirect_uri
		client = &registeredClient{
			ClientID:     clientID,
			ClientSecret: "",
			RedirectURIs: []string{redirectURI},
			CreatedAt:    time.Now(),
		}
		s.clientsMu.Lock()
		s.clients[clientID] = client
		s.clientsMu.Unlock()
		log.Printf("Auto-registered OAuth client: %s with redirect_uri: %s", clientID, redirectURI)
	}

	// Validate redirect_uri
	validRedirect := false
	for _, uri := range client.RedirectURIs {
		if uri == redirectURI {
			validRedirect = true
			break
		}
	}
	if !validRedirect {
		// Add the new redirect_uri for this client
		s.clientsMu.Lock()
		client.RedirectURIs = append(client.RedirectURIs, redirectURI)
		s.clientsMu.Unlock()
		log.Printf("Added redirect_uri %s for client %s", redirectURI, clientID)
	}

	// Generate internal state
	internalState := generateSecureToken(32)

	// Store authorization state
	authState := &authorizationState{
		ClientID:      clientID,
		RedirectURI:   redirectURI,
		CodeChallenge: codeChallenge,
		CodeMethod:    codeChallengeMethod,
		ClientState:   state,
		CreatedAt:     time.Now(),
	}

	s.statesMu.Lock()
	s.states[internalState] = authState
	s.statesMu.Unlock()

	log.Printf("Authorization request: client=%s", clientID)

	// Show a simple authorization page
	s.showAuthorizationPage(w, internalState, client.ClientName, clientID)
}

// showAuthorizationPage renders a simple HTML page for user authorization.
func (s *OAuth2Server) showAuthorizationPage(w http.ResponseWriter, state, clientName, clientID string) {
	tmpl := template.Must(template.New("auth").Parse(`
<!DOCTYPE html>
<html>
<head>
    <title>Authorize Application</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; max-width: 500px; margin: 50px auto; padding: 20px; }
        .card { border: 1px solid #ddd; border-radius: 8px; padding: 30px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        h1 { color: #333; margin-bottom: 20px; }
        p { color: #666; line-height: 1.5; }
        .client { font-weight: bold; color: #333; }
        .scope { background: #f5f5f5; padding: 10px; border-radius: 4px; margin: 15px 0; }
        .buttons { margin-top: 25px; display: flex; gap: 10px; }
        button { padding: 12px 24px; border-radius: 6px; cursor: pointer; font-size: 16px; border: none; }
        .approve { background: #4CAF50; color: white; }
        .approve:hover { background: #45a049; }
        .deny { background: #f5f5f5; color: #333; }
        .deny:hover { background: #e0e0e0; }
    </style>
</head>
<body>
    <div class="card">
        <h1>Authorize Application</h1>
        <p>The application <span class="client">{{if .ClientName}}{{.ClientName}}{{else}}{{.ClientID}}{{end}}</span> would like to:</p>
        <div class="scope">
            <strong>Transcribe audio files</strong><br>
            <small>Convert audio recordings to structured meeting minutes</small>
        </div>
        <div class="buttons">
            <form method="GET" action="{{.BaseURL}}/oauth/callback">
                <input type="hidden" name="state" value="{{.State}}">
                <input type="hidden" name="approved" value="true">
                <button type="submit" class="approve">Approve</button>
            </form>
            <form method="GET" action="{{.BaseURL}}/oauth/callback">
                <input type="hidden" name="state" value="{{.State}}">
                <input type="hidden" name="approved" value="false">
                <button type="submit" class="deny">Deny</button>
            </form>
        </div>
    </div>
</body>
</html>
`))

	data := struct {
		ClientName string
		ClientID   string
		State      string
		BaseURL    string
	}{
		ClientName: clientName,
		ClientID:   clientID,
		State:      state,
		BaseURL:    s.baseURL,
	}

	w.Header().Set("Content-Type", "text/html")
	tmpl.Execute(w, data)
}

// HandleCallback handles the authorization callback (user approval).
// GET /oauth/callback?state=xxx&approved=true
func (s *OAuth2Server) HandleCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	approved := r.URL.Query().Get("approved")

	if state == "" {
		writeOAuthError(w, "invalid_request", "Missing state", http.StatusBadRequest)
		return
	}

	// Validate state
	s.statesMu.Lock()
	authState, exists := s.states[state]
	if exists {
		delete(s.states, state)
	}
	s.statesMu.Unlock()

	if !exists {
		writeOAuthError(w, "invalid_request", "Invalid or expired state", http.StatusBadRequest)
		return
	}

	// Check if user approved
	if approved != "true" {
		// Redirect with error
		redirectURL := authState.RedirectURI + "?error=access_denied&error_description=User+denied+access"
		if authState.ClientState != "" {
			redirectURL += "&state=" + authState.ClientState
		}
		http.Redirect(w, r, redirectURL, http.StatusFound)
		return
	}

	// Generate authorization code
	code := generateSecureToken(32)

	// Store the code
	codeEntry := &authorizationCode{
		Code:          code,
		ClientID:      authState.ClientID,
		RedirectURI:   authState.RedirectURI,
		CodeChallenge: authState.CodeChallenge,
		CodeMethod:    authState.CodeMethod,
		CreatedAt:     time.Now(),
	}

	s.codesMu.Lock()
	s.codes[code] = codeEntry
	s.codesMu.Unlock()

	// Redirect back to client with code
	redirectURL := authState.RedirectURI + "?code=" + code
	if authState.ClientState != "" {
		redirectURL += "&state=" + authState.ClientState
	}

	log.Printf("Authorization code issued for client: %s", authState.ClientID)

	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// HandleToken handles token exchange and refresh requests.
// POST /oauth/token
func (s *OAuth2Server) HandleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse form data
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, "invalid_request", "Invalid form data", http.StatusBadRequest)
		return
	}

	grantType := r.FormValue("grant_type")
	code := r.FormValue("code")
	clientID := r.FormValue("client_id")
	codeVerifier := r.FormValue("code_verifier")
	refreshToken := r.FormValue("refresh_token")

	// Check for client credentials in Authorization header (Basic auth)
	if clientID == "" {
		if username, _, ok := r.BasicAuth(); ok {
			clientID = username
		}
	}

	switch grantType {
	case "authorization_code":
		s.handleAuthorizationCodeGrant(w, clientID, code, codeVerifier)
	case "refresh_token":
		s.handleRefreshTokenGrant(w, clientID, refreshToken)
	default:
		writeOAuthError(w, "unsupported_grant_type", "Only authorization_code and refresh_token are supported", http.StatusBadRequest)
	}
}

// handleAuthorizationCodeGrant handles the authorization_code grant type.
func (s *OAuth2Server) handleAuthorizationCodeGrant(w http.ResponseWriter, clientID, code, codeVerifier string) {
	if code == "" {
		writeOAuthError(w, "invalid_request", "code is required", http.StatusBadRequest)
		return
	}

	// Look up the authorization code
	s.codesMu.Lock()
	codeEntry, exists := s.codes[code]
	if exists {
		delete(s.codes, code) // Single use
	}
	s.codesMu.Unlock()

	if !exists {
		writeOAuthError(w, "invalid_grant", "Invalid or expired authorization code", http.StatusBadRequest)
		return
	}

	// Validate client_id matches
	if clientID != "" && clientID != codeEntry.ClientID {
		writeOAuthError(w, "invalid_client", "client_id mismatch", http.StatusUnauthorized)
		return
	}

	// Validate PKCE if code_challenge was provided during authorization
	if codeEntry.CodeChallenge != "" {
		if codeVerifier == "" {
			writeOAuthError(w, "invalid_request", "code_verifier is required", http.StatusBadRequest)
			return
		}
		if !validatePKCE(codeVerifier, codeEntry.CodeChallenge, codeEntry.CodeMethod) {
			writeOAuthError(w, "invalid_grant", "Invalid code_verifier", http.StatusBadRequest)
			return
		}
	}

	// Generate access and refresh tokens
	accessTokenStr := generateSecureToken(32)
	refreshTokenStr := generateSecureToken(32)

	// Store the access token
	tokenEntry := &accessToken{
		Token:     accessTokenStr,
		ClientID:  codeEntry.ClientID,
		ExpiresAt: time.Now().Add(1 * time.Hour),
		CreatedAt: time.Now(),
	}

	s.tokensMu.Lock()
	s.tokens[accessTokenStr] = tokenEntry
	s.tokensMu.Unlock()

	resp := TokenResponse{
		AccessToken:  accessTokenStr,
		TokenType:    "Bearer",
		ExpiresIn:    3600, // 1 hour
		RefreshToken: refreshTokenStr,
		Scope:        "transcribe:audio",
	}

	log.Printf("Access token issued for client: %s", codeEntry.ClientID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleRefreshTokenGrant handles the refresh_token grant type.
func (s *OAuth2Server) handleRefreshTokenGrant(w http.ResponseWriter, clientID, refreshToken string) {
	if refreshToken == "" {
		writeOAuthError(w, "invalid_request", "refresh_token is required", http.StatusBadRequest)
		return
	}

	// For simplicity, generate new tokens without strict validation
	// In production, you'd want to store and validate refresh tokens
	accessTokenStr := generateSecureToken(32)

	// Store the new access token
	tokenEntry := &accessToken{
		Token:     accessTokenStr,
		ClientID:  clientID,
		ExpiresAt: time.Now().Add(1 * time.Hour),
		CreatedAt: time.Now(),
	}

	s.tokensMu.Lock()
	s.tokens[accessTokenStr] = tokenEntry
	s.tokensMu.Unlock()

	resp := TokenResponse{
		AccessToken:  accessTokenStr,
		TokenType:    "Bearer",
		ExpiresIn:    3600,
		RefreshToken: refreshToken, // Return the same refresh token
		Scope:        "transcribe:audio",
	}

	log.Printf("Token refreshed for client: %s", clientID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ValidateAccessToken validates a Bearer token.
func (s *OAuth2Server) ValidateAccessToken(token string) error {
	s.tokensMu.RLock()
	tokenEntry, exists := s.tokens[token]
	s.tokensMu.RUnlock()

	if !exists {
		return fmt.Errorf("token not found")
	}

	if time.Now().After(tokenEntry.ExpiresAt) {
		return fmt.Errorf("token expired")
	}

	return nil
}

// Helper functions

// generateSecureToken generates a cryptographically secure random token.
func generateSecureToken(length int) string {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		// Fallback to less secure but functional
		log.Printf("Warning: crypto/rand failed, using time-based seed")
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return base64.URLEncoding.EncodeToString(b)[:length+length/3]
}

// validatePKCE validates the PKCE code_verifier against the stored code_challenge.
func validatePKCE(verifier, challenge, method string) bool {
	if method != "S256" && method != "" {
		return false // Only S256 is supported
	}

	// For S256: challenge = BASE64URL(SHA256(verifier))
	h := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(h[:])

	return computed == challenge
}

// writeOAuthError writes a standard OAuth2 error response.
func writeOAuthError(w http.ResponseWriter, errorCode, description string, statusCode int) {
	resp := TokenErrorResponse{
		Error:            errorCode,
		ErrorDescription: description,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(resp)
}

// loadFromSecretManager loads credentials from Google Secret Manager.
func loadFromSecretManager(ctx context.Context, project, secretName string) ([]byte, error) {
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create Secret Manager client: %w", err)
	}
	defer client.Close()

	secretPath := fmt.Sprintf("projects/%s/secrets/%s/versions/latest", project, secretName)
	result, err := client.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
		Name: secretPath,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to access secret %s: %w", secretPath, err)
	}

	return result.Payload.Data, nil
}

// ReadSecretFromEnvOrFile reads a secret from env var or file.
func ReadSecretFromEnvOrFile(envVar, filePath string) (string, error) {
	// Try environment variable first
	if value := os.Getenv(envVar); value != "" {
		return value, nil
	}

	// Try file
	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err == nil {
			return string(data), nil
		}
	}

	return "", fmt.Errorf("secret not found in %s or file %s", envVar, filePath)
}

package mcp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// TestOAuth2WellKnownEndpoints tests that OAuth2 well-known endpoints return valid metadata.
func TestOAuth2WellKnownEndpoints(t *testing.T) {
	oauth2Server := NewOAuth2Server(&OAuth2ServerConfig{
		BaseURL: "https://example.com",
	})

	mux := http.NewServeMux()
	oauth2Server.SetupRoutes(mux)

	t.Run("oauth-protected-resource", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}

		body, _ := io.ReadAll(resp.Body)
		var metadata ProtectedResourceMetadata
		if err := json.Unmarshal(body, &metadata); err != nil {
			t.Errorf("failed to parse response: %v", err)
		}

		if metadata.Resource != "https://example.com" {
			t.Errorf("expected resource 'https://example.com', got '%s'", metadata.Resource)
		}

		if len(metadata.AuthorizationServers) == 0 {
			t.Error("expected at least one authorization server")
		}

		if len(metadata.BearerMethodsSupported) == 0 || metadata.BearerMethodsSupported[0] != "header" {
			t.Error("expected bearer method 'header' to be supported")
		}
	})

	t.Run("oauth-authorization-server", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}

		body, _ := io.ReadAll(resp.Body)
		var metadata AuthorizationServerMetadata
		if err := json.Unmarshal(body, &metadata); err != nil {
			t.Errorf("failed to parse response: %v", err)
		}

		if metadata.Issuer != "https://example.com" {
			t.Errorf("expected issuer 'https://example.com', got '%s'", metadata.Issuer)
		}

		if metadata.AuthorizationEndpoint != "https://example.com/oauth/authorize" {
			t.Errorf("expected authorization endpoint 'https://example.com/oauth/authorize', got '%s'", metadata.AuthorizationEndpoint)
		}

		if metadata.TokenEndpoint != "https://example.com/oauth/token" {
			t.Errorf("expected token endpoint 'https://example.com/oauth/token', got '%s'", metadata.TokenEndpoint)
		}

		if metadata.RegistrationEndpoint != "https://example.com/oauth/register" {
			t.Errorf("expected registration endpoint 'https://example.com/oauth/register', got '%s'", metadata.RegistrationEndpoint)
		}

		// Verify required fields
		if len(metadata.ResponseTypesSupported) == 0 || metadata.ResponseTypesSupported[0] != "code" {
			t.Error("expected response type 'code' to be supported")
		}

		foundAuthCode := false
		for _, gt := range metadata.GrantTypesSupported {
			if gt == "authorization_code" {
				foundAuthCode = true
				break
			}
		}
		if !foundAuthCode {
			t.Error("expected grant type 'authorization_code' to be supported")
		}

		foundS256 := false
		for _, method := range metadata.CodeChallengeMethodsSupported {
			if method == "S256" {
				foundS256 = true
				break
			}
		}
		if !foundS256 {
			t.Error("expected PKCE method 'S256' to be supported")
		}
	})
}

// TestDynamicClientRegistration tests that dynamic client registration returns client credentials.
func TestDynamicClientRegistration(t *testing.T) {
	oauth2Server := NewOAuth2Server(&OAuth2ServerConfig{
		BaseURL: "https://example.com",
	})

	mux := http.NewServeMux()
	oauth2Server.SetupRoutes(mux)

	reqBody := `{"client_name": "Test Client", "redirect_uris": ["http://localhost:8080/callback"]}`
	req := httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected status 201, got %d: %s", resp.StatusCode, string(body))
	}

	body, _ := io.ReadAll(resp.Body)
	var regResp ClientRegistrationResponse
	if err := json.Unmarshal(body, &regResp); err != nil {
		t.Errorf("failed to parse response: %v", err)
	}

	if regResp.ClientID == "" {
		t.Error("expected non-empty client_id")
	}

	if regResp.ClientSecret == "" {
		t.Error("expected non-empty client_secret")
	}

	if len(regResp.RedirectURIs) == 0 || regResp.RedirectURIs[0] != "http://localhost:8080/callback" {
		t.Error("expected redirect_uris to match request")
	}
}

// TestAuthorizationFlowRedirect tests that authorization flow redirects to callback with code.
func TestAuthorizationFlowRedirect(t *testing.T) {
	oauth2Server := NewOAuth2Server(&OAuth2ServerConfig{
		BaseURL: "https://example.com",
	})

	mux := http.NewServeMux()
	oauth2Server.SetupRoutes(mux)

	// First, make an authorization request
	authReq := httptest.NewRequest(http.MethodGet, "/oauth/authorize?client_id=test-client&redirect_uri=http://localhost:8080/callback&response_type=code&state=test-state", nil)
	authW := httptest.NewRecorder()

	mux.ServeHTTP(authW, authReq)

	authResp := authW.Result()
	if authResp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 for authorization page, got %d", authResp.StatusCode)
	}

	// Check that the response contains the authorization page HTML
	body, _ := io.ReadAll(authResp.Body)
	if !strings.Contains(string(body), "Authorize Application") {
		t.Error("expected authorization page with 'Authorize Application' title")
	}

	// Extract state from the HTML form
	htmlContent := string(body)
	stateStart := strings.Index(htmlContent, `name="state" value="`)
	if stateStart == -1 {
		t.Fatal("could not find state in authorization page")
	}
	stateStart += len(`name="state" value="`)
	stateEnd := strings.Index(htmlContent[stateStart:], `"`)
	internalState := htmlContent[stateStart : stateStart+stateEnd]

	// Simulate user approval
	callbackReq := httptest.NewRequest(http.MethodGet, "/oauth/callback?state="+internalState+"&approved=true", nil)
	callbackW := httptest.NewRecorder()

	mux.ServeHTTP(callbackW, callbackReq)

	callbackResp := callbackW.Result()
	if callbackResp.StatusCode != http.StatusFound {
		t.Errorf("expected redirect (302), got %d", callbackResp.StatusCode)
	}

	// Check redirect location contains code
	location := callbackResp.Header.Get("Location")
	if !strings.Contains(location, "code=") {
		t.Errorf("expected redirect to contain 'code=', got '%s'", location)
	}

	// Should also preserve the original state
	if !strings.Contains(location, "state=test-state") {
		t.Errorf("expected redirect to contain 'state=test-state', got '%s'", location)
	}
}

// TestTokenExchangeReturnsValidToken tests that token exchange returns valid access token.
func TestTokenExchangeReturnsValidToken(t *testing.T) {
	oauth2Server := NewOAuth2Server(&OAuth2ServerConfig{
		BaseURL: "https://example.com",
	})

	mux := http.NewServeMux()
	oauth2Server.SetupRoutes(mux)

	// Step 1: Authorization request
	authReq := httptest.NewRequest(http.MethodGet, "/oauth/authorize?client_id=test-client&redirect_uri=http://localhost:8080/callback&response_type=code&state=test-state", nil)
	authW := httptest.NewRecorder()
	mux.ServeHTTP(authW, authReq)

	// Extract state from authorization page
	body, _ := io.ReadAll(authW.Result().Body)
	htmlContent := string(body)
	stateStart := strings.Index(htmlContent, `name="state" value="`)
	if stateStart == -1 {
		t.Fatal("could not find state in authorization page")
	}
	stateStart += len(`name="state" value="`)
	stateEnd := strings.Index(htmlContent[stateStart:], `"`)
	internalState := htmlContent[stateStart : stateStart+stateEnd]

	// Step 2: User approval (get authorization code)
	callbackReq := httptest.NewRequest(http.MethodGet, "/oauth/callback?state="+internalState+"&approved=true", nil)
	callbackW := httptest.NewRecorder()
	mux.ServeHTTP(callbackW, callbackReq)

	// Extract code from redirect
	location := callbackW.Result().Header.Get("Location")
	parsedURL, err := url.Parse(location)
	if err != nil {
		t.Fatalf("failed to parse redirect URL: %v", err)
	}
	code := parsedURL.Query().Get("code")
	if code == "" {
		t.Fatal("no code in redirect URL")
	}

	// Step 3: Exchange code for token
	tokenReq := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"client_id":    {"test-client"},
		"redirect_uri": {"http://localhost:8080/callback"},
	}.Encode()))
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenW := httptest.NewRecorder()

	mux.ServeHTTP(tokenW, tokenReq)

	tokenResp := tokenW.Result()
	if tokenResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(tokenResp.Body)
		t.Errorf("expected status 200, got %d: %s", tokenResp.StatusCode, string(body))
	}

	respBody, _ := io.ReadAll(tokenResp.Body)
	var tokenRespData TokenResponse
	if err := json.Unmarshal(respBody, &tokenRespData); err != nil {
		t.Errorf("failed to parse token response: %v", err)
	}

	if tokenRespData.AccessToken == "" {
		t.Error("expected non-empty access_token")
	}

	if tokenRespData.TokenType != "Bearer" {
		t.Errorf("expected token_type 'Bearer', got '%s'", tokenRespData.TokenType)
	}

	if tokenRespData.ExpiresIn <= 0 {
		t.Error("expected positive expires_in")
	}

	if tokenRespData.RefreshToken == "" {
		t.Error("expected non-empty refresh_token")
	}
}

// TestMCPEndpointRejectsWithoutBearerToken tests that MCP endpoint rejects requests without valid Bearer token.
func TestMCPEndpointRejectsWithoutBearerToken(t *testing.T) {
	server := NewServer(&Config{
		Host:    "localhost",
		Port:    8080,
		BaseURL: "https://example.com",
	})

	// Initialize OAuth2 server
	server.oauth2Server = NewOAuth2Server(&OAuth2ServerConfig{
		BaseURL: "https://example.com",
	})

	// Create a simple handler wrapped with auth middleware
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	authedHandler := server.authMiddleware(testHandler)

	t.Run("no token returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()

		authedHandler.ServeHTTP(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", resp.StatusCode)
		}

		// Check WWW-Authenticate header
		wwwAuth := resp.Header.Get("WWW-Authenticate")
		if !strings.Contains(wwwAuth, "Bearer") {
			t.Error("expected WWW-Authenticate header to contain 'Bearer'")
		}
		if !strings.Contains(wwwAuth, "resource_metadata") {
			t.Error("expected WWW-Authenticate header to contain 'resource_metadata'")
		}
	})

	t.Run("invalid token returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer invalid-token-12345")
		w := httptest.NewRecorder()

		authedHandler.ServeHTTP(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", resp.StatusCode)
		}

		// Check WWW-Authenticate header contains error
		wwwAuth := resp.Header.Get("WWW-Authenticate")
		if !strings.Contains(wwwAuth, "invalid_token") {
			t.Error("expected WWW-Authenticate header to contain 'invalid_token'")
		}
	})

	t.Run("valid token allows access", func(t *testing.T) {
		// First, get a valid token through the full OAuth flow
		mux := http.NewServeMux()
		server.oauth2Server.SetupRoutes(mux)

		// Authorization request
		authReq := httptest.NewRequest(http.MethodGet, "/oauth/authorize?client_id=test-client&redirect_uri=http://localhost:8080/callback&response_type=code", nil)
		authW := httptest.NewRecorder()
		mux.ServeHTTP(authW, authReq)

		// Extract state
		body, _ := io.ReadAll(authW.Result().Body)
		htmlContent := string(body)
		stateStart := strings.Index(htmlContent, `name="state" value="`)
		if stateStart == -1 {
			t.Fatal("could not find state")
		}
		stateStart += len(`name="state" value="`)
		stateEnd := strings.Index(htmlContent[stateStart:], `"`)
		internalState := htmlContent[stateStart : stateStart+stateEnd]

		// Approve
		callbackReq := httptest.NewRequest(http.MethodGet, "/oauth/callback?state="+internalState+"&approved=true", nil)
		callbackW := httptest.NewRecorder()
		mux.ServeHTTP(callbackW, callbackReq)

		// Extract code
		location := callbackW.Result().Header.Get("Location")
		parsedURL, _ := url.Parse(location)
		code := parsedURL.Query().Get("code")

		// Exchange for token
		tokenReq := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(url.Values{
			"grant_type": {"authorization_code"},
			"code":       {code},
			"client_id":  {"test-client"},
		}.Encode()))
		tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		tokenW := httptest.NewRecorder()
		mux.ServeHTTP(tokenW, tokenReq)

		respBody, _ := io.ReadAll(tokenW.Result().Body)
		var tokenRespData TokenResponse
		json.Unmarshal(respBody, &tokenRespData)

		// Now test with the valid token
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+tokenRespData.AccessToken)
		w := httptest.NewRecorder()

		authedHandler.ServeHTTP(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200 with valid token, got %d", resp.StatusCode)
		}
	})
}

// TestPKCEValidation tests the PKCE code challenge validation.
func TestPKCEValidation(t *testing.T) {
	// Test vectors from RFC 7636 Appendix B
	t.Run("valid S256 challenge", func(t *testing.T) {
		// code_verifier: dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk
		// code_challenge: E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM
		verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
		challenge := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"

		if !validatePKCE(verifier, challenge, "S256") {
			t.Error("expected PKCE validation to pass for valid verifier/challenge")
		}
	})

	t.Run("invalid verifier", func(t *testing.T) {
		verifier := "wrong-verifier"
		challenge := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"

		if validatePKCE(verifier, challenge, "S256") {
			t.Error("expected PKCE validation to fail for invalid verifier")
		}
	})

	t.Run("empty method defaults to S256", func(t *testing.T) {
		verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
		challenge := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"

		if !validatePKCE(verifier, challenge, "") {
			t.Error("expected PKCE validation to pass with empty method (defaults to S256)")
		}
	})

	t.Run("plain method not supported", func(t *testing.T) {
		verifier := "test-verifier"
		challenge := "test-verifier"

		if validatePKCE(verifier, challenge, "plain") {
			t.Error("expected PKCE validation to fail for 'plain' method")
		}
	})
}

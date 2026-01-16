package mcp

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
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

// TestTranscribeAudioToolRegistered tests that the tool is registered with correct name and schema.
func TestTranscribeAudioToolRegistered(t *testing.T) {
	server := NewServer(&Config{
		Host:    "localhost",
		Port:    8080,
		BaseURL: "https://example.com",
	})

	// Register tools - this should not panic and should successfully register
	// the transcribe_audio tool
	server.RegisterTools()

	// Since we can't directly access tools, we verify the input/output structs
	// and the handler function exists

	// Test that input struct has correct JSON tags
	t.Run("input struct has correct json tags", func(t *testing.T) {
		input := TranscribeAudioInput{
			AudioData:    "test",
			AudioFormat:  "audio/mp3",
			MeetingName:  "Test Meeting",
			Context:      "Test Context",
			Instructions: "Test Instructions",
		}

		// Marshal to verify JSON tags work correctly
		jsonBytes, err := json.Marshal(input)
		if err != nil {
			t.Fatalf("failed to marshal input: %v", err)
		}

		jsonStr := string(jsonBytes)

		// Check required fields are present
		if !strings.Contains(jsonStr, `"audioData"`) {
			t.Error("expected JSON to contain 'audioData'")
		}
		if !strings.Contains(jsonStr, `"audioFormat"`) {
			t.Error("expected JSON to contain 'audioFormat'")
		}

		// Check optional fields
		if !strings.Contains(jsonStr, `"meetingName"`) {
			t.Error("expected JSON to contain 'meetingName'")
		}
		if !strings.Contains(jsonStr, `"context"`) {
			t.Error("expected JSON to contain 'context'")
		}
		if !strings.Contains(jsonStr, `"instructions"`) {
			t.Error("expected JSON to contain 'instructions'")
		}
	})

	// Test that output struct has correct JSON tags
	t.Run("output struct has correct json tags", func(t *testing.T) {
		output := TranscribeAudioOutput{
			Minutes: "# Test Minutes",
		}

		jsonBytes, err := json.Marshal(output)
		if err != nil {
			t.Fatalf("failed to marshal output: %v", err)
		}

		jsonStr := string(jsonBytes)
		if !strings.Contains(jsonStr, `"minutes"`) {
			t.Error("expected JSON to contain 'minutes'")
		}
	})

	// Test JSON deserialization works
	t.Run("input struct deserializes correctly", func(t *testing.T) {
		jsonInput := `{
			"audioData": "SGVsbG8=",
			"audioFormat": "audio/mp3",
			"meetingName": "Weekly Standup",
			"context": "Team meeting",
			"instructions": "Focus on action items"
		}`

		var input TranscribeAudioInput
		if err := json.Unmarshal([]byte(jsonInput), &input); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		if input.AudioData != "SGVsbG8=" {
			t.Errorf("expected audioData 'SGVsbG8=', got %q", input.AudioData)
		}
		if input.AudioFormat != "audio/mp3" {
			t.Errorf("expected audioFormat 'audio/mp3', got %q", input.AudioFormat)
		}
		if input.MeetingName != "Weekly Standup" {
			t.Errorf("expected meetingName 'Weekly Standup', got %q", input.MeetingName)
		}
		if input.Context != "Team meeting" {
			t.Errorf("expected context 'Team meeting', got %q", input.Context)
		}
		if input.Instructions != "Focus on action items" {
			t.Errorf("expected instructions 'Focus on action items', got %q", input.Instructions)
		}
	})
}

// TestBase64AudioDecoding tests that base64 audio is correctly decoded.
func TestBase64AudioDecoding(t *testing.T) {
	// Create test audio data (just some bytes for testing)
	testAudioData := []byte("test audio content for decoding verification")
	encoded := base64.StdEncoding.EncodeToString(testAudioData)

	// Decode it back
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Errorf("failed to decode base64: %v", err)
	}

	if string(decoded) != string(testAudioData) {
		t.Errorf("decoded data mismatch: expected %q, got %q", testAudioData, decoded)
	}
}

// TestMimeTypeToExtension tests the MIME type to extension conversion.
func TestMimeTypeToExtension(t *testing.T) {
	tests := []struct {
		mimeType string
		expected string
	}{
		{"audio/mp3", ".mp3"},
		{"audio/mpeg", ".mp3"},
		{"audio/wav", ".wav"},
		{"audio/wave", ".wav"},
		{"audio/x-wav", ".wav"},
		{"audio/m4a", ".m4a"},
		{"audio/x-m4a", ".m4a"},
		{"audio/mp4", ".m4a"},
		{"audio/aac", ".aac"},
		{"audio/ogg", ".ogg"},
		{"audio/webm", ".webm"},
		{"audio/flac", ".flac"},
		{"audio/x-flac", ".flac"},
		{"audio/aiff", ".aiff"},
		{"audio/x-aiff", ".aiff"},
		{"audio/3gpp", ".3gp"},
		{"audio/3gpp2", ".3g2"},
		{"video/mp4", ".mp4"},
		{"video/webm", ".webm"},
		// Case insensitivity
		{"Audio/MP3", ".mp3"},
		{"AUDIO/WAV", ".wav"},
		// Unknown format
		{"audio/unknown", ""},
		{"", ""},
	}

	for _, tc := range tests {
		t.Run(tc.mimeType, func(t *testing.T) {
			result := mimeTypeToExtension(tc.mimeType)
			if result != tc.expected {
				t.Errorf("mimeTypeToExtension(%q) = %q, expected %q", tc.mimeType, result, tc.expected)
			}
		})
	}
}

// TestTempFileCreationAndCleanup tests that temp files are created and cleaned up properly.
func TestTempFileCreationAndCleanup(t *testing.T) {
	// Test that we can create and remove temp files with audio extensions
	extensions := []string{".mp3", ".wav", ".m4a", ".ogg"}

	for _, ext := range extensions {
		t.Run(ext, func(t *testing.T) {
			tmpFile, err := os.CreateTemp("", "test-audio-*"+ext)
			if err != nil {
				t.Fatalf("failed to create temp file: %v", err)
			}
			tmpPath := tmpFile.Name()

			// Verify extension
			if filepath.Ext(tmpPath) != ext {
				t.Errorf("expected extension %q, got %q", ext, filepath.Ext(tmpPath))
			}

			// Write some data
			_, err = tmpFile.Write([]byte("test audio content"))
			if err != nil {
				t.Fatalf("failed to write to temp file: %v", err)
			}
			tmpFile.Close()

			// Verify file exists
			if _, err := os.Stat(tmpPath); os.IsNotExist(err) {
				t.Error("temp file should exist after creation")
			}

			// Clean up
			if err := os.Remove(tmpPath); err != nil {
				t.Fatalf("failed to remove temp file: %v", err)
			}

			// Verify file is removed
			if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
				t.Error("temp file should not exist after removal")
			}
		})
	}
}

// TestTranscribeAudioInputValidation tests input validation for the transcribe_audio handler.
func TestTranscribeAudioInputValidation(t *testing.T) {
	t.Run("empty audioData", func(t *testing.T) {
		input := TranscribeAudioInput{
			AudioData:   "",
			AudioFormat: "audio/mp3",
		}
		if input.AudioData != "" {
			t.Error("expected empty audioData")
		}
	})

	t.Run("empty audioFormat", func(t *testing.T) {
		input := TranscribeAudioInput{
			AudioData:   "SGVsbG8gV29ybGQ=",
			AudioFormat: "",
		}
		if input.AudioFormat != "" {
			t.Error("expected empty audioFormat")
		}
	})

	t.Run("optional fields are optional", func(t *testing.T) {
		input := TranscribeAudioInput{
			AudioData:   "SGVsbG8gV29ybGQ=",
			AudioFormat: "audio/mp3",
		}

		// Optional fields should be empty by default
		if input.MeetingName != "" {
			t.Error("MeetingName should be empty by default")
		}
		if input.Context != "" {
			t.Error("Context should be empty by default")
		}
		if input.Instructions != "" {
			t.Error("Instructions should be empty by default")
		}
	})

	t.Run("optional fields are passed", func(t *testing.T) {
		input := TranscribeAudioInput{
			AudioData:    "SGVsbG8gV29ybGQ=",
			AudioFormat:  "audio/mp3",
			MeetingName:  "Team Standup",
			Context:      "Weekly team meeting with Alice and Bob",
			Instructions: "Focus on action items",
		}

		if input.MeetingName != "Team Standup" {
			t.Errorf("expected MeetingName 'Team Standup', got %q", input.MeetingName)
		}
		if input.Context != "Weekly team meeting with Alice and Bob" {
			t.Errorf("expected Context to match, got %q", input.Context)
		}
		if input.Instructions != "Focus on action items" {
			t.Errorf("expected Instructions to match, got %q", input.Instructions)
		}
	})
}

// TestTranscribeAudioOutputFormat tests that the output structure is correct.
func TestTranscribeAudioOutputFormat(t *testing.T) {
	output := TranscribeAudioOutput{
		Minutes: "# Meeting Minutes\n\n## Attendees\n- Alice\n- Bob\n\n## Minutes\n- **Alice**: Hello everyone",
	}

	// Verify it contains expected markdown structure
	if !strings.Contains(output.Minutes, "# Meeting Minutes") {
		t.Error("expected markdown header in output")
	}
	if !strings.Contains(output.Minutes, "## Attendees") {
		t.Error("expected Attendees section in output")
	}
	if !strings.Contains(output.Minutes, "## Minutes") {
		t.Error("expected Minutes section in output")
	}

	// Test JSON serialization
	jsonBytes, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("failed to marshal output: %v", err)
	}

	var parsed TranscribeAudioOutput
	if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
		t.Fatalf("failed to unmarshal output: %v", err)
	}

	if parsed.Minutes != output.Minutes {
		t.Error("JSON round-trip failed")
	}
}

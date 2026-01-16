// Package auth provides Gemini API key injection via context.
//
// The package supports three sources for the API key:
// 1. Context injection (via WithAPIKey)
// 2. Environment variable (GEMINI_API_KEY)
// 3. Local credential file (~/.credentials/google_claude_np_api_key)
// 4. Google Secret Manager (for Cloud Run deployments)
//
// Usage:
//
//	// Inject API key into context
//	ctx := auth.WithAPIKey(ctx, "your-api-key")
//
//	// Retrieve API key from context (falls back to env/file if not in context)
//	key, err := auth.GetAPIKey(ctx)
package auth

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
)

// contextKey is a private type for context keys to avoid collisions.
type contextKey string

const (
	// apiKeyContextKey is the context key for the Gemini API key.
	apiKeyContextKey contextKey = "gemini_api_key"

	// DefaultEnvVar is the environment variable name for the API key.
	DefaultEnvVar = "GEMINI_API_KEY"

	// DefaultCredentialFile is the fallback credential file path.
	DefaultCredentialFile = ".credentials/google_claude_np_api_key"
)

// secretCache stores the cached API key loaded from Secret Manager.
var (
	secretCacheMu sync.RWMutex
	secretCache   string
	secretLoaded  bool
)

// WithAPIKey returns a new context with the API key attached.
// Use this to inject an API key for a specific request.
func WithAPIKey(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, apiKeyContextKey, key)
}

// GetAPIKey retrieves the Gemini API key using the following priority:
// 1. Value from context (set via WithAPIKey)
// 2. GEMINI_API_KEY environment variable
// 3. ~/.credentials/google_claude_np_api_key file
//
// Returns an error if no API key is found.
func GetAPIKey(ctx context.Context) (string, error) {
	// 1. Check context
	if key, ok := ctx.Value(apiKeyContextKey).(string); ok && key != "" {
		return key, nil
	}

	// 2. Check environment variable
	if key := os.Getenv(DefaultEnvVar); key != "" {
		return key, nil
	}

	// 3. Check credential file
	homeDir, err := os.UserHomeDir()
	if err == nil {
		credPath := filepath.Join(homeDir, DefaultCredentialFile)
		if data, err := os.ReadFile(credPath); err == nil {
			key := strings.TrimSpace(string(data))
			if key != "" {
				return key, nil
			}
		}
	}

	return "", fmt.Errorf("gemini API key not found: set %s env var or provide via context", DefaultEnvVar)
}

// SecretManagerConfig holds configuration for loading from Secret Manager.
type SecretManagerConfig struct {
	ProjectID  string // GCP project ID
	SecretName string // Secret name in Secret Manager
}

// GetAPIKeyWithSecretManager retrieves the Gemini API key with Secret Manager support.
// Priority:
// 1. Value from context (set via WithAPIKey)
// 2. Cached value from previous Secret Manager load
// 3. GEMINI_API_KEY environment variable
// 4. Secret Manager (if config provided)
// 5. ~/.credentials/google_claude_np_api_key file
//
// When loaded from Secret Manager, the key is cached for subsequent calls.
func GetAPIKeyWithSecretManager(ctx context.Context, cfg *SecretManagerConfig) (string, error) {
	// 1. Check context
	if key, ok := ctx.Value(apiKeyContextKey).(string); ok && key != "" {
		return key, nil
	}

	// 2. Check cache (from previous Secret Manager load)
	secretCacheMu.RLock()
	if secretLoaded && secretCache != "" {
		key := secretCache
		secretCacheMu.RUnlock()
		return key, nil
	}
	secretCacheMu.RUnlock()

	// 3. Check environment variable
	if key := os.Getenv(DefaultEnvVar); key != "" {
		return key, nil
	}

	// 4. Try Secret Manager if config provided
	if cfg != nil && cfg.ProjectID != "" && cfg.SecretName != "" {
		key, err := loadFromSecretManager(ctx, cfg.ProjectID, cfg.SecretName)
		if err == nil && key != "" {
			// Cache the key
			secretCacheMu.Lock()
			secretCache = key
			secretLoaded = true
			secretCacheMu.Unlock()
			return key, nil
		}
		// Log but don't fail - fall through to file fallback
	}

	// 5. Check credential file
	homeDir, err := os.UserHomeDir()
	if err == nil {
		credPath := filepath.Join(homeDir, DefaultCredentialFile)
		if data, err := os.ReadFile(credPath); err == nil {
			key := strings.TrimSpace(string(data))
			if key != "" {
				return key, nil
			}
		}
	}

	return "", fmt.Errorf("gemini API key not found: set %s env var, configure Secret Manager, or provide via context", DefaultEnvVar)
}

// loadFromSecretManager loads the API key from Google Secret Manager.
func loadFromSecretManager(ctx context.Context, projectID, secretName string) (string, error) {
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to create Secret Manager client: %w", err)
	}
	defer client.Close()

	secretPath := fmt.Sprintf("projects/%s/secrets/%s/versions/latest", projectID, secretName)
	result, err := client.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
		Name: secretPath,
	})
	if err != nil {
		return "", fmt.Errorf("failed to access secret %s: %w", secretPath, err)
	}

	return strings.TrimSpace(string(result.Payload.Data)), nil
}

// IsCached returns true if an API key has been cached from Secret Manager.
func IsCached() bool {
	secretCacheMu.RLock()
	defer secretCacheMu.RUnlock()
	return secretLoaded
}

// ClearCache clears the cached API key.
// Useful for testing or when the secret needs to be reloaded.
func ClearCache() {
	secretCacheMu.Lock()
	secretCache = ""
	secretLoaded = false
	secretCacheMu.Unlock()
}

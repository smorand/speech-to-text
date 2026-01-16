package auth

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestWithAPIKey(t *testing.T) {
	ctx := context.Background()
	testKey := "test-api-key-12345"

	// Inject key into context
	ctxWithKey := WithAPIKey(ctx, testKey)

	// Verify key is in context
	value, ok := ctxWithKey.Value(apiKeyContextKey).(string)
	if !ok {
		t.Fatal("expected API key to be a string in context")
	}
	if value != testKey {
		t.Errorf("expected key %q, got %q", testKey, value)
	}

	// Verify original context is unchanged
	if _, ok := ctx.Value(apiKeyContextKey).(string); ok {
		t.Error("original context should not have the key")
	}
}

func TestGetAPIKey_FromContext(t *testing.T) {
	// Clear any env var that might interfere
	originalEnv := os.Getenv(DefaultEnvVar)
	os.Unsetenv(DefaultEnvVar)
	defer func() {
		if originalEnv != "" {
			os.Setenv(DefaultEnvVar, originalEnv)
		}
	}()

	ctx := context.Background()
	testKey := "context-api-key-67890"

	// Inject key into context
	ctxWithKey := WithAPIKey(ctx, testKey)

	// Retrieve key from context
	key, err := GetAPIKey(ctxWithKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != testKey {
		t.Errorf("expected key %q, got %q", testKey, key)
	}
}

func TestGetAPIKey_FallbackToEnvVar(t *testing.T) {
	ctx := context.Background()
	testKey := "env-api-key-abcdef"

	// Set env var
	originalEnv := os.Getenv(DefaultEnvVar)
	os.Setenv(DefaultEnvVar, testKey)
	defer func() {
		if originalEnv != "" {
			os.Setenv(DefaultEnvVar, originalEnv)
		} else {
			os.Unsetenv(DefaultEnvVar)
		}
	}()

	// Retrieve key without context injection
	key, err := GetAPIKey(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != testKey {
		t.Errorf("expected key %q, got %q", testKey, key)
	}
}

func TestGetAPIKey_FallbackToCredentialFile(t *testing.T) {
	// Clear env var
	originalEnv := os.Getenv(DefaultEnvVar)
	os.Unsetenv(DefaultEnvVar)
	defer func() {
		if originalEnv != "" {
			os.Setenv(DefaultEnvVar, originalEnv)
		}
	}()

	// Create temp credential file
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}

	credPath := filepath.Join(homeDir, DefaultCredentialFile)

	// Check if the file exists and read its content
	if data, err := os.ReadFile(credPath); err == nil {
		testKey := string(data)

		ctx := context.Background()
		key, err := GetAPIKey(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// The key should be trimmed of whitespace
		if key != testKey[:len(testKey)-1] && key != testKey {
			t.Errorf("expected key to match file content")
		}
	} else {
		// File doesn't exist, verify error is returned
		ctx := context.Background()
		_, err := GetAPIKey(ctx)
		if err == nil {
			t.Error("expected error when no API key source available")
		}
	}
}

func TestGetAPIKey_ContextPrecedenceOverEnvVar(t *testing.T) {
	ctx := context.Background()
	contextKey := "context-key"
	envKey := "env-key"

	// Set both env var and context
	originalEnv := os.Getenv(DefaultEnvVar)
	os.Setenv(DefaultEnvVar, envKey)
	defer func() {
		if originalEnv != "" {
			os.Setenv(DefaultEnvVar, originalEnv)
		} else {
			os.Unsetenv(DefaultEnvVar)
		}
	}()

	ctxWithKey := WithAPIKey(ctx, contextKey)

	// Context should take precedence
	key, err := GetAPIKey(ctxWithKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != contextKey {
		t.Errorf("expected context key %q to take precedence, got %q", contextKey, key)
	}
}

func TestGetAPIKey_EmptyContextValue(t *testing.T) {
	ctx := context.Background()
	envKey := "env-key-for-empty-test"

	// Set env var
	originalEnv := os.Getenv(DefaultEnvVar)
	os.Setenv(DefaultEnvVar, envKey)
	defer func() {
		if originalEnv != "" {
			os.Setenv(DefaultEnvVar, originalEnv)
		} else {
			os.Unsetenv(DefaultEnvVar)
		}
	}()

	// Inject empty key
	ctxWithKey := WithAPIKey(ctx, "")

	// Should fall back to env var when context value is empty
	key, err := GetAPIKey(ctxWithKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != envKey {
		t.Errorf("expected env key %q when context key is empty, got %q", envKey, key)
	}
}

func TestGetAPIKeyWithSecretManager_CacheAfterLoad(t *testing.T) {
	// Clear cache before test
	ClearCache()
	defer ClearCache()

	// Verify cache is initially empty
	if IsCached() {
		t.Error("cache should be empty initially")
	}

	// Manually set the cache (simulating Secret Manager load)
	secretCacheMu.Lock()
	secretCache = "cached-api-key"
	secretLoaded = true
	secretCacheMu.Unlock()

	// Verify cache is set
	if !IsCached() {
		t.Error("cache should be set after manual assignment")
	}

	// Clear env var to ensure we're testing cache
	originalEnv := os.Getenv(DefaultEnvVar)
	os.Unsetenv(DefaultEnvVar)
	defer func() {
		if originalEnv != "" {
			os.Setenv(DefaultEnvVar, originalEnv)
		}
	}()

	// Get key should return cached value
	ctx := context.Background()
	key, err := GetAPIKeyWithSecretManager(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "cached-api-key" {
		t.Errorf("expected cached key %q, got %q", "cached-api-key", key)
	}
}

func TestGetAPIKeyWithSecretManager_ContextPrecedenceOverCache(t *testing.T) {
	// Set up cache
	ClearCache()
	defer ClearCache()

	secretCacheMu.Lock()
	secretCache = "cached-key"
	secretLoaded = true
	secretCacheMu.Unlock()

	// Clear env var
	originalEnv := os.Getenv(DefaultEnvVar)
	os.Unsetenv(DefaultEnvVar)
	defer func() {
		if originalEnv != "" {
			os.Setenv(DefaultEnvVar, originalEnv)
		}
	}()

	// Context should take precedence over cache
	ctx := WithAPIKey(context.Background(), "context-key")
	key, err := GetAPIKeyWithSecretManager(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "context-key" {
		t.Errorf("expected context key %q, got %q", "context-key", key)
	}
}

func TestGetAPIKeyWithSecretManager_EnvVarAfterCache(t *testing.T) {
	// Clear cache
	ClearCache()
	defer ClearCache()

	// Set env var
	testKey := "env-key-with-sm-test"
	originalEnv := os.Getenv(DefaultEnvVar)
	os.Setenv(DefaultEnvVar, testKey)
	defer func() {
		if originalEnv != "" {
			os.Setenv(DefaultEnvVar, originalEnv)
		} else {
			os.Unsetenv(DefaultEnvVar)
		}
	}()

	// Without cache, should fall back to env var
	ctx := context.Background()
	key, err := GetAPIKeyWithSecretManager(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != testKey {
		t.Errorf("expected env key %q, got %q", testKey, key)
	}
}

func TestClearCache(t *testing.T) {
	// Set up cache
	secretCacheMu.Lock()
	secretCache = "test-key"
	secretLoaded = true
	secretCacheMu.Unlock()

	// Verify cache is set
	if !IsCached() {
		t.Error("cache should be set")
	}

	// Clear cache
	ClearCache()

	// Verify cache is cleared
	if IsCached() {
		t.Error("cache should be cleared")
	}

	secretCacheMu.RLock()
	if secretCache != "" {
		t.Error("cache value should be empty")
	}
	secretCacheMu.RUnlock()
}

func TestIsCached(t *testing.T) {
	ClearCache()
	defer ClearCache()

	// Initially not cached
	if IsCached() {
		t.Error("should not be cached initially")
	}

	// Set cache
	secretCacheMu.Lock()
	secretLoaded = true
	secretCacheMu.Unlock()

	// Now should be cached
	if !IsCached() {
		t.Error("should be cached after setting flag")
	}
}

// Package mcp provides the MCP (Model Context Protocol) server implementation
// for speech-to-text, enabling AI assistants to transcribe audio remotely.
package mcp

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Config holds the MCP server configuration.
type Config struct {
	Host           string
	Port           int
	BaseURL        string // Base URL for OAuth callbacks (e.g., https://example.com)
	SecretProject  string // GCP project for Secret Manager
	SecretName     string // Secret Manager secret name for OAuth credentials
	CredentialFile string // Local credential file path (fallback)
}

// Server wraps the MCP server and HTTP server.
type Server struct {
	config       *Config
	mcpServer    *mcp.Server
	httpServer   *http.Server
	oauth2Server *OAuth2Server
}

// NewServer creates a new MCP server with the given configuration.
func NewServer(cfg *Config) *Server {
	// Create the MCP server
	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    "speech-to-text",
		Version: "1.0.0",
	}, nil)

	return &Server{
		config:    cfg,
		mcpServer: mcpServer,
	}
}

// extractBearerToken extracts the token from the Authorization header.
// Expected format: "Bearer <token>"
func extractBearerToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return ""
	}

	const bearerPrefix = "Bearer "
	if !strings.HasPrefix(authHeader, bearerPrefix) {
		return ""
	}

	return strings.TrimPrefix(authHeader, bearerPrefix)
}

// authMiddleware wraps an HTTP handler with OAuth2 Bearer token authentication.
// When no token is provided, returns 401 with WWW-Authenticate header pointing to the
// OAuth2 protected resource metadata endpoint (RFC 9728).
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract Bearer token from Authorization header
		accessToken := extractBearerToken(r)

		// If no token provided, return 401 with proper WWW-Authenticate header
		if accessToken == "" {
			// RFC 9728: WWW-Authenticate header with resource_metadata URL
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(
				`Bearer resource_metadata="%s/.well-known/oauth-protected-resource"`,
				s.config.BaseURL,
			))
			http.Error(w, "Unauthorized: Bearer token required", http.StatusUnauthorized)
			return
		}

		// Validate the access token
		if s.oauth2Server == nil {
			http.Error(w, "OAuth not configured", http.StatusInternalServerError)
			return
		}

		if err := s.oauth2Server.ValidateAccessToken(accessToken); err != nil {
			log.Printf("Token validation error: %v", err)
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(
				`Bearer error="invalid_token", resource_metadata="%s/.well-known/oauth-protected-resource"`,
				s.config.BaseURL,
			))
			http.Error(w, "Unauthorized: invalid token", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RegisterTools registers all MCP tools with the server.
func (s *Server) RegisterTools() {
	// Register ping tool for connectivity testing
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "ping",
		Description: "Test connectivity with the MCP server",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (
		*mcp.CallToolResult,
		struct {
			Message string `json:"message"`
			Time    string `json:"time"`
		},
		error,
	) {
		return nil, struct {
			Message string `json:"message"`
			Time    string `json:"time"`
		}{
			Message: "pong",
			Time:    time.Now().Format(time.RFC3339),
		}, nil
	})

	// TODO: Register transcribe_audio tool in US-00003
}

// Run starts the HTTP server and blocks until shutdown.
func (s *Server) Run(ctx context.Context) error {
	// Register tools
	s.RegisterTools()

	// Create the streamable HTTP handler for MCP
	mcpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return s.mcpServer
	}, &mcp.StreamableHTTPOptions{
		Stateless: false, // Enable session tracking
	})

	// Create HTTP mux for routing
	mux := http.NewServeMux()

	// Initialize OAuth2 server
	s.oauth2Server = NewOAuth2Server(&OAuth2ServerConfig{
		BaseURL:        s.config.BaseURL,
		SecretProject:  s.config.SecretProject,
		SecretName:     s.config.SecretName,
		CredentialFile: s.config.CredentialFile,
	})

	// Register OAuth2 routes (not protected by auth)
	s.oauth2Server.SetupRoutes(mux)
	log.Println("OAuth2 endpoints enabled:")
	log.Println("  - /.well-known/oauth-protected-resource")
	log.Println("  - /.well-known/oauth-authorization-server")
	log.Println("  - /oauth/register")
	log.Println("  - /oauth/authorize")
	log.Println("  - /oauth/callback")
	log.Println("  - /oauth/token")

	// Health check endpoint (not protected by auth)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Wrap MCP handler with authentication middleware
	authedMCPHandler := s.authMiddleware(mcpHandler)

	// MCP endpoint (protected by OAuth2 Bearer token auth)
	mux.Handle("/", authedMCPHandler)

	log.Println("Authentication mode: OAuth2 Bearer tokens")

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Setup graceful shutdown
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		log.Printf("Starting MCP server on %s", addr)
		log.Printf("Base URL: %s", s.config.BaseURL)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// Wait for shutdown signal or error
	select {
	case err := <-errChan:
		return fmt.Errorf("server error: %w", err)
	case sig := <-shutdown:
		log.Printf("Received signal %v, shutting down...", sig)
	case <-ctx.Done():
		log.Println("Context cancelled, shutting down...")
	}

	// Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown error: %w", err)
	}

	log.Println("MCP server stopped")
	return nil
}

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	mcpserver "speech-to-text/internal/mcp"
	"speech-to-text/internal/processor"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	// Check for subcommands before parsing flags
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "mcp":
			printMCPInstructions()
			os.Exit(0)
		case "serve":
			runMCPServer()
			return
		}
	}

	opts := parseFlags()

	if err := validateConfig(opts); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	audioFile := flag.Args()[0]

	logStep("=== Processing: Transcription + Analysis (Gemini) ===")

	proc, err := processor.New(ctx, opts.processor, logStep)
	if err != nil {
		log.Fatalf("Failed to create processor: %v", err)
	}
	defer proc.Close()

	result, err := proc.Process(ctx, audioFile)
	if err != nil {
		log.Fatalf("Processing failed: %v", err)
	}

	writeOutput(opts.output, result, "Meeting minutes")
}

// options holds all parsed configuration
type options struct {
	processor processor.Config
	output    string
}

// getEnvDefault returns environment variable value or default if not set
func getEnvDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// logStep logs a step with timestamp to stderr
func logStep(message string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(os.Stderr, "[%s] %s\n", timestamp, message)
}

// reorderArgs reorders os.Args so flags come before positional arguments.
// This allows users to write flags after the audio file path.
func reorderArgs() {
	if len(os.Args) <= 1 {
		return
	}

	var flags []string
	var positional []string

	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if len(arg) > 0 && arg[0] == '-' {
			flags = append(flags, arg)
			// Check if this flag takes a value (not a boolean flag)
			// Look ahead to see if next arg exists and doesn't start with '-'
			if i+1 < len(args) && len(args[i+1]) > 0 && args[i+1][0] != '-' {
				// All remaining flags take values (no boolean flags left)
				i++
				flags = append(flags, args[i])
			}
		} else {
			positional = append(positional, arg)
		}
	}

	// Rebuild os.Args: program name + flags + positional args
	os.Args = append([]string{os.Args[0]}, append(flags, positional...)...)
}

// parseFlags parses command-line flags and returns configuration
func parseFlags() options {
	// Reorder args to allow flags after positional arguments
	reorderArgs()

	var (
		// Gemini options
		geminiAPIKey = flag.String("gemini-api-key", os.Getenv("GEMINI_API_KEY"), "Gemini API key")
		projectID    = flag.String("project", os.Getenv("GCP_PROJECT"), "GCP project ID (Vertex AI only)")
		location     = flag.String("location", getEnvDefault("GCP_LOCATION", "global"), "GCP location (Vertex AI only)")
		modelName    = flag.String("model", "gemini-3-pro-preview", "Gemini model name")

		// Processing options
		meetingName  = flag.String("m", "", "Meeting name for the title")
		ctxFlag      = flag.String("context", "", "Additional context (participants, meeting type, etc.)")
		instructions = flag.String("i", "", "Custom instructions for processing")

		// Output options
		output = flag.String("o", "", "Output file path (markdown format)")

		// API options
		timeout = flag.Int("timeout", 600, "API request timeout in seconds")
	)

	flag.Parse()

	if len(flag.Args()) == 0 {
		fmt.Fprintf(os.Stderr, "Error: audio file path is required\n")
		fmt.Fprintf(os.Stderr, "\nUsage: %s [options] <audio-file>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s audio.mp3 -o minutes.md\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s audio.mp3 -o minutes.md --context \"Weekly standup with Alice, Bob\"\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Determine if we should use Vertex AI for Gemini
	useVertexAI := false
	if envValue := os.Getenv("GEMINI_USE_VERTEX_AI"); envValue != "" {
		var err error
		useVertexAI, err = strconv.ParseBool(envValue)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Invalid GEMINI_USE_VERTEX_AI value '%s', defaulting to false\n", envValue)
		}
	}

	return options{
		processor: processor.Config{
			APIKey:         *geminiAPIKey,
			ProjectID:      *projectID,
			Location:       *location,
			UseVertexAI:    useVertexAI,
			ModelName:      *modelName,
			MeetingName:    *meetingName,
			Context:        *ctxFlag,
			Instructions:   *instructions,
			RequestTimeout: *timeout,
		},
		output: *output,
	}
}

// validateConfig validates the configuration
func validateConfig(opts options) error {
	if opts.processor.UseVertexAI {
		if opts.processor.ProjectID == "" {
			return fmt.Errorf(`Vertex AI backend requires a GCP project ID.

Please provide it via one of these methods:
  1. Set GCP_PROJECT environment variable in .env file
  2. Pass --project flag

Current backend: Vertex AI (GEMINI_USE_VERTEX_AI=true)
To use Gemini API instead, set GEMINI_USE_VERTEX_AI=false and provide GEMINI_API_KEY`)
		}
	} else {
		if opts.processor.APIKey == "" {
			return fmt.Errorf(`Gemini API key is required.

Please provide it via one of these methods:
  1. Set GEMINI_API_KEY environment variable in .env file
  2. Pass --gemini-api-key flag

Get your API key from: https://makersuite.google.com/app/apikey`)
		}
	}

	return nil
}

// writeOutput writes content to file or stdout
func writeOutput(outputFile, content, contentType string) {
	if outputFile != "" {
		logStep(fmt.Sprintf("Writing %s to: %s", contentType, outputFile))
		if err := os.WriteFile(outputFile, []byte(content), 0644); err != nil {
			log.Fatalf("Failed to write output file: %v", err)
		}

		fileInfo, _ := os.Stat(outputFile)
		fileSizeKB := float64(fileInfo.Size()) / 1024
		logStep(fmt.Sprintf("Successfully wrote %.2f KB to %s", fileSizeKB, outputFile))
		fmt.Printf("✓ %s saved to: %s\n", contentType, outputFile)
	} else {
		logStep(fmt.Sprintf("Displaying %s to stdout", contentType))
		fmt.Println()
		fmt.Printf("=== %s ===\n", contentType)
		fmt.Println(content)
	}
}

// printMCPInstructions prints deployment instructions for the MCP server
func printMCPInstructions() {
	instructions := `
Speech-to-Text MCP Server Deployment Instructions
==================================================

The MCP server allows Claude Code to transcribe audio files via a remote API.
It requires deployment to Google Cloud Run with OAuth2 authentication.

Cloud Run URL Format:
---------------------
https://speech-to-text-mcp-<HASH>-<REGION>.a.run.app

Required Environment Variables:
-------------------------------
  HOST                       Server host (default: 0.0.0.0)
  PORT                       Server port (default: 8080)
  BASE_URL                   Public URL of the Cloud Run service
  PROJECT_ID                 GCP project ID
  ENVIRONMENT                Deployment environment (prd, dev)
  GEMINI_API_KEY_SECRET      Secret Manager reference for Gemini API key
  OAUTH_CREDENTIALS_SECRET   Secret Manager reference for OAuth credentials

Required Secrets in Secret Manager:
-----------------------------------
1. gemini-api-key           Your Gemini API key
2. oauth-credentials        OAuth2 client credentials (JSON format)

Deployment Steps:
-----------------
1. Initialize infrastructure:
   make init-plan
   make init-deploy

2. Generate backend configuration:
   make update-backend

3. Deploy application:
   make docker-build
   make docker-push
   make plan
   make deploy

4. Verify deployment:
   curl https://<YOUR_CLOUD_RUN_URL>/.well-known/oauth-authorization-server

Claude Code MCP Configuration:
------------------------------
Add to your Claude Code settings (settings.json or .mcp.json):

{
  "mcpServers": {
    "speech-to-text": {
      "url": "https://speech-to-text-mcp-<HASH>-<REGION>.a.run.app/mcp",
      "transport": "sse"
    }
  }
}

Available MCP Tools:
--------------------
- ping: Test connectivity with the MCP server
- transcribe_audio: Transcribe audio to meeting minutes
    Input: audioData (base64), audioFormat (MIME type), meetingName, context, instructions
    Output: Markdown-formatted meeting minutes

For more information, see README.md
`
	fmt.Print(instructions)
}

// runMCPServer starts the MCP HTTP server for Cloud Run deployment.
func runMCPServer() {
	// Get configuration from environment variables
	host := getEnvDefault("HOST", "0.0.0.0")
	port := 8080
	if portStr := os.Getenv("PORT"); portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			port = p
		}
	}
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		// Default to localhost for local development
		baseURL = fmt.Sprintf("http://%s:%d", host, port)
	}

	// Secret Manager configuration
	secretProject := os.Getenv("PROJECT_ID")
	geminiSecretName := getEnvDefault("GEMINI_API_KEY_SECRET", "scmstt-gemini-api-key")
	oauthSecretName := getEnvDefault("OAUTH_CREDENTIALS_SECRET", "scmstt-oauth-creds")

	cfg := &mcpserver.Config{
		Host:           host,
		Port:           port,
		BaseURL:        baseURL,
		SecretProject:  secretProject,
		SecretName:     geminiSecretName,
		CredentialFile: "", // Use Secret Manager in Cloud Run
	}

	// Log configuration (without secrets)
	log.Printf("Starting MCP server...")
	log.Printf("  Host: %s", host)
	log.Printf("  Port: %d", port)
	log.Printf("  Base URL: %s", baseURL)
	log.Printf("  Project: %s", secretProject)
	log.Printf("  Gemini Secret: %s", geminiSecretName)
	log.Printf("  OAuth Secret: %s", oauthSecretName)

	// Create and run the server
	server := mcpserver.NewServer(cfg)

	ctx := context.Background()
	if err := server.Run(ctx); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

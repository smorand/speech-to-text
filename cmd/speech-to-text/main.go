package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"speech-to-text/internal/transcriber"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	cfg, outputFile := parseFlags()

	if err := validateConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	trans, err := transcriber.New(ctx, cfg, logStep)
	if err != nil {
		log.Fatalf("Failed to create transcriber: %v", err)
	}
	defer trans.Close()

	audioFile := flag.Args()[0]
	transcription, err := trans.Process(ctx, audioFile)
	if err != nil {
		log.Fatalf("Transcription failed: %v", err)
	}

	writeOutput(outputFile, transcription)
}

// getEnvDefault returns environment variable value or default if not set
func getEnvDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getGCloudProject retrieves the current GCP project from gcloud config
func getGCloudProject() string {
	cmd := exec.Command("gcloud", "config", "get-value", "core/project")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// logStep logs a step with timestamp to stderr
func logStep(message string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(os.Stderr, "[%s] %s\n", timestamp, message)
}

// parseFlags parses command-line flags and returns configuration
func parseFlags() (transcriber.Config, string) {
	var (
		apiKey        = flag.String("api-key", os.Getenv("GEMINI_API_KEY"), "Gemini API key (for non-Vertex AI)")
		chunkDuration = flag.Int("chunk-duration", 20, "Duration of each chunk in minutes")
		location      = flag.String("location", getEnvDefault("GCP_LOCATION", "global"), "GCP location (Vertex AI only)")
		maxRetries    = flag.Int("max-retries", 3, "Maximum number of API retry attempts")
		meetingName   = flag.String("m", "", "Meeting name for the title")
		modelName     = flag.String("model", "gemini-2.5-flash", "Gemini model name")
		output        = flag.String("o", "", "Output file path (markdown format)")
		overlapSeconds = flag.Int("overlap-seconds", 5, "Overlap duration in seconds before/after each chunk")
		parallel      = flag.Int("parallel", 4, "Maximum number of parallel transcription workers")
		projectID     = flag.String("project", "", "GCP project ID (Vertex AI only)")
		timeout       = flag.Int("timeout", 1200, "API request timeout in seconds (default: 1200 = 20 minutes)")
	)

	flag.Parse()

	if len(flag.Args()) == 0 {
		fmt.Fprintf(os.Stderr, "Error: audio file path is required\n")
		fmt.Fprintf(os.Stderr, "\nUsage: %s [options] <audio-file>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nIMPORTANT: Options must come BEFORE the audio file path\n")
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  %s -o output.md audio.ogg\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Validate chunk parameters
	if *chunkDuration < 1 {
		fmt.Fprintf(os.Stderr, "Error: chunk-duration must be at least 1 minute\n")
		os.Exit(1)
	}
	if *overlapSeconds < 0 {
		fmt.Fprintf(os.Stderr, "Error: overlap-seconds cannot be negative\n")
		os.Exit(1)
	}
	if *overlapSeconds >= (*chunkDuration * 60) {
		fmt.Fprintf(os.Stderr, "Error: overlap-seconds must be less than chunk duration\n")
		os.Exit(1)
	}
	if *parallel < 1 {
		fmt.Fprintf(os.Stderr, "Error: parallel workers must be at least 1\n")
		os.Exit(1)
	}
	if *maxRetries < 0 {
		fmt.Fprintf(os.Stderr, "Error: max-retries cannot be negative\n")
		os.Exit(1)
	}
	if *timeout < 10 {
		fmt.Fprintf(os.Stderr, "Error: timeout must be at least 10 seconds\n")
		os.Exit(1)
	}

	// Determine if we should use Vertex AI (default: false, use Gemini API)
	useVertexAI := false
	if envValue := os.Getenv("GEMINI_USE_VERTEX_AI"); envValue != "" {
		var err error
		useVertexAI, err = strconv.ParseBool(envValue)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Invalid GEMINI_USE_VERTEX_AI value '%s' (expected 'true' or 'false'), defaulting to false (Gemini API)\n", envValue)
			useVertexAI = false
		}
	}

	// Resolve project ID: CLI flag > ENV var > gcloud config
	resolvedProjectID := *projectID
	if resolvedProjectID == "" {
		resolvedProjectID = os.Getenv("GCP_PROJECT")
	}
	if resolvedProjectID == "" && useVertexAI {
		resolvedProjectID = getGCloudProject()
	}

	return transcriber.Config{
		APIKey:               *apiKey,
		Location:             *location,
		MeetingName:          *meetingName,
		ModelName:            *modelName,
		ProjectID:            resolvedProjectID,
		UseVertexAI:          useVertexAI,
		ChunkDurationMinutes: *chunkDuration,
		OverlapSeconds:       *overlapSeconds,
		MaxParallelWorkers:   *parallel,
		MaxRetries:           *maxRetries,
		RequestTimeout:       *timeout,
	}, *output
}

// validateConfig validates the configuration
func validateConfig(cfg transcriber.Config) error {
	if cfg.UseVertexAI {
		// Vertex AI backend validation
		if cfg.ProjectID == "" {
			return fmt.Errorf(`Vertex AI backend requires a GCP project ID.

Please provide it via one of these methods:
  1. Set GCP_PROJECT environment variable in .env file
  2. Pass --project flag: --project your-project-id
  3. Configure gcloud: gcloud config set project your-project-id

Current backend: Vertex AI (GEMINI_USE_VERTEX_AI=true)
To use Gemini API instead, set GEMINI_USE_VERTEX_AI=false and provide GEMINI_API_KEY`)
		}
	} else {
		// Gemini API backend validation
		if cfg.APIKey == "" {
			return fmt.Errorf(`Gemini API backend requires an API key.

Please provide it via one of these methods:
  1. Set GEMINI_API_KEY environment variable in .env file
  2. Pass --api-key flag: --api-key your-api-key

Get your API key from: https://makersuite.google.com/app/apikey

Current backend: Gemini API (GEMINI_USE_VERTEX_AI=false or not set)
To use Vertex AI instead, set GEMINI_USE_VERTEX_AI=true and provide GCP_PROJECT`)
		}
	}
	return nil
}

// writeOutput writes transcription to file or stdout
func writeOutput(outputFile, transcription string) {
	if outputFile != "" {
		logStep(fmt.Sprintf("Writing final output to: %s", outputFile))
		if err := os.WriteFile(outputFile, []byte(transcription), 0644); err != nil {
			log.Fatalf("Failed to write output file: %v", err)
		}

		fileInfo, _ := os.Stat(outputFile)
		fileSizeKB := float64(fileInfo.Size()) / 1024
		logStep(fmt.Sprintf("Successfully wrote %.2f KB to %s", fileSizeKB, outputFile))
		fmt.Printf("✓ Meeting minutes saved to: %s\n", outputFile)
	} else {
		logStep("Displaying output to stdout")
		fmt.Println()
		fmt.Println("=== MEETING MINUTES ===")
		fmt.Println(transcription)
	}
}

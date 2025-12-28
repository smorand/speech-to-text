package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
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

// logStep logs a step with timestamp to stderr
func logStep(message string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(os.Stderr, "[%s] %s\n", timestamp, message)
}

// parseFlags parses command-line flags and returns configuration
func parseFlags() (transcriber.Config, string) {
	var (
		location    = flag.String("location", getEnvDefault("GCP_LOCATION", "global"), "GCP location")
		meetingName = flag.String("m", "", "Meeting name for the title")
		modelName   = flag.String("model", "gemini-2.5-flash", "Gemini model name")
		output      = flag.String("o", "", "Output file path (markdown format)")
		projectID   = flag.String("project", os.Getenv("GCP_PROJECT"), "GCP project ID")
	)

	flag.Parse()

	if len(flag.Args()) == 0 {
		fmt.Fprintf(os.Stderr, "Error: audio file path is required\n")
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <audio-file>\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}

	return transcriber.Config{
		Location:    *location,
		MeetingName: *meetingName,
		ModelName:   *modelName,
		ProjectID:   *projectID,
	}, *output
}

// validateConfig validates the configuration
func validateConfig(cfg transcriber.Config) error {
	if cfg.ProjectID == "" {
		return fmt.Errorf("GCP_PROJECT must be set in .env or passed via --project")
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

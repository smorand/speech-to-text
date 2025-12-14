package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
	"google.golang.org/genai"
)

const (
	chunkDurationSeconds = 30 * 60 // 30 minutes
	overlapSeconds       = 30       // 30 seconds overlap
)

// AudioTranscriber handles audio transcription
type AudioTranscriber struct {
	client       *genai.Client
	projectID    string
	location     string
	modelName    string
	meetingName  string
	systemPrompt string
}

// NewAudioTranscriber creates a new transcriber instance
func NewAudioTranscriber(ctx context.Context, projectID, location, modelName, meetingName string) (*AudioTranscriber, error) {
	// Create client with Vertex AI backend
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Project:  projectID,
		Location: location,
		Backend:  genai.BackendVertexAI,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create GenAI client: %w", err)
	}

	systemPrompt := `You are a transcriber for meeting minutes. You transcribe audio to text exactly as mentioned, without shortening or summarizing.
You keep the audio language.

Your tasks:
1. Identify the names of attendees from the conversation (listen for names mentioned during introductions or throughout)
2. Transcribe the conversation with speaker attribution using actual names when identified
3. Format the output as structured markdown meeting minutes

Output format:
# [Meeting Title - will be added later]

## Attendees
- [Attendee Name 1]
- [Attendee Name 2]
- [etc.]

## Minutes
- **[Attendee Name]**: verbatim transcription of what they said...
- **[Attendee Name]**: verbatim transcription of what they said...

If you cannot identify a speaker's name, use "Speaker 1", "Speaker 2", etc.
Transcribe everything verbatim - do not summarize or shorten.`

	return &AudioTranscriber{
		client:       client,
		projectID:    projectID,
		location:     location,
		modelName:    modelName,
		meetingName:  meetingName,
		systemPrompt: systemPrompt,
	}, nil
}

// logStep logs a step with timestamp to stderr
func logStep(message string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(os.Stderr, "[%s] %s\n", timestamp, message)
}

// getAudioDuration returns the duration of an audio file in seconds
func getAudioDuration(audioPath string) (float64, error) {
	cmd := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1", audioPath)
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to get audio duration: %w", err)
	}

	duration, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse duration: %w", err)
	}

	return duration, nil
}

// splitAudioIntoChunks splits audio file into chunks with overlaps
func splitAudioIntoChunks(audioPath string, numChunks int) ([]string, error) {
	var chunkFiles []string
	ext := filepath.Ext(audioPath)

	for i := 0; i < numChunks; i++ {
		var startTime float64
		if i == 0 {
			startTime = 0
		} else {
			startTime = float64(i*chunkDurationSeconds) - overlapSeconds
		}

		chunkPath := filepath.Join(os.TempDir(), fmt.Sprintf("chunk_%d%s", i, ext))

		duration := float64(chunkDurationSeconds + overlapSeconds)

		logStep(fmt.Sprintf("Creating chunk %d/%d - start: %.1fs, duration: %.1fs",
			i+1, numChunks, startTime, duration))

		cmd := exec.Command("ffmpeg", "-i", audioPath, "-ss", fmt.Sprintf("%.1f", startTime),
			"-t", fmt.Sprintf("%.1f", duration), "-acodec", "copy", "-y", chunkPath)
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("failed to create chunk %d: %w", i, err)
		}

		chunkFiles = append(chunkFiles, chunkPath)
	}

	return chunkFiles, nil
}

// transcribeAudioFile transcribes a single audio file
func (at *AudioTranscriber) transcribeAudioFile(ctx context.Context, audioPath string) (string, error) {
	logStep(fmt.Sprintf("Transcribing audio file: %s", filepath.Base(audioPath)))

	// Read audio file
	audioData, err := os.ReadFile(audioPath)
	if err != nil {
		return "", fmt.Errorf("failed to read audio file: %w", err)
	}

	// Get MIME type based on extension
	mimeType := getMimeType(filepath.Ext(audioPath))
	logStep(fmt.Sprintf("Audio format: %s", mimeType))

	logStep("Sending audio to Gemini API for transcription")

	// Create the request with system instruction and audio
	resp, err := at.client.Models.GenerateContent(ctx, at.modelName,
		[]*genai.Content{
			genai.NewContentFromText("Please transcribe this audio file according to your system instructions.", genai.RoleUser),
			genai.NewContentFromBytes(audioData, mimeType, genai.RoleUser),
		},
		&genai.GenerateContentConfig{
			SystemInstruction: genai.NewContentFromText(at.systemPrompt, genai.RoleUser),
		},
	)
	if err != nil {
		return "", fmt.Errorf("failed to generate content: %w", err)
	}

	logStep("Transcription completed successfully")

	// Extract text from response
	var transcription strings.Builder
	for _, cand := range resp.Candidates {
		if cand.Content != nil {
			for _, part := range cand.Content.Parts {
				if part.Text != "" {
					transcription.WriteString(part.Text)
				}
			}
		}
	}

	return transcription.String(), nil
}

// getMimeType returns MIME type for audio extension
func getMimeType(ext string) string {
	mimeTypes := map[string]string{
		".mp3":  "audio/mp3",
		".wav":  "audio/wav",
		".m4a":  "audio/mp4",
		".aac":  "audio/aac",
		".ogg":  "audio/ogg",
		".flac": "audio/flac",
	}

	if mime, ok := mimeTypes[strings.ToLower(ext)]; ok {
		return mime
	}
	return "audio/mp3"
}

// transcribeChunksParallel transcribes multiple chunks in parallel
func (at *AudioTranscriber) transcribeChunksParallel(ctx context.Context, chunkFiles []string) ([]string, error) {
	numChunks := len(chunkFiles)
	logStep(fmt.Sprintf("Starting parallel transcription of %d chunks (max 3 concurrent)", numChunks))

	transcriptions := make([]string, numChunks)
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 3) // Max 3 concurrent workers
	errChan := make(chan error, numChunks)

	for i, chunkFile := range chunkFiles {
		wg.Add(1)
		go func(idx int, file string) {
			defer wg.Done()
			semaphore <- struct{}{} // Acquire semaphore
			defer func() { <-semaphore }()

			transcription, err := at.transcribeAudioFile(ctx, file)
			if err != nil {
				errChan <- fmt.Errorf("chunk %d failed: %w", idx+1, err)
				return
			}

			transcriptions[idx] = transcription
			logStep(fmt.Sprintf("Transcription of chunk %d/%d completed", idx+1, numChunks))
		}(i, chunkFile)
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	if len(errChan) > 0 {
		return nil, <-errChan
	}

	return transcriptions, nil
}

// mergeTranscriptions merges all chunks in one pass
func (at *AudioTranscriber) mergeTranscriptions(ctx context.Context, transcriptions []string) (string, error) {
	if len(transcriptions) == 1 {
		logStep("Single chunk - no merging needed")
		return transcriptions[0], nil
	}

	logStep(fmt.Sprintf("Merging all %d chunks in one pass", len(transcriptions)))

	// Build chunks text
	var chunksText strings.Builder
	for i, transcription := range transcriptions {
		chunksText.WriteString(fmt.Sprintf("\n\n%s\nCHUNK %d/%d:\n%s\n%s",
			strings.Repeat("=", 80), i+1, len(transcriptions), strings.Repeat("=", 80), transcription))
	}

	mergePrompt := fmt.Sprintf(`You are merging %d overlapping meeting transcriptions into one complete document.

These chunks were created from a single audio recording split into 30-minute segments with 30-second overlaps between consecutive chunks.

Your task is to:
1. Identify and remove the overlapping sections between consecutive chunks
2. Merge everything into ONE seamless meeting minutes document
3. Consolidate the Attendees lists (remove duplicates, preserve all unique names)
4. Keep the conversation flow natural and continuous in the Minutes section
5. Preserve the markdown format: # Meeting Title, ## Attendees, ## Minutes
6. Use consistent attendee names throughout (don't create duplicates)

HERE ARE ALL THE CHUNKS:
%s

Return ONLY the final merged meeting minutes in proper markdown format without any explanations.`, len(transcriptions), chunksText.String())

	logStep("Sending all chunks to AI for merge reconciliation")

	// Create request without system instruction for merging
	resp, err := at.client.Models.GenerateContent(ctx, at.modelName,
		genai.Text(mergePrompt),
		nil, // No additional config
	)
	if err != nil {
		return "", fmt.Errorf("failed to merge transcriptions: %w", err)
	}

	logStep("All chunks merged successfully")

	// Extract text from response
	var merged strings.Builder
	for _, cand := range resp.Candidates {
		if cand.Content != nil {
			for _, part := range cand.Content.Parts {
				if part.Text != "" {
					merged.WriteString(part.Text)
				}
			}
		}
	}

	return merged.String(), nil
}

// finalizeOutput adds the meeting title to the transcription
func (at *AudioTranscriber) finalizeOutput(transcription, audioFilename string) string {
	var meetingTitle string
	if at.meetingName != "" {
		meetingTitle = at.meetingName
	} else {
		// Extract from filename
		name := filepath.Base(audioFilename)
		name = strings.TrimSuffix(name, filepath.Ext(name))
		name = strings.ReplaceAll(name, "_", " ")
		name = strings.ReplaceAll(name, "-", " ")
		meetingTitle = name
	}

	// Replace title if present
	if strings.HasPrefix(transcription, "#") {
		lines := strings.SplitN(transcription, "\n", 2)
		if len(lines) > 1 {
			return fmt.Sprintf("# %s\n\n%s", meetingTitle, lines[1])
		}
		return fmt.Sprintf("# %s\n\n%s", meetingTitle, transcription)
	}

	// Add title if not present
	return fmt.Sprintf("# %s\n\n%s", meetingTitle, transcription)
}

// TranscribeAudio transcribes an audio file
func (at *AudioTranscriber) TranscribeAudio(ctx context.Context, audioPath string) (string, error) {
	// Step 1: Analyze recording
	logStep(fmt.Sprintf("Analyzing recording: %s", filepath.Base(audioPath)))

	fileInfo, err := os.Stat(audioPath)
	if err != nil {
		return "", fmt.Errorf("failed to stat file: %w", err)
	}

	fileSizeMB := float64(fileInfo.Size()) / (1024 * 1024)
	logStep(fmt.Sprintf("File size: %.2f MB", fileSizeMB))

	duration, err := getAudioDuration(audioPath)
	if err != nil {
		return "", err
	}

	durationMin := duration / 60
	logStep(fmt.Sprintf("Recording duration: %.2f minutes (%.1f seconds)", durationMin, duration))

	var transcription string

	// Check if we need to chunk
	if duration <= chunkDurationSeconds {
		logStep("Recording is under 30 minutes - no chunking needed")
		transcription, err = at.transcribeAudioFile(ctx, audioPath)
		if err != nil {
			return "", err
		}
	} else {
		// Step 2: Chunk the audio
		numChunks := int((duration + chunkDurationSeconds - 1) / chunkDurationSeconds)
		logStep(fmt.Sprintf("Recording exceeds 30 minutes - will split into %d chunks", numChunks))
		logStep(fmt.Sprintf("Cutting recording into %d chunks with 30-second overlaps", numChunks))

		chunkFiles, err := splitAudioIntoChunks(audioPath, numChunks)
		if err != nil {
			return "", err
		}
		logStep(fmt.Sprintf("Successfully created %d chunk files", len(chunkFiles)))

		// Clean up chunks on exit
		defer func() {
			logStep("Cleaning up temporary chunk files")
			for _, chunkFile := range chunkFiles {
				os.Remove(chunkFile)
			}
		}()

		// Step 3: Transcribe chunks in parallel
		transcriptions, err := at.transcribeChunksParallel(ctx, chunkFiles)
		if err != nil {
			return "", err
		}

		// Step 4: Merge transcriptions
		logStep("Starting reconciliation of overlapping chunks")
		transcription, err = at.mergeTranscriptions(ctx, transcriptions)
		if err != nil {
			return "", err
		}
		logStep("Reconciliation completed successfully")
	}

	// Step 5: Finalize output
	logStep("Finalizing output with meeting title")
	return at.finalizeOutput(transcription, audioPath), nil
}

func main() {
	// Load .env file
	_ = godotenv.Load()

	// Parse command-line flags
	var (
		output      = flag.String("o", "", "Output file path (markdown format)")
		meetingName = flag.String("m", "", "Meeting name for the title")
		projectID   = flag.String("project", os.Getenv("GCP_PROJECT"), "GCP project ID")
		location    = flag.String("location", getEnvDefault("GCP_LOCATION", "global"), "GCP location")
		modelName   = flag.String("model", "gemini-2.5-flash", "Gemini model name")
	)

	flag.Parse()

	// Get audio file from positional argument
	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Error: audio file path is required\n")
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <audio-file>\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}

	audioFile := args[0]

	if *projectID == "" {
		fmt.Fprintf(os.Stderr, "Error: GCP_PROJECT must be set in .env or passed via --project\n")
		os.Exit(1)
	}

	// Create context
	ctx := context.Background()

	// Create transcriber
	transcriber, err := NewAudioTranscriber(ctx, *projectID, *location, *modelName, *meetingName)
	if err != nil {
		log.Fatalf("Failed to create transcriber: %v", err)
	}

	// Transcribe audio
	transcription, err := transcriber.TranscribeAudio(ctx, audioFile)
	if err != nil {
		log.Fatalf("Transcription failed: %v", err)
	}

	// Step 5: Write output
	if *output != "" {
		logStep(fmt.Sprintf("Writing final output to: %s", *output))
		if err := os.WriteFile(*output, []byte(transcription), 0644); err != nil {
			log.Fatalf("Failed to write output file: %v", err)
		}
		fileInfo, _ := os.Stat(*output)
		fileSizeKB := float64(fileInfo.Size()) / 1024
		logStep(fmt.Sprintf("Successfully wrote %.2f KB to %s", fileSizeKB, *output))
		fmt.Printf("✓ Meeting minutes saved to: %s\n", *output)
	} else {
		logStep("Displaying output to stdout")
		fmt.Println()
		fmt.Println("=== MEETING MINUTES ===")
		fmt.Println(transcription)
	}
}

func getEnvDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

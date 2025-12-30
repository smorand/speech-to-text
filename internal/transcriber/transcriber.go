package transcriber

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"speech-to-text/internal/audio"

	"google.golang.org/genai"
)

const (
	maxConcurrentWorkers = 3
)

// Config holds configuration for the transcriber
type Config struct {
	APIKey      string
	Location    string
	MeetingName string
	ModelName   string
	ProjectID   string
	UseVertexAI bool
}

// Transcriber handles audio transcription using Vertex AI or Gemini API
type Transcriber struct {
	client       *genai.Client
	config       Config
	logFn        func(string)
	systemPrompt string
}

// New creates a new Transcriber instance
func New(ctx context.Context, cfg Config, logFn func(string)) (*Transcriber, error) {
	var client *genai.Client
	var err error

	if cfg.UseVertexAI {
		// Use Vertex AI backend
		client, err = genai.NewClient(ctx, &genai.ClientConfig{
			Backend:  genai.BackendVertexAI,
			Location: cfg.Location,
			Project:  cfg.ProjectID,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create Vertex AI client: %w", err)
		}
		if logFn != nil {
			logFn(fmt.Sprintf("Using Vertex AI backend with model: %s", cfg.ModelName))
		}
	} else {
		// Use Gemini API backend
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("GEMINI_API_KEY is required when GEMINI_USE_VERTEX_AI is false")
		}
		client, err = genai.NewClient(ctx, &genai.ClientConfig{
			APIKey: cfg.APIKey,
			// Backend defaults to BackendGeminiAPI when not set to BackendVertexAI
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create Gemini API client: %w", err)
		}
		if logFn != nil {
			logFn(fmt.Sprintf("Using Gemini API backend (generativelanguage.googleapis.com) with model: %s", cfg.ModelName))
		}
	}

	return &Transcriber{
		client:       client,
		config:       cfg,
		logFn:        logFn,
		systemPrompt: buildSystemPrompt(),
	}, nil
}

// Close releases resources (no-op for genai.Client)
func (t *Transcriber) Close() {
	// genai.Client doesn't require explicit cleanup
}

// Process transcribes an audio file and returns the meeting minutes
func (t *Transcriber) Process(ctx context.Context, audioPath string) (string, error) {
	t.log(fmt.Sprintf("Analyzing recording: %s", filepath.Base(audioPath)))

	fileInfo, err := os.Stat(audioPath)
	if err != nil {
		return "", fmt.Errorf("failed to stat file: %w", err)
	}

	fileSizeMB := float64(fileInfo.Size()) / (1024 * 1024)
	t.log(fmt.Sprintf("File size: %.2f MB", fileSizeMB))

	duration, err := audio.GetDuration(audioPath)
	if err != nil {
		return "", err
	}

	durationMin := duration / 60
	t.log(fmt.Sprintf("Recording duration: %.2f minutes (%.1f seconds)", durationMin, duration))

	var transcription string

	if duration <= audio.ChunkDurationSeconds {
		transcription, err = t.processSingleFile(ctx, audioPath)
	} else {
		transcription, err = t.processChunkedFile(ctx, audioPath, duration)
	}

	if err != nil {
		return "", err
	}

	t.log("Finalizing output with meeting title")
	return t.addMeetingTitle(transcription, audioPath), nil
}

// addMeetingTitle adds or replaces the meeting title in the transcription
func (t *Transcriber) addMeetingTitle(transcription, audioFilename string) string {
	meetingTitle := t.config.MeetingName
	if meetingTitle == "" {
		meetingTitle = extractTitleFromFilename(audioFilename)
	}

	if strings.HasPrefix(transcription, "#") {
		lines := strings.SplitN(transcription, "\n", 2)
		if len(lines) > 1 {
			return fmt.Sprintf("# %s\n\n%s", meetingTitle, lines[1])
		}
		return fmt.Sprintf("# %s\n\n%s", meetingTitle, transcription)
	}

	return fmt.Sprintf("# %s\n\n%s", meetingTitle, transcription)
}

// buildSystemPrompt constructs the system instruction for transcription
func buildSystemPrompt() string {
	return `You are a transcriber for meeting minutes. You transcribe audio to text exactly as mentioned, without shortening or summarizing.
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
}

// extractTitleFromFilename creates a meeting title from the audio filename
func extractTitleFromFilename(audioFilename string) string {
	name := filepath.Base(audioFilename)
	name = strings.TrimSuffix(name, filepath.Ext(name))
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "-", " ")
	return name
}

// log writes a log message if a log function is configured
func (t *Transcriber) log(message string) {
	if t.logFn != nil {
		t.logFn(message)
	}
}

// mergeTranscriptions merges all chunks in one pass using AI
func (t *Transcriber) mergeTranscriptions(ctx context.Context, transcriptions []string) (string, error) {
	if len(transcriptions) == 1 {
		t.log("Single chunk - no merging needed")
		return transcriptions[0], nil
	}

	t.log(fmt.Sprintf("Merging all %d chunks in one pass", len(transcriptions)))

	chunksText := buildChunksText(transcriptions)
	mergePrompt := buildMergePrompt(len(transcriptions), chunksText)

	t.log("Sending all chunks to AI for merge reconciliation")

	resp, err := t.client.Models.GenerateContent(ctx, t.config.ModelName,
		genai.Text(mergePrompt),
		nil,
	)
	if err != nil {
		return "", fmt.Errorf("failed to merge transcriptions: %w", err)
	}

	t.log("All chunks merged successfully")

	return extractTextFromResponse(resp), nil
}

// processChunkedFile handles audio files that need to be split into chunks
func (t *Transcriber) processChunkedFile(ctx context.Context, audioPath string, duration float64) (string, error) {
	numChunks := calculateNumChunks(duration)
	t.log(fmt.Sprintf("Recording exceeds 30 minutes - will split into %d chunks", numChunks))
	t.log(fmt.Sprintf("Cutting recording into %d chunks with 30-second overlaps", numChunks))

	chunkFiles, err := audio.SplitIntoChunks(audioPath, numChunks, t.logFn)
	if err != nil {
		return "", err
	}
	t.log(fmt.Sprintf("Successfully created %d chunk files", len(chunkFiles)))

	defer t.cleanupChunks(chunkFiles)

	transcriptions, err := t.transcribeChunksParallel(ctx, chunkFiles)
	if err != nil {
		return "", err
	}

	t.log("Starting reconciliation of overlapping chunks")
	transcription, err := t.mergeTranscriptions(ctx, transcriptions)
	if err != nil {
		return "", err
	}
	t.log("Reconciliation completed successfully")

	return transcription, nil
}

// processSingleFile handles audio files that fit within a single chunk
func (t *Transcriber) processSingleFile(ctx context.Context, audioPath string) (string, error) {
	t.log("Recording is under 30 minutes - no chunking needed")
	return t.transcribeFile(ctx, audioPath)
}

// transcribeChunksParallel transcribes multiple chunks in parallel with concurrency limit
func (t *Transcriber) transcribeChunksParallel(ctx context.Context, chunkFiles []string) ([]string, error) {
	numChunks := len(chunkFiles)
	t.log(fmt.Sprintf("Starting parallel transcription of %d chunks (max %d concurrent)", numChunks, maxConcurrentWorkers))

	transcriptions := make([]string, numChunks)
	errChan := make(chan error, numChunks)
	semaphore := make(chan struct{}, maxConcurrentWorkers)
	var wg sync.WaitGroup

	for i, chunkFile := range chunkFiles {
		wg.Add(1)
		go func(idx int, file string) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			transcription, err := t.transcribeFile(ctx, file)
			if err != nil {
				errChan <- fmt.Errorf("chunk %d failed: %w", idx+1, err)
				return
			}

			transcriptions[idx] = transcription
			t.log(fmt.Sprintf("Transcription of chunk %d/%d completed", idx+1, numChunks))
		}(i, chunkFile)
	}

	wg.Wait()
	close(errChan)

	if len(errChan) > 0 {
		return nil, <-errChan
	}

	return transcriptions, nil
}

// transcribeFile transcribes a single audio file using Vertex AI
func (t *Transcriber) transcribeFile(ctx context.Context, audioPath string) (string, error) {
	t.log(fmt.Sprintf("Transcribing audio file: %s", filepath.Base(audioPath)))

	audioData, err := os.ReadFile(audioPath)
	if err != nil {
		return "", fmt.Errorf("failed to read audio file: %w", err)
	}

	mimeType := audio.GetMimeType(filepath.Ext(audioPath))
	t.log(fmt.Sprintf("Audio format: %s", mimeType))
	t.log("Sending audio to Gemini API for transcription")

	resp, err := t.client.Models.GenerateContent(ctx, t.config.ModelName,
		[]*genai.Content{
			genai.NewContentFromText("Please transcribe this audio file according to your system instructions.", genai.RoleUser),
			genai.NewContentFromBytes(audioData, mimeType, genai.RoleUser),
		},
		&genai.GenerateContentConfig{
			SystemInstruction: genai.NewContentFromText(t.systemPrompt, genai.RoleUser),
		},
	)
	if err != nil {
		return "", fmt.Errorf("failed to generate content: %w", err)
	}

	t.log("Transcription completed successfully")

	return extractTextFromResponse(resp), nil
}

// buildChunksText constructs the formatted text of all chunks for merging
func buildChunksText(transcriptions []string) string {
	var chunksText strings.Builder
	for i, transcription := range transcriptions {
		chunksText.WriteString(fmt.Sprintf("\n\n%s\nCHUNK %d/%d:\n%s\n%s",
			strings.Repeat("=", 80), i+1, len(transcriptions), strings.Repeat("=", 80), transcription))
	}
	return chunksText.String()
}

// buildMergePrompt creates the prompt for merging chunks
func buildMergePrompt(numChunks int, chunksText string) string {
	return fmt.Sprintf(`You are merging %d overlapping meeting transcriptions into one complete document.

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

Return ONLY the final merged meeting minutes in proper markdown format without any explanations.`, numChunks, chunksText)
}

// calculateNumChunks determines how many chunks are needed for a given duration
func calculateNumChunks(duration float64) int {
	return int((duration + audio.ChunkDurationSeconds - 1) / audio.ChunkDurationSeconds)
}

// cleanupChunks removes temporary chunk files
func (t *Transcriber) cleanupChunks(chunkFiles []string) {
	t.log("Cleaning up temporary chunk files")
	for _, chunkFile := range chunkFiles {
		os.Remove(chunkFile)
	}
}

// extractTextFromResponse extracts text content from a Gemini API response
func extractTextFromResponse(resp *genai.GenerateContentResponse) string {
	var text strings.Builder
	for _, cand := range resp.Candidates {
		if cand.Content != nil {
			for _, part := range cand.Content.Parts {
				if part.Text != "" {
					text.WriteString(part.Text)
				}
			}
		}
	}
	return text.String()
}

package transcriber

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"speech-to-text/internal/audio"

	"google.golang.org/genai"
)

// Config holds configuration for the transcriber
type Config struct {
	APIKey               string
	Location             string
	MeetingName          string
	ModelName            string
	ProjectID            string
	UseVertexAI          bool
	ChunkDurationMinutes int // Duration of each chunk in minutes (default: 20)
	OverlapSeconds       int // Overlap duration in seconds before/after each chunk (default: 5)
	MaxParallelWorkers   int // Maximum number of parallel transcription workers (default: 4)
	MaxRetries           int // Maximum number of API retry attempts (default: 3)
	RequestTimeout       int // API request timeout in seconds (default: 1200 = 20 minutes)
}

// TokenUsage tracks cumulative token consumption
type TokenUsage struct {
	PromptTokens      int32
	CandidatesTokens  int32
	TotalTokens       int32
	TranscriptionCalls int
	MergeCalls         int
}

// Transcriber handles audio transcription using Vertex AI or Gemini API
type Transcriber struct {
	client       *genai.Client
	config       Config
	logFn        func(string)
	systemPrompt string
	tokenUsage   TokenUsage
	usageMutex   sync.Mutex
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

	// Check if duration exceeds chunk duration threshold
	chunkThreshold := float64(t.config.ChunkDurationMinutes * 60)
	if duration <= chunkThreshold {
		transcription, err = t.processSingleFile(ctx, audioPath)
	} else {
		transcription, err = t.processChunkedFile(ctx, audioPath, duration)
	}

	if err != nil {
		return "", err
	}

	t.log("Finalizing output with meeting title")
	result := t.addMeetingTitle(transcription, audioPath)

	// Log final token usage summary
	t.logFinalTokenUsage()

	return result, nil
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

// trackTokenUsage records token consumption from an API response
func (t *Transcriber) trackTokenUsage(resp *genai.GenerateContentResponse, isTranscription bool) {
	if resp == nil || resp.UsageMetadata == nil {
		return
	}

	t.usageMutex.Lock()
	defer t.usageMutex.Unlock()

	metadata := resp.UsageMetadata
	t.tokenUsage.PromptTokens += metadata.PromptTokenCount
	t.tokenUsage.CandidatesTokens += metadata.CandidatesTokenCount
	t.tokenUsage.TotalTokens += metadata.TotalTokenCount

	if isTranscription {
		t.tokenUsage.TranscriptionCalls++
	} else {
		t.tokenUsage.MergeCalls++
	}

	t.log(fmt.Sprintf("API call tokens - Prompt: %d, Response: %d, Total: %d",
		metadata.PromptTokenCount, metadata.CandidatesTokenCount, metadata.TotalTokenCount))
}

// logFinalTokenUsage logs the cumulative token consumption summary
func (t *Transcriber) logFinalTokenUsage() {
	t.usageMutex.Lock()
	defer t.usageMutex.Unlock()

	if t.tokenUsage.TotalTokens == 0 {
		return
	}

	t.log("═══════════════════════════════════════════════════════════")
	t.log("TOKEN USAGE SUMMARY")
	t.log("═══════════════════════════════════════════════════════════")
	t.log(fmt.Sprintf("API Calls:"))
	t.log(fmt.Sprintf("  - Transcription calls: %d", t.tokenUsage.TranscriptionCalls))
	t.log(fmt.Sprintf("  - Merge calls:         %d", t.tokenUsage.MergeCalls))
	t.log(fmt.Sprintf("  - Total API calls:     %d", t.tokenUsage.TranscriptionCalls+t.tokenUsage.MergeCalls))
	t.log(fmt.Sprintf(""))
	t.log(fmt.Sprintf("Token Consumption:"))
	t.log(fmt.Sprintf("  - Input tokens:        %s", formatNumber(t.tokenUsage.PromptTokens)))
	t.log(fmt.Sprintf("  - Output tokens:       %s", formatNumber(t.tokenUsage.CandidatesTokens)))
	t.log(fmt.Sprintf("  - TOTAL TOKENS:        %s", formatNumber(t.tokenUsage.TotalTokens)))
	t.log("═══════════════════════════════════════════════════════════")
}

// formatNumber formats a number with thousand separators
func formatNumber(n int32) string {
	str := fmt.Sprintf("%d", n)
	if len(str) <= 3 {
		return str
	}

	var result []rune
	for i, digit := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, digit)
	}
	return string(result)
}

// mergeTranscriptions simply concatenates chunks with separator lines
func (t *Transcriber) mergeTranscriptions(ctx context.Context, transcriptions []string) (string, error) {
	if len(transcriptions) == 1 {
		t.log("Single chunk - no merging needed")
		return transcriptions[0], nil
	}

	t.log(fmt.Sprintf("Concatenating %d chunks with separator lines", len(transcriptions)))

	var result strings.Builder
	separator := "\n\n======= CUT =======\n\n"

	for i, transcription := range transcriptions {
		result.WriteString(transcription)

		// Add separator between chunks (but not after the last one)
		if i < len(transcriptions)-1 {
			result.WriteString(separator)
		}
	}

	t.log("All chunks concatenated successfully")
	return result.String(), nil
}

// processChunkedFile handles audio files that need to be split into chunks
func (t *Transcriber) processChunkedFile(ctx context.Context, audioPath string, duration float64) (string, error) {
	chunkDuration := float64(t.config.ChunkDurationMinutes * 60)
	numChunks := int(math.Ceil(duration / chunkDuration))

	t.log(fmt.Sprintf("Recording exceeds %d minutes - will split into %d chunks", t.config.ChunkDurationMinutes, numChunks))
	t.log(fmt.Sprintf("Cutting recording into %d chunks with %d-second overlaps (before and after)",
		numChunks, t.config.OverlapSeconds))

	chunkFiles, err := audio.SplitIntoChunks(audioPath, duration, t.config.ChunkDurationMinutes, t.config.OverlapSeconds, t.logFn)
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
	t.log(fmt.Sprintf("Recording is under %d minutes - no chunking needed", t.config.ChunkDurationMinutes))
	return t.transcribeFile(ctx, audioPath)
}

// transcribeChunksParallel transcribes multiple chunks in parallel with concurrency limit
func (t *Transcriber) transcribeChunksParallel(ctx context.Context, chunkFiles []string) ([]string, error) {
	numChunks := len(chunkFiles)
	t.log(fmt.Sprintf("Starting parallel transcription of %d chunks (max %d concurrent)", numChunks, t.config.MaxParallelWorkers))

	transcriptions := make([]string, numChunks)
	errChan := make(chan error, numChunks)
	semaphore := make(chan struct{}, t.config.MaxParallelWorkers)
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

	// Create context with timeout
	timeout := time.Duration(t.config.RequestTimeout) * time.Second
	if timeout <= 0 {
		timeout = 1200 * time.Second // default 20 minutes
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var resp *genai.GenerateContentResponse
	operation := fmt.Sprintf("transcription of %s", filepath.Base(audioPath))

	t.log(fmt.Sprintf("Sending audio to Gemini API for transcription (timeout: %v)", timeout))

	err = t.retryWithBackoff(timeoutCtx, operation, func() error {
		var apiErr error
		resp, apiErr = t.client.Models.GenerateContent(timeoutCtx, t.config.ModelName,
			[]*genai.Content{
				genai.NewContentFromText("Please transcribe this audio file according to your system instructions.", genai.RoleUser),
				genai.NewContentFromBytes(audioData, mimeType, genai.RoleUser),
			},
			&genai.GenerateContentConfig{
				SystemInstruction: genai.NewContentFromText(t.systemPrompt, genai.RoleUser),
			},
		)
		return apiErr
	})

	if err != nil {
		return "", fmt.Errorf("failed to generate content: %w", err)
	}

	t.log("Transcription completed successfully")

	// Track token usage
	t.trackTokenUsage(resp, true)

	return extractTextFromResponse(resp), nil
}

// cleanupChunks removes temporary chunk files and directory
func (t *Transcriber) cleanupChunks(chunkFiles []string) {
	if len(chunkFiles) == 0 {
		return
	}

	t.log("Cleaning up temporary chunk files")

	// Get the temp directory from the first chunk file
	tempDir := filepath.Dir(chunkFiles[0])

	// Remove the entire temp directory
	if err := os.RemoveAll(tempDir); err != nil {
		t.log(fmt.Sprintf("Warning: failed to remove temp directory %s: %v", tempDir, err))
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

// retryWithBackoff executes a function with exponential backoff retry logic
func (t *Transcriber) retryWithBackoff(ctx context.Context, operation string, fn func() error) error {
	maxRetries := t.config.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3 // default
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Calculate exponential backoff: 2^attempt seconds (2s, 4s, 8s, 16s...)
			backoff := time.Duration(1<<uint(attempt)) * time.Second
			t.log(fmt.Sprintf("Retry attempt %d/%d for %s after %v", attempt, maxRetries, operation, backoff))

			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		err := fn()
		if err == nil {
			if attempt > 0 {
				t.log(fmt.Sprintf("%s succeeded after %d retries", operation, attempt))
			}
			return nil
		}

		lastErr = err

		// Check if error is retryable
		retryable := isRetryableError(err)
		if !retryable {
			// Debug: log why it's not retryable
			t.log(fmt.Sprintf("DEBUG: Error string: '%s'", err.Error()))
			t.log(fmt.Sprintf("DEBUG: Contains 'eof': %v", strings.Contains(strings.ToLower(err.Error()), "eof")))
			t.log(fmt.Sprintf("DEBUG: Contains 'unexpected': %v", strings.Contains(strings.ToLower(err.Error()), "unexpected")))
			t.log(fmt.Sprintf("%s failed with non-retryable error: %v", operation, err))
			return err
		}

		if attempt < maxRetries {
			t.log(fmt.Sprintf("%s failed (attempt %d/%d) - will retry: %v", operation, attempt+1, maxRetries+1, err))
		} else {
			t.log(fmt.Sprintf("%s failed (final attempt %d/%d): %v", operation, attempt+1, maxRetries+1, err))
		}
	}

	return fmt.Errorf("%s failed after %d retries: %w", operation, maxRetries, lastErr)
}

// isRetryableError determines if an error should trigger a retry
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Get the full error chain as string
	errStr := err.Error()
	errStrLower := strings.ToLower(errStr)

	// Also check wrapped errors
	var unwrappedErr error = err
	for unwrappedErr != nil {
		unwrappedStr := strings.ToLower(unwrappedErr.Error())

		// Network-related errors that are retryable
		var netErr net.Error
		if errors.As(unwrappedErr, &netErr) {
			if netErr.Timeout() || netErr.Temporary() {
				return true
			}
		}

		// Check patterns on this level
		for _, pattern := range retryablePatterns() {
			if strings.Contains(unwrappedStr, pattern) {
				return true
			}
		}

		// Try to unwrap
		unwrappedErr = errors.Unwrap(unwrappedErr)
	}

	// Check patterns on full error string as fallback
	for _, pattern := range retryablePatterns() {
		if strings.Contains(errStrLower, pattern) {
			return true
		}
	}

	return false
}

// retryablePatterns returns common retryable error patterns
func retryablePatterns() []string {
	return []string{
		"unexpected eof",          // Network/connection errors
		"eof",                     // Broader EOF catch
		"connection reset",        // Network reset
		"connection refused",      // Connection issues
		"timeout",                 // Any timeout
		"deadline exceeded",       // Context deadline
		"tls handshake timeout",   // TLS issues
		"i/o timeout",            // I/O timeouts
		"no such host",           // DNS issues
		"temporary failure",       // Temporary errors
		"broken pipe",            // Pipe errors
		"503",                    // Service Unavailable
		"429",                    // Too Many Requests
		"500",                    // Internal Server Error
		"502",                    // Bad Gateway
		"504",                    // Gateway Timeout
		"error sending request",  // Generic send errors
		"dorequest",              // genai library error wrapper
	}
}

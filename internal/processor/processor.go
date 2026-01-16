package processor

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"speech-to-text/internal/audio"

	"google.golang.org/genai"
)

// Config holds configuration for the processor
type Config struct {
	// Gemini API configuration
	APIKey      string // Gemini API key (for non-Vertex AI)
	ProjectID   string // GCP project ID (for Vertex AI)
	Location    string // GCP location (for Vertex AI)
	UseVertexAI bool   // Use Vertex AI backend

	// Model configuration
	ModelName string // Gemini model name

	// Processing options
	MeetingName  string // Meeting name/title
	Context      string // Additional context (participants, meeting type, etc.)
	Instructions string // Custom instructions for processing

	// Request configuration
	RequestTimeout int // Request timeout in seconds (default: 120)
}

// Processor handles audio transcription and analysis using Gemini
type Processor struct {
	client *genai.Client
	config Config
	logFn  func(string)
}

// New creates a new Processor instance
func New(ctx context.Context, cfg Config, logFn func(string)) (*Processor, error) {
	var client *genai.Client
	var err error

	// Set defaults
	if cfg.ModelName == "" {
		cfg.ModelName = "gemini-2.5-flash"
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = 120
	}

	timeout := time.Duration(cfg.RequestTimeout) * time.Second
	httpClient := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
			TLSHandshakeTimeout: 30 * time.Second,
		},
	}

	if cfg.UseVertexAI {
		if cfg.ProjectID == "" {
			return nil, fmt.Errorf("GCP_PROJECT is required for Vertex AI backend")
		}
		client, err = genai.NewClient(ctx, &genai.ClientConfig{
			Backend:    genai.BackendVertexAI,
			Location:   cfg.Location,
			Project:    cfg.ProjectID,
			HTTPClient: httpClient,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create Vertex AI client: %w", err)
		}
		if logFn != nil {
			logFn(fmt.Sprintf("Using Vertex AI for processing (model: %s)", cfg.ModelName))
		}
	} else {
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("GEMINI_API_KEY is required for Gemini API backend")
		}
		client, err = genai.NewClient(ctx, &genai.ClientConfig{
			APIKey:     cfg.APIKey,
			HTTPClient: httpClient,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create Gemini API client: %w", err)
		}
		if logFn != nil {
			logFn(fmt.Sprintf("Using Gemini API for processing (model: %s)", cfg.ModelName))
		}
	}

	return &Processor{
		client: client,
		config: cfg,
		logFn:  logFn,
	}, nil
}

// Close releases resources
func (p *Processor) Close() {
	// No cleanup needed for genai.Client
}

// Process takes an audio file and returns structured meeting minutes
func (p *Processor) Process(ctx context.Context, audioFile string) (string, error) {
	p.log("Processing audio with Gemini")

	// Read and encode audio file
	audioData, err := os.ReadFile(audioFile)
	if err != nil {
		return "", fmt.Errorf("failed to read audio file: %w", err)
	}

	// Get MIME type
	ext := strings.ToLower(filepath.Ext(audioFile))
	mimeType := audio.GetMimeType(ext)
	if mimeType == "" {
		return "", fmt.Errorf("unsupported audio format: %s", ext)
	}

	p.log(fmt.Sprintf("Sending audio (%.2f MB) to %s...", float64(len(audioData))/(1024*1024), p.config.ModelName))

	// Build prompts
	systemPrompt := p.buildSystemPrompt()
	userPrompt := p.buildUserPrompt()

	// Build content parts: audio + text prompt
	parts := []*genai.Part{
		{
			InlineData: &genai.Blob{
				MIMEType: mimeType,
				Data:     audioData,
			},
		},
		genai.NewPartFromText(userPrompt),
	}

	resp, err := p.client.Models.GenerateContent(ctx, p.config.ModelName,
		[]*genai.Content{
			{
				Parts: parts,
				Role:  genai.RoleUser,
			},
		},
		&genai.GenerateContentConfig{
			SystemInstruction: &genai.Content{
				Parts: []*genai.Part{genai.NewPartFromText(systemPrompt)},
				Role:  genai.RoleUser,
			},
		},
	)
	if err != nil {
		return "", fmt.Errorf("failed to process audio: %w", err)
	}

	result := extractTextFromResponse(resp)

	// Add meeting title if configured
	if p.config.MeetingName != "" {
		result = p.addMeetingTitle(result)
	}

	p.log("Processing completed")
	return result, nil
}

// buildSystemPrompt constructs the system instruction
func (p *Processor) buildSystemPrompt() string {
	basePrompt := `You are a meeting transcription and minutes formatter. You will receive an audio recording that you need to transcribe and format.

Your tasks:
1. Listen to the audio carefully and transcribe it accurately
2. Identify speakers by their voices and track who says what
3. Identify speaker names from the conversation (listen for names mentioned during introductions or throughout)
4. Format the output as structured markdown meeting minutes
5. Preserve the original language of the audio
6. Keep all content - do not summarize unless specifically instructed

Output format:
# [Meeting Title]

## Attendees
- [Attendee Name 1]
- [Attendee Name 2]
- [etc.]

## Minutes
- **[Speaker Name]**: verbatim transcription of what they said...
- **[Speaker Name]**: verbatim transcription of what they said...

If you cannot identify a speaker's name, use "Speaker 1", "Speaker 2", etc. consistently based on their voice.`

	// Add custom instructions if provided
	if p.config.Instructions != "" {
		basePrompt += fmt.Sprintf("\n\nAdditional instructions:\n%s", p.config.Instructions)
	}

	return basePrompt
}

// buildUserPrompt constructs the user prompt with context
func (p *Processor) buildUserPrompt() string {
	var prompt strings.Builder

	prompt.WriteString("Please transcribe the attached audio and create meeting minutes.\n\n")

	// Add context if provided
	if p.config.Context != "" {
		prompt.WriteString("Context:\n")
		prompt.WriteString(p.config.Context)
		prompt.WriteString("\n\n")
	}

	// Add meeting name if provided
	if p.config.MeetingName != "" {
		prompt.WriteString(fmt.Sprintf("Meeting title: %s\n\n", p.config.MeetingName))
	}

	return prompt.String()
}

// addMeetingTitle ensures the meeting title is at the top
func (p *Processor) addMeetingTitle(content string) string {
	meetingTitle := p.config.MeetingName

	if strings.HasPrefix(content, "#") {
		// Replace existing title
		lines := strings.SplitN(content, "\n", 2)
		if len(lines) > 1 {
			return fmt.Sprintf("# %s\n%s", meetingTitle, lines[1])
		}
	}

	return fmt.Sprintf("# %s\n\n%s", meetingTitle, content)
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

// log writes a log message
func (p *Processor) log(message string) {
	if p.logFn != nil {
		p.logFn(message)
	}
}

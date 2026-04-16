package processor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"speech-to-text/internal/audio"
)

const openRouterURL = "https://openrouter.ai/api/v1/chat/completions"

// OpenAI Chat Completions API types for OpenRouter

type openRouterRequest struct {
	Model    string              `json:"model"`
	Messages []openRouterMessage `json:"messages"`
	Stream   bool                `json:"stream"`
}

type openRouterMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string or []contentPart
}

type contentPart struct {
	Type       string      `json:"type"`
	Text       string      `json:"text,omitempty"`
	InputAudio *inputAudio `json:"input_audio,omitempty"`
}

type inputAudio struct {
	Data   string `json:"data"`   // base64 encoded audio
	Format string `json:"format"` // e.g. "mp3", "wav"
}

// processOpenRouter handles audio transcription via OpenRouter's OpenAI-compatible API
func (p *Processor) processOpenRouter(ctx context.Context, audioFile string) (string, error) {
	p.log("Processing audio with OpenRouter")

	// Read audio file
	audioData, err := os.ReadFile(audioFile)
	if err != nil {
		return "", fmt.Errorf("failed to read audio file: %w", err)
	}

	fileSizeMB := float64(len(audioData)) / (1024 * 1024)
	p.log(fmt.Sprintf("Audio file size: %.2f MB", fileSizeMB))

	// Get MIME type and format
	ext := strings.ToLower(filepath.Ext(audioFile))
	mimeType := audio.GetMimeType(ext)
	if mimeType == "" {
		return "", fmt.Errorf("unsupported audio format: %s", ext)
	}
	format := strings.TrimPrefix(ext, ".") // "mp3", "wav", etc.

	// Base64 encode
	encoded := base64.StdEncoding.EncodeToString(audioData)
	p.log(fmt.Sprintf("Base64 encoded size: %.2f MB", float64(len(encoded))/(1024*1024)))

	// Build prompts (reuse existing methods)
	systemPrompt := p.buildSystemPrompt()
	userPrompt := p.buildUserPrompt()

	// Build request
	req := openRouterRequest{
		Model:  p.openRouterModel,
		Stream: true,
		Messages: []openRouterMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: []contentPart{
				{Type: "input_audio", InputAudio: &inputAudio{
					Data:   encoded,
					Format: format,
				}},
				{Type: "text", Text: userPrompt},
			}},
		},
	}

	p.log(fmt.Sprintf("Transcribing with model: %s", p.openRouterModel))
	return p.streamOpenRouter(ctx, req)
}

// streamOpenRouter sends a streaming request to OpenRouter and returns the accumulated response
func (p *Processor) streamOpenRouter(ctx context.Context, reqBody openRouterRequest) (string, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", openRouterURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.config.OpenRouterAPIKey)
	req.Header.Set("HTTP-Referer", "https://github.com/smorand/speech-to-text")
	req.Header.Set("X-Title", "speech-to-text")

	p.log("Sending streaming request to OpenRouter...")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		if strings.Contains(err.Error(), "EOF") {
			return "", fmt.Errorf("connection closed unexpectedly (EOF). Original error: %w", err)
		}
		return "", fmt.Errorf("failed to send request to OpenRouter: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("OpenRouter API error (status %d): %s", resp.StatusCode, string(errBody))
	}

	// Parse SSE stream
	resultText, err := p.parseSSEStream(resp.Body)
	if err != nil {
		return "", err
	}

	// Add meeting title if configured
	if p.config.MeetingName != "" {
		resultText = p.addMeetingTitle(resultText)
	}

	p.log("Processing completed")
	return resultText, nil
}

// parseSSEStream parses a Server-Sent Events stream from an OpenAI-compatible API
func (p *Processor) parseSSEStream(body io.Reader) (string, error) {
	var result strings.Builder
	scanner := bufio.NewScanner(body)

	// Increase buffer for potentially large SSE lines
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines (SSE delimiter)
		if line == "" {
			continue
		}

		// SSE lines start with "data: "
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		// Check for stream end
		if data == "[DONE]" {
			break
		}

		// Parse the JSON chunk
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Error *struct {
				Message string `json:"message"`
				Code    int    `json:"code"`
			} `json:"error"`
		}

		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			p.log(fmt.Sprintf("Warning: failed to parse SSE chunk: %v", err))
			continue
		}

		// Check for error in stream
		if chunk.Error != nil {
			return "", fmt.Errorf("OpenRouter stream error (%d): %s", chunk.Error.Code, chunk.Error.Message)
		}

		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				result.WriteString(choice.Delta.Content)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading SSE stream: %w", err)
	}

	return result.String(), nil
}

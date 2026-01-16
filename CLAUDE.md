# CLAUDE.md - AI Interaction Guidelines

## Project Overview

**Project Name**: speech-to-text-go
**Type**: Personal Go CLI Tool
**Purpose**: Audio transcription using Google Gemini for meeting minutes generation
**Owner**: Sebastien MORAND (sebastien.morand@loreal.com)

## Architecture

```
┌──────────────────┐     ┌────────────────────────────────┐
│  Audio File      │────▶│  Gemini Processing             │
│  (MP3/WAV/etc.)  │     │  - Full transcription          │
└──────────────────┘     │  - Speaker identification      │
                         │  - Meeting minutes formatting  │
                         └──────────────┬─────────────────┘
                                        │
                                        ▼
                         ┌────────────────────────────────┐
                         │  Output: Structured Minutes    │
                         └────────────────────────────────┘
```

## Technology Stack

- **Language**: Go 1.21+
- **Build System**: Go modules (go.mod)
- **Main Dependencies**:
  - `google.golang.org/genai` - Gemini SDK for Go
  - `github.com/joho/godotenv` - Environment configuration
- **External Services**:
  - Google Gemini API or Vertex AI (required)

## Project Structure

```
speech-to-text/
├── cmd/
│   └── speech-to-text/
│       └── main.go          # CLI entry point, orchestration
├── internal/
│   ├── audio/
│   │   └── processor.go     # Audio utilities (MIME types)
│   └── processor/
│       └── processor.go     # Gemini processing
├── bin/                     # Compiled binaries (gitignored)
├── go.mod                   # Go module definition
├── go.sum                   # Dependency checksums
├── Makefile                 # Build automation
├── README.md                # User documentation
├── CLAUDE.md                # This file
├── .env                     # Local configuration (gitignored)
└── .env.example             # Example configuration
```

## Key Components

### Entry Point (cmd/speech-to-text/main.go)

- CLI flag parsing
- Configuration validation
- Processor orchestration
- Output handling (file or stdout)

### Processor Package (internal/processor/processor.go)

- **Purpose**: Gemini audio processing for transcription
- **Config Struct**: Gemini API key, model, context, instructions
- **Key Methods**:
  - `New()`: Creates Gemini client (API or Vertex AI)
  - `Process(ctx, audioFile)`: Main entry point, sends audio to Gemini
  - `buildSystemPrompt()`: Constructs system instructions
  - `buildUserPrompt()`: Adds context and instructions

### Audio Package (internal/audio/processor.go)

- **Functions**:
  - `GetMimeType()`: Maps file extensions to MIME types

## Environment Configuration

```bash
# Gemini (Required)
GEMINI_API_KEY=xxx       # Gemini API key
# OR for Vertex AI:
GEMINI_USE_VERTEX_AI=true
GCP_PROJECT=xxx
GCP_LOCATION=global
```

## CLI Flags

```
Gemini:
  --gemini-api-key   Gemini API key
  --project          GCP project (Vertex AI)
  --location         GCP location (Vertex AI)
  --model            Gemini model name (default: gemini-3-pro-preview)

Processing:
  -m                 Meeting name/title
  --context          Additional context (participants, etc.)
  -i                 Custom instructions for processing

Output:
  -o                 Output file path

API:
  --timeout          API request timeout in seconds (default: 600)
```

## Development Guidelines

### When Adding Features

1. **Follow Standard Go Project Layout**:
   - Entry points in `cmd/`
   - Business logic in `internal/`
2. **Use Go Conventions**: Naming, error handling, code style
3. **Error Handling**: Return errors with context using `fmt.Errorf` with `%w`

### When Debugging

1. **Check API Keys**: Verify GEMINI_API_KEY
2. **Build Issues**: Run `go mod tidy`

### Code Style

- **Comments**: Godoc-style for exported functions
- **Formatting**: Run `go fmt ./...` before committing
- **Error Messages**: Lowercase, no trailing punctuation

## Common Operations

### Building

```bash
make build              # Build binary
make rebuild            # Clean and rebuild
make build-all          # Build for all platforms
```

### Testing

```bash
# Basic transcription
bin/speech-to-text audio.mp3 -o minutes.md

# With context
bin/speech-to-text audio.mp3 -o minutes.md \
  --context "Meeting with Alice and Bob" \
  -i "Focus on action items"
```

### Adding New Audio Format

1. Update `GetMimeType()` in `internal/audio/processor.go`
2. Add MIME type mapping
3. Update README.md

## File References

- CLI orchestration: `cmd/speech-to-text/main.go`
- Gemini processing: `internal/processor/processor.go`
- Audio handling: `internal/audio/processor.go`
- Dependencies: `go.mod`
- Build: `Makefile`

## Quick Reference

### Build
```bash
make build
```

### Run
```bash
bin/speech-to-text audio.mp3 -o minutes.md
```

### With Context
```bash
bin/speech-to-text audio.mp3 -o minutes.md \
  --context "Weekly standup" \
  -i "Extract action items"
```

## Notes for AI

- This is a **personal project**
- **Simple architecture**: Audio → Gemini → Meeting minutes
- **Go best practices**: Follow conventions
- **Single binary**: One of Go's main advantages

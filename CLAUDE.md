# CLAUDE.md - AI Interaction Guidelines

## Project Overview

**Project Name**: speech-to-text-go
**Type**: Personal Go CLI Tool
**Purpose**: Audio transcription using Google Vertex AI Gemini models
**Owner**: Sebastien MORAND (sebastien.morand@loreal.com)

## Technology Stack

- **Language**: Go 1.21+
- **Build System**: Go modules (go.mod)
- **Main Dependencies**:
  - `cloud.google.com/go/vertexai/genai` - Vertex AI SDK for Go
  - `github.com/joho/godotenv` - Environment configuration
- **GCP Services**: Vertex AI (Gemini models)

## Project Structure

```
speech-to-text/
├── cmd/
│   └── speech-to-text/
│       └── main.go          # CLI entry point (minimal, wiring only)
├── internal/
│   ├── audio/
│   │   └── processor.go     # Audio processing utilities (chunking, MIME types)
│   └── transcriber/
│       └── transcriber.go   # Core transcription logic
├── bin/                     # Compiled binaries (gitignored)
│   └── speech-to-text       # Main binary (after build)
├── go.mod                   # Go module definition
├── go.sum                   # Dependency checksums
├── Makefile                 # Build automation
├── README.md                # User documentation
├── CLAUDE.md                # This file (AI interaction guidelines)
├── .env                     # Local configuration (gitignored)
└── .env.example             # Example configuration
```

**Note**: This follows the Standard Go Project Layout with `cmd/` for entry points and `internal/` for private application code.

## Key Components

### Entry Point (cmd/speech-to-text/main.go)

- Minimal entry point for CLI
- Flag parsing and configuration
- Initialization and wiring
- Output handling

### Audio Package (internal/audio/processor.go)

- **Functions**:
  - `GetDuration()`: Gets audio file duration using ffprobe
  - `GetMimeType()`: Maps file extensions to MIME types
  - `SplitIntoChunks()`: Splits audio into overlapping chunks using ffmpeg
  - `calculateStartTime()`: Helper for chunk timing
  - `createChunk()`: Creates individual chunks

### Transcriber Package (internal/transcriber/transcriber.go)

- **Transcriber Struct**: Main orchestrator for transcription
- **Config Struct**: Configuration for transcription
- **Key Methods**:
  - `New()`: Creates and initializes Transcriber
  - `Process()`: Main orchestration function
  - `transcribeFile()`: Single file transcription via Vertex AI
  - `transcribeChunksParallel()`: Parallel chunk processing with goroutines
  - `mergeTranscriptions()`: AI-based merge of overlapping chunks
  - `addMeetingTitle()`: Adds or replaces meeting title
- **Helper Functions**:
  - `buildSystemPrompt()`: Constructs transcription prompt
  - `extractTextFromResponse()`: Extracts text from Gemini response
  - `buildChunksText()`: Formats chunks for merging

## Development Guidelines

### When Adding Features

1. **Follow Standard Go Project Layout**:
   - Entry points go in `cmd/`
   - Business logic goes in `internal/`
   - Keep `main()` minimal - only wiring and initialization
2. **Package Organization**:
   - Organize by domain/feature, not by layer
   - Keep related functionality together
3. **Use Go Conventions**: Follow Go naming, error handling, and code style
4. **Error Handling**: Return errors with context using `fmt.Errorf` with `%w`
5. **Logging**: Pass log functions as dependencies (dependency injection)
6. **Concurrency**: Use goroutines with sync.WaitGroup and channels
7. **Documentation**: Update README.md and CLAUDE.md, add godoc comments

### When Debugging

1. **Check Authentication**: Verify GCP auth with `gcloud auth application-default login`
2. **Environment Variables**: Ensure `.env` has correct `GCP_PROJECT`
3. **Vertex AI Quota**: Check project quotas if requests fail
4. **Build Issues**: Run `go mod tidy` to fix dependency issues
5. **FFmpeg**: Ensure ffmpeg is installed and in PATH

### Code Style

- **Comments**: Use godoc-style comments for exported functions/types
- **Formatting**: Run `go fmt ./...` before committing
- **Naming**: Use Go naming conventions (camelCase for private, PascalCase for public)
- **Error Messages**: Lowercase, no trailing punctuation (Go convention)
- **Constants**: Use UPPER_CASE or camelCase depending on scope

## Common Operations

### Building the Project

```bash
# Using Makefile (recommended)
make build

# Rebuild from scratch
make rebuild

# Build for all platforms
make build-all

# Manual build
go build -o speech-to-text ./cmd/speech-to-text

# Build for different platforms
GOOS=linux GOARCH=amd64 go build -o speech-to-text-linux ./cmd/speech-to-text
GOOS=darwin GOARCH=arm64 go build -o speech-to-text-macos ./cmd/speech-to-text
GOOS=windows GOARCH=amd64 go build -o speech-to-text.exe ./cmd/speech-to-text
```

### Testing the Tool

```bash
# Basic test with sample audio
bin/speech-to-text path/to/test.mp3

# Test with output file
bin/speech-to-text test.mp3 -o result.md

# Test with different model
bin/speech-to-text test.mp3 --model gemini-2.0-flash-exp
```

### Adding New Audio Format Support

1. Update `GetMimeType()` function in internal/audio/processor.go
2. Add MIME type mapping to the map (keep alphabetically sorted)
3. Test with sample file
4. Update README.md supported formats list

### Modifying Transcription Behavior

1. Edit `buildSystemPrompt()` function in internal/transcriber/transcriber.go
2. Adjust prompts in `buildMergePrompt()` if needed for merging behavior
3. Test with various audio samples
4. Document changes in README.md

## Environment Configuration

### Required Variables

- `GCP_PROJECT`: GCP project ID with Vertex AI enabled

### Optional Variables

- `GCP_LOCATION`: GCP region (default: global)

## GCP Integration

### Required GCP Setup

1. **Vertex AI API**: Must be enabled in the project
2. **Authentication**: Application Default Credentials or service account
3. **Permissions**: `aiplatform.endpoints.predict` or equivalent
4. **Billing**: Active billing account (API usage costs apply)

### Authentication Methods

1. **Standard Account**: `gcloud auth application-default login`
2. **ADM Account** (for sensitive ops): Use impersonation
3. **Service Account**: Set `GOOGLE_APPLICATION_CREDENTIALS` env var

## AI Assistance Context

### When Helping with This Project

- **Go Idioms**: Use Go best practices and idioms
- **Vertex AI SDK**: Use official Google Cloud SDK for Go
- **Error Handling**: Follow Go error handling conventions
- **Concurrency**: Use goroutines and channels properly
- **Memory Management**: Be mindful of goroutine leaks and memory usage

### Common User Requests

1. **"Transcribe this audio"**: Use the compiled binary directly
2. **"Add support for X format"**: Update MIME type mapping
3. **"Change output format"**: Modify system prompt or post-processing
4. **"Handle longer files"**: Adjust timeout or improve chunking
5. **"Reduce costs"**: Suggest different model

### File References

When discussing code, reference specific packages and functions:
- Audio processing: `internal/audio/processor.go`
- Transcription logic: `internal/transcriber/transcriber.go:Transcriber` struct
- CLI entry point: `cmd/speech-to-text/main.go:main()` function
- Dependencies: `go.mod`
- Build process: `Makefile`

## Dependencies Management

### Adding New Dependencies

```bash
# Add a new dependency
go get package-name

# Update go.mod
go mod tidy

# Verify dependencies
go mod verify
```

### Updating Dependencies

```bash
# Update all dependencies
go get -u ./...

# Update specific dependency
go get -u package-name

# Clean up
go mod tidy
```

## Future Enhancements (Potential)

- [ ] Support for video file transcription
- [ ] Batch processing multiple files
- [ ] Translation alongside transcription
- [ ] Custom vocabulary support
- [ ] Real-time streaming transcription
- [ ] Progress bar for long transcriptions
- [ ] Retry logic with exponential backoff
- [ ] Configuration file support (YAML/JSON)

## Notes for AI

- This is a **personal project**, not a BTDP/L'Oréal enterprise project
- **No special BTDP skills needed**
- Standard GCP operations can be run directly
- Focus on **simplicity and performance**
- **Go best practices**: Follow Go conventions and idioms
- **Single binary**: One of the main advantages of Go version

## Quick Reference

### Build Project
```bash
make build
```

### Run Transcription
```bash
bin/speech-to-text audio.mp3 -o output.md
```

### Run Tests (when implemented)
```bash
make test
```

### Update Dependencies
```bash
make install
```

### Format Code
```bash
make fmt
```

### Check for Issues
```bash
make vet
# Or run all checks
make check
```

### See All Make Targets
```bash
make help
```

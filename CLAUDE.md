# CLAUDE.md - AI Interaction Guidelines

## Project Overview

**Project Name**: speech-to-text-go
**Type**: Personal Go CLI Tool
**Purpose**: Audio transcription using Google Vertex AI Gemini models
**Owner**: Sebastien MORAND (sebastien.morand@*******)

## Technology Stack

- **Language**: Go 1.21+
- **Build System**: Go modules (go.mod)
- **Main Dependencies**:
  - `cloud.google.com/go/vertexai/genai` - Vertex AI SDK for Go
  - `github.com/joho/godotenv` - Environment configuration
- **GCP Services**: Vertex AI (Gemini models)

## Project Structure

```
speech-to-text-go/
├── src/
│   └── main.go          # Main application code
├── go.mod               # Go module definition
├── go.sum               # Dependency checksums
├── build.sh             # Build script (executable)
├── speech-to-text       # Compiled binary (after build)
├── README.md            # User documentation
├── CLAUDE.md            # This file
├── .env                 # Local configuration (gitignored)
└── .env.example         # Example configuration
```

## Key Components

### Main Package (src/main.go)

- **AudioTranscriber Struct**: Encapsulates transcription logic
- **Key Functions**:
  - `NewAudioTranscriber()`: Initializes Vertex AI client and model
  - `TranscribeAudio()`: Main orchestration function
  - `transcribeAudioFile()`: Single file transcription
  - `transcribeChunksParallel()`: Parallel chunk processing with goroutines
  - `mergeTranscriptions()`: One-pass AI-based merge
  - `splitAudioIntoChunks()`: FFmpeg-based audio splitting
  - `logStep()`: Timestamped logging to stderr

### CLI Entry Point

- **Function**: `main()` in src/main.go
- **Features**: Flag parsing, error handling, output formatting
- **Binary Name**: `speech-to-text` (created by build.sh)

## Development Guidelines

### When Adding Features

1. **Maintain Struct Organization**: Keep logic in `AudioTranscriber` struct methods
2. **Use Go Conventions**: Follow Go naming, error handling, and code style
3. **Error Handling**: Return errors with context using `fmt.Errorf` with `%w`
4. **Logging**: Use `logStep()` for all progress messages
5. **Concurrency**: Use goroutines with sync.WaitGroup and channels
6. **Documentation**: Update README.md and add godoc comments

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
# Using build script (recommended)
./build.sh

# Manual build
go build -o speech-to-text ./src

# Build for different platforms
GOOS=linux GOARCH=amd64 go build -o speech-to-text-linux ./src
GOOS=darwin GOARCH=arm64 go build -o speech-to-text-macos ./src
GOOS=windows GOARCH=amd64 go build -o speech-to-text.exe ./src
```

### Testing the Tool

```bash
# Basic test with sample audio
./speech-to-text path/to/test.mp3

# Test with output file
./speech-to-text test.mp3 -o result.md

# Test with different model
./speech-to-text test.mp3 --model gemini-2.0-flash-exp
```

### Adding New Audio Format Support

1. Update `getMimeType()` function in src/main.go
2. Add MIME type mapping in the map
3. Test with sample file
4. Update README.md supported formats list

### Modifying Transcription Behavior

1. Edit `systemPrompt` in `NewAudioTranscriber()` function in src/main.go
2. Adjust prompts in transcription/merge functions if needed
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

When discussing code, reference specific functions:
- Transcription logic: `src/main.go:AudioTranscriber` struct
- CLI entry point: `src/main.go:main()` function
- Dependencies: `go.mod`
- Build process: `build.sh`

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

- This is a **personal project**, not a *******/L'******* enterprise project
- **No special ******* skills needed**
- Standard GCP operations can be run directly
- Focus on **simplicity and performance**
- **Go best practices**: Follow Go conventions and idioms
- **Single binary**: One of the main advantages of Go version

## Quick Reference

### Build Project
```bash
./build.sh
```

### Run Transcription
```bash
./speech-to-text --audio audio.mp3
```

### Run Tests (when implemented)
```bash
go test ./...
```

### Update Dependencies
```bash
go get -u ./...
go mod tidy
```

### Format Code
```bash
go fmt ./...
```

### Check for Issues
```bash
go vet ./...
```

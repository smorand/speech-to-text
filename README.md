# Speech-to-Text Transcription Tool

A Go-based audio transcription tool using Google Vertex AI Gemini models. This tool transcribes audio files as structured **meeting minutes** with automatic attendee name detection, speaker attribution, and markdown formatting.

## Features

- **Meeting Minutes Format**: Automatically formats output as structured markdown with attendees list and conversation
- **Attendee Name Detection**: Identifies speaker names from the conversation
- **Auto-Chunking**: Splits audio files longer than 30 minutes with 30-second overlaps for seamless reconstruction
- **Parallel Transcription**: Processes multiple chunks concurrently (max 3 workers)
- **One-Pass Merging**: AI-based reconciliation of all chunks in a single API call
- **Multiple Audio Format Support**: MP3, WAV, M4A, AAC, OGG, FLAC
- **Language Preservation**: Maintains the original audio language
- **Timestamped Logging**: Detailed progress logs with timestamps on stderr
- **Single Binary**: Compiled Go binary with no dependencies

## Prerequisites

- Go 1.21 or higher
- Google Cloud Project with Vertex AI API enabled
- GCP authentication configured (via gcloud or service account)
- `ffmpeg` installed on your system (for audio processing)

## Installation

### Install ffmpeg

```bash
# macOS
brew install ffmpeg

# Ubuntu/Debian
sudo apt-get install ffmpeg

# Windows
# Download from https://ffmpeg.org/download.html
```

### Build the Binary

```bash
# Clone or navigate to the project directory
cd speech-to-text

# Build using Makefile (recommended)
make build
```

This will:
1. Download all Go dependencies
2. Compile the code
3. Create the `speech-to-text` binary in the current directory

### Install to System Path

```bash
# Install to /usr/local/bin
make install

# Or install to custom directory
TARGET=/opt/bin make install
```

### Uninstall from System

```bash
make uninstall
```

## Configuration

The tool supports two backends: **Vertex AI** (default) and **Gemini API**.

### Option 1: Vertex AI Backend (Default)

#### 1. Enable Vertex AI API

```bash
gcloud services enable aiplatform.googleapis.com --project=your-project-id
```

#### 2. Authentication

```bash
# Standard authentication
gcloud auth application-default login

# Or for ADM account (sensitive operations)
gcloud auth application-default login --impersonate-service-account=your-adm-account@domain.com
```

#### 3. Environment Configuration

Create a `.env` file:

```bash
cp .env.example .env
```

Edit `.env`:

```bash
# Use Vertex AI (default if not set)
GEMINI_USE_VERTEX_AI=true

# Project ID (optional if using gcloud config)
# Will auto-detect from: gcloud config get-value core/project
GCP_PROJECT=your-gcp-project-id

# Optional: GCP region (default: global)
GCP_LOCATION=global
```

**Note**: If `GCP_PROJECT` is not set via environment variable or `--project` flag, it will automatically use the project from your gcloud configuration.

### Option 2: Gemini API Backend

#### 1. Get API Key

Get your API key from [Google AI Studio](https://makersuite.google.com/app/apikey)

#### 2. Environment Configuration

Create a `.env` file:

```bash
cp .env.example .env
```

Edit `.env`:

```bash
# Use Gemini API instead of Vertex AI
GEMINI_USE_VERTEX_AI=false

# Required: Your Gemini API key
GEMINI_API_KEY=your-api-key-here
```

## Usage

### Basic Usage

```bash
# Transcribe and display in terminal
bin/speech-to-text path/to/audio/file.mp3

# Save meeting minutes to markdown file
bin/speech-to-text audio.mp3 -o minutes.md

# Specify custom meeting name
bin/speech-to-text audio.mp3 -o minutes.md -m "Weekly Team Sync"
```

### Advanced Usage

```bash
# Use different Gemini model
bin/speech-to-text audio.mp3 --model gemini-2.0-flash-exp

# Specify project explicitly
bin/speech-to-text audio.mp3 --project my-gcp-project

# Custom meeting name with output file
bin/speech-to-text recording.ogg -o meeting.md -m "Q4 Planning Session"

# Different GCP location
bin/speech-to-text audio.mp3 --location us-central1
```

### Command-Line Options

```
<audio-file>          Path to the audio file to transcribe (required, positional argument)
-o string             Output file path in markdown format (optional)
-m string             Custom meeting name for the title (optional, defaults to filename)
--project string      GCP project ID (overrides GCP_PROJECT env var)
--location string     GCP location (default: "global")
--model string        Gemini model to use (default: "gemini-2.5-flash")
```

## Output Format

The transcription is formatted as structured markdown meeting minutes:

```markdown
# Meeting Name

## Attendees
- Sebastien Morand
- John Doe
- Jane Smith

## Minutes
- **Sebastien Morand**: Welcome everyone, let's start with the quarterly review...
- **John Doe**: Thank you. I'd like to discuss the budget allocation...
- **Jane Smith**: I agree with John's points. Additionally, we should consider...
```

## How It Works

### Audio Processing

1. **Duration Check**: Determines if audio is longer than 30 minutes
2. **Chunking** (if needed):
   - Splits audio into 30-minute chunks using ffmpeg
   - Includes 30-second overlaps between chunks
   - Processes chunks in parallel (max 3 concurrent workers)

### Transcription

1. **Name Detection**: Gemini identifies attendee names from the conversation
2. **Speaker Attribution**: Associates each statement with the correct speaker
3. **Verbatim Transcription**: Captures exact words without summarization
4. **Markdown Formatting**: Structures output as meeting minutes

### Intelligent Merging

1. **One-Pass Merge**: All chunks sent to AI in a single API call
2. **Overlap Detection**: AI identifies the 30-second overlap between consecutive chunks
3. **Duplicate Removal**: Seamlessly removes repeated content
4. **Attendee Consolidation**: Merges attendee lists without duplicates
5. **Continuity Preservation**: Maintains natural conversation flow

## Logging

All progress logs are written to stderr with timestamps in format `[YYYY-MM-DD HH:MM:SS]`:

```
[2025-12-10 14:23:45] Analyzing recording: filename.ogg
[2025-12-10 14:23:45] File size: 22.45 MB
[2025-12-10 14:23:45] Recording duration: 35.23 minutes (2114.0 seconds)
[2025-12-10 14:23:45] Recording exceeds 30 minutes - will split into 2 chunks
[2025-12-10 14:23:45] Cutting recording into 2 chunks with 30-second overlaps
[2025-12-10 14:23:46] Creating chunk 1/2 - start: 0.0s, duration: 1830.0s
[2025-12-10 14:23:47] Creating chunk 2/2 - start: 1770.0s, duration: 1830.0s
[2025-12-10 14:23:47] Successfully created 2 chunk files
[2025-12-10 14:23:47] Starting parallel transcription of 2 chunks (max 3 concurrent)
[2025-12-10 14:25:30] Transcription of chunk 1/2 completed
[2025-12-10 14:25:42] Transcription of chunk 2/2 completed
[2025-12-10 14:25:42] Starting reconciliation of overlapping chunks
[2025-12-10 14:25:42] Merging all 2 chunks in one pass
[2025-12-10 14:25:42] Sending all chunks to AI for merge reconciliation
[2025-12-10 14:26:15] All chunks merged successfully
[2025-12-10 14:26:15] Reconciliation completed successfully
[2025-12-10 14:26:15] Finalizing output with meeting title
[2025-12-10 14:26:15] Writing final output to: minutes.md
[2025-12-10 14:26:15] Successfully wrote 15.67 KB to minutes.md
```

## Project Structure

```
speech-to-text-go/
├── src/
│   └── main.go         # Main application code
├── go.mod              # Go module dependencies
├── go.sum              # Dependency checksums
├── build.sh            # Build script (executable)
├── speech-to-text      # Compiled binary (after build)
├── README.md           # This file
├── CLAUDE.md           # AI interaction guidelines
└── .env                # Environment configuration (not in git)
```

## Development

### Building from Source

```bash
# Show all available make targets
make help

# Using Makefile (recommended) - only rebuilds when sources change
make build

# Or using build script
./build.sh

# Or manually
go build -o speech-to-text ./src

# Download/update dependencies
make deps

# Install to system path
make install

# Uninstall from system
make uninstall

# Clean build artifacts
make clean
```

### Running Tests

```bash
# Run tests (when implemented)
go test ./...
```

## Supported Audio Formats

- MP3 (`.mp3`)
- WAV (`.wav`)
- M4A (`.m4a`)
- AAC (`.aac`)
- OGG (`.ogg`)
- FLAC (`.flac`)

## Limitations

- Audio chunking happens at fixed 30-minute intervals (may split mid-sentence)
- Requires active internet connection
- Vertex AI costs apply based on model usage
- Large files may take several minutes to process
- Rate limits may apply (429 errors) - wait a few minutes and retry

## Troubleshooting

### Authentication Errors

Re-authenticate with GCP:
```bash
gcloud auth application-default login
```

### Project Not Found

Ensure `GCP_PROJECT` is set correctly in `.env` or passed via `--project` flag.

### API Not Enabled

Enable Vertex AI API:
```bash
gcloud services enable aiplatform.googleapis.com --project=your-project-id
```

### Rate Limit (429) Error

The Gemini API has rate limits. Wait a few minutes before retrying.

### ffmpeg Not Found

Install ffmpeg:
```bash
# macOS
brew install ffmpeg

# Ubuntu/Debian
sudo apt-get install ffmpeg
```

### Build Errors

Make sure you have Go 1.21+ installed:
```bash
go version
```

Update dependencies:
```bash
go mod tidy
```

## Author

**Sebastien MORAND**
Email: sebastien.morand@loreal.com
Role: CTO Data & AI at L'Oréal

## License

Personal project.

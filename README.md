# Speech-to-Text Transcription Tool

A Go-based audio transcription tool using Google Gemini for transcription and meeting minutes generation.

## Features

- **Gemini Audio Processing**: Direct audio transcription with speaker identification
- **Meeting Minutes**: Automatic formatting as structured markdown
- **Custom Instructions**: Add context and instructions via CLI
- **Multiple Audio Formats**: MP3, WAV, M4A, AAC, OGG, FLAC
- **Single Binary**: Compiled Go binary with no dependencies

## Prerequisites

- Go 1.24 or higher
- Google Gemini API key (or Vertex AI access)
- Docker (for containerized deployment)

## Installation

### Build the Binary

```bash
cd speech-to-text
make build
```

## Configuration

Create a `.env` file:

```bash
# Gemini API (Required)
GEMINI_API_KEY=your-gemini-api-key

# Or use Vertex AI instead of Gemini API
GEMINI_USE_VERTEX_AI=true
GCP_PROJECT=your-gcp-project-id
GCP_LOCATION=global
```

## Usage

### Basic Usage

```bash
# Transcribe audio to meeting minutes
bin/speech-to-text audio.mp3 -o minutes.md
```

### With Context and Instructions

```bash
# Add context about participants for better speaker identification
bin/speech-to-text audio.mp3 -o minutes.md \
  --context "Weekly standup with Alice, Bob, and Charlie from the Dev team"

# Custom processing instructions
bin/speech-to-text audio.mp3 -o minutes.md \
  -i "Focus only on action items and decisions. Ignore small talk."

# Combined
bin/speech-to-text audio.mp3 -o minutes.md \
  -m "Q4 Planning" \
  --context "Quarterly planning session with Product and Engineering" \
  -i "Extract key decisions and assigned owners"
```

### Command-Line Options

```
Positional Arguments:
  <audio-file>           Path to the audio file to transcribe (required)

Gemini Processing:
  --gemini-api-key string  Gemini API key (env: GEMINI_API_KEY)
  --project string         GCP project ID for Vertex AI (env: GCP_PROJECT)
  --location string        GCP location for Vertex AI (default: "global")
  --model string           Gemini model name (default: "gemini-3-pro-preview")

Processing Options:
  -m string              Meeting name for the title
  --context string       Additional context (participants, meeting type, etc.)
  -i string              Custom instructions for processing

Output Options:
  -o string              Output file path (markdown format)

API Options:
  --timeout int          API request timeout in seconds (default: 600)
```

## Output Format

### Meeting Minutes

```markdown
# Weekly Team Sync

## Attendees
- Alice Smith
- Bob Johnson
- Charlie Brown

## Minutes
- **Alice Smith**: Welcome everyone, let's start with the sprint review...
- **Bob Johnson**: The API integration is complete. We hit some issues with...
- **Charlie Brown**: I can help with the testing next week...
```

## How It Works

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

## MCP Server

The speech-to-text tool also includes an MCP (Model Context Protocol) server for remote audio transcription, enabling AI assistants like Claude to transcribe audio files.

### Available MCP Tools

#### `ping`
Test connectivity with the MCP server.

#### `transcribe_audio`
Transcribe audio recording to structured meeting minutes in markdown format.

**Input Parameters:**
| Parameter | Required | Description |
|-----------|----------|-------------|
| `audioData` | Yes | Base64-encoded audio file content |
| `audioFormat` | Yes | MIME type of the audio (e.g., `audio/mp3`, `audio/wav`, `audio/m4a`) |
| `meetingName` | No | Optional meeting title or name |
| `context` | No | Optional additional context (participants, meeting type, etc.) |
| `instructions` | No | Optional custom instructions for transcription processing |

**Output:**
- `minutes`: Formatted meeting minutes in markdown format

### Authentication

The MCP server uses OAuth2 with PKCE support (RFC 2.1) for authentication:
- RFC 9728 Protected Resource Metadata
- RFC 8414 Authorization Server Metadata
- RFC 7591 Dynamic Client Registration
- Bearer token authentication for MCP requests

## Project Structure

```
speech-to-text/
├── cmd/
│   └── speech-to-text/
│       └── main.go          # CLI entry point
├── internal/
│   ├── audio/
│   │   └── processor.go     # Audio utilities (MIME types)
│   ├── mcp/
│   │   ├── server.go        # MCP server + transcribe_audio tool
│   │   ├── server_test.go   # MCP server tests
│   │   └── oauth2.go        # OAuth2 authorization server
│   └── processor/
│       └── processor.go     # Gemini processing
├── pkg/
│   └── auth/
│       ├── auth.go          # API key injection via context
│       └── auth_test.go     # Auth package tests
├── init/                    # Terraform init (one-time setup)
│   ├── provider.tf          # Google provider configuration
│   ├── local.tf             # Load config.yaml
│   ├── state-backend.tf     # GCS bucket for tfstate
│   ├── service-accounts.tf  # Cloud Run service account
│   └── services.tf          # Enable required GCP APIs
├── iac/                     # Terraform application infrastructure
│   ├── provider.tf.template # Provider template with BACKEND_PLACEHOLDER
│   ├── local.tf             # Load config.yaml
│   ├── workload-mcp.tf      # Cloud Run and Artifact Registry
│   └── secrets.tf           # Secret Manager secrets
├── bin/                     # Compiled binaries
├── Dockerfile               # Multi-stage Docker build for Cloud Run
├── config.yaml              # Infrastructure configuration
├── go.mod                   # Go module definition
├── Makefile                 # Build automation
├── README.md                # This file
├── CLAUDE.md                # AI interaction guidelines
└── .env                     # Local configuration (gitignored)
```

## Docker Deployment

### Build and Run Locally

```bash
# Build Docker image (~20MB)
make docker-build

# Run locally for testing
docker run -p 8080:8080 \
  -e GEMINI_API_KEY=your-api-key \
  speech-to-text-mcp:latest

# Test health endpoint
curl http://localhost:8080/health
```

### Deploy to Cloud Run

```bash
# Build and push to Artifact Registry
make docker-push

# Deploy to Cloud Run
make cloud-run-deploy
```

### Dockerfile Features

- **Multi-stage build**: Builds in Go 1.24-alpine, runs in distroless
- **Small image size**: ~20MB final image
- **Non-root user**: Runs as `nonroot` user for security
- **Static binary**: CGO disabled, fully static linking
- **Health check**: `/health` endpoint for Cloud Run probes

## Infrastructure

The MCP server is designed to be deployed on Google Cloud Run. Infrastructure is managed with Terraform.

### Configuration

Infrastructure configuration is defined in `config.yaml`:

```yaml
prefix: scmstt                              # Resource naming prefix
gcp:
  project_id: your-project-id               # GCP project ID
  location: europe-west1                    # GCP region
  services:                                 # APIs to enable
    - run.googleapis.com
    - secretmanager.googleapis.com
    - artifactregistry.googleapis.com
```

### Terraform Init (One-Time Setup)

The `init/` directory creates foundational resources:

```bash
cd init
terraform init
terraform plan
terraform apply
```

This creates:
- GCS bucket for Terraform state
- Cloud Run service account with Secret Manager access
- Enables required GCP APIs

### Terraform IAC (Application Infrastructure)

The `iac/` directory creates application infrastructure:

```bash
# Generate provider.tf from template (after init-deploy)
make update-backend

cd iac
terraform init
terraform plan
terraform apply
```

This creates:
- Artifact Registry repository for Docker images
- Cloud Run service with configured resources:
  - CPU: 1, Memory: 512Mi
  - Min instances: 0, Max instances: 3
  - Max concurrent requests: 2
- Secret Manager secrets (with placeholders - update manually):
  - OAuth credentials
  - Gemini API key

## API Key Management

The `pkg/auth` package provides flexible API key management with context injection:

### Priority Order

1. **Context injection** - Use `auth.WithAPIKey(ctx, key)` for per-request keys
2. **Environment variable** - `GEMINI_API_KEY`
3. **Credential file** - `~/.credentials/google_claude_np_api_key`
4. **Secret Manager** - For Cloud Run deployments (with caching)

### Usage

```go
import "speech-to-text/pkg/auth"

// Option 1: Let it auto-detect from env/file
key, err := auth.GetAPIKey(ctx)

// Option 2: Inject key for a specific request
ctx = auth.WithAPIKey(ctx, "your-api-key")
key, err := auth.GetAPIKey(ctx)

// Option 3: With Secret Manager support (for Cloud Run)
cfg := &auth.SecretManagerConfig{
    ProjectID:  "your-project",
    SecretName: "gemini-api-key",
}
key, err := auth.GetAPIKeyWithSecretManager(ctx, cfg)
```

## Logging

All progress logs are written to stderr with timestamps:

```
[2025-01-10 14:23:45] === Processing: Transcription + Analysis (Gemini) ===
[2025-01-10 14:23:45] Using Gemini API for processing (model: gemini-3-pro-preview)
[2025-01-10 14:23:45] Processing audio with Gemini
[2025-01-10 14:23:45] Sending audio (22.45 MB) to gemini-3-pro-preview...
[2025-01-10 14:25:00] Processing completed
[2025-01-10 14:25:00] Writing Meeting minutes to: minutes.md
[2025-01-10 14:25:00] Successfully wrote 15.67 KB to minutes.md
✓ Meeting minutes saved to: minutes.md
```

## Supported Audio Formats

- MP3 (`.mp3`)
- WAV (`.wav`)
- M4A (`.m4a`)
- AAC (`.aac`)
- OGG (`.ogg`)
- FLAC (`.flac`)

## Troubleshooting

### Gemini API Errors

Get your API key from [Google AI Studio](https://makersuite.google.com/app/apikey)

### Missing Speaker Names

Add context about participants to help with speaker identification:
```bash
bin/speech-to-text meeting.mp3 -o minutes.md \
  --context "Meeting between John (deep voice) and Sarah (higher pitch)"
```

## Development

```bash
make help         # Show all available make targets
make build        # Build for current platform
make build-all    # Build for all platforms
make docker-build # Build Docker image
make fmt          # Format code
make check        # Run checks
make clean        # Clean build artifacts
```

## Author

**Sebastien MORAND**
Email: sebastien.morand@loreal.com

## License

Personal project.

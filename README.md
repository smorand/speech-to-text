# Speech-to-Text Transcription Tool

A Go-based audio transcription tool using Google Gemini for transcription and meeting minutes generation.

## Features

- **Gemini Audio Processing**: Uses Files API + Streaming for reliable transcription
- **OpenRouter Support**: Route through OpenRouter for access to any compatible model
- **Default Model**: `gemini-3-pro-preview` with 20-minute timeout
- **Meeting Minutes**: Automatic formatting as structured markdown
- **Custom Instructions**: Add context and instructions via CLI
- **Multiple Audio Formats**: MP3, WAV, M4A, AAC, OGG, FLAC
- **Single Binary**: Compiled Go binary with no dependencies
- **VPN Detection**: Warns if connection issues may be VPN-related

## Prerequisites

- Go 1.24 or higher
- Google Gemini API key, Vertex AI access, or OpenRouter API key
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
# Option 1: Gemini API (direct)
GEMINI_API_KEY=your-gemini-api-key

# Option 2: Vertex AI
GEMINI_USE_VERTEX_AI=true
GCP_PROJECT=your-gcp-project-id
GCP_LOCATION=global

# Option 3: OpenRouter (use with --model openrouter/<model>)
OPENROUTER_API_KEY=your-openrouter-api-key
```

## Usage

### Basic Usage

```bash
# Transcribe audio to meeting minutes
bin/speech-to-text audio.mp3 -o minutes.md
```

### Using OpenRouter

```bash
# Use any model available on OpenRouter by prefixing with openrouter/
bin/speech-to-text audio.mp3 -o minutes.md \
  --model openrouter/google/gemini-3-pro-preview

# With a different provider/model
bin/speech-to-text audio.mp3 -o minutes.md \
  --model openrouter/openai/gpt-4o-audio-preview
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

Model & Backend:
  --model string             Model name (default: "gemini-3-pro-preview")
                             Use openrouter/<model> for OpenRouter (e.g. openrouter/google/gemini-3-pro-preview)
  --gemini-api-key string    Gemini API key (env: GEMINI_API_KEY)
  --openrouter-api-key string OpenRouter API key (env: OPENROUTER_API_KEY)
  --project string           GCP project ID for Vertex AI (env: GCP_PROJECT)
  --location string          GCP location for Vertex AI (default: "global")

Processing Options:
  -m string              Meeting name for the title
  --context string       Additional context (participants, meeting type, etc.)
  -i string              Custom instructions for processing

Output Options:
  -o string              Output file path (default: <audio-file>_transcription.md)
  -f                     Force transcription even if output file is newer than audio

API Options:
  --timeout int          API request timeout in seconds (default: 1200 / 20 minutes)
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
│  Audio File      │────▶│  Files API Upload (4s)         │
│  (MP3/WAV/etc.)  │     │  - Uploads audio to Gemini     │
└──────────────────┘     │  - Returns file URI            │
                         └──────────────┬─────────────────┘
                                        │
                                        ▼
                         ┌────────────────────────────────┐
                         │  Streaming Transcription       │
                         │  - streamGenerateContent API   │
                         │  - Speaker identification      │
                         │  - Meeting minutes formatting  │
                         └──────────────┬─────────────────┘
                                        │
                                        ▼
                         ┌────────────────────────────────┐
                         │  Output: Structured Minutes    │
                         │  Auto cleanup uploaded file    │
                         └────────────────────────────────┘
```

## MCP Server

The speech-to-text tool includes an MCP (Model Context Protocol) server for remote audio transcription, enabling AI assistants like Claude to transcribe audio files via a secure API.

### MCP Server Overview

The MCP server provides a remote API for audio transcription, deployed on Google Cloud Run with OAuth2 authentication.

```
┌────────────────┐     ┌──────────────────┐     ┌─────────────────────┐
│  MCP Client    │────▶│  OAuth2 Server   │────▶│  MCP Server         │
│  (Claude Code) │     │  - RFC 9728/8414 │     │  - ping tool        │
└────────────────┘     │  - PKCE support  │     │  - transcribe_audio │
                       └──────────────────┘     └──────────┬──────────┘
                                                           │
                                                           ▼
                                                  ┌────────────────┐
                                                  │ Gemini API     │
                                                  └────────────────┘
```

### Available MCP Tools

#### `ping`
Test connectivity with the MCP server.

**Input:** None

**Output:**
```json
{
  "message": "pong",
  "time": "2025-01-16T10:30:00Z"
}
```

#### `transcribe_audio`
Transcribe audio recording to structured meeting minutes in markdown format. Identifies speakers, preserves original language, and formats as attendees list followed by verbatim speaker-attributed transcript.

**Input Parameters:**
| Parameter | Required | Description |
|-----------|----------|-------------|
| `audioData` | Yes | Base64-encoded audio file content |
| `audioFormat` | Yes | MIME type of the audio (e.g., `audio/mp3`, `audio/wav`, `audio/m4a`) |
| `meetingName` | No | Optional meeting title or name |
| `context` | No | Optional additional context (participants, meeting type, etc.) |
| `instructions` | No | Optional custom instructions for transcription processing |

**Supported Audio Formats:**
- `audio/mpeg`, `audio/mp3` (MP3)
- `audio/wav`, `audio/wave`, `audio/x-wav` (WAV)
- `audio/m4a`, `audio/x-m4a`, `audio/mp4` (M4A)
- `audio/aac` (AAC)
- `audio/ogg` (OGG)
- `audio/flac`, `audio/x-flac` (FLAC)
- `audio/webm`, `video/webm` (WebM)
- `audio/aiff`, `audio/x-aiff` (AIFF)

**Output:**
```json
{
  "minutes": "# Meeting Title\n\n## Attendees\n- Speaker 1\n- Speaker 2\n\n## Minutes\n..."
}
```

### Authentication Flow

The MCP server implements OAuth 2.1 with PKCE support for secure authentication:

```
1. Discovery:       GET /.well-known/oauth-protected-resource
                         ↓
2. Metadata:        GET /.well-known/oauth-authorization-server
                         ↓
3. Registration:    POST /oauth/register (Dynamic Client Registration)
                         ↓
4. Authorization:   GET /oauth/authorize?client_id=xxx&redirect_uri=xxx
                         &response_type=code&code_challenge=xxx&code_challenge_method=S256
                         ↓
5. User Approval:   User clicks "Approve" on authorization page
                         ↓
6. Token Exchange:  POST /oauth/token (exchange code for access token)
                         ↓
7. API Access:      Bearer token in Authorization header for MCP requests
```

**Supported Standards:**
- RFC 9728: Protected Resource Metadata
- RFC 8414: Authorization Server Metadata
- RFC 7591: Dynamic Client Registration
- RFC 7636: PKCE (S256 method)
- OAuth 2.1: Modern OAuth best practices

**Endpoints:**
| Endpoint | Method | Description |
|----------|--------|-------------|
| `/.well-known/oauth-protected-resource` | GET | Protected resource metadata |
| `/.well-known/oauth-authorization-server` | GET | Authorization server metadata |
| `/oauth/register` | POST | Dynamic client registration |
| `/oauth/authorize` | GET | Authorization endpoint |
| `/oauth/callback` | GET | Authorization callback |
| `/oauth/token` | POST | Token exchange |
| `/health` | GET | Health check (no auth required) |

### Claude Code Configuration

To use the MCP server with Claude Code, add the following to your Claude Code MCP settings:

**Option 1: Global settings (settings.json)**
```json
{
  "mcpServers": {
    "speech-to-text": {
      "url": "https://speech-to-text-mcp-<HASH>-<REGION>.a.run.app",
      "transport": "sse"
    }
  }
}
```

**Option 2: Project-level (.mcp.json in project root)**
```json
{
  "mcpServers": {
    "speech-to-text": {
      "url": "https://speech-to-text-mcp-<HASH>-<REGION>.a.run.app",
      "transport": "sse"
    }
  }
}
```

Replace `<HASH>` and `<REGION>` with your actual Cloud Run service values (e.g., `abc123def-ew1`).

**Getting the URL:**
```bash
# After deployment, get the Cloud Run URL:
gcloud run services describe speech-to-text-mcp --region=europe-west1 --format='value(status.url)'
```

### MCP Server Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `HOST` | No | `0.0.0.0` | Server host address |
| `PORT` | No | `8080` | Server port (Cloud Run provides this) |
| `BASE_URL` | Yes | - | Public URL of the Cloud Run service |
| `PROJECT_ID` | Yes | - | GCP project ID for Secret Manager |
| `GEMINI_API_KEY_SECRET` | No | `scmstt-gemini-api-key` | Secret Manager secret name for Gemini API key |
| `OAUTH_CREDENTIALS_SECRET` | No | `scmstt-oauth-creds` | Secret Manager secret name for OAuth credentials |

### MCP Server Commands

```bash
# Print deployment instructions
bin/speech-to-text mcp

# Start the MCP server (used by Cloud Run)
bin/speech-to-text serve
```

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
│       ├── processor.go     # Gemini processing
│       └── openrouter.go    # OpenRouter processing
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
[2026-01-26 13:08:52] === Processing: Transcription + Analysis (Gemini) ===
[2026-01-26 13:08:52] Using Gemini API for processing (model: gemini-3-flash-preview)
[2026-01-26 13:08:52] Processing audio with Gemini
[2026-01-26 13:08:52] Audio file size: 18.84 MB
[2026-01-26 13:08:52] Uploading file to Gemini Files API...
[2026-01-26 13:08:56] File uploaded successfully: https://generativelanguage.googleapis.com/v1beta/files/xxx
[2026-01-26 13:08:56] Waiting for file to be processed...
[2026-01-26 13:08:56] File is ready for processing
[2026-01-26 13:08:56] Transcribing with model: gemini-3-flash-preview
[2026-01-26 13:08:56] Sending streaming request...
[2026-01-26 13:10:17] Processing completed
[2026-01-26 13:10:19] Cleaned up uploaded file
[2026-01-26 13:10:19] Writing Meeting minutes to: /path/to/audio_transcription.md
[2026-01-26 13:10:19] Successfully wrote 25.57 KB to /path/to/audio_transcription.md
✓ Meeting minutes saved to: /path/to/audio_transcription.md
```

## Supported Audio Formats

- MP3 (`.mp3`)
- WAV (`.wav`)
- M4A (`.m4a`)
- AAC (`.aac`)
- OGG (`.ogg`)
- FLAC (`.flac`)

## Troubleshooting

### Connection Closed Unexpectedly (EOF)

**Error:** `connection closed unexpectedly (EOF) - Are you sure you don't have an active VPN?`

**Root Cause:** VPNs can interfere with long-running HTTP connections.

**Solutions:**
- Pause or disable your VPN temporarily
- Retry the transcription without VPN active
- Check your VPN timeout settings

**Why This Happens:**
- Files API upload succeeds quickly (~4 seconds)
- Streaming transcription can take 60+ seconds
- Some VPNs have aggressive connection timeouts (60-65 seconds)
- This causes premature connection termination

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
Email: sebastien.morand@*******

## License

Personal project.

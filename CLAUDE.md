# CLAUDE.md - AI Interaction Guidelines

## Project Overview

**Project Name**: speech-to-text-go
**Type**: Personal Go CLI Tool
**Purpose**: Audio transcription using Google Gemini for meeting minutes generation
**Owner**: Sebastien MORAND (sebastien.morand@loreal.com)

## Architecture

### CLI Mode
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

### MCP Server Mode (Cloud Run)
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

## Technology Stack

- **Language**: Go 1.24+
- **Build System**: Go modules (go.mod)
- **Container**: Multi-stage Docker build (distroless), managed via Terraform (kreuzwerker/docker)
- **Main Dependencies**:
  - `google.golang.org/genai` - Gemini SDK for Go
  - `github.com/joho/godotenv` - Environment configuration
  - `github.com/modelcontextprotocol/go-sdk/mcp` - MCP server SDK
  - `cloud.google.com/go/secretmanager` - Secret Manager client
- **External Services**:
  - Google Gemini API or Vertex AI (required)
  - Google Secret Manager (for Cloud Run deployment)

## Project Structure

```
speech-to-text/
├── cmd/
│   └── speech-to-text/
│       └── main.go          # CLI entry point, orchestration
├── internal/
│   ├── audio/
│   │   └── processor.go     # Audio utilities (MIME types)
│   ├── mcp/
│   │   ├── server.go        # MCP server structure
│   │   ├── server_test.go   # MCP server tests
│   │   └── oauth2.go        # OAuth2 authorization server
│   └── processor/
│       └── processor.go     # Gemini processing
├── pkg/
│   └── auth/
│       ├── auth.go          # API key context injection
│       └── auth_test.go     # Auth package tests
├── init/                    # Terraform init (one-time setup)
│   ├── provider.tf          # Google provider configuration
│   ├── local.tf             # Load config.yaml
│   ├── state-backend.tf     # GCS bucket for tfstate
│   ├── service-accounts.tf  # Cloud Run service account
│   └── services.tf          # Enable required GCP APIs
├── iac/                     # Terraform application infrastructure
│   ├── provider.tf.template # Provider template with BACKEND_PLACEHOLDER
│   ├── provider.tf          # Generated provider (gitignored)
│   ├── local.tf             # Load config.yaml
│   ├── docker.tf            # Docker build and push (kreuzwerker/docker)
│   ├── workload-mcp.tf      # Cloud Run and Artifact Registry
│   └── secrets.tf           # Secret Manager secrets
├── bin/                     # Compiled binaries (gitignored)
├── Dockerfile               # Multi-stage Docker build for Cloud Run
├── config.yaml              # Infrastructure configuration
├── go.mod                   # Go module definition
├── go.sum                   # Dependency checksums
├── Makefile                 # Build automation
├── README.md                # User documentation
├── CLAUDE.md                # This file
├── .agent_docs/             # Detailed AI documentation
├── .env                     # Local configuration (gitignored)
└── .env.example             # Example configuration
```

## Key Components

### Entry Point (cmd/speech-to-text/main.go)

- CLI flag parsing
- Configuration validation
- Processor orchestration
- Output handling (file or stdout)
- Subcommands:
  - `serve`: Start MCP HTTP server (for Cloud Run)
  - `mcp`: Print deployment instructions

### Processor Package (internal/processor/processor.go)

- **Purpose**: Gemini audio processing for transcription
- **Default Model**: `gemini-3-pro-preview`
- **Default Timeout**: 1200 seconds (20 minutes)
- **Processing Method**:
  - Uses **Files API** to upload audio (avoids inline data timeout issues)
  - Uses **Streaming API** (`GenerateContentStream`) for transcription
  - Automatically cleans up uploaded files after processing
- **Config Struct**: Gemini API key, model, context, instructions
- **Key Methods**:
  - `New()`: Creates Gemini client (API or Vertex AI)
  - `Process(ctx, audioFile)`: Main entry point, uploads file and streams transcription
  - `buildSystemPrompt()`: Constructs system instructions
  - `buildUserPrompt()`: Adds context and instructions
- **VPN Warning**: Shows helpful message if EOF errors occur (VPN interference)

### Audio Package (internal/audio/processor.go)

- **Functions**:
  - `GetMimeType()`: Maps file extensions to MIME types

### MCP Package (internal/mcp/)

- **server.go**: MCP server structure and tools
  - `Config`: Host, Port, BaseURL, credential configuration
  - `Server`: Wraps MCP server with HTTP and OAuth2
  - `NewServer()`: Creates server with MCP SDK
  - `Run()`: Starts HTTP server with graceful shutdown
  - `RegisterTools()`: Registers MCP tools (ping, transcribe_audio)
  - `authMiddleware()`: Bearer token validation middleware
  - `handleTranscribeAudio()`: Audio transcription handler
  - `mimeTypeToExtension()`: MIME type to file extension mapping

- **MCP Tools**:
  - `ping`: Test connectivity with the MCP server
  - `transcribe_audio`: Transcribe audio to meeting minutes
    - Input: `audioData` (base64), `audioFormat` (MIME type), `meetingName`, `context`, `instructions`
    - Output: `minutes` (markdown formatted meeting minutes)

- **oauth2.go**: OAuth2 authorization server (RFC 2.1)
  - `OAuth2Server`: Handles OAuth2 flow
  - Well-known endpoints (RFC 9728, RFC 8414)
  - Dynamic client registration (RFC 7591)
  - PKCE support (S256)
  - In-memory token store with expiration cleanup

### Auth Package (pkg/auth/auth.go)

- **Purpose**: Gemini API key injection via context
- **Key Functions**:
  - `WithAPIKey(ctx, key)`: Injects API key into context
  - `GetAPIKey(ctx)`: Retrieves key (context → env → file)
  - `GetAPIKeyWithSecretManager(ctx, cfg)`: With Secret Manager support
  - `IsCached()`: Check if key is cached from Secret Manager
  - `ClearCache()`: Clear cached key
- **Priority Order**:
  1. Context injection (per-request)
  2. `GEMINI_API_KEY` environment variable
  3. `~/.credentials/google_claude_np_api_key` file
  4. Secret Manager (Cloud Run, with caching)

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
  -o                 Output file path (default: <audio-file>_transcription.md)
  -f                 Force transcription even if output file is newer than audio

API:
  --timeout          API request timeout in seconds (default: 1200 / 20 minutes)
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
3. **EOF Errors**: Check if VPN is active - VPNs can cause connection timeouts during streaming
   - Error message includes VPN warning: `"Are you sure you don't have an active VPN?"`
   - Solution: Pause VPN temporarily during transcription

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
make docker-build       # Build Docker image locally (for testing)
```

### Testing

```bash
# Basic transcription (output to default file: audio_transcription.md)
bin/speech-to-text audio.mp3

# Specify output file
bin/speech-to-text audio.mp3 -o minutes.md

# With context and model selection
bin/speech-to-text audio.mp3 \
  --context "Meeting with Alice and Bob" \
  -i "Focus on action items" \
  -model gemini-3-flash-preview
```

### Adding New Audio Format

1. Update `GetMimeType()` in `internal/audio/processor.go`
2. Add MIME type mapping
3. Update README.md

## Infrastructure

### Terraform Structure

Infrastructure is managed with Terraform in two directories:

1. **init/**: One-time setup (state bucket, service accounts, APIs)
2. **iac/**: Application infrastructure (Cloud Run, secrets, Artifact Registry)

### init/ Directory

Creates foundational GCP resources:
- **state-backend.tf**: GCS bucket for Terraform state (`{project_id}-tfstate`)
- **service-accounts.tf**: Cloud Run service account with Secret Manager access
- **services.tf**: Enables required GCP APIs:
  - `run.googleapis.com`
  - `secretmanager.googleapis.com`
  - `artifactregistry.googleapis.com`
  - `cloudresourcemanager.googleapis.com`
  - `iam.googleapis.com`

### iac/ Directory

Creates application infrastructure:
- **provider.tf.template**: Provider with BACKEND_PLACEHOLDER (replaced by `make update-backend`)
  - Includes `kreuzwerker/docker` provider for Docker image management
- **local.tf**: Loads config.yaml with Cloud Run and secrets configuration
- **docker.tf**: Docker image build and push (via kreuzwerker/docker provider)
  - `docker_image`: Builds locally with file hash triggers (auto-rebuild on source changes)
  - `docker_registry_image`: Pushes to Artifact Registry with state tracking
- **workload-mcp.tf**: Cloud Run service and Artifact Registry
  - Cloud Run: CPU 1, Memory 512Mi, max 3 instances, 2 concurrent requests
  - Artifact Registry: Docker repository for container images
  - Health check probes configured
  - References Terraform-managed Docker image for proper dependency tracking
- **secrets.tf**: Secret Manager secrets for OAuth and Gemini API key
  - Secrets created with placeholder values (update manually)
  - IAM bindings for Cloud Run service account access

### config.yaml

Single source of truth for infrastructure configuration:
- `prefix`: Resource naming prefix (`scmstt`)
- `gcp.project_id`: GCP project ID
- `gcp.location`: Region (`europe-west1`)
- `gcp.services`: APIs to enable
- `gcp.resources.cloud_run`: Cloud Run configuration
- `gcp.resources.artifact_registry`: Artifact Registry configuration
- `secrets`: Secret names for OAuth and Gemini API key

### Deployment Workflow

**Full deployment (first time):**

```bash
# 1. Initialize foundational resources
make init-plan          # Review state bucket, service accounts, APIs
make init-deploy        # Creates GCS bucket, service account, enables APIs

# 2. Update secrets in Secret Manager (manual step)
# Upload OAuth credentials and Gemini API key to:
# - scmstt-oauth-creds
# - scmstt-gemini-api-key

# 3. Configure Docker authentication (one-time per machine)
make docker-auth        # Configure gcloud for Artifact Registry

# 4. Deploy everything (infrastructure + Docker build + push + Cloud Run)
make plan               # Review all changes (including Docker image)
make deploy             # Creates all resources, builds/pushes Docker image
```

**Subsequent deployments:**

```bash
# Single command deploys everything
# Docker image auto-rebuilds only when source files change
make deploy
```

**Docker build triggers (auto-rebuild):**
- `Dockerfile`
- `go.mod`, `go.sum`
- `cmd/speech-to-text/main.go`
- `internal/processor/processor.go`
- `internal/mcp/server.go`, `internal/mcp/oauth2.go`
- `pkg/auth/auth.go`

### Terraform Commands

```bash
# Initialize terraform in init/
cd init && terraform init

# Validate configuration
terraform validate

# Plan changes
terraform plan

# Apply changes
terraform apply
```

## File References

- CLI orchestration: `cmd/speech-to-text/main.go`
- Gemini processing: `internal/processor/processor.go`
- Audio handling: `internal/audio/processor.go`
- MCP server: `internal/mcp/server.go`
- OAuth2 server: `internal/mcp/oauth2.go`
- MCP tests: `internal/mcp/server_test.go`
- API key auth: `pkg/auth/auth.go`
- Auth tests: `pkg/auth/auth_test.go`
- Init Terraform: `init/*.tf`
- IAC Terraform: `iac/*.tf`
- Docker build: `iac/docker.tf`
- Infrastructure config: `config.yaml`
- Dependencies: `go.mod`
- Build: `Makefile`
- Container: `Dockerfile`

## Quick Reference

### Build
```bash
make build
```

### Run
```bash
# Output to default file (audio_transcription.md)
bin/speech-to-text audio.mp3

# Or specify output file
bin/speech-to-text audio.mp3 -o minutes.md
```

### With Context
```bash
bin/speech-to-text audio.mp3 \
  --context "Weekly standup" \
  -i "Extract action items"
```

## Documentation Index

Detailed documentation is available in `.agent_docs/`:

| File | Description |
|------|-------------|
| [mcp-server.md](.agent_docs/mcp-server.md) | MCP server implementation details, OAuth2 flow, tools specification |

## Notes for AI

- This is a **personal project**
- **Simple architecture**: Audio → Gemini → Meeting minutes
- **Go best practices**: Follow conventions
- **Single binary**: One of Go's main advantages

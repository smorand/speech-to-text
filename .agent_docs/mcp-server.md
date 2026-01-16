# MCP Server Implementation Details

## Overview

The MCP (Model Context Protocol) server allows AI assistants like Claude Code to remotely transcribe audio files. It implements OAuth 2.1 authorization for secure access.

## OAuth2 Flow

```
1. Client → /.well-known/oauth-protected-resource
   Returns resource metadata with authorization server URL

2. Client → /.well-known/oauth-authorization-server
   Returns server metadata (endpoints, supported methods)

3. Client → POST /oauth/register
   Dynamic client registration, returns client_id and client_secret

4. Client → GET /oauth/authorize
   Shows authorization page to user

5. User approves → GET /oauth/callback
   Issues authorization code, redirects to client

6. Client → POST /oauth/token
   Exchanges code for access token (with PKCE validation)

7. Client → MCP requests with Bearer token
   Token validated by authMiddleware
```

## Key Components

### server.go

- `Config`: Server configuration
  - `Host`, `Port`: HTTP server binding
  - `BaseURL`: External URL for OAuth redirects
  - `SecretProject`, `SecretName`: Secret Manager config
  - `CredentialFile`: Local credential fallback

- `Server`: Main server struct
  - `mcpServer`: MCP SDK server instance
  - `httpServer`: HTTP server
  - `oauth2Server`: OAuth2 authorization server

- `authMiddleware()`: Bearer token validation
  - Extracts token from Authorization header
  - Validates against token store
  - Returns 401 with WWW-Authenticate header on failure

### oauth2.go

- `OAuth2Server`: Authorization server
  - `clients`: Registered clients (auto-registration supported)
  - `states`: Authorization request states (10 min TTL)
  - `codes`: Authorization codes (10 min TTL, single use)
  - `tokens`: Access tokens (1 hour TTL)

- Cleanup goroutines run every 1-5 minutes to remove expired entries

## Security Features

- PKCE (S256) support for authorization code flow
- Bearer tokens with 1 hour expiration
- Single-use authorization codes
- State parameter to prevent CSRF

## MCP Tools

### ping

Test connectivity with the MCP server.
- Input: none
- Output: `{ "message": "pong", "time": "<RFC3339 timestamp>" }`

### transcribe_audio

Transcribe audio recording to structured meeting minutes.

**Input schema:**
- `audioData` (required): Base64-encoded audio file content
- `audioFormat` (required): MIME type (e.g., audio/mp3, audio/wav, audio/m4a)
- `meetingName` (optional): Meeting title
- `context` (optional): Additional context (participants, meeting type)
- `instructions` (optional): Custom transcription instructions

**Output:**
- `minutes`: Markdown-formatted meeting minutes with speaker attribution

**Processing flow:**
1. Decode base64 audio data
2. Save to temporary file with correct extension
3. Get Gemini API key (from context, env, or Secret Manager)
4. Process with Gemini processor
5. Clean up temporary file
6. Return markdown result

## Testing

Run tests with:
```bash
go test -v ./internal/mcp/...
```

Test coverage:
- Well-known endpoints return valid metadata
- Dynamic client registration works
- Authorization flow redirects with code
- Token exchange returns valid access token
- MCP endpoint rejects without Bearer token
- PKCE validation (S256)

## Supported Audio Formats

MIME types mapped to file extensions:
- `audio/mpeg`, `audio/mp3` → `.mp3`
- `audio/wav`, `audio/wave`, `audio/x-wav` → `.wav`
- `audio/m4a`, `audio/x-m4a`, `audio/mp4` → `.m4a`
- `audio/aac` → `.aac`
- `audio/ogg` → `.ogg`
- `audio/webm` → `.webm`
- `audio/flac`, `audio/x-flac` → `.flac`
- `audio/aiff`, `audio/x-aiff` → `.aiff`
- `audio/3gpp` → `.3gp`
- `audio/3gpp2` → `.3g2`
- `video/mp4` → `.mp4`
- `video/webm` → `.webm`

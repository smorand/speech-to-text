# Docker build and push for MCP Server
# Uses kreuzwerker/docker provider for proper Terraform state management

locals {
  # Docker image configuration
  docker_image_name = local.cloud_run_config.name
  docker_image_tag  = "latest"
  docker_image_full = "${local.location}-docker.pkg.dev/${local.project_id}/${google_artifact_registry_repository.docker.repository_id}/${local.docker_image_name}:${local.docker_image_tag}"
}

# Build Docker image locally
resource "docker_image" "mcp_server" {
  name = local.docker_image_full

  build {
    context    = "${path.root}/.."
    dockerfile = "Dockerfile"

    label = {
      "org.opencontainers.image.source" = "https://github.com/smorand/speech-to-text"
      "environment"                     = local.env
      "managed_by"                      = "terraform"
    }
  }

  # Triggers rebuild when source files change
  triggers = {
    dockerfile_hash = filesha256("${path.root}/../Dockerfile")
    go_mod_hash     = filesha256("${path.root}/../go.mod")
    go_sum_hash     = filesha256("${path.root}/../go.sum")
    main_hash       = filesha256("${path.root}/../cmd/speech-to-text/main.go")
    processor_hash  = filesha256("${path.root}/../internal/processor/processor.go")
    mcp_server_hash = filesha256("${path.root}/../internal/mcp/server.go")
    mcp_oauth_hash  = filesha256("${path.root}/../internal/mcp/oauth2.go")
    auth_hash       = filesha256("${path.root}/../pkg/auth/auth.go")
  }

  depends_on = [google_artifact_registry_repository.docker]
}

# Push image to Artifact Registry
resource "docker_registry_image" "mcp_server" {
  name = docker_image.mcp_server.name

  # Keep old images during updates
  keep_remotely = true

  # Trigger push when local image changes
  triggers = {
    image_id = docker_image.mcp_server.image_id
  }

  depends_on = [google_artifact_registry_repository.docker]
}

# Outputs
output "docker_image_url" {
  description = "Full Docker image URL"
  value       = docker_registry_image.mcp_server.name
}

output "docker_image_digest" {
  description = "Docker image SHA256 digest"
  value       = docker_registry_image.mcp_server.sha256_digest
}

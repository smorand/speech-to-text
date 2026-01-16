# Cloud Run and Artifact Registry for MCP Server

# Artifact Registry repository for Docker images
resource "google_artifact_registry_repository" "docker" {
  repository_id = local.artifact_registry_config.name
  location      = local.location
  format        = local.artifact_registry_config.format
  description   = "Docker repository for Speech-to-Text MCP Server"

  labels = local.common_labels
}

# Cloud Run service for MCP Server
resource "google_cloud_run_v2_service" "mcp_server" {
  name     = local.cloud_run_config.name
  location = local.cloud_run_config.region

  template {
    service_account = local.cloudrun_sa_email

    scaling {
      min_instance_count = local.cloud_run_config.min_instances
      max_instance_count = local.cloud_run_config.max_instances
    }

    max_instance_request_concurrency = local.cloud_run_config.max_concurrent_requests

    containers {
      image = "${local.location}-docker.pkg.dev/${local.project_id}/${google_artifact_registry_repository.docker.repository_id}/${local.cloud_run_config.name}:latest"

      resources {
        limits = {
          cpu    = local.cloud_run_config.cpu
          memory = local.cloud_run_config.memory
        }
        cpu_idle = true
      }

      # Application environment variables
      env {
        name  = "HOST"
        value = "0.0.0.0"
      }

      env {
        name  = "PROJECT_ID"
        value = local.project_id
      }

      env {
        name  = "ENVIRONMENT"
        value = local.env
      }

      # Secret reference for Gemini API key
      env {
        name = "GEMINI_API_KEY_SECRET"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.gemini_api_key.secret_id
            version = "latest"
          }
        }
      }

      # Secret reference for OAuth credentials
      env {
        name = "OAUTH_CREDENTIALS_SECRET"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.oauth_credentials.secret_id
            version = "latest"
          }
        }
      }

      # Health check probe
      startup_probe {
        http_get {
          path = "/health"
          port = 8080
        }
        initial_delay_seconds = 5
        period_seconds        = 10
        failure_threshold     = 3
      }

      liveness_probe {
        http_get {
          path = "/health"
          port = 8080
        }
        period_seconds    = 30
        failure_threshold = 3
      }
    }
  }

  # Allow traffic to the service
  traffic {
    type    = "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST"
    percent = 100
  }

  labels = local.common_labels

  depends_on = [
    google_artifact_registry_repository.docker,
    google_secret_manager_secret_version.gemini_api_key,
    google_secret_manager_secret_version.oauth_credentials
  ]
}

# IAM policy for unauthenticated access (OAuth2 handles auth)
resource "google_cloud_run_v2_service_iam_member" "public_access" {
  count = local.cloud_run_config.allow_unauthenticated ? 1 : 0

  project  = local.project_id
  location = google_cloud_run_v2_service.mcp_server.location
  name     = google_cloud_run_v2_service.mcp_server.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

# Outputs
output "cloud_run_url" {
  description = "Cloud Run service URL"
  value       = google_cloud_run_v2_service.mcp_server.uri
}

output "artifact_registry_url" {
  description = "Artifact Registry repository URL"
  value       = "${local.location}-docker.pkg.dev/${local.project_id}/${google_artifact_registry_repository.docker.repository_id}"
}

output "docker_image" {
  description = "Full Docker image path"
  value       = "${local.location}-docker.pkg.dev/${local.project_id}/${google_artifact_registry_repository.docker.repository_id}/${local.cloud_run_config.name}:latest"
}

# Secret Manager secrets for MCP Server

# OAuth credentials secret
resource "google_secret_manager_secret" "oauth_credentials" {
  secret_id = local.secrets_config.oauth_credentials

  replication {
    user_managed {
      replicas {
        location = local.location
      }
    }
  }

  labels = local.common_labels
}

# OAuth credentials secret version (placeholder - actual value set manually)
resource "google_secret_manager_secret_version" "oauth_credentials" {
  secret      = google_secret_manager_secret.oauth_credentials.id
  secret_data = "PLACEHOLDER_OAUTH_CREDENTIALS"

  lifecycle {
    ignore_changes = [secret_data]
  }
}

# Gemini API key secret
resource "google_secret_manager_secret" "gemini_api_key" {
  secret_id = local.secrets_config.gemini_api_key

  replication {
    user_managed {
      replicas {
        location = local.location
      }
    }
  }

  labels = local.common_labels
}

# Gemini API key secret version (placeholder - actual value set manually)
resource "google_secret_manager_secret_version" "gemini_api_key" {
  secret      = google_secret_manager_secret.gemini_api_key.id
  secret_data = "PLACEHOLDER_GEMINI_API_KEY"

  lifecycle {
    ignore_changes = [secret_data]
  }
}

# Grant Cloud Run service account access to secrets
resource "google_secret_manager_secret_iam_member" "oauth_access" {
  secret_id = google_secret_manager_secret.oauth_credentials.id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${local.cloudrun_sa_email}"
}

resource "google_secret_manager_secret_iam_member" "gemini_access" {
  secret_id = google_secret_manager_secret.gemini_api_key.id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${local.cloudrun_sa_email}"
}

# Outputs
output "oauth_secret_id" {
  description = "OAuth credentials secret ID"
  value       = google_secret_manager_secret.oauth_credentials.secret_id
}

output "gemini_api_key_secret_id" {
  description = "Gemini API key secret ID"
  value       = google_secret_manager_secret.gemini_api_key.secret_id
}

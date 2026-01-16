# Service accounts for speech-to-text MCP server

locals {
  service_accounts = {
    cloudrun = "${local.prefix}-cloudrun-${local.env}"
  }
}

# Cloud Run service account
resource "google_service_account" "cloudrun" {
  account_id   = local.service_accounts.cloudrun
  display_name = "Cloud Run Service Account for Speech-to-Text MCP"
  description  = "Custom service account for Cloud Run services (never use default)"
}

# Cloud Run IAM permissions - basic viewer role
resource "google_project_iam_member" "cloudrun_viewer" {
  project = local.project_id
  role    = "roles/viewer"
  member  = "serviceAccount:${google_service_account.cloudrun.email}"
}

# Secret Manager Secret Accessor - required for reading API keys
resource "google_project_iam_member" "cloudrun_secret_accessor" {
  project = local.project_id
  role    = "roles/secretmanager.secretAccessor"
  member  = "serviceAccount:${google_service_account.cloudrun.email}"
}

# Outputs
output "service_accounts" {
  description = "Created service account emails"
  value = {
    cloudrun = google_service_account.cloudrun.email
  }
}

locals {
  # Load configuration from config.yaml
  config_file = yamldecode(file("${path.root}/../config.yaml"))
  config      = local.config_file

  # Global fields
  prefix       = local.config.prefix
  env          = local.config.env
  project_name = local.config.project_name

  # GCP configuration
  gcp = lookup(local.config, "gcp", {})

  # GCP Project ID (explicit from config)
  project_id = local.gcp.project_id

  # GCP Location
  location = local.gcp.location

  # Detect if multi-region
  is_multi_region = contains(["us", "eu", "asia"], local.location)

  # Compute location_id for naming
  location_id = local.is_multi_region ? local.location : (
    "${substr(local.location, 0, 1)}${substr(split("-", local.location)[1], 0, 1)}${regex("\\d+", local.location)}"
  )

  # Cloud Run configuration from config.yaml
  cloud_run_config = lookup(local.gcp.resources, "cloud_run", {})

  # Artifact Registry configuration from config.yaml
  artifact_registry_config = lookup(local.gcp.resources, "artifact_registry", {})

  # Secrets configuration from config.yaml
  secrets_config = lookup(local.config, "secrets", {})

  # Service account email (created by init/)
  cloudrun_sa_email = "${local.prefix}-cloudrun-${local.env}@${local.project_id}.iam.gserviceaccount.com"

  # Resource labels
  common_labels = {
    environment = local.env
    project     = local.project_name
    managed_by  = "terraform"
  }
}

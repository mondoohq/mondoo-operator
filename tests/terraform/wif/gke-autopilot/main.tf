resource "random_string" "suffix" {
  length  = 4
  special = false
  upper   = false
}

locals {
  name_prefix = "mondoo-wif-ap-${random_string.suffix.result}"
}

################################################################################
# Management Cluster (Autopilot - Workload Identity enabled by default)
################################################################################

resource "google_container_cluster" "management" {
  name     = "${local.name_prefix}-mgmt"
  project  = var.project_id
  location = var.region

  # Autopilot mode - manages node pools automatically
  enable_autopilot    = true
  deletion_protection = false
}

################################################################################
# Target Cluster (Autopilot)
################################################################################

resource "google_container_cluster" "target" {
  name     = "${local.name_prefix}-target"
  project  = var.project_id
  location = var.region

  enable_autopilot    = true
  deletion_protection = false
}

################################################################################
# Google Service Account for the scanner
################################################################################

resource "google_service_account" "scanner" {
  account_id   = "${local.name_prefix}-scan"
  display_name = "Mondoo WIF Autopilot Scanner (${random_string.suffix.result})"
  project      = var.project_id
}

# Allow the management cluster KSA to impersonate this GSA
resource "google_service_account_iam_binding" "workload_identity" {
  service_account_id = google_service_account.scanner.name
  role               = "roles/iam.workloadIdentityUser"

  members = [
    "serviceAccount:${var.project_id}.svc.id.goog[${var.scanner_namespace}/${var.scanner_service_account}]",
  ]
}

# Grant the GSA permission to view/access the target cluster
resource "google_project_iam_member" "cluster_viewer" {
  project = var.project_id
  role    = "roles/container.clusterViewer"
  member  = "serviceAccount:${google_service_account.scanner.email}"
}

################################################################################
# Kubeconfig for management cluster
################################################################################

data "google_client_config" "default" {}

module "gke_auth" {
  source = "terraform-google-modules/kubernetes-engine/google//modules/auth"

  project_id   = var.project_id
  cluster_name = google_container_cluster.management.name
  location     = var.region
}

resource "local_file" "kubeconfig" {
  content  = module.gke_auth.kubeconfig_raw
  filename = "${path.module}/kubeconfig-mgmt"
}

resource "random_string" "suffix" {
  length  = 4
  special = false
  upper   = false
}

locals {
  name_prefix = "mondoo-wif-${random_string.suffix.result}"
}

################################################################################
# Management Cluster (runs the operator)
################################################################################

resource "google_container_cluster" "management" {
  name               = "${local.name_prefix}-mgmt"
  project            = var.project_id
  location           = var.zone
  min_master_version = var.k8s_version
  deletion_protection = false

  # Workload Identity is required for WIF
  workload_identity_config {
    workload_pool = "${var.project_id}.svc.id.goog"
  }

  remove_default_node_pool = true
  initial_node_count       = 1
}

resource "google_container_node_pool" "management" {
  name     = "${local.name_prefix}-mgmt-pool"
  location = var.zone
  project  = var.project_id
  cluster  = google_container_cluster.management.id

  node_count = 1

  node_config {
    spot         = true
    machine_type = "e2-standard-2"

    # Required for Workload Identity on nodes
    workload_metadata_config {
      mode = "GKE_METADATA"
    }

    oauth_scopes = [
      "https://www.googleapis.com/auth/cloud-platform",
    ]
  }
}

################################################################################
# Target Cluster (to be scanned)
################################################################################

resource "google_container_cluster" "target" {
  name               = "${local.name_prefix}-target"
  project            = var.project_id
  location           = var.zone
  min_master_version = var.k8s_version
  deletion_protection = false

  remove_default_node_pool = true
  initial_node_count       = 1
}

resource "google_container_node_pool" "target" {
  name     = "${local.name_prefix}-target-pool"
  location = var.zone
  project  = var.project_id
  cluster  = google_container_cluster.target.id

  node_count = 1

  node_config {
    spot         = true
    machine_type = "e2-standard-2"

    oauth_scopes = [
      "https://www.googleapis.com/auth/cloud-platform",
    ]
  }
}

################################################################################
# Google Service Account for the scanner
################################################################################

resource "google_service_account" "scanner" {
  account_id   = "${local.name_prefix}-scanner"
  display_name = "Mondoo WIF Scanner (${random_string.suffix.result})"
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
  location     = var.zone

  depends_on = [google_container_node_pool.management]
}

resource "local_file" "kubeconfig" {
  content  = module.gke_auth.kubeconfig_raw
  filename = "${path.module}/kubeconfig-mgmt"
}

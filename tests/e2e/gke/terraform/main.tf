# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

resource "random_string" "suffix" {
  length  = 4
  special = false
  upper   = false
}

locals {
  name_prefix = "mondoo-e2e-${random_string.suffix.result}"
}

################################################################################
# GKE Cluster
################################################################################

resource "google_container_cluster" "e2e" {
  name     = "${local.name_prefix}-cluster"
  project  = var.project_id
  location = var.region

  enable_autopilot    = var.autopilot ? true : null
  deletion_protection = false

  # Workload Identity is required for WIF external cluster scanning
  dynamic "workload_identity_config" {
    for_each = var.enable_wif_test ? [1] : []
    content {
      workload_pool = "${var.project_id}.svc.id.goog"
    }
  }

  # Standard clusters: remove the default node pool, we manage our own below
  remove_default_node_pool = var.autopilot ? null : true
  initial_node_count       = var.autopilot ? null : 1
}

resource "google_container_node_pool" "e2e" {
  count = var.autopilot ? 0 : 1

  name     = "default"
  project  = var.project_id
  location = var.region
  cluster  = google_container_cluster.e2e.name

  node_count = 1

  node_config {
    machine_type = "e2-standard-2"
    oauth_scopes = [
      "https://www.googleapis.com/auth/cloud-platform",
    ]

    # Required for Workload Identity on nodes
    dynamic "workload_metadata_config" {
      for_each = var.enable_wif_test ? [1] : []
      content {
        mode = "GKE_METADATA"
      }
    }
  }
}

################################################################################
# Artifact Registry Repository
################################################################################

resource "google_artifact_registry_repository" "e2e" {
  location      = var.region
  repository_id = "${local.name_prefix}-repo"
  format        = "DOCKER"
  project       = var.project_id
}

################################################################################
# Mirror Registry (optional, for registry mirroring tests)
# A second Artifact Registry repo used as the mirror target.
################################################################################

resource "google_artifact_registry_repository" "mirror" {
  count = var.enable_mirror_test ? 1 : 0

  location      = var.region
  repository_id = "${local.name_prefix}-mirror"
  format        = "DOCKER"
  project       = var.project_id
}

# Service account with read-only access to the mirror repo.
# Used to create an imagePullSecret, testing the full imagePullSecrets path.
resource "google_service_account" "mirror_reader" {
  count = var.enable_mirror_test ? 1 : 0

  account_id   = "${local.name_prefix}-mirror-sa"
  display_name = "Mirror registry reader for e2e tests"
  project      = var.project_id
}

resource "google_artifact_registry_repository_iam_member" "mirror_reader" {
  count = var.enable_mirror_test ? 1 : 0

  project    = var.project_id
  location   = var.region
  repository = google_artifact_registry_repository.mirror[0].repository_id
  role       = "roles/artifactregistry.reader"
  member     = "serviceAccount:${google_service_account.mirror_reader[0].email}"
}

resource "google_service_account_key" "mirror_reader" {
  count = var.enable_mirror_test ? 1 : 0

  service_account_id = google_service_account.mirror_reader[0].name
}

################################################################################
# Target Cluster (optional, for external cluster scanning tests)
################################################################################

resource "google_container_cluster" "target" {
  count = var.enable_target_cluster ? 1 : 0

  name     = "${local.name_prefix}-target"
  project  = var.project_id
  location = var.region

  enable_autopilot    = var.autopilot ? true : null
  deletion_protection = false

  # Standard clusters: remove the default node pool, we manage our own below
  remove_default_node_pool = var.autopilot ? null : true
  initial_node_count       = var.autopilot ? null : 1
}

resource "google_container_node_pool" "target" {
  count = var.enable_target_cluster && !var.autopilot ? 1 : 0

  name     = "default"
  project  = var.project_id
  location = var.region
  cluster  = google_container_cluster.target[0].name

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
# Kubeconfig
################################################################################

data "google_client_config" "default" {}

module "gke_auth" {
  source = "terraform-google-modules/kubernetes-engine/google//modules/auth"

  project_id   = var.project_id
  cluster_name = google_container_cluster.e2e.name
  location     = var.region
}

resource "local_file" "kubeconfig" {
  content  = module.gke_auth.kubeconfig_raw
  filename = "${path.module}/kubeconfig"
}

################################################################################
# Kubeconfig for Target Cluster
################################################################################

module "gke_auth_target" {
  count  = var.enable_target_cluster ? 1 : 0
  source = "terraform-google-modules/kubernetes-engine/google//modules/auth"

  project_id   = var.project_id
  cluster_name = google_container_cluster.target[0].name
  location     = var.region
}

resource "local_file" "kubeconfig_target" {
  count    = var.enable_target_cluster ? 1 : 0
  content  = module.gke_auth_target[0].kubeconfig_raw
  filename = "${path.module}/kubeconfig-target"
}

################################################################################
# WIF: Google Service Account + IAM (optional, for WIF external cluster tests)
################################################################################

resource "google_service_account" "wif_scanner" {
  count = var.enable_wif_test ? 1 : 0

  account_id   = "${local.name_prefix}-wif-scanner"
  display_name = "WIF scanner for e2e tests"
  project      = var.project_id
}

# Allow the management cluster KSAs to impersonate this GSA.
# - mondoo-client-wif-target-cluster: external cluster scanning
# - mondoo-client-cr-wif: container registry WIF scanning
resource "google_service_account_iam_binding" "wif_identity" {
  count = var.enable_wif_test ? 1 : 0

  service_account_id = google_service_account.wif_scanner[0].name
  role               = "roles/iam.workloadIdentityUser"

  members = [
    "serviceAccount:${var.project_id}.svc.id.goog[mondoo-operator/mondoo-client-wif-target-cluster]",
    "serviceAccount:${var.project_id}.svc.id.goog[mondoo-operator/mondoo-client-cr-wif]",
  ]
}

# Grant the GSA permission to view/access the target cluster
resource "google_project_iam_member" "wif_cluster_viewer" {
  count = var.enable_wif_test ? 1 : 0

  project = var.project_id
  role    = "roles/container.clusterViewer"
  member  = "serviceAccount:${google_service_account.wif_scanner[0].email}"
}

# Grant the GSA read access to the Artifact Registry repo (for container registry WIF scanning)
resource "google_artifact_registry_repository_iam_member" "wif_ar_reader" {
  count = var.enable_wif_test ? 1 : 0

  project    = var.project_id
  location   = var.region
  repository = google_artifact_registry_repository.e2e.repository_id
  role       = "roles/artifactregistry.reader"
  member     = "serviceAccount:${google_service_account.wif_scanner[0].email}"
}


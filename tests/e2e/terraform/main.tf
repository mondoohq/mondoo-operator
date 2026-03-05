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

# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

output "project_id" {
  value = var.project_id
}

output "region" {
  value = var.region
}

output "cluster_name" {
  value = google_container_cluster.e2e.name
}

output "kubeconfig_path" {
  value = local_file.kubeconfig.filename
}

output "artifact_registry_repo" {
  value = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.e2e.repository_id}"
}

output "mondoo_credentials_b64" {
  value     = mondoo_service_account.e2e.credential
  sensitive = true
}

output "mondoo_space_mrn" {
  value = mondoo_space.e2e.mrn
}

output "name_prefix" {
  value = local.name_prefix
}

output "autopilot" {
  value = var.autopilot
}

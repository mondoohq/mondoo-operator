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

output "enable_target_cluster" {
  value = var.enable_target_cluster
}

output "target_cluster_name" {
  value = var.enable_target_cluster ? google_container_cluster.target[0].name : ""
}

output "target_kubeconfig_path" {
  value = var.enable_target_cluster ? local_file.kubeconfig_target[0].filename : ""
}

output "enable_mirror_test" {
  value = var.enable_mirror_test
}

output "mirror_registry_repo" {
  value = var.enable_mirror_test ? "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.mirror[0].repository_id}" : ""
}

output "mirror_sa_key_b64" {
  value     = var.enable_mirror_test ? google_service_account_key.mirror_reader[0].private_key : ""
  sensitive = true
}

output "enable_proxy_test" {
  value = var.enable_proxy_test
}

output "squid_proxy_ip" {
  value = var.enable_proxy_test ? google_compute_instance.squid_proxy[0].network_interface[0].network_ip : ""
}

output "enable_wif_test" {
  value = var.enable_wif_test
}

output "wif_gsa_email" {
  value = var.enable_wif_test ? google_service_account.wif_scanner[0].email : ""
}

output "enable_space_splitting_test" {
  value = var.enable_space_splitting_test
}

output "scanner_space_id" {
  value = var.enable_space_splitting_test ? mondoo_space.e2e.id : ""
}

output "target_space_id" {
  value = (var.enable_space_splitting_test || var.enable_asset_routing_test) ? mondoo_space.target[0].id : ""
}

output "target_space_mrn" {
  value = (var.enable_space_splitting_test || var.enable_asset_routing_test) ? mondoo_space.target[0].mrn : ""
}

output "org_credentials_b64" {
  value     = (var.enable_space_splitting_test || var.enable_asset_routing_test) ? mondoo_service_account.org[0].credential : ""
  sensitive = true
}

output "enable_asset_routing_test" {
  value = var.enable_asset_routing_test
}

output "developers_space_id" {
  value = var.enable_asset_routing_test ? mondoo_space.developers[0].id : ""
}

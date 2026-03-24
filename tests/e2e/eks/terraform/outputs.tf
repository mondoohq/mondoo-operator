# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

output "region" {
  value = var.region
}

output "cluster_name" {
  value = module.eks.cluster_name
}

output "ecr_repo" {
  value = aws_ecr_repository.e2e.repository_url
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

output "enable_target_cluster" {
  value = var.enable_target_cluster
}

output "target_cluster_name" {
  value = var.enable_target_cluster ? module.eks_target[0].cluster_name : ""
}

output "target_kubeconfig_path" {
  value = var.enable_target_cluster ? local_file.kubeconfig_target[0].filename : ""
}

output "enable_wif_test" {
  value = var.enable_wif_test
}

output "scanner_role_arn" {
  value = var.enable_wif_test && var.enable_target_cluster ? aws_iam_role.scanner[0].arn : ""
}

output "profile" {
  value = var.profile != null ? var.profile : ""
}

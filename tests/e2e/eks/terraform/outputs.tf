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

output "private_test_ecr_repo" {
  value = var.enable_wif_test ? aws_ecr_repository.private_test[0].repository_url : ""
}

output "target_cluster_endpoint" {
  description = "API server endpoint of the target EKS cluster (for endpoint override tests)."
  value       = var.enable_target_cluster ? module.eks_target[0].cluster_endpoint : ""
}

output "enable_private_endpoint_access" {
  description = "Whether the scanner-to-target private endpoint SG rule is enabled."
  value       = var.enable_private_endpoint_access
}

output "enable_integration" {
  value = var.enable_integration
}

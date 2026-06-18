# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

output "mondoo_space_id" {
  value = local.space_id
}

output "operator_namespace" {
  value = var.operator_namespace
}

output "target_namespace" {
  value = kubernetes_namespace_v1.targets.metadata[0].name
}

output "scanner_memory_limit" {
  value = var.scanner_memory_limit
}

output "stress_image_count" {
  value = length(var.stress_images)
}

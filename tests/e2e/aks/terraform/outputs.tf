# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

output "region" {
  value = var.location
}

output "cluster_name" {
  value = azurerm_kubernetes_cluster.scanner.name
}

output "resource_group" {
  value = azurerm_resource_group.e2e.name
}

output "acr_login_server" {
  value = azurerm_container_registry.e2e.login_server
}

output "acr_repo" {
  value = "${azurerm_container_registry.e2e.login_server}/mondoo-operator"
}

output "acr_admin_username" {
  value     = azurerm_container_registry.e2e.admin_username
  sensitive = true
}

output "acr_admin_password" {
  value     = azurerm_container_registry.e2e.admin_password
  sensitive = true
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

output "subscription_id" {
  value = var.subscription_id
}

output "enable_target_cluster" {
  value = var.enable_target_cluster
}

output "target_cluster_name" {
  value = var.enable_target_cluster ? azurerm_kubernetes_cluster.target[0].name : ""
}

output "target_kubeconfig_path" {
  value = var.enable_target_cluster ? local_file.kubeconfig_target[0].filename : ""
}

output "enable_wif_test" {
  value = var.enable_wif_test
}

output "wif_client_id" {
  value = var.enable_wif_test && var.enable_target_cluster ? azurerm_user_assigned_identity.scanner[0].client_id : ""
}

output "wif_tenant_id" {
  value = var.enable_wif_test && var.enable_target_cluster ? azurerm_user_assigned_identity.scanner[0].tenant_id : ""
}

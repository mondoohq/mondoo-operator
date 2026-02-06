output "subscription_id" {
  description = "Azure subscription ID."
  value       = data.azurerm_subscription.current.subscription_id
}

output "tenant_id" {
  description = "Azure AD tenant ID."
  value       = data.azurerm_client_config.current.tenant_id
}

output "client_id" {
  description = "Azure AD application (client) ID for the scanner."
  value       = azuread_application.scanner.client_id
}

output "resource_group" {
  description = "Resource group containing both clusters."
  value       = azurerm_resource_group.rg.name
}

output "management_cluster_name" {
  description = "Name of the management AKS cluster."
  value       = azurerm_kubernetes_cluster.management.name
}

output "target_cluster_name" {
  description = "Name of the target AKS cluster."
  value       = azurerm_kubernetes_cluster.target.name
}

output "mondoo_audit_config_snippet" {
  description = "MondooAuditConfig YAML snippet for the AKS WIF external cluster."
  value       = <<-EOT
    externalClusters:
      - name: ${azurerm_kubernetes_cluster.target.name}
        workloadIdentity:
          provider: aks
          aks:
            subscriptionId: ${data.azurerm_subscription.current.subscription_id}
            resourceGroup: ${azurerm_resource_group.rg.name}
            clusterName: ${azurerm_kubernetes_cluster.target.name}
            clientId: ${azuread_application.scanner.client_id}
            tenantId: ${data.azurerm_client_config.current.tenant_id}
  EOT
}

output "kubeconfig_commands" {
  description = "Commands to configure kubectl for both clusters."
  value       = <<-EOT
    # Management cluster:
    az aks get-credentials --resource-group ${azurerm_resource_group.rg.name} --name ${azurerm_kubernetes_cluster.management.name} --context mgmt

    # Target cluster:
    az aks get-credentials --resource-group ${azurerm_resource_group.rg.name} --name ${azurerm_kubernetes_cluster.target.name} --context target
  EOT
}

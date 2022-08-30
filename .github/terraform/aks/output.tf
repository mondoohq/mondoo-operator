output "resource_group_name" {
  value = azurerm_resource_group.rg.name
}

output "tls_private_key" {
  value     = tls_private_key.attacker_vm_ssh.private_key_pem
  sensitive = true
}

resource "local_file" "kubeconfig" {
  depends_on   = [azurerm_kubernetes_cluster.cluster]
  filename     = "kubeconfig"
  content      = azurerm_kubernetes_cluster.cluster.kube_config_raw
}
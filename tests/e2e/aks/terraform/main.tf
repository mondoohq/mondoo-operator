# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

resource "random_string" "suffix" {
  length  = 4
  special = false
  upper   = false
}

locals {
  name_prefix = "mondoo-e2e-${random_string.suffix.result}"
  default_tags = {
    Name       = local.name_prefix
    GitHubRepo = "mondoo-operator"
    GitHubOrg  = "mondoohq"
    Terraform  = "true"
  }
}

data "azurerm_subscription" "current" {}

################################################################################
# Resource Group
################################################################################

resource "azurerm_resource_group" "e2e" {
  name     = "${local.name_prefix}-rg"
  location = var.location
  tags     = local.default_tags
}

################################################################################
# Virtual Network
################################################################################

resource "azurerm_virtual_network" "e2e" {
  name                = "${local.name_prefix}-vnet"
  location            = azurerm_resource_group.e2e.location
  resource_group_name = azurerm_resource_group.e2e.name
  address_space       = ["10.0.0.0/16"]
  tags                = local.default_tags
}

resource "azurerm_subnet" "scanner" {
  name                 = "scanner-subnet"
  resource_group_name  = azurerm_resource_group.e2e.name
  virtual_network_name = azurerm_virtual_network.e2e.name
  address_prefixes     = ["10.0.1.0/24"]
}

resource "azurerm_subnet" "target" {
  count                = var.enable_target_cluster ? 1 : 0
  name                 = "target-subnet"
  resource_group_name  = azurerm_resource_group.e2e.name
  virtual_network_name = azurerm_virtual_network.e2e.name
  address_prefixes     = ["10.0.2.0/24"]
}

################################################################################
# AKS Cluster (scanner)
################################################################################

resource "azurerm_kubernetes_cluster" "scanner" {
  name                = "${local.name_prefix}-cluster"
  location            = azurerm_resource_group.e2e.location
  resource_group_name = azurerm_resource_group.e2e.name
  dns_prefix          = "${local.name_prefix}-cluster"
  kubernetes_version  = var.k8s_version

  default_node_pool {
    name           = "default"
    node_count     = 1
    vm_size        = "Standard_D2s_v3"
    vnet_subnet_id = azurerm_subnet.scanner.id
  }

  identity {
    type = "SystemAssigned"
  }

  # OIDC issuer is required for Workload Identity
  oidc_issuer_enabled       = var.enable_wif_test
  workload_identity_enabled = var.enable_wif_test

  network_profile {
    network_plugin = "azure"
    service_cidr   = "172.16.0.0/16"
    dns_service_ip = "172.16.0.10"
  }

  tags = local.default_tags
}

################################################################################
# ACR Repository
################################################################################

resource "azurerm_container_registry" "e2e" {
  name                = replace("${local.name_prefix}acr", "-", "")
  location            = azurerm_resource_group.e2e.location
  resource_group_name = azurerm_resource_group.e2e.name
  sku                 = "Basic"
  admin_enabled       = true
  tags                = local.default_tags
}

# Grant scanner cluster AcrPull access
resource "azurerm_role_assignment" "scanner_acr_pull" {
  principal_id                     = azurerm_kubernetes_cluster.scanner.kubelet_identity[0].object_id
  role_definition_name             = "AcrPull"
  scope                            = azurerm_container_registry.e2e.id
  skip_service_principal_aad_check = true
}

# Grant target cluster AcrPull access (for pulling private test images)
resource "azurerm_role_assignment" "target_acr_pull" {
  count                            = var.enable_target_cluster ? 1 : 0
  principal_id                     = azurerm_kubernetes_cluster.target[0].kubelet_identity[0].object_id
  role_definition_name             = "AcrPull"
  scope                            = azurerm_container_registry.e2e.id
  skip_service_principal_aad_check = true
}

################################################################################
# Target Cluster (optional, for external cluster scanning tests)
################################################################################

resource "azurerm_kubernetes_cluster" "target" {
  count               = var.enable_target_cluster ? 1 : 0
  name                = "${local.name_prefix}-target"
  location            = azurerm_resource_group.e2e.location
  resource_group_name = azurerm_resource_group.e2e.name
  dns_prefix          = "${local.name_prefix}-target"
  kubernetes_version  = var.k8s_version

  default_node_pool {
    name           = "default"
    node_count     = 1
    vm_size        = "Standard_D2s_v3"
    vnet_subnet_id = azurerm_subnet.target[0].id
  }

  identity {
    type = "SystemAssigned"
  }

  network_profile {
    network_plugin = "azure"
    service_cidr   = "172.17.0.0/16"
    dns_service_ip = "172.17.0.10"
  }

  tags = local.default_tags
}

# Allow scanner cluster to reach target cluster API server.
# Both clusters are in the same VNet but on different subnets.
# NSG rule ensures scanner subnet can reach target subnet on port 443.
resource "azurerm_network_security_group" "scanner_to_target" {
  count               = var.enable_target_cluster ? 1 : 0
  name                = "${local.name_prefix}-scanner-to-target-nsg"
  location            = azurerm_resource_group.e2e.location
  resource_group_name = azurerm_resource_group.e2e.name
  tags                = local.default_tags
}

resource "azurerm_network_security_rule" "scanner_to_target_api" {
  count                       = var.enable_target_cluster ? 1 : 0
  name                        = "scanner-to-target-api"
  priority                    = 100
  direction                   = "Inbound"
  access                      = "Allow"
  protocol                    = "Tcp"
  source_port_range           = "*"
  destination_port_range      = "443"
  source_address_prefix       = azurerm_subnet.scanner.address_prefixes[0]
  destination_address_prefix  = azurerm_subnet.target[0].address_prefixes[0]
  resource_group_name         = azurerm_resource_group.e2e.name
  network_security_group_name = azurerm_network_security_group.scanner_to_target[0].name
}

resource "azurerm_subnet_network_security_group_association" "target" {
  count                     = var.enable_target_cluster ? 1 : 0
  subnet_id                 = azurerm_subnet.target[0].id
  network_security_group_id = azurerm_network_security_group.scanner_to_target[0].id
}

################################################################################
# Kubeconfig
################################################################################

resource "local_file" "kubeconfig" {
  content = templatefile("${path.module}/kubeconfig.tpl", {
    cluster_name     = azurerm_kubernetes_cluster.scanner.name
    cluster_endpoint = azurerm_kubernetes_cluster.scanner.kube_config[0].host
    cluster_ca       = azurerm_kubernetes_cluster.scanner.kube_config[0].cluster_ca_certificate
  })
  filename = "${path.module}/kubeconfig"
}

resource "local_file" "kubeconfig_target" {
  count = var.enable_target_cluster ? 1 : 0
  content = templatefile("${path.module}/kubeconfig.tpl", {
    cluster_name     = azurerm_kubernetes_cluster.target[0].name
    cluster_endpoint = azurerm_kubernetes_cluster.target[0].kube_config[0].host
    cluster_ca       = azurerm_kubernetes_cluster.target[0].kube_config[0].cluster_ca_certificate
  })
  filename = "${path.module}/kubeconfig-target"
}

################################################################################
# WIF: User-Assigned Managed Identity + Federated Credential
################################################################################

resource "azurerm_user_assigned_identity" "scanner" {
  count               = var.enable_wif_test && var.enable_target_cluster ? 1 : 0
  name                = "${local.name_prefix}-wif-scanner"
  location            = azurerm_resource_group.e2e.location
  resource_group_name = azurerm_resource_group.e2e.name
  tags                = local.default_tags
}

# Federated credential: trust the scanner cluster OIDC issuer for the WIF KSA
resource "azurerm_federated_identity_credential" "scanner" {
  count     = var.enable_wif_test && var.enable_target_cluster ? 1 : 0
  name      = "${local.name_prefix}-wif-fed-cred"
  user_assigned_identity_id = azurerm_user_assigned_identity.scanner[0].id
  audience            = ["api://AzureADTokenExchange"]
  issuer              = azurerm_kubernetes_cluster.scanner.oidc_issuer_url
  # KSA name: mondoo-client-wif-target-cluster (from WIFServiceAccountName)
  subject = "system:serviceaccount:mondoo-operator:mondoo-client-wif-target-cluster"
}

# Grant the managed identity "Azure Kubernetes Service Cluster User Role" on the target cluster
# This allows `az aks get-credentials` to succeed
resource "azurerm_role_assignment" "scanner_target_user" {
  count                = var.enable_wif_test && var.enable_target_cluster ? 1 : 0
  scope                = azurerm_kubernetes_cluster.target[0].id
  role_definition_name = "Azure Kubernetes Service Cluster User Role"
  principal_id         = azurerm_user_assigned_identity.scanner[0].principal_id
}

# Grant the managed identity "Azure Kubernetes Service RBAC Reader" on the target cluster
# This allows read access to cluster resources (analogous to ClusterRole view)
resource "azurerm_role_assignment" "scanner_target_rbac_reader" {
  count                = var.enable_wif_test && var.enable_target_cluster ? 1 : 0
  scope                = azurerm_kubernetes_cluster.target[0].id
  role_definition_name = "Azure Kubernetes Service RBAC Reader"
  principal_id         = azurerm_user_assigned_identity.scanner[0].principal_id
}

# Grant the managed identity AcrPull on the ACR (for container registry WIF scanning)
resource "azurerm_role_assignment" "scanner_acr_pull_wif" {
  count                = var.enable_wif_test && var.enable_target_cluster ? 1 : 0
  scope                = azurerm_container_registry.e2e.id
  role_definition_name = "AcrPull"
  principal_id         = azurerm_user_assigned_identity.scanner[0].principal_id
}

# Federated credential for the container registry WIF KSA (mondoo-client-cr-wif)
resource "azurerm_federated_identity_credential" "cr_wif" {
  count                         = var.enable_wif_test && var.enable_target_cluster ? 1 : 0
  name                          = "${local.name_prefix}-cr-wif-fed-cred"
  resource_group_name           = azurerm_resource_group.e2e.name
  user_assigned_identity_id     = azurerm_user_assigned_identity.scanner[0].id
  audience                      = ["api://AzureADTokenExchange"]
  issuer                        = azurerm_kubernetes_cluster.scanner.oidc_issuer_url
  subject                       = "system:serviceaccount:mondoo-operator:mondoo-client-cr-wif"
}

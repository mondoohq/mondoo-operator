resource "random_string" "suffix" {
  length  = 4
  special = false
  upper   = false
}

locals {
  name_prefix = "mondoo-wif-${random_string.suffix.result}"
}

data "azurerm_subscription" "current" {}
data "azurerm_client_config" "current" {}

################################################################################
# Resource Group
################################################################################

resource "azurerm_resource_group" "rg" {
  name     = "${local.name_prefix}-rg"
  location = var.location

  tags = {
    Environment = "Mondoo Operator WIF Tests"
  }
}

################################################################################
# Management Cluster (runs the operator, OIDC + Workload Identity enabled)
################################################################################

resource "azurerm_kubernetes_cluster" "management" {
  name                = "${local.name_prefix}-mgmt"
  location            = azurerm_resource_group.rg.location
  resource_group_name = azurerm_resource_group.rg.name
  dns_prefix          = "${local.name_prefix}-mgmt"
  kubernetes_version  = var.k8s_version

  oidc_issuer_enabled       = true
  workload_identity_enabled = true

  default_node_pool {
    name       = "default"
    node_count = 1
    vm_size    = "Standard_D2s_v3"
  }

  identity {
    type = "SystemAssigned"
  }

  tags = {
    Environment = "Mondoo Operator WIF Tests"
  }
}

################################################################################
# Target Cluster (to be scanned, Azure RBAC enabled)
################################################################################

resource "azurerm_kubernetes_cluster" "target" {
  name                = "${local.name_prefix}-target"
  location            = azurerm_resource_group.rg.location
  resource_group_name = azurerm_resource_group.rg.name
  dns_prefix          = "${local.name_prefix}-target"
  kubernetes_version  = var.k8s_version

  azure_active_directory_role_based_access_control {
    managed            = true
    azure_rbac_enabled = true
  }

  default_node_pool {
    name       = "default"
    node_count = 1
    vm_size    = "Standard_D2s_v3"
  }

  identity {
    type = "SystemAssigned"
  }

  tags = {
    Environment = "Mondoo Operator WIF Tests"
  }
}

################################################################################
# Azure AD Application + Service Principal for the scanner
################################################################################

resource "azuread_application" "scanner" {
  display_name = "${local.name_prefix}-scanner"
}

resource "azuread_service_principal" "scanner" {
  client_id = azuread_application.scanner.client_id
}

# Federated identity credential: links the management cluster's KSA to the Azure AD app
resource "azuread_application_federated_identity_credential" "scanner" {
  application_id = azuread_application.scanner.id
  display_name   = "mondoo-operator-wif"
  audiences      = ["api://AzureADTokenExchange"]
  issuer         = azurerm_kubernetes_cluster.management.oidc_issuer_url
  subject        = "system:serviceaccount:${var.scanner_namespace}:${var.scanner_service_account}"
}

################################################################################
# Role Assignments on the target cluster
################################################################################

# Allow the service principal to get cluster credentials
resource "azurerm_role_assignment" "cluster_user" {
  scope                = azurerm_kubernetes_cluster.target.id
  role_definition_name = "Azure Kubernetes Service Cluster User Role"
  principal_id         = azuread_service_principal.scanner.object_id
}

# Allow the service principal to read K8s resources via Azure RBAC
resource "azurerm_role_assignment" "rbac_reader" {
  scope                = azurerm_kubernetes_cluster.target.id
  role_definition_name = "Azure Kubernetes Service RBAC Reader"
  principal_id         = azuread_service_principal.scanner.object_id
}

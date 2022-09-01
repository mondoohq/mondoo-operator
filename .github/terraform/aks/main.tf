resource "random_string" "suffix" {
  length  = 4
  special = false
  upper   = false
}

resource "azurerm_resource_group" "rg" {
  name      = "mondoo-operator-tests-${random_string.suffix.result}"
  location  = var.resource_group_location
}

# Generate random text for a unique storage account name
resource "random_id" "randomId" {
  keepers = {
    # Generate a new ID only when a new resource group is defined
    resource_group = azurerm_resource_group.rg.name
  }

  byte_length = 8
}

# create aks cluster
resource "azurerm_kubernetes_cluster" "cluster" {
  name                = "mondoo-operator-tests-${random_string.suffix.result}"
  location            = azurerm_resource_group.rg.location
  resource_group_name = azurerm_resource_group.rg.name
  dns_prefix          = "mondoo-operator-${random_string.suffix.result}"
  kubernetes_version  = var.k8s_version

  default_node_pool {
    name       = "default"
    node_count = "1"
    vm_size    = "standard_d2_v2"
    enable_node_public_ip = true
  }

  identity {
    type = "SystemAssigned"
  }

  tags = {
    Environment = "Mondoo Operator Tests"
  }
}

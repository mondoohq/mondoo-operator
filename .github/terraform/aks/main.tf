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

# Create storage account for boot diagnostics
resource "azurerm_storage_account" "mystorageaccount" {
  name                     = "diaglunalectric${random_string.suffix.result}"
  location                 = azurerm_resource_group.rg.location
  resource_group_name      = azurerm_resource_group.rg.name
  account_tier             = "Standard"
  account_replication_type = "LRS"
}

# Create (and display) an SSH key
resource "tls_private_key" "attacker_vm_ssh" {
  algorithm = "RSA"
  rsa_bits  = 4096
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
    tags = {
      keyvault = "keyvaultmo-${random_string.suffix.result}"
    }
  }

  linux_profile {
    admin_username = "ubuntu"

    ssh_key {
        key_data = tls_private_key.attacker_vm_ssh.public_key_openssh
    }
  }

  identity {
    type = "SystemAssigned"
  }

  tags = {
    Environment = "Mondoo Operator Tests"
  }
}

# configure keyvault
data "azurerm_client_config" "current"{}

resource "azurerm_key_vault" "keyvault" {
  name = "keyvaultmo-${random_string.suffix.result}"
  location            = azurerm_resource_group.rg.location
  resource_group_name = azurerm_resource_group.rg.name
  sku_name = "standard"
  tags = {
    "createdBy"   = "hello@mondoo.com"
  }

  tenant_id = data.azurerm_client_config.current.tenant_id
  access_policy {
    tenant_id = data.azurerm_client_config.current.tenant_id
    object_id = data.azurerm_client_config.current.object_id

    key_permissions = [
      "Create",
      "Get",
      "List",
    ]

    secret_permissions = [
      "Set",
      "Get",
      "Delete",
      "Purge",
      "Recover",
      "List"
    ]
  }
  access_policy {
    object_id = azurerm_kubernetes_cluster.cluster.kubelet_identity[0].object_id
    tenant_id = azurerm_kubernetes_cluster.cluster.identity[0].tenant_id
    secret_permissions = ["Get","List"]
    key_permissions = [
      "Create",
      "Get",
      "List",
    ]
  }

  depends_on = [azurerm_kubernetes_cluster.cluster]
}

resource "azurerm_key_vault_secret" "example-secret" {
  name         = "secret-sauce"
  value        = "example-pass"
  key_vault_id = azurerm_key_vault.keyvault.id
}

resource "azurerm_key_vault_secret" "ssh-key" {
  name         = "private-ssh-key"
  value        = tls_private_key.attacker_vm_ssh.private_key_pem
  key_vault_id = azurerm_key_vault.keyvault.id
}
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

terraform {
  required_version = ">= 1.3"

  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 4.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.5"
    }
    mondoo = {
      source  = "mondoohq/mondoo"
      version = ">= 0.19"
    }
    local = {
      source  = "hashicorp/local"
      version = ">= 2.0"
    }
  }
}

provider "azurerm" {
  features {}
  subscription_id = var.subscription_id
}

provider "mondoo" {}

# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

terraform {
  required_version = ">= 1.3"

  required_providers {
    mondoo = {
      source  = "mondoohq/mondoo"
      version = ">= 0.19"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = ">= 2.20"
    }
  }
}

provider "mondoo" {}

provider "kubernetes" {
  config_path    = var.kubeconfig_path
  config_context = var.kubeconfig_context
}

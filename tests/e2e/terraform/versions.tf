# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

terraform {
  required_version = ">= 1.3"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
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

provider "google" {
  project = var.project_id
  region  = var.region
}

provider "mondoo" {}

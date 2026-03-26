# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

terraform {
  required_version = ">= 1.3"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
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

provider "aws" {
  region  = var.region
  profile = var.profile
}

provider "mondoo" {}

# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

variable "subscription_id" {
  description = "Azure subscription ID"
  type        = string
}

variable "location" {
  description = "Azure region"
  type        = string
  default     = "westeurope"
}

variable "k8s_version" {
  description = "Kubernetes version for AKS clusters (null = latest supported)"
  type        = string
  default     = null
}

variable "mondoo_org_id" {
  description = "Mondoo organization ID for space creation"
  type        = string
}

variable "enable_target_cluster" {
  description = "Create a second AKS cluster as a scan target for external cluster testing"
  type        = bool
  default     = false
}

variable "enable_wif_test" {
  description = "Enable Azure Workload Identity for external cluster scanning"
  type        = bool
  default     = false
}

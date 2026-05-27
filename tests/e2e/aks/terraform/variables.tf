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

variable "enable_private_endpoint_access" {
  description = "Allow scanner cluster subnet to reach target cluster API server via private endpoint (NSG rule). Set to false for endpoint override tests where scanner must use the public endpoint."
  type        = bool
  default     = true
}

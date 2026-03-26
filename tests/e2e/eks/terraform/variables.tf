# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

variable "region" {
  description = "AWS region"
  type        = string
  default     = "eu-central-1"
}

variable "profile" {
  description = "AWS CLI profile to use for authentication"
  type        = string
  default     = null
}

variable "k8s_version" {
  description = "Kubernetes version for EKS clusters"
  type        = string
  default     = "1.30"
}

variable "mondoo_org_id" {
  description = "Mondoo organization ID for space creation"
  type        = string
}

variable "enable_target_cluster" {
  description = "Create a second EKS cluster as a scan target for external cluster testing"
  type        = bool
  default     = false
}

variable "enable_wif_test" {
  description = "Enable IRSA (Workload Identity) for external cluster scanning"
  type        = bool
  default     = false
}

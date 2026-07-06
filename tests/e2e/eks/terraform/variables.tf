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

variable "enable_private_endpoint_access" {
  description = "Allow scanner cluster nodes to reach target cluster API server via private endpoint (SG rule). Set to false for endpoint override tests where scanner must use the public endpoint."
  type        = bool
  default     = true
}

variable "enable_integration" {
  description = "Create a K8s integration and use integration token instead of service account credentials"
  type        = bool
  default     = false
}

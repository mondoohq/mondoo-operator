# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

variable "project_id" {
  description = "GCP project ID"
  type        = string
}

variable "region" {
  description = "GCP region"
  type        = string
  default     = "europe-west3"
}

variable "mondoo_org_id" {
  description = "Mondoo organization ID for space creation"
  type        = string
}

variable "autopilot" {
  description = "Use GKE Autopilot (true) or Standard (false) cluster mode"
  type        = bool
  default     = true
}

variable "enable_target_cluster" {
  description = "Create a second GKE cluster as a scan target for external cluster testing"
  type        = bool
  default     = false
}

variable "enable_mirror_test" {
  description = "Create a mirror Artifact Registry repo for registry mirroring and imagePullSecrets testing"
  type        = bool
  default     = false
}

variable "enable_proxy_test" {
  description = "Create a Squid proxy VM for proxy testing (requires enable_mirror_test=true)"
  type        = bool
  default     = false
}

variable "enable_wif_test" {
  description = "Enable GKE Workload Identity Federation for external cluster scanning"
  type        = bool
  default     = false
}

variable "enable_space_splitting_test" {
  description = "Create a second Mondoo space to test org-level SA with spaceId routing"
  type        = bool
  default     = false
}

variable "enable_asset_routing_test" {
  description = "Create spaces and routing rules for server-side asset routing tests"
  type        = bool
  default     = false
}

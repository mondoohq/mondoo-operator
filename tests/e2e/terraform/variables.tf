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

# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

variable "mondoo_org_id" {
  description = "Mondoo organization ID for space creation (not needed if mondoo_space_id is set)"
  type        = string
  default     = ""
}

variable "mondoo_space_id" {
  description = "Use an existing Mondoo space instead of creating one (for local dev server)"
  type        = string
  default     = ""
}


variable "kubeconfig_path" {
  description = "Path to kubeconfig file (defaults to ~/.kube/config)"
  type        = string
  default     = "~/.kube/config"
}

variable "kubeconfig_context" {
  description = "Kubeconfig context to use (defaults to current context)"
  type        = string
  default     = null
}

variable "operator_namespace" {
  description = "Namespace where the mondoo-operator is installed"
  type        = string
  default     = "mondoo-operator"
}

variable "target_namespace" {
  description = "Namespace for the stress test target pods"
  type        = string
  default     = "oom-stress-targets"
}

variable "scanner_memory_limit" {
  description = "Memory limit for the container scanner (lower = OOM faster)"
  type        = string
  default     = "512Mi"
}

variable "scanner_memory_request" {
  description = "Memory request for the container scanner"
  type        = string
  default     = "128Mi"
}

variable "docker_hub_username" {
  description = "Docker Hub username for authenticated pulls (avoids rate limits)"
  type        = string
  default     = ""
}

variable "docker_hub_password" {
  description = "Docker Hub password or personal access token"
  type        = string
  default     = ""
  sensitive   = true

  validation {
    condition     = var.docker_hub_username == "" || var.docker_hub_password != ""
    error_message = "docker_hub_password is required when docker_hub_username is set."
  }
}

variable "scan_schedule" {
  description = "Cron schedule for the container scan (default: every 5 minutes)"
  type        = string
  default     = "*/5 * * * *"
}

variable "stress_images" {
  description = "List of container images to deploy as scan targets. Each must support the host architecture."
  type = list(object({
    name  = string
    image = string
  }))
  default = [
    { name = "ubuntu-2404", image = "ubuntu:24.04" },
    { name = "ubuntu-2204", image = "ubuntu:22.04" },
    { name = "ubuntu-2004", image = "ubuntu:20.04" },
    { name = "debian-12", image = "debian:12" },
    { name = "debian-11", image = "debian:11" },
    { name = "fedora-40", image = "fedora:40" },
    { name = "fedora-39", image = "fedora:39" },
    { name = "amazonlinux-2023", image = "amazonlinux:2023" },
    { name = "amazonlinux-2", image = "amazonlinux:2" },
    { name = "rockylinux-9", image = "rockylinux:9" },
    { name = "almalinux-9", image = "almalinux:9" },
    { name = "oraclelinux-9", image = "oraclelinux:9" },
    { name = "alpine-319", image = "alpine:3.19" },
    { name = "python-312", image = "python:3.12" },
    { name = "python-311", image = "python:3.11" },
    { name = "node-20", image = "node:20" },
    { name = "node-22", image = "node:22" },
    { name = "ruby-33", image = "ruby:3.3" },
  ]
}

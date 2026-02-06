variable "project_id" {
  description = "GCP project ID."
  type        = string
}

variable "region" {
  description = "GCP region for the Autopilot clusters."
  type        = string
  default     = "us-central1"
}

variable "scanner_namespace" {
  description = "Namespace where the Mondoo operator scanner runs."
  type        = string
  default     = "mondoo-operator"
}

variable "scanner_service_account" {
  description = "Kubernetes ServiceAccount name used by the scanner."
  type        = string
  default     = "mondoo-client-wif-target"
}

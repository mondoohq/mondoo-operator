variable "project_id" {
  description = "GCP project ID."
  type        = string
}

variable "region" {
  description = "GCP region for the clusters."
  type        = string
  default     = "us-central1"
}

variable "zone" {
  description = "GCP zone for zonal clusters."
  type        = string
  default     = "us-central1-a"
}

variable "k8s_version" {
  description = "Kubernetes version for the clusters."
  type        = string
  default     = "1.30"
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

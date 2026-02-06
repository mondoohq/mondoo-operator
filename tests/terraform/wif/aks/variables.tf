variable "location" {
  description = "Azure region for all resources."
  type        = string
  default     = "eastus"
}

variable "k8s_version" {
  description = "Kubernetes version for the AKS clusters."
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

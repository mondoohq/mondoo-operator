variable "k8s_version" {
  default     = "1.23"
  description = "Kubernetes version to use for the cluster."
}

variable "project_id" {
  default     = "mondoo-operator-testing"
  description = "The GCP project ID in which the cluster will be created."
}
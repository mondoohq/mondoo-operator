// VARIABLES

variable "region" {
  default     = "eu-central-1"
  description = "AWS Region"
}

variable "test_name" {
  description = "A name to be applied as a suffix to project resources"
  type        = string
}

variable "kubernetes_version" {
  default     = "1.23"
  description = "Kubernetes cluster version used with EKS"
}

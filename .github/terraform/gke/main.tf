resource "random_string" "suffix" {
  length  = 4
  special = false
  upper   = false
}

resource "google_container_cluster" "cluster" {
  name               = "mondoo-operator-tests-${random_string.suffix.result}"
  project            = var.project_id
  location           = "us-central1-a"
  initial_node_count = 1

  min_master_version = var.k8s_version
  node_version       = var.k8s_version
}

data "google_client_config" "default" {}

provider "kubernetes" {
  host                   = google_container_cluster.cluster.endpoint
  token                  = data.google_client_config.default.access_token
  cluster_ca_certificate = base64decode(google_container_cluster.cluster.master_auth.0.cluster_ca_certificate)
}

module "gke_auth" {
  source               = "terraform-google-modules/kubernetes-engine/google//modules/auth"

  project_id           = var.project_id
  cluster_name         = google_container_cluster.cluster.name
  location             = "us-central1-a"
}
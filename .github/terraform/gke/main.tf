resource "random_string" "suffix" {
  length  = 4
  special = false
  upper   = false
}

resource "google_container_cluster" "cluster" {
  name               = "mondoo-operator-tests-${random_string.suffix.result}"
  project            = var.project_id
  location           = "us-central1-a"
  min_master_version = var.k8s_version

  remove_default_node_pool = true
  initial_node_count       = 1
}

resource "google_container_node_pool" "node_pool" {
  name       = "mondoo-operator-pool-${random_string.suffix.result}"
  location   = "us-central1-a"
  cluster    = google_container_cluster.cluster.id
  node_count = 1

  node_config {
    machine_type = "e2-standard-2"
  }
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
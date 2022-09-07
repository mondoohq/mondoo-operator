resource "local_file" "kubeconfig" {
  depends_on   = [google_container_cluster.cluster]
  content      = module.gke_auth.kubeconfig_raw
  filename     = "kubeconfig"
}
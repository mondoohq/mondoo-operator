output "project_id" {
  description = "GCP project ID."
  value       = var.project_id
}

output "management_cluster_name" {
  description = "Name of the management GKE cluster."
  value       = google_container_cluster.management.name
}

output "target_cluster_name" {
  description = "Name of the target GKE cluster."
  value       = google_container_cluster.target.name
}

output "cluster_location" {
  description = "Zone of both clusters."
  value       = var.zone
}

output "google_service_account" {
  description = "Email of the Google Service Account for the scanner."
  value       = google_service_account.scanner.email
}

output "mondoo_audit_config_snippet" {
  description = "MondooAuditConfig YAML snippet for the GKE WIF external cluster."
  value       = <<-EOT
    externalClusters:
      - name: ${google_container_cluster.target.name}
        workloadIdentity:
          provider: gke
          gke:
            projectId: ${var.project_id}
            clusterName: ${google_container_cluster.target.name}
            clusterLocation: ${var.zone}
            googleServiceAccount: ${google_service_account.scanner.email}
  EOT
}

output "target_cluster_rbac" {
  description = "Manual step: apply this ClusterRoleBinding on the target cluster."
  value       = <<-EOT
    # Get credentials for the target cluster:
    #   gcloud container clusters get-credentials ${google_container_cluster.target.name} --zone ${var.zone} --project ${var.project_id}
    #
    # Then apply:
    apiVersion: rbac.authorization.k8s.io/v1
    kind: ClusterRoleBinding
    metadata:
      name: mondoo-wif-scanner
    roleRef:
      apiGroup: rbac.authorization.k8s.io
      kind: ClusterRole
      name: view
    subjects:
    - apiGroup: rbac.authorization.k8s.io
      kind: User
      name: ${google_service_account.scanner.email}
  EOT
}

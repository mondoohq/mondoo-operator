output "region" {
  description = "AWS region."
  value       = var.region
}

output "management_cluster_name" {
  description = "Name of the management EKS cluster."
  value       = module.management.cluster_name
}

output "target_cluster_name" {
  description = "Name of the target EKS cluster."
  value       = module.target.cluster_name
}

output "scanner_role_arn" {
  description = "ARN of the IAM role for the scanner."
  value       = aws_iam_role.scanner.arn
}

output "mondoo_audit_config_snippet" {
  description = "MondooAuditConfig YAML snippet for the EKS IRSA external cluster."
  value       = <<-EOT
    externalClusters:
      - name: ${module.target.cluster_name}
        workloadIdentity:
          provider: eks
          eks:
            region: ${var.region}
            clusterName: ${module.target.cluster_name}
            roleArn: ${aws_iam_role.scanner.arn}
  EOT
}

output "kubeconfig_update_commands" {
  description = "Commands to configure kubectl for both clusters."
  value       = <<-EOT
    # Management cluster:
    aws eks update-kubeconfig --name ${module.management.cluster_name} --region ${var.region} --alias mgmt

    # Target cluster:
    aws eks update-kubeconfig --name ${module.target.cluster_name} --region ${var.region} --alias target
  EOT
}

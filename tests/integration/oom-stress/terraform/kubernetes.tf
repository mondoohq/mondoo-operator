# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Kubernetes resources for the OOM stress test:
# - Target namespace with package-heavy pods for scanning
# - Mondoo credentials secret
# - MondooAuditConfig with reduced memory limits

# --- Target namespace + pods ---

resource "kubernetes_namespace_v1" "targets" {
  metadata {
    name = var.target_namespace
  }
}

resource "kubernetes_pod_v1" "stress_target" {
  for_each = { for img in var.stress_images : img.name => img }

  metadata {
    name      = each.value.name
    namespace = kubernetes_namespace_v1.targets.metadata[0].name

    labels = {
      "app.kubernetes.io/name"    = each.value.name
      "app.kubernetes.io/part-of" = "oom-stress-test"
    }
  }

  spec {
    container {
      name    = "target"
      image   = "${local.registry_host}/${each.value.image}"
      command = ["sleep", "infinity"]
    }
  }

  depends_on = [kubernetes_job_v1.seed_registry]
}

# --- Mondoo credentials ---

resource "kubernetes_secret_v1" "mondoo_client" {
  metadata {
    name      = "mondoo-client"
    namespace = var.operator_namespace
  }

  data = {
    config = base64decode(mondoo_service_account.oom_stress.credential)
  }

  lifecycle {
    # Don't destroy the secret if the operator is still running — it causes
    # reconcile errors. The operator namespace cleanup handles this.
    prevent_destroy = false
  }
}

# --- MondooAuditConfig ---

resource "kubernetes_manifest" "audit_config" {
  manifest = {
    apiVersion = "k8s.mondoo.com/v1alpha2"
    kind       = "MondooAuditConfig"

    metadata = {
      name      = "oom-stress-test"
      namespace = var.operator_namespace
    }

    spec = {
      mondooCredsSecretRef = {
        name = kubernetes_secret_v1.mondoo_client.metadata[0].name
      }

      # Using service account directly, not a console integration
      consoleIntegration = {
        enable = false
      }

      containers = {
        enable   = true
        schedule = var.scan_schedule
        resources = {
          limits = {
            memory = var.scanner_memory_limit
          }
          requests = {
            memory = var.scanner_memory_request
          }
        }
      }

      filtering = {
        namespaces = {
          include = [var.target_namespace]
        }
      }
    }
  }

  depends_on = [
    kubernetes_secret_v1.mondoo_client,
    kubernetes_namespace_v1.targets,
    kubernetes_pod_v1.stress_target,
  ]
}

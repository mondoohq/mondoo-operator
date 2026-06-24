# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# In-cluster registry to avoid Docker Hub rate limits during container image
# scanning. A crane Job copies each stress image once from Docker Hub into the
# local registry. The stress-target pods and the scanner both reference the
# local registry, so Docker Hub is only hit during the seed phase.

resource "kubernetes_deployment_v1" "registry" {
  metadata {
    name      = "registry"
    namespace = kubernetes_namespace_v1.targets.metadata[0].name
    labels    = { app = "oom-stress-registry" }
  }

  spec {
    replicas = 1

    selector {
      match_labels = { app = "oom-stress-registry" }
    }

    template {
      metadata {
        labels = { app = "oom-stress-registry" }
      }

      spec {
        container {
          name  = "registry"
          image = "registry:2"

          port {
            container_port = 5000
          }

          resources {
            requests = { memory = "64Mi", cpu = "50m" }
            limits   = { memory = "256Mi" }
          }
        }
      }
    }
  }
}

resource "kubernetes_service_v1" "registry" {
  metadata {
    name      = "registry"
    namespace = kubernetes_namespace_v1.targets.metadata[0].name
  }

  spec {
    selector = { app = "oom-stress-registry" }

    port {
      port        = 80
      target_port = 5000
    }
  }
}

locals {
  registry_host = "registry.${var.target_namespace}.svc.cluster.local"

  # Build the crane copy script. Each line copies one image from Docker Hub to
  # the local registry. crane streams layers directly (no local disk needed).
  crane_login = var.docker_hub_username != "" ? "crane auth login -u \"$DOCKER_HUB_USERNAME\" -p \"$DOCKER_HUB_PASSWORD\" index.docker.io" : "echo 'No Docker Hub credentials, using anonymous pulls'"

  crane_copies = join("\n", [
    for img in var.stress_images :
    "echo \"Copying ${img.image}...\" && crane copy \"docker.io/library/${img.image}\" \"${local.registry_host}/${img.image}\" --insecure"
  ])
}

resource "kubernetes_job_v1" "seed_registry" {
  metadata {
    name      = "seed-registry"
    namespace = kubernetes_namespace_v1.targets.metadata[0].name
  }

  spec {
    backoff_limit = 2

    template {
      metadata {
        labels = { app = "seed-registry" }
      }

      spec {
        restart_policy = "Never"

        # Wait for the registry to be reachable before copying.
        init_container {
          name    = "wait-for-registry"
          image   = "busybox:1.36"
          command = ["sh", "-c", "until wget -qO- http://${local.registry_host}/v2/ >/dev/null 2>&1; do echo 'waiting for registry...'; sleep 2; done"]
        }

        container {
          name  = "crane"
          image = "gcr.io/go-containerregistry/crane:latest"
          command = ["sh", "-c", <<-EOT
            set -e
            ${local.crane_login}
            ${local.crane_copies}
            echo "All images seeded."
          EOT
          ]

          dynamic "env" {
            for_each = var.docker_hub_username != "" ? [1] : []
            content {
              name  = "DOCKER_HUB_USERNAME"
              value = var.docker_hub_username
            }
          }

          dynamic "env" {
            for_each = var.docker_hub_username != "" ? [1] : []
            content {
              name  = "DOCKER_HUB_PASSWORD"
              value = var.docker_hub_password
            }
          }

          resources {
            requests = { memory = "64Mi", cpu = "100m" }
            limits   = { memory = "256Mi" }
          }
        }
      }
    }
  }

  wait_for_completion = true

  timeouts {
    create = "15m"
  }

  depends_on = [
    kubernetes_deployment_v1.registry,
    kubernetes_service_v1.registry,
  ]
}

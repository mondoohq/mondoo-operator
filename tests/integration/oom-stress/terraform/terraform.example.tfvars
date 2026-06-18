# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Copy to terraform.tfvars and fill in your values.
#
# Auth: set MONDOO_CONFIG_PATH to a service account file, or use cnspec login.
# The provider reads the API endpoint from the SA file automatically.

mondoo_org_id = "your-mondoo-org-id"

# To use an existing space instead of creating one:
# mondoo_space_id = "existing-space-id"

# Docker Hub credentials (required to avoid anonymous pull rate limits):
# docker_hub_username = "your-dockerhub-user"
# docker_hub_password = "dckr_pat_..."

# Optional overrides:
# kubeconfig_path       = "~/.kube/config"
# kubeconfig_context    = "minikube"
# operator_namespace    = "mondoo-operator"
# scanner_memory_limit  = "512Mi"   # lower = OOM faster (try 256Mi if 512Mi doesn't OOM)
# scan_schedule         = "*/5 * * * *"

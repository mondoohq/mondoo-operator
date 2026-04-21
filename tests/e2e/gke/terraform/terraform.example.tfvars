# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

project_id    = "your-gcp-project-id"
mondoo_org_id = "your-mondoo-organization-id"
region        = "europe-west3"
autopilot     = true

# Create a second GKE cluster as a scan target for external cluster testing
enable_target_cluster = false

# Create a mirror AR repo for registry mirroring/imagePullSecrets tests
enable_mirror_test = false
# Provision a Squid proxy VM for proxy tests (requires enable_mirror_test)
enable_proxy_test = false

# Enable GKE Workload Identity Federation for external cluster scanning
enable_wif_test = false

# Test org-level SA with spaceId routing (requires enable_target_cluster)
enable_space_splitting_test = false

# Test server-side asset routing rules (requires enable_target_cluster)
enable_asset_routing_test = false

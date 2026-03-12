# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

project_id    = "your-gcp-project-id"
mondoo_org_id = "your-mondoo-organization-id"
autopilot     = true

# Set to true to provision a mirror AR repo for registry mirroring/imagePullSecrets tests
enable_mirror_test = false
# Set to true to also provision a Squid proxy VM for proxy tests (requires enable_mirror_test)
enable_proxy_test  = false

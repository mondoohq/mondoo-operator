# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Mondoo space + service account + policy assignments for the OOM stress test.
# Policies match the SVA incident: mondoo-sbom, mondoo-linux-security, mondoo-edr-policy.
#
# Two modes:
#   1. Create a new space: set mondoo_org_id, leave mondoo_space_id empty
#   2. Use existing space: set mondoo_space_id (e.g. for local dev server)

resource "mondoo_space" "oom_stress" {
  count  = var.mondoo_space_id == "" ? 1 : 0
  name   = "oom-stress-test"
  org_id = var.mondoo_org_id
}

locals {
  space_id = var.mondoo_space_id != "" ? var.mondoo_space_id : mondoo_space.oom_stress[0].id
}

resource "mondoo_service_account" "oom_stress" {
  name        = "oom-stress-scanner"
  description = "Service account for OOM stress test container scanning"
  roles       = ["//iam.api.mondoo.app/roles/agent"]
  space_id    = local.space_id
}

resource "mondoo_policy_assignment" "sbom" {
  space_id = local.space_id

  policies = [
    "//policy.api.mondoo.app/policies/mondoo-sbom",
  ]

  state = "enabled"
}

resource "mondoo_policy_assignment" "linux_security" {
  space_id = local.space_id

  policies = [
    "//policy.api.mondoo.app/policies/mondoo-linux-security",
  ]

  state = "enabled"
}

resource "mondoo_policy_assignment" "edr" {
  space_id = local.space_id

  policies = [
    "//policy.api.mondoo.app/policies/mondoo-edr-policy",
  ]

  state = "enabled"
}

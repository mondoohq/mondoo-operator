# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

resource "mondoo_space" "e2e" {
  name   = "e2e-${local.name_prefix}"
  org_id = var.mondoo_org_id
}

resource "mondoo_service_account" "e2e" {
  name        = "e2e-operator-sa"
  description = "Service account for e2e testing of mondoo-operator"
  roles       = ["//iam.api.mondoo.app/roles/agent"]
  space_id    = mondoo_space.e2e.id

  depends_on = [mondoo_space.e2e]
}

################################################################################
# Space Splitting Test: second space + org-level service account
################################################################################

resource "mondoo_space" "target" {
  count  = (var.enable_space_splitting_test || var.enable_asset_routing_test) ? 1 : 0
  name   = "e2e-target-${local.name_prefix}"
  org_id = var.mondoo_org_id
}

resource "mondoo_service_account" "org" {
  count       = (var.enable_space_splitting_test || var.enable_asset_routing_test) ? 1 : 0
  name        = "e2e-org-sa"
  description = "Org-level service account for space splitting e2e test"
  roles       = ["//iam.api.mondoo.app/roles/agent"]
  org_id      = var.mondoo_org_id
}

################################################################################
# Asset Routing Test: developers space + routing table
################################################################################

resource "mondoo_space" "developers" {
  count  = var.enable_asset_routing_test ? 1 : 0
  name   = "e2e-developers-${local.name_prefix}"
  org_id = var.mondoo_org_id
}

resource "mondoo_asset_routing_table" "e2e" {
  count   = var.enable_asset_routing_test ? 1 : 0
  org_mrn = "//captain.api.mondoo.app/organizations/${var.mondoo_org_id}"

  # Priority 1: k8s workload label app=nginx-developers → developers space
  rule {
    target_space_mrn = mondoo_space.developers[0].mrn
    condition {
      field    = "LABEL"
      key      = "app"
      operator = "EQUAL"
      values   = ["nginx-developers"]
    }
  }

  # Priority 2: external cluster annotation (set by operator) → target space
  rule {
    target_space_mrn = mondoo_space.target[0].mrn
    condition {
      field    = "LABEL"
      key      = "mondoo.com/audit-config/cluster-name"
      operator = "EQUAL"
      values   = ["target-cluster"]
    }
  }

  # Catch-all → default e2e space
  rule {
    target_space_mrn = mondoo_space.e2e.mrn
  }

  depends_on = [mondoo_space.developers, mondoo_space.target, mondoo_space.e2e]
}

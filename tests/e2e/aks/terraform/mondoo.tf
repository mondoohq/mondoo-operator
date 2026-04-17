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

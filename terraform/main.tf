terraform {
  required_providers {
    mondoo = {
      source  = "mondoohq/mondoo"
      version = ">= 0.21"
    }
  }
}

provider "mondoo" {}

resource "mondoo_iam_binding" "team_permissions" {
  identity_mrn = "//captain.api.mondoo.app/users/29ytZiLLwFcxDXCrwXRPcm3BYsV"
  resource_mrn = "//captain.api.mondoo.app/organizations/mondoo-operator-testing"
  roles        = ["//iam.api.mondoo.app/roles/owner"]
}

#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Test: WIF External Cluster Scanning with Endpoint Override
# Validates that the optional `endpoint` field on WIF provider configs correctly
# overrides the API server URL in the generated kubeconfig. This tests the fix
# for split-horizon DNS issues (GitHub #1442).
#
# The infrastructure MUST be provisioned with enable_private_endpoint_access=false
# so that the scanner cluster cannot reach the target cluster's private endpoint.
# The scanner is forced to use the public endpoint via the `endpoint` field —
# if the override doesn't work, the scan fails with "i/o timeout".
#
# Prerequisites:
#   - Terraform provisioned with:
#       enable_target_cluster=true
#       enable_wif_test=true
#       enable_private_endpoint_access=false
#   - Cloud CLI authenticated, docker, helm, kubectl available
#
# Usage:
#   ./run-wif-endpoint-override.sh <cloud>    (eks|aks)

set -euo pipefail

CLOUD="${1:?Usage: $0 <cloud> (eks|aks)}"
export CLOUD_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/${CLOUD}" && pwd)"
source "${CLOUD_DIR}/../scripts/common.sh"

info "=========================================="
info "  Test: WIF Endpoint Override (${CLOUD})"
info "=========================================="

# Step 1: Load Terraform outputs
load_tf_outputs

if [[ "${ENABLE_TARGET_CLUSTER}" != "true" ]]; then
  die "Target cluster is not enabled. Run: terraform apply -var='enable_target_cluster=true' -var='enable_wif_test=true' -var='enable_private_endpoint_access=false'"
fi

if [[ "${ENABLE_WIF_TEST}" != "true" ]]; then
  die "WIF test is not enabled. Run: terraform apply -var='enable_wif_test=true' -var='enable_private_endpoint_access=false'"
fi

if [[ "${PRIVATE_ENDPOINT_ACCESS:-true}" == "true" ]]; then
  die "Private endpoint access is enabled. This test requires enable_private_endpoint_access=false so the scanner cannot reach the target via the private endpoint. Re-run: terraform apply -var='enable_private_endpoint_access=false'"
fi

if [[ -z "${TARGET_CLUSTER_ENDPOINT:-}" ]]; then
  die "TARGET_CLUSTER_ENDPOINT is not set. Ensure your Terraform outputs include target_cluster_endpoint."
fi

info "Target cluster endpoint: ${TARGET_CLUSTER_ENDPOINT}"

# Step 2: Build and push operator image
info "--- Step: Build and Push ---"
source "${E2E_DIR}/scripts/build-and-push.sh"

# Step 3: Deploy test workloads to scanner cluster (public + private-registry)
info "--- Step: Deploy Test Workloads (scanner cluster) ---"
source "${E2E_DIR}/scripts/deploy-test-workload.sh"
source "${E2E_DIR}/scripts/deploy-private-test-workload.sh"

# Step 4: Deploy operator from local chart
info "--- Step: Deploy Operator ---"
source "${E2E_DIR}/scripts/deploy-operator.sh"

# Step 5: Deploy workload to target cluster (no kubeconfig Secret — WIF handles auth)
info "--- Step: Deploy Target Workload ---"
source "${E2E_DIR}/scripts/deploy-target-workload-only.sh"

# Step 6: Setup WIF (cloud-specific RBAC)
info "--- Step: Setup WIF ---"
source "${E2E_DIR}/scripts/setup-wif.sh"

# Step 7: Apply MondooAuditConfig with endpoint override
info "--- Step: Apply Mondoo Config (with WIF + endpoint override) ---"
export ENABLE_WIF_TEST="true"
export ENABLE_ENDPOINT_OVERRIDE_TEST="true"
source "${E2E_DIR}/scripts/apply-mondoo-config.sh"

# Step 8: Wait for operator to reconcile (longer wait — init container needs cloud auth)
info "Waiting 120s for operator to reconcile and WIF init container..."
sleep 120

# Step 9: Verify local scanning
info "--- Step: Verify (local) ---"
source "${E2E_DIR}/scripts/verify.sh"

# Step 10: Verify WIF external cluster scanning with endpoint override
info "--- Step: Verify (WIF endpoint override) ---"
source "${E2E_DIR}/scripts/verify-wif-endpoint-override.sh"

# Step 11: Verify WIF container registry scanning
info "--- Step: Verify (WIF container registry) ---"
source "${E2E_DIR}/scripts/verify-wif-container-registry.sh"

info ""
info "=========================================="
info "  Test: WIF Endpoint Override (${CLOUD}) - COMPLETE"
info "=========================================="

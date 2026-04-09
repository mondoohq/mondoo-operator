#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Test: Space Splitting
# Deploys a SINGLE operator on the scanner cluster with TWO MondooAuditConfigs,
# each routing assets to a different Mondoo space using the same org-level SA:
#   - mondoo-scanner: scans the local (scanner) cluster → scanner space
#   - mondoo-target:  scans the remote (target) cluster → target space
#
# Prerequisites:
#   - Terraform provisioned with enable_target_cluster=true AND
#     enable_space_splitting_test=true
#   - Cloud CLI authenticated, docker, helm, kubectl available
#
# Usage:
#   ./run-space-splitting.sh <cloud>    (gke|eks|aks)

set -euo pipefail

CLOUD="${1:?Usage: $0 <cloud> (gke|eks|aks)}"
export CLOUD_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/${CLOUD}" && pwd)"
source "${CLOUD_DIR}/../scripts/common.sh"

info "=========================================="
info "  Test: Space Splitting (${CLOUD})"
info "=========================================="

# Step 1: Load Terraform outputs
load_tf_outputs

if [[ "${ENABLE_SPACE_SPLITTING_TEST}" != "true" ]]; then
  die "Space splitting test not enabled. Run terraform with enable_space_splitting_test=true"
fi
if [[ "${ENABLE_TARGET_CLUSTER}" != "true" ]]; then
  die "Target cluster not enabled. Run terraform with enable_target_cluster=true"
fi

# Step 2: Build and push operator image
info "--- Step: Build and Push ---"
source "${E2E_DIR}/scripts/build-and-push.sh"

# Step 3: Deploy test workload on scanner cluster
info "--- Step: Deploy Test Workload (scanner cluster) ---"
source "${E2E_DIR}/scripts/deploy-test-workload.sh"

# Step 4: Deploy operator on scanner cluster (single instance)
info "--- Step: Deploy Operator ---"
source "${E2E_DIR}/scripts/deploy-operator.sh"

# Step 5: Deploy workload on target cluster and create kubeconfig Secret
info "--- Step: Deploy Target Workload + Kubeconfig Secret ---"
source "${E2E_DIR}/scripts/deploy-target-workload.sh"

# Step 6: Apply both MondooAuditConfigs with org-level SA and space routing
info "--- Step: Apply Mondoo Configs (space splitting) ---"
source "${E2E_DIR}/scripts/apply-mondoo-config-space-splitting.sh"

# Step 7: Wait for operator to reconcile both configs
info "Waiting 90s for operator to reconcile both MondooAuditConfigs..."
sleep 90

# Step 8: Verify space splitting
info "--- Step: Verify Space Splitting ---"
source "${E2E_DIR}/scripts/verify-space-splitting.sh"

info ""
info "=========================================="
info "  Test: Space Splitting (${CLOUD}) - COMPLETE"
info "=========================================="
info ""
info "Scanner cluster scan → Space: ${SCANNER_SPACE_ID}"
info "Target cluster scan  → Space: ${TARGET_SPACE_ID}"
info "Both MondooAuditConfigs use the same org-level service account."

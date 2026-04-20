#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Test: Asset Routing
# Deploys a SINGLE operator on the scanner cluster with TWO MondooAuditConfigs
# using an org-level SA WITHOUT spaceId. Server-side asset routing rules
# direct assets to spaces based on inherent asset properties:
#   - mondoo-scanner: scans the local cluster → e2e space (catch-all)
#   - mondoo-target:  scans the remote cluster → target space (mondoo.com/cluster-name label)
#   - Workloads with app=nginx-developers k8s label → developers space
#
# Prerequisites:
#   - Terraform provisioned with enable_target_cluster=true AND
#     enable_asset_routing_test=true
#   - Cloud CLI authenticated, docker, helm, kubectl available
#
# Usage:
#   ./run-asset-routing.sh <cloud>    (gke|eks|aks)

set -euo pipefail

CLOUD="${1:?Usage: $0 <cloud> (gke|eks|aks)}"
export CLOUD_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/${CLOUD}" && pwd)"
source "${CLOUD_DIR}/../scripts/common.sh"

info "=========================================="
info "  Test: Asset Routing (${CLOUD})"
info "=========================================="

# Step 1: Load Terraform outputs
load_tf_outputs

if [[ "${ENABLE_ASSET_ROUTING_TEST}" != "true" ]]; then
  die "Asset routing test not enabled. Run terraform with enable_asset_routing_test=true"
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

# Step 4: Deploy workload in developers namespace (for routing test)
info "--- Step: Deploy Developers Workload ---"
source "${E2E_DIR}/scripts/deploy-developers-workload.sh"

# Step 5: Deploy operator on scanner cluster (single instance)
info "--- Step: Deploy Operator ---"
source "${E2E_DIR}/scripts/deploy-operator.sh"

# Step 6: Deploy workload on target cluster and create kubeconfig Secret
info "--- Step: Deploy Target Workload + Kubeconfig Secret ---"
source "${E2E_DIR}/scripts/deploy-target-workload.sh"

# Step 7: Apply MondooAuditConfigs with org-level SA (no spaceId, no annotations)
info "--- Step: Apply Mondoo Configs (asset routing) ---"
source "${E2E_DIR}/scripts/apply-mondoo-config-asset-routing.sh"

# Step 8: Wait for operator to reconcile both configs
info "Waiting 90s for operator to reconcile MondooAuditConfigs..."
sleep 90

# Step 9: Verify asset routing setup
info "--- Step: Verify Asset Routing ---"
source "${E2E_DIR}/scripts/verify-asset-routing.sh"

info ""
info "=========================================="
info "  Test: Asset Routing (${CLOUD}) - COMPLETE"
info "=========================================="
info ""
info "Routing rules (server-side):"
info "  1. app=nginx-developers → developers space"
info "  2. mondoo.com/cluster-name=<target> → target space"
info "  3. catch-all → e2e space"
info ""
info "All MondooAuditConfigs use org-level SA, no spaceId, no annotations."

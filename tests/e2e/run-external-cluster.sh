#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Test: External Cluster Scanning
# Builds operator from current branch, deploys to scanner cluster, configures
# external cluster scanning against a target cluster, and verifies scanning.
#
# Prerequisites:
#   - Terraform infrastructure provisioned with enable_target_cluster=true
#   - Cloud CLI authenticated, docker, helm, kubectl available
#
# Usage:
#   ./run-external-cluster.sh <cloud>    (gke|eks)

set -euo pipefail

CLOUD="${1:?Usage: $0 <cloud> (gke|eks)}"
export CLOUD_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/${CLOUD}" && pwd)"
source "${CLOUD_DIR}/../scripts/common.sh"

info "=========================================="
info "  Test: External Cluster Scanning (${CLOUD})"
info "=========================================="

# Step 1: Load Terraform outputs
load_tf_outputs

if [[ "${ENABLE_TARGET_CLUSTER}" != "true" ]]; then
  die "Target cluster is not enabled. Run: terraform apply -var='enable_target_cluster=true'"
fi

# Step 2: Build and push operator image
info "--- Step: Build and Push ---"
source "${E2E_DIR}/scripts/build-and-push.sh"

# Step 3: Deploy test workload to scanner cluster
info "--- Step: Deploy Test Workload (scanner cluster) ---"
source "${E2E_DIR}/scripts/deploy-test-workload.sh"

# Step 4: Deploy operator from local chart
info "--- Step: Deploy Operator ---"
source "${E2E_DIR}/scripts/deploy-operator.sh"

# Step 5: Deploy workload to target cluster and create kubeconfig Secret
info "--- Step: Deploy Target Workload + Kubeconfig Secret ---"
source "${E2E_DIR}/scripts/deploy-target-workload.sh"

# Step 6: Apply MondooAuditConfig with external clusters
info "--- Step: Apply Mondoo Config (with external clusters) ---"
source "${E2E_DIR}/scripts/apply-mondoo-config.sh"

# Step 7: Wait for operator to reconcile
info "Waiting 60s for operator to reconcile..."
sleep 60

# Step 8: Verify local scanning
info "--- Step: Verify (local) ---"
source "${E2E_DIR}/scripts/verify.sh"

# Step 9: Verify external cluster scanning
info "--- Step: Verify (external cluster) ---"
source "${E2E_DIR}/scripts/verify-external.sh"

info ""
info "=========================================="
info "  Test: External Cluster Scanning (${CLOUD}) - COMPLETE"
info "=========================================="

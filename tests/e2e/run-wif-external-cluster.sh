#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Test: GKE Workload Identity Federation External Cluster Scanning
# Builds operator from current branch, deploys to scanner cluster (with WIF enabled),
# configures WIF-based external cluster scanning against a target cluster, and verifies.
#
# Prerequisites:
#   - Terraform provisioned with enable_target_cluster=true enable_wif_test=true
#   - gcloud authenticated, docker, helm, kubectl available
#
# Usage:
#   ./run-wif-external-cluster.sh

set -euo pipefail

E2E_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${E2E_ROOT}/scripts/common.sh"

info "=========================================="
info "  Test: WIF External Cluster Scanning"
info "=========================================="

# Step 1: Load Terraform outputs (includes WIF_GSA_EMAIL)
load_tf_outputs

if [[ "${ENABLE_TARGET_CLUSTER}" != "true" ]]; then
  die "Target cluster is not enabled. Run: terraform apply -var='enable_target_cluster=true' -var='enable_wif_test=true'"
fi

if [[ "${ENABLE_WIF_TEST}" != "true" ]]; then
  die "WIF test is not enabled. Run: terraform apply -var='enable_wif_test=true'"
fi

# Step 2: Build and push operator image
info "--- Step: Build and Push ---"
source "${E2E_ROOT}/scripts/build-and-push.sh"

# Step 3: Deploy test workload to scanner cluster
info "--- Step: Deploy Test Workload (scanner cluster) ---"
source "${E2E_ROOT}/scripts/deploy-test-workload.sh"

# Step 4: Deploy operator from local chart
info "--- Step: Deploy Operator ---"
source "${E2E_ROOT}/scripts/deploy-operator.sh"

# Step 5: Deploy workload to target cluster (no kubeconfig Secret — WIF handles auth)
info "--- Step: Deploy Target Workload ---"
source "${E2E_ROOT}/scripts/deploy-target-workload-only.sh"

# Step 6: Setup WIF (RBAC on target cluster + CRD update)
info "--- Step: Setup WIF ---"
source "${E2E_ROOT}/scripts/setup-wif.sh"

# Step 7: Apply MondooAuditConfig with WIF
info "--- Step: Apply Mondoo Config (with WIF) ---"
export ENABLE_WIF_TEST="true"
source "${E2E_ROOT}/scripts/apply-mondoo-config.sh"

# Step 8: Wait for operator to reconcile (longer wait — init container needs gcloud auth)
info "Waiting 120s for operator to reconcile and WIF init container..."
sleep 120

# Step 9: Verify local scanning
info "--- Step: Verify (local) ---"
source "${E2E_ROOT}/scripts/verify.sh"

# Step 10: Verify WIF external cluster scanning
info "--- Step: Verify (WIF external cluster) ---"
source "${E2E_ROOT}/scripts/verify-wif-external.sh"

info ""
info "=========================================="
info "  Test: WIF External Cluster Scanning - COMPLETE"
info "=========================================="

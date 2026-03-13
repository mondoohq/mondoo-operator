#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Test: Vault-Authenticated External Cluster Scanning
# Builds operator from current branch, deploys Vault with Kubernetes auth + secrets
# engine, configures external cluster scanning via Vault, and verifies scanning.
#
# Prerequisites:
#   - Terraform infrastructure provisioned with enable_target_cluster=true
#   - gcloud authenticated, docker, helm, kubectl available
#
# Usage:
#   ./run-vault-external-cluster.sh

set -euo pipefail

E2E_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${E2E_ROOT}/scripts/common.sh"

info "=========================================="
info "  Test: Vault External Cluster Scanning"
info "=========================================="

# Step 1: Load Terraform outputs
load_tf_outputs

if [[ "${ENABLE_TARGET_CLUSTER}" != "true" ]]; then
  die "Target cluster is not enabled. Run: terraform apply -var='enable_target_cluster=true'"
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

# Step 5: Ensure CRDs include vaultAuth field (Helm doesn't upgrade CRDs)
info "--- Step: Update CRDs ---"
kubectl apply --server-side --force-conflicts -f "${REPO_ROOT}/config/crd/bases/"

# Step 6: Deploy test workload to target cluster (no kubeconfig Secret — Vault handles auth)
info "--- Step: Deploy Target Workload ---"
source "${E2E_ROOT}/scripts/deploy-target-workload-only.sh"

# Step 7: Deploy and configure Vault
info "--- Step: Deploy and Configure Vault ---"
source "${E2E_ROOT}/scripts/deploy-vault.sh"

# Step 8: Apply MondooAuditConfig with Vault auth
info "--- Step: Apply Mondoo Config (with Vault auth) ---"
export ENABLE_VAULT_TEST="true"
source "${E2E_ROOT}/scripts/apply-mondoo-config.sh"

# Step 9: Wait for operator to reconcile
info "Waiting 90s for operator to reconcile and Vault token fetch..."
sleep 90

# Step 10: Verify local scanning
info "--- Step: Verify (local) ---"
source "${E2E_ROOT}/scripts/verify.sh"

# Step 11: Verify Vault-based external cluster scanning
info "--- Step: Verify (Vault external cluster) ---"
source "${E2E_ROOT}/scripts/verify-vault-external.sh"

info ""
info "=========================================="
info "  Test: Vault External Cluster Scanning - COMPLETE"
info "=========================================="

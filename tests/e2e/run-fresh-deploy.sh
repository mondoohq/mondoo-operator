#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Test: Fresh Deploy
# Builds operator from current branch, deploys to cluster, verifies scanning.
#
# Prerequisites:
#   - Terraform infrastructure provisioned (cd <cloud>/terraform && terraform apply)
#   - Cloud CLI authenticated, docker, helm, kubectl available
#
# Usage:
#   ./run-fresh-deploy.sh <cloud>    (gke|eks|aks)

set -euo pipefail

CLOUD="${1:?Usage: $0 <cloud> (gke|eks|aks)}"
export CLOUD_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/${CLOUD}" && pwd)"
source "${CLOUD_DIR}/../scripts/common.sh"

info "=========================================="
info "  Test: Fresh Deploy (${CLOUD})"
info "=========================================="

# Step 1: Load Terraform outputs
load_tf_outputs

# Step 2: Build and push operator image
info "--- Step: Build and Push ---"
source "${E2E_DIR}/scripts/build-and-push.sh"

# Step 3: Deploy test workload
info "--- Step: Deploy Test Workload ---"
source "${E2E_DIR}/scripts/deploy-test-workload.sh"

# Step 4: Deploy operator from local chart
info "--- Step: Deploy Operator ---"
source "${E2E_DIR}/scripts/deploy-operator.sh"

# Step 5: Apply MondooAuditConfig
info "--- Step: Apply Mondoo Config ---"
source "${E2E_DIR}/scripts/apply-mondoo-config.sh"

# Step 6: Wait for operator to reconcile
info "Waiting 60s for operator to reconcile..."
sleep 60

# Step 7: Verify
info "--- Step: Verify ---"
source "${E2E_DIR}/scripts/verify.sh"

info ""
info "=========================================="
info "  Test: Fresh Deploy (${CLOUD}) - COMPLETE"
info "=========================================="

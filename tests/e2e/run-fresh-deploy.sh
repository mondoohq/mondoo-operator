#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Test 1.0: Fresh Deploy
# Builds operator from current branch, deploys to GKE Autopilot, verifies scanning.
#
# Prerequisites:
#   - Terraform infrastructure provisioned (cd terraform && terraform apply)
#   - gcloud authenticated, docker, helm, kubectl available
#
# Usage:
#   ./run-fresh-deploy.sh

set -euo pipefail

E2E_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${E2E_ROOT}/scripts/common.sh"

info "=========================================="
info "  Test 1.0: Fresh Deploy"
info "=========================================="

# Step 1: Load Terraform outputs
load_tf_outputs

# Step 2: Build and push operator image
info "--- Step: Build and Push ---"
source "${E2E_ROOT}/scripts/build-and-push.sh"

# Step 3: Deploy test workload
info "--- Step: Deploy Test Workload ---"
source "${E2E_ROOT}/scripts/deploy-test-workload.sh"

# Step 4: Deploy operator from local chart
info "--- Step: Deploy Operator ---"
source "${E2E_ROOT}/scripts/deploy-operator.sh"

# Step 4: Apply MondooAuditConfig
info "--- Step: Apply Mondoo Config ---"
source "${E2E_ROOT}/scripts/apply-mondoo-config.sh"

# Step 5: Wait for operator to reconcile
info "Waiting 60s for operator to reconcile..."
sleep 60

# Step 6: Verify
info "--- Step: Verify ---"
source "${E2E_ROOT}/scripts/verify.sh"

info ""
info "=========================================="
info "  Test 1.0: Fresh Deploy - COMPLETE"
info "=========================================="

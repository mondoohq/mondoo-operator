#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Test 2: Upgrade
# Deploys a baseline released version, verifies, upgrades to current branch, verifies again.
#
# Prerequisites:
#   - Terraform infrastructure provisioned (cd terraform && terraform apply)
#   - gcloud authenticated, docker, helm, kubectl available
#
# Usage:
#   ./run-upgrade.sh <baseline-version>
#   ./run-upgrade.sh 12.0.1

set -euo pipefail

E2E_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${E2E_ROOT}/scripts/common.sh"

export BASELINE_VERSION="${1:?Usage: $0 <baseline-version>}"

info "=========================================="
info "  Test 2: Upgrade (baseline: v${BASELINE_VERSION})"
info "=========================================="

# Step 1: Load Terraform outputs
load_tf_outputs

# Step 2: Build and push operator image (so it's ready for upgrade)
info "--- Step: Build and Push ---"
source "${E2E_ROOT}/scripts/build-and-push.sh"

# Step 3: Deploy test workload
info "--- Step: Deploy Test Workload ---"
source "${E2E_ROOT}/scripts/deploy-test-workload.sh"

# Step 4: Deploy baseline released version
info "--- Step: Deploy Baseline v${BASELINE_VERSION} ---"
source "${E2E_ROOT}/scripts/deploy-baseline.sh"

# Step 4: Apply MondooAuditConfig
info "--- Step: Apply Mondoo Config ---"
source "${E2E_ROOT}/scripts/apply-mondoo-config.sh"

# Step 5: Wait and verify baseline
info "Waiting 60s for baseline operator to reconcile..."
sleep 60

info "--- Step: Verify Baseline ---"
source "${E2E_ROOT}/scripts/verify.sh"

info ""
info "=========================================="
info "  Upgrading to current branch..."
info "=========================================="

# Step 6: Upgrade to current branch image
info "--- Step: Deploy Operator (Upgrade) ---"
source "${E2E_ROOT}/scripts/deploy-operator.sh"

# Step 7: Wait and verify upgrade
info "Waiting 60s for upgraded operator to reconcile..."
sleep 60

info "--- Step: Verify Upgrade ---"
source "${E2E_ROOT}/scripts/verify.sh"

info ""
info "=========================================="
info "  Test 2: Upgrade - COMPLETE"
info "=========================================="

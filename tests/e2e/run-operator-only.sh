#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Test: Operator-Only Deploy
# Builds operator from current branch and deploys to cluster.
# Does NOT create a MondooAuditConfig — apply one manually from the UI.
#
# Prerequisites:
#   - Terraform infrastructure provisioned (cd <cloud>/terraform && terraform apply)
#   - Cloud CLI authenticated, docker, helm, kubectl available
#
# Usage:
#   ./run-operator-only.sh <cloud>    (gke|eks)

set -euo pipefail

CLOUD="${1:?Usage: $0 <cloud> (gke|eks)}"
export CLOUD_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/${CLOUD}" && pwd)"
source "${CLOUD_DIR}/../scripts/common.sh"

info "=========================================="
info "  Test: Operator-Only Deploy (${CLOUD})"
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

info ""
info "=========================================="
info "  Operator deployed. Apply AuditConfig from the UI."
info "=========================================="

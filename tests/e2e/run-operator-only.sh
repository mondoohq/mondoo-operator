#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Test: Operator-Only Deploy
# Builds operator from current branch and deploys to GKE Autopilot.
# Does NOT create a MondooAuditConfig — apply one manually from the UI.
#
# Prerequisites:
#   - Terraform infrastructure provisioned (cd terraform && terraform apply)
#   - gcloud authenticated, docker, helm, kubectl available
#
# Usage:
#   ./run-operator-only.sh

set -euo pipefail

E2E_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${E2E_ROOT}/scripts/common.sh"

info "=========================================="
info "  Test: Operator-Only Deploy"
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

info ""
info "=========================================="
info "  Operator deployed. Apply AuditConfig from the UI."
info "=========================================="

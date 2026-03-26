#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Test: Registry Mirroring, imagePullSecrets & Proxy
# Builds operator, deploys with mirror registry and optional proxy,
# verifies image references, pull secrets, and proxy configuration.
#
# Prerequisites:
#   - Terraform infrastructure provisioned with enable_mirror_test=true
#   - crane CLI installed (go install github.com/google/go-containerregistry/cmd/crane@latest)
#   - Cloud CLI authenticated, docker, helm, kubectl available
#   - For proxy testing: also set enable_proxy_test=true in terraform
#
# Usage:
#   ./run-registry-mirroring.sh <cloud>    (gke|eks|aks)

set -euo pipefail

CLOUD="${1:?Usage: $0 <cloud> (gke|eks|aks)}"
export CLOUD_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/${CLOUD}" && pwd)"
source "${CLOUD_DIR}/../scripts/common.sh"

info "=========================================="
info "  Test: Registry Mirroring & Proxy (${CLOUD})"
info "=========================================="

# Step 1: Load Terraform outputs (includes mirror and proxy outputs)
load_tf_outputs

# Validate that mirror test infra is provisioned
if [[ "${ENABLE_MIRROR_TEST}" != "true" ]]; then
  die "This test requires enable_mirror_test=true in Terraform. Run: terraform apply -var='enable_mirror_test=true'"
fi

# Step 2: Build and push operator image
info "--- Step: Build and Push ---"
source "${E2E_DIR}/scripts/build-and-push.sh"

# Step 3: Create imagePullSecret for mirror repo
info "--- Step: Setup Mirror Registry Credentials ---"
source "${E2E_DIR}/scripts/setup-mirror-registry.sh"

# Step 4: Populate mirror repo with cnspec image
info "--- Step: Populate Mirror Registry ---"
source "${E2E_DIR}/scripts/populate-mirror-registry.sh"

# Step 5: Deploy test workload
info "--- Step: Deploy Test Workload ---"
source "${E2E_DIR}/scripts/deploy-test-workload.sh"

# Step 6: Set proxy env vars from Terraform if enabled
if [[ "${ENABLE_PROXY_TEST}" == "true" ]]; then
  info "Proxy testing enabled — Squid proxy at ${SQUID_PROXY_IP}"
else
  info "Proxy testing disabled — skipping proxy configuration"
fi

# Step 7: Deploy operator with mirroring/proxy configuration
info "--- Step: Deploy Operator with Mirroring ---"
source "${E2E_DIR}/scripts/deploy-operator-mirroring.sh"

# Step 8: Apply MondooAuditConfig
info "--- Step: Apply Mondoo Config ---"
source "${E2E_DIR}/scripts/apply-mondoo-config.sh"

# Step 9: Wait for operator to reconcile
info "Waiting 90s for operator to reconcile with mirrored images..."
sleep 90

# Step 10: Standard verification
info "--- Step: Standard Verify ---"
source "${E2E_DIR}/scripts/verify.sh"

# Step 11: Mirroring-specific verification
info "--- Step: Verify Mirroring & Proxy ---"
source "${E2E_DIR}/scripts/verify-mirroring.sh"

info ""
info "=========================================="
info "  Test: Registry Mirroring & Proxy (${CLOUD}) - COMPLETE"
info "=========================================="

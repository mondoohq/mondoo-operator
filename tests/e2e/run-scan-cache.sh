#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Test: Scan Cache (server-side score refresh for container images)
#
# Sets up the environment: builds operator + optional dev cnspec, pushes a
# mutable-tag test image, deploys workload, deploys operator, and applies
# the scan cache MondooAuditConfig.
#
# After setup completes, use the helper scripts manually:
#   ./scripts/swap-scan-cache-workload.sh   — re-push same tag with new digest
#   ./scripts/verify-scan-cache.sh          — check inventory + scan logs
#
# Prerequisites:
#   - Terraform infrastructure provisioned (cd <cloud>/terraform && terraform apply)
#   - Cloud CLI authenticated, docker, helm, kubectl available
#
# Optional:
#   - CNSPEC_REPO=<path>   Build dev cnspec image from local repo
#   - MQL_REPO=<path>      Build providers from local mql repo and bake into
#                           the cnspec image (requires CNSPEC_REPO)
#   - MQL_PROVIDERS="k8s os"  Which providers to build (default: "k8s os")
#
# Usage:
#   ./run-scan-cache.sh <cloud>
#   CNSPEC_REPO=~/repos/cnspec ./run-scan-cache.sh <cloud>
#   MQL_REPO=~/repos/mql CNSPEC_REPO=~/repos/cnspec ./run-scan-cache.sh <cloud>

set -euo pipefail

CLOUD="${1:?Usage: $0 <cloud> (gke|eks|aks)}"
export CLOUD_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/${CLOUD}" && pwd)"
source "${CLOUD_DIR}/../scripts/common.sh"

info "=========================================="
info "  Test: Scan Cache — Setup (${CLOUD})"
info "=========================================="

# Step 1: Load Terraform outputs
load_tf_outputs

# Step 2: Build and push operator image
info "--- Step: Build and Push Operator ---"
source "${E2E_DIR}/scripts/build-and-push.sh"

# Step 3: Build and push dev cnspec image (if CNSPEC_REPO is set)
if [[ -n "${CNSPEC_REPO:-}" ]]; then
  info "--- Step: Build and Push Dev cnspec ---"
  source "${E2E_DIR}/scripts/build-and-push-cnspec.sh"
else
  info "--- Step: Using official cnspec image (no CNSPEC_REPO set) ---"
  export CNSPEC_IMAGE_NAME=""
  export CNSPEC_IMAGE_TAG=""
fi

# Step 4: Build and push mutable-tag test image
info "--- Step: Build Cache Test Image ---"
source "${E2E_DIR}/scripts/build-cache-test-image.sh"

# Step 5: Deploy test workload
info "--- Step: Deploy Scan Cache Workload ---"
source "${E2E_DIR}/scripts/deploy-scan-cache-workload.sh"

# Step 5b: Create pull secret for private Artifact Registry
info "--- Step: Setup Pull Secret ---"
source "${E2E_DIR}/scripts/setup-scan-cache-pull-secret.sh"

# Step 5c: Deploy workloads on target cluster (if enabled)
if [[ "${ENABLE_TARGET_CLUSTER:-}" == "true" ]]; then
  info "--- Step: Deploy Cache Test Workloads on Target Cluster ---"
  source "${E2E_DIR}/scripts/deploy-scan-cache-target-workload.sh"
fi

# Step 6: Deploy operator
info "--- Step: Deploy Operator ---"
if [[ -n "${CNSPEC_IMAGE_NAME:-}" ]]; then
  source "${E2E_DIR}/scripts/deploy-operator-scan-cache.sh"
else
  source "${E2E_DIR}/scripts/deploy-operator.sh"
fi

# Step 7: Apply scan cache MondooAuditConfig
info "--- Step: Apply Scan Cache Config ---"
source "${E2E_DIR}/scripts/apply-mondoo-config-scan-cache.sh"

info ""
info "=========================================="
info "  Setup complete. Scan CronJob is scheduled."
info ""
info "  Useful commands:"
info "    kubectl get cronjobs -n ${NAMESPACE}"
info "    kubectl get jobs -n ${NAMESPACE} -w"
info "    kubectl logs job/<JOB_NAME> -n ${NAMESPACE}"
info ""
info "  After a scan completes:"
info "    NAMESPACE=${NAMESPACE} ./scripts/verify-scan-cache.sh"
info ""
info "  To rebuild cnspec and redeploy (iterate on cnspec changes):"
info "    CNSPEC_REPO=${CNSPEC_REPO:-~/repos/cnspec} REGISTRY_REPO=${REGISTRY_REPO} REGION=${REGION} ./scripts/redeploy-cnspec.sh"
info ""
info "  To simulate a digest change (same tag, new content):"
info "    REGISTRY_REPO=${REGISTRY_REPO} REGION=${REGION} ./scripts/swap-scan-cache-workload.sh"
info ""
info "  Then wait for the next scan and re-run verify."
info "=========================================="

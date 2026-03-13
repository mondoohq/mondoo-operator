#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Deploy a test nginx workload to the target cluster WITHOUT creating a kubeconfig
# Secret. Used by the Vault test where auth is handled by Vault's Kubernetes
# secrets engine instead of a static kubeconfig.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

: "${TARGET_CLUSTER_NAME:?TARGET_CLUSTER_NAME must be set}"
: "${TARGET_KUBECONFIG_PATH:?TARGET_KUBECONFIG_PATH must be set}"
: "${REGION:?REGION must be set}"
: "${PROJECT_ID:?PROJECT_ID must be set}"

# Refresh target cluster credentials so the kubeconfig has a fresh token
info "Refreshing target cluster credentials..."
KUBECONFIG="${TARGET_KUBECONFIG_PATH}" \
  gcloud container clusters get-credentials "${TARGET_CLUSTER_NAME}" \
  --region "${REGION}" --project "${PROJECT_ID}" --quiet

info "Deploying nginx test workload to target cluster..."
kubectl --kubeconfig "${TARGET_KUBECONFIG_PATH}" apply -f "${E2E_DIR}/manifests/nginx-workload.yaml"

info "Waiting for nginx deployment on target cluster..."
kubectl --kubeconfig "${TARGET_KUBECONFIG_PATH}" rollout status deployment/nginx-test-workload \
  -n default --timeout=300s

info "Target cluster workload deployed."

#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Deploy a test nginx workload to the target cluster and create kubeconfig Secret
# in the scanner cluster for external cluster scanning.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

: "${TARGET_KUBECONFIG_PATH:?TARGET_KUBECONFIG_PATH must be set}"
: "${NAMESPACE:?NAMESPACE must be set}"

# Refresh target cluster credentials (cloud-specific)
refresh_target_credentials

info "Deploying nginx test workload to target cluster..."
kubectl --kubeconfig "${TARGET_KUBECONFIG_PATH}" apply -f "${SHARED_MANIFESTS_DIR}/nginx-workload.yaml"

info "Waiting for nginx deployment on target cluster..."
kubectl --kubeconfig "${TARGET_KUBECONFIG_PATH}" rollout status deployment/nginx-test-workload \
  -n default --timeout=300s

info "Creating target-kubeconfig Secret in scanner cluster..."
kubectl create secret generic target-kubeconfig \
  --namespace "${NAMESPACE}" \
  --from-file=kubeconfig="${TARGET_KUBECONFIG_PATH}" \
  --dry-run=client -o yaml | kubectl apply -f -

info "Target cluster workload deployed and kubeconfig Secret created."

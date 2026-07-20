#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Deploy cache-test workloads on the target (external) cluster:
#   1. Same image as the scanner cluster (tests cross-cluster dedup)
#   2. A different image (tests that new images are scanned)
#
# Also creates the kubeconfig Secret in the scanner cluster so the operator
# can reach the target.
#
# Requires:
#   CACHE_TEST_IMAGE         — same image deployed on scanner cluster
#   TARGET_KUBECONFIG_PATH   — path to target cluster kubeconfig
#   NAMESPACE                — operator namespace (for kubeconfig Secret)

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

: "${CACHE_TEST_IMAGE:?CACHE_TEST_IMAGE must be set}"
: "${TARGET_KUBECONFIG_PATH:?TARGET_KUBECONFIG_PATH must be set}"
: "${NAMESPACE:?NAMESPACE must be set}"

refresh_target_credentials

info "Deploying cache-test workloads on target cluster..."

# Same image as scanner cluster — should be deduplicated
export CACHE_TEST_IMAGE
envsubst < "${SHARED_MANIFESTS_DIR}/scan-cache-workload.yaml.tpl" \
  | kubectl --kubeconfig "${TARGET_KUBECONFIG_PATH}" apply -f -

# Different image — should always be scanned
kubectl --kubeconfig "${TARGET_KUBECONFIG_PATH}" apply -f - <<'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: cache-test-unique
  namespace: default
  labels:
    app: cache-test-unique
spec:
  replicas: 1
  selector:
    matchLabels:
      app: cache-test-unique
  template:
    metadata:
      labels:
        app: cache-test-unique
    spec:
      containers:
      - name: app
        image: alpine:3.20
        command: ["sleep", "infinity"]
EOF

kubectl --kubeconfig "${TARGET_KUBECONFIG_PATH}" rollout status deployment/cache-test -n default --timeout=120s
kubectl --kubeconfig "${TARGET_KUBECONFIG_PATH}" rollout status deployment/cache-test-unique -n default --timeout=120s

info "Creating target-kubeconfig Secret in scanner cluster..."
kubectl create secret generic target-kubeconfig \
  --namespace "${NAMESPACE}" \
  --from-file=kubeconfig="${TARGET_KUBECONFIG_PATH}" \
  --dry-run=client -o yaml | kubectl apply -f -

info "Target cluster workloads deployed."

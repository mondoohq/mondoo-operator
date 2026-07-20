#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Deploy a test workload that uses a mutable-tag image from Artifact Registry.
# Can be run standalone (requires kubectl context and CACHE_TEST_IMAGE).
#
# Requires:
#   CACHE_TEST_IMAGE  — full image ref (e.g. <AR_REPO>/cache-test:latest)

set -euo pipefail

_info()  { echo "[INFO]  $(date '+%H:%M:%S') $*"; }

: "${CACHE_TEST_IMAGE:?CACHE_TEST_IMAGE must be set (run build-cache-test-image.sh first)}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MANIFESTS_DIR="$(cd "${SCRIPT_DIR}/../manifests" && pwd)"

_info "Deploying scan cache test workload (image: ${CACHE_TEST_IMAGE})..."
export CACHE_TEST_IMAGE
envsubst < "${MANIFESTS_DIR}/scan-cache-workload.yaml.tpl" | kubectl apply -f -

kubectl rollout status deployment/cache-test -n default --timeout=120s

DIGEST=$(kubectl get pods -n default -l app=cache-test \
  -o jsonpath='{.items[0].status.containerStatuses[0].imageID}' 2>/dev/null || echo "pending")
_info "cache-test workload running (imageID: ${DIGEST})"

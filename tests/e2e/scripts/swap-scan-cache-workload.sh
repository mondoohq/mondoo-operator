#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Simulate a digest change WITHOUT changing the tag — the :latest scenario.
# Re-builds the cache test image (new content → new digest), pushes to the
# same tag, then restarts the deployment so the kubelet pulls the new digest.
#
# Can be run standalone — only needs REGISTRY_REPO, REGION, and kubectl context.
#
# Requires:
#   REGISTRY_REPO  — cloud container registry
#   REGION         — cloud region (for Docker auth)

set -euo pipefail

_info()  { echo "[INFO]  $(date '+%H:%M:%S') $*"; }
_warn()  { echo "[WARN]  $(date '+%H:%M:%S') $*" >&2; }

: "${REGISTRY_REPO:?REGISTRY_REPO must be set}"
: "${REGION:?REGION must be set}"

export CACHE_TEST_IMAGE="${REGISTRY_REPO}/cache-test:latest"

OLD_DIGEST=$(kubectl get pods -n default -l app=cache-test \
  -o jsonpath='{.items[0].status.containerStatuses[0].imageID}' 2>/dev/null || echo "unknown")
_info "Current digest: ${OLD_DIGEST}"

# Re-build and push the same tag with different content
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
_info "Re-pushing ${CACHE_TEST_IMAGE} with new content..."
source "${SCRIPT_DIR}/build-cache-test-image.sh"

# Restart the deployment to pull the new digest (imagePullPolicy: Always)
_info "Restarting cache-test deployment to pick up new digest..."
kubectl rollout restart deployment/cache-test -n default
kubectl rollout status deployment/cache-test -n default --timeout=120s

sleep 5
NEW_DIGEST=$(kubectl get pods -n default -l app=cache-test \
  -o jsonpath='{.items[0].status.containerStatuses[0].imageID}' 2>/dev/null || echo "pending")
_info "New digest: ${NEW_DIGEST}"

if [[ "${OLD_DIGEST}" == "${NEW_DIGEST}" ]]; then
  _warn "Digest did not change — cache test may not detect a miss."
else
  _info "Digest changed — next scan should show a cache miss for this image."
fi

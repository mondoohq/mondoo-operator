#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Build a minimal test image and push to the cloud registry with a fixed tag.
# Calling this again produces a new digest under the same tag — simulating a
# "latest" repush that should invalidate the SBOM cache.
#
# Sets CACHE_TEST_IMAGE for use by other scripts.
#
# Can be run standalone or sourced from a runner script.
#
# Requires:
#   REGISTRY_REPO  — cloud container registry
#   REGION         — cloud region (for Docker auth)

set -euo pipefail

_info()  { echo "[INFO]  $(date '+%H:%M:%S') $*"; }

: "${REGISTRY_REPO:?REGISTRY_REPO must be set}"
: "${REGION:?REGION must be set}"

export CACHE_TEST_IMAGE="${REGISTRY_REPO}/cache-test:latest"

BUILD_ID="build-$(date +%s)-${RANDOM}"

_info "Building cache test image (build_id=${BUILD_ID})..."

TMPDIR_BUILD=$(mktemp -d)
trap 'rm -rf "${TMPDIR_BUILD}"' EXIT

cat > "${TMPDIR_BUILD}/Dockerfile" <<'DEOF'
FROM alpine:3.20
ARG BUILD_ID=unknown
RUN echo "${BUILD_ID}" > /build-id
RUN apk add --no-cache coreutils
CMD ["sleep", "infinity"]
DEOF

docker build --platform=linux/amd64 \
  --build-arg "BUILD_ID=${BUILD_ID}" \
  -t "${CACHE_TEST_IMAGE}" \
  "${TMPDIR_BUILD}"

_info "Configuring Docker for Artifact Registry..."
gcloud auth configure-docker "${REGION}-docker.pkg.dev" --quiet

_info "Pushing ${CACHE_TEST_IMAGE} ..."
docker push "${CACHE_TEST_IMAGE}"

DIGEST=$(docker inspect --format='{{index .RepoDigests 0}}' "${CACHE_TEST_IMAGE}" 2>/dev/null \
  | sed 's/.*@//' || echo "unknown")
_info "Pushed ${CACHE_TEST_IMAGE} (digest: ${DIGEST})"

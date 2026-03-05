#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Build the operator image from the current branch and push to Artifact Registry

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

: "${AR_REPO:?AR_REPO must be set}"
: "${GIT_SHA:?GIT_SHA must be set}"
: "${REGION:?REGION must be set}"

export OPERATOR_IMAGE="${AR_REPO}/mondoo-operator:${GIT_SHA}"

info "Building operator image: ${OPERATOR_IMAGE}"
cd "${REPO_ROOT}"

make docker-build IMG="${OPERATOR_IMAGE}" TARGET_ARCH=amd64

info "Configuring Docker for Artifact Registry..."
gcloud auth configure-docker "${REGION}-docker.pkg.dev" --quiet

info "Pushing image..."
docker push "${OPERATOR_IMAGE}"

info "Image pushed: ${OPERATOR_IMAGE}"

# Export for downstream scripts
export IMAGE_REPO="${AR_REPO}/mondoo-operator"
export IMAGE_TAG="${GIT_SHA}"

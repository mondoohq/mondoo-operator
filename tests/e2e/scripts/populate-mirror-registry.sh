#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Copy cnspec image from ghcr.io into the mirror Artifact Registry repo.
# Uses crane locally with gcloud auth (caller is already authenticated to GCP).
#
# The operator image is pulled directly from the main AR repo (set via Helm
# --set values) and does NOT go through the mirror. Only images the operator
# spawns (cnspec) get resolved through registryMirrors.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

: "${MIRROR_REGISTRY:?MIRROR_REGISTRY must be set}"
: "${REGION:?REGION must be set}"

# cnspec image constants (must match pkg/utils/mondoo/container_image_resolver.go)
CNSPEC_IMAGE="ghcr.io/mondoohq/mondoo-operator/cnspec"
CNSPEC_TAG="12-rootless"

CNSPEC_SRC="${CNSPEC_IMAGE}:${CNSPEC_TAG}"
# Mirror path preserves the ghcr.io path structure so registryMirrors mapping works:
#   ghcr.io/mondoohq/mondoo-operator/cnspec:tag -> MIRROR_REGISTRY/mondoohq/mondoo-operator/cnspec:tag
CNSPEC_MIRROR="${MIRROR_REGISTRY}/mondoohq/mondoo-operator/cnspec:${CNSPEC_TAG}"

info "Populating mirror registry..."
info "  cnspec: ${CNSPEC_SRC} -> ${CNSPEC_MIRROR}"

# Verify crane is available
if ! command -v crane &>/dev/null; then
  die "crane CLI is required. Install with: go install github.com/google/go-containerregistry/cmd/crane@latest"
fi

# Configure crane auth for the mirror AR repo (uses gcloud credentials)
info "Configuring crane auth for Artifact Registry..."
crane auth login "${REGION}-docker.pkg.dev" -u oauth2accesstoken -p "$(gcloud auth print-access-token)"

# Copy cnspec image from ghcr.io (public) to mirror AR repo
info "Copying cnspec image to mirror..."
crane copy "${CNSPEC_SRC}" "${CNSPEC_MIRROR}"

info "Mirror registry populated successfully."

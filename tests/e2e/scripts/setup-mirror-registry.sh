#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Create the imagePullSecret for the mirror Artifact Registry repo.
# The mirror AR repo and a read-only service account are provisioned by Terraform.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

: "${NAMESPACE:?NAMESPACE must be set}"
: "${MIRROR_REGISTRY:?MIRROR_REGISTRY must be set}"
: "${MIRROR_SA_KEY_B64:?MIRROR_SA_KEY_B64 must be set}"
: "${REGION:?REGION must be set}"

info "Setting up mirror registry credentials..."

# The mirror AR repo URL is REGION-docker.pkg.dev, extract the server
MIRROR_SERVER="${REGION}-docker.pkg.dev"

# Decode the GCP SA key (Terraform outputs it as base64)
SA_KEY_JSON=$(echo "${MIRROR_SA_KEY_B64}" | base64 -d)

# Create docker-registry secret in operator namespace for imagePullSecrets
info "Creating mirror-registry-creds secret in ${NAMESPACE}..."
kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -
kubectl create secret docker-registry mirror-registry-creds \
  --namespace "${NAMESPACE}" \
  --docker-server="${MIRROR_SERVER}" \
  --docker-username="_json_key" \
  --docker-password="${SA_KEY_JSON}" \
  --dry-run=client -o yaml | kubectl apply -f -

info "Mirror registry credentials created for ${MIRROR_SERVER}"

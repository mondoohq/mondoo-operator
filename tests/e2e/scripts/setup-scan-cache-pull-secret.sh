#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Create a docker-registry pull secret so the container-image scanner can
# authenticate to the private Artifact Registry and fetch manifests.
# Without this, only publicly accessible images are scanned.
#
# On GKE this uses a short-lived access token from the active gcloud credential.
# On EKS/AKS the cloud-specific helper must be sourced first.
#
# Requires:
#   NAMESPACE      — operator namespace
#   REGION         — cloud region (for Docker server URL)
#   CLOUD_PROVIDER — gke|eks|aks

set -euo pipefail

_info()  { echo "[INFO]  $(date '+%H:%M:%S') $*"; }

: "${NAMESPACE:?NAMESPACE must be set}"
: "${REGION:?REGION must be set}"
: "${CLOUD_PROVIDER:?CLOUD_PROVIDER must be set}"

SECRET_NAME="ar-pull-secret"
export PULL_SECRET_NAME="${SECRET_NAME}"

case "${CLOUD_PROVIDER}" in
  gke)
    DOCKER_SERVER="${REGION}-docker.pkg.dev"
    DOCKER_PASSWORD="$(gcloud auth print-access-token)"
    DOCKER_USERNAME="oauth2accesstoken"
    ;;
  *)
    _info "Pull secret setup not implemented for ${CLOUD_PROVIDER}, skipping"
    export PULL_SECRET_NAME=""
    exit 0
    ;;
esac

_info "Creating pull secret '${SECRET_NAME}' in ${NAMESPACE} for ${DOCKER_SERVER}..."
kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -
kubectl create secret docker-registry "${SECRET_NAME}" \
  --namespace "${NAMESPACE}" \
  --docker-server="${DOCKER_SERVER}" \
  --docker-username="${DOCKER_USERNAME}" \
  --docker-password="${DOCKER_PASSWORD}" \
  --dry-run=client -o yaml | kubectl apply -f -

_info "Pull secret created: ${SECRET_NAME}"

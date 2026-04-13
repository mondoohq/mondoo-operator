#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Push an nginx image to the cloud-managed container registry and deploy a
# workload that references it WITHOUT imagePullSecrets on both the scanner
# and target clusters. This lets us verify that:
# 1. The container-image scanner uses WIF to authenticate to the registry
# 2. The external cluster scanner can scan private images on the target cluster
#
# Requires: REGISTRY_REPO, CLOUD_PROVIDER, and cloud-specific vars set by
# common-{gke,eks,aks}.sh.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

: "${REGISTRY_REPO:?REGISTRY_REPO must be set}"

# ECR repos are flat (no nesting), so EKS uses a dedicated repo.
# GKE and AKS support nested image paths under the registry.
case "${CLOUD_PROVIDER}" in
  eks)
    : "${PRIVATE_TEST_ECR_REPO:?PRIVATE_TEST_ECR_REPO must be set for EKS WIF tests}"
    PRIVATE_IMAGE="${PRIVATE_TEST_ECR_REPO}:stable"
    ;;
  *)
    PRIVATE_IMAGE="${REGISTRY_REPO}/nginx-private:stable"
    ;;
esac
export PRIVATE_IMAGE

info "Pulling public nginx:stable..."
docker pull --platform linux/amd64 nginx:stable

info "Tagging as ${PRIVATE_IMAGE}..."
docker tag nginx:stable "${PRIVATE_IMAGE}"

info "Pushing to private registry..."
docker push "${PRIVATE_IMAGE}"
info "Pushed: ${PRIVATE_IMAGE}"

# Deploy on scanner cluster
info "Deploying private-image workload on scanner cluster..."
envsubst < "${SHARED_MANIFESTS_DIR}/nginx-private-workload.yaml.tpl" \
  | kubectl apply -f -

wait_for_deployment "default" "nginx-private-workload"
info "Private-image workload deployed on scanner cluster."

# Deploy on target cluster (if available) so external cluster scanning can
# discover a private image too.
if [[ "${ENABLE_TARGET_CLUSTER:-false}" == "true" ]] && [[ -n "${TARGET_KUBECONFIG_PATH:-}" ]]; then
  info "Deploying private-image workload on target cluster..."
  envsubst < "${SHARED_MANIFESTS_DIR}/nginx-private-workload.yaml.tpl" \
    | kubectl --kubeconfig "${TARGET_KUBECONFIG_PATH}" apply -f -

  info "Waiting for deployment on target cluster..."
  kubectl --kubeconfig "${TARGET_KUBECONFIG_PATH}" rollout status deployment/nginx-private-workload \
    -n default --timeout=300s
  info "Private-image workload deployed on target cluster."
fi

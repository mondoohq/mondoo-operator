#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Deploy the operator with skipContainerResolution enabled (for dev cnspec images
# that aren't published to ghcr.io and lack OCI version labels).

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

: "${IMAGE_REPO:?IMAGE_REPO must be set}"
: "${IMAGE_TAG:?IMAGE_TAG must be set}"
: "${NAMESPACE:?NAMESPACE must be set}"

info "Deploying operator with skipContainerResolution (dev cnspec image)..."

# Apply CRDs
info "Applying CRDs from local chart..."
kubectl apply --server-side --force-conflicts -f "${REPO_ROOT}/charts/mondoo-operator/crds/"

for crd in $(kubectl get crds -o name 2>/dev/null | grep mondoo || true); do
  info "Adopting existing CRD for Helm: ${crd}"
  kubectl label "${crd}" app.kubernetes.io/managed-by=Helm --overwrite
  kubectl annotate "${crd}" meta.helm.sh/release-name=mondoo-operator meta.helm.sh/release-namespace="${NAMESPACE}" --overwrite
done

helm upgrade --install mondoo-operator "${REPO_ROOT}/charts/mondoo-operator" \
  --namespace "${NAMESPACE}" --create-namespace \
  --set controllerManager.manager.image.repository="${IMAGE_REPO}" \
  --set controllerManager.manager.image.tag="${IMAGE_TAG}" \
  --set controllerManager.manager.imagePullPolicy=Always \
  --set controllerManager.manager.secureMetrics=true \
  --set operator.skipContainerResolution=true \
  --wait --timeout 5m

wait_for_deployment "${NAMESPACE}" "mondoo-operator-controller-manager"

info "Operator deployed with skipContainerResolution."

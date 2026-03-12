#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Deploy the operator from the local Helm chart with mirror/proxy configuration
# Uses a values-override file to avoid Helm --set escaping issues with dots in keys

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

: "${IMAGE_REPO:?IMAGE_REPO must be set}"
: "${IMAGE_TAG:?IMAGE_TAG must be set}"
: "${NAMESPACE:?NAMESPACE must be set}"
: "${MIRROR_REGISTRY:?MIRROR_REGISTRY must be set}"

# Proxy settings (optional)
SQUID_PROXY_IP="${SQUID_PROXY_IP:-}"

info "Deploying operator with mirroring configuration..."
info "  Mirror registry: ${MIRROR_REGISTRY}"
if [[ -n "${SQUID_PROXY_IP}" ]]; then
  info "  Proxy: http://${SQUID_PROXY_IP}:3128"
fi

# Build proxy values
PROXY_HTTP=""
PROXY_HTTPS=""
PROXY_NO=""
PROXY_CONTAINER=""
if [[ -n "${SQUID_PROXY_IP}" ]]; then
  PROXY_HTTP="http://${SQUID_PROXY_IP}:3128"
  PROXY_HTTPS="http://${SQUID_PROXY_IP}:3128"
  PROXY_CONTAINER="http://${SQUID_PROXY_IP}:3128"

  # Get both the external and in-cluster Kubernetes API server IPs.
  # External: from kubeconfig (used by kubectl from outside)
  # Internal: the kubernetes.default ClusterIP (used by pods via KUBERNETES_SERVICE_HOST)
  K8S_API_EXTERNAL=$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}' | sed -E 's|https?://||;s|:[0-9]+$||')
  K8S_API_INTERNAL=$(kubectl get svc kubernetes -n default -o jsonpath='{.spec.clusterIP}')
  info "Kubernetes API server IPs: external=${K8S_API_EXTERNAL}, internal=${K8S_API_INTERNAL}"
  PROXY_NO="10.0.0.0/8,172.16.0.0/12,.cluster.local,.svc,localhost,127.0.0.1,${K8S_API_EXTERNAL},${K8S_API_INTERNAL}"
fi

# Generate values override file
# Using a file avoids Helm --set escaping issues with dots in registry mirror keys
VALUES_FILE=$(mktemp /tmp/mondoo-mirror-values-XXXXXX.yaml)
trap 'rm -f "${VALUES_FILE}"' EXIT

cat > "${VALUES_FILE}" <<EOF
operator:
  skipContainerResolution: true
  registryMirrors:
    ghcr.io: "${MIRROR_REGISTRY}"
  imagePullSecrets:
    - name: mirror-registry-creds
  httpProxy: "${PROXY_HTTP}"
  httpsProxy: "${PROXY_HTTPS}"
  noProxy: "${PROXY_NO}"
  containerProxy: "${PROXY_CONTAINER}"
EOF

info "Generated values override:"
cat "${VALUES_FILE}"

# Adopt any existing Mondoo CRDs so Helm can manage them
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
  --values "${VALUES_FILE}" \
  --wait --timeout 5m

wait_for_deployment "${NAMESPACE}" "mondoo-operator-controller-manager"

info "Operator deployed with mirroring configuration."

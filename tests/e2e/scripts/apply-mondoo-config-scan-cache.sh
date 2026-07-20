#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Create Mondoo credentials and apply scan-cache-enabled MondooAuditConfig.
# Uses the cloud-specific scan cache manifest templates.
#
# Environment:
#   CNSPEC_IMAGE_NAME  — custom cnspec image name (empty = use default)
#   CNSPEC_IMAGE_TAG   — custom cnspec image tag (empty = use default)

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

: "${NAMESPACE:?NAMESPACE must be set}"
: "${MONDOO_CREDS_B64:?MONDOO_CREDS_B64 must be set}"

export CNSPEC_IMAGE_NAME="${CNSPEC_IMAGE_NAME:-}"
export CNSPEC_IMAGE_TAG="${CNSPEC_IMAGE_TAG:-}"
export PULL_SECRET_NAME="${PULL_SECRET_NAME:-ar-pull-secret}"

# Create credentials secret
info "Creating mondoo-client secret..."
echo "${MONDOO_CREDS_B64}" | base64 -d > /tmp/mondoo-creds.json
kubectl create secret generic mondoo-client \
  --from-file=config=/tmp/mondoo-creds.json \
  --namespace "${NAMESPACE}" \
  --dry-run=client -o yaml | kubectl apply -f -
rm -f /tmp/mondoo-creds.json

# Apply MondooAuditConfig
MANIFEST="${MANIFESTS_DIR}/mondoo-audit-config-scan-cache.yaml.tpl"
if [[ ! -f "${MANIFEST}" ]]; then
  die "Scan cache manifest not found: ${MANIFEST}"
fi

info "Applying scan cache MondooAuditConfig..."
info "  cnspec image: ${CNSPEC_IMAGE_NAME:-default}:${CNSPEC_IMAGE_TAG:-default}"

envsubst < "${MANIFEST}" | kubectl apply -f -

# Patch in external cluster if target cluster is enabled
if [[ "${ENABLE_TARGET_CLUSTER:-}" == "true" ]]; then
  info "Adding external cluster to MondooAuditConfig..."
  kubectl patch mondooauditconfig mondoo-client -n "${NAMESPACE}" --type=merge -p '
spec:
  kubernetesResources:
    externalClusters:
      - name: target-cluster
        kubeconfigSecretRef:
          name: target-kubeconfig
'
fi

info "Scan cache MondooAuditConfig applied."

#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Create the Mondoo credentials secret and apply MondooAuditConfig

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

: "${MONDOO_CREDS_B64:?MONDOO_CREDS_B64 must be set}"
: "${NAMESPACE:?NAMESPACE must be set}"

info "Creating mondoo-client secret..."

# Decode the base64-encoded credentials and create/update the secret
echo "${MONDOO_CREDS_B64}" | base64 -d > /tmp/mondoo-creds.json
kubectl create secret generic mondoo-client \
  --from-file=config=/tmp/mondoo-creds.json \
  --namespace "${NAMESPACE}" \
  --dry-run=client -o yaml | kubectl apply -f -
rm -f /tmp/mondoo-creds.json

info "Applying MondooAuditConfig..."
export NAMESPACE

# Select manifest based on test type and cluster mode.
# Autopilot variants are GKE-specific; other clouds just use the base manifest.
if [[ "${ENABLE_WIF_TEST:-false}" == "true" ]]; then
  if [[ "${AUTOPILOT:-false}" == "true" ]]; then
    MANIFEST="${MANIFESTS_DIR}/mondoo-audit-config-wif-external-autopilot.yaml.tpl"
  else
    MANIFEST="${MANIFESTS_DIR}/mondoo-audit-config-wif-external.yaml.tpl"
  fi
elif [[ "${ENABLE_VAULT_TEST:-false}" == "true" ]]; then
  if [[ "${AUTOPILOT:-false}" == "true" ]]; then
    MANIFEST="${MANIFESTS_DIR}/mondoo-audit-config-vault-external-autopilot.yaml.tpl"
  else
    MANIFEST="${MANIFESTS_DIR}/mondoo-audit-config-vault-external.yaml.tpl"
  fi
elif [[ "${ENABLE_TARGET_CLUSTER:-false}" == "true" ]]; then
  if [[ "${AUTOPILOT:-false}" == "true" ]]; then
    MANIFEST="${MANIFESTS_DIR}/mondoo-audit-config-external-autopilot.yaml.tpl"
  else
    MANIFEST="${MANIFESTS_DIR}/mondoo-audit-config-external.yaml.tpl"
  fi
elif [[ "${AUTOPILOT:-false}" == "true" ]]; then
  MANIFEST="${MANIFESTS_DIR}/mondoo-audit-config-autopilot.yaml.tpl"
else
  MANIFEST="${MANIFESTS_DIR}/mondoo-audit-config.yaml.tpl"
fi
info "Using manifest: $(basename "${MANIFEST}")"
envsubst < "${MANIFEST}" | kubectl apply -f -

info "MondooAuditConfig applied."

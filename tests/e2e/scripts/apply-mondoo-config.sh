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
if [[ "${ENABLE_VAULT_TEST:-false}" == "true" ]]; then
  if [[ "${AUTOPILOT}" == "true" ]]; then
    MANIFEST="${E2E_DIR}/manifests/mondoo-audit-config-vault-external-autopilot.yaml.tpl"
  else
    MANIFEST="${E2E_DIR}/manifests/mondoo-audit-config-vault-external.yaml.tpl"
  fi
elif [[ "${ENABLE_TARGET_CLUSTER:-false}" == "true" ]]; then
  if [[ "${AUTOPILOT}" == "true" ]]; then
    MANIFEST="${E2E_DIR}/manifests/mondoo-audit-config-external-autopilot.yaml.tpl"
  else
    MANIFEST="${E2E_DIR}/manifests/mondoo-audit-config-external.yaml.tpl"
  fi
elif [[ "${AUTOPILOT}" == "true" ]]; then
  MANIFEST="${E2E_DIR}/manifests/mondoo-audit-config-autopilot.yaml.tpl"
else
  MANIFEST="${E2E_DIR}/manifests/mondoo-audit-config.yaml.tpl"
fi
info "Using manifest: $(basename "${MANIFEST}")"
envsubst < "${MANIFEST}" | kubectl apply -f -

info "MondooAuditConfig applied."

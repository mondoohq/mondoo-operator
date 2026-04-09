#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Create the Mondoo credentials secret from an org-level SA and apply
# TWO MondooAuditConfigs with spaceId routing:
#   - mondoo-scanner: local cluster → scanner space
#   - mondoo-target:  external cluster → target space
#
# Credentials can come from:
#   - ORG_CREDS_B64 env var (base64-encoded, from Terraform)
#   - MONDOO_CONFIG_PATH env var (path to a mondoo.json file)

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

: "${SCANNER_SPACE_ID:?SCANNER_SPACE_ID must be set}"
: "${TARGET_SPACE_ID:?TARGET_SPACE_ID must be set}"
: "${NAMESPACE:?NAMESPACE must be set}"

info "Creating mondoo-client secret from org-level service account..."

if [[ -n "${MONDOO_CONFIG_PATH:-}" ]]; then
  info "Using credentials from MONDOO_CONFIG_PATH=${MONDOO_CONFIG_PATH}"
  [[ -f "${MONDOO_CONFIG_PATH}" ]] || die "MONDOO_CONFIG_PATH file not found: ${MONDOO_CONFIG_PATH}"
  kubectl create secret generic mondoo-client \
    --from-file=config="${MONDOO_CONFIG_PATH}" \
    --namespace "${NAMESPACE}" \
    --dry-run=client -o yaml | kubectl apply -f -
elif [[ -n "${ORG_CREDS_B64:-}" ]]; then
  info "Using credentials from ORG_CREDS_B64"
  trap 'rm -f /tmp/mondoo-creds.json' EXIT
  echo "${ORG_CREDS_B64}" | base64 -d > /tmp/mondoo-creds.json
  kubectl create secret generic mondoo-client \
    --from-file=config=/tmp/mondoo-creds.json \
    --namespace "${NAMESPACE}" \
    --dry-run=client -o yaml | kubectl apply -f -
  rm -f /tmp/mondoo-creds.json
else
  die "Either MONDOO_CONFIG_PATH or ORG_CREDS_B64 must be set"
fi

info "Applying MondooAuditConfigs with space splitting..."
info "  mondoo-scanner → spaceId=${SCANNER_SPACE_ID}"
info "  mondoo-target  → spaceId=${TARGET_SPACE_ID}"
export NAMESPACE SCANNER_SPACE_ID TARGET_SPACE_ID

if [[ "${AUTOPILOT:-false}" == "true" ]]; then
  MANIFEST="${MANIFESTS_DIR}/mondoo-audit-config-space-splitting-autopilot.yaml.tpl"
else
  MANIFEST="${MANIFESTS_DIR}/mondoo-audit-config-space-splitting.yaml.tpl"
fi
info "Using manifest: $(basename "${MANIFEST}")"
envsubst < "${MANIFEST}" | kubectl apply -f -

info "Both MondooAuditConfigs with space splitting applied."

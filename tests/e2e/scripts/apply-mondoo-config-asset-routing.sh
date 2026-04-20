#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Create the Mondoo credentials secret from an org-level SA and apply
# TWO MondooAuditConfigs for asset routing (no spaceId, no annotations):
#   - mondoo-scanner: local cluster (k8s + containers + nodes)
#   - mondoo-target:  external cluster only
#
# Server-side routing rules determine which space assets land in.
#
# Credentials can come from:
#   - ORG_CREDS_B64 env var (base64-encoded, from Terraform)
#   - MONDOO_CONFIG_PATH env var (path to a mondoo.json file)

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

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
  tmpfile=$(mktemp /tmp/mondoo-creds.XXXXXX)
  chmod 600 "${tmpfile}"
  trap 'rm -f "${tmpfile}"' EXIT
  echo "${ORG_CREDS_B64}" | base64 -d > "${tmpfile}"
  kubectl create secret generic mondoo-client \
    --from-file=config="${tmpfile}" \
    --namespace "${NAMESPACE}" \
    --dry-run=client -o yaml | kubectl apply -f -
  rm -f "${tmpfile}"
else
  die "Either MONDOO_CONFIG_PATH or ORG_CREDS_B64 must be set"
fi

info "Applying MondooAuditConfigs for asset routing..."
info "  mondoo-scanner → local cluster (k8s + containers + nodes)"
info "  mondoo-target  → external cluster only"
info "  No spaceId, no annotations — server-side routing rules handle placement."
export NAMESPACE

if [[ "${AUTOPILOT:-false}" == "true" ]]; then
  MANIFEST="${MANIFESTS_DIR}/mondoo-audit-config-asset-routing-autopilot.yaml.tpl"
else
  MANIFEST="${MANIFESTS_DIR}/mondoo-audit-config-asset-routing.yaml.tpl"
fi
info "Using manifest: $(basename "${MANIFEST}")"
envsubst < "${MANIFEST}" | kubectl apply -f -

info "MondooAuditConfigs for asset routing applied."

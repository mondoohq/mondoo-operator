#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Create the Mondoo credentials secret and apply MondooAuditConfig.
#
# Integration mode (ENABLE_INTEGRATION=true) is orthogonal to the manifest
# variant — any suite can opt in via the terraform variable. When enabled,
# consoleIntegration and mondooTokenSecretRef are injected into whichever
# manifest the suite selects.
#
# In integration mode the mondoo-client secret is NOT pre-created — the
# operator creates it via token exchange (IntegrationRegister). Pre-creating
# it would block the exchange because the operator uses CreateIfNotExist.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

: "${NAMESPACE:?NAMESPACE must be set}"

# --- Credentials ---

if [[ "${ENABLE_INTEGRATION:-false}" == "true" ]]; then
  # Integration mode: create integration, then only the token secret.
  # The operator will exchange the token for SA creds + integration MRN
  # and create the mondoo-client secret itself via CreateIfNotExist.
  # Delete any pre-existing mondoo-client secret so the exchange isn't skipped.
  info "Deleting stale mondoo-client secret (if any)..."
  kubectl delete secret mondoo-client -n "${NAMESPACE}" --ignore-not-found

  source "${SCRIPT_DIR}/create-integration.sh"

  info "Creating mondoo-token secret..."
  kubectl create secret generic mondoo-token \
    --from-literal=token="${INTEGRATION_TOKEN}" \
    --namespace "${NAMESPACE}" \
    --dry-run=client -o yaml | kubectl apply -f -
else
  # Non-integration mode: create SA creds secret directly.
  : "${MONDOO_CREDS_B64:?MONDOO_CREDS_B64 must be set}"

  info "Creating mondoo-client secret..."
  echo "${MONDOO_CREDS_B64}" | base64 -d > /tmp/mondoo-creds.json
  kubectl create secret generic mondoo-client \
    --from-file=config=/tmp/mondoo-creds.json \
    --namespace "${NAMESPACE}" \
    --dry-run=client -o yaml | kubectl apply -f -
  rm -f /tmp/mondoo-creds.json
fi

# --- Manifest selection ---

info "Applying MondooAuditConfig..."
export NAMESPACE

# Select manifest based on test type and cluster mode.
if [[ "${ENABLE_ENDPOINT_OVERRIDE_TEST:-false}" == "true" ]]; then
  MANIFEST="${MANIFESTS_DIR}/mondoo-audit-config-wif-endpoint-override.yaml.tpl"
elif [[ "${ENABLE_WIF_TEST:-false}" == "true" ]]; then
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
RENDERED=$(envsubst < "${MANIFEST}")

# Inject consoleIntegration fields when integration mode is enabled.
# This works with any manifest variant — no need for separate -integration templates.
if [[ "${ENABLE_INTEGRATION:-false}" == "true" ]]; then
  info "Injecting consoleIntegration into manifest"
  RENDERED=$(echo "${RENDERED}" | sed '/^spec:/a\
  mondooTokenSecretRef:\
    name: mondoo-token\
  consoleIntegration:\
    enable: true')
fi

echo "${RENDERED}" | kubectl apply -f -

info "MondooAuditConfig applied."

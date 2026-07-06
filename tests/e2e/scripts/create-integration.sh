#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Idempotent integration lifecycle — creates once per terraform apply cycle,
# reuses on subsequent suite runs within the same infra.
#
# State is persisted to $TF_DIR/.integration-state so cleanup.sh can delete it.
# The token is single-use (consumed by the operator's token exchange), so we
# delete and recreate the integration on each suite run to get a fresh token.
#
# Exports:
#   INTEGRATION_MRN   — MRN of the integration
#   INTEGRATION_TOKEN — registration token
#
# Auth: uses MONDOO_CONFIG_PATH, MONDOO_CONFIG_BASE64, or MONDOO_CREDS_B64
# Required: MONDOO_SPACE_MRN, TF_DIR

set -euo pipefail

if ! type info &>/dev/null; then
  info()  { echo "[INFO]  $(date '+%H:%M:%S') $*"; }
  err()   { echo "[ERROR] $(date '+%H:%M:%S') $*" >&2; }
fi

: "${MONDOO_SPACE_MRN:?MONDOO_SPACE_MRN must be set}"
: "${TF_DIR:?TF_DIR must be set}"

SCRIPT_DIR="${SCRIPT_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)}"
REPO_ROOT="${REPO_ROOT:-$(cd "${SCRIPT_DIR}/../../.." && pwd)}"

STATE_FILE="${TF_DIR}/.integration-state"

# Clean up previous integration if one exists (token is single-use, can't reuse)
if [[ -f "${STATE_FILE}" ]]; then
  source "${STATE_FILE}"
  if [[ -n "${INTEGRATION_MRN:-}" ]]; then
    info "Deleting previous integration: ${INTEGRATION_MRN}"
    export INTEGRATION_MRN
    go run "${REPO_ROOT}/tests/e2e/cmd/integration" delete 2>/dev/null || true
    unset INTEGRATION_MRN
  fi
  rm -f "${STATE_FILE}"
fi

info "Creating K8s integration in space ${MONDOO_SPACE_MRN}..."

OUTPUT=$(go run "${REPO_ROOT}/tests/e2e/cmd/integration" create)

INTEGRATION_MRN=$(echo "${OUTPUT}" | grep '^MRN=' | cut -d= -f2-)
INTEGRATION_TOKEN=$(echo "${OUTPUT}" | grep '^TOKEN=' | cut -d= -f2-)

if [[ -z "${INTEGRATION_MRN}" ]]; then
  err "Failed to create integration"
  exit 1
fi

# Persist state for cleanup
cat > "${STATE_FILE}" <<EOF
INTEGRATION_MRN="${INTEGRATION_MRN}"
INTEGRATION_SPACE_MRN="${MONDOO_SPACE_MRN}"
EOF

export INTEGRATION_MRN
export INTEGRATION_TOKEN

info "Created integration: ${INTEGRATION_MRN}"

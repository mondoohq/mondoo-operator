#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Deletes a Mondoo K8s integration and removes the state file.
#
# Reads INTEGRATION_MRN from $TF_DIR/.integration-state if not already set.
# Auth: uses MONDOO_CONFIG_PATH, MONDOO_CONFIG_BASE64, or MONDOO_CREDS_B64

set -euo pipefail

if ! type info &>/dev/null; then
  info()  { echo "[INFO]  $(date '+%H:%M:%S') $*"; }
  warn()  { echo "[WARN]  $(date '+%H:%M:%S') $*" >&2; }
  err()   { echo "[ERROR] $(date '+%H:%M:%S') $*" >&2; }
fi

SCRIPT_DIR="${SCRIPT_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)}"
REPO_ROOT="${REPO_ROOT:-$(cd "${SCRIPT_DIR}/../../.." && pwd)}"

STATE_FILE="${TF_DIR:+${TF_DIR}/.integration-state}"

# Load from state file if INTEGRATION_MRN is not already set
if [[ -z "${INTEGRATION_MRN:-}" && -n "${STATE_FILE}" && -f "${STATE_FILE}" ]]; then
  source "${STATE_FILE}"
fi

if [[ -z "${INTEGRATION_MRN:-}" ]]; then
  info "No integration to delete (no MRN found)"
  return 0 2>/dev/null || exit 0
fi

info "Deleting integration ${INTEGRATION_MRN}..."

export INTEGRATION_MRN
go run "${REPO_ROOT}/tests/e2e/cmd/integration" delete

# Clean up state file
if [[ -n "${STATE_FILE}" && -f "${STATE_FILE}" ]]; then
  rm -f "${STATE_FILE}"
fi

info "Deleted integration: ${INTEGRATION_MRN}"

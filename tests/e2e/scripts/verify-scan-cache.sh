#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Verify scan cache e2e test: CronJob inventory and scan Job logs.
# Can be run standalone — only needs kubectl context and NAMESPACE.

set -euo pipefail

_info()  { echo "[INFO]  $(date '+%H:%M:%S') $*"; }
_err()   { echo "[ERROR] $(date '+%H:%M:%S') $*" >&2; }

NAMESPACE="${NAMESPACE:-mondoo-operator}"

PASS=0
FAIL=0

check() {
  local desc="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    _info "PASS: ${desc}"
    PASS=$((PASS + 1))
  else
    _err "FAIL: ${desc}"
    FAIL=$((FAIL + 1))
  fi
}

_info "=== Scan Cache Verification (namespace: ${NAMESPACE}) ==="

# CronJob exists for containers
CRONJOB_NAME=$(kubectl get cronjobs -n "${NAMESPACE}" -l mondoo_cr=mondoo-client \
  -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null | grep container | head -1)

if [[ -n "${CRONJOB_NAME}" ]]; then
  _info "Found container CronJob: ${CRONJOB_NAME}"

  # Check inventory ConfigMap for digests-exclude (populated after first scan + refresh)
  CM_NAME=$(kubectl get configmaps -n "${NAMESPACE}" -l mondoo_cr=mondoo-client \
    -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null | grep containers-inventory | head -1)
  if [[ -n "${CM_NAME}" ]]; then
    check "Inventory ConfigMap exists" true
    INVENTORY=$(kubectl get configmap "${CM_NAME}" -n "${NAMESPACE}" -o jsonpath='{.data.inventory}' 2>/dev/null || true)
    if echo "${INVENTORY}" | grep -q 'digests-exclude'; then
      _info "PASS: Inventory has digests-exclude option (server refresh returned digests)"
      PASS=$((PASS + 1))
    else
      _info "INFO: Inventory does not have digests-exclude (expected on first scan or if no prior assets exist server-side)"
    fi
  else
    _err "FAIL: No containers-inventory ConfigMap found"
    FAIL=$((FAIL + 1))
  fi
else
  _err "FAIL: No container CronJob found"
  FAIL=$((FAIL + 1))
fi

# Jobs
JOB_COUNT=$(kubectl get jobs -n "${NAMESPACE}" -l mondoo_cr=mondoo-client \
  --field-selector=status.successful=1 -o name 2>/dev/null | wc -l | tr -d ' ')
check "At least one scan Job completed successfully" \
  bash -c "[[ ${JOB_COUNT} -gt 0 ]]"

_info ""
_info "=== Scan Job Logs ==="

JOBS=$(kubectl get jobs -n "${NAMESPACE}" -l mondoo_cr=mondoo-client \
  --sort-by=.status.startTime -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null || true)

for job in ${JOBS}; do
  LOGS=$(kubectl logs "job/${job}" -n "${NAMESPACE}" 2>/dev/null || true)
  [[ -z "${LOGS}" ]] && continue

  SCANNED=$(echo "${LOGS}" | grep -c "start scan" || true)
  SKIPPED=$(echo "${LOGS}" | grep -c "skipping" || true)

  [[ $((SCANNED + SKIPPED)) -gt 0 ]] && _info "  job/${job}: scanned=${SCANNED} skipped=${SKIPPED}"
done

_info ""
_info "=== Operator Logs (RefreshAssetScores) ==="
OPERATOR_POD=$(kubectl get pods -n "${NAMESPACE}" -l app.kubernetes.io/name=mondoo-operator \
  -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)
if [[ -n "${OPERATOR_POD}" ]]; then
  kubectl logs "${OPERATOR_POD}" -n "${NAMESPACE}" 2>/dev/null \
    | grep -E '(RefreshAssetScores|digestsToExclude)' \
    | tail -10 \
    | sed 's/^/  /' || _info "  (no RefreshAssetScores log lines found)"
fi

_info ""
_info "=== Results: ${PASS} passed, ${FAIL} failed ==="
[[ ${FAIL} -gt 0 ]] && { _err "Some checks failed."; exit 1; }

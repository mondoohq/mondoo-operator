#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Verify external cluster scanning resources are created

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

: "${NAMESPACE:?NAMESPACE must be set}"

PASS=0
FAIL=0

check() {
  local desc="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    info "PASS: ${desc}"
    PASS=$((PASS + 1))
  else
    err "FAIL: ${desc}"
    FAIL=$((FAIL + 1))
  fi
}

info "=== External Cluster Verification ==="

# Check target-kubeconfig Secret exists
check "target-kubeconfig Secret exists" \
  kubectl get secret target-kubeconfig -n "${NAMESPACE}"

# Check CronJobs include one for external cluster scanning
check "CronJob for external cluster exists" \
  bash -c "kubectl get cronjobs -n '${NAMESPACE}' -l mondoo_cr=mondoo-client -o name | grep -q target-cluster"

info ""
info "=== External Cluster Resource Details ==="

info "--- CronJobs (filtered for target-cluster) ---"
kubectl get cronjobs -n "${NAMESPACE}" -l mondoo_cr=mondoo-client -o wide 2>/dev/null | grep -E "NAME|target-cluster" || true

info ""
info "=== External Cluster Results: ${PASS} passed, ${FAIL} failed ==="

if [[ ${FAIL} -gt 0 ]]; then
  err "Some external cluster checks failed. Review the output above."
  exit 1
fi

info ""
info "=== Manual Verification ==="
info "Check the Mondoo console for external cluster assets"
info "  Space MRN: ${MONDOO_SPACE_MRN:-unknown}"
info "  - Target cluster K8s resources should appear as assets"
info "  - nginx-test-workload from the target cluster should be visible"
read -rp "Press Enter once verified (or Ctrl+C to abort)... "

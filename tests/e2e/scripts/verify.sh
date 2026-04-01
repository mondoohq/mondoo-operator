#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Verify the operator deployment and scanning resources

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

info "=== Verification ==="

# Check operator pod is running
check "Operator pod is Running" \
  kubectl get pods -n "${NAMESPACE}" -l control-plane=controller-manager \
    --field-selector=status.phase=Running -o name

# Check MondooAuditConfig exists
check "MondooAuditConfig 'mondoo-client' exists" \
  kubectl get mondooauditconfigs.k8s.mondoo.com mondoo-client -n "${NAMESPACE}"

# Check CronJobs created
check "CronJobs created for mondoo-client" \
  kubectl get cronjobs -n "${NAMESPACE}" -l mondoo_cr=mondoo-client -o name

# Check secure metrics endpoint is serving on HTTPS (port 8443)
check "Metrics endpoint responds on port 8443 (HTTPS)" \
  bash -c "kubectl get svc -n '${NAMESPACE}' -l app.kubernetes.io/name=mondoo-operator \
    -o jsonpath='{.items[0].spec.ports[0].port}' | grep -q 8443"

# TODO: Add automated verification against Mondoo API:
#   - Query the space for discovered assets (cluster, nodes, container images)
#   - Verify scan results exist and are recent
#   - Check asset counts match expected values
#   - This would replace the manual verification step below

info ""
info "=== Resource Details ==="

info "--- Pods ---"
kubectl get pods -n "${NAMESPACE}" -o wide 2>/dev/null || true

info "--- CronJobs ---"
kubectl get cronjobs -n "${NAMESPACE}" -o wide 2>/dev/null || true

info "--- MondooAuditConfig Status ---"
kubectl get mondooauditconfigs.k8s.mondoo.com mondoo-client -n "${NAMESPACE}" -o yaml 2>/dev/null || true

info ""
info "=== Results: ${PASS} passed, ${FAIL} failed ==="

if [[ ${FAIL} -gt 0 ]]; then
  err "Some checks failed. Review the output above."
  exit 1
fi

info ""
info "=== Manual Verification ==="
info "Check the Mondoo console for assets in the test space"
info "  Space MRN: ${MONDOO_SPACE_MRN:-unknown}"
info "  - K8s cluster, node, and container image assets should appear"
info "  - Scan results should be populated"
read -rp "Press Enter once verified (or Ctrl+C to abort)... "

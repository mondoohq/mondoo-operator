#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Verify space splitting: single operator, two MondooAuditConfigs,
# each routing to a different space via org-level SA + spaceId.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

: "${NAMESPACE:?NAMESPACE must be set}"
: "${SCANNER_SPACE_ID:?SCANNER_SPACE_ID must be set}"
: "${TARGET_SPACE_ID:?TARGET_SPACE_ID must be set}"

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

info "=== Space Splitting Verification ==="

# ---- Operator ----
check "Operator pod is Running" \
  kubectl get pods -n "${NAMESPACE}" -l control-plane=controller-manager \
    --field-selector=status.phase=Running -o name

# ---- Scanner MondooAuditConfig (local cluster → scanner space) ----
info ""
info "--- mondoo-scanner (local → ${SCANNER_SPACE_ID}) ---"

check "MondooAuditConfig 'mondoo-scanner' exists" \
  kubectl get mondooauditconfigs.k8s.mondoo.com mondoo-scanner -n "${NAMESPACE}"

check "mondoo-scanner has spaceId=${SCANNER_SPACE_ID}" \
  bash -c "kubectl get mondooauditconfigs.k8s.mondoo.com mondoo-scanner -n '${NAMESPACE}' \
    -o jsonpath='{.spec.spaceId}' | grep -q '${SCANNER_SPACE_ID}'"

SCANNER_OVERRIDE="mondoo-scanner-config-override"
check "Config override secret '${SCANNER_OVERRIDE}' exists" \
  kubectl get secret "${SCANNER_OVERRIDE}" -n "${NAMESPACE}"

check "Scanner override has correct scope_mrn" \
  bash -c "kubectl get secret '${SCANNER_OVERRIDE}' -n '${NAMESPACE}' \
    -o jsonpath='{.data.config}' | base64 -d | grep -q '${SCANNER_SPACE_ID}'"

check "Scanner override owned by MondooAuditConfig" \
  bash -c "kubectl get secret '${SCANNER_OVERRIDE}' -n '${NAMESPACE}' \
    -o jsonpath='{.metadata.ownerReferences[0].kind}' | grep -q 'MondooAuditConfig'"

check "Scanner CronJobs created" \
  kubectl get cronjobs -n "${NAMESPACE}" -l mondoo_cr=mondoo-scanner -o name

check "Scanner CronJobs use override secret" \
  bash -c "kubectl get cronjobs -n '${NAMESPACE}' -l mondoo_cr=mondoo-scanner \
    -o yaml | grep -q '${SCANNER_OVERRIDE}'"

# ---- Target MondooAuditConfig (external cluster → target space) ----
info ""
info "--- mondoo-target (external → ${TARGET_SPACE_ID}) ---"

check "MondooAuditConfig 'mondoo-target' exists" \
  kubectl get mondooauditconfigs.k8s.mondoo.com mondoo-target -n "${NAMESPACE}"

check "mondoo-target has spaceId=${TARGET_SPACE_ID}" \
  bash -c "kubectl get mondooauditconfigs.k8s.mondoo.com mondoo-target -n '${NAMESPACE}' \
    -o jsonpath='{.spec.spaceId}' | grep -q '${TARGET_SPACE_ID}'"

TARGET_OVERRIDE="mondoo-target-config-override"
check "Config override secret '${TARGET_OVERRIDE}' exists" \
  kubectl get secret "${TARGET_OVERRIDE}" -n "${NAMESPACE}"

check "Target override has correct scope_mrn" \
  bash -c "kubectl get secret '${TARGET_OVERRIDE}' -n '${NAMESPACE}' \
    -o jsonpath='{.data.config}' | base64 -d | grep -q '${TARGET_SPACE_ID}'"

check "Target override owned by MondooAuditConfig" \
  bash -c "kubectl get secret '${TARGET_OVERRIDE}' -n '${NAMESPACE}' \
    -o jsonpath='{.metadata.ownerReferences[0].kind}' | grep -q 'MondooAuditConfig'"

check "Target CronJobs created" \
  kubectl get cronjobs -n "${NAMESPACE}" -l mondoo_cr=mondoo-target -o name

check "Target CronJobs use override secret" \
  bash -c "kubectl get cronjobs -n '${NAMESPACE}' -l mondoo_cr=mondoo-target \
    -o yaml | grep -q '${TARGET_OVERRIDE}'"

check "External cluster CronJob references target-cluster" \
  bash -c "kubectl get cronjobs -n '${NAMESPACE}' -l mondoo_cr=mondoo-target \
    -o yaml | grep -q 'target-cluster'"

# ---- Both share the same org SA secret ----
info ""
info "--- Shared credentials ---"

check "Both configs reference the same mondoo-client secret" \
  bash -c "
    s1=\$(kubectl get mondooauditconfigs.k8s.mondoo.com mondoo-scanner -n '${NAMESPACE}' -o jsonpath='{.spec.mondooCredsSecretRef.name}')
    s2=\$(kubectl get mondooauditconfigs.k8s.mondoo.com mondoo-target -n '${NAMESPACE}' -o jsonpath='{.spec.mondooCredsSecretRef.name}')
    [[ \"\$s1\" == \"mondoo-client\" && \"\$s2\" == \"mondoo-client\" ]]
  "

check "Override secrets have DIFFERENT scope_mrn values" \
  bash -c "
    s1=\$(kubectl get secret '${SCANNER_OVERRIDE}' -n '${NAMESPACE}' -o jsonpath='{.data.config}' | base64 -d)
    s2=\$(kubectl get secret '${TARGET_OVERRIDE}' -n '${NAMESPACE}' -o jsonpath='{.data.config}' | base64 -d)
    echo \"\$s1\" | grep -q '${SCANNER_SPACE_ID}' && echo \"\$s2\" | grep -q '${TARGET_SPACE_ID}'
  "

# ---- Summary ----
info ""
info "--- Resource Overview ---"
info "Pods:"
kubectl get pods -n "${NAMESPACE}" -o wide 2>/dev/null || true
info "CronJobs:"
kubectl get cronjobs -n "${NAMESPACE}" -o wide 2>/dev/null || true
info "Secrets (override):"
for s in "${SCANNER_OVERRIDE}" "${TARGET_OVERRIDE}"; do
  info "  ${s}:"
  kubectl get secret "${s}" -n "${NAMESPACE}" \
    -o jsonpath='{.data.config}' 2>/dev/null | base64 -d | python3 -c "import json,sys; d=json.load(sys.stdin); print('    scope_mrn:', d.get('scope_mrn','NOT SET'))" 2>/dev/null || true
done

info ""
info "=== Results: ${PASS} passed, ${FAIL} failed ==="

if [[ ${FAIL} -gt 0 ]]; then
  err "Some checks failed. Review the output above."
  exit 1
fi

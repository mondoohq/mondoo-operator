#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Verify asset routing: single operator, two MondooAuditConfigs,
# server-side routing rules place assets into the correct spaces.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

: "${NAMESPACE:?NAMESPACE must be set}"

PASS=0
FAIL=0

check() {
  local desc="$1"
  shift
  local output
  if output=$("$@" 2>&1); then
    info "PASS: ${desc}"
    PASS=$((PASS + 1))
  else
    err "FAIL: ${desc}"
    err "  output: ${output}"
    FAIL=$((FAIL + 1))
  fi
}

info "=== Asset Routing Verification ==="

# ---- Operator ----
check "Operator pod is Running" \
  kubectl get pods -n "${NAMESPACE}" -l control-plane=controller-manager \
    --field-selector=status.phase=Running -o name

# ---- mondoo-scanner (local cluster) ----
info ""
info "--- mondoo-scanner (local cluster) ---"

check "MondooAuditConfig 'mondoo-scanner' exists" \
  kubectl get mondooauditconfigs.k8s.mondoo.com mondoo-scanner -n "${NAMESPACE}"

check "mondoo-scanner has NO spaceId set" \
  bash -c "[[ -z \$(kubectl get mondooauditconfigs.k8s.mondoo.com mondoo-scanner -n '${NAMESPACE}' \
    -o jsonpath='{.spec.spaceId}') ]]"

check "mondoo-scanner has NO annotations set" \
  bash -c "[[ -z \$(kubectl get mondooauditconfigs.k8s.mondoo.com mondoo-scanner -n '${NAMESPACE}' \
    -o jsonpath='{.spec.annotations}') ]]"

check "mondoo-scanner references mondoo-client secret" \
  bash -c "kubectl get mondooauditconfigs.k8s.mondoo.com mondoo-scanner -n '${NAMESPACE}' \
    -o jsonpath='{.spec.mondooCredsSecretRef.name}' | grep -q 'mondoo-client'"

check "mondoo-scanner k8sResources enabled" \
  bash -c "[[ \$(kubectl get mondooauditconfigs.k8s.mondoo.com mondoo-scanner -n '${NAMESPACE}' \
    -o jsonpath='{.spec.kubernetesResources.enable}') == 'true' ]]"

check "mondoo-scanner containers enabled" \
  bash -c "[[ \$(kubectl get mondooauditconfigs.k8s.mondoo.com mondoo-scanner -n '${NAMESPACE}' \
    -o jsonpath='{.spec.containers.enable}') == 'true' ]]"

check "Scanner CronJobs created" \
  kubectl get cronjobs -n "${NAMESPACE}" -l mondoo_cr=mondoo-scanner -o name

# ---- mondoo-target (external cluster only) ----
info ""
info "--- mondoo-target (external cluster) ---"

check "MondooAuditConfig 'mondoo-target' exists" \
  kubectl get mondooauditconfigs.k8s.mondoo.com mondoo-target -n "${NAMESPACE}"

check "mondoo-target has NO spaceId set" \
  bash -c "[[ -z \$(kubectl get mondooauditconfigs.k8s.mondoo.com mondoo-target -n '${NAMESPACE}' \
    -o jsonpath='{.spec.spaceId}') ]]"

check "mondoo-target has NO annotations set" \
  bash -c "[[ -z \$(kubectl get mondooauditconfigs.k8s.mondoo.com mondoo-target -n '${NAMESPACE}' \
    -o jsonpath='{.spec.annotations}') ]]"

check "mondoo-target references mondoo-client secret" \
  bash -c "kubectl get mondooauditconfigs.k8s.mondoo.com mondoo-target -n '${NAMESPACE}' \
    -o jsonpath='{.spec.mondooCredsSecretRef.name}' | grep -q 'mondoo-client'"

check "mondoo-target has externalClusters configured" \
  bash -c "kubectl get mondooauditconfigs.k8s.mondoo.com mondoo-target -n '${NAMESPACE}' \
    -o jsonpath='{.spec.kubernetesResources.externalClusters[0].name}' | grep -q 'target-cluster'"

check "Target external cluster CronJob created" \
  kubectl get cronjobs -n "${NAMESPACE}" -l mondoo_cr=mondoo-target -o name

# ---- Shared credentials ----
info ""
info "--- Shared credentials ---"

check "Both configs reference the same mondoo-client secret" \
  bash -c "
    s1=\$(kubectl get mondooauditconfigs.k8s.mondoo.com mondoo-scanner -n '${NAMESPACE}' -o jsonpath='{.spec.mondooCredsSecretRef.name}')
    s2=\$(kubectl get mondooauditconfigs.k8s.mondoo.com mondoo-target -n '${NAMESPACE}' -o jsonpath='{.spec.mondooCredsSecretRef.name}')
    [[ \"\$s1\" == \"mondoo-client\" && \"\$s2\" == \"mondoo-client\" ]]
  "

check "Neither config has spaceId set" \
  bash -c "
    s1=\$(kubectl get mondooauditconfigs.k8s.mondoo.com mondoo-scanner -n '${NAMESPACE}' -o jsonpath='{.spec.spaceId}')
    s2=\$(kubectl get mondooauditconfigs.k8s.mondoo.com mondoo-target -n '${NAMESPACE}' -o jsonpath='{.spec.spaceId}')
    [[ -z \"\$s1\" && -z \"\$s2\" ]]
  "

# ---- Developers workload exists (for routing verification) ----
info ""
info "--- Developers workload ---"

check "nginx-developers deployment exists in developers namespace" \
  kubectl get deployment nginx-developers -n developers

# ---- Summary ----
info ""
info "--- Resource Overview ---"
info "Pods:"
kubectl get pods -n "${NAMESPACE}" -o wide 2>/dev/null || true
info "CronJobs:"
kubectl get cronjobs -n "${NAMESPACE}" -o wide 2>/dev/null || true

info ""
info "=== Results: ${PASS} passed, ${FAIL} failed ==="
info ""
info "Manual verification required:"
info "  - Check e2e space: local cluster assets (catch-all)"
info "  - Check target space: external cluster assets (mondoo.com/cluster-name label)"
info "  - Check developers space: assets with app=nginx-developers label"

if [[ ${FAIL} -gt 0 ]]; then
  err "Some checks failed. Review the output above."
  exit 1
fi

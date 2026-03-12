#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Verify mirroring, imagePullSecrets, and proxy configuration

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

: "${NAMESPACE:?NAMESPACE must be set}"
: "${MIRROR_REGISTRY:?MIRROR_REGISTRY must be set}"

SQUID_PROXY_IP="${SQUID_PROXY_IP:-}"
ENABLE_PROXY_TEST="${ENABLE_PROXY_TEST:-false}"

PASS=0
FAIL=0
WARN_COUNT=0

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

check_warn() {
  local desc="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    info "PASS: ${desc}"
    PASS=$((PASS + 1))
  else
    warn "WARN: ${desc}"
    WARN_COUNT=$((WARN_COUNT + 1))
  fi
}

info "=== Mirroring & Proxy Verification ==="

# Check MondooOperatorConfig has registryMirrors
check "MondooOperatorConfig has registryMirrors" \
  bash -c "kubectl get mondoooperatorconfigs.k8s.mondoo.com -n '${NAMESPACE}' -o jsonpath='{.items[0].spec.registryMirrors}' | grep -q '${MIRROR_REGISTRY}'"

# Check MondooOperatorConfig has imagePullSecrets
check "MondooOperatorConfig has imagePullSecrets" \
  bash -c "kubectl get mondoooperatorconfigs.k8s.mondoo.com -n '${NAMESPACE}' -o jsonpath='{.items[0].spec.imagePullSecrets[0].name}' | grep -q 'mirror-registry-creds'"

# Check CronJob pod specs have imagePullSecrets
check "CronJob pods have imagePullSecrets" \
  bash -c "kubectl get cronjobs -n '${NAMESPACE}' -l mondoo_cr=mondoo-client -o json | jq -e '.items[0].spec.jobTemplate.spec.template.spec.imagePullSecrets[] | select(.name==\"mirror-registry-creds\")'"

# Check CronJob container images reference the mirror registry
check "CronJob images use mirror registry" \
  bash -c "kubectl get cronjobs -n '${NAMESPACE}' -l mondoo_cr=mondoo-client -o json | jq -e '.items[0].spec.jobTemplate.spec.template.spec.containers[0].image' | grep -q '${MIRROR_REGISTRY}'"

# Check operator deployment has imagePullSecrets
check "Operator deployment has imagePullSecrets" \
  bash -c "kubectl get deployment mondoo-operator-controller-manager -n '${NAMESPACE}' -o json | jq -e '.spec.template.spec.imagePullSecrets[] | select(.name==\"mirror-registry-creds\")'"

# Proxy-specific checks
if [[ "${ENABLE_PROXY_TEST}" == "true" && -n "${SQUID_PROXY_IP}" ]]; then
  info ""
  info "--- Proxy Checks ---"

  # Check MondooOperatorConfig has proxy settings
  check "MondooOperatorConfig has httpProxy" \
    bash -c "kubectl get mondoooperatorconfigs.k8s.mondoo.com -n '${NAMESPACE}' -o jsonpath='{.items[0].spec.httpProxy}' | grep -q '${SQUID_PROXY_IP}'"

  check "MondooOperatorConfig has httpsProxy" \
    bash -c "kubectl get mondoooperatorconfigs.k8s.mondoo.com -n '${NAMESPACE}' -o jsonpath='{.items[0].spec.httpsProxy}' | grep -q '${SQUID_PROXY_IP}'"

  # Check that CronJob containers have HTTP_PROXY env vars
  check "CronJob containers have HTTP_PROXY env var" \
    bash -c "kubectl get cronjobs -n '${NAMESPACE}' -l mondoo_cr=mondoo-client -o json | jq -e '.items[0].spec.jobTemplate.spec.template.spec.containers[0].env[] | select(.name==\"HTTP_PROXY\")'"

  check "CronJob containers have HTTPS_PROXY env var" \
    bash -c "kubectl get cronjobs -n '${NAMESPACE}' -l mondoo_cr=mondoo-client -o json | jq -e '.items[0].spec.jobTemplate.spec.template.spec.containers[0].env[] | select(.name==\"HTTPS_PROXY\")'"

  # Check Squid access log for traffic (WARN, not FAIL — timing-dependent)
  info ""
  info "--- Squid Proxy Log Check ---"
  SQUID_ZONE="${REGION:-europe-west3}-a"
  SQUID_INSTANCE="${NAME_PREFIX:-mondoo-e2e}-squid-proxy"

  check_warn "Squid proxy shows access log traffic" \
    bash -c "gcloud compute ssh '${SQUID_INSTANCE}' --zone='${SQUID_ZONE}' --project='${PROJECT_ID}' --tunnel-through-iap --command='sudo tail -20 /var/log/squid/access.log' 2>/dev/null | grep -q ."

  info "Squid access log (last 20 lines):"
  gcloud compute ssh "${SQUID_INSTANCE}" --zone="${SQUID_ZONE}" --project="${PROJECT_ID}" \
    --tunnel-through-iap --command="sudo tail -20 /var/log/squid/access.log" 2>/dev/null || warn "Could not retrieve Squid logs"
else
  info ""
  info "--- Proxy checks skipped (enable_proxy_test != true) ---"
fi

info ""
info "=== Resource Details (Mirroring) ==="

info "--- CronJob Image References ---"
kubectl get cronjobs -n "${NAMESPACE}" -l mondoo_cr=mondoo-client \
  -o jsonpath='{range .items[*]}{.metadata.name}: {.spec.jobTemplate.spec.template.spec.containers[0].image}{"\n"}{end}' 2>/dev/null || true

info ""
info "--- MondooOperatorConfig ---"
kubectl get mondoooperatorconfigs.k8s.mondoo.com -n "${NAMESPACE}" -o yaml 2>/dev/null || true

info ""
info "=== Results: ${PASS} passed, ${FAIL} failed, ${WARN_COUNT} warnings ==="

if [[ ${FAIL} -gt 0 ]]; then
  err "Some checks failed. Review the output above."
  exit 1
fi

info ""
info "=== Manual Verification ==="
info "Check the Mondoo console for assets in the test space"
info "  Space MRN: ${MONDOO_SPACE_MRN:-unknown}"
info "  - Verify assets were scanned successfully through mirrored images"
info "  - Container images should reference the mirror registry"
if [[ "${ENABLE_PROXY_TEST}" == "true" ]]; then
  info "  - Check Squid proxy logs for HTTPS CONNECT traffic"
fi
read -rp "Press Enter once verified (or Ctrl+C to abort)... "

#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Verify WIF container registry scanning resources (cloud-agnostic)
# Uses CR_WIF_ANNOTATION_KEY, cr_wif_annotation_value(), CR_WIF_INIT_IMAGE_PATTERN
# from common-{provider}.sh.

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

info "=== WIF Container Registry Scanning Verification ==="

EXPECTED_CR_WIF_VALUE="$(cr_wif_annotation_value)"

# Check container registry WIF ServiceAccount exists
check "Container registry WIF ServiceAccount exists" \
  kubectl get serviceaccount mondoo-client-cr-wif -n "${NAMESPACE}"

# Check WIF SA has correct annotation
check "Container registry WIF SA has ${CR_WIF_ANNOTATION_KEY} annotation" \
  bash -c "
    ANNOTATION=\$(kubectl get serviceaccount mondoo-client-cr-wif -n '${NAMESPACE}' \
      -o jsonpath='{.metadata.annotations.${CR_WIF_ANNOTATION_KEY//./\\.}}')
    [[ \"\${ANNOTATION}\" == '${EXPECTED_CR_WIF_VALUE}' ]]
  "

# Check containers-scan CronJob exists
check "Container image scanning CronJob exists" \
  bash -c "kubectl get cronjobs -n '${NAMESPACE}' -o name | grep -q containers-scan"

# Check CronJob has generate-registry-creds init container
check "CronJob has generate-registry-creds init container" \
  bash -c "
    kubectl get cronjobs -n '${NAMESPACE}' -l app=mondoo-container-scan \
      -o jsonpath='{.items[0].spec.jobTemplate.spec.template.spec.initContainers[0].name}' \
      | grep -q generate-registry-creds
  "

# Check init container uses the correct cloud CLI image
check "Init container uses ${CR_WIF_INIT_IMAGE_PATTERN} image" \
  bash -c "
    kubectl get cronjobs -n '${NAMESPACE}' -l app=mondoo-container-scan \
      -o jsonpath='{.items[0].spec.jobTemplate.spec.template.spec.initContainers[0].image}' \
      | grep -q '${CR_WIF_INIT_IMAGE_PATTERN}'
  "

# Check CronJob uses the WIF ServiceAccount
check "CronJob uses container registry WIF ServiceAccount" \
  bash -c "
    SA=\$(kubectl get cronjobs -n '${NAMESPACE}' -l app=mondoo-container-scan \
      -o jsonpath='{.items[0].spec.jobTemplate.spec.template.spec.serviceAccountName}')
    [[ \"\${SA}\" == 'mondoo-client-cr-wif' ]]
  "

# Check CronJob has docker-config emptyDir volume
check "CronJob has docker-config emptyDir volume" \
  bash -c "
    kubectl get cronjobs -n '${NAMESPACE}' -l app=mondoo-container-scan \
      -o jsonpath='{.items[0].spec.jobTemplate.spec.template.spec.volumes}' \
      | grep -q 'docker-config'
  "

# Check main container has DOCKER_CONFIG env var
check "Main container has DOCKER_CONFIG env var" \
  bash -c "
    kubectl get cronjobs -n '${NAMESPACE}' -l app=mondoo-container-scan \
      -o jsonpath='{.items[0].spec.jobTemplate.spec.template.spec.containers[0].env}' \
      | grep -q 'DOCKER_CONFIG'
  "

# Check no static pull-secrets volume (WIF manages auth)
check "No static pull-secrets volume (WIF manages auth)" \
  bash -c "
    ! kubectl get cronjobs -n '${NAMESPACE}' -l app=mondoo-container-scan \
      -o jsonpath='{.items[0].spec.jobTemplate.spec.template.spec.volumes[*].name}' \
      | grep -q 'pull-secrets'
  "

# Check private-image workload is running (proves the image was pushable)
check "Private-image workload (nginx-private-workload) is running" \
  kubectl get deployment nginx-private-workload -n default

# Check that at least one container-scan Job completed successfully (init container worked)
check "At least one container-scan Job completed" \
  bash -c "
    kubectl get jobs -n '${NAMESPACE}' -l app=mondoo-container-scan \
      -o jsonpath='{.items[*].status.succeeded}' | grep -q '1'
  "

info ""
info "=== Container Registry WIF Resource Details ==="

info "--- Container Registry WIF ServiceAccount ---"
kubectl get serviceaccount mondoo-client-cr-wif -n "${NAMESPACE}" -o yaml 2>/dev/null | head -20 || true

info ""
info "--- Container Scan CronJobs ---"
kubectl get cronjobs -n "${NAMESPACE}" -l app=mondoo-container-scan -o wide 2>/dev/null || true

info ""
info "=== Container Registry WIF Results: ${PASS} passed, ${FAIL} failed ==="

if [[ ${FAIL} -gt 0 ]]; then
  err "Some container registry WIF checks failed. Review the output above."
  exit 1
fi

info ""
info "=== Manual Verification ==="
info "Check the Mondoo console for WIF-authenticated container image scan results"
info "  Space MRN: ${MONDOO_SPACE_MRN:-unknown}"
info "  - Container images from the scanner cluster should appear as scanned assets"
info "  - Auth was via ${WIF_AUTH_DESCRIPTION} (no static imagePullSecrets)"
read -rp "Press Enter once verified (or Ctrl+C to abort)... "

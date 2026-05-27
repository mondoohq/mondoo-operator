#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Verify WIF external cluster scanning with endpoint override.
# Extends verify-wif-external.sh with additional checks that the endpoint
# override env var is set on the init container.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

: "${NAMESPACE:?NAMESPACE must be set}"
: "${TARGET_CLUSTER_ENDPOINT:?TARGET_CLUSTER_ENDPOINT must be set}"

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

info "=== WIF Endpoint Override Verification ==="

EXPECTED_WIF_VALUE="$(wif_annotation_value)"

# --- Standard WIF checks (same as verify-wif-external.sh) ---

check "WIF ServiceAccount exists" \
  kubectl get serviceaccount mondoo-client-wif-target-cluster -n "${NAMESPACE}"

check "WIF SA has ${WIF_ANNOTATION_KEY} annotation" \
  bash -c "
    ANNOTATION=\$(kubectl get serviceaccount mondoo-client-wif-target-cluster -n '${NAMESPACE}' \
      -o jsonpath='{.metadata.annotations.${WIF_ANNOTATION_KEY//./\\.}}')
    [[ \"\${ANNOTATION}\" == '${EXPECTED_WIF_VALUE}' ]]
  "

check "CronJob for WIF external cluster exists" \
  bash -c "kubectl get cronjobs -n '${NAMESPACE}' -l mondoo_cr=mondoo-client -o name | grep -q target-cluster"

check "CronJob has generate-kubeconfig init container" \
  bash -c "
    kubectl get cronjobs -n '${NAMESPACE}' -l cluster_name=target-cluster \
      -o jsonpath='{.items[0].spec.jobTemplate.spec.template.spec.initContainers[0].name}' \
      | grep -q generate-kubeconfig
  "

check "Init container uses ${WIF_INIT_IMAGE_PATTERN} image" \
  bash -c "
    kubectl get cronjobs -n '${NAMESPACE}' -l cluster_name=target-cluster \
      -o jsonpath='{.items[0].spec.jobTemplate.spec.template.spec.initContainers[0].image}' \
      | grep -q '${WIF_INIT_IMAGE_PATTERN}'
  "

# --- Endpoint override specific checks ---

# Determine the expected env var name based on cloud provider.
# EKS uses ENDPOINT (native --endpoint flag), GKE/AKS use ENDPOINT_OVERRIDE.
CLOUD_PROVIDER="$(basename "${CLOUD_DIR}")"
case "${CLOUD_PROVIDER}" in
  eks) ENDPOINT_ENV_NAME="ENDPOINT" ;;
  *)   ENDPOINT_ENV_NAME="ENDPOINT_OVERRIDE" ;;
esac

check "Init container has ${ENDPOINT_ENV_NAME} env var" \
  bash -c "
    kubectl get cronjobs -n '${NAMESPACE}' -l cluster_name=target-cluster \
      -o jsonpath='{.items[0].spec.jobTemplate.spec.template.spec.initContainers[0].env[*].name}' \
      | tr ' ' '\n' | grep -q '${ENDPOINT_ENV_NAME}'
  "

check "Init container ${ENDPOINT_ENV_NAME} value matches target endpoint" \
  bash -c "
    ENV_JSON=\$(kubectl get cronjobs -n '${NAMESPACE}' -l cluster_name=target-cluster \
      -o jsonpath='{.items[0].spec.jobTemplate.spec.template.spec.initContainers[0].env}')
    ACTUAL=\$(echo \"\${ENV_JSON}\" | python3 -c \"
import json, sys
envs = json.loads(sys.stdin.read())
for e in envs:
    if e['name'] == '${ENDPOINT_ENV_NAME}':
        print(e['value'])
        break
\")
    [[ \"\${ACTUAL}\" == '${TARGET_CLUSTER_ENDPOINT}' ]]
  "

check "No static target-kubeconfig Secret (WIF manages auth)" \
  bash -c "! kubectl get secret target-kubeconfig -n '${NAMESPACE}' 2>/dev/null"

check "Inventory ConfigMap for external cluster exists" \
  bash -c "kubectl get configmaps -n '${NAMESPACE}' -l cluster_name=target-cluster -o name | grep -q inventory"

# --- Wait for at least one Job to be created and check its init container ---

info ""
info "Waiting for a scan Job to be created (up to 360s)..."
JOB_FOUND="false"
for i in $(seq 1 72); do
  JOB_NAME=$(kubectl get jobs -n "${NAMESPACE}" -l cluster_name=target-cluster \
    -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)
  if [[ -n "${JOB_NAME}" ]]; then
    JOB_FOUND="true"
    break
  fi
  sleep 5
done

if [[ "${JOB_FOUND}" == "true" ]]; then
  check "Scan Job was created from CronJob" test -n "${JOB_NAME}"

  # Check that the Job's pod init container completed successfully
  info "Waiting for Job pod to start (up to 120s)..."
  POD_NAME=""
  for i in $(seq 1 24); do
    POD_NAME=$(kubectl get pods -n "${NAMESPACE}" -l job-name="${JOB_NAME}" \
      -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)
    if [[ -n "${POD_NAME}" ]]; then
      break
    fi
    sleep 5
  done

  if [[ -n "${POD_NAME}" ]]; then
    info "Found pod: ${POD_NAME}"

    # Wait for init container to complete
    info "Waiting for init container to finish (up to 120s)..."
    for i in $(seq 1 24); do
      INIT_STATUS=$(kubectl get pod "${POD_NAME}" -n "${NAMESPACE}" \
        -o jsonpath='{.status.initContainerStatuses[0].state}' 2>/dev/null || true)
      if echo "${INIT_STATUS}" | grep -q "terminated"; then
        break
      fi
      sleep 5
    done

    INIT_EXIT=$(kubectl get pod "${POD_NAME}" -n "${NAMESPACE}" \
      -o jsonpath='{.status.initContainerStatuses[0].state.terminated.exitCode}' 2>/dev/null || echo "unknown")

    check "Init container (generate-kubeconfig) exited successfully" \
      test "${INIT_EXIT}" = "0"

    if [[ "${INIT_EXIT}" != "0" ]]; then
      err "Init container logs:"
      kubectl logs "${POD_NAME}" -n "${NAMESPACE}" -c generate-kubeconfig 2>/dev/null || true
    fi
  else
    err "FAIL: No pod found for Job ${JOB_NAME}"
    FAIL=$((FAIL + 1))
  fi
else
  err "FAIL: No scan Job was created within timeout"
  FAIL=$((FAIL + 1))
fi

info ""
info "=== WIF Endpoint Override Results: ${PASS} passed, ${FAIL} failed ==="

if [[ ${FAIL} -gt 0 ]]; then
  err "Some endpoint override checks failed. Review the output above."
  exit 1
fi

info ""
info "=== Manual Verification ==="
info "Check the Mondoo console for WIF-authenticated external cluster assets"
info "  Space MRN: ${MONDOO_SPACE_MRN:-unknown}"
info "  - Target cluster K8s resources should appear as assets"
info "  - Auth was via ${WIF_AUTH_DESCRIPTION} with endpoint override: ${TARGET_CLUSTER_ENDPOINT}"
read -rp "Press Enter once verified (or Ctrl+C to abort)... "

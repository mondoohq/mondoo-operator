#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Verify WIF external cluster scanning resources are created

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

: "${NAMESPACE:?NAMESPACE must be set}"
: "${WIF_GSA_EMAIL:?WIF_GSA_EMAIL must be set}"

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

info "=== WIF External Cluster Verification ==="

# Check WIF ServiceAccount exists
check "WIF ServiceAccount exists" \
  kubectl get serviceaccount mondoo-client-wif-target-cluster -n "${NAMESPACE}"

# Check WIF SA has correct annotation
check "WIF SA has iam.gke.io/gcp-service-account annotation" \
  bash -c "
    ANNOTATION=\$(kubectl get serviceaccount mondoo-client-wif-target-cluster -n '${NAMESPACE}' \
      -o jsonpath='{.metadata.annotations.iam\\.gke\\.io/gcp-service-account}')
    [[ \"\${ANNOTATION}\" == '${WIF_GSA_EMAIL}' ]]
  "

# Check CronJob for external cluster exists
check "CronJob for WIF external cluster exists" \
  bash -c "kubectl get cronjobs -n '${NAMESPACE}' -l mondoo_cr=mondoo-client -o name | grep -q target-cluster"

# Check CronJob has generate-kubeconfig init container
check "CronJob has generate-kubeconfig init container" \
  bash -c "
    kubectl get cronjobs -n '${NAMESPACE}' -l cluster_name=target-cluster \
      -o jsonpath='{.items[0].spec.jobTemplate.spec.template.spec.initContainers[0].name}' \
      | grep -q generate-kubeconfig
  "

# Check init container uses gcloud image
check "Init container uses gcloud image" \
  bash -c "
    kubectl get cronjobs -n '${NAMESPACE}' -l cluster_name=target-cluster \
      -o jsonpath='{.items[0].spec.jobTemplate.spec.template.spec.initContainers[0].image}' \
      | grep -q 'google-cloud-cli'
  "

# Check CronJob uses the WIF ServiceAccount
check "CronJob uses WIF ServiceAccount" \
  bash -c "
    SA=\$(kubectl get cronjobs -n '${NAMESPACE}' -l cluster_name=target-cluster \
      -o jsonpath='{.items[0].spec.jobTemplate.spec.template.spec.serviceAccountName}')
    [[ \"\${SA}\" == 'mondoo-client-wif-target-cluster' ]]
  "

# Check CronJob has kubeconfig emptyDir volume
check "CronJob has kubeconfig emptyDir volume" \
  bash -c "
    kubectl get cronjobs -n '${NAMESPACE}' -l cluster_name=target-cluster \
      -o jsonpath='{.items[0].spec.jobTemplate.spec.template.spec.volumes}' \
      | grep -q 'kubeconfig'
  "

# Check no static target-kubeconfig Secret exists (WIF manages auth)
check "No static target-kubeconfig Secret (WIF manages auth)" \
  bash -c "! kubectl get secret target-kubeconfig -n '${NAMESPACE}' 2>/dev/null"

# Check inventory ConfigMap exists
check "Inventory ConfigMap for external cluster exists" \
  bash -c "kubectl get configmaps -n '${NAMESPACE}' -l cluster_name=target-cluster -o name | grep -q inventory"

info ""
info "=== WIF External Cluster Resource Details ==="

info "--- WIF ServiceAccount ---"
kubectl get serviceaccount mondoo-client-wif-target-cluster -n "${NAMESPACE}" -o yaml 2>/dev/null | head -20 || true

info ""
info "--- CronJobs (filtered for target-cluster) ---"
kubectl get cronjobs -n "${NAMESPACE}" -l mondoo_cr=mondoo-client -o wide 2>/dev/null | grep -E "NAME|target-cluster" || true

info ""
info "=== WIF External Cluster Results: ${PASS} passed, ${FAIL} failed ==="

if [[ ${FAIL} -gt 0 ]]; then
  err "Some WIF external cluster checks failed. Review the output above."
  exit 1
fi

info ""
info "=== Manual Verification ==="
info "Check the Mondoo console for WIF-authenticated external cluster assets"
info "  Space MRN: ${MONDOO_SPACE_MRN:-unknown}"
info "  - Target cluster K8s resources should appear as assets"
info "  - nginx-test-workload from the target cluster should be visible"
info "  - Auth was via GKE Workload Identity (no static kubeconfig Secret)"
read -rp "Press Enter once verified (or Ctrl+C to abort)... "

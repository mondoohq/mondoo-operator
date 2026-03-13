#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Verify Vault-authenticated external cluster scanning resources are created

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

info "=== Vault External Cluster Verification ==="

# Check that NO static target-kubeconfig Secret exists (Vault manages auth)
check "No static target-kubeconfig Secret (Vault manages auth)" \
  bash -c "! kubectl get secret target-kubeconfig -n '${NAMESPACE}' 2>/dev/null"

# Check the operator-created vault-kubeconfig Secret exists
check "Vault-generated kubeconfig Secret exists" \
  bash -c "kubectl get secrets -n '${NAMESPACE}' -o name | grep -q vault-kubeconfig"

# Verify the vault-kubeconfig Secret has a 'kubeconfig' data key
check "Vault kubeconfig Secret has 'kubeconfig' key" \
  bash -c "kubectl get secrets -n '${NAMESPACE}' -l cluster_name=target-cluster -o jsonpath='{.items[0].data.kubeconfig}' | base64 -d | grep -q 'server:'"

# Check CronJob for external cluster scanning exists
check "CronJob for Vault external cluster exists" \
  bash -c "kubectl get cronjobs -n '${NAMESPACE}' -l mondoo_cr=mondoo-client -o name | grep -q target-cluster"

# Check the CronJob does NOT have init containers (Vault auth is operator-side)
check "CronJob has no init containers" \
  bash -c "
    INIT_CONTAINERS=\$(kubectl get cronjobs -n '${NAMESPACE}' -l cluster_name=target-cluster \
      -o jsonpath='{.items[0].spec.jobTemplate.spec.template.spec.initContainers}')
    [[ -z \"\${INIT_CONTAINERS}\" || \"\${INIT_CONTAINERS}\" == 'null' ]]
  "

# Check automountServiceAccountToken is false on the CronJob
check "CronJob has automountServiceAccountToken=false" \
  bash -c "
    AUTOMOUNT=\$(kubectl get cronjobs -n '${NAMESPACE}' -l cluster_name=target-cluster \
      -o jsonpath='{.items[0].spec.jobTemplate.spec.template.spec.automountServiceAccountToken}')
    [[ \"\${AUTOMOUNT}\" == 'false' ]]
  "

# Check the CronJob mounts the vault-kubeconfig volume
check "CronJob mounts vault-kubeconfig volume" \
  bash -c "
    kubectl get cronjobs -n '${NAMESPACE}' -l cluster_name=target-cluster \
      -o jsonpath='{.items[0].spec.jobTemplate.spec.template.spec.volumes[*].secret.secretName}' \
      | grep -q vault-kubeconfig
  "

# Check the ConfigMap inventory was created for the external cluster
check "Inventory ConfigMap for external cluster exists" \
  bash -c "kubectl get configmaps -n '${NAMESPACE}' -l cluster_name=target-cluster -o name | grep -q inventory"

# Verify the kubeconfig has the correct target server
check "Kubeconfig contains target server URL" \
  bash -c "
    kubectl get secrets -n '${NAMESPACE}' -l cluster_name=target-cluster \
      -o jsonpath='{.items[0].data.kubeconfig}' | base64 -d \
      | grep -q '${VAULT_TARGET_SERVER:-https://}'
  "

# Verify the kubeconfig has certificate-authority-data (since we provided targetCACertSecretRef)
check "Kubeconfig has certificate-authority-data (not insecure)" \
  bash -c "
    kubectl get secrets -n '${NAMESPACE}' -l cluster_name=target-cluster \
      -o jsonpath='{.items[0].data.kubeconfig}' | base64 -d \
      | grep -q 'certificate-authority-data'
  "

# Check the target CA cert Secret was created
check "Target CA cert Secret exists" \
  kubectl get secret vault-target-ca-cert -n "${NAMESPACE}"

# Verify Vault pod is still healthy
check "Vault pod is running" \
  bash -c "kubectl get pod vault-0 -n vault -o jsonpath='{.status.phase}' | grep -q Running"

info ""
info "=== Vault External Cluster Resource Details ==="

info "--- Vault-Generated Kubeconfig Secrets ---"
kubectl get secrets -n "${NAMESPACE}" -l cluster_name=target-cluster -o wide 2>/dev/null || true

info ""
info "--- CronJobs (filtered for target-cluster) ---"
kubectl get cronjobs -n "${NAMESPACE}" -l mondoo_cr=mondoo-client -o wide 2>/dev/null | grep -E "NAME|target-cluster" || true

info ""
info "--- MondooAuditConfig Status ---"
kubectl get mondooauditconfigs.k8s.mondoo.com -n "${NAMESPACE}" -o yaml 2>/dev/null \
  | grep -A 20 "status:" || true

info ""
info "=== Vault External Cluster Results: ${PASS} passed, ${FAIL} failed ==="

if [[ ${FAIL} -gt 0 ]]; then
  err "Some Vault external cluster checks failed. Review the output above."
  exit 1
fi

info ""
info "=== Manual Verification ==="
info "Check the Mondoo console for Vault-authenticated external cluster assets"
info "  Space MRN: ${MONDOO_SPACE_MRN:-unknown}"
info "  - Target cluster K8s resources should appear as assets"
info "  - nginx-test-workload from the target cluster should be visible"
info "  - Auth was via Vault (no static kubeconfig Secret)"
read -rp "Press Enter once verified (or Ctrl+C to abort)... "

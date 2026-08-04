#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Test: Token Exchange Coexistence
#
# Verifies that the token exchange path (enable: true + mondooTokenSecretRef)
# works correctly alongside the auto-create default (autoCreate: true).
#
# The key invariant: when consoleIntegration.enable is true, the token exchange
# path handles integration registration and autoCreateIntegration does NOT
# interfere (ConsoleIntegrationActive() returns true via enable, so it skips).
#
# Prerequisites:
#   - Operator deployed on cluster (e.g. via deploy-operator.sh)
#   - MONDOO_SPACE_MRN set
#   - Mondoo API credentials available (MONDOO_CONFIG_PATH, MONDOO_CONFIG_BASE64,
#     or MONDOO_CREDS_B64)
#   - NAMESPACE set (default: mondoo-operator)
#
# Usage:
#   NAMESPACE=mondoo-operator MONDOO_SPACE_MRN=... ./test-token-exchange-coexistence.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
E2E_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_ROOT="$(cd "${E2E_DIR}/../.." && pwd)"

info()  { echo "[INFO]  $(date '+%H:%M:%S') $*"; }
err()   { echo "[ERROR] $(date '+%H:%M:%S') $*" >&2; }
die()   { err "$@"; exit 1; }

: "${NAMESPACE:=mondoo-operator}"
: "${MONDOO_SPACE_MRN:?MONDOO_SPACE_MRN must be set}"

PASS=0
FAIL=0
INTEGRATION_MRN=""
TEST_START_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

check() {
  local desc="$1"; shift
  if "$@" >/dev/null 2>&1; then
    info "PASS: ${desc}"
    PASS=$((PASS + 1))
  else
    err "FAIL: ${desc}"
    FAIL=$((FAIL + 1))
  fi
}

cleanup() {
  info "=== Cleanup ==="

  # Delete MondooAuditConfig (strip finalizers to avoid waiting for integration cleanup)
  kubectl patch mondooauditconfig mondoo-client -n "${NAMESPACE}" \
    --type=merge -p '{"metadata":{"finalizers":[]}}' 2>/dev/null || true
  kubectl delete mondooauditconfig mondoo-client -n "${NAMESPACE}" \
    --ignore-not-found --timeout=30s 2>/dev/null || true

  # Delete secrets
  kubectl delete secret mondoo-client -n "${NAMESPACE}" --ignore-not-found
  kubectl delete secret mondoo-token -n "${NAMESPACE}" --ignore-not-found
  kubectl delete secret mondoo-client-provisioner -n "${NAMESPACE}" --ignore-not-found

  # Delete integration from Mondoo console
  if [[ -n "${INTEGRATION_MRN}" ]]; then
    info "Deleting integration ${INTEGRATION_MRN}..."
    INTEGRATION_MRN="${INTEGRATION_MRN}" \
      go run "${REPO_ROOT}/tests/e2e/cmd/integration" delete 2>/dev/null || true
  fi

  info "Cleanup done."
}

trap cleanup EXIT

# ──────────────────────────────────────────────
# Step 1: Create integration via API + get token
# ──────────────────────────────────────────────
info "=== Step 1: Create integration ==="

export MONDOO_SPACE_MRN
export TF_DIR="${TF_DIR:-/tmp}"

OUTPUT=$(go run "${REPO_ROOT}/tests/e2e/cmd/integration" create)
INTEGRATION_MRN=$(echo "${OUTPUT}" | grep '^MRN=' | cut -d= -f2-)
INTEGRATION_TOKEN=$(echo "${OUTPUT}" | grep '^TOKEN=' | cut -d= -f2-)

[[ -n "${INTEGRATION_MRN}" ]] || die "Failed to create integration"
[[ -n "${INTEGRATION_TOKEN}" ]] || die "No token returned"
info "Created integration: ${INTEGRATION_MRN}"

# ──────────────────────────────────────────────
# Step 2: Create token secret
# ──────────────────────────────────────────────
info "=== Step 2: Create token secret ==="

kubectl delete secret mondoo-token -n "${NAMESPACE}" --ignore-not-found
kubectl create secret generic mondoo-token \
  --from-literal=token="${INTEGRATION_TOKEN}" \
  --namespace "${NAMESPACE}"

# Delete any stale creds secret so the token exchange can create it fresh
kubectl delete secret mondoo-client -n "${NAMESPACE}" --ignore-not-found

# ──────────────────────────────────────────────
# Step 3: Apply MondooAuditConfig with enable: true
# ──────────────────────────────────────────────
info "=== Step 3: Apply MondooAuditConfig ==="

# Note: autoCreate defaults to true via the CRD, so this tests coexistence.
# The enable: true + mondooTokenSecretRef triggers the token exchange path.
# autoCreateIntegration should be a no-op because ConsoleIntegrationActive()
# returns true (via enable).
kubectl apply -f - <<EOF
apiVersion: k8s.mondoo.com/v1alpha2
kind: MondooAuditConfig
metadata:
  name: mondoo-client
  namespace: ${NAMESPACE}
spec:
  mondooCredsSecretRef:
    name: mondoo-client
  mondooTokenSecretRef:
    name: mondoo-token
  consoleIntegration:
    enable: true
  kubernetesResources:
    enable: true
    schedule: "*/5 * * * *"
  containers:
    enable: true
    schedule: "*/5 * * * *"
EOF

# ──────────────────────────────────────────────
# Step 4: Wait for token exchange to complete
# ──────────────────────────────────────────────
info "=== Step 4: Wait for token exchange ==="

info "Waiting for mondoo-client secret to be created by token exchange..."
for i in $(seq 1 30); do
  if kubectl get secret mondoo-client -n "${NAMESPACE}" >/dev/null 2>&1; then
    break
  fi
  sleep 2
done

check "mondoo-client secret created" \
  kubectl get secret mondoo-client -n "${NAMESPACE}"

# ──────────────────────────────────────────────
# Step 5: Verify token exchange path worked
# ──────────────────────────────────────────────
info "=== Step 5: Verify ==="

# The creds secret should have both the SA and integration MRN
check "creds secret has service account key" \
  bash -c "kubectl get secret mondoo-client -n '${NAMESPACE}' -o jsonpath='{.data.config}' | base64 -d | grep -q mrn"

check "creds secret has integration MRN" \
  bash -c "kubectl get secret mondoo-client -n '${NAMESPACE}' -o jsonpath='{.data.integrationmrn}' | base64 -d | grep -q '${INTEGRATION_MRN}'"

# The integration MRN in the secret should match the one we created
STORED_MRN=$(kubectl get secret mondoo-client -n "${NAMESPACE}" \
  -o jsonpath='{.data.integrationmrn}' 2>/dev/null | base64 -d 2>/dev/null || echo "")
check "integration MRN matches the pre-created one" \
  test "${STORED_MRN}" = "${INTEGRATION_MRN}"

# spec.consoleIntegration.enable should still be true (not modified)
ENABLE=$(kubectl get mondooauditconfig mondoo-client -n "${NAMESPACE}" \
  -o jsonpath='{.spec.consoleIntegration.enable}')
check "spec.consoleIntegration.enable is still true" \
  test "${ENABLE}" = "true"

# No provisioner secret should exist (auto-create didn't fire)
check "no provisioner secret (auto-create did not run)" \
  bash -c "! kubectl get secret mondoo-client-provisioner -n '${NAMESPACE}' 2>/dev/null"

# Wait for integration controller to check in (runs on a 10 min timer)
info "Waiting up to 11 min for integration controller check-in..."
for i in $(seq 1 132); do
  STATUS=$(kubectl get mondooauditconfig mondoo-client -n "${NAMESPACE}" \
    -o jsonpath='{.status.conditions[?(@.type=="IntegrationDegraded")].status}' 2>/dev/null)
  if [[ "${STATUS}" == "False" ]]; then break; fi
  sleep 5
done

INTEGRATION_STATUS=$(kubectl get mondooauditconfig mondoo-client -n "${NAMESPACE}" \
  -o jsonpath='{.status.conditions[?(@.type=="IntegrationDegraded")].status}')
check "IntegrationDegraded is False (check-in working)" \
  test "${INTEGRATION_STATUS}" = "False"

# ──────────────────────────────────────────────
# Step 6: Verify auto-create did NOT interfere
# ──────────────────────────────────────────────
info "=== Step 6: Check auto-create did not interfere ==="

# Check operator logs for auto-create activity (only since test started)
AUTO_CREATE_LOG=$(kubectl logs deployment/mondoo-operator-controller-manager \
  -n "${NAMESPACE}" --since-time="${TEST_START_TIME}" 2>&1 | grep "auto-creating console integration" || true)
check "no auto-create log messages" \
  test -z "${AUTO_CREATE_LOG}"

# status.integrationMRN should be empty (auto-create didn't set it;
# the token exchange path uses enable: true, not status.integrationMRN)
STATUS_MRN=$(kubectl get mondooauditconfig mondoo-client -n "${NAMESPACE}" \
  -o jsonpath='{.status.integrationMRN}' 2>/dev/null || echo "")
check "status.integrationMRN is empty (auto-create path not used)" \
  test -z "${STATUS_MRN}"

# ──────────────────────────────────────────────
# Results
# ──────────────────────────────────────────────
info ""
info "=== Results: ${PASS} passed, ${FAIL} failed ==="

if [[ ${FAIL} -gt 0 ]]; then
  err "Some checks failed."
  info "--- Operator logs (last 30 lines) ---"
  kubectl logs deployment/mondoo-operator-controller-manager \
    -n "${NAMESPACE}" --tail=30 2>&1 || true
  exit 1
fi

info "Token exchange + auto-create coexistence: OK"

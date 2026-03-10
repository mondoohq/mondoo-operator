#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Clean up all resources created by e2e test suites (everything except Terraform infra)

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

NAMESPACE="${NAMESPACE:-mondoo-operator}"

info "=== E2E Cleanup ==="

# Strip finalizers from MondooAuditConfigs so they can be deleted cleanly
info "Removing finalizers from MondooAuditConfigs..."
for mac in $(kubectl get mondooauditconfigs.k8s.mondoo.com -n "${NAMESPACE}" -o name 2>/dev/null || true); do
  kubectl patch "${mac}" -n "${NAMESPACE}" --type=merge -p '{"metadata":{"finalizers":[]}}' 2>/dev/null || true
done

# Delete MondooAuditConfigs
info "Deleting MondooAuditConfigs..."
kubectl delete mondooauditconfigs.k8s.mondoo.com --all -n "${NAMESPACE}" --ignore-not-found --timeout=30s || true

# Mondoo credentials secret
info "Deleting mondoo-client secret..."
kubectl delete secret mondoo-client -n "${NAMESPACE}" --ignore-not-found

# Target cluster kubeconfig secret
info "Deleting target-kubeconfig secret..."
kubectl delete secret target-kubeconfig -n "${NAMESPACE}" --ignore-not-found

# Helm release
info "Uninstalling mondoo-operator Helm release..."
helm uninstall mondoo-operator -n "${NAMESPACE}" --wait --timeout 2m 2>/dev/null || true

# Namespace — patch any remaining resources with finalizers that block deletion
info "Deleting namespace ${NAMESPACE}..."
kubectl delete namespace "${NAMESPACE}" --ignore-not-found --timeout=30s 2>/dev/null || {
  warn "Namespace deletion timed out, clearing stuck finalizers..."
  # Find and patch any resources still stuck with finalizers
  for resource in $(kubectl api-resources --verbs=list --namespaced -o name 2>/dev/null); do
    for item in $(kubectl get "${resource}" -n "${NAMESPACE}" -o name 2>/dev/null || true); do
      kubectl patch "${item}" -n "${NAMESPACE}" --type=merge -p '{"metadata":{"finalizers":[]}}' 2>/dev/null || true
    done
  done
  kubectl delete namespace "${NAMESPACE}" --ignore-not-found --timeout=60s || true
}

# CRDs
info "Deleting Mondoo CRDs..."
kubectl delete crds mondooauditconfigs.k8s.mondoo.com mondoooperatorconfigs.k8s.mondoo.com --ignore-not-found || true

# Test workload
info "Deleting nginx test workload..."
kubectl delete deployment nginx-test-workload -n default --ignore-not-found

# Target cluster test workload
if [[ -n "${TARGET_KUBECONFIG_PATH:-}" && -f "${TARGET_KUBECONFIG_PATH}" ]]; then
  info "Deleting nginx test workload on target cluster..."
  kubectl --kubeconfig "${TARGET_KUBECONFIG_PATH}" delete deployment nginx-test-workload \
    -n default --ignore-not-found || true
fi

# Helm repo
info "Removing mondoo Helm repo..."
helm repo remove mondoo 2>/dev/null || true

info "=== Cleanup complete ==="

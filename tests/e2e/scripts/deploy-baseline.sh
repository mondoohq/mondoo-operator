#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Deploy a released baseline version of the operator from the Helm repo

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

: "${BASELINE_VERSION:?BASELINE_VERSION must be set}"
: "${NAMESPACE:?NAMESPACE must be set}"

HELM_REPO_URL="https://mondoohq.github.io/mondoo-operator"

info "Deploying baseline operator version ${BASELINE_VERSION}..."

# Adopt any existing Mondoo CRDs so Helm can manage them
for crd in $(kubectl get crds -o name 2>/dev/null | grep mondoo || true); do
  info "Adopting existing CRD for Helm: ${crd}"
  kubectl label "${crd}" app.kubernetes.io/managed-by=Helm --overwrite
  kubectl annotate "${crd}" meta.helm.sh/release-name=mondoo-operator meta.helm.sh/release-namespace="${NAMESPACE}" --overwrite
done

helm repo add mondoo "${HELM_REPO_URL}" --force-update
helm repo update mondoo

helm upgrade --install mondoo-operator mondoo/mondoo-operator \
  --version "${BASELINE_VERSION}" \
  --namespace "${NAMESPACE}" --create-namespace \
  --wait --timeout 5m

wait_for_deployment "${NAMESPACE}" "mondoo-operator-controller-manager"

info "Baseline operator v${BASELINE_VERSION} deployed successfully."

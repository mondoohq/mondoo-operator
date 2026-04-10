#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Common utilities for e2e test scripts (cloud-agnostic)

set -euo pipefail

# Logging
info()  { echo "[INFO]  $(date '+%H:%M:%S') $*"; }
warn()  { echo "[WARN]  $(date '+%H:%M:%S') $*" >&2; }
err()   { echo "[ERROR] $(date '+%H:%M:%S') $*" >&2; }
die()   { err "$@"; exit 1; }

# Resolve paths
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
E2E_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_ROOT="$(cd "${E2E_DIR}/../.." && pwd)"

# CLOUD_DIR must be set by the runner script (e.g., tests/e2e/gke or tests/e2e/eks)
: "${CLOUD_DIR:?CLOUD_DIR must be set before sourcing common.sh}"

TF_DIR="${CLOUD_DIR}/terraform"
MANIFESTS_DIR="${CLOUD_DIR}/manifests"
SHARED_MANIFESTS_DIR="${E2E_DIR}/manifests"

export SCRIPT_DIR E2E_DIR REPO_ROOT TF_DIR CLOUD_DIR MANIFESTS_DIR SHARED_MANIFESTS_DIR

# Load Terraform outputs into environment variables.
# Delegates to cloud-specific loader (common-gke.sh or common-eks.sh).
load_tf_outputs() {
  info "Loading Terraform outputs..."
  cd "${TF_DIR}"

  # Common outputs (all clouds)
  export REGION="$(terraform output -raw region)"
  export CLUSTER_NAME="$(terraform output -raw cluster_name)"
  export MONDOO_CREDS_B64="$(terraform output -raw mondoo_credentials_b64)"
  export MONDOO_SPACE_MRN="$(terraform output -raw mondoo_space_mrn)"
  export NAME_PREFIX="$(terraform output -raw name_prefix)"
  export GIT_SHA="$(git -C "${REPO_ROOT}" rev-parse --short HEAD)"
  export NAMESPACE="mondoo-operator"

  export ENABLE_TARGET_CLUSTER="$(terraform output -raw enable_target_cluster 2>/dev/null || echo "false")"
  if [[ "${ENABLE_TARGET_CLUSTER}" == "true" ]]; then
    export TARGET_CLUSTER_NAME="$(terraform output -raw target_cluster_name)"
    export TARGET_KUBECONFIG_PATH="$(cd "${TF_DIR}" && realpath "$(terraform output -raw target_kubeconfig_path)")"
  fi

  export ENABLE_WIF_TEST="$(terraform output -raw enable_wif_test 2>/dev/null || echo "false")"

  export ENABLE_SPACE_SPLITTING_TEST="$(terraform output -raw enable_space_splitting_test 2>/dev/null || echo "false")"
  if [[ "${ENABLE_SPACE_SPLITTING_TEST}" == "true" ]]; then
    export SCANNER_SPACE_ID="$(terraform output -raw scanner_space_id)"
    export TARGET_SPACE_ID="$(terraform output -raw target_space_id)"
    export TARGET_SPACE_MRN="$(terraform output -raw target_space_mrn)"
    export ORG_CREDS_B64="$(terraform output -raw org_credentials_b64)"
  fi

  cd ->/dev/null

  # Delegate to cloud-specific loader for remaining outputs and credential setup
  _load_cloud_outputs

  info "Loaded outputs: cluster=${CLUSTER_NAME}, repo=${REGISTRY_REPO}"
  if [[ "${ENABLE_TARGET_CLUSTER}" == "true" ]]; then
    info "Target cluster enabled: ${TARGET_CLUSTER_NAME}"
  fi
}

# Wait for a deployment to be rolled out
# Usage: wait_for_deployment <namespace> <deployment-name> [timeout]
wait_for_deployment() {
  local ns="$1" name="$2" timeout="${3:-300s}"
  info "Waiting for deployment ${ns}/${name} (timeout: ${timeout})..."
  kubectl rollout status deployment/"${name}" -n "${ns}" --timeout="${timeout}"
}

# Wait for pods matching a label selector to be ready
# Usage: wait_for_pods_ready <namespace> <label-selector> [timeout]
wait_for_pods_ready() {
  local ns="$1" selector="$2" timeout="${3:-300s}"
  info "Waiting for pods with selector '${selector}' in ${ns} (timeout: ${timeout})..."
  kubectl wait --for=condition=Ready pods -l "${selector}" -n "${ns}" --timeout="${timeout}"
}

# Source the cloud-specific common script
CLOUD_PROVIDER="$(basename "${CLOUD_DIR}")"
source "${SCRIPT_DIR}/common-${CLOUD_PROVIDER}.sh"

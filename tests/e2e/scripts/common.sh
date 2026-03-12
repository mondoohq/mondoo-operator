#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Common utilities for e2e test scripts

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
TF_DIR="${E2E_DIR}/terraform"

export SCRIPT_DIR E2E_DIR REPO_ROOT TF_DIR

# Load Terraform outputs into environment variables
load_tf_outputs() {
  info "Loading Terraform outputs..."
  cd "${TF_DIR}"

  export PROJECT_ID="$(terraform output -raw project_id)"
  export REGION="$(terraform output -raw region)"
  export CLUSTER_NAME="$(terraform output -raw cluster_name)"
  export AR_REPO="$(terraform output -raw artifact_registry_repo)"
  export MONDOO_CREDS_B64="$(terraform output -raw mondoo_credentials_b64)"
  export MONDOO_SPACE_MRN="$(terraform output -raw mondoo_space_mrn)"
  export NAME_PREFIX="$(terraform output -raw name_prefix)"
  export AUTOPILOT="$(terraform output -raw autopilot)"
  export GIT_SHA="$(git -C "${REPO_ROOT}" rev-parse --short HEAD)"
  export NAMESPACE="mondoo-operator"

  export ENABLE_TARGET_CLUSTER="$(terraform output -raw enable_target_cluster 2>/dev/null || echo "false")"
  if [[ "${ENABLE_TARGET_CLUSTER}" == "true" ]]; then
    export TARGET_CLUSTER_NAME="$(terraform output -raw target_cluster_name)"
    # Resolve to absolute path since we're currently in TF_DIR
    export TARGET_KUBECONFIG_PATH="$(cd "${TF_DIR}" && realpath "$(terraform output -raw target_kubeconfig_path)")"
  fi

  export ENABLE_MIRROR_TEST="$(terraform output -raw enable_mirror_test 2>/dev/null || echo "false")"
  if [[ "${ENABLE_MIRROR_TEST}" == "true" ]]; then
    export MIRROR_REGISTRY="$(terraform output -raw mirror_registry_repo)"
    export MIRROR_SA_KEY_B64="$(terraform output -raw mirror_sa_key_b64)"
  fi

  export ENABLE_PROXY_TEST="$(terraform output -raw enable_proxy_test 2>/dev/null || echo "false")"
  if [[ "${ENABLE_PROXY_TEST}" == "true" ]]; then
    export SQUID_PROXY_IP="$(terraform output -raw squid_proxy_ip)"
  fi

  cd ->/dev/null

  # Use gcloud to get cluster credentials (auto-refreshing auth)
  info "Fetching GKE credentials..."
  gcloud container clusters get-credentials "${CLUSTER_NAME}" \
    --region "${REGION}" --project "${PROJECT_ID}" --quiet

  info "Loaded outputs: cluster=${CLUSTER_NAME}, repo=${AR_REPO}"
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

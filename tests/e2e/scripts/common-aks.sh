#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# AKS-specific extensions for common.sh

# AKS-specific Terraform outputs and credential setup
_load_cloud_outputs() {
  cd "${TF_DIR}"

  export AZURE_SUBSCRIPTION_ID="$(terraform output -raw subscription_id)"
  export AZURE_RESOURCE_GROUP="$(terraform output -raw resource_group)"
  export ACR_LOGIN_SERVER="$(terraform output -raw acr_login_server)"
  export ACR_REPO="$(terraform output -raw acr_repo)"

  # Generic alias used by cloud-agnostic scripts
  export REGISTRY_REPO="${ACR_REPO}"

  # AKS has no autopilot concept
  export AUTOPILOT="false"

  if [[ "${ENABLE_WIF_TEST}" == "true" ]]; then
    export WIF_CLIENT_ID="$(terraform output -raw wif_client_id)"
    export WIF_TENANT_ID="$(terraform output -raw wif_tenant_id)"
  fi

  cd ->/dev/null

  # Fetch AKS credentials
  info "Fetching AKS credentials..."
  az aks get-credentials \
    --resource-group "${AZURE_RESOURCE_GROUP}" \
    --name "${CLUSTER_NAME}" \
    --subscription "${AZURE_SUBSCRIPTION_ID}" \
    --overwrite-existing
}

# Authenticate Docker to ACR and push image
build_and_push() {
  : "${REGISTRY_REPO:?REGISTRY_REPO must be set}"
  : "${GIT_SHA:?GIT_SHA must be set}"
  : "${ACR_LOGIN_SERVER:?ACR_LOGIN_SERVER must be set}"

  export OPERATOR_IMAGE="${REGISTRY_REPO}:${GIT_SHA}"

  info "Building operator image: ${OPERATOR_IMAGE}"
  cd "${REPO_ROOT}"
  make docker-build IMG="${OPERATOR_IMAGE}" TARGET_ARCH=amd64

  info "Configuring Docker for ACR..."
  az acr login --name "${ACR_LOGIN_SERVER%%.*}"

  info "Pushing image..."
  docker push "${OPERATOR_IMAGE}"
  info "Image pushed: ${OPERATOR_IMAGE}"

  export IMAGE_REPO="${REGISTRY_REPO}"
  export IMAGE_TAG="${GIT_SHA}"
}

# Refresh target cluster kubeconfig
refresh_target_credentials() {
  : "${TARGET_CLUSTER_NAME:?TARGET_CLUSTER_NAME must be set}"
  : "${TARGET_KUBECONFIG_PATH:?TARGET_KUBECONFIG_PATH must be set}"
  : "${AZURE_RESOURCE_GROUP:?AZURE_RESOURCE_GROUP must be set}"
  : "${AZURE_SUBSCRIPTION_ID:?AZURE_SUBSCRIPTION_ID must be set}"

  info "Refreshing target cluster credentials..."
  az aks get-credentials \
    --resource-group "${AZURE_RESOURCE_GROUP}" \
    --name "${TARGET_CLUSTER_NAME}" \
    --subscription "${AZURE_SUBSCRIPTION_ID}" \
    --file "${TARGET_KUBECONFIG_PATH}" \
    --overwrite-existing
}

# Setup WIF RBAC on target cluster
# AKS uses Azure RBAC configured in Terraform (role assignments), so no manual RBAC needed.
setup_wif_rbac() {
  info "AKS: RBAC is configured via Azure role assignments in Terraform (no manual step needed)."
}

# WIF annotation key and expected value for verification
WIF_ANNOTATION_KEY="azure.workload.identity/client-id"
wif_annotation_value() { echo "${WIF_CLIENT_ID}"; }

# Init container image pattern for WIF verification
WIF_INIT_IMAGE_PATTERN="azure-cli"

# WIF description for manual verification messages
WIF_AUTH_DESCRIPTION="AKS Workload Identity"

# Container registry WIF verification values
CR_WIF_ANNOTATION_KEY="azure.workload.identity/client-id"
cr_wif_annotation_value() { echo "${WIF_CLIENT_ID}"; }
CR_WIF_INIT_IMAGE_PATTERN="azure-cli"

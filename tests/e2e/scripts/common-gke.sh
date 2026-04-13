#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# GKE-specific extensions for common.sh

# GKE-specific Terraform outputs and credential setup
_load_cloud_outputs() {
  cd "${TF_DIR}"

  export PROJECT_ID="$(terraform output -raw project_id)"
  export AR_REPO="$(terraform output -raw artifact_registry_repo)"
  export AUTOPILOT="$(terraform output -raw autopilot)"

  # Generic alias used by cloud-agnostic scripts
  export REGISTRY_REPO="${AR_REPO}"

  export ENABLE_MIRROR_TEST="$(terraform output -raw enable_mirror_test 2>/dev/null || echo "false")"
  if [[ "${ENABLE_MIRROR_TEST}" == "true" ]]; then
    export MIRROR_REGISTRY="$(terraform output -raw mirror_registry_repo)"
    export MIRROR_SA_KEY_B64="$(terraform output -raw mirror_sa_key_b64)"
  fi

  export ENABLE_PROXY_TEST="$(terraform output -raw enable_proxy_test 2>/dev/null || echo "false")"
  if [[ "${ENABLE_PROXY_TEST}" == "true" ]]; then
    export SQUID_PROXY_IP="$(terraform output -raw squid_proxy_ip)"
  fi

  if [[ "${ENABLE_WIF_TEST}" == "true" ]]; then
    export WIF_GSA_EMAIL="$(terraform output -raw wif_gsa_email)"
  fi

  cd ->/dev/null

  # Fetch GKE credentials
  info "Fetching GKE credentials..."
  gcloud container clusters get-credentials "${CLUSTER_NAME}" \
    --region "${REGION}" --project "${PROJECT_ID}" --quiet
}

# Authenticate Docker to Artifact Registry and push image
build_and_push() {
  : "${REGISTRY_REPO:?REGISTRY_REPO must be set}"
  : "${GIT_SHA:?GIT_SHA must be set}"
  : "${REGION:?REGION must be set}"

  export OPERATOR_IMAGE="${REGISTRY_REPO}/mondoo-operator:${GIT_SHA}"

  info "Building operator image: ${OPERATOR_IMAGE}"
  cd "${REPO_ROOT}"
  make docker-build IMG="${OPERATOR_IMAGE}" TARGET_ARCH=amd64

  info "Configuring Docker for Artifact Registry..."
  gcloud auth configure-docker "${REGION}-docker.pkg.dev" --quiet

  info "Pushing image..."
  docker push "${OPERATOR_IMAGE}"
  info "Image pushed: ${OPERATOR_IMAGE}"

  export IMAGE_REPO="${REGISTRY_REPO}/mondoo-operator"
  export IMAGE_TAG="${GIT_SHA}"
}

# Refresh target cluster kubeconfig
refresh_target_credentials() {
  : "${TARGET_CLUSTER_NAME:?TARGET_CLUSTER_NAME must be set}"
  : "${TARGET_KUBECONFIG_PATH:?TARGET_KUBECONFIG_PATH must be set}"
  : "${REGION:?REGION must be set}"
  : "${PROJECT_ID:?PROJECT_ID must be set}"

  info "Refreshing target cluster credentials..."
  KUBECONFIG="${TARGET_KUBECONFIG_PATH}" \
    gcloud container clusters get-credentials "${TARGET_CLUSTER_NAME}" \
    --region "${REGION}" --project "${PROJECT_ID}" --quiet
}

# Setup WIF RBAC on target cluster (GKE: ClusterRoleBinding with GSA as User)
setup_wif_rbac() {
  : "${WIF_GSA_EMAIL:?WIF_GSA_EMAIL must be set}"
  : "${TARGET_KUBECONFIG_PATH:?TARGET_KUBECONFIG_PATH must be set}"

  info "Applying ClusterRoleBinding on target cluster for GSA ${WIF_GSA_EMAIL}..."
  kubectl --kubeconfig "${TARGET_KUBECONFIG_PATH}" apply -f - <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: mondoo-wif-scanner
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: view
subjects:
- apiGroup: rbac.authorization.k8s.io
  kind: User
  name: ${WIF_GSA_EMAIL}
EOF
}

# WIF annotation key and expected value for verification
WIF_ANNOTATION_KEY="iam.gke.io/gcp-service-account"
wif_annotation_value() { echo "${WIF_GSA_EMAIL}"; }

# Init container image pattern for WIF verification
WIF_INIT_IMAGE_PATTERN="google-cloud-cli"

# WIF description for manual verification messages
WIF_AUTH_DESCRIPTION="GKE Workload Identity"

# Container registry WIF verification values
CR_WIF_ANNOTATION_KEY="iam.gke.io/gcp-service-account"
cr_wif_annotation_value() { echo "${WIF_GSA_EMAIL}"; }
CR_WIF_INIT_IMAGE_PATTERN="google-cloud-cli"

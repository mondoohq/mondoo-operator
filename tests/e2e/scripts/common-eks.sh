#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# EKS-specific extensions for common.sh

# Helper: build aws CLI args with --profile if set
_aws() {
  if [[ -n "${AWS_PROFILE:-}" ]]; then
    aws --profile "${AWS_PROFILE}" "$@"
  else
    aws "$@"
  fi
}

# EKS-specific Terraform outputs and credential setup
_load_cloud_outputs() {
  cd "${TF_DIR}"

  export AWS_REGION="${REGION}"
  export ECR_REPO="$(terraform output -raw ecr_repo)"

  local profile
  profile="$(terraform output -raw profile 2>/dev/null || true)"
  if [[ -n "${profile}" ]]; then
    export AWS_PROFILE="${profile}"
    info "Using AWS profile: ${AWS_PROFILE}"
  fi

  # Generic alias used by cloud-agnostic scripts
  export REGISTRY_REPO="${ECR_REPO}"

  # EKS has no autopilot concept
  export AUTOPILOT="false"

  if [[ "${ENABLE_WIF_TEST}" == "true" ]]; then
    export SCANNER_ROLE_ARN="$(terraform output -raw scanner_role_arn)"
    export PRIVATE_TEST_ECR_REPO="$(terraform output -raw private_test_ecr_repo)"
  fi

  cd ->/dev/null

  # Use the terraform-generated kubeconfig for the scanner cluster
  export KUBECONFIG="${TF_DIR}/kubeconfig"
  info "Using kubeconfig: ${KUBECONFIG}"
}

# Authenticate Docker to ECR and push image
build_and_push() {
  : "${REGISTRY_REPO:?REGISTRY_REPO must be set}"
  : "${GIT_SHA:?GIT_SHA must be set}"
  : "${AWS_REGION:?AWS_REGION must be set}"

  export OPERATOR_IMAGE="${REGISTRY_REPO}:${GIT_SHA}"

  info "Building operator image: ${OPERATOR_IMAGE}"
  cd "${REPO_ROOT}"
  make docker-build IMG="${OPERATOR_IMAGE}" TARGET_ARCH=amd64

  info "Configuring Docker for ECR..."
  _aws ecr get-login-password --region "${AWS_REGION}" \
    | docker login --username AWS --password-stdin "$(echo "${REGISTRY_REPO}" | cut -d/ -f1)"

  info "Pushing image..."
  docker push "${OPERATOR_IMAGE}"
  info "Image pushed: ${OPERATOR_IMAGE}"

  export IMAGE_REPO="${REGISTRY_REPO}"
  export IMAGE_TAG="${GIT_SHA}"
}

# Refresh target cluster kubeconfig
# EKS: no-op — terraform generates the kubeconfig file directly.
refresh_target_credentials() {
  info "EKS: Using terraform-generated kubeconfig at ${TARGET_KUBECONFIG_PATH}"
}

# Setup WIF RBAC on target cluster
# EKS uses Access Entries configured in Terraform, so no manual RBAC needed.
setup_wif_rbac() {
  info "EKS: RBAC is configured via Access Entries in Terraform (no manual step needed)."
}

# WIF annotation key and expected value for verification
WIF_ANNOTATION_KEY="eks.amazonaws.com/role-arn"
wif_annotation_value() { echo "${SCANNER_ROLE_ARN}"; }

# Init container image pattern for WIF verification
WIF_INIT_IMAGE_PATTERN="aws-cli"

# WIF description for manual verification messages
WIF_AUTH_DESCRIPTION="EKS IRSA"

# Container registry WIF verification values
CR_WIF_ANNOTATION_KEY="eks.amazonaws.com/role-arn"
cr_wif_annotation_value() { echo "${SCANNER_ROLE_ARN}"; }
CR_WIF_INIT_IMAGE_PATTERN="aws-cli"

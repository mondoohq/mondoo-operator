#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Setup WIF: update CRDs on scanner cluster and apply RBAC on target cluster

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

: "${TARGET_CLUSTER_NAME:?TARGET_CLUSTER_NAME must be set}"
: "${TARGET_KUBECONFIG_PATH:?TARGET_KUBECONFIG_PATH must be set}"
: "${REGION:?REGION must be set}"
: "${PROJECT_ID:?PROJECT_ID must be set}"
: "${WIF_GSA_EMAIL:?WIF_GSA_EMAIL must be set}"

# Update CRDs on scanner cluster (WIF fields may not exist in released chart CRDs)
info "Updating CRDs on scanner cluster..."
kubectl apply --server-side --force-conflicts -f "${REPO_ROOT}/config/crd/bases/"

# Refresh target cluster credentials
info "Refreshing target cluster credentials..."
KUBECONFIG="${TARGET_KUBECONFIG_PATH}" \
  gcloud container clusters get-credentials "${TARGET_CLUSTER_NAME}" \
  --region "${REGION}" --project "${PROJECT_ID}" --quiet

# Apply ClusterRoleBinding on target cluster granting the GSA 'view' access
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

info "WIF setup complete."

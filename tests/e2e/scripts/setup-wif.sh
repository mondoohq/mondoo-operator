#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Setup WIF: update CRDs on scanner cluster and configure target cluster RBAC

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

# Update CRDs on scanner cluster (WIF fields may not exist in released chart CRDs)
info "Updating CRDs on scanner cluster..."
kubectl apply --server-side --force-conflicts -f "${REPO_ROOT}/config/crd/bases/"

# Cloud-specific RBAC setup (GKE: ClusterRoleBinding, EKS: no-op via Access Entries)
setup_wif_rbac

info "WIF setup complete."

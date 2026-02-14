#!/bin/bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1
#
# This script updates the CRDs in the Helm chart from the generated CRDs.
# CRDs are placed in charts/mondoo-operator/crds/ which Helm installs
# automatically before other chart resources.
#
# Usage: ./hack/update-helm-crds.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
CRD_BASES="${ROOT_DIR}/config/crd/bases"
CHART_CRDS="${ROOT_DIR}/charts/mondoo-operator/crds"

echo "Copying CRDs from ${CRD_BASES} to ${CHART_CRDS}..."
cp "${CRD_BASES}/k8s.mondoo.com_mondooauditconfigs.yaml" "${CHART_CRDS}/"
cp "${CRD_BASES}/k8s.mondoo.com_mondoooperatorconfigs.yaml" "${CHART_CRDS}/"

echo "CRDs updated successfully in ${CHART_CRDS}"
echo ""
echo "Please review the changes with: git diff charts/mondoo-operator/crds/"

#!/bin/bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1
#
# This script updates the CRDs in the Helm chart from the generated CRDs.
#
# CRDs are maintained in two locations:
# - crds/        Helm installs these automatically on first install (before templates)
# - files/crds/  Used by templates via .Files.Get to keep CRDs updated on upgrade
#                (Helm skips the crds/ directory on upgrade)
#
# Usage: ./hack/update-helm-crds.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
CRD_BASES="${ROOT_DIR}/config/crd/bases"
CHART_CRDS="${ROOT_DIR}/charts/mondoo-operator/crds"
CHART_FILES_CRDS="${ROOT_DIR}/charts/mondoo-operator/files/crds"

echo "Copying CRDs from ${CRD_BASES}..."
mkdir -p "${CHART_CRDS}" "${CHART_FILES_CRDS}"
for f in k8s.mondoo.com_mondooauditconfigs.yaml k8s.mondoo.com_mondoooperatorconfigs.yaml; do
  cp "${CRD_BASES}/${f}" "${CHART_CRDS}/"
  cp "${CRD_BASES}/${f}" "${CHART_FILES_CRDS}/"
done

echo "CRDs updated in crds/ and files/crds/"
echo ""
echo "Please review the changes with: git diff charts/mondoo-operator/crds/ charts/mondoo-operator/files/crds/"

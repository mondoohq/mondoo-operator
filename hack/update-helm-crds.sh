#!/bin/bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1
#
# This script updates the CRD templates in the Helm chart from the generated CRDs.
# It applies the necessary transformations to make them Helm-compatible:
# - Adds Helm labels template
# - Replaces webhook namespace with Helm template
#
# Usage: ./hack/update-helm-crds.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
CHART_DIR="${ROOT_DIR}/charts/mondoo-operator/templates"

# Check for required tools
if ! command -v yq &> /dev/null; then
    echo "Error: yq is required but not installed."
    echo "Install with: brew install yq (macOS) or go install github.com/mikefarah/yq/v4@latest"
    exit 1
fi

if ! command -v kustomize &> /dev/null && [ ! -f "${ROOT_DIR}/bin/kustomize" ]; then
    echo "Error: kustomize is required. Run 'make kustomize' first."
    exit 1
fi

KUSTOMIZE="${ROOT_DIR}/bin/kustomize"
if [ ! -f "$KUSTOMIZE" ]; then
    KUSTOMIZE="kustomize"
fi

echo "Building CRDs with kustomize..."
CRD_OUTPUT=$("$KUSTOMIZE" build "${ROOT_DIR}/config/crd")

# Process mondooauditconfigs CRD
echo "Processing mondooauditconfigs CRD..."
echo "$CRD_OUTPUT" | yq eval 'select(.metadata.name == "mondooauditconfigs.k8s.mondoo.com")' - | \
    yq eval 'del(.metadata.labels)' - | \
    yq eval '.spec.conversion.webhook.clientConfig.service.namespace = "HELM_NAMESPACE_PLACEHOLDER"' - | \
    sed 's/name: mondooauditconfigs.k8s.mondoo.com/name: mondooauditconfigs.k8s.mondoo.com\n  labels:\n  {{- include "mondoo-operator.labels" . | nindent 4 }}/' | \
    sed "s/HELM_NAMESPACE_PLACEHOLDER/'{{ .Release.Namespace }}'/" \
    > "${CHART_DIR}/mondooauditconfig-crd.yaml"

# Process mondoooperatorconfigs CRD
echo "Processing mondoooperatorconfigs CRD..."
echo "$CRD_OUTPUT" | yq eval 'select(.metadata.name == "mondoooperatorconfigs.k8s.mondoo.com")' - | \
    yq eval 'del(.metadata.labels)' - | \
    sed 's/name: mondoooperatorconfigs.k8s.mondoo.com/name: mondoooperatorconfigs.k8s.mondoo.com\n  labels:\n  {{- include "mondoo-operator.labels" . | nindent 4 }}/' \
    > "${CHART_DIR}/mondoooperatorconfig-crd.yaml"

echo "CRDs updated successfully in ${CHART_DIR}"
echo ""
echo "Please review the changes with: git diff charts/mondoo-operator/templates/*-crd.yaml"

#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Rebuild cnspec from local repo, push new image, and patch the
# MondooAuditConfig to use it. For quick iteration without re-running
# the full suite.
#
# Usage:
#   CNSPEC_REPO=~/repos/cnspec REGISTRY_REPO=<ar-repo> REGION=<region> ./redeploy-cnspec.sh
#
# To also bake in locally built providers from an mql checkout:
#   MQL_REPO=~/repos/mql CNSPEC_REPO=~/repos/cnspec REGISTRY_REPO=<ar-repo> REGION=<region> ./redeploy-cnspec.sh

set -euo pipefail

_info()  { echo "[INFO]  $(date '+%H:%M:%S') $*"; }

: "${CNSPEC_REPO:?CNSPEC_REPO must be set}"
: "${REGISTRY_REPO:?REGISTRY_REPO must be set}"
: "${REGION:?REGION must be set}"

NAMESPACE="${NAMESPACE:-mondoo-operator}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Build and push
source "${SCRIPT_DIR}/build-and-push-cnspec.sh"

# Patch MondooAuditConfig with new image
_info "Patching MondooAuditConfig scanner image to ${CNSPEC_IMAGE_NAME}:${CNSPEC_IMAGE_TAG}..."
kubectl patch mondooauditconfigs.k8s.mondoo.com mondoo-client -n "${NAMESPACE}" --type=merge \
  -p "{\"spec\":{\"scanner\":{\"image\":{\"name\":\"${CNSPEC_IMAGE_NAME}\",\"tag\":\"${CNSPEC_IMAGE_TAG}\"}}}}"

_info "Done. The operator will reconcile and update the CronJob."
_info "Watch with: kubectl get jobs -n ${NAMESPACE} -w"

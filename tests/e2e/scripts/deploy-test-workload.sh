#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Deploy a test nginx workload for the operator to scan

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

info "Deploying nginx test workload..."
kubectl apply -f "${SHARED_MANIFESTS_DIR}/nginx-workload.yaml"

wait_for_deployment "default" "nginx-test-workload"

info "Nginx test workload deployed."

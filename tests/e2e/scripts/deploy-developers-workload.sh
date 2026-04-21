#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Deploy a test workload into the 'developers' namespace on the scanner cluster.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

info "Creating developers namespace..."
kubectl create namespace developers --dry-run=client -o yaml | kubectl apply -f -

info "Deploying nginx workload to developers namespace..."
kubectl apply -f "${SHARED_MANIFESTS_DIR}/nginx-developers-workload.yaml"

wait_for_deployment developers nginx-developers 120s
info "Developers workload deployed."

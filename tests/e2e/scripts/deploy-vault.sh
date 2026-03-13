#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Deploy HashiCorp Vault in dev mode and configure:
#   1. Kubernetes auth method (so the operator pod can authenticate)
#   2. Kubernetes secrets engine (to generate target cluster credentials)
#
# Exports:
#   VAULT_ADDR_INTERNAL  - in-cluster Vault URL for MondooAuditConfig
#   VAULT_TARGET_SERVER  - target cluster API server URL

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

: "${TARGET_KUBECONFIG_PATH:?TARGET_KUBECONFIG_PATH must be set}"
: "${NAMESPACE:?NAMESPACE must be set}"

VAULT_NAMESPACE="vault"
VAULT_DEV_ROOT_TOKEN="e2e-root-token"
VAULT_ADDR_INTERNAL="http://vault.${VAULT_NAMESPACE}.svc:8200"

################################################################################
# Step 1: Install Vault via Helm (dev mode)
################################################################################

info "Adding HashiCorp Helm repo..."
helm repo add hashicorp https://helm.releases.hashicorp.com 2>/dev/null || true
helm repo update hashicorp

info "Installing Vault in dev mode (namespace: ${VAULT_NAMESPACE})..."
helm upgrade --install vault hashicorp/vault \
  --namespace "${VAULT_NAMESPACE}" --create-namespace \
  --set "server.dev.enabled=true" \
  --set "server.dev.devRootToken=${VAULT_DEV_ROOT_TOKEN}" \
  --set "server.image.tag=1.19" \
  --set "injector.enabled=false" \
  --set "csi.enabled=false" \
  --wait --timeout 5m

info "Waiting for Vault pod to be ready..."
kubectl wait --for=condition=Ready pod/vault-0 \
  -n "${VAULT_NAMESPACE}" --timeout=300s

# Helper to exec vault commands inside the pod
vexec() {
  kubectl exec vault-0 -n "${VAULT_NAMESPACE}" -- \
    env VAULT_ADDR=http://127.0.0.1:8200 VAULT_TOKEN="${VAULT_DEV_ROOT_TOKEN}" \
    vault "$@"
}

################################################################################
# Step 2: Configure Kubernetes auth method (scanner cluster)
################################################################################

info "Enabling Kubernetes auth method in Vault..."
vexec auth enable kubernetes 2>/dev/null || info "Kubernetes auth already enabled"

# Configure Vault to validate tokens against the scanner cluster's API server.
# Since Vault runs inside the cluster, it can use the in-cluster CA and host.
vexec write auth/kubernetes/config \
  kubernetes_host="https://kubernetes.default.svc"

# Create a policy that allows generating credentials via the secrets engine.
# Write the policy file inside the pod first, then reference it (heredoc stdin
# doesn't survive kubectl exec reliably).
kubectl exec vault-0 -n "${VAULT_NAMESPACE}" -- \
  sh -c 'cat > /tmp/mondoo-policy.hcl <<EOF
path "kubernetes/creds/*" {
  capabilities = ["update"]
}
EOF'
vexec policy write mondoo-vault-creds /tmp/mondoo-policy.hcl

# Create an auth role for the operator's service account.
# The operator controller-manager pod authenticates with this role.
vexec write auth/kubernetes/role/mondoo-operator \
  bound_service_account_names=mondoo-operator-controller-manager \
  bound_service_account_namespaces="${NAMESPACE}" \
  policies=mondoo-vault-creds \
  ttl=1h

info "Vault Kubernetes auth configured."

################################################################################
# Step 3: Set up target cluster service account for Vault secrets engine
################################################################################

info "Creating Vault service account in target cluster..."

kubectl --kubeconfig "${TARGET_KUBECONFIG_PATH}" create namespace vault-secrets-engine 2>/dev/null || true
kubectl --kubeconfig "${TARGET_KUBECONFIG_PATH}" apply -f - <<'EOF'
apiVersion: v1
kind: ServiceAccount
metadata:
  name: vault-secrets-engine
  namespace: vault-secrets-engine
---
apiVersion: v1
kind: Secret
metadata:
  name: vault-secrets-engine-token
  namespace: vault-secrets-engine
  annotations:
    kubernetes.io/service-account.name: vault-secrets-engine
type: kubernetes.io/service-account-token
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: vault-secrets-engine-admin
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-admin
subjects:
  - kind: ServiceAccount
    name: vault-secrets-engine
    namespace: vault-secrets-engine
EOF

# Wait for the token Secret to be populated by the token controller
info "Waiting for target cluster service account token..."
for i in $(seq 1 30); do
  TARGET_SA_TOKEN="$(kubectl --kubeconfig "${TARGET_KUBECONFIG_PATH}" \
    get secret vault-secrets-engine-token -n vault-secrets-engine \
    -o jsonpath='{.data.token}' 2>/dev/null | base64 -d)" || true
  if [[ -n "${TARGET_SA_TOKEN}" ]]; then
    break
  fi
  sleep 2
done

if [[ -z "${TARGET_SA_TOKEN:-}" ]]; then
  die "Timed out waiting for vault-secrets-engine-token in target cluster"
fi

# Extract target cluster details from kubeconfig
VAULT_TARGET_SERVER="$(kubectl --kubeconfig "${TARGET_KUBECONFIG_PATH}" \
  config view --raw -o jsonpath='{.clusters[0].cluster.server}')"

TARGET_CA_DATA="$(kubectl --kubeconfig "${TARGET_KUBECONFIG_PATH}" \
  config view --raw -o jsonpath='{.clusters[0].cluster.certificate-authority-data}')"

info "Target cluster: ${VAULT_TARGET_SERVER}"

################################################################################
# Step 4: Configure Kubernetes secrets engine (target cluster)
################################################################################

info "Enabling Kubernetes secrets engine in Vault..."
vexec secrets enable kubernetes 2>/dev/null || info "Kubernetes secrets engine already enabled"

# Write the target cluster CA to a temp file inside the Vault pod
echo "${TARGET_CA_DATA}" | base64 -d > /tmp/vault-target-ca.crt
kubectl cp /tmp/vault-target-ca.crt "${VAULT_NAMESPACE}/vault-0:/tmp/target-ca.crt"
rm -f /tmp/vault-target-ca.crt

# Write the SA JWT to a temp file inside the Vault pod to avoid leaking it in process tables
echo "${TARGET_SA_TOKEN}" > /tmp/vault-sa-jwt.token
kubectl cp /tmp/vault-sa-jwt.token "${VAULT_NAMESPACE}/vault-0:/tmp/sa-jwt.token"
rm -f /tmp/vault-sa-jwt.token

# Configure the secrets engine with the target cluster connection
vexec write kubernetes/config \
  kubernetes_host="${VAULT_TARGET_SERVER}" \
  kubernetes_ca_cert=@/tmp/target-ca.crt \
  service_account_jwt=@/tmp/sa-jwt.token

# Create a role that generates short-lived tokens with read access
# Using generated_role_rules so Vault creates temporary ClusterRoleBindings
vexec write kubernetes/roles/target-cluster-scanner \
  allowed_kubernetes_namespaces="*" \
  token_default_ttl="1h" \
  token_max_ttl="2h" \
  generated_role_rules='{"rules":[{"apiGroups":[""],"resources":["pods","services","namespaces","nodes","configmaps","serviceaccounts","replicationcontrollers"],"verbs":["get","list","watch"]},{"apiGroups":["apps"],"resources":["deployments","daemonsets","replicasets","statefulsets"],"verbs":["get","list","watch"]},{"apiGroups":["batch"],"resources":["jobs","cronjobs"],"verbs":["get","list","watch"]},{"apiGroups":["networking.k8s.io"],"resources":["ingresses","networkpolicies"],"verbs":["get","list","watch"]},{"apiGroups":["rbac.authorization.k8s.io"],"resources":["roles","rolebindings","clusterroles","clusterrolebindings"],"verbs":["get","list","watch"]},{"apiGroups":["policy"],"resources":["podsecuritypolicies"],"verbs":["get","list","watch"]}]}'

info "Vault Kubernetes secrets engine configured."

################################################################################
# Step 5: Create target cluster CA cert Secret in operator namespace
################################################################################

info "Creating target cluster CA cert Secret..."
echo "${TARGET_CA_DATA}" | base64 -d > /tmp/vault-target-ca.crt
kubectl create secret generic vault-target-ca-cert \
  --namespace "${NAMESPACE}" \
  --from-file=ca.crt=/tmp/vault-target-ca.crt \
  --dry-run=client -o yaml | kubectl apply -f -
rm -f /tmp/vault-target-ca.crt

################################################################################
# Export variables for MondooAuditConfig template
################################################################################

export VAULT_ADDR_INTERNAL
export VAULT_TARGET_SERVER

info "Vault deployment and configuration complete."
info "  Vault address (in-cluster): ${VAULT_ADDR_INTERNAL}"
info "  Target server: ${VAULT_TARGET_SERVER}"
info "  Auth role: mondoo-operator"
info "  Creds role: target-cluster-scanner"

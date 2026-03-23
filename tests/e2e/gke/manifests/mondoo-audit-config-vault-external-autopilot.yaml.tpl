# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

apiVersion: k8s.mondoo.com/v1alpha2
kind: MondooAuditConfig
metadata:
  name: mondoo-client
  namespace: ${NAMESPACE}
spec:
  mondooCredsSecretRef:
    name: mondoo-client
  filtering:
    namespaces:
      exclude:
        - kube-system
        - gke-managed-system
        - gke-managed-cim
  kubernetesResources:
    enable: true
    schedule: "*/5 * * * *"
    externalClusters:
      - name: target-cluster
        vaultAuth:
          server: "${VAULT_TARGET_SERVER}"
          vaultAddr: "${VAULT_ADDR_INTERNAL}"
          authRole: "mondoo-operator"
          credsRole: "target-cluster-scanner"
          ttl: "1h"
          targetCACertSecretRef:
            name: vault-target-ca-cert
  containers:
    enable: true
    schedule: "*/5 * * * *"
  nodes:
    # Disabled: GKE Autopilot does not allow hostPath volumes on /
    enable: false

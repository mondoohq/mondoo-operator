# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# MondooAuditConfig for scanning the LOCAL (scanner) cluster.
# Routes assets to the scanner space using spaceId.
apiVersion: k8s.mondoo.com/v1alpha2
kind: MondooAuditConfig
metadata:
  name: mondoo-scanner
  namespace: ${NAMESPACE}
spec:
  mondooCredsSecretRef:
    name: mondoo-client
  spaceId: "${SCANNER_SPACE_ID}"
  filtering:
    namespaces:
      exclude:
        - kube-system
        - gke-managed-system
        - gke-managed-cim
  kubernetesResources:
    enable: true
    schedule: "*/5 * * * *"
  containers:
    enable: true
    schedule: "*/5 * * * *"
  nodes:
    # Disabled: GKE Autopilot does not allow hostPath volumes on /
    enable: false
---
# MondooAuditConfig for scanning the REMOTE (target) cluster.
# Routes assets to the target space using spaceId.
apiVersion: k8s.mondoo.com/v1alpha2
kind: MondooAuditConfig
metadata:
  name: mondoo-target
  namespace: ${NAMESPACE}
spec:
  mondooCredsSecretRef:
    name: mondoo-client
  spaceId: "${TARGET_SPACE_ID}"
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
        kubeconfigSecretRef:
          name: target-kubeconfig
  containers:
    enable: false
  nodes:
    enable: false

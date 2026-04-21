# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# MondooAuditConfig for scanning the LOCAL cluster (Autopilot — no nodes).
# No spaceId — server-side asset routing rules determine destination spaces.
apiVersion: k8s.mondoo.com/v1alpha2
kind: MondooAuditConfig
metadata:
  name: mondoo-scanner
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
  containers:
    enable: true
    schedule: "*/5 * * * *"
  nodes:
    enable: false
---
# MondooAuditConfig for scanning the REMOTE (target) cluster.
# No spaceId — server-side asset routing rules determine destination spaces.
# enable: false prevents local cluster scanning; externalClusters reconciles independently.
apiVersion: k8s.mondoo.com/v1alpha2
kind: MondooAuditConfig
metadata:
  name: mondoo-target
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
    enable: false
    schedule: "*/5 * * * *"
    externalClusters:
      - name: target-cluster
        kubeconfigSecretRef:
          name: target-kubeconfig
  containers:
    enable: false
  nodes:
    enable: false

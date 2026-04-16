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
        containerImageScanning: true
        workloadIdentity:
          provider: gke
          gke:
            projectId: ${PROJECT_ID}
            clusterName: ${TARGET_CLUSTER_NAME}
            clusterLocation: ${REGION}
            googleServiceAccount: ${WIF_GSA_EMAIL}
  containers:
    enable: true
    schedule: "*/5 * * * *"
    workloadIdentity:
      provider: gke
      gke:
        projectId: ${PROJECT_ID}
        clusterName: ${CLUSTER_NAME}
        clusterLocation: ${REGION}
        googleServiceAccount: ${WIF_GSA_EMAIL}
  nodes:
    # Disabled: GKE Autopilot does not allow hostPath volumes on /
    enable: false

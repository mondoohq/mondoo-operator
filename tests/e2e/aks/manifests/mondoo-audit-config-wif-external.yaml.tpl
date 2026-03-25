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
  kubernetesResources:
    enable: true
    schedule: "*/5 * * * *"
    externalClusters:
      - name: target-cluster
        containerImageScanning: true
        workloadIdentity:
          provider: aks
          aks:
            subscriptionId: ${AZURE_SUBSCRIPTION_ID}
            resourceGroup: ${AZURE_RESOURCE_GROUP}
            clusterName: ${TARGET_CLUSTER_NAME}
            clientId: ${WIF_CLIENT_ID}
            tenantId: ${WIF_TENANT_ID}
  containers:
    enable: true
    schedule: "*/5 * * * *"
  nodes:
    enable: true
    style: cronjob
    schedule: "*/5 * * * *"

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
          provider: eks
          eks:
            region: ${AWS_REGION}
            clusterName: ${TARGET_CLUSTER_NAME}
            roleArn: ${SCANNER_ROLE_ARN}
  containers:
    enable: true
    schedule: "*/5 * * * *"
    workloadIdentity:
      provider: eks
      eks:
        region: ${AWS_REGION}
        clusterName: ${CLUSTER_NAME}
        roleArn: ${SCANNER_ROLE_ARN}
  nodes:
    enable: true
    style: cronjob
    schedule: "*/5 * * * *"

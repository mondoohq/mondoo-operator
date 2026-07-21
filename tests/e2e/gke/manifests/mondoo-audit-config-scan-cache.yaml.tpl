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
  scanner:
    image:
      name: "${CNSPEC_IMAGE_NAME}"
      tag: "${CNSPEC_IMAGE_TAG}"
    privateRegistriesPullSecretRef:
      name: "${PULL_SECRET_NAME}"
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
    scanCache:
      enable: true
      cacheTTL: 5m
  nodes:
    enable: true
    schedule: "*/5 * * * *"

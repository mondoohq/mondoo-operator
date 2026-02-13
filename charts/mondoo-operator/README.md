# Mondoo Operator Helm Chart

The Mondoo Operator provides a Kubernetes-native way to deploy and manage [Mondoo](https://mondoo.com) security scanning in your clusters.

## Prerequisites

- Kubernetes 1.26+
- Helm 3.x

## Installation

### Add the Helm repository

```bash
helm repo add mondoo https://mondoohq.github.io/mondoo-operator
helm repo update
```

### Install the chart

```bash
helm install mondoo-operator mondoo/mondoo-operator --namespace mondoo-operator --create-namespace
```

### Uninstall the chart

```bash
helm uninstall mondoo-operator --namespace mondoo-operator
```

## Parameters

### Controller Manager Configuration

| Name                                                 | Description                                                     | Value                                                                                              |
| ---------------------------------------------------- | --------------------------------------------------------------- | -------------------------------------------------------------------------------------------------- |
| `controllerManager.manager.args`                     | Command-line arguments passed to the operator manager container | `["operator","--health-probe-bind-address=:8081","--metrics-bind-address=:8080","--leader-elect"]` |
| `controllerManager.manager.containerSecurityContext` | Security context for the manager container                      | `{}`                                                                                               |
| `controllerManager.manager.image.repository`         | Container image repository for the operator                     | `ghcr.io/mondoohq/mondoo-operator`                                                                 |
| `controllerManager.manager.image.tag`                | Container image tag for the operator                            | `v12.0.1`                                                                                          |
| `controllerManager.manager.imagePullPolicy`          | Image pull policy for the operator container                    | `IfNotPresent`                                                                                     |
| `controllerManager.manager.resources`                | Resource requests and limits for the manager container          | `{}`                                                                                               |
| `controllerManager.podSecurityContext`               | Pod-level security context for the controller manager           | `{}`                                                                                               |
| `controllerManager.replicas`                         | Number of controller manager replicas                           | `1`                                                                                                |
| `controllerManager.serviceAccount.annotations`       | Annotations to add to the controller manager service account    | `{}`                                                                                               |

### Kubernetes Resources Scanning Configuration

| Name                                              | Description                                                             | Value |
| ------------------------------------------------- | ----------------------------------------------------------------------- | ----- |
| `k8SResourcesScanning.serviceAccount.annotations` | Annotations to add to the Kubernetes resources scanning service account | `{}`  |

### General Configuration

| Name                      | Description                            | Value           |
| ------------------------- | -------------------------------------- | --------------- |
| `kubernetesClusterDomain` | Kubernetes cluster domain used for DNS | `cluster.local` |

### Manager Config

| Name                                        | Description                                            | Value                                                                                                                                                                                                                                                                                                       |
| ------------------------------------------- | ------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `managerConfig.controllerManagerConfigYaml` | Embedded YAML configuration for the controller manager | `# Copyright (c) Mondoo, Inc.
# SPDX-License-Identifier: BUSL-1.1
apiVersion: controller-runtime.sigs.k8s.io/v1alpha1
kind: ControllerManagerConfig
health:
  healthProbeBindAddress: :8081
metrics:
  bindAddress: 127.0.0.1:8080
leaderElection:
  leaderElect: true
  resourceName: 60679458.mondoo.com` |

### Metrics Service Configuration

| Name                   | Description                                     | Value       |
| ---------------------- | ----------------------------------------------- | ----------- |
| `metricsService.ports` | Ports configuration for the metrics service     | `[]`        |
| `metricsService.type`  | Kubernetes service type for the metrics service | `ClusterIP` |

### Pre-delete Cleanup Hook Configuration

| Name              | Description                                                       | Value  |
| ----------------- | ----------------------------------------------------------------- | ------ |
| `cleanup.enabled` | Enable or disable the pre-delete cleanup hook                     | `true` |
| `cleanup.timeout` | Timeout for waiting for MondooAuditConfig resources to be deleted | `2m`   |


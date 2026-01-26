# Architecture

![Architecture](img/architecture.svg)

## Overview

The Mondoo Operator uses a simple, CronJob-based architecture to scan Kubernetes clusters. Each scan type runs as an independent CronJob that executes `cnspec` directly against the target.

## Components

### Mondoo Operator Controller

The main controller watches `MondooAuditConfig` resources and reconciles the desired state by creating and managing:

- **CronJobs** for scheduled scanning
- **ConfigMaps** for scan configuration (inventory files)
- **ServiceAccounts** for RBAC permissions

### Scanning Components

#### Kubernetes Resources Scanning

Scans Kubernetes API resources (Pods, Deployments, Services, Namespaces, etc.) using `cnspec scan k8s`.

- **Schedule**: Configurable (default: hourly)
- **Resources scanned**: clusters, pods, jobs, cronjobs, statefulsets, deployments, replicasets, daemonsets, ingresses, namespaces, services
- **Configuration**: Via inventory ConfigMap

#### Node Scanning

Scans Kubernetes nodes for security compliance. Supports three deployment styles:

- **CronJob** (default): Runs on schedule across all nodes
- **DaemonSet**: Runs continuously on each node
- **Deployment**: Runs as a deployment with configurable interval

#### Container Image Scanning

Scans container images running in the cluster for vulnerabilities.

- **Schedule**: Configurable (default: daily)
- **Features**:
  - Private registry support

## Data Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Mondoo Operator                                    │
│                                                                              │
│  ┌──────────────────┐     ┌──────────────────┐     ┌──────────────────┐    │
│  │ MondooAuditConfig │────▶│    Controller    │────▶│    CronJobs      │    │
│  │     (CRD)        │     │   (Reconciler)   │     │                  │    │
│  └──────────────────┘     └──────────────────┘     └────────┬─────────┘    │
│                                                              │              │
└──────────────────────────────────────────────────────────────┼──────────────┘
                                                               │
                    ┌──────────────────────────────────────────┼───────────────┐
                    │                                          ▼               │
                    │  ┌─────────────┐   ┌─────────────┐   ┌─────────────┐    │
                    │  │  K8s Scan   │   │  Node Scan  │   │ Image Scan  │    │
                    │  │   CronJob   │   │  CronJob/DS │   │   CronJob   │    │
                    │  └──────┬──────┘   └──────┬──────┘   └──────┬──────┘    │
                    │         │                 │                 │           │
                    │         ▼                 ▼                 ▼           │
                    │  ┌─────────────────────────────────────────────────┐    │
                    │  │                  cnspec scan                     │    │
                    │  │  (k8s | local | container-image)                │    │
                    │  └─────────────────────────────────────────────────┘    │
                    │                          │                              │
                    │                          ▼                              │
                    │  ┌─────────────────────────────────────────────────┐    │
                    │  │              Mondoo Platform                     │    │
                    │  │         (Results & Reporting)                   │    │
                    │  └─────────────────────────────────────────────────┘    │
                    │                     Kubernetes Cluster                   │
                    └──────────────────────────────────────────────────────────┘
```

## Configuration Resources

### MondooAuditConfig

The primary configuration resource. Defines what to scan and how:

```yaml
apiVersion: k8s.mondoo.com/v1alpha2
kind: MondooAuditConfig
metadata:
  name: mondoo-client
  namespace: mondoo-operator
spec:
  mondooCredsSecretRef:
    name: mondoo-client

  # Scan Kubernetes resources
  kubernetesResources:
    enable: true
    schedule: "0 * * * *"  # Hourly

  # Scan nodes
  nodes:
    enable: true
    style: cronjob  # or "daemonset", "deployment"

  # Scan container images
  containers:
    enable: true
    schedule: "0 0 * * *"  # Daily

  # Namespace filtering
  filtering:
    namespaces:
      exclude:
        - kube-system
```

### MondooOperatorConfig

Cluster-wide operator configuration:

```yaml
apiVersion: k8s.mondoo.com/v1alpha2
kind: MondooOperatorConfig
metadata:
  name: mondoo-operator-config
spec:
  skipContainerResolution: false
  metrics:
    enable: true
```

# MondooOperatorConfig Guide

This guide explains how to configure the Mondoo Operator for enterprise environments including corporate proxies, air-gapped clusters, and private container registries.

- [MondooOperatorConfig Guide](#mondoooperatorconfig-guide)
  - [Overview](#overview)
  - [Quick Start](#quick-start)
    - [Using Helm](#using-helm)
    - [Using kubectl](#using-kubectl)
  - [Configuration Reference](#configuration-reference)
  - [Use Cases](#use-cases)
    - [Corporate Proxy Configuration](#corporate-proxy-configuration)
    - [Air-Gapped / Disconnected Clusters](#air-gapped--disconnected-clusters)
    - [Private Registry Authentication](#private-registry-authentication)
    - [GKE Autopilot / Restricted Environments](#gke-autopilot--restricted-environments)
    - [Metrics and Monitoring](#metrics-and-monitoring)
  - [How Configuration Flows to Components](#how-configuration-flows-to-components)
  - [Troubleshooting](#troubleshooting)
  - [Helm Configuration Reference](#helm-configuration-reference)

## Overview

`MondooOperatorConfig` is a **cluster-scoped** custom resource that configures operator-wide settings. Unlike `MondooAuditConfig` (which defines what to scan), `MondooOperatorConfig` defines how the operator itself behaves across all scanning workloads.

Key characteristics:

- **Cluster-scoped**: One instance applies to the entire cluster
- **Fixed name**: Must be named `mondoo-operator-config`
- **Single instance**: Only one `MondooOperatorConfig` is allowed per cluster
- **Applies globally**: Settings affect all `MondooAuditConfig` resources

**Relationship to MondooAuditConfig:**

| Resource | Scope | Purpose |
|----------|-------|---------|
| `MondooOperatorConfig` | Cluster | Operator-wide settings (proxies, registries, metrics) |
| `MondooAuditConfig` | Namespace | What to scan (nodes, K8s resources, containers) |

## Quick Start

### Using Helm

The simplest way to configure `MondooOperatorConfig` is through Helm values:

```bash
helm install mondoo-operator mondoo/mondoo-operator \
  --namespace mondoo-operator \
  --create-namespace \
  --set operator.httpProxy="http://proxy.example.com:3128" \
  --set operator.httpsProxy="http://proxy.example.com:3128"
```

### Using kubectl

Apply a `MondooOperatorConfig` resource directly:

```yaml
apiVersion: k8s.mondoo.com/v1alpha2
kind: MondooOperatorConfig
metadata:
  name: mondoo-operator-config
spec:
  metrics:
    enable: true
```

```bash
kubectl apply -f mondoo-operator-config.yaml
```

## Configuration Reference

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `metrics.enable` | bool | `false` | Enable Prometheus metrics and ServiceMonitor creation |
| `metrics.resourceLabels` | map[string]string | `{}` | Extra labels to add to metrics-related resources (e.g., ServiceMonitor) |
| `httpProxy` | string | `""` | HTTP proxy URL for outbound connections |
| `httpsProxy` | string | `""` | HTTPS proxy URL for outbound connections |
| `noProxy` | string | `""` | Comma-separated list of hosts/CIDRs that bypass the proxy |
| `containerProxy` | string | `""` | Proxy for container image operations |
| `imagePullSecrets` | []LocalObjectReference | `[]` | Secrets for pulling Mondoo container images |
| `imageRegistry` | string | `""` | Custom registry prefix for all Mondoo images (simple mirror) |
| `registryMirrors` | map[string]string | `{}` | Map of public registries to private mirrors |
| `skipContainerResolution` | bool | `false` | Skip resolving container image digests from upstream |
| `skipProxyForCnspec` | bool | `false` | Disable proxy settings for cnspec-based components |

## Use Cases

### Corporate Proxy Configuration

Configure the operator to route traffic through your corporate proxy:

```yaml
apiVersion: k8s.mondoo.com/v1alpha2
kind: MondooOperatorConfig
metadata:
  name: mondoo-operator-config
spec:
  httpProxy: "http://proxy.example.com:3128"
  httpsProxy: "http://proxy.example.com:3128"
  noProxy: "10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,.cluster.local,.svc,localhost,127.0.0.1"
```

**noProxy format:**

The `noProxy` field accepts a comma-separated list following the same conventions as the standard `NO_PROXY` environment variable:

| Pattern | Description | Example |
|---------|-------------|---------|
| IP address | Exact IP match | `192.168.1.1` |
| CIDR notation | IP range | `10.0.0.0/8` |
| Domain | Exact domain match | `internal.example.com` |
| Domain suffix | Matches domain and subdomains (leading `.`) | `.example.com` |
| Wildcard | Matches all hosts (use carefully) | `*` |

**Recommended noProxy entries for Kubernetes:**

```
10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,.cluster.local,.svc,localhost,127.0.0.1
```

This bypasses proxy for:
- Private IP ranges (pod and service CIDRs)
- Kubernetes DNS domains
- Localhost connections

### Air-Gapped / Disconnected Clusters

For clusters without internet access, configure the operator to use your internal registry:

**Option 1: Simple registry mirror (all images from one mirror)**

Use `imageRegistry` when all Mondoo images are mirrored to the same registry:

```yaml
apiVersion: k8s.mondoo.com/v1alpha2
kind: MondooOperatorConfig
metadata:
  name: mondoo-operator-config
spec:
  # Rewrites: ghcr.io/mondoohq/cnspec:latest -> registry.example.com/ghcr.io.docker/mondoohq/cnspec:latest
  imageRegistry: "registry.example.com/ghcr.io.docker"
  # Skip attempts to resolve latest image digests from upstream
  skipContainerResolution: true
  # Image pull credentials
  imagePullSecrets:
    - name: registry-credentials
```

**Option 2: Multiple registry mirrors (different mirrors per source)**

Use `registryMirrors` when different source registries map to different mirrors:

```yaml
apiVersion: k8s.mondoo.com/v1alpha2
kind: MondooOperatorConfig
metadata:
  name: mondoo-operator-config
spec:
  registryMirrors:
    ghcr.io: "artifactory.example.com/ghcr-remote"
    docker.io: "artifactory.example.com/docker-hub-remote"
    quay.io: "artifactory.example.com/quay-remote"
  skipContainerResolution: true
  imagePullSecrets:
    - name: artifactory-credentials
```

> **Note:** If both `imageRegistry` and `registryMirrors` are set, `registryMirrors` takes precedence.

**Step-by-step air-gapped setup:**

1. **Mirror the required images** to your internal registry:
   - `ghcr.io/mondoohq/mondoo-operator:<version>`
   - `ghcr.io/mondoohq/cnspec:<version>`

2. **Create the image pull secret:**
   ```bash
   kubectl create secret docker-registry registry-credentials \
     --namespace mondoo-operator \
     --docker-server=registry.example.com \
     --docker-username=user \
     --docker-password=password
   ```

3. **Apply the MondooOperatorConfig** with your registry settings

4. **Deploy MondooAuditConfig** as normal - the operator applies registry settings automatically

### Private Registry Authentication

Configure credentials for pulling Mondoo images from authenticated registries:

```yaml
apiVersion: k8s.mondoo.com/v1alpha2
kind: MondooOperatorConfig
metadata:
  name: mondoo-operator-config
spec:
  imagePullSecrets:
    - name: ghcr-credentials
    - name: docker-hub-credentials
```

**Creating the required secrets:**

```bash
# For GitHub Container Registry
kubectl create secret docker-registry ghcr-credentials \
  --namespace mondoo-operator \
  --docker-server=ghcr.io \
  --docker-username=USERNAME \
  --docker-password=GITHUB_TOKEN

# For Docker Hub (rate limiting)
kubectl create secret docker-registry docker-hub-credentials \
  --namespace mondoo-operator \
  --docker-server=docker.io \
  --docker-username=USERNAME \
  --docker-password=PASSWORD
```

**Which components use imagePullSecrets:**

The `imagePullSecrets` from `MondooOperatorConfig` are applied to all Mondoo-managed workloads:
- Node scanning DaemonSets
- Kubernetes resource scanning CronJobs
- Container image scanning CronJobs

> **Note:** These are different from `scanner.privateRegistriesPullSecretRef` in `MondooAuditConfig`, which provides credentials for scanning your application's container images (not for pulling Mondoo's own images).

### GKE Autopilot / Restricted Environments

Some managed Kubernetes environments restrict certain operations. Use these settings to work around limitations:

```yaml
apiVersion: k8s.mondoo.com/v1alpha2
kind: MondooOperatorConfig
metadata:
  name: mondoo-operator-config
spec:
  # Skip image digest resolution (requires outbound HTTPS to ghcr.io)
  skipContainerResolution: true
  # Don't set proxy env vars for cnspec (useful when API is internal)
  skipProxyForCnspec: true
```

**When to use `skipContainerResolution`:**

- Air-gapped clusters without access to ghcr.io
- GKE Autopilot where network policies block registry access
- Environments where you want to pin exact image versions

**When to use `skipProxyForCnspec`:**

- Your Mondoo API endpoint is internal (no proxy needed)
- Proxy causes issues with certificate validation
- cnspec components need direct access but other components need proxy

### Metrics and Monitoring

Enable Prometheus metrics collection:

```yaml
apiVersion: k8s.mondoo.com/v1alpha2
kind: MondooOperatorConfig
metadata:
  name: mondoo-operator-config
spec:
  metrics:
    enable: true
    resourceLabels:
      prometheus: main
      team: security
```

**What happens when metrics are enabled:**

1. The operator creates a `ServiceMonitor` resource for Prometheus Operator
2. Metrics are exposed on the operator's metrics endpoint (port 8080)
3. The `resourceLabels` are added to the ServiceMonitor for label-based selection

**Prometheus Operator integration:**

If you use Prometheus Operator, ensure your Prometheus instance is configured to select the ServiceMonitor. The `resourceLabels` can be used to match your Prometheus's `serviceMonitorSelector`:

```yaml
# Example Prometheus configuration
apiVersion: monitoring.coreos.com/v1
kind: Prometheus
metadata:
  name: main
spec:
  serviceMonitorSelector:
    matchLabels:
      prometheus: main  # Matches the resourceLabel above
```

**Available metrics:**

The operator exports standard controller-runtime metrics including:
- Reconciliation counts and durations
- Work queue depth and latency
- Controller error counts

## How Configuration Flows to Components

The following table shows which `MondooOperatorConfig` settings affect which components:

| Setting | Operator | Node Scanning | K8s Resource Scanning | Container Scanning |
|---------|:--------:|:-------------:|:---------------------:|:------------------:|
| `httpProxy` | | ✓ | ✓ | ✓* |
| `httpsProxy` | | ✓ | ✓ | ✓* |
| `noProxy` | | ✓ | ✓ | ✓* |
| `containerProxy` | | | | ✓ |
| `imagePullSecrets` | | ✓ | ✓ | ✓ |
| `imageRegistry` | ✓ | ✓ | ✓ | ✓ |
| `registryMirrors` | ✓ | ✓ | ✓ | ✓ |
| `skipContainerResolution` | ✓ | | | |
| `skipProxyForCnspec` | | | ✓ | ✓ |
| `metrics.enable` | ✓ | | | |

*\* Unless `skipProxyForCnspec: true`*

**Configuration inheritance:**

```
MondooOperatorConfig (cluster-wide settings)
         │
         ▼
  ┌──────────────┐
  │   Operator   │ ◄── Uses imageRegistry, skipContainerResolution, metrics
  └──────────────┘
         │
         │ Creates workloads with inherited settings
         ▼
  ┌──────────────────────────────────────────────────┐
  │                MondooAuditConfig                 │
  │  ┌────────────┐ ┌────────────┐ ┌──────────────┐  │
  │  │   Nodes    │ │ K8s Scan   │ │  Containers  │  │
  │  │ DaemonSet  │ │  CronJob   │ │   CronJob    │  │
  │  └────────────┘ └────────────┘ └──────────────┘  │
  │        │              │               │          │
  │        └──────────────┼───────────────┘          │
  │                       ▼                          │
  │        Proxy, ImagePullSecrets, Registry         │
  └──────────────────────────────────────────────────┘
```

## Troubleshooting

### Images not pulling from private registry

**Symptoms:** Pods fail with `ImagePullBackOff` or `ErrImagePull`

**Checklist:**

1. Verify the secret exists in the `mondoo-operator` namespace:
   ```bash
   kubectl get secrets -n mondoo-operator
   ```

2. Verify the secret is correctly formatted:
   ```bash
   kubectl get secret registry-credentials -n mondoo-operator -o jsonpath='{.data.\.dockerconfigjson}' | base64 -d
   ```

3. Test the credentials manually:
   ```bash
   docker login registry.example.com -u user -p password
   docker pull registry.example.com/ghcr.io.docker/mondoohq/cnspec:latest
   ```

4. Check that `imagePullSecrets` references the correct secret name in `MondooOperatorConfig`

5. Restart the operator to pick up changes:
   ```bash
   kubectl rollout restart deployment mondoo-operator-controller-manager -n mondoo-operator
   ```

### Proxy not working for some components

**Symptoms:** Some scans work, others fail with connection errors

**Checklist:**

1. Verify proxy environment variables in the pod:
   ```bash
   kubectl exec -n mondoo-operator <pod-name> -- env | grep -i proxy
   ```

2. Check if `skipProxyForCnspec` is accidentally enabled:
   ```bash
   kubectl get mondoooperatorconfig mondoo-operator-config -o yaml | grep skipProxyForCnspec
   ```

3. Verify `noProxy` includes Kubernetes internal addresses:
   ```
   noProxy: "10.0.0.0/8,.cluster.local,.svc,localhost"
   ```

4. Test proxy connectivity from within a pod:
   ```bash
   kubectl run -it --rm proxy-test --image=curlimages/curl -- \
     curl -x http://proxy.example.com:3128 https://api.mondoo.com
   ```

### MondooOperatorConfig not being applied

**Symptoms:** Configuration changes don't take effect

**Checklist:**

1. Verify the resource name is exactly `mondoo-operator-config`:
   ```bash
   kubectl get mondoooperatorconfig
   ```

2. Check for validation errors in the resource:
   ```bash
   kubectl describe mondoooperatorconfig mondoo-operator-config
   ```

3. Check operator logs for configuration loading:
   ```bash
   kubectl logs -n mondoo-operator deployment/mondoo-operator-controller-manager | grep -i config
   ```

4. Restart the operator to force configuration reload:
   ```bash
   kubectl rollout restart deployment mondoo-operator-controller-manager -n mondoo-operator
   ```

### Metrics not appearing in Prometheus

**Symptoms:** ServiceMonitor created but no metrics in Prometheus

**Checklist:**

1. Verify ServiceMonitor exists:
   ```bash
   kubectl get servicemonitor -n mondoo-operator
   ```

2. Check ServiceMonitor labels match your Prometheus selector:
   ```bash
   kubectl get servicemonitor -n mondoo-operator -o yaml | grep -A5 labels
   ```

3. Verify Prometheus is configured to watch the namespace:
   ```bash
   kubectl get prometheus -o yaml | grep -A10 serviceMonitorNamespaceSelector
   ```

4. Check Prometheus targets page for the mondoo-operator target

5. Verify the metrics endpoint is accessible:
   ```bash
   kubectl port-forward -n mondoo-operator svc/mondoo-operator-controller-manager-metrics-service 8080:8080
   curl http://localhost:8080/metrics
   ```

## Helm Configuration Reference

When using Helm, the following `values.yaml` settings map to `MondooOperatorConfig` fields:

```yaml
operator:
  # Create MondooOperatorConfig resource
  createConfig: true

  # Proxy settings
  httpProxy: "http://proxy.example.com:3128"
  httpsProxy: "http://proxy.example.com:3128"
  noProxy: "10.0.0.0/8,.cluster.local"
  containerProxy: ""

  # Registry settings
  imageRegistry: "registry.example.com/ghcr.io.docker"
  registryMirrors:
    ghcr.io: "registry.example.com/ghcr-remote"
    docker.io: "registry.example.com/docker-hub-remote"

  # Image pull credentials
  imagePullSecrets:
    - name: registry-credentials

  # Behavior flags
  skipContainerResolution: true
  skipProxyForCnspec: false
```

**Mapping table:**

| Helm Value | MondooOperatorConfig Field |
|------------|---------------------------|
| `operator.createConfig` | Controls whether CR is created |
| `operator.httpProxy` | `spec.httpProxy` |
| `operator.httpsProxy` | `spec.httpsProxy` |
| `operator.noProxy` | `spec.noProxy` |
| `operator.containerProxy` | `spec.containerProxy` |
| `operator.imageRegistry` | `spec.imageRegistry` |
| `operator.registryMirrors` | `spec.registryMirrors` |
| `operator.imagePullSecrets` | `spec.imagePullSecrets` |
| `operator.skipContainerResolution` | `spec.skipContainerResolution` |
| `operator.skipProxyForCnspec` | `spec.skipProxyForCnspec` |

> **Note:** Metrics configuration is not currently exposed via Helm values. Apply a `MondooOperatorConfig` directly to enable metrics.

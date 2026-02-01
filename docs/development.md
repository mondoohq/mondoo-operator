# Development Guide

This guide helps you set up a local development environment for the Mondoo Operator.

## Prerequisites

- Go 1.21+
- [k3d](https://k3d.io/) (recommended) or [minikube](https://minikube.sigs.k8s.io/docs/start/) as fallback
- kubectl
- A Mondoo account with a service account credential file (`creds.json`)

### Installing k3d

```bash
# macOS
brew install k3d

# Linux
curl -s https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | bash

# Windows (with chocolatey)
choco install k3d
```

## Quick Start

### 1. Start a Local Cluster

```bash
# Using k3d (recommended - faster startup, lower resource usage)
k3d cluster create dev

# Or using minikube (fallback)
minikube start
```

### 2. Install CRDs and Run the Operator

```bash
# Install Custom Resource Definitions
make install

# Run the operator locally (in a separate terminal)
make run
```

### 3. Deploy a Test Configuration

```bash
# Create the namespace
kubectl create namespace mondoo-operator

# Create the ServiceAccount and RBAC for scanning
# (These are normally created by the operator deployment manifests)
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ServiceAccount
metadata:
  name: mondoo-operator-k8s-resources-scanning
  namespace: mondoo-operator
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: mondoo-operator-k8s-resources-scanning
rules:
- apiGroups: ["*"]
  resources: ["*"]
  verbs: ["get", "watch", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: mondoo-operator-k8s-resources-scanning
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: mondoo-operator-k8s-resources-scanning
subjects:
- kind: ServiceAccount
  name: mondoo-operator-k8s-resources-scanning
  namespace: mondoo-operator
EOF

# Create the Mondoo credentials secret
# (Get credentials from Mondoo Console > Settings > Service Accounts)
# IMPORTANT: The key must be named "config" (use --from-file=config=<filename>)
kubectl create secret generic mondoo-client \
  --namespace mondoo-operator \
  --from-file=config=creds.json

# Apply a sample configuration
kubectl apply -f config/samples/k8s_v1alpha2_mondooauditconfig_minimal.yaml
```

### 4. Verify It's Working

```bash
# Check the MondooAuditConfig status
kubectl get mondooauditconfigs -n mondoo-operator

# Wait for the operator to create the CronJobs (may take a few seconds after applying MondooAuditConfig)
kubectl get cronjobs -n mondoo-operator -w
# Wait until you see the CronJob appear, then Ctrl+C:
# NAME                     SCHEDULE      SUSPEND   ACTIVE   LAST SCHEDULE   AGE
# mondoo-client-k8s-scan   0 * * * *     False     0        <none>          10s

# If no CronJobs appear after 30 seconds, check operator logs for errors:
# kubectl logs -n mondoo-operator -l control-plane=controller-manager

# Trigger a scan immediately (instead of waiting for schedule)
# NOTE: This command will fail if the CronJob hasn't been created yet
kubectl create job k8s-scan-now --from=cronjob/mondoo-client-k8s-scan -n mondoo-operator

# Watch the job and logs
kubectl get jobs -n mondoo-operator -w
kubectl logs -n mondoo-operator job/k8s-scan-now -f

# View operator logs (in a separate terminal)
kubectl logs -n mondoo-operator -l control-plane=controller-manager -f
```

## Configuration Reference

### MondooAuditConfig

The `MondooAuditConfig` CRD defines what to scan in your cluster.

#### Minimal Configuration

```yaml
apiVersion: k8s.mondoo.com/v1alpha2
kind: MondooAuditConfig
metadata:
  name: mondoo-client
  namespace: mondoo-operator
spec:
  mondooCredsSecretRef:
    name: mondoo-client
  kubernetesResources:
    enable: true
  containers:
    enable: true
```

#### With Node Scanning

```yaml
apiVersion: k8s.mondoo.com/v1alpha2
kind: MondooAuditConfig
metadata:
  name: mondoo-client
  namespace: mondoo-operator
spec:
  mondooCredsSecretRef:
    name: mondoo-client
  kubernetesResources:
    enable: true
  containers:
    enable: true
  nodes:
    enable: true
    style: cronjob  # or "deployment" or "daemonset"
```

#### Full Configuration Reference

See [config/samples/k8s_v1alpha2_mondooauditconfig.yaml](../config/samples/k8s_v1alpha2_mondooauditconfig.yaml) for a fully documented example with all available options.

### MondooOperatorConfig

The `MondooOperatorConfig` CRD configures cluster-wide operator behavior:

```yaml
apiVersion: k8s.mondoo.com/v1alpha2
kind: MondooOperatorConfig
metadata:
  name: mondoo-operator-config
spec:
  metrics:
    enable: true
    resourceLabels:
      prometheus: mondoo
  skipContainerResolution: false
```

| Field | Description |
|-------|-------------|
| `metrics.enable` | Enable Prometheus metrics endpoint |
| `metrics.resourceLabels` | Labels to add to ServiceMonitor for Prometheus discovery |
| `skipContainerResolution` | Skip resolving image tags to SHA digests |

### Namespace Filtering

Filter which namespaces are scanned:

```yaml
spec:
  filtering:
    namespaces:
      # Exclude specific namespaces
      exclude:
        - kube-system
        - kube-public
      # OR include only specific namespaces (overrides exclude)
      # include:
      #   - production
      #   - staging
```

Note: If both `include` and `exclude` are specified, only `include` is used.

## Running as a Container

Instead of running locally with `make run`, you can deploy the operator as a container:

```bash
# Build and load image into k3d (recommended)
make docker-build
k3d image import ghcr.io/mondoohq/mondoo-operator:latest -c dev

# Or build and load image into minikube (fallback)
make load-minikube

# Deploy the operator
make deploy

# Or deploy using OLM (requires operator-sdk)
make deploy-olm
```

## Running Tests

### Unit Tests

```bash
make test
```

### Integration Tests

```bash
# With k3d (recommended)
k3d cluster create test
K8S_DISTRO=k3d make test/integration

# With minikube (fallback)
minikube start
K8S_DISTRO=minikube make test/integration
```

## Common Development Tasks

### Regenerate Code After Modifying Types

```bash
make generate   # Regenerate deepcopy functions
make manifests  # Regenerate CRD manifests
```

### Trigger a Manual Scan

Instead of waiting for the CronJob schedule, you can trigger scans immediately:

```bash
# List available cronjobs
kubectl get cronjobs -n mondoo-operator
```

Example output:
```
NAME                        SCHEDULE      SUSPEND   ACTIVE   LAST SCHEDULE
mondoo-client-k8s-scan      5 * * * *     False     0        45m
mondoo-client-container     0 * * * *     False     0        15m
```

#### Trigger Kubernetes Resources Scan

```bash
kubectl create job k8s-scan-now --from=cronjob/mondoo-client-k8s-scan -n mondoo-operator
```

#### Trigger Container Image Scan

```bash
kubectl create job container-scan-now --from=cronjob/mondoo-client-container -n mondoo-operator
```

#### Watch Scan Progress

```bash
# Watch job status
kubectl get jobs -n mondoo-operator -w

# Find the pod created by the job
kubectl get pods -n mondoo-operator -l job-name=k8s-scan-now

# Stream logs from the job's pod (easiest method)
kubectl logs -n mondoo-operator job/k8s-scan-now -f

# Or get the pod name and tail logs
POD=$(kubectl get pods -n mondoo-operator -l job-name=k8s-scan-now -o jsonpath='{.items[0].metadata.name}')
kubectl logs -n mondoo-operator $POD -f
```

#### Check Scan Results

```bash
# List recent jobs and their status
kubectl get jobs -n mondoo-operator --sort-by=.metadata.creationTimestamp

# Get detailed job status (includes events and pod info)
kubectl describe job k8s-scan-now -n mondoo-operator

# View logs from completed job
kubectl logs -n mondoo-operator job/k8s-scan-now

# Check pod status if job seems stuck
kubectl get pods -n mondoo-operator -l job-name=k8s-scan-now
kubectl describe pod -n mondoo-operator -l job-name=k8s-scan-now
```

#### Clean Up Manual Jobs

```bash
# Delete a specific job
kubectl delete job k8s-scan-now -n mondoo-operator

# Delete all completed jobs
kubectl delete jobs -n mondoo-operator --field-selector status.successful=1
```

#### Use a Short Schedule for Testing

Alternatively, set a frequent schedule during development:

```yaml
spec:
  kubernetesResources:
    enable: true
    schedule: "*/2 * * * *"  # Every 2 minutes
  containers:
    enable: true
    schedule: "*/2 * * * *"  # Every 2 minutes
```

### Clean Up

```bash
# Delete the MondooAuditConfig
kubectl delete -f config/samples/k8s_v1alpha2_mondooauditconfig_minimal.yaml

# Delete the namespace
kubectl delete namespace mondoo-operator

# Uninstall CRDs
make uninstall

# Delete k3d cluster (if using k3d)
k3d cluster delete dev

# Or delete minikube cluster (if using minikube)
minikube delete
```

## Troubleshooting

### Operator not creating resources?

1. Check operator logs:
   ```bash
   kubectl logs -n mondoo-operator -l control-plane=controller-manager
   ```

2. Check MondooAuditConfig status:
   ```bash
   kubectl describe mondooauditconfig mondoo-client -n mondoo-operator
   ```

3. Verify the credentials secret exists:
   ```bash
   kubectl get secret mondoo-client -n mondoo-operator
   ```

### CRDs not found?

```bash
kubectl get crd | grep mondoo
# Should show:
# mondooauditconfigs.k8s.mondoo.com
# mondoooperatorconfigs.k8s.mondoo.com
```

If missing, run `make install`.

### Scan jobs stuck in ContainerCreating?

Check for mount errors:
```bash
kubectl describe pod -n mondoo-operator -l job-name=<job-name>
```

Common issues:
- **ServiceAccount not found**: Create the RBAC resources (see step 3 above)
- **Secret key not found**: Ensure secret has key named `config`:
  ```bash
  kubectl get secret mondoo-client -n mondoo-operator -o jsonpath='{.data}' | jq 'keys'
  # Should show: ["config"]

  # If wrong, recreate:
  kubectl delete secret mondoo-client -n mondoo-operator
  kubectl create secret generic mondoo-client \
    --namespace mondoo-operator \
    --from-file=config=creds.json
  ```

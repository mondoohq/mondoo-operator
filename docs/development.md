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

#### With External Cluster Scanning

External clusters can be authenticated using four methods:

**1. Kubeconfig (most flexible)**

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
    externalClusters:
      - name: production
        kubeconfigSecretRef:
          name: prod-kubeconfig
```

```bash
# Create kubeconfig secret for remote cluster
kubectl create secret generic prod-kubeconfig \
  --namespace mondoo-operator \
  --from-file=kubeconfig=/path/to/remote-kubeconfig.yaml
```

**2. Service Account Token (simpler alternative)**

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
    externalClusters:
      - name: production
        serviceAccountAuth:
          server: "https://prod-cluster.example.com:6443"
          credentialsSecretRef:
            name: prod-sa-credentials
```

```bash
# Create SA credentials secret (from target cluster)
kubectl create secret generic prod-sa-credentials \
  --namespace mondoo-operator \
  --from-literal=token="$(kubectl get secret scanner-token -o jsonpath='{.data.token}' | base64 -d)" \
  --from-literal=ca.crt="$(kubectl config view --raw -o jsonpath='{.clusters[0].cluster.certificate-authority-data}' | base64 -d)"
```

**3. Workload Identity Federation (cloud-native, no static credentials)**

For GKE:
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
    externalClusters:
      - name: gke-prod
        workloadIdentity:
          provider: gke
          gke:
            projectId: my-gcp-project
            clusterName: production-cluster
            clusterLocation: us-central1-a
            googleServiceAccount: mondoo-scanner@my-gcp-project.iam.gserviceaccount.com
```

For EKS (IRSA):
```yaml
spec:
  kubernetesResources:
    externalClusters:
      - name: eks-prod
        workloadIdentity:
          provider: eks
          eks:
            region: us-west-2
            clusterName: production-cluster
            roleArn: arn:aws:iam::123456789012:role/MondooScannerRole
```

For AKS:
```yaml
spec:
  kubernetesResources:
    externalClusters:
      - name: aks-prod
        workloadIdentity:
          provider: aks
          aks:
            subscriptionId: 12345678-1234-1234-1234-123456789012
            resourceGroup: my-resource-group
            clusterName: production-cluster
            clientId: abcdef12-3456-7890-abcd-ef1234567890
            tenantId: fedcba98-7654-3210-fedc-ba9876543210
```

**4. SPIFFE/SPIRE (zero-trust, auto-rotating X.509 certificates)**

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
    externalClusters:
      - name: spiffe-cluster
        spiffeAuth:
          server: "https://remote-cluster.example.com:6443"
          trustBundleSecretRef:
            name: remote-cluster-ca
          # Optional: custom socket path (default: /run/spire/sockets/agent.sock)
          # socketPath: "/run/spire/sockets/agent.sock"
          # Optional: audience for SVID
          # audience: "k8s-api"
```

```bash
# Create secret with remote cluster's CA certificate
kubectl create secret generic remote-cluster-ca \
  --namespace mondoo-operator \
  --from-file=ca.crt=/path/to/remote-cluster-ca.crt
```

**SPIFFE/SPIRE Prerequisites:**

1. **SPIRE deployed on management cluster**: SPIRE server + agents running as DaemonSet
2. **Workload registration**: Scanner workload must be registered in SPIRE with appropriate SPIFFE ID
3. **Trust relationship**: Remote cluster's API server must be configured to trust the SPIRE CA for client certificate authentication
4. **RBAC on remote cluster**: Create ClusterRole/ClusterRoleBinding for the SPIFFE identity

Example SPIRE registration entry:
```bash
spire-server entry create \
  -spiffeID spiffe://cluster.local/ns/mondoo-operator/sa/mondoo-operator-k8s-resources-scanning \
  -parentID spiffe://cluster.local/ns/spire/sa/spire-agent \
  -selector k8s:ns:mondoo-operator \
  -selector k8s:sa:mondoo-operator-k8s-resources-scanning
```

Example RBAC on remote cluster:
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: mondoo-spiffe-scanner
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: view  # or a custom role with appropriate permissions
subjects:
- apiGroup: rbac.authorization.k8s.io
  kind: User
  name: spiffe://cluster.local/ns/mondoo-operator/sa/mondoo-operator-k8s-resources-scanning
```

**WIF Prerequisites:**

- **GKE**: Enable Workload Identity on management cluster, create IAM binding for KSA→GSA, grant GSA `roles/container.clusterViewer` on target cluster
- **EKS**: Configure IRSA on management cluster, create IAM trust policy, grant role cluster access
- **AKS**: Install Azure Workload Identity, create federated identity credential, grant managed identity RBAC on target cluster

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

## Testing External Cluster Scanning

This section describes how to test external cluster scanning locally using k3d.

### Setup: Two-Cluster Environment

Create two k3d clusters - one as the management cluster (runs the operator) and one as the target cluster (to be scanned):

```bash
# Create the management cluster (where the operator runs)
k3d cluster create management --api-port 6443 -p "8080:80@loadbalancer"

# Create the target cluster (to be scanned externally)
k3d cluster create target --api-port 6444
```

Verify both clusters are running:

```bash
k3d cluster list
# NAME         SERVERS   AGENTS   LOADBALANCER
# management   1/1       0/0      true
# target       1/1       0/0      true
```

### Test Method 1: Kubeconfig Authentication

This is the simplest method to test external cluster scanning.

#### Step 1: Get the target cluster's kubeconfig

```bash
# Switch to target cluster and get its kubeconfig
k3d kubeconfig get target > /tmp/target-kubeconfig.yaml

# The kubeconfig uses localhost, which won't work from within the management cluster.
# We need to update the server URL to use the Docker network IP.

# Get the target cluster's API server IP (accessible from management cluster)
TARGET_IP=$(docker inspect k3d-target-server-0 --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}')
echo "Target cluster IP: $TARGET_IP"

# Update the kubeconfig to use the Docker network IP
sed -i.bak "s|server:.*|server: https://${TARGET_IP}:6443|" /tmp/target-kubeconfig.yaml

# Verify the kubeconfig works (from your host)
KUBECONFIG=/tmp/target-kubeconfig.yaml kubectl get nodes
```

#### Step 2: Set up the management cluster

```bash
# Switch to management cluster
kubectl config use-context k3d-management

# Create namespace and RBAC
kubectl create namespace mondoo-operator

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

# Install CRDs
make install

# Create the kubeconfig secret for the target cluster
kubectl create secret generic target-kubeconfig \
  --namespace mondoo-operator \
  --from-file=kubeconfig=/tmp/target-kubeconfig.yaml

# Create Mondoo credentials secret
kubectl create secret generic mondoo-client \
  --namespace mondoo-operator \
  --from-file=config=creds.json
```

#### Step 3: Deploy MondooAuditConfig with external cluster

```bash
cat <<EOF | kubectl apply -f -
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
    schedule: "*/5 * * * *"  # Every 5 minutes for testing
    externalClusters:
      - name: target
        kubeconfigSecretRef:
          name: target-kubeconfig
EOF
```

#### Step 4: Run the operator and test

```bash
# In one terminal, run the operator
make run

# In another terminal, trigger a scan manually
kubectl create job external-scan-test \
  --from=cronjob/mondoo-client-k8s-scan-target \
  -n mondoo-operator

# Watch the job
kubectl logs -n mondoo-operator job/external-scan-test -f
```

### Test Method 2: Service Account Token Authentication

#### Step 1: Create a service account on the target cluster

```bash
# Switch to target cluster
kubectl config use-context k3d-target

# Create scanner service account with permissions
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ServiceAccount
metadata:
  name: mondoo-scanner
  namespace: default
---
apiVersion: v1
kind: Secret
metadata:
  name: mondoo-scanner-token
  namespace: default
  annotations:
    kubernetes.io/service-account.name: mondoo-scanner
type: kubernetes.io/service-account-token
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: mondoo-scanner
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: view
subjects:
- kind: ServiceAccount
  name: mondoo-scanner
  namespace: default
EOF

# Wait for token to be populated
sleep 5

# Get the token and CA certificate
TOKEN=$(kubectl get secret mondoo-scanner-token -o jsonpath='{.data.token}' | base64 -d)
CA_CERT=$(kubectl get secret mondoo-scanner-token -o jsonpath='{.data.ca\.crt}' | base64 -d)

# Get target cluster IP
TARGET_IP=$(docker inspect k3d-target-server-0 --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}')
echo "Target cluster IP: $TARGET_IP"
```

#### Step 2: Create credentials on management cluster

```bash
# Switch to management cluster
kubectl config use-context k3d-management

# Create the SA credentials secret
kubectl create secret generic target-sa-credentials \
  --namespace mondoo-operator \
  --from-literal=token="$TOKEN" \
  --from-literal=ca.crt="$CA_CERT"
```

#### Step 3: Deploy MondooAuditConfig with service account auth

```bash
cat <<EOF | kubectl apply -f -
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
    schedule: "*/5 * * * *"
    externalClusters:
      - name: target
        serviceAccountAuth:
          server: "https://${TARGET_IP}:6443"
          credentialsSecretRef:
            name: target-sa-credentials
EOF
```

### Cleanup

```bash
# Delete both clusters
k3d cluster delete management
k3d cluster delete target

# Or delete all k3d clusters
k3d cluster delete --all
```

### Troubleshooting External Cluster Tests

#### Connection refused or timeout

The most common issue is network connectivity between clusters. Ensure:

```bash
# Verify the target IP is correct
docker inspect k3d-target-server-0 --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}'

# Test connectivity from management cluster
kubectl run -it --rm debug --image=curlimages/curl --restart=Never -- \
  curl -k https://<TARGET_IP>:6443/healthz
```

#### Certificate errors

If you see certificate validation errors:

```bash
# Ensure the CA certificate is correct
kubectl get secret target-sa-credentials -n mondoo-operator -o jsonpath='{.data.ca\.crt}' | base64 -d | openssl x509 -text -noout

# Or use the kubeconfig's embedded CA
kubectl get secret target-kubeconfig -n mondoo-operator -o jsonpath='{.data.kubeconfig}' | base64 -d
```

#### Check CronJob and Job status

```bash
# List all external cluster CronJobs
kubectl get cronjobs -n mondoo-operator

# Check job logs
kubectl logs -n mondoo-operator -l job-name=<job-name>

# Describe job for events
kubectl describe job <job-name> -n mondoo-operator
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

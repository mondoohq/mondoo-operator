# Development Guide

This guide helps you set up a local development environment for the Mondoo Operator.

## Prerequisites

- Go 1.25+
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
    style: cronjob # or "deployment" or "daemonset"
    # Optional: set priority class for node scanning workloads
    # priorityClassName: high-priority
```

#### With Real-Time Resource Watching

Instead of waiting for the CronJob schedule, you can enable real-time resource watching that scans resources immediately when they change:

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
    resourceWatcher:
      enable: true
      # How long to batch changes before triggering a scan (default: 10s)
      debounceInterval: 10s
      # Minimum time between scans - rate limit (default: 2m)
      minimumScanInterval: 2m
      # Watch all resources including ephemeral ones like Pods (default: false)
      # When false, only watches: Deployments, DaemonSets, StatefulSets, ReplicaSets
      watchAllResources: false
      # Custom list of resource types to watch (optional)
      # resourceTypes:
      #   - deployments
      #   - daemonsets
      #   - statefulsets
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
  name: view # or a custom role with appropriate permissions
subjects:
  - apiGroup: rbac.authorization.k8s.io
    kind: User
    name: spiffe://cluster.local/ns/mondoo-operator/sa/mondoo-operator-k8s-resources-scanning
```

**WIF Prerequisites:**

- **GKE**: Enable Workload Identity on management cluster, create IAM binding for KSA→GSA, grant GSA `roles/container.clusterViewer` on target cluster
- **EKS**: Configure IRSA on management cluster, create IAM trust policy, grant role cluster access
- **AKS**: Install Azure Workload Identity, create federated identity credential, grant managed identity RBAC on target cluster

**Additional External Cluster Options:**

Each external cluster also supports these optional fields:

```yaml
externalClusters:
  - name: production
    kubeconfigSecretRef:
      name: prod-kubeconfig
    # Override the default schedule for this cluster
    schedule: "0 */2 * * *"
    # Cluster-specific namespace filtering
    filtering:
      namespaces:
        exclude:
          - kube-system
    # Enable container image scanning for this external cluster
    containerImageScanning: true
    # Reference to private registry credentials for this cluster
    privateRegistriesPullSecretRef:
      name: prod-registry-creds
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

| Field                     | Description                                              |
| ------------------------- | -------------------------------------------------------- |
| `metrics.enable`          | Enable Prometheus metrics endpoint                       |
| `metrics.resourceLabels`  | Labels to add to ServiceMonitor for Prometheus discovery |
| `skipContainerResolution` | Skip resolving image tags to SHA digests                 |

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

### Scanner Configuration

Configure the scanner that runs K8s resources and container image scans:

```yaml
spec:
  scanner:
    serviceAccountName: mondoo-operator-k8s-resources-scanning
    image:
      name: ghcr.io/mondoohq/mondoo-operator
      tag: latest
    resources:
      requests:
        cpu: 100m
        memory: 100Mi
      limits:
        cpu: 500m
        memory: 500Mi
    # Single private registry secret (deprecated, use privateRegistriesPullSecretRefs)
    # privateRegistriesPullSecretRef:
    #   name: registry-creds
    # Multiple private registry secrets (preferred)
    privateRegistriesPullSecretRefs:
      - name: team-a-registry-creds
      - name: team-b-registry-creds
    # Custom environment variables for the scanner
    env:
      - name: CUSTOM_VAR
        value: "custom-value"
```

Environment variables can also be set on node and container scanners:

```yaml
spec:
  nodes:
    enable: true
    env:
      - name: NODE_SCAN_VAR
        value: "node-value"
  containers:
    enable: true
    env:
      - name: CONTAINER_SCAN_VAR
        value: "container-value"
```

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

## Testing Workload Identity Federation (WIF)

This section describes how to test WIF for external cluster scanning. WIF supports three cloud providers:
- **GKE** (Google Kubernetes Engine) - uses GCP Workload Identity
- **EKS** (Elastic Kubernetes Service) - uses AWS IAM Roles for Service Accounts (IRSA)
- **AKS** (Azure Kubernetes Service) - uses Azure Workload Identity

### Local Testing

WIF resource creation is covered by unit tests:

```bash
# Run WIF-related unit tests
go test -v ./controllers/k8s_scan/... -run "WIF|WorkloadIdentity"
```

These tests verify ServiceAccount annotations, init container configuration, and validation logic for all three providers (GKE, EKS, AKS). The tests include:
- `TestWIFServiceAccount` - Verifies ServiceAccount creation with correct annotations
- `TestWIFInitContainer` - Verifies init container configuration (images, env vars, volume mounts)
- `TestValidateExternalClusterAuth` - Verifies validation rejects invalid configurations

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

**Important: Connect the Docker networks**

k3d creates each cluster on a separate Docker network by default. For the management cluster to reach
the target cluster, we need to connect them:

```bash
# Connect the management cluster's server to the target cluster's network
docker network connect k3d-target k3d-management-server-0

# Verify the connection
docker inspect k3d-management-server-0 --format '{{range $k, $v := .NetworkSettings.Networks}}{{$k}} {{end}}'
# Should show: k3d-management k3d-target
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

# The k3d certificate was generated for localhost, not the Docker network IP.
# We need to skip TLS verification for testing purposes.
# This adds insecure-skip-tls-verify: true and removes certificate-authority-data
yq -i '(.clusters[].cluster.insecure-skip-tls-verify) = true | del(.clusters[].cluster.certificate-authority-data)' /tmp/target-kubeconfig.yaml
```

> **Note:** After updating the kubeconfig to use the Docker network IP, you cannot verify it from your host machine.
> The Docker network IP (e.g., `172.21.0.3`) is only accessible from within containers on the same Docker network.
> The operator's scanning pods will be able to reach this IP because they run inside the management cluster.

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

# (Optional) Verify connectivity from within the cluster
# This runs a pod that uses the kubeconfig to connect to the target cluster
kubectl run connectivity-test \
  --namespace mondoo-operator \
  --image=bitnami/kubectl:latest \
  --restart=Never \
  --rm -it \
  --overrides='
{
  "spec": {
    "containers": [{
      "name": "connectivity-test",
      "image": "bitnami/kubectl:latest",
      "command": ["kubectl", "get", "nodes"],
      "env": [{"name": "KUBECONFIG", "value": "/etc/kubeconfig/kubeconfig"}],
      "volumeMounts": [{"name": "kubeconfig", "mountPath": "/etc/kubeconfig", "readOnly": true}]
    }],
    "volumes": [{"name": "kubeconfig", "secret": {"secretName": "target-kubeconfig"}}]
  }
}'
# Expected output: node/k3d-target-server-0   Ready
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
# Note: Use a different metrics port if 8080 is in use (e.g., by Docker Desktop)
MONDOO_NAMESPACE_OVERRIDE=mondoo-operator go run ./cmd/mondoo-operator/main.go operator --metrics-bind-address=:9090

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

### Test Method 3: SPIFFE/SPIRE Authentication

SPIFFE authentication is the most complex to test locally, but provides zero-trust security with auto-rotating certificates.

> **Note:** This setup is significantly more complex than kubeconfig or service account token authentication.
> It's primarily documented here for completeness and for testing the SPIFFE code path.

#### Prerequisites

- Both clusters from the [Setup: Two-Cluster Environment](#setup-two-cluster-environment) section
- `helm` installed for SPIRE deployment

#### Step 1: Install SPIRE on the management cluster

```bash
# Switch to management cluster
kubectl config use-context k3d-management

# Add the SPIFFE Helm repository
helm repo add spiffe https://spiffe.github.io/helm-charts-hardened/
helm repo update

# Install SPIRE with a minimal configuration for testing
helm install spire spiffe/spire \
  --namespace spire-system \
  --create-namespace \
  --set global.spire.trustDomain=cluster.local \
  --set spire-agent.hostPID=true \
  --wait

# Wait for SPIRE to be ready
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=spire-server -n spire-system --timeout=120s
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=spire-agent -n spire-system --timeout=120s
```

#### Step 2: Register the scanner workload in SPIRE

```bash
# Get the SPIRE server pod name
SPIRE_SERVER=$(kubectl get pod -n spire-system -l app.kubernetes.io/name=spire-server -o jsonpath='{.items[0].metadata.name}')

# Register the mondoo scanner workload
kubectl exec -n spire-system $SPIRE_SERVER -- \
  /opt/spire/bin/spire-server entry create \
  -spiffeID spiffe://cluster.local/ns/mondoo-operator/sa/mondoo-operator-k8s-resources-scanning \
  -parentID spiffe://cluster.local/spire/agent/k8s_psat/k3d-management \
  -selector k8s:ns:mondoo-operator \
  -selector k8s:sa:mondoo-operator-k8s-resources-scanning

# Verify the registration
kubectl exec -n spire-system $SPIRE_SERVER -- \
  /opt/spire/bin/spire-server entry show
```

#### Step 3: Get the SPIRE CA bundle

```bash
# Export the SPIRE trust bundle (CA certificate)
kubectl exec -n spire-system $SPIRE_SERVER -- \
  /opt/spire/bin/spire-server bundle show -format pem > /tmp/spire-bundle.pem
```

#### Step 4: Configure the target cluster to trust SPIRE CA

This is the most complex step. The target cluster's API server must be configured to accept
client certificates signed by the SPIRE CA.

```bash
# Switch to target cluster
kubectl config use-context k3d-target

# Get target cluster IP for later
TARGET_IP=$(docker inspect k3d-target-server-0 --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}')
echo "Target cluster IP: $TARGET_IP"

# Get the target cluster's CA certificate
kubectl config view --raw -o jsonpath='{.clusters[?(@.name=="k3d-target")].cluster.certificate-authority-data}' | base64 -d > /tmp/target-ca.crt

# For k3d/k3s, we need to add the SPIRE CA to the API server's client CA bundle.
# This requires modifying the k3s configuration.

# Copy the SPIRE bundle into the k3d container
docker cp /tmp/spire-bundle.pem k3d-target-server-0:/var/lib/rancher/k3s/server/tls/spire-ca.pem

# Create a combined CA bundle (original + SPIRE)
docker exec k3d-target-server-0 sh -c 'cat /var/lib/rancher/k3s/server/tls/client-ca.crt /var/lib/rancher/k3s/server/tls/spire-ca.pem > /var/lib/rancher/k3s/server/tls/client-ca-combined.crt'

# Update k3s to use the combined CA bundle for client authentication
# Note: This requires restarting k3s with --kube-apiserver-arg=client-ca-file
docker exec k3d-target-server-0 sh -c 'kill 1'  # Restart k3s (it will auto-restart)

# Wait for the target cluster to be ready again
sleep 10
kubectl config use-context k3d-target
kubectl wait --for=condition=ready node --all --timeout=60s
```

> **Important:** The above k3s reconfiguration is simplified for testing. In production,
> you would configure the API server's `--client-ca-file` flag properly during cluster creation.

#### Step 5: Create RBAC for the SPIFFE identity on the target cluster

```bash
# Still on target cluster
cat <<EOF | kubectl apply -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: mondoo-spiffe-scanner
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: view
subjects:
- apiGroup: rbac.authorization.k8s.io
  kind: User
  name: spiffe://cluster.local/ns/mondoo-operator/sa/mondoo-operator-k8s-resources-scanning
EOF
```

#### Step 6: Set up the management cluster

```bash
# Switch back to management cluster
kubectl config use-context k3d-management

# Create the trust bundle secret (target cluster's CA)
kubectl create secret generic target-ca \
  --namespace mondoo-operator \
  --from-file=ca.crt=/tmp/target-ca.crt

# Ensure the mondoo-operator namespace and RBAC exist (from earlier setup)
# If not already done:
kubectl create namespace mondoo-operator --dry-run=client -o yaml | kubectl apply -f -
```

#### Step 7: Deploy MondooAuditConfig with SPIFFE auth

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
        spiffeAuth:
          server: "https://${TARGET_IP}:6443"
          trustBundleSecretRef:
            name: target-ca
          # Use the default SPIRE socket path
          # socketPath: "/run/spire/sockets/agent.sock"
EOF
```

#### Step 8: Run the operator and test

```bash
# In one terminal, run the operator
make run

# In another terminal, verify the CronJob was created
kubectl get cronjobs -n mondoo-operator

# Trigger a scan manually
kubectl create job spiffe-scan-test \
  --from=cronjob/mondoo-client-k8s-scan-target \
  -n mondoo-operator

# Watch the job (look for the init container fetching SVID)
kubectl logs -n mondoo-operator job/spiffe-scan-test -c spiffe-init -f

# Then watch the main container
kubectl logs -n mondoo-operator job/spiffe-scan-test -f
```

#### Troubleshooting SPIFFE

**SPIRE agent socket not found:**
```bash
# Verify SPIRE agent is running and socket exists
kubectl exec -n spire-system -l app.kubernetes.io/name=spire-agent -- ls -la /run/spire/sockets/
```

**Certificate fetch timeout:**
```bash
# Check SPIRE agent logs
kubectl logs -n spire-system -l app.kubernetes.io/name=spire-agent

# Verify workload is registered
kubectl exec -n spire-system $SPIRE_SERVER -- /opt/spire/bin/spire-server entry show
```

**API server rejects certificate:**
```bash
# Verify the target cluster trusts the SPIRE CA
# The API server logs will show certificate validation errors
docker logs k3d-target-server-0 2>&1 | grep -i "client certificate"
```

### Test Method 8: AKS Azure Workload Identity

This method tests Workload Identity Federation (WIF) with Azure AKS. The management cluster
uses Azure AD Workload Identity to authenticate to a target AKS cluster without static credentials.

> **Note:** This requires Azure resources and two AKS clusters. It cannot be tested locally with k3d.

**Key AKS-specific details:**
- ServiceAccount annotation: `azure.workload.identity/client-id`
- ServiceAccount label: `azure.workload.identity/use: "true"` (required)
- Init container image: `mcr.microsoft.com/azure-cli:2.67.0`
- Kubeconfig command: `az aks get-credentials --resource-group $RESOURCE_GROUP --name $CLUSTER_NAME --subscription $SUBSCRIPTION_ID`
- Environment variables: `CLUSTER_NAME`, `RESOURCE_GROUP`, `SUBSCRIPTION_ID`
- API fields: `subscriptionId`, `resourceGroup`, `clusterName`, `clientId`, `tenantId`

#### Prerequisites

- Azure CLI (`az`) installed and authenticated
- Two AKS clusters (management and target)
- Permissions to create Azure AD App Registrations and manage AKS RBAC

#### Step 1: Set up environment variables

```bash
# Azure configuration
export AZURE_SUBSCRIPTION_ID="your-subscription-id"
export AZURE_TENANT_ID="your-tenant-id"
export AZURE_RESOURCE_GROUP="mondoo-operator-test"
export AZURE_LOCATION="eastus"

# Cluster names
export MGMT_CLUSTER_NAME="mondoo-mgmt"
export TARGET_CLUSTER_NAME="mondoo-target"

# Workload Identity configuration
export WI_APP_NAME="mondoo-operator-scanner"
export WI_SERVICE_ACCOUNT_NAMESPACE="mondoo-operator"
export WI_SERVICE_ACCOUNT_NAME="mondoo-client-wif-target"
```

#### Step 2: Create resource group and AKS clusters

```bash
# Create resource group
az group create --name $AZURE_RESOURCE_GROUP --location $AZURE_LOCATION

# Create the management cluster with OIDC issuer and workload identity enabled
az aks create \
  --resource-group $AZURE_RESOURCE_GROUP \
  --name $MGMT_CLUSTER_NAME \
  --node-count 1 \
  --enable-oidc-issuer \
  --enable-workload-identity \
  --generate-ssh-keys

# Create the target cluster
az aks create \
  --resource-group $AZURE_RESOURCE_GROUP \
  --name $TARGET_CLUSTER_NAME \
  --node-count 1 \
  --generate-ssh-keys

# Get credentials for both clusters
az aks get-credentials --resource-group $AZURE_RESOURCE_GROUP --name $MGMT_CLUSTER_NAME --context mgmt
az aks get-credentials --resource-group $AZURE_RESOURCE_GROUP --name $TARGET_CLUSTER_NAME --context target
```

#### Step 3: Get the OIDC issuer URL

```bash
# Get the OIDC issuer URL from the management cluster
export OIDC_ISSUER_URL=$(az aks show \
  --resource-group $AZURE_RESOURCE_GROUP \
  --name $MGMT_CLUSTER_NAME \
  --query "oidcIssuerProfile.issuerUrl" -o tsv)

echo "OIDC Issuer URL: $OIDC_ISSUER_URL"
```

#### Step 4: Create Azure AD App Registration

```bash
# Create an Azure AD application
az ad app create --display-name $WI_APP_NAME

# Get the application (client) ID
export WI_CLIENT_ID=$(az ad app list --display-name $WI_APP_NAME --query "[0].appId" -o tsv)
echo "Client ID: $WI_CLIENT_ID"

# Create a service principal for the application
az ad sp create --id $WI_CLIENT_ID

# Get the service principal object ID (needed for RBAC)
export WI_SP_OBJECT_ID=$(az ad sp show --id $WI_CLIENT_ID --query "id" -o tsv)
echo "Service Principal Object ID: $WI_SP_OBJECT_ID"
```

#### Step 5: Create Federated Identity Credential

This links the Kubernetes ServiceAccount to the Azure AD application:

```bash
# Create the federated identity credential
az ad app federated-credential create \
  --id $WI_CLIENT_ID \
  --parameters '{
    "name": "mondoo-operator-federation",
    "issuer": "'$OIDC_ISSUER_URL'",
    "subject": "system:serviceaccount:'$WI_SERVICE_ACCOUNT_NAMESPACE':'$WI_SERVICE_ACCOUNT_NAME'",
    "audiences": ["api://AzureADTokenExchange"]
  }'

# Verify the federated credential
az ad app federated-credential list --id $WI_CLIENT_ID
```

#### Step 6: Grant RBAC on target cluster

```bash
# Get the target cluster's resource ID
export TARGET_CLUSTER_ID=$(az aks show \
  --resource-group $AZURE_RESOURCE_GROUP \
  --name $TARGET_CLUSTER_NAME \
  --query "id" -o tsv)

# Grant "Azure Kubernetes Service Cluster User Role" to access the cluster
az role assignment create \
  --assignee-object-id $WI_SP_OBJECT_ID \
  --assignee-principal-type ServicePrincipal \
  --role "Azure Kubernetes Service Cluster User Role" \
  --scope $TARGET_CLUSTER_ID

# Also grant "Azure Kubernetes Service RBAC Reader" for reading K8s resources
az role assignment create \
  --assignee-object-id $WI_SP_OBJECT_ID \
  --assignee-principal-type ServicePrincipal \
  --role "Azure Kubernetes Service RBAC Reader" \
  --scope $TARGET_CLUSTER_ID
```

#### Step 7: Set up the management cluster

```bash
# Switch to management cluster
kubectl config use-context mgmt

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

# Create Mondoo credentials secret
kubectl create secret generic mondoo-client \
  --namespace mondoo-operator \
  --from-file=config=creds.json
```

#### Step 8: Deploy MondooAuditConfig with AKS WIF

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
        workloadIdentity:
          provider: aks
          aks:
            subscriptionId: "${AZURE_SUBSCRIPTION_ID}"
            resourceGroup: "${AZURE_RESOURCE_GROUP}"
            clusterName: "${TARGET_CLUSTER_NAME}"
            clientId: "${WI_CLIENT_ID}"
            tenantId: "${AZURE_TENANT_ID}"
EOF
```

#### Step 9: Run the operator and test

```bash
# In one terminal, run the operator
make run

# In another terminal, verify the WIF ServiceAccount was created
kubectl get sa -n mondoo-operator
# Should see: mondoo-client-wif-target with azure.workload.identity annotations

# Verify the ServiceAccount has the correct annotations
kubectl get sa mondoo-client-wif-target -n mondoo-operator -o yaml
# Should show:
#   annotations:
#     azure.workload.identity/client-id: <your-client-id>
#   labels:
#     azure.workload.identity/use: "true"

# Verify the CronJob was created
kubectl get cronjobs -n mondoo-operator

# Trigger a scan manually
kubectl create job aks-wif-scan-test \
  --from=cronjob/mondoo-client-k8s-scan-target \
  -n mondoo-operator

# Watch the init container (Azure CLI fetching credentials)
kubectl logs -n mondoo-operator job/aks-wif-scan-test -c generate-kubeconfig -f

# Then watch the main container
kubectl logs -n mondoo-operator job/aks-wif-scan-test -f
```

#### Troubleshooting AKS WIF

**Init container fails with "AADSTS70021" error:**
```bash
# This usually means the federated credential subject doesn't match
# Verify the subject format matches exactly:
# system:serviceaccount:<namespace>:<service-account-name>

az ad app federated-credential list --id $WI_CLIENT_ID -o table
```

**"Unauthorized" when accessing target cluster:**
```bash
# Verify the role assignments exist
az role assignment list --assignee $WI_SP_OBJECT_ID --scope $TARGET_CLUSTER_ID -o table

# The service principal needs both:
# - "Azure Kubernetes Service Cluster User Role" (to get kubeconfig)
# - "Azure Kubernetes Service RBAC Reader" (to read K8s resources)
```

**Azure CLI in init container fails:**
```bash
# Check the init container logs for detailed error
kubectl logs -n mondoo-operator job/aks-wif-scan-test -c generate-kubeconfig

# Verify the ServiceAccount token is being projected
kubectl get pod -n mondoo-operator -l job-name=aks-wif-scan-test -o yaml | grep -A20 "serviceAccountToken"
```

**Cleanup Azure Resources:**
```bash
# Delete the resource group (deletes everything)
az group delete --name $AZURE_RESOURCE_GROUP --yes --no-wait

# Or delete individual resources:
az ad app delete --id $WI_CLIENT_ID
az aks delete --resource-group $AZURE_RESOURCE_GROUP --name $MGMT_CLUSTER_NAME --yes --no-wait
az aks delete --resource-group $AZURE_RESOURCE_GROUP --name $TARGET_CLUSTER_NAME --yes --no-wait
```

### Test Method 7: EKS IAM Roles for Service Accounts (IRSA)

This method tests Workload Identity using AWS EKS IRSA. The management cluster uses IAM Roles
for Service Accounts to authenticate to a target EKS cluster without static credentials.

> **Note:** This requires AWS resources and two EKS clusters. It cannot be tested locally with k3d.

**Key EKS-specific details:**
- ServiceAccount annotation: `eks.amazonaws.com/role-arn`
- Init container image: `amazon/aws-cli:2.22.0`
- Kubeconfig command: `aws eks update-kubeconfig --name $CLUSTER_NAME --region $AWS_REGION`
- Environment variables: `CLUSTER_NAME`, `AWS_REGION`
- API fields: `region`, `clusterName`, `roleArn`

#### Prerequisites

- AWS CLI (`aws`) installed and configured
- `eksctl` installed (recommended for EKS cluster creation)
- Two EKS clusters (management and target)
- Permissions to create IAM roles and policies

#### Step 1: Set up environment variables

```bash
# AWS configuration
export AWS_REGION="us-west-2"
export AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)

# Cluster names
export MGMT_CLUSTER_NAME="mondoo-mgmt"
export TARGET_CLUSTER_NAME="mondoo-target"

# IAM role configuration
export IAM_ROLE_NAME="mondoo-operator-scanner"
export WIF_SERVICE_ACCOUNT_NAMESPACE="mondoo-operator"
export WIF_SERVICE_ACCOUNT_NAME="mondoo-client-wif-target"
```

#### Step 2: Create EKS clusters

```bash
# Create the management cluster with OIDC provider
eksctl create cluster \
  --name $MGMT_CLUSTER_NAME \
  --region $AWS_REGION \
  --nodes 2 \
  --with-oidc

# Create the target cluster
eksctl create cluster \
  --name $TARGET_CLUSTER_NAME \
  --region $AWS_REGION \
  --nodes 2

# Verify clusters are running
eksctl get cluster --region $AWS_REGION

# Get credentials for both clusters
aws eks update-kubeconfig --name $MGMT_CLUSTER_NAME --region $AWS_REGION --alias mgmt
aws eks update-kubeconfig --name $TARGET_CLUSTER_NAME --region $AWS_REGION --alias target
```

#### Step 3: Get the OIDC provider URL

```bash
# Get the OIDC provider URL from the management cluster
export OIDC_PROVIDER=$(aws eks describe-cluster \
  --name $MGMT_CLUSTER_NAME \
  --region $AWS_REGION \
  --query "cluster.identity.oidc.issuer" \
  --output text | sed 's|https://||')

echo "OIDC Provider: $OIDC_PROVIDER"

# Verify the OIDC provider is associated with IAM
aws iam list-open-id-connect-providers | grep $OIDC_PROVIDER
```

#### Step 4: Create IAM role with trust policy

```bash
# Create the trust policy document
cat > /tmp/trust-policy.json << EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "arn:aws:iam::${AWS_ACCOUNT_ID}:oidc-provider/${OIDC_PROVIDER}"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": {
          "${OIDC_PROVIDER}:aud": "sts.amazonaws.com",
          "${OIDC_PROVIDER}:sub": "system:serviceaccount:${WIF_SERVICE_ACCOUNT_NAMESPACE}:${WIF_SERVICE_ACCOUNT_NAME}"
        }
      }
    }
  ]
}
EOF

# Create the IAM role
aws iam create-role \
  --role-name $IAM_ROLE_NAME \
  --assume-role-policy-document file:///tmp/trust-policy.json \
  --description "Role for Mondoo Operator to scan EKS clusters"

# Get the role ARN
export IAM_ROLE_ARN=$(aws iam get-role --role-name $IAM_ROLE_NAME --query 'Role.Arn' --output text)
echo "IAM Role ARN: $IAM_ROLE_ARN"
```

#### Step 5: Attach permissions to access target cluster

```bash
# Create a policy that allows describing and accessing EKS clusters
cat > /tmp/eks-access-policy.json << EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "eks:DescribeCluster",
        "eks:ListClusters"
      ],
      "Resource": "*"
    }
  ]
}
EOF

# Create and attach the policy
aws iam put-role-policy \
  --role-name $IAM_ROLE_NAME \
  --policy-name eks-access \
  --policy-document file:///tmp/eks-access-policy.json
```

#### Step 6: Grant Kubernetes RBAC on target cluster

```bash
# Switch to target cluster
kubectl config use-context target

# Create a ClusterRole and ClusterRoleBinding for the IAM role
# The IAM role will be mapped to a Kubernetes user via aws-auth ConfigMap

# First, get the current aws-auth ConfigMap
kubectl get configmap aws-auth -n kube-system -o yaml > /tmp/aws-auth.yaml

# Add the IAM role mapping (you may need to edit this manually)
# Add this to the mapRoles section:
cat << EOF

# Add this entry to mapRoles in aws-auth ConfigMap:
# - rolearn: ${IAM_ROLE_ARN}
#   username: mondoo-scanner
#   groups:
#     - mondoo-scanners

EOF

# Apply the updated aws-auth ConfigMap
# Option 1: Use eksctl (recommended)
eksctl create iamidentitymapping \
  --cluster $TARGET_CLUSTER_NAME \
  --region $AWS_REGION \
  --arn $IAM_ROLE_ARN \
  --username mondoo-scanner \
  --group mondoo-scanners

# Create RBAC for the mondoo-scanners group
cat <<EOF | kubectl apply -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: mondoo-scanner-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: view
subjects:
- apiGroup: rbac.authorization.k8s.io
  kind: Group
  name: mondoo-scanners
EOF
```

#### Step 7: Set up the management cluster

```bash
# Switch to management cluster
kubectl config use-context mgmt

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

# Create Mondoo credentials secret
kubectl create secret generic mondoo-client \
  --namespace mondoo-operator \
  --from-file=config=creds.json
```

#### Step 8: Deploy MondooAuditConfig with EKS IRSA

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
        workloadIdentity:
          provider: eks
          eks:
            region: "${AWS_REGION}"
            clusterName: "${TARGET_CLUSTER_NAME}"
            roleArn: "${IAM_ROLE_ARN}"
EOF
```

#### Step 9: Run the operator and test

```bash
# In one terminal, run the operator
make run

# In another terminal, verify the IRSA ServiceAccount was created
kubectl get sa -n mondoo-operator
# Should see: mondoo-client-wif-target with eks.amazonaws.com/role-arn annotation

# Verify the ServiceAccount has the correct annotation
kubectl get sa mondoo-client-wif-target -n mondoo-operator -o yaml
# Should show:
#   annotations:
#     eks.amazonaws.com/role-arn: arn:aws:iam::<account>:role/mondoo-operator-scanner

# Verify the CronJob was created
kubectl get cronjobs -n mondoo-operator

# Trigger a scan manually
kubectl create job eks-irsa-scan-test \
  --from=cronjob/mondoo-client-k8s-scan-target \
  -n mondoo-operator

# Watch the init container (AWS CLI fetching credentials)
kubectl logs -n mondoo-operator job/eks-irsa-scan-test -c generate-kubeconfig -f

# Then watch the main container
kubectl logs -n mondoo-operator job/eks-irsa-scan-test -f
```

#### Troubleshooting EKS IRSA

**Init container fails with "An error occurred (AccessDenied)":**
```bash
# This usually means the trust policy doesn't match the ServiceAccount
# Verify the trust policy subject matches exactly:
# system:serviceaccount:<namespace>:<service-account-name>

aws iam get-role --role-name $IAM_ROLE_NAME --query 'Role.AssumeRolePolicyDocument'

# The OIDC provider URL must match exactly (no https://)
echo "Expected: ${OIDC_PROVIDER}:sub = system:serviceaccount:${WIF_SERVICE_ACCOUNT_NAMESPACE}:${WIF_SERVICE_ACCOUNT_NAME}"
```

**"error: You must be logged in to the server (Unauthorized)":**
```bash
# The IAM role is not mapped in the target cluster's aws-auth ConfigMap
# Verify the mapping exists:
kubectl config use-context target
kubectl get configmap aws-auth -n kube-system -o yaml | grep -A5 $IAM_ROLE_ARN

# Or check with eksctl:
eksctl get iamidentitymapping --cluster $TARGET_CLUSTER_NAME --region $AWS_REGION
```

**AWS CLI in init container fails:**
```bash
# Check the init container logs for detailed error
kubectl logs -n mondoo-operator job/eks-irsa-scan-test -c generate-kubeconfig

# Verify the IRSA webhook is injecting the environment variables
kubectl get pod -n mondoo-operator -l job-name=eks-irsa-scan-test -o yaml | grep -A5 "AWS_"
# Should see AWS_ROLE_ARN and AWS_WEB_IDENTITY_TOKEN_FILE
```

**Verify OIDC provider is correctly associated:**
```bash
# List OIDC providers
aws iam list-open-id-connect-providers

# Verify the provider exists for the management cluster
aws eks describe-cluster --name $MGMT_CLUSTER_NAME --region $AWS_REGION \
  --query "cluster.identity.oidc.issuer" --output text
```

**Cleanup AWS Resources:**
```bash
# Delete IAM role and policies
aws iam delete-role-policy --role-name $IAM_ROLE_NAME --policy-name eks-access
aws iam delete-role --role-name $IAM_ROLE_NAME

# Delete EKS clusters (this takes several minutes)
eksctl delete cluster --name $MGMT_CLUSTER_NAME --region $AWS_REGION --wait
eksctl delete cluster --name $TARGET_CLUSTER_NAME --region $AWS_REGION --wait
```

### Test Method 6: GKE Workload Identity

This method tests Workload Identity Federation with Google Kubernetes Engine. The management cluster
uses GCP Workload Identity to authenticate to a target GKE cluster without static credentials.

> **Note:** This requires GCP resources and two GKE clusters. It cannot be tested locally with k3d.

**Key GKE-specific details:**
- ServiceAccount annotation: `iam.gke.io/gcp-service-account`
- Init container image: `gcr.io/google.com/cloudsdktool/google-cloud-cli:499.0.0-slim`
- Kubeconfig command: `gcloud container clusters get-credentials $CLUSTER_NAME --project $PROJECT_ID --location $CLUSTER_LOCATION`
- Environment variables: `CLUSTER_NAME`, `PROJECT_ID`, `CLUSTER_LOCATION`
- API fields: `projectId`, `clusterName`, `clusterLocation`, `googleServiceAccount`

#### Prerequisites

- Google Cloud CLI (`gcloud`) installed and authenticated
- Two GKE clusters (management and target)
- Permissions to create IAM service accounts and manage GKE RBAC

#### Step 1: Set up environment variables

```bash
# GCP configuration
export GCP_PROJECT_ID="your-project-id"
export GCP_REGION="us-central1"
export GCP_ZONE="us-central1-a"

# Cluster names
export MGMT_CLUSTER_NAME="mondoo-mgmt"
export TARGET_CLUSTER_NAME="mondoo-target"

# Workload Identity configuration
export GSA_NAME="mondoo-scanner"
export WIF_SERVICE_ACCOUNT_NAMESPACE="mondoo-operator"
export WIF_SERVICE_ACCOUNT_NAME="mondoo-client-wif-target"
```

#### Step 2: Create GKE clusters

```bash
# Create the management cluster with Workload Identity enabled
gcloud container clusters create $MGMT_CLUSTER_NAME \
  --project=$GCP_PROJECT_ID \
  --zone=$GCP_ZONE \
  --num-nodes=2 \
  --workload-pool=${GCP_PROJECT_ID}.svc.id.goog

# Create the target cluster
gcloud container clusters create $TARGET_CLUSTER_NAME \
  --project=$GCP_PROJECT_ID \
  --zone=$GCP_ZONE \
  --num-nodes=2

# Verify clusters are running
gcloud container clusters list --project=$GCP_PROJECT_ID

# Get credentials for both clusters
gcloud container clusters get-credentials $MGMT_CLUSTER_NAME \
  --project=$GCP_PROJECT_ID --zone=$GCP_ZONE
kubectl config rename-context $(kubectl config current-context) mgmt

gcloud container clusters get-credentials $TARGET_CLUSTER_NAME \
  --project=$GCP_PROJECT_ID --zone=$GCP_ZONE
kubectl config rename-context $(kubectl config current-context) target
```

#### Step 3: Create GCP Service Account

```bash
# Create a Google Service Account (GSA)
gcloud iam service-accounts create $GSA_NAME \
  --project=$GCP_PROJECT_ID \
  --display-name="Mondoo Scanner Service Account"

# Get the GSA email
export GSA_EMAIL="${GSA_NAME}@${GCP_PROJECT_ID}.iam.gserviceaccount.com"
echo "GSA Email: $GSA_EMAIL"
```

#### Step 4: Create IAM binding for Workload Identity

This links the Kubernetes ServiceAccount to the GCP Service Account:

```bash
# Allow the KSA to impersonate the GSA
gcloud iam service-accounts add-iam-policy-binding $GSA_EMAIL \
  --project=$GCP_PROJECT_ID \
  --role="roles/iam.workloadIdentityUser" \
  --member="serviceAccount:${GCP_PROJECT_ID}.svc.id.goog[${WIF_SERVICE_ACCOUNT_NAMESPACE}/${WIF_SERVICE_ACCOUNT_NAME}]"
```

#### Step 5: Grant GSA access to target cluster

```bash
# Grant the GSA permission to get cluster credentials
gcloud projects add-iam-policy-binding $GCP_PROJECT_ID \
  --member="serviceAccount:${GSA_EMAIL}" \
  --role="roles/container.clusterViewer"

# For reading Kubernetes resources, we need to grant RBAC on the target cluster
kubectl config use-context target

cat <<EOF | kubectl apply -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: mondoo-scanner-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: view
subjects:
- apiGroup: rbac.authorization.k8s.io
  kind: User
  name: ${GSA_EMAIL}
EOF
```

#### Step 6: Set up the management cluster

```bash
# Switch to management cluster
kubectl config use-context mgmt

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

# Create Mondoo credentials secret
kubectl create secret generic mondoo-client \
  --namespace mondoo-operator \
  --from-file=config=creds.json
```

#### Step 7: Deploy MondooAuditConfig with GKE WIF

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
        workloadIdentity:
          provider: gke
          gke:
            projectId: "${GCP_PROJECT_ID}"
            clusterName: "${TARGET_CLUSTER_NAME}"
            clusterLocation: "${GCP_ZONE}"
            googleServiceAccount: "${GSA_EMAIL}"
EOF
```

#### Step 8: Run the operator and test

```bash
# In one terminal, run the operator
make run

# In another terminal, verify the WIF ServiceAccount was created
kubectl get sa -n mondoo-operator
# Should see: mondoo-client-wif-target with iam.gke.io/gcp-service-account annotation

# Verify the ServiceAccount has the correct annotation
kubectl get sa mondoo-client-wif-target -n mondoo-operator -o yaml
# Should show:
#   annotations:
#     iam.gke.io/gcp-service-account: mondoo-scanner@<project>.iam.gserviceaccount.com

# Verify the CronJob was created
kubectl get cronjobs -n mondoo-operator

# Trigger a scan manually
kubectl create job gke-wif-scan-test \
  --from=cronjob/mondoo-client-k8s-scan-target \
  -n mondoo-operator

# Watch the init container (gcloud fetching credentials)
kubectl logs -n mondoo-operator job/gke-wif-scan-test -c generate-kubeconfig -f

# Then watch the main container
kubectl logs -n mondoo-operator job/gke-wif-scan-test -f
```

#### Troubleshooting GKE WIF

**Init container fails with "ERROR: (gcloud.container.clusters.get-credentials)":**
```bash
# This usually means the GSA doesn't have permission to get cluster credentials
# Verify the GSA has the container.clusterViewer role:
gcloud projects get-iam-policy $GCP_PROJECT_ID \
  --flatten="bindings[].members" \
  --filter="bindings.members:${GSA_EMAIL}" \
  --format="table(bindings.role)"
```

**"Forbidden" when accessing target cluster:**
```bash
# The GSA needs RBAC on the target cluster
# Verify the ClusterRoleBinding exists:
kubectl config use-context target
kubectl get clusterrolebinding mondoo-scanner-binding -o yaml
```

**Workload Identity not working:**
```bash
# Verify the management cluster has Workload Identity enabled
gcloud container clusters describe $MGMT_CLUSTER_NAME \
  --project=$GCP_PROJECT_ID --zone=$GCP_ZONE \
  --format="value(workloadIdentityConfig.workloadPool)"
# Should show: <project>.svc.id.goog

# Verify the IAM binding exists
gcloud iam service-accounts get-iam-policy $GSA_EMAIL --project=$GCP_PROJECT_ID
# Should show the workloadIdentityUser binding for the KSA
```

**gcloud in init container fails:**
```bash
# Check the init container logs for detailed error
kubectl logs -n mondoo-operator job/gke-wif-scan-test -c generate-kubeconfig

# Verify the projected token is available
kubectl get pod -n mondoo-operator -l job-name=gke-wif-scan-test -o yaml | grep -A10 "serviceAccountToken"
```

**Cleanup GCP Resources:**
```bash
# Delete GKE clusters
gcloud container clusters delete $MGMT_CLUSTER_NAME \
  --project=$GCP_PROJECT_ID --zone=$GCP_ZONE --quiet
gcloud container clusters delete $TARGET_CLUSTER_NAME \
  --project=$GCP_PROJECT_ID --zone=$GCP_ZONE --quiet

# Delete the GSA
gcloud iam service-accounts delete $GSA_EMAIL --project=$GCP_PROJECT_ID --quiet
```

### WIF Testing Summary

| Test Type | How to Run |
|-----------|-----------|
| Unit tests (all providers) | `go test -v ./controllers/k8s_scan/... -run "WIF\|WorkloadIdentity"` |
| GKE end-to-end | Follow Test Method 6 |
| EKS end-to-end | Follow Test Method 7 |
| AKS end-to-end | Follow Test Method 8 |

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
    schedule: "*/2 * * * *" # Every 2 minutes
  containers:
    enable: true
    schedule: "*/2 * * * *" # Every 2 minutes
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

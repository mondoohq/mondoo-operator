# Admission Controller Removal - Migration Guide

Starting with version 12.1.0, the Mondoo Operator no longer includes the admission controller functionality. This document explains the change and provides alternatives for achieving similar security controls.

## Why Was Admission Scanning Removed?

The admission controller was removed to simplify the operator and reduce operational complexity:

1. **TLS Certificate Management**: The admission webhook required managing TLS certificates, either through cert-manager or OpenShift's service serving certificates. This added significant operational burden.

2. **Pipeline-Based Scanning Is More Effective**: Policy enforcement belongs in the CI/CD pipeline, before workloads are deployed. This provides faster feedback to developers and prevents policy violations earlier in the development cycle.

3. **Reduced Attack Surface**: Removing the webhook reduces the operator's attack surface and eliminates a potential point of failure in the Kubernetes API server request path.

## Migration Steps

### 1. Remove Admission Configuration from MondooAuditConfig

If your `MondooAuditConfig` includes admission configuration, remove it:

**Before:**
```yaml
apiVersion: k8s.mondoo.com/v1alpha2
kind: MondooAuditConfig
metadata:
  name: mondoo-client
spec:
  mondooCredsSecretRef:
    name: mondoo-client
  admission:
    enable: true
    mode: enforcing
    certificateProvisioning:
      mode: cert-manager
  kubernetesResources:
    enable: true
```

**After:**
```yaml
apiVersion: k8s.mondoo.com/v1alpha2
kind: MondooAuditConfig
metadata:
  name: mondoo-client
spec:
  mondooCredsSecretRef:
    name: mondoo-client
  kubernetesResources:
    enable: true
```

### 2. Automatic Resource Cleanup

Starting with version 12.1.0, the operator automatically cleans up orphaned admission
resources when it detects deprecated admission configuration in your MondooAuditConfig.

The following resources are automatically removed:
- ValidatingWebhookConfiguration (`{namespace}-{name}-mondoo`)
- Webhook Deployment (`{name}-webhook-manager`)
- Webhook Service (`{name}-webhook-service`)
- Webhook TLS Secret (`{name}-webhook-server-cert`)

**Note**: If you used cert-manager, you should still manually clean up Certificate
and Issuer resources:

```bash
kubectl delete certificate webhook-serving-cert -n <namespace> --ignore-not-found
kubectl delete issuer mondoo-operator-selfsigned-issuer -n <namespace> --ignore-not-found
```

### 3. Manual Cleanup (Optional)

If you prefer to manually clean up admission resources, or if you already removed the
admission configuration from your MondooAuditConfig before upgrading, clean up the leftover resources. Replace `<namespace>` with your namespace (e.g., `mondoo-operator`) and `<name>` with your MondooAuditConfig name (e.g., `mondoo-client`):

```bash
# Delete the ValidatingWebhookConfiguration (cluster-scoped)
# Name format: {namespace}-{name}-mondoo
kubectl delete validatingwebhookconfiguration <namespace>-<name>-mondoo --ignore-not-found

# Delete the webhook deployment
kubectl delete deployment <name>-webhook-manager -n <namespace> --ignore-not-found

# Delete the webhook service
kubectl delete service <name>-webhook-service -n <namespace> --ignore-not-found

# Delete webhook TLS secret
kubectl delete secret <name>-webhook-server-cert -n <namespace> --ignore-not-found

# If using cert-manager, delete the certificate and issuer
kubectl delete certificate webhook-serving-cert -n <namespace> --ignore-not-found
kubectl delete issuer mondoo-operator-selfsigned-issuer -n <namespace> --ignore-not-found
```

For example, if your namespace is `mondoo-operator` and your MondooAuditConfig is named `mondoo-client`:

```bash
kubectl delete validatingwebhookconfiguration mondoo-operator-mondoo-client-mondoo --ignore-not-found
kubectl delete deployment mondoo-client-webhook-manager -n mondoo-operator --ignore-not-found
kubectl delete service mondoo-client-webhook-service -n mondoo-operator --ignore-not-found
kubectl delete secret mondoo-client-webhook-server-cert -n mondoo-operator --ignore-not-found
kubectl delete certificate webhook-serving-cert -n mondoo-operator --ignore-not-found
kubectl delete issuer mondoo-operator-selfsigned-issuer -n mondoo-operator --ignore-not-found
```

### 4. Implement CI/CD Pipeline Scanning

To achieve policy enforcement before deployment, integrate cnspec scanning into your CI/CD pipelines.

#### GitHub Actions

```yaml
name: Security Scan
on: [push, pull_request]

jobs:
  scan-manifests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Install cnspec
        run: |
          curl -sSL https://install.mondoo.com/sh | bash

      - name: Scan Kubernetes manifests
        run: |
          cnspec scan k8s --path ./manifests --score-threshold 80
        env:
          MONDOO_CONFIG_BASE64: ${{ secrets.MONDOO_SERVICE_ACCOUNT }}
```

#### GitLab CI

```yaml
scan-manifests:
  image: mondoo/cnspec:latest
  stage: test
  script:
    - cnspec scan k8s --path ./manifests --score-threshold 80
  variables:
    MONDOO_CONFIG_BASE64: $MONDOO_SERVICE_ACCOUNT
```

#### Argo CD

For Argo CD, the recommended approach is to scan manifests in your CI pipeline before they are committed to the GitOps repository. This ensures policy violations are caught before Argo CD ever sees the manifests.

Add a scan step to your CI pipeline that runs before pushing to the GitOps repo:

```yaml
# Example: GitHub Actions workflow for a GitOps repository
name: Validate Manifests
on:
  pull_request:
    paths:
      - 'apps/**'
      - 'clusters/**'

jobs:
  scan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Install cnspec
        run: curl -sSL https://install.mondoo.com/sh | bash

      - name: Scan Kubernetes manifests
        run: cnspec scan k8s --path . --score-threshold 80
        env:
          MONDOO_CONFIG_BASE64: ${{ secrets.MONDOO_SERVICE_ACCOUNT }}
```

#### Tekton Pipeline

```yaml
apiVersion: tekton.dev/v1
kind: Task
metadata:
  name: mondoo-scan
spec:
  params:
    - name: manifest-path
      type: string
      default: "./manifests"
  workspaces:
    - name: source
  steps:
    - name: scan
      image: mondoo/cnspec:latest
      script: |
        cnspec scan k8s --path $(workspaces.source.path)/$(params.manifest-path) --score-threshold 80
      env:
        - name: MONDOO_CONFIG_BASE64
          valueFrom:
            secretKeyRef:
              name: mondoo-credentials
              key: config
```

## Benefits of Pipeline-Based Scanning

- **Shift Left**: Catch policy violations during development, not at deployment time
- **Faster Feedback**: Developers see issues immediately in their PR/MR
- **No Production Impact**: Scanning happens outside the cluster
- **Better Visibility**: Scan results are visible in CI/CD logs and PR comments
- **Simpler Operations**: No webhook infrastructure to maintain

## Continued In-Cluster Scanning

The Mondoo Operator continues to provide:

- **Kubernetes Resources Scanning**: Scheduled scanning of deployed workloads, RBAC, and cluster configuration
- **Container Image Scanning**: Vulnerability scanning of container images running in the cluster
- **Node Scanning**: CIS benchmark and vulnerability scanning of Kubernetes nodes

These features provide continuous visibility into your cluster's security posture without the complexity of admission webhooks.

## Questions?

If you have questions about this migration, please open an issue at [https://github.com/mondoohq/mondoo-operator/issues](https://github.com/mondoohq/mondoo-operator/issues).

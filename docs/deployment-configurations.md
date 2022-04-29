# Audit Configuration

Mondoo Client can be configured to run security policies against Kubernetes objects in all namespaces or a specific namespace. This can be achieved by altering the inventory in the MondooAuditConfig CRD.

> By default Mondoo Client is configured to run policies against objects in all namespaces.

## Example Config

~~To run Mondoo policies against Kubernetes objects in the example namespace the following configuration can be used.~~
This is not possible at the moment. A follow-up feature will allow this.

# Operator Configuration

The Mondoo Operator can be configured via a cluster-scoped MondooOperatorConfig resource named `mondoo-operator-config`. This allows for setting up features and behaviors that control the Mondoo Operator itself.

## Example Config

To enable metrics reporting via Prometheus and to disable the translation of container image/tags to image/checksums, the following configuration would be used.

```yaml
apiVersion: k8s.mondoo.com/v1alpha1
Kind: MondooOperatorConfig
metadata:
  name: mondoo-operator-config
spec:
  metrics:
    enable: true
    resourceLabels:
      prom-k8s: release
  skipContainerResolution: true
```

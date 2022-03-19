# Deployment Configuration

Mondoo Client can be configured to run security policies against Kubernetes objects in all namespaces or a specific namespace. This can be achieved by altering the inventory in the MondooAuditConfig CRD.

> By default Mondoo Client is configured to run policies against objects in all namespaces.

# Example Config

To run Mondoo policies against Kubernetes objects in the example namespace the following configuration can be used.

```yaml
apiVersion: k8s.mondoo.com/v1alpha1
kind: MondooAuditConfig
metadata:
  name: mondoo-client
  namespace: mondoo-operator
spec:
  workloads:
    enable: true
    inventory: |
      apiVersion: v1
      kind: Inventory
      metadata:
        name: mondoo-k8s-inventory
        labels:
          environment: production
      spec:
        assets:
          - id: api
            connections:
              - backend: k8s
                options:
                  namespace: "example"
    serviceAccount: mondoo-operator-workload
  nodes:
    enable: true
  mondooSecretRef: mondoo-client
```

# Install Mondoo Operator with kubectl

The following steps sets up the mondoo operator using `kubectl` and a manifest file.

## Preconditions:

- `kubectl` and cluster with admin role

## Deployment of Operator using Manifests

1. GET operator manifests

```bash
kubectl apply -f https://github.com/mondoohq/mondoo-operator/releases/latest/download/mondoo-operator-manifests.yaml
```

or

```bash
curl -sSL https://github.com/mondoohq/mondoo-operator/releases/latest/download/mondoo-operator-manifests.yaml > mondoo-operator-manifests.yaml
kubectl apply -f mondoo-operator-manifests.yaml
```

2. Configure the Mondoo secret:

- Create a new Mondoo service account to report assessments to [Mondoo Platform](https://mondoo.com/docs/platform/service_accounts)
- Store the service account json into a local file `creds.json`
- Store service account as a secret in the mondoo namespace via:

```bash
kubectl create secret generic mondoo-client --namespace mondoo-operator-system --from-file=config=creds.json
```

Once the secret is configure, we configure the operator to define the scan targets:

3. Create `mondoo-config.yaml`

```yaml
apiVersion: k8s.mondoo.com/v1alpha1
kind: MondooAuditConfig
metadata:
  name: mondoo-client
  namespace: mondoo-operator-system
spec:
  workloads:
    enable: true
    serviceAccount: mondoo-operator-workload
  nodes:
    enable: true
    serviceAccount: mondoo-operator-workload
  mondooSecretRef: mondoo-client
```

4. Apply the configuration via:

```bash
kubectl apply -f mondoo-config.yaml
```

## FAQ

**I do not see the service running, only the operator?**

First check that the CRD is properly registered with the operator:

```bash
kubectl get crd
NAME                           CREATED AT
mondooauditconfigs.k8s.mondoo.com   2022-01-14T14:07:28Z
```

Then make sure a configuration for the mondoo client is deployed:

```bash
kubectl get mondooauditconfigs
NAME                  AGE
mondoo-client        2m44s
```

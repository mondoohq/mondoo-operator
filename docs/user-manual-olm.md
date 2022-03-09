# Install Mondoo Operator with olm

The following steps sets up the mondoo operator using kubectl and a manifest file.

## Preconditions:

- `kubectl` with admin role
- [`operator-lifecycle-manager`](https://olm.operatorframework.io/) installed in cluster
- `operator-sdk`

## Deployment of Operator using Manifests

1. Configure the Mondoo secret:

- Create a new Mondoo service account to report assessments to [Mondoo Platform](https://mondoo.com/docs/platform/service_accounts)
- Store the service account json into a local file `creds.json`
- Store service account as a secret in the mondoo namespace via:

```bash
kubectl create secret generic mondoo-client --namespace mondoo-operator --from-file=config=creds.json
```

2. Verify that operator-lifecycle-manager is up

```bash
operator-sdk olm status | echo $?
```

3. Run the operator-bundle

```bash
operator-sdk run bundle ghcr.io/mondoohq/mondoo-operator-bundle:latest --namespace=mondoo-operator
```

4. Create `mondoo-config.yaml`

```yaml
apiVersion: k8s.mondoo.com/v1alpha1
kind: MondooAuditConfig
metadata:
  name: mondoo-client
  namespace: mondoo-operator
spec:
  workloads:
    enable: true
    serviceAccount: mondoo-operator-workload
  nodes:
    enable: true
  mondooSecretRef: mondoo-client
```

5. Apply the configuration via:

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

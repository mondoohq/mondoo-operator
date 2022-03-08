# Development Setup

The following steps setup a development Kubernetes to test the operator using helm.

## Preconditions:

- Kubernetes Cluster with admin access
- `kubectl`
- `helm 3`

## Deployment of Operator

1. Get helm Repo Info

```bash
helm repo add mondoo https://mondoohq.github.io/mondoo-operator
helm repo update
```

2. Deploy the operator using helm:

```bash
helm install mondoo-operator mondoo/mondoo-operator --namespace mondoo-operator-system --create-namespace
```

3. Configure the Mondoo secret:

- Create a new Mondoo service account to report assessments to [Mondoo Platform](https://mondoo.com/docs/platform/service_accounts)
- Store the service account json into a local file `creds.json`
- Store service account as a secret in the mondoo namespace via:

```bash
kubectl create secret generic mondoo-client --namespace mondoo-operator-system --from-file=config=creds.json
```

Once the secret is configure, we configure the operator to define the scan targets:

4. Create `mondoo-config.yaml`

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
mondoo-client   2m44s
```

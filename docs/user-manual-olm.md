# Install Mondoo Operator with olm

The following steps sets up the Mondoo operator using [Operator Lifecycle Manager (OLM)].

## Preconditions

- `kubectl` with admin role
- [`operator-lifecycle-manager`](https://olm.operatorframework.io/) installed in cluster
- [`operator-sdk`](https://sdk.operatorframework.io/docs/installation/) installed locally

## Deployment of Operator using Manifests

1. [Install Operator Lifecycle Manager (OLM)](https://olm.operatorframework.io/docs/getting-started/) in your Kubernetes cluster.
2. Verify that operator-lifecycle-manager is up

```bash
operator-sdk olm status | echo $?
0
INFO[0000] Fetching CRDs for version "v0.20.0"
INFO[0000] Fetching resources for resolved version "v0.20.0"
INFO[0001] Successfully got OLM status for version "v0.20.0"
```

3. Install the Mondoo Operator Bundle

```bash
kubectl create namespace mondoo-operator
operator-sdk run bundle ghcr.io/mondoohq/mondoo-operator-bundle:latest --namespace=mondoo-operator
```

4. Verify that the operator is properly installed

```bash
kubectl get csv -n operators
```

5. Configure the Mondoo Secret:

- Create a new Mondoo service account to report assessments to [Mondoo Platform](https://mondoo.com/docs/platform/service_accounts)
- Store the service account json into a local file `creds.json`
- Store service account as a secret in the mondoo namespace via:

```bash
kubectl create secret generic mondoo-client --namespace mondoo-operator --from-file=config=creds.json
```

6. Create `mondoo-config.yaml`

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

Apply the configuration via:

```bash
kubectl apply -f mondoo-config.yaml
```
## Remove operator 

To remove the operator run: 
```bash
operator-sdk olm uninstall mondoo-operator
```

## FAQ

**I do not see the service running, only the operator?**

First check that the CRD is properly registered with the operator:

```bash
kubectl get crd
NAME                           CREATED AT
mondooauditconfigs.k8s.mondoo.com   2022-01-14T14:07:28Z
```

Then make sure a configuration for the Mondoo Client is deployed:

```bash
kubectl get mondooauditconfigs -A
NAME                  AGE
mondoo-client        2m44s
```

**I see the issue that deployment is marked unschedulable**

For development testing you can resources the allocated resources for the Mondoo Client:

```
spec:
  workloads:
    enable: true
    serviceAccount: mondoo-operator-workload
    resources:
      limits:
        cpu: 500m
        memory: 900Mi
      requests:
        cpu: 100m
        memory: 20Mi
  nodes:
    enable: true
    resources:
      limits:
        cpu: 500m
        memory: 900Mi
      requests:
        cpu: 100m
        memory: 20Mi
  mondooSecretRef: mondoo-client
```

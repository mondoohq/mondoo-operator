# Install Mondoo Operator with olm

The following steps sets up the mondoo operator using kubectl and a manifest file.

## Preconditions:

- `kubectl` with admin role
- [`operator-lifecycle-manager`](https://olm.operatorframework.io/) installed in cluster
- `operator-sdk`

## Deployment of Operator using Manifests

1. Install Operator Lifecycle Manager (OLM)
   `curl -sL https://github.com/operator-framework/operator-lifecycle-manager/releases/download/v0.20.0/install.sh | bash -s v0.20.0`

2. Configure the Mondoo secret:

- Create a new Mondoo service account to report assessments to [Mondoo Platform](https://mondoo.com/docs/platform/service_accounts)
- Store the service account json into a local file `creds.json`
- Store service account as a secret in the mondoo namespace via:

```bash
kubectl create namespace mondoo-operator
kubectl create secret generic mondoo-client --namespace mondoo-operator --from-file=config=creds.json
```

3. Verify that operator-lifecycle-manager is up

```bash
operator-sdk olm status | echo $?
0
INFO[0000] Fetching CRDs for version "v0.20.0"
INFO[0000] Fetching resources for resolved version "v0.20.0"
INFO[0001] Successfully got OLM status for version "v0.20.0"
```

4. Install the Mondoo Operator Bundle

```bash
operator-sdk run bundle ghcr.io/mondoohq/mondoo-operator-bundle:latest --namespace=mondoo-operator
```

5. Verify that the operator is properly installed

```bash
kubectl get csv -n operators
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
kubectl get mondooauditconfigs -A
NAME                  AGE
mondoo-client        2m44s
```

**I see the issue that deploymend is marked unschedulable**

For development testing you can resources the allocated resources for the mondoo client:

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

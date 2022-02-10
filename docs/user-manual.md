# User Setup

The following steps sets up the mondoo operator using kubectl and a manifest file.

## Preconditions:

- kubectl with admin role

## Deployment of Operator using Manifests

```bash
IMG=mondoolabs/mondoo-operator:latest make deploy-yaml
kubectl apply -f mondoo-operator-manifests.yaml 
```

Next we configure the Mondoo secret:

1. Download service account from [Mondooo](https://mondoo.com)
2. Convert json to yaml via:

```bash
yq e -P creds.json > creds.yaml
```

3. Store service account as a secret in the mondoo namespace via:

```bash
kubectl create secret generic mondoo-client --namespace mondoo-operator-system --from-file=config=creds.yaml
```

Once the secret is configure, we configure the operator to define the scan targets:

1. Create `mondoo-config.yaml`

```yaml
apiVersion: k8s.mondoo.com/v1alpha1
kind: MondooAuditConfig
metadata:
  name: mondoo-client
  namespace: mondoo-operator-system
data:
  workloads:
    enable: true
    workloadserviceaccount: mondoo-operator-workload
    replicas: 2
  nodes:
    enable: true
  mondoosecretref: mondoo-client
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
mondooauditconfig-sample   2m44s
```
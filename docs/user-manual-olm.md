# User Setup

The following steps sets up the mondoo operator using kubectl and a manifest file.

## Preconditions:

- kubectl with admin role
- operator-lifecycle-manager installed in cluster
- operator-sdk

## Deployment of Operator using Manifests

1. Configure the Mondoo secret:

 - Download service account from [Mondooo](https://mondoo.com)
 - Convert json to yaml via:

```bash
yq e -P creds.json > creds.yaml
```

 - Store service account as a secret in the mondoo namespace via:

```bash
kubectl create secret generic mondoo-client --namespace mondoo-operator-system --from-file=config=creds.yaml
```
2. Verify that operator-lifecycle-manager is up
```bash
operator-sdk olm status | echo $?
```
3. Run the operator-bundle
```bash
operator-sdk run bundle ghcr.io/mondoolabs/mondoo-operator-bundle:v0.0.1 --namespace=mondoo-operator-system
```

4. Create `mondoo-config.yaml`
```yaml
apiVersion: k8s.mondoo.com/v1alpha1
kind: MondooClient
metadata:
  name: mondoo-client
  namespace: mondoo-operator-system
spec:
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
mondooclients.k8s.mondoo.com   2022-01-14T14:07:28Z
```

Then make sure a configuration for the mondoo client is deployed:

```bash
kubectl get mondooclients
NAME                  AGE
mondooclient-sample   2m44s
```
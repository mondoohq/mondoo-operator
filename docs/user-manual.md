
# User Setup

The following steps sets up the mondoo operator using kubectl and a manifest file.

## Preconditions:

- kubectl with admin role

1. Download service account from mondooo
2. Convert json to yaml via `yq e -P creds.json > creds.yaml`
3. Create namespace using `kubectl create namespace mondoo-operator-system`
4. Store service account as a secret in the mondoo namespace via `kubectl create secret generic mondoo-client --namespace mondoo-operator-system --from-file=config=creds.yaml`

## Deployment of Operator using Manifests

```bash
kubectl apply -f mondoo-operator-manifests.yaml 
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
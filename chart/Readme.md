# Installation Using Helm

The following steps setup a helm release of mondoo-operator on a Kubernetes. In this walk-through, we are going to use [minikube](https://minikube.sigs.k8s.io/docs/).

## Preconditions:

- [Minikube Installation](https://minikube.sigs.k8s.io/docs/start/)
- helm 3.x
- kubectl (or use the one bundled with minikube  bundled with minikube `alias kubectl="minikube kubectl --"`)


## Deployment of Operator

As the first step, we need to make sure the cluster is up and running:

```bash
minikube start
```

Now, we completed the setup for the operator. To start the service, we need to configure the client:

1. Download service account from mondooo and store it in a file called `creds.json`
2. Convert json to yaml via `yq e -P creds.json > creds.yml`
3. Create namespace using `kubectl create namespace mondoo-operator-system`
4. Store service account as a secret in the mondoo namespace via `kubectl create secret generic mondoo-client --namespace mondoo-operator-system --from-file=config=creds.yml`
5. Update SecretName created in step 4 in the mondoo-client CRD.
6. Install the operator release in minikube using ` helm install mondoo-operator ./chart --namespace mondoo-operator-system`

Then apply the configuration:

```bash
kubectl apply -f config/samples/k8s_v1alpha1_mondooclient.yaml
```

Validate that everything is running:

```
kubectl get pods --namespace mondoo-operator-system
NAME                                                 READY   STATUS    RESTARTS   AGE
mondoo-client-cd84f967c-v9h8r                        1/1     Running   0          34m
mondoo-client-wqtq9                                  1/1     Running   0          34m
mondoo-operator-controller-manager-6568cc55c-gf7rb   2/2     Running   0          35m
```

To delete the client configuration, run:

```bash
kubectl delete -f config/samples/k8s_v1alpha1_mondooclient.yaml 
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
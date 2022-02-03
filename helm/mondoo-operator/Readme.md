# Installation Using Helm

The following steps setup a helm release of mondoo-operator on a Kubernetes. In this walk-through, we are going to use [minikube](https://minikube.sigs.k8s.io/docs/).

## Preconditions:

- [Minikube Installation](https://minikube.sigs.k8s.io/docs/start/)
- helm 3.x
- kubectl (or use the one bundled with minikube  bundled with minikube `alias kubectl="minikube kubectl --"`)
- optional: `operator-sdk`

## Deployment of Operator

As the first step, we need to make sure the cluster is up and running:

```bash
minikube start
```

As preparation you need to build the operator container image for deployment in Kubernetes. The easiest way is to use the docker environment included in minikube. This ensures that the latest image is available in the cluster.

```bash
# build the operator image in minikube
eval $(minikube docker-env)
make docker-build
```


Now, we completed the setup for the operator. To start the service, we need to configure the client:

1. Download service account from mondooo
2. Convert json to yaml via `yq e -P creds.json > creds.yml`
3. Create namespace using `kubectl create namespace mondoo-operator-system`
4. Store service account as a secret in the mondoo namespace via `kubectl create secret generic mondoo-client --namespace mondoo-operator-system --from-file=config=creds.yml`
5. Update SecretName created in step 4 in the mondoo-client CRD.
6. Install the operator release in minikube using ` helm install mondoo-operator ./helm/mondoo-operator --namespace mondoo-operator-system`

Then apply the configuration:

```bash
kubectl apply -f config/samples/k8s_v1alpha1_mondooclient.yaml
```

Validate that everything is running:

```
kubectl get pods --namespace mondoo-operator-system
NAME                                                  READY   STATUS    RESTARTS   AGE
mondoo-client-hjt8z                                   1/1     Running   0          16m
mondoo-operator-controller-manager-556c7d4b56-qqsqh   2/2     Running   0          88m
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
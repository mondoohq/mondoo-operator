# Development Setup

The following steps setup a development Kubernetes to test the operator locally using helm. In this walk-through, we are going to use [minikube](https://minikube.sigs.k8s.io/docs/).

## Preconditions:

- Kubernetes Cluster, e.g. [Minikube Installation](https://minikube.sigs.k8s.io/docs/start/)
- `kubectl` (or use the one bundled with minikube bundled with minikube `alias kubectl="minikube kubectl --"`)
- `helm`
- optional: `operator-sdk`

## Deployment of Operator

Optional: As the first step, we need to make sure the cluster is up and running:

```bash
minikube start
```

Next let us deploy the operator application:

```bash
helm install mondoo-operator ./chart --namespace mondoo-operator-system --create-namespace
```

> NOTE: Make sure helm is configured to use minikube

Now, we completed the setup for the operator. To start the service, we need to configure the client:

1. Configure the Mondoo secret:

- Create a new Mondoo service account to report assessments to [Mondoo Platform](https://mondoo.com/docs/platform/service_accounts)
- Store the service account json into a local file `creds.json`
- Store service account as a secret in the mondoo namespace via:

```bash
kubectl create secret generic mondoo-client --namespace mondoo-operator-system --from-file=config=creds.json
```

2. Update SecretName created in step 4 in the mondooauditconfig CRD.

Then apply the configuration:

```bash
kubectl apply -f config/samples/k8s_v1alpha1_mondooauditconfig.yaml
```

Validate that everything is running:

```bash
kubectl get pods --namespace mondoo-operator-system
NAME                                                  READY   STATUS    RESTARTS   AGE
mondoo-client-hjt8z                                   1/1     Running   0          16m
mondoo-operator-controller-manager-556c7d4b56-qqsqh   2/2     Running   0          88m
```

To delete the client configuration, run:

```bash
kubectl delete -f config/samples/k8s_v1alpha1_mondooauditconfig.yaml
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

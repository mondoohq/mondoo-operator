# Development Setup for Operator

The following steps setup a development Kubernetes to test the operator locally. In this walk-through, we are going to use [minikube](https://minikube.sigs.k8s.io/docs/).

## Preconditions:

- [Minikube Installation](https://minikube.sigs.k8s.io/docs/start/)
- kubectl (or use the one bundled with minikube bundled with minikube `alias kubectl="minikube kubectl --"`)
- optional: [`operator-sdk`](https://sdk.operatorframework.io/docs/installation/)

## Run Operator

As the first step, we need to make sure the cluster is up and running:

```bash
minikube start
```

### Local Go Binary

Run the operator:

```bash
# configure crd
make install

# run the operator locally
make run
```

Create `Mondoo Secret` and add the `MondooAuditConfig`:

```bash
kubectl create namespace mondoo-operator
kubectl create secret generic mondoo-client --namespace mondoo-operator --from-file=config=creds.json
kubectl apply -f config/samples/k8s_v1alpha2_mondooauditconfig_minimal.yaml
```

### Docker Build

As preparation you need to build the operator container image for deployment in Kubernetes. This ensures that the latest image is available in the cluster.

```bash
make load-minikube
```

Next let us deploy the operator application:

```
make deploy
# Or if you want to deploy using OLM
make deploy-olm
```

> NOTE: deploy target uses `kubectl` under the cover, therefore make sure kubectl is configured to use minikube
> NOTE: deploy-olm target uses operator-sdk and depends on olm being installed

Now, we completed the setup for the operator. To start the service, we need to configure the client:

1. Create namespace using

```bash
kubectl create namespace mondoo-operator
```

2. Configure the Mondoo secret:

- Create a new Mondoo service account to report assessments to [Mondoo Platform](https://mondoo.com/docs/platform/maintain/access/service_accounts/)
- Store the service account json into a local file `creds.json`
- Store service account as a secret in the mondoo namespace via:

```bash
kubectl create secret generic mondoo-client --namespace mondoo-operator --from-file=config=creds.json
```

3. Update SecretName created in step 4 in the mondoo-client CRD.

Then apply the configuration:

```bash
kubectl apply -f config/samples/k8s_v1alpha2_mondooauditconfig_minimal.yaml
```

Validate that everything is running:

```
kubectl get pods --namespace mondoo-operator
NAME                                                  READY   STATUS    RESTARTS   AGE
mondoo-client-hjt8z                                   1/1     Running   0          16m
mondoo-operator-controller-manager-556c7d4b56-qqsqh   2/2     Running   0          88m
```

To delete the client configuration, run:

```bash
kubectl delete -f config/samples/k8s_v1alpha2_mondooauditconfig_minimal.yaml
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
kubectl get mondooauditconfigs
NAME                  AGE
mondooauditconfig-sample   2m44s
```

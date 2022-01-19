# Deployment in K8S

In Kubernetes, we support running in two ways:

- one-off job - Job runs on any node, not all nodes
- daemon - DaemonSet runs on every node and reports continuously
- deployment - Deployment runs to scan kubeapi server 

## Preparation

1. Download service account from Mondoo's dashboard
2. Convert json to yaml via `yq e -P creds.json`
3. Update the `mondoo-credentials.yaml` with the Mondoo service account

## Deploy the service account

```bash
kubectl apply -f mondoo-credentials.yaml
```

## Deploy individual job

```bash
# deploy the service account (needs to be available before the job starts)
kubectl apply -f ./job/role-binding.yaml -f ./job/role.yaml -f ./job/service-account.yaml
# deploy  job
kubectl apply -f ./job/
# list job
kubectl get jobs
NAME         COMPLETIONS   DURATION   AGE
mondoo-job   0/1           11s        11s
# describe pod
kubectl describe jobs mondoo-job  

# delete job
# alternative: kubectl delete jobs mondoo-job 
kubectl delete -f ./job
```

## Deploy Kube Node Scanner

```bash
# install daemonset
kubectl apply -f ./node-scanner

# delete daemonset
# alternative: kubectl delete daemonsets.apps mondoo-daemonset
kubectl delete -f ./node-scanner
```

## Deploy Kube API Scanner Deployment

```bash
# deploy the service account (needs to be available before the job starts)
kubectl apply -f ./kube-scanner/role-binding.yaml -f ./job/role.yaml -f ./job/service-account.yaml

# install deployment
kubectl apply -f ./kube-scanner

# delete daemonset
# alternative: kubectl delete deploy mondoo-app-scanner
kubectl delete -f ./kube-scanner
```

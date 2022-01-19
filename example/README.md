# Deployment in K8S

In Kubernetes, we support running in two ways:

- one-off job - Job runs on any node, not all nodes
- daemon - DaemonSet runs on every node and reports continuously
- deployment - Deployment runs to scan kubeapi server 

## Preparation

1. Download service account from Mondoo's dashboard
2. Convert json to yaml via `yq e -P creds.json`
3. Update the `mondoo-credentials.yaml` with the Mondoo service account
4. Apply `mondoo-credentials.yaml` secret to the cluster

## Deploy individual job

```bash
# run job
kubectl apply -f job.yaml -f mondoo-inventory.yaml
# list job
kubectl get jobs
NAME         COMPLETIONS   DURATION   AGE
mondoo-job   0/1           11s        11s
# describe pod
kubectl describe jobs mondoo-job  

# delete job
kubectl delete jobs mondoo-job 
```

## Deploy daemon set
> Following operations need to be executed from node-scanner folder
```bash
# install daemonset
kubectl apply -f daemonset.yaml -f mondoo-inventory.yaml
# delete daemonset
kubectl delete daemonsets.apps mondoo-daemonset
```
## Deploy Deployment
> Following operations need to be executed from kube-scanner folder
```bash
# install deployment
kubectl apply -f daemonset.yaml -f mondoo-inventory.yaml
# delete daemonset
kubectl delete deploy mondoo-app-scanner

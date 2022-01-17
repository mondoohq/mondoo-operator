# Deployment in K8S

In Kubernetes, we support running in two ways:

- one-off job - Job runs on any node, not all nodes
- daemon - DaemonSet runs on every node and reports continuously

## Preparation

1. Download service account from mondooo
2. Convert json to yaml via `yq e -P creds.json`
3. Update the `mondoo-credentials.yaml` with the service account 


## Deploy job

```bash
# run job
kubectl apply -f job.yaml -f mondoo-credentials.yaml -f mondoo-inventory.yaml
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

```bash
# install daemonset
kubectl apply -f daemonset.yaml -f mondoo-credentials.yaml -f mondoo-inventory.yaml
# delete daemonset
kubectl delete daemonsets.apps mondoo-daemonset
```
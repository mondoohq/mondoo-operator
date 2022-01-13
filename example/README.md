# Deployment in K8S

In Kubernetes, we support running in two ways:

- one-off job - Job runs on any node, not all nodes
- daemon - DaemonSet runs on every node and reports continuously

## Preparation

1. Download service account from mondooo
2. Convert json to yaml via `yq e -P creds.json`
3. Update the `daemonset-config.yaml` and/or `job-config.yaml` with the service account

## Deploy job

```bash
# run job
kubectl apply -f job.yaml -f job-config.yaml
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
kubectl apply -f daemonset.yaml -f daemonset-config.yaml 
# delete daemonset
kubectl delete daemonsets.apps mondoo-daemonset
```
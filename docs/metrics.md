# Metrics
By default, controller-runtime builds a global prometheus registry and publishes a collection of performance metrics for each controller. These metrics are exposed at /metrics endpoint.

# Configuration
Mondoo-operator metrics can be scraped by using [prometheus-operator](https://github.com/prometheus-operator/prometheus-operator). 
> If prometheus operator has been installed as a part of [kube-prometheus-stack](https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack) this rolebinding already exists.
For a prometheus server to scrape /metrics endpoint, a clusterrolebinding must be created between the service-account of the prometheus server and the clusterrole indicated at 
config/rbac/auth_proxy_client_clusterrole.yaml

A sample clusterolebinding is as follows 
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: prometheus-k8s-rolebinding
  namespace: <operator-namespace>
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: mondoo-operator-metrics-reader
subjects:
  - kind: ServiceAccount
    name: <prometheus-service-account>
    namespace: <prometheus-service-account-namespace>
```
> If servicemonitor for mondoo-operato is not visible in Prometheus, please check if the prometheus server configuration includes the right serviceMonitorSelector
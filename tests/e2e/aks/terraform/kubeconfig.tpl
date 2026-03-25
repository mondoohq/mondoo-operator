apiVersion: v1
kind: Config
clusters:
- cluster:
    certificate-authority-data: ${cluster_ca}
    server: ${cluster_endpoint}
  name: ${cluster_name}
contexts:
- context:
    cluster: ${cluster_name}
    user: ${cluster_name}
  name: ${cluster_name}
current-context: ${cluster_name}
users:
- name: ${cluster_name}
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1beta1
      command: kubelogin
      args:
        - get-token
        - --login
        - azurecli
        - --server-id
        - 6dae42f8-4368-4678-94ff-3960e28e3630

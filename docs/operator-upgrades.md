# Upgrading the operator
The Mondoo operator versions are expressed as `x.y.z`, where `x` is the major version, `y` is the minor version, and `z` is the patch version, following [Semantic Versioning](https://semver.org/) terminology. The operator releases are done in a way that makes sure there are no breaking changes between 2 adjacent minor versions. For example, upgrading the operator from `v0.2.15` to `v0.3.0` is possible without any manual actions. The operator will automatically execute any migration and/or cleanup steps needed.

When upgrading the operator it is important to note that skipping patch versions is possible but skipping minor versions is not. Upgrading from `v0.2.0` directly to `v0.4.0` is possible but can result in the operator not functioning as expected and/or unused operator resource being left behind in the cluster. Performing such an upgrade will require manual actions to ensure the operator is fully functional.

**Never upgrade the operator by simply changing the tag for the Mondoo operator container image!**

## Recommended operator upgrade approach
Follow the steps below to ensure smooth Mondoo operator upgrade procedure:
1. Verify the Mondoo operator version currently running in the cluster:
    ```bash
    kubectl get deployments -n mondoo-operator -o jsonpath='{.items[*].spec.template.spec.containers[0].image}'
    ```
2. Verify the current latest version for the Mondoo operator by clicking [here](https://github.com/mondoohq/mondoo-operator/releases/latest).

Based on whether there is more than 1 minor version difference between the installed version and the current latest, follow the sections below.

### Not more than 1 minor version difference
If there is not more than 1 minor version difference between the installed version and the current latest, simply apply the latest manifest to the cluster:
```bash
kubectl apply -f https://github.com/mondoohq/mondoo-operator/releases/latest/download/mondoo-operator-manifests.yaml
```

### More than 1 minor version difference
If there is more than 1 minor version difference between the installed version and the current latest, the manifest files for each minor version in between the 2 need to be applied step-by-step. For example, if the version installed is `v0.2.0` and the current latest is `v0.4.3` the upgrade will consist of the following steps:

1. Apply the manifest for `v0.3.0`:
    ```bash
    kubectl apply -f https://github.com/mondoohq/mondoo-operator/releases/v0.3.0/download/mondoo-operator-manifests.yaml
    ```
2. Wait until the new version of the operator is running and verify there are no errors in the operator log:
    ```bash
    kubectl logs -n mondoo-operator deployment/mondoo-operator-controller-manager
    ```
    Waiting for the new version of the operator to be ready and checking the logs is essential as directly upgrading to the next version might result in skipping internal upgrade procedures.
3. Apply the manifest for `v0.4.3` (which is the latest):
    ```bash
    kubectl apply -f https://github.com/mondoohq/mondoo-operator/releases/latest/download/mondoo-operator-manifests.yaml
    ```
# Upgrading the Mondoo Operator
The Mondoo Operator version numbers are expressed as `x.y.z`, where `x` is the major version, `y` is the minor version, and `z` is the patch version, following [Semantic Versioning](https://semver.org/) standards. Our release approach ensures there are no breaking changes between two adjacent minor versions. For example, you can upgrade the Mondoo Operator from `v0.2.15` to `v0.3.0` without any manual actions. The Mondoo Operator automatically executes any required migration and/or cleanup steps.

You can skip Mondoo Operator patch and minor versions.

## Version 1.0 and later
As of version 1.0, the Mondoo Operator can be upgraded from major version to major version as long as each major version is visited during the upgrade. For example, if you are on version 1.3.2 and wanted to upgrade to version 3.0.5, you should first upgrade to a 2.y.z release to ensure any migrations that need to take place from 1.x to 2.x are completed before upgrading to 3.x. Jumping from any 1.x to any 2.x is a safe operation.

## Pre 1.0
For pre-1.0 releases, we don't recommend skipping minor versions. For example, if you upgrade from `v0.2.0` directly to `v0.4.0`, the Mondoo Operator may not behave as expected, and you may leave behind unused resources in the cluster. Skipping a minor version may require manual actions to ensure the Mondoo Operator is fully functional.

**NOTE: all upgrade text below is written against 1.0 behavior. If using pre-1.0, then all mentions that follow of checking major versions should be read as checking against minor versions.**

**WARNING: Never try to upgrade the Mondoo Operator by simply changing the tag for the Mondoo Operator container image.**

## Recommended Mondoo Operator upgrade process
Follow these steps for a smooth Mondoo Operator upgrade:
1. Verify the Mondoo Operator version currently running in the cluster:
    ```bash
    kubectl get deployment -n mondoo-operator mondoo-operator-controller-manager -o jsonpath='{.spec.template.spec.containers[0].image}'
    ```
2. Check the latest version of the Mondoo Operator on our [Releases](https://github.com/mondoohq/mondoo-operator/releases/latest) GitHub page.

Based on the version difference between your Mondo Operator and the latest release, follow the steps below.

### If your current Mondoo Operator is no more than one minor version behind  
If there is **not** more than one major version difference between the installed Mondoo Operator and the latest release, apply the latest manifest to the cluster:
```bash
kubectl apply -f https://github.com/mondoohq/mondoo-operator/releases/latest/download/mondoo-operator-manifests.yaml
```

### If your current Mondoo Operator is more than one major version behind
If there **is** more than one major version difference between the installed Mondoo Operator and the latest release, you must apply the manifest files for each major version between the two versions. For example, if the version installed is `v1.2.0` and the latest version is `v3.4.3`, you must install something from the `v2.x` releases. Follow these steps:

1. Apply the manifest for `v2.0.0` (the version you skipped):
    ```bash
    kubectl apply -f https://github.com/mondoohq/mondoo-operator/releases/v2.0.0/download/mondoo-operator-manifests.yaml
    ```
2. Wait until the new version of the Mondoo Operator is running and verify there are no errors in the operator log:
    ```bash
    kubectl logs -n mondoo-operator deployment/mondoo-operator-controller-manager
    ```
    It's essential to check the logs and wait for the new version of the operator to run; directly upgrading to the next version can result in skipped internal upgrade procedures and unexpected behavior.

    Check the `Status.ReconciledByOperatorVersion` field of your `MondooAuditConfig` to be sure the new operator version reconciled every object:
    ```bash
    kubectl get mondooauditconfigs.k8s.mondoo.com -o jsonpath='{range .items[*]}{.status.reconciledByOperatorVersion}{"\n"}{end}' -A | uniq
    ```
    The version of your running Mondoo Operator and the version in the `Status` field have to be the same before you can proceed with the next version update.

3. Apply the manifest for `v3.4.3` (the latest version):
    ```bash
    kubectl apply -f https://github.com/mondoohq/mondoo-operator/releases/latest/download/mondoo-operator-manifests.yaml
    ```
Adjust the steps above to fit your current situation. There may be multiple major release versions between your installed version and the latest release. You must install each major version independently, wait between each update to verify that the version installed properly and the log is error-free.

## Upgrading to Mondoo Operator v0.8.0
In case you are running a Mondoo Operator with a version older than v0.8.0 in your cluster, it is required to perform extra steps before upgrading.

### Helm and kubectl installations
For Helm and kubectl installations before applying the `v0.8.0` manifests run:
```bash
kubectl delete -n mondoo-operator deployments.apps mondoo-operator-controller-manager
```

### OLM installations
For OLM installations first list the subscriptions:
```bash
kubectl get subscription -n mondoo-operator
```

Delete the Mondoo Operator subscription:
```bash
kubectl delete sub -n mondoo-operator mondoo-operator-v0-7-1-sub  
```

Delete the Mondoo Operator cluster service version:
```bash
kubectl delete csv -n mondoo-operator mondoo-operator.v0.7.1
```

After that you can install the latest Mondoo Operator version using the standard OLM installation command.

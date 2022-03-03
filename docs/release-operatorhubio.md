# Release process to operatorhub.io

The following steps sets are necessary to release the operator to operatorhub.io

## Preconditions:

- `operator-sdk`

## Release process

1. Generate bundle 

```bash
make bundle IMG="ghcr.io/mondoohq/mondoo-operator:v0.0.10" VERSION="0.0.10"
```

This target generates a `/bundle` directory with manifests and metadata.  

2. Cross repo PR to https://github.com/k8s-operatorhub/community-operators

fork https://github.com/k8s-operatorhub/community-operators

In the forked repo add the following files from the `/bundle` directory: 

- bundle/manifests/k8s.mondoo.com_mondooauditconfigs.yaml
- bundle/manifests/mondoo-operator.clusterserviceversion.yaml

and update the `operators/mondoo-operator/mondoo-operator.package.yaml` to point to latest version of the operator.

commit and do a cross repo PR.

## FAQ

**I want to run tests locally before commiting**

The following test are the core of the CI that is being used on the `k8s-operatorhub` repo, and they can be run locally:

```bash
bash <(curl -sL https://raw.githubusercontent.com/redhat-openshift-ecosystem/community-operators-pipeline/ci/latest/ci/scripts/opp.sh) kiwi, lemon, orange operators/mondoo-operator/0.0.10
```

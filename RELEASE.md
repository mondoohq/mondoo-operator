# Operator Release

This document describes the release process for the operator

NOTE: Until there is an automated way to keep the webhook image deployed by the mondoo-operator in sync with the mondoo-operator container image, ensure the image referred to in controllers/webhook-manifests.yaml is updated with the "latest" version/release of the operator before making a build/release.

## Helm Chart

Mondoo Operator helm chart has been auto-generated using the [helmify](https://github.com/arttor/helmify) tool. The CI uses [chart-releaser-action](https://github.com/helm/chart-releaser-action) to self host the charts using GitHub pages.

The following steps need to be followed to release Helm chart.

### Steps

1. Update `Chart.yaml` in the `charts/mondoo-operator` repository with the latest appVersion.
2. Update `Chart.yaml` in the `charts/mondoo-operator` repository with the corresponding version.
3. Run `CHART_NAME=charts/mondoo-operator make helm`

### Helm Chart Release Workflow

Helm chart release action is executed on every push to main. It checks each chart in the charts folder, and whenever there's a new chart version, creates a corresponding GitHub release named for the chart version, adds Helm chart artifacts to the release, and creates or updates an index.yaml file with metadata about those releases, which is then hosted on GitHub Pages.

# OLM Bundle for operatorhub.io

The following steps sets are necessary to release the operator to operatorhub.io

## Preconditions:

- `operator-sdk` needs to be installed

## Steps

1. Generate bundle

```bash
make bundle IMG="ghcr.io/mondoohq/mondoo-operator:v0.0.10" VERSION="0.0.10"
```

The make target generates a `/bundle` directory with the required manifests and metadata.

2. Create a PR to https://github.com/k8s-operatorhub/community-operators

- fork https://github.com/k8s-operatorhub/community-operators
- in the forked repo add/update the following files from the `/bundle` directory:
  - `bundle/manifests/k8s.mondoo.com_mondooauditconfigs.yaml`
  - `bundle/manifests/mondoo-operator.clusterserviceversion.yaml`
- update the `operators/mondoo-operator/mondoo-operator.package.yaml` to point to latest version of the operator
- commit and open PR

## FAQ

**I want to run tests locally before committing**

The following tests are the core of the CI that is being used on the `k8s-operatorhub` repo, and can be run locally:

```bash
bash <(curl -sL https://raw.githubusercontent.com/redhat-openshift-ecosystem/community-operators-pipeline/ci/latest/ci/scripts/opp.sh) kiwi, lemon, orange operators/mondoo-operator/0.0.10
```

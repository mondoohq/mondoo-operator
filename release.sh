#!/bin/bash
VERSION=0.0.12
CHART_VERSION=0.1.5
make manifests
yq -i ".appVersion = \"${VERSION}\"" charts/mondoo-operator/Chart.yaml
yq -i ".version = \"${CHART_VERSION}\"" charts/mondoo-operator/Chart.yaml
CHART_NAME=charts/mondoo-operator make helm
make bundle IMG="ghcr.io/mondoohq/mondoo-operator:v${VERSION}" VERSION="${VERSION}"
# TODO: update controllers/webhook-manifests.yaml
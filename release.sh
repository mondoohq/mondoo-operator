#!/bin/bash
# This script updates all the versions to do a release:
# ./release.sh 0.0.13 0.1.6
echo "Version: $1";
VERSION=$1
echo "Chart-Version: $2";
CHART_VERSION=$2

make manifests
yq -i ".appVersion = \"${VERSION}\"" charts/mondoo-operator/Chart.yaml
yq -i ".version = \"${CHART_VERSION}\"" charts/mondoo-operator/Chart.yaml
yq -i ".controllerManager.manager.image.tag = \"v${VERSION}\"" charts/mondoo-operator/values.yaml
CHART_NAME=charts/mondoo-operator make helm
make bundle IMG="ghcr.io/mondoohq/mondoo-operator:v${VERSION}" VERSION="${VERSION}"

package utils

import (
	"github.com/google/go-containerregistry/pkg/name"
	"go.mondoo.com/cnquery/v9/providers/os/resources/discovery/container_registry"
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/assets"
	v1 "k8s.io/api/core/v1"
)

func ExcludeClusterAsset(as []assets.AssetWithScore) []assets.AssetWithScore {
	var newAssets []assets.AssetWithScore
	for _, asset := range as {
		if asset.Asset.AssetType != "k8s.cluster" {
			newAssets = append(newAssets, asset)
		}
	}
	return newAssets
}

func AssetNames(assets []assets.AssetWithScore) []string {
	assetNames := make([]string, 0, len(assets))
	for _, asset := range assets {
		assetNames = append(assetNames, asset.Asset.Name)
	}
	return assetNames
}

func ContainerImages(pods []v1.Pod, auditConfig v1alpha2.MondooAuditConfig) ([]string, error) {
	runningImages := make(map[string]struct{})
	nsFiltering := auditConfig.Spec.Filtering.Namespaces
	for _, p := range pods {
		scan, err := utils.AllowNamespace(p.Namespace, nsFiltering.Include, nsFiltering.Exclude)
		if err != nil {
			return nil, err
		}
		if scan {
			for i := range UniqueImagesForPod(p, nil) {
				runningImages[i] = struct{}{}
			}
		}
	}

	ccresolver := container_registry.NewContainerRegistryResolver()
	images := make([]string, 0, len(runningImages))
	for i := range runningImages {
		ref, err := name.ParseReference(i, name.WeakValidation)
		if err != nil {
			return nil, err
		}

		a, err := ccresolver.GetImage(ref, nil)
		if err != nil {
			return images, err
		}
		images = append(images, a.Name)
	}

	return images, nil
}

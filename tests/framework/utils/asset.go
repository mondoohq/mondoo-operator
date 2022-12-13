package utils

import (
	"github.com/google/go-containerregistry/pkg/name"
	"go.mondoo.com/cnquery/motor/discovery/container_registry"
	"go.mondoo.com/cnquery/motor/discovery/k8s"
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus"
	v1 "k8s.io/api/core/v1"
)

func AssetNames(assets []nexus.AssetWithScore) []string {
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
			for i := range k8s.UniqueImagesForPod(p, nil) {
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
		images = append(images, a.Name)
	}

	return images, nil
}

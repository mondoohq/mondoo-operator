// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package utils

import (
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"go.mondoo.com/cnquery/v11/providers/os/id/containerid"
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/assets"
	nexusK8s "go.mondoo.com/mondoo-operator/tests/framework/nexus/k8s"
	v1 "k8s.io/api/core/v1"
)

func ExcludeNonDetermenisticAssets(as []assets.AssetWithScore) []assets.AssetWithScore {
	var newAssets []assets.AssetWithScore
	for _, asset := range as {
		if asset.AssetType != "k8s.cluster" && asset.AssetType != "k8s.service" {
			newAssets = append(newAssets, asset)
		}
	}
	return newAssets
}

func AssetNames(assets []assets.AssetWithScore) []string {
	assetNames := make([]string, 0, len(assets))
	for _, asset := range assets {
		assetNames = append(assetNames, asset.Name)
	}
	return assetNames
}

func CiCdJobNames(assets []nexusK8s.CiCdJob) []string {
	assetNames := make([]string, 0, len(assets))
	for _, asset := range assets {
		assetNames = append(assetNames, asset.Namespace+"/"+asset.Name)
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

	images := make([]string, 0, len(runningImages))
	for i := range runningImages {
		ref, err := name.ParseReference(i, name.WeakValidation)
		if err != nil {
			return nil, err
		}

		repoName := ref.Context().Name()
		img, err := remote.Image(ref)
		if err != nil {
			return nil, err
		}
		digest, err := img.Digest()
		if err != nil {
			return nil, err
		}
		images = append(images, repoName+"@"+containerid.ShortContainerImageID(digest.String()))
	}

	return images, nil
}

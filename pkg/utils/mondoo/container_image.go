/*
Copyright 2022 Mondoo, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package mondoo

import (
	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

const (
	MondooImage         = "docker.io/mondoo/client"
	MondooTag           = "latest"
	MondooOperatorImage = "ghcr.io/mondoohq/mondoo-operator"
	MondooOperatorTag   = "latest"
)

type getRemoteImageFunc func(ref name.Reference, options ...remote.Option) (*remote.Descriptor, error)

var GetRemoteImage getRemoteImageFunc = remote.Get

func ResolveMondooImage(log logr.Logger, userImageName, userImageTag string, skipResolveImage bool) (string, error) {
	useImage := MondooImage
	useTag := MondooTag
	if userImageName != "" {
		useImage = userImageName
	}
	if userImageTag != "" {
		useTag = userImageTag
	}
	mondooContainer := useImage + ":" + useTag
	imageUrl, err := getImage(skipResolveImage, log, mondooContainer)
	if err != nil {
		log.Error(err, "Failed resolve image")
		return imageUrl, err
	}
	return imageUrl, nil

}

func ResolveMondooOperatorImage(log logr.Logger, userImageName, userImageTag string, skipResolveImage bool) (string, error) {
	useImage := MondooOperatorImage
	useTag := MondooOperatorTag
	if userImageName != "" {
		useImage = userImageName
	}
	if userImageTag != "" {
		useTag = userImageTag
	}
	mondooContainer := useImage + ":" + useTag

	imageUrl, err := getImage(skipResolveImage, log, mondooContainer)
	if err != nil {
		log.Error(err, "Failed to resolve image")
		return imageUrl, err
	}
	return imageUrl, nil
}

func getImage(skipResolveImage bool, log logr.Logger, mondooContainer string) (string, error) {
	if !skipResolveImage {
		imageUrl, err := parseReference(log, mondooContainer)
		if err != nil {
			log.Error(err, "Failed to parse reference")
			return "", err
		}
		return imageUrl, nil
	}
	return mondooContainer, nil
}

func parseReference(log logr.Logger, container string) (string, error) {
	ref, err := name.ParseReference(container)
	if err != nil {
		log.Error(err, "Failed to parse container reference")
		return "", err
	}

	desc, err := GetRemoteImage(ref)
	if err != nil {
		log.Error(err, "Failed to get container reference")
		return "", err
	}
	imgDigest := desc.Digest.String()
	repoName := ref.Context().Name()
	imageUrl := repoName + "@" + imgDigest

	return imageUrl, nil
}

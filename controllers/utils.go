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

package controllers

import (
	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

const (
	mondooImage = "docker.io/mondoolabs/mondoo"
	mondooTag   = "latest"
)

func resolveImage(log logr.Logger, userImageName, userImageTag string) (string, error) {
	useImage := mondooImage
	useTag := mondooTag
	if userImageName != "" {
		useImage = userImageName
	}
	if userImageTag != "" {
		useTag = userImageTag
	}
	mondooContainer := useImage + ":" + useTag

	ref, err := name.ParseReference(mondooContainer)
	if err != nil {
		log.Error(err, "Failed to parse container reference")
		return "", err
	}

	desc, err := remote.Get(ref)
	if err != nil {
		log.Error(err, "Failed to get container reference")
		return "", err
	}
	imgDigest := desc.Digest.String()
	repoName := ref.Context().Name()
	imageUrl := repoName + "@" + imgDigest

	return imageUrl, nil
}

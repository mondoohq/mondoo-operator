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
	"github.com/prometheus/common/log"
)

const (
	mondooImage         = "docker.io/mondoo/client"
	mondooTag           = "latest"
	mondooOperatorImage = "ghcr.io/mondoohq/mondoo-operator"
	mondooOperatorTag   = "latest"
)

func resolveMondooImage(log logr.Logger, userImageName, userImageTag string) (string, error) {
	useImage := mondooImage
	useTag := mondooTag
	if userImageName != "" {
		useImage = userImageName
	}
	if userImageTag != "" {
		useTag = userImageTag
	}
	mondooContainer := useImage + ":" + useTag

	imageUrl, err := parseReference(mondooContainer)

	if err != nil {
		log.Error(err, "Failed to parse reference")
		return "", err
	}

	return imageUrl, nil
}

func resolveMondooOperatorImage(log logr.Logger, userImageName, userImageTag string) (string, error) {
	useImage := mondooOperatorImage
	useTag := mondooOperatorTag
	if userImageName != "" {
		useImage = userImageName
	}
	if userImageTag != "" {
		useTag = userImageTag
	}
	mondooContainer := useImage + ":" + useTag

	imageUrl, err := parseReference(mondooContainer)

	if err != nil {
		log.Error(err, "Failed to parse reference")
		return "", err
	}
	return imageUrl, nil
}

func parseReference(container string) (string, error) {
	ref, err := name.ParseReference(container)
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

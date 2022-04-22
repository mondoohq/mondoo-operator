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
	"fmt"

	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"go.mondoo.com/mondoo-operator/pkg/version"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	MondooClientImage   = "docker.io/mondoo/client"
	MondooClientTag     = "latest"
	MondooOperatorImage = "ghcr.io/mondoohq/mondoo-operator"
)

// On a normal mondoo-operator build, the Version variable will be set at build time to match
// the $VERSION being built (or default to the git SHA). In the event that someone did a manual
// build of mondoo-operator and failed to set the Version variable it will get a default value of
// "latest".
var MondooOperatorTag = version.Version

type getRemoteImageFunc func(ref name.Reference, options ...remote.Option) (*remote.Descriptor, error)

// Used only for testing purposes, so we can test the code without actually querying a remote container registry.
var getRemoteImage getRemoteImageFunc = remote.Get

type ContainerImageResolver interface {
	// MondooClientImage return the Mondoo client image. If skipResolveImage is false, then the image tag is replaced
	// by a digest. If userImage or userTag are empty strings, default values are used.
	MondooClientImage(userImage, userTag string, skipResolveImage bool) (string, error)

	// MondooOperatorImage return the Mondoo operator image. If skipResolveImage is false, then the image tag is replaced
	// by a digest. If userImage or userTag are empty strings, default values are used.
	MondooOperatorImage(userImage, userTag string, skipResolveImage bool) (string, error)
}

type containerImageResolver struct {
	imageCache map[string]string
	logger     logr.Logger
}

func NewContainerImageResolver() ContainerImageResolver {
	return &containerImageResolver{logger: ctrl.Log.WithName("container-image-resolver")}
}

func (c *containerImageResolver) MondooClientImage(userImage, userTag string, skipResolveImage bool) (string, error) {
	image := userImageOrDefault(MondooClientImage, MondooClientTag, userImage, userTag)
	return c.resolveImage(image, skipResolveImage)
}

func (c *containerImageResolver) MondooOperatorImage(userImage, userTag string, skipResolveImage bool) (string, error) {
	image := userImageOrDefault(MondooOperatorImage, MondooOperatorTag, userImage, userTag)
	return c.resolveImage(image, skipResolveImage)
}

func (c *containerImageResolver) resolveImage(image string, skipResolveImage bool) (string, error) {
	if skipResolveImage {
		return image, nil
	}

	// Check if the image already exists in the cache. If yes, then return the cached value.
	imageWithDigest, ok := c.imageCache[image]
	if ok {
		return imageWithDigest, nil
	}

	imageWithDigest, err := c.getImageWithDigest(image)
	if err != nil {
		c.logger.Error(err, "Failed to get image with digest")
		return "", err
	}
	c.imageCache[image] = imageWithDigest // Cache the result for consecutive calls.
	return imageWithDigest, nil
}

func (c *containerImageResolver) getImageWithDigest(image string) (string, error) {
	ref, err := name.ParseReference(image)
	if err != nil {
		c.logger.Error(err, "Failed to parse container reference")
		return "", err
	}

	desc, err := getRemoteImage(ref)
	if err != nil {
		c.logger.Error(err, "Failed to get remote container reference")
		return "", err
	}
	imgDigest := desc.Digest.String()
	repoName := ref.Context().Name()
	imageUrl := repoName + "@" + imgDigest
	return imageUrl, nil
}

func userImageOrDefault(defaultImage, defaultTag, userImage, userTag string) string {
	image := defaultImage
	tag := defaultTag
	if userImage != "" {
		image = userImage
	}
	if userTag != "" {
		tag = userTag
	}
	return fmt.Sprintf("%s:%s", image, tag)
}

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

	ctrl "sigs.k8s.io/controller-runtime"

	"go.mondoo.com/mondoo-operator/pkg/imagecache"
	"go.mondoo.com/mondoo-operator/pkg/version"
)

const (
	MondooClientImage   = "docker.io/mondoo/client"
	MondooClientTag     = "6-rootless"
	MondooOperatorImage = "ghcr.io/mondoohq/mondoo-operator"
)

// On a normal mondoo-operator build, the Version variable will be set at build time to match
// the $VERSION being built (or default to the git SHA). In the event that someone did a manual
// build of mondoo-operator and failed to set the Version variable it will get a default value of
// "latest".
var MondooOperatorTag = version.Version

type ContainerImageResolver interface {
	// MondooClientImage return the Mondoo client image. If skipResolveImage is false, then the image tag is replaced
	// by a digest. If userImage or userTag are empty strings, default values are used.
	MondooClientImage(userImage, userTag string, skipImageResolution bool) (string, error)

	// MondooOperatorImage return the Mondoo operator image. If skipResolveImage is false, then the image tag is replaced
	// by a digest. If userImage or userTag are empty strings, default values are used.
	MondooOperatorImage(userImage, userTag string, skipImageResolution bool) (string, error)
}

type containerImageResolver struct {
	logger logr.Logger

	imageCacher imagecache.ImageCacher
}

func NewContainerImageResolver() ContainerImageResolver {
	return &containerImageResolver{
		logger:      ctrl.Log.WithName("container-image-resolver"),
		imageCacher: imagecache.NewImageCacher(),
	}
}

func (c *containerImageResolver) MondooClientImage(userImage, userTag string, skipImageResolution bool) (string, error) {
	image := userImageOrDefault(MondooClientImage, MondooClientTag, userImage, userTag)
	return c.resolveImage(image, skipImageResolution)
}

func (c *containerImageResolver) MondooOperatorImage(userImage, userTag string, skipImageResolution bool) (string, error) {
	image := userImageOrDefault(MondooOperatorImage, MondooOperatorTag, userImage, userTag)
	return c.resolveImage(image, skipImageResolution)
}

func (c *containerImageResolver) resolveImage(image string, skipImageResolution bool) (string, error) {
	if skipImageResolution {
		return image, nil
	}

	imageWithDigest, err := c.imageCacher.GetImage(image)
	if err != nil {
		c.logger.Error(err, "failed to resolve image plus digest")
		return "", err
	}

	return imageWithDigest, nil
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

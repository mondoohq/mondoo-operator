/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
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
	CnspecImage              = "docker.io/mondoo/cnspec"
	CnspecTag                = "7-rootless"
	OpenShiftMondooClientTag = "7-ubi-rootless"
	MondooOperatorImage      = "ghcr.io/mondoohq/mondoo-operator"
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
	logger              logr.Logger
	resolveForOpenShift bool
	imageCacher         imagecache.ImageCacher
}

func NewContainerImageResolver(isOpenShift bool) ContainerImageResolver {
	return &containerImageResolver{
		logger:              ctrl.Log.WithName("container-image-resolver"),
		imageCacher:         imagecache.NewImageCacher(),
		resolveForOpenShift: isOpenShift,
	}
}

func (c *containerImageResolver) MondooClientImage(userImage, userTag string, skipImageResolution bool) (string, error) {
	defaultTag := CnspecTag

	if c.resolveForOpenShift {
		defaultTag = OpenShiftMondooClientTag
	}

	defaultImage := CnspecImage
	image := userImageOrDefault(defaultImage, defaultTag, userImage, userTag)
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

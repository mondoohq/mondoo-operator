// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package fake

import (
	"context"
	"fmt"

	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
)

type noOpContainerImageResolver struct{}

func NewNoOpContainerImageResolver() mondoo.ContainerImageResolver {
	return &noOpContainerImageResolver{}
}

func (c *noOpContainerImageResolver) CnspecImage(userImage, userTag, userDigest string, skipResolveImage bool) (string, error) {
	image := mondoo.CnspecImage
	if userImage != "" {
		image = userImage
	}
	if userDigest != "" {
		return fmt.Sprintf("%s@%s", image, userDigest), nil
	}
	tag := mondoo.CnspecTag
	if userTag != "" {
		tag = userTag
	}
	return fmt.Sprintf("%s:%s", image, tag), nil
}

func (c *noOpContainerImageResolver) MondooOperatorImage(ctx context.Context, userImage, userTag, userDigest string, skipResolveImage bool) (string, error) {
	image := mondoo.MondooOperatorImage
	if userImage != "" {
		image = userImage
	}
	if userDigest != "" {
		return fmt.Sprintf("%s@%s", image, userDigest), nil
	}
	tag := mondoo.MondooOperatorTag
	if userTag != "" {
		tag = userTag
	}
	return fmt.Sprintf("%s:%s", image, tag), nil
}

// ContainerImageResolverMock is a configurable mock for ContainerImageResolver
type ContainerImageResolverMock struct {
	CnspecImageFunc         func(userImage, userTag, userDigest string, skipResolveImage bool) (string, error)
	MondooOperatorImageFunc func(ctx context.Context, userImage, userTag, userDigest string, skipResolveImage bool) (string, error)
}

func (c *ContainerImageResolverMock) CnspecImage(userImage, userTag, userDigest string, skipResolveImage bool) (string, error) {
	if c.CnspecImageFunc != nil {
		return c.CnspecImageFunc(userImage, userTag, userDigest, skipResolveImage)
	}
	return fmt.Sprintf("%s:%s", mondoo.CnspecImage, mondoo.CnspecTag), nil
}

func (c *ContainerImageResolverMock) MondooOperatorImage(ctx context.Context, userImage, userTag, userDigest string, skipResolveImage bool) (string, error) {
	if c.MondooOperatorImageFunc != nil {
		return c.MondooOperatorImageFunc(ctx, userImage, userTag, userDigest, skipResolveImage)
	}
	return fmt.Sprintf("%s:%s", mondoo.MondooOperatorImage, mondoo.MondooOperatorTag), nil
}

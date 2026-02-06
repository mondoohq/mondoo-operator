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

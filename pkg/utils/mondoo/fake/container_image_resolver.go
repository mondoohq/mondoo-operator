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

func (c *noOpContainerImageResolver) CnspecImage(userImage, userTag string, skipResolveImage bool) (string, error) {
	return fmt.Sprintf("%s:%s", mondoo.CnspecImage, mondoo.CnspecTag), nil
}

func (c *noOpContainerImageResolver) MondooOperatorImage(ctx context.Context, userImage, userTag string, skipResolveImage bool) (string, error) {
	return fmt.Sprintf("%s:%s", mondoo.MondooOperatorImage, mondoo.MondooOperatorTag), nil
}

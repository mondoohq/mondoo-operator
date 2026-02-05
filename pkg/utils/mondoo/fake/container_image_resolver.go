// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package fake

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"

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

func (c *noOpContainerImageResolver) WithImageRegistry(imageRegistry string) mondoo.ContainerImageResolver {
	return c
}

func (c *noOpContainerImageResolver) WithRegistryMirrors(registryMirrors map[string]string) mondoo.ContainerImageResolver {
	return c
}

func (c *noOpContainerImageResolver) WithImagePullSecrets(imagePullSecrets []corev1.LocalObjectReference) mondoo.ContainerImageResolver {
	return c
}

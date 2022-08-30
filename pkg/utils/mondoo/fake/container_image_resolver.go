package fake

/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
*/

import (
	"fmt"

	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
)

type noOpContainerImageResolver struct{}

func NewNoOpContainerImageResolver() mondoo.ContainerImageResolver {
	return &noOpContainerImageResolver{}
}

func (c *noOpContainerImageResolver) MondooClientImage(userImage, userTag string, skipResolveImage bool) (string, error) {
	return fmt.Sprintf("%s:%s", mondoo.MondooClientImage, mondoo.MondooClientTag), nil
}

func (c *noOpContainerImageResolver) MondooOperatorImage(userImage, userTag string, skipResolveImage bool) (string, error) {
	return fmt.Sprintf("%s:%s", mondoo.MondooOperatorImage, mondoo.MondooOperatorTag), nil
}

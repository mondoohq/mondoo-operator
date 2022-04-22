package fake

import (
	"fmt"

	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
)

type noOpContainerImageResolver struct {
}

func NewNoOpContainerImageResolver() mondoo.ContainerImageResolver {
	return &noOpContainerImageResolver{}
}

func (c *noOpContainerImageResolver) MondooClientImage(userImage, userTag string, skipResolveImage bool) (string, error) {
	return fmt.Sprintf("%s:%s", mondoo.MondooClientImage, mondoo.MondooClientTag), nil
}

func (c *noOpContainerImageResolver) MondooOperatorImage(userImage, userTag string, skipResolveImage bool) (string, error) {
	return fmt.Sprintf("%s:%s", mondoo.MondooOperatorImage, mondoo.MondooOperatorTag), nil
}

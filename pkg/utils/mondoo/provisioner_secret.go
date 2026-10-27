// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package mondoo

import (
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
)

// ProvisionerSecretName returns the name of the Secret that keeps the provisioner
// credential derived from the user's credentials Secret credsSecretName.
func ProvisionerSecretName(credsSecretName string) string {
	return k8s.Name(credsSecretName, constants.MondooProvisionerSecretSuffix)
}

// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package utils

import (
	"fmt"

	mondoov1alpha2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
)

// ModeStringToAdmissionMode will take a string and convert it to a known
// admission mode, or sets an error on return if it is an unknown/invalid mode.
func ModeStringToAdmissionMode(mode string) (mondoov1alpha2.AdmissionMode, error) {
	switch mode {
	case string(mondoov1alpha2.Enforcing):
		return mondoov1alpha2.Enforcing, nil
	case string(mondoov1alpha2.Permissive):
		return mondoov1alpha2.Permissive, nil
	default:
		return mondoov1alpha2.Permissive, fmt.Errorf("mode %s is not valid", mode)
	}
}

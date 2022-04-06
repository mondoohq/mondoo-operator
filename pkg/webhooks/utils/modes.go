package utils

import (
	"fmt"

	mondoov1alpha1 "go.mondoo.com/mondoo-operator/api/v1alpha1"
)

// ModeStringToWebhookMode will take a string and convert it to a known
// webhook mode, or sets an error on return if it is an unknown/invalid mode.
func ModeStringToWebhookMode(mode string) (mondoov1alpha1.WebhookMode, error) {
	switch mode {
	case string(mondoov1alpha1.Enforcing):
		return mondoov1alpha1.Enforcing, nil
	case string(mondoov1alpha1.Permissive):
		return mondoov1alpha1.Permissive, nil
	default:
		return mondoov1alpha1.Permissive, fmt.Errorf("mode %s is not valid", mode)
	}
}

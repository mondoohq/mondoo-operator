// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	corev1 "k8s.io/api/core/v1"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
)

// ProxyEnvVars returns environment variables for HTTP/HTTPS proxy configuration.
// It sets both uppercase and lowercase variants for compatibility with different tools.
func ProxyEnvVars(cfg v1alpha2.MondooOperatorConfig) []corev1.EnvVar {
	var envVars []corev1.EnvVar

	if cfg.Spec.HttpProxy != nil {
		envVars = append(envVars,
			corev1.EnvVar{Name: "HTTP_PROXY", Value: *cfg.Spec.HttpProxy},
			corev1.EnvVar{Name: "http_proxy", Value: *cfg.Spec.HttpProxy},
		)
	}
	if cfg.Spec.HttpsProxy != nil {
		envVars = append(envVars,
			corev1.EnvVar{Name: "HTTPS_PROXY", Value: *cfg.Spec.HttpsProxy},
			corev1.EnvVar{Name: "https_proxy", Value: *cfg.Spec.HttpsProxy},
		)
	}
	if cfg.Spec.NoProxy != nil {
		envVars = append(envVars,
			corev1.EnvVar{Name: "NO_PROXY", Value: *cfg.Spec.NoProxy},
			corev1.EnvVar{Name: "no_proxy", Value: *cfg.Spec.NoProxy},
		)
	}

	return envVars
}

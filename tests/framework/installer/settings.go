// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package installer

import (
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"go.mondoo.com/mondoo-operator/tests/framework/utils"
	corev1 "k8s.io/api/core/v1"
)

const MondooNamespace = "mondoo-operator"

type Settings struct {
	SuiteName      string
	Namespace      string
	token          string
	installRelease bool
}

func (s Settings) SetToken(token string) Settings {
	s.token = token
	return s
}

// GetSecret returns the operator secret. If token is set, then mondoo-token secret is returned.
// Otherwise, mondoo-client secret is returned.
func (s Settings) GetSecret(ns string) corev1.Secret {
	secret := corev1.Secret{Type: corev1.SecretTypeOpaque}
	secret.Namespace = ns

	secret.Name = utils.MondooTokenSecret
	secret.Data = map[string][]byte{
		constants.MondooTokenSecretKey: []byte(s.token),
	}

	return secret
}

func NewDefaultSettings() Settings {
	return Settings{Namespace: MondooNamespace, installRelease: false}
}

func NewReleaseSettings() Settings {
	return Settings{Namespace: MondooNamespace, installRelease: true}
}

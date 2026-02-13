// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"k8s.io/utils/ptr"
)

func TestProxyEnvVars_AllSet(t *testing.T) {
	cfg := v1alpha2.MondooOperatorConfig{
		Spec: v1alpha2.MondooOperatorConfigSpec{
			HttpProxy:  ptr.To("http://proxy:8080"),
			HttpsProxy: ptr.To("https://proxy:8443"),
			NoProxy:    ptr.To("localhost,10.0.0.0/8"),
		},
	}

	envVars := ProxyEnvVars(cfg)
	require.Len(t, envVars, 6)

	envMap := map[string]string{}
	for _, e := range envVars {
		envMap[e.Name] = e.Value
	}
	assert.Equal(t, "http://proxy:8080", envMap["HTTP_PROXY"])
	assert.Equal(t, "http://proxy:8080", envMap["http_proxy"])
	assert.Equal(t, "https://proxy:8443", envMap["HTTPS_PROXY"])
	assert.Equal(t, "https://proxy:8443", envMap["https_proxy"])
	assert.Equal(t, "localhost,10.0.0.0/8", envMap["NO_PROXY"])
	assert.Equal(t, "localhost,10.0.0.0/8", envMap["no_proxy"])
}

func TestProxyEnvVars_OnlyHttpProxy(t *testing.T) {
	cfg := v1alpha2.MondooOperatorConfig{
		Spec: v1alpha2.MondooOperatorConfigSpec{
			HttpProxy: ptr.To("http://proxy:8080"),
		},
	}

	envVars := ProxyEnvVars(cfg)
	require.Len(t, envVars, 2)
	assert.Equal(t, "HTTP_PROXY", envVars[0].Name)
	assert.Equal(t, "http_proxy", envVars[1].Name)
}

func TestProxyEnvVars_NoneSet(t *testing.T) {
	cfg := v1alpha2.MondooOperatorConfig{}

	envVars := ProxyEnvVars(cfg)
	assert.Empty(t, envVars)
}

func TestAPIProxyURL_PrefersHttpsProxy(t *testing.T) {
	cfg := v1alpha2.MondooOperatorConfig{
		Spec: v1alpha2.MondooOperatorConfigSpec{
			HttpProxy:  ptr.To("http://proxy:8080"),
			HttpsProxy: ptr.To("https://proxy:8443"),
		},
	}

	result := APIProxyURL(cfg)
	require.NotNil(t, result)
	assert.Equal(t, "https://proxy:8443", *result)
}

func TestAPIProxyURL_FallsBackToHttpProxy(t *testing.T) {
	cfg := v1alpha2.MondooOperatorConfig{
		Spec: v1alpha2.MondooOperatorConfigSpec{
			HttpProxy: ptr.To("http://proxy:8080"),
		},
	}

	result := APIProxyURL(cfg)
	require.NotNil(t, result)
	assert.Equal(t, "http://proxy:8080", *result)
}

func TestAPIProxyURL_NilWhenNoneSet(t *testing.T) {
	cfg := v1alpha2.MondooOperatorConfig{}

	result := APIProxyURL(cfg)
	assert.Nil(t, result)
}

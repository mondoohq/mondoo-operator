// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package controllers

import (
	"testing"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mondoov1alpha2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
)

func TestMetricsEndpoint_HTTP(t *testing.T) {
	sm := &ServiceMonitor{
		Config: &mondoov1alpha2.MondooOperatorConfig{
			Spec: mondoov1alpha2.MondooOperatorConfigSpec{
				Metrics: mondoov1alpha2.Metrics{
					Enable:        true,
					SecureMetrics: false,
				},
			},
		},
	}

	ep := sm.metricsEndpoint()

	assert.Equal(t, "/metrics", ep.Path)
	assert.Equal(t, "metrics", ep.Port)
	require.NotNil(t, ep.Scheme)
	assert.Equal(t, monitoringv1.SchemeHTTP, *ep.Scheme)
	assert.Empty(t, ep.BearerTokenFile) //nolint:staticcheck
	assert.Nil(t, ep.TLSConfig)
}

func TestMetricsEndpoint_HTTPS(t *testing.T) {
	sm := &ServiceMonitor{
		Config: &mondoov1alpha2.MondooOperatorConfig{
			Spec: mondoov1alpha2.MondooOperatorConfigSpec{
				Metrics: mondoov1alpha2.Metrics{
					Enable:        true,
					SecureMetrics: true,
				},
			},
		},
	}

	ep := sm.metricsEndpoint()

	assert.Equal(t, "/metrics", ep.Path)
	assert.Equal(t, "https", ep.Port)
	require.NotNil(t, ep.Scheme)
	assert.Equal(t, monitoringv1.SchemeHTTPS, *ep.Scheme)
	assert.Equal(t, "/var/run/secrets/kubernetes.io/serviceaccount/token", ep.BearerTokenFile) //nolint:staticcheck
	require.NotNil(t, ep.TLSConfig)
	require.NotNil(t, ep.TLSConfig.InsecureSkipVerify)
	assert.True(t, *ep.TLSConfig.InsecureSkipVerify)
}

func TestServiceMonitorForMondoo_HTTP(t *testing.T) {
	config := &mondoov1alpha2.MondooOperatorConfig{
		Spec: mondoov1alpha2.MondooOperatorConfigSpec{
			Metrics: mondoov1alpha2.Metrics{
				Enable:        true,
				SecureMetrics: false,
			},
		},
	}
	sm := &ServiceMonitor{
		Config:          config,
		TargetNamespace: "test-ns",
	}

	monitor := sm.serviceMonitorForMondoo(config)

	require.Len(t, monitor.Spec.Endpoints, 1)
	ep := monitor.Spec.Endpoints[0]
	assert.Equal(t, "metrics", ep.Port)
	require.NotNil(t, ep.Scheme)
	assert.Equal(t, monitoringv1.SchemeHTTP, *ep.Scheme)
}

func TestServiceMonitorForMondoo_HTTPS(t *testing.T) {
	config := &mondoov1alpha2.MondooOperatorConfig{
		Spec: mondoov1alpha2.MondooOperatorConfigSpec{
			Metrics: mondoov1alpha2.Metrics{
				Enable:        true,
				SecureMetrics: true,
			},
		},
	}
	sm := &ServiceMonitor{
		Config:          config,
		TargetNamespace: "test-ns",
	}

	monitor := sm.serviceMonitorForMondoo(config)

	require.Len(t, monitor.Spec.Endpoints, 1)
	ep := monitor.Spec.Endpoints[0]
	assert.Equal(t, "https", ep.Port)
	require.NotNil(t, ep.Scheme)
	assert.Equal(t, monitoringv1.SchemeHTTPS, *ep.Scheme)
	assert.NotEmpty(t, ep.BearerTokenFile) //nolint:staticcheck
	require.NotNil(t, ep.TLSConfig)
}

// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package resource_watcher

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToSingular(t *testing.T) {
	tests := []struct {
		plural   string
		expected string
	}{
		{"pods", "pod"},
		{"deployments", "deployment"},
		{"daemonsets", "daemonset"},
		{"statefulsets", "statefulset"},
		{"replicasets", "replicaset"},
		{"jobs", "job"},
		{"cronjobs", "cronjob"},
		{"services", "service"},
		{"ingresses", "ingress"},
		{"namespaces", "namespace"},
		{"configmaps", "configmap"},
		{"secrets", "secret"},
		{"serviceaccounts", "serviceaccount"},
		// Case insensitivity
		{"Deployments", "deployment"},
		{"INGRESSES", "ingress"},
		// Fallback for unknown types
		{"unknownresources", "unknownresource"},
		{"widgets", "widget"},
	}
	for _, tt := range tests {
		t.Run(tt.plural, func(t *testing.T) {
			assert.Equal(t, tt.expected, ToSingular(tt.plural))
		})
	}
}

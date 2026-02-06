// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s_scan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
)

const testClusterUID = "abcdefg"

func TestInventory_WithAnnotations(t *testing.T) {
	auditConfig := v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "mondoo-client"},
		Spec: v1alpha2.MondooAuditConfigSpec{
			Annotations: map[string]string{
				"env":  "prod",
				"team": "platform",
			},
		},
	}

	inv, err := Inventory("", testClusterUID, auditConfig, v1alpha2.MondooOperatorConfig{})
	assert.NoError(t, err, "unexpected error generating inventory")
	assert.Contains(t, inv, "env")
	assert.Contains(t, inv, "prod")
	assert.Contains(t, inv, "team")
	assert.Contains(t, inv, "platform")
}

func TestExternalClusterInventory_WithAnnotations(t *testing.T) {
	auditConfig := v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "mondoo-client"},
		Spec: v1alpha2.MondooAuditConfigSpec{
			Annotations: map[string]string{
				"env":  "staging",
				"team": "security",
			},
		},
	}

	cluster := v1alpha2.ExternalCluster{
		Name: "remote-cluster",
	}

	inv, err := ExternalClusterInventory("", testClusterUID, cluster, auditConfig, v1alpha2.MondooOperatorConfig{})
	assert.NoError(t, err, "unexpected error generating inventory")
	assert.Contains(t, inv, "env")
	assert.Contains(t, inv, "staging")
	assert.Contains(t, inv, "team")
	assert.Contains(t, inv, "security")
}

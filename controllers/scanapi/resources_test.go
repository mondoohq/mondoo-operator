// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package scanapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	testMondooAuditConfigName = "mondoo-config"
	testNamespace             = "mondoo-operator"
)

func TestResources_GOMEMLIMIT(t *testing.T) {
	tests := []struct {
		name               string
		mondooauditconfig  func() *v1alpha2.MondooAuditConfig
		expectedGoMemLimit string
	}{
		{
			name: "resources should match default",
			mondooauditconfig: func() *v1alpha2.MondooAuditConfig {
				return testMondooAuditConfig()
			},
			expectedGoMemLimit: "405000000",
		},
		{
			name: "resources should match spec",
			mondooauditconfig: func() *v1alpha2.MondooAuditConfig {
				mac := testMondooAuditConfig()
				mac.Spec.Scanner.Resources = corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("100Mi"),
					},
				}
				return mac
			},
			expectedGoMemLimit: "94371840",
		},
		{
			name: "resources should match off",
			mondooauditconfig: func() *v1alpha2.MondooAuditConfig {
				mac := testMondooAuditConfig()
				mac.Spec.Scanner.Resources = corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("100m"),
					},
				}
				return mac
			},
			expectedGoMemLimit: "off",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mac := *test.mondooauditconfig()
			dep := ScanApiDeployment(testNamespace, "test123", mac, v1alpha2.MondooOperatorConfig{}, "", false)
			goMemLimitEnv := corev1.EnvVar{}
			for _, env := range dep.Spec.Template.Spec.Containers[0].Env {
				if env.Name == "GOMEMLIMIT" {
					goMemLimitEnv = env
					break
				}
			}
			assert.Equal(t, test.expectedGoMemLimit, goMemLimitEnv.Value)
		})
	}
}

func testMondooAuditConfig() *v1alpha2.MondooAuditConfig {
	return &v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testMondooAuditConfigName,
			Namespace: testNamespace,
		},
	}
}

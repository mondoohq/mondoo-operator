// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package nodes

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/tests/framework/utils"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	testMondooAuditConfigName = "mondoo-config"
	testClusterUID            = "abcdefg"
)

func TestCronJobName(t *testing.T) {
	prefix := "mondoo-client"
	tests := []struct {
		name string
		data func() (suffix, expected string)
	}{
		{
			name: "should be prefix+base+suffix when shorter than 52 chars",
			data: func() (suffix, expected string) {
				base := fmt.Sprintf("%s%s", prefix, CronJobNameBase)
				suffix = utils.RandString(k8s.ResourceNameMaxLength - len(base))
				return suffix, fmt.Sprintf("%s%s", base, suffix)
			},
		},
		{
			name: "should be prefix+base+hash when longer than 52 chars",
			data: func() (suffix, expected string) {
				base := fmt.Sprintf("%s%s", prefix, CronJobNameBase)
				suffix = utils.RandString(53 - len(base))

				hash := fmt.Sprintf("%x", sha256.Sum256([]byte(suffix)))
				return suffix, fmt.Sprintf("%s%s", base, hash[:k8s.ResourceNameMaxLength-len(base)])
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			suffix, expected := test.data()
			assert.Equal(t, expected, CronJobName(prefix, suffix))
		})
	}
}

func TestGarbageCollectCronJobName(t *testing.T) {
	prefix := "mondoo-client"

	assert.Equal(t, fmt.Sprintf("%s%s", prefix, GarbageCollectCronJobNameBase), GarbageCollectCronJobName(prefix))
}

func TestConfigMapName(t *testing.T) {
	prefix := "mondoo-client"
	tests := []struct {
		name string
		data func() (suffix, expected string)
	}{
		{
			name: "should be prefix+base+suffix when shorter than 52 chars",
			data: func() (suffix, expected string) {
				base := fmt.Sprintf("%s%s", prefix, InventoryConfigMapBase)
				suffix = utils.RandString(k8s.ResourceNameMaxLength - len(base))
				return suffix, fmt.Sprintf("%s%s", base, suffix)
			},
		},
		{
			name: "should be prefix+base+hash when longer than 52 chars",
			data: func() (suffix, expected string) {
				base := fmt.Sprintf("%s%s", prefix, InventoryConfigMapBase)
				suffix = utils.RandString(53 - len(base))

				hash := fmt.Sprintf("%x", sha256.Sum256([]byte(suffix)))
				return suffix, fmt.Sprintf("%s%s", base, hash[:k8s.ResourceNameMaxLength-len(base)])
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			suffix, expected := test.data()
			assert.Equal(t, expected, ConfigMapName(prefix, suffix))
		})
	}
}

func TestResources(t *testing.T) {
	tests := []struct {
		name              string
		mondooauditconfig func() *v1alpha2.MondooAuditConfig
		expectedResources corev1.ResourceRequirements
	}{
		{
			name: "resources should match default",
			mondooauditconfig: func() *v1alpha2.MondooAuditConfig {
				return testMondooAuditConfig()
			},
			expectedResources: k8s.DefaultNodeScanningResources,
		},
		{
			name: "resources should match spec",
			mondooauditconfig: func() *v1alpha2.MondooAuditConfig {
				mac := testMondooAuditConfig()
				mac.Spec.Nodes.Resources = corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("100m"),
						corev1.ResourceCPU:    resource.MustParse("100m"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("100m"),
						corev1.ResourceCPU:    resource.MustParse("100m"),
					},
				}
				return mac
			},
			expectedResources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("100m"),
					corev1.ResourceCPU:    resource.MustParse("100m"),
				},

				Requests: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("100m"),
					corev1.ResourceCPU:    resource.MustParse("100m"),
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testNode := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-name",
				},
			}
			mac := *test.mondooauditconfig()
			cj := &batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Name: "name", Namespace: mac.Namespace}}
			UpdateCronJob(cj, "test123", *testNode, &mac, false, v1alpha2.MondooOperatorConfig{})
			assert.Equal(t, test.expectedResources, cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Resources)
		})
	}
}

func TestCronJob_PrivilegedOpenshift(t *testing.T) {
	testNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-name",
		},
	}
	mac := testMondooAuditConfig()
	cj := &batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Name: "name", Namespace: mac.Namespace}}
	UpdateCronJob(cj, "test123", *testNode, mac, true, v1alpha2.MondooOperatorConfig{})
	assert.True(t, *cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].SecurityContext.Privileged)
	assert.True(t, *cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].SecurityContext.AllowPrivilegeEscalation)
}

func TestCronJob_Privileged(t *testing.T) {
	testNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-name",
		},
	}
	mac := testMondooAuditConfig()
	cj := &batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Name: "name", Namespace: mac.Namespace}}
	UpdateCronJob(cj, "test123", *testNode, mac, false, v1alpha2.MondooOperatorConfig{})
	assert.False(t, *cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].SecurityContext.Privileged)
	assert.False(t, *cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].SecurityContext.AllowPrivilegeEscalation)
}

func TestInventory(t *testing.T) {
	randName := utils.RandString(10)
	auditConfig := v1alpha2.MondooAuditConfig{ObjectMeta: metav1.ObjectMeta{Name: "mondoo-client"}}

	inventory, err := Inventory(corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: randName}}, "", testClusterUID, auditConfig)
	assert.NoError(t, err, "unexpected error generating inventory")
	assert.Contains(t, inventory, randName)
	assert.NotContains(t, inventory, constants.MondooAssetsIntegrationLabel)

	const integrationMRN = "//test-MRN"
	inventory, err = Inventory(corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: randName}}, integrationMRN, testClusterUID, auditConfig)
	assert.NoError(t, err, "unexpected error generating inventory")
	assert.Contains(t, inventory, randName)
	assert.Contains(t, inventory, constants.MondooAssetsIntegrationLabel)
	assert.Contains(t, inventory, integrationMRN)
}

func testMondooAuditConfig() *v1alpha2.MondooAuditConfig {
	return &v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testMondooAuditConfigName,
			Namespace: testNamespace,
		},
	}
}

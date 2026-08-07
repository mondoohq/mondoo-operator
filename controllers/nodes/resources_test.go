// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package nodes

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/tests/framework/utils"
	"go.mondoo.com/mql/v13/providers-sdk/v1/inventory"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
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
			testNode := corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-name",
				},
			}
			mac := test.mondooauditconfig()
			cj := CronJob("test123", testNode, mac, false, v1alpha2.MondooOperatorConfig{})
			assert.Equal(t, test.expectedResources, cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Resources)
		})
	}
}

func TestCronJob_HasReportTypeNone(t *testing.T) {
	m := testMondooAuditConfig()
	node := corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "test-node"}}
	cfg := v1alpha2.MondooOperatorConfig{}

	cj := CronJob("test-image:latest", node, m, false, cfg)
	container := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0]

	cmd := strings.Join(container.Command, " ")
	assert.Contains(t, cmd, "--report-type none")
}

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
			expectedGoMemLimit: "225000000",
		},
		{
			name: "resources should match spec",
			mondooauditconfig: func() *v1alpha2.MondooAuditConfig {
				mac := testMondooAuditConfig()
				mac.Spec.Nodes.Resources = corev1.ResourceRequirements{
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
				mac.Spec.Nodes.Resources = corev1.ResourceRequirements{
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
			testNode := corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-name",
				},
			}
			mac := test.mondooauditconfig()
			cj := CronJob("test123", testNode, mac, false, v1alpha2.MondooOperatorConfig{})
			goMemLimitEnv := corev1.EnvVar{}
			for _, env := range cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Env {
				if env.Name == "GOMEMLIMIT" {
					goMemLimitEnv = env
					break
				}
			}
			assert.Equal(t, test.expectedGoMemLimit, goMemLimitEnv.Value)
		})
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mac := *test.mondooauditconfig()
			ds := DaemonSet(mac, false, "test123", v1alpha2.MondooOperatorConfig{}, nil)
			goMemLimitEnv := corev1.EnvVar{}
			for _, env := range ds.Spec.Template.Spec.Containers[0].Env {
				if env.Name == "GOMEMLIMIT" {
					goMemLimitEnv = env
					break
				}
			}
			assert.Equal(t, test.expectedGoMemLimit, goMemLimitEnv.Value)
		})
	}
}

func TestCronJob_PrivilegedOpenshift(t *testing.T) {
	testNode := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-name",
		},
	}
	mac := testMondooAuditConfig()
	cj := CronJob("test123", testNode, mac, true, v1alpha2.MondooOperatorConfig{})
	sc := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].SecurityContext
	assert.True(t, *sc.Privileged)
	assert.True(t, *sc.AllowPrivilegeEscalation)
	assert.Empty(t, sc.Capabilities.Add, "OpenShift runs privileged, no extra capabilities needed")
}

func TestCronJob_Privileged(t *testing.T) {
	testNode := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-name",
		},
	}
	mac := testMondooAuditConfig()
	cj := CronJob("test123", testNode, mac, false, v1alpha2.MondooOperatorConfig{})
	sc := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].SecurityContext
	assert.False(t, *sc.Privileged)
	assert.False(t, *sc.AllowPrivilegeEscalation)
	assert.Contains(t, sc.Capabilities.Add, corev1.Capability("DAC_READ_SEARCH"))
	assert.Contains(t, sc.Capabilities.Add, corev1.Capability("SYS_PTRACE"))
	assert.Contains(t, sc.Capabilities.Drop, corev1.Capability("ALL"))
}

func TestCronJob_JobOverrides(t *testing.T) {
	testNode := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-name",
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{{Key: "node-taint", Value: "value", Effect: corev1.TaintEffectNoSchedule}},
		},
	}
	mac := testMondooAuditConfig()
	mac.Spec.Nodes.JobOverrides = v1alpha2.JobOverrides{
		TTLSecondsAfterFinished: ptr.To(int32(300)),
		Labels:                  map[string]string{"team": "security"},
		Annotations:             map[string]string{"karpenter.sh/do-not-disrupt": "true"},
		NodeSelector:            map[string]string{"workload-type": "mondoo-scan"},
		Tolerations: []corev1.Toleration{
			{Key: "workload-type", Operator: corev1.TolerationOpEqual, Value: "mondoo-scan", Effect: corev1.TaintEffectNoSchedule},
		},
	}

	cj := CronJob("test123", testNode, mac, false, v1alpha2.MondooOperatorConfig{})
	assert.Equal(t, ptr.To(int32(300)), cj.Spec.JobTemplate.Spec.TTLSecondsAfterFinished)

	// User labels are applied
	assert.Equal(t, "security", cj.Spec.JobTemplate.Labels["team"])
	assert.Equal(t, "security", cj.Spec.JobTemplate.Spec.Template.Labels["team"])

	// User annotations are merged; operator-managed annotations are kept
	assert.Equal(t, "true", cj.Spec.JobTemplate.Annotations["karpenter.sh/do-not-disrupt"])
	assert.Equal(t, ignoreAnnotationValue, cj.Spec.JobTemplate.Annotations[ignoreQueryAnnotationPrefix+"mondoo-kubernetes-security-job-runasnonroot"])
	assert.Equal(t, "true", cj.Spec.JobTemplate.Spec.Template.Annotations["karpenter.sh/do-not-disrupt"])
	assert.Equal(t, ignoreAnnotationValue, cj.Spec.JobTemplate.Spec.Template.Annotations[ignoreQueryAnnotationPrefix+"mondoo-kubernetes-security-pod-runasnonroot"])

	// Node scan pods are pinned to a node, so the nodeSelector must not be applied
	assert.Nil(t, cj.Spec.JobTemplate.Spec.Template.Spec.NodeSelector)

	// User tolerations are appended to the taint-derived ones
	tolerations := cj.Spec.JobTemplate.Spec.Template.Spec.Tolerations
	require.Len(t, tolerations, 2)
	assert.Equal(t, "node-taint", tolerations[0].Key)
	assert.Equal(t, mac.Spec.Nodes.JobOverrides.Tolerations[0], tolerations[1])
}

func TestCronJob_GlobalJobOverrides(t *testing.T) {
	testNode := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node-name"},
	}
	mac := testMondooAuditConfig()
	mac.Spec.JobOverrides = v1alpha2.JobOverrides{
		TTLSecondsAfterFinished: ptr.To(int32(3600)),
		Labels:                  map[string]string{"team": "security", "env": "prod"},
	}
	mac.Spec.Nodes.JobOverrides = v1alpha2.JobOverrides{
		TTLSecondsAfterFinished: ptr.To(int32(300)),
		Labels:                  map[string]string{"team": "platform"},
	}

	cj := CronJob("test123", testNode, mac, false, v1alpha2.MondooOperatorConfig{})
	assert.Equal(t, ptr.To(int32(300)), cj.Spec.JobTemplate.Spec.TTLSecondsAfterFinished)
	assert.Equal(t, "platform", cj.Spec.JobTemplate.Labels["team"])
	assert.Equal(t, "prod", cj.Spec.JobTemplate.Labels["env"])
}

func TestDaemonSet_Capabilities(t *testing.T) {
	mac := testMondooAuditConfig()

	t.Run("non-OpenShift adds proc capabilities", func(t *testing.T) {
		ds := DaemonSet(*mac, false, "test123", v1alpha2.MondooOperatorConfig{}, nil)
		sc := ds.Spec.Template.Spec.Containers[0].SecurityContext
		assert.False(t, *sc.Privileged)
		assert.Contains(t, sc.Capabilities.Add, corev1.Capability("DAC_READ_SEARCH"))
		assert.Contains(t, sc.Capabilities.Add, corev1.Capability("SYS_PTRACE"))
		assert.Contains(t, sc.Capabilities.Drop, corev1.Capability("ALL"))
	})

	t.Run("OpenShift runs privileged without extra capabilities", func(t *testing.T) {
		ds := DaemonSet(*mac, true, "test123", v1alpha2.MondooOperatorConfig{}, nil)
		sc := ds.Spec.Template.Spec.Containers[0].SecurityContext
		assert.True(t, *sc.Privileged)
		assert.Empty(t, sc.Capabilities.Add)
	})
}

func TestInventory(t *testing.T) {
	auditConfig := v1alpha2.MondooAuditConfig{ObjectMeta: metav1.ObjectMeta{Name: "mondoo-client"}}

	inventory, err := Inventory("", testClusterUID, auditConfig)
	assert.NoError(t, err, "unexpected error generating inventory")
	assert.NotContains(t, inventory, constants.MondooAssetsIntegrationLabel)

	const integrationMRN = "//test-MRN"
	inventory, err = Inventory(integrationMRN, testClusterUID, auditConfig)
	assert.NoError(t, err, "unexpected error generating inventory")
	assert.Contains(t, inventory, constants.MondooAssetsIntegrationLabel)
	assert.Contains(t, inventory, integrationMRN)
}

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

	invStr, err := Inventory("", testClusterUID, auditConfig)
	require.NoError(t, err, "unexpected error generating inventory")

	var inv inventory.Inventory
	require.NoError(t, yaml.Unmarshal([]byte(invStr), &inv))
	require.NotEmpty(t, inv.Spec.Assets, "expected at least one asset")

	for _, asset := range inv.Spec.Assets {
		assert.Equal(t, "prod", asset.Annotations["env"], "asset %s missing env annotation", asset.Name)
		assert.Equal(t, "platform", asset.Annotations["team"], "asset %s missing team annotation", asset.Name)
	}
}

func TestCronJob_WithProxy(t *testing.T) {
	testNode := corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "test-node-name"}}
	mac := testMondooAuditConfig()
	cfg := v1alpha2.MondooOperatorConfig{
		Spec: v1alpha2.MondooOperatorConfigSpec{
			HttpProxy:  ptr.To("http://proxy:8080"),
			HttpsProxy: ptr.To("https://proxy:8443"),
		},
	}

	cj := CronJob("test123", testNode, mac, false, cfg)
	container := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0]

	cmdStr := strings.Join(container.Command, " ")
	assert.Contains(t, cmdStr, "--api-proxy")
	assert.Contains(t, cmdStr, "https://proxy:8443")

	envMap := envToMap(container.Env)
	assert.Equal(t, "http://proxy:8080", envMap["HTTP_PROXY"])
	assert.Equal(t, "https://proxy:8443", envMap["HTTPS_PROXY"])
}

func TestCronJob_SkipProxyForCnspec(t *testing.T) {
	testNode := corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "test-node-name"}}
	mac := testMondooAuditConfig()
	cfg := v1alpha2.MondooOperatorConfig{
		Spec: v1alpha2.MondooOperatorConfigSpec{
			HttpProxy:          ptr.To("http://proxy:8080"),
			SkipProxyForCnspec: true,
		},
	}

	cj := CronJob("test123", testNode, mac, false, cfg)
	container := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0]

	cmdStr := strings.Join(container.Command, " ")
	assert.NotContains(t, cmdStr, "--api-proxy")

	envMap := envToMap(container.Env)
	_, hasHTTPProxy := envMap["HTTP_PROXY"]
	assert.False(t, hasHTTPProxy, "HTTP_PROXY should not be set when SkipProxyForCnspec is true")
}

func TestCronJob_WithImagePullSecrets(t *testing.T) {
	testNode := corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "test-node-name"}}
	mac := testMondooAuditConfig()
	cfg := v1alpha2.MondooOperatorConfig{
		Spec: v1alpha2.MondooOperatorConfigSpec{
			ImagePullSecrets: []corev1.LocalObjectReference{
				{Name: "my-registry-secret"},
			},
		},
	}

	cj := CronJob("test123", testNode, mac, false, cfg)
	secrets := cj.Spec.JobTemplate.Spec.Template.Spec.ImagePullSecrets
	require.Len(t, secrets, 1)
	assert.Equal(t, "my-registry-secret", secrets[0].Name)
}

func TestDaemonSet_WithProxy(t *testing.T) {
	mac := *testMondooAuditConfig()
	cfg := v1alpha2.MondooOperatorConfig{
		Spec: v1alpha2.MondooOperatorConfigSpec{
			HttpProxy:  ptr.To("http://proxy:8080"),
			HttpsProxy: ptr.To("https://proxy:8443"),
		},
	}

	ds := DaemonSet(mac, false, "test123", cfg, nil)
	container := ds.Spec.Template.Spec.Containers[0]

	cmdStr := strings.Join(container.Command, " ")
	assert.Contains(t, cmdStr, "--api-proxy")
	assert.Contains(t, cmdStr, "https://proxy:8443")

	envMap := envToMap(container.Env)
	assert.Equal(t, "http://proxy:8080", envMap["HTTP_PROXY"])
	assert.Equal(t, "https://proxy:8443", envMap["HTTPS_PROXY"])
}

func TestDaemonSet_SkipProxyForCnspec(t *testing.T) {
	mac := *testMondooAuditConfig()
	cfg := v1alpha2.MondooOperatorConfig{
		Spec: v1alpha2.MondooOperatorConfigSpec{
			HttpProxy:          ptr.To("http://proxy:8080"),
			SkipProxyForCnspec: true,
		},
	}

	ds := DaemonSet(mac, false, "test123", cfg, nil)
	container := ds.Spec.Template.Spec.Containers[0]

	cmdStr := strings.Join(container.Command, " ")
	assert.NotContains(t, cmdStr, "--api-proxy")

	envMap := envToMap(container.Env)
	_, hasHTTPProxy := envMap["HTTP_PROXY"]
	assert.False(t, hasHTTPProxy, "HTTP_PROXY should not be set when SkipProxyForCnspec is true")
}

func TestDaemonSet_WithImagePullSecrets(t *testing.T) {
	mac := *testMondooAuditConfig()
	cfg := v1alpha2.MondooOperatorConfig{
		Spec: v1alpha2.MondooOperatorConfigSpec{
			ImagePullSecrets: []corev1.LocalObjectReference{
				{Name: "my-registry-secret"},
			},
		},
	}

	ds := DaemonSet(mac, false, "test123", cfg, nil)
	secrets := ds.Spec.Template.Spec.ImagePullSecrets
	require.Len(t, secrets, 1)
	assert.Equal(t, "my-registry-secret", secrets[0].Name)
}

func TestDaemonSet_WithJobOverrides(t *testing.T) {
	mac := *testMondooAuditConfig()
	mac.Spec.JobOverrides = v1alpha2.JobOverrides{
		NodeSelector: map[string]string{"workload-type": "mondoo-scan"},
		Tolerations: []corev1.Toleration{
			{Key: "dedicated", Operator: corev1.TolerationOpEqual, Value: "mondoo", Effect: corev1.TaintEffectNoSchedule},
		},
		Labels:      map[string]string{"team": "security"},
		Annotations: map[string]string{"karpenter.sh/do-not-disrupt": "true"},
	}

	existingTolerations := []corev1.Toleration{
		{Key: "node.kubernetes.io/not-ready", Operator: corev1.TolerationOpExists},
	}

	ds := DaemonSet(mac, false, "test123", v1alpha2.MondooOperatorConfig{}, existingTolerations)

	assert.Equal(t, map[string]string{"workload-type": "mondoo-scan"}, ds.Spec.Template.Spec.NodeSelector)
	assert.Len(t, ds.Spec.Template.Spec.Tolerations, 2)
	assert.Equal(t, "security", ds.Spec.Template.Labels["team"])
	assert.Equal(t, "true", ds.Spec.Template.Annotations["karpenter.sh/do-not-disrupt"])
	// Operator-managed labels must survive
	assert.Equal(t, "mondoo", ds.Spec.Template.Labels["app"])
}

func TestDaemonSet_PerTypeJobOverridesOverrideGlobal(t *testing.T) {
	mac := *testMondooAuditConfig()
	mac.Spec.JobOverrides = v1alpha2.JobOverrides{
		NodeSelector: map[string]string{"workload-type": "global"},
	}
	mac.Spec.Nodes.JobOverrides = v1alpha2.JobOverrides{
		NodeSelector: map[string]string{"workload-type": "node-scan"},
	}

	ds := DaemonSet(mac, false, "test123", v1alpha2.MondooOperatorConfig{}, nil)

	assert.Equal(t, map[string]string{"workload-type": "node-scan"}, ds.Spec.Template.Spec.NodeSelector)
}

// envToMap converts a slice of EnvVar to a map for easy lookup.
func envToMap(envVars []corev1.EnvVar) map[string]string {
	m := make(map[string]string, len(envVars))
	for _, e := range envVars {
		m[e.Name] = e.Value
	}
	return m
}

func testMondooAuditConfig() *v1alpha2.MondooAuditConfig {
	return &v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testMondooAuditConfigName,
			Namespace: testNamespace,
		},
	}
}

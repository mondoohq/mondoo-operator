// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package nodes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func profileResourceRequirements(memory string) corev1.ResourceRequirements {
	return corev1.ResourceRequirements{
		Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse(memory)},
		Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("100M")},
	}
}

func nodeWithLabels(name string, nodeLabels map[string]string) corev1.Node {
	return corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: nodeLabels}}
}

func TestProfileForNode(t *testing.T) {
	small := v1alpha2.NodeScanProfile{
		Name:         "small",
		NodeSelector: map[string]string{"node.kubernetes.io/instance-type": "t3.small"},
		Resources:    profileResourceRequirements("200M"),
	}
	large := v1alpha2.NodeScanProfile{
		Name:         "large",
		NodeSelector: map[string]string{"node.kubernetes.io/instance-type": "m5.4xlarge"},
		Resources:    profileResourceRequirements("2G"),
	}
	catchAll := v1alpha2.NodeScanProfile{
		Name:      "catch-all",
		Resources: profileResourceRequirements("500M"),
	}

	tests := []struct {
		name     string
		profiles []v1alpha2.NodeScanProfile
		node     corev1.Node
		expected string
	}{
		{
			name:     "no profiles matches nothing",
			profiles: nil,
			node:     nodeWithLabels("node-a", map[string]string{"node.kubernetes.io/instance-type": "t3.small"}),
			expected: "",
		},
		{
			name:     "selector matches the node labels",
			profiles: []v1alpha2.NodeScanProfile{small, large},
			node:     nodeWithLabels("node-a", map[string]string{"node.kubernetes.io/instance-type": "m5.4xlarge"}),
			expected: "large",
		},
		{
			name:     "first match wins",
			profiles: []v1alpha2.NodeScanProfile{catchAll, large},
			node:     nodeWithLabels("node-a", map[string]string{"node.kubernetes.io/instance-type": "m5.4xlarge"}),
			expected: "catch-all",
		},
		{
			name:     "an empty selector matches every node",
			profiles: []v1alpha2.NodeScanProfile{small, catchAll},
			node:     nodeWithLabels("node-a", map[string]string{"kubernetes.io/os": "linux"}),
			expected: "catch-all",
		},
		{
			name:     "a node without a match returns nil",
			profiles: []v1alpha2.NodeScanProfile{small, large},
			node:     nodeWithLabels("node-a", map[string]string{"kubernetes.io/os": "linux"}),
			expected: "",
		},
		{
			name:     "a node without labels returns nil",
			profiles: []v1alpha2.NodeScanProfile{small, large},
			node:     nodeWithLabels("node-a", nil),
			expected: "",
		},
		{
			name:     "all selector labels must match",
			profiles: []v1alpha2.NodeScanProfile{{Name: "both", NodeSelector: map[string]string{"a": "1", "b": "2"}}},
			node:     nodeWithLabels("node-a", map[string]string{"a": "1"}),
			expected: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			profile := ProfileForNode(v1alpha2.Nodes{Profiles: test.profiles}, test.node)
			if test.expected == "" {
				assert.Nil(t, profile)
				return
			}
			require.NotNil(t, profile)
			assert.Equal(t, test.expected, profile.Name)
		})
	}
}

func TestProfileResources(t *testing.T) {
	nodeDefaults := v1alpha2.Nodes{Resources: profileResourceRequirements("400M")}

	t.Run("no profile falls back to spec.nodes.resources", func(t *testing.T) {
		got := ProfileResources(nodeDefaults, nil)
		assert.Equal(t, nodeDefaults.Resources, got)
	})

	t.Run("a profile without resources falls back to spec.nodes.resources", func(t *testing.T) {
		got := ProfileResources(nodeDefaults, &v1alpha2.NodeScanProfile{Name: "small"})
		assert.Equal(t, nodeDefaults.Resources, got)
	})

	t.Run("a profile with resources wins", func(t *testing.T) {
		profile := &v1alpha2.NodeScanProfile{Name: "large", Resources: profileResourceRequirements("2G")}
		got := ProfileResources(nodeDefaults, profile)
		assert.Equal(t, profile.Resources, got)
	})
}

func TestProfileTolerations(t *testing.T) {
	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
		Spec: corev1.NodeSpec{Taints: []corev1.Taint{
			{Key: "dedicated", Value: "scan", Effect: corev1.TaintEffectNoSchedule},
		}},
	}
	extra := corev1.Toleration{Key: "storage", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoExecute}

	t.Run("no profile keeps the node taint tolerations", func(t *testing.T) {
		assert.Equal(t, k8s.TaintsToTolerations(node.Spec.Taints), ProfileTolerations(node, nil))
	})

	t.Run("a profile appends its tolerations", func(t *testing.T) {
		got := ProfileTolerations(node, &v1alpha2.NodeScanProfile{Name: "large", Tolerations: []corev1.Toleration{extra}})
		require.Len(t, got, 2)
		assert.Equal(t, k8s.TaintsToTolerations(node.Spec.Taints)[0], got[0])
		assert.Equal(t, extra, got[1])
	})
}

func TestCronJob_UsesMatchingProfile(t *testing.T) {
	mac := testMondooAuditConfig()
	mac.Spec.Nodes.Resources = profileResourceRequirements("400M")
	mac.Spec.Nodes.Profiles = []v1alpha2.NodeScanProfile{
		{
			Name:         "large",
			NodeSelector: map[string]string{"size": "large"},
			Resources:    profileResourceRequirements("2G"),
			Tolerations:  []corev1.Toleration{{Key: "workload", Operator: corev1.TolerationOpExists}},
		},
	}

	large := CronJob("test-image", nodeWithLabels("node-large", map[string]string{"size": "large"}), mac, false, v1alpha2.MondooOperatorConfig{})
	largeSpec := large.Spec.JobTemplate.Spec.Template.Spec
	assert.Equal(t, resource.MustParse("2G"), largeSpec.Containers[0].Resources.Limits[corev1.ResourceMemory])
	assert.Contains(t, largeSpec.Tolerations, corev1.Toleration{Key: "workload", Operator: corev1.TolerationOpExists})

	small := CronJob("test-image", nodeWithLabels("node-small", map[string]string{"size": "small"}), mac, false, v1alpha2.MondooOperatorConfig{})
	smallSpec := small.Spec.JobTemplate.Spec.Template.Spec
	assert.Equal(t, resource.MustParse("400M"), smallSpec.Containers[0].Resources.Limits[corev1.ResourceMemory])
	assert.NotContains(t, smallSpec.Tolerations, corev1.Toleration{Key: "workload", Operator: corev1.TolerationOpExists})
}

func TestCronJob_WithoutProfilesIsUnchanged(t *testing.T) {
	mac := testMondooAuditConfig()
	node := nodeWithLabels("node-a", map[string]string{"size": "large"})

	before := CronJob("test-image", node, mac, false, v1alpha2.MondooOperatorConfig{})
	assert.Equal(t, k8s.DefaultNodeScanningResources, before.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Resources)
	assert.Equal(t, k8s.TaintsToTolerations(node.Spec.Taints), before.Spec.JobTemplate.Spec.Template.Spec.Tolerations)
}

func TestCronJob_ProfileDrivesGoMemLimit(t *testing.T) {
	mac := testMondooAuditConfig()
	mac.Spec.Nodes.Profiles = []v1alpha2.NodeScanProfile{
		{Name: "large", NodeSelector: map[string]string{"size": "large"}, Resources: profileResourceRequirements("2G")},
	}

	cj := CronJob("test-image", nodeWithLabels("node-large", map[string]string{"size": "large"}), mac, false, v1alpha2.MondooOperatorConfig{})
	var goMemLimit string
	for _, env := range cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Env {
		if env.Name == "GOMEMLIMIT" {
			goMemLimit = env.Value
		}
	}
	assert.NotEmpty(t, goMemLimit)
	assert.NotEqual(t, "", goMemLimit)
}

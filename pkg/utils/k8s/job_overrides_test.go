// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
)

func TestApplyJobOverrides_Empty(t *testing.T) {
	cj := &batchv1.CronJob{}
	ApplyJobOverrides(cj, v1alpha2.JobOverrides{})

	assert.Nil(t, cj.Spec.JobTemplate.Spec.TTLSecondsAfterFinished)
	assert.Nil(t, cj.Spec.JobTemplate.Annotations)
	assert.Nil(t, cj.Spec.JobTemplate.Spec.Template.Annotations)
	assert.Nil(t, cj.Spec.JobTemplate.Spec.Template.Spec.NodeSelector)
	assert.Nil(t, cj.Spec.JobTemplate.Spec.Template.Spec.Tolerations)
}

func TestApplyJobOverrides_TTLSecondsAfterFinished(t *testing.T) {
	cj := &batchv1.CronJob{}
	ApplyJobOverrides(cj, v1alpha2.JobOverrides{TTLSecondsAfterFinished: ptr.To(int32(300))})

	assert.Equal(t, ptr.To(int32(300)), cj.Spec.JobTemplate.Spec.TTLSecondsAfterFinished)
}

func TestApplyJobOverrides_Annotations(t *testing.T) {
	cj := &batchv1.CronJob{}
	cj.Spec.JobTemplate.Annotations = map[string]string{"operator-managed": "job"}
	cj.Spec.JobTemplate.Spec.Template.Annotations = map[string]string{"operator-managed": "pod"}

	ApplyJobOverrides(cj, v1alpha2.JobOverrides{
		Annotations: map[string]string{
			"karpenter.sh/do-not-disrupt": "true",
			"operator-managed":            "overwritten",
		},
	})

	assert.Equal(t, map[string]string{
		"karpenter.sh/do-not-disrupt": "true",
		"operator-managed":            "job",
	}, cj.Spec.JobTemplate.Annotations)
	assert.Equal(t, map[string]string{
		"karpenter.sh/do-not-disrupt": "true",
		"operator-managed":            "pod",
	}, cj.Spec.JobTemplate.Spec.Template.Annotations)
}

func TestApplyJobOverrides_AnnotationsNilMaps(t *testing.T) {
	cj := &batchv1.CronJob{}
	ApplyJobOverrides(cj, v1alpha2.JobOverrides{
		Annotations: map[string]string{"karpenter.sh/do-not-disrupt": "true"},
	})

	assert.Equal(t, map[string]string{"karpenter.sh/do-not-disrupt": "true"}, cj.Spec.JobTemplate.Annotations)
	assert.Equal(t, map[string]string{"karpenter.sh/do-not-disrupt": "true"}, cj.Spec.JobTemplate.Spec.Template.Annotations)
}

func TestApplyJobOverrides_NodeSelector(t *testing.T) {
	cj := &batchv1.CronJob{}
	nodeSelector := map[string]string{"workload-type": "mondoo-scan"}
	ApplyJobOverrides(cj, v1alpha2.JobOverrides{NodeSelector: nodeSelector})

	assert.Equal(t, nodeSelector, cj.Spec.JobTemplate.Spec.Template.Spec.NodeSelector)
}

func TestApplyJobOverrides_NodeSelectorSkippedForPinnedPods(t *testing.T) {
	cj := &batchv1.CronJob{}
	cj.Spec.JobTemplate.Spec.Template.Spec.NodeName = "node01"
	ApplyJobOverrides(cj, v1alpha2.JobOverrides{NodeSelector: map[string]string{"workload-type": "mondoo-scan"}})

	assert.Nil(t, cj.Spec.JobTemplate.Spec.Template.Spec.NodeSelector)
}

func TestApplyJobOverrides_Tolerations(t *testing.T) {
	existing := corev1.Toleration{Key: "node.kubernetes.io/unreachable", Operator: corev1.TolerationOpExists}
	user := corev1.Toleration{Key: "workload-type", Operator: corev1.TolerationOpEqual, Value: "mondoo-scan", Effect: corev1.TaintEffectNoSchedule}

	cj := &batchv1.CronJob{}
	cj.Spec.JobTemplate.Spec.Template.Spec.Tolerations = []corev1.Toleration{existing}
	ApplyJobOverrides(cj, v1alpha2.JobOverrides{Tolerations: []corev1.Toleration{user}})

	assert.Equal(t, []corev1.Toleration{existing, user}, cj.Spec.JobTemplate.Spec.Template.Spec.Tolerations)
}

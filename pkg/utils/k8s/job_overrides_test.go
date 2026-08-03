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
	assert.Nil(t, cj.Spec.JobTemplate.Labels)
	assert.Nil(t, cj.Spec.JobTemplate.Annotations)
	assert.Nil(t, cj.Spec.JobTemplate.Spec.Template.Labels)
	assert.Nil(t, cj.Spec.JobTemplate.Spec.Template.Annotations)
	assert.Nil(t, cj.Spec.JobTemplate.Spec.Template.Spec.NodeSelector)
	assert.Nil(t, cj.Spec.JobTemplate.Spec.Template.Spec.Tolerations)
}

func TestApplyJobOverrides_TTLSecondsAfterFinished(t *testing.T) {
	cj := &batchv1.CronJob{}
	ApplyJobOverrides(cj, v1alpha2.JobOverrides{TTLSecondsAfterFinished: ptr.To(int32(300))})

	assert.Equal(t, ptr.To(int32(300)), cj.Spec.JobTemplate.Spec.TTLSecondsAfterFinished)
}

func TestApplyJobOverrides_Labels(t *testing.T) {
	cj := &batchv1.CronJob{}
	cj.Spec.JobTemplate.Labels = map[string]string{"operator-managed": "job"}
	cj.Spec.JobTemplate.Spec.Template.Labels = map[string]string{"operator-managed": "pod"}

	ApplyJobOverrides(cj, v1alpha2.JobOverrides{
		Labels: map[string]string{
			"team":             "security",
			"operator-managed": "overwritten",
		},
	})

	assert.Equal(t, map[string]string{
		"team":             "security",
		"operator-managed": "job",
	}, cj.Spec.JobTemplate.Labels)
	assert.Equal(t, map[string]string{
		"team":             "security",
		"operator-managed": "pod",
	}, cj.Spec.JobTemplate.Spec.Template.Labels)
}

func TestApplyJobOverrides_LabelsNilMaps(t *testing.T) {
	cj := &batchv1.CronJob{}
	ApplyJobOverrides(cj, v1alpha2.JobOverrides{
		Labels: map[string]string{"team": "security"},
	})

	assert.Equal(t, map[string]string{"team": "security"}, cj.Spec.JobTemplate.Labels)
	assert.Equal(t, map[string]string{"team": "security"}, cj.Spec.JobTemplate.Spec.Template.Labels)
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

func TestApplyJobOverrides_TolerationsDeduplicated(t *testing.T) {
	toleration := corev1.Toleration{Key: "workload-type", Operator: corev1.TolerationOpEqual, Value: "mondoo-scan", Effect: corev1.TaintEffectNoSchedule}

	cj := &batchv1.CronJob{}
	cj.Spec.JobTemplate.Spec.Template.Spec.Tolerations = []corev1.Toleration{toleration}
	ApplyJobOverrides(cj, v1alpha2.JobOverrides{Tolerations: []corev1.Toleration{toleration}})

	assert.Len(t, cj.Spec.JobTemplate.Spec.Template.Spec.Tolerations, 1)
}

// MergeTolerations tests

func TestMergeTolerations_BothEmpty(t *testing.T) {
	assert.Nil(t, MergeTolerations(nil, nil))
}

func TestMergeTolerations_Dedup(t *testing.T) {
	t1 := corev1.Toleration{Key: "key1", Operator: corev1.TolerationOpEqual, Value: "val1", Effect: corev1.TaintEffectNoSchedule}
	t2 := corev1.Toleration{Key: "key2", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoExecute}

	result := MergeTolerations([]corev1.Toleration{t1, t2}, []corev1.Toleration{t1})
	assert.Equal(t, []corev1.Toleration{t1, t2}, result)
}

func TestMergeTolerations_DifferentTolerationSeconds(t *testing.T) {
	t1 := corev1.Toleration{Key: "key1", Operator: corev1.TolerationOpExists, TolerationSeconds: ptr.To(int64(300))}
	t2 := corev1.Toleration{Key: "key1", Operator: corev1.TolerationOpExists, TolerationSeconds: ptr.To(int64(600))}

	result := MergeTolerations([]corev1.Toleration{t1}, []corev1.Toleration{t2})
	assert.Len(t, result, 2)
}

func TestMergeTolerations_DeepCopyTolerationSeconds(t *testing.T) {
	seconds := int64(300)
	t1 := corev1.Toleration{Key: "key1", TolerationSeconds: &seconds}

	result := MergeTolerations(nil, []corev1.Toleration{t1})
	assert.Equal(t, int64(300), *result[0].TolerationSeconds)

	seconds = 999
	assert.Equal(t, int64(300), *result[0].TolerationSeconds, "should not alias the original pointer")
}

// MergeJobOverrides tests

func TestMergeJobOverrides_BothEmpty(t *testing.T) {
	result := MergeJobOverrides(v1alpha2.JobOverrides{}, v1alpha2.JobOverrides{})
	assert.Equal(t, v1alpha2.JobOverrides{}, result)
}

func TestMergeJobOverrides_GlobalOnly(t *testing.T) {
	global := v1alpha2.JobOverrides{
		TTLSecondsAfterFinished: ptr.To(int32(3600)),
		Labels:                  map[string]string{"team": "security"},
		Annotations:             map[string]string{"a": "1"},
		NodeSelector:            map[string]string{"node": "infra"},
		Tolerations: []corev1.Toleration{
			{Key: "key1", Operator: corev1.TolerationOpExists},
		},
	}

	result := MergeJobOverrides(global, v1alpha2.JobOverrides{})
	assert.Equal(t, ptr.To(int32(3600)), result.TTLSecondsAfterFinished)
	assert.Equal(t, map[string]string{"team": "security"}, result.Labels)
	assert.Equal(t, map[string]string{"a": "1"}, result.Annotations)
	assert.Equal(t, map[string]string{"node": "infra"}, result.NodeSelector)
	assert.Len(t, result.Tolerations, 1)
}

func TestMergeJobOverrides_PerTypeOverridesGlobal(t *testing.T) {
	global := v1alpha2.JobOverrides{
		TTLSecondsAfterFinished: ptr.To(int32(3600)),
		Labels:                  map[string]string{"team": "security", "env": "prod"},
		Annotations:             map[string]string{"a": "1", "b": "2"},
		NodeSelector:            map[string]string{"node": "infra"},
	}
	perType := v1alpha2.JobOverrides{
		TTLSecondsAfterFinished: ptr.To(int32(7200)),
		Labels:                  map[string]string{"team": "platform"},
		Annotations:             map[string]string{"b": "3", "c": "4"},
		NodeSelector:            map[string]string{"node": "scan"},
	}

	result := MergeJobOverrides(global, perType)
	assert.Equal(t, ptr.To(int32(7200)), result.TTLSecondsAfterFinished)
	assert.Equal(t, map[string]string{"team": "platform", "env": "prod"}, result.Labels)
	assert.Equal(t, map[string]string{"a": "1", "b": "3", "c": "4"}, result.Annotations)
	assert.Equal(t, map[string]string{"node": "scan"}, result.NodeSelector)
}

func TestMergeJobOverrides_TolerationsUnionMerged(t *testing.T) {
	t1 := corev1.Toleration{Key: "key1", Operator: corev1.TolerationOpExists}
	t2 := corev1.Toleration{Key: "key2", Operator: corev1.TolerationOpExists}

	global := v1alpha2.JobOverrides{Tolerations: []corev1.Toleration{t1}}
	perType := v1alpha2.JobOverrides{Tolerations: []corev1.Toleration{t1, t2}}

	result := MergeJobOverrides(global, perType)
	assert.Len(t, result.Tolerations, 2)
	assert.Equal(t, t1.Key, result.Tolerations[0].Key)
	assert.Equal(t, t2.Key, result.Tolerations[1].Key)
}

func TestMergeJobOverrides_PerTypeOnlyTTL(t *testing.T) {
	global := v1alpha2.JobOverrides{TTLSecondsAfterFinished: ptr.To(int32(3600))}
	perType := v1alpha2.JobOverrides{TTLSecondsAfterFinished: ptr.To(int32(0))}

	result := MergeJobOverrides(global, perType)
	assert.Equal(t, ptr.To(int32(0)), result.TTLSecondsAfterFinished)
}

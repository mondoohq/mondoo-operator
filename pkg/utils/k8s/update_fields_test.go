// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestUpdateCronJobFields_ImagePullSecrets(t *testing.T) {
	desired := &batchv1.CronJob{
		Spec: batchv1.CronJobSpec{
			Schedule: "*/5 * * * *",
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{Name: "test", Image: "test:latest"}},
							ImagePullSecrets: []corev1.LocalObjectReference{
								{Name: "my-secret"},
								{Name: "another-secret"},
							},
						},
					},
				},
			},
		},
	}

	obj := &batchv1.CronJob{}
	UpdateCronJobFields(obj, desired)

	assert.Equal(t, desired.Spec.Schedule, obj.Spec.Schedule)
	assert.Equal(t, desired.Spec.JobTemplate.Spec.Template.Spec.Containers, obj.Spec.JobTemplate.Spec.Template.Spec.Containers)
	assert.Equal(t, desired.Spec.JobTemplate.Spec.Template.Spec.ImagePullSecrets, obj.Spec.JobTemplate.Spec.Template.Spec.ImagePullSecrets)
}

func TestUpdateCronJobFields_JobOverrides(t *testing.T) {
	desired := &batchv1.CronJob{
		Spec: batchv1.CronJobSpec{
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{"karpenter.sh/do-not-disrupt": "true"},
				},
				Spec: batchv1.JobSpec{
					TTLSecondsAfterFinished: ptr.To(int32(300)),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{"karpenter.sh/do-not-disrupt": "true"},
						},
						Spec: corev1.PodSpec{
							NodeSelector: map[string]string{"workload-type": "mondoo-scan"},
							Tolerations: []corev1.Toleration{
								{Key: "workload-type", Operator: corev1.TolerationOpEqual, Value: "mondoo-scan", Effect: corev1.TaintEffectNoSchedule},
							},
						},
					},
				},
			},
		},
	}

	obj := &batchv1.CronJob{}
	UpdateCronJobFields(obj, desired)

	assert.Equal(t, desired.Spec.JobTemplate.Spec.TTLSecondsAfterFinished, obj.Spec.JobTemplate.Spec.TTLSecondsAfterFinished)
	assert.Equal(t, desired.Spec.JobTemplate.Annotations, obj.Spec.JobTemplate.Annotations)
	assert.Equal(t, desired.Spec.JobTemplate.Spec.Template.Annotations, obj.Spec.JobTemplate.Spec.Template.Annotations)
	assert.Equal(t, desired.Spec.JobTemplate.Spec.Template.Spec.NodeSelector, obj.Spec.JobTemplate.Spec.Template.Spec.NodeSelector)
	assert.Equal(t, desired.Spec.JobTemplate.Spec.Template.Spec.Tolerations, obj.Spec.JobTemplate.Spec.Template.Spec.Tolerations)
}

func TestUpdateCronJobFields_PreservesUnmanagedFields(t *testing.T) {
	obj := &batchv1.CronJob{
		Spec: batchv1.CronJobSpec{
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							DNSPolicy:     corev1.DNSClusterFirst,
							SchedulerName: "custom-scheduler",
						},
					},
				},
			},
		},
	}

	desired := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test"}},
		Spec: batchv1.CronJobSpec{
			Schedule: "*/10 * * * *",
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{Name: "test"}},
						},
					},
				},
			},
		},
	}

	UpdateCronJobFields(obj, desired)

	// Managed fields are updated
	assert.Equal(t, "*/10 * * * *", obj.Spec.Schedule)
	// Unmanaged fields are preserved
	assert.Equal(t, corev1.DNSClusterFirst, obj.Spec.JobTemplate.Spec.Template.Spec.DNSPolicy)
	assert.Equal(t, "custom-scheduler", obj.Spec.JobTemplate.Spec.Template.Spec.SchedulerName)
}

func TestUpdateDeploymentFields_ImagePullSecrets(t *testing.T) {
	desired := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "test", Image: "test:latest"}},
					ImagePullSecrets: []corev1.LocalObjectReference{
						{Name: "my-secret"},
					},
				},
			},
		},
	}

	obj := &appsv1.Deployment{}
	UpdateDeploymentFields(obj, desired)

	assert.Equal(t, desired.Spec.Template.Spec.ImagePullSecrets, obj.Spec.Template.Spec.ImagePullSecrets)
	assert.Equal(t, desired.Spec.Template.Spec.Containers, obj.Spec.Template.Spec.Containers)
}

func TestUpdateDaemonSetFields_ImagePullSecrets(t *testing.T) {
	desired := &appsv1.DaemonSet{
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "test", Image: "test:latest"}},
					ImagePullSecrets: []corev1.LocalObjectReference{
						{Name: "my-secret"},
					},
				},
			},
		},
	}

	obj := &appsv1.DaemonSet{}
	UpdateDaemonSetFields(obj, desired)

	assert.Equal(t, desired.Spec.Template.Spec.ImagePullSecrets, obj.Spec.Template.Spec.ImagePullSecrets)
	assert.Equal(t, desired.Spec.Template.Spec.Containers, obj.Spec.Template.Spec.Containers)
}

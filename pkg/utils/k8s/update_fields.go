// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
)

// UpdateCronJobFields copies managed fields from desired to obj,
// preserving server-set defaults on unmanaged fields like
// Suspend, Completions, Parallelism, DNSPolicy, SchedulerName, etc.
func UpdateCronJobFields(obj, desired *batchv1.CronJob) {
	obj.Labels = desired.Labels
	obj.Annotations = desired.Annotations
	obj.Spec.Schedule = desired.Spec.Schedule
	obj.Spec.ConcurrencyPolicy = desired.Spec.ConcurrencyPolicy
	obj.Spec.SuccessfulJobsHistoryLimit = desired.Spec.SuccessfulJobsHistoryLimit
	obj.Spec.FailedJobsHistoryLimit = desired.Spec.FailedJobsHistoryLimit

	obj.Spec.JobTemplate.Labels = desired.Spec.JobTemplate.Labels
	obj.Spec.JobTemplate.Annotations = desired.Spec.JobTemplate.Annotations
	obj.Spec.JobTemplate.Spec.BackoffLimit = desired.Spec.JobTemplate.Spec.BackoffLimit

	obj.Spec.JobTemplate.Spec.Template.Labels = desired.Spec.JobTemplate.Spec.Template.Labels
	obj.Spec.JobTemplate.Spec.Template.Annotations = desired.Spec.JobTemplate.Spec.Template.Annotations

	ps := &obj.Spec.JobTemplate.Spec.Template.Spec
	dps := &desired.Spec.JobTemplate.Spec.Template.Spec
	ps.RestartPolicy = dps.RestartPolicy
	ps.NodeName = dps.NodeName
	ps.Tolerations = dps.Tolerations
	ps.ServiceAccountName = dps.ServiceAccountName
	ps.AutomountServiceAccountToken = dps.AutomountServiceAccountToken
	ps.PriorityClassName = dps.PriorityClassName
	ps.InitContainers = dps.InitContainers
	ps.Containers = dps.Containers
	ps.Volumes = dps.Volumes
}

// UpdateDeploymentFields copies managed fields from desired to obj,
// preserving server-set defaults on unmanaged fields like
// RevisionHistoryLimit, ProgressDeadlineSeconds, Strategy, DNSPolicy, etc.
func UpdateDeploymentFields(obj, desired *appsv1.Deployment) {
	obj.Labels = desired.Labels
	obj.Spec.Replicas = desired.Spec.Replicas
	obj.Spec.Selector = desired.Spec.Selector
	obj.Spec.Template.Labels = desired.Spec.Template.Labels
	obj.Spec.Template.Annotations = desired.Spec.Template.Annotations

	ps := &obj.Spec.Template.Spec
	dps := &desired.Spec.Template.Spec
	ps.ServiceAccountName = dps.ServiceAccountName
	ps.Containers = dps.Containers
	ps.Volumes = dps.Volumes
}

// UpdateDaemonSetFields copies managed fields from desired to obj,
// preserving server-set defaults on unmanaged fields like
// UpdateStrategy, RevisionHistoryLimit, DNSPolicy, etc.
func UpdateDaemonSetFields(obj, desired *appsv1.DaemonSet) {
	obj.Labels = desired.Labels
	obj.Annotations = desired.Annotations
	obj.Spec.Selector = desired.Spec.Selector
	obj.Spec.Template.Labels = desired.Spec.Template.Labels
	obj.Spec.Template.Annotations = desired.Spec.Template.Annotations

	ps := &obj.Spec.Template.Spec
	dps := &desired.Spec.Template.Spec
	ps.PriorityClassName = dps.PriorityClassName
	ps.AutomountServiceAccountToken = dps.AutomountServiceAccountToken
	ps.Tolerations = dps.Tolerations
	ps.Containers = dps.Containers
	ps.Volumes = dps.Volumes
}

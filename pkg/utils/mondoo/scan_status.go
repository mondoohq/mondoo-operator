// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package mondoo

import (
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
)

// ScanStatusFromCronJob builds scan status from the CronJob Kubernetes exposes for scheduled scans.
func ScanStatusFromCronJob(scanType v1alpha2.MondooAuditConfigScanType, target string, cronJob batchv1.CronJob) v1alpha2.MondooAuditConfigScanStatus {
	status := v1alpha2.MondooAuditConfigScanStatus{
		Type:               scanType,
		Target:             target,
		Phase:              v1alpha2.MondooAuditConfigScanPhasePending,
		CronJob:            cronJob.Name,
		ActiveJobs:         activeJobNames(cronJob.Status.Active),
		LastScheduleTime:   cronJob.Status.LastScheduleTime,
		LastSuccessfulTime: cronJob.Status.LastSuccessfulTime,
		Message:            "Scan has not been scheduled yet",
	}

	switch {
	case len(cronJob.Status.Active) > 0:
		status.Phase = v1alpha2.MondooAuditConfigScanPhaseRunning
		status.Message = "Scan is running"
		if cronJob.Status.LastScheduleTime != nil &&
			(cronJob.Status.LastSuccessfulTime == nil || cronJob.Status.LastSuccessfulTime.Before(cronJob.Status.LastScheduleTime)) {
			status.Message = "Scan is running; previous scheduled scan has not completed successfully"
		}
	case cronJob.Status.LastScheduleTime == nil:
		status.Phase = v1alpha2.MondooAuditConfigScanPhasePending
		status.Message = "Scan has not been scheduled yet"
	case cronJob.Status.LastSuccessfulTime == nil || cronJob.Status.LastSuccessfulTime.Before(cronJob.Status.LastScheduleTime):
		status.Phase = v1alpha2.MondooAuditConfigScanPhaseFailed
		status.Message = "Last scheduled scan has not completed successfully"
	default:
		status.Phase = v1alpha2.MondooAuditConfigScanPhaseSucceeded
		status.Message = "Last scheduled scan completed successfully"
	}

	return status
}

// DisabledScanStatus builds scan status for configured scan workloads that are intentionally disabled.
func DisabledScanStatus(scanType v1alpha2.MondooAuditConfigScanType, target string) v1alpha2.MondooAuditConfigScanStatus {
	return v1alpha2.MondooAuditConfigScanStatus{
		Type:    scanType,
		Target:  target,
		Phase:   v1alpha2.MondooAuditConfigScanPhaseDisabled,
		Message: "Scan is disabled",
	}
}

// ReplaceScanStatuses replaces all statuses for one scan type with the supplied statuses.
func ReplaceScanStatuses(config *v1alpha2.MondooAuditConfig, scanType v1alpha2.MondooAuditConfigScanType, statuses ...v1alpha2.MondooAuditConfigScanStatus) {
	filtered := make([]v1alpha2.MondooAuditConfigScanStatus, 0, len(config.Status.Scans)+len(statuses))
	for _, status := range config.Status.Scans {
		if status.Type == scanType {
			continue
		}
		filtered = append(filtered, status)
	}
	config.Status.Scans = append(filtered, statuses...)
}

func activeJobNames(active []corev1.ObjectReference) []string {
	if len(active) == 0 {
		return nil
	}
	names := make([]string, 0, len(active))
	for _, ref := range active {
		names = append(names, ref.Name)
	}
	return names
}

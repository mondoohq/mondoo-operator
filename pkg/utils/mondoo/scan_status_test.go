// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package mondoo

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
)

func TestScanStatusFromCronJob(t *testing.T) {
	now := metav1.NewTime(time.Now())
	before := metav1.NewTime(now.Add(-time.Hour))
	after := metav1.NewTime(now.Add(time.Hour))

	tests := []struct {
		name    string
		status  batchv1.CronJobStatus
		phase   v1alpha2.MondooAuditConfigScanPhase
		message string
		active  []string
	}{
		{
			name:    "pending when never scheduled",
			phase:   v1alpha2.MondooAuditConfigScanPhasePending,
			message: "Scan has not been scheduled yet",
		},
		{
			name: "running when jobs are active",
			status: batchv1.CronJobStatus{
				LastScheduleTime:   &now,
				LastSuccessfulTime: &now,
				Active:             []corev1.ObjectReference{{Name: "scan-job"}},
			},
			phase:   v1alpha2.MondooAuditConfigScanPhaseRunning,
			message: "Scan is running",
			active:  []string{"scan-job"},
		},
		{
			name: "running with previous failure hint when latest schedule has no success",
			status: batchv1.CronJobStatus{
				LastScheduleTime:   &now,
				LastSuccessfulTime: &before,
				Active:             []corev1.ObjectReference{{Name: "scan-job"}},
			},
			phase:   v1alpha2.MondooAuditConfigScanPhaseRunning,
			message: "Scan is running; previous scheduled scan has not completed successfully",
			active:  []string{"scan-job"},
		},
		{
			name: "failed when latest schedule has no success",
			status: batchv1.CronJobStatus{
				LastScheduleTime: &now,
			},
			phase:   v1alpha2.MondooAuditConfigScanPhaseFailed,
			message: "Last scheduled scan has not completed successfully",
		},
		{
			name: "failed when latest success is before latest schedule",
			status: batchv1.CronJobStatus{
				LastScheduleTime:   &now,
				LastSuccessfulTime: &before,
			},
			phase:   v1alpha2.MondooAuditConfigScanPhaseFailed,
			message: "Last scheduled scan has not completed successfully",
		},
		{
			name: "succeeded when latest success is after latest schedule",
			status: batchv1.CronJobStatus{
				LastScheduleTime:   &now,
				LastSuccessfulTime: &after,
			},
			phase:   v1alpha2.MondooAuditConfigScanPhaseSucceeded,
			message: "Last scheduled scan completed successfully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cronJob := batchv1.CronJob{
				ObjectMeta: metav1.ObjectMeta{Name: "scan"},
				Status:     tt.status,
			}

			got := ScanStatusFromCronJob(v1alpha2.MondooAuditConfigScanTypeKubernetesResources, "local", cronJob)

			assert.Equal(t, v1alpha2.MondooAuditConfigScanTypeKubernetesResources, got.Type)
			assert.Equal(t, "local", got.Target)
			assert.Equal(t, "scan", got.CronJob)
			assert.Equal(t, tt.phase, got.Phase)
			assert.Equal(t, tt.message, got.Message)
			assert.Equal(t, tt.active, got.ActiveJobs)
			assert.Equal(t, tt.status.LastScheduleTime, got.LastScheduleTime)
			assert.Equal(t, tt.status.LastSuccessfulTime, got.LastSuccessfulTime)
		})
	}
}

func TestReplaceScanStatuses(t *testing.T) {
	config := &v1alpha2.MondooAuditConfig{
		Status: v1alpha2.MondooAuditConfigStatus{
			Scans: []v1alpha2.MondooAuditConfigScanStatus{
				{Type: v1alpha2.MondooAuditConfigScanTypeKubernetesResources, Target: "old"},
				{Type: v1alpha2.MondooAuditConfigScanTypeContainerImages, Target: "local"},
			},
		},
	}

	ReplaceScanStatuses(
		config,
		v1alpha2.MondooAuditConfigScanTypeKubernetesResources,
		v1alpha2.MondooAuditConfigScanStatus{Type: v1alpha2.MondooAuditConfigScanTypeKubernetesResources, Target: "new"},
	)

	assert.Equal(t, []v1alpha2.MondooAuditConfigScanStatus{
		{Type: v1alpha2.MondooAuditConfigScanTypeContainerImages, Target: "local"},
		{Type: v1alpha2.MondooAuditConfigScanTypeKubernetesResources, Target: "new"},
	}, config.Status.Scans)
}

func TestDisabledScanStatus(t *testing.T) {
	got := DisabledScanStatus(v1alpha2.MondooAuditConfigScanTypeNodes, "all")

	assert.Equal(t, v1alpha2.MondooAuditConfigScanStatus{
		Type:    v1alpha2.MondooAuditConfigScanTypeNodes,
		Target:  "all",
		Phase:   v1alpha2.MondooAuditConfigScanPhaseDisabled,
		Message: "Scan is disabled",
	}, got)
}

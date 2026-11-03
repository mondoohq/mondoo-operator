// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
)

func TestGuardCronSchedule(t *testing.T) {
	tests := []struct {
		name     string
		schedule string
		want     string
		clamped  bool
	}{
		{name: "every five minutes", schedule: "*/5 * * * *", want: "0 * * * *", clamped: true},
		{name: "every minute", schedule: "* * * * *", want: "0 * * * *", clamped: true},
		{name: "keeps the other fields", schedule: "*/15 9-17 * * 1-5", want: "0 9-17 * * 1-5", clamped: true},
		{name: "minute list", schedule: "0,30 * * * *", want: "0 * * * *", clamped: true},
		{name: "minute range", schedule: "5-20 * * * *", want: "5 * * * *", clamped: true},
		{name: "hourly", schedule: "30 * * * *", want: "30 * * * *"},
		{name: "daily", schedule: "0 0 * * *", want: "0 0 * * *"},
		{name: "two fires a day", schedule: "0 9,10 * * *", want: "0 9,10 * * *"},
		{name: "empty", schedule: "", want: ""},
		{name: "unparsable left alone", schedule: "@every 5m", want: "@every 5m"},
		{name: "garbage left alone", schedule: "not a schedule", want: "not a schedule"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, clamped := GuardCronSchedule(tt.schedule)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.clamped, clamped)
		})
	}
}

func TestApplyScheduleGuard_ClampsEveryScanSchedule(t *testing.T) {
	spec := v1alpha2.MondooAuditConfigSpec{
		KubernetesResources: v1alpha2.KubernetesResources{
			Enable:   true,
			Schedule: "*/5 * * * *",
			ExternalClusters: []v1alpha2.ExternalCluster{
				{Name: "prod", Schedule: "*/10 * * * *"},
				{Name: "inherits"},
				{Name: "fine", Schedule: "15 * * * *"},
			},
		},
		Nodes: v1alpha2.Nodes{
			Enable:        true,
			Schedule:      "*/2 * * * *",
			IntervalTimer: 5,
		},
		Containers: v1alpha2.Containers{
			Enable:   true,
			Schedule: "*/30 * * * *",
		},
	}

	changes := ApplyScheduleGuard(&spec)

	assert.Equal(t, "0 * * * *", spec.KubernetesResources.Schedule)
	assert.Equal(t, "0 * * * *", spec.KubernetesResources.ExternalClusters[0].Schedule)
	assert.Equal(t, "", spec.KubernetesResources.ExternalClusters[1].Schedule)
	assert.Equal(t, "15 * * * *", spec.KubernetesResources.ExternalClusters[2].Schedule)
	assert.Equal(t, "0 * * * *", spec.Nodes.Schedule)
	assert.Equal(t, 60, spec.Nodes.IntervalTimer)
	assert.Equal(t, "0 * * * *", spec.Containers.Schedule)

	assert.Equal(t, []ScheduleGuardChange{
		{Field: "spec.kubernetesResources.schedule", From: "*/5 * * * *", To: "0 * * * *"},
		{Field: "spec.kubernetesResources.externalClusters[prod].schedule", From: "*/10 * * * *", To: "0 * * * *"},
		{Field: "spec.nodes.schedule", From: "*/2 * * * *", To: "0 * * * *"},
		{Field: "spec.nodes.intervalTimer", From: "5", To: "60"},
		{Field: "spec.containers.schedule", From: "*/30 * * * *", To: "0 * * * *"},
	}, changes)
}

func TestApplyScheduleGuard_LeavesSaneSchedulesAlone(t *testing.T) {
	spec := v1alpha2.MondooAuditConfigSpec{
		KubernetesResources: v1alpha2.KubernetesResources{Enable: true, Schedule: "41 * * * *"},
		Nodes:               v1alpha2.Nodes{Enable: true, Schedule: "0 */6 * * *", IntervalTimer: 60},
		Containers:          v1alpha2.Containers{Enable: true, Schedule: "0 2 * * *"},
	}
	want := *spec.DeepCopy()

	changes := ApplyScheduleGuard(&spec)

	assert.Empty(t, changes)
	assert.Equal(t, want, spec)
}

func TestApplyScheduleGuard_UnsetIntervalTimerIsNotAChange(t *testing.T) {
	spec := v1alpha2.MondooAuditConfigSpec{Nodes: v1alpha2.Nodes{Enable: true, Schedule: "0 * * * *"}}

	changes := ApplyScheduleGuard(&spec)

	assert.Empty(t, changes)
	assert.Equal(t, 0, spec.Nodes.IntervalTimer)
}

func TestApplyScheduleGuard_Disabled(t *testing.T) {
	spec := v1alpha2.MondooAuditConfigSpec{
		ScheduleGuard:       v1alpha2.ScheduleGuard{Disable: true},
		KubernetesResources: v1alpha2.KubernetesResources{Enable: true, Schedule: "*/5 * * * *"},
		Nodes:               v1alpha2.Nodes{Enable: true, Schedule: "*/5 * * * *", IntervalTimer: 5},
		Containers:          v1alpha2.Containers{Enable: true, Schedule: "*/5 * * * *"},
	}
	want := *spec.DeepCopy()

	changes := ApplyScheduleGuard(&spec)

	assert.Empty(t, changes)
	assert.Equal(t, want, spec)
}

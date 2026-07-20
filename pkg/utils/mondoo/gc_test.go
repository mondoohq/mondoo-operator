// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package mondoo

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestManagedByLabels(t *testing.T) {
	t.Run("ManagedByLabel", func(t *testing.T) {
		assert.Equal(t, "mondoo-operator", ManagedByLabel(""))
		assert.Equal(t, "mondoo-operator-abc123", ManagedByLabel("abc123"))
	})

	t.Run("ManagedByContainersLabel", func(t *testing.T) {
		assert.Equal(t, "mondoo-operator-containers", ManagedByContainersLabel(""))
		assert.Equal(t, "mondoo-operator-containers-abc123", ManagedByContainersLabel("abc123"))
	})

	t.Run("ManagedByNodesLabel", func(t *testing.T) {
		assert.Equal(t, "mondoo-operator-nodes", ManagedByNodesLabel(""))
		assert.Equal(t, "mondoo-operator-nodes-abc123", ManagedByNodesLabel("abc123"))
	})

	t.Run("labels are distinct", func(t *testing.T) {
		uid := "test-uid"
		labels := []string{ManagedByLabel(uid), ManagedByContainersLabel(uid), ManagedByNodesLabel(uid)}
		for i := range labels {
			for j := i + 1; j < len(labels); j++ {
				assert.NotEqual(t, labels[i], labels[j])
			}
		}
	})
}

func TestGCOlderThanFromSchedule(t *testing.T) {
	tests := []struct {
		name     string
		schedule string
		expect   time.Duration
	}{
		{
			name:     "hourly schedule",
			schedule: "30 * * * *",
			expect:   2 * time.Hour,
		},
		{
			name:     "every 6 hours",
			schedule: "0 */6 * * *",
			expect:   12 * time.Hour,
		},
		{
			name:     "daily schedule",
			schedule: "0 2 * * *",
			expect:   48 * time.Hour,
		},
		{
			name:     "every 15 minutes",
			schedule: "*/15 * * * *",
			expect:   30 * time.Minute,
		},
		{
			name:     "empty schedule falls back to default",
			schedule: "",
			expect:   defaultGCOlderThan,
		},
		{
			name:     "invalid schedule falls back to default",
			schedule: "not-a-cron",
			expect:   defaultGCOlderThan,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, gcOlderThanFromSchedule(tt.schedule))
		})
	}
}

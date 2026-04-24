// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package mondoo

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

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

// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
)

// MinimumScanInterval is the shortest interval between two runs of the same scan that the
// schedule guard allows.
const MinimumScanInterval = time.Hour

// scheduleGuardSampleSize is how many consecutive cron fires are inspected to find the
// shortest interval of a schedule.
const scheduleGuardSampleSize = 50

// ScheduleGuardChange records one field the schedule guard changed.
type ScheduleGuardChange struct {
	Field string
	From  string
	To    string
}

func (c ScheduleGuardChange) String() string {
	return fmt.Sprintf("%s %q -> %q", c.Field, c.From, c.To)
}

// ApplyScheduleGuard rewrites every scan schedule in spec that fires more often than
// MinimumScanInterval so that it fires at most once an hour, and raises
// spec.nodes.intervalTimer to at least MinimumScanInterval. It changes spec in place and
// returns one entry per changed field. It does nothing when spec.ScheduleGuard.Disable is set.
func ApplyScheduleGuard(spec *v1alpha2.MondooAuditConfigSpec) []ScheduleGuardChange {
	if spec.ScheduleGuard.Disable {
		return nil
	}

	var changes []ScheduleGuardChange
	guard := func(field string, schedule *string) {
		guarded, clamped := GuardCronSchedule(*schedule)
		if !clamped {
			return
		}
		changes = append(changes, ScheduleGuardChange{Field: field, From: *schedule, To: guarded})
		*schedule = guarded
	}

	guard("spec.kubernetesResources.schedule", &spec.KubernetesResources.Schedule)
	for i := range spec.KubernetesResources.ExternalClusters {
		cluster := &spec.KubernetesResources.ExternalClusters[i]
		guard(fmt.Sprintf("spec.kubernetesResources.externalClusters[%s].schedule", cluster.Name), &cluster.Schedule)
	}
	guard("spec.nodes.schedule", &spec.Nodes.Schedule)

	minTimer := int(MinimumScanInterval / time.Minute)
	if spec.Nodes.IntervalTimer > 0 && spec.Nodes.IntervalTimer < minTimer {
		changes = append(changes, ScheduleGuardChange{
			Field: "spec.nodes.intervalTimer",
			From:  strconv.Itoa(spec.Nodes.IntervalTimer),
			To:    strconv.Itoa(minTimer),
		})
		spec.Nodes.IntervalTimer = minTimer
	}

	guard("spec.containers.schedule", &spec.Containers.Schedule)

	return changes
}

// GuardCronSchedule returns the schedule to run and whether it differs from the given one.
// A schedule whose runs can be closer together than MinimumScanInterval gets its minute
// field replaced by the first minute it matches, which makes it fire at most once an hour
// while keeping the hour, day, month and weekday fields. Schedules that cannot be parsed
// are returned unchanged.
func GuardCronSchedule(schedule string) (string, bool) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	sched, err := parser.Parse(schedule)
	if err != nil {
		return schedule, false
	}
	if shortestInterval(sched) >= MinimumScanInterval {
		return schedule, false
	}

	fields := strings.Fields(schedule)
	if len(fields) != 5 {
		return schedule, false
	}
	// The first fire after an hour boundary lands on the smallest minute the schedule matches.
	// Using a fixed reference time keeps the result stable across reconciles and restarts.
	first := sched.Next(time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC).Add(-time.Minute))
	fields[0] = strconv.Itoa(first.Minute())
	guarded := strings.Join(fields, " ")
	if guarded == schedule {
		return schedule, false
	}
	return guarded, true
}

func shortestInterval(sched cron.Schedule) time.Duration {
	shortest := time.Duration(math.MaxInt64)
	prev := sched.Next(time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC))
	for range scheduleGuardSampleSize {
		next := sched.Next(prev)
		if next.IsZero() {
			break
		}
		if interval := next.Sub(prev); interval < shortest {
			shortest = interval
		}
		prev = next
	}
	return shortest
}

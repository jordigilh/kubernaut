/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package retention

import (
	"fmt"
	"time"

	"github.com/jordigilh/kubernaut/pkg/datastorage/partition"
)

// Config holds the retention worker configuration, driven by Helm values.
type Config struct {
	Enabled              bool          // Master switch: disabled by default (BR-AUDIT-004)
	Interval             time.Duration // How often the worker runs (default: 24h)
	BatchSize            int           // Max rows per DELETE batch (default: 1000)
	PartitionDropEnabled bool          // Whether to attempt DROP PARTITION on empty months
}

// DefaultConfig returns the production default configuration (disabled).
func DefaultConfig() Config {
	return Config{
		Enabled:              false,
		Interval:             24 * time.Hour,
		BatchSize:            1000,
		PartitionDropEnabled: false,
	}
}

// MinRetentionDays is the minimum allowed retention period.
const MinRetentionDays = 1

// MaxRetentionDays is the maximum allowed retention period (~7 years).
const MaxRetentionDays = 2555

// DefaultRetentionDays is used when no per-event or category policy is set.
const DefaultRetentionDays = 1

// CategoryPolicy represents a row from audit_retention_policies.
type CategoryPolicy struct {
	EventCategory string
	RetentionDays int
}

// AuditEvent holds the fields relevant to retention eligibility.
type AuditEvent struct {
	EventDate     time.Time // UTC date (partition-aligned)
	RetentionDays int       // Per-event override (1..2555)
	LegalHold     bool      // If true, never eligible for deletion
}

// IsEligibleForPurge returns true if the event is time-expired, not under legal hold,
// and the effective retention has been exceeded.
// Effective retention = MAX(event.RetentionDays, categoryFloor).
// Uses strict < comparison: an event expiring today is NOT yet eligible.
func IsEligibleForPurge(event AuditEvent, categoryFloor int, now time.Time) bool {
	if event.LegalHold {
		return false
	}
	var floor *int
	if categoryFloor > 0 {
		floor = &categoryFloor
	}
	effectiveDays := EffectiveRetention(event.RetentionDays, floor)
	expiryDate := event.EventDate.AddDate(0, 0, effectiveDays)
	// Truncate now to date-only (midnight UTC) for partition-aligned eligibility.
	// An event expiring today is NOT eligible (strict < on date boundary).
	todayUTC := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return todayUTC.After(expiryDate)
}

// EffectiveRetention computes the retention days applying policy precedence:
// MAX(clamp(eventRetentionDays), COALESCE(categoryFloor, DefaultRetentionDays)).
// Values are clamped to [MinRetentionDays, MaxRetentionDays].
func EffectiveRetention(eventRetentionDays int, categoryFloor *int) int {
	clamped := clampRetention(eventRetentionDays)
	if categoryFloor == nil {
		return clamped
	}
	floor := clampRetention(*categoryFloor)
	if floor > clamped {
		return floor
	}
	return clamped
}

func clampRetention(days int) int {
	if days < MinRetentionDays {
		return MinRetentionDays
	}
	if days > MaxRetentionDays {
		return MaxRetentionDays
	}
	return days
}

// ValidateRetentionDays returns an error if days is outside [MinRetentionDays, MaxRetentionDays].
// Use at API boundaries before inserting into the database.
func ValidateRetentionDays(days int) error {
	if days < MinRetentionDays || days > MaxRetentionDays {
		return fmt.Errorf("retention_days must be between %d and %d, got %d",
			MinRetentionDays, MaxRetentionDays, days)
	}
	return nil
}

// PurgeSQL is the canonical DELETE statement for retention enforcement.
// It mirrors the Go IsEligibleForPurge logic at the SQL layer.
//
//	DELETE FROM audit_events
//	WHERE event_date + (retention_days * INTERVAL '1 day') < CURRENT_DATE AT TIME ZONE 'UTC'
//	  AND legal_hold = FALSE
const PurgeSQL = `DELETE FROM audit_events
WHERE event_date + (retention_days * INTERVAL '1 day') < $1::DATE
  AND legal_hold = FALSE`

// Clock is re-exported from the partition package for retention worker usage.
type Clock = partition.Clock

// UTCClock is the production clock.
type UTCClock = partition.UTCClock

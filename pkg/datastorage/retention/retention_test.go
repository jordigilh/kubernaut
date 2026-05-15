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

package retention_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/retention"
)

var _ = Describe("Retention Eligibility — Unit Tests", func() {

	// Fixed reference time for all tests
	now := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)

	// UT-DS-485-001: Eligibility predicate — time expired
	// BR-AUDIT-009: Retention policies for audit data
	Describe("UT-DS-485-001: IsEligibleForPurge — time expired baseline", func() {

		It("should return eligible when event_date + retention_days < now (baseline)", func() {
			event := retention.AuditEvent{
				EventDate:     time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
				RetentionDays: 30, // expires 2026-01-31
				LegalHold:     false,
			}
			Expect(retention.IsEligibleForPurge(event, 0, now)).To(BeTrue())
		})

		It("should return NOT eligible when event_date + retention_days >= now", func() {
			event := retention.AuditEvent{
				EventDate:     time.Date(2026, time.April, 5, 0, 0, 0, 0, time.UTC),
				RetentionDays: 30, // expires 2026-05-05 — still in window
				LegalHold:     false,
			}
			Expect(retention.IsEligibleForPurge(event, 0, now)).To(BeFalse())
		})

		It("should handle minimum retention (1 day)", func() {
			event := retention.AuditEvent{
				EventDate:     time.Date(2026, time.April, 8, 0, 0, 0, 0, time.UTC),
				RetentionDays: retention.MinRetentionDays, // 1 day → expires 2026-04-09
				LegalHold:     false,
			}
			Expect(retention.IsEligibleForPurge(event, 0, now)).To(BeTrue())
		})

		It("should handle maximum retention (2555 days = ~7 years)", func() {
			event := retention.AuditEvent{
				EventDate:     time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC),
				RetentionDays: retention.MaxRetentionDays, // 2555 days → expires ~2027-01-01
				LegalHold:     false,
			}
			Expect(retention.IsEligibleForPurge(event, 0, now)).To(BeFalse())
		})

		It("should handle boundary: expires exactly today → NOT eligible (strict <)", func() {
			event := retention.AuditEvent{
				EventDate:     time.Date(2026, time.March, 11, 0, 0, 0, 0, time.UTC),
				RetentionDays: 30, // 2026-03-11 + 30d = 2026-04-10 = now → NOT eligible
				LegalHold:     false,
			}
			Expect(retention.IsEligibleForPurge(event, 0, now)).To(BeFalse())
		})
	})

	// UT-DS-485-002: Legal hold exemption
	// BR-AUDIT-004: Immutability / integrity of audit records
	Describe("UT-DS-485-002: IsEligibleForPurge — legal hold exemption", func() {

		It("should return NOT eligible when legal_hold=true even if time-expired", func() {
			event := retention.AuditEvent{
				EventDate:     time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC),
				RetentionDays: 1, // massively expired
				LegalHold:     true,
			}
			Expect(retention.IsEligibleForPurge(event, 0, now)).To(BeFalse())
		})

		It("should return NOT eligible when legal_hold=true even with default retention", func() {
			event := retention.AuditEvent{
				EventDate:     time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC),
				RetentionDays: retention.DefaultRetentionDays,
				LegalHold:     true,
			}
			Expect(retention.IsEligibleForPurge(event, 0, now)).To(BeFalse())
		})
	})

	// UT-DS-485-003: Category policy merge (MAX semantics)
	// BR-AUDIT-009: Retention policies for audit data
	Describe("UT-DS-485-003: EffectiveRetention — policy precedence", func() {

		It("should use event retention when it exceeds category floor", func() {
			categoryFloor := intPtr(30)
			Expect(retention.EffectiveRetention(90, categoryFloor)).To(Equal(90))
		})

		It("should use category floor when it exceeds event retention", func() {
			categoryFloor := intPtr(365)
			Expect(retention.EffectiveRetention(30, categoryFloor)).To(Equal(365))
		})

		It("should use event retention when both are equal", func() {
			categoryFloor := intPtr(90)
			Expect(retention.EffectiveRetention(90, categoryFloor)).To(Equal(90))
		})
	})

	// UT-DS-485-004: retention_days = 0 → rejected by CHECK constraint (logic layer)
	// BR-AUDIT-009: Retention policies for audit data
	Describe("UT-DS-485-004: EffectiveRetention — constraint boundary (min)", func() {
		It("should clamp retention_days below minimum to MinRetentionDays", func() {
			Expect(retention.EffectiveRetention(0, nil)).To(Equal(retention.MinRetentionDays))
		})

		It("should clamp negative retention_days to MinRetentionDays", func() {
			Expect(retention.EffectiveRetention(-5, nil)).To(Equal(retention.MinRetentionDays))
		})
	})

	// UT-DS-485-005: retention_days = 2556 → exceeds max (logic layer)
	// BR-AUDIT-009: Retention policies for audit data
	Describe("UT-DS-485-005: EffectiveRetention — constraint boundary (max)", func() {
		It("should clamp retention_days above maximum to MaxRetentionDays", func() {
			Expect(retention.EffectiveRetention(2556, nil)).To(Equal(retention.MaxRetentionDays))
		})

		It("should accept exactly MaxRetentionDays", func() {
			Expect(retention.EffectiveRetention(retention.MaxRetentionDays, nil)).To(Equal(retention.MaxRetentionDays))
		})
	})

	// UT-DS-485-006: Category floor overrides shorter per-event retention
	// BR-AUDIT-009: Retention policies for audit data
	Describe("UT-DS-485-006: EffectiveRetention — category floor enforcement", func() {
		It("should apply category floor when event has shorter retention", func() {
			categoryFloor := intPtr(365)
			Expect(retention.EffectiveRetention(30, categoryFloor)).To(Equal(365))
		})

		It("should respect category floor with minimum event retention", func() {
			categoryFloor := intPtr(90)
			Expect(retention.EffectiveRetention(retention.MinRetentionDays, categoryFloor)).To(Equal(90))
		})
	})

	// UT-DS-485-007: NULL category → default retention applies
	// BR-AUDIT-009: Retention policies for audit data
	Describe("UT-DS-485-007: EffectiveRetention — NULL category policy", func() {
		It("should use per-event retention when no category policy exists (nil)", func() {
			Expect(retention.EffectiveRetention(90, nil)).To(Equal(90))
		})

		It("should use MinRetentionDays when event has minimum and no category policy", func() {
			Expect(retention.EffectiveRetention(retention.MinRetentionDays, nil)).To(Equal(retention.MinRetentionDays))
		})
	})

	// Compound: eligibility with category floor
	Describe("IsEligibleForPurge — with category floor applied", func() {
		It("should use category floor when it extends retention beyond expiry", func() {
			event := retention.AuditEvent{
				EventDate:     time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
				RetentionDays: 10, // per-event: expires 2026-03-11
				LegalHold:     false,
			}
			// Category floor = 365 → effective = 365 → expires 2027-02-24 → NOT eligible
			Expect(retention.IsEligibleForPurge(event, 365, now)).To(BeFalse())
		})

		It("should use event retention when it exceeds category floor", func() {
			event := retention.AuditEvent{
				EventDate:     time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC),
				RetentionDays: 365, // effective = 365 → expires 2026-01-01
				LegalHold:     false,
			}
			// Category floor = 30 → effective = 365 (event wins) → eligible
			Expect(retention.IsEligibleForPurge(event, 30, now)).To(BeTrue())
		})
	})
})

func intPtr(v int) *int {
	return &v
}

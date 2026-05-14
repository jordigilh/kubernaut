/*
Copyright 2026 Jordi Gil.

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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/retention"
)

// ========================================
// PHASE 9C-RED: Retention Category Floor Tests
// ========================================
//
// Issue: #1088 GA Readiness — FED-H1, FED-M3
// File Under Test: pkg/datastorage/retention/retention.go
//
// FED-H1: Purge must use GREATEST(event.retention_days, category_floor)
// FED-M3: defaultDays config should be used as the category floor fallback
// ========================================

var _ = Describe("Phase 9C: Retention Category Floors (FED-H1, FED-M3)", func() {

	Describe("UT-DS-1088-GA-050: PurgeSQLBatched uses GREATEST semantics", func() {
		It("should use EffectiveRetention in SQL WHERE clause", func() {
			// FED-H1: The batched purge SQL must apply MAX(event.retention_days, category_floor)
			// PurgeSQLBatched must include GREATEST() or equivalent logic
			Expect(retention.PurgeSQLBatched).To(ContainSubstring("GREATEST"),
				"PurgeSQLBatched should use GREATEST(retention_days, $3) to enforce category floor")
		})
	})

	Describe("UT-DS-1088-GA-051: EffectiveRetention applied in purge eligibility", func() {
		It("should not purge event with 30d retention when category floor is 365d", func() {
			// FED-H1: An event with retention_days=30 but category floor=365
			// must NOT be eligible for purge until 365 days have passed
			now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
			event := retention.AuditEvent{
				EventDate:     time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), // 31 days ago
				RetentionDays: 30,                                           // per-event: 30 days
				LegalHold:     false,
			}
			categoryFloor := 365 // category policy: 1 year minimum

			eligible := retention.IsEligibleForPurge(event, categoryFloor, now)
			Expect(eligible).To(BeFalse(),
				"event with 30d retention should NOT be purged when category floor is 365d")
		})

		It("should purge event when both event retention and category floor are exceeded", func() {
			now := time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC)
			event := retention.AuditEvent{
				EventDate:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), // 2 years ago
				RetentionDays: 30,
				LegalHold:     false,
			}
			categoryFloor := 365

			eligible := retention.IsEligibleForPurge(event, categoryFloor, now)
			Expect(eligible).To(BeTrue(),
				"event should be purged when both event retention (30d) and category floor (365d) are exceeded")
		})
	})
})

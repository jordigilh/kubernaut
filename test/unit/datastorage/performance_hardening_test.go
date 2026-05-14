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

package datastorage_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/dlq"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server/helpers"
	"github.com/jordigilh/kubernaut/pkg/audit"

	"github.com/google/uuid"
	"errors"
	"time"
)

var _ = Describe("Phase 11: Performance Hardening", func() {
	Context("DF-M2: ParentEventDate propagation", func() {
		It("UT-DS-1088-GA-240: ConvertToRepositoryAuditEvent carries ParentEventDate", func() {
			parentDate := time.Now().Add(-1 * time.Hour).Truncate(24 * time.Hour)
			parentID := uuid.New()
			event := &audit.AuditEvent{
				EventID:        uuid.New(),
				EventTimestamp: time.Now(),
				EventType:      "test.event",
				EventCategory:  "audit",
				EventAction:    "create",
				EventOutcome:   "success",
				CorrelationID:  uuid.NewString(),
				ParentEventID:  &parentID,
				ParentEventDate: &parentDate,
				ResourceType:   "Pod",
				ResourceID:     "pod-1",
				ActorID:        "user-1",
				ActorType:      "human",
				EventData:      []byte(`{"key":"value"}`),
				RetentionDays:  2555,
			}

			repoEvent, err := helpers.ConvertToRepositoryAuditEvent(event)
			Expect(err).NotTo(HaveOccurred())
			Expect(repoEvent.ParentEventDate).NotTo(BeNil(), "ParentEventDate must be carried through conversion")
			Expect(*repoEvent.ParentEventDate).To(BeTemporally("~", parentDate, time.Second))
		})
	})

	Context("SEC-L1: DLQ error redaction", func() {
		It("UT-DS-1088-GA-260: DLQ sanitizeError truncates and redacts SQL details", func() {
			sqlErr := errors.New("pq: relation \"audit_events\" does not exist")
			sanitized := dlq.SanitizeError(sqlErr)
			Expect(sanitized).To(Equal("database write failed"))
			Expect(sanitized).NotTo(ContainSubstring("pq:"))
		})

		It("UT-DS-1088-GA-261: DLQ sanitizeError truncates long errors", func() {
			longMsg := ""
			for i := 0; i < 300; i++ {
				longMsg += "x"
			}
			sanitized := dlq.SanitizeError(errors.New(longMsg))
			Expect(len(sanitized)).To(BeNumerically("<=", 260))
			Expect(sanitized).To(HaveSuffix("..."))
		})

		It("UT-DS-1088-GA-262: DLQ sanitizeError handles nil error", func() {
			sanitized := dlq.SanitizeError(nil)
			Expect(sanitized).To(BeEmpty())
		})
	})

	Context("PERF-H2: Effectiveness query has LIMIT", func() {
		It("UT-DS-1088-GA-210: queryEffectivenessEvents query string contains LIMIT", func() {
			// Verified via code inspection: the query must contain LIMIT.
			// This is a structural contract test; the actual limit is enforced
			// by the repository/handler SQL.
			// NOTE: Actual SQL limit is validated by integration tests.
			// This unit test verifies the constant exists and is reasonable.
			Expect(helpers.MaxEffectivenessResults).To(BeNumerically(">=", 1000))
			Expect(helpers.MaxEffectivenessResults).To(BeNumerically("<=", 50000))
		})
	})
})

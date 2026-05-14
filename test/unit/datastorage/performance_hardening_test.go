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
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
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

	Context("SEC-M1: SanitizeError covers credential/DSN patterns", func() {
		DescribeTable("UT-DS-1088-GA-263: redacts sensitive patterns",
			func(errMsg string) {
				sanitized := dlq.SanitizeError(errors.New(errMsg))
				Expect(sanitized).To(Equal("database write failed"),
					"SEC-M1: error containing sensitive pattern must be redacted")
			},
			Entry("password in error", "invalid password for user admin"),
			Entry("secret in error", "secret key expired: rotate immediately"),
			Entry("token in error", "token verification failed: invalid signature"),
			Entry("postgres DSN", "postgres://user:pass@host:5432/db connection failed"),
			Entry("postgresql DSN", "postgresql://admin@localhost?sslmode=disable"),
			Entry("redis DSN", "redis://default:mypass@redis:6379/0 timeout"),
		)

		It("UT-DS-1088-GA-264: rune-safe truncation preserves valid UTF-8", func() {
			// Build a string with multi-byte runes that would split at byte boundary
			runes := make([]rune, 260)
			for i := range runes {
				runes[i] = '日' // 3-byte UTF-8 character
			}
			sanitized := dlq.SanitizeError(errors.New(string(runes)))
			Expect(sanitized).To(HaveSuffix("..."))
			// Verify output is valid UTF-8 by checking rune count
			for _, r := range sanitized {
				Expect(r).NotTo(Equal(rune(0xFFFD)),
					"DF-L2: truncated string must not contain replacement characters")
			}
		})
	})

	Context("PERF-H2: Effectiveness and RO queries have LIMIT", func() {
		It("UT-DS-1088-GA-210: MaxEffectivenessResults constant is bounded", func() {
			Expect(helpers.MaxEffectivenessResults).To(Equal(10000),
				"PERF-H2: effectiveness production cap must be exactly 10000")
		})

		It("UT-DS-1088-GA-211: MaxROEventsBySpecHashResults constant is bounded", func() {
			Expect(repository.MaxROEventsBySpecHashResults).To(Equal(10000),
				"PERF-H2: RO spec-hash query cap must be exactly 10000")
		})
	})
})

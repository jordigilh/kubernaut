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

package datastorage

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
// - Unit tests (70%+): Business logic in isolation
// - Integration tests (>50%): Infrastructure interaction, microservices coordination
// - E2E tests (10-15%): Complete workflow validation
//
// BR-STORAGE-032: Unified audit trail for compliance and cross-service correlation
//
// BEHAVIOR TESTING PRINCIPLES:
// - Test WHAT the system does, not HOW it does it
// - Tests should not break when implementation changes
// - Focus on business outcomes, not PostgreSQL internals

var _ = Describe("Audit Events Schema Integration Tests", func() {
	BeforeEach(func() {

		// Clean up test data before each test
		_, err := db.Exec("DELETE FROM audit_events WHERE correlation_id LIKE 'test-%'")
		if err != nil {
			GinkgoWriter.Printf("Note: cleanup skipped (table may not exist): %v\n", err)
		}
	})

	Context("BR-STORAGE-032: Audit Event Storage", func() {
		// ================================================================
		// BEHAVIOR: System can store audit events
		// ================================================================
		It("should store audit events with all required fields", func() {
			eventID := uuid.New().String()
			eventTimestamp := time.Now().UTC()
			eventDate := eventTimestamp.Truncate(24 * time.Hour)

			_, err := db.Exec(`
				INSERT INTO audit_events (
					event_id, event_timestamp, event_date, event_type, event_category, correlation_id,
					resource_type, resource_id, event_action, event_outcome, actor_type, actor_id, event_data
				) VALUES (
					$1, $2, $3, 'gateway.signal.received', 'gateway', 'test-store-001',
					'remediationrequest', 'rr-001', 'receive_signal', 'success', 'service', 'gateway-service',
					'{"version": "1.0", "status": "success"}'::jsonb
				)
			`, eventID, eventTimestamp, eventDate)

			Expect(err).ToNot(HaveOccurred(), "Should store audit event successfully")

			// Verify we can retrieve it
			var retrievedID string
			err = db.QueryRow(`SELECT event_id FROM audit_events WHERE event_id = $1`, eventID).Scan(&retrievedID)
			Expect(err).ToNot(HaveOccurred())
			Expect(retrievedID).To(Equal(eventID))
		})

		// ================================================================
		// BEHAVIOR: System can store events for current and future months
		// ================================================================
		It("should accept audit events for current month", func() {
			eventID := uuid.New().String()
			now := time.Now().UTC()

			_, err := db.Exec(`
				INSERT INTO audit_events (
					event_id, event_timestamp, event_date, event_type, event_category, correlation_id,
					resource_type, resource_id, event_action, event_outcome, actor_type, actor_id, event_data
				) VALUES ($1, $2, $3, 'test.current.month', 'test', 'test-current-month',
					'test', 'test-001', 'test', 'success', 'service', 'test', '{}'::jsonb)
			`, eventID, now, now.Truncate(24*time.Hour))

			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("Should accept audit events for %s", now.Format("January 2006")))
		})

		It("should accept audit events for next 3 months", func() {
			now := time.Now().UTC()

			for i := 1; i <= 3; i++ {
				futureMonth := now.AddDate(0, i, 0)
				eventID := uuid.New().String()
				eventDate := time.Date(futureMonth.Year(), futureMonth.Month(), 15, 12, 0, 0, 0, time.UTC)

				_, err := db.Exec(`
					INSERT INTO audit_events (
						event_id, event_timestamp, event_date, event_type, event_category, correlation_id,
						resource_type, resource_id, event_action, event_outcome, actor_type, actor_id, event_data
					) VALUES ($1, $2, $3, 'test.future.month', 'test', $4,
						'test', 'test-001', 'test', 'success', 'service', 'test', '{}'::jsonb)
				`, eventID, eventDate, eventDate.Truncate(24*time.Hour),
					fmt.Sprintf("test-future-month-%d", i))

				Expect(err).ToNot(HaveOccurred(),
					fmt.Sprintf("Should accept audit events for %s (+%d months)",
						futureMonth.Format("January 2006"), i))
			}
		})

		// ================================================================
		// BEHAVIOR: System can query events by correlation ID
		// ================================================================
		It("should retrieve events by correlation_id efficiently", func() {
			correlationID := "test-correlation-query-001"

			// Insert multiple events with same correlation ID
			for i := 0; i < 3; i++ {
				eventID := uuid.New().String()
				now := time.Now().UTC()
				_, err := db.Exec(`
					INSERT INTO audit_events (
						event_id, event_timestamp, event_date, event_type, event_category, correlation_id,
						resource_type, resource_id, event_action, event_outcome, actor_type, actor_id, event_data
					) VALUES ($1, $2, $3, $4, 'test', $5,
						'test', 'test-001', 'test', 'success', 'service', 'test', '{}'::jsonb)
				`, eventID, now, now.Truncate(24*time.Hour),
					fmt.Sprintf("test.event.%d", i), correlationID)
				Expect(err).ToNot(HaveOccurred())
			}

			// Query by correlation ID
			rows, err := db.Query(`
				SELECT event_id, event_type FROM audit_events
				WHERE correlation_id = $1
				ORDER BY event_timestamp
			`, correlationID)
			Expect(err).ToNot(HaveOccurred())
			defer rows.Close()

			var count int
			for rows.Next() {
				var id, eventType string
				Expect(rows.Scan(&id, &eventType)).To(Succeed())
				count++
			}
			Expect(count).To(Equal(3), "Should retrieve all events with matching correlation_id")
		})

		// ================================================================
		// BEHAVIOR: System can query JSONB event_data
		// ================================================================
		It("should support JSONB queries on event_data", func() {
			eventID := uuid.New().String()
			now := time.Now().UTC()

			_, err := db.Exec(`
				INSERT INTO audit_events (
					event_id, event_timestamp, event_date, event_type, event_category, correlation_id,
					resource_type, resource_id, event_action, event_outcome, actor_type, actor_id, event_data
				) VALUES ($1, $2, $3, 'ai.analysis.completed', 'aianalysis', 'test-jsonb-query',
					'investigation', 'inv-001', 'analyze', 'success', 'service', 'analysis-service',
					'{"analysis": {"confidence": 0.95, "model": "gpt-4"}}'::jsonb)
			`, eventID, now, now.Truncate(24*time.Hour))
			Expect(err).ToNot(HaveOccurred())

			// Query using JSONB containment
			var foundID string
			err = db.QueryRow(`
				SELECT event_id FROM audit_events
				WHERE event_data @> '{"analysis": {"confidence": 0.95}}'
				AND correlation_id = 'test-jsonb-query'
			`).Scan(&foundID)
			Expect(err).ToNot(HaveOccurred())
			Expect(foundID).To(Equal(eventID), "JSONB query should find matching event")
		})

		// ================================================================
		// BEHAVIOR: Parent-child relationships are enforced (immutability)
		// ================================================================
		It("should prevent deletion of parent events with children (immutability)", func() {
			parentID := uuid.New().String()
			childID := uuid.New().String()
			now := time.Now().UTC()
			eventDate := now.Truncate(24 * time.Hour)

			// Insert parent event
			_, err := db.Exec(`
				INSERT INTO audit_events (
					event_id, event_timestamp, event_date, event_type, event_category, correlation_id,
					resource_type, resource_id, event_action, event_outcome, actor_type, actor_id, event_data
				) VALUES ($1, $2, $3, 'gateway.signal.received', 'gateway', 'test-immutability',
					'alert', 'alert-001', 'receive', 'success', 'service', 'gateway-service', '{}'::jsonb)
			`, parentID, now, eventDate)
			Expect(err).ToNot(HaveOccurred())

			// Insert child event referencing parent
			_, err = db.Exec(`
				INSERT INTO audit_events (
					event_id, event_timestamp, event_date, event_type, event_category, correlation_id,
					parent_event_id, parent_event_date, resource_type, resource_id, event_action,
					event_outcome, actor_type, actor_id, event_data
				) VALUES ($1, $2, $3, 'ai.investigation.started', 'aianalysis', 'test-immutability',
					$4, $5, 'investigation', 'inv-001', 'start', 'success', 'service', 'analysis-service', '{}'::jsonb)
			`, childID, now.Add(time.Second), eventDate, parentID, eventDate)
			Expect(err).ToNot(HaveOccurred())

			// Attempt to delete parent - should fail
			_, err = db.Exec(`DELETE FROM audit_events WHERE event_id = $1`, parentID)
			Expect(err).To(HaveOccurred(), "Deleting parent with children should fail")
			Expect(err.Error()).To(ContainSubstring("foreign key"),
				"Error should indicate FK constraint violation")

			// Verify parent still exists
			var exists bool
			err = db.QueryRow(`SELECT EXISTS(SELECT 1 FROM audit_events WHERE event_id = $1)`, parentID).Scan(&exists)
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeTrue(), "Parent event should still exist (immutability enforced)")
		})

		// ================================================================
		// BEHAVIOR: Child events correctly reference parents
		// ================================================================
		It("should maintain parent-child relationships", func() {
			parentID := uuid.New().String()
			childID := uuid.New().String()
			now := time.Now().UTC()
			eventDate := now.Truncate(24 * time.Hour)

			// Insert parent
			_, err := db.Exec(`
				INSERT INTO audit_events (
					event_id, event_timestamp, event_date, event_type, event_category, correlation_id,
					resource_type, resource_id, event_action, event_outcome, actor_type, actor_id, event_data
				) VALUES ($1, $2, $3, 'parent.event', 'test', 'test-parent-child',
					'test', 'test-001', 'test', 'success', 'service', 'test', '{}'::jsonb)
			`, parentID, now, eventDate)
			Expect(err).ToNot(HaveOccurred())

			// Insert child referencing parent
			_, err = db.Exec(`
				INSERT INTO audit_events (
					event_id, event_timestamp, event_date, event_type, event_category, correlation_id,
					parent_event_id, parent_event_date, resource_type, resource_id, event_action,
					event_outcome, actor_type, actor_id, event_data
				) VALUES ($1, $2, $3, 'child.event', 'test', 'test-parent-child',
					$4, $5, 'test', 'test-001', 'test', 'success', 'service', 'test', '{}'::jsonb)
			`, childID, now.Add(time.Second), eventDate, parentID, eventDate)
			Expect(err).ToNot(HaveOccurred())

			// Verify relationship
			var retrievedParentID sql.NullString
			err = db.QueryRow(`SELECT parent_event_id FROM audit_events WHERE event_id = $1`, childID).Scan(&retrievedParentID)
			Expect(err).ToNot(HaveOccurred())
			Expect(retrievedParentID.Valid).To(BeTrue())
			Expect(retrievedParentID.String).To(Equal(parentID))
		})

		// ================================================================
		// BEHAVIOR: Event date is correctly stored
		// ================================================================
		It("should store and retrieve event_date correctly", func() {
			eventID := uuid.New().String()
			testTimestamp := time.Date(2025, 11, 15, 10, 30, 0, 0, time.UTC)
			testDate := testTimestamp.Truncate(24 * time.Hour)

			_, err := db.Exec(`
				INSERT INTO audit_events (
					event_id, event_timestamp, event_date, event_type, event_category, correlation_id,
					resource_type, resource_id, event_action, event_outcome, actor_type, actor_id, event_data
				) VALUES ($1, $2, $3, 'test.date.check', 'test', 'test-date-check',
					'test', 'test-001', 'test', 'success', 'service', 'test', '{}'::jsonb)
			`, eventID, testTimestamp, testDate)
			Expect(err).ToNot(HaveOccurred())

			var retrievedDate time.Time
			err = db.QueryRow(`SELECT event_date FROM audit_events WHERE event_id = $1`, eventID).Scan(&retrievedDate)
			Expect(err).ToNot(HaveOccurred())

			Expect(retrievedDate.Format("2006-01-02")).To(Equal(testDate.Format("2006-01-02")),
				"event_date should match the stored date")
		})
	})
})

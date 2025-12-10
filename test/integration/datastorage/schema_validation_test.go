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
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-STORAGE-003: Database Schema Validation (Integration Tests)
//
// BEHAVIOR TESTING PRINCIPLES:
// - Test WHAT the system does, not HOW it does it
// - Validate that we can INSERT and SELECT data with expected fields
// - Tests should not break when implementation changes (column types, index names)
// - Focus on business outcomes: "Can we store and retrieve notification audits?"

var _ = Describe("BR-STORAGE-003: Notification Audit Storage", Serial, Ordered, func() {
	BeforeEach(func() {
		// Clean up test data
		_, _ = db.Exec("DELETE FROM notification_audit WHERE notification_id LIKE 'test-%'")
	})

	Context("Storing Notification Audits", func() {
		It("should store a notification audit with all required fields", func() {
			notificationID := fmt.Sprintf("test-%s", uuid.New().String())

			_, err := db.Exec(`
				INSERT INTO notification_audit (
					remediation_id, notification_id, recipient, channel,
					message_summary, status, sent_at
				) VALUES ($1, $2, $3, $4, $5, $6, $7)
			`, "remediation-001", notificationID, "user@example.com", "email",
				"Test notification message", "sent", time.Now())

			Expect(err).ToNot(HaveOccurred(), "Should store notification audit")

			// Verify we can retrieve it
			var retrievedID string
			err = db.QueryRow(`
				SELECT notification_id FROM notification_audit WHERE notification_id = $1
			`, notificationID).Scan(&retrievedID)
			Expect(err).ToNot(HaveOccurred())
			Expect(retrievedID).To(Equal(notificationID))
		})

		It("should store optional fields (delivery_status, error_message, escalation_level)", func() {
			notificationID := fmt.Sprintf("test-%s", uuid.New().String())

			_, err := db.Exec(`
				INSERT INTO notification_audit (
					remediation_id, notification_id, recipient, channel,
					message_summary, status, sent_at,
					delivery_status, error_message, escalation_level
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			`, "remediation-002", notificationID, "user@example.com", "slack",
				"Test with optional fields", "failed", time.Now(),
				"bounced", "Recipient not found", 2)

			Expect(err).ToNot(HaveOccurred(), "Should store notification audit with optional fields")

			// Verify optional fields are stored
			var deliveryStatus, errorMessage string
			var escalationLevel int
			err = db.QueryRow(`
				SELECT delivery_status, error_message, escalation_level
				FROM notification_audit WHERE notification_id = $1
			`, notificationID).Scan(&deliveryStatus, &errorMessage, &escalationLevel)
			Expect(err).ToNot(HaveOccurred())
			Expect(deliveryStatus).To(Equal("bounced"))
			Expect(errorMessage).To(Equal("Recipient not found"))
			Expect(escalationLevel).To(Equal(2))
		})

		It("should enforce unique notification_id", func() {
			notificationID := fmt.Sprintf("test-unique-%s", uuid.New().String())

			// First insert should succeed
			_, err := db.Exec(`
				INSERT INTO notification_audit (
					remediation_id, notification_id, recipient, channel,
					message_summary, status, sent_at
				) VALUES ($1, $2, $3, $4, $5, $6, $7)
			`, "remediation-003", notificationID, "user@example.com", "email",
				"First notification", "sent", time.Now())
			Expect(err).ToNot(HaveOccurred())

			// Second insert with same notification_id should fail
			_, err = db.Exec(`
				INSERT INTO notification_audit (
					remediation_id, notification_id, recipient, channel,
					message_summary, status, sent_at
				) VALUES ($1, $2, $3, $4, $5, $6, $7)
			`, "remediation-004", notificationID, "other@example.com", "slack",
				"Duplicate notification", "sent", time.Now())
			Expect(err).To(HaveOccurred(), "Duplicate notification_id should fail")
			Expect(err.Error()).To(ContainSubstring("duplicate key"),
				"Error should indicate unique constraint violation")
		})
	})

	Context("Querying Notification Audits", func() {
		It("should query by remediation_id efficiently", func() {
			remediationID := fmt.Sprintf("test-rem-%s", uuid.New().String())

			// Insert multiple notifications for same remediation
			for i := 0; i < 3; i++ {
				notificationID := fmt.Sprintf("test-%s-%d", uuid.New().String(), i)
				_, err := db.Exec(`
					INSERT INTO notification_audit (
						remediation_id, notification_id, recipient, channel,
						message_summary, status, sent_at
					) VALUES ($1, $2, $3, $4, $5, $6, $7)
				`, remediationID, notificationID, fmt.Sprintf("user%d@example.com", i),
					"email", "Notification message", "sent", time.Now())
				Expect(err).ToNot(HaveOccurred())
			}

			// Query by remediation_id
			rows, err := db.Query(`
				SELECT notification_id FROM notification_audit
				WHERE remediation_id = $1
			`, remediationID)
			Expect(err).ToNot(HaveOccurred())
			defer rows.Close()

			var count int
			for rows.Next() {
				count++
			}
			Expect(count).To(Equal(3), "Should retrieve all notifications for remediation")
		})

		It("should query by channel", func() {
			// Insert notifications with different channels
			for _, channel := range []string{"email", "slack", "pagerduty"} {
				notificationID := fmt.Sprintf("test-channel-%s-%s", channel, uuid.New().String())
				_, err := db.Exec(`
					INSERT INTO notification_audit (
						remediation_id, notification_id, recipient, channel,
						message_summary, status, sent_at
					) VALUES ($1, $2, $3, $4, $5, $6, $7)
				`, "remediation-channel-test", notificationID, "user@example.com",
					channel, "Channel test", "sent", time.Now())
				Expect(err).ToNot(HaveOccurred())
			}

			// Query by channel
			var count int
			err := db.QueryRow(`
				SELECT COUNT(*) FROM notification_audit
				WHERE channel = 'slack' AND notification_id LIKE 'test-channel-%'
			`).Scan(&count)
			Expect(err).ToNot(HaveOccurred())
			Expect(count).To(BeNumerically(">=", 1), "Should find slack notifications")
		})

		It("should query by status", func() {
			// Insert notifications with different statuses
			// Valid statuses per migration 010: 'sent', 'failed', 'acknowledged', 'escalated'
			for _, status := range []string{"sent", "failed", "acknowledged"} {
				notificationID := fmt.Sprintf("test-status-%s-%s", status, uuid.New().String())
				_, err := db.Exec(`
					INSERT INTO notification_audit (
						remediation_id, notification_id, recipient, channel,
						message_summary, status, sent_at
					) VALUES ($1, $2, $3, $4, $5, $6, $7)
				`, "remediation-status-test", notificationID, "user@example.com",
					"email", "Status test", status, time.Now())
				Expect(err).ToNot(HaveOccurred())
			}

			// Query by status
			var count int
			err := db.QueryRow(`
				SELECT COUNT(*) FROM notification_audit
				WHERE status = 'failed' AND notification_id LIKE 'test-status-%'
			`).Scan(&count)
			Expect(err).ToNot(HaveOccurred())
			Expect(count).To(BeNumerically(">=", 1), "Should find failed notifications")
		})
	})

	Context("Timestamp Handling", func() {
		It("should auto-populate created_at and updated_at", func() {
			notificationID := fmt.Sprintf("test-timestamps-%s", uuid.New().String())
			sentAt := time.Now().Add(-1 * time.Hour)

			_, err := db.Exec(`
				INSERT INTO notification_audit (
					remediation_id, notification_id, recipient, channel,
					message_summary, status, sent_at
				) VALUES ($1, $2, $3, $4, $5, $6, $7)
			`, "remediation-timestamps", notificationID, "user@example.com",
				"email", "Timestamp test", "sent", sentAt)
			Expect(err).ToNot(HaveOccurred())

			// Verify timestamps are populated
			var createdAt, updatedAt time.Time
			err = db.QueryRow(`
				SELECT created_at, updated_at FROM notification_audit
				WHERE notification_id = $1
			`, notificationID).Scan(&createdAt, &updatedAt)
			Expect(err).ToNot(HaveOccurred())
			Expect(createdAt).ToNot(BeZero(), "created_at should be populated")
			Expect(updatedAt).ToNot(BeZero(), "updated_at should be populated")
		})
	})
})

var _ = Describe("BR-STORAGE-003: Resource Action Traces Storage", Serial, Ordered, func() {
	// CRITICAL: This test validates that we can store ADR-033 multi-dimensional tracking data
	// The column names (execution_status, not status) are business-critical

	BeforeEach(func() {
		// Clean up test data
		_, _ = db.Exec("DELETE FROM resource_action_traces WHERE action_id LIKE 'test-%'")
	})

	Context("Storing Action Traces", func() {
		// Helper to create required parent records with unique names
		var createTestActionHistory = func() int64 {
			now := time.Now()
			// resource_uid is VARCHAR(36), exactly UUID length
			resourceUID := uuid.New().String()
			// name must be unique per (namespace, kind, name) constraint
			resourceName := fmt.Sprintf("test-deploy-%s", resourceUID[:8])

			// Create resource_references first (required by action_histories FK)
			var resourceID int64
			err := db.QueryRow(`
				INSERT INTO resource_references (
					resource_uid, api_version, kind, name, namespace
				) VALUES ($1, $2, $3, $4, $5)
				RETURNING id
			`, resourceUID, "apps/v1", "Deployment", resourceName, "default").Scan(&resourceID)
			Expect(err).ToNot(HaveOccurred())

			// Create action_histories (required by resource_action_traces FK)
			var actionHistoryID int64
			err = db.QueryRow(`
				INSERT INTO action_histories (resource_id) VALUES ($1) RETURNING id
			`, resourceID).Scan(&actionHistoryID)
			Expect(err).ToNot(HaveOccurred())

			// Update last_action_at
			_, err = db.Exec(`UPDATE action_histories SET last_action_at = $1 WHERE id = $2`, now, actionHistoryID)
			Expect(err).ToNot(HaveOccurred())

			return actionHistoryID
		}

		It("should store action trace with all ADR-033 required fields", func() {
			// action_id is VARCHAR(64), use short prefix + UUID (36 chars)
			actionID := uuid.New().String()
			now := time.Now()
			actionHistoryID := createTestActionHistory()

			// Insert the action trace
			_, err := db.Exec(`
				INSERT INTO resource_action_traces (
					action_history_id, action_id, action_type, action_timestamp,
					execution_status, signal_name, signal_severity,
					model_used, model_confidence, incident_type,
					workflow_id, workflow_version, ai_selected_workflow, ai_chained_workflows
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
			`, actionHistoryID, actionID, "scale_deployment", now,
				"success", "OOMKilled", "critical",
				"gpt-4", 0.95, "memory_pressure",
				"wf-001", "v1.0.0", true, false)

			Expect(err).ToNot(HaveOccurred(), "Should store action trace with ADR-033 fields")

			// Verify we can retrieve it
			var retrievedActionID string
			err = db.QueryRow(`
				SELECT action_id FROM resource_action_traces WHERE action_id = $1
			`, actionID).Scan(&retrievedActionID)
			Expect(err).ToNot(HaveOccurred())
			Expect(retrievedActionID).To(Equal(actionID))
		})

		It("should store effectiveness tracking fields", func() {
			// action_id is VARCHAR(64), use UUID only
			actionID := uuid.New().String()
			now := time.Now()
			actionHistoryID := createTestActionHistory()

			// Insert with effectiveness fields
			_, err := db.Exec(`
				INSERT INTO resource_action_traces (
					action_history_id, action_id, action_type, action_timestamp,
					execution_status, signal_name, signal_severity,
					model_used, model_confidence,
					effectiveness_score, effectiveness_assessment_method
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			`, actionHistoryID, actionID, "restart_pod", now,
				"success", "CrashLoopBackOff", "high",
				"gpt-4", 0.88,
				0.95, "symptom_resolution")

			Expect(err).ToNot(HaveOccurred(), "Should store effectiveness tracking fields")

			// Verify effectiveness fields
			var effectivenessScore float64
			var assessmentMethod string
			err = db.QueryRow(`
				SELECT effectiveness_score, effectiveness_assessment_method
				FROM resource_action_traces WHERE action_id = $1
			`, actionID).Scan(&effectivenessScore, &assessmentMethod)
			Expect(err).ToNot(HaveOccurred())
			Expect(effectivenessScore).To(BeNumerically("~", 0.95, 0.01))
			Expect(assessmentMethod).To(Equal("symptom_resolution"))
		})
	})

	Context("Querying Action Traces", func() {
		// Helper to create required parent records with unique names
		var createQueryTestActionHistory = func() int64 {
			// resource_uid is VARCHAR(36), exactly UUID length
			resourceUID := uuid.New().String()
			// name must be unique per (namespace, kind, name) constraint
			resourceName := fmt.Sprintf("test-query-%s", resourceUID[:8])

			var resourceID int64
			err := db.QueryRow(`
				INSERT INTO resource_references (
					resource_uid, api_version, kind, name, namespace
				) VALUES ($1, $2, $3, $4, $5)
				RETURNING id
			`, resourceUID, "apps/v1", "Deployment", resourceName, "default").Scan(&resourceID)
			Expect(err).ToNot(HaveOccurred())

			var actionHistoryID int64
			err = db.QueryRow(`
				INSERT INTO action_histories (resource_id) VALUES ($1) RETURNING id
			`, resourceID).Scan(&actionHistoryID)
			Expect(err).ToNot(HaveOccurred())

			return actionHistoryID
		}

		It("should query by execution_status", func() {
			now := time.Now()
			actionHistoryID := createQueryTestActionHistory()

			// Insert traces with different statuses
			for _, status := range []string{"success", "failed", "pending"} {
				// action_id is VARCHAR(64), use short prefix + UUID[:8]
				actionID := fmt.Sprintf("stat-%s-%s", status[:4], uuid.New().String()[:8])
				_, err := db.Exec(`
					INSERT INTO resource_action_traces (
						action_history_id, action_id, action_type, action_timestamp,
						execution_status, signal_name, signal_severity,
						model_used, model_confidence
					) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
				`, actionHistoryID, actionID, "test_action", now,
					status, "TestSignal", "medium",
					"test-model", 0.9)
				Expect(err).ToNot(HaveOccurred())
			}

			// Query by execution_status
			var count int
			err := db.QueryRow(`
				SELECT COUNT(*) FROM resource_action_traces
				WHERE execution_status = 'success'
				AND action_id LIKE 'stat-succ-%'
			`).Scan(&count)
			Expect(err).ToNot(HaveOccurred())
			Expect(count).To(BeNumerically(">=", 1), "Should find traces with success status")
		})

		It("should query by incident_type for ADR-033 aggregations", func() {
			now := time.Now()
			actionHistoryID := createQueryTestActionHistory()

			// Insert traces with different incident types
			for _, incidentType := range []string{"memory_pressure", "cpu_throttling", "network_issue"} {
				// action_id is VARCHAR(64), use short prefix + UUID[:8]
				actionID := fmt.Sprintf("inc-%s", uuid.New().String()[:8])
				_, err := db.Exec(`
					INSERT INTO resource_action_traces (
						action_history_id, action_id, action_type, action_timestamp,
						execution_status, signal_name, signal_severity,
						model_used, model_confidence, incident_type
					) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
				`, actionHistoryID, actionID, "test_action", now,
					"success", "TestSignal", "high",
					"gpt-4", 0.9, incidentType)
				Expect(err).ToNot(HaveOccurred())
			}

			// Query by incident_type
			var count int
			err := db.QueryRow(`
				SELECT COUNT(*) FROM resource_action_traces
				WHERE incident_type = 'memory_pressure'
				AND action_id LIKE 'inc-%'
			`).Scan(&count)
			Expect(err).ToNot(HaveOccurred())
			Expect(count).To(BeNumerically(">=", 1), "Should find traces with memory_pressure incident type")
		})
	})

	Context("Partitioning Behavior", func() {
		It("should accept data for current month", func() {
			now := time.Now()
			// resource_uid is VARCHAR(36), exactly UUID length
			resourceUID := uuid.New().String()
			// name must be unique per (namespace, kind, name) constraint
			resourceName := fmt.Sprintf("test-part-%s", resourceUID[:8])

			// Create resource_references
			var resourceID int64
			err := db.QueryRow(`
				INSERT INTO resource_references (
					resource_uid, api_version, kind, name, namespace
				) VALUES ($1, $2, $3, $4, $5)
				RETURNING id
			`, resourceUID, "apps/v1", "Deployment", resourceName, "default").Scan(&resourceID)
			Expect(err).ToNot(HaveOccurred())

			// Create action_histories
			var actionHistoryID int64
			err = db.QueryRow(`
				INSERT INTO action_histories (resource_id) VALUES ($1) RETURNING id
			`, resourceID).Scan(&actionHistoryID)
			Expect(err).ToNot(HaveOccurred())

			// action_id is VARCHAR(64), use UUID only
			actionID := uuid.New().String()
			_, err = db.Exec(`
				INSERT INTO resource_action_traces (
					action_history_id, action_id, action_type, action_timestamp,
					execution_status, signal_name, signal_severity,
					model_used, model_confidence
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			`, actionHistoryID, actionID, "test_action", now,
				"success", "TestSignal", "low",
				"test-model", 0.85)

			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("Should accept data for %s", now.Format("January 2006")))
		})
	})
})

var _ = Describe("BR-STORAGE-003: pgvector Extension", Serial, func() {
	Context("Vector Operations", func() {
		It("should support vector similarity search", func() {
			// This test validates that pgvector is working, not the extension name
			// We test behavior: can we store and search vectors?

			// Skip if workflow catalog table doesn't exist
			var exists bool
			err := db.QueryRow(`
				SELECT EXISTS (
					SELECT FROM pg_tables
					WHERE tablename = 'remediation_workflow_catalog'
				)
			`).Scan(&exists)
			if err != nil || !exists {
				Skip("remediation_workflow_catalog table not available")
			}

			// Test that we can perform vector operations
			// This is a behavior test - we don't care about extension details
			_, err = db.Exec(`
				SELECT '[1,2,3]'::vector <-> '[4,5,6]'::vector AS distance
			`)
			Expect(err).ToNot(HaveOccurred(), "Should support vector distance operations")
		})
	})
})

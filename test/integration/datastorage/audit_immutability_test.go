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

// IT-DS-SOC2-AU9: Audit event immutability integration tests
// BR-AUDIT-004: Immutability / integrity of audit records
// FedRAMP AU-9: Protection of Audit Information
// SOC2 CC8.1: Tamper-evident audit trail
//
// These tests verify that the PostgreSQL schema enforces immutability
// constraints required by SOC2 and FedRAMP compliance:
// 1. Duplicate event_id rejection (PK constraint)
// 2. UPDATE protection on critical audit fields (trigger-based)

var _ = Describe("IT-DS-SOC2-AU9: Audit Event Immutability [AU-9, CC8.1]", func() {

	insertTestEvent := func(eventID, correlationID string, eventDate time.Time) {
		_, err := db.ExecContext(ctx, `
			INSERT INTO audit_events (
				event_id, event_timestamp, event_date, event_type, event_category,
				correlation_id, resource_type, resource_id, event_action,
				event_outcome, actor_type, actor_id, event_data
			) VALUES (
				$1, $2, $3, 'gateway.signal.received', 'gateway', $4,
				'remediationrequest', 'rr-001', 'receive_signal', 'success',
				'service', 'gateway-service',
				'{"signal_name": "HighCPU", "signal_type": "alert"}'::jsonb
			)
		`, eventID, eventDate, eventDate.Truncate(24*time.Hour), correlationID)
		Expect(err).ToNot(HaveOccurred(), "test setup: inserting base audit event")
	}

	// ================================================================
	// AU-9 / CC8.1: Duplicate event_id rejection
	// ================================================================
	Context("IT-DS-SOC2-AU9-001: Duplicate event_id rejection [AU-9]", func() {

		It("should reject INSERT with duplicate event_id and same event_date [AU-9]", func() {
			eventID := uuid.New().String()
			correlationID := fmt.Sprintf("test-au9-dup-%s", uuid.New().String()[:8])
			now := time.Now().UTC()

			insertTestEvent(eventID, correlationID, now)

			_, err := db.ExecContext(ctx, `
				INSERT INTO audit_events (
					event_id, event_timestamp, event_date, event_type, event_category,
					correlation_id, resource_type, resource_id, event_action,
					event_outcome, actor_type, actor_id, event_data
				) VALUES (
					$1, $2, $3, 'gateway.signal.received', 'gateway', $4,
					'remediationrequest', 'rr-002', 'receive_signal', 'success',
					'service', 'gateway-service', '{}'::jsonb
				)
			`, eventID, now, now.Truncate(24*time.Hour), correlationID)

			Expect(err).To(HaveOccurred(),
				"AU-9: duplicate event_id with same event_date must be rejected by PK constraint")
			Expect(err.Error()).To(ContainSubstring("duplicate key"),
				"AU-9: error must indicate PK violation")
		})

		It("should confirm original event is unmodified after duplicate rejection [CC8.1]", func() {
			eventID := uuid.New().String()
			correlationID := fmt.Sprintf("test-au9-intact-%s", uuid.New().String()[:8])
			now := time.Now().UTC()

			insertTestEvent(eventID, correlationID, now)

			// Attempt duplicate
			_, _ = db.ExecContext(ctx, `
				INSERT INTO audit_events (
					event_id, event_timestamp, event_date, event_type, event_category,
					correlation_id, resource_type, resource_id, event_action,
					event_outcome, actor_type, actor_id, event_data
				) VALUES (
					$1, $2, $3, 'orchestrator.lifecycle.created', 'orchestrator', $4,
					'remediationrequest', 'rr-001', 'create', 'success',
					'service', 'orchestrator-service', '{}'::jsonb
				)
			`, eventID, now, now.Truncate(24*time.Hour), correlationID)

			var eventType string
			err := db.QueryRowContext(ctx,
				`SELECT event_type FROM audit_events WHERE event_id = $1 AND event_date = $2`,
				eventID, now.Truncate(24*time.Hour),
			).Scan(&eventType)

			Expect(err).ToNot(HaveOccurred())
			Expect(eventType).To(Equal("gateway.signal.received"),
				"CC8.1: original event must remain unmodified after duplicate insertion attempt")
		})
	})

	// ================================================================
	// AU-9 / CC8.1: UPDATE protection on critical audit fields
	// ================================================================
	Context("IT-DS-SOC2-AU9-002: Critical field UPDATE immutability [AU-9]", func() {

		It("should reject UPDATE on event_data [AU-9]", func() {
			eventID := uuid.New().String()
			correlationID := fmt.Sprintf("test-au9-upd-data-%s", uuid.New().String()[:8])
			now := time.Now().UTC()

			insertTestEvent(eventID, correlationID, now)

			_, err := db.ExecContext(ctx, `
				UPDATE audit_events
				SET event_data = '{"tampered": true}'::jsonb
				WHERE event_id = $1 AND event_date = $2
			`, eventID, now.Truncate(24*time.Hour))

			Expect(err).To(HaveOccurred(),
				"AU-9: UPDATE on event_data must be rejected to protect audit integrity")
			Expect(err.Error()).To(Or(
				ContainSubstring("immutable"),
				ContainSubstring("AU-9"),
				ContainSubstring("cannot be modified"),
			), "AU-9: error message should indicate immutability enforcement")
		})

		It("should reject UPDATE on event_type [AU-9]", func() {
			eventID := uuid.New().String()
			correlationID := fmt.Sprintf("test-au9-upd-type-%s", uuid.New().String()[:8])
			now := time.Now().UTC()

			insertTestEvent(eventID, correlationID, now)

			_, err := db.ExecContext(ctx, `
				UPDATE audit_events
				SET event_type = 'tampered.event.type'
				WHERE event_id = $1 AND event_date = $2
			`, eventID, now.Truncate(24*time.Hour))

			Expect(err).To(HaveOccurred(),
				"AU-9: UPDATE on event_type must be rejected to protect audit integrity")
			Expect(err.Error()).To(Or(
				ContainSubstring("immutable"),
				ContainSubstring("AU-9"),
				ContainSubstring("cannot be modified"),
			), "AU-9: error message should indicate immutability enforcement")
		})

		It("should reject UPDATE on event_outcome [AU-9]", func() {
			eventID := uuid.New().String()
			correlationID := fmt.Sprintf("test-au9-upd-outcome-%s", uuid.New().String()[:8])
			now := time.Now().UTC()

			insertTestEvent(eventID, correlationID, now)

			_, err := db.ExecContext(ctx, `
				UPDATE audit_events
				SET event_outcome = 'failure'
				WHERE event_id = $1 AND event_date = $2
			`, eventID, now.Truncate(24*time.Hour))

			Expect(err).To(HaveOccurred(),
				"AU-9: UPDATE on event_outcome must be rejected to protect audit integrity")
			Expect(err.Error()).To(Or(
				ContainSubstring("immutable"),
				ContainSubstring("AU-9"),
				ContainSubstring("cannot be modified"),
			), "AU-9: error message should indicate immutability enforcement")
		})

		It("should reject UPDATE on actor_id [AU-9]", func() {
			eventID := uuid.New().String()
			correlationID := fmt.Sprintf("test-au9-upd-actor-%s", uuid.New().String()[:8])
			now := time.Now().UTC()

			insertTestEvent(eventID, correlationID, now)

			_, err := db.ExecContext(ctx, `
				UPDATE audit_events
				SET actor_id = 'tampered-actor'
				WHERE event_id = $1 AND event_date = $2
			`, eventID, now.Truncate(24*time.Hour))

			Expect(err).To(HaveOccurred(),
				"AU-9: UPDATE on actor_id must be rejected to protect audit integrity")
			Expect(err.Error()).To(Or(
				ContainSubstring("immutable"),
				ContainSubstring("AU-9"),
				ContainSubstring("cannot be modified"),
			), "AU-9: error message should indicate immutability enforcement")
		})

		It("should reject UPDATE on correlation_id [AU-9]", func() {
			eventID := uuid.New().String()
			correlationID := fmt.Sprintf("test-au9-upd-corr-%s", uuid.New().String()[:8])
			now := time.Now().UTC()

			insertTestEvent(eventID, correlationID, now)

			_, err := db.ExecContext(ctx, `
				UPDATE audit_events
				SET correlation_id = 'tampered-correlation-id'
				WHERE event_id = $1 AND event_date = $2
			`, eventID, now.Truncate(24*time.Hour))

			Expect(err).To(HaveOccurred(),
				"AU-9: UPDATE on correlation_id must be rejected to protect audit integrity")
			Expect(err.Error()).To(Or(
				ContainSubstring("immutable"),
				ContainSubstring("AU-9"),
				ContainSubstring("cannot be modified"),
			), "AU-9: error message should indicate immutability enforcement")
		})
	})

	// ================================================================
	// AU-9: Allowed UPDATE paths (legal_hold, retention_days)
	// ================================================================
	Context("IT-DS-SOC2-AU9-003: Permitted UPDATE fields [AU-9]", func() {

		It("should allow UPDATE on retention_days (operational field) [AU-11]", func() {
			eventID := uuid.New().String()
			correlationID := fmt.Sprintf("test-au9-ret-ok-%s", uuid.New().String()[:8])
			now := time.Now().UTC()

			insertTestEvent(eventID, correlationID, now)

			_, err := db.ExecContext(ctx, `
				UPDATE audit_events
				SET retention_days = 365
				WHERE event_id = $1 AND event_date = $2
			`, eventID, now.Truncate(24*time.Hour))

			Expect(err).ToNot(HaveOccurred(),
				"AU-11: retention_days is an operational field and must remain updatable")
		})
	})
})

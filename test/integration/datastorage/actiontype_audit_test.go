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
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	dsaudit "github.com/jordigilh/kubernaut/pkg/datastorage/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server/helpers"
)

// ========================================
// IT-AT-300-006: ActionType Audit Events Integration Tests
// ========================================
//
// Authority: BR-WORKFLOW-007.4 (ActionType Audit Trail)
// Test Plan: docs/testing/300/TEST_PLAN.md
//
// Tests that ActionType audit events are correctly constructed,
// converted to repository format, and persisted to the audit_events table
// with correct JSONB payloads.
//
// Pipeline: Audit constructor → OpenAPI type → Repository type → PostgreSQL
//
// ========================================

var _ = Describe("IT-AT-300-006: ActionType Audit Events", Label("integration", "actiontype", "audit"), func() {
	var (
		auditRepo *repository.AuditEventsRepository
		testID    string
	)

	BeforeEach(func() {
		auditRepo = repository.NewAuditEventsRepository(db.DB, logger)
		testID = generateTestID()
	})

	AfterEach(func() {
		if db != nil {
			_, _ = db.ExecContext(ctx,
				"DELETE FROM audit_events WHERE correlation_id LIKE $1",
				fmt.Sprintf("it-at-006-%s%%", testID))
		}
	})

	// Helper: convert ogen AuditEventRequest to repository.AuditEvent and persist
	persistAuditEvent := func(ogenEvent *ogenclient.AuditEventRequest) *repository.AuditEvent {
		GinkgoHelper()

		// Override correlation_id for test isolation
		ogenEvent.CorrelationID = fmt.Sprintf("it-at-006-%s-%s", testID, uuid.New().String()[:8])

		internalEvent, err := helpers.ConvertAuditEventRequest(*ogenEvent)
		Expect(err).ToNot(HaveOccurred(), "ConvertAuditEventRequest should succeed")

		repoEvent, err := helpers.ConvertToRepositoryAuditEvent(internalEvent)
		Expect(err).ToNot(HaveOccurred(), "ConvertToRepositoryAuditEvent should succeed")

		created, err := auditRepo.Create(ctx, repoEvent)
		Expect(err).ToNot(HaveOccurred(), "audit event should be persisted to DB")

		return created
	}

	// Helper: read event_data JSONB from DB
	readEventData := func(eventID uuid.UUID) map[string]interface{} {
		GinkgoHelper()
		var eventDataJSON []byte
		row := db.QueryRowContext(ctx,
			`SELECT event_data FROM audit_events WHERE event_id = $1`, eventID)
		Expect(row.Scan(&eventDataJSON)).To(Succeed(), "event_data should be readable from DB")

		var data map[string]interface{}
		Expect(json.Unmarshal(eventDataJSON, &data)).To(Succeed(), "event_data should be valid JSON")
		return data
	}

	// ========================================
	// datastorage.actiontype.created
	// ========================================
	Describe("Created event", func() {
		It("should persist with correct event_type and JSONB payload", func() {
			desc := ogenclient.ActionTypeDescriptionPayload{
				What:      "Kill and recreate pods",
				WhenToUse: "Transient runtime issue",
			}
			ogenEvent, err := dsaudit.NewActionTypeCreatedAuditEvent(
				"RestartPod", desc, "admin@kubernaut.ai", false)
			Expect(err).ToNot(HaveOccurred())

			created := persistAuditEvent(ogenEvent)

			// Verify event metadata in DB
			var dbEventType, dbCategory, dbAction, dbOutcome string
			row := db.QueryRowContext(ctx,
				`SELECT event_type, event_category, event_action, event_outcome
				 FROM audit_events WHERE event_id = $1`, created.EventID)
			Expect(row.Scan(&dbEventType, &dbCategory, &dbAction, &dbOutcome)).To(Succeed())

			Expect(dbEventType).To(Equal(dsaudit.EventTypeActionTypeCreated))
			Expect(dbCategory).To(Equal("actiontype"))
			Expect(dbAction).To(Equal("create"))
			Expect(dbOutcome).To(Equal("success"))

			// Verify JSONB payload (ogen uses snake_case JSON tags)
			data := readEventData(created.EventID)
			Expect(data["event_type"]).To(Equal("datastorage.actiontype.created"))
			Expect(data["action_type"]).To(Equal("RestartPod"))
			Expect(data["registered_by"]).To(Equal("admin@kubernaut.ai"))
			Expect(data["was_reenabled"]).To(BeFalse())

			descMap, ok := data["description"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "description should be a JSON object")
			Expect(descMap["what"]).To(Equal("Kill and recreate pods"))
			Expect(descMap["when_to_use"]).To(Equal("Transient runtime issue"))
		})
	})

	// ========================================
	// datastorage.actiontype.updated
	// ========================================
	Describe("Updated event", func() {
		It("should persist with old and new description in JSONB payload", func() {
			oldDesc := ogenclient.ActionTypeDescriptionPayload{
				What:      "Old description",
				WhenToUse: "Old when",
			}
			newDesc := ogenclient.ActionTypeDescriptionPayload{
				What:      "New description",
				WhenToUse: "New when",
			}
			ogenEvent, err := dsaudit.NewActionTypeUpdatedAuditEvent(
				"RestartPod", oldDesc, newDesc, "editor@kubernaut.ai", []string{"what", "whenToUse"})
			Expect(err).ToNot(HaveOccurred())

			created := persistAuditEvent(ogenEvent)

			var dbEventType string
			row := db.QueryRowContext(ctx,
				`SELECT event_type FROM audit_events WHERE event_id = $1`, created.EventID)
			Expect(row.Scan(&dbEventType)).To(Succeed())
			Expect(dbEventType).To(Equal(dsaudit.EventTypeActionTypeUpdated))

			data := readEventData(created.EventID)
			Expect(data["event_type"]).To(Equal("datastorage.actiontype.updated"))
			Expect(data["action_type"]).To(Equal("RestartPod"))
			Expect(data["updated_by"]).To(Equal("editor@kubernaut.ai"))

			oldDescMap, ok := data["old_description"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(oldDescMap["what"]).To(Equal("Old description"))

			newDescMap, ok := data["new_description"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(newDescMap["what"]).To(Equal("New description"))

			updatedFields, ok := data["updated_fields"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(updatedFields).To(ConsistOf("what", "whenToUse"))
		})
	})

	// ========================================
	// datastorage.actiontype.disabled
	// ========================================
	Describe("Disabled event", func() {
		It("should persist with disabledBy and disabledAt in JSONB payload", func() {
			disabledAt := time.Now().UTC().Truncate(time.Millisecond)
			ogenEvent, err := dsaudit.NewActionTypeDisabledAuditEvent(
				"RestartPod", "ops@kubernaut.ai", disabledAt)
			Expect(err).ToNot(HaveOccurred())

			created := persistAuditEvent(ogenEvent)

			var dbEventType, dbOutcome string
			row := db.QueryRowContext(ctx,
				`SELECT event_type, event_outcome FROM audit_events WHERE event_id = $1`, created.EventID)
			Expect(row.Scan(&dbEventType, &dbOutcome)).To(Succeed())
			Expect(dbEventType).To(Equal(dsaudit.EventTypeActionTypeDisabled))
			Expect(dbOutcome).To(Equal("success"))

			data := readEventData(created.EventID)
			Expect(data["event_type"]).To(Equal("datastorage.actiontype.disabled"))
			Expect(data["action_type"]).To(Equal("RestartPod"))
			Expect(data["disabled_by"]).To(Equal("ops@kubernaut.ai"))
		})
	})

	// ========================================
	// datastorage.actiontype.reenabled
	// ========================================
	Describe("Reenabled event", func() {
		It("should persist with previous state details in JSONB payload", func() {
			prevDisabledAt := time.Now().Add(-24 * time.Hour).UTC().Truncate(time.Millisecond)
			ogenEvent, err := dsaudit.NewActionTypeReenabledAuditEvent(
				"RestartPod", "admin@kubernaut.ai", prevDisabledAt, "ops@kubernaut.ai")
			Expect(err).ToNot(HaveOccurred())

			created := persistAuditEvent(ogenEvent)

			var dbEventType string
			row := db.QueryRowContext(ctx,
				`SELECT event_type FROM audit_events WHERE event_id = $1`, created.EventID)
			Expect(row.Scan(&dbEventType)).To(Succeed())
			Expect(dbEventType).To(Equal(dsaudit.EventTypeActionTypeReenabled))

			data := readEventData(created.EventID)
			Expect(data["event_type"]).To(Equal("datastorage.actiontype.reenabled"))
			Expect(data["action_type"]).To(Equal("RestartPod"))
			Expect(data["reenabled_by"]).To(Equal("admin@kubernaut.ai"))
			Expect(data["previous_state"]).To(Equal("disabled"))
			Expect(data["disabled_by"]).To(Equal("ops@kubernaut.ai"))
		})
	})

	// ========================================
	// datastorage.actiontype.disable_denied
	// ========================================
	Describe("Disable denied event", func() {
		It("should persist with dependent workflow details in JSONB payload", func() {
			ogenEvent, err := dsaudit.NewActionTypeDisableDeniedAuditEvent(
				"RestartPod", "ops@kubernaut.ai", 3,
				[]string{"wf-alpha", "wf-beta", "wf-gamma"})
			Expect(err).ToNot(HaveOccurred())

			created := persistAuditEvent(ogenEvent)

			var dbEventType, dbOutcome string
			row := db.QueryRowContext(ctx,
				`SELECT event_type, event_outcome FROM audit_events WHERE event_id = $1`, created.EventID)
			Expect(row.Scan(&dbEventType, &dbOutcome)).To(Succeed())
			Expect(dbEventType).To(Equal(dsaudit.EventTypeActionTypeDisableDenied))
			Expect(dbOutcome).To(Equal("failure"))

			data := readEventData(created.EventID)
			Expect(data["event_type"]).To(Equal("datastorage.actiontype.disable_denied"))
			Expect(data["action_type"]).To(Equal("RestartPod"))
			Expect(data["requested_by"]).To(Equal("ops@kubernaut.ai"))
			Expect(data["dependent_workflow_count"]).To(BeNumerically("==", 3))

			workflows, ok := data["dependent_workflows"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(workflows).To(ConsistOf("wf-alpha", "wf-beta", "wf-gamma"))
		})
	})
})

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

package audit

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	dsaudit "github.com/jordigilh/kubernaut/pkg/datastorage/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// ========================================
// UT-DS-AUDIT-001: ActionType Audit Event CorrelationID Tests
// ========================================
//
// Authority: BR-AUDIT-024 (Audit Event Construction)
//
// Validates that all action-type audit event constructors produce events
// with a non-empty correlation_id that satisfies the OpenAPI minLength: 1
// constraint. These events are internal to DataStorage (no RemediationRequest
// correlation) and use a deterministic "actiontype-{name}" format.
//
// Root cause: Before this fix, all NewActionType* constructors left
// correlation_id as "" (Go zero value), which failed OpenAPI validation
// at the middleware level, silently dropping audit events.
//
// ========================================

var _ = Describe("UT-DS-AUDIT-001: ActionType Audit Event CorrelationID", Label("unit", "actiontype", "audit"), func() {

	It("UT-DS-AUDIT-001-001: Created event has non-empty correlation_id", func() {
		desc := ogenclient.ActionTypeDescriptionPayload{
			What:      "Kill and recreate pods",
			WhenToUse: "Transient runtime issue",
		}
		event, err := dsaudit.NewActionTypeCreatedAuditEvent("RestartPod", desc, "admin@kubernaut.ai", false)
		Expect(err).ToNot(HaveOccurred())
		Expect(event.CorrelationID).ToNot(BeEmpty(), "correlation_id must not be empty (OpenAPI minLength: 1)")
		Expect(event.CorrelationID).To(Equal("actiontype-restartpod"))
	})

	It("UT-DS-AUDIT-001-002: Updated event has non-empty correlation_id", func() {
		oldDesc := ogenclient.ActionTypeDescriptionPayload{What: "Old"}
		newDesc := ogenclient.ActionTypeDescriptionPayload{What: "New"}
		event, err := dsaudit.NewActionTypeUpdatedAuditEvent("ScaleUp", oldDesc, newDesc, "editor@kubernaut.ai", []string{"what"})
		Expect(err).ToNot(HaveOccurred())
		Expect(event.CorrelationID).To(Equal("actiontype-scaleup"))
	})

	It("UT-DS-AUDIT-001-003: Disabled event has non-empty correlation_id", func() {
		event, err := dsaudit.NewActionTypeDisabledAuditEvent("RestartPod", "ops@kubernaut.ai", time.Now().UTC())
		Expect(err).ToNot(HaveOccurred())
		Expect(event.CorrelationID).To(Equal("actiontype-restartpod"))
	})

	It("UT-DS-AUDIT-001-004: Reenabled event has non-empty correlation_id", func() {
		event, err := dsaudit.NewActionTypeReenabledAuditEvent("RestartPod", "admin@kubernaut.ai", time.Now().UTC(), "ops@kubernaut.ai")
		Expect(err).ToNot(HaveOccurred())
		Expect(event.CorrelationID).To(Equal("actiontype-restartpod"))
	})

	It("UT-DS-AUDIT-001-005: Disable denied event has non-empty correlation_id", func() {
		event, err := dsaudit.NewActionTypeDisableDeniedAuditEvent("RestartPod", "ops@kubernaut.ai", 2, []string{"wf-a", "wf-b"})
		Expect(err).ToNot(HaveOccurred())
		Expect(event.CorrelationID).To(Equal("actiontype-restartpod"))
	})

	It("UT-DS-AUDIT-001-006: Correlation ID is deterministic and lowercased", func() {
		desc := ogenclient.ActionTypeDescriptionPayload{What: "test"}
		e1, _ := dsaudit.NewActionTypeCreatedAuditEvent("IncreaseMemory", desc, "a@b.c", false)
		e2, _ := dsaudit.NewActionTypeCreatedAuditEvent("IncreaseMemory", desc, "d@e.f", false)
		Expect(e1.CorrelationID).To(Equal(e2.CorrelationID), "same action type should produce same correlation_id")
		Expect(e1.CorrelationID).To(Equal("actiontype-increasememory"))
	})
})

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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	dsaudit "github.com/jordigilh/kubernaut/pkg/datastorage/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

var _ = Describe("ActionType DS Audit Constructors [BR-WORKFLOW-007]", func() {

	desc := ogenclient.ActionTypeDescriptionPayload{
		What:      "Kill and recreate one or more pods.",
		WhenToUse: "Root cause is a transient runtime state issue.",
	}

	Describe("NewActionTypeCreatedAuditEvent", func() {
		It("UT-AT-AUDIT-001: creates event with correct envelope for new action type", func() {
			event, err := dsaudit.NewActionTypeCreatedAuditEvent("RestartPod", desc, "system:sa:authwebhook", false)
			Expect(err).NotTo(HaveOccurred())
			Expect(event).NotTo(BeNil())

			Expect(event.EventType).To(Equal(dsaudit.EventTypeActionTypeCreated))
			Expect(string(event.EventCategory)).To(Equal(dsaudit.EventCategoryActionType))
			Expect(event.EventAction).To(Equal(dsaudit.ActionCreate))
			Expect(event.EventOutcome).To(Equal(ogenclient.AuditEventRequestEventOutcomeSuccess))
			Expect(event.ResourceType.Value).To(Equal("ActionType"))
			Expect(event.ResourceID.Value).To(Equal("RestartPod"))
			Expect(event.ActorType.Value).To(Equal("service"))
			Expect(event.ActorID.Value).To(Equal("system:sa:authwebhook"))
		})

		It("UT-AT-AUDIT-002: creates event with wasReenabled=true for re-enabled type", func() {
			event, err := dsaudit.NewActionTypeCreatedAuditEvent("RestartPod", desc, "admin", true)
			Expect(err).NotTo(HaveOccurred())
			Expect(event).NotTo(BeNil())

			Expect(event.EventData.Type).To(Equal(ogenclient.ActionTypeCatalogCreatedPayloadAuditEventRequestEventData))
			Expect(event.EventData.ActionTypeCatalogCreatedPayload.WasReenabled).To(BeTrue())
			Expect(event.EventData.ActionTypeCatalogCreatedPayload.ActionType).To(Equal("RestartPod"))
		})
	})

	Describe("NewActionTypeUpdatedAuditEvent", func() {
		oldDesc := ogenclient.ActionTypeDescriptionPayload{
			What:      "Old text",
			WhenToUse: "Old use case",
		}
		newDesc := ogenclient.ActionTypeDescriptionPayload{
			What:      "New text",
			WhenToUse: "Old use case",
		}

		It("UT-AT-AUDIT-003: creates event with old+new descriptions for SOC2", func() {
			event, err := dsaudit.NewActionTypeUpdatedAuditEvent("RestartPod", oldDesc, newDesc, "admin", []string{"what"})
			Expect(err).NotTo(HaveOccurred())
			Expect(event).NotTo(BeNil())

			Expect(event.EventType).To(Equal(dsaudit.EventTypeActionTypeUpdated))
			Expect(event.EventData.ActionTypeCatalogUpdatedPayload.OldDescription.What).To(Equal("Old text"))
			Expect(event.EventData.ActionTypeCatalogUpdatedPayload.NewDescription.What).To(Equal("New text"))
			Expect(event.EventData.ActionTypeCatalogUpdatedPayload.UpdatedFields).To(Equal([]string{"what"}))
			Expect(event.EventData.ActionTypeCatalogUpdatedPayload.UpdatedBy).To(Equal("admin"))
		})

		It("UT-AT-AUDIT-004: supports multiple updated fields", func() {
			event, err := dsaudit.NewActionTypeUpdatedAuditEvent("RestartPod", oldDesc, newDesc, "admin", []string{"what", "whenToUse"})
			Expect(err).NotTo(HaveOccurred())
			Expect(event.EventData.ActionTypeCatalogUpdatedPayload.UpdatedFields).To(Equal([]string{"what", "whenToUse"}))
		})
	})

	Describe("NewActionTypeDisabledAuditEvent", func() {
		It("UT-AT-AUDIT-005: creates event with correct timestamp", func() {
			now := time.Now().UTC()
			event, err := dsaudit.NewActionTypeDisabledAuditEvent("RestartPod", "admin", now)
			Expect(err).NotTo(HaveOccurred())

			Expect(event.EventType).To(Equal(dsaudit.EventTypeActionTypeDisabled))
			Expect(event.EventOutcome).To(Equal(ogenclient.AuditEventRequestEventOutcomeSuccess))
			Expect(event.EventData.ActionTypeCatalogDisabledPayload.DisabledBy).To(Equal("admin"))
			Expect(event.EventData.ActionTypeCatalogDisabledPayload.DisabledAt).To(BeTemporally("~", now, time.Second))
		})
	})

	Describe("NewActionTypeReenabledAuditEvent", func() {
		It("UT-AT-AUDIT-006: creates event with previous disable info", func() {
			prevDisabledAt := time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)
			event, err := dsaudit.NewActionTypeReenabledAuditEvent("RestartPod", "admin", prevDisabledAt, "ops")
			Expect(err).NotTo(HaveOccurred())

			Expect(event.EventType).To(Equal(dsaudit.EventTypeActionTypeReenabled))
			payload := event.EventData.ActionTypeCatalogReenabledPayload
			Expect(string(payload.PreviousState)).To(Equal("disabled"))
			Expect(payload.DisabledAt).To(BeTemporally("~", prevDisabledAt, time.Second))
			Expect(payload.DisabledBy).To(Equal("ops"))
			Expect(payload.ReenabledBy).To(Equal("admin"))
		})
	})

	Describe("NewActionTypeDisableDeniedAuditEvent", func() {
		It("UT-AT-AUDIT-007: creates event with dependent workflow details", func() {
			workflows := []string{"wf-a", "wf-b", "wf-c"}
			event, err := dsaudit.NewActionTypeDisableDeniedAuditEvent("RestartPod", "admin", 3, workflows)
			Expect(err).NotTo(HaveOccurred())

			Expect(event.EventType).To(Equal(dsaudit.EventTypeActionTypeDisableDenied))
			Expect(event.EventOutcome).To(Equal(ogenclient.AuditEventRequestEventOutcomeFailure))
			payload := event.EventData.ActionTypeCatalogDisableDeniedPayload
			Expect(payload.DependentWorkflowCount).To(Equal(3))
			Expect(payload.DependentWorkflows).To(Equal([]string{"wf-a", "wf-b", "wf-c"}))
			Expect(payload.RequestedBy).To(Equal("admin"))
		})
	})

	Describe("Event data discriminator", func() {
		It("UT-AT-AUDIT-008: all constructors produce correctly typed EventData", func() {
			createdEvt, _ := dsaudit.NewActionTypeCreatedAuditEvent("T", desc, "s", false)
			Expect(createdEvt.EventData.Type).To(Equal(ogenclient.ActionTypeCatalogCreatedPayloadAuditEventRequestEventData))

			updatedEvt, _ := dsaudit.NewActionTypeUpdatedAuditEvent("T", desc, desc, "s", []string{})
			Expect(updatedEvt.EventData.Type).To(Equal(ogenclient.ActionTypeCatalogUpdatedPayloadAuditEventRequestEventData))

			disabledEvt, _ := dsaudit.NewActionTypeDisabledAuditEvent("T", "s", time.Now())
			Expect(disabledEvt.EventData.Type).To(Equal(ogenclient.ActionTypeCatalogDisabledPayloadAuditEventRequestEventData))

			reenabledEvt, _ := dsaudit.NewActionTypeReenabledAuditEvent("T", "s", time.Now(), "o")
			Expect(reenabledEvt.EventData.Type).To(Equal(ogenclient.ActionTypeCatalogReenabledPayloadAuditEventRequestEventData))

			deniedEvt, _ := dsaudit.NewActionTypeDisableDeniedAuditEvent("T", "s", 0, nil)
			Expect(deniedEvt.EventData.Type).To(Equal(ogenclient.ActionTypeCatalogDisableDeniedPayloadAuditEventRequestEventData))
		})
	})
})

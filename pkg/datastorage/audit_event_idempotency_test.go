/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
*/

package datastorage_test

import (
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server/helpers"
)

var _ = Describe("DD-AUDIT-010 audit request idempotency", func() {
	It("UT-DS-AUDIT-2512-001: preserves the client event ID through request conversion", func() {
		eventID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440099")
		request := ogenclient.AuditEventRequest{
			EventID:        ogenclient.OptNilUUID{Value: eventID, Set: true},
			Version:        "1.0",
			EventType:      "workflowexecution.execution.started",
			EventTimestamp: time.Now().UTC(),
			EventCategory:  ogenclient.AuditEventRequestEventCategoryWorkflowexecution,
			EventAction:    "started",
			EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
			CorrelationID:  "rr-2512-audit",
			EventData: ogenclient.NewAuditEventRequestEventDataWorkflowexecutionWorkflowStartedAuditEventRequestEventData(
				ogenclient.WorkflowExecutionAuditPayload{EventType: ogenclient.WorkflowExecutionAuditPayloadEventTypeWorkflowexecutionExecutionStarted},
			),
		}

		converted, err := helpers.ConvertAuditEventRequest(request, "system:serviceaccount:kubernaut-system:workflowexecution")
		Expect(err).NotTo(HaveOccurred())
		Expect(converted.EventID).To(Equal(eventID))
	})
})

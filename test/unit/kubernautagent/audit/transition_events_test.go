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

package audit_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
)

var _ = Describe("Audit Transition Events — PR4 DD-INTERACTIVE-002 AUD-01", func() {

	Describe("UT-KA-AUD-001: Takeover emits aiagent.session.suspended for autonomous", func() {
		It("should define EventTypeSessionSuspended constant", func() {
			Expect(audit.EventTypeSessionSuspended).To(Equal("aiagent.session.suspended"))
			Expect(audit.ActionSessionSuspended).To(Equal("session_suspended"))
		})
	})

	Describe("UT-KA-AUD-002: Takeover emits aiagent.interactive.started with acting_user", func() {
		It("should define EventTypeInteractiveStarted constant", func() {
			Expect(audit.EventTypeInteractiveStarted).To(Equal("aiagent.interactive.started"))
			Expect(audit.ActionInteractiveStarted).To(Equal("interactive_started"))
		})
	})

	Describe("UT-KA-AUD-003: Disconnect emits aiagent.interactive.completed", func() {
		It("should define EventTypeInteractiveCompleted constant", func() {
			Expect(audit.EventTypeInteractiveCompleted).To(Equal("aiagent.interactive.completed"))
			Expect(audit.ActionInteractiveCompleted).To(Equal("interactive_completed"))
		})
	})

	Describe("UT-KA-AUD-004: Reconstruction emits aiagent.session.resumed", func() {
		It("should define EventTypeSessionResumed constant", func() {
			Expect(audit.EventTypeSessionResumed).To(Equal("aiagent.session.resumed"))
			Expect(audit.ActionSessionResumed).To(Equal("session_resumed"))
		})
	})

	Describe("UT-KA-AUD-005: All transition events carry session_id and correlation_id", func() {
		It("should create valid events with session_id and correlation_id", func() {
			event := audit.NewEvent(audit.EventTypeSessionSuspended, "rr-aud-005",
				audit.WithSessionID("auto-sess-001"),
				audit.WithActingUser("system:serviceaccount:kubernaut:kubernaut-agent"),
			)
			Expect(event.CorrelationID).To(Equal("rr-aud-005"))
			Expect(event.SessionID).To(Equal("auto-sess-001"))
			Expect(event.ActingUser).To(Equal("system:serviceaccount:kubernaut:kubernaut-agent"))
			Expect(event.EventType).To(Equal(audit.EventTypeSessionSuspended))
			Expect(event.EventCategory).To(Equal(audit.EventCategory))
			Expect(event.Data).To(HaveKey("event_id"))
		})
	})

	Describe("UT-KA-AUD-006: Cancel emits aiagent.interactive.completed with reason explicit", func() {
		It("should allow reason data in interactive.completed event", func() {
			event := audit.NewEvent(audit.EventTypeInteractiveCompleted, "rr-aud-006",
				audit.WithSessionID("int-sess-001"),
				audit.WithActingUser("alice@example.com"),
			)
			event.EventAction = audit.ActionInteractiveCompleted
			event.EventOutcome = audit.OutcomeSuccess
			event.Data["reason"] = "explicit"

			Expect(event.EventType).To(Equal("aiagent.interactive.completed"))
			Expect(event.Data["reason"]).To(Equal("explicit"))
		})
	})

	Describe("UT-KA-AUD-007: Timeout emits aiagent.interactive.completed with reason timeout", func() {
		It("should allow reason data in interactive.completed event for timeout", func() {
			event := audit.NewEvent(audit.EventTypeInteractiveCompleted, "rr-aud-007",
				audit.WithSessionID("int-sess-002"),
			)
			event.EventAction = audit.ActionInteractiveCompleted
			event.EventOutcome = audit.OutcomeSuccess
			event.Data["reason"] = "timeout"

			Expect(event.Data["reason"]).To(Equal("timeout"))
		})
	})

	Describe("AllEventTypes includes new transition events", func() {
		It("should include all 4 new transition event types in AllEventTypes", func() {
			Expect(audit.AllEventTypes).To(ContainElement(audit.EventTypeSessionSuspended))
			Expect(audit.AllEventTypes).To(ContainElement(audit.EventTypeInteractiveStarted))
			Expect(audit.AllEventTypes).To(ContainElement(audit.EventTypeInteractiveCompleted))
			Expect(audit.AllEventTypes).To(ContainElement(audit.EventTypeSessionResumed))
		})
	})
})

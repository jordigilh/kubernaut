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

var _ = Describe("NewEvent SessionID — #703", func() {

	Describe("UT-KA-703-G01: WithSessionID sets SessionID field on AuditEvent", func() {
		It("should populate SessionID via functional option", func() {
			event := audit.NewEvent(audit.EventTypeLLMRequest, "corr-123", audit.WithSessionID("session-abc"))
			Expect(event.SessionID).To(Equal("session-abc"))
			Expect(event.CorrelationID).To(Equal("corr-123"))
			Expect(event.EventType).To(Equal(audit.EventTypeLLMRequest))
		})
	})

	Describe("UT-KA-703-G02: NewEvent without options leaves SessionID empty", func() {
		It("should default to empty string for autonomous events", func() {
			event := audit.NewEvent(audit.EventTypeLLMRequest, "corr-456")
			Expect(event.SessionID).To(Equal(""))
			Expect(event.CorrelationID).To(Equal("corr-456"))
		})
	})

	Describe("UT-KA-703-G03: SessionID is a top-level field, not in Data map", func() {
		It("should not store session_id redundantly in the Data map", func() {
			event := audit.NewEvent(audit.EventTypeSessionStarted, "corr-789", audit.WithSessionID("session-xyz"))
			Expect(event.SessionID).To(Equal("session-xyz"))
			_, existsInData := event.Data["session_id"]
			Expect(existsInData).To(BeFalse())
		})
	})

	Describe("UT-KA-703-G04: NewEvent retains all existing behavior", func() {
		It("should still set event_category and event_id in Data", func() {
			event := audit.NewEvent(audit.EventTypeRCAComplete, "corr-abc", audit.WithSessionID("session-def"))
			Expect(event.EventCategory).To(Equal(audit.EventCategory))
			Expect(event.Data).To(HaveKey("event_id"))
			Expect(event.Data["event_id"]).NotTo(BeEmpty())
		})
	})
})

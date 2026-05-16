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

var _ = Describe("AuditEvent ActingUser — #703 BR-INTERACTIVE-005", func() {

	Describe("UT-KA-703-L01: WithActingUser sets ActingUser field on AuditEvent", func() {
		It("should populate ActingUser via functional option", func() {
			event := audit.NewEvent(audit.EventTypeLLMRequest, "corr-123",
				audit.WithSessionID("session-abc"),
				audit.WithActingUser("alice@example.com"),
			)
			Expect(event.ActingUser).To(Equal("alice@example.com"))
			Expect(event.SessionID).To(Equal("session-abc"))
			Expect(event.CorrelationID).To(Equal("corr-123"))
		})
	})

	Describe("UT-KA-703-L02: Interactive LLM request event carries acting_user", func() {
		It("should support acting_user on LLM request events for SOC2 attribution", func() {
			event := audit.NewEvent(audit.EventTypeLLMRequest, "rr-001",
				audit.WithSessionID("mcp-session-1"),
				audit.WithActingUser("operator@company.io"),
			)
			Expect(event.EventType).To(Equal(audit.EventTypeLLMRequest))
			Expect(event.ActingUser).To(Equal("operator@company.io"))
			Expect(event.SessionID).To(Equal("mcp-session-1"))
		})
	})

	Describe("UT-KA-703-L03: Interactive tool call event carries acting_user", func() {
		It("should support acting_user on tool call events for SOC2 attribution", func() {
			event := audit.NewEvent(audit.EventTypeLLMToolCall, "rr-002",
				audit.WithSessionID("mcp-session-2"),
				audit.WithActingUser("sre-bob@company.io"),
			)
			Expect(event.EventType).To(Equal(audit.EventTypeLLMToolCall))
			Expect(event.ActingUser).To(Equal("sre-bob@company.io"))
			Expect(event.SessionID).To(Equal("mcp-session-2"))
		})
	})

	Describe("UT-KA-703-L04: Autonomous events leave ActingUser empty (backward compat)", func() {
		It("should default ActingUser to empty string when no option provided", func() {
			event := audit.NewEvent(audit.EventTypeLLMRequest, "rr-autonomous")
			Expect(event.ActingUser).To(Equal(""))
			Expect(event.SessionID).To(Equal(""))
		})
	})
})

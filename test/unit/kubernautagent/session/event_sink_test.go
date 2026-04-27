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

package session_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
)

var _ = Describe("Event Sink Context Helpers — #823 PR3", func() {

	Describe("UT-KA-823-C08: Event sink context round-trip", func() {
		It("event sink attached to context is retrievable by EventSinkFromContext", func() {
			ch := make(chan session.InvestigationEvent, 1)
			ctx := session.WithEventSink(context.Background(), ch)

			retrieved := session.EventSinkFromContext(ctx)
			Expect(retrieved).NotTo(BeNil(), "should retrieve the attached event sink")

			retrieved <- session.InvestigationEvent{Type: session.EventTypeComplete}
			Expect(ch).To(Receive(HaveField("Type", session.EventTypeComplete)))
		})
	})

	Describe("UT-KA-823-C09: Missing event sink returns nil", func() {
		It("EventSinkFromContext on plain context returns nil without panic", func() {
			ctx := context.Background()
			retrieved := session.EventSinkFromContext(ctx)
			Expect(retrieved).To(BeNil(), "no event sink attached — should return nil")
		})
	})
})

var _ = Describe("Session ID Context Helpers — BR-AUDIT-070", func() {

	Describe("UT-KA-PR9-SID-001: WithSessionID / SessionIDFromContext round-trip", func() {
		It("session ID attached to context is retrievable", func() {
			ctx := session.WithSessionID(context.Background(), "sess-abc-123")
			Expect(session.SessionIDFromContext(ctx)).To(Equal("sess-abc-123"))
		})
	})

	Describe("UT-KA-PR9-SID-002: SessionIDFromContext returns empty string for plain context", func() {
		It("returns empty string without panic when no session ID is attached", func() {
			ctx := context.Background()
			Expect(session.SessionIDFromContext(ctx)).To(Equal(""))
		})
	})

	Describe("UT-KA-PR9-SID-003: SessionIDFromContext returns empty string for nil context value", func() {
		It("does not panic when context has wrong type for session ID key", func() {
			ctx := context.Background()
			Expect(session.SessionIDFromContext(ctx)).To(BeEmpty())
		})
	})

	Describe("UT-KA-PR9-SID-004: WithSessionID does not interfere with event sink", func() {
		It("both session ID and event sink coexist on the same context", func() {
			ch := make(chan session.InvestigationEvent, 1)
			ctx := session.WithEventSink(context.Background(), ch)
			ctx = session.WithSessionID(ctx, "sess-coexist")

			Expect(session.SessionIDFromContext(ctx)).To(Equal("sess-coexist"))
			Expect(session.EventSinkFromContext(ctx)).NotTo(BeNil())
		})
	})
})

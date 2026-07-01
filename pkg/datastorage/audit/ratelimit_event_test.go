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

	pkgaudit "github.com/jordigilh/kubernaut/pkg/audit"
	dsaudit "github.com/jordigilh/kubernaut/pkg/datastorage/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// BR-STORAGE-1505 (GAP-09, Issue #1505): Data Storage self-audits per-IP
// rate-limit denials (FedRAMP AU-12 — audit generation for security-relevant
// events).
var _ = Describe("NewRatelimitDeniedAuditEvent", Label("unit", "audit", "ratelimit"), func() {
	It("UT-DS-AUDIT-002-001: produces a well-formed, non-empty correlation_id and required envelope fields", func() {
		event := dsaudit.NewRatelimitDeniedAuditEvent("203.0.113.1", "/api/v1/audit/events", "POST")

		Expect(event.EventType).To(Equal("datastorage.ratelimit.denied"))
		Expect(string(event.EventCategory)).To(Equal("security"))
		Expect(event.EventAction).To(Equal("denied"))
		Expect(event.EventOutcome).To(Equal(pkgaudit.OutcomeFailure))
		Expect(event.CorrelationID).NotTo(BeEmpty(), "correlation_id must not be empty (OpenAPI minLength: 1)")

		actorType, ok := event.ActorType.Get()
		Expect(ok).To(BeTrue())
		Expect(actorType).To(Equal("external"))
		actorID, ok := event.ActorID.Get()
		Expect(ok).To(BeTrue())
		Expect(actorID).To(Equal("203.0.113.1"))
	})

	It("UT-DS-AUDIT-002-002: sets the typed DatastorageRatelimitDeniedPayload with source IP, path, and method", func() {
		event := dsaudit.NewRatelimitDeniedAuditEvent("198.51.100.5", "/api/v1/workflows", "GET")

		Expect(event.EventData.IsDatastorageRatelimitDeniedPayload()).To(BeTrue())
		payload := event.EventData.DatastorageRatelimitDeniedPayload

		sourceIP, ok := payload.SourceIP.Get()
		Expect(ok).To(BeTrue())
		Expect(sourceIP).To(Equal("198.51.100.5"))

		path, ok := payload.Path.Get()
		Expect(ok).To(BeTrue())
		Expect(path).To(Equal("/api/v1/workflows"))

		method, ok := payload.Method.Get()
		Expect(ok).To(BeTrue())
		Expect(method).To(Equal("GET"))

		Expect(payload.EventType).To(Equal(ogenclient.DatastorageRatelimitDeniedPayloadEventTypeDatastorageRatelimitDenied))
		Expect(payload.EventID).NotTo(BeEmpty())
	})

	It("UT-DS-AUDIT-002-003: tolerates an empty source IP (falls back to actor \"unknown\")", func() {
		event := dsaudit.NewRatelimitDeniedAuditEvent("", "", "")

		actorID, ok := event.ActorID.Get()
		Expect(ok).To(BeTrue())
		Expect(actorID).To(Equal("unknown"))

		payload := event.EventData.DatastorageRatelimitDeniedPayload
		_, hasSourceIP := payload.SourceIP.Get()
		Expect(hasSourceIP).To(BeFalse())
	})

	It("UT-DS-AUDIT-002-004: generates a distinct correlation_id per call", func() {
		e1 := dsaudit.NewRatelimitDeniedAuditEvent("10.0.0.1", "/a", "GET")
		e2 := dsaudit.NewRatelimitDeniedAuditEvent("10.0.0.1", "/a", "GET")
		Expect(e1.CorrelationID).NotTo(Equal(e2.CorrelationID))
	})
})

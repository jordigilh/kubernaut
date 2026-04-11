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
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/audit"
)

func validAuditEventData() []byte {
	env := audit.NewEventData("gateway", "signal_received", "success", map[string]interface{}{"id": "x"})
	b, err := env.ToJSON()
	Expect(err).NotTo(HaveOccurred())
	return b
}

var _ = Describe("BR-STORAGE-001: shared pkg/audit coverage (issue 668)", func() {

	Describe("NewAuditEvent (BR-STORAGE-001)", func() {
		It("initializes EventID, version 1.0, UTC timestamp, retention, and IsSensitive=false (BR-STORAGE-001)", func() {
			ev := audit.NewAuditEvent()
			Expect(ev.EventID).NotTo(Equal(uuid.Nil))
			Expect(ev.EventVersion).To(Equal("1.0"))
			Expect(ev.EventTimestamp.Location()).To(Equal(time.UTC))
			Expect(ev.EventTimestamp).To(BeTemporally("~", time.Now().UTC(), time.Minute))
			Expect(ev.RetentionDays).To(Equal(2555))
			Expect(ev.IsSensitive).To(BeFalse())
		})
	})

	Describe("AuditEvent.Validate (BR-STORAGE-001)", func() {
		It("returns nil when all required fields are present (BR-STORAGE-001)", func() {
			ev := audit.NewAuditEvent()
			ev.EventType = "gateway.signal.received"
			ev.EventCategory = "signal"
			ev.EventAction = "received"
			ev.EventOutcome = "success"
			ev.ActorType = "service"
			ev.ActorID = "gateway"
			ev.ResourceType = "Signal"
			ev.ResourceID = "sig-1"
			ev.CorrelationID = "corr-1"
			ev.EventData = validAuditEventData()
			Expect(ev.Validate()).To(Succeed())
		})

		DescribeTable("returns a clear error when a required field is missing (BR-STORAGE-001)",
			func(mutate func(*audit.AuditEvent), substr string) {
				ev := audit.NewAuditEvent()
				ev.EventType = "gateway.signal.received"
				ev.EventCategory = "signal"
				ev.EventAction = "received"
				ev.EventOutcome = "success"
				ev.ActorType = "service"
				ev.ActorID = "gateway"
				ev.ResourceType = "Signal"
				ev.ResourceID = "sig-1"
				ev.CorrelationID = "corr-1"
				ev.EventData = validAuditEventData()
				mutate(ev)
				Expect(ev.Validate()).To(MatchError(ContainSubstring(substr)))
			},
			Entry("event_type (BR-STORAGE-001)", func(e *audit.AuditEvent) { e.EventType = "" }, "event_type"),
			Entry("event_category (BR-STORAGE-001)", func(e *audit.AuditEvent) { e.EventCategory = "" }, "event_category"),
			Entry("event_action (BR-STORAGE-001)", func(e *audit.AuditEvent) { e.EventAction = "" }, "event_action"),
			Entry("event_outcome (BR-STORAGE-001)", func(e *audit.AuditEvent) { e.EventOutcome = "" }, "event_outcome"),
			Entry("actor_type (BR-STORAGE-001)", func(e *audit.AuditEvent) { e.ActorType = "" }, "actor_type"),
			Entry("actor_id (BR-STORAGE-001)", func(e *audit.AuditEvent) { e.ActorID = "" }, "actor_id"),
			Entry("resource_type (BR-STORAGE-001)", func(e *audit.AuditEvent) { e.ResourceType = "" }, "resource_type"),
			Entry("resource_id (BR-STORAGE-001)", func(e *audit.AuditEvent) { e.ResourceID = "" }, "resource_id"),
			Entry("correlation_id (BR-STORAGE-001)", func(e *audit.AuditEvent) { e.CorrelationID = "" }, "correlation_id"),
			Entry("event_data (BR-STORAGE-001)", func(e *audit.AuditEvent) { e.EventData = nil }, "event_data"),
			Entry("retention_days (BR-STORAGE-001)", func(e *audit.AuditEvent) { e.RetentionDays = 0 }, "retention_days"),
		)
	})

	Describe("CommonEnvelope helpers (BR-STORAGE-001)", func() {
		It("NewEventData sets version 1.0 and copies fields (BR-STORAGE-001)", func() {
			pl := map[string]interface{}{"a": 1}
			e := audit.NewEventData("gateway", "op", "ok", pl)
			Expect(e.Version).To(Equal("1.0"))
			Expect(e.Service).To(Equal("gateway"))
			Expect(e.Operation).To(Equal("op"))
			Expect(e.Status).To(Equal("ok"))
			Expect(e.Payload).To(Equal(pl))
		})

		It("WithSourcePayload mutates and returns the same pointer for chaining (BR-STORAGE-001)", func() {
			src := map[string]interface{}{"raw": true}
			e := audit.NewEventData("svc", "op", "ok", map[string]interface{}{})
			out := e.WithSourcePayload(src)
			Expect(out).To(BeIdenticalTo(e))
			Expect(e.SourcePayload).To(Equal(src))
		})

		It("ToJSON marshals the envelope and FromJSON round-trips (BR-STORAGE-001)", func() {
			e := audit.NewEventData("svc", "op", "pending", map[string]interface{}{"k": "v"})
			e.WithSourcePayload(map[string]interface{}{"orig": 1})
			raw, err := e.ToJSON()
			Expect(err).NotTo(HaveOccurred())
			got, err := audit.FromJSON(raw)
			Expect(err).NotTo(HaveOccurred())
			Expect(got.Validate()).To(Succeed())
			Expect(got.Service).To(Equal("svc"))
			Expect(got.Operation).To(Equal("op"))
			Expect(got.Status).To(Equal("pending"))
			Expect(got.Payload).To(HaveKeyWithValue("k", "v"))
			Expect(got.SourcePayload).To(HaveKeyWithValue("orig", float64(1))) // JSON numbers
		})

		It("ToJSON returns an error when JSON marshaling fails (BR-STORAGE-001)", func() {
			e := audit.NewEventData("svc", "op", "ok", map[string]interface{}{"ch": make(chan int)})
			_, err := e.ToJSON()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to marshal event data"))
		})

		It("FromJSON returns an error for invalid JSON (BR-STORAGE-001)", func() {
			_, err := audit.FromJSON([]byte(`{"version":`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to unmarshal event data"))
		})

		DescribeTable("CommonEnvelope.Validate required fields (BR-STORAGE-001)",
			func(mutate func(*audit.CommonEnvelope)) {
				env := audit.NewEventData("svc", "op", "ok", map[string]interface{}{})
				mutate(env)
				Expect(env.Validate()).To(HaveOccurred())
			},
			Entry("missing version (BR-STORAGE-001)", func(e *audit.CommonEnvelope) { e.Version = "" }),
			Entry("missing service (BR-STORAGE-001)", func(e *audit.CommonEnvelope) { e.Service = "" }),
			Entry("missing operation (BR-STORAGE-001)", func(e *audit.CommonEnvelope) { e.Operation = "" }),
			Entry("missing status (BR-STORAGE-001)", func(e *audit.CommonEnvelope) { e.Status = "" }),
			Entry("nil payload (BR-STORAGE-001)", func(e *audit.CommonEnvelope) { e.Payload = nil }),
		)
	})

	Describe("OpenAPI audit helpers (BR-STORAGE-001)", func() {
		It("SetClusterName SetDuration and SetSeverity set OptNil fields on AuditEventRequest (BR-STORAGE-001)", func() {
			req := audit.NewAuditEventRequest()
			audit.SetClusterName(req, "kind-local")
			audit.SetDuration(req, 42)
			audit.SetSeverity(req, "warning")

			cn, ok := req.ClusterName.Get()
			Expect(ok).To(BeTrue())
			Expect(cn).To(Equal("kind-local"))

			dur, ok := req.DurationMs.Get()
			Expect(ok).To(BeTrue())
			Expect(dur).To(Equal(42))

			sev, ok := req.Severity.Get()
			Expect(ok).To(BeTrue())
			Expect(sev).To(Equal("warning"))
		})
	})
})

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

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server/helpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("UT-DS-1048-P5: Actor Identity Enrichment (AU-3)", func() {
	var baseReq *ogenclient.AuditEventRequest

	BeforeEach(func() {
		baseReq = &ogenclient.AuditEventRequest{
			Version:        "1.0",
			EventType:      "test.event",
			EventCategory:  ogenclient.AuditEventRequestEventCategory("remediation"),
			EventAction:    "create",
			EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
			EventTimestamp: time.Now().UTC().Truncate(time.Millisecond),
			CorrelationID:  "corr-enrichment-test",
			EventData:        ogenclient.AuditEventRequestEventData{},
		}
	})

	Describe("UT-DS-1048-P5-040: Header present overrides body actor_id", func() {
		It("should use authenticated actor from header", func() {
			baseReq.ActorType.SetTo("service")
			baseReq.ActorID.SetTo("other-service")

			event, err := helpers.ConvertAuditEventRequest(*baseReq, "user@example.com")
			Expect(err).NotTo(HaveOccurred())
			Expect(event.ActorID).To(Equal("user@example.com"))
			Expect(event.ActorType).To(Equal("user"))
		})
	})

	Describe("UT-DS-1048-P5-041: Header absent uses body actor_id", func() {
		It("should use body actor_id when header is absent", func() {
			baseReq.ActorType.SetTo("service")
			baseReq.ActorID.SetTo("gateway-service")

			event, err := helpers.ConvertAuditEventRequest(*baseReq, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(event.ActorID).To(Equal("gateway-service"))
			Expect(event.ActorType).To(Equal("service"))
		})
	})

	Describe("UT-DS-1048-P5-042: Both absent uses synthetic default", func() {
		It("should generate synthetic actor_id from category", func() {
			event, err := helpers.ConvertAuditEventRequest(*baseReq, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(event.ActorID).To(Equal("remediation-service"))
			Expect(event.ActorType).To(Equal("service"))
		})
	})

	Describe("UT-DS-1048-P5-043: Empty header treated as absent", func() {
		It("should treat empty header as absent", func() {
			baseReq.ActorType.SetTo("service")
			baseReq.ActorID.SetTo("test-actor")

			event, err := helpers.ConvertAuditEventRequest(*baseReq, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(event.ActorID).To(Equal("test-actor"))
			Expect(event.ActorType).To(Equal("service"))
		})
	})

	Describe("UT-DS-1048-P5-045: actor_type set correctly", func() {
		It("should set actor_type to 'user' when header present", func() {
			event, err := helpers.ConvertAuditEventRequest(*baseReq, "admin@company.com")
			Expect(err).NotTo(HaveOccurred())
			Expect(event.ActorType).To(Equal("user"))
		})

		It("should preserve actor_type from body when header absent", func() {
			baseReq.ActorType.SetTo("external")
			baseReq.ActorID.SetTo("aws-cloudwatch")

			event, err := helpers.ConvertAuditEventRequest(*baseReq, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(event.ActorType).To(Equal("external"))
		})
	})
})

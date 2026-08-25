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
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	pkgaudit "github.com/jordigilh/kubernaut/pkg/audit"
	dsaudit "github.com/jordigilh/kubernaut/pkg/datastorage/audit"
)

// GAP-11 (Issue #2285): Data Storage's own outbound CA-cert hot-reload had
// no audit trail parity with every other hot-reloadable component
// (SOC2 CC7.2 / FedRAMP CM-3, AU-2, AU-12).
var _ = Describe("Config reload self-audit events (GAP-11, Issue #2285)", Label("unit", "audit", "config-reload"), func() {

	Describe("UT-DS-2285-001: NewConfigReloadedAuditEvent", func() {
		It("produces a well-formed success event with the ca_cert component", func() {
			event := dsaudit.NewConfigReloadedAuditEvent("ca_cert")

			Expect(event.EventType).To(Equal("datastorage.config.reloaded"))
			Expect(string(event.EventCategory)).To(Equal("security"))
			Expect(event.EventAction).To(Equal("reloaded"))
			Expect(event.EventOutcome).To(Equal(pkgaudit.OutcomeSuccess))
			Expect(event.CorrelationID).NotTo(BeEmpty(), "correlation_id must not be empty (OpenAPI minLength: 1)")

			Expect(event.EventData.IsDatastorageConfigReloadedPayload()).To(BeTrue())
			payload := event.EventData.DatastorageConfigReloadedPayload
			Expect(payload.Component).To(Equal("ca_cert"))
		})
	})

	Describe("UT-DS-2285-002: NewConfigRejectedAuditEvent", func() {
		It("produces a well-formed failure event carrying the rejection reason", func() {
			event := dsaudit.NewConfigRejectedAuditEvent("ca_cert", errors.New("invalid PEM content"))

			Expect(event.EventType).To(Equal("datastorage.config.rejected"))
			Expect(string(event.EventCategory)).To(Equal("security"))
			Expect(event.EventAction).To(Equal("rejected"))
			Expect(event.EventOutcome).To(Equal(pkgaudit.OutcomeFailure))

			Expect(event.EventData.IsDatastorageConfigRejectedPayload()).To(BeTrue())
			payload := event.EventData.DatastorageConfigRejectedPayload
			Expect(payload.Component).To(Equal("ca_cert"))
			Expect(payload.RejectionReason).To(Equal("invalid PEM content"))
		})
	})
})

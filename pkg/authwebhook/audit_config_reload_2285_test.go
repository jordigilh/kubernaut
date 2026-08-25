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

// audit_config_reload_2285_test contains unit tests for AuthWebhook's
// CA-cert hot-reload audit-trail parity (GAP-11, Issue #2285).
//
// BR-AUDIT-002 / DD-AUDIT-003: mirrors Gateway's shipped
// gateway.config.{reloaded,rejected} events.
package authwebhook_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

var _ = Describe("RecordConfigReloaded (GAP-11, Issue #2285)", func() {
	var (
		mockStore *MockAuditStore
		ctx       context.Context
	)

	BeforeEach(func() {
		mockStore = &MockAuditStore{}
		ctx = context.Background()
	})

	It("UT-AW-2285-001: emits authwebhook.config.reloaded on successful reload", func() {
		authwebhook.RecordConfigReloaded(ctx, mockStore, "ca_cert", nil)

		Expect(mockStore.StoredEvents).To(HaveLen(1))
		event := mockStore.StoredEvents[0]
		Expect(event.EventType).To(Equal(authwebhook.EventTypeConfigReloaded))
		Expect(event.EventOutcome).To(Equal(ogenclient.AuditEventRequestEventOutcomeSuccess))

		payload, ok := event.EventData.GetAuthwebhookConfigReloadedPayload()
		Expect(ok).To(BeTrue())
		Expect(payload.Component).To(Equal("ca_cert"))
	})

	It("UT-AW-2285-002: emits authwebhook.config.rejected with reason on failed reload", func() {
		reloadErr := errors.New("invalid PEM content")
		authwebhook.RecordConfigReloaded(ctx, mockStore, "ca_cert", reloadErr)

		Expect(mockStore.StoredEvents).To(HaveLen(1))
		event := mockStore.StoredEvents[0]
		Expect(event.EventType).To(Equal(authwebhook.EventTypeConfigRejected))
		Expect(event.EventOutcome).To(Equal(ogenclient.AuditEventRequestEventOutcomeFailure))

		payload, ok := event.EventData.GetAuthwebhookConfigRejectedPayload()
		Expect(ok).To(BeTrue())
		Expect(payload.Component).To(Equal("ca_cert"))
		Expect(payload.RejectionReason).To(Equal("invalid PEM content"))
	})

	It("UT-AW-2285-003: does not panic and does not store when audit store is nil", func() {
		Expect(func() {
			authwebhook.RecordConfigReloaded(ctx, nil, "ca_cert", nil)
		}).NotTo(Panic())
	})
})

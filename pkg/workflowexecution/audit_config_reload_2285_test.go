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

// audit_config_reload_2285_test contains unit tests for WorkflowExecution's
// CA-cert hot-reload audit-trail parity (GAP-11, Issue #2285).
//
// BR-AUDIT-002 / DD-AUDIT-003: mirrors Gateway's shipped
// gateway.config.{reloaded,rejected} events.
package workflowexecution_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/workflowexecution/audit"
)

var _ = Describe("Audit Manager — RecordConfigReloaded (GAP-11, Issue #2285)", func() {
	var (
		store *mockAuditStore
		mgr   *audit.Manager
		ctx   context.Context
	)

	BeforeEach(func() {
		store = &mockAuditStore{events: make([]*ogenclient.AuditEventRequest, 0)}
		mgr = audit.NewManager(store, logr.Discard())
		ctx = context.Background()
	})

	It("UT-WE-2285-001: emits workflowexecution.config.reloaded on successful reload", func() {
		mgr.RecordConfigReloaded(ctx, "ca_cert", nil)

		Expect(store.events).To(HaveLen(1))
		event := store.events[0]
		Expect(event.EventType).To(Equal(audit.EventTypeConfigReloaded))
		Expect(event.EventOutcome).To(Equal(ogenclient.AuditEventRequestEventOutcomeSuccess))

		payload, ok := event.EventData.GetWorkflowExecutionConfigReloadedPayload()
		Expect(ok).To(BeTrue())
		Expect(payload.Component).To(Equal("ca_cert"))
	})

	It("UT-WE-2285-002: emits workflowexecution.config.rejected with reason on failed reload", func() {
		reloadErr := errors.New("invalid PEM content")
		mgr.RecordConfigReloaded(ctx, "ca_cert", reloadErr)

		Expect(store.events).To(HaveLen(1))
		event := store.events[0]
		Expect(event.EventType).To(Equal(audit.EventTypeConfigRejected))
		Expect(event.EventOutcome).To(Equal(ogenclient.AuditEventRequestEventOutcomeFailure))

		payload, ok := event.EventData.GetWorkflowExecutionConfigRejectedPayload()
		Expect(ok).To(BeTrue())
		Expect(payload.Component).To(Equal("ca_cert"))
		Expect(payload.RejectionReason).To(Equal("invalid PEM content"))
	})

	It("UT-WE-2285-003: does not panic and does not store when audit store is nil", func() {
		nilStoreMgr := audit.NewManager(nil, logr.Discard())
		Expect(func() {
			nilStoreMgr.RecordConfigReloaded(ctx, "ca_cert", nil)
		}).NotTo(Panic())
	})
})

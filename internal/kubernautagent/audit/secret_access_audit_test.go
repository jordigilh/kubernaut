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
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// ========================================
// GAP-13 (Issue #1505): KubernautAgent Secret access audit event
// ========================================
//
// Detective control compensating for KubernautAgent's intentionally broad
// read RBAC on Secrets: every Get/List against the core Secret resource must
// produce an independently queryable aiagent.secret.accessed event.
// ========================================

var _ = Describe("GAP-13: Secret Access Audit Event", func() {
	Describe("buildEventData produces correct payload for aiagent.secret.accessed", func() {
		It("populates a success payload for a get", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			event := audit.NewEvent(audit.EventTypeSecretAccessed, "rr-1505-test")
			event.EventAction = audit.ActionSecretAccessed
			event.EventOutcome = audit.OutcomeSuccess
			event.Data["verb"] = "get"
			event.Data["namespace"] = "prod"
			event.Data["secret_name"] = "db-creds"

			Expect(store.StoreAudit(context.Background(), event)).To(Succeed())
			Expect(recorder.calls).To(HaveLen(1))

			req := recorder.calls[0]
			Expect(req.EventType).To(Equal(audit.EventTypeSecretAccessed))
			Expect(req.EventOutcome).To(Equal(ogenclient.AuditEventRequestEventOutcomeSuccess))
			Expect(req.EventData.Type).NotTo(BeEmpty(),
				"buildEventData must handle EventTypeSecretAccessed and set a discriminator type")

			payload, ok := req.EventData.GetAIAgentSecretAccessedPayload()
			Expect(ok).To(BeTrue())
			Expect(payload.Verb).To(Equal(ogenclient.AIAgentSecretAccessedPayloadVerbGet))
			Expect(payload.Namespace.Value).To(Equal("prod"))
			Expect(payload.SecretName.Value).To(Equal("db-creds"))
			Expect(payload.OutcomeDetail.Set).To(BeFalse())
		})

		It("populates a failure payload with outcome_detail for a list", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			event := audit.NewEvent(audit.EventTypeSecretAccessed, "rr-1505-test-2")
			event.EventAction = audit.ActionSecretAccessed
			event.EventOutcome = audit.OutcomeFailure
			event.Data["verb"] = "list"
			event.Data["namespace"] = "kube-system"
			event.Data["outcome_detail"] = "forbidden: user cannot list secrets in kube-system"

			Expect(store.StoreAudit(context.Background(), event)).To(Succeed())
			Expect(recorder.calls).To(HaveLen(1))

			payload, ok := recorder.calls[0].EventData.GetAIAgentSecretAccessedPayload()
			Expect(ok).To(BeTrue())
			Expect(payload.Verb).To(Equal(ogenclient.AIAgentSecretAccessedPayloadVerbList))
			Expect(payload.SecretName.Set).To(BeFalse(), "secret_name must be omitted for list operations")
			Expect(payload.OutcomeDetail.Value).To(Equal("forbidden: user cannot list secrets in kube-system"))
		})
	})

	Describe("EventTypeSecretAccessed is registered in AllEventTypes", func() {
		It("includes aiagent.secret.accessed", func() {
			Expect(audit.AllEventTypes).To(ContainElement(audit.EventTypeSecretAccessed))
		})

		It("has the correct event type string value", func() {
			Expect(audit.EventTypeSecretAccessed).To(Equal("aiagent.secret.accessed"))
		})

		It("has a non-empty action constant", func() {
			Expect(audit.ActionSecretAccessed).NotTo(BeEmpty())
		})
	})
})

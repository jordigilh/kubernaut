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
)

var _ = Describe("K8s Call Audit Event — #898, BR-INTERACTIVE-003, BR-AUDIT-005", func() {

	Describe("UT-KA-898-006: buildEventData produces correct payload for k8s_call event", func() {
		It("should populate event_data when event type is aiagent.interactive.k8s_call", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			event := audit.NewEvent(audit.EventTypeInteractiveK8sCall, "rr-898-test")
			event.EventAction = audit.ActionInteractiveK8sCall
			event.EventOutcome = audit.OutcomeSuccess
			event.SessionID = "sess-898"
			event.ActingUser = "jane@example.com"
			event.Data["resource"] = "pods"
			event.Data["verb"] = "get"
			event.Data["namespace"] = "default"
			event.Data["resource_name"] = "my-pod"
			event.Data["http_status_code"] = 200

			err := store.StoreAudit(context.Background(), event)
			Expect(err).NotTo(HaveOccurred())
			Expect(recorder.calls).To(HaveLen(1))

			req := recorder.calls[0]
			Expect(req.EventData.Type).NotTo(BeEmpty(),
				"buildEventData must handle EventTypeInteractiveK8sCall and set a discriminator type")
		})
	})

	Describe("UT-KA-898-007: EventTypeInteractiveK8sCall is registered in AllEventTypes", func() {
		It("should include the k8s_call event type in AllEventTypes", func() {
			Expect(audit.AllEventTypes).To(ContainElement(audit.EventTypeInteractiveK8sCall),
				"AllEventTypes must include aiagent.interactive.k8s_call for event type validation")
		})

		It("should have a total of 25 event types after adding k8s_call", func() {
			Expect(audit.AllEventTypes).To(HaveLen(25))
		})

		It("should have the correct event type string value", func() {
			Expect(audit.EventTypeInteractiveK8sCall).To(Equal("aiagent.interactive.k8s_call"))
		})

		It("should have a non-empty action constant for k8s_call", func() {
			Expect(audit.ActionInteractiveK8sCall).NotTo(BeEmpty())
		})
	})
})

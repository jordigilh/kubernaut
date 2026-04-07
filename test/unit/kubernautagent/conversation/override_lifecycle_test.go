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

package conversation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/conversation"
)

var _ = Describe("Override + Lifecycle — #592", func() {

	Describe("UT-CS-592-019: Override validates workflow against catalog", func() {
		It("should reject an override for a workflow not in the catalog", func() {
			catalog := &mockCatalog{workflows: map[string]bool{
				"oom-recovery-v2.0": true,
			}}
			advisor := conversation.NewOverrideAdvisor(catalog)
			session := &conversation.Session{
				State: conversation.SessionInteractive,
			}

			valid, reason := advisor.ValidateOverride("nonexistent-workflow", session)
			Expect(valid).To(BeFalse(), "override to non-existent workflow must be rejected")
			Expect(reason).To(ContainSubstring("not found"),
				"rejection reason should mention workflow not found")
		})

		It("should accept an override for a workflow in the catalog", func() {
			catalog := &mockCatalog{workflows: map[string]bool{
				"oom-recovery-v2.0": true,
			}}
			advisor := conversation.NewOverrideAdvisor(catalog)
			session := &conversation.Session{
				State: conversation.SessionInteractive,
			}

			valid, _ := advisor.ValidateOverride("oom-recovery-v2.0", session)
			Expect(valid).To(BeTrue(), "override to existing workflow must be accepted")
		})
	})

	Describe("UT-CS-592-020: Override disabled when RAR decided", func() {
		It("should reject override when session is read-only (RAR Approved)", func() {
			catalog := &mockCatalog{workflows: map[string]bool{
				"oom-recovery-v2.0": true,
			}}
			advisor := conversation.NewOverrideAdvisor(catalog)
			session := &conversation.Session{
				State: conversation.SessionReadOnly,
			}

			valid, reason := advisor.ValidateOverride("oom-recovery-v2.0", session)
			Expect(valid).To(BeFalse(), "override must be rejected for read-only session")
			Expect(reason).To(ContainSubstring("read-only"),
				"rejection reason should mention read-only state")
		})
	})

	Describe("UT-CS-592-021: RAR Approved -> session IsReadOnly", func() {
		It("should transition session to read-only when RAR is Approved", func() {
			lm := conversation.NewLifecycleManager()
			session := &conversation.Session{
				State: conversation.SessionInteractive,
			}

			lm.ApplyRARDecision(session, "Approved")

			Expect(session.IsReadOnly()).To(BeTrue(),
				"session must be read-only after RAR Approved")
			Expect(session.IsInteractive()).To(BeFalse())
		})
	})

	Describe("UT-CS-592-022: RAR Expired -> session IsClosed", func() {
		It("should transition session to closed when RAR is Expired", func() {
			lm := conversation.NewLifecycleManager()
			session := &conversation.Session{
				State: conversation.SessionInteractive,
			}

			lm.ApplyRARDecision(session, "Expired")

			Expect(session.IsClosed()).To(BeTrue(),
				"session must be closed after RAR Expired")
			Expect(session.IsInteractive()).To(BeFalse())
		})
	})

	Describe("UT-CS-592-025: RAR pending -> session remains interactive", func() {
		It("should keep session interactive when RAR decision is empty", func() {
			lm := conversation.NewLifecycleManager()
			session := &conversation.Session{
				State: conversation.SessionInteractive,
			}

			lm.ApplyRARDecision(session, "")

			Expect(session.IsInteractive()).To(BeTrue(),
				"session must remain interactive when RAR decision is pending")
		})
	})

	Describe("UT-CS-592-033: RR completed/failed -> session read-only for review", func() {
		It("should transition session to read-only when RR is completed", func() {
			lm := conversation.NewLifecycleManager()
			session := &conversation.Session{
				State: conversation.SessionInteractive,
			}

			lm.ApplyRRCompletion(session, "Completed")

			Expect(session.IsReadOnly()).To(BeTrue(),
				"session must be read-only after RR Completed")
		})

		It("should transition session to read-only when RR has failed", func() {
			lm := conversation.NewLifecycleManager()
			session := &conversation.Session{
				State: conversation.SessionInteractive,
			}

			lm.ApplyRRCompletion(session, "Failed")

			Expect(session.IsReadOnly()).To(BeTrue(),
				"session must be read-only after RR Failed")
		})
	})
})

type mockCatalog struct {
	workflows map[string]bool
}

func (m *mockCatalog) Exists(workflowID string) bool {
	return m.workflows[workflowID]
}

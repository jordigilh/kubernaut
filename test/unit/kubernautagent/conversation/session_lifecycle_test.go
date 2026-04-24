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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/conversation"
)

var _ = Describe("Session Lifecycle & Tool Guardrails — #592", func() {

	Describe("UT-CS-592-025: RAR pending → session remains interactive", func() {
		It("should create sessions in interactive state by default (pending RAR)", func() {
			mgr := conversation.NewSessionManager(30*time.Minute, nil)
			s, err := mgr.Create("rar-pending", "ns-1", "user:alice", "")
			Expect(err).ToNot(HaveOccurred())

			Expect(s.State).To(Equal(conversation.SessionInteractive),
				"new session for a pending RAR must start in interactive state")
			Expect(s.IsInteractive()).To(BeTrue(),
				"IsInteractive() must return true for a pending RAR session")
			Expect(s.IsReadOnly()).To(BeFalse(),
				"IsReadOnly() must be false when session is interactive")
			Expect(s.IsClosed()).To(BeFalse(),
				"IsClosed() must be false when session is interactive")
		})
	})

	Describe("UT-CS-592-030: Session TTL — inactive sessions expire", func() {
		It("should expire a session that has been idle beyond its TTL", func() {
			ttl := 100 * time.Millisecond
			mgr := conversation.NewSessionManager(ttl, nil)
			s, err := mgr.Create("rar-ttl", "ns-1", "user:alice", "")
			Expect(err).ToNot(HaveOccurred())

			_, err = mgr.Get(s.ID)
			Expect(err).ToNot(HaveOccurred(),
				"session should be retrievable immediately after creation")

			time.Sleep(ttl + 50*time.Millisecond) // ✅ APPROVED EXCEPTION: intentional TTL expiry timing test

			_, err = mgr.Get(s.ID)
			Expect(err).To(HaveOccurred(),
				"session must not be retrievable after TTL has elapsed")
			Expect(err.Error()).To(ContainSubstring("expired"),
				"error message should indicate expiry, not 'not found'")
		})
	})

	Describe("UT-CS-592-031: Read-only tool call succeeds during conversation", func() {
		It("should allow kubectl get in the session namespace", func() {
			g := conversation.NewGuardrails("production", "rr-oom-1")

			err := g.ValidateToolCall("kubectl_get_by_name", map[string]interface{}{
				"namespace": "production",
			})
			Expect(err).ToNot(HaveOccurred(),
				"read-only tool call in the correct namespace must succeed")
		})

		It("should allow kubectl describe without explicit namespace", func() {
			g := conversation.NewGuardrails("production", "rr-oom-1")

			err := g.ValidateToolCall("kubectl_describe", map[string]interface{}{})
			Expect(err).ToNot(HaveOccurred(),
				"read-only tool call without namespace arg must succeed (namespace enforced at tool level)")
		})
	})

	Describe("UT-CS-592-032: Mutating tool call rejected", func() {
		It("should reject kubectl delete even in the correct namespace", func() {
			g := conversation.NewGuardrails("production", "rr-oom-1")

			err := g.ValidateToolCall("kubectl_delete", map[string]interface{}{
				"namespace": "production",
			})
			Expect(err).To(HaveOccurred(),
				"mutating tool call must be rejected in conversation mode")
			Expect(err.Error()).To(ContainSubstring("read-only"),
				"error should explain that only read-only operations are allowed")
		})

		It("should reject kubectl apply in any namespace", func() {
			g := conversation.NewGuardrails("production", "rr-oom-1")

			err := g.ValidateToolCall("kubectl_apply", map[string]interface{}{
				"namespace": "production",
			})
			Expect(err).To(HaveOccurred(),
				"kubectl_apply is a mutating operation and must be rejected")
		})
	})

	Describe("UT-CS-592-033: RR completed/failed → session read-only", func() {
		It("should transition to read-only when State is set to SessionReadOnly", func() {
			mgr := conversation.NewSessionManager(30*time.Minute, nil)
			s, err := mgr.Create("rar-done", "ns-1", "user:alice", "")
			Expect(err).ToNot(HaveOccurred())
			Expect(s.IsInteractive()).To(BeTrue())

			s.State = conversation.SessionReadOnly

			Expect(s.IsReadOnly()).To(BeTrue(),
				"session must be read-only after RAR approval or RR completion")
			Expect(s.IsInteractive()).To(BeFalse(),
				"session must not be interactive when read-only")
			Expect(s.IsClosed()).To(BeFalse(),
				"read-only is distinct from closed — allows viewing history")
		})

		It("should transition to closed when State is set to SessionClosed", func() {
			mgr := conversation.NewSessionManager(30*time.Minute, nil)
			s, err := mgr.Create("rar-expired", "ns-1", "user:alice", "")
			Expect(err).ToNot(HaveOccurred())

			s.State = conversation.SessionClosed

			Expect(s.IsClosed()).To(BeTrue(),
				"session must be closed after RAR expiry")
			Expect(s.IsReadOnly()).To(BeFalse())
			Expect(s.IsInteractive()).To(BeFalse())
		})
	})
})

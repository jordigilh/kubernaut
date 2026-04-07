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
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/conversation"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
)

// stubTool implements tools.Tool for test purposes.
type stubTool struct {
	name string
}

func (s *stubTool) Name() string                                                    { return s.name }
func (s *stubTool) Description() string                                             { return s.name + " desc" }
func (s *stubTool) Parameters() json.RawMessage                                     { return json.RawMessage(`{}`) }
func (s *stubTool) Execute(_ context.Context, _ json.RawMessage) (string, error)    { return "", nil }

var _ = Describe("Session Guardrails — #592 Phase 2", func() {

	Describe("UT-CS-592-C2-001: Per-session guardrails scoping", func() {
		It("should create sessions with distinct guardrails scoped to their RAR namespace", func() {
			mgr := conversation.NewSessionManager(30*time.Minute, nil)

			s1, err := mgr.Create("rar-prod", "production", "user:alice", "")
			Expect(err).ToNot(HaveOccurred())
			Expect(s1.Guardrails.ReadOnlyToolNames()).To(ContainElement("kubectl_get_by_name"),
				"session guardrails must expose read-only tools for its namespace")

			s2, err := mgr.Create("rar-staging", "staging", "user:bob", "")
			Expect(err).ToNot(HaveOccurred())
			Expect(s2.Guardrails.ReadOnlyToolNames()).To(ContainElement("kubectl_get_by_name"),
				"session guardrails must expose read-only tools for its namespace")

			Expect(s1.Guardrails.ValidateToolCall("kubectl_get", map[string]interface{}{
				"namespace": "production",
			})).To(Succeed(), "production session should allow production namespace")

			Expect(s1.Guardrails.ValidateToolCall("kubectl_get", map[string]interface{}{
				"namespace": "staging",
			})).To(HaveOccurred(), "production session should reject staging namespace")

			Expect(s2.Guardrails.ValidateToolCall("kubectl_get", map[string]interface{}{
				"namespace": "staging",
			})).To(Succeed(), "staging session should allow staging namespace")
		})
	})

	Describe("UT-CS-592-C2-002: ReadOnlyToolNames returns sorted tool names", func() {
		It("should return a sorted, deterministic list of read-only tool names", func() {
			g := conversation.NewGuardrails("ns", "rr")
			names := g.ReadOnlyToolNames()

			Expect(len(names)).To(BeNumerically(">=", 10),
				"must expose a comprehensive set of read-only K8s tools")
			Expect(names).To(ContainElement("kubectl_get_by_name"))
			Expect(names).To(ContainElement("kubectl_describe"))
			Expect(names).NotTo(ContainElement("todo_write"),
				"todo_write is per-session injected, not in the static read-only set")

			for i := 1; i < len(names); i++ {
				Expect(names[i] > names[i-1]).To(BeTrue(),
					"tool names must be sorted lexicographically: %s should come after %s", names[i], names[i-1])
			}
		})
	})

	Describe("UT-CS-592-C2-003: IncrementTurnAndTouch atomic mutation", func() {
		It("should atomically increment TurnCount and update LastActivity", func() {
			mgr := conversation.NewSessionManager(30*time.Minute, nil)
			s, err := mgr.Create("rar-1", "ns-1", "user:alice", "")
			Expect(err).ToNot(HaveOccurred())
			Expect(s.TurnCount).To(Equal(0))

			now := time.Now()
			count, err := mgr.IncrementTurnAndTouch(s.ID, now)
			Expect(err).ToNot(HaveOccurred())
			Expect(count).To(Equal(1))

			s2, _ := mgr.Get(s.ID)
			Expect(s2.TurnCount).To(Equal(1))
			Expect(s2.LastActivity).To(BeTemporally("~", now, time.Second))
		})

		It("should return error for non-existent session", func() {
			mgr := conversation.NewSessionManager(30*time.Minute, nil)
			_, err := mgr.IncrementTurnAndTouch("non-existent", time.Now())
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("UT-CS-592-C2-004: Rate limit check-then-increment pattern", func() {
		It("should not consume a turn when rate limit is exceeded", func() {
			limiter := conversation.NewRateLimiter(100, 2)

			Expect(limiter.AllowSession("s1", 1)).To(BeTrue(), "turn 1 within limit")
			Expect(limiter.AllowSession("s1", 2)).To(BeTrue(), "turn 2 at limit")
			Expect(limiter.AllowSession("s1", 3)).To(BeFalse(), "turn 3 exceeds limit")
		})
	})

	Describe("UT-CS-592-C2-005: FilterTools returns only read-only tools", func() {
		It("should filter tools using IsReadOnlyTool (map + prefix match)", func() {
			g := conversation.NewGuardrails("ns", "rr")

			allTools := []tools.Tool{
				&stubTool{name: "kubectl_get_by_name"},
				&stubTool{name: "kubectl_delete"},
				&stubTool{name: "kubectl_describe"},
				&stubTool{name: "todo_write"},
				&stubTool{name: "dangerous_mutate"},
				&stubTool{name: "kubectl_get_custom"},
			}

			filtered := g.FilterTools(allTools)
			filteredNames := make([]string, len(filtered))
			for i, t := range filtered {
				filteredNames[i] = t.Name()
			}

			Expect(filteredNames).To(ContainElement("kubectl_get_by_name"),
				"exact map match")
			Expect(filteredNames).To(ContainElement("kubectl_describe"),
				"exact map match")
			Expect(filteredNames).To(ContainElement("kubectl_get_custom"),
				"prefix match via IsReadOnlyTool")
			Expect(filteredNames).ToNot(ContainElement("todo_write"),
				"todo_write is per-session injected, not in read-only set")
			Expect(filteredNames).ToNot(ContainElement("kubectl_delete"))
			Expect(filteredNames).ToNot(ContainElement("dangerous_mutate"))
		})
	})
})

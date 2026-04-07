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

	"github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/conversation"
)

var _ = Describe("Config + TTL + Read-Only Tools — #592", func() {

	Describe("UT-CS-592-023: Conversation LLM config defaults to investigation model", func() {
		It("should return the investigation LLM config when conversation LLM is nil", func() {
			convCfg := &config.ConversationConfig{
				Enabled: true,
				LLM:     nil,
			}
			investigationLLM := config.LLMConfig{
				Provider: "openai",
				Model:    "gpt-4",
				Endpoint: "https://api.openai.com/v1",
			}

			effective := convCfg.EffectiveLLM(investigationLLM)
			Expect(effective.Model).To(Equal("gpt-4"),
				"when conversation LLM is nil, should fall back to investigation model")
			Expect(effective.Provider).To(Equal("openai"))
		})
	})

	Describe("UT-CS-592-024: Conversation LLM config uses override when set", func() {
		It("should return the conversation-specific LLM config when explicitly set", func() {
			convCfg := &config.ConversationConfig{
				Enabled: true,
				LLM: &config.LLMConfig{
					Provider: "openai",
					Model:    "gpt-3.5-turbo",
					Endpoint: "https://api.openai.com/v1",
				},
			}
			investigationLLM := config.LLMConfig{
				Provider: "openai",
				Model:    "gpt-4",
				Endpoint: "https://api.openai.com/v1",
			}

			effective := convCfg.EffectiveLLM(investigationLLM)
			Expect(effective.Model).To(Equal("gpt-3.5-turbo"),
				"when conversation LLM is set, should use it instead of investigation model")
			Expect(effective.Provider).To(Equal("openai"))
		})
	})

	Describe("UT-CS-592-030: Session TTL expiry", func() {
		It("should expire sessions that exceed TTL", func() {
			mgr := conversation.NewSessionManager(50*time.Millisecond, nil)

			session, err := mgr.Create("payment-svc-oomkill", "production", "user-1", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(session).NotTo(BeNil())

			retrieved, err := mgr.Get(session.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(retrieved).NotTo(BeNil(), "session should exist immediately after creation")

			time.Sleep(100 * time.Millisecond) // ✅ APPROVED EXCEPTION: deterministic TTL expiry test with 50ms TTL

			expired, err := mgr.Get(session.ID)
			Expect(err).To(HaveOccurred(),
				"session should not be retrievable after TTL expiry")
			Expect(expired).To(BeNil())
		})
	})

	Describe("UT-CS-592-031: Read-only tool call succeeds during conversation", func() {
		It("should allow read-only kubectl operations in the correct namespace", func() {
			g := conversation.NewGuardrails("production", "payment-svc-oomkill")

			Expect(g.IsReadOnlyTool("kubectl_get")).To(BeTrue(),
				"kubectl_get is a read-only tool and must be allowed")
			Expect(g.IsReadOnlyTool("kubectl_describe")).To(BeTrue(),
				"kubectl_describe is a read-only tool and must be allowed")
			Expect(g.IsReadOnlyTool("kubectl_logs")).To(BeTrue(),
				"kubectl_logs is a read-only tool and must be allowed")

			err := g.ValidateToolCall("kubectl_get", map[string]interface{}{
				"namespace": "production",
				"kind":      "Pod",
			})
			Expect(err).NotTo(HaveOccurred(),
				"read-only tool call in the correct namespace must succeed")
		})
	})

	Describe("UT-CS-592-032: Mutating tool call rejected", func() {
		It("should reject mutating tool calls even in the correct namespace", func() {
			g := conversation.NewGuardrails("production", "payment-svc-oomkill")

			Expect(g.IsReadOnlyTool("kubectl_delete")).To(BeFalse(),
				"kubectl_delete is a mutating tool and must NOT be classified as read-only")

			err := g.ValidateToolCall("kubectl_delete", map[string]interface{}{
				"namespace": "production",
				"kind":      "Pod",
				"name":      "payment-svc-abc123",
			})
			Expect(err).To(HaveOccurred(),
				"mutating tool calls must be rejected during conversation, even in the correct namespace")
			Expect(err.Error()).To(ContainSubstring("read-only"),
				"error should indicate that only read-only operations are allowed")
		})
	})
})

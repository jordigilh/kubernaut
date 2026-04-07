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

package prompt_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
)

var _ = Describe("Conversation Prompt — #592 Phase 4", func() {
	var builder *prompt.Builder

	BeforeEach(func() {
		var err error
		builder, err = prompt.NewBuilder()
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("UT-CS-592-G2-001: RenderConversation with all fields", func() {
		It("should render template with all fields populated", func() {
			result, err := builder.RenderConversation(prompt.ConversationTemplateData{
				RARName:              "payment-svc-oomkill",
				Namespace:            "production",
				AvailableTools:       []string{"kubectl_get", "kubectl_describe", "kubectl_logs"},
				InvestigationSummary: "Pod exceeded memory limits due to leak in /api/payments handler",
				AuditHistory:         "Turn 1: User asked about OOM cause. Turn 2: Agent checked pod logs.",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(ContainSubstring("payment-svc-oomkill"))
			Expect(result).To(ContainSubstring("production"))
			Expect(result).To(ContainSubstring("kubectl_get"))
			Expect(result).To(ContainSubstring("kubectl_describe"))
			Expect(result).To(ContainSubstring("exceeded memory limits"))
			Expect(result).To(ContainSubstring("Turn 1"))
		})
	})

	Describe("UT-CS-592-G2-002: RenderConversation with empty optional fields", func() {
		It("should render cleanly without optional fields", func() {
			result, err := builder.RenderConversation(prompt.ConversationTemplateData{
				RARName:   "rar-basic",
				Namespace: "default",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(ContainSubstring("rar-basic"))
			Expect(result).To(ContainSubstring("default"))
			Expect(result).ToNot(ContainSubstring("INVESTIGATION CONTEXT"))
			Expect(result).ToNot(ContainSubstring("CONVERSATION HISTORY"))
		})
	})

	Describe("UT-CS-592-G2-003: Template includes scoping and override advisory", func() {
		It("should include namespace scoping rules and override advisory", func() {
			result, err := builder.RenderConversation(prompt.ConversationTemplateData{
				RARName:   "rar-scope",
				Namespace: "staging",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(ContainSubstring("scoped to namespace"))
			Expect(result).To(ContainSubstring("OVERRIDE ADVISORY"))
		})
	})
})

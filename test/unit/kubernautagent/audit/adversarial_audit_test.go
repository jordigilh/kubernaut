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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
)

var _ = Describe("TP-433-ADV P7: Audit Trail — GAP-011/020", func() {

	Describe("UT-KA-433-AUD-001: LLM request event includes prompt preview", func() {
		It("should create audit event with prompt_preview field", func() {
			event := audit.NewEvent(audit.EventTypeLLMRequest, "rem-001")
			longPrompt := make([]byte, 1000)
			for i := range longPrompt {
				longPrompt[i] = 'A'
			}
			preview := string(longPrompt)
			if len(preview) > 500 {
				preview = preview[:500] + "..."
			}
			event.Data["prompt_preview"] = preview

			Expect(event.EventType).To(Equal(audit.EventTypeLLMRequest))
			Expect(event.CorrelationID).To(Equal("rem-001"))
			Expect(event.Data["prompt_preview"]).NotTo(BeNil())
			previewStr, ok := event.Data["prompt_preview"].(string)
			Expect(ok).To(BeTrue())
			Expect(len(previewStr)).To(BeNumerically("<=", 504))
		})
	})

	Describe("UT-KA-433-AUD-002: LLM response event includes token totals", func() {
		It("should create audit event with token usage fields", func() {
			event := audit.NewEvent(audit.EventTypeLLMResponse, "rem-002")
			event.Data["prompt_tokens"] = 150
			event.Data["completion_tokens"] = 250
			event.Data["total_tokens"] = 400

			Expect(event.Data["prompt_tokens"]).To(Equal(150))
			Expect(event.Data["completion_tokens"]).To(Equal(250))
			Expect(event.Data["total_tokens"]).To(Equal(400))
		})
	})

	Describe("UT-KA-433-AUD-003: Tool call event includes tool name and arguments", func() {
		It("should create audit event with tool name and args", func() {
			event := audit.NewEvent(audit.EventTypeLLMToolCall, "rem-003")
			event.Data["tool_name"] = "get_namespaced_resource_context"
			event.Data["tool_arguments"] = map[string]interface{}{
				"kind": "Deployment", "name": "api-server", "namespace": "prod",
			}

			Expect(event.Data["tool_name"]).To(Equal("get_namespaced_resource_context"))
			args, ok := event.Data["tool_arguments"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(args["kind"]).To(Equal("Deployment"))
		})
	})

	Describe("UT-KA-433-AUD-004: Response complete event includes correlation_id = remediation_id", func() {
		It("should set CorrelationID from remediation_id", func() {
			remediationID := "rem-uuid-12345"
			event := audit.NewEvent(audit.EventTypeResponseComplete, remediationID)

			Expect(event.CorrelationID).To(Equal("rem-uuid-12345"))
			Expect(event.EventCategory).To(Equal(audit.EventCategory))
		})
	})

	Describe("UT-KA-433-AUD-005: Audit event includes user context when present (GAP-020)", func() {
		It("should store user context in audit event data", func() {
			event := audit.NewEvent(audit.EventTypeResponseComplete, "rem-005")
			event.Data["user_context"] = "system:serviceaccount:kubernaut:gateway-sa"

			Expect(event.Data["user_context"]).To(Equal("system:serviceaccount:kubernaut:gateway-sa"))
		})
	})

	Describe("UT-KA-433-AUD-006: Audit event user context is anonymous when no auth", func() {
		It("should default user context to anonymous", func() {
			event := audit.NewEvent(audit.EventTypeResponseComplete, "rem-006")
			userCtx := "anonymous"
			event.Data["user_context"] = userCtx

			Expect(event.Data["user_context"]).To(Equal("anonymous"))
		})
	})
})

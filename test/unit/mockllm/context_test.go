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
package mockllm_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	openai "github.com/jordigilh/kubernaut/pkg/shared/types/openai"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/conversation"
)

var _ = Describe("Conversation Context Extraction", func() {

	Describe("UT-MOCK-014-001: CountToolResults counts role=tool messages", func() {
		It("should correctly count tool result messages", func() {
			msgs := []openai.Message{
				{Role: "user", Content: stringPtr("analyze")},
				{Role: "assistant", Content: stringPtr("calling tool")},
				{Role: "tool", Content: stringPtr(`{"result": "data1"}`)},
				{Role: "assistant", Content: stringPtr("calling again")},
				{Role: "tool", Content: stringPtr(`{"result": "data2"}`)},
			}
			ctx := conversation.NewContext(msgs)
			Expect(ctx.CountToolResults()).To(Equal(2))
		})

		It("should return 0 when no tool messages exist", func() {
			msgs := []openai.Message{
				{Role: "user", Content: stringPtr("hello")},
				{Role: "assistant", Content: stringPtr("hi")},
			}
			ctx := conversation.NewContext(msgs)
			Expect(ctx.CountToolResults()).To(Equal(0))
		})
	})

	Describe("UT-MOCK-014-002: HasThreeStepTools identifies list_available_actions", func() {
		It("should return true when tools include list_available_actions", func() {
			tools := []openai.Tool{
				{Type: "function", Function: openai.ToolDefinition{Name: "list_available_actions"}},
				{Type: "function", Function: openai.ToolDefinition{Name: "list_workflows"}},
				{Type: "function", Function: openai.ToolDefinition{Name: "get_workflow"}},
			}
			Expect(conversation.HasThreeStepTools(tools)).To(BeTrue())
		})

		It("should return false when tools only include search_workflow_catalog", func() {
			tools := []openai.Tool{
				{Type: "function", Function: openai.ToolDefinition{Name: "search_workflow_catalog"}},
			}
			Expect(conversation.HasThreeStepTools(tools)).To(BeFalse())
		})
	})

	Describe("UT-MOCK-014-003: HasPhase3Markers requires ALL three markers present", func() {
		It("should return true when all three Phase 3 markers are present", func() {
			content := "Some preamble\n## Enrichment Context (Phase 2\ndata here\n## Phase 1 Root Cause Analysis\nRCA here\n**Root Owner**: Deployment/my-app"
			ctx := conversation.NewContext([]openai.Message{
				{Role: "user", Content: stringPtr(content)},
			})
			Expect(ctx.HasPhase3Markers()).To(BeTrue())
		})

		It("should return false when only two of three markers are present", func() {
			content := "## Enrichment Context (Phase 2\ndata\n## Phase 1 Root Cause Analysis\nno root owner here"
			ctx := conversation.NewContext([]openai.Message{
				{Role: "user", Content: stringPtr(content)},
			})
			Expect(ctx.HasPhase3Markers()).To(BeFalse())
		})

		It("should return false when no markers are present", func() {
			ctx := conversation.NewContext([]openai.Message{
				{Role: "user", Content: stringPtr("just a normal message")},
			})
			Expect(ctx.HasPhase3Markers()).To(BeFalse())
		})
	})

	Describe("UT-MOCK-014-004: ExtractResource pulls resource name and namespace", func() {
		It("should extract resource details from Signal Name structured content", func() {
			content := "Investigate alert.\n- Signal Name: OOMKilled\n- Namespace: production\n- Pod: my-pod-abc123"
			ctx := conversation.NewContext([]openai.Message{
				{Role: "user", Content: stringPtr(content)},
			})
			res := ctx.ExtractResource()
			Expect(res.Namespace).To(Equal("production"))
			Expect(res.Name).To(Equal("my-pod-abc123"))
			Expect(res.SignalName).To(Equal("OOMKilled"))
		})

		It("should extract resource from KA prompt template format (- Resource: ns/kind/name)", func() {
			content := "# Incident Analysis\n- Signal Name: BackOff\n- Resource: fp-e2e-496-123/Deployment/memory-eater\n- Error: OOMKilled"
			ctx := conversation.NewContext([]openai.Message{
				{Role: "system", Content: stringPtr(content)},
			})
			res := ctx.ExtractResource()
			Expect(res.Kind).To(Equal("Deployment"))
			Expect(res.Name).To(Equal("memory-eater"))
			Expect(res.Namespace).To(Equal("fp-e2e-496-123"))
			Expect(res.SignalName).To(Equal("BackOff"))
		})

		It("should prefer Owner Chain root owner over Resource line", func() {
			content := "- Signal Name: BackOff\n- Resource: fp-e2e-496-123/Pod/memory-eater-5b9d684998-kg7xw\n**Owner Chain**: ReplicaSet/memory-eater-5b9d684998(fp-e2e-496-123) → Deployment/memory-eater(fp-e2e-496-123)"
			ctx := conversation.NewContext([]openai.Message{
				{Role: "user", Content: stringPtr(content)},
			})
			res := ctx.ExtractResource()
			Expect(res.Kind).To(Equal("Deployment"))
			Expect(res.Name).To(Equal("memory-eater"))
			Expect(res.Namespace).To(Equal("fp-e2e-496-123"))
			Expect(res.SignalName).To(Equal("BackOff"))
		})

		It("should extract root owner from single-entry owner chain", func() {
			content := "- Resource: default/Pod/nginx-abc123\n**Owner Chain**: ReplicaSet/nginx-abc(default)"
			ctx := conversation.NewContext([]openai.Message{
				{Role: "user", Content: stringPtr(content)},
			})
			res := ctx.ExtractResource()
			Expect(res.Kind).To(Equal("ReplicaSet"))
			Expect(res.Name).To(Equal("nginx-abc"))
			Expect(res.Namespace).To(Equal("default"))
		})

		It("should extract root owner without namespace annotation", func() {
			content := "- Resource: default/Pod/my-pod\n**Owner Chain**: ReplicaSet/my-pod-abc → Deployment/my-pod"
			ctx := conversation.NewContext([]openai.Message{
				{Role: "user", Content: stringPtr(content)},
			})
			res := ctx.ExtractResource()
			Expect(res.Kind).To(Equal("Deployment"))
			Expect(res.Name).To(Equal("my-pod"))
			Expect(res.Namespace).To(Equal("default"))
		})

		It("should fall back to Resource line when no owner chain present", func() {
			content := "- Signal Name: OOMKilled\n- Resource: staging/StatefulSet/my-db\n- Pod: some-pod\n- Namespace: other-ns"
			ctx := conversation.NewContext([]openai.Message{
				{Role: "user", Content: stringPtr(content)},
			})
			res := ctx.ExtractResource()
			Expect(res.Kind).To(Equal("StatefulSet"))
			Expect(res.Name).To(Equal("my-db"))
			Expect(res.Namespace).To(Equal("staging"))
		})
	})

	Describe("UT-MOCK-014-005: ExtractRootOwner handles LLM-prefixed JSON", func() {
		It("should extract root_owner from tool result with prefix text before JSON", func() {
			toolContent := `HolmesGPT analysis complete. {"root_owner": {"kind": "Deployment", "name": "my-app", "namespace": "production"}, "labels": {}}`
			ctx := conversation.NewContext([]openai.Message{
				{Role: "tool", Content: stringPtr(toolContent)},
			})
			owner := ctx.ExtractRootOwner()
			Expect(owner).NotTo(BeNil())
			Expect(owner.Kind).To(Equal("Deployment"))
			Expect(owner.Name).To(Equal("my-app"))
			Expect(owner.Namespace).To(Equal("production"))
		})

		It("should return nil when no JSON with root_owner is found", func() {
			ctx := conversation.NewContext([]openai.Message{
				{Role: "tool", Content: stringPtr("no json here")},
			})
			owner := ctx.ExtractRootOwner()
			Expect(owner).To(BeNil())
		})
	})
})

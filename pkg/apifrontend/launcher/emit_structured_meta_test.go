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

package launcher_test

import (
	"context"
	"strings"

	"github.com/a2aproject/a2a-go/a2a"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
)

var _ = Describe("EmitStructuredMeta — TP-1395-1396 (#1395)", func() {

	Describe("UT-AF-1395-001: Structured JSON payload passes through without truncation", func() {
		It("should emit full 600-char JSON without ... suffix", func() {
			queue := &fakeQueue{}
			taskID := a2a.TaskID("task-structured-001")
			ctx := launcher.WithEventBridge(context.Background(), queue, taskID, "ctx-001", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			payload := `{"session_id":"sess-1","summary":"OOMKill","rca":{"severity":"critical","confidence":0.92,"causal_chain":["Memory leak in data-processor","Container hit 512Mi limit","OOMKill signal sent by kernel"],"target":"Deployment/data-processor in production","tool_calls_count":19,"llm_turns":17},"options":[{"workflow_id":"wf-restart","name":"Restart Pod","description":"Rolling restart of affected deployment pods to recover from OOM state","risk":"low","recommended":true,"parameters":{"namespace":"production","deployment":"data-processor"}},{"workflow_id":"wf-scale","name":"Increase Memory","description":"Scale memory limit","risk":"medium"}]}`
			Expect(len(payload)).To(BeNumerically(">", 512), "test payload must exceed 512 chars")

			err := bridge.EmitStructuredMeta(ctx, payload, map[string]any{"type": launcher.MetaTypeDecision})
			Expect(err).NotTo(HaveOccurred())
			Expect(queue.events).To(HaveLen(1))

			evt, ok := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(ok).To(BeTrue())
			textPart, ok := evt.Status.Message.Parts[0].(a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(textPart.Text).NotTo(HaveSuffix("..."), "structured payload must NOT be truncated")
			Expect(len(textPart.Text)).To(BeNumerically(">", 512))
			Expect(evt.Metadata["type"]).To(Equal("decision"))
		})
	})

	Describe("UT-AF-1395-002: Oversized payload rejected entirely", func() {
		It("should not emit 9000-char payload; should emit fallback status instead", func() {
			queue := &fakeQueue{}
			m := &spyBridgeMetrics{}
			taskID := a2a.TaskID("task-oversize-001")
			ctx := launcher.WithEventBridge(context.Background(), queue, taskID, "ctx-002", m)
			bridge := launcher.EventBridgeFromContext(ctx)

			// Use realistic JSON that won't be redacted (has spaces/structure)
			oversized := `{"data":"` + strings.Repeat("some data value ", 600) + `"}`
			Expect(len(oversized)).To(BeNumerically(">", 8192))

			err := bridge.EmitStructuredMeta(ctx, oversized, map[string]any{"type": launcher.MetaTypeDecision})
			Expect(err).NotTo(HaveOccurred())

			Expect(queue.events).To(HaveLen(1), "should emit exactly one fallback event")
			evt, ok := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(ok).To(BeTrue())
			textPart, ok := evt.Status.Message.Parts[0].(a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(textPart.Text).To(ContainSubstring("too large"))
			Expect(textPart.Text).NotTo(ContainSubstring("some data value some data"))
		})
	})

	Describe("UT-AF-1395-003: Oversized rejection increments metric and emits fallback", func() {
		It("should increment bridge write failure metric on oversized payload", func() {
			queue := &fakeQueue{}
			m := &spyBridgeMetrics{}
			taskID := a2a.TaskID("task-metric-001")
			ctx := launcher.WithEventBridge(context.Background(), queue, taskID, "ctx-003", m)
			bridge := launcher.EventBridgeFromContext(ctx)

			oversized := `{"data":"` + strings.Repeat("some data value ", 600) + `"}`
			_ = bridge.EmitStructuredMeta(ctx, oversized, map[string]any{"type": launcher.MetaTypeDecision})

			Expect(m.failuresInc).To(Equal(1), "should increment bridge write failures")
		})
	})

	Describe("UT-AF-1395-004: JWT in structured JSON is redacted", func() {
		It("should redact bearer token in JSON field value", func() {
			queue := &fakeQueue{}
			taskID := a2a.TaskID("task-redact-001")
			ctx := launcher.WithEventBridge(context.Background(), queue, taskID, "ctx-004", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			payload := `{"description":"token=Bearer eyJhbGciOiJSUzI1NiJ9.payload.sig","name":"test"}`

			err := bridge.EmitStructuredMeta(ctx, payload, map[string]any{"type": launcher.MetaTypeDecision})
			Expect(err).NotTo(HaveOccurred())
			Expect(queue.events).To(HaveLen(1))

			evt := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			textPart := evt.Status.Message.Parts[0].(a2a.TextPart)
			Expect(textPart.Text).NotTo(ContainSubstring("eyJhbGci"), "JWT should be redacted")
		})
	})

	Describe("UT-AF-1395-005: Free-text EmitStatus still truncates at 512 runes", func() {
		It("should truncate free-text status events at 512 runes with ... suffix", func() {
			queue := &fakeQueue{}
			taskID := a2a.TaskID("task-freetext-001")
			ctx := launcher.WithEventBridge(context.Background(), queue, taskID, "ctx-005", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			// Use text with spaces so it won't be redacted as a "secret"
			longText := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 15)
			Expect(len([]rune(longText))).To(BeNumerically(">", 512))

			err := bridge.EmitStatus(ctx, longText)
			Expect(err).NotTo(HaveOccurred())
			Expect(queue.events).To(HaveLen(1))

			evt := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			textPart := evt.Status.Message.Parts[0].(a2a.TextPart)
			Expect(textPart.Text).To(HaveSuffix("..."), "free-text should be truncated with ...")
			Expect(len([]rune(textPart.Text))).To(BeNumerically("<=", 515)) // 512 + "..."
		})
	})
})

var _ = Describe("EmitStructuredMetaSafe — TP-1398 (#1398)", func() {

	It("UT-AF-1398-005: EmitStructuredMetaSafe returns nil when no bridge in context", func() {
		ctx := context.Background()
		err := launcher.EmitStructuredMetaSafe(ctx, `{"test":"payload"}`, map[string]any{"type": launcher.MetaTypeApprovalRequest})
		Expect(err).NotTo(HaveOccurred())
	})

	It("UT-AF-1398-006: EmitStructuredMetaSafe emits correctly when bridge present", func() {
		queue := &fakeQueue{}
		taskID := a2a.TaskID("task-safe-001")
		ctx := launcher.WithEventBridge(context.Background(), queue, taskID, "ctx-safe-001", nil)

		payload := `{"name":"rar-rr-test","confidence":0.72,"reason":"Test approval"}`
		err := launcher.EmitStructuredMetaSafe(ctx, payload, map[string]any{"type": launcher.MetaTypeApprovalRequest})
		Expect(err).NotTo(HaveOccurred())
		Expect(queue.events).To(HaveLen(1))

		evt, ok := queue.events[0].(*a2a.TaskStatusUpdateEvent)
		Expect(ok).To(BeTrue())
		Expect(evt.Metadata["type"]).To(Equal("approval_request"))
		textPart, ok := evt.Status.Message.Parts[0].(a2a.TextPart)
		Expect(ok).To(BeTrue())
		Expect(textPart.Text).To(ContainSubstring("rar-rr-test"))
	})

	It("UT-AF-1398-007: MetaType constants have correct values", func() {
		Expect(launcher.MetaTypeApprovalRequest).To(Equal("approval_request"))
		Expect(launcher.MetaTypeApprovalRequestResolved).To(Equal("approval_request_resolved"))
	})
})

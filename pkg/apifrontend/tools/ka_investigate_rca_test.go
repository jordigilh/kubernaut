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

package tools_test

import (
	"context"
	"encoding/json"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("bridgeEventsCollectSummary RCA Parsing — TP-1395-1396 (#1396)", func() {

	Describe("UT-AF-1396-020: bridgeEventsCollectSummary populates RCA from complete event Data", func() {
		It("should extract structured RCA from EventTypeComplete Data field", func() {
			events := make(chan ka.InvestigationEvent, 5)

			rcaPayload := map[string]interface{}{
				"severity":         "critical",
				"confidence":       0.92,
				"causal_chain":     []string{"Memory leak", "OOMKill"},
				"target":           "Deployment/worker in production",
				"rca_summary":      "OOMKill caused by memory leak",
				"total_llm_turns":  17,
				"total_tool_calls": 19,
			}
			rcaJSON, err := json.Marshal(rcaPayload)
			Expect(err).NotTo(HaveOccurred())

			events <- ka.InvestigationEvent{
				Type: ka.EventTypeTokenDelta,
				Data: json.RawMessage(`{"delta":"Investigating..."}`),
			}
			events <- ka.InvestigationEvent{
				Type: ka.EventTypeComplete,
				Data: rcaJSON,
			}

			ctx := context.Background()
			summary, rca := tools.BridgeEventsCollectSummary(ctx, events, 5*time.Second)

			Expect(summary).To(ContainSubstring("Investigating..."))
			Expect(rca).NotTo(BeNil(), "RCA should be populated from complete event Data")
			Expect(rca.Severity).To(Equal("critical"))
			Expect(rca.Confidence).To(BeNumerically("~", 0.92, 0.001))
			Expect(rca.CausalChain).To(ConsistOf("Memory leak", "OOMKill"))
			Expect(rca.Target).To(Equal("Deployment/worker in production"))
			Expect(rca.RCASummary).To(Equal("OOMKill caused by memory leak"))
			Expect(rca.TotalLLMTurns).To(Equal(17))
			Expect(rca.TotalToolCalls).To(Equal(19))
		})
	})

	Describe("UT-AF-1396-021: bridgeEventsCollectSummary with empty Data on complete event", func() {
		It("should fall back to text summary with nil RCA", func() {
			events := make(chan ka.InvestigationEvent, 5)

			events <- ka.InvestigationEvent{
				Type: ka.EventTypeTokenDelta,
				Data: json.RawMessage(`{"delta":"Found the issue."}`),
			}
			events <- ka.InvestigationEvent{
				Type: ka.EventTypeComplete,
			}

			ctx := context.Background()
			summary, rca := tools.BridgeEventsCollectSummary(ctx, events, 5*time.Second)

			Expect(summary).To(Equal("Found the issue."))
			Expect(rca).To(BeNil(), "RCA should be nil when complete event has no Data")
		})
	})
})

// =============================================================================
// Issue #1407: Progressive RCA Emission — early RCA status-update on completion
// =============================================================================

var _ = Describe("Progressive RCA Emission — #1407", func() {

	Describe("UT-AF-1407-001: SI-4 early RCA emitted as decision status-update on investigation complete", func() {
		It("should emit a TaskStatusUpdateEvent with metadata.type=decision containing structured RCA", func() {
			queue := &bridgeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-1407-001", "ctx-1407-001", nil)

			events := make(chan ka.InvestigationEvent, 5)
			rcaPayload := map[string]interface{}{
				"severity":         "critical",
				"confidence":       0.92,
				"causal_chain":     []string{"Memory leak", "OOMKill"},
				"target":           "Deployment/worker in production",
				"rca_summary":      "OOMKill caused by memory leak",
				"total_llm_turns":  17,
				"total_tool_calls": 19,
			}
			rcaJSON, err := json.Marshal(rcaPayload)
			Expect(err).NotTo(HaveOccurred())

			events <- ka.InvestigationEvent{
				Type: ka.EventTypeComplete,
				Data: rcaJSON,
			}

			_, rca := tools.BridgeEventsCollectSummary(ctx, events, 5*time.Second)
			Expect(rca).NotTo(BeNil())

			allEvents := queue.Events()
			var decisionEvents []*a2a.TaskStatusUpdateEvent
			for _, evt := range allEvents {
				statusEvt, ok := evt.(*a2a.TaskStatusUpdateEvent)
				if !ok {
					continue
				}
				metaType, _ := statusEvt.Metadata["type"].(string)
				if metaType == launcher.MetaTypeDecision {
					decisionEvents = append(decisionEvents, statusEvt)
				}
			}
			Expect(decisionEvents).To(HaveLen(1),
				"SI-4: exactly one early RCA decision status-update must be emitted")

			decisionEvt := decisionEvents[0]
			Expect(decisionEvt.Metadata["schema"]).To(Equal("early_rca"),
				"SI-4: schema must identify this as early RCA for audit trail differentiation")
			Expect(decisionEvt.Metadata["schema_version"]).To(Equal("1.0"))

			Expect(decisionEvt.Status.Message).NotTo(BeNil())
			Expect(decisionEvt.Status.Message.Parts).To(HaveLen(1))
			textPart, ok := decisionEvt.Status.Message.Parts[0].(a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(textPart.Text).To(ContainSubstring("critical"),
				"AU-3: RCA severity must be included for auditable decision trail")
			Expect(textPart.Text).To(ContainSubstring("OOMKill caused by memory leak"))
		})
	})

	Describe("UT-AF-1407-002: SI-4 no early RCA emitted when investigation has no structured result", func() {
		It("should NOT emit a decision event when EventTypeComplete has empty Data", func() {
			queue := &bridgeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-1407-002", "ctx-1407-002", nil)

			events := make(chan ka.InvestigationEvent, 5)
			events <- ka.InvestigationEvent{
				Type: ka.EventTypeTokenDelta,
				Data: json.RawMessage(`{"delta":"Investigating..."}`),
			}
			events <- ka.InvestigationEvent{
				Type: ka.EventTypeComplete,
			}

			_, rca := tools.BridgeEventsCollectSummary(ctx, events, 5*time.Second)
			Expect(rca).To(BeNil())

			allEvents := queue.Events()
			for _, evt := range allEvents {
				statusEvt, ok := evt.(*a2a.TaskStatusUpdateEvent)
				if !ok {
					continue
				}
				metaType, _ := statusEvt.Metadata["type"].(string)
				Expect(metaType).NotTo(Equal(launcher.MetaTypeDecision),
					"SI-4: no decision event must be emitted without structured RCA data")
			}
		})
	})

	Describe("UT-AF-1407-003: AU-3 early RCA includes confidence and causal chain for FedRAMP audit", func() {
		It("should serialize confidence and causal chain in the structured payload", func() {
			queue := &bridgeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-1407-003", "ctx-1407-003", nil)

			events := make(chan ka.InvestigationEvent, 5)
			rcaPayload := map[string]interface{}{
				"severity":    "warning",
				"confidence":  0.78,
				"causal_chain": []string{"High CPU from runaway goroutine", "Throttled response time"},
				"target":      "Pod/api-gateway in staging",
				"rca_summary": "Runaway goroutine causing CPU throttling",
			}
			rcaJSON, _ := json.Marshal(rcaPayload)
			events <- ka.InvestigationEvent{
				Type: ka.EventTypeComplete,
				Data: rcaJSON,
			}

			tools.BridgeEventsCollectSummary(ctx, events, 5*time.Second)

			allEvents := queue.Events()
			var decisionEvt *a2a.TaskStatusUpdateEvent
			for _, evt := range allEvents {
				statusEvt, ok := evt.(*a2a.TaskStatusUpdateEvent)
				if !ok {
					continue
				}
				metaType, _ := statusEvt.Metadata["type"].(string)
				if metaType == launcher.MetaTypeDecision {
					decisionEvt = statusEvt
					break
				}
			}
			Expect(decisionEvt).NotTo(BeNil(),
				"AU-3: early RCA must be emitted for audit traceability")

			textPart, ok := decisionEvt.Status.Message.Parts[0].(a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(textPart.Text).To(ContainSubstring("0.78"),
				"AU-3: confidence must appear in payload for auditable risk assessment")
			Expect(textPart.Text).To(ContainSubstring("warning"))
			Expect(textPart.Text).To(ContainSubstring("Runaway goroutine"))
		})
	})

	Describe("UT-AF-1407-004: SI-4 early RCA emitted without EventBridge is no-op", func() {
		It("should not panic when no bridge is in context", func() {
			events := make(chan ka.InvestigationEvent, 5)
			rcaPayload := map[string]interface{}{
				"severity":    "critical",
				"confidence":  0.95,
				"rca_summary": "Should not panic",
			}
			rcaJSON, _ := json.Marshal(rcaPayload)
			events <- ka.InvestigationEvent{
				Type: ka.EventTypeComplete,
				Data: rcaJSON,
			}

			ctx := context.Background()
			summary, rca := tools.BridgeEventsCollectSummary(ctx, events, 5*time.Second)
			Expect(rca).NotTo(BeNil())
			Expect(summary).To(ContainSubstring("Should not panic"))
		})
	})
})

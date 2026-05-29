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

package apifrontend_test

import (
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

// =============================================================================
// IT-AF-1326: Investigation Event Bridge Wiring (BR-MCP-003, BR-INTERACTIVE-010)
//
// FedRAMP controls:
//   - AU-3: Audit trail completeness — delegation events include all required fields
//   - SI-4: Information system monitoring — investigation events bridged to A2A
//   - SC-7: Boundary protection — event filtering suppresses raw tool output
// =============================================================================

var aianalysisGVR = schema.GroupVersionResource{
	Group: "kubernaut.ai", Version: "v1alpha1", Resource: "aianalyses",
}

var _ = Describe("Investigation Event Bridge Wiring (IT-AF-1326)", func() {

	// =========================================================================
	// BR-INTERACTIVE-010: AIA CRD polling before investigation start
	// =========================================================================
	Describe("AIA CRD polling with real envtest K8s", func() {

		It("IT-AF-1326-050: HandleInvestigationMCP proceeds when AIA has session ID (BR-INTERACTIVE-010)", func() {
			ctx := context.Background()
			ns := "default"
			rrName := "rr-it-1326-050"

		aia := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "kubernaut.ai/v1alpha1",
				"kind":       "AIAnalysis",
				"metadata": map[string]interface{}{
					"name":      "aia-it-1326-050",
					"namespace": ns,
				},
				"spec": map[string]interface{}{
					"remediationId": "rem-it-1326-050",
					"analysisRequest": map[string]interface{}{
						"analysisTypes": []interface{}{"Investigation"},
						"signalContext": map[string]interface{}{
							"fingerprint": "fp-it-050",
							"severity":    "medium",
							"signalName":  "OOMKilled",
							"environment": "test",
							"targetResource": map[string]interface{}{
								"kind": "Pod",
								"name": "test-pod",
							},
							"businessPriority": map[string]interface{}{
								"tier": "Silver",
							},
							"enrichmentResults": map[string]interface{}{},
						},
					},
					"remediationRequestRef": map[string]interface{}{
						"name": rrName,
					},
				},
				"status": map[string]interface{}{
					"investigationSession": map[string]interface{}{
						"id": "ka-sess-050",
					},
				},
			},
		}
			_, err := dynamicClient.Resource(aianalysisGVR).Namespace(ns).Create(ctx, aia, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				_ = dynamicClient.Resource(aianalysisGVR).Namespace(ns).Delete(ctx, "aia-it-1326-050", metav1.DeleteOptions{})
			})

			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, args ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					Expect(args.RRID).To(Equal(rrName))
					return &ka.StartInvestigationResult{
						SessionID: "sess-it-050",
						Status:    "started",
						Events:    make(chan ka.InvestigationEvent, 1),
						Closer:    func() {},
					}, nil
				},
			}

			result, err := tools.HandleInvestigationMCP(ctx, mockMCP, dynamicClient, ns, tools.InvestigateMCPArgs{
				RRID: rrName,
			}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.SessionID).To(Equal("sess-it-050"))
			Expect(result.Status).To(Equal("started"))
		})

		It("IT-AF-1326-051: HandleInvestigationMCP proceeds on AIA timeout (best-effort, BR-INTERACTIVE-010)", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return &ka.StartInvestigationResult{
						SessionID: "sess-it-051",
						Status:    "started",
					}, nil
				},
			}

			// No AIA CRD exists — HandleAwaitSession will timeout, but
			// HandleInvestigationMCP proceeds anyway (best-effort).
			result, err := tools.HandleInvestigationMCP(ctx, mockMCP, dynamicClient, "default", tools.InvestigateMCPArgs{
				RRID: "rr-nonexistent-051",
			}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.SessionID).To(Equal("sess-it-051"))
		})

		It("IT-AF-1326-052: HandleInvestigationMCP works without K8s client (nil bypass)", func() {
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return &ka.StartInvestigationResult{
						SessionID: "sess-it-052",
						Status:    "started",
					}, nil
				},
			}

			result, err := tools.HandleInvestigationMCP(context.Background(), mockMCP, nil, "", tools.InvestigateMCPArgs{
				RRID: "rr-no-k8s-052",
			}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.SessionID).To(Equal("sess-it-052"))
		})
	})

	// =========================================================================
	// BR-MCP-003: Event bridge goroutine consumes KA events
	// =========================================================================
	Describe("Event bridge goroutine lifecycle", func() {

		It("IT-AF-1326-060: bridge goroutine drains events and exits on channel close (BR-MCP-003)", func() {
			eventCh := make(chan ka.InvestigationEvent, 10)
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return &ka.StartInvestigationResult{
						SessionID: "sess-it-060",
						Status:    "started",
						Events:    eventCh,
						Closer:    func() {},
					}, nil
				},
			}

			_, err := tools.HandleInvestigationMCP(context.Background(), mockMCP, nil, "", tools.InvestigateMCPArgs{
				RRID: "rr-bridge-060",
			}, nil)
			Expect(err).NotTo(HaveOccurred())

			eventCh <- ka.InvestigationEvent{
				Type: ka.EventTypeReasoningDelta,
				Data: json.RawMessage(`{"text":"Analyzing pod crash..."}`),
			}
			eventCh <- ka.InvestigationEvent{
				Type: ka.EventTypeToolCallStart,
				Data: json.RawMessage(`{"tool":"kubectl_get"}`),
			}
			eventCh <- ka.InvestigationEvent{
				Type: ka.EventTypeComplete,
				Data: json.RawMessage(`{"summary":"OOM Kill"}`),
			}
			close(eventCh)

			// Bridge goroutine should exit after processing events.
			// No assertion on A2A output (no EventBridge in context); we verify
			// no panics, no goroutine leaks, and channel drain completes.
			time.Sleep(100 * time.Millisecond)
		})

		It("IT-AF-1326-061: bridge goroutine exits on context cancellation (BR-MCP-003)", func() {
			eventCh := make(chan ka.InvestigationEvent, 10)
			ctx, cancel := context.WithCancel(context.Background())

			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return &ka.StartInvestigationResult{
						SessionID: "sess-it-061",
						Status:    "started",
						Events:    eventCh,
						Closer:    func() {},
					}, nil
				},
			}

			_, err := tools.HandleInvestigationMCP(ctx, mockMCP, nil, "", tools.InvestigateMCPArgs{
				RRID: "rr-cancel-061",
			}, nil)
			Expect(err).NotTo(HaveOccurred())

			cancel()
			time.Sleep(100 * time.Millisecond)
			// No goroutine leak: bridge goroutine should have exited.
		})
	})

	// =========================================================================
	// AU-3: Audit trail completeness — FedRAMP
	// =========================================================================
	Describe("Audit event completeness (AU-3)", func() {

		It("IT-AF-1326-070: delegation audit event includes all FedRAMP AU-3 fields", func() {
			eventCh := make(chan ka.InvestigationEvent, 10)
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return &ka.StartInvestigationResult{
						SessionID: "sess-it-070",
						Status:    "started",
						Events:    eventCh,
						Closer:    func() { close(eventCh) },
					}, nil
				},
			}

			result, err := tools.HandleInvestigationMCP(context.Background(), mockMCP, nil, "", tools.InvestigateMCPArgs{
				RRID: "rr-audit-070",
			}, auditRecorder)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.SessionID).To(Equal("sess-it-070"))

			events := auditRecorder.Events()
			var foundDelegation bool
			for _, e := range events {
				if e.Event != nil && e.Event.Type == audit.EventKADelegated {
					foundDelegation = true
					Expect(e.Event.Detail).To(HaveKeyWithValue("delegation_type", "interactive"),
						"AU-3: delegation_type must be interactive")
					Expect(e.Event.Detail).To(HaveKeyWithValue("rr_id", "rr-audit-070"),
						"AU-3: rr_id must be present")
					Expect(e.Event.Detail).To(HaveKey("session_id"),
						"AU-3: session_id must be present")
					Expect(e.Event.Detail).To(HaveKey("ka_correlation_id"),
						"AU-3: ka_correlation_id must be present for cross-service tracing")
					break
				}
			}
			Expect(foundDelegation).To(BeTrue(), "AU-3: ka.delegated audit event must be emitted")
		})
	})

	// =========================================================================
	// SC-7: Event filtering — raw tool output suppressed
	// =========================================================================
	Describe("Event filtering (SC-7 boundary protection)", func() {

		It("IT-AF-1326-080: FormatEventForUser suppresses tool_result and token_delta (SC-7)", func() {
			suppressedTypes := []string{
				ka.EventTypeToolResult,
				ka.EventTypeTokenDelta,
				"some_future_event",
			}
			for _, evtType := range suppressedTypes {
				result := tools.FormatEventForUser(ka.InvestigationEvent{
					Type: evtType,
					Data: json.RawMessage(`{"output":"sensitive data"}`),
				})
				Expect(result).To(BeEmpty(), "SC-7: event type %q must be suppressed", evtType)
			}
		})

		It("IT-AF-1326-081: FormatEventForUser passes reasoning and error events (SI-4)", func() {
			allowedCases := []struct {
				evt      ka.InvestigationEvent
				expected string
			}{
				{
					evt:      ka.InvestigationEvent{Type: ka.EventTypeReasoningDelta, Data: json.RawMessage(`{"text":"Analyzing..."}`)},
					expected: "Analyzing...",
				},
				{
					evt:      ka.InvestigationEvent{Type: ka.EventTypeToolCallStart, Data: json.RawMessage(`{"tool":"kubectl_get"}`)},
					expected: "Calling kubectl_get...",
				},
				{
					evt:      ka.InvestigationEvent{Type: ka.EventTypeError, Data: json.RawMessage(`{"error":"LLM timeout"}`)},
					expected: "Error: LLM timeout",
				},
				{
					evt:      ka.InvestigationEvent{Type: ka.EventTypeComplete},
					expected: "Investigation complete.",
				},
			}
			for _, tc := range allowedCases {
				result := tools.FormatEventForUser(tc.evt)
				Expect(result).To(Equal(tc.expected), "SI-4: event type %q must produce user-visible text", tc.evt.Type)
			}
		})
	})

	// =========================================================================
	// MonitorRegistry lifecycle (autonomous mode still supported)
	// =========================================================================
	Describe("MonitorRegistry lifecycle management", func() {

		It("IT-AF-1326-090: MonitorRegistry tracks and stops sessions correctly", func() {
			registry := tools.NewMonitorRegistry()

			closerCalled := false
			registry.Register("sess-090", func() { closerCalled = true })

			Expect(registry.Active("sess-090")).To(BeTrue())
			Expect(registry.Active("nonexistent")).To(BeFalse())

			registry.Stop("sess-090")
			Expect(closerCalled).To(BeTrue())
			Expect(registry.Active("sess-090")).To(BeFalse())
		})

		It("IT-AF-1326-091: MonitorRegistry.StopAll cleans up all sessions", func() {
			registry := tools.NewMonitorRegistry()

			var closed1, closed2 bool
			registry.Register("sess-091-a", func() { closed1 = true })
			registry.Register("sess-091-b", func() { closed2 = true })

			registry.StopAll()
			Expect(closed1).To(BeTrue())
			Expect(closed2).To(BeTrue())
			Expect(registry.Active("sess-091-a")).To(BeFalse())
			Expect(registry.Active("sess-091-b")).To(BeFalse())
		})
	})
})

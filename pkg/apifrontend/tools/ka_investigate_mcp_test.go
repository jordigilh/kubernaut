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
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("HandleInvestigationMCP — #1326 BR-MCP-002 non-blocking MCP investigate", func() {

	Describe("UT-AF-1326-020: starts autonomous MCP investigation and returns immediately", func() {
		It("should return session_id and autonomous_started status without blocking", func() {
			eventCh := make(chan ka.InvestigationEvent, 10)
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, args ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					Expect(args.RRID).To(Equal("rr-mcp-001"))
					return &ka.StartInvestigationResult{
						SessionID: "sess-mcp-001",
						Status:    "autonomous_started",
						Events:    eventCh,
						Closer:    func() { close(eventCh) },
					}, nil
				},
			}

			result, err := tools.HandleInvestigationMCP(context.Background(), mockMCP, nil, "", tools.InvestigateMCPArgs{
				RRID: "rr-mcp-001",
			}, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.SessionID).To(Equal("sess-mcp-001"))
			Expect(result.Status).To(Equal("autonomous_started"))
		})
	})

	Describe("UT-AF-1326-021: emits ka.delegated audit event on successful start", func() {
		It("should emit delegation audit event with session_id and rr_id", func() {
			eventCh := make(chan ka.InvestigationEvent, 10)
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return &ka.StartInvestigationResult{
						SessionID: "sess-audit-001",
						Status:    "autonomous_started",
						Events:    eventCh,
						Closer:    func() { close(eventCh) },
					}, nil
				},
			}

			recorder := &auditRecorder{}
			_, err := tools.HandleInvestigationMCP(context.Background(), mockMCP, nil, "", tools.InvestigateMCPArgs{
				RRID: "rr-audit-001",
			}, recorder)
			Expect(err).NotTo(HaveOccurred())

			Expect(recorder.events).To(HaveLen(1))
			Expect(recorder.events[0].Type).To(Equal(audit.EventKADelegated))
			Expect(recorder.events[0].Detail["delegation_type"]).To(Equal("interactive"))
			Expect(recorder.events[0].Detail["session_id"]).To(Equal("sess-audit-001"))
		})
	})

	Describe("UT-AF-1326-022: propagates MCP connection errors", func() {
		It("should return error when MCPClient.StartInvestigation fails", func() {
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return nil, ka.ErrMCPUnavailable
				},
			}

			_, err := tools.HandleInvestigationMCP(context.Background(), mockMCP, nil, "", tools.InvestigateMCPArgs{
				RRID: "rr-fail-001",
			}, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("MCP"))
		})
	})

	Describe("UT-AF-1326-023: requires rr_id", func() {
		It("should return error when RRID is empty", func() {
			mockMCP := &ka.MockMCPClient{}

			_, err := tools.HandleInvestigationMCP(context.Background(), mockMCP, nil, "", tools.InvestigateMCPArgs{
				RRID: "",
			}, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("rr_id"))
		})
	})

	Describe("UT-AF-1326-024: MonitorRegistry tracks active sessions", func() {
		It("should register the autonomous session in the monitor registry", func() {
			eventCh := make(chan ka.InvestigationEvent, 10)
			var closerCalled atomic.Int32
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return &ka.StartInvestigationResult{
						SessionID: "sess-monitor-001",
						Status:    "autonomous_started",
						Events:    eventCh,
						Closer:    func() { closerCalled.Add(1) },
					}, nil
				},
			}

			registry := tools.NewMonitorRegistry()
			result, err := tools.HandleInvestigationMCPWithRegistry(context.Background(), mockMCP, nil, "", tools.InvestigateMCPArgs{
				RRID: "rr-monitor-001",
			}, nil, registry, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.SessionID).To(Equal("sess-monitor-001"))

			Expect(registry.Active("sess-monitor-001")).To(BeTrue())
		})
	})

	Describe("UT-AF-1326-025: MonitorRegistry cancels session on Stop", func() {
		It("should call closer and remove from registry when stopped", func() {
			eventCh := make(chan ka.InvestigationEvent, 10)
			var closerCalled atomic.Int32
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return &ka.StartInvestigationResult{
						SessionID: "sess-stop-001",
						Status:    "autonomous_started",
						Events:    eventCh,
						Closer:    func() { closerCalled.Add(1) },
					}, nil
				},
			}

			registry := tools.NewMonitorRegistry()
			_, err := tools.HandleInvestigationMCPWithRegistry(context.Background(), mockMCP, nil, "", tools.InvestigateMCPArgs{
				RRID: "rr-stop-001",
			}, nil, registry, nil)
			Expect(err).NotTo(HaveOccurred())

			registry.Stop("sess-stop-001")

			Eventually(func() int32 {
				return closerCalled.Load()
			}, 2*time.Second).Should(BeNumerically(">=", 1))

			Expect(registry.Active("sess-stop-001")).To(BeFalse())
		})
	})

	Describe("UT-AF-1326-100: audit trail completeness — delegation event has all AU-3 fields", func() {
		It("should include session_id, ka_correlation_id, delegation_type, rr_id in audit detail", func() {
			eventCh := make(chan ka.InvestigationEvent, 10)
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return &ka.StartInvestigationResult{
						SessionID: "sess-au3-001",
						Status:    "autonomous_started",
						Events:    eventCh,
						Closer:    func() { close(eventCh) },
					}, nil
				},
			}

			recorder := &auditRecorder{}
			_, err := tools.HandleInvestigationMCP(context.Background(), mockMCP, nil, "", tools.InvestigateMCPArgs{
				RRID: "rr-au3-001",
			}, recorder)
			Expect(err).NotTo(HaveOccurred())

			Expect(recorder.events).To(HaveLen(1))
			evt := recorder.events[0]
			Expect(evt.Detail).To(HaveKey("session_id"))
			Expect(evt.Detail).To(HaveKey("ka_correlation_id"))
			Expect(evt.Detail).To(HaveKey("delegation_type"))
			Expect(evt.Detail).To(HaveKey("rr_id"))
		})
	})
})

var _ = Describe("formatEventForUser — #1326 BR-MCP-008 event filtering", func() {

	Describe("UT-AF-1326-040: reasoning_delta events produce text", func() {
		It("should extract the text field from reasoning_delta events", func() {
			evt := ka.InvestigationEvent{
				Type: ka.EventTypeReasoningDelta,
				Data: json.RawMessage(`{"text":"Analyzing pod crash..."}`),
			}
			result := tools.FormatEventForUser(evt)
			Expect(result).To(Equal("Analyzing pod crash..."))
		})
	})

	Describe("UT-AF-1326-041: tool_call_start events produce descriptive text", func() {
		It("should format tool name with 'Calling ...' prefix", func() {
			evt := ka.InvestigationEvent{
				Type: ka.EventTypeToolCallStart,
				Data: json.RawMessage(`{"tool":"kubectl_get"}`),
			}
			result := tools.FormatEventForUser(evt)
			Expect(result).To(Equal("Calling kubectl_get..."))
		})
	})

	Describe("UT-AF-1326-042: error events produce error text", func() {
		It("should format error message", func() {
			evt := ka.InvestigationEvent{
				Type: ka.EventTypeError,
				Data: json.RawMessage(`{"error":"LLM provider unavailable"}`),
			}
			result := tools.FormatEventForUser(evt)
			Expect(result).To(Equal("Error: LLM provider unavailable"))
		})
	})

	Describe("UT-AF-1326-043: complete events produce terminal text", func() {
		It("should return 'Investigation complete.'", func() {
			evt := ka.InvestigationEvent{
				Type: ka.EventTypeComplete,
			}
			result := tools.FormatEventForUser(evt)
			Expect(result).To(Equal("Investigation complete."))
		})
	})

	Describe("UT-AF-1326-044: tool_result events are suppressed", func() {
		It("should return empty string for tool_result events", func() {
			evt := ka.InvestigationEvent{
				Type: ka.EventTypeToolResult,
				Data: json.RawMessage(`{"output":"lots of data"}`),
			}
			result := tools.FormatEventForUser(evt)
			Expect(result).To(BeEmpty())
		})
	})

	Describe("UT-AF-1326-045: token_delta events are suppressed", func() {
		It("should return empty string for token_delta events", func() {
			evt := ka.InvestigationEvent{
				Type: ka.EventTypeTokenDelta,
				Data: json.RawMessage(`{"token":"a"}`),
			}
			result := tools.FormatEventForUser(evt)
			Expect(result).To(BeEmpty())
		})
	})

	Describe("UT-AF-1326-046: unknown event types are suppressed", func() {
		It("should return empty string for unknown event types", func() {
			evt := ka.InvestigationEvent{
				Type: "some_future_event",
				Data: json.RawMessage(`{"foo":"bar"}`),
			}
			result := tools.FormatEventForUser(evt)
			Expect(result).To(BeEmpty())
		})
	})

	Describe("UT-AF-1326-047: error event with missing error field uses fallback", func() {
		It("should return generic error message when error field is absent", func() {
			evt := ka.InvestigationEvent{
				Type: ka.EventTypeError,
				Data: json.RawMessage(`{}`),
			}
			result := tools.FormatEventForUser(evt)
			Expect(result).To(Equal("Investigation error occurred"))
		})
	})

	Describe("UT-AF-1326-048: reasoning_delta with empty data returns empty", func() {
		It("should return empty string when data is nil", func() {
			evt := ka.InvestigationEvent{
				Type: ka.EventTypeReasoningDelta,
				Data: nil,
			}
			result := tools.FormatEventForUser(evt)
			Expect(result).To(BeEmpty())
		})
	})
})

var _ = Describe("bridgeEventsToA2A — #1326 BR-MCP-003 event bridge goroutine", func() {

	Describe("UT-AF-1326-050: bridge drains event channel on close", func() {
		It("should exit cleanly when the event channel is closed", func() {
			eventCh := make(chan ka.InvestigationEvent, 5)
			eventCh <- ka.InvestigationEvent{Type: ka.EventTypeReasoningDelta, Data: json.RawMessage(`{"text":"step 1"}`)}
			close(eventCh)

			done := make(chan struct{})
			go func() {
				tools.BridgeEventsToA2A(context.Background(), eventCh)
				close(done)
			}()

			Eventually(done, 2*time.Second).Should(BeClosed())
		})
	})

	Describe("UT-AF-1326-051: bridge exits on context cancellation", func() {
		It("should exit when context is cancelled", func() {
			eventCh := make(chan ka.InvestigationEvent, 5)

			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() {
				tools.BridgeEventsToA2A(ctx, eventCh)
				close(done)
			}()

			cancel()
			Eventually(done, 2*time.Second).Should(BeClosed())
		})
	})
})

var _ = Describe("HandleInvestigationMCP — delegation_type audit event", func() {

	Describe("UT-AF-1326-060: audit event uses interactive delegation type", func() {
		It("should emit interactive in the delegation_type field", func() {
			eventCh := make(chan ka.InvestigationEvent, 10)
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return &ka.StartInvestigationResult{
						SessionID: "sess-delegate-060",
						Status:    "started",
						Events:    eventCh,
						Closer:    func() { close(eventCh) },
					}, nil
				},
			}

			recorder := &auditRecorder{}
			_, err := tools.HandleInvestigationMCP(context.Background(), mockMCP, nil, "", tools.InvestigateMCPArgs{
				RRID: "rr-delegate-060",
			}, recorder)
			Expect(err).NotTo(HaveOccurred())
			Expect(recorder.events).To(HaveLen(1))
			Expect(recorder.events[0].Detail["delegation_type"]).To(Equal("interactive"))
		})
	})
})

// auditRecorder captures audit events for test assertions.
type auditRecorder struct {
	events []*audit.Event
}

func (r *auditRecorder) Emit(_ context.Context, e *audit.Event) {
	r.events = append(r.events, e)
}

// Ensure auditRecorder satisfies audit.Emitter at compile time (if exported).
var _ audit.Emitter = (*auditRecorder)(nil)

var _ = Describe("HandleInvestigationMCPWithRegistry — AIA polling timeout cap (#E2E-FIX)", func() {

	Describe("UT-AF-1326-070: investigate path uses ≤10s AIA poll, not 3-min global timeout", func() {
		It("should complete well under 30s even when no AIA CRD exists", func() {
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, args ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return &ka.StartInvestigationResult{
						SessionID: "sess-fast-001",
						Status:    "autonomous_started",
						Closer:    func() {},
					}, nil
				},
			}

			client := newSeededAIAnalysisClient()
			registry := tools.NewMonitorRegistry()

			start := time.Now()
			result, err := tools.HandleInvestigationMCPWithRegistry(
				context.Background(), mockMCP, client, "kubernaut-system",
				tools.InvestigateMCPArgs{RRID: "rr-timeout-001"},
				nil, registry, nil,
			)
			elapsed := time.Since(start)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.SessionID).To(Equal("sess-fast-001"))
			Expect(elapsed).To(BeNumerically("<", 15*time.Second),
				"investigate path must not block for 3 minutes when no AIA CRD exists")
		})
	})

	Describe("UT-AF-1326-071: investigate with nil k8sClient skips AIA poll entirely", func() {
		It("should proceed immediately without any AIA polling", func() {
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return &ka.StartInvestigationResult{
						SessionID: "sess-nok8s-001",
						Status:    "autonomous_started",
						Closer:    func() {},
					}, nil
				},
			}

			start := time.Now()
			result, err := tools.HandleInvestigationMCPWithRegistry(
				context.Background(), mockMCP, nil, "",
				tools.InvestigateMCPArgs{RRID: "rr-nok8s-001"},
				nil, nil, nil,
			)
			elapsed := time.Since(start)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.SessionID).To(Equal("sess-nok8s-001"))
			Expect(elapsed).To(BeNumerically("<", 2*time.Second),
				"nil k8sClient must skip AIA polling entirely")
		})
	})

	Describe("UT-AF-1326-072: investigate with empty namespace skips AIA poll", func() {
		It("should proceed immediately when namespace is empty", func() {
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return &ka.StartInvestigationResult{
						SessionID: "sess-nons-001",
						Status:    "autonomous_started",
						Closer:    func() {},
					}, nil
				},
			}

			client := newDynamicFakeClient()
			start := time.Now()
			result, err := tools.HandleInvestigationMCPWithRegistry(
				context.Background(), mockMCP, client, "",
				tools.InvestigateMCPArgs{RRID: "rr-nons-001"},
				nil, nil, nil,
			)
			elapsed := time.Since(start)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.SessionID).To(Equal("sess-nons-001"))
			Expect(elapsed).To(BeNumerically("<", 2*time.Second),
				"empty namespace must skip AIA polling entirely")
		})
	})

	Describe("UT-AF-1326-073: investigate with existing AIA CRD finds session immediately", func() {
		It("should detect the AIA CRD and proceed without timeout", func() {
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return &ka.StartInvestigationResult{
						SessionID: "sess-aia-found-001",
						Status:    "autonomous_started",
						Closer:    func() {},
					}, nil
				},
			}

			aiaObj := newUnstructuredAIAnalysis("kubernaut-system", "aia-rr-aia-001", "rr-aia-001", "ka-sess-external")
			client := newSeededAIAnalysisClient(aiaObj)
			registry := tools.NewMonitorRegistry()

			start := time.Now()
			result, err := tools.HandleInvestigationMCPWithRegistry(
				context.Background(), mockMCP, client, "kubernaut-system",
				tools.InvestigateMCPArgs{RRID: "rr-aia-001"},
				nil, registry, nil,
			)
			elapsed := time.Since(start)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.SessionID).To(Equal("sess-aia-found-001"))
			Expect(elapsed).To(BeNumerically("<", 2*time.Second),
				"should find existing AIA immediately, no polling delay")
		})
	})

	Describe("UT-AF-1326-074: parent context cancellation overrides 10s poll timeout", func() {
		It("should honour parent context cancellation during AIA poll", func() {
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return &ka.StartInvestigationResult{
						SessionID: "sess-cancel-001",
						Status:    "autonomous_started",
						Closer:    func() {},
					}, nil
				},
			}

			client := newSeededAIAnalysisClient()
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()

			start := time.Now()
			result, err := tools.HandleInvestigationMCPWithRegistry(
				ctx, mockMCP, client, "kubernaut-system",
				tools.InvestigateMCPArgs{RRID: "rr-cancel-001"},
				nil, nil, nil,
			)
			elapsed := time.Since(start)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.SessionID).To(Equal("sess-cancel-001"))
			Expect(elapsed).To(BeNumerically("<", 5*time.Second),
				"parent context cancellation must abort AIA poll")
		})
	})
})

// Suppress unused import warning for json and time
var _ = json.Marshal
var _ time.Duration

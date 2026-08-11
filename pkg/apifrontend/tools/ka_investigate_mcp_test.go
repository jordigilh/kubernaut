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

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
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
			}, nil, registry, nil, false, nil, "", nil, nil)
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
			}, nil, registry, nil, false, nil, "", nil, nil)
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

var _ = Describe("DD-AF-011 (#1899): InteractionMode field on InvestigateMCPArgs", func() {

	Describe("IT-AF-1899-001: interaction_mode is a JSON-visible argument on kubernaut_investigate", func() {
		It("should serialize a declared mode under the interaction_mode key", func() {
			args := tools.InvestigateMCPArgs{
				RRID:            "rr-1899-001",
				InteractionMode: "full_remediation_autonomous",
			}
			b, err := json.Marshal(args)
			Expect(err).NotTo(HaveOccurred())

			var decoded map[string]any
			Expect(json.Unmarshal(b, &decoded)).To(Succeed())
			Expect(decoded["interaction_mode"]).To(Equal("full_remediation_autonomous"),
				"the LLM-visible JSON key must be interaction_mode so the ADK tool schema exposes it")
		})

		It("should omit interaction_mode from JSON when left empty (fail-safe default is implicit)", func() {
			args := tools.InvestigateMCPArgs{RRID: "rr-1899-002"}
			b, err := json.Marshal(args)
			Expect(err).NotTo(HaveOccurred())

			var decoded map[string]any
			Expect(json.Unmarshal(b, &decoded)).To(Succeed())
			Expect(decoded).NotTo(HaveKey("interaction_mode"),
				"an omitted mode must not force the LLM to always specify one")
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
				Data: json.RawMessage(`{"tool_name":"kubectl_get"}`),
			}
			result := tools.FormatEventForUser(evt)
			Expect(result).To(Equal("Calling kubectl_get..."))
		})
	})

	Describe("UT-AF-2086-007: tool_call_start key mismatch fix — reads tool_name, not tool", func() {
		It("should extract the tool name from the real KA wire key \"tool_name\"", func() {
			evt := ka.InvestigationEvent{
				Type: ka.EventTypeToolCallStart,
				Data: json.RawMessage(`{"tool_name":"kubectl_get"}`),
			}
			result := tools.FormatEventForUser(evt)
			Expect(result).To(Equal("Calling kubectl_get..."),
				"#2086: KA's real emission (investigator.go) uses the \"tool_name\" key -- "+
					"FormatEventForUser must read it for tool-call status text to ever render")
		})

		It("should return empty string for the old (wrong) \"tool\" key, proving the mismatch is fixed and not just relocated", func() {
			evt := ka.InvestigationEvent{
				Type: ka.EventTypeToolCallStart,
				Data: json.RawMessage(`{"tool":"kubectl_get"}`),
			}
			result := tools.FormatEventForUser(evt)
			Expect(result).To(BeEmpty(),
				"#2086: the old \"tool\" key never appears on KA's real wire format; a fixed "+
					"FormatEventForUser must not silently keep accepting it")
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

	Describe("UT-AF-1351-021: FormatEventForUser applies RedactError on error events (AF-HIGH-4)", func() {
		It("should redact internal error details before returning to user", func() {
			evt := ka.InvestigationEvent{
				Type: ka.EventTypeError,
				Data: json.RawMessage(`{"error": "connection refused: dial tcp 10.0.0.1:8080: connect: connection refused"}`),
			}
			text := tools.FormatEventForUser(evt)
			Expect(text).NotTo(ContainSubstring("10.0.0.1"),
				"Internal IP addresses must be redacted from user-facing error events (AF-HIGH-4)")
			Expect(text).To(HavePrefix("Error: "),
				"Error events should still have the Error: prefix")
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
				tools.BridgeEventsToA2A(context.Background(), eventCh, tools.BridgeInactivityTimeout)
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
				tools.BridgeEventsToA2A(ctx, eventCh, tools.BridgeInactivityTimeout)
				close(done)
			}()

			cancel()
			Eventually(done, 2*time.Second).Should(BeClosed())
		})
	})
})

var _ = Describe("A2A status channel routing — event type aware emission", func() {

	Describe("UT-AF-STATUS-010: tool call start events route to status channel", func() {
		It("should emit TaskStatusUpdateEvent with metadata.type=status for EventTypeToolCallStart", func() {
			queue := &bridgeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-route-010", "", nil)

			eventCh := make(chan ka.InvestigationEvent, 5)
			eventCh <- ka.InvestigationEvent{
				Type: ka.EventTypeToolCallStart,
				Data: json.RawMessage(`{"tool_name":"kubectl_get"}`),
			}
			eventCh <- ka.InvestigationEvent{Type: ka.EventTypeComplete}
			close(eventCh)

			tools.BridgeEventsToA2A(ctx, eventCh, tools.BridgeInactivityTimeout)

			events := queue.Events()
			Expect(len(events)).To(BeNumerically(">=", 1))

			var statusEvents []*a2a.TaskStatusUpdateEvent
			for _, evt := range events {
				if se, ok := evt.(*a2a.TaskStatusUpdateEvent); ok {
					statusEvents = append(statusEvents, se)
				}
			}
			Expect(statusEvents).NotTo(BeEmpty(),
				"tool call start must produce TaskStatusUpdateEvent")

			foundToolCall := false
			for _, se := range statusEvents {
				text := se.Status.Message.Parts[0].(a2a.TextPart).Text
				if text == "Calling kubectl_get..." {
					Expect(se.Metadata).To(HaveKeyWithValue("type", "status"))
					foundToolCall = true
					break
				}
			}
			Expect(foundToolCall).To(BeTrue(),
				"tool call start status event must contain formatted tool name")
		})
	})

	Describe("UT-AF-STATUS-011: reasoning deltas route to reasoning metadata", func() {
		It("should emit TaskStatusUpdateEvent with metadata.type=reasoning for EventTypeReasoningDelta", func() {
			queue := &bridgeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-route-011", "", nil)

			eventCh := make(chan ka.InvestigationEvent, 5)
			eventCh <- ka.InvestigationEvent{
				Type: ka.EventTypeReasoningDelta,
				Data: json.RawMessage(`{"text":"Pod is in CrashLoopBackOff"}`),
			}
			eventCh <- ka.InvestigationEvent{Type: ka.EventTypeComplete}
			close(eventCh)

			tools.BridgeEventsToA2A(ctx, eventCh, tools.BridgeInactivityTimeout)

			events := queue.Events()
			var reasoningEvents []*a2a.TaskStatusUpdateEvent
			for _, evt := range events {
				se, ok := evt.(*a2a.TaskStatusUpdateEvent)
				if !ok {
					continue
				}
				if metaType, ok := se.Metadata["type"].(string); ok && metaType == "reasoning" {
					reasoningEvents = append(reasoningEvents, se)
				}
			}
			Expect(reasoningEvents).NotTo(BeEmpty(),
				"reasoning deltas must produce TaskStatusUpdateEvent with metadata.type=reasoning")

			text := reasoningEvents[0].Status.Message.Parts[0].(a2a.TextPart).Text
			Expect(text).To(ContainSubstring("Pod is in CrashLoopBackOff"))
			Expect(reasoningEvents[0].Metadata).To(HaveKeyWithValue("type", "reasoning"))
		})
	})

	Describe("UT-AF-STATUS-012: token deltas route to reasoning metadata", func() {
		It("should emit TaskStatusUpdateEvent with metadata.type=reasoning for EventTypeTokenDelta", func() {
			queue := &bridgeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-route-012", "", nil)

			eventCh := make(chan ka.InvestigationEvent, 5)
			eventCh <- ka.InvestigationEvent{
				Type: ka.EventTypeTokenDelta,
				Data: json.RawMessage(`{"delta":"The root cause"}`),
			}
			eventCh <- ka.InvestigationEvent{Type: ka.EventTypeComplete}
			close(eventCh)

			tools.BridgeEventsToA2A(ctx, eventCh, tools.BridgeInactivityTimeout)

			events := queue.Events()
			var reasoningEvents []*a2a.TaskStatusUpdateEvent
			for _, evt := range events {
				se, ok := evt.(*a2a.TaskStatusUpdateEvent)
				if !ok {
					continue
				}
				if metaType, ok := se.Metadata["type"].(string); ok && metaType == "reasoning" {
					reasoningEvents = append(reasoningEvents, se)
				}
			}
			Expect(reasoningEvents).NotTo(BeEmpty(),
				"token deltas must produce TaskStatusUpdateEvent with metadata.type=reasoning")

			text := reasoningEvents[0].Status.Message.Parts[0].(a2a.TextPart).Text
			Expect(text).To(ContainSubstring("The root cause"))
			Expect(reasoningEvents[0].Metadata).To(HaveKeyWithValue("type", "reasoning"))
		})
	})

	Describe("UT-AF-STATUS-013: complete event routes to status channel", func() {
		It("should emit TaskStatusUpdateEvent with metadata.type=status for EventTypeComplete", func() {
			queue := &bridgeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-route-013", "", nil)

			eventCh := make(chan ka.InvestigationEvent, 5)
			eventCh <- ka.InvestigationEvent{Type: ka.EventTypeComplete}
			close(eventCh)

			tools.BridgeEventsToA2A(ctx, eventCh, tools.BridgeInactivityTimeout)

			events := queue.Events()
			var statusEvents []*a2a.TaskStatusUpdateEvent
			for _, evt := range events {
				if se, ok := evt.(*a2a.TaskStatusUpdateEvent); ok {
					statusEvents = append(statusEvents, se)
				}
			}
			Expect(statusEvents).NotTo(BeEmpty(),
				"complete event must produce TaskStatusUpdateEvent")

			found := false
			for _, se := range statusEvents {
				text := se.Status.Message.Parts[0].(a2a.TextPart).Text
				if text == "Investigation complete." {
					Expect(se.Metadata).To(HaveKeyWithValue("type", "status"))
					found = true
					break
				}
			}
			Expect(found).To(BeTrue())
		})
	})

	Describe("UT-AF-STATUS-014: mixed event stream separates metadata types correctly", func() {
		It("should route each event type to the correct metadata.type", func() {
			queue := &bridgeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-route-014", "", nil)

			eventCh := make(chan ka.InvestigationEvent, 10)
			eventCh <- ka.InvestigationEvent{
				Type: ka.EventTypeToolCallStart,
				Data: json.RawMessage(`{"tool_name":"kubectl_get"}`),
			}
			eventCh <- ka.InvestigationEvent{
				Type: ka.EventTypeReasoningDelta,
				Data: json.RawMessage(`{"text":"Analyzing pods..."}`),
			}
			eventCh <- ka.InvestigationEvent{
				Type: ka.EventTypeTokenDelta,
				Data: json.RawMessage(`{"delta":"Root cause found"}`),
			}
			eventCh <- ka.InvestigationEvent{
				Type: ka.EventTypeToolCallStart,
				Data: json.RawMessage(`{"tool_name":"kubectl_describe"}`),
			}
			eventCh <- ka.InvestigationEvent{Type: ka.EventTypeComplete}
			close(eventCh)

			tools.BridgeEventsToA2A(ctx, eventCh, tools.BridgeInactivityTimeout)

			events := queue.Events()
			var reasoningCount, statusCount int
			for _, evt := range events {
				se, ok := evt.(*a2a.TaskStatusUpdateEvent)
				if !ok {
					continue
				}
				metaType, ok := se.Metadata["type"].(string)
				if !ok {
					continue
				}
				switch metaType {
				case "reasoning":
					reasoningCount++
				case "status":
					statusCount++
				}
			}

			Expect(reasoningCount).To(Equal(2),
				"reasoning_delta + token_delta = 2 reasoning metadata events")
			Expect(statusCount).To(BeNumerically(">=", 2),
				"tool_call_start x2 + complete = at least 3 status metadata events (2 tool calls + complete)")
		})
	})

	Describe("UT-AF-STATUS-015: error events route to status channel (F1 AC-4)", func() {
		It("should emit error text on TaskStatusUpdateEvent with metadata.type=status, not reasoning", func() {
			queue := &bridgeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-route-015", "", nil)

			eventCh := make(chan ka.InvestigationEvent, 5)
			eventCh <- ka.InvestigationEvent{
				Type: ka.EventTypeError,
				Data: json.RawMessage(`{"error":"connection refused"}`),
			}
			eventCh <- ka.InvestigationEvent{Type: ka.EventTypeComplete}
			close(eventCh)

			tools.BridgeEventsToA2A(ctx, eventCh, tools.BridgeInactivityTimeout)

			events := queue.Events()
			for _, evt := range events {
				se, ok := evt.(*a2a.TaskStatusUpdateEvent)
				if !ok || se.Status.Message == nil {
					continue
				}
				metaType, _ := se.Metadata["type"].(string)
				if metaType != "reasoning" {
					continue
				}
				text := se.Status.Message.Parts[0].(a2a.TextPart).Text
				Expect(text).NotTo(ContainSubstring("Error:"),
					"error text must NOT appear on reasoning metadata stream (AC-4 information flow violation)")
			}

			foundErrorOnStatus := false
			for _, evt := range events {
				se, ok := evt.(*a2a.TaskStatusUpdateEvent)
				if !ok || se.Status.Message == nil {
					continue
				}
				text := se.Status.Message.Parts[0].(a2a.TextPart).Text
				if text != "" && text != "Investigation complete." {
					Expect(se.Metadata).To(HaveKeyWithValue("type", "status"))
					foundErrorOnStatus = true
					break
				}
			}
			Expect(foundErrorOnStatus).To(BeTrue(),
				"error text must appear on status metadata stream as ephemeral message")
		})
	})
})

var _ = Describe("AF-C1: Non-blocking bridge context detachment (#1356)", func() {

	Describe("UT-AF-1356-001: bridge goroutine survives after parent context is cancelled", func() {
		It("should process events even when the original ctx is cancelled", func() {
			eventCh := make(chan ka.InvestigationEvent, 5)
			closerCalled := make(chan struct{})

			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return &ka.StartInvestigationResult{
						SessionID: "sess-af-c1-001",
						Status:    "autonomous_started",
						Events:    eventCh,
						Closer: func() {
							close(closerCalled)
						},
					}, nil
				},
			}

			// Parent context that will be cancelled immediately after handler returns
			parentCtx, parentCancel := context.WithCancel(context.Background())

			registry := tools.NewMonitorRegistry()
			result, err := tools.HandleInvestigationMCPWithRegistry(
				parentCtx, mockMCP, nil, "",
				tools.InvestigateMCPArgs{RRID: "rr-af-c1-001"},
				nil, registry, nil, false, nil, "", nil, nil,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.SessionID).To(Equal("sess-af-c1-001"))

			// Cancel parent -- simulates wrapTool's `defer cancel()`
			parentCancel()

			// Send events AFTER parent context is cancelled
			eventCh <- ka.InvestigationEvent{Type: ka.EventTypeReasoningDelta, Data: json.RawMessage(`{"text":"step 1"}`)}
			eventCh <- ka.InvestigationEvent{Type: ka.EventTypeReasoningDelta, Data: json.RawMessage(`{"text":"step 2"}`)}
			eventCh <- ka.InvestigationEvent{Type: ka.EventTypeComplete}

			// Bridge goroutine should complete and call cleanup
			Eventually(closerCalled, 5*time.Second).Should(BeClosed())
		})
	})

	Describe("UT-AF-1356-002: bridge goroutine exits on inactivity timeout", func() {
		It("should exit when no events arrive within BridgeInactivityTimeout", func() {
			eventCh := make(chan ka.InvestigationEvent, 5)
			done := make(chan struct{})
			go func() {
				tools.BridgeEventsToA2A(context.Background(), eventCh, 200*time.Millisecond)
				close(done)
			}()

			// No events sent -- bridge should exit after 200ms inactivity
			Eventually(done, 2*time.Second).Should(BeClosed())
		})
	})

	Describe("UT-AF-1356-003: NonBlockingBridgeTTL caps bridge lifetime", func() {
		It("should exit when TTL is exceeded even if events keep coming", func() {
			// Override TTL to a short value
			original := tools.NonBlockingBridgeTTL
			tools.NonBlockingBridgeTTL = 300 * time.Millisecond
			defer func() { tools.NonBlockingBridgeTTL = original }()

			// Override inactivity to not interfere
			origInactivity := tools.BridgeInactivityTimeout
			tools.BridgeInactivityTimeout = 5 * time.Second
			defer func() { tools.BridgeInactivityTimeout = origInactivity }()

			eventCh := make(chan ka.InvestigationEvent, 100)
			closerCalled := make(chan struct{})

			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return &ka.StartInvestigationResult{
						SessionID: "sess-af-c1-003",
						Status:    "autonomous_started",
						Events:    eventCh,
						Closer: func() {
							close(closerCalled)
						},
					}, nil
				},
			}

			registry := tools.NewMonitorRegistry()
			_, err := tools.HandleInvestigationMCPWithRegistry(
				context.Background(), mockMCP, nil, "",
				tools.InvestigateMCPArgs{RRID: "rr-af-c1-003"},
				nil, registry, nil, false, nil, "", nil, nil,
			)
			Expect(err).NotTo(HaveOccurred())

			// Keep sending events to prevent inactivity timeout from firing
			go func() {
				for {
					select {
					case eventCh <- ka.InvestigationEvent{Type: ka.EventTypeReasoningDelta, Data: json.RawMessage(`{"text":"x"}`)}:
						time.Sleep(50 * time.Millisecond)
					case <-closerCalled:
						return
					}
				}
			}()

			// Bridge should exit after 300ms TTL
			Eventually(closerCalled, 2*time.Second).Should(BeClosed())
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

			tc := newTypedAIAnalysisClient()
			registry := tools.NewMonitorRegistry()

			start := time.Now()
			result, err := tools.HandleInvestigationMCPWithRegistry(
				context.Background(), mockMCP, tc, "kubernaut-system",
				tools.InvestigateMCPArgs{RRID: "rr-timeout-001"},
				nil, registry, nil, false, nil, "", nil, nil,
			)
			elapsed := time.Since(start)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.SessionID).To(Equal("sess-fast-001"))
			Expect(elapsed).To(BeNumerically("<", 30*time.Second),
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
				nil, nil, nil, false, nil, "", nil, nil,
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

			tc := newTypedClientForInvestigate()
			start := time.Now()
			result, err := tools.HandleInvestigationMCPWithRegistry(
				context.Background(), mockMCP, tc, "",
				tools.InvestigateMCPArgs{RRID: "rr-nons-001"},
				nil, nil, nil, false, nil, "", nil, nil,
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

			aiaObj := newTypedAIAnalysis("kubernaut-system", "aia-rr-aia-001", "rr-aia-001", "ka-sess-external")
			tc := newTypedAIAnalysisClient(aiaObj)
			registry := tools.NewMonitorRegistry()

			start := time.Now()
			result, err := tools.HandleInvestigationMCPWithRegistry(
				context.Background(), mockMCP, tc, "kubernaut-system",
				tools.InvestigateMCPArgs{RRID: "rr-aia-001"},
				nil, registry, nil, false, nil, "", nil, nil,
			)
			elapsed := time.Since(start)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.SessionID).To(Equal("sess-aia-found-001"))
			Expect(elapsed).To(BeNumerically("<", 10*time.Second),
				"should not block indefinitely when AIA exists but no Active IS")
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

			tc := newTypedAIAnalysisClient()
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()

			start := time.Now()
			result, err := tools.HandleInvestigationMCPWithRegistry(
				ctx, mockMCP, tc, "kubernaut-system",
				tools.InvestigateMCPArgs{RRID: "rr-cancel-001"},
				nil, nil, nil, false, nil, "", nil, nil,
			)
			elapsed := time.Since(start)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.SessionID).To(Equal("sess-cancel-001"))
			Expect(elapsed).To(BeNumerically("<", 5*time.Second),
				"parent context cancellation must abort AIA poll")
		})
	})
})

var _ = Describe("HandleInvestigationMCPWithRegistry — blocking mode (A2A path)", func() {

	Describe("UT-AF-1326-080: blocking mode waits for channel close and returns summary", func() {
		It("should collect reasoning_delta events into summary and return completed", func() {
			eventCh := make(chan ka.InvestigationEvent, 10)
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					go func() {
						eventCh <- ka.InvestigationEvent{
							Type: ka.EventTypeReasoningDelta,
							Data: json.RawMessage(`{"text":"The pod is crashing "}`),
						}
						eventCh <- ka.InvestigationEvent{
							Type: ka.EventTypeReasoningDelta,
							Data: json.RawMessage(`{"text":"due to OOM."}`),
						}
						eventCh <- ka.InvestigationEvent{
							Type: ka.EventTypeComplete,
							Data: json.RawMessage(`{}`),
						}
						close(eventCh)
					}()
					return &ka.StartInvestigationResult{
						SessionID: "sess-block-001",
						Status:    "autonomous_started",
						Events:    eventCh,
						Closer:    func() {},
					}, nil
				},
			}

			result, err := tools.HandleInvestigationMCPWithRegistry(
				context.Background(), mockMCP, nil, "",
				tools.InvestigateMCPArgs{RRID: "rr-block-001"},
				nil, nil, nil, true, nil, "", nil, nil,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.SessionID).To(Equal("sess-block-001"))
			Expect(result.Status).To(Equal("completed"))
			Expect(result.Summary).To(Equal("The pod is crashing due to OOM."))
		})
	})

	Describe("UT-AF-1326-081: blocking mode returns timeout on context cancellation", func() {
		It("should return partial summary and timeout status", func() {
			eventCh := make(chan ka.InvestigationEvent, 10)
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()

			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					go func() {
						eventCh <- ka.InvestigationEvent{
							Type: ka.EventTypeReasoningDelta,
							Data: json.RawMessage(`{"text":"Partial analysis"}`),
						}
						// Don't close channel — simulate long-running investigation
					}()
					return &ka.StartInvestigationResult{
						SessionID: "sess-block-timeout-001",
						Status:    "autonomous_started",
						Events:    eventCh,
						Closer:    func() {},
					}, nil
				},
			}

			result, err := tools.HandleInvestigationMCPWithRegistry(
				ctx, mockMCP, nil, "",
				tools.InvestigateMCPArgs{RRID: "rr-block-timeout-001"},
				nil, nil, nil, true, nil, "", nil, nil,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status).To(Equal("timeout"))
			Expect(result.Summary).To(Equal("Partial analysis"))
		})
	})

	Describe("UT-AF-1326-082: blocking mode with nil events returns immediately", func() {
		It("should return without blocking when Events channel is nil", func() {
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return &ka.StartInvestigationResult{
						SessionID: "sess-nil-events-001",
						Status:    "autonomous_started",
						Events:    nil,
						Closer:    func() {},
					}, nil
				},
			}

			start := time.Now()
			result, err := tools.HandleInvestigationMCPWithRegistry(
				context.Background(), mockMCP, nil, "",
				tools.InvestigateMCPArgs{RRID: "rr-nil-events-001"},
				nil, nil, nil, true, nil, "", nil, nil,
			)
			elapsed := time.Since(start)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.SessionID).To(Equal("sess-nil-events-001"))
			Expect(result.Summary).To(BeEmpty())
			Expect(elapsed).To(BeNumerically("<", 2*time.Second))
		})
	})

	Describe("UT-AF-1326-083: blocking mode filters non-reasoning events from summary", func() {
		It("should include only reasoning_delta text in summary, not tool_call_start", func() {
			eventCh := make(chan ka.InvestigationEvent, 10)
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					go func() {
						eventCh <- ka.InvestigationEvent{
							Type: ka.EventTypeToolCallStart,
							Data: json.RawMessage(`{"tool_name":"kubectl_get"}`),
						}
						eventCh <- ka.InvestigationEvent{
							Type: ka.EventTypeReasoningDelta,
							Data: json.RawMessage(`{"text":"Root cause: memory limit too low."}`),
						}
						eventCh <- ka.InvestigationEvent{
							Type: ka.EventTypeComplete,
							Data: json.RawMessage(`{}`),
						}
						close(eventCh)
					}()
					return &ka.StartInvestigationResult{
						SessionID: "sess-filter-001",
						Status:    "autonomous_started",
						Events:    eventCh,
						Closer:    func() {},
					}, nil
				},
			}

			result, err := tools.HandleInvestigationMCPWithRegistry(
				context.Background(), mockMCP, nil, "",
				tools.InvestigateMCPArgs{RRID: "rr-filter-001"},
				nil, nil, nil, true, nil, "", nil, nil,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Summary).To(Equal("Root cause: memory limit too low."))
			Expect(result.Summary).NotTo(ContainSubstring("kubectl_get"))
		})
	})
})

// #2086 (BR-INTERACTIVE-010, FedRAMP AU-3/SI-11): bridgeEventsCollectSummary
// previously returned identically whether it received a genuine terminal KA
// event (EventTypeComplete/Cancelled/SessionEnded) or simply gave up after
// its inactivity timer fired — the caller's status := "completed" only
// checked the OUTER ctx.Err(), never why the bridge itself returned. Live
// forensics on rr-cc99762025f0-5977eb36 showed KA's silent gate-retry LLM
// calls (see gate_keepalive_2086_test.go) blow past the 60s inactivity
// budget, so AF told the driving agent the investigation was "completed"
// with an empty RCA — the agent then never called discover_workflows. These
// specs prove the bridge now distinguishes "silently timed out" from
// "genuinely finished" and that the distinction reaches InvestigateMCPResult.
var _ = Describe("#2086: bridgeEventsCollectSummary/InvestigateMCPResult must not report false completion on inactivity timeout", func() {

	Describe("UT-AF-2086-004: BridgeEventsCollectSummary returns completed=false on inactivity timeout without a terminal event", func() {
		It("should signal completed=false when the bridge exits via its inactivity timer, not a terminal KA event", func() {
			eventCh := make(chan ka.InvestigationEvent, 5)
			eventCh <- ka.InvestigationEvent{
				Type: ka.EventTypeReasoningDelta,
				Data: json.RawMessage(`{"text":"Partial analysis before silence"}`),
			}
			// No further events, no close — simulate a silent gap (KA's
			// gate-retry LLM call) that outlasts inactivityTimeout without
			// KA ever actually completing (#2086).

			summary, rca, _, completed := tools.BridgeEventsCollectSummary(context.Background(), eventCh, 100*time.Millisecond)
			Expect(completed).To(BeFalse(),
				"UT-AF-2086-004: exiting via the inactivity timer (no EventTypeComplete/Cancelled/SessionEnded "+
					"seen) must be reported as NOT completed, or callers cannot distinguish a genuine finish "+
					"from a silent gap (#2086)")
			Expect(summary).To(Equal("Partial analysis before silence"))
			Expect(rca).To(BeNil())
		})
	})

	Describe("UT-AF-2086-004b: BridgeEventsCollectSummary returns completed=true on a genuine terminal event", func() {
		It("should signal completed=true when the bridge exits via EventTypeComplete", func() {
			eventCh := make(chan ka.InvestigationEvent, 5)
			eventCh <- ka.InvestigationEvent{
				Type: ka.EventTypeReasoningDelta,
				Data: json.RawMessage(`{"text":"Full analysis"}`),
			}
			eventCh <- ka.InvestigationEvent{Type: ka.EventTypeComplete, Data: json.RawMessage(`{}`)}
			close(eventCh)

			_, _, _, completed := tools.BridgeEventsCollectSummary(context.Background(), eventCh, 5*time.Second)
			Expect(completed).To(BeTrue(),
				"UT-AF-2086-004b: a genuine EventTypeComplete must be reported as completed=true, "+
					"so this regression guard fails loudly if a future change breaks the true-completion path")
		})
	})

	Describe("UT-AF-2086-005: HandleInvestigationMCPWithRegistry does not report status=completed when the bridge times out without a terminal event", func() {
		It("should return a truthful, non-completed status with guidance to poll instead of retry (#2086)", func() {
			origInactivity := tools.BridgeInactivityTimeout
			tools.BridgeInactivityTimeout = 100 * time.Millisecond
			defer func() { tools.BridgeInactivityTimeout = origInactivity }()

			eventCh := make(chan ka.InvestigationEvent, 5)
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					go func() {
						eventCh <- ka.InvestigationEvent{
							Type: ka.EventTypeReasoningDelta,
							Data: json.RawMessage(`{"text":"Investigating"}`),
						}
						// Simulate KA's silent gate-retry LLM call (#2086 root
						// cause): no further events arrive within
						// BridgeInactivityTimeout, and the investigation is
						// NOT actually complete.
					}()
					return &ka.StartInvestigationResult{
						SessionID: "sess-2086-005",
						Status:    "autonomous_started",
						Events:    eventCh,
						Closer:    func() {},
					}, nil
				},
			}

			// Outer ctx is never cancelled — the ONLY reason the bridge
			// returns here is BridgeInactivityTimeout, exactly like the live
			// #2086 incident where AF's outer ctx was healthy the whole time.
			result, err := tools.HandleInvestigationMCPWithRegistry(
				context.Background(), mockMCP, nil, "",
				tools.InvestigateMCPArgs{RRID: "rr-2086-005"},
				nil, nil, nil, true, nil, "", nil, nil,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status).NotTo(Equal("completed"),
				"UT-AF-2086-005: inactivity timeout without a terminal event must NOT be reported as "+
					"'completed' — this is the exact defect that caused #2086: the driving agent believed "+
					"the investigation had finished with an empty RCA and never called discover_workflows")
			Expect(result.Error).NotTo(BeEmpty(),
				"UT-AF-2086-005: the result must carry guidance so the driving agent knows to poll "+
					"kubernaut_get_remediation instead of retrying kubernaut_investigate")
			Expect(result.Error).NotTo(ContainSubstring("investigation complete"),
				"UT-AF-2086-005: guidance text must not imply the investigation finished")
		})
	})

	Describe("UT-AF-2086-006: bridgeEventsCollectSummary's summary excludes keepalive status text", func() {
		It("should not splice EventTypeToolCallStart keepalive text into the RCA summary (spike-discovered regression guard)", func() {
			eventCh := make(chan ka.InvestigationEvent, 5)
			eventCh <- ka.InvestigationEvent{
				Type: ka.EventTypeReasoningDelta,
				Data: json.RawMessage(`{"text":"Root cause: memory limit too low."}`),
			}
			// Fix 1's keepalive: emitted around KA's silent gate-retry LLM
			// call using EventTypeToolCallStart (NOT reasoning_delta/token_delta)
			// specifically so it cannot be concatenated into the summary below.
			eventCh <- ka.InvestigationEvent{
				Type: ka.EventTypeToolCallStart,
				Data: json.RawMessage(`{"tool_name":"revalidating_remediation_target"}`),
			}
			eventCh <- ka.InvestigationEvent{Type: ka.EventTypeComplete, Data: json.RawMessage(`{}`)}
			close(eventCh)

			summary, _, _, completed := tools.BridgeEventsCollectSummary(context.Background(), eventCh, 5*time.Second)
			Expect(completed).To(BeTrue())
			Expect(summary).To(Equal("Root cause: memory limit too low."),
				"UT-AF-2086-006: a keepalive emitted as EventTypeToolCallStart must NOT be concatenated "+
					"into the RCA summary returned to the driving agent — using EventTypeReasoningDelta for "+
					"the keepalive (the original plan) would have spliced placeholder status text directly "+
					"into the analysis content returned to the agent (spike-discovered content-correctness bug)")
			Expect(summary).NotTo(ContainSubstring("revalidating_remediation_target"))
		})
	})
})

// IT-AF-2086-010 (Pyramid Invariant: IT proves wiring): drives the FULL
// production dispatch path (HandleInvestigationMCPWithRegistry -> pool
// handoff -> bridgeEventsCollectSummary) with a fake KA session that goes
// silent for longer than BridgeInactivityTimeout before ever sending a
// terminal event -- the exact live #2086 shape. Proves two things a UT of
// bridgeEventsCollectSummary alone cannot: (1) the truthful non-completed
// status reaches the caller through the real handler, and (2) the pooled
// session handoff (so a later kubernaut_get_remediation/select_workflow can
// reuse the same MCP connection) still happens even though the investigation
// was not reported as completed.
var _ = Describe("IT-AF-2086-010: full dispatch path survives a silent gap without falsely reporting completion or dropping the pooled session", func() {
	It("should report a non-completed status AND still hand the session off to the pool (#2086)", func() {
		origInactivity := tools.BridgeInactivityTimeout
		tools.BridgeInactivityTimeout = 50 * time.Millisecond
		defer func() { tools.BridgeInactivityTimeout = origInactivity }()

		mockSession := &mockPoolSession{}
		eventCh := make(chan ka.InvestigationEvent, 5)
		mockMCP := &ka.MockMCPClient{
			StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
				go func() {
					eventCh <- ka.InvestigationEvent{
						Type: ka.EventTypeReasoningDelta,
						Data: json.RawMessage(`{"text":"Investigating before the silent gate-retry gap"}`),
					}
					// Silence longer than BridgeInactivityTimeout with no
					// ctx cancellation -- this is KA's silent gate-retry
					// LLM call from the live #2086 incident. The bridge
					// MUST give up here; any later send on eventCh below is
					// deliberately never read by THIS blocking call (it
					// already returned), matching the real incident where
					// KA's genuine completion arrived ~10s after AF had
					// already told the driving agent "completed".
					time.Sleep(150 * time.Millisecond)
					eventCh <- ka.InvestigationEvent{Type: ka.EventTypeComplete, Data: json.RawMessage(`{}`)}
					close(eventCh)
				}()
				return &ka.StartInvestigationResult{
					SessionID: "sess-it-2086-010",
					Status:    "autonomous_started",
					Events:    eventCh,
					Closer:    func() {},
					Session:   mockSession,
				}, nil
			},
		}

		registry := tools.NewMonitorRegistry()
		pool := ka.NewKASessionPool(ka.PoolConfig{
			Factory: func(_ context.Context) (ka.PoolSession, error) {
				return &mockPoolSession{}, nil
			},
			MaxEntries: 10,
			Logger:     logr.Discard(),
		})

		result, err := tools.HandleInvestigationMCPWithRegistry(
			context.Background(), mockMCP, nil, "",
			tools.InvestigateMCPArgs{RRID: "rr-it-2086-010"},
			nil, registry, nil, true, pool, "alice", nil, nil,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).NotTo(Equal("completed"),
			"IT-AF-2086-010: a silent gap that outlasts BridgeInactivityTimeout without a terminal event "+
				"must not be reported as completed through the real production dispatch path (#2086)")

		acquired, acqErr := pool.Acquire(context.Background(), "rr-it-2086-010", "alice")
		Expect(acqErr).NotTo(HaveOccurred())
		Expect(acquired).To(BeIdenticalTo(mockSession),
			"IT-AF-2086-010: the MCP session must still be handed off to the pool even though the "+
				"investigation was not reported as completed, so a later kubernaut_get_remediation or "+
				"select_workflow call can reuse the connection instead of KA's session being orphaned")

		Expect(registry.Active("sess-it-2086-010")).To(BeFalse(),
			"IT-AF-2086-010: session must be deregistered from MonitorRegistry once handed off to the pool")
	})
})

var _ = Describe("HandleInvestigationMCPWithRegistry — pool handoff (session persistence)", func() {

	Describe("UT-AF-1332-010: blocking mode injects session into pool after investigation", func() {
		It("should inject session and deregister from MonitorRegistry", func() {
			mockSession := &mockPoolSession{}
			eventCh := make(chan ka.InvestigationEvent, 10)
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					go func() {
						eventCh <- ka.InvestigationEvent{
							Type: ka.EventTypeReasoningDelta,
							Data: json.RawMessage(`{"text":"Analysis complete."}`),
						}
						eventCh <- ka.InvestigationEvent{
							Type: ka.EventTypeComplete,
							Data: json.RawMessage(`{}`),
						}
						close(eventCh)
					}()
					return &ka.StartInvestigationResult{
						SessionID: "sess-handoff-001",
						Status:    "autonomous_started",
						Events:    eventCh,
						Closer:    func() {},
						Session:   mockSession,
					}, nil
				},
			}

			registry := tools.NewMonitorRegistry()
			pool := ka.NewKASessionPool(ka.PoolConfig{
				Factory: func(_ context.Context) (ka.PoolSession, error) {
					return &mockPoolSession{}, nil
				},
				MaxEntries: 10,
				Logger:     logr.Discard(),
			})

			result, err := tools.HandleInvestigationMCPWithRegistry(
				context.Background(), mockMCP, nil, "",
				tools.InvestigateMCPArgs{RRID: "rr-handoff-001"},
				nil, registry, nil, true, pool, "alice", nil, nil,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status).To(Equal("completed"))
			Expect(result.Summary).To(Equal("Analysis complete."))

			// Verify session was injected into pool
			acquired, err := pool.Acquire(context.Background(), "rr-handoff-001", "alice")
			Expect(err).NotTo(HaveOccurred())
			Expect(acquired).To(BeIdenticalTo(mockSession),
				"pool should return the injected investigation session")

			// Verify session was deregistered from MonitorRegistry
			Expect(registry.Active("sess-handoff-001")).To(BeFalse(),
				"session should be deregistered from MonitorRegistry after handoff")
		})
	})

	Describe("UT-AF-1332-011: blocking mode falls back to cleanup when pool is nil", func() {
		It("should call cleanup (closer) when pool is nil", func() {
			var closerCalled int32
			eventCh := make(chan ka.InvestigationEvent, 10)
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					go func() {
						eventCh <- ka.InvestigationEvent{
							Type: ka.EventTypeComplete,
							Data: json.RawMessage(`{}`),
						}
						close(eventCh)
					}()
					return &ka.StartInvestigationResult{
						SessionID: "sess-nil-pool-001",
						Status:    "autonomous_started",
						Events:    eventCh,
						Closer:    func() { atomic.AddInt32(&closerCalled, 1) },
					}, nil
				},
			}

			result, err := tools.HandleInvestigationMCPWithRegistry(
				context.Background(), mockMCP, nil, "",
				tools.InvestigateMCPArgs{RRID: "rr-nil-pool-001"},
				nil, nil, nil, true, nil, "", nil, nil,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status).To(Equal("completed"))
			Expect(atomic.LoadInt32(&closerCalled)).To(Equal(int32(1)),
				"cleanup (closer) should be called when pool is nil")
		})
	})
})

var _ = Describe("HandleInvestigationMCPWithRegistry — session ID forwarding (#1452 BR-INTERACTIVE-010)", func() {

	Describe("UT-AF-1452-001 [SI-4]: AIA CRD session ID forwarded to StartInvestigation", func() {
		It("should pass the AIA-provided session ID to the MCP client", func() {
			var receivedArgs ka.StartInvestigationArgs
			eventCh := make(chan ka.InvestigationEvent, 10)
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, args ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					receivedArgs = args
					return &ka.StartInvestigationResult{
						SessionID: "ka-sess-1452-001",
						Status:    "started",
						Events:    eventCh,
						Closer:    func() { close(eventCh) },
					}, nil
				},
			}

			aiaObj := newTypedAIAnalysis("kubernaut-system", "aia-rr-1452-001", "rr-1452-001", "ka-sess-1452-001")
			tc := newTypedAIAnalysisClient(aiaObj)
			registry := tools.NewMonitorRegistry()

			_, err := tools.HandleInvestigationMCPWithRegistry(
				context.Background(), mockMCP, tc, "kubernaut-system",
				tools.InvestigateMCPArgs{RRID: "rr-1452-001"},
				nil, registry, nil, false, nil, "", nil, nil,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedArgs.SessionID).To(Equal("ka-sess-1452-001"),
				"SI-4: AIA CRD session ID must be forwarded to KA for deterministic session lookup")
		})
	})

	Describe("UT-AF-1452-002 [SI-4]: timeout path forwards empty session ID (graceful degradation)", func() {
		It("should pass empty SessionID when AIA CRD does not become ready", func() {
			var receivedArgs ka.StartInvestigationArgs
			eventCh := make(chan ka.InvestigationEvent, 10)
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, args ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					receivedArgs = args
					return &ka.StartInvestigationResult{
						SessionID: "ka-new-sess",
						Status:    "started",
						Events:    eventCh,
						Closer:    func() { close(eventCh) },
					}, nil
				},
			}

			tc := newTypedAIAnalysisClient()
			registry := tools.NewMonitorRegistry()

			_, err := tools.HandleInvestigationMCPWithRegistry(
				context.Background(), mockMCP, tc, "kubernaut-system",
				tools.InvestigateMCPArgs{RRID: "rr-1452-002"},
				nil, registry, nil, false, nil, "", nil, nil,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedArgs.SessionID).To(BeEmpty(),
				"SI-4: when AIA CRD times out, SessionID must be empty so KA falls back to RRID scan")
		})
	})

	Describe("UT-AF-1452-005 [SC-8]: forwarded SessionID matches AIA CRD value exactly", func() {
		It("should transmit the session ID unmodified from AIA CRD to MCP client", func() {
			const aiaSessionID = "1d87a525-exact-match-test"
			var receivedArgs ka.StartInvestigationArgs
			eventCh := make(chan ka.InvestigationEvent, 10)
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, args ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					receivedArgs = args
					return &ka.StartInvestigationResult{
						SessionID: aiaSessionID,
						Status:    "started",
						Events:    eventCh,
						Closer:    func() { close(eventCh) },
					}, nil
				},
			}

			aiaObj := newTypedAIAnalysis("kubernaut-system", "aia-rr-1452-005", "rr-1452-005", aiaSessionID)
			tc := newTypedAIAnalysisClient(aiaObj)
			registry := tools.NewMonitorRegistry()

			_, err := tools.HandleInvestigationMCPWithRegistry(
				context.Background(), mockMCP, tc, "kubernaut-system",
				tools.InvestigateMCPArgs{RRID: "rr-1452-005"},
				nil, registry, nil, false, nil, "", nil, nil,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedArgs.SessionID).To(Equal(aiaSessionID),
				"SC-8: session ID must be transmitted unmodified through the AF forwarding path")
		})
	})

	Describe("UT-AF-1452-006 [AU-3]: audit event ka_correlation_id matches KA-confirmed session", func() {
		It("should emit delegation audit event with ka_correlation_id from KA response", func() {
			eventCh := make(chan ka.InvestigationEvent, 10)
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return &ka.StartInvestigationResult{
						SessionID: "ka-confirmed-1452-006",
						Status:    "started",
						Events:    eventCh,
						Closer:    func() { close(eventCh) },
					}, nil
				},
			}

			aiaObj := newTypedAIAnalysis("kubernaut-system", "aia-rr-1452-006", "rr-1452-006", "ka-confirmed-1452-006")
			tc := newTypedAIAnalysisClient(aiaObj)
			recorder := &auditRecorder{}
			registry := tools.NewMonitorRegistry()

			_, err := tools.HandleInvestigationMCPWithRegistry(
				context.Background(), mockMCP, tc, "kubernaut-system",
				tools.InvestigateMCPArgs{RRID: "rr-1452-006"},
				recorder, registry, nil, false, nil, "", nil, nil,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(recorder.events).To(HaveLen(1))
			Expect(recorder.events[0].Detail["ka_correlation_id"]).To(Equal("ka-confirmed-1452-006"),
				"AU-3: audit event must reference the KA-confirmed session ID for traceability")
		})
	})
})

// mockPoolSession implements ka.PoolSession for testing.
type mockPoolSession struct{}

func (m *mockPoolSession) CallTool(_ context.Context, _ *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	return &mcp.CallToolResult{}, nil
}
func (m *mockPoolSession) Ping(_ context.Context, _ *mcp.PingParams) error { return nil }
func (m *mockPoolSession) Close() error                                    { return nil }

// Suppress unused import warning for json and time
var _ = json.Marshal
var _ time.Duration

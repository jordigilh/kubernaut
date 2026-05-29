package investigator_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
)

type delayedFakeTool struct {
	name      string
	result    string
	delay     time.Duration
	active    *atomic.Int32
	execCount *atomic.Int32
}

func (f *delayedFakeTool) Name() string                { return f.name }
func (f *delayedFakeTool) Description() string         { return "delayed fake " + f.name }
func (f *delayedFakeTool) Parameters() json.RawMessage { return nil }
func (f *delayedFakeTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	if f.active != nil {
		f.active.Add(1)
		defer f.active.Add(-1)
	}
	if f.execCount != nil {
		f.execCount.Add(1)
	}
	select {
	case <-time.After(f.delay):
		return f.result, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

type ctxAwareMockLLMClient struct {
	calls     []llm.ChatRequest
	responses []llm.ChatResponse
	callIdx   int
}

func (m *ctxAwareMockLLMClient) Close() error { return nil }

func (m *ctxAwareMockLLMClient) StreamChat(ctx context.Context, req llm.ChatRequest, _ func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	return m.Chat(ctx, req)
}

func (m *ctxAwareMockLLMClient) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	if err := ctx.Err(); err != nil {
		return llm.ChatResponse{}, err
	}
	m.calls = append(m.calls, req)
	if m.callIdx < len(m.responses) {
		resp := m.responses[m.callIdx]
		m.callIdx++
		return resp, nil
	}
	return llm.ChatResponse{
		Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"done","confidence":0.1}`},
	}, nil
}

var _ = Describe("BR-PERFORMANCE-970: Parallel Tool Execution in runLLMLoop", func() {

	var (
		invLogger  logr.Logger
		auditStore *recordingAuditStore
		builder    *prompt.Builder
		rp         *parser.ResultParser
		enricher   *enrichment.Enricher
		phaseTools katypes.PhaseToolMap
	)

	BeforeEach(func() {
		invLogger = logr.Discard()
		auditStore = &recordingAuditStore{}
		builder, _ = prompt.NewBuilder()
		rp = parser.NewResultParser()
		k8sClient := &fakeK8sClient{ownerChain: []enrichment.OwnerChainEntry{}}
		dsClient := &fakeDataStorageClient{history: &enrichment.RemediationHistoryResult{}}
		enricher = enrichment.NewEnricher(k8sClient, dsClient, auditStore, invLogger)
		phaseTools = investigator.DefaultPhaseToolMap()
	})

	signal := katypes.SignalContext{
		Name: "api-server", Namespace: "default", Severity: "warning",
		Environment: "Development", Priority: "P2",
		Message: "CrashLoopBackOff", RemediationID: "rem-970-test",
	}

	Describe("IT-KA-970-001: Parallel Execution — Ordering, Timing, and Audit", func() {
		It("should execute tool calls in parallel with wall-time < 0.6x sequential, preserving message and audit order", func() {
			const toolDelay = 100 * time.Millisecond
			const numTools = 4

			reg := registry.New()
			toolNames := []string{"kubectl_describe", "kubectl_events", "kubectl_logs", "kubectl_get_by_name"}
			for _, tn := range toolNames {
				reg.Register(&delayedFakeTool{name: tn, result: fmt.Sprintf(`{"tool":"%s","status":"ok"}`, tn), delay: toolDelay})
			}

			toolCalls := make([]llm.ToolCall, numTools)
			for i, tn := range toolNames {
				toolCalls[i] = llm.ToolCall{ID: fmt.Sprintf("tc_%d", i), Name: tn, Arguments: `{}`}
			}

			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{
						Message:   llm.Message{Role: "assistant", Content: "Investigating..."},
						ToolCalls: toolCalls,
					},
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"pod crash due to OOM","confidence":0.9,"severity":"high","signal_name":"oom-kill"}`}},
					wfToolResp(`{"workflow_id":"restart-pod","confidence":0.8}`),
				},
			}

			detector := investigator.NewAnomalyDetector(
				investigator.AnomalyConfig{MaxToolCallsPerTool: 100, MaxTotalToolCalls: 100, MaxRepeatedFailures: 100},
				nil,
			)

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: invLogger,
				MaxTurns: 15, PhaseTools: phaseTools, Registry: reg,
				Pipeline: investigator.Pipeline{AnomalyDetector: detector},
			})

			start := time.Now()
			_, err := inv.Investigate(context.Background(), signal)
			elapsed := time.Since(start)
			Expect(err).NotTo(HaveOccurred())

			sequentialBaseline := time.Duration(numTools) * toolDelay
			Expect(elapsed).To(BeNumerically("<", time.Duration(float64(sequentialBaseline)*0.6)),
				"parallel execution should complete in < 60%% of sequential baseline (%v); got %v", sequentialBaseline, elapsed)

			// Verify message ordering: the 2nd LLM call's messages should end with
			// tool results in declaration order (tc_0, tc_1, tc_2, tc_3)
			Expect(mockClient.calls).To(HaveLen(3))
			secondCallMsgs := mockClient.calls[1].Messages
			toolMsgs := filterToolMessages(secondCallMsgs)
			Expect(toolMsgs).To(HaveLen(numTools))
			for i, tm := range toolMsgs {
				Expect(tm.ToolCallID).To(Equal(fmt.Sprintf("tc_%d", i)),
					"tool message at position %d should have ToolCallID tc_%d", i, i)
				Expect(tm.ToolName).To(Equal(toolNames[i]),
					"tool message at position %d should be for %s", i, toolNames[i])
			}

			// Verify audit events: tool_call_index should match declaration order
			toolCallEvents := filterAuditEvents(auditStore.events, audit.EventTypeLLMToolCall)
			rcaToolCallEvents := toolCallEvents[:numTools]
			for i, ev := range rcaToolCallEvents {
				Expect(ev.Data["tool_call_index"]).To(Equal(i),
					"audit event %d should have tool_call_index=%d", i, i)
				Expect(ev.Data["tool_name"]).To(Equal(toolNames[i]),
					"audit event %d should reference tool %s", i, toolNames[i])
			}
		})
	})

	Describe("IT-KA-970-002: Budget Enforcement Under Parallel Execution", func() {
		It("should reject tool calls that exceed the budget even under parallel dispatch", func() {
			const numRequested = 5
			const budget = 3

			execCounter := &atomic.Int32{}
			reg := registry.New()
			for i := 0; i < numRequested; i++ {
				tn := fmt.Sprintf("tool_%d", i)
				reg.Register(&delayedFakeTool{
					name: tn, result: fmt.Sprintf(`{"tool":"%s"}`, tn),
					delay: 10 * time.Millisecond, execCount: execCounter,
				})
			}

			toolCalls := make([]llm.ToolCall, numRequested)
			for i := 0; i < numRequested; i++ {
				toolCalls[i] = llm.ToolCall{ID: fmt.Sprintf("tc_%d", i), Name: fmt.Sprintf("tool_%d", i), Arguments: `{}`}
			}

			mock := &mockLLMClient{
				responses: []llm.ChatResponse{
					{
						Message:   llm.Message{Role: "assistant", Content: "checking all"},
						ToolCalls: toolCalls,
					},
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"done","confidence":0.9,"severity":"low","signal_name":"test"}`}},
					wfToolResp(`{"workflow_id":"noop","confidence":0.5}`),
				},
			}

			detector := investigator.NewAnomalyDetector(
				investigator.AnomalyConfig{MaxToolCallsPerTool: 100, MaxTotalToolCalls: budget, MaxRepeatedFailures: 100},
				nil,
			)

			inv := investigator.New(investigator.Config{
				Client: mock, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: invLogger,
				MaxTurns: 15, PhaseTools: phaseTools, Registry: reg,
				Pipeline: investigator.Pipeline{AnomalyDetector: detector},
			})

			_, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())

			Expect(int(execCounter.Load())).To(Equal(budget),
				"exactly %d tools should have been executed via registry; got %d", budget, execCounter.Load())

			// Verify via audit: tool call events for rejected tools contain error
			toolCallEvents := filterAuditEvents(auditStore.events, audit.EventTypeLLMToolCall)
			rejectedCount := 0
			for _, ev := range toolCallEvents {
				if result, ok := ev.Data["tool_result"].(string); ok && containsErrorJSON(result) {
					rejectedCount++
				}
			}
			Expect(rejectedCount).To(Equal(numRequested - budget),
				"exactly %d tool calls should have been rejected", numRequested-budget)
		})
	})

	Describe("IT-KA-970-003: Context Cancellation Propagation", func() {
		It("should terminate all in-flight tool goroutines when context is cancelled", func() {
			const numTools = 4
			activeCounter := &atomic.Int32{}

			reg := registry.New()
			tn := []string{"slow_tool_0", "slow_tool_1", "slow_tool_2", "slow_tool_3"}
			for _, name := range tn {
				reg.Register(&delayedFakeTool{
					name: name, result: `{"status":"ok"}`,
					delay: 5 * time.Second, active: activeCounter,
				})
			}

			toolCalls := make([]llm.ToolCall, numTools)
			for i, name := range tn {
				toolCalls[i] = llm.ToolCall{ID: fmt.Sprintf("tc_%d", i), Name: name, Arguments: `{}`}
			}

			mock := &ctxAwareMockLLMClient{
				responses: []llm.ChatResponse{
					{
						Message:   llm.Message{Role: "assistant", Content: "checking"},
						ToolCalls: toolCalls,
					},
				},
			}

			detector := investigator.NewAnomalyDetector(investigator.DefaultAnomalyConfig(), nil)

			inv := investigator.New(investigator.Config{
				Client: mock, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: invLogger,
				MaxTurns: 15, PhaseTools: phaseTools, Registry: reg,
				Pipeline: investigator.Pipeline{AnomalyDetector: detector},
			})

			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			result, err := inv.Investigate(ctx, signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Cancelled).To(BeTrue(),
				"investigation result should be marked as cancelled when context times out")

			Eventually(activeCounter.Load, 2*time.Second, 50*time.Millisecond).Should(Equal(int32(0)),
				"all tool goroutines should have exited after context cancellation")
		})
	})
})

func filterToolMessages(msgs []llm.Message) []llm.Message {
	var result []llm.Message
	for _, m := range msgs {
		if m.Role == "tool" {
			result = append(result, m)
		}
	}
	return result
}

func filterAuditEvents(events []*audit.AuditEvent, eventType string) []*audit.AuditEvent {
	var result []*audit.AuditEvent
	for _, e := range events {
		if e.EventType == eventType {
			result = append(result, e)
		}
	}
	return result
}

func containsErrorJSON(content string) bool {
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return false
	}
	_, hasError := parsed["error"]
	return hasError
}

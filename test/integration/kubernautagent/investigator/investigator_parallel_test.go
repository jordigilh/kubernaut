package investigator_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
)

type delayedFakeTool struct {
	name      string
	result    string
	delay     time.Duration
	active    *atomic.Int32
	peak      *atomic.Int32
	execCount *atomic.Int32
}

func (f *delayedFakeTool) Name() string                { return f.name }
func (f *delayedFakeTool) Description() string         { return "delayed fake " + f.name }
func (f *delayedFakeTool) Parameters() json.RawMessage { return nil }
func (f *delayedFakeTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	if f.active != nil {
		cur := f.active.Add(1)
		defer f.active.Add(-1)
		if f.peak != nil {
			for {
				p := f.peak.Load()
				if cur <= p || f.peak.CompareAndSwap(p, cur) {
					break
				}
			}
		}
	}
	if f.execCount != nil {
		f.execCount.Add(1)
	}
	timer := time.NewTimer(f.delay)
	defer timer.Stop()
	select {
	case <-timer.C:
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
		auditStore *capturingAuditStore
		builder    *prompt.Builder
		rp         *parser.ResultParser
		enricher   *enrichment.Enricher
		phaseTools katypes.PhaseToolMap
	)

	BeforeEach(func() {
		invLogger = logr.Discard()
		auditStore = newCapturingAuditStore(suiteAuditStore)
		builder, _ = prompt.NewBuilder()
		rp = parser.NewResultParser()
		k8sClient := &k8sFixtureClient{ownerChain: []enrichment.OwnerChainEntry{}}
		enricher = enrichment.NewEnricher(k8sClient, suiteDSAdapter, auditStore, invLogger)
		phaseTools = investigator.DefaultPhaseToolMap()
	})

	signal := katypes.SignalContext{
		Name: "api-server", Namespace: "default", Severity: "warning",
		Environment: "Development", Priority: "P2",
		Message: "CrashLoopBackOff", RemediationID: "rem-970-test",
	}

	Describe("IT-KA-970-001: Parallel Execution — Ordering, Timing, and Audit", func() {
		It("should execute tool calls in parallel with wall-time < sequential baseline, preserving message and audit order", func() {
			const toolDelay = 200 * time.Millisecond
			const numTools = 4

			active := &atomic.Int32{}
			peak := &atomic.Int32{}
			reg := registry.New()
			toolNames := []string{"kubectl_describe", "kubectl_events", "kubectl_logs", "kubectl_get_by_name"}
			for _, tn := range toolNames {
				reg.Register(&delayedFakeTool{name: tn, result: fmt.Sprintf(`{"tool":"%s","status":"ok"}`, tn), delay: toolDelay, active: active, peak: peak})
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

			// PRIMARY proof of parallelism (#1552): peak concurrent active tool
			// executions is a deterministic structural signal, unlike wall-clock
			// timing. A tight elapsed-vs-sequential-baseline assertion is
			// intrinsically flaky on shared CI runners -- this exact check was
			// already loosened once (0.6x -> 1.0x sequential baseline in
			// 7c4c14d3b) and still failed at 941ms and 852ms against the 800ms
			// (1.0x) ceiling despite genuinely parallel dispatch. If dispatch
			// ever regressed to sequential, peak would deterministically be 1,
			// not a value that only sometimes drifts past a timing threshold.
			Expect(peak.Load()).To(Equal(int32(numTools)),
				"all %d tool calls should have been in flight concurrently at some point, proving genuine parallel dispatch rather than a sequential fallback", numTools)

			// SECONDARY sanity ceiling: guards against a true hang, not tuned to
			// prove speed. Anchored to a single tool's delay plus a large fixed
			// margin (not a multiple of numTools) so CI scheduling jitter can
			// never trip it; a real sequential-fallback regression would still
			// be caught above by the peak-concurrency assertion.
			sanityCeiling := toolDelay + 5*time.Second
			Expect(elapsed).To(BeNumerically("<", sanityCeiling),
				"parallel execution took %v, exceeding the generous sanity ceiling of %v -- possible hang", elapsed, sanityCeiling)

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
			// BR-KA-017: budget enforcement must reach phase-eligible readers,
			// not stop earlier on the separate off-phase authorization guard.
			Expect(len(phaseTools[katypes.PhaseRCA])).To(BeNumerically(">=", numRequested))
			toolNames := phaseTools[katypes.PhaseRCA][:numRequested]
			for _, tn := range toolNames {
				reg.Register(&delayedFakeTool{
					name: tn, result: fmt.Sprintf(`{"tool":"%s"}`, tn),
					delay: 10 * time.Millisecond, execCount: execCounter,
				})
			}

			toolCalls := make([]llm.ToolCall, numRequested)
			for i, name := range toolNames {
				toolCalls[i] = llm.ToolCall{ID: fmt.Sprintf("tc_%d", i), Name: name, Arguments: `{}`}
			}

			mock := &mockLLMClient{
				responses: []llm.ChatResponse{
					{
						Message:   llm.Message{Role: "assistant", Content: "checking all"},
						ToolCalls: toolCalls,
					},
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"done","confidence":0.9,"severity":"info","signal_name":"test"}`}},
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
			Expect(rejectedCount).To(Equal(numRequested-budget),
				"exactly %d tool calls should have been rejected", numRequested-budget)
		})
	})

	Describe("IT-KA-970-003: Context Cancellation Propagation", func() {
		It("should terminate all in-flight readers and persist cancellation [BR-PERFORMANCE-970, BR-AUDIT-005, AU-2, AU-3, CC7.2]", func() {
			const numTools = 4
			activeCounter := &atomic.Int32{}
			execCounter := &atomic.Int32{}

			reg := registry.New()
			// BR-KA-017: cancellation must reach the external readers, not
			// return early through the independent off-phase denial contract.
			tn := []string{"kubectl_describe", "kubectl_events", "kubectl_logs", "kubectl_get_by_name"}
			for _, name := range tn {
				reg.Register(&delayedFakeTool{
					name: name, result: `{"status":"ok"}`,
					delay: 5 * time.Second, active: activeCounter, execCount: execCounter,
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
						Usage:     llm.TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
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

			cancelSignal := signal
			cancelSignal.RemediationID = "rem-970-cancel-" + uuid.NewString()
			cancelSignal.IncidentID = cancelSignal.RemediationID
			cancelSignal.ResourceKind = "Pod"
			cancelSignal.ResourceName = signal.Name
			cancelSignal.ResourceAPIVersion = "v1"
			sessionID := "session-970-" + uuid.NewString()
			ctx, cancel := context.WithCancel(session.WithSessionID(context.Background(), sessionID))
			defer cancel()

			var result *katypes.InvestigationResult
			var investigationErr error
			done := make(chan struct{})
			go func() {
				defer GinkgoRecover()
				defer close(done)
				result, investigationErr = inv.Investigate(ctx, cancelSignal)
			}()
			DeferCleanup(func() {
				cancel()
				Eventually(done, 10*time.Second, 50*time.Millisecond).Should(BeClosed(),
					"the fixture investigation must exit even if an assertion fails")
			})

			By("waiting for real enrichment/audit dispatch and all four readers before the cancellation window")
			Eventually(execCounter.Load, 10*time.Second, 50*time.Millisecond).Should(Equal(int32(numTools)))
			Expect(activeCounter.Load()).To(Equal(int32(numTools)), "all readers must still be in flight")
			// BR-PERFORMANCE-970: the unchanged 100ms window measures cancellation
			// of running tools, not unrelated authenticated DS setup latency.
			cancelTimer := time.AfterFunc(100*time.Millisecond, cancel)
			defer cancelTimer.Stop()
			Eventually(done, 10*time.Second, 50*time.Millisecond).Should(BeClosed())
			Expect(investigationErr).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Cancelled).To(BeTrue(),
				"investigation result should be marked as cancelled after the 100ms window")
			Expect(result.CancelledPhase).To(Equal(string(katypes.PhaseRCA)))
			Expect(result.WorkflowID).To(BeEmpty(), "cancellation cannot produce an executable selection")
			Expect(mock.calls).To(HaveLen(1), "no workflow-selection provider call may follow cancellation")
			Expect(execCounter.Load()).To(Equal(int32(numTools)),
				"BR-PERFORMANCE-970: cancellation must stop tools that actually started; phase-denied calls cannot prove goroutine cleanup")

			Eventually(activeCounter.Load, 2*time.Second, 50*time.Millisecond).Should(Equal(int32(0)),
				"all tool goroutines should have exited after context cancellation")

			By("proving AU-2/AU-3 and CC7.2 through the authenticated persisted cancellation record")
			cancelEvents := filterAuditEvents(auditStore.events, audit.EventTypeInvestigationCancelled)
			Expect(cancelEvents).To(HaveLen(1), "exactly one cancellation event must be emitted")
			var persisted *ogenclient.AuditEventsQueryResponse
			Eventually(func(g Gomega) {
				records, queryErr := ogenClient.QueryAuditEvents(context.Background(), ogenclient.QueryAuditEventsParams{
					CorrelationID: ogenclient.NewOptString(cancelSignal.RemediationID),
					EventType:     ogenclient.NewOptString(audit.EventTypeInvestigationCancelled),
				})
				g.Expect(queryErr).NotTo(HaveOccurred())
				g.Expect(records).NotTo(BeNil())
				g.Expect(records.Data).To(HaveLen(1), "cancellation must reach durable storage, not only the capture spy")
				persisted = records
			}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

			stored := persisted.Data[0]
			Expect(stored.EventType).To(Equal(audit.EventTypeInvestigationCancelled))
			Expect(stored.CorrelationID).To(Equal(cancelSignal.RemediationID))
			Expect(stored.EventAction).To(Equal(audit.ActionInvestigationCancelled))
			Expect(string(stored.EventOutcome)).To(Equal(audit.OutcomeFailure))
			Expect(stored.ActorType).To(Equal(ogenclient.NewOptString("service")))
			Expect(stored.ActorID).To(Equal(ogenclient.NewOptString("kubernaut-agent")))
			storedID, hasID := stored.EventID.Get()
			Expect(hasID).To(BeTrue())
			Expect(storedID).NotTo(Equal(uuid.Nil))
			payload, isCancellation := stored.EventData.GetAIAgentInvestigationCancelledPayload()
			Expect(isCancellation).To(BeTrue(), "typed DS ingestion/query must preserve the cancellation discriminator")
			payloadID, parseErr := uuid.Parse(payload.EventID)
			Expect(parseErr).NotTo(HaveOccurred())
			Expect(payloadID).NotTo(Equal(uuid.Nil))
			Expect(payload.EventID).To(Equal(cancelEvents[0].Data["event_id"]))
			Expect(payload.CancelledPhase).To(Equal(result.CancelledPhase))
			Expect(payload.CancelledAtTurn).To(Equal(result.CancelledAtTurn))
			Expect(payload.SessionID).To(Equal(ogenclient.NewOptString(sessionID)))
			Expect(payload.TotalPromptTokens).To(Equal(ogenclient.NewOptInt(100)))
			Expect(payload.TotalCompletionTokens).To(Equal(ogenclient.NewOptInt(50)))
			Expect(payload.TotalTokens).To(Equal(ogenclient.NewOptInt(150)))
			messages, hasMessages := payload.AccumulatedMessages.Get()
			Expect(hasMessages).To(BeTrue())
			Expect(json.Valid([]byte(messages))).To(BeTrue())
			for _, name := range tn {
				Expect(messages).To(ContainSubstring(name), "the cancelled attempt must be reconstructable from persisted messages")
			}
			allRecords, queryErr := ogenClient.QueryAuditEvents(context.Background(), ogenclient.QueryAuditEventsParams{
				CorrelationID: ogenclient.NewOptString(cancelSignal.RemediationID),
				Limit:         ogenclient.NewOptInt(1000),
			})
			Expect(queryErr).NotTo(HaveOccurred())
			Expect(allRecords).NotTo(BeNil())
			for _, record := range allRecords.Data {
				Expect(record.EventType).NotTo(Equal(audit.EventTypeSelectionValidated), "cancelled RCA cannot validate a selection")
			}
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

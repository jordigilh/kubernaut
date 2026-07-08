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

package investigator_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/go-logr/logr"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/metrics"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/shared/backoff"
)

// streamingRetryFastBackoff keeps these tests fast and deterministic —
// mirrors chat_with_params_test.go's fastBackoff in the llm package.
var streamingRetryFastBackoff = &backoff.Config{
	BasePeriod:    1 * time.Millisecond,
	MaxPeriod:     5 * time.Millisecond,
	Multiplier:    2.0,
	JitterPercent: 0,
}

// scriptedStreamClient scripts StreamChat's behavior per call index: chunks
// to emit via the callback, then either an error or a success response.
// Calls beyond the scripted length fall back to succeeding immediately with
// the last scripted response (or a generic fallback), so later
// turns/phases in a full Investigate() run complete normally without
// additional scripting. cancelAfterCall, when > 0, invokes cancelFn once
// the given 1-based call number completes — simulating an operator cancel
// arriving while a retryable failure is in flight (#1612).
type scriptedStreamClient struct {
	mu              sync.Mutex
	calls           int
	chunksSeq       [][]string
	errSeq          []error
	responses       []llm.ChatResponse
	cancelAfterCall int
	cancelFn        context.CancelFunc
}

func (m *scriptedStreamClient) StreamChat(_ context.Context, _ llm.ChatRequest, callback func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	m.mu.Lock()
	idx := m.calls
	m.calls++
	m.mu.Unlock()

	var chunks []string
	if idx < len(m.chunksSeq) {
		chunks = m.chunksSeq[idx]
	}
	for _, c := range chunks {
		_ = callback(llm.ChatStreamEvent{Delta: c})
	}

	var stepErr error
	if idx < len(m.errSeq) {
		stepErr = m.errSeq[idx]
	}

	if m.cancelAfterCall > 0 && idx+1 == m.cancelAfterCall && m.cancelFn != nil {
		m.cancelFn()
	}

	if stepErr != nil {
		return llm.ChatResponse{}, stepErr
	}

	resp := m.fallbackResponse()
	if idx < len(m.responses) {
		resp = m.responses[idx]
	}
	_ = callback(llm.ChatStreamEvent{Done: true})
	return resp, nil
}

func (m *scriptedStreamClient) fallbackResponse() llm.ChatResponse {
	if len(m.responses) > 0 {
		return m.responses[len(m.responses)-1]
	}
	return llm.ChatResponse{
		Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"fallback","confidence":0.1}`},
	}
}

// Chat is required by llm.Client but unused by these tests: chatOrStream
// only dispatches to Chat (via ChatWithParams) when no event sink is
// present in ctx, and every test in this file installs one via
// session.WithEventSink to force the streaming path.
func (m *scriptedStreamClient) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	return m.fallbackResponse(), nil
}

func (m *scriptedStreamClient) Close() error { return nil }

func (m *scriptedStreamClient) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

// fixedPhaseResolver implements investigator.PhaseClientResolver, returning
// the same client/model/runtime-params for every phase — lets these tests
// inject a RuntimeParams.MaxRetries/RetryBackoff that the zero-value
// fallback (no Swappable, no PhaseResolver) can't express.
type fixedPhaseResolver struct {
	client        llm.Client
	runtimeParams llm.RuntimeParams
}

func (r fixedPhaseResolver) ResolvePhase(_ katypes.Phase) (llm.Client, string, llm.RuntimeParams) {
	return r.client, "test-model", r.runtimeParams
}

func streamingRetryTestInvestigator(client llm.Client, runtimeParams llm.RuntimeParams, auditStore audit.AuditStore) *investigator.Investigator {
	logger := logr.Discard()
	builder, _ := prompt.NewBuilder()
	rp := parser.NewResultParser()
	enricher := enrichment.NewEnricher(nopK8sClient{}, nopDSClient{}, audit.NopAuditStore{}, logger)
	return investigator.New(investigator.Config{
		Client:        client,
		Builder:       builder,
		ResultParser:  rp,
		Enricher:      enricher,
		AuditStore:    auditStore,
		Logger:        logger,
		MaxTurns:      15,
		PhaseTools:    investigator.DefaultPhaseToolMap(),
		PhaseResolver: fixedPhaseResolver{client: client, runtimeParams: runtimeParams},
	})
}

// streamingRetryTestInvestigatorWithMetrics is a variant of
// streamingRetryTestInvestigator that additionally wires a *metrics.Metrics
// instance, for tests asserting on retry-attempt telemetry (#1612 gap).
func streamingRetryTestInvestigatorWithMetrics(client llm.Client, runtimeParams llm.RuntimeParams, auditStore audit.AuditStore, m *metrics.Metrics) *investigator.Investigator {
	logger := logr.Discard()
	builder, _ := prompt.NewBuilder()
	rp := parser.NewResultParser()
	enricher := enrichment.NewEnricher(nopK8sClient{}, nopDSClient{}, audit.NopAuditStore{}, logger)
	return investigator.New(investigator.Config{
		Client:        client,
		Builder:       builder,
		ResultParser:  rp,
		Enricher:      enricher,
		AuditStore:    auditStore,
		Logger:        logger,
		MaxTurns:      15,
		PhaseTools:    investigator.DefaultPhaseToolMap(),
		PhaseResolver: fixedPhaseResolver{client: client, runtimeParams: runtimeParams},
		Metrics:       m,
	})
}

var streamingRetrySignal = katypes.SignalContext{
	Name:          "test-pod",
	Namespace:     "default",
	Severity:      "critical",
	Message:       "OOMKilled",
	RemediationID: "rem-streaming-retry",
}

// Issue #1612: chatOrStream (the streaming path, used whenever a session
// event sink is present — i.e. every real interactive/production
// investigation) had no retry logic at all. These tests prove the fix:
// retry-with-backoff wired into chatOrStream via llm.RetryWithBackoff,
// side-effect-safe (no retry once a stream callback has already fired
// this attempt), classification-aware (#1585's non-retryable errors fail
// fast), and cancellation-safe (a genuine parent-context cancel still
// produces CancelledResult, never a hard error).
var _ = Describe("Streaming LLM call retry — #1612", func() {

	Describe("UT-KA-1612-004: retries a pre-callback failure and succeeds on the next attempt", func() {
		It("completes the investigation normally after one retryable failure with zero events emitted", func() {
			mock := &scriptedStreamClient{
				errSeq: []error{errors.New("transient network error")},
				responses: []llm.ChatResponse{
					{},
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"pod OOM killed","confidence":0.9}`}},
				},
			}
			runtimeParams := llm.RuntimeParams{MaxRetries: 2, RetryBackoff: streamingRetryFastBackoff}

			// Buffered event sink forces chatOrStream onto the streaming
			// path (#1612's bug only manifests there); events are not
			// drained since this test only cares about the final outcome —
			// emitToSink's non-blocking send means a full/unread buffer
			// never stalls the investigation.
			eventCh := make(chan session.InvestigationEvent, 128)
			ctx := session.WithEventSink(context.Background(), eventCh)

			inv := streamingRetryTestInvestigator(mock, runtimeParams, audit.NopAuditStore{})
			result, err := inv.Investigate(ctx, streamingRetrySignal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RCASummary).NotTo(BeEmpty())
			Expect(mock.callCount()).To(BeNumerically(">=", 2), "must have retried at least once")
		})
	})

	Describe("UT-KA-1612-005: no retry once a stream callback has already fired this attempt", func() {
		It("fails the turn immediately even though the error is retryable and budget remains", func() {
			mock := &scriptedStreamClient{
				chunksSeq: [][]string{{"partial", "delta"}},
				errSeq:    []error{errors.New("transient error mid-stream")},
			}
			runtimeParams := llm.RuntimeParams{MaxRetries: 3, RetryBackoff: streamingRetryFastBackoff}

			eventCh := make(chan session.InvestigationEvent, 128)
			ctx := session.WithEventSink(context.Background(), eventCh)

			inv := streamingRetryTestInvestigator(mock, runtimeParams, audit.NopAuditStore{})
			result, err := inv.Investigate(ctx, streamingRetrySignal)
			close(eventCh)

			Expect(err).To(HaveOccurred(), "must not retry after a partial stream delivery — would duplicate emitted events")
			Expect(result).To(BeNil())
			Expect(mock.callCount()).To(Equal(1))
		})
	})

	Describe("UT-KA-1612-006: retries exhausted with parent context still alive", func() {
		It("returns a generic ResponseFailed-audited error, not a CancelledResult, and emits a distinguishable EventTypeError to the observer", func() {
			mock := &scriptedStreamClient{
				errSeq: []error{
					errors.New("transient error 1"),
					errors.New("transient error 2"),
					errors.New("transient error 3"),
				},
			}
			runtimeParams := llm.RuntimeParams{MaxRetries: 2, RetryBackoff: streamingRetryFastBackoff}
			spy := &cancelTestSpyAuditStore{}

			eventCh := make(chan session.InvestigationEvent, 128)
			ctx := session.WithEventSink(context.Background(), eventCh)

			inv := streamingRetryTestInvestigator(mock, runtimeParams, spy)
			result, err := inv.Investigate(ctx, streamingRetrySignal)
			close(eventCh)

			Expect(err).To(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(mock.callCount()).To(Equal(3), "1 initial attempt + 2 retries, all exhausted")
			Expect(ctx.Err()).NotTo(HaveOccurred(), "parent context must still be alive — this is not a cancellation")

			failedEvents := spy.eventsByType(audit.EventTypeResponseFailed)
			Expect(failedEvents).NotTo(BeEmpty(), "exhausted retries must audit as ResponseFailed, not silently disappear")

			var events []session.InvestigationEvent
			for ev := range eventCh {
				events = append(events, ev)
			}
			var errorEvent *session.InvestigationEvent
			for i := range events {
				if events[i].Type == session.EventTypeError {
					errorEvent = &events[i]
					break
				}
			}
			Expect(errorEvent).NotTo(BeNil(), "an exhausted-retry LLM failure must surface as a distinguishable EventTypeError, not silence until the whole investigation fails (#1612)")
			var data map[string]interface{}
			Expect(json.Unmarshal(errorEvent.Data, &data)).To(Succeed())
			Expect(data).To(HaveKey("error"), "must match the wire shape ka.EventTypeError consumers (API Frontend) already expect")
			Expect(data["error"]).To(ContainSubstring("transient error 3"), "must carry the last (most relevant) underlying error message")
		})
	})

	Describe("UT-KA-1612-007: genuine parent-context cancellation still yields CancelledResult", func() {
		It("aborts via CancelledResult, not an error, when the operator cancels between attempts", func() {
			ctx, cancel := context.WithCancel(context.Background())
			mock := &scriptedStreamClient{
				errSeq:          []error{errors.New("transient error")},
				cancelAfterCall: 1,
				cancelFn:        cancel,
			}
			runtimeParams := llm.RuntimeParams{MaxRetries: 3, RetryBackoff: streamingRetryFastBackoff}

			eventCh := make(chan session.InvestigationEvent, 128)
			ctx = session.WithEventSink(ctx, eventCh)

			inv := streamingRetryTestInvestigator(mock, runtimeParams, audit.NopAuditStore{})
			result, err := inv.Investigate(ctx, streamingRetrySignal)
			close(eventCh)

			Expect(err).NotTo(HaveOccurred(), "cancellation must not surface as an error")
			Expect(result).NotTo(BeNil())
			Expect(result.Cancelled).To(BeTrue())
			Expect(mock.callCount()).To(Equal(1), "must abort during the backoff sleep, not attempt again after cancel")
		})
	})

	Describe("UT-KA-1612-008: MaxRetries=0 makes exactly one attempt", func() {
		It("does not retry a retryable error when the budget is zero", func() {
			mock := &scriptedStreamClient{
				errSeq: []error{errors.New("transient error")},
				responses: []llm.ChatResponse{
					{},
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"would have succeeded","confidence":0.9}`}},
				},
			}
			runtimeParams := llm.RuntimeParams{MaxRetries: 0, RetryBackoff: streamingRetryFastBackoff}

			eventCh := make(chan session.InvestigationEvent, 128)
			ctx := session.WithEventSink(context.Background(), eventCh)

			inv := streamingRetryTestInvestigator(mock, runtimeParams, audit.NopAuditStore{})
			result, err := inv.Investigate(ctx, streamingRetrySignal)
			close(eventCh)

			Expect(err).To(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(mock.callCount()).To(Equal(1))
		})
	})

	Describe("UT-KA-1612-010: retry-attempt telemetry", func() {
		It("records an exhausted-outcome retry metric when a stream call exhausts its retry budget", func() {
			mock := &scriptedStreamClient{
				errSeq: []error{
					errors.New("transient error 1"),
					errors.New("transient error 2"),
				},
			}
			runtimeParams := llm.RuntimeParams{MaxRetries: 1, RetryBackoff: streamingRetryFastBackoff}
			m := metrics.NewMetricsWithRegistry(prometheus.NewRegistry())

			eventCh := make(chan session.InvestigationEvent, 128)
			ctx := session.WithEventSink(context.Background(), eventCh)

			inv := streamingRetryTestInvestigatorWithMetrics(mock, runtimeParams, audit.NopAuditStore{}, m)
			_, err := inv.Investigate(ctx, streamingRetrySignal)
			close(eventCh)

			Expect(err).To(HaveOccurred())
			Expect(mock.callCount()).To(Equal(2), "1 initial attempt + 1 retry, exhausted")
			Expect(testutil.ToFloat64(m.LLMCallRetriesTotal.WithLabelValues("rca", "exhausted"))).To(Equal(float64(1)),
				"a retried-and-exhausted stream call must be observable via Prometheus, not just logs (#1612 gap)")
		})

		It("records a succeeded-outcome retry metric when a retry recovers", func() {
			mock := &scriptedStreamClient{
				errSeq: []error{errors.New("transient error")},
				responses: []llm.ChatResponse{
					{},
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"recovered","confidence":0.9}`}},
				},
			}
			runtimeParams := llm.RuntimeParams{MaxRetries: 1, RetryBackoff: streamingRetryFastBackoff}
			m := metrics.NewMetricsWithRegistry(prometheus.NewRegistry())

			eventCh := make(chan session.InvestigationEvent, 128)
			ctx := session.WithEventSink(context.Background(), eventCh)

			inv := streamingRetryTestInvestigatorWithMetrics(mock, runtimeParams, audit.NopAuditStore{}, m)
			_, err := inv.Investigate(ctx, streamingRetrySignal)
			close(eventCh)

			_ = err
			Expect(mock.callCount()).To(BeNumerically(">=", 2))
			Expect(testutil.ToFloat64(m.LLMCallRetriesTotal.WithLabelValues("rca", "succeeded"))).To(Equal(float64(1)),
				"a retry that recovers must be counted separately from one that exhausts its budget")
		})
	})

	Describe("UT-KA-1612-009: a non-retryable (#1585) error fails fast regardless of remaining budget", func() {
		It("makes exactly one attempt even though no stream events were sent and retries remain", func() {
			mock := &scriptedStreamClient{
				errSeq: []error{llm.MarkNonRetryable(errors.New("401: invalid api key"))},
			}
			runtimeParams := llm.RuntimeParams{MaxRetries: 3, RetryBackoff: streamingRetryFastBackoff}

			eventCh := make(chan session.InvestigationEvent, 128)
			ctx := session.WithEventSink(context.Background(), eventCh)

			inv := streamingRetryTestInvestigator(mock, runtimeParams, audit.NopAuditStore{})
			result, err := inv.Investigate(ctx, streamingRetrySignal)
			close(eventCh)

			Expect(err).To(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(mock.callCount()).To(Equal(1), "non-retryable classification must fail fast regardless of budget or side-effect state")

			var sawErrorEvent bool
			for ev := range eventCh {
				if ev.Type == session.EventTypeError {
					sawErrorEvent = true
				}
			}
			Expect(sawErrorEvent).To(BeTrue(), "a non-retryable failure must also surface as EventTypeError to the observer")
		})
	})
})

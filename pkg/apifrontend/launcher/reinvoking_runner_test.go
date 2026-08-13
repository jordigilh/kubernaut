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
	"iter"
	"strconv"
	"sync"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/adk/agent"
	adksession "google.golang.org/adk/session"
	"google.golang.org/genai"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
)

// scriptedRunnerCall captures the arguments of one Run() invocation against
// scriptedRunner, so tests can assert exactly what reinvokingRunner passed
// through on each turn (in particular, the synthetic continuation message).
type scriptedRunnerCall struct {
	userID    string
	sessionID string
	msg       *genai.Content
}

// scriptedRunner is a fake underlying Runner (the seam reinvokingRunner
// wraps). Each call to Run() appends the next scripted response event to the
// shared session (mirroring how the real *runner.Runner commits non-partial
// events) and yields it, then returns. It has no reinvocation awareness of
// its own — that decision belongs entirely to reinvokingRunner.
type scriptedRunner struct {
	sessionSvc adksession.Service
	responses  []*adksession.Event
	calls      []scriptedRunnerCall
}

func (r *scriptedRunner) Run(ctx context.Context, userID, sessionID string, msg *genai.Content, _ agent.RunConfig) iter.Seq2[*adksession.Event, error] {
	callIdx := len(r.calls)
	r.calls = append(r.calls, scriptedRunnerCall{userID: userID, sessionID: sessionID, msg: msg})
	return func(yield func(*adksession.Event, error) bool) {
		if callIdx >= len(r.responses) {
			return
		}
		resp, err := r.sessionSvc.Get(ctx, &adksession.GetRequest{AppName: "test-app", UserID: userID, SessionID: sessionID})
		if err != nil {
			yield(nil, err)
			return
		}
		event := r.responses[callIdx]
		if appendErr := r.sessionSvc.AppendEvent(ctx, resp.Session, event); appendErr != nil {
			yield(nil, appendErr)
			return
		}
		yield(event, nil)
	}
}

func textOnlyModelEvent(invocationID, text string) *adksession.Event {
	event := adksession.NewEvent(invocationID)
	event.Author = "model"
	event.Content = genai.NewContentFromText(text, genai.RoleModel)
	return event
}

func toolCallModelEvent(invocationID string) *adksession.Event {
	event := adksession.NewEvent(invocationID)
	event.Author = "model"
	event.Content = &genai.Content{
		Role: "model",
		Parts: []*genai.Part{
			{FunctionCall: &genai.FunctionCall{Name: "kubernaut_investigate", Args: map[string]any{}}},
		},
	}
	return event
}

var _ = Describe("reinvokingRunner (BR-SESS-013, issue #1776)", func() {
	It("UT-AF-REINV-002: Run() re-invokes when last event has no tool call (session Active, count < Max)", func() {
		sessionSvc := adksession.InMemoryService()
		createResp, err := sessionSvc.Create(context.Background(), &adksession.CreateRequest{
			AppName: "test-app", UserID: "user-1", SessionID: "sess-1",
		})
		Expect(err).NotTo(HaveOccurred())
		// DD-AF-011 (#1899): reinvocation now also requires an active driver
		// session in state -- this test represents a genuinely stalled
		// mid-investigation continuation, so declare one.
		driverEvt := adksession.NewEvent("setup")
		driverEvt.Actions.StateDelta = map[string]any{session.StateKeyDriverActive: true}
		Expect(sessionSvc.AppendEvent(context.Background(), createResp.Session, driverEvt)).To(Succeed())

		fake := &scriptedRunner{
			sessionSvc: sessionSvc,
			responses: []*adksession.Event{
				textOnlyModelEvent("inv-1", "I need more information."),
				toolCallModelEvent("inv-2"),
			},
		}
		rr := launcher.NewReinvokingRunnerForTest(fake, sessionSvc, "test-app", logr.Discard(), nil)

		var events []*adksession.Event
		for event, runErr := range rr.Run(context.Background(), "user-1", "sess-1", genai.NewContentFromText("investigate", genai.RoleUser), agent.RunConfig{}) {
			Expect(runErr).NotTo(HaveOccurred())
			events = append(events, event)
		}

		Expect(fake.calls).To(HaveLen(2),
			"Run() must re-invoke the inner runner exactly once when the last event has no tool call")
		Expect(fake.calls[1].msg).To(Equal(session.SyntheticMessage()),
			"the reinvocation call must pass the synthetic continuation message, not the original message")
		Expect(events).To(HaveLen(2), "both the original and reinvoked turn's events must be yielded")
	})

	It("UT-AF-REINV-003: Run() does NOT re-invoke when last event has a tool call", func() {
		sessionSvc := adksession.InMemoryService()
		_, err := sessionSvc.Create(context.Background(), &adksession.CreateRequest{
			AppName: "test-app", UserID: "user-1", SessionID: "sess-2",
		})
		Expect(err).NotTo(HaveOccurred())

		fake := &scriptedRunner{
			sessionSvc: sessionSvc,
			responses:  []*adksession.Event{toolCallModelEvent("inv-1")},
		}
		rr := launcher.NewReinvokingRunnerForTest(fake, sessionSvc, "test-app", logr.Discard(), nil)

		var events []*adksession.Event
		for event, runErr := range rr.Run(context.Background(), "user-1", "sess-2", genai.NewContentFromText("investigate", genai.RoleUser), agent.RunConfig{}) {
			Expect(runErr).NotTo(HaveOccurred())
			events = append(events, event)
		}

		Expect(fake.calls).To(HaveLen(1),
			"Run() must NOT re-invoke the inner runner when the last event already contains a tool call")
		Expect(events).To(HaveLen(1))
	})
})

// checkpointRunnerCall records the DD-AF-011 (#1899) checkpoint-flag values
// observed in session state at the moment a given inner Run() call started,
// so tests can prove exactly when (and how often) clearing happened.
type checkpointRunnerCall struct {
	phase2AtEntry any
	phase3AtEntry any
}

// checkpointObservingRunner is a fake inner Runner that records the
// checkpoint-flag state visible at the start of each call, optionally
// applying a scripted mid-call state mutation (sideEffects) before yielding
// its response event -- used to simulate a tool call (e.g. a re-triggered
// kubernaut_investigate) re-blocking a checkpoint within the same genuine
// top-level turn.
type checkpointObservingRunner struct {
	sessionSvc  adksession.Service
	appName     string
	responses   []*adksession.Event
	sideEffects map[int]map[string]any
	calls       []checkpointRunnerCall
}

func (r *checkpointObservingRunner) Run(ctx context.Context, userID, sessionID string, _ *genai.Content, _ agent.RunConfig) iter.Seq2[*adksession.Event, error] {
	callIdx := len(r.calls)
	return func(yield func(*adksession.Event, error) bool) {
		resp, err := r.sessionSvc.Get(ctx, &adksession.GetRequest{AppName: r.appName, UserID: userID, SessionID: sessionID})
		if err != nil {
			yield(nil, err)
			return
		}
		p2, _ := resp.Session.State().Get(session.StateKeyPhase2Blocked)
		p3, _ := resp.Session.State().Get(session.StateKeyPhase3Blocked)
		r.calls = append(r.calls, checkpointRunnerCall{phase2AtEntry: p2, phase3AtEntry: p3})

		if delta, ok := r.sideEffects[callIdx]; ok {
			effect := adksession.NewEvent("side-effect")
			effect.Actions.StateDelta = delta
			if appendErr := r.sessionSvc.AppendEvent(ctx, resp.Session, effect); appendErr != nil {
				yield(nil, appendErr)
				return
			}
		}

		if callIdx >= len(r.responses) {
			return
		}
		refreshed, err := r.sessionSvc.Get(ctx, &adksession.GetRequest{AppName: r.appName, UserID: userID, SessionID: sessionID})
		if err != nil {
			yield(nil, err)
			return
		}
		event := r.responses[callIdx]
		if appendErr := r.sessionSvc.AppendEvent(ctx, refreshed.Session, event); appendErr != nil {
			yield(nil, appendErr)
			return
		}
		yield(event, nil)
	}
}

var _ = Describe("reinvokingRunner checkpoint-flag clearing (DD-AF-011, #1899)", func() {
	const appName = "test-app"

	It("IT-AF-1899-006: a genuine top-level Run() call clears leftover checkpoint flags before invoking the inner runner", func() {
		ctx := context.Background()
		sessionSvc := adksession.InMemoryService()
		createResp, err := sessionSvc.Create(ctx, &adksession.CreateRequest{
			AppName: appName, UserID: "user-1", SessionID: "sess-ckpt-1",
		})
		Expect(err).NotTo(HaveOccurred())

		// Simulate leftover blocked checkpoints from a prior turn (e.g. the
		// user asked a question mid-investigation and never confirmed).
		leftover := adksession.NewEvent("leftover")
		leftover.Actions.StateDelta = map[string]any{
			session.StateKeyPhase2Blocked: true,
			session.StateKeyPhase3Blocked: true,
		}
		Expect(sessionSvc.AppendEvent(ctx, createResp.Session, leftover)).To(Succeed())

		fake := &checkpointObservingRunner{
			sessionSvc: sessionSvc,
			appName:    appName,
			responses:  []*adksession.Event{toolCallModelEvent("inv-1")},
		}
		rr := launcher.NewReinvokingRunnerForTest(fake, sessionSvc, appName, logr.Discard(), nil)

		for _, runErr := range rr.Run(ctx, "user-1", "sess-ckpt-1", genai.NewContentFromText("go ahead", genai.RoleUser), agent.RunConfig{}) {
			Expect(runErr).NotTo(HaveOccurred())
		}

		Expect(fake.calls).To(HaveLen(1))
		Expect(fake.calls[0].phase2AtEntry).To(Equal(false),
			"a genuine top-level user turn must clear af_phase2_blocked before the inner runner is ever invoked")
		Expect(fake.calls[0].phase3AtEntry).To(Equal(false),
			"a genuine top-level user turn must clear af_phase3_blocked before the inner runner is ever invoked")
	})

	It("IT-AF-1899-007: a checkpoint re-blocked mid-turn correctly suppresses reinvocation and is NOT wiped by Run() itself afterward", func() {
		ctx := context.Background()
		sessionSvc := adksession.InMemoryService()
		createResp, err := sessionSvc.Create(ctx, &adksession.CreateRequest{
			AppName: appName, UserID: "user-1", SessionID: "sess-ckpt-2",
		})
		Expect(err).NotTo(HaveOccurred())

		leftover := adksession.NewEvent("leftover")
		leftover.Actions.StateDelta = map[string]any{
			session.StateKeyPhase2Blocked: true,
			session.StateKeyDriverActive:  true,
		}
		Expect(sessionSvc.AppendEvent(ctx, createResp.Session, leftover)).To(Succeed())

		fake := &checkpointObservingRunner{
			sessionSvc: sessionSvc,
			appName:    appName,
			// call 0 re-blocks phase2 mid-turn (as phaseGuardAfter would
			// after an in-turn kubernaut_investigate) and ends with a
			// text-only event (no tool call). Per DD-AF-011 (#1899), a
			// blocked checkpoint must ALSO suppress reinvocation itself
			// (never nudge past a gate the harness just put up) -- so this
			// call is expected to be the ONLY inner call for this turn.
			sideEffects: map[int]map[string]any{
				0: {session.StateKeyPhase2Blocked: true},
			},
			responses: []*adksession.Event{
				textOnlyModelEvent("inv-1", "let me think about that"),
				toolCallModelEvent("inv-2"),
			},
		}
		rr := launcher.NewReinvokingRunnerForTest(fake, sessionSvc, appName, logr.Discard(), nil)

		for _, runErr := range rr.Run(ctx, "user-1", "sess-ckpt-2", genai.NewContentFromText("go ahead", genai.RoleUser), agent.RunConfig{}) {
			Expect(runErr).NotTo(HaveOccurred())
		}

		Expect(fake.calls).To(HaveLen(1),
			"a checkpoint re-blocked mid-turn must suppress reinvocation for the rest of THIS turn -- "+
				"the model must wait for the user, not be nudged again")
		Expect(fake.calls[0].phase2AtEntry).To(Equal(false),
			"the genuine top-level call must see the leftover checkpoint cleared before it runs")

		getResp, err := sessionSvc.Get(ctx, &adksession.GetRequest{AppName: appName, UserID: "user-1", SessionID: "sess-ckpt-2"})
		Expect(err).NotTo(HaveOccurred())
		final, err := getResp.Session.State().Get(session.StateKeyPhase2Blocked)
		Expect(err).NotTo(HaveOccurred())
		Expect(final).To(Equal(true),
			"Run() must NOT clear the checkpoint flag a second time after correctly deciding not to reinvoke -- "+
				"clearCheckpointFlags only runs once, at genuine top-level entry, never mid-loop")
	})
})

// toolResponseEvent builds a tool-call FunctionResponse event, mirroring
// ADK's own base_flow.go convention (errMsg populates the "error" key;
// otherwise the response is a plain success).
func toolResponseEvent(toolName, errMsg string) *adksession.Event {
	resp := map[string]any{}
	if errMsg != "" {
		resp["error"] = errMsg
	} else {
		resp["output"] = "ok"
	}
	event := adksession.NewEvent("inv-cb")
	event.Author = genai.RoleModel
	event.Content = &genai.Content{
		Role: "user",
		Parts: []*genai.Part{
			{FunctionResponse: &genai.FunctionResponse{Name: toolName, Response: resp}},
		},
	}
	return event
}

// unboundedFailureRunner simulates ADK's own base_flow.go Flow.Run internal
// loop getting stuck retrying the same failing tool call with no cap of its
// own (#2078) -- it will yield up to maxFailures consecutive same-tool
// failure events for a single Run() call, tracking in yielded exactly how
// many it actually produced before the consumer (reinvokingRunner.Run) ever
// stopped pulling, so tests can prove the breaker stopped it early rather
// than merely observing a scripted, already-finite sequence complete.
type unboundedFailureRunner struct {
	toolName    string
	maxFailures int
	yielded     int
}

func (r *unboundedFailureRunner) Run(_ context.Context, _, _ string, _ *genai.Content, _ agent.RunConfig) iter.Seq2[*adksession.Event, error] {
	return func(yield func(*adksession.Event, error) bool) {
		for i := 0; i < r.maxFailures; i++ {
			r.yielded++
			if !yield(toolResponseEvent(r.toolName, "schema validation failed"), nil) {
				return
			}
		}
	}
}

// scriptedEventRunner yields a fixed, pre-built sequence of events for a
// single Run() call, tracking how many it actually yielded.
type scriptedEventRunner struct {
	events  []*adksession.Event
	yielded int
}

func (r *scriptedEventRunner) Run(_ context.Context, _, _ string, _ *genai.Content, _ agent.RunConfig) iter.Seq2[*adksession.Event, error] {
	return func(yield func(*adksession.Event, error) bool) {
		for _, ev := range r.events {
			r.yielded++
			if !yield(ev, nil) {
				return
			}
		}
	}
}

// auditSpyEmitter records every audit event emitted, for assertions on
// circuit-breaker-trip audit visibility (IT-AF-2078-003).
type auditSpyEmitter struct {
	mu     sync.Mutex
	events []*audit.Event
}

func (s *auditSpyEmitter) Emit(_ context.Context, event *audit.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

func (s *auditSpyEmitter) eventsByType(t audit.EventType) []*audit.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*audit.Event
	for _, e := range s.events {
		if e.Type == t {
			out = append(out, e)
		}
	}
	return out
}

var _ = Describe("reinvokingRunner tool-retry circuit breaker (#2078, DD-AF-013)", func() {
	It("IT-AF-2078-001: an unbounded stream of same-tool failures is stopped at the threshold, not left to run forever", func() {
		sessionSvc := adksession.InMemoryService()
		_, err := sessionSvc.Create(context.Background(), &adksession.CreateRequest{
			AppName: "test-app", UserID: "user-1", SessionID: "sess-cb-1",
		})
		Expect(err).NotTo(HaveOccurred())

		fake := &unboundedFailureRunner{toolName: "kubernaut_present_decision", maxFailures: 100}
		rr := launcher.NewReinvokingRunnerForTest(fake, sessionSvc, "test-app", logr.Discard(), nil)

		var sawError bool
		for _, runErr := range rr.Run(context.Background(), "user-1", "sess-cb-1", genai.NewContentFromText("investigate", genai.RoleUser), agent.RunConfig{}) {
			if runErr != nil {
				sawError = true
			}
		}

		Expect(fake.yielded).To(Equal(launcher.DefaultToolRetryCircuitBreakerThreshold),
			"the inner runner's iterator must be stopped after exactly the threshold's worth of consecutive failures, not left to keep yielding")
		Expect(sawError).To(BeTrue(), "the turn must end in an error once the circuit breaker trips")
	})

	It("IT-AF-2078-002: failures below the threshold followed by a genuine success do not trip", func() {
		sessionSvc := adksession.InMemoryService()
		_, err := sessionSvc.Create(context.Background(), &adksession.CreateRequest{
			AppName: "test-app", UserID: "user-1", SessionID: "sess-cb-2",
		})
		Expect(err).NotTo(HaveOccurred())

		var events []*adksession.Event
		for i := 0; i < launcher.DefaultToolRetryCircuitBreakerThreshold-1; i++ {
			events = append(events, toolResponseEvent("kubernaut_present_decision", "schema validation failed"))
		}
		events = append(events, toolResponseEvent("kubernaut_present_decision", ""))
		fake := &scriptedEventRunner{events: events}
		rr := launcher.NewReinvokingRunnerForTest(fake, sessionSvc, "test-app", logr.Discard(), nil)

		var sawError bool
		var yieldedCount int
		for _, runErr := range rr.Run(context.Background(), "user-1", "sess-cb-2", genai.NewContentFromText("investigate", genai.RoleUser), agent.RunConfig{}) {
			yieldedCount++
			if runErr != nil {
				sawError = true
			}
		}

		Expect(sawError).To(BeFalse(), "a turn that eventually succeeds within the threshold must not trip the breaker")
		Expect(fake.yielded).To(Equal(len(events)), "every scripted event must have been consumed -- no premature stop")
		Expect(yieldedCount).To(Equal(len(events)))
	})

	It("IT-AF-2078-003: on trip, the configured auditor receives exactly one EventCircuitBreakerTrip event naming the failing tool", func() {
		sessionSvc := adksession.InMemoryService()
		_, err := sessionSvc.Create(context.Background(), &adksession.CreateRequest{
			AppName: "test-app", UserID: "user-1", SessionID: "sess-cb-3",
		})
		Expect(err).NotTo(HaveOccurred())

		spy := &auditSpyEmitter{}
		fake := &unboundedFailureRunner{toolName: "kubernaut_present_decision", maxFailures: 100}
		rr := launcher.NewReinvokingRunnerForTest(fake, sessionSvc, "test-app", logr.Discard(), spy)

		for range rr.Run(context.Background(), "user-1", "sess-cb-3", genai.NewContentFromText("investigate", genai.RoleUser), agent.RunConfig{}) {
		}

		trips := spy.eventsByType(audit.EventCircuitBreakerTrip)
		Expect(trips).To(HaveLen(1), "exactly one circuit-breaker-trip audit event must be emitted")
		Expect(trips[0].Detail["circuit_name"]).To(Equal("kubernaut_present_decision"))
		Expect(trips[0].Detail["failure_count"]).To(Equal(strconv.Itoa(launcher.DefaultToolRetryCircuitBreakerThreshold)))
	})

	It("IT-AF-2078-004: a failing tool interleaved with a different, successful tool does not trip (per-tool isolation, wired end-to-end)", func() {
		sessionSvc := adksession.InMemoryService()
		_, err := sessionSvc.Create(context.Background(), &adksession.CreateRequest{
			AppName: "test-app", UserID: "user-1", SessionID: "sess-cb-4",
		})
		Expect(err).NotTo(HaveOccurred())

		var events []*adksession.Event
		for i := 0; i < launcher.DefaultToolRetryCircuitBreakerThreshold-1; i++ {
			events = append(events,
				toolResponseEvent("kubernaut_present_decision", "schema validation failed"),
				toolResponseEvent("kubernaut_list_workflows", ""),
			)
		}
		fake := &scriptedEventRunner{events: events}
		rr := launcher.NewReinvokingRunnerForTest(fake, sessionSvc, "test-app", logr.Discard(), nil)

		var sawError bool
		for _, runErr := range rr.Run(context.Background(), "user-1", "sess-cb-4", genai.NewContentFromText("investigate", genai.RoleUser), agent.RunConfig{}) {
			if runErr != nil {
				sawError = true
			}
		}

		Expect(sawError).To(BeFalse(),
			"kubernaut_present_decision's failures stay below the threshold on its own; the interleaved successful kubernaut_list_workflows calls must not push it over")
		Expect(fake.yielded).To(Equal(len(events)), "every scripted event must have been consumed -- no premature stop")
	})
})

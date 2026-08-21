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
package launcher

import (
	"context"
	"fmt"
	"iter"
	"slices"
	"strconv"

	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/go-logr/logr"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/plugin"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/adk/v2/server/adka2a"
	adksession "google.golang.org/adk/v2/session"
	"google.golang.org/genai"

	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
)

// reinvokingRunner wraps the real ADK runner to implement BR-SESS-013's
// re-invocation loop *inside* runner.Runner.Run's own iterator, instead of
// after adka2a.Executor has already reported the turn as final (issue
// #1776).
//
// adka2a.Executor.process only calls writeFinalTaskStatus — which publishes
// the "final" TaskStatusUpdateEvent the a2a-go consumer watches for — after
// the Runner's Run iterator is exhausted. By looping inside Run itself,
// reinvocation happens before that final event is ever produced, so the
// a2a-go consumer never observes a premature "final" event and never tears
// the shared context down mid-reinvocation.
type reinvokingRunner struct {
	inner              adka2a.Runner
	sessionService     adksession.Service
	appName            string
	logger             logr.Logger
	auditor            audit.Emitter
	toolRetryThreshold int
}

// newReinvokingRunner constructs a reinvokingRunner around inner (the
// underlying agent-invocation seam). inner is typically a plainRunnerAdapter
// wrapping a real *runner.Runner in production, or a fake in unit tests.
// auditor may be nil (audit emission is best-effort and skipped when unset,
// matching this package's other AuditFunc-style call sites); it is used to
// record SI-4/AU-3 evidence when the #2078 tool-retry circuit breaker trips.
func newReinvokingRunner(inner adka2a.Runner, sessionService adksession.Service, appName string, logger logr.Logger, auditor audit.Emitter) *reinvokingRunner {
	if logger.GetSink() == nil {
		logger = logr.Discard()
	}
	return &reinvokingRunner{
		inner:              inner,
		sessionService:     sessionService,
		appName:            appName,
		logger:             logger,
		auditor:            auditor,
		toolRetryThreshold: DefaultToolRetryCircuitBreakerThreshold,
	}
}

// Run implements adka2a.Runner. It delegates each turn to the wrapped inner
// Runner, then — before yielding control back to the caller — checks whether
// BR-SESS-013 requires a reinvocation (session Active, last event has no
// tool call, under MaxReinvocations). If so, it loops, invoking inner.Run
// again with the synthetic continuation message. The outer iterator does
// not return (and therefore adka2a.Executor does not see a final event)
// until every reinvocation has genuinely finished.
func (r *reinvokingRunner) Run(ctx context.Context, userID, sessionID string, msg *genai.Content, cfg agent.RunConfig) iter.Seq2[*adksession.Event, error] {
	return func(yield func(*adksession.Event, error) bool) {
		// DD-AF-011 (#1899): clear any leftover phase checkpoint flags
		// exactly once, here, at the genuine top-level entry point for a
		// real inbound A2A message. This must NOT run inside the
		// reinvocation loop below (which re-enters r.inner.Run with a
		// synthetic message, not a real user turn) -- doing so would
		// immediately erase a checkpoint the current turn just set.
		r.clearCheckpointFlags(ctx, userID, sessionID)

		currentMsg := msg
		reinvokeCount := 0
		for {
			outcome := r.runOneTurn(ctx, userID, sessionID, currentMsg, cfg, yield)
			if outcome.consumerStopped {
				return
			}
			if outcome.tripped {
				r.handleCircuitBreakerTrip(ctx, sessionID, outcome.trippedTool, yield)
				return
			}
			// Mirrors the previous StreamingExecutor.runReinvocationLoop
			// contract: any error from a turn stops the loop immediately,
			// no further reinvocation attempts.
			if outcome.hadError {
				return
			}

			if !r.needsReinvocation(ctx, userID, sessionID, reinvokeCount) {
				return
			}
			reinvokeCount++
			r.logger.Info("re-invoking agent after text-only turn end",
				"session_id", sessionID,
				"reinvoke_count", reinvokeCount,
			)
			currentMsg = session.SyntheticMessage()
		}
	}
}

// turnOutcome summarizes how a single r.inner.Run() call ended, letting
// Run's reinvocation loop stay a flat sequence of independent checks rather
// than nested state tracking (gocognit).
type turnOutcome struct {
	// consumerStopped means yield itself returned false (the caller of
	// Run stopped pulling); Run must return immediately without any
	// further processing of this turn.
	consumerStopped bool
	hadError        bool
	tripped         bool
	trippedTool     string
}

// runOneTurn drives a single call to r.inner.Run to completion (or until the
// #2078 tool-retry circuit breaker trips, or the consumer stops pulling),
// forwarding every event to yield as it arrives.
func (r *reinvokingRunner) runOneTurn(ctx context.Context, userID, sessionID string, msg *genai.Content, cfg agent.RunConfig, yield func(*adksession.Event, error) bool) turnOutcome {
	// DD-AF-013 (#2078): a fresh breaker per inner.Run() call --
	// google.golang.org/adk's own model-generation loop
	// (internal/llminternal/base_flow.go's Flow.Run) has no retry cap of
	// its own when a tool call repeatedly fails, and can yield an
	// unbounded number of same-tool failure events within this single
	// call. Scoped per-call (not across reinvocations) because a
	// synthetic reinvocation is a fresh model turn, not a continuation of
	// the same stuck tool-call attempt.
	breaker := newToolRetryCircuitBreaker(r.toolRetryThreshold)
	var outcome turnOutcome
	for event, err := range r.inner.Run(ctx, userID, sessionID, msg, cfg) {
		if !yield(event, err) {
			outcome.consumerStopped = true
			return outcome
		}
		if err != nil {
			outcome.hadError = true
		}
		if name, didTrip := breaker.observe(event); didTrip {
			outcome.tripped = true
			outcome.trippedTool = name
			return outcome
		}
	}
	return outcome
}

// handleCircuitBreakerTrip logs, audits, and yields the terminal error for a
// #2078 tool-retry circuit breaker trip.
func (r *reinvokingRunner) handleCircuitBreakerTrip(ctx context.Context, sessionID, trippedTool string, yield func(*adksession.Event, error) bool) {
	r.logger.Error(nil, "tool retry circuit breaker tripped: stopping turn after consecutive same-tool failures",
		"session_id", sessionID,
		"tool_name", trippedTool,
		"threshold", r.toolRetryThreshold,
	)
	r.emitCircuitBreakerTrip(ctx, trippedTool)
	yield(nil, fmt.Errorf("tool retry circuit breaker tripped for tool %q after %d consecutive failures", trippedTool, r.toolRetryThreshold))
}

// needsReinvocation fetches the most recent session event and delegates the
// BR-SESS-013 decision to session.NeedsReinvocationCtx. A session-fetch
// failure is logged (not silently swallowed, per AGENTS.md error-handling
// rules) and treated as "do not re-invoke" -- an unreadable session must
// fail safe rather than spin the loop on stale/absent state.
func (r *reinvokingRunner) needsReinvocation(ctx context.Context, userID, sessionID string, reinvokeCount int) bool {
	resp, getErr := r.sessionService.Get(ctx, &adksession.GetRequest{
		AppName:         r.appName,
		UserID:          userID,
		SessionID:       sessionID,
		NumRecentEvents: 1,
	})
	if getErr != nil {
		r.logger.Error(getErr, "failed to fetch session for reinvocation check, stopping without re-invoking",
			"session_id", sessionID,
		)
		return false
	}
	if resp == nil || resp.Session == nil {
		return false
	}
	return session.NeedsReinvocationCtx(ctx, isv1alpha1.SessionPhaseActive, resp.Session.Events(), resp.Session.State(), reinvokeCount)
}

// emitCircuitBreakerTrip records a SOC2 AU-2/FedRAMP SI-4 audit event for a
// #2078 tool-retry circuit breaker trip. Detail keys ("circuit_name",
// "failure_count") match audit.buildCircuitBreakerTripPayload's expected
// schema so the event serializes through the shared
// apifrontend.circuitbreaker.trip OpenAPI payload, not just a fire-and-forget
// log line -- a no-op when no auditor is configured (nil, best-effort).
func (r *reinvokingRunner) emitCircuitBreakerTrip(ctx context.Context, toolName string) {
	if r.auditor == nil {
		return
	}
	r.auditor.Emit(ctx, &audit.Event{
		Type: audit.EventCircuitBreakerTrip,
		Detail: map[string]string{
			"circuit_name":  toolName,
			"failure_count": strconv.Itoa(r.toolRetryThreshold),
		},
	})
}

// clearCheckpointFlags clears any DD-AF-011 (#1899) phase checkpoint flags
// left over from a prior turn, so a genuine new user message always starts
// with a clean slate for phase-transition consent. Per the empirically
// confirmed spike findings (see the #1899 plan), a direct
// sessionService.Get(...).Session.State().Set(...) call would silently no-op
// against the real ADK InMemoryService -- Get()/Create() return copies, and
// only AppendEvent applying Event.Actions.StateDelta durably persists to the
// canonical stored session. This mirrors exactly what ADK's own base_flow.go
// does automatically inside real tool callbacks; reinvokingRunner sits
// outside any tool callback, so it must do this explicitly.
//
// A session-fetch failure (e.g. first-ever turn, session not yet created) is
// logged and treated as "nothing to clear" -- fail safe, never block the
// turn on this best-effort cleanup.
func (r *reinvokingRunner) clearCheckpointFlags(ctx context.Context, userID, sessionID string) {
	resp, getErr := r.sessionService.Get(ctx, &adksession.GetRequest{
		AppName:   r.appName,
		UserID:    userID,
		SessionID: sessionID,
	})
	if getErr != nil || resp == nil || resp.Session == nil {
		return
	}

	state := resp.Session.State()
	phase2, _ := state.Get(session.StateKeyPhase2Blocked)
	phase3, _ := state.Get(session.StateKeyPhase3Blocked)
	if phase2 != true && phase3 != true {
		return
	}

	event := adksession.NewEvent(ctx, "checkpoint-clear")
	event.Actions.StateDelta = map[string]any{
		session.StateKeyPhase2Blocked: false,
		session.StateKeyPhase3Blocked: false,
	}
	if appendErr := r.sessionService.AppendEvent(ctx, resp.Session, event); appendErr != nil {
		r.logger.Error(appendErr, "failed to clear checkpoint flags for new user turn",
			"session_id", sessionID,
		)
	}
}

// plainRunnerAdapter adapts a real *runner.Runner (whose Run method has an
// extra variadic RunOption parameter) to the plain adka2a.Runner interface
// that reinvokingRunner wraps. Mirrors ADK's own defaultRunner in
// server/adka2a/v2/executor.go.
type plainRunnerAdapter struct {
	runner *runner.Runner
}

func (a plainRunnerAdapter) Run(ctx context.Context, userID, sessionID string, msg *genai.Content, cfg agent.RunConfig) iter.Seq2[*adksession.Event, error] {
	return a.runner.Run(ctx, userID, sessionID, msg, cfg)
}

// newReinvokingRunnerProvider builds an adka2a.RunnerProvider that produces a
// reinvokingRunner instead of ADK's default pass-through runner. Mirrors
// ADK's own newDefaultRunnerProvider (server/adka2a/v2/executor.go): appends
// the executor's a2a-bridging plugin to baseConfig's plugin list (required
// for AfterEventCallback/ExecutorContext to work) and constructs a real
// *runner.Runner from it, then wraps that runner instead of returning it
// directly. auditor may be nil; it is threaded through to reinvokingRunner
// for #2078 tool-retry circuit breaker trip audit events.
func newReinvokingRunnerProvider(baseConfig runner.Config, logger logr.Logger, auditor audit.Emitter) adka2a.RunnerProvider {
	return func(_ context.Context, _ *a2asrv.RequestContext, p *plugin.Plugin) (adka2a.RunnerConfig, adka2a.Runner, error) {
		if baseConfig.Agent == nil {
			return adka2a.RunnerConfig{}, nil, fmt.Errorf("runner.Config.Agent is not provided")
		}
		if baseConfig.SessionService == nil {
			return adka2a.RunnerConfig{}, nil, fmt.Errorf("runner.Config.SessionService is not provided")
		}

		cfg := baseConfig
		cfg.PluginConfig.Plugins = append(slices.Clone(cfg.PluginConfig.Plugins), p)
		realRunner, err := runner.New(cfg)
		if err != nil {
			return adka2a.RunnerConfig{}, nil, err
		}

		runnerConfig := adka2a.RunnerConfig{AppName: cfg.AppName, Agent: cfg.Agent, SessionService: cfg.SessionService}
		wrapped := newReinvokingRunner(plainRunnerAdapter{runner: realRunner}, cfg.SessionService, cfg.AppName, logger, auditor)
		return runnerConfig, wrapped, nil
	}
}

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

	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/go-logr/logr"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/plugin"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/server/adka2a"
	adksession "google.golang.org/adk/session"
	"google.golang.org/genai"

	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
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
	inner          adka2a.Runner
	sessionService adksession.Service
	appName        string
	logger         logr.Logger
}

// newReinvokingRunner constructs a reinvokingRunner around inner (the
// underlying agent-invocation seam). inner is typically a plainRunnerAdapter
// wrapping a real *runner.Runner in production, or a fake in unit tests.
func newReinvokingRunner(inner adka2a.Runner, sessionService adksession.Service, appName string, logger logr.Logger) *reinvokingRunner {
	if logger.GetSink() == nil {
		logger = logr.Discard()
	}
	return &reinvokingRunner{inner: inner, sessionService: sessionService, appName: appName, logger: logger}
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
		currentMsg := msg
		reinvokeCount := 0
		for {
			hadError := false
			for event, err := range r.inner.Run(ctx, userID, sessionID, currentMsg, cfg) {
				if !yield(event, err) {
					return
				}
				if err != nil {
					hadError = true
				}
			}
			// Mirrors the previous StreamingExecutor.runReinvocationLoop
			// contract: any error from a turn stops the loop immediately,
			// no further reinvocation attempts.
			if hadError {
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
	return session.NeedsReinvocationCtx(ctx, isv1alpha1.SessionPhaseActive, resp.Session.Events(), reinvokeCount)
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
// directly.
func newReinvokingRunnerProvider(baseConfig runner.Config, logger logr.Logger) adka2a.RunnerProvider {
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
		wrapped := newReinvokingRunner(plainRunnerAdapter{runner: realRunner}, cfg.SessionService, cfg.AppName, logger)
		return runnerConfig, wrapped, nil
	}
}

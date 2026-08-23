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
	"errors"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// reconAutoMgr extends interactiveAutoMgr with configurable RCA summary lookup.
type reconAutoMgr struct {
	interactiveAutoMgr
	rcaSummary string
	rcaOK      bool
}

func (m *reconAutoMgr) GetLatestRCASummaryByRemediationID(_ string) (string, bool) {
	return m.rcaSummary, m.rcaOK
}
func (m *reconAutoMgr) GetLatestRCAResultByRemediationID(_ string) (*katypes.InvestigationResult, bool) {
	return nil, false
}

// messagesCapturingInvestigatorRunner records LLM messages passed to RunInteractiveTurn.
type messagesCapturingInvestigatorRunner struct {
	response         string
	capturedMessages []mcptools.LLMMessage
}

func (r *messagesCapturingInvestigatorRunner) RunInteractiveTurn(_ context.Context, messages []mcptools.LLMMessage, _ string) (string, error) {
	r.capturedMessages = messages
	return r.response, nil
}

func (r *messagesCapturingInvestigatorRunner) RunRCAExtraction(_ context.Context, _ []mcptools.LLMMessage, _ string) (*katypes.InvestigationResult, error) {
	return &katypes.InvestigationResult{RCASummary: "mock RCA"}, nil
}

func (r *messagesCapturingInvestigatorRunner) RunWorkflowDiscovery(_ context.Context, _ katypes.SignalContext, _ *katypes.InvestigationResult, _ *prompt.EnrichmentData, _ string) (*katypes.InvestigationResult, error) {
	return &katypes.InvestigationResult{RCASummary: "mock RCA"}, nil
}

func (r *messagesCapturingInvestigatorRunner) RunFullInvestigation(_ context.Context, _ katypes.SignalContext) (*katypes.InvestigationResult, error) {
	return &katypes.InvestigationResult{RCASummary: "mock RCA"}, nil
}

func newReconSessionMgr(rrID, sessionID string) *mockSessionManager {
	sess := &mcpinternal.InteractiveSession{
		SessionID:     sessionID,
		CorrelationID: rrID,
		ActingUser:    mcpinternal.UserInfo{Username: "alice"},
	}
	return &mockSessionManager{
		takeoverSession: sess,
		isActive:        true,
		getDriverResult: sess,
	}
}

var _ = Describe("BR-INTERACTIVE-010: Context reconstruction from audit trail", func() {

	Describe("UT-KA-1293-008: RCA summary available → single turn", func() {
		It("should store one assistant turn from RCA summary without calling Reconstruct", func() {
			const rrID = "rr-recon-008"
			sessionMgr := newReconSessionMgr(rrID, "sess-recon-008")
			autoMgr := &reconAutoMgr{
				rcaSummary: "Pod OOMKilled due to memory limit",
				rcaOK:      true,
			}
			recon := &mockContextReconstructor{
				turns: []mcpinternal.ConversationTurn{
					{Role: "user", Content: "should not be used"},
				},
			}
			runner := &messagesCapturingInvestigatorRunner{response: "continuing investigation"}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr)
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   rrID,
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("started"))
			Expect(recon.reconstructCalls.Load()).To(Equal(int32(0)),
				"Reconstruct must not be called when RCA summary is available")

			_, err = tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:    rrID,
				Action:  mcptools.ActionMessage,
				Message: "continue",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(runner.capturedMessages).To(HaveLen(2),
				"one RCA assistant turn plus current user message")
			Expect(runner.capturedMessages[0].Role).To(Equal("assistant"))
			Expect(runner.capturedMessages[0].Content).To(Equal(
				"Previous investigation RCA summary: Pod OOMKilled due to memory limit"))
			Expect(runner.capturedMessages[1].Role).To(Equal("user"))
			Expect(runner.capturedMessages[1].Content).To(Equal("continue"))
		})
	})

	Describe("UT-KA-1293-009: no RCA, DS returns turns → multiple turns stored", func() {
		It("should store reconstructed turns from DS when no RCA summary exists", func() {
			const rrID = "rr-recon-009"
			sessionMgr := newReconSessionMgr(rrID, "sess-recon-009")
			autoMgr := &interactiveAutoMgr{}
			recon := &mockContextReconstructor{
				turns: []mcpinternal.ConversationTurn{
					{Role: "user", Content: "first question"},
					{Role: "assistant", Content: "first answer"},
				},
			}
			runner := &messagesCapturingInvestigatorRunner{response: "follow-up response"}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr)
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   rrID,
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("started"))
			Expect(recon.reconstructCalls.Load()).To(Equal(int32(1)),
				"Reconstruct must be called when no RCA summary is available")

			_, err = tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:    rrID,
				Action:  mcptools.ActionMessage,
				Message: "next step",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(runner.capturedMessages).To(HaveLen(3),
				"two reconstructed turns plus current user message")
			Expect(runner.capturedMessages[0]).To(Equal(mcptools.LLMMessage{
				Role: "user", Content: "first question",
			}))
			Expect(runner.capturedMessages[1]).To(Equal(mcptools.LLMMessage{
				Role: "assistant", Content: "first answer",
			}))
			Expect(runner.capturedMessages[2]).To(Equal(mcptools.LLMMessage{
				Role: "user", Content: "next step",
			}))
		})
	})

	Describe("UT-KA-1293-010: no RCA, DS error → empty context", func() {
		It("should proceed with empty context when DS reconstruction fails", func() {
			const rrID = "rr-recon-010"
			sessionMgr := newReconSessionMgr(rrID, "sess-recon-010")
			autoMgr := &interactiveAutoMgr{}
			recon := &mockContextReconstructor{
				err: errors.New("DS unavailable"),
			}
			runner := &messagesCapturingInvestigatorRunner{response: "fresh start response"}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr)
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   rrID,
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("started"))
			Expect(recon.reconstructCalls.Load()).To(Equal(int32(1)),
				"Reconstruct should still be attempted when no RCA summary exists")

			_, err = tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:    rrID,
				Action:  mcptools.ActionMessage,
				Message: "start fresh",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(runner.capturedMessages).To(HaveLen(1),
				"no prior context should be prepended when reconstruction fails")
			Expect(runner.capturedMessages[0]).To(Equal(mcptools.LLMMessage{
				Role: "user", Content: "start fresh",
			}))
		})
	})
})

var _ = Describe("BR-INTERACTIVE-010 SC-3, #2155: takeover waits for a real completion signal to close the race with a still-finishing autonomous investigation", func() {

	Describe("UT-KA-2155-001: handleTakeover waits for the completion signal before reconstructing context", func() {
		It("should observe the reconstructed turns that land only after the completion channel closes", func() {
			const rrID = "rr-2155-001"
			sessionMgr := newReconSessionMgr(rrID, "sess-2155-001")
			waitCh := make(chan struct{})
			autoMgr := &interactiveAutoMgr{
				findResult: "auto-2155-001",
				findOK:     true,
				waitCh:     waitCh,
			}
			recon := &mockContextReconstructor{
				turns: []mcpinternal.ConversationTurn{
					{Role: "assistant", Content: "prior RCA landed just before the signal closed"},
				},
			}
			runner := &messagesCapturingInvestigatorRunner{response: "n/a"}
			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr)

			// Close the signal a moment after the takeover call starts waiting on
			// it, simulating the investigation goroutine's deferred close(done)
			// landing its result just after handleTakeover began waiting.
			go func() {
				time.Sleep(20 * time.Millisecond)
				close(waitCh)
			}()

			start := time.Now()
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   rrID,
				Action: mcptools.ActionTakeover,
			}, mcpinternal.UserInfo{Username: "alice"})
			elapsed := time.Since(start)

			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("takeover_started"))
			Expect(out.Response).To(Equal("1 prior turns reconstructed"),
				"#2155: handleTakeover must wait for the real completion signal instead of "+
					"reading context before the investigation goroutine has finished")
			Expect(elapsed).To(BeNumerically(">=", 20*time.Millisecond),
				"the takeover call must actually have waited for the signal to close")
		})
	})

	Describe("UT-KA-2155-002: handleTakeover proceeds immediately when there is nothing to wait for", func() {
		It("should not add any latency tax when WaitForCompletionByRemediationID is already closed", func() {
			const rrID = "rr-2155-002"
			sessionMgr := newReconSessionMgr(rrID, "sess-2155-002")
			autoMgr := &interactiveAutoMgr{
				findResult: "auto-2155-002",
				findOK:     true,
				// waitCh left nil: WaitForCompletionByRemediationID returns an
				// already-closed channel, as it would for a genuinely-new
				// takeover with no prior investigation still finishing.
			}
			recon := &mockContextReconstructor{}
			runner := &messagesCapturingInvestigatorRunner{response: "n/a"}
			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr)

			start := time.Now()
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   rrID,
				Action: mcptools.ActionTakeover,
			}, mcpinternal.UserInfo{Username: "alice"})
			elapsed := time.Since(start)

			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("takeover_started"))
			Expect(elapsed).To(BeNumerically("<", 50*time.Millisecond),
				"#2155: a genuinely-new takeover with nothing to wait for must not pay any "+
					"arbitrary latency tax (the old design taxed every such call up to 400ms)")
		})
	})

	Describe("UT-KA-2155-003: handleTakeover's wait aborts promptly when the request context is cancelled", func() {
		It("should return without waiting for the signal once ctx is cancelled", func() {
			const rrID = "rr-2155-003"
			sessionMgr := newReconSessionMgr(rrID, "sess-2155-003")
			autoMgr := &interactiveAutoMgr{
				findResult: "auto-2155-003",
				findOK:     true,
				waitCh:     make(chan struct{}), // never closes
			}
			recon := &mockContextReconstructor{}
			runner := &messagesCapturingInvestigatorRunner{response: "n/a"}
			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr)

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			start := time.Now()
			out, err := tool.Handle(ctx, mcptools.InvestigateInput{
				RRID:   rrID,
				Action: mcptools.ActionTakeover,
			}, mcpinternal.UserInfo{Username: "alice"})
			elapsed := time.Since(start)

			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("takeover_started"))
			Expect(elapsed).To(BeNumerically("<", 100*time.Millisecond),
				"BR-KA-267/#1949: an already-cancelled context (e.g. inactivity timeout) must abort "+
					"the wait immediately rather than blocking on a signal that may never close")
		})
	})
})

// selfHealAutoMgr simulates the #2259 self-heal scenario: no cached RCA and
// no in-flight/pending session initially (so resolveRCAForDiscovery's dead-end
// branch fires), but -- unlike fallbackAutoMgr's StartInvestigation, which
// only records the call for inspection -- this mock's StartInvestigation
// actually executes the given InvestigateFunc and caches its result, so a
// subsequent GetLatestRCAResultByRemediationID call can observe it, letting
// tests assert on the self-heal path end-to-end without a real
// session.Manager.
type selfHealAutoMgr struct {
	fallbackAutoMgr

	// waitCh, if set, is returned by WaitForCompletionByRemediationID
	// instead of an already-closed channel, letting tests simulate a
	// still-running self-heal investigation the wait must block on.
	waitCh <-chan struct{}

	// runningRRID, if set, makes FindByRemediationID report an in-flight
	// investigation for that rr_id, exercising the idempotency guard
	// (UT-KA-2259-002): triggerFreshInvestigationForDiscovery must not call
	// StartInvestigation again when one is already running.
	runningRRID string

	mu       sync.Mutex
	result   *katypes.InvestigationResult
	resultOK bool
}

func (m *selfHealAutoMgr) FindByRemediationID(rrID string) (string, bool) {
	if m.runningRRID != "" && m.runningRRID == rrID {
		return "auto-running", true
	}
	return "", false
}

func (m *selfHealAutoMgr) StartInvestigation(ctx context.Context, fn session.InvestigateFunc, metadata map[string]string) (string, error) {
	_, _ = m.fallbackAutoMgr.StartInvestigation(ctx, fn, metadata)
	result, _ := fn(ctx)
	m.mu.Lock()
	m.result = result
	m.resultOK = true
	m.mu.Unlock()
	return "self-heal-session", nil
}

func (m *selfHealAutoMgr) GetLatestRCAResultByRemediationID(_ string) (*katypes.InvestigationResult, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.result, m.resultOK
}

// setResult presets the RCA result as if already produced by the in-flight
// investigation UT-KA-2259-002 simulates via runningRRID, so the test can
// assert that triggerFreshInvestigationForDiscovery reuses it instead of
// calling StartInvestigation again.
func (m *selfHealAutoMgr) setResult(result *katypes.InvestigationResult) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.result = result
	m.resultOK = true
}

func (m *selfHealAutoMgr) WaitForCompletionByRemediationID(_ string) <-chan struct{} {
	if m.waitCh != nil {
		return m.waitCh
	}
	return mcptools.ClosedChan()
}

var _ = Describe("BR-INTERACTIVE-010 SC-5, #2259: discover_workflows self-heals a takeover that raced ahead of the first LLM call", func() {

	Describe("UT-KA-2259-001: dead end (no cached RCA, no reconstructable turns, no in-flight investigation) triggers a fresh investigation", func() {
		It("should self-heal via a fresh investigation and return workflows_discovered instead of failing permanently", func() {
			const rrID = "rr-2259-001"
			sess := &mcpinternal.InteractiveSession{
				SessionID:     "sess-2259-001",
				CorrelationID: rrID,
				ActingUser:    mcpinternal.UserInfo{Username: "alice"},
			}
			sessionMgr := &mockSessionManager{isActive: true, getDriverResult: sess}
			autoMgr := &selfHealAutoMgr{}
			recon := &mockContextReconstructor{} // no reconstructable turns
			runner := &mockInvestigatorRunner{
				rcaResult: &katypes.InvestigationResult{
					RCASummary: "self-healed: OOMKilled on api-server",
					Confidence: 0.88,
				},
				workflowDiscoveryResult: &katypes.InvestigationResult{
					RCASummary: "self-healed: OOMKilled on api-server",
					WorkflowID: "restart-pod-v1",
					Confidence: 0.88,
				},
			}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr,
				mcptools.WithSignalContextResolver(&mockSignalResolver{}),
				mcptools.WithWorkflowCatalog(&mockWorkflowCatalog{
					workflow: &mcptools.CatalogWorkflow{WorkflowID: "restart-pod-v1", WorkflowName: "Restart Pod"},
				}))

			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   rrID,
				Action: mcptools.ActionDiscoverWorkflows,
			}, mcpinternal.UserInfo{Username: "alice"})

			Expect(err).NotTo(HaveOccurred(),
				"#2259: a genuine dead end (no RCA, no reconstructable audit trail) must self-heal, not fail permanently")
			Expect(out.Status).To(Equal("workflows_discovered"))
			Expect(autoMgr.startCalled.Load()).To(Equal(int32(1)),
				"a fresh investigation must be started via the existing createFreshInteractiveSession pipeline")
			Expect(autoMgr.capturedMode()).To(Equal("interactive_fresh_start"))
		})
	})

	Describe("UT-KA-2259-002: an already-in-flight self-heal investigation is reused instead of duplicated", func() {
		It("should wait on the existing investigation and reuse its result without calling StartInvestigation again", func() {
			const rrID = "rr-2259-002"
			sess := &mcpinternal.InteractiveSession{
				SessionID:     "sess-2259-002",
				CorrelationID: rrID,
				ActingUser:    mcpinternal.UserInfo{Username: "alice"},
			}
			sessionMgr := &mockSessionManager{isActive: true, getDriverResult: sess}
			waitCh := make(chan struct{})
			autoMgr := &selfHealAutoMgr{
				runningRRID: rrID, // simulates a self-heal investigation already in flight
				waitCh:      waitCh,
			}
			recon := &mockContextReconstructor{}
			runner := &mockInvestigatorRunner{}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr,
				mcptools.WithSignalContextResolver(&mockSignalResolver{}),
				mcptools.WithWorkflowCatalog(&mockWorkflowCatalog{
					workflow: &mcptools.CatalogWorkflow{WorkflowID: "already-running-wf", WorkflowName: "Already Running"},
				}))

			go func() {
				time.Sleep(20 * time.Millisecond)
				autoMgr.setResult(&katypes.InvestigationResult{
					RCASummary: "produced by the already-in-flight investigation",
					WorkflowID: "already-running-wf",
					Confidence: 0.7,
				})
				close(waitCh)
			}()

			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   rrID,
				Action: mcptools.ActionDiscoverWorkflows,
			}, mcpinternal.UserInfo{Username: "alice"})

			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("workflows_discovered"))
			Expect(autoMgr.startCalled.Load()).To(Equal(int32(0)),
				"#2259: must not launch a duplicate self-heal investigation when one is already in flight for this rr_id")
		})
	})

	Describe("UT-KA-2259-003: ctx cancelled while a self-heal investigation is still in flight returns a retriable error", func() {
		It("should return promptly with a distinguishable, retriable error instead of the old permanent-dead-end wording", func() {
			const rrID = "rr-2259-003"
			sess := &mcpinternal.InteractiveSession{
				SessionID:     "sess-2259-003",
				CorrelationID: rrID,
				ActingUser:    mcpinternal.UserInfo{Username: "alice"},
			}
			sessionMgr := &mockSessionManager{isActive: true, getDriverResult: sess}
			autoMgr := &selfHealAutoMgr{
				runningRRID: rrID,
				waitCh:      make(chan struct{}), // never closes
			}
			recon := &mockContextReconstructor{}
			runner := &mockInvestigatorRunner{}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr,
				mcptools.WithSignalContextResolver(&mockSignalResolver{}),
				mcptools.WithWorkflowCatalog(&mockWorkflowCatalog{}))

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			start := time.Now()
			_, err := tool.Handle(ctx, mcptools.InvestigateInput{
				RRID:   rrID,
				Action: mcptools.ActionDiscoverWorkflows,
			}, mcpinternal.UserInfo{Username: "alice"})
			elapsed := time.Since(start)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("in progress"),
				"#2259: the error must signal the investigation is still in progress and retriable")
			Expect(err.Error()).NotTo(ContainSubstring("no conversation context available"),
				"#2259: must not resurface the old wording that implied a permanent dead end")
			Expect(elapsed).To(BeNumerically("<", 100*time.Millisecond),
				"an already-cancelled context must abort the wait immediately, not block indefinitely")
		})
	})
})

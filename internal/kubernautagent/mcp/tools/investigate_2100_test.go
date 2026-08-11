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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
)

// #2100: QE's live evidence showed interactive.maxConcurrentSessions
// capacity eroding monotonically under load. One of the two converging
// root causes: handleStart's fallback-exhausted branch (no autonomous
// session for the RR, reattachOrCreateFallback's own fallback session
// creation fails, AND ForceTransitionToUserDriving also fails) silently
// fell through to a "started" response with an empty
// InvestigationSessionID -- holding the just-acquired Lease with nothing
// behind it (matches session_teardown.go's "CRITICAL: no session found for
// either completion path" log line). The only reclaim path was
// TimeoutManager's ~10-minute inactivity timeout, which is why live
// evidence showed leases lingering minutes rather than forever, not a
// designed safety net for this specific failure.
var _ = Describe("Fix #2100: handleStart fail-closed on fallback exhaustion", func() {

	Describe("UT-KA-2100-003: handleStart releases the lease and fails closed when every fallback path is exhausted", func() {
		It("should call Release and return ErrCodeNoInvestigationAvailable instead of silently succeeding with an empty InvestigationSessionID", func() {
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "lease-sess-2100-003",
					CorrelationID: "rr-2100-003",
				},
			}
			autoMgr := &fallbackAutoMgr{
				findOK:   false, // no autonomous session at all for this RR
				startErr: fmt.Errorf("simulated StartInvestigation failure"), // reattachOrCreateFallback -> ""
				forceErr: fmt.Errorf("simulated ForceTransitionToUserDriving failure"),
			}
			completer := &reattachHTTPCompleter{found: false} // no existing user_driving session either
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr, mcptools.WithHTTPCompleter(completer))
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-2100-003",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "sre-erin", Groups: []string{"sre-team"}})

			Expect(err).To(HaveOccurred(), "#2100: must fail closed instead of returning a misleading success")
			var mcpErr *mcptools.MCPError
			Expect(errors.As(err, &mcpErr)).To(BeTrue())
			Expect(mcpErr.Code).To(Equal("no_investigation_available"))
			Expect(out).To(Equal(mcptools.InvestigateOutput{}))

			releasedID, releasedReason := sessionMgr.getReleased()
			Expect(releasedID).To(Equal("lease-sess-2100-003"),
				"#2100: the just-acquired lease must be released immediately, not left to the janitor/TimeoutManager backstops")
			Expect(releasedReason).To(Equal("no_investigation_available"))
		})
	})

	Describe("UT-KA-2100-004 (regression guard): the terminal-session fallback branch is unaffected by the new fail-closed behavior", func() {
		It("should still fall back to investigationSessionID = autoSessionID when reattachOrCreateFallback fails after UpgradeToInteractive reports terminal", func() {
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "lease-sess-2100-004",
					CorrelationID: "rr-2100-004",
				},
			}
			autoMgr := &fallbackAutoMgr{
				findResult: "old-terminal-session-004",
				findOK:     true,
				upgradeErr: session.ErrSessionTerminal,
				startErr:   fmt.Errorf("simulated StartInvestigation failure"), // reattachOrCreateFallback -> ""
			}
			completer := &reattachHTTPCompleter{found: false}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr, mcptools.WithHTTPCompleter(completer))
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-2100-004",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "sre-frank", Groups: []string{"sre-team"}})

			Expect(err).NotTo(HaveOccurred())
			Expect(out.InvestigationSessionID).To(Equal("old-terminal-session-004"),
				"the terminal-session branch's own pre-existing autoSessionID fallback must remain unchanged by #2100's new fail-closed logic, "+
					"which is scoped only to the genuinely-empty (no autonomous session at all) case")

			releasedID, _ := sessionMgr.getReleased()
			Expect(releasedID).To(BeEmpty(),
				"the terminal-session branch must not release the lease -- it already has a valid (if stale) session to attach to")
		})
	})
})

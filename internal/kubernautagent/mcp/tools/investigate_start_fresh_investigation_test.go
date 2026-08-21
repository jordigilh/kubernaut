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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
)

// DD-AA-KA-001 Amendment Gap 3 revision (PR #2189 CI evidence): Gap 3, as
// originally implemented (#2170), failed closed whenever no autonomous
// session and no completed RCA existed for an RR, on the assumption the
// only alternative was a hardcoded, disconnected placeholder. CI runs of
// PR #2189 proved that assumption wrong: test/e2e/fullpipeline's
// "FP-MCP-002: AF-style fresh start lifecycle" (mirrored in the
// apifrontend/fleet/kubernautagent E2E suites) is a legitimate, explicitly
// named product journey -- a user interactively investigating a
// RemediationRequest AA has not (or will never) pick up autonomously.
//
// These tests prove handleStart now starts a genuinely real investigation
// (createFreshInteractiveSession, reusing the same signal-resolution +
// RunFullInvestigation pipeline handleStartAutonomous uses) instead of
// failing closed, whenever the signal-resolution dependency is available --
// and that it still fails closed defensively when that dependency is
// missing (a wiring-configuration problem, not a legitimate "genuinely no
// investigation" business scenario).
var _ = Describe("DD-AA-KA-001 Amendment Gap 3 revision: real fresh investigation instead of fail-closed", func() {

	Describe("UT-KA-2170-020: handleStart starts a real investigation when no session/RCA exists but signal resolution is available", func() {
		It("should succeed with a real, upgraded-to-interactive session instead of failing closed", func() {
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "lease-sess-2170-020",
					CorrelationID: "rr-fresh-start-020",
				},
			}
			autoMgr := &fallbackAutoMgr{
				findOK:      false, // no autonomous session at all for this RR
				startResult: "fresh-real-investigation-020",
			}
			completer := &reattachHTTPCompleter{found: false} // no existing user_driving session either
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr,
				mcptools.WithHTTPCompleter(completer),
				mcptools.WithSignalContextResolver(&mockSignalResolver{}))
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-fresh-start-020",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "sre-erin", Groups: []string{"sre-team"}})

			Expect(err).NotTo(HaveOccurred(),
				"a genuinely fresh RR with signal resolution available must start a real investigation, not fail closed")
			Expect(out.Status).To(Equal("started"))
			Expect(out.InvestigationSessionID).To(Equal("fresh-real-investigation-020"))

			Expect(autoMgr.startCalled.Load()).To(Equal(int32(1)),
				"StartInvestigation must be called to run a real RunFullInvestigation-backed session")
			Expect(autoMgr.capturedMode()).To(Equal("interactive_fresh_start"))
			Expect(autoMgr.upgradeCalled.Load()).To(Equal(int32(1)),
				"the fresh session must be immediately marked for interactive jump-in via UpgradeToInteractive")

			releasedID, _ := sessionMgr.getReleased()
			Expect(releasedID).To(BeEmpty(),
				"the lease must not be released -- a real investigation now backs it")
		})
	})

	Describe("UT-KA-2170-021 (defensive): handleStart still fails closed when the signal-resolution dependency itself is unavailable", func() {
		It("should release the lease and return ErrCodeNoInvestigationAvailable without calling StartInvestigation", func() {
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "lease-sess-2170-021",
					CorrelationID: "rr-no-resolver-021",
				},
			}
			autoMgr := &fallbackAutoMgr{
				findOK:   false,
				forceErr: mcpinternal.ErrSessionNotFound,
			}
			completer := &reattachHTTPCompleter{found: false}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			// No WithSignalContextResolver option: production always wires
			// one (cmd/kubernautagent/routes.go), so this exercises the
			// defensive "dependency not configured" branch only.
			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr,
				mcptools.WithHTTPCompleter(completer))
			_, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-no-resolver-021",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "sre-erin", Groups: []string{"sre-team"}})

			var mcpErr *mcptools.MCPError
			Expect(errors.As(err, &mcpErr)).To(BeTrue())
			Expect(mcpErr.Code).To(Equal(mcptools.ErrCodeNoInvestigationAvailable.Code))
			Expect(autoMgr.startCalled.Load()).To(Equal(int32(0)),
				"StartInvestigation must not be attempted when signal resolution is unavailable")
		})
	})
})

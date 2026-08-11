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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
)

// #2103: aiagent_mcp_interactive_sessions_active's decrement moved from
// InvestigateTool's own explicit RecordInteractiveSessionEnded() calls (in
// handleComplete/handleCancel) into LeaseSessionManager.Release() itself,
// so every Release() caller -- including complete_no_action, workflow
// selection, and the no_matching_workflows auto-close, none of which ever
// called it directly -- decrements the gauge automatically. These tests
// guard against reintroducing a double-decrement: with the shared mock
// SessionManager here (which does NOT itself call metrics, unlike the real
// LeaseSessionManager post-#2103), handleComplete/handleCancel must show
// ZERO direct RecordInteractiveSessionEnded calls.
var _ = Describe("InvestigateTool metrics after centralization — #2103", func() {

	Describe("UT-KA-2103-004 (regression guard): handleComplete no longer directly decrements the gauge", func() {
		It("should not call RecordInteractiveSessionEnded itself -- that responsibility moved to LeaseSessionManager.Release", func() {
			metrics := &recordingToolMetrics{}
			sessionMgr := &mockSessionManager{
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "sess-2103-complete",
					CorrelationID: "rr-2103-complete",
					ActingUser:    mcpinternal.UserInfo{Username: "alice"},
				},
				isActive: true,
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mcptools.NopAutonomousManager{},
				mcptools.WithToolMetrics(metrics))
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-2103-complete",
				Action: mcptools.ActionComplete,
			}, mcpinternal.UserInfo{Username: "alice"})

			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("completed"))
			Expect(sessionMgr.releasedID).To(Equal("sess-2103-complete"),
				"Release must still be called -- only the direct metrics call moved")
			Expect(metrics.sessionEnded).To(Equal(0),
				"#2103: handleComplete must not call RecordInteractiveSessionEnded directly anymore -- "+
					"the real LeaseSessionManager.Release() does it centrally now, and this mock does not "+
					"simulate that, so a nonzero count here would mean a double-decrement in production")
		})
	})

	Describe("UT-KA-2103-005 (regression guard): handleCancel no longer directly decrements the gauge", func() {
		It("should not call RecordInteractiveSessionEnded itself -- same rationale as handleComplete", func() {
			metrics := &recordingToolMetrics{}
			sessionMgr := &mockSessionManager{
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "sess-2103-cancel",
					CorrelationID: "rr-2103-cancel",
					ActingUser:    mcpinternal.UserInfo{Username: "alice"},
				},
				isActive: true,
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mcptools.NopAutonomousManager{},
				mcptools.WithToolMetrics(metrics))
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-2103-cancel",
				Action: mcptools.ActionCancel,
			}, mcpinternal.UserInfo{Username: "alice"})

			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("cancelled"))
			Expect(sessionMgr.releasedReason).To(Equal("explicit"))
			Expect(metrics.sessionEnded).To(Equal(0),
				"#2103: handleCancel must not call RecordInteractiveSessionEnded directly anymore")
		})
	})

	Describe("UT-KA-2103-006: handleStart records Started immediately, even on the fallback-exhausted branch that releases the lease", func() {
		It("should call RecordInteractiveSessionStarted before the #2100 fail-closed Release, keeping the gauge paired with activeCount", func() {
			metrics := &recordingToolMetrics{}
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "lease-sess-2103-006",
					CorrelationID: "rr-2103-006",
				},
			}
			autoMgr := &fallbackAutoMgr{
				findOK:   false,
				startErr: fmt.Errorf("simulated StartInvestigation failure"),
				forceErr: fmt.Errorf("simulated ForceTransitionToUserDriving failure"),
			}
			completer := &reattachHTTPCompleter{found: false}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr,
				mcptools.WithHTTPCompleter(completer), mcptools.WithToolMetrics(metrics))
			_, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-2103-006",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "sre-erin", Groups: []string{"sre-team"}})

			Expect(err).To(HaveOccurred(), "#2100: this branch still fails closed")

			releasedID, releasedReason := sessionMgr.getReleased()
			Expect(releasedID).To(Equal("lease-sess-2103-006"))
			Expect(releasedReason).To(Equal("no_investigation_available"))

			Expect(metrics.sessionStarted).To(Equal(1),
				"#2103: Takeover acquired a genuine new lease (activeCount.Add(1)) before this branch released "+
					"it again -- RecordInteractiveSessionStarted must fire for that lease's brief lifetime so the "+
					"now-centralized Release() decrement (which fires unconditionally) has a matching increment "+
					"to pair with, instead of driving the gauge negative")
		})
	})

	Describe("UT-KA-2103-007 (regression guard): Started is still not recorded when no new lease was acquired", func() {
		It("should not call RecordInteractiveSessionStarted on a reconnect", func() {
			metrics := &recordingToolMetrics{}
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "lease-sess-2103-007a",
					CorrelationID: "rr-2103-007a",
					Reconnected:   true,
				},
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mcptools.NopAutonomousManager{},
				mcptools.WithToolMetrics(metrics))
			_, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-2103-007a",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "sre-erin", Groups: []string{"sre-team"}})

			Expect(err).To(HaveOccurred(), "a reconnect on action=start returns the session_active error")
			Expect(metrics.sessionStarted).To(Equal(0),
				"#2103: a reconnect never acquires a new lease (Takeover returns early, no activeCount.Add(1)), "+
					"so it must not record a Started that has no matching lease lifetime")
		})

		It("should not call RecordInteractiveSessionStarted when Takeover itself fails", func() {
			metrics := &recordingToolMetrics{}
			sessionMgr := &mockSessionManager{
				takeoverErr: mcpinternal.ErrMaxSessionsReached,
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mcptools.NopAutonomousManager{},
				mcptools.WithToolMetrics(metrics))
			_, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-2103-007b",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "sre-erin", Groups: []string{"sre-team"}})

			Expect(err).To(HaveOccurred())
			Expect(metrics.sessionStarted).To(Equal(0),
				"#2103: no lease was ever acquired when Takeover itself fails")
		})
	})
})

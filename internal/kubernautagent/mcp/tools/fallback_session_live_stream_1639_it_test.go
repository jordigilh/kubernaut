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
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
)

// IT-KA-1639-004 exercises the full production wiring path — a real
// session.Manager, not mocks — for the exact scenario E2E-AF-1637-001
// depends on: a user calls kubernaut_investigate on an RR with no prior
// autonomous investigation (the common case, createFallbackSession), then
// drives it with kubernaut_message. Before #1639/#1640, this combination
// NEVER streamed a live event: #1640's metadata key mismatch made the
// fallback session invisible to FindUserDrivingByRemediationID, and #1639's
// missing enrichment meant handleMessage never attached a sink even if it
// had been found. This test proves both fixes together, wired exactly as
// production wires them (InvestigateTool + real session.Manager as both
// autoMgr and httpCompleter).
var _ = Describe("Fresh interactive session live event streaming — #1639 + #1640", func() {

	Describe("IT-KA-1639-004: kubernaut_message on a fallback-origin session attaches the real LazySink", func() {
		It("should resolve the SAME fallback session across start->message and stream once subscribed", func() {
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, logr.Discard(), audit.NopAuditStore{}, nil)

			rrID := "rr-it-1639-004"
			takeoverSess := &mcpinternal.InteractiveSession{
				SessionID:     "mcp-sess-it-1639-004",
				CorrelationID: rrID,
				ActingUser:    mcpinternal.UserInfo{Username: "alice"},
			}
			sessionMgr := &mockSessionManager{
				takeoverSession: takeoverSess,
				isActive:        true,
				getDriverResult: takeoverSess,
			}
			runner := &mockInvestigatorRunner{response: "investigating..."}
			recon := &mockContextReconstructor{}

			// autoMgr and httpCompleter are the SAME real *session.Manager
			// instance, matching production wiring (cmd/kubernautagent/routes.go
			// passes d.autoMgr for both).
			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mgr,
				mcptools.WithHTTPCompleter(mgr))

			By("Step 1: action=start creates a fresh fallback session (no prior autonomous investigation)")
			startOut, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   rrID,
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(startOut.Status).To(Equal("started"))
			Expect(startOut.InvestigationSessionID).NotTo(BeEmpty(),
				"handleStart must create a fallback session since no autonomous investigation exists for this RR")

			By("Step 2: fallback session settles into StatusUserDriving")
			Eventually(func() session.Status {
				s, _ := mgr.GetSession(startOut.InvestigationSessionID)
				if s == nil {
					return ""
				}
				return s.Status
			}, 2*time.Second).Should(Equal(session.StatusUserDriving))

			By("Step 3: action=message must resolve to the SAME fallback session and attach its LazySink")
			_, err = tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:    rrID,
				Action:  mcptools.ActionMessage,
				Message: "continue investigating",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())

			capturedCtx := runner.capturedCtx
			Expect(capturedCtx).NotTo(BeNil())
			Expect(session.SessionIDFromContext(capturedCtx)).To(Equal(startOut.InvestigationSessionID),
				"IT-KA-1639-004: kubernaut_message must resolve to the SAME fallback session kubernaut_investigate created — "+
					"proves #1640's metadata-key fix makes the fallback session discoverable by FindUserDrivingByRemediationID")

			_, foundSink := mgr.GetSessionLazySink(startOut.InvestigationSessionID)
			Expect(foundSink).To(BeTrue())
			Expect(session.EventSinkFromContext(capturedCtx)).To(BeNil(),
				"no subscriber has activated the sink yet — nil until Subscribe() is called, matching production behavior")

			By("Step 4: once a subscriber activates the sink (Subscribe), the NEXT message turn streams live events")
			eventCh, subErr := mgr.Subscribe(context.Background(), startOut.InvestigationSessionID)
			Expect(subErr).NotTo(HaveOccurred())
			Expect(eventCh).NotTo(BeNil())

			_, err = tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:    rrID,
				Action:  mcptools.ActionMessage,
				Message: "any updates?",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())

			sink := session.EventSinkFromContext(runner.capturedCtx)
			Expect(sink).NotTo(BeNil(),
				"IT-KA-1639-004: once subscribed, a kubernaut_message turn must carry an active event sink — "+
					"proving #1639+#1640 together, end-to-end, with a real session.Manager (no mocks for autoMgr/httpCompleter)")
		})
	})
})

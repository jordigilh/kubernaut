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
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// IT-KA-1811-001 exercises the exact production dispatch path for #1811 — the
// real InvestigateTool.Handle (action=start) and InvestigateTool.SubscribeEvents,
// wired to a real *session.Manager as autoMgr (matching cmd/kubernautagent's
// production wiring), not the session package's internal Manager/LazySink API
// directly. This proves the wiring point, complementing session package's
// UT-KA-1811-002/003 which prove the LazySink buffering logic itself.
//
// Scenario: AA submits an investigation non-interactively via
// Manager.StartInvestigation (mirrors AA's real submission path, outside the
// kubernaut_investigate MCP tool). The investigation resolves fast enough
// that AF's kubernaut_investigate action=start call reaches
// upgradeOrCreateInteractiveSession -> autoMgr.UpgradeToInteractive while the
// goroutine is still running -- then the goroutine's InteractiveHold observes
// the upgrade and returns. Before the #1811 fix, every event emitted during
// that window (by session.EmitEvent, matching investigator.emitToSink and
// manager_events.go's emitCompleteEvent) was silently dropped because
// wireInvestigationEventBridge's tool.SubscribeEvents() call — which always
// runs strictly after handleStart returns — activated the LazySink too late.
var _ = Describe("Fix #1811 — Late-Subscribe Event Replay IT (Pyramid Invariant: wiring proof)", func() {

	Describe("IT-KA-1811-001: kubernaut_investigate action=start upgrade-in-place replays buffered events on SubscribeEvents", func() {
		It("delivers RCA-phase events and the terminal complete event through the real InvestigateTool + session.Manager wiring", func() {
			mgr := session.NewManager(session.NewStore(30*time.Minute), logr.Discard(), audit.NopAuditStore{}, nil)

			rrID := "rr-it-1811-001"
			readyForUpgrade := make(chan struct{})
			upgradeApplied := make(chan struct{})
			investigationDone := make(chan struct{})

			// Mirrors AA's real submission path: a non-interactive
			// StartInvestigation call outside the kubernaut_investigate MCP
			// tool entirely (AA does not call this tool).
			autoSessionID, err := mgr.StartInvestigation(context.Background(),
				func(ctx context.Context) (*katypes.InvestigationResult, error) {
					defer close(investigationDone)

					// Mirrors investigator.emitToSink's RCA-phase progress
					// events, emitted before anyone has subscribed.
					session.EmitEvent(ctx, session.InvestigationEvent{
						Type: session.EventTypeReasoningDelta, Turn: 0, Phase: "rca",
					})

					// Mirrors the exact CI-observed ordering: AF's
					// kubernaut_investigate reaches the real
					// upgradeOrCreateInteractiveSession -> UpgradeToInteractive
					// call while this goroutine is still running (status is
					// still StatusRunning, so the upgrade succeeds in-place),
					// and only afterward does the InteractiveHold check below
					// observe the flag and short-circuit.
					close(readyForUpgrade)
					<-upgradeApplied

					return &katypes.InvestigationResult{
						RCASummary:      "CrashLoopBackOff: container exits with code 137",
						InteractiveHold: session.InteractiveUpgradeFromContext(ctx),
					}, nil
				},
				map[string]string{"remediation_id": rrID},
			)
			Expect(err).NotTo(HaveOccurred())

			// Real InvestigateTool wired exactly as production wires it
			// (cmd/kubernautagent passes the same *session.Manager for both
			// autoMgr and the HTTP completer).
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "mcp-sess-it-1811-001",
					CorrelationID: rrID,
					ActingUser:    mcpinternal.UserInfo{Username: "sre-user"},
				},
			}
			tool := mcptools.NewInvestigateTool(sessionMgr, &mockInvestigatorRunner{}, &mockContextReconstructor{}, mgr,
				mcptools.WithHTTPCompleter(mgr))

			By("AF's kubernaut_investigate action=start reaches the real upgrade-in-place path while the goroutine is still mid-RCA-turn")
			Eventually(readyForUpgrade, 2*time.Second).Should(BeClosed())
			startOut, startErr := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   rrID,
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "sre-user"})
			Expect(startErr).NotTo(HaveOccurred())
			Expect(startOut.InvestigationSessionID).To(Equal(autoSessionID),
				"handleStart must upgrade the existing autonomous session in-place, not create a fresh fallback session")
			close(upgradeApplied)

			Eventually(investigationDone, 2*time.Second).Should(BeClosed())
			Eventually(func() session.Status {
				sess, _ := mgr.GetSession(autoSessionID)
				if sess == nil {
					return ""
				}
				return sess.Status
			}, 2*time.Second).Should(Equal(session.StatusUserDriving),
				"InteractiveHold must land the session in StatusUserDriving (non-terminal, channel kept open for a later Subscribe)")

			By("registration.go's wireInvestigationEventBridge subscribes strictly after handleStart returns — by which point the race above has already happened")
			eventCh, subErr := tool.SubscribeEvents(context.Background(), startOut.InvestigationSessionID)
			Expect(subErr).NotTo(HaveOccurred())
			Expect(eventCh).NotTo(BeNil())

			var received []session.InvestigationEvent
			Eventually(func() []session.InvestigationEvent {
				for {
					select {
					case evt := <-eventCh:
						received = append(received, evt)
					default:
						return received
					}
				}
			}, 2*time.Second).Should(HaveLen(2),
				"IT-KA-1811-001: both the buffered RCA event and the terminal complete event must be replayed to the late subscriber")
			Expect(received[0].Type).To(Equal(session.EventTypeReasoningDelta))
			Expect(received[1].Type).To(Equal(session.EventTypeComplete))
		})
	})
})

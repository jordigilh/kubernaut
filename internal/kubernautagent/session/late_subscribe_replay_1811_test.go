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

package session_test

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// Reproduces the exact #1811 ordering: AA submits non-interactively via
// StartInvestigation (not the "pending" StartInteractiveSession path), the
// investigation resolves fast enough to hit an InteractiveHold short-circuit
// (mirrors investigator.checkRCAEarlyReturn) and return *before* AF's
// kubernaut_investigate call reaches UpgradeToInteractive+Subscribe. Prior to
// the #1811 fix, every event emitted during that window was silently dropped
// because LazySink had no memory of emissions that occurred before a channel
// was attached.
var _ = Describe("Late-Subscribe Event Replay — AA/AF Interactive-Upgrade Race (#1811)", func() {

	var (
		store  *session.Store
		mgr    *session.Manager
		logger logr.Logger
	)

	BeforeEach(func() {
		logger = logr.Discard()
		store = session.NewStore(10*time.Minute, session.WithLogger(logger))
		mgr = session.NewManager(store, logger, nil, nil)
	})

	Describe("UT-KA-1811-002: buffered events survive a late Subscribe after in-place upgrade", func() {
		It("delivers the RCA-phase events and the terminal complete event to a subscriber that joins after the goroutine already finished", func() {
			investigationDone := make(chan struct{})
			readyForUpgrade := make(chan struct{})
			upgradeApplied := make(chan struct{})

			id, err := mgr.StartInvestigation(context.Background(),
				func(ctx context.Context) (*katypes.InvestigationResult, error) {
					defer close(investigationDone)

					// Simulate the RCA phase emitting a couple of progress
					// events, exactly as investigator.go's emitToSink does —
					// via the context-carried sink, before anyone has
					// subscribed (sink is nil at this point).
					session.EmitEvent(ctx, session.InvestigationEvent{
						Type: session.EventTypeReasoningDelta, Turn: 0, Phase: "rca",
					})
					session.EmitEvent(ctx, session.InvestigationEvent{
						Type: session.EventTypeToolCallStart, Turn: 0, Phase: "rca",
					})

					// Mirrors the exact CI-observed ordering: AF's
					// kubernaut_investigate reaches UpgradeToInteractive
					// *while the goroutine is still running* (status is
					// still StatusRunning, so the upgrade succeeds), and
					// only afterward does investigator.checkRCAEarlyReturn's
					// InteractiveUpgradeFromContext(ctx) check observe the
					// flag and short-circuit with InteractiveHold.
					close(readyForUpgrade)
					<-upgradeApplied

					return &katypes.InvestigationResult{
						RCASummary:      "CrashLoopBackOff: container exits with code 137",
						InteractiveHold: session.InteractiveUpgradeFromContext(ctx),
					}, nil
				},
				map[string]string{"remediation_id": "rr-1811-001"},
			)
			Expect(err).NotTo(HaveOccurred())

			// AF's kubernaut_investigate arrives while the goroutine is
			// still mid-RCA-turn: upgrade-in-place succeeds (session is
			// still StatusRunning), exactly as observed in the CI logs.
			Eventually(readyForUpgrade, 2*time.Second).Should(BeClosed())
			Expect(mgr.UpgradeToInteractive(id, "sre-user", []string{"sre"})).To(Succeed())
			close(upgradeApplied)

			Eventually(investigationDone, 2*time.Second).Should(BeClosed())
			Eventually(func() session.Status {
				sess, _ := store.Get(id)
				return sess.Status
			}, 2*time.Second).Should(Equal(session.StatusUserDriving),
				"InteractiveHold must land the session in StatusUserDriving (non-terminal, channel kept open)")

			// registration.go's Subscribe wiring runs after handleStart
			// returns — by which point, in the race, the goroutine has
			// already emitted (and dropped, pre-fix) everything above.
			eventCh, subErr := mgr.Subscribe(context.Background(), id)
			Expect(subErr).NotTo(HaveOccurred())

			var received []session.InvestigationEvent
			Eventually(func() int {
				for {
					select {
					case evt, ok := <-eventCh:
						if !ok {
							return len(received)
						}
						received = append(received, evt)
					default:
						return len(received)
					}
				}
			}, 2*time.Second).Should(BeNumerically(">=", 3),
				"the 2 buffered RCA events plus the terminal complete event must all be replayed")

			Expect(received[0].Type).To(Equal(session.EventTypeReasoningDelta))
			Expect(received[1].Type).To(Equal(session.EventTypeToolCallStart))
			Expect(received[2].Type).To(Equal(session.EventTypeComplete),
				"emitCompleteEvent's terminal event must also survive the buffering/replay path")
			Expect(string(received[2].Data)).To(ContainSubstring("CrashLoopBackOff"),
				"the replayed complete event must carry the real RCA content, not an empty placeholder")
		})
	})

	Describe("UT-KA-1811-003: already-working case (sink active before emission) is unaffected", func() {
		It("delivers events live with no duplication when Subscribe happens before the goroutine emits", func() {
			pendingID, err := mgr.StartInteractiveSession(context.Background(),
				func(ctx context.Context) (*katypes.InvestigationResult, error) {
					session.EmitEvent(ctx, session.InvestigationEvent{Type: session.EventTypeReasoningDelta, Turn: 0})
					return &katypes.InvestigationResult{RCASummary: "ok"}, nil
				},
				map[string]string{"remediation_id": "rr-1811-002"},
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(mgr.LaunchDeferredInvestigation(pendingID)).To(Succeed())

			eventCh, subErr := mgr.Subscribe(context.Background(), pendingID)
			Expect(subErr).NotTo(HaveOccurred())

			var received []session.InvestigationEvent
			for evt := range eventCh {
				received = append(received, evt)
			}

			Expect(received).To(HaveLen(2), "exactly the 1 live event + 1 complete event — no duplicate replay")
			Expect(received[0].Type).To(Equal(session.EventTypeReasoningDelta))
			Expect(received[1].Type).To(Equal(session.EventTypeComplete))
		})
	})
})

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
	"encoding/json"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// Issue #1794 (backport from main): the live SSE "complete" event never
// carried the investigation's RCA result -- only the reconnect/
// terminalSessionSSE path did, because Manager.runInvestigation emits the
// complete event before Store.Update persists the result (manager.go), and
// emitCompleteEvent never received the result to attach in the first place.
// AF's emitEarlyRCA/emitFallbackInvestigationArtifact key off this event's
// Data field, so a live-streaming Console user never saw the RCA rendered --
// an AU-3/SI-4 gap. Root-caused via E2E-FP-1189-005 on main/v1.6.
var _ = Describe("Live complete event carries RCA data — #1794", func() {

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

	Describe("UT-KA-1794-001: live complete event Data is populated from a non-nil result", func() {
		It("should attach the bounded RCA subset to the live EventTypeComplete event", func() {
			investigationDone := make(chan struct{})

			pendingID, err := mgr.StartInteractiveSession(context.Background(),
				func(_ context.Context) (*katypes.InvestigationResult, error) {
					defer close(investigationDone)
					return &katypes.InvestigationResult{
						Severity:   "critical",
						Confidence: 0.92,
						RCASummary: "OOMKill caused by memory leak",
						WorkflowID: "wf-should-not-leak",
					}, nil
				},
				map[string]string{"remediation_id": "rr-1794-001"},
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(mgr.LaunchDeferredInvestigation(pendingID)).To(Succeed())

			eventCh, subErr := mgr.Subscribe(context.Background(), pendingID)
			Expect(subErr).NotTo(HaveOccurred())

			Eventually(investigationDone, 5*time.Second).Should(BeClosed())

			var completeEvt *session.InvestigationEvent
			Eventually(func() *session.InvestigationEvent {
				select {
				case evt, ok := <-eventCh:
					if !ok {
						return nil
					}
					if evt.Type == session.EventTypeComplete {
						completeEvt = &evt
					}
				default:
				}
				return completeEvt
			}, 5*time.Second, 10*time.Millisecond).ShouldNot(BeNil(), "live complete event must be received")

			Expect(completeEvt.Data).NotTo(BeEmpty(),
				"AU-3/SI-4: the live complete event must carry the RCA result, "+
					"not an empty Data field -- otherwise a streaming Console user never sees the RCA")

			var parsed map[string]interface{}
			Expect(json.Unmarshal(completeEvt.Data, &parsed)).To(Succeed())
			Expect(parsed).To(HaveKeyWithValue("severity", "critical"))
			Expect(parsed).To(HaveKeyWithValue("rca_summary", "OOMKill caused by memory leak"))

			By("SI-10: verifying the live complete event uses the bounded RCA subset, not a raw dump")
			Expect(parsed).NotTo(HaveKey("workflow_id"),
				"internal workflow state must not leak into the AF/Console-facing complete event")
		})
	})

	Describe("UT-KA-1794-002: live complete event has empty Data when investigation returns nil result", func() {
		It("should not populate Data for a nil result (e.g. a failed investigation)", func() {
			investigationDone := make(chan struct{})

			pendingID, err := mgr.StartInteractiveSession(context.Background(),
				func(_ context.Context) (*katypes.InvestigationResult, error) {
					defer close(investigationDone)
					return nil, nil
				},
				map[string]string{"remediation_id": "rr-1794-002"},
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(mgr.LaunchDeferredInvestigation(pendingID)).To(Succeed())

			eventCh, subErr := mgr.Subscribe(context.Background(), pendingID)
			Expect(subErr).NotTo(HaveOccurred())

			Eventually(investigationDone, 5*time.Second).Should(BeClosed())

			var completeEvt *session.InvestigationEvent
			Eventually(func() *session.InvestigationEvent {
				select {
				case evt, ok := <-eventCh:
					if !ok {
						return nil
					}
					if evt.Type == session.EventTypeComplete {
						completeEvt = &evt
					}
				default:
				}
				return completeEvt
			}, 5*time.Second, 10*time.Millisecond).ShouldNot(BeNil(), "live complete event must still be received")

			Expect(completeEvt.Data).To(BeEmpty(), "no RCA data exists to attach for a nil result")
		})
	})
})

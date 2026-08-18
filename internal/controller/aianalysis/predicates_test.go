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

// Issue #1449 / FedRAMP SI-4: the AgentSession event predicate must pass
// terminal transitions to enable immediate reconciliation when the KA-driven
// AgentSession completes. DD-AA-KA-001: this supersedes the retired
// ISEventPredicate (InvestigationSession CRD watch) -- AgentSessionEventPredicate
// is the direct replacement, watching Phase/Interactive/SessionID instead of
// the old Phase/KACorrelationID pair.
package aianalysis_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/aianalysis"
)

var _ = Describe("AgentSessionEventPredicate (#1449, DD-AA-KA-001)", func() {

	makeAS := func(phase agentsessionv1.AgentSessionPhase) *agentsessionv1.AgentSession {
		return &agentsessionv1.AgentSession{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "as-test-1449",
				Namespace: "default",
			},
			Status: agentsessionv1.AgentSessionStatus{
				Phase: phase,
			},
		}
	}

	// ═══════════════════════════════════════════════════════════════════════
	// FedRAMP SI-4: Terminal phase transitions must wake the controller
	// ═══════════════════════════════════════════════════════════════════════

	Context("SI-4: Terminal phase transitions trigger reconciliation", func() {
		DescribeTable("UT-AA-1449-010: passes Update events for terminal phase transitions",
			func(newPhase agentsessionv1.AgentSessionPhase) {
				pred := controller.AgentSessionEventPredicate()
				oldAS := makeAS(agentsessionv1.AgentSessionPhaseInvestigating)
				newAS := makeAS(newPhase)

				updateEvent := event.TypedUpdateEvent[*agentsessionv1.AgentSession]{
					ObjectOld: oldAS,
					ObjectNew: newAS,
				}

				Expect(pred.Update(updateEvent)).To(BeTrue(),
					"SI-4: Update to terminal phase %s must pass the predicate", newPhase)
			},
			Entry("Completed", agentsessionv1.AgentSessionPhaseCompleted),
			Entry("Cancelled", agentsessionv1.AgentSessionPhaseCancelled),
			Entry("Failed", agentsessionv1.AgentSessionPhaseFailed),
		)
	})

	Context("SI-4: Non-terminal transitions are filtered", func() {
		DescribeTable("UT-AA-1449-011: drops Update events for non-terminal phase transitions",
			func(oldPhase, newPhase agentsessionv1.AgentSessionPhase) {
				pred := controller.AgentSessionEventPredicate()
				oldAS := makeAS(oldPhase)
				newAS := makeAS(newPhase)

				updateEvent := event.TypedUpdateEvent[*agentsessionv1.AgentSession]{
					ObjectOld: oldAS,
					ObjectNew: newAS,
				}

				Expect(pred.Update(updateEvent)).To(BeFalse(),
					"SI-4: Non-terminal transition %s→%s must be filtered to avoid unnecessary reconciles", oldPhase, newPhase)
			},
			Entry("Investigating→Investigating (no change)", agentsessionv1.AgentSessionPhaseInvestigating, agentsessionv1.AgentSessionPhaseInvestigating),
			Entry("Pending→Pending (no change)", agentsessionv1.AgentSessionPhasePending, agentsessionv1.AgentSessionPhasePending),
		)

		It("UT-AA-1449-012: drops Update event when ObjectNew is nil", func() {
			pred := controller.AgentSessionEventPredicate()
			updateEvent := event.TypedUpdateEvent[*agentsessionv1.AgentSession]{
				ObjectOld: makeAS(agentsessionv1.AgentSessionPhaseInvestigating),
				ObjectNew: nil,
			}
			Expect(pred.Update(updateEvent)).To(BeFalse())
		})

		It("UT-AA-1449-013: drops Update event when ObjectOld is nil", func() {
			pred := controller.AgentSessionEventPredicate()
			updateEvent := event.TypedUpdateEvent[*agentsessionv1.AgentSession]{
				ObjectOld: nil,
				ObjectNew: makeAS(agentsessionv1.AgentSessionPhaseCompleted),
			}
			Expect(pred.Update(updateEvent)).To(BeFalse())
		})
	})

	// ═══════════════════════════════════════════════════════════════════════
	// Issue #2030 (main-tracking clone of #2029) Part B / FedRAMP SI-4 /
	// DD-AA-KA-001 Amendment Gap 1: Interactive/SessionID-only changes must
	// also wake the controller immediately, not just terminal transitions.
	// Without this, KA's dispatch acknowledgment (which only changes
	// Interactive/SessionID, not Phase) is silently dropped and AA only
	// learns about it on its next scheduled poll instead of near-instantly.
	// ═══════════════════════════════════════════════════════════════════════

	makeASWithSession := func(phase agentsessionv1.AgentSessionPhase, interactive bool, sessionID string) *agentsessionv1.AgentSession {
		as := makeAS(phase)
		as.Status.Interactive = interactive
		as.Status.SessionID = sessionID
		return as
	}

	Context("#2030 SI-4: Interactive/SessionID changes trigger reconciliation", func() {
		It("UT-AA-2030-013: passes Update events when only SessionID changes (Phase stays non-terminal)", func() {
			pred := controller.AgentSessionEventPredicate()
			oldAS := makeASWithSession(agentsessionv1.AgentSessionPhaseInvestigating, false, "ka-session-old")
			newAS := makeASWithSession(agentsessionv1.AgentSessionPhaseInvestigating, false, "ka-session-new")

			updateEvent := event.TypedUpdateEvent[*agentsessionv1.AgentSession]{
				ObjectOld: oldAS,
				ObjectNew: newAS,
			}

			Expect(pred.Update(updateEvent)).To(BeTrue(),
				"#2030 Part B: a SessionID change must pass the predicate even when Phase doesn't change")
		})

		It("UT-AA-2030-013a: passes Update events when only Interactive changes (Gap 1 dispatch ack)", func() {
			pred := controller.AgentSessionEventPredicate()
			oldAS := makeASWithSession(agentsessionv1.AgentSessionPhaseInvestigating, false, "ka-session-same")
			newAS := makeASWithSession(agentsessionv1.AgentSessionPhaseInvestigating, true, "ka-session-same")

			updateEvent := event.TypedUpdateEvent[*agentsessionv1.AgentSession]{
				ObjectOld: oldAS,
				ObjectNew: newAS,
			}

			Expect(pred.Update(updateEvent)).To(BeTrue(),
				"DD-AA-KA-001 Amendment Gap 1: an Interactive flip must pass the predicate even when Phase doesn't change")
		})

		It("UT-AA-2030-013b: drops Update events when SessionID, Interactive, and Phase are all unchanged (no regression)", func() {
			pred := controller.AgentSessionEventPredicate()
			oldAS := makeASWithSession(agentsessionv1.AgentSessionPhaseInvestigating, false, "ka-session-same")
			newAS := makeASWithSession(agentsessionv1.AgentSessionPhaseInvestigating, false, "ka-session-same")

			updateEvent := event.TypedUpdateEvent[*agentsessionv1.AgentSession]{
				ObjectOld: oldAS,
				ObjectNew: newAS,
			}

			Expect(pred.Update(updateEvent)).To(BeFalse(),
				"no-op updates (e.g. resync) must still be filtered to avoid unnecessary reconciles")
		})

		It("UT-AA-2030-013c: existing terminal-phase cases still pass when SessionID is held at its zero value (no regression)", func() {
			pred := controller.AgentSessionEventPredicate()
			oldAS := makeAS(agentsessionv1.AgentSessionPhaseInvestigating)
			newAS := makeAS(agentsessionv1.AgentSessionPhaseCompleted)

			updateEvent := event.TypedUpdateEvent[*agentsessionv1.AgentSession]{
				ObjectOld: oldAS,
				ObjectNew: newAS,
			}

			Expect(pred.Update(updateEvent)).To(BeTrue(),
				"UT-AA-1449-010's terminal-phase case must be unaffected by the #2030 widening")
		})
	})

	Context("Create and Delete events still pass through", func() {
		It("UT-AA-1449-014: passes Create events", func() {
			pred := controller.AgentSessionEventPredicate()
			createEvent := event.TypedCreateEvent[*agentsessionv1.AgentSession]{
				Object: makeAS(agentsessionv1.AgentSessionPhaseInvestigating),
			}
			Expect(pred.Create(createEvent)).To(BeTrue())
		})

		It("UT-AA-1449-015: passes Delete events", func() {
			pred := controller.AgentSessionEventPredicate()
			deleteEvent := event.TypedDeleteEvent[*agentsessionv1.AgentSession]{
				Object: makeAS(agentsessionv1.AgentSessionPhaseCompleted),
			}
			Expect(pred.Delete(deleteEvent)).To(BeTrue())
		})
	})
})

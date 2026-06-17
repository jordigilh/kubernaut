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

// Issue #1449 / FedRAMP SI-4: IS event predicate must pass terminal transitions
// to enable immediate reconciliation when InvestigationSession completes externally.
package aianalysis_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/aianalysis"
)

var _ = Describe("isEventPredicate (#1449)", func() {

	makeIS := func(phase isv1alpha1.SessionPhase) *isv1alpha1.InvestigationSession {
		return &isv1alpha1.InvestigationSession{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "is-test-1449",
				Namespace: "default",
			},
			Status: isv1alpha1.InvestigationSessionStatus{
				Phase: phase,
			},
		}
	}

	// ═══════════════════════════════════════════════════════════════════════
	// FedRAMP SI-4: Terminal phase transitions must wake the controller
	// ═══════════════════════════════════════════════════════════════════════

	Context("SI-4: Terminal phase transitions trigger reconciliation", func() {
		DescribeTable("UT-AA-1449-010: passes Update events for terminal phase transitions",
			func(newPhase isv1alpha1.SessionPhase) {
				pred := controller.ISEventPredicate()
				oldIS := makeIS(isv1alpha1.SessionPhaseActive)
				newIS := makeIS(newPhase)

				updateEvent := event.TypedUpdateEvent[*isv1alpha1.InvestigationSession]{
					ObjectOld: oldIS,
					ObjectNew: newIS,
				}

				Expect(pred.Update(updateEvent)).To(BeTrue(),
					"SI-4: Update to terminal phase %s must pass the predicate", newPhase)
			},
			Entry("Completed", isv1alpha1.SessionPhaseCompleted),
			Entry("Cancelled", isv1alpha1.SessionPhaseCancelled),
			Entry("Failed", isv1alpha1.SessionPhaseFailed),
		)
	})

	Context("SI-4: Non-terminal transitions are filtered", func() {
		DescribeTable("UT-AA-1449-011: drops Update events for non-terminal phase transitions",
			func(oldPhase, newPhase isv1alpha1.SessionPhase) {
				pred := controller.ISEventPredicate()
				oldIS := makeIS(oldPhase)
				newIS := makeIS(newPhase)

				updateEvent := event.TypedUpdateEvent[*isv1alpha1.InvestigationSession]{
					ObjectOld: oldIS,
					ObjectNew: newIS,
				}

				Expect(pred.Update(updateEvent)).To(BeFalse(),
					"SI-4: Non-terminal transition %s→%s must be filtered to avoid unnecessary reconciles", oldPhase, newPhase)
			},
			Entry("Active→Active (no change)", isv1alpha1.SessionPhaseActive, isv1alpha1.SessionPhaseActive),
			Entry("Active→Disconnected", isv1alpha1.SessionPhaseActive, isv1alpha1.SessionPhaseDisconnected),
			Entry("Disconnected→Active (reconnect)", isv1alpha1.SessionPhaseDisconnected, isv1alpha1.SessionPhaseActive),
		)

		It("UT-AA-1449-012: drops Update event when ObjectNew is nil", func() {
			pred := controller.ISEventPredicate()
			updateEvent := event.TypedUpdateEvent[*isv1alpha1.InvestigationSession]{
				ObjectOld: makeIS(isv1alpha1.SessionPhaseActive),
				ObjectNew: nil,
			}
			Expect(pred.Update(updateEvent)).To(BeFalse())
		})

		It("UT-AA-1449-013: drops Update event when ObjectOld is nil", func() {
			pred := controller.ISEventPredicate()
			updateEvent := event.TypedUpdateEvent[*isv1alpha1.InvestigationSession]{
				ObjectOld: nil,
				ObjectNew: makeIS(isv1alpha1.SessionPhaseCompleted),
			}
			Expect(pred.Update(updateEvent)).To(BeFalse())
		})
	})

	Context("Create and Delete events still pass through", func() {
		It("UT-AA-1449-014: passes Create events", func() {
			pred := controller.ISEventPredicate()
			createEvent := event.TypedCreateEvent[*isv1alpha1.InvestigationSession]{
				Object: makeIS(isv1alpha1.SessionPhaseActive),
			}
			Expect(pred.Create(createEvent)).To(BeTrue())
		})

		It("UT-AA-1449-015: passes Delete events", func() {
			pred := controller.ISEventPredicate()
			deleteEvent := event.TypedDeleteEvent[*isv1alpha1.InvestigationSession]{
				Object: makeIS(isv1alpha1.SessionPhaseCompleted),
			}
			Expect(pred.Delete(deleteEvent)).To(BeTrue())
		})
	})
})

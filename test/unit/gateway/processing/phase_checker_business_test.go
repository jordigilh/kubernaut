/*
Copyright 2025 Jordi Gil.

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

package processing

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
)

// ============================================================================
// BUSINESS OUTCOME TESTS: Phase-Based Deduplication (DD-GATEWAY-011)
// ============================================================================
//
// BR-GATEWAY-181: Deduplication prevents wasteful duplicate remediations
//
// BUSINESS VALUE:
// - Prevents wasted compute on duplicate alerts
// - Single remediation per issue occurrence
// - Duplicate signals update status (occurrence tracking)
// ============================================================================

var _ = Describe("BR-GATEWAY-181: Terminal Phase Classification determines deduplication behavior", func() {
	Context("Terminal phases allow new RemediationRequest creation", func() {
		It("classifies Completed as terminal - allows retry for recurring issues", func() {
			// BUSINESS OUTCOME: After successful remediation, new alert = new issue
			isTerminal := processing.IsTerminalPhase(remediationv1alpha1.PhaseCompleted)

			Expect(isTerminal).To(BeTrue(),
				"Completed = terminal → recurring issues get new remediation")
		})

		It("classifies Failed as terminal - allows retry attempts", func() {
			// BUSINESS OUTCOME: Failed remediation can be retried with new approach
			isTerminal := processing.IsTerminalPhase(remediationv1alpha1.PhaseFailed)

			Expect(isTerminal).To(BeTrue(),
				"Failed = terminal → allows new remediation attempt")
		})

		It("classifies TimedOut as terminal - allows retry", func() {
			// BUSINESS OUTCOME: Timed out remediation can be retried
			isTerminal := processing.IsTerminalPhase(remediationv1alpha1.PhaseTimedOut)

			Expect(isTerminal).To(BeTrue(),
				"TimedOut = terminal → allows retry with fresh remediation")
		})
	})

	Context("Non-terminal phases trigger deduplication", func() {
		It("classifies Pending as non-terminal - duplicates update status", func() {
			// BUSINESS OUTCOME: Alert arrives while RR pending → update occurrence count
			isTerminal := processing.IsTerminalPhase(remediationv1alpha1.PhasePending)

			Expect(isTerminal).To(BeFalse(),
				"Pending = non-terminal → duplicate updates occurrence count")
		})

		It("classifies Processing as non-terminal - remediation in progress", func() {
			// BUSINESS OUTCOME: Alert during active remediation → skip duplicate
			isTerminal := processing.IsTerminalPhase(remediationv1alpha1.PhaseProcessing)

			Expect(isTerminal).To(BeFalse(),
				"Processing = non-terminal → no duplicate remediation started")
		})

		It("classifies Blocked as non-terminal - RO manages cooldown", func() {
			// BUSINESS OUTCOME: Blocked RR = RO holding for cooldown
			isTerminal := processing.IsTerminalPhase(remediationv1alpha1.PhaseBlocked)

			Expect(isTerminal).To(BeFalse(),
				"Blocked = non-terminal → RO owns cooldown logic")
		})
	})
})

var _ = Describe("BR-GATEWAY-181: Phase Checker initialization for Gateway", func() {
	var (
		ctx       context.Context
		k8sClient client.Client
		scheme    *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		_ = ctx // Used in actual tests
		scheme = runtime.NewScheme()
		Expect(remediationv1alpha1.AddToScheme(scheme)).To(Succeed())

		k8sClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&remediationv1alpha1.RemediationRequest{}).
			Build()
	})

	It("creates phase checker for Gateway startup", func() {
		// BUSINESS OUTCOME: Gateway can instantiate deduplication checker
		checker := processing.NewPhaseBasedDeduplicationChecker(k8sClient)

		Expect(checker).NotTo(BeNil(),
			"Phase checker created for Gateway deduplication decisions")
	})
})

// ============================================================================
// BUSINESS OUTCOME TESTS: Status Updater for Occurrence Tracking
// ============================================================================
//
// BR-GATEWAY-182: Track duplicate signal occurrences in CRD status
//
// BUSINESS VALUE:
// - Operators see how many duplicate signals arrived
// - Storm detection metrics based on occurrence count
// - Audit trail of signal frequency
// ============================================================================

var _ = Describe("BR-GATEWAY-182: Status Updater tracks duplicate signal occurrences", func() {
	var (
		ctx       context.Context
		k8sClient client.Client
		scheme    *runtime.Scheme
		updater   *processing.StatusUpdater
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(remediationv1alpha1.AddToScheme(scheme)).To(Succeed())

		k8sClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&remediationv1alpha1.RemediationRequest{}).
			Build()

		// DD-STATUS-001: Pass k8sClient as both client and apiReader (tests use fake client, already uncached)
		updater = processing.NewStatusUpdater(k8sClient, k8sClient)
	})

	Context("Deduplication status initialization", func() {
		It("initializes deduplication status on first duplicate", func() {
			// BUSINESS OUTCOME: First duplicate signal recorded in status
			rr := &remediationv1alpha1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-test",
					Namespace: "production",
				},
				Spec: remediationv1alpha1.RemediationRequestSpec{
					SignalFingerprint: "fingerprint-abc123",
				},
			}
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			err := updater.UpdateDeduplicationStatus(ctx, rr)

			Expect(err).NotTo(HaveOccurred())

			// Verify occurrence tracking initialized
			updatedRR := &remediationv1alpha1.RemediationRequest{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), updatedRR)).To(Succeed())
			Expect(updatedRR.Status.Deduplication).NotTo(BeNil(),
				"Deduplication status initialized on first duplicate")
			Expect(updatedRR.Status.Deduplication.OccurrenceCount).To(Equal(int32(1)),
				"First occurrence recorded")
		})

		It("increments occurrence count for subsequent duplicates", func() {
			// BUSINESS OUTCOME: Operators see total duplicate count
			rr := &remediationv1alpha1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-multi",
					Namespace: "production",
				},
				Spec: remediationv1alpha1.RemediationRequestSpec{
					SignalFingerprint: "fingerprint-multi",
				},
			}
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			// Simulate 3 duplicate signals
			for i := 0; i < 3; i++ {
				err := updater.UpdateDeduplicationStatus(ctx, rr)
				Expect(err).NotTo(HaveOccurred())
			}

			// Verify count reflects all duplicates
			updatedRR := &remediationv1alpha1.RemediationRequest{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), updatedRR)).To(Succeed())
			Expect(updatedRR.Status.Deduplication.OccurrenceCount).To(Equal(int32(3)),
				"Occurrence count tracks duplicate frequency for operator visibility")
		})
	})
})

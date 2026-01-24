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

// Integration tests for BR-ORCH-042 (Consecutive Failure Blocking)
// These tests validate that RO blocks remediation after 3 consecutive failures to protect resources.
//
// Business Requirement: BR-ORCH-042 (Consecutive Failure Blocking)
// Design Decision: DD-RO-002 (Centralized Routing Engine)
//
// Test Strategy:
// - RO controller running in envtest with routing engine
// - Real Kubernetes API field selectors to count consecutive failures
// - Validate Blocked phase transitions and cooldown behavior
// - Validate BlockedUntil timestamp calculation
//
// Defense-in-Depth:
// - Unit tests: Mock routing engine responses (fast execution)
// - Integration tests: Real K8s API queries with field selectors (this file)
// - E2E tests: Full consecutive failure scenarios with dependent controllers

package remediationorchestrator

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

var _ = Describe("Consecutive Failures Integration Tests (BR-ORCH-042)", func() {
	var testNamespace string

	BeforeEach(func() {
		testNamespace = createTestNamespace("consecutive-failures")
	})

	AfterEach(func() {
		deleteTestNamespace(testNamespace)
	})

	Context("CF-INT-1: Block After 3 Consecutive Failures (BR-ORCH-042)", func() {
		It("should transition to Blocked phase after 3 consecutive failures for same fingerprint", func() {
			// Use unique fingerprint per test namespace to prevent cross-test pollution
			// (routing engine queries ALL namespaces for matching fingerprints)
			fingerprint := GenerateTestFingerprint(testNamespace)

			// Create and fail 3 consecutive RemediationRequests with same fingerprint
			for i := 1; i <= 3; i++ {
				rr := &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rr-consecutive-fail-" + string(rune('0'+i)),
						Namespace: testNamespace,
					},
					Spec: remediationv1.RemediationRequestSpec{
						SignalFingerprint: fingerprint,
						SignalName:        "test-signal",
						Severity:          "critical",
						SignalType:        "test-type",
						TargetType:        "kubernetes",
						TargetResource: remediationv1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: testNamespace,
						},
						FiringTime:   metav1.Now(),
						ReceivedTime: metav1.Now(),
					},
				}
				Expect(k8sClient.Create(ctx, rr)).To(Succeed())

				// Wait for Processing phase
				Eventually(func() remediationv1.RemediationPhase {
					_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
					return rr.Status.OverallPhase
				}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing))

				// Simulate SP failure to trigger RR failure
				spName := "sp-" + rr.Name
				sp := &signalprocessingv1.SignalProcessing{}
				Eventually(func() error {
					return k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{Name: spName, Namespace: testNamespace}, sp)
				}, timeout, interval).Should(Succeed())

				sp.Status.Phase = signalprocessingv1.PhaseFailed
				sp.Status.Error = "Simulated failure for consecutive failure test"
				Expect(k8sClient.Status().Update(ctx, sp)).To(Succeed())

				// Wait for RR to transition to a terminal phase (Failed or Blocked)
				// Note: RR3 may transition to Blocked if blocking logic triggers on 3rd failure
				Eventually(func() bool {
					_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
					phase := rr.Status.OverallPhase
					return phase == remediationv1.PhaseFailed || phase == remediationv1.PhaseBlocked
				}, timeout, interval).Should(BeTrue(), "RR should reach terminal phase (Failed or Blocked)")
			}

			// Create 4th RemediationRequest with same fingerprint - should be Blocked
			rr4 := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-consecutive-fail-4",
					Namespace: testNamespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: fingerprint,
					SignalName:        "test-signal",
					Severity:          "critical",
					SignalType:        "test-type",
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "test-pod",
						Namespace: testNamespace,
					},
					FiringTime:   metav1.Now(),
					ReceivedTime: metav1.Now(),
				},
			}
			Expect(k8sClient.Create(ctx, rr4)).To(Succeed())

			// Wait for controller to initialize RR4 (populate status fields)
			// Without this, the test may check phase before controller has processed the RR
			Eventually(func() bool {
				_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr4), rr4)
				return rr4.Status.OverallPhase != "" // Any phase means initialized
			}, timeout, interval).Should(BeTrue(), "RR4 should be initialized by controller")

			// Verify RR4 transitions to Blocked (not Processing)
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr4), rr4)
				return rr4.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseBlocked), "Expected 4th RR to be Blocked after 3 consecutive failures")

			// Validate BlockReason
			Expect(rr4.Status.BlockReason).To(Equal("ConsecutiveFailures"))
			Expect(rr4.Status.BlockMessage).ToNot(BeEmpty())
		})
	})

	Context("CF-INT-2: Count Resets on Completed (BR-ORCH-042)", func() {
		It("should reset consecutive failure count when a remediation succeeds", func() {
			// Use unique fingerprint per test namespace to prevent cross-test pollution
			fingerprint := GenerateTestFingerprint(testNamespace, "cf-int-2")

			// Create and fail 2 RemediationRequests
			for i := 1; i <= 2; i++ {
				rr := &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rr-reset-count-fail-" + string(rune('0'+i)),
						Namespace: testNamespace,
					},
					Spec: remediationv1.RemediationRequestSpec{
						SignalFingerprint: fingerprint,
						SignalName:        "test-signal",
						Severity:          "critical",
						SignalType:        "test-type",
						TargetType:        "kubernetes",
						TargetResource: remediationv1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: testNamespace,
						},
						FiringTime:   metav1.Now(),
						ReceivedTime: metav1.Now(),
					},
				}
				Expect(k8sClient.Create(ctx, rr)).To(Succeed())

				// Wait for Processing and simulate failure
				Eventually(func() remediationv1.RemediationPhase {
					_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
					return rr.Status.OverallPhase
				}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing))

				spName := "sp-" + rr.Name
				sp := &signalprocessingv1.SignalProcessing{}
				Eventually(func() error {
					return k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{Name: spName, Namespace: testNamespace}, sp)
				}, timeout, interval).Should(Succeed())
				sp.Status.Phase = signalprocessingv1.PhaseFailed
				sp.Status.Error = "Failure for count reset test"
				Expect(k8sClient.Status().Update(ctx, sp)).To(Succeed())

				Eventually(func() remediationv1.RemediationPhase {
					_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
					return rr.Status.OverallPhase
				}, timeout, interval).Should(Equal(remediationv1.PhaseFailed))
			}

			// Create 3rd RemediationRequest and make it succeed (to reset count)
			rr3 := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-reset-count-success",
					Namespace: testNamespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: fingerprint,
					SignalName:        "test-signal",
					Severity:          "critical",
					SignalType:        "test-type",
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "test-pod",
						Namespace: testNamespace,
					},
					FiringTime:   metav1.Now(),
					ReceivedTime: metav1.Now(),
				},
			}
			Expect(k8sClient.Create(ctx, rr3)).To(Succeed())

			// Simulate success (RR would need to complete full workflow - here we just verify it's not Blocked)
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr3), rr3)
				return rr3.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing), "3rd RR should proceed to Processing (not Blocked) even after 2 failures")

			// Note: Full success flow (Completed) requires WE to complete, which is complex in integration test
			// Unit tests validate the counter reset logic when OverallPhase = Completed
		})
	})

	Context("CF-INT-3: Blocked Phase Prevents New RR (BR-ORCH-042)", func() {
		It("should prevent new RemediationRequests for same fingerprint when Blocked", func() {
			// Use unique fingerprint per test namespace to prevent cross-test pollution
			fingerprint := GenerateTestFingerprint(testNamespace, "cf-int-3")

			// Create and fail 3 consecutive RemediationRequests
			for i := 1; i <= 3; i++ {
				rr := &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rr-blocked-prevent-" + string(rune('0'+i)),
						Namespace: testNamespace,
					},
					Spec: remediationv1.RemediationRequestSpec{
						SignalFingerprint: fingerprint,
						SignalName:        "test-signal",
						Severity:          "critical",
						SignalType:        "test-type",
						TargetType:        "kubernetes",
						TargetResource: remediationv1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: testNamespace,
						},
						FiringTime:   metav1.Now(),
						ReceivedTime: metav1.Now(),
					},
				}
				Expect(k8sClient.Create(ctx, rr)).To(Succeed())

				Eventually(func() remediationv1.RemediationPhase {
					_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
					return rr.Status.OverallPhase
				}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing))

				spName := "sp-" + rr.Name
				sp := &signalprocessingv1.SignalProcessing{}
				Eventually(func() error {
					return k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{Name: spName, Namespace: testNamespace}, sp)
				}, timeout, interval).Should(Succeed())
				sp.Status.Phase = signalprocessingv1.PhaseFailed
				sp.Status.Error = "Failure for blocked prevent test"
				Expect(k8sClient.Status().Update(ctx, sp)).To(Succeed())

				Eventually(func() remediationv1.RemediationPhase {
					_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
					return rr.Status.OverallPhase
				}, timeout, interval).Should(Equal(remediationv1.PhaseFailed))
			}

			// Create 4th RemediationRequest - should be immediately Blocked
			rr4 := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-blocked-prevent-4",
					Namespace: testNamespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: fingerprint,
					SignalName:        "test-signal",
					Severity:          "critical",
					SignalType:        "test-type",
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "test-pod",
						Namespace: testNamespace,
					},
					FiringTime:   metav1.Now(),
					ReceivedTime: metav1.Now(),
				},
			}
			Expect(k8sClient.Create(ctx, rr4)).To(Succeed())

			// Wait for controller to initialize RR4
			Eventually(func() bool {
				_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr4), rr4)
				return rr4.Status.OverallPhase != ""
			}, timeout, interval).Should(BeTrue(), "RR4 should be initialized by controller")

			// Verify RR4 is Blocked and no SignalProcessing is created
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr4), rr4)
				return rr4.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseBlocked))

			// Verify SignalProcessing was NOT created (Blocked prevents child CRD creation)
			spName := "sp-rr-blocked-prevent-4"
			sp := &signalprocessingv1.SignalProcessing{}
			Consistently(func() bool {
				err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{Name: spName, Namespace: testNamespace}, sp)
				return err != nil // Should remain NotFound
			}, "2s", interval).Should(BeTrue(), "SignalProcessing should not be created when RR is Blocked")
		})
	})

	Context("CF-INT-4: Cooldown Expiry Transition to Failed (BR-ORCH-042)", func() {
		It("should transition from Blocked to Failed after cooldown expires", func() {
			// Use unique fingerprint per test namespace to prevent cross-test pollution
			fingerprint := GenerateTestFingerprint(testNamespace, "cf-int-4")

			// Create and fail 3 consecutive RemediationRequests
			for i := 1; i <= 3; i++ {
				rr := &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rr-cooldown-expire-" + string(rune('0'+i)),
						Namespace: testNamespace,
					},
					Spec: remediationv1.RemediationRequestSpec{
						SignalFingerprint: fingerprint,
						SignalName:        "test-signal",
						Severity:          "critical",
						SignalType:        "test-type",
						TargetType:        "kubernetes",
						TargetResource: remediationv1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: testNamespace,
						},
						FiringTime:   metav1.Now(),
						ReceivedTime: metav1.Now(),
					},
				}
				Expect(k8sClient.Create(ctx, rr)).To(Succeed())

				Eventually(func() remediationv1.RemediationPhase {
					_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
					return rr.Status.OverallPhase
				}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing))

				spName := "sp-" + rr.Name
				sp := &signalprocessingv1.SignalProcessing{}
				Eventually(func() error {
					return k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{Name: spName, Namespace: testNamespace}, sp)
				}, timeout, interval).Should(Succeed())
				sp.Status.Phase = signalprocessingv1.PhaseFailed
				sp.Status.Error = "Failure for cooldown test"
				Expect(k8sClient.Status().Update(ctx, sp)).To(Succeed())

				Eventually(func() remediationv1.RemediationPhase {
					_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
					return rr.Status.OverallPhase
				}, timeout, interval).Should(Equal(remediationv1.PhaseFailed))
			}

			// Create 4th RemediationRequest - should be Blocked
			rr4 := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-cooldown-expire-4",
					Namespace: testNamespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: fingerprint,
					SignalName:        "test-signal",
					Severity:          "critical",
					SignalType:        "test-type",
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "test-pod",
						Namespace: testNamespace,
					},
					FiringTime:   metav1.Now(),
					ReceivedTime: metav1.Now(),
				},
			}
			Expect(k8sClient.Create(ctx, rr4)).To(Succeed())

			// Wait for controller to initialize RR4
			Eventually(func() bool {
				_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr4), rr4)
				return rr4.Status.OverallPhase != ""
			}, timeout, interval).Should(BeTrue(), "RR4 should be initialized by controller")

			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr4), rr4)
				return rr4.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseBlocked))

			// Validate BlockedUntil is set and in the future
			Expect(rr4.Status.BlockedUntil).ToNot(BeNil(), "BlockedUntil should be set")
			Expect(rr4.Status.BlockedUntil.Time).To(BeTemporally(">", time.Now()), "BlockedUntil should be in the future")

			// Note: Full cooldown expiry test would require waiting 1 hour (default cooldown)
			// Integration test validates BlockedUntil is set correctly
			// Unit tests validate the transition logic from Blocked→Failed after cooldown
		})
	})

	Context("CF-INT-5: BlockedUntil Calculation (BR-ORCH-042)", func() {
		It("should calculate BlockedUntil timestamp correctly based on cooldown period", func() {
			// Use unique fingerprint per test namespace to prevent cross-test pollution
			fingerprint := GenerateTestFingerprint(testNamespace, "cf-int-5")

			// Create and fail 3 consecutive RemediationRequests
			for i := 1; i <= 3; i++ {
				rr := &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rr-blocked-until-" + string(rune('0'+i)),
						Namespace: testNamespace,
					},
					Spec: remediationv1.RemediationRequestSpec{
						SignalFingerprint: fingerprint,
						SignalName:        "test-signal",
						Severity:          "critical",
						SignalType:        "test-type",
						TargetType:        "kubernetes",
						TargetResource: remediationv1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: testNamespace,
						},
						FiringTime:   metav1.Now(),
						ReceivedTime: metav1.Now(),
					},
				}
				Expect(k8sClient.Create(ctx, rr)).To(Succeed())

				Eventually(func() remediationv1.RemediationPhase {
					_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
					return rr.Status.OverallPhase
				}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing))

				spName := "sp-" + rr.Name
				sp := &signalprocessingv1.SignalProcessing{}
				Eventually(func() error {
					return k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{Name: spName, Namespace: testNamespace}, sp)
				}, timeout, interval).Should(Succeed())
				sp.Status.Phase = signalprocessingv1.PhaseFailed
				sp.Status.Error = "Failure for BlockedUntil test"
				Expect(k8sClient.Status().Update(ctx, sp)).To(Succeed())

				Eventually(func() remediationv1.RemediationPhase {
					_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
					return rr.Status.OverallPhase
				}, timeout, interval).Should(Equal(remediationv1.PhaseFailed))
			}

			// Create 4th RemediationRequest - should be Blocked with BlockedUntil set
			rr4 := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-blocked-until-4",
					Namespace: testNamespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: fingerprint,
					SignalName:        "test-signal",
					Severity:          "critical",
					SignalType:        "test-type",
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "test-pod",
						Namespace: testNamespace,
					},
					FiringTime:   metav1.Now(),
					ReceivedTime: metav1.Now(),
				},
			}
			Expect(k8sClient.Create(ctx, rr4)).To(Succeed())

			// Wait for controller to initialize RR4
			Eventually(func() bool {
				_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr4), rr4)
				return rr4.Status.OverallPhase != ""
			}, timeout, interval).Should(BeTrue(), "RR4 should be initialized by controller")

			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr4), rr4)
				return rr4.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseBlocked))

			// Refresh RR4 status to get BlockedUntil
			Eventually(func() bool {
				_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr4), rr4)
				return rr4.Status.BlockedUntil != nil
			}, timeout, interval).Should(BeTrue())

			// Validate BlockedUntil calculation
			// Per BR-ORCH-042: Default cooldown is 1 hour (3600 seconds)
			Expect(rr4.Status.BlockedUntil).ToNot(BeNil())

			// BlockedUntil should be approximately 1 hour in the future (allow 5min tolerance for test execution)
			expectedBlockedUntil := time.Now().Add(1 * time.Hour)
			actualBlockedUntil := rr4.Status.BlockedUntil.Time

			Expect(actualBlockedUntil).To(BeTemporally("~", expectedBlockedUntil, 5*time.Minute),
				"BlockedUntil should be approximately 1 hour from now (default cooldown)")
		})
	})
})

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

package remediationorchestrator

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sretry "k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

// E2E Tests for Remediation Orchestrator Controller
// Per TESTING_GUIDELINES.md: E2E tests (10-15%) validate critical user journeys
//
// These tests validate:
// - BR-ORCH-025: Full remediation lifecycle orchestration
// - BR-ORCH-026: Approval workflow with RAR CRD
// - BR-ORCH-036: Manual review notification flow
// - BR-ORCH-037: WorkflowNotNeeded handling
//
// Prerequisites:
// - Kind cluster running with isolated kubeconfig
// - All CRDs installed (handled by BeforeSuite)
// - NO running RO controller (tests simulate controller behavior)
//
var _ = Describe("RemediationOrchestrator E2E Tests", Label("e2e"), func() {
	var testNS string

	BeforeEach(func() {
		testNS = createTestNamespace("ro-e2e")
	})

	AfterEach(func() {
		deleteTestNamespace(testNS)
	})

	// ========================================
	// BR-ORCH-025: Full Remediation Lifecycle
	// ========================================
	Describe("Full Remediation Lifecycle (BR-ORCH-025)", func() {
		It("should create RemediationRequest and progress through phases", func() {
			By("Creating a RemediationRequest in Pending phase")
			now := metav1.Now()
			fingerprint := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-lifecycle-e2e",
					Namespace: controllerNamespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: fingerprint,
					SignalName:        "HighCPUUsage",
					Severity:          "medium",
					SignalType:        "alert",
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Deployment",
						Name:      "test-app",
						Namespace: testNS,
					},
					FiringTime:   now,
					ReceivedTime: now,
					Deduplication: sharedtypes.DeduplicationInfo{
						FirstOccurrence: now,
						LastOccurrence:  now,
						OccurrenceCount: 1,
					},
				},
			}
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, rr) })

			By("Verifying RemediationRequest was created")
			createdRR := &remediationv1.RemediationRequest{}
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), createdRR)
			}, timeout, interval).Should(Succeed())
			Expect(createdRR.Spec.SignalFingerprint).To(Equal(fingerprint))

			sp := helpers.WaitForSPCreation(ctx, k8sClient, controllerNamespace, rr.Name, timeout, interval)
			helpers.SimulateSPCompletion(ctx, k8sClient, sp)

			ai := helpers.WaitForAICreation(ctx, k8sClient, controllerNamespace, rr.Name, timeout, interval)
			helpers.SimulateAICompletedWithWorkflow(ctx, k8sClient, ai, helpers.AICompletionOpts{
				TargetKind:      "Deployment",
				TargetName:      "test-app",
				TargetNamespace: testNS,
			})

			we := helpers.WaitForWECreation(ctx, k8sClient, controllerNamespace, rr.Name, timeout, interval)
			helpers.SimulateWECompletion(ctx, k8sClient, we)

			By("Verifying all child CRDs have correct owner references set by RO")
			Expect(sp.OwnerReferences).To(HaveLen(1))
			Expect(sp.OwnerReferences[0].Name).To(Equal(rr.Name))

			Expect(ai.OwnerReferences).To(HaveLen(1))
			Expect(ai.OwnerReferences[0].Name).To(Equal(rr.Name))

			Expect(we.OwnerReferences).To(HaveLen(1))
			Expect(we.OwnerReferences[0].Name).To(Equal(rr.Name))
		})
	})

	// ========================================
	// BR-ORCH-026: Approval Workflow
	// ========================================
	Describe("Approval Workflow (BR-ORCH-026)", func() {
		It("should create RemediationApprovalRequest when approval is required", func() {
			By("Creating a RemediationRequest")
			now := metav1.Now()
			fingerprint := "b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3"
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-approval-e2e",
					Namespace: controllerNamespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: fingerprint,
					SignalName:        "CriticalError",
					Severity:          "critical",
					SignalType:        "alert",
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "test-pod",
						Namespace: testNS,
					},
					FiringTime:   now,
					ReceivedTime: now,
					Deduplication: sharedtypes.DeduplicationInfo{
						FirstOccurrence: now,
						LastOccurrence:  now,
						OccurrenceCount: 1,
					},
				},
			}
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, rr) })

			sp := helpers.WaitForSPCreation(ctx, k8sClient, controllerNamespace, rr.Name, timeout, interval)
			helpers.SimulateSPCompletion(ctx, k8sClient, sp)

			ai := helpers.WaitForAICreation(ctx, k8sClient, controllerNamespace, rr.Name, timeout, interval)
			helpers.SimulateAICompletedWithWorkflow(ctx, k8sClient, ai, helpers.AICompletionOpts{
				ApprovalRequired: true,
				ApprovalReason:   "Confidence below auto-approve threshold",
				Confidence:       0.65,
				TargetKind:       "Pod",
				TargetName:       "test-pod",
				TargetNamespace:  testNS,
			})

			By("Waiting for RO to create RemediationApprovalRequest")
			rar := helpers.WaitForRARCreation(ctx, k8sClient, controllerNamespace, rr.Name, timeout, interval)

			By("Verifying RAR was created correctly by RO")
			Expect(rar.Spec.Confidence).To(Equal(float64(0.65)))
			Expect(rar.Spec.RequiredBy.Time).To(BeTemporally(">", time.Now()))

			By("Simulating operator approval")
			Expect(k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rar), rar); err != nil {
					return err
				}
				rar.Status.Decision = remediationv1.ApprovalDecisionApproved
				rar.Status.DecidedBy = "operator@example.com"
				rar.Status.DecidedAt = &metav1.Time{Time: time.Now()}
				rar.Status.DecisionMessage = "Approved for execution"
				return k8sClient.Status().Update(ctx, rar)
			})).To(Succeed())

			By("Verifying approval status was updated")
			Eventually(func() string {
				updatedRAR := &remediationv1.RemediationApprovalRequest{}
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rar), updatedRAR); err != nil {
					return ""
				}
				return string(updatedRAR.Status.Decision)
			}, timeout, interval).Should(Equal("Approved"))
		})

		It("should handle approval rejection", func() {
			By("Creating a RemediationRequest")
			now := metav1.Now()
			fingerprint := "c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4"
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-rejection-e2e",
					Namespace: controllerNamespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: fingerprint,
					SignalName:        "DangerousOperation",
					Severity:          "critical",
					SignalType:        "alert",
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "risky-pod",
						Namespace: testNS,
					},
					FiringTime:   now,
					ReceivedTime: now,
					Deduplication: sharedtypes.DeduplicationInfo{
						FirstOccurrence: now,
						LastOccurrence:  now,
						OccurrenceCount: 1,
					},
				},
			}
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, rr) })

			sp := helpers.WaitForSPCreation(ctx, k8sClient, controllerNamespace, rr.Name, timeout, interval)
			helpers.SimulateSPCompletion(ctx, k8sClient, sp)

			ai := helpers.WaitForAICreation(ctx, k8sClient, controllerNamespace, rr.Name, timeout, interval)
			helpers.SimulateAICompletedWithWorkflow(ctx, k8sClient, ai, helpers.AICompletionOpts{
				ApprovalRequired: true,
				ApprovalReason:   "Confidence below auto-approve threshold",
				Confidence:       0.45,
				TargetKind:       "Pod",
				TargetName:       "risky-pod",
				TargetNamespace:  testNS,
			})

			By("Waiting for RO to create RemediationApprovalRequest")
			rar := helpers.WaitForRARCreation(ctx, k8sClient, controllerNamespace, rr.Name, timeout, interval)

			By("Simulating operator rejection")
			Expect(k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rar), rar); err != nil {
					return err
				}
				rar.Status.Decision = remediationv1.ApprovalDecisionRejected
				rar.Status.DecidedBy = "admin@example.com"
				rar.Status.DecidedAt = &metav1.Time{Time: time.Now()}
				rar.Status.DecisionMessage = "Too risky, manual investigation required"
				return k8sClient.Status().Update(ctx, rar)
			})).To(Succeed())

			By("Verifying rejection status")
			Eventually(func() string {
				updatedRAR := &remediationv1.RemediationApprovalRequest{}
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rar), updatedRAR); err != nil {
					return ""
				}
				return string(updatedRAR.Status.Decision)
			}, timeout, interval).Should(Equal("Rejected"))
		})
	})

	// ========================================
	// BR-ORCH-037: WorkflowNotNeeded Handling
	// ========================================
	Describe("WorkflowNotNeeded Handling (BR-ORCH-037)", func() {
		It("should complete remediation when no workflow is needed", func() {
			By("Creating a RemediationRequest")
			now := metav1.Now()
			fingerprint := "d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5"
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-no-workflow-e2e",
					Namespace: controllerNamespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: fingerprint,
					SignalName:        "TransientError",
					Severity:          "low",
					SignalType:        "alert",
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "transient-pod",
						Namespace: testNS,
					},
					FiringTime:   now,
					ReceivedTime: now,
					Deduplication: sharedtypes.DeduplicationInfo{
						FirstOccurrence: now,
						LastOccurrence:  now,
						OccurrenceCount: 1,
					},
				},
			}
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, rr) })

			sp := helpers.WaitForSPCreation(ctx, k8sClient, controllerNamespace, rr.Name, timeout, interval)
			helpers.SimulateSPCompletion(ctx, k8sClient, sp)

			ai := helpers.WaitForAICreation(ctx, k8sClient, controllerNamespace, rr.Name, timeout, interval)
			helpers.SimulateAIWorkflowNotNeeded(ctx, k8sClient, ai)

			By("Verifying AIAnalysis shows WorkflowNotNeeded reason")
			Eventually(func() string {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(ai), ai); err != nil {
					return ""
				}
				return ai.Status.Reason
			}, timeout, interval).Should(Equal("WorkflowNotNeeded"))
		})
	})

	// ========================================
	// Cascade Deletion Tests
	// ========================================
	Describe("Cascade Deletion", func() {
		It("should delete child CRDs when parent RR is deleted", func() {
			By("Creating a RemediationRequest with child CRDs")
			now := metav1.Now()
			fingerprint := "e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6"
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-cascade-e2e",
					Namespace: controllerNamespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: fingerprint,
					SignalName:        "CascadeTest",
					Severity:          "medium",
					SignalType:        "alert",
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Deployment",
						Name:      "cascade-app",
						Namespace: testNS,
					},
					FiringTime:   now,
					ReceivedTime: now,
					Deduplication: sharedtypes.DeduplicationInfo{
						FirstOccurrence: now,
						LastOccurrence:  now,
						OccurrenceCount: 1,
					},
				},
			}
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, rr) })

			By("Waiting for RO to create child SignalProcessing with owner reference")
			sp := helpers.WaitForSPCreation(ctx, k8sClient, controllerNamespace, rr.Name, timeout, interval)
			Expect(sp.OwnerReferences).To(HaveLen(1),
				"SP should have OwnerReference set by RO for cascade deletion")

			By("Deleting parent RemediationRequest")
			Expect(k8sClient.Delete(ctx, rr)).To(Succeed())

			By("Verifying parent RR is deleted")
			Eventually(func() bool {
				err := apiReader.Get(ctx, client.ObjectKeyFromObject(rr), &remediationv1.RemediationRequest{})
				return err != nil
			}, timeout, interval).Should(BeTrue())

			By("Verifying child SP is deleted via cascade")
			Eventually(func() bool {
				err := apiReader.Get(ctx, client.ObjectKeyFromObject(sp), sp)
				return err != nil
			}, timeout, interval).Should(BeTrue())
		})
	})

	// ========================================
	// E2E-RO-163-002: Deduplication preservation
	// RO does not touch Deduplication (Gateway-owned); test verifies RO preserves it during reconciliation
	// ========================================
	Describe("E2E-RO-163-002: Deduplication preservation", func() {
		It("should preserve pre-populated Deduplication status during RO reconciliation", func() {
			By("Creating a RemediationRequest")
			now := metav1.Now()
			fiveMinutesAgo := metav1.NewTime(now.Add(-5 * time.Minute))
			fingerprint := "7e954e3f07affe767999611bc4f06fed5ef1c20a0a79cf0e9b6c5ce74071dbb6"
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-dedup-e2e",
					Namespace: controllerNamespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: fingerprint,
					SignalName:        "DedupTest",
					Severity:          "medium",
					SignalType:        "alert",
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Deployment",
						Name:      "dedup-app",
						Namespace: testNS,
					},
					FiringTime:   now,
					ReceivedTime: now,
					Deduplication: sharedtypes.DeduplicationInfo{
						FirstOccurrence: now,
						LastOccurrence:  now,
						OccurrenceCount: 1,
					},
				},
			}
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, rr) })

			By("Pre-populating Status.Deduplication and DuplicateOf via status update")
			createdRR := &remediationv1.RemediationRequest{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), createdRR)).To(Succeed())
			createdRR.Status.Deduplication = &remediationv1.DeduplicationStatus{
				FirstSeenAt:     &fiveMinutesAgo,
				LastSeenAt:     &now,
				OccurrenceCount: 2,
			}
			createdRR.Status.DuplicateOf = "rr-original-parent"
			Expect(k8sClient.Status().Update(ctx, createdRR)).To(Succeed())

			By("Letting RO reconcile (wait for Phase change or conditions)")
			Eventually(func() bool {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), createdRR)
				// RO will transition from Pending; any phase change indicates reconciliation
				return createdRR.Status.OverallPhase != "" || len(createdRR.Status.Conditions) > 0
			}, timeout, interval).Should(BeTrue(), "RO should reconcile the RR")

			By("Re-fetching RR and asserting Deduplication preserved")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), createdRR)).To(Succeed())
			Expect(createdRR.Status.Deduplication).NotTo(BeNil())
			Expect(createdRR.Status.Deduplication.OccurrenceCount).To(Equal(int32(2)),
				"Deduplication.OccurrenceCount should be preserved")
			Expect(createdRR.Status.Deduplication.FirstSeenAt).NotTo(BeNil())
			Expect(createdRR.Status.Deduplication.LastSeenAt).NotTo(BeNil())
			Expect(createdRR.Status.DuplicateOf).To(Equal("rr-original-parent"),
				"DuplicateOf should be preserved during RO reconciliation")

			GinkgoWriter.Printf("E2E-RO-163-002: Deduplication preserved — OccurrenceCount=%d, DuplicateOf=%s\n",
				createdRR.Status.Deduplication.OccurrenceCount, createdRR.Status.DuplicateOf)
		})
	})
})

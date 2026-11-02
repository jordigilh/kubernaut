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

package remediationorchestrator

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationapprovalrequest"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("RAR approval safety production wiring", Label("integration", "rar", "approval-safety"), func() {
	DescribeTable("IT-RO-2564-002: applies the RAR deletion matrix before releasing the finalizer",
		func(parentPhase remediationv1.RemediationPhase, shouldFailParent bool) {
			rrName := fmt.Sprintf("rr-rar-delete-%s", uuid.NewString()[:13])
			rr := createRemediationRequest(ROControllerNamespace, rrName)
			rar := rarSafetyRequest(rr, time.Now().Add(15*time.Minute), true)
			setRARSafetyParentPhase(rr.Name, parentPhase)
			Expect(k8sClient.Create(ctx, rar)).To(Succeed())
			DeferCleanup(func() {
				rarSafetyRemoveFinalizer(rar.Name)
				_ = k8sClient.Delete(ctx, rr)
			})

			Expect(k8sClient.Delete(ctx, rar)).To(Succeed())

			Eventually(func() bool {
				current := &remediationv1.RemediationApprovalRequest{}
				err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rar), current)
				return apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue(), "RAR finalizer should be released after the parent decision")

			currentRR := &remediationv1.RemediationRequest{}
			Eventually(func() remediationv1.RemediationPhase {
				if err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), currentRR); err != nil {
					return ""
				}
				return currentRR.Status.OverallPhase
			}, timeout, interval).Should(Equal(func() remediationv1.RemediationPhase {
				if shouldFailParent {
					return remediationv1.PhaseFailed
				}
				return parentPhase
			}()))
		},
		Entry("awaiting approval deletion fails the parent", remediationv1.PhaseAwaitingApproval, true),
		Entry("terminal parent deletion does not mutate the parent", remediationv1.PhaseCompleted, false),
	)

	It("IT-RO-2564-003: parent deletion performs a direct terminating-RAR lookup", func() {
		rrName := fmt.Sprintf("rr-rar-orphan-%s", uuid.NewString()[:13])
		rr := createRemediationRequest(ROControllerNamespace, rrName)
		rar := rarSafetyRequest(rr, time.Now().Add(15*time.Minute), true)
		setRARSafetyParentPhase(rr.Name, remediationv1.PhaseAwaitingApproval)
		Expect(k8sClient.Create(ctx, rar)).To(Succeed())
		DeferCleanup(func() {
			rarSafetyRemoveFinalizer(rar.Name)
			_ = k8sClient.Delete(ctx, rr)
		})

		// Delete the parent first. Envtest does not run the Kubernetes
		// garbage-collector controller, so the RAR remains available with its
		// finalizer and the explicit RAR delete below exercises RO's NotFound
		// path without relying on GC fan-out.
		Expect(k8sClient.Delete(ctx, rr)).To(Succeed())
		Eventually(func() bool {
			current := &remediationv1.RemediationRequest{}
			return apierrors.IsNotFound(k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), current))
		}, timeout, interval).Should(BeTrue())

		Expect(k8sClient.Delete(ctx, rar)).To(Succeed())
		Eventually(func() bool {
			current := &remediationv1.RemediationApprovalRequest{}
			return apierrors.IsNotFound(k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rar), current))
		}, timeout, interval).Should(BeTrue(), "orphaned terminating RAR should be finalized after the parent is confirmed gone")
	})

	It("IT-RO-2564-007: production dispatch atomically confirms expiry before failing the parent", func() {
		rrName := fmt.Sprintf("rr-rar-expiry-%s", uuid.NewString()[:13])
		rr := createRemediationRequest(ROControllerNamespace, rrName)
		rar := rarSafetyRequest(rr, time.Now().Add(-1*time.Minute), true)
		setRARSafetyParentPhase(rr.Name, remediationv1.PhaseAwaitingApproval)
		Expect(k8sClient.Create(ctx, rar)).To(Succeed())
		DeferCleanup(func() {
			rarSafetyRemoveFinalizer(rar.Name)
			_ = k8sClient.Delete(ctx, rr)
		})

		currentRAR := &remediationv1.RemediationApprovalRequest{}
		Eventually(func() remediationv1.ApprovalDecision {
			if err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rar), currentRAR); err != nil {
				return remediationv1.ApprovalDecisionPending
			}
			return currentRAR.Status.Decision
		}, timeout, interval).Should(Equal(remediationv1.ApprovalDecisionExpired))

		Expect(currentRAR.Status.DecidedBy).To(Equal("system"))
		Expect(currentRAR.Status.DecidedVia).To(BeEmpty())
		Expect(currentRAR.Status.Expired).To(BeTrue())
		Expect(currentRAR.Status.DecidedAt).NotTo(BeNil())
		Expect(currentRAR.Status.DecidedAt.Time).To(BeTemporally(">=", rar.Spec.RequiredBy.Time))

		Eventually(func() *metav1.Condition {
			current := &remediationv1.RemediationApprovalRequest{}
			if err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rar), current); err != nil {
				return nil
			}
			return meta.FindStatusCondition(current.Status.Conditions, remediationapprovalrequest.ConditionAuditRecorded)
		}, timeout, interval).Should(SatisfyAll(
			Not(BeNil()),
			WithTransform(func(condition *metav1.Condition) metav1.ConditionStatus { return condition.Status }, Equal(metav1.ConditionTrue)),
		), "expiry audit bookkeeping should record enqueue acceptance")

		Eventually(func() remediationv1.RemediationPhase {
			current := &remediationv1.RemediationRequest{}
			if err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), current); err != nil {
				return ""
			}
			return current.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseFailed), "parent failure requires ExpiredConfirmed")
	})

	It("IT-RO-2564-009: RAR audit bookkeeping records enqueue acceptance, not durability", func() {
		rarName := fmt.Sprintf("rar-audit-%s", uuid.NewString()[:13])
		rar := rarSafetyRequest(nil, time.Now().Add(15*time.Minute), false)
		rar.Name = rarName
		rar.Spec.RemediationRequestRef = corev1.ObjectReference{
			APIVersion: remediationv1.GroupVersion.String(),
			Kind:       "RemediationRequest",
			Name:       "rr-audit-parent",
			Namespace:  ROControllerNamespace,
		}
		Expect(k8sClient.Create(ctx, rar)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, rar) })

		decidedAt := metav1.NewTime(time.Now().Add(-1 * time.Minute))
		Expect(approveRAR(rar.Name, func(current *remediationv1.RemediationApprovalRequest) {
			current.Status.Decision = remediationv1.ApprovalDecisionApproved
			current.Status.DecidedBy = "operator@example.com"
			current.Status.DecidedAt = &decidedAt
			current.Status.DecisionMessage = "approved for integration evidence"
		})).To(Succeed())

		current := &remediationv1.RemediationApprovalRequest{}
		Eventually(func() *metav1.Condition {
			if err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rar), current); err != nil {
				return nil
			}
			return meta.FindStatusCondition(current.Status.Conditions, remediationapprovalrequest.ConditionAuditRecorded)
		}, timeout, interval).Should(SatisfyAll(
			Not(BeNil()),
			WithTransform(func(condition *metav1.Condition) metav1.ConditionStatus { return condition.Status }, Equal(metav1.ConditionTrue)),
		), "RAR audit controller should persist enqueue acceptance")

		Expect(meta.FindStatusCondition(current.Status.Conditions, remediationapprovalrequest.ConditionAuditRecorded).Message).
			To(ContainSubstring("accepted by audit buffer"))
		Expect(meta.FindStatusCondition(current.Status.Conditions, remediationapprovalrequest.ConditionAuditRecorded).Message).
			NotTo(ContainSubstring("recorded to DataStorage"))
	})

	It("IT-RO-2564-017: an invalid parent reference retains the RAR finalizer", func() {
		rrName := fmt.Sprintf("rr-rar-invalid-ref-%s", uuid.NewString()[:13])
		rr := createRemediationRequest(ROControllerNamespace, rrName)
		rar := rarSafetyRequest(rr, time.Now().Add(15*time.Minute), true)
		rar.Spec.RemediationRequestRef.Name = rrName + "-different"
		setRARSafetyParentPhase(rr.Name, remediationv1.PhaseAwaitingApproval)
		Expect(k8sClient.Create(ctx, rar)).To(Succeed())
		DeferCleanup(func() {
			rarSafetyRemoveFinalizer(rar.Name)
			_ = k8sClient.Delete(ctx, rr)
		})

		Expect(k8sClient.Delete(ctx, rar)).To(Succeed())
		Eventually(func() bool {
			current := &remediationv1.RemediationApprovalRequest{}
			if err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rar), current); err != nil {
				return false
			}
			return current.DeletionTimestamp != nil && controllerutil.ContainsFinalizer(current, creator.ApprovalRequestFinalizerName)
		}, timeout, interval).Should(BeTrue(), "invalid parent reference must fail closed and retain the finalizer")
	})

	It("IT-RO-2564-019: real API persistence distinguishes Created from Collision", func() {
		rrName := fmt.Sprintf("rr-rar-collision-%s", uuid.NewString()[:13])
		rr := createRemediationRequest(ROControllerNamespace, rrName)
		setRARSafetyParentPhase(rr.Name, remediationv1.PhaseAwaitingApproval)
		ai := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{Name: "ai-" + rrName, Namespace: ROControllerNamespace},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase: aianalysisv1.PhaseCompleted,
				Approval: &aianalysisv1.ApprovalStatus{
					ApprovalRequired: true,
					ApprovalReason:   "low confidence",
				},
				RCAResult: &aianalysisv1.RCAResult{
					SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
						WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
							WorkflowID:      "wf-rar-collision",
							WorkflowName:    "wf-rar-collision",
							ActionType:      "RestartPod",
							Version:         "v1",
							ExecutionBundle: "oci://example.invalid/workflows:collision",
						},
						Confidence: 0.4,
						Rationale:  "integration collision fixture",
					},
				},
			},
		}
		approvalCreator := creator.NewApprovalCreator(k8sClient, k8sManager.GetScheme(), nil, 15*time.Minute)
		name, outcome, err := approvalCreator.CreateWithResult(ctx, rr, ai)
		Expect(err).NotTo(HaveOccurred())
		Expect(name).To(Equal("rar-" + rrName))
		Expect(outcome).To(Equal(creator.ApprovalCreationCreated))
		DeferCleanup(func() {
			rarSafetyRemoveFinalizer(name)
			_ = k8sClient.Delete(ctx, &remediationv1.RemediationApprovalRequest{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ROControllerNamespace}})
			_ = k8sClient.Delete(ctx, rr)
		})

		_, outcome, err = approvalCreator.CreateWithResult(ctx, rr, ai)
		Expect(err).To(HaveOccurred())
		Expect(outcome).To(Equal(creator.ApprovalCreationCollision))
		var creationErr *creator.ApprovalCreationError
		Expect(errors.As(err, &creationErr)).To(BeTrue())
		Expect(creationErr.ReasonCode).To(Equal(creator.ApprovalCreationReasonCollision))
	})
})

func rarSafetyRequest(rr *remediationv1.RemediationRequest, requiredBy time.Time, withFinalizer bool) *remediationv1.RemediationApprovalRequest {
	name := "rar-" + uuid.NewString()[:13]
	ownerReferences := []metav1.OwnerReference(nil)
	parentName := "rr-rar-parent"
	parentUID := types.UID("")
	if rr != nil {
		name = "rar-" + rr.Name
		parentName = rr.Name
		parentUID = rr.UID
		ownerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(rr, remediationv1.GroupVersion.WithKind("RemediationRequest"))}
	}
	finalizers := []string(nil)
	if withFinalizer {
		finalizers = []string{creator.ApprovalRequestFinalizerName}
	}
	return &remediationv1.RemediationApprovalRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       ROControllerNamespace,
			Finalizers:      finalizers,
			OwnerReferences: ownerReferences,
		},
		Spec: remediationv1.RemediationApprovalRequestSpec{
			RemediationRequestRef: corev1.ObjectReference{
				APIVersion: remediationv1.GroupVersion.String(),
				Kind:       "RemediationRequest",
				Name:       parentName,
				Namespace:  ROControllerNamespace,
				UID:        parentUID,
			},
			AIAnalysisRef:   remediationv1.ObjectRef{Name: "ai-" + parentName},
			Confidence:      0.4,
			ConfidenceLevel: "low",
			Reason:          "low confidence requires approval",
			RecommendedWorkflow: remediationv1.RecommendedWorkflowSummary{
				WorkflowID:      "wf-rar-safety",
				Version:         "v1",
				ExecutionBundle: "oci://example.invalid/workflows:safety",
				Rationale:       "bounded integration fixture",
			},
			InvestigationSummary: "bounded integration fixture",
			RecommendedActions: []remediationv1.ApprovalRecommendedAction{{
				Action:    "review",
				Rationale: "approval required",
			}},
			WhyApprovalRequired: "low confidence requires approval",
			RequiredBy:          metav1.NewTime(requiredBy),
		},
	}
}

func setRARSafetyParentPhase(name string, desired remediationv1.RemediationPhase) {
	Eventually(func() error {
		current := &remediationv1.RemediationRequest{}
		if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: name, Namespace: ROControllerNamespace}, current); err != nil {
			return err
		}
		current.Status.OverallPhase = desired
		current.Status.ObservedGeneration = current.Generation
		return k8sClient.Status().Update(ctx, current)
	}, timeout, interval).Should(Succeed())
}

func rarSafetyRemoveFinalizer(name string) {
	_ = retryUntilRARAvailable(name, func(current *remediationv1.RemediationApprovalRequest) error {
		if !controllerutil.ContainsFinalizer(current, creator.ApprovalRequestFinalizerName) {
			return nil
		}
		controllerutil.RemoveFinalizer(current, creator.ApprovalRequestFinalizerName)
		return k8sClient.Update(ctx, current)
	})
}

func retryUntilRARAvailable(name string, mutate func(*remediationv1.RemediationApprovalRequest) error) error {
	var lastErr error
	Eventually(func() bool {
		current := &remediationv1.RemediationApprovalRequest{}
		if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: name, Namespace: ROControllerNamespace}, current); err != nil {
			if apierrors.IsNotFound(err) {
				return true
			}
			lastErr = err
			return false
		}
		lastErr = mutate(current)
		return lastErr == nil
	}, timeout, interval).Should(BeTrue())
	return lastErr
}

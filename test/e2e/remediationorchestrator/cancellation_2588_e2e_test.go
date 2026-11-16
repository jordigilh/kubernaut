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

// Issue #2588 deployed cancellation barrier coverage.
// Business requirements: BR-AUDIT-005, BR-ORCH-029, BR-ORCH-030, BR-ORCH-031.
// Controls: SOC2 CC7.2; FedRAMP AU-2, AU-3, AU-9, AC-6.

package remediationorchestrator

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	roaudit "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/audit"
	"github.com/jordigilh/kubernaut/pkg/shared/cancellation"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

const e2eCancellationFinalizer = "kubernaut.ai/remediationrequest-cancellation"

var _ = Describe("Issue #2588 deployed cancellation", Label("e2e", "cancellation", "issue-2588"), func() {
	It("E2E-ORCH-2588-001 / E2E-NT-2588-003: holds RR cancellation until child acknowledgement and terminal notification delivery", NodeTimeout(4*time.Minute), func(ctx SpecContext) {
		targetNamespace := createTestNamespace(ctx, "ro-cancel-2588")
		DeferCleanup(func() { deleteTestNamespace(targetNamespace) })

		rrName := fmt.Sprintf("e2e-cancel-2588-%s", uuid.NewString()[:8])
		now := metav1.Now()
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: rrName, Namespace: controllerNamespace},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: fmt.Sprintf("%064x", uuid.New()),
				SignalName:        "E2ECancellation",
				Severity:          "critical",
				SignalType:        "alert",
				TargetType:        "kubernetes",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind: "Deployment", Name: "cancel-target", Namespace: targetNamespace,
				},
				FiringTime: now, ReceivedTime: now,
			},
		}
		Expect(k8sClient.Create(ctx, rr)).To(Succeed())
		DeferCleanup(func() { _ = client.IgnoreNotFound(k8sClient.Delete(ctx, rr)) })

		By("waiting for the deployed RO to install its cancellation finalizer")
		Eventually(func() bool {
			current := &remediationv1.RemediationRequest{}
			if err := apiReader.Get(ctx, client.ObjectKeyFromObject(rr), current); err != nil {
				return false
			}
			return controllerutil.ContainsFinalizer(current, e2eCancellationFinalizer)
		}, timeout, interval).Should(BeTrue())

		By("creating a pending child and recording it on the RR")
		wfe := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{Name: "wfe-" + rrName, Namespace: controllerNamespace},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				RemediationRequestRef: corev1.ObjectReference{
					APIVersion: remediationv1.GroupVersion.String(), Kind: "RemediationRequest",
					Name: rr.Name, Namespace: rr.Namespace, UID: rr.UID,
				},
				WorkflowRef: workflowexecutionv1.WorkflowRef{WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
					WorkflowID: "e2e-cancel-workflow", WorkflowName: "e2e-cancel-workflow",
					ActionType: "RestartPod", Version: "v1", ExecutionBundle: "e2e/cancel:v1",
				}},
				TargetResource: fmt.Sprintf("%s/deployment/cancel-target", targetNamespace),
			},
		}
		Expect(controllerutil.SetControllerReference(rr, wfe, scheme.Scheme)).To(Succeed())
		Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

		Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
			current := &remediationv1.RemediationRequest{}
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), current); err != nil {
				return err
			}
			current.Status.OverallPhase = remediationv1.PhaseCancelling
			cancellationStatus := current.Status.EnsureCancellation()
			cancellationStatus.RequestID = fmt.Sprintf("%s/%s:cancellation", rr.Namespace, rr.Name)
			cancellationStatus.RequestedAt = &now
			cancellationStatus.RequestedBy = "system"
			cancellationStatus.Source = remediationv1.CancellationSourceSystem
			cancellationStatus.Reason = "E2E-WE-2588 cancellation barrier"
			cancellationStatus.State = remediationv1.CancellationStateRequested
			current.Status.EnsurePhaseProgress().WorkflowExecutionRef = &corev1.ObjectReference{
				APIVersion: workflowexecutionv1.GroupVersion.String(), Kind: "WorkflowExecution",
				Name: wfe.Name, Namespace: wfe.Namespace,
			}
			return k8sClient.Status().Update(ctx, current)
		})).To(Succeed())

		By("waiting for RO to create and track the durable cancellation notification")
		notification := &notificationv1.NotificationRequest{}
		Eventually(func() error {
			return apiReader.Get(ctx, types.NamespacedName{
				Name: "nr-cancellation-" + rrName, Namespace: controllerNamespace,
			}, notification)
		}, timeout, interval).Should(Succeed())
		Expect(notification.Spec.Type).To(Equal(notificationv1.NotificationTypeCancellation))
		Expect(notification.Spec.RemediationRequestRef).NotTo(BeNil())
		Expect(notification.Spec.RemediationRequestRef.Name).To(Equal(rrName))

		By("proving the deployed parent remains Cancelling while the child is pending")
		Eventually(func() remediationv1.RemediationPhase {
			current := &remediationv1.RemediationRequest{}
			if err := apiReader.Get(ctx, client.ObjectKeyFromObject(rr), current); err != nil {
				return ""
			}
			return current.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseCancelling))

		By("marking notification delivery terminal and child stop verified")
		Expect(apiReader.Get(ctx, client.ObjectKeyFromObject(notification), notification)).To(Succeed())
		notification.Status.Phase = notificationv1.NotificationPhaseSent
		Expect(k8sClient.Status().Update(ctx, notification)).To(Succeed())
		Expect(apiReader.Get(ctx, client.ObjectKeyFromObject(wfe), wfe)).To(Succeed())
		wfe.Status.Phase = workflowexecutionv1.PhaseCancelled
		cancellation.SetAcknowledged(&wfe.Status.Conditions, wfe.Generation, "backend termination verified by deployed journey")
		Expect(k8sClient.Status().Update(ctx, wfe)).To(Succeed())

		By("waiting for the deployed RO to complete cancellation and remove its finalizer")
		Eventually(func() bool {
			current := &remediationv1.RemediationRequest{}
			if err := apiReader.Get(ctx, client.ObjectKeyFromObject(rr), current); err != nil {
				return false
			}
			return current.Status.OverallPhase == remediationv1.PhaseCancelled &&
				current.Status.Cancellation.State == remediationv1.CancellationStateCompleted &&
				!controllerutil.ContainsFinalizer(current, e2eCancellationFinalizer)
		}, timeout, interval).Should(BeTrue())

		var finalRR remediationv1.RemediationRequest
		Expect(apiReader.Get(ctx, client.ObjectKeyFromObject(rr), &finalRR)).To(Succeed())
		Expect(finalRR.Status.EnsureCompletionStatus().NotificationStatus).To(Equal("Sent"))
		notificationCondition := meta.FindStatusCondition(finalRR.Status.Conditions, "NotificationDelivered")
		Expect(notificationCondition).NotTo(BeNil())
		Expect(notificationCondition.Status).To(Equal(metav1.ConditionTrue))

		By("verifying cancellation lifecycle audit correlation")
		Expect(auditClient).NotTo(BeNil())
		Eventually(func() bool {
			response, err := auditClient.QueryAuditEvents(ctx, ogenAuditQuery(rrName))
			return err == nil && len(response.Data) > 0
		}, timeout, interval).Should(BeTrue(), "cancellation audit must be queryable by RR correlation ID")
	})
})

func ogenAuditQuery(correlationID string) ogenclient.QueryAuditEventsParams {
	return ogenclient.QueryAuditEventsParams{
		CorrelationID: ogenclient.NewOptString(correlationID),
		EventCategory: ogenclient.NewOptString(roaudit.EventCategoryOrchestration),
	}
}

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

// Issue #2588 integration coverage.
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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	apifrontendtools "github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	roaudit "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/audit"
	"github.com/jordigilh/kubernaut/pkg/shared/cancellation"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

const cancellationFinalizer = "kubernaut.ai/remediationrequest-cancellation"

var _ = Describe("Issue #2588 RemediationRequest cancellation", Label("integration", "cancellation", "issue-2588"), func() {
	var targetNamespace string

	BeforeEach(func() {
		targetNamespace = createTestNamespace(ctx, "ro-cancel-2588")
	})

	AfterEach(func() {
		deleteTestNamespace(targetNamespace)
	})

	It("IT-ORCH-2588-001: production cancel API dispatches an RR into the cancellation path", func() {
		rrName := fmt.Sprintf("rr-api-cancel-%s", uuid.NewString()[:8])
		rr := createRemediationRequest(targetNamespace, rrName)

		result, err := apifrontendtools.HandleCancelRemediation(ctx, k8sClient, apifrontendtools.CancelRemediationArgs{
			RRID:      rrName,
			Namespace: rr.Namespace,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal(string(remediationv1.PhaseCancelling)))

		var current remediationv1.RemediationRequest
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), &current)
		}, timeout, interval).Should(Succeed())
		Expect(current.Status.OverallPhase).To(Equal(remediationv1.PhaseCancelling))
		Expect(current.Status.Cancellation).NotTo(BeNil())
		Expect(current.Status.Cancellation.RequestID).To(Equal(rr.Namespace + "/" + rr.Name + ":cancellation"))
		Expect(current.Finalizers).To(ContainElement(cancellationFinalizer))
	})

	It("IT-ORCH-2588-002 / IT-NT-2588-001 / IT-AUDIT-2588-001: waits for verified child stop and terminal notification before cancelling", func() {
		rrName := fmt.Sprintf("rr-cancel-%s", uuid.NewString()[:8])
		rr := createRemediationRequest(targetNamespace, rrName)
		wfeName := "wfe-" + rrName
		wfe := newCancellationWorkflowExecution(rr, wfeName)
		Expect(controllerutil.SetControllerReference(rr, wfe, k8sClient.Scheme())).To(Succeed())
		Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

		By("marking the child as cancellation-observed but not stopped")
		wfe.Status.Phase = workflowexecutionv1.PhaseCancelling
		cancellation.SetRequested(&wfe.Status.Conditions, wfe.Generation, "backend termination is still pending")
		Expect(k8sClient.Status().Update(ctx, wfe)).To(Succeed())

		By("requesting cancellation through the parent status contract")
		setRemediationCancellation(rrName, wfeName, remediationv1.CancellationSourceSystem)

		var notification notificationv1.NotificationRequest
		Eventually(func() bool {
			var current remediationv1.RemediationRequest
			if err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), &current); err != nil {
				return false
			}
			if current.Status.OverallPhase != remediationv1.PhaseCancelling ||
				!controllerutil.ContainsFinalizer(&current, cancellationFinalizer) {
				return false
			}
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name:      "nr-cancellation-" + rrName,
				Namespace: ROControllerNamespace,
			}, &notification) == nil
		}, timeout, interval).Should(BeTrue(), "RO must retain the parent and create the cancellation notification")

		Expect(notification.Spec.Type).To(Equal(notificationv1.NotificationTypeCancellation))
		Expect(notification.Spec.RemediationRequestRef).NotTo(BeNil())
		Expect(notification.Spec.RemediationRequestRef.Name).To(Equal(rrName))
		Expect(notification.Spec.RemediationRequestRef.UID).To(Equal(rr.UID))
		Expect(notification.Spec.RetryPolicy).NotTo(BeNil())
		Expect(notification.Spec.RetryPolicy.MaxAttempts).To(BeNumerically(">", 0))

		By("proving that a pending child prevents terminal parent state")
		var pending remediationv1.RemediationRequest
		Expect(k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), &pending)).To(Succeed())
		Expect(pending.Status.OverallPhase).To(Equal(remediationv1.PhaseCancelling))
		Expect(pending.Status.Cancellation.State).To(Equal(remediationv1.CancellationStateInProgress))
		Expect(pending.Finalizers).To(ContainElement(cancellationFinalizer))

		By("marking notification delivery terminal and child backend termination verified")
		Expect(k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(&notification), &notification)).To(Succeed())
		notification.Status.Phase = notificationv1.NotificationPhaseFailed
		notification.Status.Message = "test terminal delivery failure"
		Expect(k8sClient.Status().Update(ctx, &notification)).To(Succeed())

		Expect(k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(wfe), wfe)).To(Succeed())
		cancellation.SetAcknowledged(&wfe.Status.Conditions, wfe.Generation, "backend termination verified")
		Expect(k8sClient.Status().Update(ctx, wfe)).To(Succeed())

		By("waiting for RO to complete the aggregate cancellation")
		Eventually(func() bool {
			var current remediationv1.RemediationRequest
			if err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), &current); err != nil {
				return false
			}
			return current.Status.OverallPhase == remediationv1.PhaseCancelled &&
				current.Status.Cancellation.State == remediationv1.CancellationStateCompleted &&
				!controllerutil.ContainsFinalizer(&current, cancellationFinalizer) &&
				current.Status.EnsureCompletionStatus().NotificationStatus == "Failed"
		}, timeout, interval).Should(BeTrue(), "parent cancellation must wait for child acknowledgement and terminal notification delivery")

		var cancelled remediationv1.RemediationRequest
		Expect(k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), &cancelled)).To(Succeed())
		deliveryCondition := meta.FindStatusCondition(cancelled.Status.Conditions, "NotificationDelivered")
		Expect(deliveryCondition).NotTo(BeNil())
		Expect(deliveryCondition.Status).To(Equal(metav1.ConditionFalse))
		Expect(deliveryCondition.Reason).To(Equal("DeliveryFailed"))

		By("reconstructing the cancellation transitions by correlation ID")
		Expect(auditStore.Flush(ctx)).To(Succeed())
		var events []ogenAuditEvent
		Eventually(func() bool {
			events = queryAuditEventsOpenAPI(dsClients.OpenAPIClient, rrName, roaudit.EventTypeLifecycleTransitioned)
			return hasCancellationTransitions(events)
		}, 30*time.Second, 2*time.Second).Should(BeTrue(), "audit correlation must contain Cancelling and Cancelled transitions")
		Expect(hasCancellationTransition(events, string(remediationv1.PhaseCancelling))).To(BeTrue())
		Expect(hasCancellationTransition(events, string(remediationv1.PhaseCancelled))).To(BeTrue())
	})

	It("IT-ORCH-2588-003: retains the deletion finalizer until child acknowledgement and notification terminality", func() {
		rrName := fmt.Sprintf("rr-delete-cancel-%s", uuid.NewString()[:8])
		rr := createRemediationRequest(targetNamespace, rrName)
		Eventually(func() bool {
			var current remediationv1.RemediationRequest
			if err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), &current); err != nil {
				return false
			}
			return controllerutil.ContainsFinalizer(&current, cancellationFinalizer)
		}, timeout, interval).Should(BeTrue(), "RO must install the cancellation finalizer before deletion can be requested")
		wfeName := "wfe-" + rrName
		wfe := newCancellationWorkflowExecution(rr, wfeName)
		Expect(controllerutil.SetControllerReference(rr, wfe, k8sClient.Scheme())).To(Succeed())
		Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

		wfe.Status.Phase = workflowexecutionv1.PhaseCancelling
		cancellation.SetRequested(&wfe.Status.Conditions, wfe.Generation, "backend termination is still pending")
		Expect(k8sClient.Status().Update(ctx, wfe)).To(Succeed())
		setRemediationCancellation(rrName, wfeName, remediationv1.CancellationSourceDeletion)

		By("deleting the parent while cancellation is still in progress")
		Expect(k8sClient.Delete(ctx, rr)).To(Succeed())

		var notification notificationv1.NotificationRequest
		Eventually(func() bool {
			var current remediationv1.RemediationRequest
			if err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), &current); err != nil {
				return false
			}
			if current.DeletionTimestamp.IsZero() || !controllerutil.ContainsFinalizer(&current, cancellationFinalizer) ||
				current.Status.OverallPhase != remediationv1.PhaseCancelling {
				return false
			}
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name:      "nr-cancellation-" + rrName,
				Namespace: ROControllerNamespace,
			}, &notification) == nil
		}, timeout, interval).Should(BeTrue(), "deletionTimestamp cancellation must retain the parent finalizer")

		By("verifying the finalizer blocks deletion while the child remains unacknowledged")
		var deleting remediationv1.RemediationRequest
		Expect(k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), &deleting)).To(Succeed())
		Expect(deleting.DeletionTimestamp.IsZero()).To(BeFalse())
		Expect(deleting.Finalizers).To(ContainElement(cancellationFinalizer))

		notification.Status.Phase = notificationv1.NotificationPhaseSent
		Expect(k8sClient.Status().Update(ctx, &notification)).To(Succeed())
		var current remediationv1.RemediationRequest
		Expect(k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), &current)).To(Succeed())
		if ref := current.Status.GetPhaseProgress().SignalProcessingRef; ref != nil {
			By("acknowledging a SignalProcessing child created before deletion")
			sp := &signalprocessingv1.SignalProcessing{}
			namespace := ref.Namespace
			if namespace == "" {
				namespace = current.Namespace
			}
			Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name:      ref.Name,
				Namespace: namespace,
			}, sp)).To(Succeed())
			cancellation.SetAcknowledged(&sp.Status.Conditions, sp.Generation, "backend termination verified")
			Expect(k8sClient.Status().Update(ctx, sp)).To(Succeed())
		}
		Expect(k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(wfe), wfe)).To(Succeed())
		cancellation.SetAcknowledged(&wfe.Status.Conditions, wfe.Generation, "backend termination verified")
		Expect(k8sClient.Status().Update(ctx, wfe)).To(Succeed())

		Eventually(func() bool {
			var current remediationv1.RemediationRequest
			err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), &current)
			return apierrors.IsNotFound(err)
		}, timeout, interval).Should(BeTrue(), "parent should be deleted after verified cancellation completion")
	})
})

type ogenAuditEvent = ogenclient.AuditEvent

func newCancellationWorkflowExecution(rr *remediationv1.RemediationRequest, name string) *workflowexecutionv1.WorkflowExecution {
	return &workflowexecutionv1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ROControllerNamespace},
		Spec: workflowexecutionv1.WorkflowExecutionSpec{
			RemediationRequestRef: corev1.ObjectReference{
				APIVersion: remediationv1.GroupVersion.String(),
				Kind:       "RemediationRequest",
				Name:       rr.Name,
				Namespace:  rr.Namespace,
				UID:        rr.UID,
			},
			WorkflowRef: workflowexecutionv1.WorkflowRef{WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
				WorkflowID:      "cancel-test-workflow",
				WorkflowName:    "cancel-test-workflow",
				ActionType:      "RestartPod",
				Version:         "v1",
				ExecutionBundle: "ghcr.io/kubernaut/workflows/cancel-test:v1",
			}},
			TargetResource: fmt.Sprintf("%s/deployment/test-app", targetNamespaceFor(rr)),
		},
	}
}

func targetNamespaceFor(rr *remediationv1.RemediationRequest) string {
	return rr.Spec.TargetResource.Namespace
}

func setRemediationCancellation(rrName, wfeName string, source remediationv1.CancellationSource) {
	Eventually(func() error {
		var current remediationv1.RemediationRequest
		if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rrName, Namespace: ROControllerNamespace}, &current); err != nil {
			return err
		}
		current.Status.OverallPhase = remediationv1.PhaseCancelling
		cancellationStatus := current.Status.EnsureCancellation()
		cancellationStatus.RequestID = current.Namespace + "/" + current.Name + ":cancellation"
		now := metav1.Now()
		cancellationStatus.RequestedAt = &now
		cancellationStatus.RequestedBy = "system"
		cancellationStatus.Source = source
		cancellationStatus.Reason = "issue 2588 integration cancellation"
		cancellationStatus.State = remediationv1.CancellationStateRequested
		current.Status.EnsurePhaseProgress().WorkflowExecutionRef = &corev1.ObjectReference{
			APIVersion: workflowexecutionv1.GroupVersion.String(),
			Kind:       "WorkflowExecution",
			Name:       wfeName,
			Namespace:  current.Namespace,
		}
		return k8sClient.Status().Update(ctx, &current)
	}, timeout, interval).Should(Succeed())
}

func hasCancellationTransitions(events []ogenAuditEvent) bool {
	return hasCancellationTransition(events, string(remediationv1.PhaseCancelling)) &&
		hasCancellationTransition(events, string(remediationv1.PhaseCancelled))
}

func hasCancellationTransition(events []ogenAuditEvent, toPhase string) bool {
	for _, event := range events {
		payload, ok := event.EventData.GetRemediationOrchestratorAuditPayload()
		if ok && payload.ToPhase.Value == toPhase {
			return true
		}
	}
	return false
}

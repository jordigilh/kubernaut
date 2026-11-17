package workflowexecution

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/cancellation"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
)

type cancellationExecutor struct {
	verification executor.StopVerification
	stopCalls    int
	stopErr      error
	createErr    error
}

func (e *cancellationExecutor) Engine() string { return "job" }

func (e *cancellationExecutor) Create(context.Context, *workflowexecutionv1.WorkflowExecution, string, executor.CreateOptions) (*executor.CreateResult, error) {
	if e.createErr != nil {
		return nil, e.createErr
	}
	return &executor.CreateResult{ResourceName: "job-1"}, nil
}

func (e *cancellationExecutor) GetStatus(context.Context, *workflowexecutionv1.WorkflowExecution, string) (*executor.ExecutionResult, error) {
	return &executor.ExecutionResult{Phase: workflowexecutionv1.PhaseRunning}, nil
}

func (e *cancellationExecutor) Cleanup(context.Context, *workflowexecutionv1.WorkflowExecution, string) error {
	return nil
}

func (e *cancellationExecutor) RequestStop(context.Context, *workflowexecutionv1.WorkflowExecution, string) error {
	e.stopCalls++
	return e.stopErr
}

func (e *cancellationExecutor) VerifyStopped(context.Context, *workflowexecutionv1.WorkflowExecution, string) (executor.StopVerification, error) {
	return e.verification, nil
}

var _ = Describe("WorkflowExecution parent cancellation", func() {
	It("UT-WE-2588-022: does not classify an ownerless missing-parent WFE as cancelled", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())
		wfe := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{Name: "wfe-synthetic", Namespace: "system"},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: "missing", Namespace: "system"},
				WorkflowRef: workflowexecutionv1.WorkflowRef{WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
					ExecutionEngine: "job",
				}},
			},
			Status: workflowexecutionv1.WorkflowExecutionStatus{Phase: workflowexecutionv1.PhasePending},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wfe).Build()
		reconciler := &WorkflowExecutionReconciler{Client: c}

		stopRequested, err := reconciler.parentCancellationRequested(context.Background(), wfe)
		Expect(err).NotTo(HaveOccurred())
		Expect(stopRequested).To(BeFalse())
	})

	It("UT-WE-2588-023: treats an owned missing-parent WFE as an orphan stop request", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())
		controller := true
		wfe := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "wfe-orphan",
				Namespace: "system",
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: remediationv1.GroupVersion.String(),
					Kind:       "RemediationRequest",
					Name:       "missing",
					UID:        types.UID("missing-uid"),
					Controller: &controller,
				}},
			},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: "missing", Namespace: "system", UID: types.UID("missing-uid")},
				WorkflowRef: workflowexecutionv1.WorkflowRef{WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
					ExecutionEngine: "job",
				}},
			},
			Status: workflowexecutionv1.WorkflowExecutionStatus{Phase: workflowexecutionv1.PhasePending},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wfe).Build()
		reconciler := &WorkflowExecutionReconciler{Client: c}

		stopRequested, err := reconciler.parentCancellationRequested(context.Background(), wfe)
		Expect(err).NotTo(HaveOccurred())
		Expect(stopRequested).To(BeTrue())
	})

	It("UT-WE-2588-018: keeps an AWX recovery miss Pending and stops after the bounded discovery budget", func() {
		scheme := runtime.NewScheme()
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())
		wfe := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{Name: "wfe-awx-recovery-pending", Namespace: "system"},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				WorkflowRef: workflowexecutionv1.WorkflowRef{WorkflowSnapshot: sharedtypes.WorkflowSnapshot{ExecutionEngine: "job"}},
			},
			Status: workflowexecutionv1.WorkflowExecutionStatus{Phase: workflowexecutionv1.PhasePending},
		}
		executorMock := &cancellationExecutor{
			createErr: fmt.Errorf("%w: no AWX job matches the persisted label", executor.ErrAWXLaunchRecoveryPending),
		}
		registry := executor.NewRegistry()
		registry.Register("job", executorMock)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(wfe).
			WithObjects(wfe).
			Build()
		reconciler := &WorkflowExecutionReconciler{
			Client:           c,
			ExecutorRegistry: registry,
		}

		for attempt := int32(1); attempt <= awxLaunchRecoveryMaxRetries; attempt++ {
			_, result, err, shouldReturn := reconciler.createPendingExecutionResource(
				context.Background(), wfe, "job-1", executor.CreateOptions{}, logr.Discard())
			Expect(shouldReturn).To(BeTrue())
			Expect(err).NotTo(HaveOccurred())
			Expect(wfe.Status.AWXLaunchRecoveryRetryCount).To(Equal(attempt))
			if attempt < awxLaunchRecoveryMaxRetries {
				Expect(result.RequeueAfter).To(Equal(awxLaunchRecoveryRequeue))
			} else {
				Expect(result).To(Equal(ctrl.Result{}))
			}
		}

		var got workflowexecutionv1.WorkflowExecution
		Expect(c.Get(context.Background(), client.ObjectKeyFromObject(wfe), &got)).To(Succeed())
		Expect(got.Status.Phase).To(Equal(workflowexecutionv1.PhasePending))
		Expect(got.Status.Conditions).NotTo(BeEmpty())
		Expect(got.Status.Conditions[len(got.Status.Conditions)-1].Reason).To(Equal("AWXLaunchRecoveryExhausted"))
	})

	It("UT-WE-2588-008: preserves a terminal execution outcome while acknowledging cancellation", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())

		parent := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-terminal", Namespace: "system", UID: types.UID("rr-terminal-uid")},
			Status:     remediationv1.RemediationRequestStatus{OverallPhase: remediationv1.PhaseCancelling},
		}
		wfe := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{Name: "wfe-terminal", Namespace: "system"},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: parent.Name, Namespace: parent.Namespace, UID: parent.UID},
				WorkflowRef: workflowexecutionv1.WorkflowRef{WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
					ExecutionEngine: "job",
				}},
			},
			Status: workflowexecutionv1.WorkflowExecutionStatus{Phase: workflowexecutionv1.PhaseCompleted},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(parent, wfe).
			WithObjects(parent, wfe).Build()
		reconciler := &WorkflowExecutionReconciler{Client: c}

		_, handled, err := reconciler.reconcileParentCancellation(context.Background(), wfe, logr.Discard())
		Expect(err).NotTo(HaveOccurred())
		Expect(handled).To(BeTrue())
		Expect(wfe.Status.Phase).To(Equal(workflowexecutionv1.PhaseCompleted))
		Expect(cancellation.IsAcknowledged(wfe.Status.Conditions)).To(BeTrue())
	})

	It("UT-WE-2588-021: verifies a terminal backend before acknowledging parent cancellation", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())

		parent := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-terminal-verify", Namespace: "system", UID: types.UID("rr-terminal-verify-uid")},
			Status:     remediationv1.RemediationRequestStatus{OverallPhase: remediationv1.PhaseCancelling},
		}
		wfe := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{Name: "wfe-terminal-verify", Namespace: "system"},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: parent.Name, Namespace: parent.Namespace, UID: parent.UID},
				WorkflowRef:           workflowexecutionv1.WorkflowRef{WorkflowSnapshot: sharedtypes.WorkflowSnapshot{ExecutionEngine: "job"}},
			},
			Status: workflowexecutionv1.WorkflowExecutionStatus{
				Phase:        workflowexecutionv1.PhaseCompleted,
				ExecutionRef: &corev1.LocalObjectReference{Name: "job-1"},
			},
		}
		executorMock := &cancellationExecutor{verification: executor.StopVerification{State: executor.StopStateRunning, Message: "job still active"}}
		registry := executor.NewRegistry()
		registry.Register("job", executorMock)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(parent, wfe).
			WithObjects(parent, wfe).
			Build()
		reconciler := &WorkflowExecutionReconciler{Client: c, ExecutorRegistry: registry, ExecutionNamespace: "system"}

		result, handled, err := reconciler.reconcileParentCancellation(context.Background(), wfe, logr.Discard())
		Expect(err).NotTo(HaveOccurred())
		Expect(handled).To(BeTrue())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))
		Expect(wfe.Status.Phase).To(Equal(workflowexecutionv1.PhaseCompleted))
		Expect(cancellation.IsAcknowledged(wfe.Status.Conditions)).To(BeFalse())
		Expect(executorMock.stopCalls).To(Equal(1))

		executorMock.verification = executor.StopVerification{State: executor.StopStateStopped, Message: "job termination verified"}
		_, handled, err = reconciler.reconcileParentCancellation(context.Background(), wfe, logr.Discard())
		Expect(err).NotTo(HaveOccurred())
		Expect(handled).To(BeTrue())
		Expect(wfe.Status.Phase).To(Equal(workflowexecutionv1.PhaseCompleted))
		Expect(cancellation.IsAcknowledged(wfe.Status.Conditions)).To(BeTrue())
		Expect(executorMock.stopCalls).To(Equal(2))
	})

	It("IT-WE-2588-001: does not acknowledge until backend termination is verified", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())

		parent := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-1", Namespace: "system", UID: types.UID("rr-1-uid")},
			Status:     remediationv1.RemediationRequestStatus{OverallPhase: remediationv1.PhaseCancelling},
		}
		wfe := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{Name: "wfe-rr-1", Namespace: "system"},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: "rr-1", Namespace: "system", UID: parent.UID},
				WorkflowRef: workflowexecutionv1.WorkflowRef{WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
					ExecutionEngine: "job",
				}},
			},
			Status: workflowexecutionv1.WorkflowExecutionStatus{Phase: workflowexecutionv1.PhaseRunning},
		}
		executorMock := &cancellationExecutor{verification: executor.StopVerification{State: executor.StopStateRunning, Message: "job still active"}}
		registry := executor.NewRegistry()
		registry.Register("job", executorMock)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(parent, wfe).
			WithObjects(parent, wfe).Build()
		reconciler := &WorkflowExecutionReconciler{Client: c, ExecutorRegistry: registry, ExecutionNamespace: "system"}

		_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: wfe.Name, Namespace: wfe.Namespace}})
		Expect(err).NotTo(HaveOccurred())
		var got workflowexecutionv1.WorkflowExecution
		Expect(c.Get(context.Background(), client.ObjectKeyFromObject(wfe), &got)).To(Succeed())
		Expect(got.Status.Phase).To(Equal(workflowexecutionv1.PhaseCancelling))
		Expect(cancellation.IsAcknowledged(got.Status.Conditions)).To(BeFalse())

		executorMock.verification = executor.StopVerification{State: executor.StopStateStopped, Message: "job termination verified"}
		_, err = reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: wfe.Name, Namespace: wfe.Namespace}})
		Expect(err).NotTo(HaveOccurred())
		Expect(c.Get(context.Background(), client.ObjectKeyFromObject(wfe), &got)).To(Succeed())
		Expect(got.Status.Phase).To(Equal(workflowexecutionv1.PhaseCancelled))
		Expect(cancellation.IsAcknowledged(got.Status.Conditions)).To(BeTrue())
		Expect(executorMock.stopCalls).To(Equal(2))
	})

	It("UT-WE-2588-009: refuses cancellation for a same-name parent with a stale UID", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())

		parent := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-current", Namespace: "system", UID: types.UID("rr-current-uid")},
			Status:     remediationv1.RemediationRequestStatus{OverallPhase: remediationv1.PhaseCancelling},
		}
		wfe := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{Name: "wfe-rr-current", Namespace: "system"},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: parent.Name, Namespace: parent.Namespace, UID: types.UID("rr-old-uid")},
				WorkflowRef:           workflowexecutionv1.WorkflowRef{WorkflowSnapshot: sharedtypes.WorkflowSnapshot{ExecutionEngine: "job"}},
			},
			Status: workflowexecutionv1.WorkflowExecutionStatus{Phase: workflowexecutionv1.PhaseRunning},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(parent, wfe).
			WithObjects(parent, wfe).Build()
		reconciler := &WorkflowExecutionReconciler{Client: c}

		_, handled, err := reconciler.reconcileParentCancellation(context.Background(), wfe, logr.Discard())
		Expect(err).To(MatchError(ContainSubstring("identity mismatch")))
		Expect(handled).To(BeFalse())
		Expect(wfe.Status.Phase).To(Equal(workflowexecutionv1.PhaseRunning))
		Expect(cancellation.IsAcknowledged(wfe.Status.Conditions)).To(BeFalse())
	})

	It("IT-WE-2588-002: verifies a deterministic backend before acknowledging a pending WFE without an ExecutionRef", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())

		parent := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-pending-backend", Namespace: "system", UID: types.UID("rr-pending-backend-uid")},
			Status:     remediationv1.RemediationRequestStatus{OverallPhase: remediationv1.PhaseCancelling},
		}
		wfe := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{Name: "wfe-pending-backend", Namespace: "system"},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: parent.Name, Namespace: parent.Namespace, UID: parent.UID},
				WorkflowRef: workflowexecutionv1.WorkflowRef{WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
					ExecutionEngine: "job",
				}},
			},
			Status: workflowexecutionv1.WorkflowExecutionStatus{Phase: workflowexecutionv1.PhasePending},
		}
		executorMock := &cancellationExecutor{verification: executor.StopVerification{
			State:   executor.StopStateStopped,
			Message: "deterministic backend is absent or already terminated",
		}}
		registry := executor.NewRegistry()
		registry.Register("job", executorMock)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(parent, wfe).
			WithObjects(parent, wfe).Build()
		reconciler := &WorkflowExecutionReconciler{Client: c, ExecutorRegistry: registry, ExecutionNamespace: "system"}

		_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: wfe.Name, Namespace: wfe.Namespace}})
		Expect(err).NotTo(HaveOccurred())
		Expect(executorMock.stopCalls).To(Equal(1), "cancellation must verify the backend instead of treating nil ExecutionRef as proof of absence")

		var got workflowexecutionv1.WorkflowExecution
		Expect(c.Get(context.Background(), client.ObjectKeyFromObject(wfe), &got)).To(Succeed())
		Expect(got.Status.Phase).To(Equal(workflowexecutionv1.PhaseCancelled))
		Expect(cancellation.IsAcknowledged(got.Status.Conditions)).To(BeTrue())
	})

	It("UT-WE-2588-003: persists terminal cancellation failure and stops requeue after the retry budget", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())

		parent := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-retry-exhausted", Namespace: "system", UID: types.UID("rr-retry-exhausted-uid")},
			Status:     remediationv1.RemediationRequestStatus{OverallPhase: remediationv1.PhaseCancelling},
		}
		wfe := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{Name: "wfe-retry-exhausted", Namespace: "system"},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: parent.Name, Namespace: parent.Namespace, UID: parent.UID},
				WorkflowRef:           workflowexecutionv1.WorkflowRef{WorkflowSnapshot: sharedtypes.WorkflowSnapshot{ExecutionEngine: "job"}},
			},
			Status: workflowexecutionv1.WorkflowExecutionStatus{
				Phase:                  workflowexecutionv1.PhaseRunning,
				CancellationRetryCount: workflowExecutionCancellationMaxRetries - 1,
			},
		}
		executorMock := &cancellationExecutor{verification: executor.StopVerification{State: executor.StopStateRunning, Message: "job still active"}}
		registry := executor.NewRegistry()
		registry.Register("job", executorMock)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(parent, wfe).
			WithObjects(parent, wfe).Build()
		reconciler := &WorkflowExecutionReconciler{Client: c, ExecutorRegistry: registry, ExecutionNamespace: "system"}

		result, handled, err := reconciler.reconcileParentCancellation(context.Background(), wfe, logr.Discard())
		Expect(err).NotTo(HaveOccurred())
		Expect(handled).To(BeTrue())
		Expect(result).To(Equal(ctrl.Result{}))
		Expect(wfe.Status.CancellationRetryCount).To(Equal(workflowExecutionCancellationMaxRetries))
		Expect(cancellation.IsFailed(wfe.Status.Conditions)).To(BeTrue())
		Expect(executorMock.stopCalls).To(Equal(1))

		result, handled, err = reconciler.reconcileParentCancellation(context.Background(), wfe, logr.Discard())
		Expect(err).NotTo(HaveOccurred())
		Expect(handled).To(BeTrue())
		Expect(result).To(Equal(ctrl.Result{}))
		Expect(executorMock.stopCalls).To(Equal(1), "an exhausted cancellation must not call the backend again")
	})

	It("UT-WE-2588-007: bounds executor lookup failures with the cancellation retry budget", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())

		parent := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-missing-executor", Namespace: "system", UID: types.UID("rr-missing-executor-uid")},
			Status:     remediationv1.RemediationRequestStatus{OverallPhase: remediationv1.PhaseCancelling},
		}
		wfe := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{Name: "wfe-missing-executor", Namespace: "system"},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: parent.Name, Namespace: parent.Namespace, UID: parent.UID},
				WorkflowRef:           workflowexecutionv1.WorkflowRef{WorkflowSnapshot: sharedtypes.WorkflowSnapshot{ExecutionEngine: "job"}},
			},
			Status: workflowexecutionv1.WorkflowExecutionStatus{Phase: workflowexecutionv1.PhaseRunning},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(parent, wfe).
			WithObjects(parent, wfe).Build()
		reconciler := &WorkflowExecutionReconciler{Client: c, ExecutionNamespace: "system"}

		for attempt := int32(1); attempt <= workflowExecutionCancellationMaxRetries; attempt++ {
			result, handled, err := reconciler.reconcileParentCancellation(context.Background(), wfe, logr.Discard())
			Expect(err).NotTo(HaveOccurred())
			Expect(handled).To(BeTrue())
			if attempt < workflowExecutionCancellationMaxRetries {
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))
			} else {
				Expect(result).To(Equal(ctrl.Result{}))
			}
		}

		Expect(wfe.Status.CancellationRetryCount).To(Equal(workflowExecutionCancellationMaxRetries))
		Expect(cancellation.IsFailed(wfe.Status.Conditions)).To(BeTrue())
		var got workflowexecutionv1.WorkflowExecution
		Expect(c.Get(context.Background(), client.ObjectKeyFromObject(wfe), &got)).To(Succeed())
		Expect(got.Status.CancellationRetryCount).To(Equal(workflowExecutionCancellationMaxRetries))

		result, handled, err := reconciler.reconcileParentCancellation(context.Background(), wfe, logr.Discard())
		Expect(err).NotTo(HaveOccurred())
		Expect(handled).To(BeTrue())
		Expect(result).To(Equal(ctrl.Result{}))
		Expect(wfe.Status.CancellationRetryCount).To(Equal(workflowExecutionCancellationMaxRetries))
	})

	It("UT-WE-2588-005: does not acknowledge terminal deletion until backend termination is verified", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())

		parent := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-terminal-delete-active", Namespace: "system", UID: types.UID("rr-terminal-delete-active-uid")},
			Status:     remediationv1.RemediationRequestStatus{OverallPhase: remediationv1.PhaseCancelling},
		}
		deletionTime := metav1.Now()
		wfe := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "wfe-terminal-delete-active",
				Namespace:         "system",
				DeletionTimestamp: &deletionTime,
				Finalizers:        []string{FinalizerName},
			},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: parent.Name, Namespace: parent.Namespace, UID: parent.UID},
				WorkflowRef:           workflowexecutionv1.WorkflowRef{WorkflowSnapshot: sharedtypes.WorkflowSnapshot{ExecutionEngine: "job"}},
			},
			Status: workflowexecutionv1.WorkflowExecutionStatus{Phase: workflowexecutionv1.PhaseCompleted},
		}
		executorMock := &cancellationExecutor{verification: executor.StopVerification{State: executor.StopStateRunning, Message: "job still active"}}
		registry := executor.NewRegistry()
		registry.Register("job", executorMock)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(parent, wfe).
			WithObjects(parent, wfe).Build()
		reconciler := &WorkflowExecutionReconciler{Client: c, ExecutorRegistry: registry, ExecutionNamespace: "system"}

		result, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: wfe.Name, Namespace: wfe.Namespace}})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))
		Expect(executorMock.stopCalls).To(Equal(1))

		var got workflowexecutionv1.WorkflowExecution
		Expect(c.Get(context.Background(), client.ObjectKeyFromObject(wfe), &got)).To(Succeed())
		Expect(got.Status.Phase).To(Equal(workflowexecutionv1.PhaseCompleted))
		Expect(cancellation.IsAcknowledged(got.Status.Conditions)).To(BeFalse())
		Expect(got.Finalizers).To(ContainElement(FinalizerName))
	})

	It("UT-WE-2588-006: counts each failed backend stop reconcile once", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())

		parent := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-stop-error-budget", Namespace: "system", UID: types.UID("rr-stop-error-budget-uid")},
			Status:     remediationv1.RemediationRequestStatus{OverallPhase: remediationv1.PhaseCancelling},
		}
		wfe := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{Name: "wfe-stop-error-budget", Namespace: "system"},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: parent.Name, Namespace: parent.Namespace, UID: parent.UID},
				WorkflowRef:           workflowexecutionv1.WorkflowRef{WorkflowSnapshot: sharedtypes.WorkflowSnapshot{ExecutionEngine: "job"}},
			},
			Status: workflowexecutionv1.WorkflowExecutionStatus{Phase: workflowexecutionv1.PhaseRunning},
		}
		executorMock := &cancellationExecutor{stopErr: errors.New("backend stop request failed")}
		registry := executor.NewRegistry()
		registry.Register("job", executorMock)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(parent, wfe).
			WithObjects(parent, wfe).Build()
		reconciler := &WorkflowExecutionReconciler{Client: c, ExecutorRegistry: registry, ExecutionNamespace: "system"}

		for attempt := int32(1); attempt <= workflowExecutionCancellationMaxRetries; attempt++ {
			result, handled, err := reconciler.reconcileParentCancellation(context.Background(), wfe, logr.Discard())
			Expect(handled).To(BeTrue())
			Expect(executorMock.stopCalls).To(Equal(int(attempt)), "attempt %d returned %v", attempt, err)
			if attempt < workflowExecutionCancellationMaxRetries {
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))
			} else {
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))
			}
		}

		Expect(executorMock.stopCalls).To(Equal(int(workflowExecutionCancellationMaxRetries)))
		Expect(wfe.Status.CancellationRetryCount).To(Equal(workflowExecutionCancellationMaxRetries))
		Expect(cancellation.IsFailed(wfe.Status.Conditions)).To(BeTrue())

		_, handled, err := reconciler.reconcileParentCancellation(context.Background(), wfe, logr.Discard())
		Expect(err).NotTo(HaveOccurred())
		Expect(handled).To(BeTrue())
		Expect(executorMock.stopCalls).To(Equal(int(workflowExecutionCancellationMaxRetries)))
	})

	It("UT-WE-2588-004: stops deletion-finalizer retries after persisting cancellation failure", func() {
		scheme := runtime.NewScheme()
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())

		deletionTime := metav1.Now()
		wfe := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "wfe-delete-retry-exhausted",
				Namespace:         "system",
				DeletionTimestamp: &deletionTime,
				Finalizers:        []string{FinalizerName},
			},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				WorkflowRef: workflowexecutionv1.WorkflowRef{WorkflowSnapshot: sharedtypes.WorkflowSnapshot{ExecutionEngine: "job"}},
			},
			Status: workflowexecutionv1.WorkflowExecutionStatus{
				Phase:                  workflowexecutionv1.PhaseCancelling,
				CancellationRetryCount: workflowExecutionCancellationMaxRetries - 1,
			},
		}
		executorMock := &cancellationExecutor{verification: executor.StopVerification{State: executor.StopStateRunning, Message: "job still active"}}
		registry := executor.NewRegistry()
		registry.Register("job", executorMock)
		c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(wfe).WithObjects(wfe).Build()
		reconciler := &WorkflowExecutionReconciler{Client: c, ExecutorRegistry: registry, ExecutionNamespace: "system"}

		result, handled, err := reconciler.reconcileDeleteExecutionStop(context.Background(), wfe, logr.Discard())
		Expect(err).NotTo(HaveOccurred())
		Expect(handled).To(BeTrue())
		Expect(result).To(Equal(ctrl.Result{}))
		Expect(wfe.Status.CancellationRetryCount).To(Equal(workflowExecutionCancellationMaxRetries))
		Expect(cancellation.IsFailed(wfe.Status.Conditions)).To(BeTrue())
		Expect(executorMock.stopCalls).To(Equal(1))

		_, handled, err = reconciler.reconcileDeleteExecutionStop(context.Background(), wfe, logr.Discard())
		Expect(err).NotTo(HaveOccurred())
		Expect(handled).To(BeTrue())
		Expect(executorMock.stopCalls).To(Equal(1), "an exhausted deletion cancellation must not call the backend again")
	})

	It("UT-WE-2588-010: schedules deletion stop failures after persisting cancellation status", func() {
		scheme := runtime.NewScheme()
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())

		deletionTime := metav1.Now()
		wfe := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "wfe-delete-stop-error",
				Namespace:         "system",
				DeletionTimestamp: &deletionTime,
				Finalizers:        []string{FinalizerName},
			},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				WorkflowRef: workflowexecutionv1.WorkflowRef{WorkflowSnapshot: sharedtypes.WorkflowSnapshot{ExecutionEngine: "job"}},
			},
			Status: workflowexecutionv1.WorkflowExecutionStatus{Phase: workflowexecutionv1.PhaseCompleted},
		}
		executorMock := &cancellationExecutor{stopErr: errors.New("backend stop request failed")}
		registry := executor.NewRegistry()
		registry.Register("job", executorMock)
		c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(wfe).WithObjects(wfe).Build()
		reconciler := &WorkflowExecutionReconciler{Client: c, ExecutorRegistry: registry, ExecutionNamespace: "system"}

		result, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: wfe.Name, Namespace: wfe.Namespace}})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))

		var got workflowexecutionv1.WorkflowExecution
		Expect(c.Get(context.Background(), client.ObjectKeyFromObject(wfe), &got)).To(Succeed())
		Expect(got.Status.CancellationRetryCount).To(Equal(int32(1)))
		Expect(cancellation.IsFailed(got.Status.Conditions)).To(BeTrue())
		Expect(got.Finalizers).To(ContainElement(FinalizerName))
	})
})

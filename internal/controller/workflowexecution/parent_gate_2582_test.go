package workflowexecution

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

var _ = Describe("WorkflowExecution parent gate", func() {
	It("UT-WE-2582-001: keeps a pre-created WE from dispatching before RR Executing [BR-WE-007, AC-4]", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-precreated-we", Namespace: "default"},
			Status:     remediationv1.RemediationRequestStatus{OverallPhase: remediationv1.PhaseAnalyzing},
		}
		wfe := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{Name: "we-rr-precreated-we", Namespace: "default"},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: rr.Name, Namespace: rr.Namespace},
			},
			Status: workflowexecutionv1.WorkflowExecutionStatus{Phase: workflowexecutionv1.PhasePending},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rr, wfe).WithStatusSubresource(wfe).Build()
		reconciler := &WorkflowExecutionReconciler{Client: client, APIReader: client}

		result, err, handled := reconciler.enforceRemediationParentGate(context.Background(), wfe, log.Log)

		Expect(err).NotTo(HaveOccurred())
		Expect(handled).To(BeTrue())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))
		Expect(wfe.Status.Phase).To(Equal(workflowexecutionv1.PhasePending))
	})
})

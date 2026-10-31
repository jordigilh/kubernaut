package workflowexecution_test

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
)

var _ = Describe("UT-WE-2582-005: AWX cancellation verification [BR-WE-015, AC-4]", func() {
	It("does not treat an accepted cancel request as termination until AWX is terminal", func() {
		status := "running"
		awx := &mockAWXClient{getStatusFunc: func(context.Context, int) (*executor.AWXJobStatus, error) {
			return &executor.AWXJobStatus{ID: 77, Status: status}, nil
		}}
		ansible := executor.NewAnsibleExecutor(awx, nil, nil, 1, logr.Discard())
		wfe := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{Name: "we-awx-cancel", Namespace: "default"},
			Status: workflowexecutionv1.WorkflowExecutionStatus{
				ExecutionRef: &corev1.LocalObjectReference{Name: "awx-job-77"},
			},
		}

		Expect(ansible.RequestCancellation(context.Background(), wfe, "default")).To(Succeed())
		pending, err := ansible.VerifyCancellation(context.Background(), wfe, "default")
		Expect(err).NotTo(HaveOccurred())
		Expect(pending.Terminated).To(BeFalse())

		status = "canceled"
		verified, err := ansible.VerifyCancellation(context.Background(), wfe, "default")
		Expect(err).NotTo(HaveOccurred())
		Expect(verified.Terminated).To(BeTrue())
	})
})

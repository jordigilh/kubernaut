package workflowexecution_test

import (
	"context"
	"sync/atomic"

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

	It("recovers a missing ExecutionRef before cancelling a partially persisted AWX launch", func() {
		var cancelledJobID atomic.Int32
		awx := &recoverableAWXClient{
			mockAWXClient: &mockAWXClient{
				cancelFunc: func(_ context.Context, jobID int) error {
					cancelledJobID.Store(int32(jobID))
					return nil
				},
			},
			recoveredJobID: 77,
		}
		ansible := executor.NewAnsibleExecutor(awx, nil, nil, 1, logr.Discard())
		wfe := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "we-awx-partial-launch",
				Namespace: "default",
				UID:       "wfe-awx-partial-launch",
			},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: "rr-awx-partial-launch", Namespace: "default"},
			},
		}

		Expect(ansible.RequestCancellation(context.Background(), wfe, "default")).To(Succeed())
		Expect(wfe.Status.ExecutionRef).NotTo(BeNil())
		Expect(wfe.Status.ExecutionRef.Name).To(Equal("awx-job-77"))
		Expect(cancelledJobID.Load()).To(Equal(int32(77)))
	})
})

type recoverableAWXClient struct {
	*mockAWXClient
	recoveredJobID int
}

func (c *recoverableAWXClient) FindJobByExecutionIdentity(context.Context, string, string, string) (int, error) {
	return c.recoveredJobID, nil
}

type sequencedRecoverableAWXClient struct {
	*mockAWXClient
	recoveryCalls int
}

func (c *sequencedRecoverableAWXClient) FindJobByExecutionIdentity(context.Context, string, string, string) (int, error) {
	c.recoveryCalls++
	if c.recoveryCalls == 1 {
		return 0, nil
	}
	return 77, nil
}

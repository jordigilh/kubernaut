package executor_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
)

var _ = Describe("UT-WE-2582-003: Job backend cancellation proof [BR-WE-014, AC-4]", func() {
	newWFE := func() *workflowexecutionv1.WorkflowExecution {
		return &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{Name: "we-cancel-2582", Namespace: "default", UID: "wfe-uid-2582"},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: "rr-cancel-2582", Namespace: "default", UID: "rr-uid-2582"},
				TargetResource:        "default/Deployment/api",
				WorkflowRef: workflowexecutionv1.WorkflowRef{WorkflowSnapshot: types.WorkflowSnapshot{
					WorkflowID:      "wf-cancel-2582",
					WorkflowName:    "cancel",
					ActionType:      "restart",
					Version:         "v1",
					ExecutionBundle: "registry.example/cancel@sha256:abc",
					ExecutionEngine: "job",
				}},
			},
		}
	}

	It("reports verified termination after the owned Job and Pods are gone", func() {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(batchv1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())
		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		exec := executor.NewJobExecutor(client)
		wfe := newWFE()
		created, err := exec.Create(context.Background(), wfe, "default", executor.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		wfe.Status.ExecutionRef = &corev1.LocalObjectReference{Name: created.ResourceName}

		Expect(exec.RequestCancellation(context.Background(), wfe, "default")).To(Succeed())
		result, err := exec.VerifyCancellation(context.Background(), wfe, "default")

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Terminated).To(BeTrue())
	})

	It("does not claim cancellation while an owned Pod is still running", func() {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(batchv1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())
		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		exec := executor.NewJobExecutor(client)
		wfe := newWFE()
		created, err := exec.Create(context.Background(), wfe, "default", executor.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		wfe.Status.ExecutionRef = &corev1.LocalObjectReference{Name: created.ResourceName}
		Expect(client.Create(context.Background(), &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Name: "pod-still-running", Namespace: "default", Labels: map[string]string{"job-name": created.ResourceName},
		}, Status: corev1.PodStatus{Phase: corev1.PodRunning}})).To(Succeed())

		Expect(exec.RequestCancellation(context.Background(), wfe, "default")).To(Succeed())
		result, err := exec.VerifyCancellation(context.Background(), wfe, "default")

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Terminated).To(BeFalse())
		Expect(result.Reason).To(Equal("PodsStillActive"))
	})
})

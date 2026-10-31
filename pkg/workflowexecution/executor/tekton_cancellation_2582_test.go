package executor_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
)

var _ = Describe("UT-WE-2582-004: Tekton cancellation verification [BR-WE-014, AC-4]", func() {
	It("keeps cancellation unverified while a dependent TaskRun is active", func() {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(tektonv1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())
		pr := &tektonv1.PipelineRun{ObjectMeta: metav1.ObjectMeta{
			Name: "wfe-tekton-cancel", Namespace: "default", Labels: map[string]string{"kubernaut.ai/workflow-execution": "we-tekton-cancel"},
		}}
		taskRun := &tektonv1.TaskRun{ObjectMeta: metav1.ObjectMeta{
			Name: "task-still-running", Namespace: "default", Labels: map[string]string{"tekton.dev/pipelineRun": pr.Name},
		}}
		client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pr, taskRun).Build()
		tekton := executor.NewTektonExecutor(client)
		wfe := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{Name: "we-tekton-cancel", Namespace: "default"},
			Spec:       workflowexecutionv1.WorkflowExecutionSpec{TargetResource: "default/Deployment/api"},
			Status:     workflowexecutionv1.WorkflowExecutionStatus{ExecutionRef: &corev1.LocalObjectReference{Name: pr.Name}},
		}

		Expect(tekton.RequestCancellation(context.Background(), wfe, "default")).To(Succeed())
		result, err := tekton.VerifyCancellation(context.Background(), wfe, "default")

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Terminated).To(BeFalse())
		Expect(result.Reason).To(Equal("TaskRunStillActive"))
	})

	It("keeps cancellation unverified while a terminal TaskRun still owns an active Pod", func() {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(tektonv1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())
		pr := &tektonv1.PipelineRun{ObjectMeta: metav1.ObjectMeta{
			Name: "wfe-tekton-pod-cancel", Namespace: "default", Labels: map[string]string{"kubernaut.ai/workflow-execution": "we-tekton-pod-cancel"},
		}}
		taskRun := &tektonv1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{Name: "task-terminal-pod-active", Namespace: "default", Labels: map[string]string{"tekton.dev/pipelineRun": pr.Name}},
			Status:     tektonv1.TaskRunStatus{TaskRunStatusFields: tektonv1.TaskRunStatusFields{PodName: "pod-terminal-pod-active"}},
		}
		taskRun.Status.Conditions = []apis.Condition{{Type: apis.ConditionSucceeded, Status: corev1.ConditionTrue}}
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Name: "pod-terminal-pod-active", Namespace: "default", Labels: map[string]string{"tekton.dev/taskRun": taskRun.Name},
		}, Status: corev1.PodStatus{Phase: corev1.PodRunning}}
		client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pr, taskRun, pod).Build()
		tekton := executor.NewTektonExecutor(client)
		wfe := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{Name: "we-tekton-pod-cancel", Namespace: "default"},
			Spec:       workflowexecutionv1.WorkflowExecutionSpec{TargetResource: "default/Deployment/api"},
			Status:     workflowexecutionv1.WorkflowExecutionStatus{ExecutionRef: &corev1.LocalObjectReference{Name: pr.Name}},
		}

		Expect(tekton.RequestCancellation(context.Background(), wfe, "default")).To(Succeed())
		result, err := tekton.VerifyCancellation(context.Background(), wfe, "default")

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Terminated).To(BeFalse())
		Expect(result.Reason).To(Equal("PodsStillActive"))
	})
})

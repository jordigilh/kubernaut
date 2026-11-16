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

package workflowexecution

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	weexecutor "github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
)

var _ = Describe("WorkflowExecution snapshot isolation", func() {
	It("UT-WE-1661-SNAPSHOT-001: does not re-verify the catalog after dispatch", func() {
		scheme := runtime.NewScheme()
		Expect(workflowexecutionv1alpha1.AddToScheme(scheme)).To(Succeed())

		wfe := &workflowexecutionv1alpha1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "snapshot-isolation-wfe",
				Namespace:  "default",
				Finalizers: []string{FinalizerName},
			},
			Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
				RemediationRequestRef: corev1.ObjectReference{
					APIVersion: "remediationorchestrator.kubernaut.ai/v1alpha1",
					Kind:       "RemediationRequest",
					Name:       "snapshot-isolation-rr",
					Namespace:  "default",
				},
				WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
					WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
						WorkflowID:      "deleted-source-workflow",
						WorkflowName:    "snapshot-isolation",
						ActionType:      "RestartPod",
						Version:         "1.0.0",
						ExecutionBundle: "quay.io/kubernaut-cicd/tekton-bundles/hello-world:v1.0.0",
						ExecutionEngine: "job",
					},
				},
				TargetResource: "default/deployment/snapshot-isolation",
			},
			Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
				Phase: workflowexecutionv1alpha1.PhaseRunning,
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wfe).Build()
		exec := &snapshotIsolationExecutor{}
		registry := weexecutor.NewRegistry()
		registry.Register("job", exec)

		reconciler := &WorkflowExecutionReconciler{
			Client:             fakeClient,
			Scheme:             scheme,
			Recorder:           record.NewFakeRecorder(10),
			ExecutionNamespace: "kubernaut-workflows",
			ExecutorRegistry:   registry,
		}

		result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
			NamespacedName: types.NamespacedName{Name: wfe.Name, Namespace: wfe.Namespace},
		})

		Expect(err).To(MatchError("execution backend was polled"), "a dispatched WFE must not depend on the live catalog")
		Expect(exec.statusCalls).To(Equal(1), "the controller should poll the execution resource from the snapshot")
		Expect(result).To(Equal(reconcile.Result{}))
	})
})

type snapshotIsolationExecutor struct {
	statusCalls int
}

func (e *snapshotIsolationExecutor) Create(context.Context, *workflowexecutionv1alpha1.WorkflowExecution, string, weexecutor.CreateOptions) (*weexecutor.CreateResult, error) {
	return &weexecutor.CreateResult{ResourceName: "snapshot-isolation-execution"}, nil
}

func (e *snapshotIsolationExecutor) GetStatus(context.Context, *workflowexecutionv1alpha1.WorkflowExecution, string) (*weexecutor.ExecutionResult, error) {
	e.statusCalls++
	return nil, errors.New("execution backend was polled")
}

func (*snapshotIsolationExecutor) Cleanup(context.Context, *workflowexecutionv1alpha1.WorkflowExecution, string) error {
	return nil
}

func (*snapshotIsolationExecutor) Engine() string {
	return "job"
}

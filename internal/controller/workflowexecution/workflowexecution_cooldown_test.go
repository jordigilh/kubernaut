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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

var _ = Describe("WorkflowExecution cooldown scope [BR-FLEET-004]", func() {
	It("UT-WE-FLEET-004: does not apply a hub cooldown to the same target on a spoke", func() {
		ctx := context.Background()
		scheme := runtime.NewScheme()
		Expect(workflowexecutionv1alpha1.AddToScheme(scheme)).To(Succeed())

		completedAt := metav1.NewTime(time.Now().Add(-time.Minute))
		hubWFE := &workflowexecutionv1alpha1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{Name: "hub-completed", Namespace: "kubernaut-system"},
			Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
				TargetResource: "history/Deployment/memory-eater",
				ClusterID:      "hub",
			},
			Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
				Phase:          workflowexecutionv1alpha1.PhaseCompleted,
				CompletionTime: &completedAt,
			},
		}
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithIndex(&workflowexecutionv1alpha1.WorkflowExecution{}, "spec.targetResource", func(obj client.Object) []string {
				return []string{obj.(*workflowexecutionv1alpha1.WorkflowExecution).Spec.TargetResource}
			}).
			WithObjects(hubWFE).
			Build()
		reconciler := &WorkflowExecutionReconciler{Client: fakeClient, CooldownPeriod: 10 * time.Minute}

		remaining, active := reconciler.CheckCooldownActive(
			ctx,
			hubWFE.Spec.TargetResource,
			"remote-cluster",
			"kubernaut-system/current-spoke-wfe",
		)

		Expect(active).To(BeFalse())
		Expect(remaining).To(BeZero())
	})

	It("UT-WE-FLEET-005: preserves cooldown blocking within the same cluster", func() {
		ctx := context.Background()
		scheme := runtime.NewScheme()
		Expect(workflowexecutionv1alpha1.AddToScheme(scheme)).To(Succeed())

		completedAt := metav1.NewTime(time.Now().Add(-time.Minute))
		hubWFE := &workflowexecutionv1alpha1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{Name: "hub-same-scope", Namespace: "kubernaut-system"},
			Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
				TargetResource: "history/Deployment/memory-eater",
				ClusterID:      "hub",
			},
			Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
				Phase:          workflowexecutionv1alpha1.PhaseCompleted,
				CompletionTime: &completedAt,
			},
		}
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithIndex(&workflowexecutionv1alpha1.WorkflowExecution{}, "spec.targetResource", func(obj client.Object) []string {
				return []string{obj.(*workflowexecutionv1alpha1.WorkflowExecution).Spec.TargetResource}
			}).
			WithObjects(hubWFE).
			Build()
		reconciler := &WorkflowExecutionReconciler{Client: fakeClient, CooldownPeriod: 10 * time.Minute}

		remaining, active := reconciler.CheckCooldownActive(
			ctx,
			hubWFE.Spec.TargetResource,
			"hub",
			"kubernaut-system/current-hub-wfe",
		)

		Expect(active).To(BeTrue())
		Expect(remaining).To(BeNumerically(">", 0))
	})
})

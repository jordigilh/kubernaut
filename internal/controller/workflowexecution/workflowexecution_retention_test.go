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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

var _ = Describe("UT-WE-2392-RET: failed execution retention lifecycle (BR-WE-019, AU-11)", func() {
	It("UT-WE-2392-RET-001: calculates remaining retention from terminal completion time", func() {
		completedAt := metav1.NewTime(time.Now().Add(-30 * time.Second))
		r := &WorkflowExecutionReconciler{
			RetainFailedExecutions:   true,
			FailedExecutionRetention: 2 * time.Minute,
		}
		wfe := &workflowexecutionv1alpha1.WorkflowExecution{
			Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
				Phase:          workflowexecutionv1alpha1.PhaseFailed,
				CompletionTime: &completedAt,
			},
		}

		remaining, retain := r.failedExecutionRetentionRemaining(wfe)

		Expect(retain).To(BeTrue())
		Expect(remaining).To(BeNumerically(">", 0))
		Expect(remaining).To(BeNumerically("<", 2*time.Minute))
	})

	It("UT-WE-2392-RET-002: does not retain completed executions", func() {
		r := &WorkflowExecutionReconciler{
			RetainFailedExecutions:   true,
			FailedExecutionRetention: 2 * time.Minute,
		}
		wfe := &workflowexecutionv1alpha1.WorkflowExecution{
			Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
				Phase: workflowexecutionv1alpha1.PhaseCompleted,
			},
		}

		remaining, retain := r.failedExecutionRetentionRemaining(wfe)

		Expect(retain).To(BeFalse())
		Expect(remaining).To(Equal(time.Duration(0)))
	})

	It("UT-WE-2392-RET-003: retains a failed WFE being deleted when completion time is absent", func() {
		deletedAt := metav1.NewTime(time.Now())
		r := &WorkflowExecutionReconciler{
			RetainFailedExecutions:   true,
			FailedExecutionRetention: 2 * time.Minute,
		}
		wfe := &workflowexecutionv1alpha1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &deletedAt},
			Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
				Phase: workflowexecutionv1alpha1.PhaseFailed,
			},
		}

		remaining, retain := r.failedExecutionRetentionRemaining(wfe)

		Expect(retain).To(BeTrue())
		Expect(remaining).To(BeNumerically(">", 0))
	})

	It("UT-WE-2392-RET-004: recognizes only failed terminal PipelineRuns for replacement", func() {
		failed := &tektonv1.PipelineRun{Status: tektonv1.PipelineRunStatus{Status: duckv1.Status{Conditions: []apis.Condition{{Type: apis.ConditionSucceeded, Status: corev1.ConditionFalse}}}}}
		completed := &tektonv1.PipelineRun{Status: tektonv1.PipelineRunStatus{Status: duckv1.Status{Conditions: []apis.Condition{{Type: apis.ConditionSucceeded, Status: corev1.ConditionTrue}}}}}

		Expect(pipelineRunFailed(failed)).To(BeTrue())
		Expect(pipelineRunFailed(completed)).To(BeFalse())
	})
})

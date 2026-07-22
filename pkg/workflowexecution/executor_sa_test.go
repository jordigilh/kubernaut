/*
Copyright 2025 Jordi Gil.

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

package workflowexecution_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// ========================================
// PER-WORKFLOW SA SPEC-LEVEL TESTS (#650, #1661 Change 11f)
// ========================================
// Authority: DD-WE-005 v2.0, Issue #650
// Issue #650 originally resolved ServiceAccountName from a DS catalog
// round-trip into WFE Status at runtime. Issue #1661 Change 11d moved it
// onto the CRD-embedded WorkflowRef spec snapshot (RO copies it from
// AIAnalysis.Status.SelectedWorkflow), and Change 11f removed the Status
// mirror entirely -- every consumer now reads
// wfe.Spec.WorkflowRef.ServiceAccountName directly.
// ========================================

var _ = Describe("Per-Workflow ServiceAccount Spec Tests [DD-WE-005] (#650, #1661 Change 11f)", func() {

	buildWFE := func(saName string) *workflowexecutionv1alpha1.WorkflowExecution {
		wfe := &workflowexecutionv1alpha1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-wfe",
				Namespace: "kubernaut-workflows",
			},
			Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
				WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
					WorkflowID:         "wf-123",
					ExecutionBundle:    "quay.io/test:v1@sha256:abc123",
					ServiceAccountName: saName,
				},
				TargetResource: "default/Deployment/nginx",
			},
		}
		return wfe
	}

	Context("Spec.WorkflowRef.ServiceAccountName (CRD-embedded snapshot, engine-agnostic)", func() {

		It("UT-WE-501-001: should read SA directly from Spec.WorkflowRef.ServiceAccountName", func() {
			wfe := buildWFE("custom-sa")
			Expect(wfe.Spec.WorkflowRef.ServiceAccountName).To(Equal("custom-sa"))
		})

		It("UT-WE-501-002: should be empty string when no SA is specified", func() {
			wfe := buildWFE("")
			Expect(wfe.Spec.WorkflowRef.ServiceAccountName).To(Equal(""))
		})

		It("UT-WE-501-003: should be independent of ExecutionConfig", func() {
			wfe := buildWFE("top-level-sa")
			wfe.Spec.ExecutionConfig = &workflowexecutionv1alpha1.ExecutionConfig{
				Timeout: &metav1.Duration{Duration: 30 * 60e9},
			}
			Expect(wfe.Spec.WorkflowRef.ServiceAccountName).To(Equal("top-level-sa"),
				"SA should be on WorkflowRef (CRD-embedded snapshot), not inside ExecutionConfig")
		})
	})
})

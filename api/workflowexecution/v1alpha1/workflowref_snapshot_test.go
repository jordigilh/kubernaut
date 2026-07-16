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

package v1alpha1_test

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// ========================================
// UT-WFE-339-001 (Issue #1661 Change 11c, DD-WORKFLOW-018)
// ========================================
// Authority: DD-WORKFLOW-018. WorkflowRef must carry the same CRD-embedded
// execution snapshot fields AIAnalysis.Status.SelectedWorkflow already gained
// in Change 11b (Phase 41) -- ExecutionEngine, ServiceAccountName,
// Dependencies, Resources, DeclaredParameterNames -- so RemediationOrchestrator
// (Change 11d) can pass them straight through from AA's snapshot into the
// WorkflowExecution spec, and WorkflowExecution (Change 11e) can stop
// re-fetching this data from DataStorage entirely.
//
// WorkflowExecutionSpec already carries a struct-level
// +kubebuilder:validation:XValidation:rule="self == oldSelf" (ADR-001) that
// covers WorkflowRef as a whole -- no new per-field CEL rule is needed, only
// the fields themselves and (this test) a DeepCopy correctness check, since
// pointer/map fields are the classic place a generated DeepCopyInto silently
// stays shallow.
//
// RED: WorkflowRef has no ExecutionEngine/ServiceAccountName/Dependencies/
// Resources/DeclaredParameterNames fields yet -- this file must fail to
// compile.
// ========================================
var _ = Describe("WorkflowRef — Issue #1661 Change 11c CRD-embedded execution snapshot", func() {
	It("UT-WFE-339-001: carries ExecutionEngine/ServiceAccountName/Dependencies/Resources/DeclaredParameterNames and deep-copies them independently", func() {
		original := workflowexecutionv1alpha1.WorkflowRef{
			WorkflowID:      "wf-oom-recovery",
			Version:         "v1.0.0",
			ExecutionBundle: "quay.io/kubernaut/oom-recovery:v1",
			ExecutionEngine: "job",
			ServiceAccountName: "kubernaut-workflow-runner",
			Dependencies: &sharedtypes.WorkflowDependencies{
				Secrets: []sharedtypes.WorkflowResourceDependency{{Name: "db-creds"}},
			},
			Resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
				Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m")},
			},
			DeclaredParameterNames: map[string]bool{"TARGET_NAMESPACE": true, "REPLICAS": true},
		}

		clone := original.DeepCopy()

		By("the clone carrying equal values for every new field")
		Expect(clone.ExecutionEngine).To(Equal("job"))
		Expect(clone.ServiceAccountName).To(Equal("kubernaut-workflow-runner"))
		Expect(clone.Dependencies).ToNot(BeNil())
		Expect(clone.Dependencies.Secrets).To(Equal(original.Dependencies.Secrets))
		Expect(clone.Resources).ToNot(BeNil())
		Expect(clone.Resources.Requests.Cpu().String()).To(Equal("100m"))
		Expect(clone.DeclaredParameterNames).To(Equal(original.DeclaredParameterNames))

		By("the clone's pointer/map fields being independent copies, not aliases (deep, not shallow, copy)")
		Expect(clone.Dependencies).ToNot(BeIdenticalTo(original.Dependencies))
		Expect(clone.Resources).ToNot(BeIdenticalTo(original.Resources))

		clone.Dependencies.Secrets[0].Name = "mutated"
		clone.DeclaredParameterNames["REPLICAS"] = false
		Expect(original.Dependencies.Secrets[0].Name).To(Equal("db-creds"), "mutating the clone must not affect the original (DeepCopyInto must deep-copy the slice, not just the pointer)")
		Expect(original.DeclaredParameterNames["REPLICAS"]).To(BeTrue(), "mutating the clone's map must not affect the original")
	})
})

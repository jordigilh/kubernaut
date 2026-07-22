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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// Issue #1661 Change 11f (DD-WORKFLOW-018): resolveWorkflowCatalog (the
// former Spec->Status mirror for ExecutionEngine/ServiceAccountName/
// Resources) is removed entirely. Every production consumer now reads
// wfe.Spec.WorkflowRef.{ExecutionEngine,ServiceAccountName,Resources,
// ActionType} directly -- there is nothing left to "resolve" at runtime
// since WorkflowRef is RO's already-validated, immutable, CRD-embedded
// snapshot (Change 11c/11d). validateExecutionEngineResolved is the sole
// remaining function in this file: a defensive fail-closed guard (not a
// resolution step) against a WorkflowRef that somehow lacks an execution
// engine, which should be unreachable in practice since RO's
// validateSelectedWorkflow already enforces this before the WFE is ever
// created.
var _ = Describe("validateExecutionEngineResolved (Issue #1661 Change 11f)", func() {
	var (
		r   *WorkflowExecutionReconciler
		wfe *workflowexecutionv1alpha1.WorkflowExecution
	)

	BeforeEach(func() {
		r = &WorkflowExecutionReconciler{}

		wfe = &workflowexecutionv1alpha1.WorkflowExecution{
			Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
				WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
					WorkflowID:            "wf-catalog-red-001",
					Version:               "1.0.0",
					ExecutionBundle:       "quay.io/kubernaut/workflows/red-001:v1@sha256:deadbeef",
					ExecutionBundleDigest: "sha256:deadbeef",
					EngineConfig:          &apiextensionsv1.JSON{Raw: []byte(`{"playbookPath":"deploy.yml"}`)},
					ExecutionEngine:       "job",
					ServiceAccountName:    "workflow-runner-sa",
					ActionType:            "ScaleReplicas",
					Dependencies: &sharedtypes.WorkflowDependencies{
						Secrets: []sharedtypes.WorkflowResourceDependency{{Name: "db-creds"}},
					},
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
						Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m")},
					},
					DeclaredParameterNames: map[string]bool{"TARGET_POD": true},
				},
			},
		}
	})

	It("UT-WE-1661-011-001: passes when WorkflowRef.ExecutionEngine is declared", func() {
		Expect(r.validateExecutionEngineResolved(wfe)).ToNot(HaveOccurred())
	})

	It("UT-WE-1661-011-002: fails when WorkflowRef.ExecutionEngine is empty (defensive guard -- unreachable via RO's normal creation path)", func() {
		wfe.Spec.WorkflowRef.ExecutionEngine = ""
		Expect(r.validateExecutionEngineResolved(wfe)).To(HaveOccurred())
	})

	It("UT-WE-1661-011-003: ExecutionEngine/ServiceAccountName/Resources/ActionType are read straight off the immutable WorkflowRef snapshot -- there is no Status mirror to keep in sync", func() {
		// #1661 Change 11f: these four all moved off wfe.Status entirely
		// (WorkflowExecutionStatus has no ExecutionEngine/ServiceAccountName/
		// Resources/ActionType fields anymore -- see api/workflowexecution/
		// v1alpha1/workflowexecution_types.go). Every production consumer
		// (pending.go, lifecycle.go, the three executors, the audit manager,
		// RO's notification_creation.go) reads wfe.Spec.WorkflowRef.X
		// directly, so this snapshot is authoritative from CRD-creation time
		// with no additional per-reconcile resolution step required.
		Expect(wfe.Spec.WorkflowRef.ExecutionEngine).To(Equal("job"))
		Expect(wfe.Spec.WorkflowRef.ServiceAccountName).To(Equal("workflow-runner-sa"))
		Expect(wfe.Spec.WorkflowRef.Resources).ToNot(BeNil())
		Expect(wfe.Spec.WorkflowRef.ActionType).To(Equal("ScaleReplicas"))
	})

	It("UT-WE-1661-011-004: WorkflowName has no source anywhere upstream and stays permanently unset on wfe.Status regardless of this change", func() {
		// Unlike its five siblings, WorkflowRef carries no WorkflowName field
		// at all (KA's autonomous selection path never emits a display name
		// distinct from WorkflowID either), so there is nothing to read from
		// Spec for this one. WorkflowID remains the functional/join key for
		// SOC2 CC8.1 reconstruction regardless (IT-AW-1111-001).
		Expect(wfe.Status.WorkflowName).To(BeEmpty())
	})
})

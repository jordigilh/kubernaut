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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// Issue #1661 Change 11e (DD-WORKFLOW-018): resolveWorkflowCatalog copies the
// execution-engine snapshot (ExecutionEngine/ServiceAccountName/Resources)
// from wfe.Spec.WorkflowRef onto Status verbatim, with zero DataStorage
// round-trips and zero spec mutation. Phase 51 REFACTOR: WorkflowQuerier no
// longer exists as a field on WorkflowExecutionReconciler at all (removed
// once Phase 50 confirmed zero remaining call sites in this package), so
// "zero WorkflowQuerier calls" is now a structural guarantee rather than a
// runtime assertion -- these tests instead assert the resulting behavior
// directly against Status/Spec.
var _ = Describe("resolveWorkflowCatalog (Issue #1661 Change 11e)", func() {
	var (
		ctx context.Context
		r   *WorkflowExecutionReconciler
		wfe *workflowexecutionv1alpha1.WorkflowExecution
	)

	BeforeEach(func() {
		ctx = context.Background()
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

	It("UT-WE-1661-001: sets Status.ExecutionEngine/ServiceAccountName/Resources from WorkflowRef", func() {
		err := r.resolveWorkflowCatalog(ctx, wfe)
		Expect(err).ToNot(HaveOccurred())

		Expect(wfe.Status.ExecutionEngine).To(Equal("job"),
			"ExecutionEngine must come from wfe.Spec.WorkflowRef, not the (forbidden) DS catalog")
		Expect(wfe.Status.ServiceAccountName).To(Equal("workflow-runner-sa"))
		Expect(wfe.Status.Resources).To(Equal(wfe.Spec.WorkflowRef.Resources))
	})

	It("UT-WE-1661-002: never mutates WorkflowRef.ExecutionBundle/ExecutionBundleDigest/EngineConfig at runtime", func() {
		originalBundle := wfe.Spec.WorkflowRef.ExecutionBundle
		originalDigest := wfe.Spec.WorkflowRef.ExecutionBundleDigest
		originalEngineConfig := wfe.Spec.WorkflowRef.EngineConfig

		err := r.resolveWorkflowCatalog(ctx, wfe)
		Expect(err).ToNot(HaveOccurred())

		// #1661 Change 11e: the WorkflowRef is RO's already-validated,
		// CRD-embedded snapshot (Change 11d) -- WorkflowExecution must never
		// overwrite it from a (forbidden) DS catalog entry at runtime, unlike
		// the pre-#1661 resolveWorkflowCatalog which did exactly this.
		Expect(wfe.Spec.WorkflowRef.ExecutionBundle).To(Equal(originalBundle))
		Expect(wfe.Spec.WorkflowRef.ExecutionBundleDigest).To(Equal(originalDigest))
		Expect(wfe.Spec.WorkflowRef.EngineConfig).To(Equal(originalEngineConfig))
	})

	It("UT-WE-1661-003: leaves Status.WorkflowName/ActionType empty (deferred readability convenience, not a functional/SOC2 requirement)", func() {
		// Design-gap resolution (escalated during Phase 49 preflight): WorkflowRef
		// has no WorkflowName/ActionType fields (KA's autonomous path never emits
		// them), so these Status fields are simply never populated post-#1661.
		// workflow_id remains the join key into the immutable workflow_content
		// captured in the Postgres audit_events ledger (IT-AW-1111-001), which is
		// sufficient for SOC2 CC8.1 reconstruction. A fast-follow issue tracks
		// wiring these end-to-end for audit readability if ever prioritized.
		err := r.resolveWorkflowCatalog(ctx, wfe)
		Expect(err).ToNot(HaveOccurred())

		Expect(wfe.Status.WorkflowName).To(BeEmpty())
		Expect(wfe.Status.ActionType).To(BeEmpty())
	})

	It("UT-WE-1661-004: is idempotent -- returns ErrAlreadyResolved (Issue #1674 sentinel) without touching Status.ExecutionEngine", func() {
		wfe.Status.ExecutionEngine = "already-resolved"

		err := r.resolveWorkflowCatalog(ctx, wfe)
		Expect(errors.Is(err, ErrAlreadyResolved)).To(BeTrue())

		Expect(wfe.Status.ExecutionEngine).To(Equal("already-resolved"))
	})
})

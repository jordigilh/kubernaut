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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// Issue #1661 Change 11e (DD-WORKFLOW-018): resolveSchemaMetadata builds
// executor.CreateOptions (Dependencies/DeclaredParameterNames) directly from
// the already-validated wfe.Spec.WorkflowRef snapshot (Change 11c/11d), with
// zero DataStorage round-trips. Phase 51 REFACTOR: WorkflowQuerier no longer
// exists as a field on WorkflowExecutionReconciler at all (removed once
// Phase 50 confirmed zero remaining call sites in this package), so "zero
// WorkflowQuerier calls" is now a structural guarantee rather than a runtime
// assertion -- these tests instead assert the resulting CreateOptions
// directly.
var _ = Describe("resolveSchemaMetadata (Issue #1661 Change 11e)", func() {
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
					WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
						WorkflowID: "wf-pending-red-001",
						Dependencies: &sharedtypes.WorkflowDependencies{
							Secrets:    []sharedtypes.WorkflowResourceDependency{{Name: "db-creds"}},
							ConfigMaps: []sharedtypes.WorkflowResourceDependency{{Name: "app-config"}},
						},
						DeclaredParameterNames: map[string]bool{"TARGET_POD": true, "NAMESPACE": true},
					},
				},
			},
		}
	})

	It("UT-WE-1661-005: builds CreateOptions.Dependencies/DeclaredParameterNames from WorkflowRef", func() {
		_, opts, err := r.resolveSchemaMetadata(ctx, wfe)
		Expect(err).ToNot(HaveOccurred())

		Expect(opts.Dependencies).To(Equal(&models.WorkflowDependencies{
			Secrets:    []models.ResourceDependency{{Name: "db-creds"}},
			ConfigMaps: []models.ResourceDependency{{Name: "app-config"}},
		}), "Dependencies must be converted from wfe.Spec.WorkflowRef.Dependencies, not fetched from the (forbidden) DS schema")
		Expect(opts.DeclaredParameterNames).To(Equal(map[string]bool{"TARGET_POD": true, "NAMESPACE": true}))
	})

	It("UT-WE-1661-006: builds nil CreateOptions.Dependencies when WorkflowRef.Dependencies is nil (no filtering, backward compatible)", func() {
		wfe.Spec.WorkflowRef.Dependencies = nil
		wfe.Spec.WorkflowRef.DeclaredParameterNames = nil

		_, opts, err := r.resolveSchemaMetadata(ctx, wfe)
		Expect(err).ToNot(HaveOccurred())

		Expect(opts.Dependencies).To(BeNil())
		Expect(opts.DeclaredParameterNames).To(BeNil())
	})
})

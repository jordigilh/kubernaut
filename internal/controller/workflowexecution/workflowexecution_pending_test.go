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

	remediationworkflowv1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
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
	var r *WorkflowExecutionReconciler

	BeforeEach(func() {
		r = &WorkflowExecutionReconciler{retainFailedExecutions: true}
	})

	It("UT-WE-2575-001: builds CreateOptions from the verified live Workflow", func() {
		workflow := &remediationworkflowv1.RemediationWorkflow{
			Status: remediationworkflowv1.RemediationWorkflowStatus{WorkflowID: "wf-pending-red-001"},
			Spec: remediationworkflowv1.RemediationWorkflowSpec{
				Dependencies: &remediationworkflowv1.RemediationWorkflowDependencies{
					Secrets:    []remediationworkflowv1.RemediationWorkflowResourceDependency{{Name: "db-creds"}},
					ConfigMaps: []remediationworkflowv1.RemediationWorkflowResourceDependency{{Name: "app-config"}},
				},
				Parameters: []remediationworkflowv1.RemediationWorkflowParameter{{Name: "TARGET_POD"}, {Name: "NAMESPACE"}},
			},
		}
		_, opts, err := r.resolveSchemaMetadataFromWorkflow(workflow)
		Expect(err).ToNot(HaveOccurred())

		Expect(opts.Dependencies).To(Equal(&models.WorkflowDependencies{
			Secrets:    []models.ResourceDependency{{Name: "db-creds"}},
			ConfigMaps: []models.ResourceDependency{{Name: "app-config"}},
		}), "Dependencies must be converted from the verified live Workflow")
		Expect(opts.DeclaredParameterNames).To(Equal(map[string]bool{"TARGET_POD": true, "NAMESPACE": true}))
		Expect(opts.RetainFailedExecutions).To(BeTrue())
	})

	It("UT-WE-2575-002: permits absent dependencies with an explicit empty live declaration set", func() {
		workflow := &remediationworkflowv1.RemediationWorkflow{
			Status: remediationworkflowv1.RemediationWorkflowStatus{WorkflowID: "wf-empty"},
		}
		_, opts, err := r.resolveSchemaMetadataFromWorkflow(workflow)
		Expect(err).ToNot(HaveOccurred())

		Expect(opts.Dependencies).To(BeNil())
		Expect(opts.DeclaredParameterNames).NotTo(BeNil())
		Expect(opts.DeclaredParameterNames).To(BeEmpty())
	})
})

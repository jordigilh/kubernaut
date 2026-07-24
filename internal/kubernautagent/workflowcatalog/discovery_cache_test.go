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

package workflowcatalog

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// rawDetectedLabelsJSON builds a *apiextensionsv1.JSON from a raw JSON
// string literal, for constructing RemediationWorkflow fixtures with a
// spec.detectedLabels value inline.
func rawDetectedLabelsJSON(raw string) *apiextensionsv1.JSON {
	return &apiextensionsv1.JSON{Raw: []byte(raw)}
}

// ========================================
// UNIT TESTS: cache-backed Step 1/2 orchestration (Issue #1677 Phase 2b)
// ========================================
// Authority: DD-WORKFLOW-019. Ported verbatim from
// pkg/datastorage/repository/workflow/discovery_cache_test.go (Issue #1661
// Change 6) -- these are the pure (no cache/K8s) orchestration functions
// Catalog.ListActions/ListWorkflowsByActionType delegate to once they have
// already fetched CRD lists from Cache -- the cache-fetch itself is proven
// by the Phase 2a integration test
// (test/integration/kubernautagent/workflowcatalog/cache_test.go), matching
// the Pyramid Invariant: "UT proves logic. IT proves wiring."
// ========================================

func rwFixture(name string, severity []string) rwv1alpha1.RemediationWorkflow {
	return rwv1alpha1.RemediationWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: rwv1alpha1.RemediationWorkflowSpec{
			ActionType: "ScaleReplicas",
			Labels:     rwv1alpha1.RemediationWorkflowLabels{Severity: severity},
		},
		Status: rwv1alpha1.RemediationWorkflowStatus{
			WorkflowID: name + "-id",
		},
	}
}

var _ = Describe("filterAndScoreCachedWorkflows (Issue #1677 Phase 2b)", func() {
	It("UT-KA-1677-614-001: keeps only workflows matching the mandatory-label filters", func() {
		workflows := []rwv1alpha1.RemediationWorkflow{
			rwFixture("wf-critical", []string{"critical"}),
			rwFixture("wf-low", []string{"low"}),
		}
		filters := &models.WorkflowDiscoveryFilters{Severity: "critical"}

		got, err := filterAndScoreCachedWorkflows(workflows, filters)
		Expect(err).ToNot(HaveOccurred())
		Expect(got).To(HaveLen(1))
		Expect(got[0].WorkflowName).To(Equal("wf-critical"))
	})

	It("UT-KA-1677-614-002: nil filters matches every workflow (unconstrained discovery)", func() {
		workflows := []rwv1alpha1.RemediationWorkflow{
			rwFixture("wf-a", nil),
			rwFixture("wf-b", nil),
		}
		got, err := filterAndScoreCachedWorkflows(workflows, nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(got).To(HaveLen(2))
	})

	It("UT-KA-1677-614-003: sorts by final_score DESC, workflow_id ASC tiebreaker (mirrors selectScoredWorkflows ORDER BY)", func() {
		gitOpsDetected := &models.DetectedLabels{GitOpsManaged: true}
		wfNoBoost := rwFixture("wf-no-boost", nil)
		wfNoBoost.Status.WorkflowID = "zzz-no-boost"
		wfBoosted := rwFixture("wf-boosted", nil)
		wfBoosted.Status.WorkflowID = "aaa-boosted"
		wfBoosted.Spec.DetectedLabels = rawDetectedLabelsJSON(`{"gitOpsManaged":true}`)

		got, err := filterAndScoreCachedWorkflows([]rwv1alpha1.RemediationWorkflow{wfNoBoost, wfBoosted}, &models.WorkflowDiscoveryFilters{DetectedLabels: gitOpsDetected})
		Expect(err).ToNot(HaveOccurred())
		Expect(got).To(HaveLen(2))
		Expect(got[0].WorkflowName).To(Equal("wf-boosted"), "higher final_score (gitOpsManaged boost) sorts first")
		Expect(got[1].WorkflowName).To(Equal("wf-no-boost"))
	})

	It("UT-KA-1677-614-004: propagates a converter error (e.g. malformed detectedLabels JSON) instead of silently dropping the workflow", func() {
		malformed := rwFixture("wf-malformed", nil)
		malformed.Spec.DetectedLabels = rawDetectedLabelsJSON(`{not-json`)

		_, err := filterAndScoreCachedWorkflows([]rwv1alpha1.RemediationWorkflow{malformed}, nil)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("sortActionTypeEntries (Issue #1677 Phase 2b)", func() {
	It("UT-KA-1677-615-006: sorts alphabetically by ActionType (mirrors ListActions' ORDER BY t.action_type)", func() {
		entries := []models.ActionTypeEntry{
			{ActionType: "ScaleReplicas"},
			{ActionType: "DrainNode"},
			{ActionType: "RestartPod"},
		}
		sortActionTypeEntries(entries)
		Expect(entries[0].ActionType).To(Equal("DrainNode"))
		Expect(entries[1].ActionType).To(Equal("RestartPod"))
		Expect(entries[2].ActionType).To(Equal("ScaleReplicas"))
	})
})

var _ = Describe("paginate (Issue #1677 Phase 2b)", func() {
	It("UT-KA-1677-616-004: slices [offset, offset+limit) and clamps to the slice bounds", func() {
		items := []int{1, 2, 3, 4, 5}
		Expect(paginate(items, 1, 2)).To(Equal([]int{2, 3}))
		Expect(paginate(items, 4, 10)).To(Equal([]int{5}))
		Expect(paginate(items, 10, 10)).To(BeEmpty())
		Expect(paginate(items, 0, 100)).To(Equal(items))
	})
})

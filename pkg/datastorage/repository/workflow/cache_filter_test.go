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

package workflow

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// ========================================
// UNIT TESTS: In-memory Go filter/scoring predicates (Issue #1661 Change 6)
// ========================================
// Authority: DD-WORKFLOW-018 (etcd single source of truth for workflow/action-type
// definitions -- DataStorage's discovery Steps 1/2 read from the Phase 28/29
// informer-backed cache instead of Postgres SQL).
// Business Requirements: BR-WORKFLOW-006, BR-WORKFLOW-007, BR-FLEET-003.
//
// These predicates are the in-memory Go equivalent of discovery.go's
// buildContextFilterSQL (mandatory-label WHERE clause + DetectedLabels hard
// filter) and scoring.go's buildDetectedLabelsBoostSQL/
// buildDetectedLabelsPenaltySQL/buildCustomLabelsBoostSQL (scoring). Behavior
// is intentionally byte-for-byte equivalent to the SQL they replace -- see the
// per-function doc comments in cache_filter.go (Phase 32 GREEN) for the SQL
// clause each mirrors.
//
// RED: none of these symbols exist yet -- this file must fail to compile.
// ========================================

var _ = Describe("matchesArrayLabel (Issue #1661 Change 6)", func() {
	// Mirrors appendMandatoryLabelConditions' EXISTS/jsonb_array_elements_text/
	// LOWER pattern shared by severity/component/environment/cluster.

	DescribeTable("UT-DS-1661-601: case-insensitive array matching with wildcard and exclusion semantics",
		func(workflowValues []string, filterValue string, expected bool) {
			Expect(matchesArrayLabel(workflowValues, filterValue)).To(Equal(expected))
		},
		Entry("exact match", []string{"critical"}, "critical", true),
		Entry("case-insensitive match", []string{"Critical"}, "critical", true),
		Entry("case-insensitive match, filter uppercase", []string{"critical"}, "CRITICAL", true),
		Entry("workflow wildcard matches any filter value", []string{"*"}, "critical", true),
		Entry("no match among multiple values", []string{"high", "warning"}, "critical", false),
		Entry("match among multiple values", []string{"high", "critical"}, "critical", true),
		Entry("empty filter value means unconstrained (always matches)", []string{"high"}, "", true),
		Entry("empty filter value matches even when workflow has no values", []string{}, "", true),
		Entry("empty workflow values + non-empty filter is exclusion (BR-FLEET-003 R6)", []string{}, "production", false),
		Entry("nil workflow values + non-empty filter is exclusion", nil, "production", false),
	)
})

var _ = Describe("matchesPriority (Issue #1661 Change 6)", func() {
	// Mirrors appendMandatoryLabelConditions' priority CASE WHEN jsonb_typeof
	// scalar branch -- workflow-side priority is a single string, not an array.

	DescribeTable("UT-DS-1661-602: case-insensitive scalar matching with wildcard",
		func(workflowPriority, filterValue string, expected bool) {
			Expect(matchesPriority(workflowPriority, filterValue)).To(Equal(expected))
		},
		Entry("exact match", "P1", "P1", true),
		Entry("case-insensitive match", "p1", "P1", true),
		Entry("workflow wildcard matches any filter value", "*", "P1", true),
		Entry("mismatch", "P2", "P1", false),
		Entry("empty filter value means unconstrained", "P2", "", true),
	)
})

var _ = Describe("matchesMandatoryLabels (Issue #1661 Change 6)", func() {
	// Mirrors buildContextFilterSQL's combination of severity/component/
	// environment/priority/cluster conditions (all ANDed together).

	baseLabels := func() models.MandatoryLabels {
		return models.MandatoryLabels{
			Severity:    []string{"critical"},
			Component:   []string{"v1/Pod"},
			Environment: []string{"production"},
			Priority:    "P1",
		}
	}

	It("UT-DS-1661-603-001: nil filters match every workflow", func() {
		Expect(matchesMandatoryLabels(baseLabels(), nil)).To(BeTrue())
	})

	It("UT-DS-1661-603-002: matches when every mandatory dimension aligns", func() {
		filters := &models.WorkflowDiscoveryFilters{
			Severity: "critical", Component: "v1/Pod", Environment: "production", Priority: "P1",
		}
		Expect(matchesMandatoryLabels(baseLabels(), filters)).To(BeTrue())
	})

	It("UT-DS-1661-603-003: rejects when severity mismatches", func() {
		filters := &models.WorkflowDiscoveryFilters{
			Severity: "warning", Component: "v1/Pod", Environment: "production", Priority: "P1",
		}
		Expect(matchesMandatoryLabels(baseLabels(), filters)).To(BeFalse())
	})

	It("UT-DS-1661-603-004: component matching is case-insensitive (K8s Kind is PascalCase)", func() {
		filters := &models.WorkflowDiscoveryFilters{Component: "V1/POD"}
		Expect(matchesMandatoryLabels(baseLabels(), filters)).To(BeTrue())
	})

	It("UT-DS-1661-603-005: cluster filter absent (non-fleet) never excludes (BR-FLEET-003)", func() {
		labels := baseLabels()
		labels.Cluster = nil
		filters := &models.WorkflowDiscoveryFilters{Cluster: ""}
		Expect(matchesMandatoryLabels(labels, filters)).To(BeTrue())
	})

	It("UT-DS-1661-603-006: cluster filter set excludes workflows with no cluster entries", func() {
		labels := baseLabels()
		labels.Cluster = nil
		filters := &models.WorkflowDiscoveryFilters{Cluster: "production"}
		Expect(matchesMandatoryLabels(labels, filters)).To(BeFalse())
	})

	It("UT-DS-1661-603-007: cluster wildcard opts a workflow into every fleet classification", func() {
		labels := baseLabels()
		labels.Cluster = []string{"*"}
		filters := &models.WorkflowDiscoveryFilters{Cluster: "staging-eu"}
		Expect(matchesMandatoryLabels(labels, filters)).To(BeTrue())
	})
})

var _ = Describe("matchesDetectedLabelsFilter (Issue #1661 Change 6)", func() {
	// Mirrors appendDetectedLabelConditions. Boolean detected-label fields are
	// intentionally NOT hard-filtered here: models.DetectedLabels serializes
	// sparsely (false is never distinguished from absent, see MarshalJSON/
	// SerializeLabels), so the equivalent SQL condition
	// "(x = 'true' OR x IS NULL)" is always true for a boolean field and
	// excludes nothing -- confirmed by inspection of the existing SQL, not a
	// gap introduced by this port. Only string fields (gitOpsTool/serviceMesh/
	// storageBackend) have real exclusion power because non-empty values are
	// distinguishable from absence.

	It("UT-DS-1661-604-001: nil filter DetectedLabels matches every workflow", func() {
		Expect(matchesDetectedLabelsFilter(models.DetectedLabels{}, nil)).To(BeTrue())
	})

	It("UT-DS-1661-604-002: boolean fields never exclude regardless of workflow value", func() {
		filter := &models.DetectedLabels{GitOpsManaged: true, PDBProtected: true}
		Expect(matchesDetectedLabelsFilter(models.DetectedLabels{}, filter)).To(BeTrue(),
			"workflow declaring nothing must still pass -- absent is treated as 'no requirement'")
		Expect(matchesDetectedLabelsFilter(models.DetectedLabels{GitOpsManaged: true, PDBProtected: true}, filter)).To(BeTrue())
	})

	It("UT-DS-1661-604-003: string field exact match passes", func() {
		filter := &models.DetectedLabels{GitOpsTool: "argocd"}
		Expect(matchesDetectedLabelsFilter(models.DetectedLabels{GitOpsTool: "argocd"}, filter)).To(BeTrue())
	})

	It("UT-DS-1661-604-004: string field mismatch excludes", func() {
		filter := &models.DetectedLabels{GitOpsTool: "argocd"}
		Expect(matchesDetectedLabelsFilter(models.DetectedLabels{GitOpsTool: "flux"}, filter)).To(BeFalse())
	})

	It("UT-DS-1661-604-005: string field absent on workflow side passes (no requirement declared)", func() {
		filter := &models.DetectedLabels{GitOpsTool: "argocd"}
		Expect(matchesDetectedLabelsFilter(models.DetectedLabels{}, filter)).To(BeTrue())
	})

	It("UT-DS-1661-604-006: workflow-side wildcard passes any requested string value", func() {
		filter := &models.DetectedLabels{ServiceMesh: "istio"}
		Expect(matchesDetectedLabelsFilter(models.DetectedLabels{ServiceMesh: "*"}, filter)).To(BeTrue())
	})
})

var _ = Describe("detectedLabelsBoost (Issue #1661 Change 6)", func() {
	// Mirrors buildDetectedLabelsBoostSQL's per-field CASE WHEN weights
	// (DD-WORKFLOW-004 v1.5).

	It("UT-DS-1661-605-001: nil/empty filter DetectedLabels contributes no boost", func() {
		Expect(detectedLabelsBoost(models.DetectedLabels{}, nil)).To(Equal(0.0))
		Expect(detectedLabelsBoost(models.DetectedLabels{}, &models.DetectedLabels{})).To(Equal(0.0))
	})

	It("UT-DS-1661-605-002: boolean field full boost when both sides true", func() {
		filter := &models.DetectedLabels{GitOpsManaged: true}
		Expect(detectedLabelsBoost(models.DetectedLabels{GitOpsManaged: true}, filter)).To(BeNumerically("~", 0.10, 1e-9))
	})

	It("UT-DS-1661-605-003: boolean field contributes nothing when workflow side is false", func() {
		filter := &models.DetectedLabels{GitOpsManaged: true}
		Expect(detectedLabelsBoost(models.DetectedLabels{}, filter)).To(Equal(0.0))
	})

	It("UT-DS-1661-605-004: string field exact match gets full boost", func() {
		filter := &models.DetectedLabels{GitOpsTool: "argocd"}
		Expect(detectedLabelsBoost(models.DetectedLabels{GitOpsTool: "argocd"}, filter)).To(BeNumerically("~", 0.10, 1e-9))
	})

	It("UT-DS-1661-605-005: workflow-side wildcard gets half boost (#215 Gap 2)", func() {
		filter := &models.DetectedLabels{GitOpsTool: "argocd"}
		Expect(detectedLabelsBoost(models.DetectedLabels{GitOpsTool: "*"}, filter)).To(BeNumerically("~", 0.05, 1e-9))
	})

	It("UT-DS-1661-605-006: query-side wildcard gets half boost for any non-empty workflow value", func() {
		filter := &models.DetectedLabels{ServiceMesh: "*"}
		Expect(detectedLabelsBoost(models.DetectedLabels{ServiceMesh: "linkerd"}, filter)).To(BeNumerically("~", 0.025, 1e-9))
	})

	It("UT-DS-1661-605-007: multiple fields sum additively", func() {
		filter := &models.DetectedLabels{GitOpsManaged: true, VirtualMachine: true}
		got := detectedLabelsBoost(models.DetectedLabels{GitOpsManaged: true, VirtualMachine: true}, filter)
		Expect(got).To(BeNumerically("~", 0.10+0.08, 1e-9))
	})
})

var _ = Describe("detectedLabelsPenalty (Issue #1661 Change 6)", func() {
	// Mirrors buildDetectedLabelsPenaltySQL -- only the two high-impact fields
	// (gitOpsManaged, gitOpsTool) apply penalties.

	It("UT-DS-1661-606-001: nil/empty filter DetectedLabels contributes no penalty", func() {
		Expect(detectedLabelsPenalty(models.DetectedLabels{}, nil)).To(Equal(0.0))
	})

	It("UT-DS-1661-606-002: penalizes when query wants GitOps but workflow is not GitOps", func() {
		filter := &models.DetectedLabels{GitOpsManaged: true}
		Expect(detectedLabelsPenalty(models.DetectedLabels{}, filter)).To(BeNumerically("~", 0.10, 1e-9))
	})

	It("UT-DS-1661-606-003: no penalty when both sides agree on GitOps", func() {
		filter := &models.DetectedLabels{GitOpsManaged: true}
		Expect(detectedLabelsPenalty(models.DetectedLabels{GitOpsManaged: true}, filter)).To(Equal(0.0))
	})

	It("UT-DS-1661-606-004: penalizes when workflow declares a different GitOps tool", func() {
		filter := &models.DetectedLabels{GitOpsTool: "argocd"}
		Expect(detectedLabelsPenalty(models.DetectedLabels{GitOpsTool: "flux"}, filter)).To(BeNumerically("~", 0.10, 1e-9))
	})

	It("UT-DS-1661-606-005: no penalty when workflow declares matching tool or wildcard", func() {
		filter := &models.DetectedLabels{GitOpsTool: "argocd"}
		Expect(detectedLabelsPenalty(models.DetectedLabels{GitOpsTool: "argocd"}, filter)).To(Equal(0.0))
		Expect(detectedLabelsPenalty(models.DetectedLabels{GitOpsTool: "*"}, filter)).To(Equal(0.0))
	})
})

var _ = Describe("customLabelsBoost (Issue #1661 Change 6)", func() {
	// Mirrors buildCustomLabelsBoostSQL's @> JSONB containment check
	// (DD-WORKFLOW-004 v1.7).

	It("UT-DS-1661-607-001: empty filter custom labels contributes no boost", func() {
		Expect(customLabelsBoost(models.CustomLabels{"team": {"payments"}}, nil)).To(Equal(0.0))
		Expect(customLabelsBoost(models.CustomLabels{}, map[string][]string{"team": {"payments"}})).To(Equal(0.0))
	})

	It("UT-DS-1661-607-002: exact value match gets full boost per key", func() {
		workflow := models.CustomLabels{"team": {"payments"}}
		filter := map[string][]string{"team": {"payments"}}
		Expect(customLabelsBoost(workflow, filter)).To(BeNumerically("~", 0.15, 1e-9))
	})

	It("UT-DS-1661-607-003: workflow-side wildcard gets half boost", func() {
		workflow := models.CustomLabels{"team": {"*"}}
		filter := map[string][]string{"team": {"payments"}}
		Expect(customLabelsBoost(workflow, filter)).To(BeNumerically("~", 0.075, 1e-9))
	})

	It("UT-DS-1661-607-004: mismatched value contributes no boost for that key", func() {
		workflow := models.CustomLabels{"team": {"platform"}}
		filter := map[string][]string{"team": {"payments"}}
		Expect(customLabelsBoost(workflow, filter)).To(Equal(0.0))
	})

	It("UT-DS-1661-607-005: multiple keys sum additively", func() {
		workflow := models.CustomLabels{"team": {"payments"}, "constraint": {"cost-constrained"}}
		filter := map[string][]string{"team": {"payments"}, "constraint": {"cost-constrained"}}
		Expect(customLabelsBoost(workflow, filter)).To(BeNumerically("~", 0.30, 1e-9))
	})
})

var _ = Describe("finalScore (Issue #1661 Change 6)", func() {
	// Mirrors the LEAST((5.0 + boost - penalty) / 10.0, 1.0) formula in
	// discovery.go's selectScoredWorkflows.

	DescribeTable("UT-DS-1661-608: normalizes and caps the raw score at 1.0",
		func(detectedBoost, customBoost, penalty, expected float64) {
			Expect(finalScore(detectedBoost, customBoost, penalty)).To(BeNumerically("~", expected, 1e-9))
		},
		Entry("no boost/penalty is the baseline 0.5", 0.0, 0.0, 0.0, 0.5),
		Entry("boost increases the score", 0.10, 0.0, 0.0, 0.51),
		Entry("penalty decreases the score", 0.0, 0.0, 0.10, 0.49),
		Entry("score is capped at 1.0 even with a large boost", 5.0, 5.0, 0.0, 1.0),
	)
})

var _ = Describe("matchesSearchFilters (Issue #1661 Change 6, Phase 55 prerequisite)", func() {
	// Mirrors applyListFilters' SQL WHERE-clause combination (crud.go): List's
	// cache-backed filter dimensions are workflow_name (exact), severity/
	// component/environment/priority (wildcard, parity with the discovery
	// path per Issue #522), and status (containment) -- no detectedLabels/
	// customLabels, matching parseWorkflowSearchFilters' query-param parsing.

	buildRW := func(name string, status sharedtypes.CatalogStatus) *rwv1alpha1.RemediationWorkflow {
		return &rwv1alpha1.RemediationWorkflow{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec: rwv1alpha1.RemediationWorkflowSpec{
				Labels: rwv1alpha1.RemediationWorkflowLabels{
					Severity:    []string{"critical"},
					Component:   []string{"v1/Pod"},
					Environment: []string{"production"},
					Priority:    "P1",
				},
			},
			Status: rwv1alpha1.RemediationWorkflowStatus{CatalogStatus: status},
		}
	}

	It("UT-DS-1661-615-001: nil filters match every workflow", func() {
		Expect(matchesSearchFilters(buildRW("wf-a", sharedtypes.CatalogStatusActive), nil)).To(BeTrue())
	})

	It("UT-DS-1661-615-002: workflow_name exact match", func() {
		rw := buildRW("wf-a", sharedtypes.CatalogStatusActive)
		Expect(matchesSearchFilters(rw, &models.WorkflowSearchFilters{WorkflowName: "wf-a"})).To(BeTrue())
		Expect(matchesSearchFilters(rw, &models.WorkflowSearchFilters{WorkflowName: "wf-b"})).To(BeFalse())
	})

	It("UT-DS-1661-615-003: mandatory label dimensions reuse matchesArrayLabel/matchesPriority wildcard semantics", func() {
		rw := buildRW("wf-a", sharedtypes.CatalogStatusActive)
		Expect(matchesSearchFilters(rw, &models.WorkflowSearchFilters{Severity: "critical"})).To(BeTrue())
		Expect(matchesSearchFilters(rw, &models.WorkflowSearchFilters{Severity: "warning"})).To(BeFalse())
		Expect(matchesSearchFilters(rw, &models.WorkflowSearchFilters{Component: "V1/POD"})).To(BeTrue(), "component matching is case-insensitive")
		Expect(matchesSearchFilters(rw, &models.WorkflowSearchFilters{Environment: "production"})).To(BeTrue())
		Expect(matchesSearchFilters(rw, &models.WorkflowSearchFilters{Priority: "P2"})).To(BeFalse())
	})

	It("UT-DS-1661-615-004: status containment -- empty filter matches any status, non-empty filter requires membership", func() {
		active := buildRW("wf-active", sharedtypes.CatalogStatusActive)
		disabled := buildRW("wf-disabled", sharedtypes.CatalogStatusDisabled)

		Expect(matchesSearchFilters(active, &models.WorkflowSearchFilters{})).To(BeTrue(), "no status filter means unconstrained -- KA's FetchValidator relies on this to see every workflow")
		Expect(matchesSearchFilters(disabled, &models.WorkflowSearchFilters{})).To(BeTrue())

		filters := &models.WorkflowSearchFilters{Status: []string{"Active"}}
		Expect(matchesSearchFilters(active, filters)).To(BeTrue())
		Expect(matchesSearchFilters(disabled, filters)).To(BeFalse())
	})

	It("UT-DS-1661-615-005: every dimension must match (AND semantics)", func() {
		rw := buildRW("wf-a", sharedtypes.CatalogStatusActive)
		filters := &models.WorkflowSearchFilters{WorkflowName: "wf-a", Severity: "critical", Status: []string{"Disabled"}}
		Expect(matchesSearchFilters(rw, filters)).To(BeFalse(), "name/severity match but status doesn't -- must reject")
	})
})

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

// Internal test package (package investigator, not investigator_test):
// enrichFromCatalog is unexported and has no other exported seam, so its
// business logic can only be unit-tested from within the package. Coexists
// safely with the investigator_test external suite already registered by
// suite_test.go — Ginkgo's global registry is process-wide, not
// package-scoped, so both packages' Describe/It nodes run under the same
// RunSpecs call.
package investigator

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// ========================================
// UT-KA-337-002 (Issue #1661 Change 11a, DD-WORKFLOW-018)
// ========================================
// enrichFromCatalog is the sole production call site that copies
// parser.WorkflowMeta fields onto the katypes.InvestigationResult KA returns
// to AA (called from finalizeSelfCorrection, the terminal step of
// runWorkflowSelection). It must copy Dependencies/Resources/
// DeclaredParameterNames the same way it already copies ExecutionEngine/
// ExecutionBundle/ServiceAccountName -- otherwise WorkflowMeta gaining these
// fields (UT-KA-337-001) is dead data that never reaches AA.
//
// RED: InvestigationResult has no Dependencies/Resources/DeclaredParameterNames
// fields yet -- this file must fail to compile.
// ========================================
var _ = Describe("enrichFromCatalog — Issue #1661 Change 11a", func() {
	It("UT-KA-337-002: copies Dependencies/Resources/DeclaredParameterNames from WorkflowMeta onto the result", func() {
		v := parser.NewValidator([]string{"wf-with-schema"})
		deps := &models.WorkflowDependencies{
			Secrets: []models.ResourceDependency{{Name: "db-creds"}},
		}
		resources := &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
			Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m")},
		}
		v.SetWorkflowMeta("wf-with-schema", parser.WorkflowMeta{
			ExecutionEngine:        "job",
			Dependencies:           deps,
			Resources:              resources,
			DeclaredParameterNames: map[string]bool{"TARGET_NAMESPACE": true, "REPLICAS": true},
		})

		result := &katypes.InvestigationResult{WorkflowID: "wf-with-schema"}

		enrichFromCatalog(result, v)

		Expect(result.Dependencies).To(Equal(deps))
		Expect(result.Resources).To(Equal(resources))
		Expect(result.DeclaredParameterNames).To(Equal(map[string]bool{"TARGET_NAMESPACE": true, "REPLICAS": true}))
	})

	It("UT-KA-337-002b: leaves Dependencies/Resources/DeclaredParameterNames nil when the workflow has no catalog metadata", func() {
		v := parser.NewValidator([]string{"wf-unknown"})
		result := &katypes.InvestigationResult{WorkflowID: "wf-unknown"}

		enrichFromCatalog(result, v)

		Expect(result.Dependencies).To(BeNil())
		Expect(result.Resources).To(BeNil())
		Expect(result.DeclaredParameterNames).To(BeNil())
	})
})

// ========================================
// UT-KA-1661-651 (Issue #1661 Change 12, DD-WORKFLOW-018)
// ========================================
// ActionType/WorkflowName are catalog-authoritative (never LLM-suppliable),
// so enrichFromCatalog must always overwrite from WorkflowMeta -- same
// unconditional-assignment pattern as Dependencies/Resources/
// DeclaredParameterNames above. This closes the gap where KA never
// populated either field in its wire response, which silently broke
// workflowexecution.execution.started's audit payload (Change 11f).
// ========================================
var _ = Describe("enrichFromCatalog — Issue #1661 Change 12", func() {
	It("UT-KA-1661-651-001: copies ActionType/WorkflowName from WorkflowMeta onto the result", func() {
		v := parser.NewValidator([]string{"wf-with-schema"})
		v.SetWorkflowMeta("wf-with-schema", parser.WorkflowMeta{
			ExecutionEngine: "job",
			ActionType:      "ScaleReplicas",
			WorkflowName:    "scale-memory-fix",
		})

		result := &katypes.InvestigationResult{WorkflowID: "wf-with-schema"}

		enrichFromCatalog(result, v)

		Expect(result.ActionType).To(Equal("ScaleReplicas"))
		Expect(result.WorkflowName).To(Equal("scale-memory-fix"))
	})

	It("UT-KA-1661-651-002: always overwrites a pre-populated ActionType/WorkflowName from the catalog (catalog-authoritative, not LLM-suppliable)", func() {
		v := parser.NewValidator([]string{"wf-with-schema"})
		v.SetWorkflowMeta("wf-with-schema", parser.WorkflowMeta{
			ActionType:   "RestartPod",
			WorkflowName: "restart-pod-fix",
		})

		result := &katypes.InvestigationResult{
			WorkflowID:   "wf-with-schema",
			ActionType:   "llm-supplied-bogus-value",
			WorkflowName: "llm-supplied-bogus-name",
		}

		enrichFromCatalog(result, v)

		Expect(result.ActionType).To(Equal("RestartPod"))
		Expect(result.WorkflowName).To(Equal("restart-pod-fix"))
	})

	It("UT-KA-1661-651-003: leaves ActionType/WorkflowName empty when the workflow has no catalog metadata", func() {
		v := parser.NewValidator([]string{"wf-unknown"})
		result := &katypes.InvestigationResult{WorkflowID: "wf-unknown"}

		enrichFromCatalog(result, v)

		Expect(result.ActionType).To(BeEmpty())
		Expect(result.WorkflowName).To(BeEmpty())
	})
})

// ========================================
// UT-KA-1711 (Issue #1711, DD-KA-001 v1.1)
// ========================================
// DD-KA-001 Step 1 (Workflow Existence) is unconditional: a workflow_id that
// does not resolve against the DS catalog must never survive as structured
// data, even when HumanReviewNeeded was already set to true by an earlier
// signal (e.g. investigation_outcome=inconclusive) that caused
// Validator.Validate() to short-circuit its own allowlist check. Before this
// fix, enrichFromCatalog's !ok branch was a silent no-op, leaving WorkflowID
// (and, by extension, every downstream Required WorkflowSnapshot field it
// gates -- see mapInvestigationResultToResponse's `r.WorkflowID != ""`
// guard) to leak through with no catalog verification at all. Mirrors the
// existing clearing pattern in SelfCorrect exhaustion (validator.go) and
// apiVersionGateExhaustion (this file).
// ========================================
var _ = Describe("enrichFromCatalog — Issue #1711 (unresolvable workflow_id must not survive)", func() {
	It("UT-KA-1711-001: clears WorkflowID when the catalog lookup fails, even though HumanReviewNeeded is already true", func() {
		v := parser.NewValidator([]string{"wf-known"}) // "wf-hallucinated" deliberately absent from the allowlist
		result := &katypes.InvestigationResult{
			WorkflowID: "wf-hallucinated",
			// Simulates the inconclusive-outcome short-circuit (parser.go):
			// HumanReviewNeeded is already true before enrichFromCatalog runs,
			// which is exactly the scenario Validate() skips its own check for.
			HumanReviewNeeded: true,
			HumanReviewReason: "investigation_inconclusive",
		}

		enrichFromCatalog(result, v)

		Expect(result.WorkflowID).To(BeEmpty(),
			"an unresolvable workflow_id must be cleared per DD-KA-001 Step 1, regardless of HumanReviewNeeded")
	})

	It("UT-KA-1711-002: does not clear WorkflowID when the catalog lookup succeeds (regression guard)", func() {
		v := parser.NewValidator([]string{"wf-known"})
		v.SetWorkflowMeta("wf-known", parser.WorkflowMeta{ExecutionEngine: "job"})
		result := &katypes.InvestigationResult{WorkflowID: "wf-known", HumanReviewNeeded: true}

		enrichFromCatalog(result, v)

		Expect(result.WorkflowID).To(Equal("wf-known"))
	})
})

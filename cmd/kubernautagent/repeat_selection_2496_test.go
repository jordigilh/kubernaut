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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/tools/custom"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/workflowcatalog"
	kaopenai "github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/openai"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

type selectionProposal2496 struct {
	workflow       string
	justification  json.RawMessage
	mode           string
	confidence     *float64
	omitConfidence bool
}

// Real cached catalog, catalog fetcher, discovery tools, validator and provider
// HTTP client. Only Kubernetes and the external inference endpoint are replaced.
// DD-KA-2496; BR-KA-016/017/191/197; SI-10/AC-4/AC-6/AU-3.
var _ = Describe("#2496 production repeat acceptance and bounded correction", func() {
	var (
		inv       *investigator.Investigator
		wfA, wfB  string
		history   *prompt.EnrichmentData
		proposals []selectionProposal2496
		mu        sync.Mutex
		requests  []map[string]json.RawMessage
	)
	BeforeEach(func() {
		scheme, err := workflowcatalog.NewScheme()
		Expect(err).NotTo(HaveOccurred())
		// Mirror the production informer index used by list_workflows. Without
		// it the fake API fails the query before selection validation is reached.
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&rwv1alpha1.RemediationWorkflow{}).
			WithIndex(&rwv1alpha1.RemediationWorkflow{}, ".spec.actionType", func(obj client.Object) []string {
				return []string{obj.(*rwv1alpha1.RemediationWorkflow).Spec.ActionType}
			}).Build()
		wfA = seedFakeWorkflow(GinkgoTB(), c, fakeCRDWorkflowYAML)
		wfB = seedFakeWorkflow(GinkgoTB(), c, strings.Replace(fakeCRDWorkflowYAML, "name: test-workflow", "name: alternate-workflow", 1))
		catalog := workflowcatalog.NewLazyCatalogReady(workflowcatalog.NewCacheFromReader(c), logr.Discard())
		reg := registry.New()
		for _, t := range custom.NewAllTools(catalog, nil, logr.Discard()) {
			reg.Register(t)
		}
		score, resolved := 0., false
		history = &prompt.EnrichmentData{HistoryResult: &enrichment.RemediationHistoryResult{
			TargetResource: "prod/Pod/api",
			Tier1:          []enrichment.Tier1Entry{{RemediationUID: "prior-rr-a", WorkflowID: wfA, WorkflowName: "test-workflow", EffectivenessScore: &score, SignalResolved: &resolved, AssessmentReason: "Full", CompletedAt: time.Now().Add(-time.Hour)}},
		}}
		proposals = []selectionProposal2496{{workflow: wfA}}
		requests = nil
		provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			var body map[string]json.RawMessage
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			requests = append(requests, body)
			turn := len(requests)
			mu.Unlock()
			name, arguments := "list_workflows", `{"action_type":"RestartPod"}`
			if turn > 1 {
				index := min(turn-2, len(proposals)-1)
				p := proposals[index]
				workflow := map[string]interface{}{"workflow_id": p.workflow, "confidence": .84, "parameters": map[string]string{"TARGET_NAMESPACE": "prod"}}
				if p.confidence != nil {
					workflow["confidence"] = *p.confidence
				}
				if p.omitConfidence {
					delete(workflow, "confidence")
				}
				if p.justification != nil {
					workflow["repeat_justification"] = p.justification
				}
				payload := map[string]interface{}{"root_cause_analysis": map[string]string{"summary": "Memory exhausted"}, "confidence": .99, "selected_workflow": workflow}
				if p.mode == "inconclusive" {
					payload["investigation_outcome"] = "inconclusive"
				}
				raw, err := json.Marshal(payload)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				name, arguments = investigator.SubmitResultWithWorkflowToolName, string(raw)
				if p.mode == "unparseable" {
					arguments = "not-json"
				}
				if p.mode == "no-workflow" {
					name, arguments = investigator.SubmitResultNoWorkflowToolName, `{"root_cause_analysis":{"summary":"No suitable candidate"},"reasoning":"Do not blindly restart"}`
				}
			}
			message := map[string]interface{}{"role": "assistant", "content": "", "tool_calls": []interface{}{map[string]interface{}{"id": fmt.Sprintf("turn-%d", turn), "type": "function", "function": map[string]string{"name": name, "arguments": arguments}}}}
			finishReason := "tool_calls"
			if turn > 1 && proposals[min(turn-2, len(proposals)-1)].mode == "text" {
				message["content"] = arguments
				delete(message, "tool_calls")
				finishReason = "stop"
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]interface{}{"id": "repeat-2496", "object": "chat.completion", "choices": []interface{}{map[string]interface{}{"index": 0, "finish_reason": finishReason, "message": message}}}); err != nil {
				GinkgoWriter.Printf("provider response: %v\n", err)
			}
		}))
		DeferCleanup(provider.Close)
		builder, err := prompt.NewBuilder()
		Expect(err).NotTo(HaveOccurred())
		inv = investigator.New(investigator.Config{
			Client: kaopenai.New("test-model", provider.URL, "test-key"), Builder: builder, ResultParser: parser.NewResultParser(), Registry: reg,
			Pipeline: investigator.Pipeline{CatalogFetcher: newWorkflowCatalogFetcher(catalog, logr.Discard())}, AuditStore: audit.NopAuditStore{}, Logger: logr.Discard(), MaxTurns: 5,
		})
	})
	valid := json.RawMessage(`{"evidence_basis":"Current workload has a transient resource spike","prior_outcome_relevance":"Prior failure cause remains unknown; consider contrary memory evidence","expected_mechanism":"Restart the process to release transient state"}`)
	invoke := func() *katypes.InvestigationResult {
		GinkgoHelper()
		result, err := inv.RunWorkflowDiscoveryFromRCA(context.Background(), katypes.SignalContext{
			Name: "OOMKilled", Namespace: "prod", ResourceKind: "Pod", ResourceName: "api", ResourceAPIVersion: "v1", Severity: "critical", Environment: "production", Priority: "P0",
		}, &katypes.InvestigationResult{RCASummary: "Memory exhausted", Confidence: .99}, history, "rr-2496")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		mu.Lock()
		captured := append([]map[string]json.RawMessage(nil), requests...)
		mu.Unlock()
		Expect(len(captured)).To(BeNumerically(">=", 2))
		Expect(string(captured[1]["messages"])).To(ContainSubstring(wfA), "the production discovery dispatch must present the first candidate")
		Expect(string(captured[1]["messages"])).To(ContainSubstring(wfB), "repeat history must not filter the alternative")
		return result
	}
	projection := func(result *katypes.InvestigationResult) map[string]interface{} {
		GinkgoHelper()
		raw, err := json.Marshal(result)
		Expect(err).NotTo(HaveOccurred())
		var value map[string]interface{}
		Expect(json.Unmarshal(raw, &value)).To(Succeed())
		return value
	}
	It("IT-KA-2496-AUDIT-006: failed history acquisition requests review before the selection provider is invoked", func() {
		var unavailable prompt.EnrichmentData
		Expect(json.Unmarshal([]byte(`{"HistoryUnavailable":true}`), &unavailable)).To(Succeed())
		result, err := inv.RunWorkflowDiscoveryFromRCA(context.Background(), katypes.SignalContext{
			Namespace: "prod", ResourceKind: "Pod", ResourceName: "api", ResourceAPIVersion: "v1", Severity: "critical",
		}, &katypes.InvestigationResult{RCASummary: "Current observation", Confidence: .93}, &unavailable, "rr-history-failure")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.HumanReviewNeeded).To(BeTrue())
		Expect(result.HumanReviewReason).To(Equal(katypes.HumanReviewReasonRCAIncomplete))
		Expect(result.WorkflowID).To(BeEmpty())
		mu.Lock()
		calls := len(requests)
		mu.Unlock()
		Expect(calls).To(BeZero(), "unavailable repeat context cannot be treated as an empty successful lookup")
	})
	It("IT-KA-2496-AUDIT-006: a legitimate empty history does not block first selection", func() {
		history.HistoryResult.Tier1 = nil
		result := invoke()
		Expect(result.HumanReviewNeeded).To(BeFalse())
		Expect(result.WorkflowID).To(Equal(wfA))
	})
	It("IT-KA-2496-AUDIT-004: sanitizer-expanding repeat reasoning exhausts into review instead of accepted missing explanation", func() {
		proposals[0].justification = json.RawMessage(`{"evidence_basis":"` + strings.Repeat("a", 1989) + ` password=x","prior_outcome_relevance":"Prior completed outcome","expected_mechanism":"Increase limit"}`)
		result := invoke()
		Expect(result.HumanReviewNeeded).To(BeTrue())
		Expect(result.WorkflowID).To(BeEmpty())
		Expect(result.RepeatValidationOutcome).To(Equal("rejected"))
	})

	DescribeTable("IT-KA-2496-003: every malformed same-ID repeat exhausts into review",
		func(justification json.RawMessage) {
			proposals[0].justification = justification
			result := invoke()
			Expect(result.HumanReviewNeeded).To(BeTrue())
			Expect(result.HumanReviewReason).NotTo(Equal("catalog_unavailable"))
			mu.Lock()
			captured := append([]map[string]json.RawMessage(nil), requests...)
			mu.Unlock()
			Expect(len(captured)).To(BeNumerically(">", 2), "missing explanation must reach existing bounded correction")
			Expect(projection(result)).To(HaveKeyWithValue("repeat_validation_outcome", "rejected"))
		},
		Entry("missing", json.RawMessage(nil)), Entry("null", json.RawMessage(`null`)),
		Entry("blank", json.RawMessage(`{"evidence_basis":" ","prior_outcome_relevance":" ","expected_mechanism":" "}`)),
		Entry("partial", json.RawMessage(`{"evidence_basis":"current"}`)),
		Entry("unknown fields instead of explanation", json.RawMessage(`{"reason":"same symptom"}`)),
		Entry("wrong field type", json.RawMessage(`{"evidence_basis":42,"prior_outcome_relevance":"prior","expected_mechanism":"mechanism"}`)),
		Entry("oversized", json.RawMessage(`{"evidence_basis":"`+strings.Repeat("x", 2001)+`","prior_outcome_relevance":"prior","expected_mechanism":"mechanism"}`)),
	)

	DescribeTable("IT-KA-2496-003: repeated selection obeys structural validation after initial, text and parse-recovered proposals",
		func(mode string) {
			proposals = []selectionProposal2496{{workflow: wfA, mode: mode}, {workflow: wfA}, {workflow: wfA, justification: valid}}
			result := invoke()
			Expect(result.HumanReviewNeeded).To(BeFalse())
			Expect(result.WorkflowID).To(Equal(wfA))
			Expect(result.WorkflowName).To(Equal("test-workflow"))
			Expect(result.Confidence).To(Equal(.84))
			Expect(projection(result)).To(HaveKeyWithValue("repeat_justification_required", true))
			Expect(projection(result)).To(HaveKeyWithValue("prior_workflow_match", true))
			Expect(projection(result)).To(HaveKeyWithValue("repeat_validation_outcome", "accepted"))
			Expect(projection(result)).To(HaveKeyWithValue("repeat_justification", HaveKeyWithValue("expected_mechanism", "Restart the process to release transient state")))
		}, Entry("initial", ""), Entry("text fallback", "text"), Entry("parse recovery", "unparseable"), Entry("retained inconclusive proposal", "inconclusive"),
	)

	It("IT-KA-2496-002: a different ID in the same action family remains eligible with no repeat requirement", func() {
		proposals = []selectionProposal2496{{workflow: wfA}, {workflow: wfB}}
		result := invoke()
		Expect(result.WorkflowID).To(Equal(wfB))
		Expect(result.HumanReviewNeeded).To(BeFalse())
		Expect(projection(result)).To(HaveKeyWithValue("repeat_justification_required", false))
		mu.Lock()
		captured := append([]map[string]json.RawMessage(nil), requests...)
		mu.Unlock()
		Expect(string(captured[1]["messages"])).To(ContainSubstring(wfA))
		Expect(string(captured[1]["messages"])).To(ContainSubstring(wfB))
	})

	It("IT-KA-2496-002: completed successful history also requires repeat reasoning", func() {
		score, resolved := .95, true
		history.HistoryResult.Tier1[0].EffectivenessScore = &score
		history.HistoryResult.Tier1[0].SignalResolved = &resolved
		Expect(invoke().HumanReviewNeeded).To(BeTrue())
	})

	It("IT-KA-2496-002: valid historical-tier assessment triggers the same ID rule despite a different recorded name", func() {
		prior := history.HistoryResult.Tier1[0]
		history.HistoryResult.Tier1 = nil
		history.HistoryResult.Tier2 = []enrichment.Tier2Summary{{
			RemediationUID: prior.RemediationUID, WorkflowID: prior.WorkflowID, WorkflowName: "old-readable-name",
			EffectivenessScore: prior.EffectivenessScore, AssessmentReason: "Partial", CompletedAt: prior.CompletedAt,
		}}
		Expect(invoke().HumanReviewNeeded).To(BeTrue())
	})

	It("IT-KA-2496-002: a matching name with a different ID cannot create a repeat requirement", func() {
		history.HistoryResult.Tier1[0].WorkflowID = wfB
		result := invoke()
		Expect(result.WorkflowID).To(Equal(wfA))
		Expect(result.HumanReviewNeeded).To(BeFalse())
		Expect(projection(result)).To(HaveKeyWithValue("prior_workflow_match", false))
	})

	It("IT-KA-2496-002: an unknown effectiveness score does not establish a completed valid repeat", func() {
		history.HistoryResult.Tier1[0].EffectivenessScore = nil
		Expect(invoke().HumanReviewNeeded).To(BeFalse())
	})

	It("IT-KA-2496-003: explicit selection zero survives Phase 1 fallback without a confidence penalty or fabricated provenance", func() {
		zero := 0.
		proposals = []selectionProposal2496{{workflow: wfA, justification: valid, confidence: &zero}}
		result := invoke()
		Expect(result.WorkflowID).To(Equal(wfA))
		Expect(result.Confidence).To(BeZero())
		Expect(projection(result)).To(HaveKeyWithValue("workflow_selection_confidence", float64(0)))
	})

	It("IT-KA-2496-003: omitted selected confidence is not fabricated from RCA confidence", func() {
		proposals = []selectionProposal2496{{workflow: wfA, justification: valid, omitConfidence: true}}
		result := invoke()
		Expect(result.Confidence).To(BeZero())
		Expect(projection(result)).NotTo(HaveKey("workflow_selection_confidence"))
	})

	DescribeTable("IT-KA-2496-002: incomplete, pending or invalid history cannot establish a required repeat",
		func(reason, id, name, target string) {
			if id == "matching-id" {
				id = wfA
			}
			history.HistoryResult.TargetResource = target
			history.HistoryResult.Tier1[0].AssessmentReason = reason
			history.HistoryResult.Tier1[0].WorkflowID = id
			history.HistoryResult.Tier1[0].WorkflowName = name
			result := invoke()
			Expect(result.HumanReviewNeeded).To(BeFalse())
			Expect(projection(result)).To(HaveKeyWithValue("repeat_justification_required", false))
		}, Entry("pending", "", "matching-id", "test-workflow", "prod/Pod/api"), Entry("SpecDrift", "SpecDrift", "matching-id", "test-workflow", "prod/Pod/api"),
		Entry("missing ID", "Full", "", "test-workflow", "prod/Pod/api"), Entry("missing name", "Full", "matching-id", "", "prod/Pod/api"),
		Entry("wrong target", "Full", "matching-id", "test-workflow", "prod/Pod/other"),
	)

	It("IT-KA-2496-003: decline remains possible during correction even though catalog candidates exist", func() {
		proposals = []selectionProposal2496{{workflow: wfA}, {mode: "no-workflow"}}
		result := invoke()
		Expect(result.WorkflowID).To(BeEmpty())
		Expect(result.HumanReviewReason).To(Equal("no_matching_workflows"))
	})

	It("IT-KA-2496-003: the actual correction request explains all three repeat parts without forcing selection", func() {
		proposals = []selectionProposal2496{{workflow: wfA}, {workflow: wfA, justification: valid}}
		result := invoke()
		Expect(result.WorkflowID).To(Equal(wfA))
		mu.Lock()
		captured := append([]map[string]json.RawMessage(nil), requests...)
		mu.Unlock()
		Expect(captured).To(HaveLen(3))
		var messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}
		Expect(json.Unmarshal(captured[2]["messages"], &messages)).To(Succeed())
		feedback := messages[len(messages)-1].Content
		for _, part := range []string{"evidence_basis", "prior_outcome_relevance", "expected_mechanism", "2,000", "submit_result_no_workflow", "current selection confidence"} {
			Expect(feedback).To(ContainSubstring(part))
		}
	})
})

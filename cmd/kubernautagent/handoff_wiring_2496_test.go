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
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/tools/custom"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/workflowcatalog"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	kaopenai "github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/openai"
	k8stools "github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/k8s"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// Reads the exact immutable model-facing JSON, not an audit/conversation lookup.
func handoffPacket2496(request map[string]json.RawMessage) map[string]json.RawMessage {
	GinkgoHelper()
	var messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	Expect(json.Unmarshal(request["messages"], &messages)).To(Succeed())
	Expect(messages).NotTo(BeEmpty())
	_, packet, found := strings.Cut(messages[0].Content, "```json\n{\"schema_version\"")
	Expect(found).To(BeTrue(), "the first Phase 3 request must contain the frozen packet")
	packet, _, found = strings.Cut(packet, "\n```")
	Expect(found).To(BeTrue())
	var result map[string]json.RawMessage
	Expect(json.Unmarshal([]byte(`{"schema_version"`+packet), &result)).To(Succeed())
	return result
}

// DD-KA-2496; BR-KA-016/211/197; SI-10, AC-4. Real production construction,
// Kubernetes tools, sanitizer, cached catalog, parser, dispatch and HTTP client.
// Only the Kubernetes API and inference endpoint are replaced.
var _ = Describe("#2496 frozen handoff production wiring", func() {
	var (
		cfg           *kaconfig.Config
		params        investigationRunnerParams
		requests      []map[string]json.RawMessage
		mu            sync.Mutex
		workflowTurns int
		signal        katypes.SignalContext
		history       *prompt.EnrichmentData
		workflowID    string
		rcaSummary    string
		runtimeLogs   []string
	)
	BeforeEach(func() {
		cfg = kaconfig.DefaultConfig()
		cfg.AI.Summarizer.MaxToolOutputSize = 128 // Deliberately smaller than evidence.
		scheme, err := workflowcatalog.NewScheme()
		Expect(err).NotTo(HaveOccurred())
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&rwv1alpha1.RemediationWorkflow{}).
			WithIndex(&rwv1alpha1.RemediationWorkflow{}, ".spec.actionType", func(obj client.Object) []string {
				return []string{obj.(*rwv1alpha1.RemediationWorkflow).Spec.ActionType}
			}).Build()
		workflowID = seedFakeWorkflow(GinkgoTB(), c, fakeCRDWorkflowYAML)
		catalog := workflowcatalog.NewLazyCatalogReady(workflowcatalog.NewCacheFromReader(c), logr.Discard())
		reg := registry.New()
		for _, tool := range custom.NewAllTools(catalog, nil, logr.Discard()) {
			reg.Register(tool)
		}
		var pod unstructured.Unstructured
		Expect(json.Unmarshal([]byte(fmt.Sprintf(`{"apiVersion":"v1","kind":"Pod","metadata":{"name":"api","namespace":"prod"},"spec":{"parameters":{"OLD_SECRET":"prior-parameter-should-not-leak"}},"status":{"message":%q}}`,
			strings.Repeat("filler ", 100)+"original-observation-tail password=private-private")), &pod)).To(Succeed())
		dyn := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), &pod)
		mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{{Version: "v1"}})
		mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Pod"}, meta.RESTScopeNamespace)
		resolver := k8stools.NewDynamicResolver(dyn, mapper, nil, logr.Discard())
		for _, tool := range k8stools.NewAllTools(kubernetesfake.NewClientset(), resolver) {
			reg.Register(tool)
		}
		requests, workflowTurns, runtimeLogs = nil, 0, nil
		rcaSummary = "Memory exhaustion"
		rcaTurns := 0
		provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			var body map[string]json.RawMessage
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			requests = append(requests, body)
			isSelection := strings.Contains(string(body["tools"]), "list_workflows")
			if isSelection {
				workflowTurns++
			} else {
				rcaTurns++
			}
			wfTurn, rcaTurn := workflowTurns, rcaTurns
			mu.Unlock()
			name := investigator.SubmitResultToolName
			arguments := fmt.Sprintf(`{"root_cause_analysis":{"summary":%q,"remediation_target":{"kind":"Pod","name":"api","namespace":"prod","api_version":"v1"}},"confidence":0.93}`, rcaSummary)
			if !isSelection && rcaTurn == 1 {
				name, arguments = "kubectl_get_by_name", `{"kind":"Pod","name":"api","namespace":"prod"}`
			}
			if isSelection {
				name, arguments = "list_workflows", `{"action_type":"RestartPod"}`
				if wfTurn > 1 {
					name = investigator.SubmitResultWithWorkflowToolName
					justification := ""
					if wfTurn > 2 {
						justification = `,"repeat_justification":{"evidence_basis":"Current transient memory observation","prior_outcome_relevance":"Prior ineffective attempt acknowledged; cause unknown","expected_mechanism":"Release transient state"}`
					}
					arguments = fmt.Sprintf(`{"root_cause_analysis":{"summary":%q},"selected_workflow":{"workflow_id":%q,"confidence":0.84,"parameters":{"TARGET_NAMESPACE":"prod"}%s}}`, rcaSummary, workflowID, justification)
				}
			}
			w.Header().Set("Content-Type", "application/json")
			response := fmt.Sprintf(`{"id":"handoff","object":"chat.completion","choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"id":"turn","type":"function","function":{"name":%q,"arguments":%q}}]}}]}`, name, arguments)
			if _, err := w.Write([]byte(response)); err != nil {
				GinkgoWriter.Printf("provider response: %v\n", err)
			}
		}))
		DeferCleanup(provider.Close)
		builder, err := prompt.NewBuilder()
		Expect(err).NotTo(HaveOccurred())
		params = investigationRunnerParams{
			cfg: cfg, llmRuntime: &kaconfig.LLMRuntimeConfig{Model: "test-model"}, effectiveLLM: kaopenai.New("test-model", provider.URL, "test-key"),
			promptBuilder: builder, resultParser: parser.NewResultParser(), effectiveReg: reg,
			catalogFetcher: newWorkflowCatalogFetcher(catalog, logr.Discard()), sanitizer: buildSanitizationPipeline(cfg, logr.Discard()),
			logger: funcr.New(func(prefix, message string) {
				mu.Lock()
				runtimeLogs = append(runtimeLogs, message)
				mu.Unlock()
				GinkgoWriter.Printf("%s %s\n", prefix, message)
			}, funcr.Options{}),
		}
		signal = katypes.SignalContext{Name: "OOMKilled", ResourceKind: "Pod", ResourceName: "api", ResourceAPIVersion: "v1", Namespace: "prod", Severity: "critical", Environment: "production", Priority: "P0", RemediationID: "rr-handoff-2496"}
		zero, unresolved := 0., false
		history = &prompt.EnrichmentData{HistoryResult: &enrichment.RemediationHistoryResult{
			TargetResource: "prod/Pod/api",
			Tier1:          []enrichment.Tier1Entry{{RemediationUID: "prior-rr", WorkflowID: workflowID, WorkflowName: "test-workflow", WorkflowSelectionConfidence: &zero, EffectivenessScore: &zero, SignalResolved: &unresolved, AssessmentReason: "Full", CompletedAt: time.Date(2026, 10, 9, 6, 0, 0, 0, time.UTC)}},
			Tier2:          []enrichment.Tier2Summary{{RemediationUID: "old-unknown", WorkflowID: workflowID, WorkflowName: "test-workflow"}},
		}}
	})
	newInv := func() *investigator.Investigator {
		swappable, err := llm.NewSwappableClient(params.effectiveLLM, params.llmRuntime.Model)
		Expect(err).NotTo(HaveOccurred())
		phaseResolver := investigator.NewDefaultPhaseResolver(swappable, nil)
		return buildInvestigator(params, nil, audit.NopAuditStore{}, nil, phaseResolver, nil)
	}
	selectionRequests := func() []map[string]json.RawMessage {
		GinkgoHelper()
		mu.Lock()
		defer mu.Unlock()
		var result []map[string]json.RawMessage
		for _, request := range requests {
			if strings.Contains(string(request["tools"]), "list_workflows") {
				result = append(result, request)
			}
		}
		return result
	}
	runInteractive := func(inv *investigator.Investigator) *katypes.InvestigationResult {
		GinkgoHelper()
		ctx := katypes.WithSignalContext(context.Background(), signal)
		_, err := inv.RunInteractiveTurn(ctx, []llm.Message{{Role: "user", Content: "Investigate memory fault"}}, signal.RemediationID)
		Expect(err).NotTo(HaveOccurred())
		rca, err := inv.RunRCAExtractionFromConversation(ctx, []llm.Message{{Role: "user", Content: "Extract the RCA"}}, signal.RemediationID)
		Expect(err).NotTo(HaveOccurred())
		result, err := inv.RunWorkflowDiscoveryFromRCA(ctx, signal, rca, history, signal.RemediationID)
		Expect(err).NotTo(HaveOccurred())
		return result
	}

	It("IT-KA-2496-007: sanitized original observations survive tool truncation and every correction unchanged", func() {
		result := runInteractive(newInv())
		Expect(result.WorkflowID).To(Equal(workflowID), "review reason: %s", result.Reason)
		selected := selectionRequests()
		Expect(selected).To(HaveLen(3))
		packet := handoffPacket2496(selected[0])
		Expect(string(packet["current_observations"])).To(ContainSubstring("original-observation-tail"))
		Expect(string(packet["current_observations"])).NotTo(ContainSubstring("private-private"))
		Expect(string(packet["current_observations"])).NotTo(ContainSubstring("prior-parameter-should-not-leak"))
		for _, key := range []string{"source_kind", "source_type", "source_ref", "source_resource", "parameters", "metadata"} {
			Expect(string(packet["current_observations"])).NotTo(ContainSubstring(`\"` + key + `\"`))
		}
		for _, request := range selected[1:] {
			Expect(handoffPacket2496(request)).To(Equal(packet), "correction must use the same frozen packet")
		}
		Expect(result.HandoffDigest).To(Equal(stringValue2496(packet["handoff_digest"])))
	})

	It("UT-KA-2496-010: canonical digest covers complete history, preserves zero/unknown and has one target", func() {
		result := runInteractive(newInv())
		selected := selectionRequests()
		Expect(selected).NotTo(BeEmpty(), "review reason: %s", result.Reason)
		packet := handoffPacket2496(selected[0])
		Expect(packet).To(HaveKey("target"))
		Expect(packet).NotTo(HaveKey("final_target"))
		Expect(packet).NotTo(HaveKey("remediation_target"))
		Expect(packet).To(HaveKey("target_resolution"))
		var attempts []map[string]json.RawMessage
		Expect(json.Unmarshal(packet["prior_attempts"], &attempts)).To(Succeed())
		Expect(attempts).To(HaveLen(2))
		Expect(string(attempts[0]["workflow_selection_confidence"])).To(Equal("null"))
		Expect(string(attempts[1]["workflow_selection_confidence"])).To(Equal("0"))
		Expect(string(attempts[0]["recorded_at"])).To(Equal("null"))
		digest := stringValue2496(packet["handoff_digest"])
		delete(packet, "handoff_digest")
		delete(packet, "frozen_at")
		canonical, err := json.Marshal(packet)
		Expect(err).NotTo(HaveOccurred())
		Expect(digest).To(Equal(fmt.Sprintf("sha256:%x", sha256.Sum256(canonical))))
		Expect(result.HandoffDigest).To(Equal(digest))
	})

	It("IT-KA-2496-008: configured packet budget routes to review before any selection-provider call", func() {
		loaded, err := kaconfig.Load([]byte("ai:\n  investigation:\n    maxEvidenceHandoffBytes: 256\n"))
		Expect(err).NotTo(HaveOccurred())
		params.cfg = loaded
		result := runInteractive(newInv())
		Expect(result.HumanReviewNeeded).To(BeTrue())
		Expect(result.HumanReviewReason).To(Equal("rca_incomplete"))
		Expect(result.WorkflowID).To(BeEmpty())
		Expect(result.Reason).To(ContainSubstring("evidence handoff"))
		Expect(selectionRequests()).To(BeEmpty(), "no partial packet or provider invocation is permitted")
		mu.Lock()
		logs := strings.Join(runtimeLogs, "\n")
		mu.Unlock()
		Expect(logs).To(ContainSubstring("observation budget exceeded"))
	})

	It("IT-KA-2496-008: whole-packet overflow is observable even when observations fit", func() {
		loaded, err := kaconfig.Load([]byte("ai:\n  investigation:\n    maxEvidenceHandoffBytes: 2048\n"))
		Expect(err).NotTo(HaveOccurred())
		params.cfg = loaded
		rcaSummary = strings.Repeat("Observed memory pressure. ", 200)
		result := runInteractive(newInv())
		Expect(result.HumanReviewNeeded).To(BeTrue())
		Expect(result.HumanReviewReason).To(Equal("rca_incomplete"))
		Expect(result.WorkflowID).To(BeEmpty())
		Expect(selectionRequests()).To(BeEmpty())
		mu.Lock()
		logs := strings.Join(runtimeLogs, "\n")
		mu.Unlock()
		Expect(logs).To(ContainSubstring("evidence handoff exceeds configured byte budget"))
		Expect(logs).NotTo(ContainSubstring("observation budget exceeded"))
	})

	It("IT-KA-2496-008: source serialization failure follows review without a selection-provider call", func() {
		nonfinite := math.NaN()
		result, err := newInv().RunWorkflowDiscoveryFromRCA(context.Background(), signal,
			&katypes.InvestigationResult{RCASummary: "Unserializable confidence", RCAConfidence: &nonfinite}, nil, signal.RemediationID)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.HumanReviewNeeded).To(BeTrue())
		Expect(result.HumanReviewReason).To(Equal("rca_incomplete"))
		Expect(result.WorkflowID).To(BeEmpty())
		Expect(selectionRequests()).To(BeEmpty())
		mu.Lock()
		logs := strings.Join(runtimeLogs, "\n")
		mu.Unlock()
		Expect(logs).To(ContainSubstring("serialize handoff source"))
	})

	It("IT-KA-2496-009: autonomous entry also delivers pre-transform evidence without an audit read", func() {
		result, err := newInv().Investigate(context.Background(), signal)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.WorkflowID).To(Equal(workflowID), "review reason: %s", result.Reason)
		packet := handoffPacket2496(selectionRequests()[0])
		Expect(string(packet["current_observations"])).To(ContainSubstring("original-observation-tail"))
		Expect(result.HandoffDigest).NotTo(BeEmpty())
	})

	It("IT-KA-2496-009: a second request cannot inherit the first request's observations", func() {
		inv := newInv()
		runInteractive(inv)
		mu.Lock()
		workflowTurns = 0
		mu.Unlock()
		signal.RemediationID = "rr-independent-2496"
		result, err := inv.RunWorkflowDiscoveryFromRCA(context.Background(), signal,
			&katypes.InvestigationResult{RCASummary: "Independent RCA"}, nil, signal.RemediationID)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.WorkflowID).To(Equal(workflowID))
		selected := selectionRequests()
		packet := handoffPacket2496(selected[len(selected)-1])
		var observations []json.RawMessage
		Expect(json.Unmarshal(packet["current_observations"], &observations)).To(Succeed())
		Expect(observations).To(BeEmpty(), "the independent request captured no observations")
		var attempts []json.RawMessage
		Expect(json.Unmarshal(packet["prior_attempts"], &attempts)).To(Succeed())
		Expect(attempts).To(BeEmpty(), "the independent request has no target history")
		Expect(string(packet["current_observations"])).NotTo(ContainSubstring("original-observation-tail"))
		Expect(string(packet["unknowns"])).To(ContainSubstring("observations"))
	})
})

func stringValue2496(raw json.RawMessage) string {
	GinkgoHelper()
	var value string
	Expect(json.Unmarshal(raw, &value)).To(Succeed())
	return value
}

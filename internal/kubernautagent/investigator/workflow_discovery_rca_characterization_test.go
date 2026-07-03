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

package investigator_test

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// rcaK8sClient is a fake K8s adapter for enrichment that lets tests fail
// GetOwnerChain for one specific resource name only, so pre-RCA vs
// post-RCA-target re-enrichment can be exercised independently.
type rcaK8sClient struct {
	notFoundFor string
}

func (f *rcaK8sClient) GetOwnerChain(_ context.Context, _, name, _, _ string) ([]enrichment.OwnerChainEntry, error) {
	if f.notFoundFor != "" && name == f.notFoundFor {
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, name)
	}
	return nil, nil
}

func (f *rcaK8sClient) GetSpecHash(_ context.Context, _, _, _, _ string) (string, error) {
	return "hash", nil
}

type rcaDSClient struct{}

func (f *rcaDSClient) GetRemediationHistory(_ context.Context, _, _, _, _ string) (*enrichment.RemediationHistoryResult, error) {
	return &enrichment.RemediationHistoryResult{}, nil
}

// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 4 §7l-1: characterization tests for
// RunWorkflowDiscoveryFromRCA (cyclomatic 27, was at 45.8% coverage) and
// runWorkflowSelection (cyclomatic 31, was at 58.7% coverage), written
// before decomposing either function, per the coverage-before-refactor
// mandate. These pin real business behavior on the interactive-mode
// workflow-discovery path (BR-AI-1044 family), not just line execution.
var _ = Describe("GO-ANTIPATTERN-AUDIT Wave 4: RunWorkflowDiscoveryFromRCA characterization", func() {

	var (
		logger  logr.Logger
		store   *gateRecordingAuditStore
		client  *gateMockLLMClient
		builder *prompt.Builder
		rp      *parser.ResultParser
	)

	BeforeEach(func() {
		logger = logr.Discard()
		store = &gateRecordingAuditStore{}
		client = &gateMockLLMClient{}
		builder, _ = prompt.NewBuilder()
		rp = parser.NewResultParser()
	})

	Describe("UT-KA-WAVE4-001: nil rcaResult is rejected", func() {
		It("should return an error without invoking the LLM", func() {
			enricher := enrichment.NewEnricher(&rcaK8sClient{}, &rcaDSClient{}, store, logger)
			inv := investigator.New(investigator.Config{
				Client: client, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: store, Logger: logger,
				MaxTurns: 15, PhaseTools: investigator.DefaultPhaseToolMap(),
			})

			result, err := inv.RunWorkflowDiscoveryFromRCA(context.Background(),
				katypes.SignalContext{Name: "sig", Namespace: "default"}, nil, nil, "corr-1")

			Expect(err).To(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(client.calls).To(BeEmpty(),
				"UT-KA-WAVE4-001: nil rcaResult must be rejected before any LLM call")
		})
	})

	Describe("UT-KA-WAVE4-002: re-enrichment on a cross-resource RCA target that no longer exists", func() {
		It("should mark the target deleted and still proceed to workflow selection", func() {
			// Signal targets "web-pod"; RCA identifies a different resource
			// ("cache-pod") as the actual remediation target that has since
			// been deleted (race between RCA and workflow discovery).
			k8s := &rcaK8sClient{notFoundFor: "cache-pod"}
			enricher := enrichment.NewEnricher(k8s, &rcaDSClient{}, store, logger)
			inv := investigator.New(investigator.Config{
				Client: client, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: store, Logger: logger,
				MaxTurns: 15, PhaseTools: investigator.DefaultPhaseToolMap(),
			})

			client.responses = []llm.ChatResponse{
				gateWfToolResp(`{"workflow_id":"restart-cache","confidence":0.9,"remediation_target":{"kind":"Pod","name":"cache-pod","namespace":"default"}}`),
			}

			signal := katypes.SignalContext{
				Name: "sig", Namespace: "default",
				ResourceKind: "Pod", ResourceName: "web-pod",
			}
			rcaResult := &katypes.InvestigationResult{
				RCASummary: "cache pod is the real root cause",
				RemediationTarget: katypes.RemediationTarget{
					Kind: "Pod", Name: "cache-pod", Namespace: "default",
				},
			}

			result, err := inv.RunWorkflowDiscoveryFromRCA(context.Background(), signal, rcaResult, nil, "corr-2")

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.WorkflowID).To(Equal("restart-cache"),
				"UT-KA-WAVE4-002: a deleted re-enrichment target must not block workflow selection")
			Expect(result.HumanReviewNeeded).To(BeFalse(),
				"UT-KA-WAVE4-002: a deleted (not hard-failed) re-enrichment target must not force human review")
			// NOTE: deletedResourceWarning is appended to the *local* rcaResult
			// copy inside RunWorkflowDiscoveryFromRCA, which FinalizeWorkflowResult
			// does not propagate into the returned workflow-selection result.
			// This pins existing behavior; it is not asserted as desired.
		})
	})

	Describe("UT-KA-WAVE4-003: apiVersion auto-resolved for a non-ambiguous kind (Issue #1051 parity)", func() {
		It("should populate api_version from the REST mapper without an extra LLM turn", func() {
			mapper := newAmbiguousSubscriptionMapper() // has exactly one Deployment GVK registered
			resolver := investigator.NewMapperScopeResolver(mapper)
			enricher := enrichment.NewEnricher(&rcaK8sClient{}, &rcaDSClient{}, store, logger)
			inv := investigator.New(investigator.Config{
				Client: client, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: store, Logger: logger,
				MaxTurns: 15, PhaseTools: investigator.DefaultPhaseToolMap(),
				ScopeResolver: resolver,
			})

			client.responses = []llm.ChatResponse{
				gateWfToolResp(`{"workflow_id":"scale-up","confidence":0.9}`),
			}

			signal := katypes.SignalContext{Name: "sig", Namespace: "production", ResourceKind: "Deployment", ResourceName: "api-server"}
			rcaResult := &katypes.InvestigationResult{
				RCASummary: "needs more replicas",
				RemediationTarget: katypes.RemediationTarget{
					Kind: "Deployment", Name: "api-server", Namespace: "production",
				},
			}

			result, err := inv.RunWorkflowDiscoveryFromRCA(context.Background(), signal, rcaResult, nil, "corr-3")

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RemediationTarget.APIVersion).To(Equal("apps/v1"),
				"UT-KA-WAVE4-003: unambiguous Deployment kind must auto-resolve apiVersion")
			Expect(client.calls).To(HaveLen(1),
				"UT-KA-WAVE4-003: auto-resolve must not consume an extra LLM turn")
		})
	})

	Describe("UT-KA-WAVE4-004: rcaResult input is copied, not mutated in place", func() {
		It("should leave the caller's rcaResult pointer untouched", func() {
			enricher := enrichment.NewEnricher(&rcaK8sClient{}, &rcaDSClient{}, store, logger)
			inv := investigator.New(investigator.Config{
				Client: client, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: store, Logger: logger,
				MaxTurns: 15, PhaseTools: investigator.DefaultPhaseToolMap(),
			})

			client.responses = []llm.ChatResponse{
				gateWfToolResp(`{"workflow_id":"noop","confidence":0.5}`),
			}

			original := &katypes.InvestigationResult{
				RCASummary: "original",
				RemediationTarget: katypes.RemediationTarget{
					Kind: "Pod", Name: "web-pod", Namespace: "default",
				},
			}
			signal := katypes.SignalContext{Name: "sig", Namespace: "default", ResourceKind: "Pod", ResourceName: "web-pod"}

			_, err := inv.RunWorkflowDiscoveryFromRCA(context.Background(), signal, original, nil, "corr-4")

			Expect(err).NotTo(HaveOccurred())
			Expect(original.RCASummary).To(Equal("original"),
				"UT-KA-WAVE4-004: RunWorkflowDiscoveryFromRCA must defensively copy its rcaResult input")
		})
	})

	Describe("UT-KA-WAVE4-005 (runWorkflowSelection): LLM explicitly declines via submit_result_no_workflow", func() {
		It("should classify as no_matching_workflows without a parse retry", func() {
			enricher := enrichment.NewEnricher(&rcaK8sClient{}, &rcaDSClient{}, store, logger)
			inv := investigator.New(investigator.Config{
				Client: client, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: store, Logger: logger,
				MaxTurns: 15, PhaseTools: investigator.DefaultPhaseToolMap(),
			})

			client.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant"}, ToolCalls: []llm.ToolCall{
					{ID: "tc1", Name: investigator.SubmitResultNoWorkflowToolName, Arguments: `{}`},
				}},
			}

			signal := katypes.SignalContext{Name: "sig", Namespace: "default", ResourceKind: "Pod", ResourceName: "web-pod"}
			rcaResult := &katypes.InvestigationResult{RCASummary: "no workflow fits"}

			result, err := inv.RunWorkflowDiscoveryFromRCA(context.Background(), signal, rcaResult, nil, "corr-5")

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.HumanReviewNeeded).To(BeTrue())
			Expect(result.HumanReviewReason).To(Equal("no_matching_workflows"))
			Expect(client.calls).To(HaveLen(1),
				"UT-KA-WAVE4-005: explicit no-workflow sentinel must not trigger a retry")
		})
	})

	Describe("UT-KA-WAVE4-006 (runWorkflowSelection): unparseable text after exhausting retries", func() {
		It("should classify as no_matching_workflows once retries are exhausted", func() {
			enricher := enrichment.NewEnricher(&rcaK8sClient{}, &rcaDSClient{}, store, logger)
			inv := investigator.New(investigator.Config{
				Client: client, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: store, Logger: logger,
				MaxTurns: 15, PhaseTools: investigator.DefaultPhaseToolMap(),
			})

			// runWorkflowSelection's first response is garbage text (triggers
			// retryWorkflowSubmit); both retries also return garbage text so
			// retries exhaust and the function falls through to the
			// no_matching_workflows classification.
			garbage := llm.ChatResponse{Message: llm.Message{Role: "assistant", Content: "not json at all, no tool call either"}}
			client.responses = []llm.ChatResponse{garbage, garbage, garbage}

			signal := katypes.SignalContext{Name: "sig", Namespace: "default", ResourceKind: "Pod", ResourceName: "web-pod"}
			rcaResult := &katypes.InvestigationResult{RCASummary: "ambiguous root cause"}

			result, err := inv.RunWorkflowDiscoveryFromRCA(context.Background(), signal, rcaResult, nil, "corr-6")

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.HumanReviewNeeded).To(BeTrue())
			Expect(result.HumanReviewReason).To(Equal("no_matching_workflows"))
		})
	})
})

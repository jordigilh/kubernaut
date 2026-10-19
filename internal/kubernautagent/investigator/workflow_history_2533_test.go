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
	"strings"
	"sync"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

type issue2533K8sClient struct{}

func (*issue2533K8sClient) GetOwnerChain(context.Context, string, string, string, string) ([]enrichment.OwnerChainEntry, error) {
	return nil, nil
}

func (*issue2533K8sClient) GetSpecHash(context.Context, string, string, string, string) (string, error) {
	return "sha256:current", nil
}

type issue2533DSClient struct{}

func (*issue2533DSClient) GetRemediationHistory(context.Context, string, string, string, string, string) (*enrichment.RemediationHistoryResult, error) {
	return &enrichment.RemediationHistoryResult{
		TargetResource: "prod/Pod/api-server",
		Tier1: []enrichment.Tier1Entry{{
			RemediationUID:    "rr-prior-2533",
			WorkflowID:        "prior-workflow",
			WorkflowName:      "prior-workflow-name",
			WorkflowRationale: "Prior evidence supported the previous mechanism",
			RepeatJustification: &sharedtypes.RepeatJustification{
				EvidenceBasis:         "Current observations",
				PriorOutcomeRelevance: "The prior outcome was ineffective",
				ExpectedMechanism:     "The mechanism is now addressed",
			},
		}},
	}, nil
}

type issue2533LLMClient struct {
	mu        sync.Mutex
	calls     []llm.ChatRequest
	responses []llm.ChatResponse
}

func (*issue2533LLMClient) Close() error { return nil }

func (c *issue2533LLMClient) Chat(_ context.Context, request llm.ChatRequest) (llm.ChatResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, request)
	if len(c.calls) <= len(c.responses) {
		return c.responses[len(c.calls)-1], nil
	}
	return llm.ChatResponse{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"fallback","confidence":0.1}`}}, nil
}

func (c *issue2533LLMClient) StreamChat(ctx context.Context, request llm.ChatRequest, _ func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	return c.Chat(ctx, request)
}

var _ = Describe("Issue #2533 Phase 3 historical decision handoff", func() {
	It("IT-KA-2533-003: sends the frozen historical decision context to the first workflow-selection request", func() {
		client := &issue2533LLMClient{responses: []llm.ChatResponse{
			{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"Pod OOMKilled","confidence":0.85}`}},
			gateWfToolResp(`{"workflow_id":"restart-pod","confidence":0.9}`),
		}}
		builder, err := prompt.NewBuilder()
		Expect(err).NotTo(HaveOccurred())
		inv := newTestInvestigator(investigator.Config{
			Client: client, Builder: builder, ResultParser: parser.NewResultParser(),
			Enricher:   enrichment.NewEnricher(&issue2533K8sClient{}, &issue2533DSClient{}, &gateRecordingAuditStore{}, logr.Discard()),
			AuditStore: &gateRecordingAuditStore{}, Logger: logr.Discard(),
			MaxTurns: 15, PhaseTools: investigator.DefaultPhaseToolMap(),
		})

		_, err = inv.Investigate(context.Background(), katypes.SignalContext{
			ResourceKind: "Pod", ResourceName: "api-server", Name: "api-server", Namespace: "prod",
			Severity: "critical", Message: "Pod OOMKilled repeatedly",
		})
		Expect(err).NotTo(HaveOccurred())
		var workflowRequest *llm.ChatRequest
		for i := range client.calls {
			if len(client.calls[i].Messages) > 0 && strings.Contains(client.calls[i].Messages[0].Content, "Historical decision context") {
				workflowRequest = &client.calls[i]
				break
			}
		}
		Expect(workflowRequest).NotTo(BeNil(), "the production workflow-selection request must receive historical decision context")
		Expect(workflowRequest.Messages[0].Content).To(ContainSubstring("Prior evidence supported the previous mechanism"))
	})
})

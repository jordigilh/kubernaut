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
	"errors"
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
)

type recurrentHistoryClient2511 struct {
	mu       sync.Mutex
	requests []string
	history  map[string]*enrichment.RemediationHistoryResult
	errors   map[string]error
}

func (c *recurrentHistoryClient2511) GetRemediationHistory(_ context.Context, kind, name, namespace, _, _ string) (*enrichment.RemediationHistoryResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := kind + "/" + name + "/" + namespace
	c.requests = append(c.requests, key)
	if err, ok := c.errors[key]; ok {
		return nil, err
	}
	if result, ok := c.history[key]; ok {
		return result, nil
	}
	return &enrichment.RemediationHistoryResult{}, nil
}

func recurrentHistory2511(target, action, signal string) *enrichment.RemediationHistoryResult {
	return &enrichment.RemediationHistoryResult{
		TargetResource: target,
		Tier1: []enrichment.Tier1Entry{{
			RemediationUID:   "prior-" + action,
			ActionType:       action,
			SignalType:       signal,
			Outcome:          "Success",
			AssessmentReason: "Full",
		}},
	}
}

// BR-KA-016 / DD-KA-2511; FedRAMP SI-10/AC-4/AU-3; SOC 2 CC7.2.
var _ = Describe("#2511 recurrent RCA continuation", func() {
	It("IT-KA-2511-002: continues within the existing RCA budget when the Agent changes target", func() {
		logger := logr.Discard()
		builder, err := prompt.NewBuilder()
		Expect(err).NotTo(HaveOccurred())
		client := &gateMockLLMClient{responses: []llm.ChatResponse{
			{ToolCalls: []llm.ToolCall{{ID: "rca-initial", Name: investigator.SubmitResultToolName, Arguments: `{"root_cause_analysis":{"summary":"initial symptom","contributing_factors":["prior certificate drift"],"causal_chain":["TLS failure","certificate drift"],"remediation_target":{"kind":"Deployment","name":"api-a","namespace":"prod","api_version":"apps/v1"}},"confidence":0.8}`}}},
			{ToolCalls: []llm.ToolCall{{ID: "rca-target-change", Name: investigator.SubmitResultToolName, Arguments: `{"root_cause_analysis":{"summary":"different owning resource","remediation_target":{"kind":"Deployment","name":"api-b","namespace":"prod","api_version":"apps/v1"},"causal_mechanism_status":"probable"},"confidence":0.75,"actionable":false,"investigation_outcome":"inconclusive"}`}}},
			{ToolCalls: []llm.ToolCall{{ID: "rca-final", Name: investigator.SubmitResultToolName, Arguments: `{"root_cause_analysis":{"summary":"validated alternate target","remediation_target":{"kind":"Deployment","name":"api-b","namespace":"prod","api_version":"apps/v1"},"causal_mechanism_status":"unknown"},"confidence":0.7,"actionable":false,"investigation_outcome":"inconclusive"}`}}},
		}}
		ds := &recurrentHistoryClient2511{history: map[string]*enrichment.RemediationHistoryResult{
			"Deployment/api-a/prod": recurrentHistory2511("prod/Deployment/api-a", "rotate_certificate", "TLSHandshakeFailure"),
			"Deployment/api-b/prod": recurrentHistory2511("prod/Deployment/api-b", "scale_replicas", "HighCPU"),
		}}
		enricher := enrichment.NewEnricher(&historySpecHashK8sClient2511{}, ds, &gateRecordingAuditStore{}, logger)
		inv := newTestInvestigator(investigator.Config{
			Client: client, Builder: builder, ResultParser: parser.NewResultParser(),
			Enricher: enricher, AuditStore: &gateRecordingAuditStore{}, Logger: logger, MaxTurns: 6,
			PhaseTools: investigator.DefaultPhaseToolMap(),
		})

		result, err := inv.Investigate(context.Background(), katypes.SignalContext{
			ResourceKind: "Pod", ResourceName: "api-a-pod", Name: "HighCPU", Namespace: "prod", Severity: "high",
			Message: "recurring signal",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RemediationTarget.Name).To(Equal("api-b"))
		Expect(result.CausalMechanismStatus).To(Equal("unknown"))
		Expect(result.ContributingFactors).To(ContainElement("prior certificate drift"))
		Expect(result.CausalChain).To(ContainElements("TLS failure", "certificate drift"))
		Expect(client.calls).To(HaveLen(3))
		Expect(client.calls[1].Messages).To(ContainElement(HaveField("Content", ContainSubstring("PROVISIONAL RCA"))))
		Expect(client.calls[1].Messages).To(ContainElement(HaveField("Content", ContainSubstring("api-a"))))
		Expect(client.calls[1].Messages).To(ContainElement(HaveField("Content", ContainSubstring("initial symptom"))))
		Expect(client.calls[1].Messages).To(ContainElement(HaveField("Content", ContainSubstring("PRIOR REMEDIATION HISTORY REVIEW"))))
		Expect(client.calls[2].Messages).To(ContainElement(HaveField("Content", ContainSubstring("prod/Deployment/api-b"))))
		Expect(ds.requests).To(ContainElements("Deployment/api-a/prod", "Deployment/api-b/prod"))
	})

	It("IT-KA-2518-009 [BR-KA-016, DD-KA-2511]: keeps valid current-target history isolated when a later RCA target source fails", func() {
		logger := logr.Discard()
		builder, err := prompt.NewBuilder()
		Expect(err).NotTo(HaveOccurred())
		client := &gateMockLLMClient{responses: []llm.ChatResponse{
			{
				ToolCalls: []llm.ToolCall{{
					ID: "rca-initial-failure", Name: investigator.SubmitResultToolName,
					Arguments: `{"root_cause_analysis":{"summary":"initial symptom","remediation_target":{"kind":"Deployment","name":"api-a","namespace":"prod","api_version":"apps/v1"}},"confidence":0.8}`,
				}},
			},
			{
				ToolCalls: []llm.ToolCall{{
					ID: "rca-target-failure", Name: investigator.SubmitResultToolName,
					Arguments: `{"root_cause_analysis":{"summary":"different owning resource","remediation_target":{"kind":"Deployment","name":"api-b","namespace":"prod","api_version":"apps/v1"},"causal_mechanism_status":"probable"},"confidence":0.75,"actionable":false,"investigation_outcome":"inconclusive"}`,
				}},
			},
		}}
		ds := &recurrentHistoryClient2511{
			history: map[string]*enrichment.RemediationHistoryResult{
				"Deployment/api-a/prod": recurrentHistory2511("prod/Deployment/api-a", "rotate_certificate", "TLSHandshakeFailure"),
			},
			errors: map[string]error{
				"Deployment/api-b/prod": errors.New("target-b history unavailable"),
			},
		}
		inv := newTestInvestigator(investigator.Config{
			Client: client, Builder: builder, ResultParser: parser.NewResultParser(),
			Enricher:   enrichment.NewEnricher(&historySpecHashK8sClient2511{}, ds, &gateRecordingAuditStore{}, logger),
			AuditStore: &gateRecordingAuditStore{}, Logger: logger, MaxTurns: 6,
			PhaseTools: investigator.DefaultPhaseToolMap(),
		})

		result, err := inv.Investigate(context.Background(), katypes.SignalContext{
			ResourceKind: "Pod", ResourceName: "api-a-pod", Name: "HighCPU", Namespace: "prod", Severity: "high",
			Message: "recurring signal", IncidentID: "incident-target-isolation",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.HumanReviewNeeded).To(BeFalse())
		Expect(result.HumanReviewReason).To(BeEmpty())
		Expect(result.EvidenceStatus).NotTo(BeNil())
		Expect(result.EvidenceStatus.Class).To(Equal(katypes.EvidenceFailureTransportUnavailable))
		Expect(result.EvidenceStatus.Outcome).To(Equal(katypes.EvidenceOutcomeTransportUnavailable))
		Expect(result.EvidenceStatus.Sources).To(ContainElement(HaveField("TargetRef", Equal("prod/Deployment/api-a"))))
		Expect(result.EvidenceStatus.Sources).To(ContainElement(HaveField("TargetRef", Equal("prod/Deployment/api-b"))))
		Expect(result.RemediationTarget.Name).To(Equal("api-b"))
		Expect(client.calls).To(HaveLen(2), "target-B history failure must stop continuation rather than reuse target-A history")
		Expect(client.calls[1].Messages).To(ContainElement(HaveField("Content", ContainSubstring("prod/Deployment/api-a"))))
		Expect(client.calls[1].Messages).NotTo(ContainElement(HaveField("Content", ContainSubstring("prod/Deployment/api-b"))))
		Expect(ds.requests).To(ContainElements("Deployment/api-a/prod", "Deployment/api-b/prod"))
	})
})

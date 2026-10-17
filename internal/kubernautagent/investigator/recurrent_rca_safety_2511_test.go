/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
*/

package investigator_test

import (
	"context"
	"errors"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type historyFailureDSClient2511 struct{}

func (historyFailureDSClient2511) GetRemediationHistory(context.Context, string, string, string, string, string) (*enrichment.RemediationHistoryResult, error) {
	return nil, errors.New("simulated history backend outage")
}

type historySpecHashK8sClient2511 struct{}

func (historySpecHashK8sClient2511) GetOwnerChain(context.Context, string, string, string, string) ([]enrichment.OwnerChainEntry, error) {
	return nil, nil
}

func (historySpecHashK8sClient2511) GetSpecHash(context.Context, string, string, string, string) (string, error) {
	return "sha256:current", nil
}

type historySpecHashFailureK8sClient2511 struct{}

func (historySpecHashFailureK8sClient2511) GetOwnerChain(context.Context, string, string, string, string) ([]enrichment.OwnerChainEntry, error) {
	return nil, nil
}

func (historySpecHashFailureK8sClient2511) GetSpecHash(context.Context, string, string, string, string) (string, error) {
	return "", errors.New("simulated spec hash outage")
}

type historyForbiddenSpecHashK8sClient2511 struct{}

func (historyForbiddenSpecHashK8sClient2511) GetOwnerChain(context.Context, string, string, string, string) ([]enrichment.OwnerChainEntry, error) {
	return nil, nil
}

func (historyForbiddenSpecHashK8sClient2511) GetSpecHash(context.Context, string, string, string, string) (string, error) {
	return "", apierrors.NewForbidden(schema.GroupResource{Group: "apps", Resource: "deployments"}, "api", errors.New("forbidden"))
}

type cancelAfterFirstChatClient2511 struct {
	inner  gateMockLLMClient
	cancel context.CancelFunc
}

func (c *cancelAfterFirstChatClient2511) Close() error { return nil }

func (c *cancelAfterFirstChatClient2511) StreamChat(ctx context.Context, req llm.ChatRequest, _ func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	return c.Chat(ctx, req)
}

func (c *cancelAfterFirstChatClient2511) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	response, err := c.inner.Chat(ctx, req)
	c.cancel()
	return response, err
}

type cancelDuringContinuationClient2511 struct {
	inner    gateMockLLMClient
	cancel   context.CancelFunc
	chatCall int
}

func (c *cancelDuringContinuationClient2511) Close() error { return nil }

func (c *cancelDuringContinuationClient2511) StreamChat(ctx context.Context, req llm.ChatRequest, _ func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	return c.Chat(ctx, req)
}

func (c *cancelDuringContinuationClient2511) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	response, err := c.inner.Chat(ctx, req)
	c.chatCall++
	if c.chatCall == 2 {
		c.cancel()
	}
	return response, err
}

// BR-KA-016 / DD-KA-2511; FedRAMP SI-10/AU-3; SOC 2 CC7.2; ASVS V2.3.1.
var _ = Describe("#2511 recurrent RCA safety boundaries", func() {
	It("IT-KA-2511-003: shares the RCA max-turn budget across provisional submit and continuation", func() {
		logger := logr.Discard()
		client := &gateMockLLMClient{responses: []llm.ChatResponse{{
			ToolCalls: []llm.ToolCall{{
				ID: "initial-submit", Name: investigator.SubmitResultToolName,
				Arguments: `{"root_cause_analysis":{"summary":"initial","remediation_target":{"kind":"Deployment","name":"api-a","namespace":"prod","api_version":"apps/v1"}},"confidence":0.8}`,
			}},
		}}}
		ds := &recurrentHistoryClient2511{history: map[string]*enrichment.RemediationHistoryResult{
			"Deployment/api-a/prod": recurrentHistory2511("prod/Deployment/api-a", "rotate_certificate", "TLSHandshakeFailure"),
		}}
		builder, err := prompt.NewBuilder()
		Expect(err).NotTo(HaveOccurred())
		inv := newTestInvestigator(investigator.Config{
			Client: client, Builder: builder, ResultParser: parser.NewResultParser(),
			Enricher:   enrichment.NewEnricher(&historySpecHashK8sClient2511{}, ds, &gateRecordingAuditStore{}, logger),
			AuditStore: &gateRecordingAuditStore{}, Logger: logger, MaxTurns: 1,
			PhaseTools: investigator.DefaultPhaseToolMap(),
		})

		result, err := inv.Investigate(context.Background(), katypes.SignalContext{
			ResourceKind: "Pod", ResourceName: "api-a-pod", Name: "HighCPU", Namespace: "prod", Severity: "high",
			Message: "recurring signal",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.HumanReviewNeeded).To(BeTrue())
		Expect(result.RCASummary).To(Equal("initial"), "budget exhaustion must preserve the accepted provisional RCA")
		Expect(result.RemediationTarget.Name).To(Equal("api-a"), "budget exhaustion must preserve the accepted provisional target")
		Expect(client.calls).To(HaveLen(1), "continuation must not obtain an extra provider turn after maxTurns is exhausted")
	})

	It("IT-KA-2511-005: fails closed when target history acquisition is unavailable", func() {
		logger := logr.Discard()
		client := &gateMockLLMClient{responses: []llm.ChatResponse{{
			ToolCalls: []llm.ToolCall{{
				ID: "history-failure-submit", Name: investigator.SubmitResultToolName,
				Arguments: `{"root_cause_analysis":{"summary":"history required","remediation_target":{"kind":"Deployment","name":"api","namespace":"prod","api_version":"apps/v1"}},"confidence":0.8}`,
			}},
		}}}
		builder, err := prompt.NewBuilder()
		Expect(err).NotTo(HaveOccurred())
		inv := newTestInvestigator(investigator.Config{
			Client: client, Builder: builder, ResultParser: parser.NewResultParser(),
			Enricher:   enrichment.NewEnricher(&historySpecHashK8sClient2511{}, historyFailureDSClient2511{}, &gateRecordingAuditStore{}, logger),
			AuditStore: &gateRecordingAuditStore{}, Logger: logger, MaxTurns: 4,
			PhaseTools: investigator.DefaultPhaseToolMap(),
		})

		result, err := inv.Investigate(context.Background(), katypes.SignalContext{
			ResourceKind: "Pod", ResourceName: "api-pod", Name: "TLSHandshakeFailure", Namespace: "prod", Severity: "high",
			Message: "history-backed recurrence",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.HumanReviewNeeded).To(BeFalse())
		Expect(result.HumanReviewReason).To(BeEmpty())
		Expect(result.EvidenceStatus).NotTo(BeNil())
		Expect(result.EvidenceStatus.Class).To(Equal(katypes.EvidenceFailureTransportUnavailable))
		Expect(result.EvidenceStatus.Outcome).To(Equal(katypes.EvidenceOutcomeTransportUnavailable))
		Expect(result.Warnings).To(ContainElement(ContainSubstring("history is unavailable")))
		Expect(client.calls).To(HaveLen(1), "failed history acquisition must not trigger an ungrounded continuation")
	})

	It("IT-KA-2511-008: preserves cancellation when it arrives before history continuation", func() {
		ctx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)
		client := &cancelAfterFirstChatClient2511{
			inner: gateMockLLMClient{responses: []llm.ChatResponse{{
				ToolCalls: []llm.ToolCall{{
					ID: "cancel-submit", Name: investigator.SubmitResultToolName,
					Arguments: `{"root_cause_analysis":{"summary":"cancel me","remediation_target":{"kind":"Deployment","name":"api-a","namespace":"prod","api_version":"apps/v1"}},"confidence":0.8}`,
				}},
			}}},
			cancel: cancel,
		}
		ds := &recurrentHistoryClient2511{history: map[string]*enrichment.RemediationHistoryResult{
			"Deployment/api-a/prod": recurrentHistory2511("prod/Deployment/api-a", "rotate_certificate", "TLSHandshakeFailure"),
		}}
		builder, err := prompt.NewBuilder()
		Expect(err).NotTo(HaveOccurred())
		inv := newTestInvestigator(investigator.Config{
			Client: client, Builder: builder, ResultParser: parser.NewResultParser(),
			Enricher:   enrichment.NewEnricher(&gateK8sClient{}, ds, &gateRecordingAuditStore{}, logr.Discard()),
			AuditStore: &gateRecordingAuditStore{}, Logger: logr.Discard(), MaxTurns: 4,
			PhaseTools: investigator.DefaultPhaseToolMap(),
		})

		result, err := inv.Investigate(ctx, katypes.SignalContext{
			ResourceKind: "Pod", ResourceName: "api-a-pod", Name: "HighCPU", Namespace: "prod", Severity: "high",
			Message: "recurring signal",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Cancelled).To(BeTrue())
		Expect(client.inner.calls).To(HaveLen(1), "cancellation must prevent a continuation provider call")
	})

	It("IT-KA-2511-009: treats spec-hash acquisition failure as unavailable history", func() {
		client := &gateMockLLMClient{responses: []llm.ChatResponse{{
			ToolCalls: []llm.ToolCall{{
				ID: "spec-hash-failure-submit", Name: investigator.SubmitResultToolName,
				Arguments: `{"root_cause_analysis":{"summary":"hash unavailable","remediation_target":{"kind":"Deployment","name":"api","namespace":"prod","api_version":"apps/v1"}},"confidence":0.8}`,
			}},
		}}}
		builder, err := prompt.NewBuilder()
		Expect(err).NotTo(HaveOccurred())
		inv := newTestInvestigator(investigator.Config{
			Client: client, Builder: builder, ResultParser: parser.NewResultParser(),
			Enricher:   enrichment.NewEnricher(&historySpecHashFailureK8sClient2511{}, historyFailureDSClient2511{}, &gateRecordingAuditStore{}, logr.Discard()),
			AuditStore: &gateRecordingAuditStore{}, Logger: logr.Discard(), MaxTurns: 4,
			PhaseTools: investigator.DefaultPhaseToolMap(),
		})

		result, err := inv.Investigate(context.Background(), katypes.SignalContext{
			ResourceKind: "Pod", ResourceName: "api-pod", Name: "TLSHandshakeFailure", Namespace: "prod", Severity: "high",
			Message: "spec hash unavailable",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.HumanReviewNeeded).To(BeFalse())
		Expect(result.HumanReviewReason).To(BeEmpty())
		Expect(result.EvidenceStatus).NotTo(BeNil())
		Expect(result.EvidenceStatus.Class).To(Equal(katypes.EvidenceFailureTransportUnavailable))
		Expect(result.EvidenceStatus.Outcome).To(Equal(katypes.EvidenceOutcomeTransportUnavailable))
		Expect(client.calls).To(HaveLen(1), "spec-hash failure must not permit an ungrounded continuation")
	})

	It("IT-KA-2511-010: fails closed when forbidden spec-hash access prevents history acquisition", func() {
		client := &gateMockLLMClient{responses: []llm.ChatResponse{{
			ToolCalls: []llm.ToolCall{{
				ID: "forbidden-spec-hash-submit", Name: investigator.SubmitResultToolName,
				Arguments: `{"root_cause_analysis":{"summary":"forbidden hash","remediation_target":{"kind":"Deployment","name":"api","namespace":"prod","api_version":"apps/v1"}},"confidence":0.8}`,
			}},
		}}}
		builder, err := prompt.NewBuilder()
		Expect(err).NotTo(HaveOccurred())
		inv := newTestInvestigator(investigator.Config{
			Client: client, Builder: builder, ResultParser: parser.NewResultParser(),
			Enricher:   enrichment.NewEnricher(&historyForbiddenSpecHashK8sClient2511{}, historyFailureDSClient2511{}, &gateRecordingAuditStore{}, logr.Discard()),
			AuditStore: &gateRecordingAuditStore{}, Logger: logr.Discard(), MaxTurns: 4,
			PhaseTools: investigator.DefaultPhaseToolMap(),
		})

		result, err := inv.Investigate(context.Background(), katypes.SignalContext{
			ResourceKind: "Pod", ResourceName: "api-pod", Name: "TLSHandshakeFailure", Namespace: "prod", Severity: "high",
			Message: "forbidden history",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.HumanReviewNeeded).To(BeFalse())
		Expect(result.HumanReviewReason).To(BeEmpty())
		Expect(result.EvidenceStatus).NotTo(BeNil())
		Expect(result.EvidenceStatus.Class).To(Equal(katypes.EvidenceFailureSchemaClientRequestFailure))
		Expect(result.EvidenceStatus.Outcome).To(Equal(katypes.EvidenceOutcomeSchemaClientFailure))
		Expect(client.calls).To(HaveLen(1), "forbidden history acquisition must not permit an ungrounded continuation")
	})

	It("IT-KA-2511-014: preserves cancellation when continuation retry is interrupted", func() {
		ctx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)
		client := &cancelDuringContinuationClient2511{
			inner: gateMockLLMClient{responses: []llm.ChatResponse{
				{ToolCalls: []llm.ToolCall{{
					ID: "retry-cancel-initial", Name: investigator.SubmitResultToolName,
					Arguments: `{"root_cause_analysis":{"summary":"preserve me","remediation_target":{"kind":"Deployment","name":"api","namespace":"prod","api_version":"apps/v1"}},"confidence":0.8}`,
				}}},
				{Message: llm.Message{Role: "assistant", Content: "malformed continuation"}},
			}},
			cancel: cancel,
		}
		ds := &recurrentHistoryClient2511{history: map[string]*enrichment.RemediationHistoryResult{
			"Deployment/api/prod": recurrentHistory2511("prod/Deployment/api", "rotate_certificate", "TLSHandshakeFailure"),
		}}
		builder, err := prompt.NewBuilder()
		Expect(err).NotTo(HaveOccurred())
		inv := newTestInvestigator(investigator.Config{
			Client: client, Builder: builder, ResultParser: parser.NewResultParser(),
			Enricher:   enrichment.NewEnricher(&gateK8sClient{}, ds, &gateRecordingAuditStore{}, logr.Discard()),
			AuditStore: &gateRecordingAuditStore{}, Logger: logr.Discard(), MaxTurns: 4,
			PhaseTools: investigator.DefaultPhaseToolMap(),
		})

		result, err := inv.Investigate(ctx, katypes.SignalContext{
			ResourceKind: "Pod", ResourceName: "api-pod", Name: "TLSHandshakeFailure", Namespace: "prod", Severity: "high",
			Message: "cancel continuation retry",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Cancelled).To(BeTrue())
		Expect(result.RCASummary).To(Equal("preserve me"))
		Expect(result.HumanReviewReason).NotTo(Equal("llm_parsing_error"))
		Expect(client.inner.calls).To(HaveLen(2), "cancellation must prevent the parse retry provider call")
	})

})

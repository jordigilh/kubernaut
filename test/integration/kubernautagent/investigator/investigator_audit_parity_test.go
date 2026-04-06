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
	"fmt"
	"log/slog"
	"os"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

type errorLLMClient struct {
	err error
}

func (e *errorLLMClient) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	return llm.ChatResponse{}, e.err
}

var _ = Describe("KA Audit Parity Integration — TP-433-AUDIT-SOC2", func() {

	var (
		logger     *slog.Logger
		auditStore *recordingAuditStore
		mockClient *mockLLMClient
		builder    *prompt.Builder
		rp         *parser.ResultParser
		enricher   *enrichment.Enricher
		phaseTools katypes.PhaseToolMap
	)

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
		auditStore = &recordingAuditStore{}
		mockClient = &mockLLMClient{}
		builder, _ = prompt.NewBuilder()
		rp = parser.NewResultParser()
		k8sClient := &fakeK8sClient{
			ownerChain: []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "api-server", Namespace: "production"},
			},
		}
		dsClient := &fakeDataStorageClient{
			history: &enrichment.RemediationHistoryResult{},
		}
		enricher = enrichment.NewEnricher(k8sClient, dsClient, auditStore, logger)
		phaseTools = investigator.DefaultPhaseToolMap()
	})

	eventsOfType := func(eventType string) []*audit.AuditEvent {
		var result []*audit.AuditEvent
		for _, e := range auditStore.events {
			if e.EventType == eventType {
				result = append(result, e)
			}
		}
		return result
	}

	signal := katypes.SignalContext{
		Name:         "api-server-abc",
		Namespace:    "production",
		Severity:     "critical",
		Message:      "OOMKilled",
		ResourceKind: "Deployment",
		ResourceName: "api-server",
		RemediationID: "rem-it-audit-parity",
	}

	Describe("IT-KA-433-AP-001: Investigation emits llm.request with model and prompt_preview", func() {
		It("should include model name and prompt_preview in llm.request event", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOMKilled","human_review_needed":true}`}},
			}
			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools,
				ModelName: "claude-sonnet-4-20250514",
			})

			_, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())

			reqEvents := eventsOfType(audit.EventTypeLLMRequest)
			Expect(reqEvents).NotTo(BeEmpty())
			first := reqEvents[0]
			Expect(first.Data["model"]).To(Equal("claude-sonnet-4-20250514"))
			Expect(first.Data["prompt_preview"]).NotTo(BeEmpty())
			Expect(first.Data["prompt_length"]).To(BeNumerically(">", 0))
		})
	})

	Describe("IT-KA-433-AP-002: Investigation emits llm.response with analysis fields", func() {
		It("should include has_analysis and analysis_preview in llm.response event", func() {
			mockClient.responses = []llm.ChatResponse{
				{
					Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOMKilled due to memory leak"}`},
					Usage:   llm.TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
				},
				{Message: llm.Message{Role: "assistant", Content: `{"workflow_id":"oom-recovery","confidence":0.9}`}},
			}
			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools,
			})

			_, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())

			respEvents := eventsOfType(audit.EventTypeLLMResponse)
			Expect(respEvents).NotTo(BeEmpty())
			first := respEvents[0]
			Expect(first.Data["has_analysis"]).To(BeTrue())
			Expect(first.Data["analysis_preview"]).NotTo(BeEmpty())
			Expect(first.Data["analysis_length"]).To(BeNumerically(">", 0))
		})
	})

	Describe("IT-KA-433-AP-003: Investigation emits per-tool-call events", func() {
		It("should emit 2 separate llm.tool_call events for 2 tool calls", func() {
			reg := registry.New()
			reg.Register(&fakeTool{name: "get_pods", result: `{"items":[{"name":"api"}]}`})
			reg.Register(&fakeTool{name: "get_logs", result: `{"logs":"OOMKilled"}`})

			mockClient.responses = []llm.ChatResponse{
				{
					Message: llm.Message{Role: "assistant", Content: "investigating"},
					ToolCalls: []llm.ToolCall{
						{ID: "tc_1", Name: "get_pods", Arguments: `{"namespace":"production"}`},
						{ID: "tc_2", Name: "get_logs", Arguments: `{"pod":"api"}`},
					},
				},
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOMKilled"}`}},
				{Message: llm.Message{Role: "assistant", Content: `{"workflow_id":"oom-recovery","confidence":0.9}`}},
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools,
				Registry: reg,
			})

			_, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())

			tcEvents := eventsOfType(audit.EventTypeLLMToolCall)
			Expect(tcEvents).To(HaveLen(2))
			Expect(tcEvents[0].Data["tool_call_index"]).To(Equal(0))
			Expect(tcEvents[0].Data["tool_name"]).To(Equal("get_pods"))
			Expect(tcEvents[0].Data["tool_result"]).NotTo(BeEmpty())
			Expect(tcEvents[1].Data["tool_call_index"]).To(Equal(1))
			Expect(tcEvents[1].Data["tool_name"]).To(Equal("get_logs"))
		})
	})

	Describe("IT-KA-433-AP-005: Investigation emits response.complete with response data", func() {
		It("should include response_data and cumulative tokens in response.complete event", func() {
			mockClient.responses = []llm.ChatResponse{
				{
					Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOMKilled"}`},
					Usage:   llm.TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
				},
				{
					Message: llm.Message{Role: "assistant", Content: `{"workflow_id":"oom-recovery","confidence":0.9,"remediation_target":{"kind":"Deployment","name":"api-server","namespace":"production"}}`},
					Usage:   llm.TokenUsage{PromptTokens: 200, CompletionTokens: 100, TotalTokens: 300},
				},
			}
			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools,
			})

			_, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())

			completeEvents := eventsOfType(audit.EventTypeResponseComplete)
			Expect(completeEvents).NotTo(BeEmpty())
			last := completeEvents[len(completeEvents)-1]
			Expect(last.Data["response_data"]).NotTo(BeEmpty())
			Expect(last.Data["total_prompt_tokens"]).To(BeNumerically(">", 0))
			Expect(last.Data["total_completion_tokens"]).To(BeNumerically(">", 0))
		})
	})

	Describe("IT-KA-433-AP-006: Investigation emits response.failed on LLM error", func() {
		It("should include error_message and phase in response.failed event", func() {
			failingClient := &errorLLMClient{err: fmt.Errorf("LLM timeout after 30s")}
			inv := investigator.New(investigator.Config{
				Client: failingClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools,
			})

			_, err := inv.Investigate(context.Background(), signal)
			Expect(err).To(HaveOccurred())

			failEvents := eventsOfType(audit.EventTypeResponseFailed)
			Expect(failEvents).NotTo(BeEmpty())
			first := failEvents[0]
			Expect(first.Data["error_message"]).NotTo(BeEmpty())
			Expect(first.Data["phase"]).NotTo(BeEmpty())
			Expect(first.EventAction).To(Equal(audit.ActionResponseFailed))
			Expect(first.EventOutcome).To(Equal(audit.OutcomeFailure))
		})
	})

	Describe("IT-KA-433-AP-007: All investigator events have UUID event_id", func() {
		It("should have valid UUID event_id in every audit event from investigator", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"test","human_review_needed":true}`}},
			}
			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools,
			})

			_, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())

			investigatorTypes := map[string]bool{
				audit.EventTypeLLMRequest:        true,
				audit.EventTypeLLMResponse:       true,
				audit.EventTypeLLMToolCall:       true,
				audit.EventTypeValidationAttempt: true,
				audit.EventTypeResponseComplete:  true,
				audit.EventTypeResponseFailed:    true,
			}

			for _, e := range auditStore.events {
				if !investigatorTypes[e.EventType] {
					continue
				}
				rawID, ok := e.Data["event_id"]
				Expect(ok).To(BeTrue(), "event_id missing on %s event", e.EventType)
				idStr, ok := rawID.(string)
				Expect(ok).To(BeTrue())
				_, parseErr := uuid.Parse(idStr)
				Expect(parseErr).NotTo(HaveOccurred(), "invalid UUID on %s event: %s", e.EventType, idStr)
			}
		})
	})

	Describe("IT-KA-433-AP-008: All investigator events have EventAction and EventOutcome", func() {
		It("should set EventAction and EventOutcome on llm.request and llm.response events", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"test"}`}},
				{Message: llm.Message{Role: "assistant", Content: `{"workflow_id":"restart","confidence":0.8}`}},
			}
			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools,
			})

			_, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())

			for _, e := range auditStore.events {
				if e.EventType == audit.EventTypeLLMRequest {
					Expect(e.EventAction).To(Equal(audit.ActionLLMRequest), "LLM request should have EventAction")
					Expect(e.EventOutcome).To(Equal(audit.OutcomeSuccess), "LLM request should have EventOutcome=success")
				}
				if e.EventType == audit.EventTypeLLMResponse {
					Expect(e.EventAction).To(Equal(audit.ActionLLMResponse), "LLM response should have EventAction")
					Expect(e.EventOutcome).To(Equal(audit.OutcomeSuccess), "LLM response should have EventOutcome=success")
				}
				if e.EventType == audit.EventTypeResponseComplete {
					Expect(e.EventAction).To(Equal(audit.ActionResponseSent))
					Expect(e.EventOutcome).To(Equal(audit.OutcomeSuccess))
				}
			}
		})
	})

	Describe("IT-KA-433-AP-020: Re-enrichment must NOT copy labels across different resource identities", func() {
		It("should mark labels as failed when RCA target differs and cannot be resolved", func() {
			scheme := runtime.NewScheme()
			_ = appsv1.AddToScheme(scheme)
			_ = autoscalingv2.AddToScheme(scheme)
			_ = policyv1.AddToScheme(scheme)
			_ = networkingv1.AddToScheme(scheme)
			_ = corev1.AddToScheme(scheme)

			apiServerDeploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-server",
					Namespace: "production",
					Labels: map[string]string{
						"app.kubernetes.io/managed-by": "Helm",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "api-server"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "api-server"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{Name: "app", Image: "app:latest"}},
						},
					},
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, apiServerDeploy)
			ld := enrichment.NewLabelDetector(dynClient)

			k8sClientForLabels := &resourceAwareK8sClient{
				chains: map[string][]enrichment.OwnerChainEntry{
					"api-server": {{Kind: "Deployment", Name: "api-server", Namespace: "production"}},
					"worker":     {{Kind: "Deployment", Name: "worker", Namespace: "production"}},
				},
			}
			labelEnricher := enrichment.NewEnricher(
				k8sClientForLabels,
				&fakeDataStorageClient{history: &enrichment.RemediationHistoryResult{}},
				auditStore,
				logger,
			).WithLabelDetector(ld)

			labelMockClient := &mockLLMClient{}
			labelMockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary": "OOM due to worker memory leak",
					"remediation_target": {"kind": "Deployment", "name": "worker", "namespace": "production"}
				}`}},
				{Message: llm.Message{Role: "assistant", Content: `{
					"workflow_id": "oom-recovery",
					"confidence": 0.9
				}`}},
			}

			labelSignal := katypes.SignalContext{
				Name:          "api-server-abc",
				Namespace:     "production",
				Severity:      "high",
				Message:       "OOMKilled",
				ResourceKind:  "Deployment",
				ResourceName:  "api-server",
				RemediationID: "rem-it-label-020",
			}

			inv := investigator.New(investigator.Config{
				Client:       labelMockClient,
				Builder:      builder,
				ResultParser: rp,
				Enricher:     labelEnricher,
				AuditStore:   auditStore,
				Logger:       logger,
				MaxTurns:     15,
				PhaseTools:   phaseTools,
			})

			result, err := inv.Investigate(context.Background(), labelSignal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(result.DetectedLabels).NotTo(BeNil(),
				"DetectedLabels must be populated from re-enrichment")

			helmVal, hasHelm := result.DetectedLabels["helmManaged"]
			if hasHelm {
				Expect(helmVal).To(BeFalse(),
					"helmManaged must NOT be inherited from api-server — worker is a different resource identity")
			}

			failedRaw, hasFailed := result.DetectedLabels["failedDetections"]
			Expect(hasFailed).To(BeTrue(),
				"failedDetections must be present since worker Deployment does not exist")
			failedSlice, ok := failedRaw.([]string)
			Expect(ok).To(BeTrue())
			Expect(failedSlice).To(HaveLen(len(enrichment.AllDetectionCategories)),
				"all detection categories must be listed as failed for non-existent resource")
		})
	})
})

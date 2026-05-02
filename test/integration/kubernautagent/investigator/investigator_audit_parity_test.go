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
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/alignment"
	alignprompt "github.com/jordigilh/kubernaut/internal/kubernautagent/alignment/prompt"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/config"
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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

// shadowMockLLMClient is a thread-safe mock for the shadow evaluator.
type shadowMockLLMClient struct {
	mu        sync.Mutex
	responses []llm.ChatResponse
	callIdx   int
	calls     int
}

func (m *shadowMockLLMClient) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.callIdx < len(m.responses) {
		resp := m.responses[m.callIdx]
		m.callIdx++
		return resp, nil
	}
	return llm.ChatResponse{
		Message: llm.Message{Role: "assistant", Content: `{"suspicious":false,"explanation":"fallback clean"}`},
	}, nil
}

func (m *shadowMockLLMClient) Close() error { return nil }

func (m *shadowMockLLMClient) totalCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

type errorLLMClient struct {
	err error
}

func (e *errorLLMClient) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	return llm.ChatResponse{}, e.err
}

func (e *errorLLMClient) Close() error { return nil }

var _ = Describe("KA Audit Parity Integration — TP-433-AUDIT-SOC2", func() {

	var (
		invLogger  logr.Logger
		auditStore *recordingAuditStore
		mockClient *mockLLMClient
		builder    *prompt.Builder
		rp         *parser.ResultParser
		enricher   *enrichment.Enricher
		phaseTools katypes.PhaseToolMap
	)

	BeforeEach(func() {
		invLogger = logr.Discard()
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
		enricher = enrichment.NewEnricher(k8sClient, dsClient, auditStore, invLogger)
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
		Name:          "api-server-abc",
		Namespace:     "production",
		Severity:      "critical",
		Message:       "OOMKilled",
		ResourceKind:  "Deployment",
		ResourceName:  "api-server",
		RemediationID: "rem-it-audit-parity",
	}

	Describe("IT-KA-433-AP-001: Investigation emits llm.request with model and prompt_preview", func() {
		It("should include model name and prompt_preview in llm.request events for both phases", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOMKilled","confidence":0.9}`}},
				wfToolResp(`{"workflow_id":"oom-increase-memory","confidence":0.9}`),
			}
			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: invLogger, MaxTurns: 15, PhaseTools: phaseTools,
				ModelName: "claude-sonnet-4-20250514",
			})

			_, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())

			reqEvents := eventsOfType(audit.EventTypeLLMRequest)
			Expect(reqEvents).To(HaveLen(2),
				"two-phase investigation (RCA + workflow selection) should emit 2 llm.request events")
			first := reqEvents[0]
			Expect(first.Data["model"]).To(Equal("claude-sonnet-4-20250514"))
			preview, ok := first.Data["prompt_preview"].(string)
			Expect(ok).To(BeTrue(), "expected type assertion to string for prompt_preview in llm.request event data to succeed")
			Expect(preview).To(ContainSubstring(signal.Name), "prompt_preview should embed signal name")
			promptLen, ok := first.Data["prompt_length"].(int)
			Expect(ok).To(BeTrue(), "expected type assertion to int for prompt_length in llm.request event data to succeed")
			Expect(promptLen).To(BeNumerically(">=", len(preview)), "prompt_length should be at least as long as the preview")
		})
	})

	Describe("IT-KA-433-AP-002: Investigation emits llm.response with analysis fields", func() {
		It("should include has_analysis and analysis_preview in llm.response event", func() {
			expectedContent := `{"rca_summary":"OOMKilled due to memory leak"}`
			mockClient.responses = []llm.ChatResponse{
				{
					Message: llm.Message{Role: "assistant", Content: expectedContent},
					Usage:   llm.TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
				},
				wfToolResp(`{"workflow_id":"oom-recovery","confidence":0.9}`),
			}
			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: invLogger, MaxTurns: 15, PhaseTools: phaseTools,
			})

			_, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())

			respEvents := eventsOfType(audit.EventTypeLLMResponse)
			Expect(respEvents).To(HaveLen(2))
			first := respEvents[0]
			Expect(first.Data["has_analysis"]).To(Equal(true))
			Expect(first.Data["analysis_preview"]).To(Equal(expectedContent))
			Expect(first.Data["analysis_full"]).To(Equal(expectedContent))
			Expect(first.Data["analysis_length"]).To(Equal(len(expectedContent)))
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
				wfToolResp(`{"workflow_id":"oom-recovery","confidence":0.9}`),
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: invLogger, MaxTurns: 15, PhaseTools: phaseTools,
				Registry: reg,
			})

			_, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())

			tcEvents := eventsOfType(audit.EventTypeLLMToolCall)
			Expect(tcEvents).To(HaveLen(2))
			Expect(tcEvents[0].Data["tool_call_index"]).To(Equal(0))
			Expect(tcEvents[0].Data["tool_name"]).To(Equal("get_pods"))
			Expect(tcEvents[0].Data["tool_result"]).To(Equal(`{"items":[{"name":"api"}]}`))
			Expect(tcEvents[1].Data["tool_call_index"]).To(Equal(1))
			Expect(tcEvents[1].Data["tool_name"]).To(Equal("get_logs"))
			Expect(tcEvents[1].Data["tool_result"]).To(Equal(`{"logs":"OOMKilled"}`))
		})
	})

	Describe("IT-KA-433-AP-005: Investigation emits response.complete with response data", func() {
		It("should include response_data and cumulative tokens in response.complete event", func() {
			mockClient.responses = []llm.ChatResponse{
				{
					Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOMKilled"}`},
					Usage:   llm.TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
				},
				func() llm.ChatResponse {
					r := wfToolResp(`{"workflow_id":"oom-recovery","confidence":0.9,"remediation_target":{"kind":"Deployment","name":"api-server","namespace":"production"}}`)
					r.Usage = llm.TokenUsage{PromptTokens: 200, CompletionTokens: 100, TotalTokens: 300}
					return r
				}(),
			}
			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: invLogger, MaxTurns: 15, PhaseTools: phaseTools,
			})

			_, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())

			completeEvents := eventsOfType(audit.EventTypeResponseComplete)
			Expect(completeEvents).To(HaveLen(1))
			last := completeEvents[0]
			Expect(last.Data["response_data"]).To(ContainSubstring("oom-recovery"), "response_data should contain the selected workflow")
			Expect(last.Data["total_prompt_tokens"]).To(Equal(300))
			Expect(last.Data["total_completion_tokens"]).To(Equal(150))
		})
	})

	Describe("IT-KA-851-AP-001: Investigation emits rca.complete after Phase 1 with forensic fields", func() {
		It("should emit aiagent.rca.complete with response_data containing causal_chain and due_diligence", func() {
			mockClient.responses = []llm.ChatResponse{
				{
					Message: llm.Message{Role: "assistant", Content: `{
						"rca_summary":"DiskPressure from emptyDir overuse",
						"severity":"critical",
						"confidence":0.92,
						"remediation_target":{"kind":"Deployment","name":"postgres-emptydir","namespace":"demo-disk"},
						"causal_chain":["DiskPressure alert fired","emptyDir writes exceed capacity","postgres container writing unbounded WAL"],
						"due_diligence":{
							"causal_completeness":"Traced to emptyDir sizing",
							"target_accuracy":"postgres-emptydir is primary writer",
							"evidence_sufficiency":"Backed by metrics",
							"alternative_hypotheses":"Considered image layers",
							"scope_completeness":"All deployments checked",
							"proportionality":"Single offender",
							"regression_awareness":"N/A",
							"confidence_calibration":"0.92"
						}
					}`},
					Usage: llm.TokenUsage{PromptTokens: 150, CompletionTokens: 80, TotalTokens: 230},
				},
				func() llm.ChatResponse {
					r := wfToolResp(`{"workflow_id":"set-ephemeral-limit","confidence":0.9,"remediation_target":{"kind":"Deployment","name":"postgres-emptydir","namespace":"demo-disk"}}`)
					r.Usage = llm.TokenUsage{PromptTokens: 200, CompletionTokens: 100, TotalTokens: 300}
					return r
				}(),
			}
			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: invLogger, MaxTurns: 15, PhaseTools: phaseTools,
			})

			_, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())

			rcaEvents := eventsOfType(audit.EventTypeRCAComplete)
			Expect(rcaEvents).To(HaveLen(1), "exactly one aiagent.rca.complete event per investigation")

			rcaEvt := rcaEvents[0]
			Expect(rcaEvt.EventAction).To(Equal(audit.ActionLLMResponse))
			Expect(rcaEvt.EventOutcome).To(Equal(audit.OutcomeSuccess))
			Expect(rcaEvt.CorrelationID).NotTo(BeEmpty())

			rd, ok := rcaEvt.Data["response_data"].(string)
			Expect(ok).To(BeTrue(), "response_data must be a JSON string")
			Expect(rd).To(ContainSubstring("DiskPressure from emptyDir overuse"))
			Expect(rd).To(ContainSubstring("causal_chain"))
			Expect(rd).To(ContainSubstring("due_diligence"))
			Expect(rd).To(ContainSubstring("postgres-emptydir"))

			completeEvents := eventsOfType(audit.EventTypeResponseComplete)
			Expect(completeEvents).To(HaveLen(1), "response.complete should still be emitted after Phase 3")
		})
	})

	Describe("IT-KA-851-AP-002: rca.complete emitted even when human review needed (early exit)", func() {
		It("should emit aiagent.rca.complete before the early-exit response.complete", func() {
			mockClient.responses = []llm.ChatResponse{
				{
					Message: llm.Message{Role: "assistant", Content: `{
						"rca_summary":"Cannot determine root cause",
						"severity":"unknown",
						"confidence":0.3,
						"needs_human_review":true,
						"human_review_reason":"insufficient_evidence"
					}`},
					Usage: llm.TokenUsage{PromptTokens: 100, CompletionTokens: 40, TotalTokens: 140},
				},
			}
			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: invLogger, MaxTurns: 15, PhaseTools: phaseTools,
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.HumanReviewNeeded).To(BeTrue())

			rcaEvents := eventsOfType(audit.EventTypeRCAComplete)
			Expect(rcaEvents).To(HaveLen(1),
				"rca.complete must be emitted even on human-review early exit for forensic completeness")

			completeEvents := eventsOfType(audit.EventTypeResponseComplete)
			Expect(completeEvents).To(HaveLen(1),
				"response.complete should still be emitted on early exit")
		})
	})

	Describe("IT-KA-433-AP-006: Investigation emits response.failed on LLM error", func() {
		It("should include error_message and phase in response.failed event", func() {
			failingClient := &errorLLMClient{err: fmt.Errorf("LLM timeout after 30s")}
			inv := investigator.New(investigator.Config{
				Client: failingClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: invLogger, MaxTurns: 15, PhaseTools: phaseTools,
			})

			_, err := inv.Investigate(context.Background(), signal)
			Expect(err).To(HaveOccurred())

			failEvents := eventsOfType(audit.EventTypeResponseFailed)
			Expect(failEvents).To(HaveLen(1))
			first := failEvents[0]
			Expect(first.Data["error_message"]).To(ContainSubstring("LLM timeout after 30s"))
			Expect(first.Data["phase"]).To(Equal("rca"))
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
				AuditStore: auditStore, Logger: invLogger, MaxTurns: 15, PhaseTools: phaseTools,
			})

			_, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())

			investigatorTypes := map[string]bool{
				audit.EventTypeLLMRequest:        true,
				audit.EventTypeLLMResponse:       true,
				audit.EventTypeLLMToolCall:       true,
				audit.EventTypeValidationAttempt: true,
				audit.EventTypeRCAComplete:       true,
				audit.EventTypeResponseComplete:  true,
				audit.EventTypeResponseFailed:    true,
				audit.EventTypeAlignmentStep:     true,
				audit.EventTypeAlignmentVerdict:  true,
			}

			for _, e := range auditStore.events {
				if !investigatorTypes[e.EventType] {
					continue
				}
				rawID, ok := e.Data["event_id"]
				Expect(ok).To(BeTrue(), "event_id missing on %s event", e.EventType)
				idStr, ok := rawID.(string)
				Expect(ok).To(BeTrue(), "expected type assertion to string for event_id in audit event data to succeed")
				_, parseErr := uuid.Parse(idStr)
				Expect(parseErr).NotTo(HaveOccurred(), "invalid UUID on %s event: %s", e.EventType, idStr)
			}
		})
	})

	Describe("IT-KA-433-AP-008: All investigator events have EventAction and EventOutcome", func() {
		It("should set EventAction and EventOutcome on llm.request and llm.response events", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"test"}`}},
				wfToolResp(`{"workflow_id":"restart","confidence":0.8}`),
			}
			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: invLogger, MaxTurns: 15, PhaseTools: phaseTools,
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

	Describe("IT-KA-433-AP-020: Re-enrichment preserves signal-target labels when RCA target is unreachable", func() {
		It("should preserve signal-target labels when RCA target cannot be resolved", func() {
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
			testMapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
				{Group: "", Version: "v1"},
				{Group: "apps", Version: "v1"},
				{Group: "autoscaling", Version: "v2"},
				{Group: "policy", Version: "v1"},
				{Group: "networking.k8s.io", Version: "v1"},
			})
			testMapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, meta.RESTScopeNamespace)
			testMapper.Add(schema.GroupVersionKind{Group: "autoscaling", Version: "v2", Kind: "HorizontalPodAutoscaler"}, meta.RESTScopeNamespace)
			testMapper.Add(schema.GroupVersionKind{Group: "policy", Version: "v1", Kind: "PodDisruptionBudget"}, meta.RESTScopeNamespace)
			testMapper.Add(schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "NetworkPolicy"}, meta.RESTScopeNamespace)
			testMapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ResourceQuota"}, meta.RESTScopeNamespace)
			ld := enrichment.NewLabelDetector(dynClient, testMapper, invLogger)

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
				invLogger,
			).WithLabelDetector(ld)

			labelMockClient := &mockLLMClient{}
			labelMockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary": "OOM due to worker memory leak",
					"remediation_target": {"kind": "Deployment", "name": "worker", "namespace": "production"}
				}`}},
				wfToolResp(`{
					"workflow_id": "oom-recovery",
					"confidence": 0.9
				}`),
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
				Logger:       invLogger,
				MaxTurns:     15,
				PhaseTools:   phaseTools,
			})

			result, err := inv.Investigate(context.Background(), labelSignal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(result.DetectedLabels).NotTo(BeNil(),
				"DetectedLabels must be populated — original signal-target labels preserved")

			// When the RCA target (worker) can't be resolved, the investigator
			// preserves the signal-target (api-server) labels instead of replacing
			// them with all-failed detections.
			helmVal, hasHelm := result.DetectedLabels["helmManaged"]
			Expect(hasHelm).To(BeTrue(), "helmManaged must be present from signal-target api-server")
			Expect(helmVal).To(BeTrue(),
				"helmManaged must be true — preserved from api-server (managed-by=Helm)")

			// failedDetections should NOT include all categories since we preserved
			// the api-server labels (which successfully detected helmManaged etc.)
			failedRaw, hasFailed := result.DetectedLabels["failedDetections"]
			if hasFailed {
				failedSlice, ok := failedRaw.([]string)
				Expect(ok).To(BeTrue(), "expected type assertion to []string for failedDetections in DetectedLabels to succeed")
				Expect(failedSlice).NotTo(HaveLen(len(enrichment.AllDetectionCategories)),
					"should NOT have all categories failed — signal-target labels were preserved")
			}
		})
	})

	Describe("IT-KA-433-AP-004: Investigation emits validation_attempt per self-correction attempt", func() {
		It("should emit one failure validation_attempt then one success when correction succeeds", func() {
			itInvLog := logr.Discard()
			auditStore := &recordingAuditStore{}
			builder, _ := prompt.NewBuilder()
			rp := parser.NewResultParser()
			k8sClient := &fakeK8sClient{ownerChain: []enrichment.OwnerChainEntry{}}
			dsClient := &fakeDataStorageClient{history: &enrichment.RemediationHistoryResult{}}
			enricher := enrichment.NewEnricher(k8sClient, dsClient, auditStore, itInvLog)
			phaseTools := investigator.DefaultPhaseToolMap()

			validator := parser.NewValidator([]string{"restart", "scale-up"})

			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"pod crashed"}`}},
					wfToolResp(`{"workflow_id":"unknown-workflow","confidence":0.8}`),
					wfToolResp(`{"workflow_id":"restart","confidence":0.7}`),
				},
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: itInvLog,
				MaxTurns: 15, PhaseTools: phaseTools,
				Pipeline: investigator.Pipeline{
					CatalogFetcher:  &staticCatalogFetcher{validator: validator},
					AnomalyDetector: investigator.NewAnomalyDetector(investigator.DefaultAnomalyConfig(), nil),
				},
			})

			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api", Namespace: "default", Severity: "warning", Message: "CrashLoop",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.WorkflowID).To(Equal("restart"))

			validationEvents := filterEvents(auditStore.events, audit.EventTypeValidationAttempt)
			Expect(validationEvents).To(HaveLen(2),
				"one failure attempt + one success attempt")

			failEvent := validationEvents[0]
			Expect(failEvent.Data["is_valid"]).To(BeFalse(),
				"first attempt should be marked invalid")
			Expect(failEvent.Data["attempt"]).To(Equal(1))
			Expect(failEvent.EventOutcome).To(Equal(audit.OutcomeFailure))

			successEvent := validationEvents[1]
			Expect(successEvent.Data["is_valid"]).To(BeTrue(),
				"final attempt should be marked valid after correction")
			Expect(successEvent.EventOutcome).To(Equal(audit.OutcomeSuccess))
		})

		It("should emit isValid=false on final event when validation is exhausted", func() {
			itInvLog := logr.Discard()
			auditStore := &recordingAuditStore{}
			builder, _ := prompt.NewBuilder()
			rp := parser.NewResultParser()
			k8sClient := &fakeK8sClient{ownerChain: []enrichment.OwnerChainEntry{}}
			dsClient := &fakeDataStorageClient{history: &enrichment.RemediationHistoryResult{}}
			enricher := enrichment.NewEnricher(k8sClient, dsClient, auditStore, itInvLog)
			phaseTools := investigator.DefaultPhaseToolMap()

			validator := parser.NewValidator([]string{"restart", "scale-up"})

			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"pod crashed"}`}},
					wfToolResp(`{"workflow_id":"bad-1","confidence":0.8}`),
					wfToolResp(`{"workflow_id":"bad-2","confidence":0.7}`),
					wfToolResp(`{"workflow_id":"bad-3","confidence":0.6}`),
				},
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: itInvLog,
				MaxTurns: 15, PhaseTools: phaseTools,
				Pipeline: investigator.Pipeline{
					CatalogFetcher:  &staticCatalogFetcher{validator: validator},
					AnomalyDetector: investigator.NewAnomalyDetector(investigator.DefaultAnomalyConfig(), nil),
				},
			})

			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api", Namespace: "default", Severity: "warning", Message: "CrashLoop",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.HumanReviewNeeded).To(BeTrue(),
				"exhaustion should trigger human review")

			validationEvents := filterEvents(auditStore.events, audit.EventTypeValidationAttempt)
			Expect(len(validationEvents)).To(BeNumerically(">=", 2),
				"at least correction attempts + final emit")

			lastEvent := validationEvents[len(validationEvents)-1]
			Expect(lastEvent.Data["is_valid"]).To(BeFalse(),
				"final validation event must reflect exhaustion (isValid=false)")
		})
	})
})

var _ = Describe("IT-KA-947: Alignment audit events emitted during investigation — BR-AI-601", func() {
	var (
		auditStore *recordingAuditStore
		builder    *prompt.Builder
		rp         *parser.ResultParser
		enricher   *enrichment.Enricher
		phaseTools katypes.PhaseToolMap
	)

	BeforeEach(func() {
		auditStore = &recordingAuditStore{}
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
		enricher = enrichment.NewEnricher(k8sClient, dsClient, auditStore, logr.Discard())
		phaseTools = investigator.DefaultPhaseToolMap()
	})

	signal := katypes.SignalContext{
		Name:          "api-server-abc",
		Namespace:     "production",
		Severity:      "critical",
		Message:       "OOMKilled",
		ResourceKind:  "Deployment",
		ResourceName:  "api-server",
		RemediationID: "rem-it-947",
	}

	// buildAlignmentWrapper wires up primary + shadow mocks into a full
	// InvestigatorWrapper, eliminating the per-test setup boilerplate.
	buildAlignmentWrapper := func(
		primaryResponses []llm.ChatResponse,
		shadowResponses []llm.ChatResponse,
		mode config.AlignmentMode,
	) (*alignment.InvestigatorWrapper, *shadowMockLLMClient) {
		primaryMock := &mockLLMClient{responses: primaryResponses}
		shadowMock := &shadowMockLLMClient{responses: shadowResponses}

		evaluator := alignment.NewEvaluator(shadowMock, alignment.EvaluatorConfig{
			Timeout: 10 * time.Second, MaxStepTokens: 4000, MaxRetries: 1,
		}, alignprompt.SystemPrompt())

		llmProxy := alignment.NewLLMProxy(primaryMock)
		inv := investigator.New(investigator.Config{
			Client: llmProxy, Builder: builder, ResultParser: rp, Enricher: enricher,
			AuditStore: auditStore, Logger: logr.Discard(), MaxTurns: 15, PhaseTools: phaseTools,
		})

		wrapper, wrapErr := alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
			Inner:          inv,
			Evaluator:      evaluator,
			VerdictTimeout: 10 * time.Second,
			AuditStore:     auditStore,
			Logger:         logr.Discard(),
			Mode:           mode,
		})
		Expect(wrapErr).NotTo(HaveOccurred())
		return wrapper, shadowMock
	}

	canaryOK := llm.ChatResponse{Message: llm.Message{Role: "assistant", Content: `{"suspicious":true,"explanation":"canary flagged correctly"}`}}
	clean := llm.ChatResponse{Message: llm.Message{Role: "assistant", Content: `{"suspicious":false,"explanation":"clean"}`}}
	suspicious := llm.ChatResponse{Message: llm.Message{Role: "assistant", Content: `{"suspicious":true,"explanation":"injection detected in tool result"}`}}

	standardPrimary := []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOMKilled","confidence":0.9}`}},
		wfToolResp(`{"workflow_id":"oom-increase-memory","confidence":0.9}`),
	}

	Describe("IT-KA-947-001: InvestigatorWrapper emits alignment.verdict for a clean investigation", func() {
		It("should emit EventTypeAlignmentVerdict with flagged=0 when shadow detects no issues", func() {
			shadowResponses := []llm.ChatResponse{canaryOK, clean, clean, clean, clean, clean}
			wrapper, shadowMock := buildAlignmentWrapper(standardPrimary, shadowResponses, config.AlignmentModeEnforce)

			result, err := wrapper.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			verdictEvents := filterEvents(auditStore.events, audit.EventTypeAlignmentVerdict)
			Expect(verdictEvents).NotTo(BeEmpty(), "must emit at least one alignment.verdict event")

			v := verdictEvents[0]
			Expect(v.Data).To(HaveKey("result"))
			Expect(v.Data).To(HaveKey("flagged"))
			Expect(v.Data["flagged"]).To(BeEquivalentTo(0), "clean investigation should have flagged=0")

			Expect(shadowMock.totalCalls()).To(BeNumerically(">=", 3),
				"shadow evaluator must have been called for canary + at least 2 steps")
		})
	})

	Describe("IT-KA-947-002: InvestigatorWrapper emits alignment.step events for suspicious findings", func() {
		It("should emit EventTypeAlignmentStep with suspicious=true when shadow flags content", func() {
			shadowResponses := []llm.ChatResponse{canaryOK, suspicious, clean, clean, clean, clean}
			wrapper, _ := buildAlignmentWrapper(standardPrimary, shadowResponses, config.AlignmentModeEnforce)

			result, err := wrapper.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			stepEvents := filterEvents(auditStore.events, audit.EventTypeAlignmentStep)
			Expect(stepEvents).NotTo(BeEmpty(), "must emit at least one alignment.step event")

			verdictEvents := filterEvents(auditStore.events, audit.EventTypeAlignmentVerdict)
			Expect(verdictEvents).NotTo(BeEmpty())
			v := verdictEvents[0]
			Expect(v.Data["flagged"]).To(BeNumerically(">=", 1),
				"at least one step should be flagged as suspicious")

			Expect(result.HumanReviewNeeded).To(BeTrue(),
				"enforce mode + suspicious findings should escalate to human review")
		})
	})

	Describe("IT-KA-947-003: Alignment events carry correct CorrelationID and event_id UUID", func() {
		It("should set CorrelationID to signal name and event_id to a valid UUID", func() {
			primary := []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"test","confidence":0.9}`}},
				wfToolResp(`{"workflow_id":"restart","confidence":0.8}`),
			}
			shadowResponses := []llm.ChatResponse{canaryOK, clean, clean, clean}
			wrapper, _ := buildAlignmentWrapper(primary, shadowResponses, config.AlignmentModeMonitor)

			_, err := wrapper.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())

			alignmentTypes := map[string]bool{
				audit.EventTypeAlignmentStep:    true,
				audit.EventTypeAlignmentVerdict: true,
			}

			found := 0
			for _, e := range auditStore.events {
				if !alignmentTypes[e.EventType] {
					continue
				}
				found++
				Expect(e.CorrelationID).To(Equal(signal.Name),
					"alignment event CorrelationID must match signal name")

				rawID, ok := e.Data["event_id"]
				Expect(ok).To(BeTrue(), "event_id missing on %s event", e.EventType)
				idStr, ok := rawID.(string)
				Expect(ok).To(BeTrue(), "event_id should be a string")
				_, parseErr := uuid.Parse(idStr)
				Expect(parseErr).NotTo(HaveOccurred(),
					"event_id must be a valid UUID on %s event: %s", e.EventType, idStr)
			}

			Expect(found).To(BeNumerically(">=", 1),
				"at least one alignment event must be present")
		})
	})
})

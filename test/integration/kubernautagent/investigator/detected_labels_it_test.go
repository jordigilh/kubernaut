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
	"log/slog"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

var _ = Describe("HAPI-KA Integration Parity — Detected Labels (TP-433-PARITY)", func() {

	var (
		logger     *slog.Logger
		auditStore *recordingAuditStore
		builder    *prompt.Builder
		rp         *parser.ResultParser
		phaseTools katypes.PhaseToolMap
	)

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
		auditStore = &recordingAuditStore{}
		builder, _ = prompt.NewBuilder()
		rp = parser.NewResultParser()
		phaseTools = investigator.DefaultPhaseToolMap()
	})

	Describe("IT-KA-433-DL-001: Enricher populates DetectedLabels via fake K8s with GitOps+HPA fixtures", func() {
		It("should detect gitOpsManaged from argocd managed-by label", func() {
			scheme := runtime.NewScheme()
			_ = appsv1.AddToScheme(scheme)
			_ = autoscalingv2.AddToScheme(scheme)
			_ = corev1.AddToScheme(scheme)
			_ = policyv1.AddToScheme(scheme)
			_ = networkingv1.AddToScheme(scheme)

			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "api-server", Namespace: "production",
					Annotations: map[string]string{"argocd.argoproj.io/managed-by": "production"},
				},
			}
			hpa := &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{Name: "api-server-hpa", Namespace: "production"},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						Kind: "Deployment", Name: "api-server",
					},
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy, hpa)
			ld := enrichment.NewLabelDetector(dynClient)

			k8sClient := &fakeK8sClient{
				ownerChain: []enrichment.OwnerChainEntry{
					{Kind: "Deployment", Name: "api-server", Namespace: "production"},
				},
			}
			dsClient := &fakeDataStorageClient{history: &enrichment.RemediationHistoryResult{}}
			enricher := enrichment.NewEnricher(k8sClient, dsClient, auditStore, logger).WithLabelDetector(ld)

			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOMKilled"}`}},
					{Message: llm.Message{Role: "assistant", Content: `{"workflow_id":"restart","confidence":0.9}`}},
				},
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})

			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server", Namespace: "production", Severity: "critical",
				Message: "OOMKilled", ResourceKind: "Pod", ResourceName: "api-server-xyz",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.DetectedLabels).NotTo(BeNil(), "DetectedLabels must be populated from enrichment")

			gitOps, ok := result.DetectedLabels["gitOpsManaged"]
			Expect(ok).To(BeTrue())
			Expect(gitOps).To(BeTrue(), "argocd annotation should set gitOpsManaged=true")

			hpaEnabled, ok := result.DetectedLabels["hpaEnabled"]
			Expect(ok).To(BeTrue())
			Expect(hpaEnabled).To(BeTrue(), "HPA targeting the Deployment should set hpaEnabled=true")
		})
	})

	Describe("IT-KA-433-DL-002: InvestigationResult carries DetectedLabels through toPromptEnrichment to prompt", func() {
		It("should include detected label information in the LLM system prompt", func() {
			scheme := runtime.NewScheme()
			_ = appsv1.AddToScheme(scheme)
			_ = autoscalingv2.AddToScheme(scheme)
			_ = corev1.AddToScheme(scheme)
			_ = policyv1.AddToScheme(scheme)
			_ = networkingv1.AddToScheme(scheme)

			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "api-server", Namespace: "production",
					Labels: map[string]string{"app.kubernetes.io/managed-by": "Helm"},
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy)
			ld := enrichment.NewLabelDetector(dynClient)

			k8sClient := &fakeK8sClient{
				ownerChain: []enrichment.OwnerChainEntry{
					{Kind: "Deployment", Name: "api-server", Namespace: "production"},
				},
			}
			dsClient := &fakeDataStorageClient{history: &enrichment.RemediationHistoryResult{}}
			enricher := enrichment.NewEnricher(k8sClient, dsClient, auditStore, logger).WithLabelDetector(ld)

			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"memory leak"}`}},
					{Message: llm.Message{Role: "assistant", Content: `{"workflow_id":"restart","confidence":0.8}`}},
				},
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server", Namespace: "production", Severity: "warning",
				Message: "CrashLoop", ResourceKind: "Pod", ResourceName: "api-server-xyz",
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(len(mockClient.calls)).To(BeNumerically(">=", 1))
			firstCall := mockClient.calls[0]
			systemPrompt := ""
			for _, msg := range firstCall.Messages {
				if msg.Role == "system" {
					systemPrompt = msg.Content
					break
				}
			}
			Expect(systemPrompt).NotTo(BeEmpty())
			Expect(systemPrompt).To(ContainSubstring("helmManaged"),
				"detected labels should appear in the investigation prompt via toPromptEnrichment")
		})
	})

	Describe("IT-KA-433-DL-003: Handler populates detected_labels in ogen IncidentResponse", func() {
		It("should include detected_labels in the aiagent.response.complete audit event", func() {
			scheme := runtime.NewScheme()
			_ = appsv1.AddToScheme(scheme)
			_ = autoscalingv2.AddToScheme(scheme)
			_ = corev1.AddToScheme(scheme)
			_ = policyv1.AddToScheme(scheme)
			_ = networkingv1.AddToScheme(scheme)

			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "api-server", Namespace: "production",
					Annotations: map[string]string{"argocd.argoproj.io/managed-by": "production"},
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy)
			ld := enrichment.NewLabelDetector(dynClient)

			k8sClient := &fakeK8sClient{
				ownerChain: []enrichment.OwnerChainEntry{
					{Kind: "Deployment", Name: "api-server", Namespace: "production"},
				},
			}
			dsClient := &fakeDataStorageClient{history: &enrichment.RemediationHistoryResult{}}
			enricher := enrichment.NewEnricher(k8sClient, dsClient, auditStore, logger).WithLabelDetector(ld)

			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"pod crashed"}`}},
					{Message: llm.Message{Role: "assistant", Content: `{"workflow_id":"restart","confidence":0.9}`}},
				},
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})

			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server", Namespace: "production", Severity: "warning",
				Message: "OOMKilled", ResourceKind: "Pod", ResourceName: "api-server-xyz",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.DetectedLabels).NotTo(BeNil())
			Expect(result.DetectedLabels["gitOpsManaged"]).To(BeTrue())

			completeEvents := filterEvents(auditStore.events, audit.EventTypeResponseComplete)
			Expect(completeEvents).To(HaveLen(1))
			responseData, ok := completeEvents[0].Data["response_data"].(string)
			Expect(ok).To(BeTrue(), "response_data must be serialized as JSON string")
			Expect(responseData).NotTo(BeEmpty())
		})
	})
})

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

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/tools/custom"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
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

// BR-AI-056 / Issue #1052: KA tools must forward DetectedLabels from enrichment
// to DataStorage catalog queries so the scoring engine activates GitOps-aware
// workflow ranking.

var _ = Describe("IT-KA-1052: DetectedLabels wiring from investigator to DS tool params", func() {

	var (
		invLogger  logr.Logger
		auditStore *recordingAuditStore
		builder    *prompt.Builder
		rp         *parser.ResultParser
		phaseTools katypes.PhaseToolMap
	)

	BeforeEach(func() {
		invLogger = logr.Discard()
		auditStore = &recordingAuditStore{}
		builder, _ = prompt.NewBuilder()
		rp = parser.NewResultParser()
		phaseTools = investigator.DefaultPhaseToolMap()
	})

	Describe("IT-KA-1052-001: Investigator wires enrichment DetectedLabels to DS query params", func() {
		It("should forward DetectedLabels JSON to list_available_actions DS params", func() {
			capturingDS := &paramCapturingDS{}
			reg := registry.New()
			for _, t := range custom.NewAllTools(capturingDS) {
				reg.Register(t)
			}

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
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "production"},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy, ns)
			ld := enrichment.NewLabelDetector(dynClient, newItTestMapper(), invLogger)

			k8sClient := &fakeK8sClient{
				ownerChain: []enrichment.OwnerChainEntry{
					{Kind: "Deployment", Name: "api-server", Namespace: "production"},
				},
			}
			dsClient := &fakeDataStorageClient{history: &enrichment.RemediationHistoryResult{}}
			enricher := enrichment.NewEnricher(k8sClient, dsClient, auditStore, invLogger).
				WithLabelDetector(ld)

			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOMKilled in production"}`}},
					{
						Message:   llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{{ID: "tc_1", Name: "list_available_actions", Arguments: `{}`}},
					},
					wfToolResp(`{"workflow_id":"restart","confidence":0.9}`),
				},
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: invLogger,
				MaxTurns: 15, PhaseTools: phaseTools, Registry: reg,
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name:         "api-server",
				Namespace:    "production",
				Severity:     "critical",
				Message:      "OOMKilled",
				ResourceKind: "Deployment",
				ResourceName: "api-server",
				Environment:  "production",
				Priority:     "P0",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(capturingDS.actionsCalled).To(BeTrue(),
				"list_available_actions must have been called during workflow selection")

			dl, ok := capturingDS.listActionsParams.DetectedLabels.Get()
			Expect(ok).To(BeTrue(),
				"DetectedLabels must be set on DS params when enrichment detects labels")
			Expect(dl).To(ContainSubstring(`"gitOpsManaged":true`),
				"DetectedLabels JSON must include gitOpsManaged:true from ArgoCD annotation")
			Expect(dl).To(ContainSubstring(`"gitOpsTool":"argocd"`),
				"DetectedLabels JSON must include gitOpsTool:argocd")
		})
	})

	Describe("IT-KA-1052-002: Re-enrichment all-labels-failed preserves original labels on DS params", func() {
		It("should use original enrichment labels when re-enrichment labels all fail", func() {
			capturingDS := &paramCapturingDS{}
			reg := registry.New()
			for _, t := range custom.NewAllTools(capturingDS) {
				reg.Register(t)
			}

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
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "production"},
			}
			// RCA target ("worker-pod") does NOT exist in the fake client,
			// so re-enrichment label detection will fail for all categories
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy, ns)
			ld := enrichment.NewLabelDetector(dynClient, newItTestMapper(), invLogger)

			// Return different owner chains per resource name:
			// - "api-server" -> known chain (initial enrichment succeeds)
			// - "worker-pod" -> empty chain (re-enrichment, labels will all fail)
			k8sClient := &resourceAwareK8sClient{
				chains: map[string][]enrichment.OwnerChainEntry{
					"api-server": {{Kind: "Deployment", Name: "api-server", Namespace: "production"}},
					"worker-pod": {{Kind: "Pod", Name: "worker-pod", Namespace: "production"}},
				},
			}
			dsClient := &fakeDataStorageClient{history: &enrichment.RemediationHistoryResult{}}
			enricher := enrichment.NewEnricher(k8sClient, dsClient, auditStore, invLogger).
				WithLabelDetector(ld)

			// RCA returns a different remediation target to trigger re-enrichment
			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"Root cause is worker-pod OOM","remediation_target":{"kind":"Pod","name":"worker-pod","namespace":"production"}}`}},
					{
						Message:   llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{{ID: "tc_1", Name: "list_available_actions", Arguments: `{}`}},
					},
					wfToolResp(`{"workflow_id":"restart","confidence":0.9}`),
				},
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: invLogger,
				MaxTurns: 15, PhaseTools: phaseTools, Registry: reg,
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name:         "api-server",
				Namespace:    "production",
				Severity:     "critical",
				Message:      "OOMKilled",
				ResourceKind: "Deployment",
				ResourceName: "api-server",
				Environment:  "production",
				Priority:     "P0",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(capturingDS.actionsCalled).To(BeTrue(),
				"list_available_actions must have been called during workflow selection")

			dl, ok := capturingDS.listActionsParams.DetectedLabels.Get()
			Expect(ok).To(BeTrue(),
				"DetectedLabels must be set from ORIGINAL enrichment when re-enrichment labels all fail")
			Expect(dl).To(ContainSubstring(`"gitOpsManaged":true`),
				"Original signal-target ArgoCD labels must be preserved")
		})
	})
})

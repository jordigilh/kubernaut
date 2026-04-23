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

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/custom"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var _ = Describe("IT-KA-795: Owner chain promotion for workflow discovery", func() {

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

	Describe("IT-KA-795-001: Pod signal with owner chain promotes Deployment to DS component on RCA parse failure", func() {
		It("should send component=deployment to list_available_actions, not component=pod", func() {
			capturingDS := &paramCapturingDS{}
			reg := registry.New()
			for _, t := range custom.NewAllTools(capturingDS) {
				reg.Register(t)
			}

			// Phase 1 (RCA): plain text triggers parse failure -> fallback to RCASummary only
			// Phase 3 (Workflow): LLM calls list_available_actions, then submits result
			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: "The pod crashed due to memory pressure on the api deployment"}},
					{
						Message:   llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{{ID: "tc_1", Name: "list_available_actions", Arguments: `{}`}},
					},
					wfToolResp(`{"workflow_id":"restart","confidence":0.9}`),
				},
			}

			k8sClient := &fakeK8sClient{ownerChain: []enrichment.OwnerChainEntry{
				{Kind: "ReplicaSet", Name: "api-rs-abc", Namespace: "production"},
				{Kind: "Deployment", Name: "api", Namespace: "production"},
			}}
			dsClient := &fakeDataStorageClient{history: &enrichment.RemediationHistoryResult{}}
			enricher := enrichment.NewEnricher(k8sClient, dsClient, auditStore, logger)

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools, Registry: reg,
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name:         "OOMKilled",
				Namespace:    "production",
				Severity:     "critical",
				Message:      "Pod api-pod-xyz OOMKilled",
				ResourceKind: "Pod",
				ResourceName: "api-pod-xyz",
				Environment:  "production",
				Priority:     "P0",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(capturingDS.actionsCalled).To(BeTrue(),
				"list_available_actions must have been called during workflow selection")

			Expect(capturingDS.listActionsParams.Component).To(Equal("deployment"),
				"IT-KA-795-001: DS Component should be owner chain root 'deployment', not signal 'pod'")
		})
	})

	Describe("IT-KA-795-002: Cluster-scoped owner chain root has namespace cleared", func() {
		It("should send component=node with empty namespace to list_available_actions", func() {
			capturingDS := &paramCapturingDS{}
			reg := registry.New()
			for _, t := range custom.NewAllTools(capturingDS) {
				reg.Register(t)
			}

			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: "Node worker-1 is under disk pressure"}},
					{
						Message:   llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{{ID: "tc_1", Name: "list_available_actions", Arguments: `{}`}},
					},
					wfToolResp(`{"workflow_id":"drain-node","confidence":0.85}`),
				},
			}

			k8sClient := &fakeK8sClient{ownerChain: []enrichment.OwnerChainEntry{
				{Kind: "DaemonSet", Name: "kube-proxy", Namespace: "kube-system"},
				{Kind: "Node", Name: "worker-1", Namespace: ""},
			}}
			dsClient := &fakeDataStorageClient{history: &enrichment.RemediationHistoryResult{}}
			enricher := enrichment.NewEnricher(k8sClient, dsClient, auditStore, logger)

			// ScopeResolver so normalizeNamespace clears namespace for Node
			mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
				{Group: "", Version: "v1"},
			})
			mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Node"}, meta.RESTScopeRoot)
			mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}, meta.RESTScopeNamespace)

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools, Registry: reg,
				ScopeResolver: investigator.NewMapperScopeResolver(mapper),
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name:         "DiskPressure",
				Namespace:    "kube-system",
				Severity:     "high",
				Message:      "Pod kube-proxy-xyz disk pressure",
				ResourceKind: "Pod",
				ResourceName: "kube-proxy-xyz",
				Environment:  "production",
				Priority:     "P1",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(capturingDS.actionsCalled).To(BeTrue(),
				"list_available_actions must have been called during workflow selection")

			Expect(capturingDS.listActionsParams.Component).To(Equal("node"),
				"IT-KA-795-002: DS Component should be cluster-scoped owner chain root 'node'")
		})
	})

	Describe("IT-KA-795-003: Owner chain promotion must NOT fire when re-enrichment ran", func() {
		It("should use re-enriched chain root, not original signal chain root", func() {
			capturingDS := &paramCapturingDS{}
			reg := registry.New()
			for _, t := range custom.NewAllTools(capturingDS) {
				reg.Register(t)
			}

			// RCA identifies a different pod (same Kind, different Name) -> triggers re-enrichment
			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{
						"rca_summary": "The crash is caused by different-pod",
						"remediation_target": {"kind": "Pod", "name": "different-pod", "namespace": "production"}
					}`}},
					{
						Message:   llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{{ID: "tc_1", Name: "list_available_actions", Arguments: `{}`}},
					},
					wfToolResp(`{"workflow_id":"scale","confidence":0.8}`),
				},
			}

			// Original pod -> Deployment chain; different pod -> StatefulSet chain
			k8sClient := &resourceAwareK8sClient{chains: map[string][]enrichment.OwnerChainEntry{
				"api-pod-xyz": {
					{Kind: "ReplicaSet", Name: "api-rs-abc", Namespace: "production"},
					{Kind: "Deployment", Name: "api", Namespace: "production"},
				},
				"different-pod": {
					{Kind: "ReplicaSet", Name: "sts-rs-abc", Namespace: "production"},
					{Kind: "StatefulSet", Name: "cache", Namespace: "production"},
				},
			}}
			dsClient := &fakeDataStorageClient{history: &enrichment.RemediationHistoryResult{}}
			enricher := enrichment.NewEnricher(k8sClient, dsClient, auditStore, logger)

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools, Registry: reg,
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name:         "OOMKilled",
				Namespace:    "production",
				Severity:     "critical",
				Message:      "Pod api-pod-xyz OOMKilled",
				ResourceKind: "Pod",
				ResourceName: "api-pod-xyz",
				Environment:  "production",
				Priority:     "P0",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(capturingDS.actionsCalled).To(BeTrue(),
				"list_available_actions must have been called during workflow selection")

			// Re-enrichment ran because RCA target (Pod/different-pod) != signal (Pod/api-pod-xyz).
			// workflowSignal.ResourceKind should be "Pod" (the RCA target kind), NOT
			// "Deployment" from the original signal's owner chain (which promotion would set).
			Expect(capturingDS.listActionsParams.Component).To(Equal("pod"),
				"IT-KA-795-003: DS Component should be RCA target 'pod' (re-enrichment ran), not original chain root 'deployment'")
		})
	})
})

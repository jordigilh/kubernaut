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
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
	toolregistry "github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

type it2609DiscoveryTool struct {
	name   string
	result string
	err    error
	calls  int
}

func (t *it2609DiscoveryTool) Name() string        { return t.name }
func (t *it2609DiscoveryTool) Description() string { return "target-cluster GVK discovery fixture" }
func (t *it2609DiscoveryTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (t *it2609DiscoveryTool) Execute(context.Context, json.RawMessage) (string, error) {
	t.calls++
	return t.result, t.err
}

type it2609OverlayResolver struct {
	tool tools.Tool
}

func (r *it2609OverlayResolver) Overlay(context.Context, string) (map[string]tools.Tool, error) {
	return map[string]tools.Tool{r.tool.Name(): r.tool}, nil
}

func it2609Signal(clusterID string) katypes.SignalContext {
	return katypes.SignalContext{
		Name:          "CrashLoopBackOff",
		Namespace:     "production",
		Severity:      "critical",
		Environment:   "Production",
		Priority:      "P1",
		ResourceKind:  "Pod",
		ResourceName:  "api-server-pod",
		ClusterID:     clusterID,
		RemediationID: "it-ka-2609",
		Message:       "pod is repeatedly restarting",
	}
}

func it2609RCA(target string) string {
	return fmt.Sprintf(`{"root_cause_analysis":{"summary":"target requires remediation","remediation_target":%s},"confidence":0.9,"actionable":true,"investigation_outcome":"actionable"}`, target)
}

func it2609Target(apiVersion string) string {
	return fmt.Sprintf(`{"kind":"Deployment","name":"api-server","namespace":"production","api_version":%q}`, apiVersion)
}

func newIT2609Enricher(logger logr.Logger, store audit.AuditStore) *enrichment.Enricher {
	return enrichment.NewEnricher(&k8sFixtureClient{ownerChain: nil, err: nil}, suiteDSAdapter, store, logger)
}

func newIT2609Investigator(
	client llm.Client,
	builder *prompt.Builder,
	resultParser *parser.ResultParser,
	enricher *enrichment.Enricher,
	store audit.AuditStore,
	logger logr.Logger,
	registry *toolregistry.Registry,
	fleetResolver investigator.FleetOverlayResolver,
	workflowIDs []string,
) *investigator.Investigator {
	validator := parser.NewValidator(workflowIDs)
	return investigator.New(investigator.Config{
		Client:               client,
		Builder:              builder,
		ResultParser:         resultParser,
		Enricher:             enricher,
		AuditStore:           store,
		Logger:               logger,
		MaxTurns:             15,
		PhaseTools:           investigator.DefaultPhaseToolMap(),
		Registry:             registry,
		FleetOverlayResolver: fleetResolver,
		ScopeResolver:        investigator.NewMapperScopeResolver(newITAmbiguousMapper()),
		Pipeline:             investigator.Pipeline{CatalogFetcher: &staticCatalogFetcher{validator: validator}},
	})
}

var _ = Describe("Target-scoped RCA GVK production wiring — #2609", Label("integration", "kubernautagent", "target-scoped-gvk"), func() {
	var (
		logger       logr.Logger
		store        *capturingAuditStore
		builder      *prompt.Builder
		resultParser *parser.ResultParser
	)

	BeforeEach(func() {
		logger = logr.Discard()
		store = newCapturingAuditStore(suiteAuditStore)
		builder, _ = prompt.NewBuilder()
		resultParser = parser.NewResultParser()
	})

	It("IT-KA-2609-001: local registry exposes discovery and independently verifies the exact GVK", func() {
		mock := &mockLLMClient{responses: []llm.ChatResponse{
			{Message: llm.Message{Role: "assistant", Content: it2609RCA(it2609Target("apps/v1"))}},
			wfToolResp(`{"workflow_id":"scale-up","confidence":0.9}`),
		}}
		inv := newIT2609Investigator(mock, builder, resultParser, newIT2609Enricher(logger, store), store, logger,
			newITTargetScopedDiscoveryRegistry(), nil, []string{"scale-up"})

		result, err := inv.Investigate(context.Background(), it2609Signal(""))
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RemediationTarget.APIVersion).To(Equal("apps/v1"))
		Expect(result.RemediationTarget.APIVersionValidated).To(BeTrue())
		Expect(result.WorkflowID).To(Equal("scale-up"))
	})

	It("IT-KA-2609-002: fleet prescoping routes discovery to the requested spoke, never the local registry", func() {
		remote := &it2609DiscoveryTool{
			name:   "api_resources_list",
			result: `{"resources":[{"apiVersion":"remote.example/v1","kind":"Deployment"}]}`,
		}
		local := &it2609DiscoveryTool{
			name:   "api_resources_list",
			result: `{"resources":[{"apiVersion":"hub.example/v1","kind":"Deployment"}]}`,
		}
		localRegistry := toolregistry.New()
		localRegistry.Register(local)
		mock := &mockLLMClient{responses: []llm.ChatResponse{
			{Message: llm.Message{Role: "assistant", Content: it2609RCA(it2609Target("remote.example/v1"))}},
			wfToolResp(`{"workflow_id":"scale-up","confidence":0.9}`),
		}}
		inv := newIT2609Investigator(mock, builder, resultParser, newIT2609Enricher(logger, store), store, logger,
			localRegistry, &it2609OverlayResolver{tool: remote}, []string{"scale-up"})

		result, err := inv.Investigate(context.Background(), it2609Signal("spoke-it-2609"))
		Expect(err).NotTo(HaveOccurred())
		Expect(remote.calls).To(BeNumerically(">", 0))
		Expect(local.calls).To(Equal(0), "fleet target must not fall back to hub discovery")
		Expect(result.RemediationTarget.APIVersion).To(Equal("remote.example/v1"))
		Expect(result.RemediationTarget.APIVersionValidated).To(BeTrue())
	})

	It("IT-KA-2609-003: autonomous selection preserves the verified target parameters", func() {
		mock := &mockLLMClient{responses: []llm.ChatResponse{
			{Message: llm.Message{Role: "assistant", Content: it2609RCA(it2609Target("apps/v1"))}},
			wfToolResp(`{"workflow_id":"scale-up","confidence":0.9}`),
		}}
		inv := newIT2609Investigator(mock, builder, resultParser, newIT2609Enricher(logger, store), store, logger,
			newITTargetScopedDiscoveryRegistry(), nil, []string{"scale-up"})

		result, err := inv.Investigate(context.Background(), it2609Signal(""))
		Expect(err).NotTo(HaveOccurred())
		Expect(result.WorkflowID).To(Equal("scale-up"))
		Expect(result.Parameters).To(HaveKeyWithValue("TARGET_RESOURCE_API_VERSION", "apps/v1"))
		Expect(result.Parameters).To(HaveKeyWithValue("TARGET_RESOURCE_KIND", "Deployment"))
		Expect(result.Parameters).To(HaveKeyWithValue("TARGET_RESOURCE_NAME", "api-server"))
	})

	It("IT-KA-2609-004: interactive discovery revalidates the RCA target before selection", func() {
		mock := &mockLLMClient{responses: []llm.ChatResponse{
			wfToolResp(`{"workflow_id":"scale-up","confidence":0.9}`),
		}}
		inv := newIT2609Investigator(mock, builder, resultParser, newIT2609Enricher(logger, store), store, logger,
			newITTargetScopedDiscoveryRegistry(), nil, []string{"scale-up"})
		rcaResult := &katypes.InvestigationResult{
			RCASummary:        "Deployment is the owning remediation target",
			RemediationTarget: katypes.RemediationTarget{Kind: "Deployment", Name: "api-server", Namespace: "production", APIVersion: "apps/v1"},
		}

		result, err := inv.RunWorkflowDiscoveryFromRCA(context.Background(), it2609Signal(""), rcaResult, nil, "it-ka-2609-004")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.WorkflowID).To(Equal("scale-up"))
		Expect(result.RemediationTarget.APIVersionValidated).To(BeTrue())
	})

	It("IT-KA-2609-005: discovery failure fails closed without an executable workflow", func() {
		failed := toolregistry.New()
		failed.Register(&it2609DiscoveryTool{name: "api_resources_list", err: fmt.Errorf("target-cluster discovery unavailable")})
		mock := &mockLLMClient{responses: []llm.ChatResponse{
			{Message: llm.Message{Role: "assistant", Content: it2609RCA(it2609Target("apps/v1"))}},
		}}
		inv := newIT2609Investigator(mock, builder, resultParser, newIT2609Enricher(logger, store), store, logger,
			failed, nil, []string{"scale-up"})

		result, err := inv.Investigate(context.Background(), it2609Signal(""))
		Expect(err).NotTo(HaveOccurred())
		Expect(result.HumanReviewNeeded).To(BeTrue())
		Expect(result.WorkflowID).To(BeEmpty())
		Expect(result.RemediationTarget.APIVersionValidated).To(BeFalse())
	})
})

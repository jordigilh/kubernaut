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
	"sync"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"

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

// newTestInvestigator wraps investigator.New, defaulting
// Pipeline.CatalogFetcher to a stub validator (stubCatalogFetcher, defined
// in wiring_test.go) accepting every fixture workflow_id referenced across
// this package's characterization tests, unless cfg already sets one
// explicitly. Shared by apiversion_gate_test.go and
// workflow_discovery_rca_characterization_test.go. #1677 hardening
// (DD-WORKFLOW-019): runWorkflowSelection now fails closed
// (HumanReviewNeeded=true, HumanReviewReason="catalog_unavailable") when a
// workflow was selected but no CatalogFetcher is configured, instead of
// silently skipping validation -- these tests' scenarios (apiVersion gate,
// RCA re-enrichment) are orthogonal to catalog validation, so they default
// to a permissive stub that accepts every fixture ID used, keeping the
// behavior actually under test isolated from this unrelated safety gate.
func newTestInvestigator(cfg investigator.Config) *investigator.Investigator {
	if cfg.Registry == nil {
		cfg.Registry = newGateDiscoveryRegistry()
	}
	if cfg.Pipeline.CatalogFetcher == nil {
		ids := []string{
			"restart-sub", "scale-up", "generic-fix", "restart-pod", // apiversion_gate_test.go
			"restart-cache", "noop", // workflow_discovery_rca_characterization_test.go
			"rotate-node-logs", "increase-memory-limit", // samekind_confidence_2119_test.go, gate_undeclared_tool_2121_test.go
		}
		validator := parser.NewValidator(ids)
		// DD-KA-2496: the characterization catalog must supply readable
		// authoritative metadata, just as the production catalog does.
		for _, id := range ids {
			validator.SetWorkflowMeta(id, parser.WorkflowMeta{WorkflowName: id})
		}
		cfg.Pipeline.CatalogFetcher = &stubCatalogFetcher{
			validator:             validator,
			discoveredWorkflowIDs: ids,
		}
	}
	return investigator.New(cfg)
}

type gateDiscoveryTool struct {
	name string
}

const gateAPIResourcesListToolName = "api_resources_list"

func (t *gateDiscoveryTool) Name() string        { return t.name }
func (t *gateDiscoveryTool) Description() string { return "target-cluster discovery fixture" }
func (t *gateDiscoveryTool) Parameters() json.RawMessage {
	if t.name == gateAPIResourcesListToolName {
		return json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string"}},"required":["kind"]}`)
	}
	return json.RawMessage(`{"type":"object","properties":{"apiVersion":{"type":"string"},"kind":{"type":"string"},"namespace":{"type":"string"}},"required":["apiVersion","kind"]}`)
}

func (t *gateDiscoveryTool) Execute(_ context.Context, args json.RawMessage) (string, error) {
	if t.name == gateAPIResourcesListToolName {
		return `{"resources":[` +
			`{"apiVersion":"apps/v1","kind":"Deployment","name":"deployments","namespaced":true},` +
			`{"apiVersion":"v1","kind":"Pod","name":"pods","namespaced":true},` +
			`{"apiVersion":"v1","kind":"Node","name":"nodes","namespaced":false},` +
			`{"apiVersion":"operators.coreos.com/v1alpha1","kind":"Subscription","name":"subscriptions","namespaced":true},` +
			`{"apiVersion":"messaging.knative.dev/v1","kind":"Subscription","name":"subscriptions","namespaced":true}` +
			`]}`, nil
	}
	var input struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", err
	}
	switch input.APIVersion + "/" + input.Kind {
	case "apps/v1/Deployment", "v1/Pod", "v1/Node", "operators.coreos.com/v1alpha1/Subscription", "messaging.knative.dev/v1/Subscription":
		return fmt.Sprintf(`{"apiVersion":%q,"kind":%q,"items":[]}`, input.APIVersion, input.Kind+"List"), nil
	default:
		return "", fmt.Errorf("exact GVK is not served: %s/%s", input.APIVersion, input.Kind)
	}
}

func newGateDiscoveryRegistry() *toolregistry.Registry {
	registry := toolregistry.New()
	registry.Register(&gateDiscoveryTool{name: gateAPIResourcesListToolName})
	registry.Register(&gateDiscoveryTool{name: "resources_list"})
	return registry
}

// gateMockLLMClient records calls and returns pre-configured responses.
type gateMockLLMClient struct {
	mu        sync.Mutex
	calls     []llm.ChatRequest
	responses []llm.ChatResponse
	errors    []error
	callIdx   int
}

func (m *gateMockLLMClient) Close() error { return nil }

func (m *gateMockLLMClient) StreamChat(_ context.Context, req llm.ChatRequest, _ func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	return m.Chat(context.Background(), req)
}

func (m *gateMockLLMClient) Chat(_ context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, req)
	idx := m.callIdx
	if idx < len(m.errors) && m.errors[idx] != nil {
		err := m.errors[idx]
		m.callIdx++
		return llm.ChatResponse{}, err
	}
	if idx < len(m.responses) {
		resp := m.responses[idx]
		m.callIdx++
		return resp, nil
	}
	return llm.ChatResponse{
		Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"fallback","confidence":0.1}`},
	}, nil
}

// gateRecordingAuditStore captures audit events for gate assertions.
type gateRecordingAuditStore struct {
	mu      sync.Mutex
	events  []*audit.AuditEvent
	ctxErrs []error
}

func (s *gateRecordingAuditStore) StoreAudit(ctx context.Context, event *audit.AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// #2121: snapshot event.Data (and EventOutcome) at call time, matching
	// real production audit stores (internal/kubernautagent/audit/ds_store.go's
	// DSAuditStore/BufferedDSAuditStore), which serialize the event into an
	// outbound request synchronously here and never see later mutations.
	// Storing the raw *audit.AuditEvent pointer previously masked a
	// pre-existing bug (#2124) where callers mutated event.Data AFTER this
	// call returned -- a real store would have missed that mutation, but a
	// live pointer read by the test after Investigate() returned would not.
	snapshot := *event
	dataCopy := make(map[string]interface{}, len(event.Data))
	for k, v := range event.Data {
		dataCopy[k] = v
	}
	snapshot.Data = dataCopy
	s.events = append(s.events, &snapshot)
	s.ctxErrs = append(s.ctxErrs, ctx.Err())
	return nil
}

func (s *gateRecordingAuditStore) eventsByAction(action string) []*audit.AuditEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*audit.AuditEvent
	for _, e := range s.events {
		if e.EventAction == action {
			out = append(out, e)
		}
	}
	return out
}

func (s *gateRecordingAuditStore) contextErrorsByAction(action string) []error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []error
	for i, event := range s.events {
		if event.EventAction == action {
			out = append(out, s.ctxErrs[i])
		}
	}
	return out
}

// gateK8sClient is a minimal fake for enrichment.
type gateK8sClient struct{}

func (f *gateK8sClient) GetOwnerChain(_ context.Context, _, _, _, _ string) ([]enrichment.OwnerChainEntry, error) {
	return nil, nil
}
func (f *gateK8sClient) GetSpecHash(_ context.Context, _, _, _, _ string) (string, error) {
	return "", nil
}

// gateDSClient is a minimal fake for enrichment.
type gateDSClient struct{}

func (f *gateDSClient) GetRemediationHistory(_ context.Context, _, _, _, _, _ string) (*enrichment.RemediationHistoryResult, error) {
	return &enrichment.RemediationHistoryResult{}, nil
}

func gateWfToolResp(jsonContent string) llm.ChatResponse {
	return llm.ChatResponse{
		Message: llm.Message{Role: "assistant", Content: ""},
		ToolCalls: []llm.ToolCall{
			{ID: "tc_wf", Name: "submit_result_with_workflow", Arguments: jsonContent},
		},
	}
}

func newAmbiguousSubscriptionMapper() *meta.DefaultRESTMapper {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		{Group: "operators.coreos.com", Version: "v1alpha1"},
		{Group: "messaging.knative.dev", Version: "v1"},
		{Group: "apps", Version: "v1"},
	})
	mapper.Add(schema.GroupVersionKind{Group: "operators.coreos.com", Version: "v1alpha1", Kind: "Subscription"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "messaging.knative.dev", Version: "v1", Kind: "Subscription"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, meta.RESTScopeNamespace)
	return mapper
}

// failingDiscoveryResolver models an unavailable Kubernetes discovery service.
type failingDiscoveryResolver struct{}

func (failingDiscoveryResolver) IsClusterScoped(string) (bool, error) {
	return false, fmt.Errorf("discovery unavailable")
}

func (failingDiscoveryResolver) IsAmbiguousKind(string) (bool, []schema.GroupVersionResource, error) {
	return false, nil, fmt.Errorf("discovery unavailable")
}

// recordingDiscoveryTool models the target cluster's external MCP endpoint.
type recordingDiscoveryTool struct{ calls int }

func (*recordingDiscoveryTool) Name() string        { return "api_resources_list" }
func (*recordingDiscoveryTool) Description() string { return "target-cluster type discovery" }
func (*recordingDiscoveryTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (t *recordingDiscoveryTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	t.calls++
	return `{"resources":[{"apiVersion":"remote.example/v1","kind":"Subscription","name":"subscriptions","namespaced":true}]}`, nil
}

type cancellingDiscoveryTool struct {
	cancel context.CancelFunc
}

func (*cancellingDiscoveryTool) Name() string { return "api_resources_list" }
func (*cancellingDiscoveryTool) Description() string {
	return "cancelling target-cluster discovery fixture"
}
func (*cancellingDiscoveryTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (t *cancellingDiscoveryTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	t.cancel()
	return `{"resources":[{"apiVersion":"apps/v1","kind":"Deployment","name":"deployments","namespaced":true}]}`, nil
}

var _ = Describe("TP-1044 / TP-1051: apiVersionValidationGate", func() {

	var (
		logger     logr.Logger
		auditStore *gateRecordingAuditStore
		mockClient *gateMockLLMClient
		builder    *prompt.Builder
		rp         *parser.ResultParser
		enricher   *enrichment.Enricher
		phaseTools katypes.PhaseToolMap
	)

	BeforeEach(func() {
		logger = logr.Discard()
		auditStore = &gateRecordingAuditStore{}
		mockClient = &gateMockLLMClient{}
		builder, _ = prompt.NewBuilder()
		rp = parser.NewResultParser()
		enricher = enrichment.NewEnricher(&gateK8sClient{}, &gateDSClient{}, auditStore, logger)
		phaseTools = investigator.DefaultPhaseToolMap()
	})

	signal := katypes.SignalContext{
		ResourceKind: "Pod",
		ResourceName: "etcd-operator-xyz",
		Name:         "etcd-operator-xyz",
		Namespace:    "demo-operator",
		Severity:     "critical",
		Message:      "RBAC denial on wrong API group",
	}

	It("UT-KA-2588-001: rejects an unregistered GVK supplied after ambiguity retry [BR-KA-212]", func() {
		mockClient.responses = []llm.ChatResponse{
			{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"Subscription needs repair","confidence":0.85,"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator"}}`}},
			{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"Subscription needs repair","confidence":0.85,"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator","api_version":"unregistered.example/v1"}}`}},
			gateWfToolResp(`{"workflow_id":"restart-sub","confidence":0.9}`),
		}
		inv := newTestInvestigator(investigator.Config{
			Client: mockClient, Builder: builder, ResultParser: rp,
			Enricher: enricher, AuditStore: auditStore, Logger: logger,
			MaxTurns: 15, PhaseTools: phaseTools,
			ScopeResolver: investigator.NewMapperScopeResolver(newAmbiguousSubscriptionMapper()),
		})

		result, err := inv.Investigate(context.Background(), signal)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.HumanReviewNeeded).To(BeTrue(), "an unregistered GVK must not be accepted")
		Expect(result.WorkflowID).To(BeEmpty(), "no workflow may be selected for an unverified GVK")
	})

	It("UT-KA-2588-002: blocks workflow selection when GVK discovery fails [BR-KA-212]", func() {
		mockClient.responses = []llm.ChatResponse{
			{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"Subscription needs repair","confidence":0.85,"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator","api_version":"operators.coreos.com/v1alpha1"}}`}},
			gateWfToolResp(`{"workflow_id":"restart-sub","confidence":0.9}`),
		}
		inv := newTestInvestigator(investigator.Config{
			Client: mockClient, Builder: builder, ResultParser: rp,
			Enricher: enricher, AuditStore: auditStore, Logger: logger,
			MaxTurns: 15, PhaseTools: phaseTools, ScopeResolver: failingDiscoveryResolver{},
			// Do not let the default discovery fixture mask the unavailable
			// target-scoped discovery path this scenario is proving.
			Registry: toolregistry.New(),
		})

		result, err := inv.Investigate(context.Background(), signal)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.HumanReviewNeeded).To(BeTrue(), "discovery failure cannot verify the GVK")
		Expect(result.WorkflowID).To(BeEmpty(), "no workflow may be selected without GVK verification")
	})

	It("UT-KA-2609-001: verifies fleet GVK against the target cluster rather than the hub [BR-INTEGRATION-054]", func() {
		mockClient.responses = []llm.ChatResponse{
			{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"Subscription needs repair","confidence":0.85,"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator","api_version":"remote.example/v1"}}`}},
			gateWfToolResp(`{"workflow_id":"restart-sub","confidence":0.9}`),
		}
		discovery := &recordingDiscoveryTool{}
		ctx := investigator.WithFleetOverlay(audit.WithClusterID(context.Background(), "spoke"), map[string]tools.Tool{
			"api_resources_list": discovery,
		})
		remoteSignal := signal
		remoteSignal.ClusterID = "spoke"
		inv := newTestInvestigator(investigator.Config{
			Client: mockClient, Builder: builder, ResultParser: rp,
			Enricher: enricher, AuditStore: auditStore, Logger: logger,
			MaxTurns: 15, PhaseTools: phaseTools,
			ScopeResolver: investigator.NewMapperScopeResolver(newAmbiguousSubscriptionMapper()),
		})

		result, err := inv.Investigate(ctx, remoteSignal)
		Expect(err).NotTo(HaveOccurred())
		Expect(discovery.calls).To(BeNumerically(">", 0), "the target-cluster discovery tool must be consulted")
		Expect(result.RemediationTarget.APIVersion).To(Equal("remote.example/v1"))
	})

	It("UT-KA-2609-021: persists the cancellation gate audit with a live context [BR-AUDIT-005]", func() {
		ctx, cancel := context.WithCancel(context.Background())
		mockClient.responses = []llm.ChatResponse{
			{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"Deployment needs repair","confidence":0.85,"remediation_target":{"kind":"Deployment","name":"api-server","namespace":"demo-operator","api_version":"apps/v1"}}`}},
			{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"Deployment needs repair","confidence":0.85,"remediation_target":{"kind":"Deployment","name":"api-server","namespace":"demo-operator","api_version":"apps/v1"}}`}},
		}
		registry := toolregistry.New()
		registry.Register(&cancellingDiscoveryTool{cancel: cancel})
		inv := newTestInvestigator(investigator.Config{
			Client: mockClient, Builder: builder, ResultParser: rp,
			Enricher: enricher, AuditStore: auditStore, Logger: logger,
			MaxTurns: 15, PhaseTools: phaseTools, Registry: registry,
			ScopeResolver: investigator.NewMapperScopeResolver(newAmbiguousSubscriptionMapper()),
		})

		result, err := inv.Investigate(ctx, signal)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		Expect(result.Cancelled).To(BeTrue(), "cancellation during target GVK verification must remain cancellation")
		gateEvents := auditStore.eventsByAction(audit.ActionAPIVersionGate)
		Expect(gateEvents).To(HaveLen(1))
		Expect(gateEvents[0].Data).To(HaveKeyWithValue("retry_outcome", "cancelled"))
		Expect(auditStore.contextErrorsByAction(audit.ActionAPIVersionGate)).To(ConsistOf(BeNil()),
			"cancellation audit persistence must not receive the canceled context")
		Expect(auditStore.contextErrorsByAction(audit.ActionToolExecution)).To(ConsistOf(BeNil()),
			"target-scoped discovery tool-call audit persistence must not receive the canceled context")
	})

	Describe("UT-KA-1044-001: Ambiguous kind, gate fires, retry provides api_version", func() {
		It("should accept the retry result with api_version populated (BR-AI-1044 AC2)", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription etcd needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator"}
				}`}},
				// Gate retry: LLM provides api_version
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription etcd needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator","api_version":"operators.coreos.com/v1alpha1"}
				}`}},
				gateWfToolResp(`{"workflow_id":"restart-sub","confidence":0.9,"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator","api_version":"operators.coreos.com/v1alpha1"}}`),
			}

			resolver := investigator.NewMapperScopeResolver(newAmbiguousSubscriptionMapper())
			inv := newTestInvestigator(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools, ScopeResolver: resolver,
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RemediationTarget.APIVersion).To(Equal("operators.coreos.com/v1alpha1"),
				"UT-KA-1044-001: gate retry must populate api_version")
			Expect(result.HumanReviewNeeded).To(BeFalse(),
				"UT-KA-1044-001: successful retry must not trigger human review")
		})
	})

	Describe("UT-KA-1044-002: Ambiguous kind, retry still omits api_version — human review", func() {
		It("should set HumanReviewNeeded=true with reason rca_incomplete (BR-AI-1044 AC3 Security)", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription etcd needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator"}
				}`}},
				// Gate retry: LLM still omits api_version
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription etcd needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator"}
				}`}},
			}

			resolver := investigator.NewMapperScopeResolver(newAmbiguousSubscriptionMapper())
			inv := newTestInvestigator(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools, ScopeResolver: resolver,
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.HumanReviewNeeded).To(BeTrue(),
				"UT-KA-1044-002: gate exhaustion must trigger human review to prevent incorrect RBAC")
			Expect(result.HumanReviewReason).To(Equal("rca_incomplete"),
				"UT-KA-1044-002: reason must be rca_incomplete")
			Expect(result.WorkflowID).To(BeEmpty(),
				"UT-KA-1044-002: workflow must be cleared on gate exhaustion to prevent incorrect RBAC grants")
		})
	})

	Describe("UT-KA-1044-003: Unambiguous kind — gate verifies the exact model GVK", func() {
		It("should verify the supplied Deployment apiVersion without LLM retry (BR-WORKFLOW-004)", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Deployment needs more replicas",
					"confidence":0.85,
					"remediation_target":{"kind":"Deployment","name":"api-server","namespace":"production","api_version":"apps/v1"}
				}`}},
				gateWfToolResp(`{"workflow_id":"scale-up","confidence":0.9,"remediation_target":{"kind":"Deployment","name":"api-server","namespace":"production"}}`),
			}

			resolver := investigator.NewMapperScopeResolver(newAmbiguousSubscriptionMapper())
			inv := newTestInvestigator(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools, ScopeResolver: resolver,
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.HumanReviewNeeded).To(BeFalse(),
				"UT-KA-1044-003: unambiguous kind must not trigger human review")
			Expect(result.WorkflowID).To(Equal("scale-up"),
				"UT-KA-1044-003: pipeline should proceed to workflow selection")
			Expect(result.RemediationTarget.APIVersion).To(Equal("apps/v1"),
				"UT-KA-1044-003: the exact model apiVersion must be preserved after target-scoped verification")
			Expect(result.RemediationTarget.APIVersionValidated).To(BeTrue(),
				"UT-KA-1044-003: the supplied GVK must carry independent verification provenance")
			Expect(mockClient.calls).To(HaveLen(2),
				"UT-KA-1044-003: exact GVK verification must not add an LLM call")
		})
	})

	Describe("UT-KA-1051-011: Core-group kind — gate verifies the exact model GVK", func() {
		It("should verify apiVersion v1 for Pod without LLM retry (BR-WORKFLOW-004)", func() {
			coreMapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
				{Group: "", Version: "v1"},
				{Group: "apps", Version: "v1"},
			})
			coreMapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}, meta.RESTScopeNamespace)
			coreMapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, meta.RESTScopeNamespace)

			// Signal uses Deployment kind so the same-kind gate does NOT fire
			// when the RCA target is Pod (cross-type target).
			podSignal := katypes.SignalContext{
				ResourceKind: "Deployment",
				ResourceName: "api-server",
				Name:         "api-server",
				Namespace:    "default",
				Severity:     "critical",
				Message:      "Pod OOMKilled",
			}

			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Pod OOMKilled",
					"confidence":0.90,
					"remediation_target":{"kind":"Pod","name":"my-pod","namespace":"default","api_version":"v1"}
				}`}},
				gateWfToolResp(`{"workflow_id":"restart-pod","confidence":0.9,"remediation_target":{"kind":"Pod","name":"my-pod","namespace":"default"}}`),
			}

			resolver := investigator.NewMapperScopeResolver(coreMapper)
			inv := newTestInvestigator(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools, ScopeResolver: resolver,
			})

			result, err := inv.Investigate(context.Background(), podSignal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RemediationTarget.APIVersion).To(Equal("v1"),
				"UT-KA-1051-011: core-group Pod must preserve the exact apiVersion v1 (no group prefix)")
			Expect(result.RemediationTarget.APIVersionValidated).To(BeTrue(),
				"UT-KA-1051-011: the supplied core-group GVK must be independently verified")
			Expect(mockClient.calls).To(HaveLen(2),
				"UT-KA-1051-011: exact GVK verification must not add an LLM call")
		})
	})

	Describe("UT-KA-1044-004: nil scopeResolver does not bypass exact-GVK validation", func() {
		It("should require review when a target GVK is missing and no mapper is available (BR-KA-212)", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator"}
				}`}},
				gateWfToolResp(`{"workflow_id":"restart-sub","confidence":0.9,"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator"}}`),
			}

			// No ScopeResolver in config
			inv := newTestInvestigator(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.HumanReviewNeeded).To(BeTrue(),
				"UT-KA-1044-004: nil resolver must not bypass missing-GVK review")
			Expect(result.WorkflowID).To(BeEmpty(),
				"UT-KA-1044-004: an unverified target must not select a workflow")
		})
	})

	Describe("UT-KA-1044-005: empty RemediationTarget.Kind skips gate", func() {
		It("should skip the gate when kind is empty (BR-AI-1044 nil/zero)", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"general issue found",
					"confidence":0.75
				}`}},
				gateWfToolResp(`{"workflow_id":"generic-fix","confidence":0.8}`),
			}

			resolver := investigator.NewMapperScopeResolver(newAmbiguousSubscriptionMapper())
			inv := newTestInvestigator(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools, ScopeResolver: resolver,
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.HumanReviewNeeded).To(BeFalse(),
				"UT-KA-1044-005: empty kind must not trigger gate")
		})
	})

	Describe("UT-KA-1044-006: nonempty api_version requires independent verification", func() {
		It("should preserve a model api_version only after target-scoped verification (BR-KA-212)", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription etcd needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator","api_version":"operators.coreos.com/v1alpha1"}
				}`}},
				gateWfToolResp(`{"workflow_id":"restart-sub","confidence":0.9,"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator","api_version":"operators.coreos.com/v1alpha1"}}`),
			}

			resolver := investigator.NewMapperScopeResolver(newAmbiguousSubscriptionMapper())
			inv := newTestInvestigator(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools, ScopeResolver: resolver,
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RemediationTarget.APIVersion).To(Equal("operators.coreos.com/v1alpha1"),
				"UT-KA-1044-006: independently verified model api_version must survive finalization")
			Expect(result.RemediationTarget.APIVersionValidated).To(BeTrue(),
				"UT-KA-1044-006: the target must carry verification provenance")
			// Only 2 LLM calls: RCA + workflow (no gate correction retry)
			Expect(mockClient.calls).To(HaveLen(2),
				"UT-KA-1044-006: a valid supplied api_version must not trigger correction")
		})
	})

	Describe("UT-KA-1044-007: Correction message names conflicting groups", func() {
		It("should include both API group names in the correction message (BR-AI-1044 AC5)", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription etcd needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator"}
				}`}},
				// Gate retry provides api_version
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription etcd needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator","api_version":"operators.coreos.com/v1alpha1"}
				}`}},
				gateWfToolResp(`{"workflow_id":"restart-sub","confidence":0.9}`),
			}

			resolver := investigator.NewMapperScopeResolver(newAmbiguousSubscriptionMapper())
			inv := newTestInvestigator(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools, ScopeResolver: resolver,
			})

			_, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())

			// The gate retry call is the 2nd LLM call (index 1).
			// Inspect the last user message for group names.
			Expect(mockClient.calls).To(HaveLen(3),
				"UT-KA-1044-007: must have 3 LLM calls (RCA, gate retry, workflow)")
			gateCall := mockClient.calls[1]
			lastMsg := gateCall.Messages[len(gateCall.Messages)-1]
			Expect(lastMsg.Content).To(ContainSubstring("operators.coreos.com"),
				"UT-KA-1044-007: correction message must name operators.coreos.com")
			Expect(lastMsg.Content).To(ContainSubstring("messaging.knative.dev"),
				"UT-KA-1044-007: correction message must name messaging.knative.dev")
			Expect(lastMsg.Content).To(ContainSubstring("api_version"),
				"UT-KA-1044-007: correction message must mention api_version")
			Expect(lastMsg.Content).To(ContainSubstring("Subscription"),
				"UT-KA-1044-007: correction message must mention the kind")
		})
	})

	Describe("UT-KA-1044-008: Gate audit event emitted with ambiguity details", func() {
		It("should emit an audit event with ambiguous_kind, conflicting_groups, retry_outcome (BR-AI-1044 AC6)", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription etcd needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator"}
				}`}},
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription etcd needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator","api_version":"operators.coreos.com/v1alpha1"}
				}`}},
				gateWfToolResp(`{"workflow_id":"restart-sub","confidence":0.9}`),
			}

			resolver := investigator.NewMapperScopeResolver(newAmbiguousSubscriptionMapper())
			inv := newTestInvestigator(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools, ScopeResolver: resolver,
			})

			_, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())

			gateEvents := auditStore.eventsByAction(audit.ActionAPIVersionGate)
			Expect(gateEvents).NotTo(BeEmpty(),
				"UT-KA-1044-008: gate must emit an audit event with ActionAPIVersionGate")
			ev := gateEvents[0]
			Expect(ev.Data).To(HaveKey("ambiguous_kind"),
				"UT-KA-1044-008: audit event must include ambiguous_kind")
			Expect(ev.Data).To(HaveKey("conflicting_groups"),
				"UT-KA-1044-008: audit event must include conflicting_groups")
			Expect(ev.Data).To(HaveKey("retry_outcome"),
				"UT-KA-1044-008: audit event must include retry_outcome")
		})
	})

	Describe("UT-KA-1044-009: IsAmbiguousKind mapper error cannot bypass the gate", func() {
		It("should require review when the mapper fails and the model omits api_version (BR-KA-212)", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription etcd needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator"}
				}`}},
				gateWfToolResp(`{"workflow_id":"restart-sub","confidence":0.9,"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator"}}`),
			}

			// Use an empty mapper that will return errors for any kind
			emptyMapper := meta.NewDefaultRESTMapper(nil)
			resolver := investigator.NewMapperScopeResolver(emptyMapper)
			inv := newTestInvestigator(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools, ScopeResolver: resolver,
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.HumanReviewNeeded).To(BeTrue(),
				"UT-KA-1044-009: mapper failure must not bypass missing-GVK review")
			Expect(result.WorkflowID).To(BeEmpty(),
				"UT-KA-1044-009: an unverified target must not select a workflow")
		})
	})

	Describe("UT-KA-1044-018: Gate retry — LLM response is unparseable", func() {
		It("should keep original result when retry response is garbage (BR-AI-1044 error)", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription etcd needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator"}
				}`}},
				// Gate retry: garbage response
				{Message: llm.Message{Role: "assistant", Content: `not valid json at all`}},
			}

			resolver := investigator.NewMapperScopeResolver(newAmbiguousSubscriptionMapper())
			inv := newTestInvestigator(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools, ScopeResolver: resolver,
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// Unparseable retry → gate keeps original → exhaustion → human review
			Expect(result.HumanReviewNeeded).To(BeTrue(),
				"UT-KA-1044-018: unparseable retry must lead to exhaustion → human review")
		})
	})

	Describe("UT-KA-1044-019: Gate retry — LLM drops RemediationTarget", func() {
		It("should keep original result when retry loses remediation_target (BR-AI-1044 error)", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription etcd needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator"}
				}`}},
				// Gate retry: no remediation_target
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"confirmed issue",
					"confidence":0.80
				}`}},
			}

			resolver := investigator.NewMapperScopeResolver(newAmbiguousSubscriptionMapper())
			inv := newTestInvestigator(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools, ScopeResolver: resolver,
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// Retry dropped target → still no api_version → exhaustion → human review
			Expect(result.HumanReviewNeeded).To(BeTrue(),
				"UT-KA-1044-019: retry losing target must lead to human review")
		})
	})

	Describe("UT-KA-1044-020: Gate retry — LLM returns empty content", func() {
		It("should keep original result when retry response is empty (BR-AI-1044 error)", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription etcd needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator"}
				}`}},
				// Gate retry: empty
				{Message: llm.Message{Role: "assistant", Content: ""}},
			}

			resolver := investigator.NewMapperScopeResolver(newAmbiguousSubscriptionMapper())
			inv := newTestInvestigator(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools, ScopeResolver: resolver,
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.HumanReviewNeeded).To(BeTrue(),
				"UT-KA-1044-020: empty retry must lead to exhaustion → human review")
		})
	})

	Describe("UT-KA-1044-021: Gate retry — LLM client returns error", func() {
		It("should keep original result when LLM returns error (BR-AI-1044 error)", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription etcd needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator"}
				}`}},
			}
			// Second call (gate retry) returns error
			mockClient.errors = []error{nil, fmt.Errorf("LLM service unavailable")}

			resolver := investigator.NewMapperScopeResolver(newAmbiguousSubscriptionMapper())
			inv := newTestInvestigator(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools, ScopeResolver: resolver,
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.HumanReviewNeeded).To(BeTrue(),
				"UT-KA-1044-021: LLM error on retry must lead to exhaustion → human review")
		})
	})

	Describe("UT-KA-1044-022: Adversarial api_version from LLM", func() {
		It("should reject a malformed api_version and require review (BR-KA-212, SI-10)", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription etcd needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator"}
				}`}},
				// Gate retry: adversarial api_version
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription etcd needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator","api_version":"../../etc/passwd"}
				}`}},
				gateWfToolResp(`{"workflow_id":"restart-sub","confidence":0.9}`),
			}

			resolver := investigator.NewMapperScopeResolver(newAmbiguousSubscriptionMapper())
			inv := newTestInvestigator(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools, ScopeResolver: resolver,
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RemediationTarget.APIVersion).To(BeEmpty(),
				"UT-KA-1044-022: malformed api_version must not survive failed verification")
			Expect(result.HumanReviewNeeded).To(BeTrue(),
				"UT-KA-1044-022: malformed api_version must require human review")
			Expect(result.WorkflowID).To(BeEmpty(),
				"UT-KA-1044-022: malformed api_version must not select a workflow")
		})
	})

	Describe("UT-KA-1044-023: Exhaustion warning includes conflicting groups", func() {
		It("should include conflicting group names in Warnings (BR-AI-1044 observability)", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription etcd needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator"}
				}`}},
				// Gate retry: still no api_version
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription etcd needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator"}
				}`}},
			}

			resolver := investigator.NewMapperScopeResolver(newAmbiguousSubscriptionMapper())
			inv := newTestInvestigator(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools, ScopeResolver: resolver,
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Warnings).NotTo(BeEmpty(),
				"UT-KA-1044-023: exhaustion must add a warning")
			warningText := ""
			for _, w := range result.Warnings {
				warningText += w + " "
			}
			Expect(warningText).To(ContainSubstring("operators.coreos.com"),
				"UT-KA-1044-023: warning must include conflicting group names")
			Expect(warningText).To(ContainSubstring("messaging.knative.dev"),
				"UT-KA-1044-023: warning must include conflicting group names")
		})
	})

	Describe("UT-KA-1044-024: gate retry preserves the retry turn's reasoning (BR-AI-086 AC6, #1578)", func() {
		It("should carry the gate-retry turn's Reasoning onto the accepted result, not lose it", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{
					Role:    "assistant",
					Content: `{"rca_summary":"Subscription etcd needs restart","confidence":0.85,"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator"}}`,
					Reasoning: &llm.ReasoningBlock{
						Text: "initial turn: identified Subscription but omitted api_version",
					},
				}},
				// Gate retry: LLM provides api_version, with its own reasoning
				// block for this turn.
				{Message: llm.Message{
					Role:    "assistant",
					Content: `{"rca_summary":"Subscription etcd needs restart","confidence":0.85,"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator","api_version":"operators.coreos.com/v1alpha1"}}`,
					Reasoning: &llm.ReasoningBlock{
						Text: "retry turn: disambiguated to the operators.coreos.com API group based on the correction prompt",
					},
				}},
				gateWfToolResp(`{"workflow_id":"restart-sub","confidence":0.9,"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator","api_version":"operators.coreos.com/v1alpha1"}}`),
			}

			resolver := investigator.NewMapperScopeResolver(newAmbiguousSubscriptionMapper())
			inv := newTestInvestigator(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools, ScopeResolver: resolver,
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Reasoning).NotTo(BeNil(),
				"UT-KA-1044-024: the accepted gate-retry result must carry a Reasoning block, not lose it")
			Expect(result.Reasoning.Text).To(Equal("retry turn: disambiguated to the operators.coreos.com API group based on the correction prompt"),
				"UT-KA-1044-024: Reasoning must reflect the winning (retry) turn, not the original pre-gate turn")
		})
	})
})

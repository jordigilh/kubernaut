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
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

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
	mu     sync.Mutex
	events []*audit.AuditEvent
}

func (s *gateRecordingAuditStore) StoreAudit(_ context.Context, event *audit.AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
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

func (f *gateDSClient) GetRemediationHistory(_ context.Context, _, _, _, _ string) (*enrichment.RemediationHistoryResult, error) {
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

var _ = Describe("TP-1044: apiVersionValidationGate", func() {

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
			inv := investigator.New(investigator.Config{
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
			inv := investigator.New(investigator.Config{
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

	Describe("UT-KA-1044-003: Unambiguous kind bypasses gate", func() {
		It("should not fire the gate for Deployment (single API group) (BR-AI-1044 AC4)", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Deployment needs more replicas",
					"confidence":0.85,
					"remediation_target":{"kind":"Deployment","name":"api-server","namespace":"production"}
				}`}},
				gateWfToolResp(`{"workflow_id":"scale-up","confidence":0.9,"remediation_target":{"kind":"Deployment","name":"api-server","namespace":"production"}}`),
			}

			resolver := investigator.NewMapperScopeResolver(newAmbiguousSubscriptionMapper())
			inv := investigator.New(investigator.Config{
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
			// Only 2 LLM calls: RCA + workflow (no gate retry)
			Expect(mockClient.calls).To(HaveLen(2),
				"UT-KA-1044-003: gate must not add extra LLM calls for unambiguous kinds")
		})
	})

	Describe("UT-KA-1044-004: nil scopeResolver graceful degradation", func() {
		It("should skip the gate when ScopeResolver is nil (BR-AI-1044 AC7)", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator"}
				}`}},
				gateWfToolResp(`{"workflow_id":"restart-sub","confidence":0.9,"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator"}}`),
			}

			// No ScopeResolver in config
			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.HumanReviewNeeded).To(BeFalse(),
				"UT-KA-1044-004: nil resolver must not trigger human review")
			Expect(result.WorkflowID).To(Equal("restart-sub"),
				"UT-KA-1044-004: pipeline should proceed normally without resolver")
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
			inv := investigator.New(investigator.Config{
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

	Describe("UT-KA-1044-006: api_version already populated bypasses gate", func() {
		It("should skip the gate when api_version is present (BR-AI-1044 AC2)", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription etcd needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator","api_version":"operators.coreos.com/v1alpha1"}
				}`}},
				gateWfToolResp(`{"workflow_id":"restart-sub","confidence":0.9,"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator","api_version":"operators.coreos.com/v1alpha1"}}`),
			}

			resolver := investigator.NewMapperScopeResolver(newAmbiguousSubscriptionMapper())
			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools, ScopeResolver: resolver,
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RemediationTarget.APIVersion).To(Equal("operators.coreos.com/v1alpha1"),
				"UT-KA-1044-006: existing api_version must be preserved")
			// Only 2 LLM calls: RCA + workflow (no gate retry)
			Expect(mockClient.calls).To(HaveLen(2),
				"UT-KA-1044-006: gate must not fire when api_version is already populated")
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
			inv := investigator.New(investigator.Config{
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
			inv := investigator.New(investigator.Config{
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

	Describe("UT-KA-1044-009: IsAmbiguousKind mapper error — gate skips", func() {
		It("should skip gate and keep original result when mapper returns error (BR-AI-1044 error)", func() {
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
			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools, ScopeResolver: resolver,
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// Gate should skip on mapper error — pipeline proceeds normally
			Expect(result.HumanReviewNeeded).To(BeFalse(),
				"UT-KA-1044-009: mapper error must not trigger human review")
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
			inv := investigator.New(investigator.Config{
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
			inv := investigator.New(investigator.Config{
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
			inv := investigator.New(investigator.Config{
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
			inv := investigator.New(investigator.Config{
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
		It("should accept adversarial api_version without gate rejection (BR-AI-1044 adversarial)", func() {
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
			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools, ScopeResolver: resolver,
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// Gate accepts any non-empty api_version; validation is downstream
			Expect(result.RemediationTarget.APIVersion).To(Equal("../../etc/passwd"),
				"UT-KA-1044-022: gate must accept any non-empty api_version (validation is downstream)")
			Expect(result.HumanReviewNeeded).To(BeFalse(),
				"UT-KA-1044-022: non-empty api_version must prevent human review")
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
			inv := investigator.New(investigator.Config{
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
})

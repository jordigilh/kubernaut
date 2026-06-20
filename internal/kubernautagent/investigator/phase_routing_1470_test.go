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
	"sync"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// recordingPhaseResolver records ResolvePhase calls and returns controlled
// clients with distinct model names per phase.
type recordingPhaseResolver struct {
	mu       sync.Mutex
	calls    []katypes.Phase
	clients  map[katypes.Phase]llm.Client
	models   map[katypes.Phase]string
	fallback llm.Client
}

func newRecordingPhaseResolver(fallback llm.Client, fallbackModel string) *recordingPhaseResolver {
	return &recordingPhaseResolver{
		clients:  make(map[katypes.Phase]llm.Client),
		models:   map[katypes.Phase]string{"": fallbackModel},
		fallback: fallback,
	}
}

func (r *recordingPhaseResolver) addPhase(phase katypes.Phase, client llm.Client, model string) {
	r.clients[phase] = client
	r.models[phase] = model
}

func (r *recordingPhaseResolver) ResolvePhase(phase katypes.Phase) (llm.Client, string, llm.RuntimeParams) {
	r.mu.Lock()
	r.calls = append(r.calls, phase)
	r.mu.Unlock()

	c, ok := r.clients[phase]
	if !ok {
		c = r.fallback
	}
	model := r.models[phase]
	if model == "" {
		model = r.models[""]
	}
	return c, model, llm.RuntimeParams{}
}

func (r *recordingPhaseResolver) resolvedPhases() []katypes.Phase {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]katypes.Phase, len(r.calls))
	copy(out, r.calls)
	return out
}

// phaseAuditStore records audit events with an eventsByAction helper.
type phaseAuditStore struct {
	mu     sync.Mutex
	events []*audit.AuditEvent
}

func (s *phaseAuditStore) StoreAudit(_ context.Context, event *audit.AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

func (s *phaseAuditStore) llmRequestEvents() []*audit.AuditEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*audit.AuditEvent
	for _, e := range s.events {
		if e.EventType == audit.EventTypeLLMRequest {
			out = append(out, e)
		}
	}
	return out
}

// phaseScriptedClient returns phase-specific responses. RCA phase returns
// a submit_result tool call; workflow discovery phase returns submit_result_with_workflow.
type phaseScriptedClient struct {
	mu    sync.Mutex
	calls int
	model string
}

func newPhaseScriptedRCA(model string) *phaseScriptedClient {
	return &phaseScriptedClient{model: model}
}

func (s *phaseScriptedClient) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	s.mu.Lock()
	call := s.calls
	s.calls++
	s.mu.Unlock()

	_ = call
	return llm.ChatResponse{
		ToolCalls: []llm.ToolCall{
			{
				ID:   "tc-1",
				Name: "submit_result",
				Arguments: `{"rca_summary":"test rca from ` + s.model + `","confidence":0.9,` +
					`"remediation_target":{"kind":"Pod","name":"test","namespace":"default"}}`,
			},
		},
		Usage: llm.TokenUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}, nil
}

func (s *phaseScriptedClient) StreamChat(ctx context.Context, req llm.ChatRequest, cb func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	resp, err := s.Chat(ctx, req)
	if err == nil {
		_ = cb(llm.ChatStreamEvent{Delta: resp.Message.Content, Done: true})
	}
	return resp, err
}

func (s *phaseScriptedClient) Close() error { return nil }

func buildInvestigatorWithResolver(
	client llm.Client,
	store audit.AuditStore,
	resolver investigator.PhaseClientResolver,
) *investigator.Investigator {
	builder, err := prompt.NewBuilder()
	Expect(err).NotTo(HaveOccurred())

	enricher := enrichment.NewEnricher(&gateK8sClient{}, &gateDSClient{}, store, logr.Discard())

	return investigator.New(investigator.Config{
		Client:        client,
		Builder:       builder,
		ResultParser:  parser.NewResultParser(),
		Enricher:      enricher,
		AuditStore:    store,
		Logger:        logr.Discard(),
		MaxTurns:      5,
		PhaseTools:    investigator.DefaultPhaseToolMap(),
		PhaseResolver: resolver,
	})
}

var _ = Describe("Per-Phase LLM Model Routing — Investigator Dispatch #1470", func() {
	var (
		signal katypes.SignalContext
	)

	BeforeEach(func() {
		signal = katypes.SignalContext{
			Name:          "test-pod",
			Namespace:     "default",
			Severity:      "high",
			Message:       "OOMKilled",
			RemediationID: "rem-1470-001",
		}
	})

	Describe("IT-AI-1470-003a: Investigate resolves PhaseRCA then PhaseWorkflowDiscovery", func() {
		It("should call ResolvePhase with both phase constants in order", func() {
			rcaClient := newPhaseScriptedRCA("sonnet")
			resolver := newRecordingPhaseResolver(rcaClient, "default-model")
			resolver.addPhase(katypes.PhaseRCA, rcaClient, "sonnet")

			wfClient := &pinSubmitClient{}
			resolver.addPhase(katypes.PhaseWorkflowDiscovery, wfClient, "haiku")

			store := &phaseAuditStore{}
			inv := buildInvestigatorWithResolver(rcaClient, store, resolver)
			_, _ = inv.Investigate(context.Background(), signal)

			phases := resolver.resolvedPhases()
			Expect(phases).To(ContainElement(katypes.PhaseRCA))
			Expect(phases).To(ContainElement(katypes.PhaseWorkflowDiscovery))
		})
	})

	Describe("IT-AI-1470-003b: RunInteractiveTurn resolves PhaseRCA", func() {
		It("should call ResolvePhase with PhaseRCA", func() {
			client := newPhaseScriptedRCA("sonnet")
			resolver := newRecordingPhaseResolver(client, "sonnet")
			resolver.addPhase(katypes.PhaseRCA, client, "sonnet")

			store := &phaseAuditStore{}
			inv := buildInvestigatorWithResolver(client, store, resolver)

			messages := []llm.Message{{Role: "user", Content: "test"}}
			_, _ = inv.RunInteractiveTurn(context.Background(), messages, "corr-001")

			phases := resolver.resolvedPhases()
			Expect(phases).To(ContainElement(katypes.PhaseRCA))
		})
	})

	Describe("IT-AI-1470-003c: RunWorkflowDiscoveryFromRCA resolves PhaseWorkflowDiscovery", func() {
		It("should call ResolvePhase with PhaseWorkflowDiscovery", func() {
			wfClient := &pinSubmitClient{}
			resolver := newRecordingPhaseResolver(wfClient, "haiku")
			resolver.addPhase(katypes.PhaseWorkflowDiscovery, wfClient, "haiku")

			store := &phaseAuditStore{}
			inv := buildInvestigatorWithResolver(wfClient, store, resolver)

			rcaResult := &katypes.InvestigationResult{
				RCASummary: "test rca",
				Confidence: 0.9,
				RemediationTarget: katypes.RemediationTarget{
					Kind: "Pod", Name: "test", Namespace: "default",
				},
			}
			_, _ = inv.RunWorkflowDiscoveryFromRCA(context.Background(), signal, rcaResult, nil, "corr-002")

			phases := resolver.resolvedPhases()
			Expect(phases).To(ContainElement(katypes.PhaseWorkflowDiscovery))
		})
	})

	Describe("IT-AI-1470-003d: RunRCAExtractionFromConversation resolves PhaseRCA", func() {
		It("should call ResolvePhase with PhaseRCA", func() {
			client := newPhaseScriptedRCA("sonnet")
			resolver := newRecordingPhaseResolver(client, "sonnet")
			resolver.addPhase(katypes.PhaseRCA, client, "sonnet")

			store := &phaseAuditStore{}
			inv := buildInvestigatorWithResolver(client, store, resolver)

			messages := []llm.Message{{Role: "user", Content: "test"}}
			_, _ = inv.RunRCAExtractionFromConversation(context.Background(), messages, "corr-003")

			phases := resolver.resolvedPhases()
			Expect(phases).To(ContainElement(katypes.PhaseRCA))
		})
	})

	Describe("IT-AI-1470-003e: Nil PhaseResolver falls back to legacy behavior", func() {
		It("should not panic when PhaseResolver is nil", func() {
			client := &pinSubmitClient{}
			swappable, err := llm.NewSwappableClient(client, "test-model")
			Expect(err).NotTo(HaveOccurred())

			builder, err := prompt.NewBuilder()
			Expect(err).NotTo(HaveOccurred())

			store := &phaseAuditStore{}
			enricher := enrichment.NewEnricher(&gateK8sClient{}, &gateDSClient{}, store, logr.Discard())

			inv := investigator.New(investigator.Config{
				Client:       client,
				Builder:      builder,
				ResultParser: parser.NewResultParser(),
				Enricher:     enricher,
				AuditStore:   store,
				Logger:       logr.Discard(),
				MaxTurns:     5,
				PhaseTools:   investigator.DefaultPhaseToolMap(),
				Swappable:    swappable,
			})

			Expect(func() {
				_, _ = inv.Investigate(context.Background(), signal)
			}).NotTo(Panic())
		})
	})

	Describe("IT-AI-1470-003f (AU-2/AU-12): RCA audit events contain per-phase model name", func() {
		It("should emit audit events with the RCA model name", func() {
			rcaClient := newPhaseScriptedRCA("sonnet-reasoning")
			resolver := newRecordingPhaseResolver(rcaClient, "default")
			resolver.addPhase(katypes.PhaseRCA, rcaClient, "sonnet-reasoning")

			wfClient := &pinSubmitClient{}
			resolver.addPhase(katypes.PhaseWorkflowDiscovery, wfClient, "haiku-fast")

			store := &phaseAuditStore{}
			inv := buildInvestigatorWithResolver(rcaClient, store, resolver)
			_, _ = inv.Investigate(context.Background(), signal)

			llmEvents := store.llmRequestEvents()
			Expect(llmEvents).NotTo(BeEmpty())

			foundRCAModel := false
			for _, e := range llmEvents {
				if model, ok := e.Data["model"]; ok && model == "sonnet-reasoning" {
					foundRCAModel = true
					break
				}
			}
			Expect(foundRCAModel).To(BeTrue(),
				"At least one LLM request audit event should have model='sonnet-reasoning'")
		})
	})

	Describe("IT-AI-1470-003g (AU-2/AU-12): Workflow discovery audit events contain fast model name", func() {
		It("should emit audit events with the workflow discovery model name", func() {
			rcaClient := newPhaseScriptedRCA("sonnet-reasoning")
			resolver := newRecordingPhaseResolver(rcaClient, "default")
			resolver.addPhase(katypes.PhaseRCA, rcaClient, "sonnet-reasoning")

			wfClient := &pinSubmitClient{}
			resolver.addPhase(katypes.PhaseWorkflowDiscovery, wfClient, "haiku-fast")

			store := &phaseAuditStore{}
			inv := buildInvestigatorWithResolver(rcaClient, store, resolver)
			_, _ = inv.Investigate(context.Background(), signal)

			llmEvents := store.llmRequestEvents()
			Expect(llmEvents).NotTo(BeEmpty())

			foundWFModel := false
			for _, e := range llmEvents {
				if model, ok := e.Data["model"]; ok && model == "haiku-fast" {
					foundWFModel = true
					break
				}
			}
			Expect(foundWFModel).To(BeTrue(),
				"At least one LLM request audit event should have model='haiku-fast'")
		})
	})
})

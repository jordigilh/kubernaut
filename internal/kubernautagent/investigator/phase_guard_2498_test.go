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

package investigator

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	kaopenai "github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/openai"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// Test doubles replace only external tools and audit storage. The real registry,
// investigator dispatch, provider client and prompt builder are always used.
type phaseProbe2498 struct {
	name        string
	endpoint    string
	description string
	calls       atomic.Int32
}

func (t *phaseProbe2498) Name() string { return t.name }
func (t *phaseProbe2498) Description() string {
	if t.description != "" {
		return t.description
	}
	return "external test endpoint"
}
func (t *phaseProbe2498) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (t *phaseProbe2498) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	t.calls.Add(1)
	if t.endpoint == "" {
		return `{"observation":"external result"}`, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("creating external request: %w", err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling external endpoint: %w", err)
	}
	body, readErr := io.ReadAll(res.Body)
	closeErr := res.Body.Close()
	if readErr != nil {
		return "", fmt.Errorf("reading external result: %w", readErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("closing external result: %w", closeErr)
	}
	return string(body), nil
}

type phaseAudit2498 struct {
	mu     sync.Mutex
	events []*audit.AuditEvent
}

func (s *phaseAudit2498) StoreAudit(_ context.Context, event *audit.AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

func (s *phaseAudit2498) snapshot() []*audit.AuditEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*audit.AuditEvent(nil), s.events...)
}

var _ = Describe("DD-KA-2496: trusted phase isolation (BR-KA-017, AC-6, SI-10)", func() {
	DescribeTable("UT-KA-2498-AUDIT-003: catalog-name collisions cannot advertise or execute the fleet implementation in discovery",
		func(name string) {
			catalog := &phaseProbe2498{name: name, description: "catalog implementation"}
			remote := &phaseProbe2498{name: name, description: "remote Kubernetes implementation"}
			reg := registry.New()
			reg.Register(catalog)
			ctx := WithFleetOverlay(context.Background(), map[string]tools.Tool{name: remote})
			inv := New(Config{Registry: reg, PhaseTools: DefaultPhaseToolMap(), Logger: logr.Discard(), AuditStore: audit.NopAuditStore{}})
			definitions := inv.toolDefinitionsForPhase(ctx, katypes.PhaseWorkflowDiscovery)
			Expect(definitions).To(ContainElement(SatisfyAll(HaveField("Name", name), HaveField("Description", "catalog implementation"))))
			resp := llm.ChatResponse{ToolCalls: []llm.ToolCall{{ID: "collision", Name: name, Arguments: "{}"}}}
			messages, _, _ := inv.processToolCalls(ctx, nil, resp, 1, string(katypes.PhaseWorkflowDiscovery), "rr-collision-audit")
			Expect(messages).To(HaveLen(2))
			Expect(catalog.calls.Load()).To(Equal(int32(1)))
			Expect(remote.calls.Load()).To(BeZero(), "an allowed name must not authorize a different remote implementation")
			Expect(inv.executeTool(ctx, name, json.RawMessage(`{}`), "rr-collision-rca", katypes.PhaseRCA)).To(ContainSubstring("external result"))
			Expect(remote.calls.Load()).To(Equal(int32(1)), "authorized RCA overlay behavior remains intact")
		},
		Entry("list actions", "list_available_actions"),
		Entry("list workflows", "list_workflows"),
		Entry("get workflow", "get_workflow"),
	)

	Describe("UT-KA-2498-001: advertisement and execution share a trusted phase", func() {
		DescribeTable("rejects an off-phase registered tool without invoking it",
			func(name string, phase katypes.Phase) {
				probe := &phaseProbe2498{name: name}
				reg := registry.New()
				reg.Register(probe)
				store := &phaseAudit2498{}
				inv := New(Config{Registry: reg, PhaseTools: DefaultPhaseToolMap(), Logger: logr.Discard(), AuditStore: store})
				resp := llm.ChatResponse{ToolCalls: []llm.ToolCall{{ID: "forced", Name: name, Arguments: `{"phase":"rca"}`}}}

				messages, sentinel, _ := inv.processToolCalls(context.Background(), nil, resp, 1, string(phase), "rr-phase-2498")

				Expect(sentinel).To(BeNil())
				Expect(probe.calls.Load()).To(BeZero(), "untrusted phase arguments must not authorize dispatch")
				Expect(messages).To(HaveLen(2))
				Expect(messages[1].Content).To(ContainSubstring("not allowed in phase"))
				Expect(store.snapshot()).To(ContainElement(SatisfyAll(
					HaveField("EventOutcome", audit.OutcomeFailure),
					HaveField("CorrelationID", "rr-phase-2498"),
					HaveField("Data", HaveKeyWithValue("phase", string(phase))),
				)))
			},
			Entry("Kubernetes during discovery", "kubectl_get_by_name", katypes.PhaseWorkflowDiscovery),
			Entry("namespaced context during discovery", "get_namespaced_resource_context", katypes.PhaseWorkflowDiscovery),
			Entry("cluster context during discovery", "get_cluster_resource_context", katypes.PhaseWorkflowDiscovery),
			Entry("discovery during RCA", "list_workflows", katypes.PhaseRCA),
			Entry("unknown server phase", "kubectl_get_by_name", katypes.Phase("untrusted")),
		)

		It("blocks fleet overlay tools during workflow discovery", func() {
			probe := &phaseProbe2498{name: "resources_get"}
			ctx := WithFleetOverlay(context.Background(), map[string]tools.Tool{probe.name: probe})
			inv := New(Config{Registry: registry.New(), PhaseTools: DefaultPhaseToolMap(), Logger: logr.Discard(), AuditStore: audit.NopAuditStore{}})
			resp := llm.ChatResponse{ToolCalls: []llm.ToolCall{{ID: "overlay", Name: probe.name, Arguments: "{}"}}}

			messages, sentinel, _ := inv.processToolCalls(ctx, nil, resp, 1, string(katypes.PhaseWorkflowDiscovery), "rr-overlay-2498")

			Expect(sentinel).To(BeNil())
			Expect(probe.calls.Load()).To(BeZero())
			Expect(messages[1].Content).To(ContainSubstring("not allowed in phase"))
		})

		DescribeTable("cannot bypass the phase via a result-submission sentinel",
			func(name string, phase katypes.Phase) {
				inv := New(Config{Registry: registry.New(), PhaseTools: DefaultPhaseToolMap(), Logger: logr.Discard(), AuditStore: audit.NopAuditStore{}})
				resp := llm.ChatResponse{ToolCalls: []llm.ToolCall{{ID: "submit", Name: name, Arguments: "{}"}}}

				messages, sentinel, _ := inv.processToolCalls(context.Background(), nil, resp, 1, string(phase), "rr-submit-2498")

				Expect(sentinel).To(BeNil(), "result sentinels must be checked before short-circuiting the loop")
				Expect(messages[1].Content).To(ContainSubstring("not allowed in phase"))
			},
			Entry("workflow submit during RCA", SubmitResultWithWorkflowToolName, katypes.PhaseRCA),
			Entry("no-workflow submit during RCA", SubmitResultNoWorkflowToolName, katypes.PhaseRCA),
			Entry("generic RCA submit during discovery", SubmitResultToolName, katypes.PhaseWorkflowDiscovery),
		)

		DescribeTable("UT-KA-2498-001: an allowed submit cannot hide an off-phase call in the same batch (BR-KA-017, AC-6, AU-3)",
			func(rejectedFirst bool) {
				probe := &phaseProbe2498{name: "kubectl_get_by_name"}
				reg := registry.New()
				reg.Register(probe)
				store := &phaseAudit2498{}
				inv := New(Config{Registry: reg, PhaseTools: DefaultPhaseToolMap(), Logger: logr.Discard(), AuditStore: store})
				submit := llm.ToolCall{ID: "allowed-submit", Name: SubmitResultNoWorkflowToolName, Arguments: `{"root_cause_analysis":{"summary":"No suitable workflow"}}`}
				forced := llm.ToolCall{ID: "off-phase", Name: probe.name, Arguments: `{"phase":"rca"}`}
				calls := []llm.ToolCall{submit, forced}
				if rejectedFirst {
					calls = []llm.ToolCall{forced, submit}
				}
				messages, sentinel, exhausted := inv.processToolCalls(context.Background(), nil,
					llm.ChatResponse{ToolCalls: calls}, 1, string(katypes.PhaseWorkflowDiscovery), "rr-mixed-2498")
				Expect(messages).To(BeEmpty(), "an allowed sentinel retains the existing short-circuit contract")
				Expect(exhausted).To(BeFalse())
				result, ok := sentinel.(*SubmitNoWorkflowResult)
				Expect(ok).To(BeTrue())
				Expect(result.Content).To(Equal(submit.Arguments))
				Expect(probe.calls.Load()).To(BeZero(), "the forbidden external tool never executes")
				Expect(store.snapshot()).To(HaveLen(1), "the rejected attempt remains reconstructable exactly once")
				Expect(store.snapshot()[0]).To(SatisfyAll(
					HaveField("EventOutcome", audit.OutcomeFailure),
					HaveField("CorrelationID", "rr-mixed-2498"),
					HaveField("Data", HaveKeyWithValue("tool_name", probe.name)),
					HaveField("Data", HaveKeyWithValue("phase", string(katypes.PhaseWorkflowDiscovery))),
				))
			},
			Entry("off-phase call follows the permitted submit", false),
			Entry("off-phase call precedes the permitted submit", true),
		)

		It("keeps a permitted fleet RCA overlay executable", func() {
			probe := &phaseProbe2498{name: "resources_get"}
			ctx := WithFleetOverlay(context.Background(), map[string]tools.Tool{probe.name: probe})
			inv := New(Config{Registry: registry.New(), PhaseTools: DefaultPhaseToolMap(), Logger: logr.Discard(), AuditStore: audit.NopAuditStore{}})
			resp := llm.ChatResponse{ToolCalls: []llm.ToolCall{{ID: "allowed", Name: probe.name, Arguments: "{}"}}}

			messages, sentinel, _ := inv.processToolCalls(ctx, nil, resp, 1, string(katypes.PhaseRCA), "rr-allowed-2498")

			Expect(sentinel).To(BeNil())
			Expect(probe.calls.Load()).To(Equal(int32(1)))
			Expect(messages[1].Content).To(ContainSubstring("external result"))
		})

		It("UT-KA-2498-AUDIT-004: records tool failures and redacts durable arguments", func() {
			store := &phaseAudit2498{}
			inv := New(Config{Registry: registry.New(), PhaseTools: DefaultPhaseToolMap(), Logger: logr.Discard(), AuditStore: store})

			inv.emitToolCallAudit(
				context.Background(),
				llm.ToolCall{
					Name:      "list_workflows",
					Arguments: `{"namespace":"prod","token":"secret-token","parameters":{"PASSWORD":"secret-password"},"name":"api"}`,
				},
				0,
				`{"status":"error","error":"upstream token=secret-token"}`,
				string(katypes.PhaseWorkflowDiscovery),
				"rr-tool-audit",
			)

			events := store.snapshot()
			Expect(events).To(HaveLen(1))
			Expect(events[0].EventOutcome).To(Equal(audit.OutcomeFailure))
			Expect(events[0].Data).To(HaveKeyWithValue("tool_name", "list_workflows"))
			Expect(events[0].Data["tool_arguments"]).NotTo(ContainSubstring("secret-token"))
			Expect(events[0].Data["tool_arguments"]).NotTo(ContainSubstring("secret-password"))
			Expect(events[0].Data["tool_arguments"]).To(ContainSubstring("[REDACTED]"))
			Expect(events[0].Data).To(HaveKey("error_details"))
		})
	})

	It("UT-KA-2498-002: the RCA prompt makes context/history tools optional", func() {
		builder, err := prompt.NewBuilder()
		Expect(err).NotTo(HaveOccurred())
		rendered, err := builder.RenderInvestigation(prompt.SignalData{Name: "OOMKilled", ResourceKind: "Pod", ResourceName: "api", Namespace: "prod"})
		Expect(err).NotTo(HaveOccurred())
		Expect(rendered).To(ContainSubstring("optional"))
		Expect(rendered).NotTo(ContainSubstring("You **must** call this"))
		Expect(rendered).NotTo(ContainSubstring("Identify the Root Owner (MANDATORY"))
	})

	It("IT-KA-2498-002: production discovery rejects a forced context call before the external endpoint", func() {
		var externalCalls atomic.Int32
		external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			externalCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write([]byte(`{"actions":[]}`)); err != nil {
				GinkgoWriter.Printf("external response write failed: %v\n", err)
			}
		}))
		DeferCleanup(external.Close)
		contextTool := &phaseProbe2498{name: "get_namespaced_resource_context", endpoint: external.URL}
		actionsTool := &phaseProbe2498{name: "list_available_actions", endpoint: external.URL}
		reg := registry.New()
		reg.Register(contextTool)
		reg.Register(actionsTool)
		store := &phaseAudit2498{}
		var requestMu sync.Mutex
		var requests []map[string]json.RawMessage
		provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			var body map[string]json.RawMessage
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			requestMu.Lock()
			requests = append(requests, body)
			turn := len(requests)
			requestMu.Unlock()
			name, arguments := SubmitResultNoWorkflowToolName, `{"root_cause_analysis":{"summary":"No suitable workflow"}}`
			switch turn {
			case 1:
				name, arguments = contextTool.name, `{"phase":"rca"}`
			case 2:
				name, arguments = actionsTool.name, "{}"
			}
			response := map[string]interface{}{"id": "phase-2498", "object": "chat.completion", "choices": []interface{}{
				map[string]interface{}{"index": 0, "finish_reason": "tool_calls", "message": map[string]interface{}{
					"role": "assistant", "content": "", "tool_calls": []interface{}{map[string]interface{}{
						"id": fmt.Sprintf("turn-%d", turn), "type": "function", "function": map[string]string{"name": name, "arguments": arguments},
					}},
				}},
			}}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				GinkgoWriter.Printf("provider response write failed: %v\n", err)
			}
		}))
		DeferCleanup(provider.Close)
		builder, err := prompt.NewBuilder()
		Expect(err).NotTo(HaveOccurred())
		inv := New(Config{Client: kaopenai.New("test-model", provider.URL, "test-key"), Builder: builder,
			ResultParser: parser.NewResultParser(), Registry: reg, AuditStore: store, Logger: logr.Discard(),
			PhaseTools: DefaultPhaseToolMap(), MaxTurns: 4})
		signal := katypes.SignalContext{Name: "OOMKilled", Severity: "critical", Namespace: "prod", ResourceKind: "Deployment", ResourceName: "api", ResourceAPIVersion: "apps/v1"}

		result, err := inv.RunWorkflowDiscoveryFromRCA(context.Background(), signal,
			&katypes.InvestigationResult{RCASummary: "Memory exhausted", Confidence: 0.99}, nil, "rr-it-2498")

		Expect(err).NotTo(HaveOccurred())
		Expect(result.HumanReviewReason).To(Equal("no_matching_workflows"))
		Expect(result.WorkflowID).To(BeEmpty())
		Expect(contextTool.calls.Load()).To(BeZero(), "registered off-phase context tool must never run")
		Expect(actionsTool.calls.Load()).To(Equal(int32(1)), "allowed catalog dispatch must remain functional")
		Expect(externalCalls.Load()).To(Equal(int32(1)), "only the permitted action lookup may reach the external endpoint")
		requestMu.Lock()
		captured := append([]map[string]json.RawMessage(nil), requests...)
		requestMu.Unlock()
		Expect(captured).To(HaveLen(3))
		Expect(string(captured[0]["tools"])).NotTo(ContainSubstring(contextTool.name))
		Expect(string(captured[0]["tools"])).To(ContainSubstring(actionsTool.name))
		Expect(string(captured[1]["messages"])).To(ContainSubstring("not allowed in phase"))
		Expect(store.snapshot()).To(ContainElement(SatisfyAll(
			HaveField("EventOutcome", audit.OutcomeFailure),
			HaveField("Data", HaveKeyWithValue("tool_name", contextTool.name)),
			HaveField("CorrelationID", "rr-it-2498"),
		)))
	})
})

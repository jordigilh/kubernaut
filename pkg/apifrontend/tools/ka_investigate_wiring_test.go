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

package tools_test

import (
	"context"
	"fmt"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	prom "github.com/jordigilh/kubernaut/pkg/apifrontend/prometheus"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
	sharedK8s "github.com/jordigilh/kubernaut/pkg/shared/k8s"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("HandleInvestigationMCPWithRegistry — wiring audit (WIRE-C01/C03)", func() {

	Describe("WIRE-C01: investigate auto-RR path uses triager for severity", func() {
		It("UT-AF-WIRE-C01: RR created by investigate uses triaged severity, not default medium", func() {
			origTimeout := tools.AwaitSessionTimeout
			tools.AwaitSessionTimeout = 10 * time.Millisecond
			defer func() { tools.AwaitSessionTimeout = origTimeout }()

			eventCh := make(chan ka.InvestigationEvent)
			close(eventCh)

			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, args ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					Expect(args.RRID).NotTo(BeEmpty())
					return &ka.StartInvestigationResult{
						SessionID: "sess-wire-c01",
						Status:    "started",
						Events:    eventCh,
						Closer:    func() {},
					}, nil
				},
			}

			mockProm := &mockPromClientForWiring{
				alerts: []prom.Alert{
					{
						State: "firing",
						Labels: map[string]string{
							"alertname": "KubePodCrashLooping",
							"severity":  "critical",
							"namespace": "prod",
							"kind":      "Deployment",
							"name":      "web-app",
						},
					},
				},
			}

			triager := severity.NewTriager(mockProm, &noopLLMForWiring{}, severity.DefaultConfig(), logr.Discard())

			ctx, cancel := context.WithTimeout(
				auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
					Username: "alice",
					Groups:   []string{"sre"},
				}),
				10*time.Second,
			)
			defer cancel()

			tc := newTypedClientForInvestigate()
			result, err := tools.HandleInvestigationMCPWithRegistry(
				ctx, mockMCP, tc, "kubernaut-system",
				tools.InvestigateMCPArgs{
					APIVersion: "apps/v1",
					Namespace:  "prod",
					Kind:       "Deployment",
					Name:       "web-app",
				},
				nil, nil, nil, true, nil, "", nil, triager,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).NotTo(BeEmpty())

			created := verifyTypedRR(tc, "kubernaut-system", result.RRID)
			Expect(created.Spec.Severity).To(Equal("critical"), "investigate path should use triaged severity from Prometheus alert, not default 'medium'")
		})
	})

	Describe("WIRE-C02 / #2068: blocking-path severity-triage fallback RCA is marked Provisional", func() {
		It("UT-AF-2068-002: HandleInvestigationMCPWithRegistry's InvestigateMCPResult.RCA carries Provisional=true when the bridge produced no structured RCA", func() {
			// Same closed-empty-channel setup as WIRE-C01: bridgeEventsCollectSummary
			// observes zero events, so its own rca return is nil and only the
			// triager's severity is available -- exactly the "full investigation
			// pending" fallback at ka_investigate_mcp.go's rca==nil && rrSeverity!=""
			// site (the one whose Provisional=false value, before #2068, let a
			// severity-triage guess get cached by phase_guard.go and clobber a
			// genuinely investigated present_decision RCA -- see UT-AF-2068-001).
			origTimeout := tools.AwaitSessionTimeout
			tools.AwaitSessionTimeout = 10 * time.Millisecond
			defer func() { tools.AwaitSessionTimeout = origTimeout }()

			eventCh := make(chan ka.InvestigationEvent)
			close(eventCh)

			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, args ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return &ka.StartInvestigationResult{
						SessionID: "sess-wire-c02",
						Status:    "started",
						Events:    eventCh,
						Closer:    func() {},
					}, nil
				},
			}

			mockProm := &mockPromClientForWiring{
				alerts: []prom.Alert{
					{
						State: "firing",
						Labels: map[string]string{
							"alertname": "KubePodCrashLooping",
							"severity":  "warning",
							"namespace": "prod",
							"kind":      "Deployment",
							"name":      "web-app-2068",
						},
					},
				},
			}
			triager := severity.NewTriager(mockProm, &noopLLMForWiring{}, severity.DefaultConfig(), logr.Discard())

			ctx, cancel := context.WithTimeout(
				auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
					Username: "alice",
					Groups:   []string{"sre"},
				}),
				10*time.Second,
			)
			defer cancel()

			tc := newTypedClientForInvestigate()
			result, err := tools.HandleInvestigationMCPWithRegistry(
				ctx, mockMCP, tc, "kubernaut-system",
				tools.InvestigateMCPArgs{
					APIVersion: "apps/v1",
					Namespace:  "prod",
					Kind:       "Deployment",
					Name:       "web-app-2068",
				},
				nil, nil, nil, true, nil, "", nil, triager,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RCA).NotTo(BeNil(), "fallback must populate RCA when the bridge produced no structured result but severity triage succeeded")
			Expect(result.RCA.Severity).To(Equal("warning"), "fallback RCA must carry the triaged severity")
			Expect(result.RCA.Provisional).To(BeTrue(),
				"#2068: a severity-triage-only guess (no genuine KA investigation) must be marked Provisional so phase_guard.go's #2023 anti-fabrication cache never treats it as a verified finding")
		})
	})

	Describe("WIRE-C03: triager audit event emission", func() {
		It("UT-AF-WIRE-C03: triager constructed with WithAuditor emits severity event", func() {
			spy := &auditSpy{}
			mockProm := &mockPromClientForWiring{
				alerts: []prom.Alert{
					{
						State:  "firing",
						Labels: map[string]string{"alertname": "HighCPU", "severity": "warning", "namespace": "prod", "kind": "Deployment", "name": "api"},
					},
				},
			}

			triager := severity.NewTriager(mockProm, &noopLLMForWiring{}, severity.DefaultConfig(), logr.Discard(),
				severity.WithAuditor(spy))

			_, err := triager.Triage(context.Background(), severity.TriageInput{
				Namespace: "prod",
				Kind:      "Deployment",
				Name:      "api",
				Labels:    map[string]string{"namespace": "prod", "kind": "Deployment", "name": "api"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(spy.events).NotTo(BeEmpty(), "triager with WithAuditor must emit audit events")
		})
	})
})

var _ = Describe("kubectl_get — RESTMapper wiring (WIRE-C04)", func() {
	Describe("WIRE-C04: CRD kind resolution requires mapper", func() {
		It("UT-AF-WIRE-C04: custom CRD kind resolves when RESTMapper is provided", func() {
			mapper := apimeta.NewDefaultRESTMapper([]schema.GroupVersion{
				{Group: "route.openshift.io", Version: "v1"},
			})
			mapper.Add(schema.GroupVersionKind{
				Group: "route.openshift.io", Version: "v1", Kind: "Route",
			}, apimeta.RESTScopeNamespace)

			gvk, err := sharedK8s.ResolveGVKForKind(mapper, "Route")
			Expect(err).NotTo(HaveOccurred())
			Expect(gvk.Kind).To(Equal("Route"))
			Expect(gvk.Group).To(Equal("route.openshift.io"))
		})

		It("UT-AF-WIRE-C04-neg: custom CRD kind fails without mapper (nil)", func() {
			_, err := sharedK8s.ResolveGVKForKind(nil, "Route")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot resolve GVK"))
		})
	})
})

var _ = Describe("DS tool nil guard (WIRE-C05)", func() {
	Describe("WIRE-C05: DS handlers return error when client is nil", func() {
		It("UT-AF-WIRE-C05-list-workflows: HandleListWorkflows returns ErrDSUnavailable", func() {
			_, err := tools.HandleListWorkflows(context.Background(), nil, tools.ListWorkflowsArgs{})
			Expect(err).To(MatchError(tools.ErrDSUnavailable))
		})

		It("UT-AF-WIRE-C05-history: HandleGetRemediationHistory returns ErrDSUnavailable", func() {
			_, err := tools.HandleGetRemediationHistory(context.Background(), nil, tools.GetRemediationHistoryArgs{})
			Expect(err).To(MatchError(tools.ErrDSUnavailable))
		})

		It("UT-AF-WIRE-C05-effectiveness: HandleGetEffectiveness returns ErrDSUnavailable", func() {
			_, err := tools.HandleGetEffectiveness(context.Background(), nil, tools.GetEffectivenessArgs{})
			Expect(err).To(MatchError(tools.ErrDSUnavailable))
		})

		It("UT-AF-WIRE-C05-audit-trail: HandleGetAuditTrail returns ErrDSUnavailable", func() {
			_, err := tools.HandleGetAuditTrail(context.Background(), nil, tools.GetAuditTrailArgs{})
			Expect(err).To(MatchError(tools.ErrDSUnavailable))
		})
	})
})

var _ = Describe("HandleInvestigationMCPWithRegistry — session_active structured result (WIRE-SESSION)", func() {

	Describe("WIRE-SESSION: session_active error from StartInvestigation returns structured result", func() {
		It("UT-AF-WIRE-SESSION-001: returns InvestigateMCPResult with status=session_active and err=nil", func() {
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return nil, fmt.Errorf("kubernaut_investigate start_autonomous: session_active: You already have an active session for this investigation; use action=reconnect to rejoin (map[driver:admin session_id:sess-123])")
				},
			}

			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "admin",
				Groups:   []string{"sre"},
			})

			result, err := tools.HandleInvestigationMCPWithRegistry(
				ctx, mockMCP, nil, "kubernaut-system",
				tools.InvestigateMCPArgs{RRID: "rr-session-001"},
				nil, nil, nil, true, nil, "admin", nil, nil,
			)
			Expect(err).NotTo(HaveOccurred(), "session_active should not propagate as Go error")
			Expect(result.Status).To(Equal("session_active"))
			Expect(result.RRID).To(Equal("rr-session-001"))
		})

		It("UT-AF-WIRE-SESSION-002: structured result Error field contains driver name", func() {
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return nil, fmt.Errorf("kubernaut_investigate start_autonomous: session_active: Investigation is being driven by another user (map[driver:bob@example.com])")
				},
			}

			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "alice",
				Groups:   []string{"sre"},
			})

			result, err := tools.HandleInvestigationMCPWithRegistry(
				ctx, mockMCP, nil, "kubernaut-system",
				tools.InvestigateMCPArgs{RRID: "rr-session-002"},
				nil, nil, nil, true, nil, "alice", nil, nil,
			)
			Expect(err).NotTo(HaveOccurred(), "session_active should not propagate as Go error")
			Expect(result.Error).NotTo(BeEmpty(), "Error field must provide actionable guidance")
			Expect(result.Error).To(ContainSubstring("bob@example.com"), "Error field must include the driver name")
		})

		It("UT-AF-WIRE-SESSION-003: session_active fallback investigation_summary artifact carries non-empty causal_chain (#1922)", func() {
			origTimeout := tools.AwaitSessionTimeout
			tools.AwaitSessionTimeout = 10 * time.Millisecond
			defer func() { tools.AwaitSessionTimeout = origTimeout }()

			mockProm := &mockPromClientForWiring{
				alerts: []prom.Alert{
					{
						State: "firing",
						Labels: map[string]string{
							"alertname": "KubePodCrashLooping",
							"severity":  "critical",
							"namespace": "prod",
							"kind":      "Deployment",
							"name":      "web-app-1922",
						},
					},
				},
			}
			triager := severity.NewTriager(mockProm, &noopLLMForWiring{}, severity.DefaultConfig(), logr.Discard())

			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return nil, fmt.Errorf("kubernaut_investigate start_autonomous: session_active: You already have an active session for this investigation; use action=reconnect to rejoin (map[driver:admin session_id:sess-1922])")
				},
			}

			queue := &bridgeQueue{}
			ctx := launcher.WithEventBridge(
				auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
					Username: "bob",
					Groups:   []string{"sre"},
				}),
				queue, "task-1922-session", "ctx-1922-session", nil,
			)

			tc := newTypedClientForInvestigate()
			result, err := tools.HandleInvestigationMCPWithRegistry(
				ctx, mockMCP, tc, "kubernaut-system",
				tools.InvestigateMCPArgs{
					APIVersion: "apps/v1",
					Namespace:  "prod",
					Kind:       "Deployment",
					Name:       "web-app-1922",
				},
				nil, nil, nil, true, nil, "bob", nil, triager,
			)
			Expect(err).NotTo(HaveOccurred(), "session_active should not propagate as Go error")
			Expect(result.Status).To(Equal("session_active"))

			var artifactEvt *a2a.TaskArtifactUpdateEvent
			for _, evt := range queue.Events() {
				if art, ok := evt.(*a2a.TaskArtifactUpdateEvent); ok {
					if schema, _ := art.Artifact.Metadata["schema"].(string); schema == "investigation_summary" {
						artifactEvt = art
						break
					}
				}
			}
			Expect(artifactEvt).NotTo(BeNil(), "session_active must emit an investigation_summary fallback artifact")

			dp, ok := artifactEvt.Artifact.Parts[0].(a2a.DataPart)
			Expect(ok).To(BeTrue(), "artifact must carry a DataPart")
			rcaData, ok := dp.Data["rca"].(map[string]any)
			Expect(ok).To(BeTrue(), "artifact data must include an rca object")
			causalChain, ok := rcaData["causal_chain"].([]string)
			Expect(ok).To(BeTrue(), "AU-3/SI-10: rca.causal_chain must be present so the Console's hasRCAData guard renders the RCA card (#1922)")
			Expect(causalChain).NotTo(BeEmpty(), "AU-3/SI-10: rca.causal_chain must be non-empty so the Console's hasRCAData guard renders the RCA card (#1922)")
		})
	})
})

var _ = Describe("mcpClient nil guard (WIRE-W04)", func() {
	Describe("WIRE-W04: HandleInvestigationMCPWithRegistry rejects nil mcpClient", func() {
		It("UT-AF-WIRE-W04: returns error when mcpClient is nil", func() {
			_, err := tools.HandleInvestigationMCPWithRegistry(
				context.Background(), nil, nil, "ns",
				tools.InvestigateMCPArgs{RRID: "rr-test"},
				nil, nil, nil, false, nil, "", nil, nil,
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("KA MCP client unavailable"))
		})
	})
})


type mockPromClientForWiring struct {
	alerts []prom.Alert
}

func (m *mockPromClientForWiring) GetAlerts(_ context.Context) ([]prom.Alert, error) {
	return m.alerts, nil
}

func (m *mockPromClientForWiring) GetRules(_ context.Context) ([]prom.RuleGroup, error) {
	return nil, nil
}

func (m *mockPromClientForWiring) InstantQuery(_ context.Context, _ string) (*prom.QueryResult, error) {
	return &prom.QueryResult{}, nil
}

type noopLLMForWiring struct{}

func (n *noopLLMForWiring) TriageWithRules(_ context.Context, _ []prom.Rule, _ severity.TriageInput) (severity.TriageResult, error) {
	return severity.TriageResult{Severity: "warning", Source: severity.SourceLLMTriage}, nil
}

type auditSpy struct {
	events []*audit.Event
}

func (a *auditSpy) Emit(_ context.Context, e *audit.Event) {
	a.events = append(a.events, e)
}

// =============================================================================
// Issue #1407: Progressive RCA Emission — IT proving wiring through production path
// =============================================================================

var _ = Describe("Progressive RCA Emission Wiring — #1407", func() {

	It("IT-AF-1407-001: SI-4 early RCA decision event flows through EventBridge on investigation complete", func() {
		rcaJSON := []byte(`{"severity":"critical","confidence":0.92,"causal_chain":["Memory leak","OOMKill"],"target":"Deployment/worker in production","rca_summary":"OOMKill caused by memory leak","total_llm_turns":17,"total_tool_calls":19}`)
		eventCh := make(chan ka.InvestigationEvent, 5)
		eventCh <- ka.InvestigationEvent{
			Type: ka.EventTypeComplete,
			Data: rcaJSON,
		}
		close(eventCh)

		mockMCP := &ka.MockMCPClient{
			StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
				return &ka.StartInvestigationResult{
					SessionID: "sess-1407-it",
					Status:    "started",
					Events:    eventCh,
					Closer:    func() {},
				}, nil
			},
		}

		queue := &bridgeQueue{}
		ctx := launcher.WithEventBridge(
			auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "alice",
				Groups:   []string{"sre"},
			}),
			queue, "task-1407-it", "ctx-1407-it", nil,
		)
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		result, err := tools.HandleInvestigationMCPWithRegistry(
			ctx, mockMCP, nil, "",
			tools.InvestigateMCPArgs{RRID: "rr-1407-test"},
			nil, nil, nil, true, nil, "alice", nil, nil,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RCA).NotTo(BeNil(), "RCA must be populated from investigation complete event")
		Expect(result.RCA.Severity).To(Equal("critical"))

		allEvents := queue.Events()
		var decisionFound bool
		for _, evt := range allEvents {
			statusEvt, ok := evt.(*a2a.TaskStatusUpdateEvent)
			if !ok {
				continue
			}
			if statusEvt.Metadata == nil {
				continue
			}
			metaType, _ := statusEvt.Metadata["type"].(string)
			if metaType == launcher.MetaTypeDecision {
				decisionFound = true
				Expect(statusEvt.Metadata["schema"]).To(Equal("early_rca"),
					"SI-4: schema must classify early RCA for audit differentiation")
				Expect(statusEvt.Metadata["schema_version"]).To(Equal("1.0"))
				tp, ok := statusEvt.Status.Message.Parts[0].(a2a.TextPart)
				Expect(ok).To(BeTrue())
				Expect(tp.Text).To(ContainSubstring("critical"),
					"AU-3: severity must appear in structured text for audit trail")
				break
			}
		}
		Expect(decisionFound).To(BeTrue(),
			"IT-AF-1407-001: early RCA decision status-update must flow through production EventBridge wiring")
	})
})

// =============================================================================
// Issue #1922 (release/v1.5 backport, #1928): session_active fallback
// investigation_summary artifact content
// =============================================================================

var _ = Describe("Fallback Investigation Artifact Content — #1922", func() {

	It("UT-AF-1922-001: AU-3 empty causal_chain is seeded with a truthful placeholder, never fabricated findings", func() {
		queue := &bridgeQueue{}
		ctx := launcher.WithEventBridge(context.Background(), queue, "task-1922-001", "ctx-1922-001", nil)

		rca := &tools.InvestigateRCA{
			Severity:   "critical",
			Confidence: 0.6,
			RCASummary: "Severity assessed from resource metadata (investigation in progress by admin)",
		}
		tools.EmitFallbackInvestigationArtifact(ctx, rca, "rr-1922-001")

		var artifactEvt *a2a.TaskArtifactUpdateEvent
		for _, evt := range queue.Events() {
			if art, ok := evt.(*a2a.TaskArtifactUpdateEvent); ok {
				artifactEvt = art
				break
			}
		}
		Expect(artifactEvt).NotTo(BeNil(), "fallback artifact must be emitted")

		dp, ok := artifactEvt.Artifact.Parts[0].(a2a.DataPart)
		Expect(ok).To(BeTrue(), "artifact must carry a DataPart")
		rcaData, ok := dp.Data["rca"].(map[string]any)
		Expect(ok).To(BeTrue(), "artifact data must include an rca object")
		causalChain, ok := rcaData["causal_chain"].([]string)
		Expect(ok).To(BeTrue(), "rca.causal_chain must be present")
		Expect(causalChain).NotTo(BeEmpty(),
			"empty CausalChain input must be seeded with a placeholder so the Console's hasRCAData guard renders the RCA card (#1922)")
		Expect(causalChain[0]).NotTo(ContainSubstring("OOMKill"),
			"AU-3: placeholder must not fabricate specific findings that were never observed")
		Expect(causalChain[0]).To(SatisfyAny(
			ContainSubstring("progress"),
			ContainSubstring("pending"),
		), "AU-3: placeholder must truthfully describe investigation status, not fabricate a root cause")
	})

	It("UT-AF-1922-002: SI-10 real causal_chain/tool_calls_count pass through unchanged and shape matches present_decision's RCAData", func() {
		queue := &bridgeQueue{}
		ctx := launcher.WithEventBridge(context.Background(), queue, "task-1922-002", "ctx-1922-002", nil)

		rca := &tools.InvestigateRCA{
			Severity:       "critical",
			Confidence:     0.92,
			CausalChain:    []string{"Memory leak", "OOMKill"},
			Target:         "Deployment/worker in production",
			RCASummary:     "OOMKill caused by memory leak",
			TotalLLMTurns:  17,
			TotalToolCalls: 19,
		}
		tools.EmitFallbackInvestigationArtifact(ctx, rca, "rr-1922-002")

		var artifactEvt *a2a.TaskArtifactUpdateEvent
		for _, evt := range queue.Events() {
			if art, ok := evt.(*a2a.TaskArtifactUpdateEvent); ok {
				artifactEvt = art
				break
			}
		}
		Expect(artifactEvt).NotTo(BeNil(), "fallback artifact must be emitted")

		dp, ok := artifactEvt.Artifact.Parts[0].(a2a.DataPart)
		Expect(ok).To(BeTrue(), "artifact must carry a DataPart")
		rcaData, ok := dp.Data["rca"].(map[string]any)
		Expect(ok).To(BeTrue(), "artifact data must include an rca object")

		causalChain, ok := rcaData["causal_chain"].([]string)
		Expect(ok).To(BeTrue(), "rca.causal_chain must be present")
		Expect(causalChain).To(ConsistOf("Memory leak", "OOMKill"),
			"real CausalChain must pass through unchanged, not be overwritten by a placeholder")

		Expect(rcaData).To(HaveKey("tool_calls_count"),
			"SI-10: shape must match present_decision's RCAData field names")
		Expect(rcaData["tool_calls_count"]).To(Equal(19))
		Expect(rcaData).To(HaveKey("llm_turns"),
			"SI-10: shape must match present_decision's RCAData field names")
		Expect(rcaData["llm_turns"]).To(Equal(17))
	})
})

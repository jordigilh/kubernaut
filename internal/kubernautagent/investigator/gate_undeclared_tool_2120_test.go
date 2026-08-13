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

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// #2120/#2121 (BR-AI-2120, FedRAMP AU-3): sameKindValidationGate and
// apiVersionValidationGate build their gate-retry LLM call with a
// submit-result-only tool declaration, but the model frequently ignores that
// constraint and calls an undeclared tool anyway (e.g. kubectl_describe) — a
// live-LLM spike against the real production Claude/Vertex client measured
// this in 10/10 trials for the same-kind gate's correction prompt. Because
// both gates only recognized a `submit_result` tool call or non-empty
// message text as "the retry answered", an undeclared-tool response fell
// through to the same fallback path as a genuinely empty response: the
// correction is silently dropped and the pre-retry (possibly wrong) result
// is kept, with no record in the audit trail of *why* the retry failed.
//
// The same spike showed a second attempt — replaying the wrong call as a
// synthetic tool_result error plus an explicit reminder — recovered the
// correct submit_result call in 10/10 trials.
//
// v1.6 clone of the release/v1.5.6 fix (#2120 -> #2121 tracking issue).
// These specs exercise both gates through the real Investigate() production
// path (mirroring this package's established convention).
var _ = Describe("#2120/#2121: gate retries must recover from (or account for) an undeclared tool call", func() {

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

	sameKindSignal := katypes.SignalContext{
		ResourceKind: "Pod",
		ResourceName: "api-server-xyz",
		Name:         "api-server-xyz",
		Namespace:    "production",
		Severity:     "critical",
		Message:      "Pod OOMKilled repeatedly",
	}

	apiVersionSignal := katypes.SignalContext{
		ResourceKind: "Deployment",
		ResourceName: "etcd-operator-xyz",
		Name:         "etcd-operator-xyz",
		Namespace:    "demo-operator",
		Severity:     "critical",
		Message:      "RBAC denial on wrong API group",
	}

	// undeclaredToolResp simulates the model calling a tool other than
	// submit_result during a gate retry — the #2120 failure mode. Real
	// responses carry no message text on a pure tool-calling turn.
	undeclaredToolResp := func(toolName string) llm.ChatResponse {
		return llm.ChatResponse{
			Message: llm.Message{Role: "assistant", Content: ""},
			ToolCalls: []llm.ToolCall{
				{ID: "tc_undeclared", Name: toolName, Arguments: `{"name":"api-server","namespace":"production"}`},
			},
		}
	}

	Describe("UT-KA-2120-001 (regression): sameKindValidationGate recovers after one undeclared-tool retry", func() {
		It("must accept the reminder-retry's result and record retry_outcome=resolved_after_other_tool_retry (BR-AI-2120 AC1)", func() {
			mockClient.responses = []llm.ChatResponse{
				// Turn 1: RCA submit — same-kind gate fires (target.Kind == signal.ResourceKind == "Pod").
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Pod OOMKilled",
					"confidence":0.85,
					"remediation_target":{"kind":"Pod","name":"api-server-xyz","namespace":"production"}
				}`}},
				// Gate retry attempt 1: LLM calls an undeclared tool instead of submit_result (#2120).
				undeclaredToolResp("kubectl_describe"),
				// Gate retry attempt 2 (after reminder): LLM correctly re-targets the parent Deployment.
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Deployment memory limit too low",
					"confidence":0.85,
					"remediation_target":{"kind":"Deployment","name":"api-server","namespace":"production","api_version":"apps/v1"}
				}`}},
				gateWfToolResp(`{"workflow_id":"increase-memory-limit","confidence":0.9}`),
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})

			result, err := inv.Investigate(context.Background(), sameKindSignal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RemediationTarget.Kind).To(Equal("Deployment"),
				"UT-KA-2120-001: final result must reflect the reminder retry's data, not the original Pod target")
			Expect(mockClient.calls).To(HaveLen(4),
				"UT-KA-2120-001: expected RCA + first gate retry + reminder retry + workflow-selection LLM calls")

			gateEvents := auditStore.eventsByAction(audit.ActionSameKindGate)
			Expect(gateEvents).NotTo(BeEmpty(), "UT-KA-2120-001: gate must emit an audit event")
			ev := gateEvents[0]
			Expect(ev.Data["retry_outcome"]).To(Equal("resolved_after_other_tool_retry"),
				"UT-KA-2120-001: audit trail must distinguish a reminder-assisted recovery from a first-try resolution")
			Expect(ev.EventOutcome).To(Equal(audit.OutcomeSuccess),
				"UT-KA-2120-001: a recovered retry is a successful gate outcome")
		})
	})

	Describe("UT-KA-2120-002: sameKindValidationGate keeps the original result when the undeclared tool call persists", func() {
		It("must keep the pre-retry result and record retry_outcome=llm_requested_other_tool (BR-AI-2120 AC2)", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Pod OOMKilled",
					"confidence":0.85,
					"remediation_target":{"kind":"Pod","name":"api-server-xyz","namespace":"production"}
				}`}},
				// Both the initial retry and the reminder retry call an undeclared tool.
				undeclaredToolResp("kubectl_describe"),
				undeclaredToolResp("kubectl_describe"),
				gateWfToolResp(`{"workflow_id":"restart-pod","confidence":0.8}`),
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})

			result, err := inv.Investigate(context.Background(), sameKindSignal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RemediationTarget.Kind).To(Equal("Pod"),
				"UT-KA-2120-002: original (pre-retry) result must be kept when the reminder retry also fails")
			Expect(mockClient.calls).To(HaveLen(4),
				"UT-KA-2120-002: expected RCA + first gate retry + reminder retry + workflow-selection LLM calls")

			gateEvents := auditStore.eventsByAction(audit.ActionSameKindGate)
			Expect(gateEvents).NotTo(BeEmpty(), "UT-KA-2120-002: gate must emit an audit event")
			ev := gateEvents[0]
			Expect(ev.Data["retry_outcome"]).To(Equal("llm_requested_other_tool"),
				"UT-KA-2120-002: audit trail must record that the LLM never answered with submit_result")
			Expect(ev.EventOutcome).To(Equal(audit.OutcomeFailure),
				"UT-KA-2120-002: an unresolved retry must be a failed gate outcome, not the hardcoded success of pre-#2120 code")
		})
	})

	Describe("UT-KA-2120-003: apiVersionValidationGate recovers after one undeclared-tool retry", func() {
		It("must accept the reminder-retry's api_version and record retry_outcome=resolved_after_other_tool_retry (BR-AI-2120 AC1)", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription etcd needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator"}
				}`}},
				// Gate retry attempt 1: LLM calls an undeclared tool instead of submit_result (#2120).
				undeclaredToolResp("kubectl_describe"),
				// Gate retry attempt 2 (after reminder): LLM correctly provides api_version.
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription etcd needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator","api_version":"operators.coreos.com/v1alpha1"}
				}`}},
				gateWfToolResp(`{"workflow_id":"restart-sub","confidence":0.9}`),
			}

			resolver := investigator.NewMapperScopeResolver(newAmbiguousSubscriptionMapper())
			// newTestInvestigator (not investigator.New directly): this
			// scenario proceeds all the way to workflow selection, which
			// (#1677 hardening, DD-WORKFLOW-019) fails closed to human
			// review when no CatalogFetcher is configured -- orthogonal to
			// the #2120 behavior under test here, so use the package's
			// permissive stub covering the "restart-sub" fixture ID below.
			inv := newTestInvestigator(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools, ScopeResolver: resolver,
			})

			result, err := inv.Investigate(context.Background(), apiVersionSignal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RemediationTarget.APIVersion).To(Equal("operators.coreos.com/v1alpha1"),
				"UT-KA-2120-003: final result must reflect the reminder retry's api_version")
			Expect(result.HumanReviewNeeded).To(BeFalse(),
				"UT-KA-2120-003: a recovered retry must not trigger human review")
			Expect(mockClient.calls).To(HaveLen(4),
				"UT-KA-2120-003: expected RCA + first gate retry + reminder retry + workflow-selection LLM calls")

			gateEvents := auditStore.eventsByAction(audit.ActionAPIVersionGate)
			Expect(gateEvents).NotTo(BeEmpty(), "UT-KA-2120-003: gate must emit an audit event")
			ev := gateEvents[0]
			Expect(ev.Data["retry_outcome"]).To(Equal("resolved_after_other_tool_retry"),
				"UT-KA-2120-003: audit trail must distinguish a reminder-assisted recovery from a first-try resolution")
			Expect(ev.EventOutcome).To(Equal(audit.OutcomeSuccess),
				"UT-KA-2120-003: a recovered retry is a successful gate outcome")
		})
	})

	Describe("UT-KA-2120-004: apiVersionValidationGate triggers human review when the undeclared tool call persists", func() {
		It("must trigger human review and record retry_outcome=llm_requested_other_tool (BR-AI-2120 AC2 Security)", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription etcd needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator"}
				}`}},
				undeclaredToolResp("kubectl_describe"),
				undeclaredToolResp("kubectl_describe"),
			}

			resolver := investigator.NewMapperScopeResolver(newAmbiguousSubscriptionMapper())
			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools, ScopeResolver: resolver,
			})

			result, err := inv.Investigate(context.Background(), apiVersionSignal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.HumanReviewNeeded).To(BeTrue(),
				"UT-KA-2120-004: an unresolved ambiguous-kind retry must still trigger human review "+
					"to prevent incorrect RBAC grants, exactly as a plain empty/unparseable retry would")
			Expect(result.HumanReviewReason).To(Equal(katypes.HumanReviewReasonRCAIncomplete),
				"UT-KA-2120-004: reason must be rca_incomplete, matching the existing exhaustion convention")

			gateEvents := auditStore.eventsByAction(audit.ActionAPIVersionGate)
			Expect(gateEvents).NotTo(BeEmpty(), "UT-KA-2120-004: gate must emit an audit event")
			ev := gateEvents[0]
			Expect(ev.Data["retry_outcome"]).To(Equal("llm_requested_other_tool"),
				"UT-KA-2120-004: audit trail must record that the LLM never answered with submit_result")
			Expect(ev.EventOutcome).To(Equal(audit.OutcomeFailure),
				"UT-KA-2120-004: gate exhaustion must be a failed outcome, not the hardcoded success of pre-#2120 code")
		})
	})

	// UT-KA-2120-005 is the regression test for the secondary finding: both
	// gates previously called audit.StoreBestEffort(gateEvent) BEFORE setting
	// gateEvent.Data["retry_outcome"]/EventOutcome, so a production audit
	// store — which serializes Data into an outbound request synchronously
	// at call time (internal/kubernautagent/audit/ds_store.go's
	// DSAuditStore/BufferedDSAuditStore) — never actually persisted those
	// fields. gateRecordingAuditStore previously masked this by capturing
	// the raw *audit.AuditEvent pointer, so a later in-memory read always
	// reflected the mutation regardless of when StoreAudit was called. Once
	// fixed to snapshot event.Data at call time (matching real production
	// stores), this test would fail against the pre-#2120 store-then-mutate
	// code and passes once the store call is deferred until every Data
	// mutation has happened (BR-AI-2120 AC3, FedRAMP AU-3).
	Describe("UT-KA-2120-005 (regression): retry_outcome/EventOutcome are captured at StoreAudit call time, not just in a later in-memory read", func() {
		It("must have set retry_outcome=resolved and EventOutcome=success on sameKindValidationGate's audit event before storing it", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Pod OOMKilled",
					"confidence":0.85,
					"remediation_target":{"kind":"Pod","name":"api-server-xyz","namespace":"production"}
				}`}},
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Deployment memory limit too low",
					"confidence":0.85,
					"remediation_target":{"kind":"Deployment","name":"api-server","namespace":"production","api_version":"apps/v1"}
				}`}},
				gateWfToolResp(`{"workflow_id":"increase-memory-limit","confidence":0.9}`),
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})

			result, err := inv.Investigate(context.Background(), sameKindSignal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			gateEvents := auditStore.eventsByAction(audit.ActionSameKindGate)
			Expect(gateEvents).NotTo(BeEmpty(), "UT-KA-2120-005: gate must emit an audit event")
			ev := gateEvents[0]
			Expect(ev.Data["retry_outcome"]).To(Equal("resolved"),
				"UT-KA-2120-005: retry_outcome must be captured by StoreAudit at call time, not mutated afterward")
			Expect(ev.EventOutcome).To(Equal(audit.OutcomeSuccess),
				"UT-KA-2120-005: EventOutcome must be captured by StoreAudit at call time, not the pre-#2120 "+
					"unconditional hardcoded success")
		})

		It("must have set retry_outcome=exhausted and EventOutcome=failure on apiVersionValidationGate's audit event before storing it", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription etcd needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator"}
				}`}},
				// Gate retry still omits api_version -> exhaustion.
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

			result, err := inv.Investigate(context.Background(), apiVersionSignal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			gateEvents := auditStore.eventsByAction(audit.ActionAPIVersionGate)
			Expect(gateEvents).NotTo(BeEmpty(), "UT-KA-2120-005: gate must emit an audit event")
			ev := gateEvents[0]
			Expect(ev.Data["retry_outcome"]).To(Equal("exhausted"),
				"UT-KA-2120-005: retry_outcome must be captured by StoreAudit at call time — this is the "+
					"pre-existing dead-mutation bug (#1044 lines 336/354) this PR fixes alongside #2120")
			Expect(ev.EventOutcome).To(Equal(audit.OutcomeFailure),
				"UT-KA-2120-005: EventOutcome must reflect the actual (failed) outcome, not the pre-#2120 "+
					"unconditional hardcoded success")
		})
	})
})

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
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// #2119 (BR-AI-2119, FedRAMP SI-11/AU-3): sameKindValidationGate's retry
// confirms the LLM's original remediation target is correct, but the LLM's
// tool call arguments for that confirmation frequently omit `confidence`
// entirely (the model treats the correction turn as a yes/no confirmation,
// not a full RCA restatement). Because the gate accepts retryResult.Confidence
// as-is whenever RemediationTarget.Kind is non-empty, this silently drops a
// previously-computed, real confidence score down to 0.0 -- corrupting the
// audit-visible RCA record (AU-3) and, downstream, any confidence-gated
// automation (BR-AI-2119) that trusts InvestigationResult.Confidence.
//
// These specs exercise the gate through the real Investigate() production
// path (mirroring apiversion_gate_test.go's/gate_history_propagation_1936_
// test.go's established pattern in this package) rather than calling the
// unexported gate function directly. newTestInvestigator (apiversion_gate_
// test.go) is used instead of investigator.New to satisfy #1677's fail-closed
// catalog-validation hardening (main-only; not present when this fix
// originated on release/v1.5 as #2118) for the workflow-selection phase that
// follows the gate in each scenario.
var _ = Describe("#2119: sameKindValidationGate must not silently drop RCA confidence on retry", func() {

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
		ResourceKind: "Node",
		ResourceName: "ip-10-0-1-23",
		Name:         "ip-10-0-1-23",
		Namespace:    "",
		Severity:     "critical",
		Message:      "Node DiskPressure",
	}

	Describe("UT-KA-2119-001 (regression): retry confirms the same target but omits confidence", func() {
		It("must backfill the pre-retry confidence instead of silently zeroing it out (BR-AI-2119 AC1)", func() {
			mockClient.responses = []llm.ChatResponse{
				// Turn 1: RCA submit — same-kind gate fires (target.Kind == signal.ResourceKind == "Node").
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Node under DiskPressure due to accumulated container logs",
					"confidence":0.88,
					"remediation_target":{"kind":"Node","name":"ip-10-0-1-23"}
				}`}},
				// Turn 2: gate retry — LLM re-confirms the same target but the tool
				// call arguments omit confidence entirely (the observed #2119 failure mode).
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Confirmed: Node itself is the correct target, log accumulation is host-level",
					"remediation_target":{"kind":"Node","name":"ip-10-0-1-23"},
					"due_diligence":{"target_accuracy":"host-level log rotation misconfiguration confirmed via kubectl describe node"}
				}`}},
				// Phase 3 (workflow selection) also omits confidence here -- a
				// realistic case, since the LLM often treats workflow selection
				// as a separate concern from the RCA confidence score. This
				// isolates whether Phase 1's own post-gate confidence (what
				// BuildPhase1Context/MergePhase1Fallbacks propagate as p1) was
				// corrupted by the gate retry, independent of Phase 3's own
				// (unrelated) confidence value.
				gateWfToolResp(`{"workflow_id":"rotate-node-logs"}`),
			}

			inv := newTestInvestigator(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RemediationTarget.Kind).To(Equal("Node"),
				"UT-KA-2119-001: retry confirming the same target must be accepted")
			Expect(result.Confidence).To(BeNumerically("==", 0.88),
				"UT-KA-2119-001: the pre-retry confidence (0.88) must be backfilled when the retry response omits "+
					"confidence, not silently dropped to 0 (#2119)")
		})
	})

	Describe("UT-KA-2119-002: retry provides its own genuine, non-zero confidence", func() {
		It("must preserve the retry's own confidence unchanged, not overwrite it with the pre-retry value (BR-AI-2119 AC2)", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Node under DiskPressure",
					"confidence":0.88,
					"remediation_target":{"kind":"Node","name":"ip-10-0-1-23"}
				}`}},
				// Gate retry: LLM re-evaluates and returns a deliberately lower,
				// genuinely-recomputed confidence for the same target.
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Re-confirmed Node as target, but evidence is less conclusive than initially assessed",
					"confidence":0.55,
					"remediation_target":{"kind":"Node","name":"ip-10-0-1-23"}
				}`}},
				gateWfToolResp(`{"workflow_id":"rotate-node-logs"}`),
			}

			inv := newTestInvestigator(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Confidence).To(BeNumerically("==", 0.55),
				"UT-KA-2119-002: a genuine, non-zero retry confidence must be preserved as-is -- "+
					"the backfill guard must not clobber a real re-assessment")
		})
	})

	Describe("UT-KA-2119-003: original pre-retry confidence was itself never set (0)", func() {
		It("must leave confidence at 0 rather than backfilling from a meaningless zero value (BR-AI-2119 edge case)", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Node under DiskPressure",
					"remediation_target":{"kind":"Node","name":"ip-10-0-1-23"}
				}`}},
				// Gate retry also omits confidence.
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Confirmed Node as target",
					"remediation_target":{"kind":"Node","name":"ip-10-0-1-23"}
				}`}},
				gateWfToolResp(`{"workflow_id":"rotate-node-logs"}`),
			}

			inv := newTestInvestigator(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Confidence).To(BeNumerically("==", 0),
				"UT-KA-2119-003: backfill must only trigger when the pre-retry confidence was a real (>0) value -- "+
					"it must not fabricate a non-zero confidence out of another zero value")
		})
	})

	Describe("UT-KA-2119-004: correction message must instruct a full RCA restatement, not a bare confirmation", func() {
		It("must ask the LLM to restate confidence alongside the target (BR-AI-2119 AC3, defense-in-depth)", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Node under DiskPressure",
					"confidence":0.88,
					"remediation_target":{"kind":"Node","name":"ip-10-0-1-23"}
				}`}},
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Confirmed Node as target",
					"confidence":0.88,
					"remediation_target":{"kind":"Node","name":"ip-10-0-1-23"}
				}`}},
				gateWfToolResp(`{"workflow_id":"rotate-node-logs","confidence":0.9}`),
			}

			inv := newTestInvestigator(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})

			_, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())

			Expect(mockClient.calls).To(HaveLen(3),
				"UT-KA-2119-004: expected RCA submit + gate retry + workflow-selection LLM calls")
			gateCall := mockClient.calls[1]
			lastMsg := gateCall.Messages[len(gateCall.Messages)-1]
			Expect(lastMsg.Content).To(ContainSubstring("confidence"),
				"UT-KA-2119-004: correction message must explicitly instruct the LLM to restate its confidence, "+
					"not just confirm/deny the target (#2119)")
			Expect(lastMsg.Content).To(SatisfyAny(
				ContainSubstring("full"),
				ContainSubstring("restate"),
				ContainSubstring("entire"),
			), "UT-KA-2119-004: correction message must instruct a full RCA restatement, not a bare yes/no confirmation")
		})
	})
})

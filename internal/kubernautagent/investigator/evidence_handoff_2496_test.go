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
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/sanitization"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// This additional operator policy actually rejects a restricted evidence label.
// It does not replace or mock the mandatory Secret/credential/injection stages:
// every test uses the real production sanitization pipeline before this stage.
type restrictedEvidencePolicy2496 struct{}

func (restrictedEvidencePolicy2496) Name() string { return "restricted-evidence-policy" }

func (restrictedEvidencePolicy2496) Sanitize(_ context.Context, input string) (string, error) {
	if strings.Contains(input, "restricted-evidence-2496") {
		return "", fmt.Errorf("operator policy rejected restricted evidence")
	}
	return input, nil
}

// DD-KA-2496 / BR-KA-016/211, SI-10/AC-4: UT proves safety of the
// pre-transform capture; cmd/kubernautagent's IT-KA-2496-007 proves dispatch.
var _ = Describe("#2496 evidence handoff observation privacy", func() {
	It("UT-KA-2496-AUDIT-006: failed history acquisition survives projection and blocks freezing", func() {
		var enrichmentResult enrichment.EnrichmentResult
		Expect(json.Unmarshal([]byte(`{"history_unavailable":true}`), &enrichmentResult)).To(Succeed())
		inv := New(Config{Logger: logr.Discard()})
		frozen, err := inv.freezeEvidenceHandoff(context.Background(), handoffInput{
			Signal: katypes.SignalContext{ResourceKind: "Pod", ResourceName: "api", Namespace: "prod"}, EnrichData: toPromptEnrichment(&enrichmentResult),
		}, "rr-history-unavailable")
		Expect(err).To(MatchError(ContainSubstring("history")))
		Expect(frozen).To(BeNil())
	})
	DescribeTable("UT-KA-2496-AUDIT-001: canonical cluster-scoped history remains in the frozen repeat scope",
		func(kind, reason string) {
			inv := New(Config{Logger: logr.Discard()})
			score := .5
			input := handoffInput{Signal: katypes.SignalContext{ResourceKind: kind, ResourceName: "target", ResourceAPIVersion: "v1"},
				EnrichData: &prompt.EnrichmentData{HistoryResult: &enrichment.RemediationHistoryResult{
					TargetResource: "/" + kind + "/target",
					Tier1:          []enrichment.Tier1Entry{{RemediationUID: "prior-cluster", WorkflowID: "same-id", WorkflowName: "catalog-name", AssessmentReason: reason, EffectivenessScore: &score}},
				}}}
			frozen, err := inv.freezeEvidenceHandoff(context.Background(), input, "cluster-history-audit")
			Expect(err).NotTo(HaveOccurred())
			Expect(frozen.repeat.CompletedPriorWorkflows).To(HaveKeyWithValue("same-id", []string{"prior-cluster"}))
			Expect(frozen.serialized).To(ContainSubstring("prior-cluster"))
			input.EnrichData.HistoryResult.TargetResource = "/" + kind + "/other"
			other, err := inv.freezeEvidenceHandoff(context.Background(), input, "wrong-cluster-history-audit")
			Expect(err).NotTo(HaveOccurred())
			Expect(other.repeat.CompletedPriorWorkflows).NotTo(HaveKey("same-id"))
			Expect(other.serialized).NotTo(ContainSubstring("prior-cluster"))
		},
		Entry("Node Full", "Node", "Full"),
		Entry("PersistentVolume Partial", "PersistentVolume", "Partial"),
	)

	DescribeTable("UT-KA-2496-010: non-observational tools and missing correlation IDs cannot share evidence (BR-KA-016, AC-4)",
		func(id, tool string) {
			inv := New(Config{Logger: logr.Discard()})
			inv.captureEvidence(context.Background(), id, tool, "not a current target observation")
			observations, err := inv.evidenceSnapshot(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(observations).To(BeEmpty())
			if id == "" {
				inv.failEvidence(id, "unscoped failure")
				observations, err = inv.evidenceSnapshot(id)
				Expect(err).NotTo(HaveOccurred(), "an empty-ID failure must not poison a shared evidence bucket")
				Expect(observations).To(BeEmpty())
			}
		},
		Entry("missing correlation", "", "kubectl_logs"),
		Entry("todo tool", "rr-todo-2496", "todo_create"),
		Entry("namespaced history", "rr-context-2496", "get_namespaced_resource_context"),
		Entry("cluster history", "rr-cluster-context-2496", "get_cluster_resource_context"),
	)

	DescribeTable("UT-KA-2496-008: Secret values never survive projection of source metadata",
		func(output string) {
			inv := New(Config{Logger: logr.Discard()})
			inv.captureEvidence(context.Background(), "rr-privacy", "kubectl_get_by_name", output)
			observations, err := inv.evidenceSnapshot("rr-privacy")
			Expect(err).NotTo(HaveOccurred())
			Expect(observations).To(HaveLen(1))
			Expect(observations[0].Content).NotTo(ContainSubstring("opaque-private-value"))
			Expect(observations[0].Content).NotTo(ContainSubstring("b3BhcXVlLXByaXZhdGUtdmFsdWU="))
			Expect(observations[0].Content).To(ContainSubstring("REDACTED"))
		},
		Entry("JSON Secret", `{"apiVersion":"v1","kind":"Secret","metadata":{"name":"s"},"data":{"arbitrary":"b3BhcXVlLXByaXZhdGUtdmFsdWU="},"stringData":{"arbitrary":"opaque-private-value"}}`),
		Entry("YAML Secret", "apiVersion: v1\nkind: Secret\nmetadata:\n  name: s\nstringData:\n  arbitrary: opaque-private-value\n"),
		Entry("JSON array", `[{"kind":"Secret","stringData":{"arbitrary":"opaque-private-value"}}]`),
		Entry("nested JSON resource", `{"resource":{"kind":"Secret","stringData":{"arbitrary":"opaque-private-value"}}}`),
		Entry("YAML sequence", "- kind: Secret\n  stringData:\n    arbitrary: opaque-private-value\n"),
		Entry("nested YAML resource", "resource:\n  kind: Secret\n  stringData:\n    arbitrary: opaque-private-value\n"),
	)

	It("UT-KA-2496-008: captures every YAML document, not a silently partial first document", func() {
		inv := New(Config{Logger: logr.Discard()})
		inv.captureEvidence(context.Background(), "rr-documents", "kubectl_get_yaml",
			"kind: Pod\nstatus:\n  message: first-observation\n---\nkind: Pod\nstatus:\n  message: final-observation\n")
		observations, err := inv.evidenceSnapshot("rr-documents")
		Expect(err).NotTo(HaveOccurred())
		Expect(observations).To(HaveLen(1))
		Expect(observations[0].Content).To(ContainSubstring("first-observation"))
		Expect(observations[0].Content).To(ContainSubstring("final-observation"))
	})

	It("UT-KA-2496-008: credential-bearing JSON is sanitized and safely encoded in the packet", func() {
		inv := New(Config{Logger: logr.Discard()})
		inv.captureEvidence(context.Background(), "rr-json", "kubectl_get_by_name",
			`{"status":{"message":"full-observation-tail password=inline-private-value"},"password":"field-private-value","parameters":{"prior":"prior-private-value"}}`)
		frozen, err := inv.freezeEvidenceHandoff(context.Background(), handoffInput{Signal: katypes.SignalContext{ResourceKind: "Pod", ResourceName: "api", Namespace: "prod"}}, "rr-json")
		Expect(err).NotTo(HaveOccurred())
		Expect(json.Valid([]byte(frozen.serialized))).To(BeTrue())
		Expect(frozen.serialized).To(ContainSubstring("full-observation-tail"))
		for _, secret := range []string{"inline-private-value", "field-private-value", "prior-private-value"} {
			Expect(frozen.serialized).NotTo(ContainSubstring(secret))
		}
	})

	It("UT-KA-2496-008: a customized tool-output pipeline cannot disable handoff credential safety", func() {
		inv := New(Config{Logger: logr.Discard(), Pipeline: Pipeline{
			Sanitizer: sanitization.NewPipeline(sanitization.NewInjectionSanitizer(nil)),
		}})
		inv.captureEvidence(context.Background(), "rr-custom", "kubectl_get_by_name", `{"status":{"message":"password=custom-private-value"}}`)
		frozen, err := inv.freezeEvidenceHandoff(context.Background(), handoffInput{RCASummary: "password=rca-private-value"}, "rr-custom")
		Expect(err).NotTo(HaveOccurred())
		Expect(frozen.serialized).NotTo(ContainSubstring("custom-private-value"))
		Expect(frozen.serialized).NotTo(ContainSubstring("rca-private-value"))
	})

	DescribeTable("UT-KA-2496-010: an additional operator policy cannot fail into a partial or raw handoff (BR-KA-016, SI-10)",
		func(raw, reason string) {
			inv := New(Config{Logger: logr.Discard(), Pipeline: Pipeline{
				Sanitizer: sanitization.NewPipeline(restrictedEvidencePolicy2496{}),
			}})
			clean, err := inv.sanitizeHandoffJSON(context.Background(), json.RawMessage(raw))
			Expect(err).To(MatchError(ContainSubstring(reason)))
			Expect(err).To(MatchError(ContainSubstring("operator policy rejected")))
			Expect(clean).To(BeNil())
		},
		Entry("string value", `"restricted-evidence-2496"`, "sanitize evidence text"),
		Entry("nested object value", `{"message":"restricted-evidence-2496"}`, "sanitize evidence text"),
		Entry("nested array value", `[{"message":"restricted-evidence-2496"}]`, "sanitize evidence text"),
		Entry("object key", `{"restricted-evidence-2496":"current observation"}`, "sanitize evidence key"),
	)

	It("UT-KA-2496-010: dispatched evidence rejected by an operator policy is withheld and observably blocks freezing (BR-KA-016, SI-10)", func() {
		endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			defer GinkgoRecover()
			_, err := w.Write([]byte(`{"observation":"restricted-evidence-2496 password=private-private"}`))
			Expect(err).NotTo(HaveOccurred())
		}))
		defer endpoint.Close()
		probe := &phaseProbe2498{name: "kubectl_get_by_name", endpoint: endpoint.URL}
		reg := registry.New()
		reg.Register(probe)
		var logs []string
		logger := funcr.New(func(prefix, args string) { logs = append(logs, prefix+args) }, funcr.Options{})
		inv := New(Config{Logger: logger, Registry: reg, PhaseTools: DefaultPhaseToolMap(), Pipeline: Pipeline{
			Sanitizer: sanitization.NewPipeline(restrictedEvidencePolicy2496{}),
		}})
		result := inv.executeTool(context.Background(), probe.name, json.RawMessage(`{}`), "rr-policy-2496", katypes.PhaseRCA)
		Expect(probe.calls.Load()).To(Equal(int32(1)), "the real registry dispatches the external observation once")
		Expect(result).To(ContainSubstring("sanitization failed: tool output withheld"))
		Expect(result).NotTo(ContainSubstring("restricted-evidence-2496"))
		Expect(result).NotTo(ContainSubstring("private-private"))
		frozen, err := inv.freezeEvidenceHandoff(context.Background(), handoffInput{}, "rr-policy-2496")
		Expect(err).To(MatchError(ContainSubstring("observation sanitization failed")))
		Expect(frozen).To(BeNil())
		Expect(strings.Join(logs, "\n")).To(ContainSubstring("sanitization failed, fail-closed"))
	})

	It("UT-KA-2496-010: source text rejected by an operator policy never becomes a frozen packet (BR-KA-016, SI-10)", func() {
		inv := New(Config{Logger: logr.Discard(), Pipeline: Pipeline{
			Sanitizer: sanitization.NewPipeline(restrictedEvidencePolicy2496{}),
		}})
		frozen, err := inv.freezeEvidenceHandoff(context.Background(), handoffInput{RCASummary: "restricted-evidence-2496"}, "rr-source-policy-2496")
		Expect(err).To(MatchError(ContainSubstring("operator policy rejected")))
		Expect(frozen).To(BeNil())
	})

	It("UT-KA-2496-008: ordinary non-YAML logs retain every sanitized line", func() {
		inv := New(Config{Logger: logr.Discard()})
		inv.captureEvidence(context.Background(), "rr-logs", "kubectl_logs", "connection: [unclosed\nlast log line password=log-private-value")
		observations, err := inv.evidenceSnapshot("rr-logs")
		Expect(err).NotTo(HaveOccurred())
		Expect(observations).To(HaveLen(1))
		Expect(observations[0].Content).To(ContainSubstring("connection: [unclosed"))
		Expect(observations[0].Content).To(ContainSubstring("last log line"))
		Expect(observations[0].Content).NotTo(ContainSubstring("log-private-value"))
	})

	DescribeTable("UT-KA-2496-010: malformed observations prevent any partial frozen packet",
		func(output string) {
			inv := New(Config{Logger: logr.Discard()})
			inv.captureEvidence(context.Background(), "rr-invalid", "kubectl_logs", "good observation")
			inv.captureEvidence(context.Background(), "rr-invalid", "kubectl_get_by_name", output)
			inv.captureEvidence(context.Background(), "rr-invalid", "kubectl_logs", "later valid output cannot clear the earlier failure")
			frozen, err := inv.freezeEvidenceHandoff(context.Background(), handoffInput{}, "rr-invalid")
			Expect(err).To(HaveOccurred())
			Expect(frozen).To(BeNil())
		},
		Entry("invalid JSON", `{"status":`),
		Entry("invalid UTF-8", string([]byte{0xff})),
		Entry("malformed YAML after a Secret document", "kind: Secret\nstringData:\n  arbitrary: opaque-private-value\n---\nstatus: [unterminated\n"),
		Entry("malformed YAML Secret", "kind: Secret\nstringData:\n  arbitrary: opaque-private-value\nstatus: [unterminated\n"),
	)

	It("UT-KA-2496-AUDIT-007: an explicit source truncation marker is preserved as opaque evidence", func() {
		inv := New(Config{Logger: logr.Discard()})
		inv.captureEvidence(context.Background(), "rr-truncated", "nodes_stats_summary", `{"node":{"pods":[`+strings.Repeat(`{"name":"pod"},`, 20)+`]}
... [TRUNCATED] Response exceeded limit.`)
		observations, err := inv.evidenceSnapshot("rr-truncated")
		Expect(err).NotTo(HaveOccurred())
		Expect(observations).To(HaveLen(1))
		Expect(observations[0].Content).To(ContainSubstring("[TRUNCATED]"))
		frozen, err := inv.freezeEvidenceHandoff(context.Background(), handoffInput{Signal: katypes.SignalContext{ResourceKind: "Pod", ResourceName: "api", Namespace: "prod"}}, "rr-truncated")
		Expect(err).NotTo(HaveOccurred())
		Expect(frozen.serialized).To(ContainSubstring("[TRUNCATED]"))
	})

	It("UT-KA-2496-010: non-finite values fail serialization without fabricating a score", func() {
		score := math.NaN()
		inv := New(Config{Logger: logr.Discard()})
		frozen, err := inv.freezeEvidenceHandoff(context.Background(), handoffInput{
			Phase1: &prompt.Phase1Data{RCAConfidence: &score},
		}, "rr-nonfinite")
		Expect(err).To(MatchError(ContainSubstring("serialize handoff source")))
		Expect(frozen).To(BeNil())
	})

	DescribeTable("UT-KA-2496-010: defensive handoff decoding cannot return malformed raw evidence (BR-KA-016, SI-10)",
		func(raw, reason string) {
			inv := New(Config{Logger: logr.Discard()})
			clean, err := inv.sanitizeHandoffJSON(context.Background(), json.RawMessage(raw))
			Expect(err).To(MatchError(ContainSubstring(reason)))
			Expect(clean).To(BeNil(), "decoding failure cannot disclose a raw or partial packet")
		},
		Entry("unterminated text", `"private-private`, "decode evidence text"),
		Entry("unterminated array", `["private-private",`, "decode evidence array"),
		Entry("unterminated object", `{"private-private":`, "decode evidence object"),
	)

	DescribeTable("UT-KA-2496-010: defensive observation decoding rejects incomplete structured data (BR-KA-016, SI-10)",
		func(raw, reason string) {
			clean, err := projectObservationJSON(context.Background(), json.RawMessage(raw))
			Expect(err).To(MatchError(ContainSubstring(reason)))
			Expect(clean).To(BeNil(), "no partial observation is available on a decode failure")
		},
		Entry("empty observation", "", "empty structured observation"),
		Entry("unterminated array", `["private-private",`, "decode observation array"),
		Entry("unterminated object", `{"private-private":`, "decode observation object"),
	)

	It("UT-KA-2496-010: an unrepresentable prior assessment cannot produce a projected handoff (BR-KA-016, SI-10)", func() {
		score := math.Inf(1)
		packet, err := buildEvidenceHandoff(handoffInput{EnrichData: &prompt.EnrichmentData{
			HistoryResult: &enrichment.RemediationHistoryResult{Tier1: []enrichment.Tier1Entry{{
				RemediationUID: "prior-invalid-2496", EffectivenessScore: &score,
			}}},
		}}, nil)
		Expect(err).To(MatchError(ContainSubstring("serialize prior attempt")))
		Expect(packet).To(BeNil())
		Expect(math.IsInf(score, 1)).To(BeTrue(), "serialization cannot rewrite the source assessment")
	})

	It("UT-KA-2496-010: sanitizer key collisions fail freezing rather than merge distinct source evidence (BR-KA-016, SI-10)", func() {
		inv := New(Config{Logger: logr.Discard()})
		inv.captureEvidence(context.Background(), "rr-colliding-keys-2496", "kubectl_logs", "valid current observation")
		frozen, err := inv.freezeEvidenceHandoff(context.Background(), handoffInput{
			EnrichData: &prompt.EnrichmentData{DetectedLabels: map[string]string{
				"password=first-private-value": "first distinct source", "password=second-private-value": "second distinct source",
			}},
		}, "rr-colliding-keys-2496")
		Expect(err).To(MatchError(ContainSubstring("evidence key cannot be safely represented")))
		Expect(frozen).To(BeNil(), "neither raw fallback nor a lossy merged packet is permitted")
		observations, snapshotErr := inv.evidenceSnapshot("rr-colliding-keys-2496")
		Expect(snapshotErr).NotTo(HaveOccurred())
		Expect(observations).To(HaveLen(1))
		Expect(observations[0].Content).To(Equal("valid current observation"))
	})

	It("UT-KA-2496-010: frozen prior chronology has deterministic ties and does not invent unknown recording times (BR-KA-016, AU-3)", func() {
		inv := New(Config{Logger: logr.Discard()})
		zero := 0.
		at := time.Date(2026, time.October, 9, 15, 0, 0, 0, time.UTC)
		history := &enrichment.RemediationHistoryResult{TargetResource: "prod/Pod/api", Tier1: []enrichment.Tier1Entry{
			{RemediationUID: "newer", WorkflowID: "workflow-new", WorkflowName: "new", CompletedAt: at.Add(time.Minute)},
			{RemediationUID: "tied", WorkflowID: "workflow-b", WorkflowName: "b", CompletedAt: at, AssessmentReason: "Full", EffectivenessScore: &zero},
			{RemediationUID: "undated", WorkflowID: "workflow-old", WorkflowName: "old"},
			{RemediationUID: "tied", WorkflowID: "workflow-a", WorkflowName: "a", CompletedAt: at, AssessmentReason: "Full", EffectivenessScore: &zero},
		}}
		frozen, err := inv.freezeEvidenceHandoff(context.Background(), handoffInput{
			Signal:     katypes.SignalContext{Namespace: "prod", ResourceKind: "Pod", ResourceName: "api", ResourceAPIVersion: "v1"},
			EnrichData: &prompt.EnrichmentData{HistoryResult: history},
			Phase1: &prompt.Phase1Data{
				RemediationTarget: prompt.Phase1RemediationTarget{Kind: "Pod", Name: "api", Namespace: "prod"},
				DueDiligence:      &katypes.DueDiligenceReview{AlternativeHypotheses: "A transient allocation spike remains possible"},
			},
		}, "rr-order-2496")
		Expect(err).NotTo(HaveOccurred())
		var packet evidenceHandoff
		Expect(json.Unmarshal([]byte(frozen.serialized), &packet)).To(Succeed())
		Expect(packet.PriorAttempts).To(HaveLen(4))
		Expect(packet.PriorAttempts[0].RemediationUID).To(Equal("undated"))
		Expect(packet.PriorAttempts[0].RecordedAt).To(BeNil())
		Expect(packet.PriorAttempts[1].WorkflowID).To(HaveValue(Equal("workflow-a")))
		Expect(packet.PriorAttempts[2].WorkflowID).To(HaveValue(Equal("workflow-b")))
		Expect(packet.PriorAttempts[3].RemediationUID).To(Equal("newer"))
		for i, prior := range packet.PriorAttempts {
			Expect(prior.Attempt).To(Equal(i + 1))
		}
		Expect(packet.TargetResolution.Basis).To(Equal("phase_1_rca"))
		Expect(packet.Hypotheses).To(ContainElement("A transient allocation spike remains possible"))
		Expect(history.Tier1[0].RemediationUID).To(Equal("newer"), "sorting the packet cannot reorder authoritative source history")
	})

	It("UT-KA-2511-011: frozen history preserves applied-state and spec-change evidence across tiers", func() {
		currentHash := "sha256:current"
		history := &enrichment.RemediationHistoryResult{
			CurrentSpecHash: currentHash,
			Tier1: []enrichment.Tier1Entry{{
				RemediationUID: "tier1-applied", AppliedState: "applied", SpecChangeStatus: "unchanged",
				PreRemediationSpecHash: "sha256:pre1", PostRemediationSpecHash: "sha256:post1",
			}},
			Tier2: []enrichment.Tier2Summary{{
				RemediationUID: "tier2-applied", AppliedState: "partially_applied", SpecChangeStatus: "changed",
				PreRemediationSpecHash: "sha256:pre2", PostRemediationSpecHash: "sha256:post2",
			}},
		}
		packet, err := buildEvidenceHandoff(handoffInput{
			Signal:     katypes.SignalContext{Namespace: "prod", ResourceKind: "Deployment", ResourceName: "api", ResourceAPIVersion: "apps/v1"},
			EnrichData: &prompt.EnrichmentData{HistoryResult: history},
		}, nil)
		Expect(err).NotTo(HaveOccurred())
		wire, _, err := packet.serializeFrozen(time.Now().UTC(), katypes.DefaultMaxEvidenceHandoffBytes)
		Expect(err).NotTo(HaveOccurred())
		var decoded evidenceHandoff
		Expect(json.Unmarshal([]byte(wire), &decoded)).To(Succeed())
		Expect(decoded.Context.CurrentSpecHash).To(HaveValue(Equal(currentHash)))
		Expect(decoded.PriorAttempts).To(HaveLen(2))
		Expect(decoded.PriorAttempts[0].RecordedState.AppliedState).To(HaveValue(Equal("applied")))
		Expect(decoded.PriorAttempts[0].RecordedState.SpecChangeStatus).To(HaveValue(Equal("unchanged")))
		Expect(decoded.PriorAttempts[0].RecordedState.PreSpecHash).To(HaveValue(Equal("sha256:pre1")))
		Expect(decoded.PriorAttempts[1].RecordedState.AppliedState).To(HaveValue(Equal("partially_applied")))
		Expect(decoded.PriorAttempts[1].RecordedState.SpecChangeStatus).To(HaveValue(Equal("changed")))
		Expect(decoded.PriorAttempts[1].RecordedState.PostSpecHash).To(HaveValue(Equal("sha256:post2")))
	})

	It("UT-KA-2496-010: an unparseable target API version stays unknown without changing the existing API gate", func() {
		inv := New(Config{Logger: logr.Discard()})
		frozen, err := inv.freezeEvidenceHandoff(context.Background(), handoffInput{
			Signal: katypes.SignalContext{ResourceKind: "Subscription", ResourceName: "etcd", Namespace: "demo-operator", ResourceAPIVersion: "../../etc/passwd"},
		}, "rr-unparseable-target")
		Expect(err).NotTo(HaveOccurred())
		Expect(frozen.source.Signal.ResourceAPIVersion).To(Equal("../../etc/passwd"), "the handoff must not rewrite the downstream validation input")
		var packet evidenceHandoff
		Expect(json.Unmarshal([]byte(frozen.serialized), &packet)).To(Succeed())
		Expect(packet.Target.APIGroup).To(BeNil())
		Expect(packet.Target.APIVersion).To(BeNil())
		Expect(packet.Unknowns).To(ContainElement("target API group/version could not be parsed"))
	})

	It("UT-KA-2496-010: the complete packet, not only observations, must fit the budget", func() {
		inv := New(Config{Logger: logr.Discard(), Pipeline: Pipeline{MaxEvidenceHandoffBytes: 2048}})
		inv.captureEvidence(context.Background(), "rr-whole", "kubectl_logs", "small observation")
		observations, err := inv.evidenceSnapshot("rr-whole")
		Expect(err).NotTo(HaveOccurred())
		Expect(observations).To(HaveLen(1))
		frozen, err := inv.freezeEvidenceHandoff(context.Background(), handoffInput{
			RCASummary: strings.Repeat("recorded detail ", 200),
		}, "rr-whole")
		Expect(err).To(MatchError(ContainSubstring("byte budget")))
		Expect(frozen).To(BeNil())
	})

	It("UT-KA-2496-010: freezing deep-copies complete history and acceptance scope", func() {
		inv := New(Config{Logger: logr.Discard()})
		zero, rcaConfidence := 0., .93
		history := &enrichment.RemediationHistoryResult{TargetResource: "prod/Pod/api"}
		for i := 0; i < 25; i++ {
			history.Tier1 = append(history.Tier1, enrichment.Tier1Entry{RemediationUID: fmt.Sprintf("prior-%02d", i),
				WorkflowID: "workflow-id", WorkflowName: "workflow-name", AssessmentReason: "Full", EffectivenessScore: &zero})
		}
		in := handoffInput{Signal: katypes.SignalContext{ResourceKind: "Pod", ResourceName: "api", Namespace: "prod", ResourceAPIVersion: "v1"},
			EnrichData: &prompt.EnrichmentData{HistoryResult: history}, Phase1: &prompt.Phase1Data{RCAConfidence: &rcaConfidence}}
		frozen, err := inv.freezeEvidenceHandoff(context.Background(), in, "rr-frozen")
		Expect(err).NotTo(HaveOccurred())
		original := frozen.serialized
		history.Tier1[0].WorkflowID, history.Tier1[0].RemediationUID = "mutated-workflow", "mutated-rr"
		rcaConfidence, zero = .01, .9
		Expect(frozen.serialized).To(Equal(original))
		Expect(frozen.source.Phase1.RCAConfidence).To(HaveValue(BeNumerically("==", .93)))
		Expect(frozen.repeat.CompletedPriorWorkflows["workflow-id"]).To(HaveLen(25))
		Expect(frozen.repeat.CompletedPriorWorkflows).NotTo(HaveKey("mutated-workflow"))
		Expect(frozen.source.EnrichData.HistoryResult.Tier1[0].EffectivenessScore).To(HaveValue(BeNumerically("==", 0.)))
	})

	It("UT-KA-2496-010: the exact byte boundary includes digest and frozen timestamp", func() {
		packet, err := buildEvidenceHandoff(handoffInput{RCASummary: "Memory pressure"}, []handoffObservation{})
		Expect(err).NotTo(HaveOccurred())
		at := time.Date(2026, time.October, 10, 15, 0, 0, 123456789, time.UTC)
		wire, digest, err := packet.serializeFrozen(at, katypes.DefaultMaxEvidenceHandoffBytes)
		Expect(err).NotTo(HaveOccurred())
		exact, exactDigest, err := packet.serializeFrozen(at, len(wire))
		Expect(err).NotTo(HaveOccurred())
		Expect(exact).To(Equal(wire))
		Expect(exactDigest).To(Equal(digest))
		overflow, overflowDigest, err := packet.serializeFrozen(at, len(wire)-1)
		Expect(err).To(MatchError(ContainSubstring("byte budget")))
		Expect(overflow).To(BeEmpty())
		Expect(overflowDigest).To(BeEmpty())
		Expect(packet.FrozenAt).To(BeNil(), "serialization must not change source state on success or failure")
		Expect(packet.HandoffDigest).To(BeEmpty())
	})

	It("UT-KA-2496-010: an unrepresentable RCA score cannot produce a frozen packet or digest (BR-KA-016, SI-10)", func() {
		score := math.Inf(1)
		packet, err := buildEvidenceHandoff(handoffInput{Phase1: &prompt.Phase1Data{RCAConfidence: &score}}, nil)
		Expect(err).NotTo(HaveOccurred())
		wire, digest, err := packet.serializeFrozen(time.Now().UTC(), katypes.DefaultMaxEvidenceHandoffBytes)
		Expect(err).To(MatchError(ContainSubstring("serialize evidence handoff")))
		Expect(wire).To(BeEmpty())
		Expect(digest).To(BeEmpty())
		Expect(packet.FrozenAt).To(BeNil())
		Expect(packet.HandoffDigest).To(BeEmpty())
		Expect(math.IsInf(*packet.RCA.Confidence, 1)).To(BeTrue(), "failure cannot rewrite the authoritative source score")
	})

	It("UT-KA-2496-010: an unrepresentable freeze timestamp cannot expose a partially digested packet (BR-KA-016, SI-10)", func() {
		packet, err := buildEvidenceHandoff(handoffInput{RCASummary: "Current memory pressure"}, nil)
		Expect(err).NotTo(HaveOccurred())
		invalidTime := time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC)
		wire, digest, err := packet.serializeFrozen(invalidTime, katypes.DefaultMaxEvidenceHandoffBytes)
		Expect(err).To(MatchError(ContainSubstring("serialize frozen evidence handoff")))
		Expect(wire).To(BeEmpty())
		Expect(digest).To(BeEmpty())
		Expect(packet.FrozenAt).To(BeNil())
		Expect(packet.HandoffDigest).To(BeEmpty())
		Expect(packet.RCA.Summary).To(Equal("Current memory pressure"))
	})

	It("UT-KA-2496-010: runtime metadata is excluded from the canonical digest", func() {
		packet, err := buildEvidenceHandoff(handoffInput{RCASummary: "Memory pressure"}, []handoffObservation{})
		Expect(err).NotTo(HaveOccurred())
		at := time.Date(2026, time.October, 10, 15, 0, 0, 123456789, time.UTC)
		wire, digest, err := packet.serializeFrozen(at, katypes.DefaultMaxEvidenceHandoffBytes)
		Expect(err).NotTo(HaveOccurred())
		packet.HandoffDigest, packet.FrozenAt = "untrusted-digest", &at
		later, laterDigest, err := packet.serializeFrozen(at.Add(time.Second), katypes.DefaultMaxEvidenceHandoffBytes)
		Expect(err).NotTo(HaveOccurred())
		Expect(later).NotTo(Equal(wire))
		Expect(laterDigest).To(Equal(digest))
		packet.RCA.Summary = "New observation"
		_, changedDigest, err := packet.serializeFrozen(at, katypes.DefaultMaxEvidenceHandoffBytes)
		Expect(err).NotTo(HaveOccurred())
		Expect(changedDigest).NotTo(Equal(digest), "evidence changes must change the digest")
	})

	It("UT-KA-2496-010: concurrent correlation scopes remain isolated and are reclaimed", func() {
		inv := New(Config{Logger: logr.Discard()})
		var wg sync.WaitGroup
		for i := 0; i < 12; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				inv.captureEvidence(context.Background(), fmt.Sprintf("rr-%02d", index), "kubectl_logs", fmt.Sprintf("observation-%02d", index))
			}(i)
		}
		wg.Wait()
		for i := 0; i < 12; i++ {
			observations, err := inv.evidenceSnapshot(fmt.Sprintf("rr-%02d", i))
			Expect(err).NotTo(HaveOccurred())
			Expect(observations).To(HaveLen(1))
			Expect(observations[0].Content).To(Equal(fmt.Sprintf("observation-%02d", i)))
		}
		inv.resetEvidence("rr-00")
		observations, err := inv.evidenceSnapshot("rr-00")
		Expect(err).NotTo(HaveOccurred())
		Expect(observations).To(BeEmpty())
		Expect(inv.pruneEvidence(-time.Second)).To(Equal(11))
	})

	It("UT-KA-2496-010: the shared lifecycle sweep reclaims abandoned evidence without losing active scopes (BR-KA-016, AC-4)", func() {
		inv := New(Config{Logger: logr.Discard()})
		const staleID = "abandoned-handoff-2496"
		const activeID = "active-handoff-2496"
		for _, id := range []string{staleID, activeID} {
			inv.captureEvidence(context.Background(), id, "kubectl_logs", id+" current observation")
			inv.anomalyDetectorFor(id)
			inv.metricsFor(id).IncLLMTurns()
			inv.recordTokenUsage(id, llm.TokenUsage{PromptTokens: 7, CompletionTokens: 3, TotalTokens: 10})
		}
		activeDetector := inv.anomalyDetectorFor(activeID)
		staleAt := time.Now().Add(-2 * DefaultAnomalyDetectorTTL)
		// No cleanup goroutine is running in this unit fixture. Timestamp control
		// removes scheduler/sleep dependence while exercising the real shared sweep.
		inv.evidence.entries[staleID].lastAccess = staleAt
		inv.anomalyScope.entries[staleID].lastAccess = staleAt
		inv.metricsScope.entries[staleID].lastAccess = staleAt
		inv.tokenScope.entries[staleID].lastAccess = staleAt

		inv.pruneInvestigationScopes(DefaultAnomalyDetectorTTL)
		stale, err := inv.evidenceSnapshot(staleID)
		Expect(err).NotTo(HaveOccurred())
		Expect(stale).To(BeEmpty(), "abandoned observations must not survive the production cleanup sweep")
		Expect(inv.InvestigationTotals(staleID).LLMTurns).To(BeZero())
		Expect(inv.InvestigationTotals(staleID).TotalTokens).To(BeZero())
		active, err := inv.evidenceSnapshot(activeID)
		Expect(err).NotTo(HaveOccurred())
		Expect(active).To(HaveLen(1))
		Expect(active[0].Content).To(Equal(activeID + " current observation"))
		Expect(inv.anomalyDetectorFor(activeID)).To(BeIdenticalTo(activeDetector))
		Expect(inv.InvestigationTotals(activeID).LLMTurns).To(Equal(1))
		Expect(inv.InvestigationTotals(activeID).TotalTokens).To(Equal(10))
		// A second sweep cannot consume a still-active request's handoff.
		inv.pruneInvestigationScopes(DefaultAnomalyDetectorTTL)
		active, err = inv.evidenceSnapshot(activeID)
		Expect(err).NotTo(HaveOccurred())
		Expect(active).To(HaveLen(1))
	})

	It("UT-KA-2496-010: the production cleanup timer reclaims abandoned evidence and preserves active handoffs (BR-KA-016, AC-4)", func() {
		inv := New(Config{Logger: logr.Discard()})
		const staleID = "timer-abandoned-2496"
		const activeID = "timer-active-2496"
		inv.captureEvidence(context.Background(), staleID, "kubectl_logs", "abandoned observation")
		inv.captureEvidence(context.Background(), activeID, "kubectl_logs", "active current observation")
		// Set age before starting the actual cancellation-owned cleanup loop.
		inv.evidence.entries[staleID].lastAccess = time.Now().Add(-2 * time.Hour)
		ctx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)
		inv.StartAnomalyDetectorCleanupLoop(ctx, 10*time.Millisecond, time.Hour)
		Eventually(func() bool {
			// Inspect without evidenceSnapshot: reading a live request refreshes
			// its access time and would make this abandoned fixture active again.
			inv.evidence.mu.Lock()
			defer inv.evidence.mu.Unlock()
			_, present := inv.evidence.entries[staleID]
			return present
		}).Should(BeFalse(), "abandoned evidence must be reclaimed through the production timer, not only a direct sweep")
		active, err := inv.evidenceSnapshot(activeID)
		Expect(err).NotTo(HaveOccurred())
		Expect(active).To(HaveLen(1))
		Expect(active[0].Content).To(Equal("active current observation"))
	})
})

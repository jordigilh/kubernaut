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

package agentsession_test

import (
	"encoding/json"
	"strings"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/utils/ptr"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/agentsession"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// BR-AA-KA-065.2: AgentSession.Spec is a lossless 1:1 translation of the
// retired agentclient.IncidentRequest -- these tests pin that contract at
// the mapping-function boundary independent of the CRD wiring.
var _ = Describe("MapSpecToSignal — BR-AA-KA-065.2", func() {
	Describe("UT-AA-KA-065-001: every incident-payload field on Spec maps to the matching SignalContext field", func() {
		It("should carry required and optional fields through without loss", func() {
			duplicateCount := 3
			dedupWindow := 5
			isDup := true
			spec := agentsessionv1.AgentSessionSpec{
				IncidentID:                 "incident-1",
				RemediationID:              "rr-1",
				SignalName:                 "OOMKilled",
				Severity:                   "critical",
				SignalSource:               "prometheus",
				ResourceNamespace:          "prod",
				ResourceKind:               "Pod",
				ResourceName:               "web-0",
				ResourceAPIVersion:         "v1",
				ErrorMessage:               "container OOM",
				Description:                "pod crashed",
				Environment:                "production",
				Priority:                   "P1",
				RiskTolerance:              "low",
				BusinessCategory:           "payments",
				ClusterName:                "cluster-a",
				Cluster:                    "fleet-prod",
				IsDuplicate:                &isDup,
				OccurrenceCount:            &duplicateCount,
				DeduplicationWindowMinutes: &dedupWindow,
				FiringTime:                 "2026-08-01T00:00:00Z",
				ReceivedTime:               "2026-08-01T00:00:01Z",
				FirstSeen:                  "2026-08-01T00:00:00Z",
				LastSeen:                   "2026-08-01T00:00:05Z",
				SignalLabels:               map[string]string{"app": "web"},
				SignalAnnotations:          map[string]string{"summary": "oom"},
				SignalMode:                 "Reactive",
			}

			sc := agentsession.MapSpecToSignal(spec)

			Expect(sc.Name).To(Equal("OOMKilled"))
			Expect(sc.Namespace).To(Equal("prod"))
			Expect(sc.Severity).To(Equal("critical"))
			Expect(sc.Message).To(Equal("container OOM"))
			Expect(sc.IncidentID).To(Equal("incident-1"))
			Expect(sc.RemediationID).To(Equal("rr-1"))
			Expect(sc.ResourceKind).To(Equal("Pod"))
			Expect(sc.ResourceName).To(Equal("web-0"))
			Expect(sc.ResourceAPIVersion).To(Equal("v1"))
			Expect(sc.ClusterName).To(Equal("cluster-a"))
			// CI RCA (run 32220596605, E2E-FLEET-017/018): ClusterID -- not
			// ClusterName -- is the field prescopeFleetOverlay
			// (internal/kubernautagent/investigator/fleet_overlay.go) reads
			// to resolve the per-investigation fleet tool overlay. Mirrors
			// the retired HTTP path's MapIncidentRequestToSignal
			// (internal/kubernautagent/server/handler.go), which mapped
			// req.ClusterName into both ClusterName and ClusterID.
			Expect(sc.ClusterID).To(Equal("cluster-a"))
			Expect(sc.ClusterClassification).To(Equal("fleet-prod"))
			Expect(sc.Environment).To(Equal("production"))
			Expect(sc.Priority).To(Equal("P1"))
			Expect(sc.RiskTolerance).To(Equal("low"))
			Expect(sc.SignalSource).To(Equal("prometheus"))
			Expect(sc.BusinessCategory).To(Equal("payments"))
			Expect(sc.Description).To(Equal("pod crashed"))
			Expect(sc.SignalMode).To(Equal("reactive"), "SignalMode must be lower-cased for the investigator's prompt-strategy switch")
			Expect(sc.FiringTime).To(Equal("2026-08-01T00:00:00Z"))
			Expect(sc.ReceivedTime).To(Equal("2026-08-01T00:00:01Z"))
			Expect(sc.FirstSeen).To(Equal("2026-08-01T00:00:00Z"))
			Expect(sc.LastSeen).To(Equal("2026-08-01T00:00:05Z"))
			Expect(sc.SignalLabels).To(Equal(map[string]string{"app": "web"}))
			Expect(sc.SignalAnnotations).To(Equal(map[string]string{"summary": "oom"}))
			// DD-AA-KA-001 Amendment Gap 1: Interactive is NOT part of this
			// mapping -- Spec has no Interactive field (it cannot hold a
			// trustworthy snapshot of a fact that can become true after
			// Create). The dispatcher sets sc.Interactive separately, from
			// its own dispatch-time InvestigationSession-existence check.
			Expect(sc.Interactive).To(BeFalse(), "MapSpecToSignal must not set Interactive -- that is the dispatcher's job")
			Expect(sc.IsDuplicate).NotTo(BeNil())
			Expect(*sc.IsDuplicate).To(BeTrue())
			Expect(sc.OccurrenceCount).NotTo(BeNil())
			Expect(*sc.OccurrenceCount).To(Equal(3))
			Expect(sc.DeduplicationWindowMinutes).NotTo(BeNil())
			Expect(*sc.DeduplicationWindowMinutes).To(Equal(5))
		})
	})

	Describe("UT-AA-KA-065-002: nil optional pointer fields do not panic and stay nil", func() {
		It("should leave IsDuplicate/OccurrenceCount/DeduplicationWindowMinutes nil when unset on Spec", func() {
			spec := agentsessionv1.AgentSessionSpec{SignalName: "PodPending", Severity: "warning"}

			sc := agentsession.MapSpecToSignal(spec)

			Expect(sc.IsDuplicate).To(BeNil())
			Expect(sc.OccurrenceCount).To(BeNil())
			Expect(sc.DeduplicationWindowMinutes).To(BeNil())
		})
	})

	Describe("UT-AA-KA-065-117: the \"default\" ClusterName sentinel must not be promoted to ClusterID (Issue #2314)", func() {
		It("should leave ClusterID empty when ClusterName is the hub-local display placeholder", func() {
			// pkg/aianalysis/handlers/request_builder.go's clusterNameFor
			// emits katypes.DefaultClusterNameSentinel ("default") as a
			// cosmetic ClusterName fallback for hub-local investigations
			// with no real target cluster. Promoting that literal into
			// ClusterID made prescopeFleetOverlay (fleet_overlay.go) treat
			// every such investigation as a fleet target and fail closed
			// with no FleetOverlayResolver configured -- confirmed root
			// cause of the kubernautagent integration and aianalysis E2E
			// "no FleetOverlayResolver configured" CI failures.
			spec := agentsessionv1.AgentSessionSpec{
				SignalName:  "PodPending",
				Severity:    "warning",
				ClusterName: katypes.DefaultClusterNameSentinel,
			}

			sc := agentsession.MapSpecToSignal(spec)

			Expect(sc.ClusterName).To(Equal("default"), "cosmetic display field is unaffected")
			Expect(sc.ClusterID).To(BeEmpty(), "prescopeFleetOverlay must see this as hub-local (skip-overlay), not a fleet target")
		})

		It("should still promote a genuine fleet cluster name to ClusterID", func() {
			spec := agentsessionv1.AgentSessionSpec{
				SignalName:  "PodPending",
				Severity:    "warning",
				ClusterName: "remote-cluster",
			}

			sc := agentsession.MapSpecToSignal(spec)

			Expect(sc.ClusterID).To(Equal("remote-cluster"))
		})
	})
})

// SI-10: AgentSessionResult must carry only the curated investigation
// subset, never internal workflow/validation/alignment state verbatim.
var _ = Describe("MapInvestigationResultToAgentSessionResult — SI-10 curated result", func() {
	logger := logr.Discard()
	const rejectedMode2496 = "rejected"
	It("UT-KA-2496-AUDIT-004: an unsafe mandatory explanation cannot persist as an accepted selection", func() {
		required := true
		result := &katypes.InvestigationResult{WorkflowID: "catalog-id", WorkflowName: "catalog-name", Confidence: .84,
			RepeatJustificationRequired: &required, RepeatValidationOutcome: "accepted",
			RepeatJustification: &sharedtypes.RepeatJustification{EvidenceBasis: strings.Repeat("a", 1989) + " password=x", PriorOutcomeRelevance: "Prior outcome", ExpectedMechanism: "Increase limit"},
		}
		mapped := agentsession.MapInvestigationResultToAgentSessionResult(logger, result, "rr-required-safety")
		Expect(mapped.NeedsHumanReview).To(BeTrue())
		Expect(mapped.SelectedWorkflow).To(BeNil())
		Expect(mapped.SelectionAssessment).NotTo(BeNil())
		Expect(string(mapped.SelectionAssessment.Raw)).To(ContainSubstring(`"repeat_validation_outcome":"rejected"`))
		Expect(result.WorkflowID).To(Equal("catalog-id"), "projection cannot mutate the source result")
	})
	DescribeTable("UT-KA-2496-AUDIT-002: selected and rejected persisted narratives redact credentials without changing execution fields",
		func(rejected bool) {
			result := &katypes.InvestigationResult{WorkflowID: "catalog-id", WorkflowName: "catalog-name", Confidence: .84,
				RepeatJustification: &sharedtypes.RepeatJustification{EvidenceBasis: "Current password=private-private", PriorOutcomeRelevance: "Prior password=private-private", ExpectedMechanism: "Mechanism password=private-private"},
				Parameters:          map[string]any{"EXECUTION_INPUT": "keep-execution-value"}, ExecutionBundle: "catalog-bundle",
			}
			if rejected {
				result.WorkflowID, result.ProposedWorkflowID = "", "catalog-id"
				result.HumanReviewNeeded = true
			}
			mapped := agentsession.MapInvestigationResultToAgentSessionResult(logger, result, "rr-safe-reasoning")
			var raw []byte
			if rejected {
				raw = mapped.SelectionAssessment.Raw
			} else {
				raw = mapped.SelectedWorkflow.Raw
				Expect(string(raw)).To(ContainSubstring("keep-execution-value"))
				Expect(string(raw)).To(ContainSubstring("catalog-bundle"))
			}
			Expect(string(raw)).NotTo(ContainSubstring("private-private"))
			Expect(string(raw)).To(ContainSubstring("REDACTED"))
			Expect(result.RepeatJustification.EvidenceBasis).To(Equal("Current password=private-private"), "mapping does not mutate the authoritative result")
		}, Entry("selected", false), Entry("rejected", true),
	)

	DescribeTable("UT-KA-2496-009: AgentSession preserves independent selection metadata without making rejected proposals executable (BR-KA-016, AU-3, SI-10)",
		func(mode string, confidence *float64, evaluated *bool) {
			rcaConfidence := .93
			result := &katypes.InvestigationResult{
				RCASummary: "Current memory pressure", RCAConfidence: &rcaConfidence, Confidence: .84,
				WorkflowID: "catalog-2496", WorkflowName: "increase-memory", WorkflowSelectionConfidence: confidence,
				RepeatJustificationRequired: evaluated, PriorWorkflowMatch: evaluated, RepeatValidationOutcome: "accepted",
				HandoffDigest: "sha256:" + strings.Repeat("b", 64), PriorRemediationUIDs: []string{"trusted-prior-2496"},
				RepeatJustification: &sharedtypes.RepeatJustification{
					EvidenceBasis: "Observed \"limit\"\nchange", PriorOutcomeRelevance: "Previous Full assessment was zero", ExpectedMechanism: "Increase the limit",
				},
				Parameters: map[string]interface{}{"PRIVATE": "private-parameters-2496"}, ExecutionBundle: "private-bundle-2496",
			}
			if confidence != nil {
				result.Confidence = *confidence
			}
			if mode == "review" || mode == rejectedMode2496 {
				result.HumanReviewNeeded, result.HumanReviewReason = true, "low_confidence"
			}
			if mode == rejectedMode2496 {
				result.WorkflowID, result.ProposedWorkflowID = "", "catalog-2496"
				result.RepeatValidationOutcome = "rejected"
			}
			before, err := json.Marshal(result)
			Expect(err).NotTo(HaveOccurred())
			res := agentsession.MapInvestigationResultToAgentSessionResult(logger, result, "rr-map-2496")
			Expect(res.IncidentID).To(Equal("rr-map-2496"))
			Expect(res.NeedsHumanReview).To(Equal(result.HumanReviewNeeded))
			var selection map[string]json.RawMessage
			if mode == rejectedMode2496 {
				Expect(res.SelectedWorkflow).To(BeNil())
				Expect(res.SelectionAssessment).NotTo(BeNil())
				Expect(json.Unmarshal(res.SelectionAssessment.Raw, &selection)).To(Succeed())
				Expect(string(selection["proposed_workflow_id"])).To(MatchJSON(`"catalog-2496"`))
				Expect(selection).NotTo(HaveKey("workflow_id"))
				Expect(selection).NotTo(HaveKey("parameters"))
				Expect(selection).NotTo(HaveKey("execution_bundle"))
				Expect(string(res.SelectionAssessment.Raw)).NotTo(ContainSubstring("private-"))
			} else {
				Expect(res.SelectedWorkflow).NotTo(BeNil())
				Expect(res.SelectionAssessment).To(BeNil())
				Expect(json.Unmarshal(res.SelectedWorkflow.Raw, &selection)).To(Succeed())
				Expect(string(selection["workflow_id"])).To(MatchJSON(`"catalog-2496"`))
				Expect(string(selection["parameters"])).To(MatchJSON(`{"PRIVATE":"private-parameters-2496"}`), "the execution snapshot is preserved at the durable boundary")
				Expect(string(selection["execution_bundle"])).To(MatchJSON(`"private-bundle-2496"`))
			}
			Expect(string(selection["workflow_name"])).To(MatchJSON(`"increase-memory"`))
			Expect(string(selection["handoff_digest"])).To(MatchJSON(`"` + result.HandoffDigest + `"`))
			Expect(string(selection["prior_remediation_uids"])).To(MatchJSON(`["trusted-prior-2496"]`))
			Expect(string(selection["repeat_validation_outcome"])).To(MatchJSON(`"` + result.RepeatValidationOutcome + `"`))
			if confidence == nil {
				Expect(selection).NotTo(HaveKey("workflow_selection_confidence"), "unknown selection confidence cannot borrow RCA or aggregate confidence")
			} else {
				var actual float64
				Expect(json.Unmarshal(selection["workflow_selection_confidence"], &actual)).To(Succeed())
				Expect(actual).To(Equal(*confidence))
			}
			for _, key := range []string{"repeat_justification_required", "prior_workflow_match"} {
				if evaluated == nil {
					Expect(selection).NotTo(HaveKey(key))
				} else {
					var actual bool
					Expect(json.Unmarshal(selection[key], &actual)).To(Succeed())
					Expect(actual).To(Equal(*evaluated))
				}
			}
			var reasoning sharedtypes.RepeatJustification
			Expect(json.Unmarshal(selection["repeat_justification"], &reasoning)).To(Succeed())
			Expect(reasoning).To(Equal(*result.RepeatJustification), "JSON metacharacters must remain data, not fabricated records")
			after, err := json.Marshal(result)
			Expect(err).NotTo(HaveOccurred())
			Expect(after).To(Equal(before), "mapping must not mutate the authoritative investigation result")
		},
		Entry("accepted selection", "selected", ptr.To(.84), ptr.To(true)),
		Entry("explicit zero/false below-floor review", "review", ptr.To(0.), ptr.To(false)),
		Entry("unknown confidence and unevaluated repeat", "selected", nil, nil),
		Entry("rejected proposal", rejectedMode2496, ptr.To(.84), ptr.To(true)),
	)

	DescribeTable("UT-KA-2496-009: rejected AgentSession reasoning omits invalid parts instead of truncating them (BR-KA-016, SI-10)",
		func(part string, retain bool) {
			result := &katypes.InvestigationResult{
				ProposedWorkflowID: "rejected-2496", HumanReviewNeeded: true, HumanReviewReason: "rca_incomplete",
				RepeatJustification: &sharedtypes.RepeatJustification{EvidenceBasis: part, PriorOutcomeRelevance: part, ExpectedMechanism: part},
			}
			res := agentsession.MapInvestigationResultToAgentSessionResult(logger, result, "rr-map-rejected-2496")
			Expect(res.SelectedWorkflow).To(BeNil())
			Expect(res.SelectionAssessment).NotTo(BeNil())
			var selection map[string]json.RawMessage
			Expect(json.Unmarshal(res.SelectionAssessment.Raw, &selection)).To(Succeed())
			Expect(selection).NotTo(HaveKey("workflow_name"), "an unavailable catalog name must stay unknown")
			Expect(selection).NotTo(HaveKey("workflow_id"))
			var reasoning map[string]string
			Expect(json.Unmarshal(selection["repeat_justification"], &reasoning)).To(Succeed())
			if retain {
				Expect(reasoning).To(Equal(map[string]string{"evidence_basis": part, "prior_outcome_relevance": part, "expected_mechanism": part}))
			} else {
				Expect(reasoning).To(BeEmpty(), "no apparently complete truncated rationale may be manufactured")
			}
			Expect(result.RepeatJustification.EvidenceBasis).To(Equal(part))
		},
		Entry("empty", "", false),
		Entry("invalid UTF-8", string([]byte{0xff}), false),
		Entry("oversized ASCII", strings.Repeat("x", 2001), false),
		Entry("oversized multibyte text", strings.Repeat("界", 2001), false),
		Entry("exact multibyte rune boundary", strings.Repeat("界", 2000), true),
	)

	It("UT-KA-2496-009: absent proposal or explanation stays absent in AgentSession review (BR-KA-016, SI-10)", func() {
		result := &katypes.InvestigationResult{ProposedWorkflowID: "rejected-2496", HumanReviewNeeded: true}
		res := agentsession.MapInvestigationResultToAgentSessionResult(logger, result, "rr-map-absent-2496")
		Expect(res.SelectedWorkflow).To(BeNil())
		Expect(res.SelectionAssessment).NotTo(BeNil())
		var selection map[string]json.RawMessage
		Expect(json.Unmarshal(res.SelectionAssessment.Raw, &selection)).To(Succeed())
		Expect(selection).NotTo(HaveKey("repeat_justification"))
		Expect(selection).NotTo(HaveKey("prior_remediation_uids"))
		Expect(selection).NotTo(HaveKey("handoff_digest"))
		result.ProposedWorkflowID = ""
		res = agentsession.MapInvestigationResultToAgentSessionResult(logger, result, "rr-map-absent-2496")
		Expect(res.SelectedWorkflow).To(BeNil())
		Expect(res.SelectionAssessment).To(BeNil(), "no-workflow review must not synthesize a rejected candidate")
	})

	Describe("UT-AA-KA-065-010: a fully-populated completed result maps every curated field", func() {
		It("should populate Analysis, Confidence, RootCauseAnalysis, SelectedWorkflow, and history fields", func() {
			isActionable := true
			result := &katypes.InvestigationResult{
				RCASummary: "OOM due to memory leak",
				Severity:   "critical",
				SignalName: "OOMKilled",
				RemediationTarget: katypes.RemediationTarget{
					Kind: "Deployment", Name: "web", Namespace: "prod",
				},
				Confidence:            0.92,
				WorkflowID:            "wf-restart",
				ExecutionBundle:       "bundle-1",
				ExecutionBundleDigest: "sha256:abc",
				ExecutionEngine:       "tekton",
				ServiceAccountName:    "wf-sa",
				WorkflowVersion:       "v2",
				WorkflowRationale:     "restart clears leaked memory",
				ActionType:            "restart",
				WorkflowName:          "restart-deployment",
				ExecutionClusterID:    "prod-east",
				EngineConfig:          &apiextensionsv1.JSON{Raw: []byte(`{"playbookPath":"site.yml"}`)},
				IsActionable:          &isActionable,
				ContributingFactors:   []string{"memory leak"},
				CausalChain:           []string{"leak", "OOM", "crash"},
				DetectedLabels:        map[string]interface{}{"gitops": true},
				AlternativeWorkflows: []katypes.AlternativeWorkflow{
					{WorkflowID: "wf-scale", ExecutionBundle: "bundle-2", Confidence: 0.4, Rationale: "scale up instead"},
				},
				ValidationAttemptsHistory: []katypes.ValidationAttemptRecord{
					{Attempt: 1, WorkflowID: "wf-restart", IsValid: true, Timestamp: "2026-08-01T00:00:00Z"},
				},
				AlignmentVerdict: &katypes.AlignmentVerdictResult{
					Result: "aligned", Summary: "no issues", Flagged: 0, Total: 3,
					Findings: []katypes.AlignmentFinding{
						{StepIndex: 0, StepKind: "tool_call", Tool: "get_pods", Explanation: "benign"},
					},
				},
			}

			res := agentsession.MapInvestigationResultToAgentSessionResult(logger, result, "incident-1")

			Expect(res.IncidentID).To(Equal("incident-1"))
			Expect(res.Analysis).To(Equal("OOM due to memory leak"))
			Expect(res.Confidence).To(Equal(0.92))
			Expect(res.NeedsHumanReview).To(BeFalse())
			Expect(res.IsActionable).NotTo(BeNil())
			Expect(*res.IsActionable).To(BeTrue())
			Expect(res.Warnings).To(BeEmpty())

			Expect(res.RootCauseAnalysis).NotTo(BeNil())
			var rca map[string]interface{}
			Expect(json.Unmarshal(res.RootCauseAnalysis.Raw, &rca)).To(Succeed())
			Expect(rca["summary"]).To(Equal("OOM due to memory leak"))
			Expect(rca["causal_chain"]).To(ConsistOf("leak", "OOM", "crash"))

			Expect(res.SelectedWorkflow).NotTo(BeNil())
			var sw map[string]interface{}
			Expect(json.Unmarshal(res.SelectedWorkflow.Raw, &sw)).To(Succeed())
			Expect(sw["workflow_id"]).To(Equal("wf-restart"))
			Expect(sw["execution_bundle_digest"]).To(Equal("sha256:abc"))
			Expect(sw["execution_cluster_id"]).To(Equal("prod-east"))
			Expect(sw["engine_config"]).To(Equal(map[string]interface{}{"playbookPath": "site.yml"}))

			Expect(res.DetectedLabels).NotTo(BeNil())

			Expect(res.AlternativeWorkflows).To(HaveLen(1))
			Expect(res.AlternativeWorkflows[0].WorkflowID).To(Equal("wf-scale"))

			Expect(res.ValidationAttemptsHistory).To(HaveLen(1))
			Expect(res.ValidationAttemptsHistory[0].Attempt).To(Equal(1))

			Expect(res.AlignmentVerdict).NotTo(BeNil())
			Expect(res.AlignmentVerdict.Result).To(Equal("aligned"))
			Expect(res.AlignmentVerdict.Findings).To(HaveLen(1))
			Expect(res.AlignmentVerdict.Findings[0].Tool).To(Equal("get_pods"))
		})
	})

	Describe("UT-KA-2327-001: ExecutionClusterID propagates onto the wire selected_workflow map", func() {
		It("should include execution_cluster_id when the workflow declares an execution cluster (DD-FLEET-008, BR-FLEET-004)", func() {
			result := &katypes.InvestigationResult{
				WorkflowID:         "wf-fleet",
				Confidence:         0.9,
				ExecutionClusterID: "prod-east",
			}

			res := agentsession.MapInvestigationResultToAgentSessionResult(logger, result, "incident-fleet")

			Expect(res.SelectedWorkflow).NotTo(BeNil())
			var sw map[string]interface{}
			Expect(json.Unmarshal(res.SelectedWorkflow.Raw, &sw)).To(Succeed())
			Expect(sw["execution_cluster_id"]).To(Equal("prod-east"),
				"AA's response_processor.go reads swMap[\"execution_cluster_id\"] into "+
					"SelectedWorkflow.ExecutionClusterID -- omitting this key here silently "+
					"drops the workflow-declared execution cluster and falls back to the "+
					"signal's origin cluster")
		})

		It("should omit execution_cluster_id when the workflow declares no execution cluster (hub-local default, unchanged behavior)", func() {
			result := &katypes.InvestigationResult{
				WorkflowID: "wf-hub-local",
				Confidence: 0.9,
			}

			res := agentsession.MapInvestigationResultToAgentSessionResult(logger, result, "incident-hub-local")

			Expect(res.SelectedWorkflow).NotTo(BeNil())
			var sw map[string]interface{}
			Expect(json.Unmarshal(res.SelectedWorkflow.Raw, &sw)).To(Succeed())
			Expect(sw).NotTo(HaveKey("execution_cluster_id"))
		})
	})

	Describe("UT-AA-KA-065-011: no workflow selected leaves SelectedWorkflow nil", func() {
		It("should not synthesize a SelectedWorkflow map when WorkflowID is empty", func() {
			result := &katypes.InvestigationResult{RCASummary: "inconclusive", Confidence: 0.2}

			res := agentsession.MapInvestigationResultToAgentSessionResult(logger, result, "incident-2")

			Expect(res.SelectedWorkflow).To(BeNil())
		})
	})

	Describe("UT-AA-KA-065-012: HumanReviewReason mapping — exact known reasons pass through unchanged", func() {
		It("should map an exact known reason string to itself", func() {
			result := &katypes.InvestigationResult{
				RCASummary: "rca incomplete", HumanReviewNeeded: true,
				HumanReviewReason: "workflow_not_found",
			}

			res := agentsession.MapInvestigationResultToAgentSessionResult(logger, result, "incident-3")

			Expect(res.NeedsHumanReview).To(BeTrue())
			Expect(res.HumanReviewReason).To(Equal("workflow_not_found"))
			Expect(res.Warnings).To(ConsistOf("Human review required: workflow_not_found"))
		})
	})

	Describe("UT-KA-2521-002: operator escalation takes precedence over stale RCA review reason", func() {
		It("should preserve operator_escalation when the legacy reason is stale", func() {
			result := &katypes.InvestigationResult{
				RCASummary:        "operator chose escalation",
				HumanReviewNeeded: true,
				HumanReviewReason: katypes.HumanReviewReasonRCAIncomplete,
				Reason:            "operator_escalation",
				TerminalOutcome:   sharedtypes.TerminalOutcomeRCAIncomplete,
			}

			mapped := agentsession.MapInvestigationResultToAgentSessionResult(logger, result, "incident-operator-escalation")

			Expect(mapped.NeedsHumanReview).To(BeTrue())
			Expect(mapped.HumanReviewReason).To(Equal("operator_escalation"),
				"operator escalation must not be downgraded to rca_incomplete")
		})
	})

	Describe("UT-AA-KA-065-013: HumanReviewReason mapping — unrecognized reason falls back safely", func() {
		It("should fall back to investigation_inconclusive for an unrecognized reason string, without panicking", func() {
			result := &katypes.InvestigationResult{
				RCASummary: "something odd happened", HumanReviewNeeded: true,
				HumanReviewReason: "some brand new llm-invented reason nobody has seen before",
			}

			res := agentsession.MapInvestigationResultToAgentSessionResult(logger, result, "incident-4")

			Expect(res.NeedsHumanReview).To(BeTrue())
			Expect(res.HumanReviewReason).To(Equal("investigation_inconclusive"))
		})
	})

	Describe("UT-AA-KA-065-014: no human review needed leaves HumanReviewReason empty", func() {
		It("should leave HumanReviewReason empty when HumanReviewNeeded is false, even if Reason is set", func() {
			result := &katypes.InvestigationResult{RCASummary: "done", Reason: "leftover reason text"}

			res := agentsession.MapInvestigationResultToAgentSessionResult(logger, result, "incident-5")

			Expect(res.NeedsHumanReview).To(BeFalse())
			Expect(res.HumanReviewReason).To(BeEmpty())
		})
	})

	Describe("UT-AA-KA-065-015: an empty RootCauseAnalysis source yields a nil JSON field, never an empty {}", func() {
		It("should return nil RootCauseAnalysis when every contributing sub-field is empty", func() {
			result := &katypes.InvestigationResult{}

			res := agentsession.MapInvestigationResultToAgentSessionResult(logger, result, "incident-6")

			Expect(res.RootCauseAnalysis).To(BeNil())
		})
	})

	Describe("UT-AA-KA-065-016 (issue #2233, supersedes an earlier, less precise nil-guard merged via PR #2222/commit 76bc20270): a nil InvestigationResult synthesizes KA's own \"no result\" placeholder instead of panicking", func() {
		It("should not panic and should produce the exact placeholder AA's isSessionTimedOutWithoutResult already special-cases", func() {
			// #2233 RCA: writeTerminalStatus's StatusCompleted branch
			// (status_writer.go) calls this function unconditionally,
			// including on the CompleteUserDriving disconnect/inactivity-
			// timeout path where no InvestigationResult was ever produced
			// (session/manager_query.go's CompleteUserDriving, called with
			// result=nil by mcp/tools/session_teardown.go on grace-period
			// expiry). Before this fix, r == nil panicked here with a nil
			// pointer dereference, crashing the whole kubernaut-agent
			// process and resetting every other in-flight MCP session on
			// that pod.
			//
			// The synthesized placeholder deliberately mirrors the retired
			// HTTP-polling handler's synthesizeNilResult default branch
			// (internal/kubernautagent/server/handler.go:685-690, #1390) --
			// AA's isSessionTimedOutWithoutResult
			// (pkg/aianalysis/handlers/investigating.go) already detects
			// this exact literal+confidence combination and reclassifies
			// SubReason=InvestigationInconclusive, so no AA-side change is
			// needed to correctly surface this outcome. An earlier,
			// independent nil-guard (PR #2222, commit 76bc20270) used a
			// different ad-hoc literal ("session completed with no
			// investigation result recorded") that prevented the panic but
			// did not match AA's detection literal -- this supersedes it.
			var res *agentsessionv1.AgentSessionResult
			Expect(func() {
				res = agentsession.MapInvestigationResultToAgentSessionResult(logger, nil, "incident-nil")
			}).NotTo(Panic())

			Expect(res).NotTo(BeNil())
			Expect(res.IncidentID).To(Equal("incident-nil"))
			Expect(res.Timestamp).NotTo(BeEmpty())
			Expect(res.Analysis).To(Equal("Investigation completed without result"))
			Expect(res.Confidence).To(Equal(0.0))
			Expect(res.NeedsHumanReview).To(BeFalse())
			Expect(res.RootCauseAnalysis).NotTo(BeNil())
			Expect(res.SelectedWorkflow).To(BeNil())
		})
	})
})

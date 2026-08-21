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

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/agentsession"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
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
})

// SI-10: AgentSessionResult must carry only the curated investigation
// subset, never internal workflow/validation/alignment state verbatim.
var _ = Describe("MapInvestigationResultToAgentSessionResult — SI-10 curated result", func() {
	logger := logr.Discard()

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

	// CI RCA (PR #2222, run 32488044647, E2E-FP fullpipeline must-gather):
	// KA panicked with a nil pointer dereference in buildRootCauseAnalysisMap
	// when Dispatcher.writeTerminalStatus's StatusCompleted branch called
	// this function with result=nil. That is a legitimate, documented call
	// pattern -- session.Manager.CompleteUserDriving's own comment
	// (manager_query.go) states the disconnect/inactivity-timeout handlers
	// call it with result=nil, and finalResult stays nil when no
	// SetPendingDecisionResult was ever attached (e.g. a session released
	// via GracefulSessionClosedHandler before any investigation result
	// existed) -- crashing the whole KA pod (exit code 2, CrashLoopBackOff),
	// unrelated to the AA/AF changes in that PR.
	Describe("UT-AA-KA-065-016: a nil InvestigationResult never panics", func() {
		It("should return a curated, non-nil AgentSessionResult carrying only IncidentID and Timestamp", func() {
			var res *agentsessionv1.AgentSessionResult
			Expect(func() {
				res = agentsession.MapInvestigationResultToAgentSessionResult(logger, nil, "incident-7")
			}).NotTo(Panic())

			Expect(res).NotTo(BeNil())
			Expect(res.IncidentID).To(Equal("incident-7"))
			Expect(res.Timestamp).NotTo(BeEmpty())
			Expect(res.Analysis).NotTo(BeEmpty(), "a curated explanation must be present when no investigation result exists")
			Expect(res.RootCauseAnalysis).To(BeNil())
			Expect(res.SelectedWorkflow).To(BeNil())
		})
	})
})

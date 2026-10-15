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

package capacityretry

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	aaconfig "github.com/jordigilh/kubernaut/internal/config/aianalysis"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/agentsession"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/creator"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// Reuse the existing isolated AgentSession/controller envtest suite. No KA
// subprocess can race the seeded terminal session; the real mapper, CRD admission,
// AgentSessionCreator, InvestigatingHandler and status manager remain in the path.
// BR-KA-016 / BR-AI-088 / DD-KA-2496; SI-10, AU-3, SOC2 CC7.2.
var _ = Describe("#2496 admitted AgentSession selection persistence", func() {
	var (
		ctx  context.Context
		ns   *corev1.Namespace
		kube client.WithWatch
	)
	BeforeEach(func() {
		ctx = context.Background()
		var err error
		kube, err = client.NewWithWatch(cfg, client.Options{Scheme: scheme.Scheme})
		Expect(err).NotTo(HaveOccurred())
		ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "selection-2496-" + randSuffix()}}
		Expect(kube.Create(ctx, ns)).To(Succeed())
	})
	AfterEach(func() {
		Expect(kube.Delete(ctx, ns)).To(Succeed())
	})

	DescribeTable("IT-AA-2496-001: KA mapping survives admitted AgentSession -> real AA reconciliation",
		func(confidence float64, review, rejected bool, phase string) {
			analysis := newCapacityRetryAnalysis(ns.Name, "selection-history", 0, 5*time.Second)
			r := historySelectionResult2496(confidence)
			r.HumanReviewNeeded, r.HumanReviewReason = review, "rca_incomplete"
			if rejected {
				r.WorkflowID, r.ProposedWorkflowID = "", "catalog-id-2496"
				r.RepeatValidationOutcome = "rejected"
				r.RepeatJustification.ExpectedMechanism = ""
			}
			seedHistorySelection2496(ctx, kube, analysis, r)
			_, err := newCapacityRetryReconciler(kube).Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(analysis)})
			Expect(err).NotTo(HaveOccurred())
			persisted := &aianalysisv1.AIAnalysis{}
			Expect(kube.Get(ctx, client.ObjectKeyFromObject(analysis), persisted)).To(Succeed())
			Expect(persisted.Status.Phase).To(Equal(phase), "the existing success/review policy must be preserved")
			Expect(persisted.Status.GetReview().NeedsHumanReview).To(Equal(phase == aianalysisv1.PhaseFailed))
			raw, err := json.Marshal(persisted.Status.GetRCAResult())
			Expect(err).NotTo(HaveOccurred())
			var rca map[string]json.RawMessage
			Expect(json.Unmarshal(raw, &rca)).To(Succeed())
			if rejected {
				Expect(persisted.Status.GetRCAResult().SelectedWorkflow).To(BeNil())
				Expect(rca).To(HaveKey("selectionAssessment"))
				var assessment map[string]json.RawMessage
				Expect(json.Unmarshal(rca["selectionAssessment"], &assessment)).To(Succeed())
				Expect(string(assessment["proposedWorkflowId"])).To(Equal(`"catalog-id-2496"`))
				Expect(string(assessment["repeatValidationOutcome"])).To(Equal(`"rejected"`))
				Expect(string(assessment["repeatJustification"])).To(ContainSubstring("New memory limit observed"))
				return
			}
			sw := persisted.Status.GetRCAResult().SelectedWorkflow
			Expect(sw).NotTo(BeNil())
			Expect(sw.WorkflowID).To(Equal(r.WorkflowID))
			Expect(sw.WorkflowName).To(Equal(r.WorkflowName))
			Expect(sw.Confidence).To(Equal(confidence))
			Expect(sw.SelectedAt).NotTo(BeNil())
			var selected map[string]json.RawMessage
			Expect(json.Unmarshal(rca["selectedWorkflow"], &selected)).To(Succeed())
			Expect(string(selected["repeatJustification"])).To(MatchJSON(`{"evidence_basis":"New memory limit observed","prior_outcome_relevance":"Prior Full assessment was ineffective","expected_mechanism":"Increase the current limit"}`))
			Expect(string(selected["repeatJustificationRequired"])).To(Equal("true"))
			Expect(string(selected["repeatValidationOutcome"])).To(Equal(`"accepted"`))
			Expect(string(selected["handoffDigest"])).To(Equal(`"` + r.HandoffDigest + `"`))
			Expect(string(selected["priorRemediationUIDs"])).To(MatchJSON(`["trusted-prior-2496"]`))
			// DD-WORKFLOW-018: identical writes are still admitted; added
			// selection reasoning freezes with the existing selectedAt marker.
			Expect(kube.Status().Update(ctx, persisted)).To(Succeed())
			for field, mutation := range map[string]json.RawMessage{
				"repeatJustification":         json.RawMessage(`{"evidence_basis":"tampered","prior_outcome_relevance":"Prior Full assessment was ineffective","expected_mechanism":"Increase the current limit"}`),
				"workflowSelectionConfidence": json.RawMessage(`0.01`),
				"repeatJustificationRequired": json.RawMessage(`false`),
				"priorWorkflowMatch":          json.RawMessage(`false`),
				"repeatValidationOutcome":     json.RawMessage(`"rejected"`),
				"handoffDigest":               json.RawMessage(`"sha256:` + strings.Repeat("c", 64) + `"`),
				"priorRemediationUIDs":        json.RawMessage(`["tampered-prior"]`),
			} {
				for _, remove := range []bool{false, true} {
					candidate := persisted.DeepCopy()
					var modified map[string]json.RawMessage
					Expect(json.Unmarshal(rca["selectedWorkflow"], &modified)).To(Succeed())
					if remove {
						delete(modified, field)
					} else {
						modified[field] = mutation
					}
					raw, err := json.Marshal(modified)
					Expect(err).NotTo(HaveOccurred())
					candidate.Status.GetRCAResult().SelectedWorkflow = &aianalysisv1.SelectedWorkflow{}
					Expect(json.Unmarshal(raw, candidate.Status.GetRCAResult().SelectedWorkflow)).To(Succeed())
					Expect(kube.Status().Update(ctx, candidate)).NotTo(Succeed(), "frozen selection metadata must reject change/removal of "+field)
				}
			}
		},
		Entry("successful selection", .84, false, false, aianalysisv1.PhaseAnalyzing),
		Entry("explicit-zero confidence review", 0., false, false, aianalysisv1.PhaseFailed),
		Entry("partial explicit human-review selection", .84, true, false, aianalysisv1.PhaseFailed),
		Entry("rejected incomplete reasoning", .84, true, true, aianalysisv1.PhaseFailed),
	)

	// This is regression proof of already implemented configuration/handler
	// wiring, not a new approval policy or a substitute for the deployed RAR journey.
	DescribeTable("IT-AA-2496-004: configured confidence gates remain independent of repeat reasoning [BR-AI-088, BR-AI-003]",
		func(confidence, floor, threshold float64, review, approval bool) {
			policyPath, err := filepath.Abs(filepath.Join("..", "testdata", "policies", "approval.rego"))
			Expect(err).NotTo(HaveOccurred())
			configPath := filepath.Join(GinkgoT().TempDir(), "aianalysis.yaml")
			yaml := fmt.Sprintf("rego:\n  policyPath: %q\n  lowConfidenceFloor: %g\n  confidenceThreshold: %g\n", policyPath, floor, threshold)
			Expect(os.WriteFile(configPath, []byte(yaml), 0600)).To(Succeed())
			config, err := aaconfig.LoadFromFile(configPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(config.Rego.LowConfidenceFloor).To(HaveValue(Equal(floor)))
			Expect(config.Rego.ConfidenceThreshold).To(HaveValue(Equal(threshold)))

			r := newCapacityRetryReconciler(kube)
			evaluator := rego.NewEvaluator(rego.Config{PolicyPath: config.Rego.PolicyPath}, r.Log)
			Expect(evaluator.StartHotReload(ctx)).To(Succeed())
			DeferCleanup(evaluator.Stop)
			// Same loader/options/handlers as cmd/aianalysis/main.go. Keep all
			// business logic real; only external audit persistence is substituted.
			r.InvestigatingHandler.Store(handlers.NewInvestigatingHandler(
				creator.NewAgentSessionCreator(kube, kube.Scheme()), r.Log, r.Metrics, r.AuditClient,
				handlers.WithLowConfidenceFloor(config.Rego.LowConfidenceFloor)))
			r.AnalyzingHandler = handlers.NewAnalyzingHandler(evaluator, r.Log, r.Metrics, r.AuditClient).
				WithConfidenceThreshold(config.Rego.ConfidenceThreshold)
			analysis := newCapacityRetryAnalysis(ns.Name, "configured-repeat", 0, 5*time.Second)
			analysis.Spec.AnalysisRequest.SignalContext.Environment = "production"
			result := historySelectionResult2496(confidence)
			rcaConfidence := .99
			result.RCAConfidence = &rcaConfidence
			result.RemediationTarget = katypes.RemediationTarget{APIVersion: "apps/v1", Kind: "Deployment", Name: "test-deploy", Namespace: ns.Name}
			seedHistorySelection2496(ctx, kube, analysis, result)
			request := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(analysis)}
			_, err = r.Reconcile(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			persisted := &aianalysisv1.AIAnalysis{}
			Expect(kube.Get(ctx, request.NamespacedName, persisted)).To(Succeed())
			selected := persisted.Status.GetRCAResult().SelectedWorkflow
			Expect(selected).NotTo(BeNil())
			Expect(selected.Confidence).To(Equal(confidence), "high RCA confidence cannot replace the current selection score")
			Expect(selected.WorkflowSelectionConfidence).To(HaveValue(Equal(confidence)))
			Expect(selected.RepeatJustification).To(Equal(result.RepeatJustification))
			Expect(selected.RepeatJustificationRequired).To(HaveValue(BeTrue()))
			Expect(selected.PriorRemediationUIDs).To(Equal(result.PriorRemediationUIDs))
			Expect(persisted.Status.GetReview().NeedsHumanReview).To(Equal(review))
			if review {
				Expect(persisted.Status.Phase).To(Equal(aianalysisv1.PhaseFailed))
				Expect(persisted.Status.GetReview().HumanReviewReason).To(Equal("low_confidence"))
				return
			}
			Expect(persisted.Status.Phase).To(Equal(aianalysisv1.PhaseAnalyzing))
			_, err = r.Reconcile(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			Expect(kube.Get(ctx, request.NamespacedName, persisted)).To(Succeed())
			Expect(persisted.Status.Phase).To(Equal(aianalysisv1.PhaseCompleted))
			Expect(persisted.Status.GetApproval().ApprovalRequired).To(Equal(approval))
			Expect(persisted.Status.GetApproval().ApprovalReason).NotTo(BeEmpty())
			Expect(persisted.Status.GetInvestigationMetadata().DegradedMode).To(BeFalse())
			Expect(persisted.Status.GetRCAResult().SelectedWorkflow).To(Equal(selected), "Rego must not rewrite the immutable selection context")
		},
		Entry("raised floor blocks a repeat despite high RCA confidence", .84, .9, .95, true, false),
		Entry("lowered floor admits a repeat below the default floor but normal Rego still requires approval", .65, .6, .8, false, true),
		Entry("raised Rego threshold requires approval despite complete reasoning", .84, .7, .9, false, true),
		Entry("configured threshold permits normal auto-approval", .84, .7, .8, false, false),
		Entry("the exact configured floor is admitted independently of the later threshold", .84, .84, .85, false, true),
	)
})

func historySelectionResult2496(confidence float64) *katypes.InvestigationResult {
	required, matched, rcaConfidence := true, true, .93
	return &katypes.InvestigationResult{
		RCASummary: "Memory pressure", RCAConfidence: &rcaConfidence, Confidence: confidence,
		WorkflowID: "catalog-id-2496", WorkflowName: "increase-memory", WorkflowVersion: "1.0.0", ActionType: "IncreaseMemory",
		ExecutionBundle:   "ghcr.io/kubernaut/increase-memory@sha256:" + strings.Repeat("a", 64),
		WorkflowRationale: "Current memory pressure warrants selection", WorkflowSelectionConfidence: &confidence,
		RepeatJustification: &sharedtypes.RepeatJustification{EvidenceBasis: "New memory limit observed",
			PriorOutcomeRelevance: "Prior Full assessment was ineffective", ExpectedMechanism: "Increase the current limit"},
		RepeatJustificationRequired: &required, PriorWorkflowMatch: &matched, RepeatValidationOutcome: "accepted",
		PriorRemediationUIDs: []string{"trusted-prior-2496"}, HandoffDigest: "sha256:" + strings.Repeat("b", 64),
	}
}

func seedHistorySelection2496(ctx context.Context, kube client.WithWatch, analysis *aianalysisv1.AIAnalysis, result *katypes.InvestigationResult) {
	GinkgoHelper()
	desiredStatus := analysis.Status
	Expect(kube.Create(ctx, analysis)).To(Succeed())
	analysis.Status = desiredStatus
	Expect(kube.Status().Update(ctx, analysis)).To(Succeed())
	as := &agentsessionv1.AgentSession{ObjectMeta: metav1.ObjectMeta{
		Name: "as-" + analysis.Spec.RemediationRequestRef.Name, Namespace: analysis.Namespace},
		Spec: handlers.NewRequestBuilder(logr.Discard()).BuildAgentSessionSpec(analysis)}
	Expect(controllerutil.SetControllerReference(analysis, as, kube.Scheme())).To(Succeed())
	Expect(kube.Create(ctx, as)).To(Succeed())
	as.Status.Phase = agentsessionv1.AgentSessionPhaseCompleted
	as.Status.SessionID = "incident-2496"
	as.Status.Result = agentsession.MapInvestigationResultToAgentSessionResult(logr.Discard(), result, as.Spec.IncidentID)
	completed := metav1.Now()
	as.Status.CompletedAt = &completed
	Expect(kube.Status().Update(ctx, as)).To(Succeed())
}

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

package aianalysis_test

import (
	"context"
	"path/filepath"
	"runtime"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// UT-AA-2714: Selected workflow identity in the Rego policy input
// (Issue #2714, BR-AI-085 FR-AI-085-007).
//
// Control objectives:
//   - FedRAMP AC-6 (least privilege): selective autonomy can be scoped to a
//     specific reviewed workflow (name+version) instead of a whole action type.
//   - FedRAMP AC-4/SI-10: the identity is catalog-authoritative
//     (WorkflowSnapshot, DD-WORKFLOW-018), never LLM-suppliable.
var _ = Describe("Rego Selected Workflow Input — #2714, BR-AI-085 FR-AI-085-007", func() {

	var (
		evaluator *rego.Evaluator
	)

	getTestdataPath := func(subpath string) string {
		_, filename, _, _ := runtime.Caller(0)
		dir := filepath.Dir(filename)
		return filepath.Join(dir, "testdata", subpath)
	}

	BeforeEach(func() {
		evaluator = rego.NewEvaluator(rego.Config{
			PolicyPath: getTestdataPath("policies/selected_workflow_approval.rego"),
		}, logr.Discard())
	})

	Describe("UT-AA-2714-001: Rego evaluator auto-approves trusted workflow identity (policy checks input.selected_workflow)", func() {
		It("should auto-approve when selected_workflow matches the trusted name and version", func() {
			input := &rego.PolicyInput{
				SignalContext: rego.SignalContextInput{SignalType: "alert", Severity: "high", Environment: "production"},
				KAResponse:    rego.KAResponseInput{Confidence: 0.95},
				ActionType:    "ScaleMemory",
				SelectedWorkflow: &rego.SelectedWorkflowInput{
					ID:      "a93e5f6a-d967-510d-8faa-9a83c5420e78",
					Name:    "increase-memory-limits-gitops-v1",
					Version: "1.4.1",
				},
			}

			result, err := evaluator.Evaluate(context.Background(), input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.ApprovalRequired).To(BeFalse(),
				"trusted workflow identity should auto-approve per selected_workflow_approval.rego")
			Expect(result.Reason).To(ContainSubstring("trusted workflow identity"))
		})
	})

	Describe("UT-AA-2714-003: Untrusted workflow identity requires approval", func() {
		It("should require approval when the workflow name is not trusted", func() {
			input := &rego.PolicyInput{
				SignalContext: rego.SignalContextInput{SignalType: "alert", Severity: "high", Environment: "production"},
				KAResponse:    rego.KAResponseInput{Confidence: 0.95},
				ActionType:    "ScaleMemory",
				SelectedWorkflow: &rego.SelectedWorkflowInput{
					ID:      "11111111-2222-3333-4444-555555555555",
					Name:    "restart-pod-v1",
					Version: "1.4.1",
				},
			}

			result, err := evaluator.Evaluate(context.Background(), input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.ApprovalRequired).To(BeTrue(),
				"untrusted workflow name should require approval")
		})

		It("should require approval when the workflow version is not trusted", func() {
			input := &rego.PolicyInput{
				SignalContext: rego.SignalContextInput{SignalType: "alert", Severity: "high", Environment: "production"},
				KAResponse:    rego.KAResponseInput{Confidence: 0.95},
				ActionType:    "ScaleMemory",
				SelectedWorkflow: &rego.SelectedWorkflowInput{
					ID:      "a93e5f6a-d967-510d-8faa-9a83c5420e78",
					Name:    "increase-memory-limits-gitops-v1",
					Version: "0.9.0-unreviewed",
				},
			}

			result, err := evaluator.Evaluate(context.Background(), input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.ApprovalRequired).To(BeTrue(),
				"untrusted workflow version should require approval — selective autonomy is version-scoped")
		})
	})

	Describe("UT-AA-2714-004: Nil selected workflow handled gracefully (backward compatibility)", func() {
		It("should not panic and should keep policy default behavior when SelectedWorkflow is nil", func() {
			input := &rego.PolicyInput{
				SignalContext: rego.SignalContextInput{SignalType: "alert", Severity: "warning", Environment: "staging"},
				KAResponse:    rego.KAResponseInput{Confidence: 0.90},
				ActionType:    "RestartPod",
				// SelectedWorkflow intentionally nil — pre-#2714 input shape
			}

			result, err := evaluator.Evaluate(context.Background(), input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.ApprovalRequired).To(BeTrue(),
				"absent selected_workflow must not auto-approve via workflow-identity rules")
			Expect(result.Degraded).To(BeFalse(),
				"absent selected_workflow is a valid input shape, not a degraded evaluation")
		})
	})

	Describe("UT-AA-2714-002: buildPolicyInput populates selected workflow identity from the catalog snapshot (production dispatch path)", func() {
		var (
			handler *handlers.AnalyzingHandler
			spy     *spyEvaluator
			ctx     context.Context
			cancel  context.CancelFunc
		)

		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())
			DeferCleanup(cancel)
			spy = newSpyEvaluator(ctx, "testdata/policies/approval.rego")
			handler = handlers.NewAnalyzingHandler(spy, ctrl.Log.WithName("test"), metrics.NewMetrics(), &noopAnalyzingAuditClient{})
		})

		It("should copy WorkflowID, WorkflowName and Version from Status.RCAResult.SelectedWorkflow", func() {
			analysis := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{Name: "test-analysis-2714", Namespace: "default"},
				Spec: aianalysisv1.AIAnalysisSpec{
					AnalysisRequest: aianalysisv1.AnalysisRequest{
						SignalContext: aianalysisv1.SignalContextInput{
							SignalName:  "OOMKilled",
							Severity:    "warning",
							Environment: "production",
						},
					},
				},
				Status: aianalysisv1.AIAnalysisStatus{
					Phase: aianalysis.PhaseAnalyzing,
					RCAResult: &aianalysisv1.RCAResult{
						SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
							// Catalog-authoritative WorkflowSnapshot (DD-WORKFLOW-018):
							// these values are DataStorage-resolved, never LLM-suppliable.
							WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
								WorkflowID:      "a93e5f6a-d967-510d-8faa-9a83c5420e78",
								WorkflowName:    "increase-memory-limits-gitops-v1",
								ActionType:      "ScaleMemory",
								Version:         "1.4.1",
								ExecutionBundle: "kubernaut.io/workflows/increase-memory:v1.4.1",
							},
							Confidence: 0.92,
							Rationale:  "Reviewed GitOps memory increase",
						},
					},
				},
			}

			_, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())

			Expect(spy.LastInput).NotTo(BeNil(), "Handle must dispatch through the Rego evaluator")
			Expect(spy.LastInput.SelectedWorkflow).NotTo(BeNil(),
				"buildPolicyInput must expose the selected workflow identity to Rego")
			Expect(spy.LastInput.SelectedWorkflow.ID).To(Equal("a93e5f6a-d967-510d-8faa-9a83c5420e78"))
			Expect(spy.LastInput.SelectedWorkflow.Name).To(Equal("increase-memory-limits-gitops-v1"))
			Expect(spy.LastInput.SelectedWorkflow.Version).To(Equal("1.4.1"))
			// Backward compatibility: action_type stays populated from the same snapshot.
			Expect(spy.LastInput.ActionType).To(Equal("ScaleMemory"))
		})
	})
})

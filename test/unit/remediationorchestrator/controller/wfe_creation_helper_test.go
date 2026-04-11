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

package controller

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	prometheus "github.com/prometheus/client_golang/prometheus"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	prodcontroller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
)

var _ = Describe("Issue #666: WFE Creation Helper (TP-666-v1 §8.3)", func() {

	var (
		ctx    context.Context
		scheme *runtime.Scheme
		m      *rometrics.Metrics
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		m = rometrics.NewMetricsWithRegistry(prometheus.NewRegistry())
	})

	minimalAI := func(workflowID string, confidence float64) *aianalysisv1.AIAnalysis {
		return &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{Name: "ai-test", Namespace: "default"},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase: "Completed",
				SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
					WorkflowID: workflowID,
					ActionType: "patch",
					Confidence: confidence,
				},
				RootCauseAnalysis: &aianalysisv1.RootCauseAnalysis{
					RemediationTarget: &aianalysisv1.RemediationTarget{
						Kind:      "Deployment",
						Name:      "my-app",
						Namespace: "default",
					},
				},
			},
		}
	}

	noopWFECallbacks := func() prodcontroller.WFECreationCallbacks {
		return prodcontroller.WFECreationCallbacks{
			EmitWorkflowCreatedAudit: func(_ context.Context, _ *remediationv1.RemediationRequest, _ *aianalysisv1.AIAnalysis, _ string) {},
			CreateWFE:                func(_ context.Context, _ *remediationv1.RemediationRequest, _ *aianalysisv1.AIAnalysis) (string, error) { return "wfe-test", nil },
			ResolveWorkflowName:      func(_ context.Context, workflowID string) string { return workflowID },
		}
	}

	It("UT-WEC-001: WFE created successfully → Advance to Executing", func() {
		rr := newRemediationRequest("wec-001", "default", remediationv1.PhaseAnalyzing)
		rr.Status.AIAnalysisRef = &corev1.ObjectReference{Name: "ai-test", Namespace: "default"}
		ai := minimalAI("wf-restart", 0.95)

		c := fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(rr).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			Build()

		cbs := noopWFECallbacks()
		auditEmitted := false
		cbs.EmitWorkflowCreatedAudit = func(_ context.Context, _ *remediationv1.RemediationRequest, _ *aianalysisv1.AIAnalysis, _ string) {
			auditEmitted = true
		}

		intent, err := prodcontroller.CreateWFEAndTransition(ctx, c, m, rr, ai, "hash123", cbs)
		Expect(err).ToNot(HaveOccurred())
		Expect(intent.Type).To(Equal(phase.TransitionAdvance))
		Expect(intent.TargetPhase).To(Equal(phase.Executing))
		Expect(auditEmitted).To(BeTrue(), "should emit workflow_created audit")
	})

	It("UT-WEC-002: WFE creation fails → RequeueAfter", func() {
		rr := newRemediationRequest("wec-002", "default", remediationv1.PhaseAnalyzing)
		rr.Status.AIAnalysisRef = &corev1.ObjectReference{Name: "ai-test", Namespace: "default"}
		ai := minimalAI("wf-restart", 0.95)

		c := fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(rr).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			Build()

		cbs := noopWFECallbacks()
		cbs.CreateWFE = func(_ context.Context, _ *remediationv1.RemediationRequest, _ *aianalysisv1.AIAnalysis) (string, error) {
			return "", fmt.Errorf("namespace terminating")
		}

		intent, err := prodcontroller.CreateWFEAndTransition(ctx, c, m, rr, ai, "", cbs)
		Expect(err).ToNot(HaveOccurred())
		Expect(intent.Type).To(Equal(phase.TransitionNone))
		Expect(intent.RequeueAfter).To(BeNumerically(">", 0), "should requeue on WFE creation failure")
	})

	It("UT-WEC-003: Status update fails → RequeueAfter", func() {
		rr := newRemediationRequest("wec-003", "default", remediationv1.PhaseAnalyzing)
		rr.Status.AIAnalysisRef = &corev1.ObjectReference{Name: "ai-test", Namespace: "default"}
		ai := minimalAI("wf-restart", 0.95)

		// Use a client without the RR object so status update will fail
		c := fake.NewClientBuilder().WithScheme(scheme).Build()

		cbs := noopWFECallbacks()
		intent, err := prodcontroller.CreateWFEAndTransition(ctx, c, m, rr, ai, "", cbs)
		Expect(err).ToNot(HaveOccurred())
		Expect(intent.Type).To(Equal(phase.TransitionNone))
		Expect(intent.RequeueAfter).To(BeNumerically(">", 0), "should requeue on status update failure")
	})

	It("UT-WEC-004: sets WorkflowExecutionRef and SelectedWorkflowRef in status", func() {
		rr := newRemediationRequest("wec-004", "default", remediationv1.PhaseAnalyzing)
		rr.Status.AIAnalysisRef = &corev1.ObjectReference{Name: "ai-test", Namespace: "default"}
		ai := minimalAI("wf-restart", 0.95)

		c := fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(rr).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			Build()

		cbs := noopWFECallbacks()
		cbs.CreateWFE = func(_ context.Context, _ *remediationv1.RemediationRequest, _ *aianalysisv1.AIAnalysis) (string, error) {
			return "wfe-created", nil
		}
		cbs.ResolveWorkflowName = func(_ context.Context, workflowID string) string {
			return "Human-Friendly-" + workflowID
		}

		_, err := prodcontroller.CreateWFEAndTransition(ctx, c, m, rr, ai, "hash456", cbs)
		Expect(err).ToNot(HaveOccurred())

		// Refetch RR to check persisted status
		updated := &remediationv1.RemediationRequest{}
		Expect(c.Get(ctx, client.ObjectKeyFromObject(rr), updated)).To(Succeed())
		Expect(updated.Status.WorkflowExecutionRef).To(HaveField("Name", Equal("wfe-created")))
		Expect(updated.Status.SelectedWorkflowRef).To(HaveField("WorkflowID", Equal("wf-restart")))
	})

	It("UT-WEC-005: increments ChildCRDCreationsTotal metric", func() {
		rr := newRemediationRequest("wec-005", "default", remediationv1.PhaseAnalyzing)
		rr.Status.AIAnalysisRef = &corev1.ObjectReference{Name: "ai-test", Namespace: "default"}
		ai := minimalAI("wf-restart", 0.95)

		c := fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(rr).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			Build()

		reg := prometheus.NewRegistry()
		localMetrics := rometrics.NewMetricsWithRegistry(reg)

		cbs := noopWFECallbacks()
		_, err := prodcontroller.CreateWFEAndTransition(ctx, c, localMetrics, rr, ai, "", cbs)
		Expect(err).ToNot(HaveOccurred())

		families, collectErr := reg.Gather()
		Expect(collectErr).ToNot(HaveOccurred())
		found := false
		for _, fam := range families {
			if fam.GetName() == "kubernaut_remediationorchestrator_child_crd_creations_total" {
				for _, metric := range fam.GetMetric() {
					for _, label := range metric.GetLabel() {
						if label.GetName() == "child_type" && label.GetValue() == "WorkflowExecution" {
							found = true
							Expect(metric.GetCounter().GetValue()).To(Equal(1.0))
						}
					}
				}
			}
		}
		Expect(found).To(BeTrue(), "ChildCRDCreationsTotal metric should be incremented")
	})

	It("UT-WEC-006: handles nil SelectedWorkflow gracefully", func() {
		rr := newRemediationRequest("wec-006", "default", remediationv1.PhaseAnalyzing)
		rr.Status.AIAnalysisRef = &corev1.ObjectReference{Name: "ai-test", Namespace: "default"}
		ai := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{Name: "ai-test", Namespace: "default"},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase: "Completed",
			},
		}

		c := fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(rr).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			Build()

		cbs := noopWFECallbacks()
		intent, err := prodcontroller.CreateWFEAndTransition(ctx, c, m, rr, ai, "", cbs)
		Expect(err).ToNot(HaveOccurred())
		Expect(intent.Type).To(Equal(phase.TransitionAdvance))
		Expect(intent.TargetPhase).To(Equal(phase.Executing))
	})
})

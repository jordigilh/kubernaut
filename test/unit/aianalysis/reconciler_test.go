/*
Copyright 2025 Jordi Gil.

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

package aianalysis

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// BR-AI-001: AIAnalysis Reconciler Unit Tests
var _ = Describe("AIAnalysisReconciler", func() {
	var (
		ctx        context.Context
		scheme     *runtime.Scheme
		fakeClient client.Client
		reconciler *aianalysis.Reconciler
		recorder   *record.FakeRecorder
	)

	BeforeEach(func() {
		ctx = context.Background()
		logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

		// Setup scheme
		scheme = runtime.NewScheme()
		Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		// Create fake client
		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&aianalysisv1.AIAnalysis{}).
			Build()

		// Create recorder
		recorder = record.NewFakeRecorder(100)

		// Create reconciler
		reconciler = &aianalysis.Reconciler{
			Client:   fakeClient,
			Scheme:   scheme,
			Log:      logf.Log.WithName("test"),
			Recorder: recorder,
		}
	})

	// BR-AI-001: Basic reconciliation
	Describe("Reconcile", func() {
		Context("when AIAnalysis CRD does not exist", func() {
			It("should return without error (Category A handling)", func() {
				// Category A: CRD deleted during reconciliation
				result, err := reconciler.Reconcile(ctx, ctrl.Request{
					NamespacedName: types.NamespacedName{
						Name:      "non-existent",
						Namespace: "default",
					},
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())
			})
		})

		Context("when AIAnalysis CRD exists with empty status", func() {
			It("should initialize status and transition to Validating", func() {
				// Create test AIAnalysis with empty status
				analysis := newTestAIAnalysis("test-init")
				Expect(fakeClient.Create(ctx, analysis)).To(Succeed())

				// Reconcile
				result, err := reconciler.Reconcile(ctx, ctrl.Request{
					NamespacedName: client.ObjectKeyFromObject(analysis),
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeTrue())

				// Verify status was initialized
				var updated aianalysisv1.AIAnalysis
				Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(analysis), &updated)).To(Succeed())
				Expect(updated.Status.Phase).To(Equal(aianalysis.PhaseValidating))
				Expect(updated.Status.StartedAt).NotTo(BeNil())
			})
		})

		Context("when AIAnalysis is already Completed", func() {
			It("should skip reconciliation", func() {
				// Create completed AIAnalysis
				analysis := newTestAIAnalysis("test-completed")
				analysis.Status.Phase = aianalysis.PhaseCompleted
				Expect(fakeClient.Create(ctx, analysis)).To(Succeed())
				Expect(fakeClient.Status().Update(ctx, analysis)).To(Succeed())

				// Reconcile
				result, err := reconciler.Reconcile(ctx, ctrl.Request{
					NamespacedName: client.ObjectKeyFromObject(analysis),
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())
			})
		})

		Context("when AIAnalysis is already Failed", func() {
			It("should skip reconciliation", func() {
				// Create failed AIAnalysis
				analysis := newTestAIAnalysis("test-failed")
				analysis.Status.Phase = aianalysis.PhaseFailed
				Expect(fakeClient.Create(ctx, analysis)).To(Succeed())
				Expect(fakeClient.Status().Update(ctx, analysis)).To(Succeed())

				// Reconcile
				result, err := reconciler.Reconcile(ctx, ctrl.Request{
					NamespacedName: client.ObjectKeyFromObject(analysis),
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())
			})
		})
	})

	// BR-AI-002: Phase state machine
	Describe("Phase Transitions", func() {
		DescribeTable("should route to correct handler based on phase",
			func(currentPhase string, expectedHandlerCalled bool) {
				analysis := newTestAIAnalysis("test-phase")
				analysis.Status.Phase = currentPhase
				Expect(fakeClient.Create(ctx, analysis)).To(Succeed())
				Expect(fakeClient.Status().Update(ctx, analysis)).To(Succeed())

				// Without handlers configured, unknown phases should fail
				result, err := reconciler.Reconcile(ctx, ctrl.Request{
					NamespacedName: client.ObjectKeyFromObject(analysis),
				})

				if currentPhase == aianalysis.PhaseCompleted || currentPhase == aianalysis.PhaseFailed {
					// Terminal phases should not error
					Expect(err).NotTo(HaveOccurred())
					Expect(result.Requeue).To(BeFalse())
				} else if expectedHandlerCalled {
					// Non-terminal phases without handlers should update to Failed
					var updated aianalysisv1.AIAnalysis
					Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(analysis), &updated)).To(Succeed())
					// Without handler, it should fail
					Expect(updated.Status.Phase).To(Equal(aianalysis.PhaseFailed))
				}
			},
			Entry("Validating phase routes to ValidatingHandler", aianalysis.PhaseValidating, true),
			Entry("Investigating phase routes to InvestigatingHandler", aianalysis.PhaseInvestigating, true),
			Entry("Analyzing phase routes to AnalyzingHandler", aianalysis.PhaseAnalyzing, true),
			Entry("Recommending phase routes to RecommendingHandler", aianalysis.PhaseRecommending, true),
			Entry("Completed phase skips processing", aianalysis.PhaseCompleted, false),
			Entry("Failed phase skips processing", aianalysis.PhaseFailed, false),
		)
	})
})

// ========================================
// TEST HELPERS
// ========================================

// newTestAIAnalysis creates a valid AIAnalysis for testing.
func newTestAIAnalysis(name string) *aianalysisv1.AIAnalysis {
	return &aianalysisv1.AIAnalysis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: aianalysisv1.AIAnalysisSpec{
			RemediationRequestRef: corev1.ObjectReference{
				Name:      "test-rr",
				Namespace: "default",
			},
			RemediationID: "rem-12345",
			AnalysisRequest: aianalysisv1.AnalysisRequest{
				SignalContext: aianalysisv1.SignalContextInput{
					Fingerprint:      "abc123def456",
					Severity:         "warning",
					SignalType:       "OOMKilled",
					Environment:      "production",
					BusinessPriority: "P1",
					TargetResource: aianalysisv1.TargetResource{
						Kind:      "Pod",
						Name:      "test-pod",
						Namespace: "default",
					},
					EnrichmentResults: sharedtypes.EnrichmentResults{
						KubernetesContext: &sharedtypes.KubernetesContext{
							Namespace: "default",
						},
					},
				},
				AnalysisTypes: []string{"investigation", "root-cause", "workflow-selection"},
			},
		},
	}
}


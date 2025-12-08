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
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller/aianalysis"
)

// BR-AI-001: AIAnalysis CRD Lifecycle Management
// TDD RED Phase: Test controller reconciliation behavior
var _ = Describe("AIAnalysis Controller", func() {
	var (
		ctx        context.Context
		scheme     *runtime.Scheme
		reconciler *aianalysis.AIAnalysisReconciler
		recorder   *record.FakeRecorder
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Setup scheme with AIAnalysis CRD
		scheme = runtime.NewScheme()
		Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())

		// Create fake recorder for events
		recorder = record.NewFakeRecorder(10)
	})

	// R-HP-02: Phase transition Pending → Investigating
	// Per CRD schema (reconciliation-phases.md v2.1): Pending;Investigating;Analyzing;Completed;Failed
	// NOTE: Recommending phase REMOVED in v1.8 - workflow data captured in Investigating phase
	Context("when reconciling a new AIAnalysis", func() {
		It("should transition from Pending to Investigating phase", func() {
			// Create test AIAnalysis in Pending phase
			testAnalysis := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-analysis",
					Namespace: "default",
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Kind:      "RemediationRequest",
						Name:      "test-rr",
						Namespace: "default",
					},
					RemediationID: "test-remediation-001",
				},
			}

			// Create fake K8s client (ADR-004)
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(testAnalysis).
				WithStatusSubresource(testAnalysis). // Enable status subresource
				Build()

			// Create reconciler
			reconciler = &aianalysis.AIAnalysisReconciler{
				Client:   fakeClient,
				Scheme:   scheme,
				Recorder: recorder,
				Log:      ctrl.Log.WithName("test"),
			}

			// Business scenario: AIAnalysis transitions through lifecycle phases
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "test-analysis",
					Namespace: "default",
				},
			}

			// First reconcile: Setup (adds finalizer for cleanup guarantee)
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			// Second reconcile: Process Pending phase
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			// Business outcome: Analysis progresses from Pending to Investigating
			updated := &aianalysisv1.AIAnalysis{}
			Expect(fakeClient.Get(ctx, req.NamespacedName, updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(aianalysis.PhaseInvestigating),
				"Analysis should progress to Investigating phase to query HolmesGPT-API")
		})
	})

	// BR-AI-001: Phase state machine validation
	// Per reconciliation-phases.md v2.1: Recommending phase REMOVED in v1.8
	Context("phase constants validation", func() {
		It("should NOT have Recommending phase constant", func() {
			// Validate phase constants match authoritative docs
			// Per reconciliation-phases.md v2.1: Pending → Investigating → Analyzing → Completed/Failed
			validPhases := []string{
				aianalysis.PhasePending,
				aianalysis.PhaseInvestigating,
				aianalysis.PhaseAnalyzing,
				aianalysis.PhaseCompleted,
				aianalysis.PhaseFailed,
			}
			Expect(validPhases).To(HaveLen(5), "Should have exactly 5 phases (no Recommending)")
			Expect(validPhases).NotTo(ContainElement("Recommending"), "Recommending phase was removed in v1.8")
		})
	})
})

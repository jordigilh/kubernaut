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
	aiaudit "github.com/jordigilh/kubernaut/pkg/aianalysis/audit"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

// BR-AI-012, BR-AI-030: Nil-safety for critical AA controller dependencies
// Issue #1116: AA controller must fail-fast when core dependencies are nil
var _ = Describe("AIAnalysis Controller Nil Safety (#1116)", func() {

	Context("UT-AA-1116-001: NewAnalyzingHandler nil evaluator", func() {
		It("MUST panic when RegoEvaluatorInterface is nil", func() {
			testMetrics := metrics.NewMetrics()
			mockAuditStore := NewMockAuditStore()
			auditClient := aiaudit.NewAuditClient(mockAuditStore, ctrl.Log.WithName("test-audit"))

			Expect(func() {
				handlers.NewAnalyzingHandler(
					nil, // nil evaluator — must panic
					ctrl.Log.WithName("test"),
					testMetrics,
					auditClient,
				)
			}).To(PanicWith(ContainSubstring("evaluator")))
		})
	})

	Context("UT-AA-1116-002: SetupWithManager rejects nil AnalyzingHandler", func() {
		It("MUST return error when AnalyzingHandler is nil", func() {
			scheme := runtime.NewScheme()
			Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())

			testMetrics := metrics.NewMetrics()
			mockAuditStore := NewMockAuditStore()
			auditClient := aiaudit.NewAuditClient(mockAuditStore, ctrl.Log.WithName("test-audit"))
			mockHolmesClient := mocks.NewMockAgentClient()

			investigatingHandler := handlers.NewInvestigatingHandler(
				mockHolmesClient,
				ctrl.Log.WithName("test"),
				testMetrics,
				auditClient,
			)

			reconciler := &aianalysis.AIAnalysisReconciler{
				Log:                  ctrl.Log.WithName("test"),
				Metrics:              testMetrics,
				InvestigatingHandler: investigatingHandler,
				AnalyzingHandler:     nil, // nil — must be rejected
				AuditClient:          auditClient,
			}

			err := reconciler.ValidateDependencies()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("AnalyzingHandler"))
		})
	})

	Context("UT-AA-1116-003: SetupWithManager rejects nil InvestigatingHandler", func() {
		It("MUST return error when InvestigatingHandler is nil", func() {
			testMetrics := metrics.NewMetrics()
			mockAuditStore := NewMockAuditStore()
			auditClient := aiaudit.NewAuditClient(mockAuditStore, ctrl.Log.WithName("test-audit"))
			mockRegoEvaluator := mocks.NewMockRegoEvaluator()

			analyzingHandler := handlers.NewAnalyzingHandler(
				mockRegoEvaluator,
				ctrl.Log.WithName("test"),
				testMetrics,
				auditClient,
			)

			reconciler := &aianalysis.AIAnalysisReconciler{
				Log:                  ctrl.Log.WithName("test"),
				Metrics:              testMetrics,
				InvestigatingHandler: nil, // nil — must be rejected
				AnalyzingHandler:     analyzingHandler,
				AuditClient:          auditClient,
			}

			err := reconciler.ValidateDependencies()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("InvestigatingHandler"))
		})
	})

	Context("UT-AA-1116-004: Reconcile guards against nil handlers", func() {
		It("MUST return permanent error when InvestigatingHandler is nil during Investigating phase", func() {
			scheme := runtime.NewScheme()
			Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())

			testAnalysis := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-nil-handler",
					Namespace:  "default",
					Finalizers: []string{aianalysis.FinalizerName},
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Kind:      "RemediationRequest",
						Name:      "test-rr",
						Namespace: "default",
					},
					RemediationID: "test-remediation-001",
				},
				Status: aianalysisv1.AIAnalysisStatus{
					Phase: aianalysis.PhaseInvestigating,
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(testAnalysis).
				WithStatusSubresource(testAnalysis).
				Build()

			testMetrics := metrics.NewMetrics()
			mockAuditStore := NewMockAuditStore()
			auditClient := aiaudit.NewAuditClient(mockAuditStore, ctrl.Log.WithName("test-audit"))

			reconciler := &aianalysis.AIAnalysisReconciler{
				Client:               fakeClient,
				Scheme:               scheme,
				Recorder:             record.NewFakeRecorder(10),
				Log:                  ctrl.Log.WithName("test"),
				Metrics:              testMetrics,
				InvestigatingHandler: nil,
				AuditClient:          auditClient,
			}

			_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "test-nil-handler",
					Namespace: "default",
				},
			})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("InvestigatingHandler"))
		})

		It("MUST return permanent error when AnalyzingHandler is nil during Analyzing phase", func() {
			scheme := runtime.NewScheme()
			Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())

			testAnalysis := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-nil-analyzing",
					Namespace:  "default",
					Finalizers: []string{aianalysis.FinalizerName},
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Kind:      "RemediationRequest",
						Name:      "test-rr",
						Namespace: "default",
					},
					RemediationID: "test-remediation-001",
				},
				Status: aianalysisv1.AIAnalysisStatus{
					Phase: aianalysis.PhaseAnalyzing,
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(testAnalysis).
				WithStatusSubresource(testAnalysis).
				Build()

			testMetrics := metrics.NewMetrics()
			mockAuditStore := NewMockAuditStore()
			auditClient := aiaudit.NewAuditClient(mockAuditStore, ctrl.Log.WithName("test-audit"))

			reconciler := &aianalysis.AIAnalysisReconciler{
				Client:          fakeClient,
				Scheme:          scheme,
				Recorder:        record.NewFakeRecorder(10),
				Log:             ctrl.Log.WithName("test"),
				Metrics:         testMetrics,
				AnalyzingHandler: nil,
				AuditClient:     auditClient,
			}

			_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "test-nil-analyzing",
					Namespace: "default",
				},
			})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("AnalyzingHandler"))
		})
	})
})

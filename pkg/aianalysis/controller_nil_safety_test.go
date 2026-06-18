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

package aianalysis_test

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
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/go-logr/logr"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller/aianalysis"
	aiaudit "github.com/jordigilh/kubernaut/pkg/aianalysis/audit"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
	aistatus "github.com/jordigilh/kubernaut/pkg/aianalysis/status"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

// BR-AI-012, BR-AI-030: Nil-safety for critical AA controller dependencies
// Issue #1116: AA controller must fail-fast when core dependencies are nil
var _ = Describe("AIAnalysis Controller Nil Safety (#1116)", func() {

	var (
		testMetrics    *metrics.Metrics
		auditClient    *aiaudit.AuditClient
		mockAuditStore *MockAuditStore
	)

	BeforeEach(func() {
		testMetrics = metrics.NewMetrics()
		mockAuditStore = NewMockAuditStore()
		auditClient = aiaudit.NewAuditClient(mockAuditStore, ctrl.Log.WithName("test-audit"))
	})

	// ──────────────────────────────────────────────────────────────────
	// AC-1: NewAnalyzingHandler panics on nil RegoEvaluatorInterface
	// ──────────────────────────────────────────────────────────────────
	Context("UT-AA-1116-001: NewAnalyzingHandler nil evaluator", func() {
		It("MUST panic when RegoEvaluatorInterface is nil", func() {
			Expect(func() {
				handlers.NewAnalyzingHandler(
					nil,
					ctrl.Log.WithName("test"),
					testMetrics,
					auditClient,
				)
			}).To(PanicWith(ContainSubstring("evaluator")))
		})
	})

	// ──────────────────────────────────────────────────────────────────
	// GAP-5: NewInvestigatingHandler nil-checks for hgClient
	// ──────────────────────────────────────────────────────────────────
	Context("UT-AA-1116-005: NewInvestigatingHandler nil agent client", func() {
		It("MUST panic when AgentClientInterface is nil", func() {
			Expect(func() {
				handlers.NewInvestigatingHandler(
					nil,
					ctrl.Log.WithName("test"),
					testMetrics,
					auditClient,
				)
			}).To(PanicWith(ContainSubstring("agent client")))
		})
	})

	// ──────────────────────────────────────────────────────────────────
	// AC-2 / AC-3: SetupWithManager returns error if handlers nil
	// GAP-1: Tests MUST call SetupWithManager, not just ValidateDependencies
	// ──────────────────────────────────────────────────────────────────
	Context("UT-AA-1116-002: SetupWithManager rejects nil AnalyzingHandler", func() {
		It("MUST return error when AnalyzingHandler is nil", func() {
			scheme := runtime.NewScheme()
			Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())
			Expect(corev1.AddToScheme(scheme)).To(Succeed())

			cfg, err := ctrl.GetConfig()
			if err != nil {
				Skip("cannot create manager without kubeconfig — falling back to ValidateDependencies")
			}

			mgr, err := manager.New(cfg, manager.Options{
				Scheme: scheme,
			})
			Expect(err).ToNot(HaveOccurred())

			mockHolmesClient := mocks.NewMockAgentClient()
			investigatingHandler := handlers.NewInvestigatingHandler(
				mockHolmesClient, ctrl.Log.WithName("test"), testMetrics, auditClient,
			)

			reconciler := &aianalysis.AIAnalysisReconciler{
				Client:           mgr.GetClient(),
				Scheme:           scheme,
				Log:              ctrl.Log.WithName("test"),
				Metrics:          testMetrics,
				StatusManager:    aistatus.NewManager(mgr.GetClient(), mgr.GetAPIReader()),
				AnalyzingHandler: nil,
				AuditClient:      auditClient,
			}
			reconciler.InvestigatingHandler.Store(investigatingHandler)

			err = reconciler.SetupWithManager(mgr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("analyzingHandler"))
		})
	})

	Context("UT-AA-1116-003: SetupWithManager rejects nil InvestigatingHandler", func() {
		It("MUST return error when InvestigatingHandler is nil", func() {
			scheme := runtime.NewScheme()
			Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())
			Expect(corev1.AddToScheme(scheme)).To(Succeed())

			cfg, err := ctrl.GetConfig()
			if err != nil {
				Skip("cannot create manager without kubeconfig — falling back to ValidateDependencies")
			}

			mgr, err := manager.New(cfg, manager.Options{
				Scheme: scheme,
			})
			Expect(err).ToNot(HaveOccurred())

			mockRegoEvaluator := rego.NewEvaluator(rego.Config{PolicyPath: "testdata/policies/always_approve.rego"}, logr.Discard())
			analyzingHandler := handlers.NewAnalyzingHandler(
				mockRegoEvaluator, ctrl.Log.WithName("test"), testMetrics, auditClient,
			)

			reconciler := &aianalysis.AIAnalysisReconciler{
				Client:           mgr.GetClient(),
				Scheme:           scheme,
				Log:              ctrl.Log.WithName("test"),
				Metrics:          testMetrics,
				StatusManager:    aistatus.NewManager(mgr.GetClient(), mgr.GetAPIReader()),
				AnalyzingHandler: analyzingHandler,
				AuditClient:      auditClient,
			}
			// InvestigatingHandler left at zero value — .Load() returns nil

			err = reconciler.SetupWithManager(mgr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("investigatingHandler"))
		})
	})

	// ──────────────────────────────────────────────────────────────────
	// GAP-2: ValidateDependencies covers all 5 mandatory deps
	// ──────────────────────────────────────────────────────────────────
	Context("UT-AA-1116-006: ValidateDependencies catches nil Metrics", func() {
		It("MUST return error when Metrics is nil", func() {
			reconciler := &aianalysis.AIAnalysisReconciler{
				Log:     ctrl.Log.WithName("test"),
				Metrics: nil,
			}
			err := reconciler.ValidateDependencies()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("metrics"))
		})
	})

	Context("UT-AA-1116-007: ValidateDependencies catches nil StatusManager", func() {
		It("MUST return error when StatusManager is nil", func() {
			reconciler := &aianalysis.AIAnalysisReconciler{
				Log:           ctrl.Log.WithName("test"),
				Metrics:       testMetrics,
				StatusManager: nil,
			}
			err := reconciler.ValidateDependencies()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("statusManager"))
		})
	})

	Context("UT-AA-1116-008: ValidateDependencies catches nil AuditClient", func() {
		It("MUST return error when AuditClient is nil", func() {
			reconciler := &aianalysis.AIAnalysisReconciler{
				Log:         ctrl.Log.WithName("test"),
				Metrics:     testMetrics,
				AuditClient: nil,
			}
			err := reconciler.ValidateDependencies()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("auditClient"))
		})
	})

	// ──────────────────────────────────────────────────────────────────
	// GAP-3: Happy path — fully-wired reconciler passes validation
	// ──────────────────────────────────────────────────────────────────
	Context("UT-AA-1116-009: ValidateDependencies accepts fully-wired reconciler", func() {
		It("MUST return nil when all dependencies are present", func() {
			scheme := runtime.NewScheme()
			Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

			mockHolmesClient := mocks.NewMockAgentClient()
			mockRegoEvaluator := rego.NewEvaluator(rego.Config{PolicyPath: "testdata/policies/always_approve.rego"}, logr.Discard())

			reconciler := &aianalysis.AIAnalysisReconciler{
				Client:        fakeClient,
				Scheme:        scheme,
				Log:           ctrl.Log.WithName("test"),
				Metrics:       testMetrics,
				StatusManager: aistatus.NewManager(fakeClient, fakeClient),
				AnalyzingHandler: handlers.NewAnalyzingHandler(
					mockRegoEvaluator, ctrl.Log.WithName("test"), testMetrics, auditClient,
				),
				AuditClient: auditClient,
			}
			reconciler.InvestigatingHandler.Store(handlers.NewInvestigatingHandler(
				mockHolmesClient, ctrl.Log.WithName("test"), testMetrics, auditClient,
			))

			Expect(reconciler.ValidateDependencies()).To(Succeed())
		})
	})

	// ──────────────────────────────────────────────────────────────────
	// GAP-4: Multiple nil deps — errors.Join aggregation
	// ──────────────────────────────────────────────────────────────────
	Context("UT-AA-1116-010: ValidateDependencies reports ALL nil deps at once", func() {
		It("MUST list every missing dependency in a single error", func() {
			reconciler := &aianalysis.AIAnalysisReconciler{
				Log: ctrl.Log.WithName("test"),
			}
			err := reconciler.ValidateDependencies()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("investigatingHandler"))
			Expect(err.Error()).To(ContainSubstring("analyzingHandler"))
			Expect(err.Error()).To(ContainSubstring("metrics"))
			Expect(err.Error()).To(ContainSubstring("statusManager"))
			Expect(err.Error()).To(ContainSubstring("auditClient"))
		})
	})

	// ──────────────────────────────────────────────────────────────────
	// AC-4: Reconcile guards against nil handlers (defense in depth)
	// ──────────────────────────────────────────────────────────────────
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

			reconciler := &aianalysis.AIAnalysisReconciler{
				Client:      fakeClient,
				Scheme:      scheme,
				Recorder:    record.NewFakeRecorder(10),
				Log:         ctrl.Log.WithName("test"),
				Metrics:     testMetrics,
				AuditClient: auditClient,
			}
			// InvestigatingHandler left at zero value — .Load() returns nil

			_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "test-nil-handler",
					Namespace: "default",
				},
			})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("investigatingHandler"))
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

			reconciler := &aianalysis.AIAnalysisReconciler{
				Client:           fakeClient,
				Scheme:           scheme,
				Recorder:         record.NewFakeRecorder(10),
				Log:              ctrl.Log.WithName("test"),
				Metrics:          testMetrics,
				AnalyzingHandler: nil,
				AuditClient:      auditClient,
			}

			_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "test-nil-analyzing",
					Namespace: "default",
				},
			})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("analyzingHandler"))
		})
	})
})

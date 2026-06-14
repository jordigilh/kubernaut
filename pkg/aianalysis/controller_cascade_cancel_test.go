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
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller/aianalysis"
	aiaudit "github.com/jordigilh/kubernaut/pkg/aianalysis/audit"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	aistatus "github.com/jordigilh/kubernaut/pkg/aianalysis/status"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

// mockISPhaseUpdater1421 tracks SetTerminalPhase calls for #1421 cascade tests.
type mockISPhaseUpdater1421 struct {
	mu             sync.Mutex
	terminalCalls  []terminalCall1421
	setTerminalErr error
}

type terminalCall1421 struct {
	RRName string
	Phase  isv1alpha1.SessionPhase
}

func (m *mockISPhaseUpdater1421) SetActivePhase(_ context.Context, _ string) error {
	return nil
}

func (m *mockISPhaseUpdater1421) SetTerminalPhase(_ context.Context, rrName string, phase isv1alpha1.SessionPhase) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.terminalCalls = append(m.terminalCalls, terminalCall1421{RRName: rrName, Phase: phase})
	return m.setTerminalErr
}

func (m *mockISPhaseUpdater1421) getTerminalCalls() []terminalCall1421 {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]terminalCall1421, len(m.terminalCalls))
	copy(out, m.terminalCalls)
	return out
}

// ============================================================================
// AA CONTROLLER CASCADE CANCEL TESTS (#1421)
// FedRAMP Control Objectives:
//   IR-4 (Incident Handling): Cancelled remediation sessions MUST be terminated
//     promptly — orphaned AI investigation sessions represent uncontrolled
//     incident handling that violates IR-4(1) automated response mechanisms.
//   AC-6 (Least Privilege): Active KA sessions hold elevated cluster access;
//     failing to revoke them when the parent operation is cancelled violates
//     the principle of least privilege by maintaining unnecessary access.
//   SI-4 (Information System Monitoring): All state transitions in the
//     remediation chain must be observable; a cancelled RR with an active IS
//     creates a blind spot in monitoring.
// ============================================================================
var _ = Describe("AA Controller Cascade Cancel to IS (#1421) [IR-4, AC-6, SI-4]", func() {

	var (
		ctx         context.Context
		scheme      *runtime.Scheme
		testMetrics *metrics.Metrics
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		_ = aianalysisv1.AddToScheme(scheme)
		_ = isv1alpha1.AddToScheme(scheme)
		testMetrics = metrics.NewMetrics()
	})

	buildReconciler := func(updater handlers.ISPhaseUpdater) (*aianalysis.AIAnalysisReconciler, *mocks.MockAgentClient) {
		mockClient := mocks.NewMockAgentClient()
		mockClient.WithSessionPollStatus("investigating")

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		mockAuditStore := &MockAuditStore{}
		auditClient := aiaudit.NewAuditClient(mockAuditStore, ctrl.Log.WithName("test-1421-audit"))
		statusManager := aistatus.NewManager(fakeClient, fakeClient)
		mockRegoEvaluator := mocks.NewMockRegoEvaluator()

		reconciler := &aianalysis.AIAnalysisReconciler{
			Client:        fakeClient,
			Scheme:        scheme,
			Recorder:      record.NewFakeRecorder(20),
			Log:           ctrl.Log.WithName("test-1421"),
			Metrics:       testMetrics,
			StatusManager: statusManager,
			AuditClient:   auditClient,
			ISPhaseUpdater: updater,
		}
		investigatingHandler := handlers.NewInvestigatingHandler(
			mockClient, ctrl.Log.WithName("test-1421-handler"), testMetrics, auditClient,
			handlers.WithSessionMode(),
			handlers.WithRecorder(record.NewFakeRecorder(20)),
		)
		reconciler.InvestigatingHandler.Store(investigatingHandler)
		reconciler.AnalyzingHandler = handlers.NewAnalyzingHandler(
			mockRegoEvaluator, ctrl.Log.WithName("test-1421-analyzing"), testMetrics, auditClient,
		)

		return reconciler, mockClient
	}

	// UT-AA-1421-001: IR-4(1) — Automated incident handling terminates IS when parent is cancelled
	It("UT-AA-1421-001: should call SetTerminalPhase(Cancelled) when AA is Failed with ParentCancelled reason [IR-4(1)]", func() {
		updater := &mockISPhaseUpdater1421{}
		reconciler, _ := buildReconciler(updater)

		analysis := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "ai-1421-001",
				Namespace:  "default",
				UID:        types.UID("ai-1421-001-uid"),
				Generation: 1,
				Finalizers: []string{"kubernaut.ai/finalizer"},
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationRequestRef: corev1.ObjectReference{
					Name:      "rr-1421-001",
					Namespace: "default",
				},
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase:              aianalysisv1.PhaseFailed,
				Reason:             aianalysisv1.ReasonParentCancelled,
				Message:            "Parent RR entered terminal phase: Cancelled",
				ObservedGeneration: 1,
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(analysis).
			WithStatusSubresource(analysis).
			Build()
		reconciler.Client = fakeClient
		reconciler.StatusManager = aistatus.NewManager(fakeClient, fakeClient)

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "ai-1421-001", Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		calls := updater.getTerminalCalls()
		Expect(calls).To(HaveLen(1), "SetTerminalPhase should be called once")
		Expect(calls[0].RRName).To(Equal("rr-1421-001"))
		Expect(calls[0].Phase).To(Equal(isv1alpha1.SessionPhaseCancelled))
	})

	// UT-AA-1421-002: AC-6 — Normal failure paths must NOT trigger cascade (least privilege scope)
	It("UT-AA-1421-002: should NOT cascade when AA is Failed with non-ParentCancelled reason [AC-6]", func() {
		updater := &mockISPhaseUpdater1421{}
		reconciler, _ := buildReconciler(updater)

		analysis := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "ai-1421-002",
				Namespace:  "default",
				UID:        types.UID("ai-1421-002-uid"),
				Generation: 1,
				Finalizers: []string{"kubernaut.ai/finalizer"},
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationRequestRef: corev1.ObjectReference{
					Name:      "rr-1421-002",
					Namespace: "default",
				},
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase:              aianalysisv1.PhaseFailed,
				Reason:             aianalysisv1.ReasonTransientError,
				Message:            "LLM timeout",
				ObservedGeneration: 1,
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(analysis).
			WithStatusSubresource(analysis).
			Build()
		reconciler.Client = fakeClient
		reconciler.StatusManager = aistatus.NewManager(fakeClient, fakeClient)

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "ai-1421-002", Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		calls := updater.getTerminalCalls()
		Expect(calls).To(BeEmpty(),
			"SetTerminalPhase should NOT be called for non-ParentCancelled reasons")
	})

	// UT-AA-1421-003: SI-4 — Controller resilience under degraded conditions (nil dependency)
	It("UT-AA-1421-003: should not panic when ISPhaseUpdater is nil and reason is ParentCancelled [SI-4]", func() {
		reconciler, _ := buildReconciler(nil)

		analysis := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "ai-1421-003",
				Namespace:  "default",
				UID:        types.UID("ai-1421-003-uid"),
				Generation: 1,
				Finalizers: []string{"kubernaut.ai/finalizer"},
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationRequestRef: corev1.ObjectReference{
					Name:      "rr-1421-003",
					Namespace: "default",
				},
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase:              aianalysisv1.PhaseFailed,
				Reason:             aianalysisv1.ReasonParentCancelled,
				Message:            "Parent RR entered terminal phase: Cancelled",
				ObservedGeneration: 1,
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(analysis).
			WithStatusSubresource(analysis).
			Build()
		reconciler.Client = fakeClient
		reconciler.StatusManager = aistatus.NewManager(fakeClient, fakeClient)

		Expect(func() {
			_, _ = reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "ai-1421-003", Namespace: "default"},
			})
		}).ToNot(Panic(), "Reconcile must not panic when ISPhaseUpdater is nil")
	})
})

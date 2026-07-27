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

package controller_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
)

// ============================================================================
// CASCADE TERMINAL UNIT TESTS (#1421)
// FedRAMP Control Objectives:
//
//	IR-4 (Incident Handling): When a remediation is cancelled, ALL downstream
//	  processing (signal analysis, AI investigation, workflow execution) MUST
//	  terminate promptly. Orphaned child CRDs represent uncontrolled incident
//	  handling resources that violate IR-4(1) automated response mechanisms.
//	AC-6 (Least Privilege): Active child CRDs hold cluster access (AI sessions,
//	  workflow PipelineRuns); cancellation must revoke these to enforce minimal
//	  necessary privilege duration.
//	SI-4 (Information System Monitoring): State transitions in the remediation
//	  chain must be fully observable; a cancelled RR with active children creates
//	  monitoring blind spots.
//	CM-3 (Configuration Change Control): Cascade is idempotent and handles
//	  missing/already-terminal children — no configuration drift from repeated
//	  reconciles.
//
// ============================================================================
var _ = Describe("Cascade Terminal to Children (#1421) [IR-4, AC-6, SI-4, CM-3]", func() {

	var (
		ctx    context.Context
		scheme = setupScheme()
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	reconcileTerminalRR := func(fakeClient client.WithWatch, rrName, namespace string) (ctrl.Result, error) {
		roMetrics := metrics.NewMetricsWithRegistry(prometheus.NewRegistry())
		recorder := record.NewFakeRecorder(20)
		reconciler := controller.NewReconciler(controller.ReconcilerDeps{
			Client:        fakeClient,
			APIReader:     fakeClient,
			Scheme:        scheme,
			AuditStore:    nil,
			Recorder:      recorder,
			Metrics:       roMetrics,
			Timeouts:      controller.TimeoutConfig{},
			RoutingEngine: &MockRoutingEngine{},
		})
		return reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: namespace},
		})
	}

	// ========================================
	// UT-RO-1421-001: IR-4(1) — Automated termination of AI investigation
	// when parent remediation is cancelled
	// ========================================
	It("UT-RO-1421-001: should patch AIAnalysis to Failed with ParentCancelled when RR is Cancelled [IR-4(1)]", func() {
		rrName := "rr-cascade-001"
		namespace := testNs
		aiName := "ai-cascade-001"

		rr := newRemediationRequestWithChildRefs(rrName, namespace, remediationv1.PhaseCancelled, "", aiName, "")
		ai := newAIAnalysis(aiName, namespace, rrName, aianalysisv1.PhaseInvestigating)

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ai).
			WithStatusSubresource(rr, ai).
			Build()

		_, err := reconcileTerminalRR(fakeClient, rrName, namespace)
		Expect(err).ToNot(HaveOccurred())

		fetchedAI := &aianalysisv1.AIAnalysis{}
		Expect(fakeClient.Get(ctx, types.NamespacedName{Name: aiName, Namespace: namespace}, fetchedAI)).To(Succeed())
		Expect(fetchedAI.Status.Phase).To(Equal(aianalysisv1.PhaseFailed),
			"AI should be Failed after parent cancellation")
		Expect(fetchedAI.Status.Reason).To(Equal(aianalysisv1.ReasonParentCancelled),
			"Reason should be ParentCancelled")
		Expect(fetchedAI.Status.Message).To(ContainSubstring("terminal phase"))
		Expect(fetchedAI.Status.CompletedAt).ToNot(BeNil(),
			"CompletedAt should be set on cascade")
	})

	// ========================================
	// UT-RO-1421-002: CM-3 — Idempotent cascade does not corrupt
	// already-terminal children on repeated reconciliation
	// ========================================
	It("UT-RO-1421-002: should skip already-terminal children (idempotent) [CM-3]", func() {
		rrName := "rr-cascade-002"
		namespace := testNs
		aiName := "ai-cascade-002"

		rr := newRemediationRequestWithChildRefs(rrName, namespace, remediationv1.PhaseCancelled, "", aiName, "")
		ai := newAIAnalysis(aiName, namespace, rrName, aianalysisv1.PhaseCompleted)

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ai).
			WithStatusSubresource(rr, ai).
			Build()

		_, err := reconcileTerminalRR(fakeClient, rrName, namespace)
		Expect(err).ToNot(HaveOccurred())

		fetchedAI := &aianalysisv1.AIAnalysis{}
		Expect(fakeClient.Get(ctx, types.NamespacedName{Name: aiName, Namespace: namespace}, fetchedAI)).To(Succeed())
		Expect(fetchedAI.Status.Phase).To(Equal(aianalysisv1.PhaseCompleted),
			"Already-terminal AI should remain Completed (not overwritten)")
		Expect(fetchedAI.Status.Reason).To(BeEmpty(),
			"Reason should remain unchanged for already-terminal child")
	})

	// ========================================
	// UT-RO-1421-003: SI-4 — Graceful degradation when child CRDs are
	// already deleted (e.g., cascade deletion race)
	// ========================================
	It("UT-RO-1421-003: should handle missing child refs gracefully (no panic) [SI-4]", func() {
		rrName := "rr-cascade-003"
		namespace := testNs

		rr := newRemediationRequestWithChildRefs(rrName, namespace, remediationv1.PhaseCancelled, "nonexistent-sp", "nonexistent-ai", "nonexistent-we")

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr).
			WithStatusSubresource(rr).
			Build()

		_, err := reconcileTerminalRR(fakeClient, rrName, namespace)
		Expect(err).ToNot(HaveOccurred(),
			"Reconcile must not fail when referenced children do not exist")
	})

	// ========================================
	// UT-RO-1421-004: IR-4(1) — Signal processing resources are terminated
	// when parent remediation is cancelled
	// ========================================
	It("UT-RO-1421-004: should patch SignalProcessing to Failed when RR is Cancelled and SP is Enriching [IR-4(1)]", func() {
		rrName := "rr-cascade-004"
		namespace := testNs
		spName := "sp-cascade-004"

		rr := newRemediationRequestWithChildRefs(rrName, namespace, remediationv1.PhaseCancelled, spName, "", "")
		sp := newSignalProcessing(spName, namespace, rrName, signalprocessingv1.PhaseEnriching)

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, sp).
			WithStatusSubresource(rr, sp).
			Build()

		_, err := reconcileTerminalRR(fakeClient, rrName, namespace)
		Expect(err).ToNot(HaveOccurred())

		fetchedSP := &signalprocessingv1.SignalProcessing{}
		Expect(fakeClient.Get(ctx, types.NamespacedName{Name: spName, Namespace: namespace}, fetchedSP)).To(Succeed())
		Expect(fetchedSP.Status.Phase).To(Equal(signalprocessingv1.PhaseFailed),
			"SP should be Failed after parent cancellation")
	})

	// ========================================
	// UT-RO-1421-005: AC-6 — Workflow execution with elevated cluster access
	// is terminated when parent remediation is cancelled
	// ========================================
	It("UT-RO-1421-005: should patch WorkflowExecution to Failed when RR is Cancelled and WE is Running [AC-6]", func() {
		rrName := "rr-cascade-005"
		namespace := testNs
		weName := "we-cascade-005"

		rr := newRemediationRequestWithChildRefs(rrName, namespace, remediationv1.PhaseCancelled, "", "", weName)
		we := newWorkflowExecution(weName, namespace, rrName, workflowexecutionv1.PhaseRunning)

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, we).
			WithStatusSubresource(rr, we).
			Build()

		_, err := reconcileTerminalRR(fakeClient, rrName, namespace)
		Expect(err).ToNot(HaveOccurred())

		fetchedWE := &workflowexecutionv1.WorkflowExecution{}
		Expect(fakeClient.Get(ctx, types.NamespacedName{Name: weName, Namespace: namespace}, fetchedWE)).To(Succeed())
		Expect(fetchedWE.Status.Phase).To(Equal(workflowexecutionv1.PhaseFailed),
			"WE should be Failed after parent cancellation")
		Expect(fetchedWE.Status.FailureReason).To(ContainSubstring("terminal phase"),
			"WE FailureReason should indicate parent terminated")
	})

	// ========================================
	// UT-RO-1421-006: IR-4 — Full cascade terminates entire remediation
	// subtree atomically (defense-in-depth: no orphaned resources)
	// ========================================
	It("UT-RO-1421-006: should cascade to all three child types simultaneously [IR-4]", func() {
		rrName := "rr-cascade-006"
		namespace := testNs
		spName := "sp-cascade-006"
		aiName := "ai-cascade-006"
		weName := "we-cascade-006"

		rr := newRemediationRequestWithChildRefs(rrName, namespace, remediationv1.PhaseCancelled, spName, aiName, weName)
		sp := newSignalProcessing(spName, namespace, rrName, signalprocessingv1.PhaseClassifying)
		ai := newAIAnalysis(aiName, namespace, rrName, aianalysisv1.PhaseInvestigating)
		we := newWorkflowExecution(weName, namespace, rrName, workflowexecutionv1.PhaseRunning)

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, sp, ai, we).
			WithStatusSubresource(rr, sp, ai, we).
			Build()

		_, err := reconcileTerminalRR(fakeClient, rrName, namespace)
		Expect(err).ToNot(HaveOccurred())

		fetchedSP := &signalprocessingv1.SignalProcessing{}
		Expect(fakeClient.Get(ctx, types.NamespacedName{Name: spName, Namespace: namespace}, fetchedSP)).To(Succeed())
		Expect(fetchedSP.Status.Phase).To(Equal(signalprocessingv1.PhaseFailed))

		fetchedAI := &aianalysisv1.AIAnalysis{}
		Expect(fakeClient.Get(ctx, types.NamespacedName{Name: aiName, Namespace: namespace}, fetchedAI)).To(Succeed())
		Expect(fetchedAI.Status.Phase).To(Equal(aianalysisv1.PhaseFailed))
		Expect(fetchedAI.Status.Reason).To(Equal(aianalysisv1.ReasonParentCancelled))

		fetchedWE := &workflowexecutionv1.WorkflowExecution{}
		Expect(fakeClient.Get(ctx, types.NamespacedName{Name: weName, Namespace: namespace}, fetchedWE)).To(Succeed())
		Expect(fetchedWE.Status.Phase).To(Equal(workflowexecutionv1.PhaseFailed))
	})
})

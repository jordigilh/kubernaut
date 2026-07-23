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

package controller_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	prodcontroller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// Issue #1677 Phase 1 (DD-WORKFLOW-018 v1.1): WorkflowDisplayName is resolved
// directly from AIAnalysis.Status.SelectedWorkflow.ActionType/.WorkflowName --
// both catalog-authoritative, `+kubebuilder:validation:Required` fields
// populated by KA at selection time (never LLM-suppliable, see
// pkg/shared/types/workflow_snapshot.go). This replaces the Issue #643 v2
// live DataStorage lookup (PR #702): that lookup existed only because, at
// the time, (a) KA did not populate ActionType/WorkflowName in its
// selection response, and (b) the only other option was an async,
// AuthWebhook-dependent RemediationWorkflow CRD status field. Both root
// causes are closed as of DD-WORKFLOW-018 v1.1, so RO no longer needs a
// live network call to render a human-readable workflow name -- it already
// holds the data on the AIAnalysis object it is reconciling.
var _ = Describe("Issue #1677 Phase 1: Workflow Name Resolution from AIAnalysis Status", func() {
	const (
		workflowUUID = "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
		workflowName = "crashloop-rollback-v1"
		actionType   = "RestartPod"
	)

	It("UT-RO-643-001: should set WorkflowDisplayName as ActionType:WorkflowName from AIAnalysis.Status.SelectedWorkflow, with no DataStorage call", func() {
		ctx := context.Background()
		scheme := setupScheme()

		rr := newRemediationRequestWithChildRefs(
			"test-rr-643", defaultFixture, remediationv1.PhaseAnalyzing,
			"sp-test-rr-643", "ai-test-rr-643", "")
		ai := newAIAnalysisCompleted("ai-test-rr-643", defaultFixture, "test-rr-643", 0.95, workflowUUID)
		// Distinct friendly name from the UUID, mirroring what KA/DS's catalog
		// would resolve -- proves RO reads it verbatim, not from a live lookup.
		ai.Status.SelectedWorkflow.WorkflowName = workflowName
		ai.Status.SelectedWorkflow.ActionType = actionType
		sp := newSignalProcessingCompleted("sp-test-rr-643", "test-rr-643")

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ai, sp).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			Build()

		recorder := record.NewFakeRecorder(20)
		reconciler := prodcontroller.NewReconciler(prodcontroller.ReconcilerDeps{
			Client:        fakeClient,
			APIReader:     fakeClient,
			Scheme:        scheme,
			AuditStore:    nil,
			Recorder:      recorder,
			Metrics:       rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			Timeouts:      prodcontroller.TimeoutConfig{},
			RoutingEngine: &MockRoutingEngine{},
		})
		// No workflow-display resolver to wire -- Issue #1677 Phase 1 removed
		// the live DataStorage lookup mechanism entirely.

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "test-rr-643", Namespace: defaultFixture},
		})
		Expect(err).NotTo(HaveOccurred())

		var updatedRR remediationv1.RemediationRequest
		err = fakeClient.Get(ctx, client.ObjectKey{Name: "test-rr-643", Namespace: defaultFixture}, &updatedRR)
		Expect(err).NotTo(HaveOccurred())

		Expect(updatedRR.Status.WorkflowDisplayName).To(
			Equal(actionType+":"+workflowName),
			"WorkflowDisplayName should be ActionType:WorkflowName sourced from AIAnalysis.Status.SelectedWorkflow")
		Expect(updatedRR.Status.WorkflowDisplayName).NotTo(
			ContainSubstring(workflowUUID),
			"WorkflowDisplayName should NOT contain the raw UUID")
	})

	It("UT-RO-643-002: should read WorkflowName/ActionType verbatim even when the fixture defaults WorkflowName to the WorkflowID", func() {
		ctx := context.Background()
		scheme := setupScheme()

		rr := newRemediationRequestWithChildRefs(
			"test-rr-643b", defaultFixture, remediationv1.PhaseAnalyzing,
			"sp-test-rr-643b", "ai-test-rr-643b", "")
		// newAIAnalysisCompleted's default fixture sets WorkflowName=workflowID
		// and ActionType="RestartPod" -- unmodified here.
		ai := newAIAnalysisCompleted("ai-test-rr-643b", defaultFixture, "test-rr-643b", 0.95, workflowUUID)
		sp := newSignalProcessingCompleted("sp-test-rr-643b", "test-rr-643b")

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ai, sp).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			Build()

		recorder := record.NewFakeRecorder(20)
		reconciler := prodcontroller.NewReconciler(prodcontroller.ReconcilerDeps{
			Client:        fakeClient,
			APIReader:     fakeClient,
			Scheme:        scheme,
			AuditStore:    nil,
			Recorder:      recorder,
			Metrics:       rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			Timeouts:      prodcontroller.TimeoutConfig{},
			RoutingEngine: &MockRoutingEngine{},
		})

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "test-rr-643b", Namespace: defaultFixture},
		})
		Expect(err).NotTo(HaveOccurred())

		var updatedRR remediationv1.RemediationRequest
		err = fakeClient.Get(ctx, client.ObjectKey{Name: "test-rr-643b", Namespace: defaultFixture}, &updatedRR)
		Expect(err).NotTo(HaveOccurred())

		Expect(updatedRR.Status.WorkflowDisplayName).To(Equal("RestartPod:" + workflowUUID))
	})
})

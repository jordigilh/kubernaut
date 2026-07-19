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

package authwebhook_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	atv1alpha1 "github.com/jordigilh/kubernaut/api/actiontype/v1alpha1"
	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	sharedcontenthash "github.com/jordigilh/kubernaut/pkg/shared/contenthash"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// mockStartupDSClient is a placeholder passed to StartupReconciler.DSWorkflow.
// #1661 Phase 55c: ActionType-side methods (CreateActionType, UpdateActionType,
// DisableActionType, ForceDisableActionType) were removed once DS's ActionType
// REST endpoints were deleted (DD-WORKFLOW-018) and StartupReconciler.DSActionType
// was removed alongside them (Change 8d already made syncActionTypeCRD a pure
// local computation with zero DS round-trips). CreateWorkflowInline/
// DisableWorkflow/GetActiveWorkflowCount were removed earlier by #1661 Change
// 8c/8d for the same reason on the Workflow side. WorkflowDSClient is still an
// empty marker interface (interface{}), so this type needs zero methods to
// satisfy it -- it exists only to give DSWorkflow a distinguishable non-nil
// value in these tests, pending the equivalent DSWorkflow field removal in
// Phase B.
type mockStartupDSClient struct{}

func makeActionTypeCRD(name, specName string) *atv1alpha1.ActionType {
	return &atv1alpha1.ActionType{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: atv1alpha1.ActionTypeSpec{
			Name: specName,
			Description: atv1alpha1.ActionTypeDescription{
				What:      "Test action type",
				WhenToUse: "Testing",
			},
		},
	}
}

func makeWorkflowCRD(name, actionType string) *rwv1alpha1.RemediationWorkflow {
	return &rwv1alpha1.RemediationWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: rwv1alpha1.RemediationWorkflowSpec{
			Version:    "1.0.0",
			ActionType: actionType,
			Description: rwv1alpha1.RemediationWorkflowDescription{
				What:      "Test workflow",
				WhenToUse: "Testing",
			},
		},
	}
}

var _ = Describe("StartupReconciler (#548)", func() {

	// ========================================
	// UT-AW-548-001: Registers ActionType CRDs locally, zero DS calls
	// (#1661 Change 8d)
	// ========================================
	Describe("UT-AW-548-001: Startup reconciler registers ActionType CRDs locally", func() {
		It("should populate .status.registered/.status.catalogStatus for each ActionType CRD without calling DS", func() {
			scheme := newTestScheme()
			at1 := makeActionTypeCRD("at-1", "ScaleMemory")
			at2 := makeActionTypeCRD("at-2", "RestartPod")

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(at1, at2).
				WithStatusSubresource(&atv1alpha1.ActionType{}, &rwv1alpha1.RemediationWorkflow{}).
				Build()

			mockDS := &mockStartupDSClient{}

			reconciler := &authwebhook.StartupReconciler{
				K8sClient:  k8sClient,
				DSWorkflow: mockDS,
				Logger:     ctrl.Log.WithName("test"),
				Timeout:    10 * time.Second,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := reconciler.Start(ctx)
			Expect(err).NotTo(HaveOccurred())

			for _, name := range []string{"at-1", "at-2"} {
				updated := &atv1alpha1.ActionType{}
				Expect(k8sClient.Get(ctx, nsName("default", name), updated)).To(Succeed())
				Expect(updated.Status.Registered).To(BeTrue(),
					"should register locally with .status.registered = true")
				Expect(string(updated.Status.CatalogStatus)).To(Equal("Active"))
				Expect(updated.Status.RegisteredBy).To(Equal("system:authwebhook-startup"))
			}
		})
	})

	// ========================================
	// UT-AW-548-002: Syncs RemediationWorkflow CRD status locally, zero DS calls
	// (#1661 Change 8c)
	// ========================================
	Describe("UT-AW-548-002: Startup reconciler syncs RemediationWorkflow CRDs locally", func() {
		It("should populate .status.workflowId for each RemediationWorkflow CRD without calling DS", func() {
			scheme := newTestScheme()
			rw1 := makeWorkflowCRD("wf-1", "ScaleMemory")
			rw2 := makeWorkflowCRD("wf-2", "RestartPod")
			rw3 := makeWorkflowCRD("wf-3", "RollbackDeployment")

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rw1, rw2, rw3).
				WithStatusSubresource(&atv1alpha1.ActionType{}, &rwv1alpha1.RemediationWorkflow{}).
				Build()

			mockDS := &mockStartupDSClient{}

			reconciler := &authwebhook.StartupReconciler{
				K8sClient:  k8sClient,
				DSWorkflow: mockDS,
				Logger:     ctrl.Log.WithName("test"),
				Timeout:    10 * time.Second,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := reconciler.Start(ctx)
			Expect(err).NotTo(HaveOccurred())

			for _, name := range []string{"wf-1", "wf-2", "wf-3"} {
				updated := &rwv1alpha1.RemediationWorkflow{}
				Expect(k8sClient.Get(ctx, nsName("default", name), updated)).To(Succeed())
				Expect(updated.Status.WorkflowID).NotTo(BeEmpty(),
					"should register 3 RemediationWorkflow CRDs locally")
			}
		})
	})

	// ========================================
	// UT-AW-548-003: ActionTypes and Workflows both sync locally in sequence
	// (#1661 Change 8c for Workflow, Change 8d for ActionType: neither phase
	// calls DS anymore; what remains verifiable is that both of Start()'s
	// sequential phases run to completion and stamp their respective CRDs.)
	// ========================================
	Describe("UT-AW-548-003: ActionTypes and Workflows both sync locally in sequence", func() {
		It("should complete Phase 1 (ActionType local sync) and Phase 2 (Workflow local sync) with zero DS calls", func() {
			scheme := newTestScheme()
			at1 := makeActionTypeCRD("at-1", "ScaleMemory")
			rw1 := makeWorkflowCRD("wf-1", "ScaleMemory")
			rw2 := makeWorkflowCRD("wf-2", "ScaleMemory")

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(at1, rw1, rw2).
				WithStatusSubresource(&atv1alpha1.ActionType{}, &rwv1alpha1.RemediationWorkflow{}).
				Build()

			mockDS := &mockStartupDSClient{}

			reconciler := &authwebhook.StartupReconciler{
				K8sClient:  k8sClient,
				DSWorkflow: mockDS,
				Logger:     ctrl.Log.WithName("test"),
				Timeout:    10 * time.Second,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := reconciler.Start(ctx)
			Expect(err).NotTo(HaveOccurred())

			updatedAT := &atv1alpha1.ActionType{}
			Expect(k8sClient.Get(ctx, nsName("default", "at-1"), updatedAT)).To(Succeed())
			Expect(updatedAT.Status.Registered).To(BeTrue(),
				"ActionType status should be populated locally once Phase 1 runs")

			for _, name := range []string{"wf-1", "wf-2"} {
				updated := &rwv1alpha1.RemediationWorkflow{}
				Expect(k8sClient.Get(ctx, nsName("default", name), updated)).To(Succeed())
				Expect(updated.Status.WorkflowID).NotTo(BeEmpty(),
					"workflow status should be populated locally once Phase 2 runs")
			}
		})
	})

	// ========================================
	// UT-AW-548-004: CRD status updated after local registration (#1661 Change 8c)
	// ========================================
	Describe("UT-AW-548-004: CRD status updated after local registration", func() {
		It("should set catalogStatus=Active, a content-derived workflowId, and registeredBy on RW CRD status", func() {
			scheme := newTestScheme()
			rw := makeWorkflowCRD("wf-status", "ScaleMemory")

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rw).
				WithStatusSubresource(&atv1alpha1.ActionType{}, &rwv1alpha1.RemediationWorkflow{}).
				Build()

			// #1661 Change 8c: rwResult is no longer consulted -- workflowId
			// is always AW's own local computation now, deliberately left
			// unset here to prove that.
			mockDS := &mockStartupDSClient{}

			reconciler := &authwebhook.StartupReconciler{
				K8sClient:  k8sClient,
				DSWorkflow: mockDS,
				Logger:     ctrl.Log.WithName("test"),
				Timeout:    10 * time.Second,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := reconciler.Start(ctx)
			Expect(err).NotTo(HaveOccurred())

			updated := &rwv1alpha1.RemediationWorkflow{}
			Expect(k8sClient.Get(ctx, nsName("default", "wf-status"), updated)).To(Succeed())

			Expect(updated.Status.ContentHash).NotTo(BeEmpty(),
				"contentHash should be populated from the local computation")
			Expect(updated.Status.WorkflowID).To(Equal(sharedcontenthash.DeterministicUUID(updated.Status.ContentHash)),
				"workflowId should be AW's own deterministic UUID derived from contentHash (#1661 Change 8a)")
			Expect(string(updated.Status.CatalogStatus)).To(Equal("Active"),
				"catalogStatus should be Active")
			Expect(updated.Status.RegisteredBy).To(Equal("system:authwebhook-startup"),
				"registeredBy should identify the startup reconciler")
			Expect(updated.Status.RegisteredAt).NotTo(BeNil(),
				"registeredAt should be set")
		})
	})

	// UT-AW-548-005 ("Retries with backoff when DS unavailable") and
	// UT-AW-548-006 ("Returns error when DS never responds") are removed:
	// #1661 Change 8d deleted syncActionTypeCRD's DS round-trip (and the
	// deadline/backoff/retry machinery that only existed to survive DS being
	// transiently unavailable) entirely, mirroring Change 8c's identical
	// removal on the Workflow side (see startup_graceful_test.go, deleted).
	// Start() no longer talks to DS at all, so there is nothing left for
	// either scenario to exercise.

	// ========================================
	// UT-AW-548-007: Empty CRD lists handled gracefully
	// ========================================
	Describe("UT-AW-548-007: Empty CRD lists handled gracefully", func() {
		It("should complete with no DS calls when no CRDs exist", func() {
			scheme := newTestScheme()

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(&atv1alpha1.ActionType{}, &rwv1alpha1.RemediationWorkflow{}).
				Build()

			mockDS := &mockStartupDSClient{}

			reconciler := &authwebhook.StartupReconciler{
				K8sClient:  k8sClient,
				DSWorkflow: mockDS,
				Logger:     ctrl.Log.WithName("test"),
				Timeout:    10 * time.Second,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := reconciler.Start(ctx)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// ========================================
	// UT-AW-548-008: Idempotent re-registration
	// ========================================
	Describe("UT-AW-548-008: Idempotent re-registration of already-synced CRDs", func() {
		It("should recompute the same local status and complete without error for already-registered CRDs", func() {
			scheme := newTestScheme()
			rw := makeWorkflowCRD("wf-idem", "ScaleMemory")
			// Pre-populate status as if a previous startup run already
			// synced this CRD -- re-running Start() must be idempotent.
			rw.Status.WorkflowID = "stale-from-previous-run"
			rw.Status.CatalogStatus = sharedtypes.CatalogStatusActive
			rw.Status.PreviouslyExisted = true

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rw).
				WithStatusSubresource(&atv1alpha1.ActionType{}, &rwv1alpha1.RemediationWorkflow{}).
				Build()

			mockDS := &mockStartupDSClient{}

			reconciler := &authwebhook.StartupReconciler{
				K8sClient:  k8sClient,
				DSWorkflow: mockDS,
				Logger:     ctrl.Log.WithName("test"),
				Timeout:    10 * time.Second,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := reconciler.Start(ctx)
			Expect(err).NotTo(HaveOccurred())

			updated := &rwv1alpha1.RemediationWorkflow{}
			Expect(k8sClient.Get(ctx, nsName("default", "wf-idem"), updated)).To(Succeed())
			Expect(updated.Status.WorkflowID).To(Equal(sharedcontenthash.DeterministicUUID(updated.Status.ContentHash)),
				"re-running Start() should overwrite the stale ID with the same deterministic computation")
			Expect(string(updated.Status.CatalogStatus)).To(Equal("Active"))
			Expect(updated.Status.PreviouslyExisted).To(BeFalse(),
				"#1661 Change 8c: PreviouslyExisted is always false now -- there is no DS-side history to report")
		})
	})
})

func nsName(namespace, name string) types.NamespacedName {
	return types.NamespacedName{Namespace: namespace, Name: name}
}

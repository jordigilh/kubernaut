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

package authwebhook_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	atv1alpha1 "github.com/jordigilh/kubernaut/api/actiontype/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// #1661 Change 8d: closes a safety gap Change 8c introduced. Once AW stopped
// forwarding RemediationWorkflow registrations to DS's Postgres catalog
// (Change 8c), ActionTypeHandler.handleDelete's dependents check --
// dsClient.DisableActionType, backed by that same now-permanently-stale
// catalog -- silently stopped seeing any RW created after Change 8c landed.
// The K8s-native attemptOrphanRecovery fallback never engaged either, since
// it only runs when DS *does* report dependents. Net effect: ActionTypes
// could be deleted out from under active dependent workflows with no denial,
// undermining BR-WORKFLOW-007.3.
//
// Fix: the K8s-native RemediationWorkflow list becomes the PRIMARY and SOLE
// dependents gate. ActionTypeHandler makes zero DS calls for CREATE, UPDATE,
// or DELETE (DD-WORKFLOW-018) -- mirroring Change 8c's RW precedent.
//
// Business Requirements: BR-WORKFLOW-007 (ActionType lifecycle), BR-WORKFLOW-006.
var _ = Describe("UT-AW-1661-8D: AW removes the DS round-trip entirely for ActionType admission (#1661 Change 8d)", func() {
	var (
		ctx           context.Context
		mockAudit     *MockAuditStoreRW
		failingMockDS *mockActionTypeCatalogClient
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockAudit = &MockAuditStoreRW{}
		// Any call to the DS catalog client fails the test -- Change 8d's
		// whole point is that handleCreate/handleUpdate/handleDelete never
		// reach this client at all anymore.
		failingMockDS = &mockActionTypeCatalogClient{
			createFn: func(_ context.Context, _ string, _ ogenclient.ActionTypeDescription, _ string) (*authwebhook.ActionTypeRegistrationResult, error) {
				Fail("CreateActionType must not be called -- AW computes/patches everything locally (#1661 Change 8d)")
				return nil, nil
			},
			updateFn: func(_ context.Context, _ string, _ ogenclient.ActionTypeDescription, _ string) (*authwebhook.ActionTypeUpdateResult, error) {
				Fail("UpdateActionType must not be called -- AW computes/patches everything locally (#1661 Change 8d)")
				return nil, nil
			},
			disableFn: func(_ context.Context, _ string, _ string) (*authwebhook.ActionTypeDisableResult, error) {
				Fail("DisableActionType must not be called -- the K8s-native RW list is now the sole dependents gate (#1661 Change 8d)")
				return nil, nil
			},
			forceDisableFn: func(_ context.Context, _ string, _ string, _ []string) (*authwebhook.ActionTypeDisableResult, error) {
				Fail("ForceDisableActionType must not be called -- there is no DS-side orphan state left to reconcile (#1661 Change 8d)")
				return nil, nil
			},
		}
	})

	It("CREATE patches .status.registered/.status.catalogStatus locally with zero DS calls", func() {
		at := buildActionType("scale-memory-decouple", "ScaleMemoryDecouple", "kubernaut-system")
		fakeK8s := fake.NewClientBuilder().
			WithScheme(newATScheme()).
			WithObjects(at).
			WithStatusSubresource(&atv1alpha1.ActionType{}).
			Build()
		handler := authwebhook.NewActionTypeHandler(failingMockDS, mockAudit, fakeK8s)

		resp := handler.Handle(ctx, buildATCreateAdmissionRequest(at))
		Expect(resp.Allowed).To(BeTrue(), "CREATE should be allowed without any DS round-trip")

		Eventually(func() bool {
			updated := &atv1alpha1.ActionType{}
			if err := fakeK8s.Get(ctx, client.ObjectKeyFromObject(at), updated); err != nil {
				return false
			}
			return updated.Status.Registered
		}, 5*time.Second, 100*time.Millisecond).Should(BeTrue(),
			"BUSINESS VALUE: .status.registered must be patched from AW's own local computation, never from a DS response")

		updated := &atv1alpha1.ActionType{}
		Expect(fakeK8s.Get(ctx, client.ObjectKeyFromObject(at), updated)).To(Succeed())
		Expect(updated.Status.CatalogStatus).To(Equal(sharedtypes.CatalogStatusActive),
			"a successfully-admitted action type is always Active once there is no DS-side lifecycle to defer to")
		Expect(updated.Status.PreviouslyExisted).To(BeFalse(),
			"there is no DS-side disabled state left to re-enable from -- always false, mirroring RW's Change 8c precedent")
	})

	It("UPDATE (description change) is allowed and audited with zero DS calls", func() {
		oldAT := buildActionType("scale-memory-decouple-upd", "ScaleMemoryDecoupleUpd", "kubernaut-system")
		newAT := buildActionType("scale-memory-decouple-upd", "ScaleMemoryDecoupleUpd", "kubernaut-system")
		newAT.Spec.Description.What = "Updated description, no DS round-trip needed."
		handler := authwebhook.NewActionTypeHandler(failingMockDS, mockAudit, nil)

		resp := handler.Handle(ctx, buildATUpdateAdmissionRequest(oldAT, newAT))
		Expect(resp.Allowed).To(BeTrue(), "UPDATE should be allowed without any DS round-trip")

		Expect(mockAudit.StoredEvents).To(HaveLen(1))
		Expect(mockAudit.StoredEvents[0].EventType).To(Equal(authwebhook.EventTypeATAdmittedUpdate))
	})

	It("DELETE denied when a live RemediationWorkflow CRD references the ActionType, with zero DS calls", func() {
		at := buildActionType("scale-memory-dependents", "ScaleMemoryDependents", "kubernaut-system")
		liveRW := buildRemediationWorkflow("scale-memory-dependents-wf", "kubernaut-system")
		liveRW.Spec.ActionType = "ScaleMemoryDependents"
		fakeK8s := fake.NewClientBuilder().
			WithScheme(newATScheme()).
			WithObjects(at, liveRW).
			Build()
		handler := authwebhook.NewActionTypeHandler(failingMockDS, mockAudit, fakeK8s)

		resp := handler.Handle(ctx, buildATDeleteAdmissionRequest(at))

		Expect(resp.Allowed).To(BeFalse(),
			"BUSINESS VALUE (BR-WORKFLOW-007.3): DELETE must be denied when a live RW CRD still depends on this ActionType")
		Expect(resp.Result.Message).To(ContainSubstring("scale-memory-dependents-wf"),
			"denial message should name the dependent workflow")
		Expect(resp.Result.Message).To(ContainSubstring("1"),
			"denial message should contain the dependent count")

		Expect(mockAudit.StoredEvents).To(HaveLen(1))
		Expect(mockAudit.StoredEvents[0].EventType).To(Equal(authwebhook.EventTypeATDeniedDelete))
	})

	It("DELETE allowed when no live RemediationWorkflow CRD references the ActionType, with zero DS calls", func() {
		at := buildActionType("scale-memory-no-deps", "ScaleMemoryNoDeps", "kubernaut-system")
		fakeK8s := fake.NewClientBuilder().
			WithScheme(newATScheme()).
			WithObjects(at).
			Build()
		handler := authwebhook.NewActionTypeHandler(failingMockDS, mockAudit, fakeK8s)

		resp := handler.Handle(ctx, buildATDeleteAdmissionRequest(at))

		Expect(resp.Allowed).To(BeTrue(), "DELETE should be allowed without any DS round-trip when no dependents exist")
		Expect(mockAudit.StoredEvents).To(HaveLen(1))
		Expect(mockAudit.StoredEvents[0].EventType).To(Equal(authwebhook.EventTypeATAdmittedDelete))
	})

	It("DELETE denied when a dependent RW exists in a different namespace than the ActionType (cluster-wide dependents check)", func() {
		at := buildActionType("scale-memory-cross-ns", "ScaleMemoryCrossNS", "kubernaut-system")
		liveRW := buildRemediationWorkflow("scale-memory-cross-ns-wf", "other-namespace")
		liveRW.Spec.ActionType = "ScaleMemoryCrossNS"
		fakeK8s := fake.NewClientBuilder().
			WithScheme(newATScheme()).
			WithObjects(at, liveRW).
			Build()
		handler := authwebhook.NewActionTypeHandler(failingMockDS, mockAudit, fakeK8s)

		resp := handler.Handle(ctx, buildATDeleteAdmissionRequest(at))

		Expect(resp.Allowed).To(BeFalse(),
			"DS's dependents check was namespace-agnostic (global Postgres catalog); the K8s-native replacement must preserve that scope")
	})
})

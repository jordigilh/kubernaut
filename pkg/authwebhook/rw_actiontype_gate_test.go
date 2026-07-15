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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	atv1alpha1 "github.com/jordigilh/kubernaut/api/actiontype/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// #1661: AW is now the sole control gate for the RW-to-ActionType relationship,
// validating spec.actionType against etcd directly (via the .spec.name field
// indexer already registered in cmd/authwebhook/main.go) instead of delegating
// to DS's Postgres-backed taxonomy check (superseded).
//
// Business Requirements: BR-WORKFLOW-006, BR-WORKFLOW-007.
var _ = Describe("RemediationWorkflow ActionType Existence Gate (#1661)", func() {
	var (
		ctx       context.Context
		mockDS    *mockWorkflowCatalogClient
		mockAudit *MockAuditStoreRW
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockDS = &mockWorkflowCatalogClient{}
		mockAudit = &MockAuditStoreRW{}
	})

	// fakeK8sWithActionTypes builds a fake client with the same .spec.name
	// index registered on the real manager (cmd/authwebhook/main.go), seeded
	// with the given ActionType objects.
	fakeK8sWithActionTypes := func(objs ...client.Object) client.Client {
		scheme := newATScheme()
		builder := fake.NewClientBuilder().
			WithScheme(scheme).
			WithIndex(&atv1alpha1.ActionType{}, ".spec.name", func(obj client.Object) []string {
				a := obj.(*atv1alpha1.ActionType)
				return []string{a.Spec.Name}
			})
		if len(objs) > 0 {
			builder = builder.WithObjects(objs...)
		}
		return builder.Build()
	}

	// ========================================
	// UT-AW-310-001: CREATE denied when no ActionType CRD matches spec.actionType
	// ========================================
	Describe("UT-AW-310-001: CREATE denied when ActionType does not exist in etcd", func() {
		It("should return Denied and never call DS", func() {
			fakeK8s := fakeK8sWithActionTypes() // no ActionType CRDs at all

			dsCalled := false
			mockDS.createFn = func(_ context.Context, _, _, _ string) error {
				dsCalled = true
				return nil
			}

			handler := authwebhook.NewRemediationWorkflowHandler(mockDS, mockAudit, fakeK8s)
			rw := buildRemediationWorkflow("scale-memory", "kubernaut-system")
			rw.Spec.ActionType = "GhostAction"
			admReq := buildCreateAdmissionRequest(rw)

			resp := handler.Handle(ctx, admReq)

			Expect(resp.Allowed).To(BeFalse(),
				"CREATE should be Denied when spec.actionType has no matching ActionType CRD")
			Expect(resp.Result.Message).To(ContainSubstring("GhostAction"),
				"Denial message should reference the missing action type")
			Expect(dsCalled).To(BeFalse(),
				"DS must never be called once the etcd-native gate denies admission")

			Expect(mockAudit.StoredEvents).To(HaveLen(1),
				"Denied audit event should be emitted")
			Expect(mockAudit.StoredEvents[0].EventType).To(Equal(authwebhook.EventTypeRWAdmittedDenied))
		})
	})

	// ========================================
	// UT-AW-310-002: CREATE denied when ActionType CRD exists but is not Active
	// ========================================
	Describe("UT-AW-310-002: CREATE denied when ActionType exists but is Disabled", func() {
		It("should return Denied and never call DS", func() {
			at := buildActionType("ghost-action", "GhostAction", "kubernaut-system")
			at.Status.CatalogStatus = sharedtypes.CatalogStatusDisabled
			fakeK8s := fakeK8sWithActionTypes(at)

			dsCalled := false
			mockDS.createFn = func(_ context.Context, _, _, _ string) error {
				dsCalled = true
				return nil
			}

			handler := authwebhook.NewRemediationWorkflowHandler(mockDS, mockAudit, fakeK8s)
			rw := buildRemediationWorkflow("scale-memory", "kubernaut-system")
			rw.Spec.ActionType = "GhostAction"
			admReq := buildCreateAdmissionRequest(rw)

			resp := handler.Handle(ctx, admReq)

			Expect(resp.Allowed).To(BeFalse(),
				"CREATE should be Denied when the matching ActionType CRD is not Active")
			Expect(resp.Result.Message).To(ContainSubstring("GhostAction"),
				"Denial message should reference the inactive action type")
			Expect(dsCalled).To(BeFalse(),
				"DS must never be called once the etcd-native gate denies admission")

			Expect(mockAudit.StoredEvents).To(HaveLen(1),
				"Denied audit event should be emitted")
			Expect(mockAudit.StoredEvents[0].EventType).To(Equal(authwebhook.EventTypeRWAdmittedDenied))
		})
	})

	// ========================================
	// UT-AW-310-003: CREATE reaches DS when ActionType CRD exists and is Active
	// ========================================
	Describe("UT-AW-310-003: CREATE succeeds locally when ActionType exists and is Active", func() {
		It("should return Allowed with zero DS calls (#1661 Change 8c)", func() {
			at := buildActionType("scale-memory-at", "ScaleMemory", "kubernaut-system")
			at.Status.CatalogStatus = sharedtypes.CatalogStatusActive
			fakeK8s := fakeK8sWithActionTypes(at)

			dsCalled := false
			mockDS.createFn = func(_ context.Context, _, _, _ string) error {
				dsCalled = true
				return nil
			}

			handler := authwebhook.NewRemediationWorkflowHandler(mockDS, mockAudit, fakeK8s)
			rw := buildRemediationWorkflow("scale-memory", "kubernaut-system")
			rw.Spec.ActionType = "ScaleMemory"
			admReq := buildCreateAdmissionRequest(rw)

			resp := handler.Handle(ctx, admReq)

			Expect(resp.Allowed).To(BeTrue(),
				"CREATE should be Allowed when the matching ActionType CRD is Active")
			Expect(dsCalled).To(BeFalse(),
				"#1661 Change 8c: DS is never called once the etcd-native gate passes -- registration is a pure local computation")
		})
	})
})

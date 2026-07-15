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
	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("RemediationWorkflow Handler DELETE — Fix A (#418)", func() {
	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	// ========================================
	// UT-AW-418-007: Fix A — goroutine fires on DELETE (#1661 Change 8c: always,
	// since there is no more DS disable call whose outcome could gate it) and,
	// per #1661 Change 8d, refreshes activeWorkflowCount from a live K8s list
	// that excludes the RW being deleted itself (still present in etcd behind
	// its finalizer at this point — see listDependentWorkflowNames doc).
	// BR-WORKFLOW-006, BR-WORKFLOW-007
	// ========================================
	Describe("UT-AW-418-007: DELETE triggers AT count refresh goroutine", func() {
		It("should update ActionType status.activeWorkflowCount to the live K8s count excluding the deleted RW", func() {
			rw := buildRemediationWorkflowWithStatus("fix-a-success", "kubernaut-system", "uuid-fix-a-007")
			rw.Spec.ActionType = "ScaleMemory"
			// A second, unrelated live RW referencing the same ActionType proves
			// the refreshed count reflects "what remains after this deletion",
			// not zero and not the pre-deletion total.
			otherRW := buildRemediationWorkflowWithStatus("fix-a-other", "kubernaut-system", "uuid-fix-a-007-other")
			otherRW.Spec.ActionType = "ScaleMemory"

			scheme := newTestScheme()
			at := buildATForReconciler("scale-memory-at", "kubernaut-system", "ScaleMemory", 2)

			fakeK8s := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(at, rw, otherRW).
				WithStatusSubresource(&atv1alpha1.ActionType{}).
				Build()

			mockDS := &mockWorkflowCatalogClient{}
			mockAudit := &MockAuditStoreRW{}

			handler := authwebhook.NewRemediationWorkflowHandler(mockDS, mockAudit, fakeK8s)

			admReq := buildDeleteAdmissionRequest(rw)
			resp := handler.Handle(ctx, admReq)

			Expect(resp.Allowed).To(BeTrue(),
				"DELETE should always be Allowed")

			Eventually(func() int {
				updated := &atv1alpha1.ActionType{}
				Expect(fakeK8s.Get(ctx, types.NamespacedName{Name: "scale-memory-at", Namespace: "kubernaut-system"}, updated)).To(Succeed())
				return updated.Status.ActiveWorkflowCount
			}, 5*time.Second, 100*time.Millisecond).Should(Equal(1),
				"activeWorkflowCount should reflect the one remaining live RW (fix-a-other), excluding the one being deleted")
		})
	})
})

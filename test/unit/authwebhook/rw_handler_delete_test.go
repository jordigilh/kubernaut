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

package authwebhook

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	atv1alpha1 "github.com/jordigilh/kubernaut/api/actiontype/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// atomicATCounter uses atomic int for goroutine-safe call tracking.
type atomicATCounter struct {
	calls     atomic.Int32
	returnErr error
}

func (m *atomicATCounter) GetActiveWorkflowCount(_ context.Context, _ string) (int, error) {
	m.calls.Add(1)
	if m.returnErr != nil {
		return 0, m.returnErr
	}
	return 0, nil
}

var _ = Describe("RemediationWorkflow Handler DELETE — Fix A (#418)", func() {
	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	// ========================================
	// UT-AW-418-007: Fix A — goroutine fires on DS success
	// BR-WORKFLOW-006
	// ========================================
	Describe("UT-AW-418-007: DELETE with DS success triggers AT count refresh goroutine", func() {
		It("should invoke refreshActionTypeWorkflowCount when DisableWorkflow succeeds", func() {
			rw := buildRemediationWorkflowWithStatus("fix-a-success", "kubernaut-system", "uuid-fix-a-007")

			scheme := newTestScheme()
			at := buildATForReconciler("scale-memory-at", "kubernaut-system", "ScaleMemory", 1)

			fakeK8s := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(at).
				WithStatusSubresource(&atv1alpha1.ActionType{}).
				Build()

			mockDS := &mockWorkflowCatalogClient{}
			mockAudit := &MockAuditStoreRW{}
			counter := &atomicATCounter{}

			handler := authwebhook.NewRemediationWorkflowHandler(
				mockDS, mockAudit, fakeK8s,
				authwebhook.WithActionTypeWorkflowCounter(counter),
			)

			admReq := buildDeleteAdmissionRequest(rw)
			resp := handler.Handle(ctx, admReq)

			Expect(resp.Allowed).To(BeTrue(),
				"DELETE should always be Allowed")

			Eventually(func() int32 {
				return counter.calls.Load()
			}, 5*time.Second, 100*time.Millisecond).Should(BeNumerically(">=", 1),
				"GetActiveWorkflowCount should be called when DS disable succeeds (goroutine fired)")
		})
	})

	// ========================================
	// UT-AW-418-008: Fix A — goroutine skipped on DS failure
	// BR-WORKFLOW-006
	// ========================================
	Describe("UT-AW-418-008: DELETE with DS failure does NOT trigger AT count refresh goroutine", func() {
		It("should NOT invoke refreshActionTypeWorkflowCount when DisableWorkflow fails", func() {
			rw := buildRemediationWorkflowWithStatus("fix-a-fail", "kubernaut-system", "uuid-fix-a-008")

			scheme := newTestScheme()
			at := buildATForReconciler("scale-memory-at-2", "kubernaut-system", "ScaleMemory", 1)

			fakeK8s := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(at).
				WithStatusSubresource(&atv1alpha1.ActionType{}).
				Build()

			mockDS := &mockWorkflowCatalogClient{
				disableFn: func(_ context.Context, _, _, _ string) error {
					return fmt.Errorf("connection refused: DS unavailable")
				},
			}
			mockAudit := &MockAuditStoreRW{}
			counter := &atomicATCounter{}

			handler := authwebhook.NewRemediationWorkflowHandler(
				mockDS, mockAudit, fakeK8s,
				authwebhook.WithActionTypeWorkflowCounter(counter),
			)

			admReq := buildDeleteAdmissionRequest(rw)
			resp := handler.Handle(ctx, admReq)

			Expect(resp.Allowed).To(BeTrue(),
				"DELETE should always be Allowed (best-effort)")

			// Give any errant goroutine time to fire (it shouldn't)
			Consistently(func() int32 {
				return counter.calls.Load()
			}, 500*time.Millisecond, 50*time.Millisecond).Should(Equal(int32(0)),
				"GetActiveWorkflowCount should NOT be called when DS disable fails (no stale write)")
		})
	})
})

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
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// buildRemediationWorkflowCRD constructs a RemediationWorkflow CRD object at
// version "1.0.0" (the only version used across all e2e/authwebhook tests).
// Per #329, metadata.name IS the workflow name (no separate workflowName field).
func buildRemediationWorkflowCRD(crdName, description string) *rwv1alpha1.RemediationWorkflow {
	return &rwv1alpha1.RemediationWorkflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crdName,
			Namespace: sharedNamespace,
		},
		Spec: rwv1alpha1.RemediationWorkflowSpec{
			Version: "1.0.0",
			Description: rwv1alpha1.RemediationWorkflowDescription{
				What:      description,
				WhenToUse: "E2E content integrity test",
			},
			ActionType: "IncreaseMemoryLimits",
			Labels: rwv1alpha1.RemediationWorkflowLabels{
				Severity:    []string{"critical"},
				Environment: []string{"production"},
				Component:   []string{"v1/Pod"},
				Priority:    "P1",
			},
			Execution: rwv1alpha1.RemediationWorkflowExecution{
				Engine: "job",
				Bundle: "quay.io/kubernaut-cicd/test-workflows/placeholder-execution:v1.0.0@sha256:377de4244cfeffcbb898a7e7cd388dd1266dd680cef43b17147b876845df29cd",
			},
			Parameters: []rwv1alpha1.RemediationWorkflowParameter{
				{
					Name:        "TARGET_RESOURCE",
					Type:        "string",
					Required:    true,
					Description: "Target resource for remediation",
				},
			},
		},
	}
}

// waitForCRDStatus polls the CRD until the .status.workflowId is non-empty,
// with a fixed 30s timeout (the only timeout used across all e2e/authwebhook tests).
func waitForCRDStatus(crdName string) *rwv1alpha1.RemediationWorkflow {
	const timeout = 30 * time.Second
	rw := &rwv1alpha1.RemediationWorkflow{}
	Eventually(func() string {
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      crdName,
			Namespace: sharedNamespace,
		}, rw); err != nil {
			return ""
		}
		return rw.Status.WorkflowID
	}, timeout, 1*time.Second).ShouldNot(BeEmpty(),
		"CRD .status.workflowId should be populated by the AW handler")
	return rw
}

// deleteCRDAndWait deletes a RemediationWorkflow CRD and waits for it to be gone.
func deleteCRDAndWait(crdName string) {
	rw := &rwv1alpha1.RemediationWorkflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crdName,
			Namespace: sharedNamespace,
		},
	}
	err := k8sClient.Delete(ctx, rw)
	if err != nil {
		return
	}
	Eventually(func() bool {
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      crdName,
			Namespace: sharedNamespace,
		}, &rwv1alpha1.RemediationWorkflow{})
		return err != nil
	}, 30*time.Second, 1*time.Second).Should(BeTrue(),
		"CRD should be deleted")
}

var _ = Describe("Workflow Content Integrity E2E Tests (BR-WORKFLOW-006)", Serial, func() {

	AfterEach(func() {
		// Clean up any lingering CRDs from this test
		rwList := &rwv1alpha1.RemediationWorkflowList{}
		if err := k8sClient.List(ctx, rwList, client.InNamespace(sharedNamespace)); err == nil {
			for i := range rwList.Items {
				_ = k8sClient.Delete(ctx, &rwList.Items[i])
			}
		}
		// Allow time for deletes to propagate
		time.Sleep(2 * time.Second)
	})

	// ========================================
	// E2E-INTEGRITY-001: First CRD registration populates status
	// ========================================
	Describe("E2E-INTEGRITY-001: First CRD registration populates status", func() {
		It("should set .status.workflowId and .status.catalogStatus after CRD creation", func() {
			suffix := uuid.New().String()[:8]
			crdName := fmt.Sprintf("e2e-integrity-001-%s", suffix)

			rw := buildRemediationWorkflowCRD(crdName, "First registration E2E test")
			Expect(k8sClient.Create(ctx, rw)).To(Succeed(),
				"CRD creation should be allowed by the webhook")

			updatedRW := waitForCRDStatus(crdName)
			Expect(updatedRW.Status.CatalogStatus).To(Equal(sharedtypes.CatalogStatusActive),
				"CRD .status.catalogStatus should be 'active' after registration")
		})
	})

	// ========================================
	// E2E-INTEGRITY-002: CRD delete removes the workflow from the cluster
	// ========================================
	// #1661 DD-WORKFLOW-018: previously this test expected DS to retain a
	// queryable "Disabled" status for a deleted workflow, mirroring the old
	// Postgres soft-delete design; DataStorage's read path became a direct
	// informer cache over the RemediationWorkflow CRD with no soft-delete
	// fallback (Change 8c/8d) -- a true etcd deletion makes the workflow
	// genuinely absent from any cache, not just "Disabled".
	//
	// #1677 (DD-WORKFLOW-019): DataStorage no longer hosts that cache or its
	// REST read path at all -- it moved to KubernautAgent. This AuthWebhook
	// E2E suite has no KA client, and re-probing an HTTP catalog endpoint was
	// never really this test's job (AW doesn't own or call that cache); its
	// actual scope is AW's own CRD lifecycle contract, checked directly
	// against the API server below. The cache's own read-your-writes
	// consistency on DELETE (no stale entries survive a real CRD delete) is
	// now proven directly against KA's informer cache by
	// IT-KA-1677-CACHE-006 (test/integration/kubernautagent/workflowcatalog/
	// cache_test.go) -- added specifically to avoid silently dropping this
	// property during the OpenAPI/E2E cleanup. The deletion is separately
	// captured for SOC2/audit reconstruction via the
	// remediationworkflow.admitted.delete audit event (BR-AUDIT-005),
	// verified by the audit-trail E2E suite.
	Describe("E2E-INTEGRITY-002: CRD delete removes the RemediationWorkflow from the cluster", func() {
		It("should make the CRD absent from the API server after deletion", func() {
			suffix := uuid.New().String()[:8]
			crdName := fmt.Sprintf("e2e-integrity-002-%s", suffix)

			rw := buildRemediationWorkflowCRD(crdName, "Delete removes from catalog E2E")
			Expect(k8sClient.Create(ctx, rw)).To(Succeed())
			waitForCRDStatus(crdName)

			By("Confirming the CRD exists before deletion")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crdName, Namespace: sharedNamespace}, &rwv1alpha1.RemediationWorkflow{})).To(Succeed(),
				"CRD should exist while it has not been deleted")

			deleteCRDAndWait(crdName)

			By("Confirming the CRD is gone after deletion")
			getErr := k8sClient.Get(ctx, types.NamespacedName{Name: crdName, Namespace: sharedNamespace}, &rwv1alpha1.RemediationWorkflow{})
			Expect(getErr).To(HaveOccurred(), "CRD should no longer exist after deletion")
		})
	})

	// ========================================
	// E2E-INTEGRITY-003: Delete + recreate same content → re-enable (same UUID)
	// ========================================
	Describe("E2E-INTEGRITY-003: Delete + recreate same content re-enables with same UUID", func() {
		It("should re-enable the workflow with the original UUID", func() {
			suffix := uuid.New().String()[:8]
			crdName := fmt.Sprintf("e2e-integrity-003-%s", suffix)

			rw := buildRemediationWorkflowCRD(crdName, "Re-enable same content E2E")
			Expect(k8sClient.Create(ctx, rw)).To(Succeed())

			updatedRW := waitForCRDStatus(crdName)
			originalUUID := updatedRW.Status.WorkflowID

			deleteCRDAndWait(crdName)

			// Recreate with identical spec
			rw2 := buildRemediationWorkflowCRD(crdName, "Re-enable same content E2E")
			Expect(k8sClient.Create(ctx, rw2)).To(Succeed())

			updatedRW2 := waitForCRDStatus(crdName)
			Expect(updatedRW2.Status.WorkflowID).To(Equal(originalUUID),
				"Re-enabled workflow should have the original UUID")
			Expect(updatedRW2.Status.CatalogStatus).To(Equal(sharedtypes.CatalogStatusActive),
				"Re-enabled workflow should have status 'active'")
		})
	})

	// ========================================
	// E2E-INTEGRITY-004: Delete + recreate different content → new UUID
	// ========================================
	Describe("E2E-INTEGRITY-004: Delete + recreate different content creates new UUID", func() {
		It("should create a new workflow record with a different UUID", func() {
			suffix := uuid.New().String()[:8]
			crdName := fmt.Sprintf("e2e-integrity-004-%s", suffix)

			rw := buildRemediationWorkflowCRD(crdName, "Original content before delete")
			Expect(k8sClient.Create(ctx, rw)).To(Succeed())

			updatedRW := waitForCRDStatus(crdName)
			originalUUID := updatedRW.Status.WorkflowID

			deleteCRDAndWait(crdName)

			// Recreate with different description (changes content hash)
			rw2 := buildRemediationWorkflowCRD(crdName, "Modified content after delete")
			Expect(k8sClient.Create(ctx, rw2)).To(Succeed())

			updatedRW2 := waitForCRDStatus(crdName)
			Expect(updatedRW2.Status.WorkflowID).ToNot(Equal(originalUUID),
				"Different content should produce a new UUID")
			Expect(updatedRW2.Status.CatalogStatus).To(Equal(sharedtypes.CatalogStatusActive),
				"New workflow should have status 'active'")
		})
	})

	// E2E-INTEGRITY-005 and E2E-INTEGRITY-006 (supersede-by-create) were removed
	// in #329. The scenario they tested — two CRDs with different metadata.name but
	// the same workflowName triggering supersede — is no longer architecturally
	// possible because metadata.name IS the workflow identity post-#329. Kubernetes
	// name-uniqueness within a namespace prevents two CRDs from sharing the same
	// workflow identity via CREATE. The delete+recreate paths (INTEGRITY-003/004)
	// cover the remaining content-hash-based lifecycle transitions.
})

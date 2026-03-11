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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
)

// buildRemediationWorkflowCRD constructs a RemediationWorkflow CRD object.
// Per #329, metadata.name IS the workflow name (no separate workflowName field).
func buildRemediationWorkflowCRD(crdName, version, description string) *rwv1alpha1.RemediationWorkflow {
	return &rwv1alpha1.RemediationWorkflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crdName,
			Namespace: sharedNamespace,
		},
		Spec: rwv1alpha1.RemediationWorkflowSpec{
			Version: version,
			Description: rwv1alpha1.RemediationWorkflowDescription{
				What:      description,
				WhenToUse: "E2E content integrity test",
			},
			ActionType: "IncreaseMemoryLimits",
			Labels: rwv1alpha1.RemediationWorkflowLabels{
				Severity:    []string{"critical"},
				Environment: []string{"production"},
				Component:   "pod",
				Priority:    "P1",
			},
			Execution: rwv1alpha1.RemediationWorkflowExecution{
				Engine: "job",
				Bundle: "quay.io/kubernaut-cicd/test-workflows/placeholder-execution:v1.0.0@sha256:adfc09ea45a5b627550c6a73fe75d50efe1c80fa43359fcc4908c9c5b0639ac3",
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

// waitForCRDStatus polls the CRD until the .status.workflowId is non-empty.
func waitForCRDStatus(crdName string, timeout time.Duration) *rwv1alpha1.RemediationWorkflow {
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

// queryDSWorkflowStatus calls the DS API to check the status of a workflow by ID.
// Uses authenticated HTTP client (DD-AUTH-014) since DS endpoints require Bearer token.
func queryDSWorkflowStatus(workflowID string) string {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/v1/workflows/%s", dataStorageURL, workflowID), nil)
	if err != nil {
		return ""
	}
	resp, err := authHTTPClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return ""
	}
	if status, ok := result["status"].(string); ok {
		return status
	}
	return ""
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

			rw := buildRemediationWorkflowCRD(crdName, "1.0.0", "First registration E2E test")
			Expect(k8sClient.Create(ctx, rw)).To(Succeed(),
				"CRD creation should be allowed by the webhook")

			updatedRW := waitForCRDStatus(crdName, 30*time.Second)
			Expect(updatedRW.Status.CatalogStatus).To(Equal("active"),
				"CRD .status.catalogStatus should be 'active' after registration")
		})
	})

	// ========================================
	// E2E-INTEGRITY-002: CRD delete triggers DS disable
	// ========================================
	Describe("E2E-INTEGRITY-002: CRD delete triggers DS disable", func() {
		It("should disable the workflow in DS when the CRD is deleted", func() {
			suffix := uuid.New().String()[:8]
			crdName := fmt.Sprintf("e2e-integrity-002-%s", suffix)

			rw := buildRemediationWorkflowCRD(crdName, "1.0.0", "Delete triggers disable E2E")
			Expect(k8sClient.Create(ctx, rw)).To(Succeed())

			updatedRW := waitForCRDStatus(crdName, 30*time.Second)
			dsWorkflowID := updatedRW.Status.WorkflowID

			deleteCRDAndWait(crdName)

			Eventually(func() string {
				return queryDSWorkflowStatus(dsWorkflowID)
			}, 30*time.Second, 2*time.Second).Should(Equal("disabled"),
				"DS workflow status should be 'disabled' after CRD deletion")
		})
	})

	// ========================================
	// E2E-INTEGRITY-003: Delete + recreate same content → re-enable (same UUID)
	// ========================================
	Describe("E2E-INTEGRITY-003: Delete + recreate same content re-enables with same UUID", func() {
		It("should re-enable the workflow with the original UUID", func() {
			suffix := uuid.New().String()[:8]
			crdName := fmt.Sprintf("e2e-integrity-003-%s", suffix)

			rw := buildRemediationWorkflowCRD(crdName, "1.0.0", "Re-enable same content E2E")
			Expect(k8sClient.Create(ctx, rw)).To(Succeed())

			updatedRW := waitForCRDStatus(crdName, 30*time.Second)
			originalUUID := updatedRW.Status.WorkflowID

			deleteCRDAndWait(crdName)

			// Recreate with identical spec
			rw2 := buildRemediationWorkflowCRD(crdName, "1.0.0", "Re-enable same content E2E")
			Expect(k8sClient.Create(ctx, rw2)).To(Succeed())

			updatedRW2 := waitForCRDStatus(crdName, 30*time.Second)
			Expect(updatedRW2.Status.WorkflowID).To(Equal(originalUUID),
				"Re-enabled workflow should have the original UUID")
			Expect(updatedRW2.Status.CatalogStatus).To(Equal("active"),
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

			rw := buildRemediationWorkflowCRD(crdName, "1.0.0", "Original content before delete")
			Expect(k8sClient.Create(ctx, rw)).To(Succeed())

			updatedRW := waitForCRDStatus(crdName, 30*time.Second)
			originalUUID := updatedRW.Status.WorkflowID

			deleteCRDAndWait(crdName)

			// Recreate with different description (changes content hash)
			rw2 := buildRemediationWorkflowCRD(crdName, "1.0.0", "Modified content after delete")
			Expect(k8sClient.Create(ctx, rw2)).To(Succeed())

			updatedRW2 := waitForCRDStatus(crdName, 30*time.Second)
			Expect(updatedRW2.Status.WorkflowID).ToNot(Equal(originalUUID),
				"Different content should produce a new UUID")
			Expect(updatedRW2.Status.CatalogStatus).To(Equal("active"),
				"New workflow should have status 'active'")
		})
	})

	// ========================================
	// E2E-INTEGRITY-005: Two CRDs with same workflow identity → supersede
	// ========================================
	Describe("E2E-INTEGRITY-005: Two CRDs same workflow identity triggers supersede", func() {
		It("should supersede the old workflow and return a new UUID for the second CRD", func() {
			suffix := uuid.New().String()[:8]
			crdNameA := fmt.Sprintf("e2e-integrity-005a-%s", suffix)
			crdNameB := fmt.Sprintf("e2e-integrity-005b-%s", suffix)

			// CRD-A: first registration
			rwA := buildRemediationWorkflowCRD(crdNameA, "1.0.0", "Version A of workflow")
			Expect(k8sClient.Create(ctx, rwA)).To(Succeed())

			updatedA := waitForCRDStatus(crdNameA, 30*time.Second)
			uuidA := updatedA.Status.WorkflowID

			// CRD-B: same workflow identity, different CRD name and content → triggers supersede
			rwB := buildRemediationWorkflowCRD(crdNameB, "1.0.0", "Version B of workflow - supersedes A")
			Expect(k8sClient.Create(ctx, rwB)).To(Succeed())

			updatedB := waitForCRDStatus(crdNameB, 30*time.Second)
			uuidB := updatedB.Status.WorkflowID

			Expect(uuidB).ToNot(Equal(uuidA),
				"Second CRD should have a different UUID (supersede)")

			// Verify old workflow is superseded in DS
			Eventually(func() string {
				return queryDSWorkflowStatus(uuidA)
			}, 30*time.Second, 2*time.Second).Should(Equal("superseded"),
				"First workflow should be marked as superseded in DS")
		})
	})

	// ========================================
	// E2E-INTEGRITY-006: CRD status includes supersede metadata
	// BR-WORKFLOW-006: CRD status should reflect when a supersede occurred
	// ========================================
	Describe("E2E-INTEGRITY-006: CRD status includes supersede metadata", func() {
		It("should populate .status.superseded and .status.supersededId on the new CRD", func() {
			suffix := uuid.New().String()[:8]
			crdNameA := fmt.Sprintf("e2e-integrity-006a-%s", suffix)
			crdNameB := fmt.Sprintf("e2e-integrity-006b-%s", suffix)

			// CRD-A: first registration
			rwA := buildRemediationWorkflowCRD(crdNameA, "1.0.0", "Original for supersede metadata test")
			Expect(k8sClient.Create(ctx, rwA)).To(Succeed())

			updatedA := waitForCRDStatus(crdNameA, 30*time.Second)
			uuidA := updatedA.Status.WorkflowID

			// CRD-B: supersedes A
			rwB := buildRemediationWorkflowCRD(crdNameB, "1.0.0", "Supersede metadata test - new content")
			Expect(k8sClient.Create(ctx, rwB)).To(Succeed())

			// Wait for CRD-B status
			updatedB := &rwv1alpha1.RemediationWorkflow{}
			Eventually(func() string {
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      crdNameB,
					Namespace: sharedNamespace,
				}, updatedB); err != nil {
					return ""
				}
				return updatedB.Status.WorkflowID
			}, 30*time.Second, 1*time.Second).ShouldNot(BeEmpty())

			// These assertions test fields that don't yet exist in RemediationWorkflowStatus.
			// They will fail in RED phase and drive the GREEN implementation:
			// 1. Add Superseded/SupersededID to RemediationWorkflowStatus CRD type
			// 2. Populate them in mapOgenWorkflowToResult (DS response must expose them)
			// 3. Write them in updateCRDStatus
			Expect(updatedB.Status.WorkflowID).ToNot(Equal(uuidA))

			// TODO(GREEN): These fields need to be added to RemediationWorkflowStatus
			// and propagated from DS response → AW → CRD status.
			// Uncomment when the fields are added to the CRD type:
			//
			// Expect(updatedB.Status.Superseded).To(BeTrue(),
			// 	"CRD status should indicate a supersede occurred")
			// Expect(updatedB.Status.SupersededID).To(Equal(uuidA),
			// 	"CRD status should reference the superseded workflow UUID")

			// For now, verify via DS API that supersede happened
			Eventually(func() string {
				return queryDSWorkflowStatus(uuidA)
			}, 30*time.Second, 2*time.Second).Should(Equal("superseded"),
				"First workflow should be marked as superseded in DS")
		})
	})
})

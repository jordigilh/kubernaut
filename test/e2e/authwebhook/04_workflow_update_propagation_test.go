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
	"k8s.io/apimachinery/pkg/types"

	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	auditclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
)

// ========================================
// E2E: RemediationWorkflow CRD UPDATE Propagation (#773)
// ========================================
//
// Authority: BR-WORKFLOW-006, DD-WEBHOOK-001, DD-WORKFLOW-012, Issue #773
// SOC2 CC8.1: UPDATE operations must produce distinct audit events
//
// These tests verify that CRD spec updates are correctly propagated to DS
// via the validating webhook, including version bump supersession, same-version
// content rejection, and idempotent re-apply.

var _ = Describe("E2E: RemediationWorkflow UPDATE Propagation (#773)", Serial, Label("e2e", "workflow-update"), func() {
	var (
		testCtx    = ctx
		crdCleanup []string
	)

	AfterEach(func() {
		for _, name := range crdCleanup {
			rw := &rwv1alpha1.RemediationWorkflow{}
			key := types.NamespacedName{Name: name, Namespace: sharedNamespace}
			if err := k8sClient.Get(testCtx, key, rw); err == nil {
				_ = k8sClient.Delete(testCtx, rw)
			}
		}
		crdCleanup = nil
	})

	// ========================================
	// E2E-AW-773-001: Version bump UPDATE propagates to DS catalog
	// ========================================
	It("E2E-AW-773-001: version bump UPDATE propagates to DS catalog", func() {
		crdName := fmt.Sprintf("e2e-update-vbump-%s", uuid.New().String()[:8])
		crdCleanup = append(crdCleanup, crdName)

		By("Creating initial RW CRD v1.0.0")
		rw := buildRemediationWorkflowCRD(crdName, "1.0.0", "Initial description for version bump test")
		Expect(k8sClient.Create(testCtx, rw)).To(Succeed())

		By("Waiting for initial workflowId in status")
		initial := waitForCRDStatus(crdName, 30*time.Second)
		initialWorkflowID := initial.Status.WorkflowID
		Expect(initialWorkflowID).ToNot(BeEmpty(), "Initial workflowId should be set")

		By("Updating CRD: bump version to 1.1.0 and change description")
		current := &rwv1alpha1.RemediationWorkflow{}
		Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: crdName, Namespace: sharedNamespace}, current)).To(Succeed())
		current.Spec.Version = "1.1.0"
		current.Spec.Description.WhenNotToUse = "Updated for version bump test"
		Expect(k8sClient.Update(testCtx, current)).To(Succeed(),
			"Version bump UPDATE should be Allowed by webhook")

		By("Waiting for new workflowId (content hash changes -> new deterministic UUID)")
		var newWorkflowID string
		Eventually(func() string {
			rw := &rwv1alpha1.RemediationWorkflow{}
			if err := k8sClient.Get(testCtx, types.NamespacedName{Name: crdName, Namespace: sharedNamespace}, rw); err != nil {
				return ""
			}
			if rw.Status.WorkflowID != "" && rw.Status.WorkflowID != initialWorkflowID {
				newWorkflowID = rw.Status.WorkflowID
				return newWorkflowID
			}
			return ""
		}, 30*time.Second, 1*time.Second).ShouldNot(BeEmpty(),
			"Status should reflect new workflowId after version bump")

		By("Verifying DS catalog has the updated workflow")
		url := fmt.Sprintf("%s/api/v1/workflows/%s", dataStorageURL, newWorkflowID)
		req, err := http.NewRequest(http.MethodGet, url, nil)
		Expect(err).ToNot(HaveOccurred())
		resp, err := authHTTPClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		body, _ := io.ReadAll(resp.Body)
		var dsWorkflow map[string]interface{}
		Expect(json.Unmarshal(body, &dsWorkflow)).To(Succeed())
		Expect(dsWorkflow["version"]).To(Equal("1.1.0"),
			"DS catalog should have version 1.1.0")

		By("Verifying audit trail has remediationworkflow.admitted.update event")
		authAuditClient := createAuthenticatedAuditClient()
		if authAuditClient != nil {
			Eventually(func() bool {
				events, err := authAuditClient.QueryAuditEvents(testCtx, auditclient.QueryAuditEventsParams{
					EventType: auditclient.NewOptString("remediationworkflow.admitted.update"),
				})
				if err != nil {
					return false
				}
				return len(events.Data) > 0
			}, 10*time.Second, 1*time.Second).Should(BeTrue(),
				"Audit trail should contain remediationworkflow.admitted.update event")
		}

		GinkgoWriter.Printf("✅ Version bump UPDATE propagated: %s -> %s\n", initialWorkflowID, newWorkflowID)
	})

	// ========================================
	// E2E-AW-773-002: Same-version content change is rejected
	// ========================================
	It("E2E-AW-773-002: same-version content change is rejected by webhook", func() {
		crdName := fmt.Sprintf("e2e-update-reject-%s", uuid.New().String()[:8])
		crdCleanup = append(crdCleanup, crdName)

		By("Creating initial RW CRD v1.0.0")
		rw := buildRemediationWorkflowCRD(crdName, "1.0.0", "Original description")
		Expect(k8sClient.Create(testCtx, rw)).To(Succeed())

		By("Waiting for initial workflowId in status")
		initial := waitForCRDStatus(crdName, 30*time.Second)
		Expect(initial.Status.WorkflowID).To(HaveLen(36),
			"Initial workflowId should be a UUID (36 chars)")

		By("Updating CRD: change description WITHOUT bumping version")
		current := &rwv1alpha1.RemediationWorkflow{}
		Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: crdName, Namespace: sharedNamespace}, current)).To(Succeed())
		current.Spec.Description.WhenNotToUse = "Changed content without version bump"

		err := k8sClient.Update(testCtx, current)
		Expect(err).To(HaveOccurred(),
			"Same-version content change should be DENIED by webhook")
		Expect(err.Error()).To(ContainSubstring("denied"),
			"Error should indicate admission denial")

		By("Verifying CRD in cluster still has original description")
		unchanged := &rwv1alpha1.RemediationWorkflow{}
		Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: crdName, Namespace: sharedNamespace}, unchanged)).To(Succeed())
		Expect(unchanged.Spec.Description.WhenNotToUse).To(BeEmpty(),
			"CRD should retain original spec (no WhenNotToUse field)")

		GinkgoWriter.Println("✅ Same-version content change correctly rejected")
	})

	// ========================================
	// E2E-AW-773-003: Idempotent re-apply (same content)
	// ========================================
	It("E2E-AW-773-003: idempotent re-apply succeeds without change", func() {
		crdName := fmt.Sprintf("e2e-update-idempotent-%s", uuid.New().String()[:8])
		crdCleanup = append(crdCleanup, crdName)

		By("Creating initial RW CRD v1.0.0")
		rw := buildRemediationWorkflowCRD(crdName, "1.0.0", "Idempotent test description")
		Expect(k8sClient.Create(testCtx, rw)).To(Succeed())

		By("Waiting for initial workflowId in status")
		initial := waitForCRDStatus(crdName, 30*time.Second)
		originalWorkflowID := initial.Status.WorkflowID
		Expect(originalWorkflowID).To(HaveLen(36),
			"Initial workflowId should be a UUID (36 chars)")

		By("Re-applying the CRD with identical spec (no changes)")
		current := &rwv1alpha1.RemediationWorkflow{}
		Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: crdName, Namespace: sharedNamespace}, current)).To(Succeed())
		Expect(k8sClient.Update(testCtx, current)).To(Succeed(),
			"Idempotent re-apply should be Allowed")

		By("Verifying workflowId is unchanged (no supersession)")
		Consistently(func() string {
			rw := &rwv1alpha1.RemediationWorkflow{}
			if err := k8sClient.Get(testCtx, types.NamespacedName{Name: crdName, Namespace: sharedNamespace}, rw); err != nil {
				return ""
			}
			return rw.Status.WorkflowID
		}, 3*time.Second, 500*time.Millisecond).Should(Equal(originalWorkflowID),
			"WorkflowId should be unchanged after idempotent re-apply")

		GinkgoWriter.Printf("✅ Idempotent re-apply: workflowId unchanged (%s)\n", originalWorkflowID)
	})
})

// createAuthenticatedAuditClient returns an audit client with bearer auth,
// or nil if the auth infrastructure is not available.
func createAuthenticatedAuditClient() *auditclient.Client {
	if dataStorageURL == "" {
		return nil
	}
	e2eToken, err := infrastructure.GetServiceAccountToken(
		ctx, sharedNamespace, "authwebhook-e2e-client", kubeconfigPath,
	)
	if err != nil {
		return nil
	}
	httpClient := &http.Client{
		Timeout:   10 * time.Second,
		Transport: testauth.NewServiceAccountTransport(e2eToken),
	}
	c, err := auditclient.NewClient(dataStorageURL, auditclient.WithClient(httpClient))
	if err != nil {
		return nil
	}
	return c
}

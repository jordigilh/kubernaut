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

package authwebhook

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"

	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	auditclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// ========================================
// E2E-AT-3XX-DENY / E2E-CRD-3XX-FORMAT (#1661, Changes 0 and 0b)
// ========================================
//
// Authority: BR-WORKFLOW-006, BR-WORKFLOW-007, DD-WORKFLOW-016, Issue #1661
//
// E2E-AT-3XX-DENY proves AW's etcd-native ActionType existence gate (Change 0)
// end-to-end against a real cluster: a RemediationWorkflow referencing a
// non-existent ActionType is rejected at admission, never persists as a CRD
// (and therefore never reaches any workflow catalog, #1677/DD-WORKFLOW-019),
// and the denial itself is fully auditable (workflow_content on the denied
// event, per Change 2's "denied events capture full content" decision).
//
// E2E-CRD-3XX-FORMAT proves the CRD `Pattern` structural-schema hardening
// (Change 0b) is enforced by the API server itself -- BEFORE the admission
// webhook is ever invoked -- distinguishing it from a webhook-level denial.

var _ = Describe("E2E: AW ActionType Gate & CRD Format Hardening (#1661)", Serial, Label("e2e", "actiontype-gate", "crd-format"), func() {
	var crdCleanup []string

	AfterEach(func() {
		for _, name := range crdCleanup {
			rw := &rwv1alpha1.RemediationWorkflow{}
			key := types.NamespacedName{Name: name, Namespace: sharedNamespace}
			if err := k8sClient.Get(ctx, key, rw); err == nil {
				_ = k8sClient.Delete(ctx, rw)
			}
		}
		crdCleanup = nil
	})

	// ========================================
	// E2E-AT-3XX-DENY: RW referencing a non-existent ActionType is denied,
	// never reaches DS, and the denial is fully auditable.
	// ========================================
	It("E2E-AT-3XX-DENY: RW referencing a non-existent ActionType is denied with zero DS catalog impact", func() {
		crdName := fmt.Sprintf("e2e-at-deny-%s", uuid.New().String()[:8])
		nonExistentActionType := fmt.Sprintf("NonExistentActionType%s", uuid.New().String()[:8])

		By("Attempting to create a RW referencing a non-existent ActionType")
		rw := buildRemediationWorkflowCRD(crdName, "E2E-AT-3XX-DENY: references a non-existent ActionType")
		rw.Spec.ActionType = nonExistentActionType

		err := k8sClient.Create(ctx, rw)
		Expect(err).To(HaveOccurred(), "CREATE should be Denied by the webhook (AW's etcd-native ActionType gate)")
		Expect(err.Error()).To(ContainSubstring("not in the action type taxonomy"),
			"Denial reason should match AW's validateActionTypeExists wording (DD-WORKFLOW-016)")

		By("Verifying the RW never persisted in the cluster (denied CREATE)")
		getErr := k8sClient.Get(ctx, types.NamespacedName{Name: crdName, Namespace: sharedNamespace}, &rwv1alpha1.RemediationWorkflow{})
		Expect(getErr).To(HaveOccurred(), "Denied CREATE should mean the CRD was never persisted")

		// "Zero catalog impact" (#1677, DD-WORKFLOW-019): every workflow
		// catalog -- DS's now-retired one and KA's current one alike -- is a
		// pure informer-cache derivation of the RemediationWorkflow CRD.
		// Having already proven above that the CRD was never persisted, no
		// catalog anywhere can have observed it; there is nothing left to
		// separately probe over the network.

		By("Verifying the remediationworkflow.admitted.denied audit event carries full workflow_content (#1661 Change 2)")
		authAuditClient := createAuthenticatedAuditClient()
		Expect(authAuditClient).ToNot(BeNil(), "DD-AUTH-014 authenticated audit client must be available in E2E")

		var deniedPayload auditclient.RemediationWorkflowWebhookAuditPayload
		Eventually(func() bool {
			// #1661: filter server-side by event_data.workflow_name (detail_key/
			// detail_value, Issue #1199) so this doesn't depend on the denied
			// event landing within the default 50-event page of an unfiltered,
			// cluster-wide event_type query.
			events, qErr := authAuditClient.QueryAuditEvents(ctx, auditclient.QueryAuditEventsParams{
				EventType:   auditclient.NewOptString("remediationworkflow.admitted.denied"),
				DetailKey:   auditclient.NewOptString("workflow_name"),
				DetailValue: auditclient.NewOptString(crdName),
			})
			if qErr != nil {
				return false
			}
			for _, evt := range events.Data {
				payload, ok := evt.EventData.GetRemediationWorkflowWebhookAuditPayload()
				if ok && payload.WorkflowName == crdName {
					deniedPayload = payload
					return true
				}
			}
			return false
		}, 10*time.Second, 1*time.Second).Should(BeTrue(),
			"Audit trail should contain a remediationworkflow.admitted.denied event for this CRD")

		Expect(deniedPayload.WorkflowContent.IsSet()).To(BeTrue(),
			"Denied events should still carry workflow_content when the CRD unmarshaled successfully (#1661 Change 2)")
		Expect(deniedPayload.WorkflowContent.Value.ActionType).To(Equal(nonExistentActionType))
		Expect(deniedPayload.ContentHash.IsSet()).To(BeTrue())
		Expect(deniedPayload.DenialReason.IsSet()).To(BeTrue())
		Expect(deniedPayload.DenialReason.Value).To(ContainSubstring("not in the action type taxonomy"))

		GinkgoWriter.Printf("✅ RW referencing non-existent ActionType %q correctly denied, zero DS impact, full audit content captured\n",
			nonExistentActionType)
	})

	// ========================================
	// E2E-CRD-3XX-FORMAT: malformed spec.version/actionType/maintainer email
	// is rejected by the API server itself (structural schema Pattern),
	// distinguishing it from a webhook-level admission denial.
	// ========================================
	Context("CRD structural-schema Pattern hardening (rejected before the webhook is ever invoked)", func() {
		runKubectlApply := func(manifest string) (string, error) {
			cmd := exec.CommandContext(context.Background(), "kubectl",
				"--kubeconfig", kubeconfigPath, "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(manifest)
			out, err := cmd.CombinedOutput()
			return string(out), err
		}

		It("E2E-CRD-3XX-FORMAT-a: malformed spec.version (non-semver) is rejected at the API server", func() {
			crdName := fmt.Sprintf("e2e-crd-format-version-%s", uuid.New().String()[:8])
			manifest := fmt.Sprintf(`
apiVersion: kubernaut.ai/v1alpha1
kind: RemediationWorkflow
metadata:
  name: %s
  namespace: %s
spec:
  version: "v1.0"
  actionType: "IncreaseMemoryLimits"
  description:
    what: "E2E-CRD-3XX-FORMAT malformed version test"
    whenToUse: "Never -- structural schema should reject this before admission"
  labels:
    severity: ["critical"]
    environment: ["production"]
    component: ["v1/Pod"]
    priority: "P1"
  execution:
    engine: "job"
    bundle: "quay.io/kubernaut-cicd/test-workflows/placeholder-execution:v1.0.0@sha256:377de4244cfeffcbb898a7e7cd388dd1266dd680cef43b17147b876845df29cd"
  parameters:
    - name: "TARGET_RESOURCE"
      type: "string"
      required: true
      description: "Target resource"
`, crdName, sharedNamespace)

			out, err := runKubectlApply(manifest)
			Expect(err).To(HaveOccurred(), "kubectl apply with a malformed version should fail: %s", out)
			Expect(out).ToNot(ContainSubstring("admission webhook"),
				"Rejection must be a structural schema error, NOT a webhook-level denial: %s", out)
			Expect(out).To(ContainSubstring("version"),
				"Error output should reference the offending field: %s", out)

			GinkgoWriter.Printf("✅ Malformed version rejected at API-server level (not webhook): %s\n", out)
		})

		It("E2E-CRD-3XX-FORMAT-b: malformed spec.actionType (non-PascalCase) is rejected at the API server", func() {
			crdName := fmt.Sprintf("e2e-crd-format-actiontype-%s", uuid.New().String()[:8])
			manifest := fmt.Sprintf(`
apiVersion: kubernaut.ai/v1alpha1
kind: RemediationWorkflow
metadata:
  name: %s
  namespace: %s
spec:
  version: "1.0.0"
  actionType: "increase_memory_limits"
  description:
    what: "E2E-CRD-3XX-FORMAT malformed actionType test"
    whenToUse: "Never -- structural schema should reject this before admission"
  labels:
    severity: ["critical"]
    environment: ["production"]
    component: ["v1/Pod"]
    priority: "P1"
  execution:
    engine: "job"
    bundle: "quay.io/kubernaut-cicd/test-workflows/placeholder-execution:v1.0.0@sha256:377de4244cfeffcbb898a7e7cd388dd1266dd680cef43b17147b876845df29cd"
  parameters:
    - name: "TARGET_RESOURCE"
      type: "string"
      required: true
      description: "Target resource"
`, crdName, sharedNamespace)

			out, err := runKubectlApply(manifest)
			Expect(err).To(HaveOccurred(), "kubectl apply with a non-PascalCase actionType should fail: %s", out)
			Expect(out).ToNot(ContainSubstring("admission webhook"),
				"Rejection must be a structural schema error, NOT a webhook-level denial: %s", out)
			Expect(out).To(ContainSubstring("actionType"),
				"Error output should reference the offending field: %s", out)

			GinkgoWriter.Printf("✅ Malformed actionType rejected at API-server level (not webhook): %s\n", out)
		})

		It("E2E-CRD-3XX-FORMAT-c: malformed maintainer email is rejected at the API server", func() {
			crdName := fmt.Sprintf("e2e-crd-format-email-%s", uuid.New().String()[:8])
			manifest := fmt.Sprintf(`
apiVersion: kubernaut.ai/v1alpha1
kind: RemediationWorkflow
metadata:
  name: %s
  namespace: %s
spec:
  version: "1.0.0"
  actionType: "IncreaseMemoryLimits"
  description:
    what: "E2E-CRD-3XX-FORMAT malformed maintainer email test"
    whenToUse: "Never -- structural schema should reject this before admission"
  labels:
    severity: ["critical"]
    environment: ["production"]
    component: ["v1/Pod"]
    priority: "P1"
  execution:
    engine: "job"
    bundle: "quay.io/kubernaut-cicd/test-workflows/placeholder-execution:v1.0.0@sha256:377de4244cfeffcbb898a7e7cd388dd1266dd680cef43b17147b876845df29cd"
  maintainers:
    - name: "Test Maintainer"
      email: "not-an-email"
  parameters:
    - name: "TARGET_RESOURCE"
      type: "string"
      required: true
      description: "Target resource"
`, crdName, sharedNamespace)

			out, err := runKubectlApply(manifest)
			Expect(err).To(HaveOccurred(), "kubectl apply with a malformed maintainer email should fail: %s", out)
			Expect(out).ToNot(ContainSubstring("admission webhook"),
				"Rejection must be a structural schema error, NOT a webhook-level denial: %s", out)
			Expect(out).To(ContainSubstring("email"),
				"Error output should reference the offending field: %s", out)

			GinkgoWriter.Printf("✅ Malformed maintainer email rejected at API-server level (not webhook): %s\n", out)
		})
	})
})

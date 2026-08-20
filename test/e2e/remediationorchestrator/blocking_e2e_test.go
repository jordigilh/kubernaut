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

package remediationorchestrator

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	roaudit "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/audit"
	canonicalhash "github.com/jordigilh/kubernaut/pkg/shared/hash"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

// ========================================
// BR-ORCH-042.5: Ineffective Remediation Chain
// Target-Resource Scoping E2E (Issue #1802)
// ========================================
//
// Real RO controller + real DataStorage + real Postgres (Kind cluster).
//
// NOTE: The historical BR-ORCH-042.1 (consecutive failure counting) stub
// that lived in this file has been superseded here. It required full
// controller-driven consecutive-failure orchestration that was never wired
// into the E2E suite (see git history for the original "pending"-labeled
// stub) and is tracked separately -- it is NOT part of Issue #1802's scope
// (BR-ORCH-042.5, ineffective *chain* detection, a distinct sub-requirement).
//
// Business Value:
// - BR-ORCH-042.5: Ineffective remediation chain detection must be scoped
//   by target resource, not spec-hash alone, to avoid cross-resource false
//   positives (Issue #1802).
// ========================================

var _ = Describe("BR-ORCH-042.5: Ineffective Remediation Chain Target-Resource Scoping E2E (Issue #1802)", Label("e2e", "blocking", "issue-1802"), func() {

	It("E2E-RO-1802-001: cross-namespace same-hash history does not block a target with no history of its own", func() {
		testID := uuid.New().String()[:8]

		By("Creating a managed namespace and Deployment for the NEW target (ns-b/app)")
		nsB := createTestNamespace(ctx, "e2e1802b")
		defer deleteTestNamespace(nsB)
		helpers.EnsureTestDeployment(ctx, k8sClient, nsB, "app")

		By("Computing the production canonical spec hash for ns-b/app (DD-EM-002)")
		var sharedHash string
		Eventually(func() error {
			obj := &unstructured.Unstructured{}
			obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"})
			if err := apiReader.Get(ctx, client.ObjectKey{Name: "app", Namespace: nsB}, obj); err != nil {
				return err
			}
			// EnsureTestDeployment's Deployment has no ConfigMap references, so the
			// composite fingerprint (pre_remediation_hash.go) reduces to the plain
			// CanonicalResourceFingerprint -- reusing the exact production function
			// (pkg/shared/hash) guarantees this matches what the RO controller
			// itself will compute for ns-b/app when the RR below reaches AI analysis.
			hash, hashErr := canonicalhash.CanonicalResourceFingerprint(obj.Object)
			if hashErr != nil {
				return hashErr
			}
			sharedHash = hash
			return nil
		}, timeout, interval).Should(Succeed(), "ns-b/app Deployment should be readable and hashable via the uncached API reader")

		By("Seeding 3 real remediation.workflow_created audit events for a DIFFERENT namespace (ns-a/app) sharing the same spec hash")
		// Reproduces the exact #1802 symptom: an unrelated Deployment (different
		// namespace, e.g. templated from the same Helm chart/GitOps repo) has an
		// ineffective-chain-triggering history under the identical spec hash.
		// ns-a has no real Deployment in the cluster -- its history is seeded
		// directly against the real DataStorage service via the authenticated
		// E2E audit client, exactly like the RO controller's own audit emission
		// (pkg/remediationorchestrator/audit, internal/controller/.../audit_events.go).
		auditManager := roaudit.NewManager(roaudit.ServiceName)
		nsA := fmt.Sprintf("e2e1802a-%s", testID)
		targetA := nsA + "/Deployment/app"
		firstCorrID := fmt.Sprintf("corr-e2e1802-a-%s-0", testID)
		for i := 0; i < 3; i++ {
			corrID := fmt.Sprintf("corr-e2e1802-a-%s-%d", testID, i)
			event, buildErr := auditManager.BuildRemediationWorkflowCreatedEvent(
				corrID, nsA, corrID, "", // clusterID="" -- target-resource-only repro (shared with release/v1.5)
				roaudit.RemediationWorkflowCreatedData{
					PreRemediationSpecHash: sharedHash,
					TargetResource:         targetA,
					WorkflowID:             "wf-e2e1802-a",
					ActionType:             "ScaleUp",
					SignalType:             "HighCPULoad",
					SignalFingerprint:      "fp-e2e1802-a-" + corrID,
				},
			)
			Expect(buildErr).ToNot(HaveOccurred(), "failed to build ns-a history event %d", i)

			resp, createErr := auditClient.CreateAuditEvent(ctx, event)
			Expect(createErr).ToNot(HaveOccurred(), "failed to seed ns-a history event %d", i)
			_, ok := resp.(*ogenclient.AuditEventResponse)
			Expect(ok).To(BeTrue(), "unexpected DataStorage response type %T when seeding ns-a history event %d", resp, i)
		}

		By("Verifying the seeded history is queryable from DataStorage before proceeding (avoid a race with the RR created below)")
		Eventually(func() int {
			resp, qErr := auditClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
				CorrelationID: ogenclient.NewOptString(firstCorrID),
				EventCategory: ogenclient.NewOptString(roaudit.EventCategoryOrchestration),
				Limit:         ogenclient.NewOptInt(10),
			})
			if qErr != nil {
				return 0
			}
			return len(resp.Data)
		}, timeout, interval).Should(BeNumerically(">", 0), "seeded ns-a history must be queryable before creating the ns-b RR")

		By("Creating a RemediationRequest targeting ns-b/app -- SAME spec hash, but zero history of its own")
		now := metav1.Now()
		fingerprintHash := sha256.Sum256([]byte(uuid.New().String()))
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rr-e2e1802-" + testID,
				Namespace: controllerNamespace,
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: hex.EncodeToString(fingerprintHash[:]),
				SignalName:        "HighCPULoad",
				Severity:          signalprocessingv1.SeverityCritical,
				SignalType:        "alert",
				TargetType:        "kubernetes",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind:      "Deployment",
					Name:      "app",
					Namespace: nsB,
				},
				FiringTime:   now,
				ReceivedTime: now,
			},
		}
		Expect(k8sClient.Create(ctx, rr)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, rr) })

		By("Completing SP so the RR reaches AI analysis")
		sp := helpers.WaitForSPCreation(ctx, k8sClient, controllerNamespace, rr.Name, timeout, interval)
		helpers.SimulateSPCompletion(ctx, k8sClient, sp)

		By("Completing AI analysis with a workflow recommendation targeting ns-b/app")
		ai := helpers.WaitForAICreation(ctx, k8sClient, controllerNamespace, rr.Name, timeout, interval)
		helpers.SimulateAICompletedWithWorkflow(ctx, k8sClient, ai, helpers.AICompletionOpts{
			TargetKind:      "Deployment",
			TargetName:      "app",
			TargetNamespace: nsB,
		})

		By("Verifying the real RO controller does NOT block ns-b/app despite ns-a's cross-namespace same-hash chain")
		we := helpers.WaitForWECreation(ctx, k8sClient, controllerNamespace, rr.Name, timeout, interval)
		Expect(we).ToNot(BeNil(), "Issue #1802 regression: ns-b/app's WorkflowExecution must be created -- "+
			"target-resource scoping must isolate it from ns-a's cross-namespace same-hash remediation chain")

		fetched := &remediationv1.RemediationRequest{}
		Expect(apiReader.Get(ctx, client.ObjectKeyFromObject(rr), fetched)).To(Succeed())
		Expect(fetched.Status.EnsureRoutingStatus().BlockReason).ToNot(Equal(remediationv1.BlockReasonIneffectiveChain),
			"ns-b/app must not be blocked with IneffectiveChain by ns-a's cross-namespace history")

		GinkgoWriter.Printf("✅ E2E-RO-1802-001: ns-b/app WorkflowExecution %s created -- not blocked by ns-a's cross-namespace chain\n", we.Name)
	})
})

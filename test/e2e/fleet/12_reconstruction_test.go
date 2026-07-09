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

package fleet

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// E2E-FLEET-CC81-001: SOC2 CC8.1 Fleet Reconstruction Compliance
// Validates that ReconstructRemediationRequest returns cluster_id
// for fleet-scoped remediations, proving end-to-end cluster provenance
// through the audit pipeline.
//
// BR-AUDIT-005 v2.0, DD-AUDIT-003 v2.2, SOC2 CC8.1
var _ = Describe("E2E-FLEET-CC81-001: Fleet Reconstruction Compliance [CC8.1]", Ordered, func() {
	var (
		rrName        string
		correlationID string
	)

	It("should include cluster_id in reconstruction response for fleet RRs", func() {
		By("Finding a completed RemediationRequest with ClusterID set")
		rrList := &remediationv1.RemediationRequestList{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.List(ctx, rrList, client.InNamespace(namespace))).To(Succeed())
			var found bool
			for _, rr := range rrList.Items {
				if rr.Spec.ClusterID != "" && rr.Status.OverallPhase != "" {
					rrName = rr.Name
					correlationID = rr.Name
					found = true
					break
				}
			}
			g.Expect(found).To(BeTrue(), "no RemediationRequest with ClusterID found")
		}, 2*time.Minute, 5*time.Second).Should(Succeed())

		GinkgoWriter.Printf("Found fleet RR: %s (correlationID: %s)\n", rrName, correlationID)

		By("Waiting for audit events to be persisted (async pipeline)")
		time.Sleep(10 * time.Second)

		By("Calling ReconstructRemediationRequest API")
		var resp *ogenclient.ReconstructionResponse
		Eventually(func(g Gomega) {
			result, err := dataStorageClient.ReconstructRemediationRequest(ctx, ogenclient.ReconstructRemediationRequestParams{
				CorrelationID: correlationID,
			})
			g.Expect(err).ToNot(HaveOccurred())

			reconResp, ok := result.(*ogenclient.ReconstructionResponse)
			g.Expect(ok).To(BeTrue(), "expected ReconstructionResponse, got %T", result)
			resp = reconResp
		}, 30*time.Second, 2*time.Second).Should(Succeed())

		By("Verifying cluster_id is present in reconstruction response")
		Expect(resp.ClusterID.Set).To(BeTrue(),
			"CC8.1 violation: cluster_id missing from reconstruction response for fleet RR %s", rrName)
		Expect(resp.ClusterID.Value).ToNot(BeEmpty(),
			"CC8.1 violation: cluster_id is empty for fleet RR %s", rrName)

		GinkgoWriter.Printf("CC8.1 PASS: cluster_id=%q in reconstruction for %s\n",
			resp.ClusterID.Value, rrName)

		By("Verifying YAML contains clusterID in spec")
		Expect(resp.RemediationRequestYaml).To(ContainSubstring("clusterID:"),
			"CC8.1 violation: reconstructed YAML missing clusterID in spec")
	})

	It("should reconstruct with valid correlation_id", func() {
		// No Skip guard here: correlationID is only ever empty if the
		// preceding ordered spec's Eventually failed, and Ginkgo's Ordered
		// decorator on the enclosing Describe already skips subsequent specs
		// automatically when an earlier one in the same container fails.
		By("Verifying correlation_id in reconstruction response")
		result, err := dataStorageClient.ReconstructRemediationRequest(ctx, ogenclient.ReconstructRemediationRequestParams{
			CorrelationID: correlationID,
		})
		Expect(err).ToNot(HaveOccurred())

		reconResp, ok := result.(*ogenclient.ReconstructionResponse)
		Expect(ok).To(BeTrue())
		Expect(reconResp.CorrelationID.Value).To(Equal(correlationID))
	})

	Context("reconstruction without fleet cluster context", func() {
		It("should return empty cluster_id for hub-only RRs", func() {
			// Deliberately create a hub-only RR rather than opportunistically
			// scanning existing RRs for one without spec.clusterID: every RR
			// in this suite is fleet-scoped (DD-TEST-014: fleet E2E targets
			// only the remote cluster for reconciliation), so scanning would
			// never find one and always Skip -- which the project forbids
			// (no Skip()/pending tests; see AGENTS.md TDD Anti-Patterns).
			// Submitting a signal with no "cluster" label reproduces the
			// genuine backward-compatibility case: prometheus_adapter.go
			// reads spec.clusterID from commonLabels["cluster"], which is
			// empty here, so resolverForCluster falls back to the local/hub
			// resolver and the resulting RR's spec.clusterID stays empty.
			By("Creating a hub-only (non-fleet) target resource on the local/hub cluster")
			const targetName = "hub-only-reconstruction-target"
			dep := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      targetName,
					Namespace: namespace,
					Labels:    map[string]string{"kubernaut.ai/managed": "true"},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To[int32](0),
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": targetName}},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": targetName}},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{Name: "app", Image: "busybox:1.36"}},
						},
					},
				},
			}
			if createErr := k8sClient.Create(ctx, dep); createErr != nil && !apierrors.IsAlreadyExists(createErr) {
				Expect(createErr).NotTo(HaveOccurred(), "Failed to create %s fixture", targetName)
			}
			DeferCleanup(func() { _ = k8sClient.Delete(context.Background(), dep) })

			By("Submitting a signal with no cluster label to create a hub-only RR")
			payload := buildPrometheusAlertWithCluster("HubOnlyReconstruction", namespace, "warning",
				"Deployment", targetName, "")
			gatewayURL := "http://localhost:30080"
			_, body := postFleetAlertUntilAccepted(gatewayURL, payload)

			var response map[string]interface{}
			Expect(json.Unmarshal(body, &response)).To(Succeed())
			Expect(response["status"]).To(Equal("created"),
				"Alert should result in a new hub-only RemediationRequest")
			hubRRName, ok := response["remediationRequestName"].(string)
			Expect(ok).To(BeTrue(), "Response must contain remediationRequestName")

			By("Waiting for the hub-only RR to be picked up by reconciliation")
			Eventually(func(g Gomega) {
				var rr remediationv1.RemediationRequest
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name: hubRRName, Namespace: namespace,
				}, &rr)).To(Succeed())
				g.Expect(rr.Spec.ClusterID).To(BeEmpty(), "hub-only RR must not have spec.clusterID set")
				g.Expect(rr.Status.OverallPhase).To(BeElementOf(
					remediationv1.PhasePending, remediationv1.PhaseProcessing, remediationv1.PhaseAnalyzing,
					remediationv1.PhaseAwaitingApproval, remediationv1.PhaseExecuting, remediationv1.PhaseVerifying,
					remediationv1.PhaseBlocked, remediationv1.PhaseCompleted, remediationv1.PhaseFailed,
					remediationv1.PhaseTimedOut, remediationv1.PhaseSkipped),
					"RR must have entered a known reconciliation phase, got %q", rr.Status.OverallPhase)
			}, timeout, interval).Should(Succeed())

			By("Waiting for audit events to be persisted (async pipeline)")
			time.Sleep(10 * time.Second)

			By(fmt.Sprintf("Reconstructing hub-only RR: %s", hubRRName))
			var resp *ogenclient.ReconstructionResponse
			Eventually(func(g Gomega) {
				result, err := dataStorageClient.ReconstructRemediationRequest(ctx, ogenclient.ReconstructRemediationRequestParams{
					CorrelationID: hubRRName,
				})
				g.Expect(err).ToNot(HaveOccurred())
				reconResp, ok := result.(*ogenclient.ReconstructionResponse)
				g.Expect(ok).To(BeTrue(), "expected ReconstructionResponse, got %T", result)
				resp = reconResp
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			Expect(resp.ClusterID.Set).To(BeFalse(),
				"hub-only RR should not have cluster_id set")

			GinkgoWriter.Printf("Backward compat PASS: hub-only RR %s has no cluster_id\n", hubRRName)
		})
	})
})

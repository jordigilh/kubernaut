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
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// E2E-FLEET-CC81-001: SOC2 CC8.1 Fleet Reconstruction Compliance
// Validates that ReconstructRemediationRequest returns cluster_name
// for fleet-scoped remediations, proving end-to-end cluster provenance
// through the audit pipeline.
//
// BR-AUDIT-005 v2.0, DD-AUDIT-003 v2.2, SOC2 CC8.1
var _ = Describe("E2E-FLEET-CC81-001: Fleet Reconstruction Compliance [CC8.1]", Ordered, func() {
	var (
		rrName        string
		correlationID string
	)

	It("should include cluster_name in reconstruction response for fleet RRs", func() {
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

		By("Verifying cluster_name is present in reconstruction response")
		Expect(resp.ClusterName.Set).To(BeTrue(),
			"CC8.1 violation: cluster_name missing from reconstruction response for fleet RR %s", rrName)
		Expect(resp.ClusterName.Value).ToNot(BeEmpty(),
			"CC8.1 violation: cluster_name is empty for fleet RR %s", rrName)

		GinkgoWriter.Printf("CC8.1 PASS: cluster_name=%q in reconstruction for %s\n",
			resp.ClusterName.Value, rrName)

		By("Verifying YAML contains clusterID in spec")
		Expect(resp.RemediationRequestYaml).To(ContainSubstring("clusterID:"),
			"CC8.1 violation: reconstructed YAML missing clusterID in spec")
	})

	It("should reconstruct with valid correlation_id", func() {
		if correlationID == "" {
			Skip("no fleet RR found in previous test")
		}

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
		It("should return empty cluster_name for hub-only RRs", func() {
			By("Finding a hub-only RR (no ClusterID)")
			rrList := &remediationv1.RemediationRequestList{}
			err := k8sClient.List(ctx, rrList, client.InNamespace(namespace))
			Expect(err).ToNot(HaveOccurred())

			var hubRRName string
			for _, rr := range rrList.Items {
				if rr.Spec.ClusterID == "" && rr.Status.OverallPhase != "" {
					hubRRName = rr.Name
					break
				}
			}

			if hubRRName == "" {
				Skip("no hub-only RR found for backward compatibility test")
			}

			By(fmt.Sprintf("Reconstructing hub-only RR: %s", hubRRName))
			result, err := dataStorageClient.ReconstructRemediationRequest(ctx, ogenclient.ReconstructRemediationRequestParams{
				CorrelationID: hubRRName,
			})
			if err != nil {
				if strings.Contains(err.Error(), "400") || strings.Contains(err.Error(), "404") {
					Skip("hub-only RR has no audit events yet")
				}
				Expect(err).ToNot(HaveOccurred())
			}

			reconResp, ok := result.(*ogenclient.ReconstructionResponse)
			Expect(ok).To(BeTrue())

			Expect(reconResp.ClusterName.Set).To(BeFalse(),
				"hub-only RR should not have cluster_name set")

			GinkgoWriter.Printf("Backward compat PASS: hub-only RR %s has no cluster_name\n", hubRRName)
		})
	})
})

// suppressUnused prevents the compiler from complaining about imported but
// potentially unused symbols when the test file compiles independently.
var _ context.Context

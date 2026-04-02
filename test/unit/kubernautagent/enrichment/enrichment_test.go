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

package enrichment_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
)

var _ = Describe("Kubernaut Agent Enrichment — #433", func() {

	Describe("UT-KA-433-028: EnrichmentResult serializes owner chain + labels + history", func() {
		It("should round-trip serialize all enrichment fields", func() {
			original := enrichment.EnrichmentResult{
				OwnerChain: []enrichment.OwnerChainEntry{
					{Kind: "Deployment", Name: "api-server", Namespace: "default"},
					{Kind: "ReplicaSet", Name: "api-server-abc123", Namespace: "default"},
					{Kind: "Pod", Name: "api-server-abc123-xyz", Namespace: "default"},
				},
				DetectedLabels: &enrichment.DetectedLabels{
					GitOpsManaged: true,
					GitOpsTool:    "argocd",
					PDBProtected:  true,
				},
				QuotaDetails: map[string]string{"cpu_hard": "4", "memory_hard": "8Gi"},
				RemediationHistory: []enrichment.RemediationHistoryEntry{
					{WorkflowID: "oom-increase-memory", Outcome: "success", Timestamp: "2026-03-01T10:00:00Z"},
					{WorkflowID: "restart-pod", Outcome: "failure", Timestamp: "2026-02-28T15:30:00Z"},
				},
			}

			data, err := json.Marshal(original)
			Expect(err).NotTo(HaveOccurred())

			var restored enrichment.EnrichmentResult
			err = json.Unmarshal(data, &restored)
			Expect(err).NotTo(HaveOccurred())

			Expect(restored.OwnerChain).To(HaveLen(3))
			Expect(restored.OwnerChain[0].Kind).To(Equal("Deployment"))
			Expect(restored.OwnerChain[0].Name).To(Equal("api-server"))
			Expect(restored.DetectedLabels).NotTo(BeNil())
			Expect(restored.DetectedLabels.GitOpsManaged).To(BeTrue())
			Expect(restored.DetectedLabels.GitOpsTool).To(Equal("argocd"))
			Expect(restored.QuotaDetails).To(HaveKeyWithValue("cpu_hard", "4"))
			Expect(restored.RemediationHistory).To(HaveLen(2))
			Expect(restored.RemediationHistory[0].WorkflowID).To(Equal("oom-increase-memory"))
			Expect(restored.RemediationHistory[1].Outcome).To(Equal("failure"))
		})
	})
})

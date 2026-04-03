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
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
)

var _ = Describe("Kubernaut Agent Enrichment — #433", func() {

	Describe("UT-KA-433-028: EnrichmentResult serializes owner chain + history", func() {
		It("should round-trip serialize baseline enrichment fields", func() {
			original := enrichment.EnrichmentResult{
				OwnerChain: []enrichment.OwnerChainEntry{
					{Kind: "Deployment", Name: "api-server", Namespace: "default"},
					{Kind: "ReplicaSet", Name: "api-server-abc123", Namespace: "default"},
				},
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

			Expect(restored.OwnerChain).To(HaveLen(2))
			Expect(restored.OwnerChain[0].Kind).To(Equal("Deployment"))
			Expect(restored.OwnerChain[0].Name).To(Equal("api-server"))
			Expect(restored.OwnerChain[1].Kind).To(Equal("ReplicaSet"))
			Expect(restored.RemediationHistory).To(HaveLen(2))
			Expect(restored.RemediationHistory[0].WorkflowID).To(Equal("oom-increase-memory"))
			Expect(restored.RemediationHistory[1].Outcome).To(Equal("failure"))
		})
	})

	Describe("UT-KA-433-130: OwnerChainEntry struct has Kind, Name, Namespace fields", func() {
		It("should serialize OwnerChainEntry with all fields", func() {
			entry := enrichment.OwnerChainEntry{
				Kind:      "Deployment",
				Name:      "api-server",
				Namespace: "production",
			}
			data, err := json.Marshal(entry)
			Expect(err).NotTo(HaveOccurred())

			var restored enrichment.OwnerChainEntry
			Expect(json.Unmarshal(data, &restored)).To(Succeed())
			Expect(restored.Kind).To(Equal("Deployment"))
			Expect(restored.Name).To(Equal("api-server"))
			Expect(restored.Namespace).To(Equal("production"))
		})

		It("should omit namespace when empty (cluster-scoped)", func() {
			entry := enrichment.OwnerChainEntry{Kind: "Node", Name: "worker-1"}
			data, err := json.Marshal(entry)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).NotTo(ContainSubstring("namespace"))
		})
	})

	Describe("UT-KA-433-131: DetectedLabels struct matches HAPI LabelDetector output", func() {
		It("should round-trip serialize all 10 DetectedLabels fields", func() {
			labels := enrichment.DetectedLabels{
				FailedDetections:         []string{"hpa_check"},
				GitOpsManaged:            true,
				GitOpsTool:               "argocd",
				PDBProtected:             true,
				HPAEnabled:               false,
				Stateful:                 false,
				HelmManaged:              true,
				NetworkIsolated:          false,
				ServiceMesh:              "istio",
				ResourceQuotaConstrained: true,
			}

			data, err := json.Marshal(labels)
			Expect(err).NotTo(HaveOccurred())

			var restored enrichment.DetectedLabels
			Expect(json.Unmarshal(data, &restored)).To(Succeed())
			Expect(restored.GitOpsManaged).To(BeTrue())
			Expect(restored.GitOpsTool).To(Equal("argocd"))
			Expect(restored.PDBProtected).To(BeTrue())
			Expect(restored.HPAEnabled).To(BeFalse())
			Expect(restored.Stateful).To(BeFalse())
			Expect(restored.HelmManaged).To(BeTrue())
			Expect(restored.NetworkIsolated).To(BeFalse())
			Expect(restored.ServiceMesh).To(Equal("istio"))
			Expect(restored.ResourceQuotaConstrained).To(BeTrue())
			Expect(restored.FailedDetections).To(ConsistOf("hpa_check"))
		})

		It("should default boolean fields to false and string fields to empty", func() {
			var labels enrichment.DetectedLabels
			Expect(labels.GitOpsManaged).To(BeFalse())
			Expect(labels.GitOpsTool).To(BeEmpty())
			Expect(labels.PDBProtected).To(BeFalse())
			Expect(labels.ServiceMesh).To(BeEmpty())
			Expect(labels.FailedDetections).To(BeNil())
		})
	})

	Describe("UT-KA-433-132: EnrichmentResult includes DetectedLabels and QuotaDetails", func() {
		It("should serialize EnrichmentResult with all enrichment fields", func() {
			result := enrichment.EnrichmentResult{
				OwnerChain: []enrichment.OwnerChainEntry{
					{Kind: "Deployment", Name: "api-server", Namespace: "production"},
				},
				DetectedLabels: &enrichment.DetectedLabels{
					GitOpsManaged: true,
					GitOpsTool:    "argocd",
					PDBProtected:  true,
				},
				QuotaDetails: map[string]string{
					"cpu_hard":    "4",
					"memory_hard": "8Gi",
				},
				RemediationHistory: []enrichment.RemediationHistoryEntry{
					{WorkflowID: "oom-recovery", Outcome: "success", Timestamp: "2026-03-01T10:00:00Z"},
				},
			}

			data, err := json.Marshal(result)
			Expect(err).NotTo(HaveOccurred())

			var restored enrichment.EnrichmentResult
			Expect(json.Unmarshal(data, &restored)).To(Succeed())

			Expect(restored.OwnerChain).To(HaveLen(1))
			Expect(restored.OwnerChain[0].Kind).To(Equal("Deployment"))
			Expect(restored.DetectedLabels).NotTo(BeNil())
			Expect(restored.DetectedLabels.GitOpsManaged).To(BeTrue())
			Expect(restored.DetectedLabels.GitOpsTool).To(Equal("argocd"))
			Expect(restored.QuotaDetails).To(HaveKeyWithValue("cpu_hard", "4"))
			Expect(restored.QuotaDetails).To(HaveKeyWithValue("memory_hard", "8Gi"))
			Expect(restored.RemediationHistory).To(HaveLen(1))
		})

		It("should omit DetectedLabels and QuotaDetails when nil", func() {
			result := enrichment.EnrichmentResult{
				OwnerChain:         []enrichment.OwnerChainEntry{{Kind: "Pod", Name: "web", Namespace: "default"}},
				RemediationHistory: []enrichment.RemediationHistoryEntry{},
			}
			data, err := json.Marshal(result)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).NotTo(ContainSubstring("detected_labels"))
			Expect(string(data)).NotTo(ContainSubstring("quota_details"))
		})
	})

	Describe("UT-KA-433-133: DataStorageClient accepts kind and specHash parameters", func() {
		It("should compile with kind, name, namespace, specHash parameters", func() {
			var client enrichment.DataStorageClient = &fakeDS{}
			history, err := client.GetRemediationHistory(nil, "Deployment", "api-server", "production", "abc123hash")
			Expect(err).NotTo(HaveOccurred())
			Expect(history).To(BeEmpty())
		})
	})

	Describe("UT-KA-433-134: K8sClient.GetOwnerChain returns []OwnerChainEntry", func() {
		It("should compile with []OwnerChainEntry return type", func() {
			var client enrichment.K8sClient = &fakeK8s{}
			chain, err := client.GetOwnerChain(nil, "Pod", "web-abc", "default")
			Expect(err).NotTo(HaveOccurred())
			Expect(chain).To(HaveLen(1))
			Expect(chain[0].Kind).To(Equal("Deployment"))
		})
	})
})

// fakeDS satisfies the updated DataStorageClient interface for compile-time verification.
type fakeDS struct{}

func (f *fakeDS) GetRemediationHistory(_ context.Context, _, _, _, _ string) ([]enrichment.RemediationHistoryEntry, error) {
	return nil, nil
}

// fakeK8s satisfies the updated K8sClient interface for compile-time verification.
type fakeK8s struct{}

func (f *fakeK8s) GetOwnerChain(_ context.Context, _, _, _ string) ([]enrichment.OwnerChainEntry, error) {
	return []enrichment.OwnerChainEntry{{Kind: "Deployment", Name: "api-server", Namespace: "default"}}, nil
}

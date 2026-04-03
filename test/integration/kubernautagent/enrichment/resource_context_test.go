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
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/custom"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
)

var _ = Describe("Kubernaut Agent Resource Context Integration — #433", func() {

	Describe("IT-KA-433-036: get_resource_context combines K8s owner chain + DS remediation history", func() {

		It("should resolve root owner from owner chain and combine with remediation history", func() {
			k8s := &fakeK8sClient{
				ownerChain: []enrichment.OwnerChainEntry{
					{Kind: "Pod", Name: "api-server-abc-xyz", Namespace: "production"},
					{Kind: "ReplicaSet", Name: "api-server-abc", Namespace: "production"},
					{Kind: "Deployment", Name: "api-server", Namespace: "production"},
				},
			}
			ds := &fakeDataStorageClient{
				history: []enrichment.RemediationHistoryEntry{
					{WorkflowID: "oom-increase-memory", Outcome: "success", Timestamp: "2026-03-01T10:00:00Z"},
					{WorkflowID: "restart-pod", Outcome: "failure", Timestamp: "2026-02-28T15:30:00Z"},
				},
			}

			reg := registry.New()
			reg.Register(custom.NewNamespacedResourceContextTool(ds, k8s))

			result, err := reg.Execute(context.Background(), "get_namespaced_resource_context",
				json.RawMessage(`{"kind":"Pod","name":"api-server-abc-xyz","namespace":"production"}`))
			Expect(err).NotTo(HaveOccurred())

			var parsed struct {
				RootOwner struct {
					Kind      string `json:"kind"`
					Name      string `json:"name"`
					Namespace string `json:"namespace"`
				} `json:"root_owner"`
				RemediationHistory []struct {
					WorkflowID string `json:"workflow_id"`
					Outcome    string `json:"outcome"`
				} `json:"remediation_history"`
			}
			Expect(json.Unmarshal([]byte(result), &parsed)).To(Succeed())

			Expect(parsed.RootOwner.Kind).To(Equal("Deployment"), "root owner should be the last entry in the chain")
			Expect(parsed.RootOwner.Name).To(Equal("api-server"))
			Expect(parsed.RootOwner.Namespace).To(Equal("production"))
			Expect(parsed.RemediationHistory).To(HaveLen(2))
			Expect(parsed.RemediationHistory[0].WorkflowID).To(Equal("oom-increase-memory"))
		})

		It("should use the resource itself as root owner when owner chain is empty", func() {
			k8s := &fakeK8sClient{ownerChain: nil}
			ds := &fakeDataStorageClient{history: []enrichment.RemediationHistoryEntry{}}

			reg := registry.New()
			reg.Register(custom.NewNamespacedResourceContextTool(ds, k8s))

			result, err := reg.Execute(context.Background(), "get_namespaced_resource_context",
				json.RawMessage(`{"kind":"StatefulSet","name":"redis-ss","namespace":"default"}`))
			Expect(err).NotTo(HaveOccurred())

			var parsed struct {
				RootOwner struct {
					Kind string `json:"kind"`
					Name string `json:"name"`
				} `json:"root_owner"`
				RemediationHistory []interface{} `json:"remediation_history"`
			}
			Expect(json.Unmarshal([]byte(result), &parsed)).To(Succeed())
			Expect(parsed.RootOwner.Kind).To(Equal("StatefulSet"), "should fall back to the queried resource")
			Expect(parsed.RootOwner.Name).To(Equal("redis-ss"))
			Expect(parsed.RemediationHistory).To(BeEmpty())
		})

		It("should return remediation history for cluster-scoped resources", func() {
			ds := &fakeDataStorageClient{
				history: []enrichment.RemediationHistoryEntry{
					{WorkflowID: "drain-reboot", Outcome: "success", Timestamp: "2026-03-02T08:00:00Z"},
				},
			}

			reg := registry.New()
			reg.Register(custom.NewClusterResourceContextTool(ds))

			result, err := reg.Execute(context.Background(), "get_cluster_resource_context",
				json.RawMessage(`{"kind":"Node","name":"worker-1"}`))
			Expect(err).NotTo(HaveOccurred())

			var parsed struct {
				RootOwner struct {
					Kind      string `json:"kind"`
					Name      string `json:"name"`
					Namespace string `json:"namespace"`
				} `json:"root_owner"`
				RemediationHistory []struct {
					WorkflowID string `json:"workflow_id"`
				} `json:"remediation_history"`
			}
			Expect(json.Unmarshal([]byte(result), &parsed)).To(Succeed())
			Expect(parsed.RootOwner.Kind).To(Equal("Node"))
			Expect(parsed.RootOwner.Name).To(Equal("worker-1"))
			Expect(parsed.RootOwner.Namespace).To(BeEmpty(), "cluster-scoped resources have no namespace")
			Expect(parsed.RemediationHistory).To(HaveLen(1))
			Expect(parsed.RemediationHistory[0].WorkflowID).To(Equal("drain-reboot"))
		})
	})
})

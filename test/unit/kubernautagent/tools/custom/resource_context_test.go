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

package custom_test

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/custom"
)

// fakeK8s returns a configurable owner chain.
type fakeK8s struct {
	chain []enrichment.OwnerChainEntry
	err   error
}

func (f *fakeK8s) GetOwnerChain(_ context.Context, _, _, _ string) ([]enrichment.OwnerChainEntry, error) {
	return f.chain, f.err
}

func (f *fakeK8s) GetSpecHash(_ context.Context, _, _, _ string) (string, error) {
	return "", nil
}

// fakeDS returns configurable remediation history.
type fakeDS struct {
	history *enrichment.RemediationHistoryResult
	err     error
}

func (f *fakeDS) GetRemediationHistory(_ context.Context, _, _, _, _ string) (*enrichment.RemediationHistoryResult, error) {
	return f.history, f.err
}

var _ = Describe("Kubernaut Agent Resource Context Tools — #433", func() {

	Describe("UT-KA-433-110: get_namespaced_resource_context returns root_owner from owner chain", func() {
		It("should return the last owner chain entry as root_owner", func() {
			k8s := &fakeK8s{chain: []enrichment.OwnerChainEntry{
				{Kind: "Pod", Name: "api-server-abc-xyz", Namespace: "production"},
				{Kind: "ReplicaSet", Name: "api-server-abc", Namespace: "production"},
				{Kind: "Deployment", Name: "api-server", Namespace: "production"},
			}}
			ds := &fakeDS{history: &enrichment.RemediationHistoryResult{}}

			tool := custom.NewNamespacedResourceContextTool(ds, k8s)
			result, err := tool.Execute(context.Background(),
				json.RawMessage(`{"kind":"Pod","name":"api-server-abc-xyz","namespace":"production"}`))
			Expect(err).NotTo(HaveOccurred())

			var resp map[string]interface{}
			Expect(json.Unmarshal([]byte(result), &resp)).To(Succeed())

			rootOwner, ok := resp["root_owner"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "root_owner should be an object")
			Expect(rootOwner["kind"]).To(Equal("Deployment"))
			Expect(rootOwner["name"]).To(Equal("api-server"))
			Expect(rootOwner["namespace"]).To(Equal("production"))
		})

		It("should default root_owner to input resource when owner chain is empty", func() {
			k8s := &fakeK8s{chain: nil}
			ds := &fakeDS{history: &enrichment.RemediationHistoryResult{}}

			tool := custom.NewNamespacedResourceContextTool(ds, k8s)
			result, err := tool.Execute(context.Background(),
				json.RawMessage(`{"kind":"Pod","name":"orphan-pod","namespace":"default"}`))
			Expect(err).NotTo(HaveOccurred())

			var resp map[string]interface{}
			Expect(json.Unmarshal([]byte(result), &resp)).To(Succeed())

			rootOwner := resp["root_owner"].(map[string]interface{})
			Expect(rootOwner["kind"]).To(Equal("Pod"))
			Expect(rootOwner["name"]).To(Equal("orphan-pod"))
			Expect(rootOwner["namespace"]).To(Equal("default"))
		})
	})

	Describe("UT-KA-433-111: get_namespaced_resource_context includes remediation_history", func() {
		It("should return remediation history from DataStorage", func() {
			k8s := &fakeK8s{chain: []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "api-server", Namespace: "prod"},
			}}
			ds := &fakeDS{history: &enrichment.RemediationHistoryResult{
				Tier1: []enrichment.Tier1Entry{
					{RemediationUID: "oom-recovery", Outcome: "success"},
				},
			}}

			tool := custom.NewNamespacedResourceContextTool(ds, k8s)
			result, err := tool.Execute(context.Background(),
				json.RawMessage(`{"kind":"Pod","name":"web","namespace":"prod"}`))
			Expect(err).NotTo(HaveOccurred())

			var resp map[string]interface{}
			Expect(json.Unmarshal([]byte(result), &resp)).To(Succeed())

			historyObj, ok := resp["remediation_history"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "remediation_history should be an object")
			tier1, ok := historyObj["tier1"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(tier1).To(HaveLen(1))
		})

		It("should return empty history object when no remediation history exists", func() {
			k8s := &fakeK8s{chain: nil}
			ds := &fakeDS{history: nil}

			tool := custom.NewNamespacedResourceContextTool(ds, k8s)
			result, err := tool.Execute(context.Background(),
				json.RawMessage(`{"kind":"Pod","name":"web","namespace":"default"}`))
			Expect(err).NotTo(HaveOccurred())

			var resp map[string]interface{}
			Expect(json.Unmarshal([]byte(result), &resp)).To(Succeed())
			Expect(resp).To(HaveKey("remediation_history"))
		})
	})

	Describe("UT-KA-433-112: get_namespaced_resource_context has valid JSON schema", func() {
		It("should return a non-nil parameter schema with required kind/name/namespace", func() {
			k8s := &fakeK8s{}
			ds := &fakeDS{}
			tool := custom.NewNamespacedResourceContextTool(ds, k8s)

			Expect(tool.Name()).To(Equal("get_namespaced_resource_context"))
			Expect(tool.Description()).NotTo(BeEmpty())

			params := tool.Parameters()
			Expect(params).NotTo(BeNil())

			var schema map[string]interface{}
			Expect(json.Unmarshal(params, &schema)).To(Succeed())
			Expect(schema["type"]).To(Equal("object"))

			required, ok := schema["required"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(required).To(ContainElement("kind"))
			Expect(required).To(ContainElement("name"))
			Expect(required).To(ContainElement("namespace"))
		})
	})

	Describe("UT-KA-433-115: get_cluster_resource_context returns root_owner without namespace", func() {
		It("should return root_owner with only kind and name", func() {
			ds := &fakeDS{history: &enrichment.RemediationHistoryResult{}}

			tool := custom.NewClusterResourceContextTool(ds)
			result, err := tool.Execute(context.Background(),
				json.RawMessage(`{"kind":"Node","name":"worker-1"}`))
			Expect(err).NotTo(HaveOccurred())

			var resp map[string]interface{}
			Expect(json.Unmarshal([]byte(result), &resp)).To(Succeed())

			rootOwner, ok := resp["root_owner"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "root_owner should be an object")
			Expect(rootOwner["kind"]).To(Equal("Node"))
			Expect(rootOwner["name"]).To(Equal("worker-1"))
			Expect(rootOwner).NotTo(HaveKey("namespace"))
		})
	})

	Describe("UT-KA-433-116: get_cluster_resource_context includes remediation_history", func() {
		It("should return remediation history from DataStorage", func() {
			ds := &fakeDS{history: &enrichment.RemediationHistoryResult{
				Tier1: []enrichment.Tier1Entry{
					{RemediationUID: "node-drain", Outcome: "success"},
				},
			}}

			tool := custom.NewClusterResourceContextTool(ds)
			result, err := tool.Execute(context.Background(),
				json.RawMessage(`{"kind":"Node","name":"worker-1"}`))
			Expect(err).NotTo(HaveOccurred())

			var resp map[string]interface{}
			Expect(json.Unmarshal([]byte(result), &resp)).To(Succeed())

			historyObj, ok := resp["remediation_history"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			tier1, ok := historyObj["tier1"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(tier1).To(HaveLen(1))
		})
	})

	Describe("UT-KA-433-117: get_cluster_resource_context has valid JSON schema", func() {
		It("should return a non-nil parameter schema with required kind/name", func() {
			ds := &fakeDS{}
			tool := custom.NewClusterResourceContextTool(ds)

			Expect(tool.Name()).To(Equal("get_cluster_resource_context"))
			Expect(tool.Description()).NotTo(BeEmpty())

			params := tool.Parameters()
			Expect(params).NotTo(BeNil())

			var schema map[string]interface{}
			Expect(json.Unmarshal(params, &schema)).To(Succeed())
			Expect(schema["type"]).To(Equal("object"))

			required, ok := schema["required"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(required).To(ContainElement("kind"))
			Expect(required).To(ContainElement("name"))
			Expect(required).NotTo(ContainElement("namespace"))
		})
	})

	Describe("UT-KA-433-118: get_cluster_resource_context omits detected_infrastructure", func() {
		It("should not include detected_infrastructure or quota_details in response", func() {
			ds := &fakeDS{history: nil}

			tool := custom.NewClusterResourceContextTool(ds)
			result, err := tool.Execute(context.Background(),
				json.RawMessage(`{"kind":"Namespace","name":"kube-system"}`))
			Expect(err).NotTo(HaveOccurred())

			Expect(result).NotTo(ContainSubstring("detected_infrastructure"))
			Expect(result).NotTo(ContainSubstring("quota_details"))
		})
	})

	Describe("UT-KA-433-119: AllToolNames includes both resource context tools", func() {
		It("should list 5 custom tool names", func() {
			Expect(custom.AllToolNames).To(HaveLen(5))
			Expect(custom.AllToolNames).To(ContainElement("get_namespaced_resource_context"))
			Expect(custom.AllToolNames).To(ContainElement("get_cluster_resource_context"))
		})
	})
})

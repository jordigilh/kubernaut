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
	"errors"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/tools/custom"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
)

// fakeK8s returns a configurable owner chain and spec hash. ownerChainCalls/
// specHashCalls (issue #2306, UT-KA-FLEET-033) let tests prove whether this
// hub-bound double was invoked at all -- distinguishing "hub client selected"
// from "overlay/no-op client selected" for a given investigation context.
type fakeK8s struct {
	chain       []enrichment.OwnerChainEntry
	err         error
	specHash    string
	specHashErr error

	capturedSpecHashKind      string
	capturedSpecHashName      string
	capturedSpecHashNamespace string

	ownerChainCalls int
	specHashCalls   int
}

func (f *fakeK8s) GetOwnerChain(_ context.Context, _, _, _, _ string) ([]enrichment.OwnerChainEntry, error) {
	f.ownerChainCalls++
	return f.chain, f.err
}

func (f *fakeK8s) GetSpecHash(_ context.Context, kind, name, namespace, _ string) (string, error) {
	f.specHashCalls++
	f.capturedSpecHashKind = kind
	f.capturedSpecHashName = name
	f.capturedSpecHashNamespace = namespace
	return f.specHash, f.specHashErr
}

// fakeDS returns configurable remediation history and captures the specHash/
// clusterID arguments (clusterID added for UT-KA-FLEET-034, issue #2306).
type fakeDS struct {
	history *enrichment.RemediationHistoryResult
	err     error

	capturedSpecHash  string
	capturedClusterID string
}

func (f *fakeDS) GetRemediationHistory(_ context.Context, _, _, _, clusterID, specHash string) (*enrichment.RemediationHistoryResult, error) {
	f.capturedSpecHash = specHash
	f.capturedClusterID = clusterID
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

			tool := custom.NewNamespacedResourceContextTool(ds, k8s, logr.Discard())
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

			tool := custom.NewNamespacedResourceContextTool(ds, k8s, logr.Discard())
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

			tool := custom.NewNamespacedResourceContextTool(ds, k8s, logr.Discard())
			result, err := tool.Execute(context.Background(),
				json.RawMessage(`{"kind":"Pod","name":"web","namespace":"prod"}`))
			Expect(err).NotTo(HaveOccurred())

			var resp map[string]interface{}
			Expect(json.Unmarshal([]byte(result), &resp)).To(Succeed())

			historyObj, ok := resp["remediation_history"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "remediation_history should be an object")
			tier1, ok := historyObj["tier1"].([]interface{})
			Expect(ok).To(BeTrue(), `expected key "tier1" in remediation_history to be a []interface{}`)
			Expect(tier1).To(HaveLen(1))
		})

		It("should return empty history object when no remediation history exists", func() {
			k8s := &fakeK8s{chain: nil}
			ds := &fakeDS{history: nil}

			tool := custom.NewNamespacedResourceContextTool(ds, k8s, logr.Discard())
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
			tool := custom.NewNamespacedResourceContextTool(ds, k8s, logr.Discard())

			Expect(tool.Name()).To(Equal("get_namespaced_resource_context"))
			Expect(tool.Description()).NotTo(BeEmpty())

			params := tool.Parameters()
			Expect(params).NotTo(BeNil())

			var schema map[string]interface{}
			Expect(json.Unmarshal(params, &schema)).To(Succeed())
			Expect(schema["type"]).To(Equal("object"))

			required, ok := schema["required"].([]interface{})
			Expect(ok).To(BeTrue(), `expected "required" in get_namespaced_resource_context schema to be a []interface{}`)
			Expect(required).To(ContainElement("kind"))
			Expect(required).To(ContainElement("name"))
			Expect(required).To(ContainElement("namespace"))
		})
	})

	Describe("UT-KA-433-115: get_cluster_resource_context returns root_owner without namespace", func() {
		It("should return root_owner with only kind and name", func() {
			k8s := &fakeK8s{}
			ds := &fakeDS{history: &enrichment.RemediationHistoryResult{}}

			tool := custom.NewClusterResourceContextTool(ds, k8s, logr.Discard())
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
			k8s := &fakeK8s{}
			ds := &fakeDS{history: &enrichment.RemediationHistoryResult{
				Tier1: []enrichment.Tier1Entry{
					{RemediationUID: "node-drain", Outcome: "success"},
				},
			}}

			tool := custom.NewClusterResourceContextTool(ds, k8s, logr.Discard())
			result, err := tool.Execute(context.Background(),
				json.RawMessage(`{"kind":"Node","name":"worker-1"}`))
			Expect(err).NotTo(HaveOccurred())

			var resp map[string]interface{}
			Expect(json.Unmarshal([]byte(result), &resp)).To(Succeed())

			historyObj, ok := resp["remediation_history"].(map[string]interface{})
			Expect(ok).To(BeTrue(), `expected key "remediation_history" in response to be a map[string]interface{}`)
			tier1, ok := historyObj["tier1"].([]interface{})
			Expect(ok).To(BeTrue(), `expected key "tier1" in remediation_history to be a []interface{}`)
			Expect(tier1).To(HaveLen(1))
		})
	})

	Describe("UT-KA-433-117: get_cluster_resource_context has valid JSON schema", func() {
		It("should return a non-nil parameter schema with required kind/name", func() {
			k8s := &fakeK8s{}
			ds := &fakeDS{}
			tool := custom.NewClusterResourceContextTool(ds, k8s, logr.Discard())

			Expect(tool.Name()).To(Equal("get_cluster_resource_context"))
			Expect(tool.Description()).NotTo(BeEmpty())

			params := tool.Parameters()
			Expect(params).NotTo(BeNil())

			var schema map[string]interface{}
			Expect(json.Unmarshal(params, &schema)).To(Succeed())
			Expect(schema["type"]).To(Equal("object"))

			required, ok := schema["required"].([]interface{})
			Expect(ok).To(BeTrue(), `expected "required" in get_cluster_resource_context schema to be a []interface{}`)
			Expect(required).To(ContainElement("kind"))
			Expect(required).To(ContainElement("name"))
			Expect(required).NotTo(ContainElement("namespace"))
		})
	})

	Describe("UT-KA-433-118: get_cluster_resource_context omits detected_infrastructure", func() {
		It("should not include detected_infrastructure or quota_details in response", func() {
			k8s := &fakeK8s{}
			ds := &fakeDS{history: nil}

			tool := custom.NewClusterResourceContextTool(ds, k8s, logr.Discard())
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

	// --- Issue #729: specHash computation ---

	Describe("UT-KA-729-001: get_namespaced_resource_context computes specHash on rootOwner — BR-AI-056", func() {
		It("should call GetSpecHash with rootOwner coordinates and forward hash to DS", func() {
			k8s := &fakeK8s{
				chain: []enrichment.OwnerChainEntry{
					{Kind: "ReplicaSet", Name: "api-server-abc", Namespace: "production"},
					{Kind: "Deployment", Name: "api-server", Namespace: "production"},
				},
				specHash: "abc123def456",
			}
			ds := &fakeDS{history: &enrichment.RemediationHistoryResult{}}

			tool := custom.NewNamespacedResourceContextTool(ds, k8s, logr.Discard())
			_, err := tool.Execute(context.Background(),
				json.RawMessage(`{"kind":"Pod","name":"api-server-abc-xyz","namespace":"production"}`))
			Expect(err).NotTo(HaveOccurred())

			Expect(k8s.capturedSpecHashKind).To(Equal("Deployment"))
			Expect(k8s.capturedSpecHashName).To(Equal("api-server"))
			Expect(k8s.capturedSpecHashNamespace).To(Equal("production"))
			Expect(ds.capturedSpecHash).To(Equal("abc123def456"))
		})
	})

	Describe("UT-KA-729-002: get_namespaced_resource_context uses input resource when no owner chain — BR-AI-056", func() {
		It("should compute specHash on input resource when chain is empty", func() {
			k8s := &fakeK8s{
				chain:    nil,
				specHash: "orphan-hash-789",
			}
			ds := &fakeDS{history: &enrichment.RemediationHistoryResult{}}

			tool := custom.NewNamespacedResourceContextTool(ds, k8s, logr.Discard())
			_, err := tool.Execute(context.Background(),
				json.RawMessage(`{"kind":"Pod","name":"orphan-pod","namespace":"default"}`))
			Expect(err).NotTo(HaveOccurred())

			Expect(k8s.capturedSpecHashKind).To(Equal("Pod"))
			Expect(k8s.capturedSpecHashName).To(Equal("orphan-pod"))
			Expect(k8s.capturedSpecHashNamespace).To(Equal("default"))
			Expect(ds.capturedSpecHash).To(Equal("orphan-hash-789"))
		})
	})

	Describe("UT-KA-729-003: get_namespaced_resource_context degrades gracefully on specHash failure — BR-AI-056", func() {
		It("should pass empty specHash to DS when GetSpecHash fails", func() {
			k8s := &fakeK8s{
				chain:       nil,
				specHashErr: errors.New("GVR resolution failed"),
			}
			ds := &fakeDS{history: &enrichment.RemediationHistoryResult{}}

			tool := custom.NewNamespacedResourceContextTool(ds, k8s, logr.Discard())
			result, err := tool.Execute(context.Background(),
				json.RawMessage(`{"kind":"CustomResource","name":"my-cr","namespace":"test"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeEmpty())
			Expect(ds.capturedSpecHash).To(Equal(""))
		})
	})

	Describe("UT-KA-729-004: get_cluster_resource_context computes specHash — BR-AI-056", func() {
		It("should call GetSpecHash with cluster-scoped coordinates and forward hash to DS", func() {
			k8s := &fakeK8s{specHash: "node-hash-abc"}
			ds := &fakeDS{history: &enrichment.RemediationHistoryResult{}}

			tool := custom.NewClusterResourceContextTool(ds, k8s, logr.Discard())
			_, err := tool.Execute(context.Background(),
				json.RawMessage(`{"kind":"Node","name":"worker-1"}`))
			Expect(err).NotTo(HaveOccurred())

			Expect(k8s.capturedSpecHashKind).To(Equal("Node"))
			Expect(k8s.capturedSpecHashName).To(Equal("worker-1"))
			Expect(k8s.capturedSpecHashNamespace).To(Equal(""))
			Expect(ds.capturedSpecHash).To(Equal("node-hash-abc"))
		})
	})

	Describe("UT-KA-729-005: get_cluster_resource_context degrades gracefully on specHash failure — BR-AI-056", func() {
		It("should pass empty specHash when GetSpecHash fails for cluster resource", func() {
			k8s := &fakeK8s{specHashErr: errors.New("node not found")}
			ds := &fakeDS{history: &enrichment.RemediationHistoryResult{}}

			tool := custom.NewClusterResourceContextTool(ds, k8s, logr.Discard())
			result, err := tool.Execute(context.Background(),
				json.RawMessage(`{"kind":"Node","name":"missing-node"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeEmpty())
			Expect(ds.capturedSpecHash).To(Equal(""))
		})
	})

	// --- Issue #2306: fleet-target K8sClient selection ---

	Describe("UT-KA-FLEET-033: resourceContextTools select the correct K8sClient per investigation type — BR-INTEGRATION-1489", func() {
		It("uses the hub-bound K8sClient for a hub-local investigation (no fleet overlay in ctx) — zero regression", func() {
			k8s := &fakeK8s{chain: []enrichment.OwnerChainEntry{{Kind: "Deployment", Name: "api-server", Namespace: "prod"}}}
			ds := &fakeDS{history: &enrichment.RemediationHistoryResult{}}

			tool := custom.NewNamespacedResourceContextTool(ds, k8s, logr.Discard())
			_, err := tool.Execute(context.Background(),
				json.RawMessage(`{"kind":"Pod","name":"api-server-abc-xyz","namespace":"prod"}`))
			Expect(err).NotTo(HaveOccurred())

			Expect(k8s.ownerChainCalls).To(Equal(1), "hub-bound K8sClient must be used when ctx carries no fleet overlay")
			Expect(k8s.specHashCalls).To(Equal(1))
		})

		It("uses the overlay K8sClient (not the hub-bound one) when ctx carries a fleet overlay publishing resources_get", func() {
			k8s := &fakeK8s{chain: []enrichment.OwnerChainEntry{{Kind: "Deployment", Name: "hub-should-not-be-called"}}}
			ds := &fakeDS{history: &enrichment.RemediationHistoryResult{}}
			getTool := &fakeGetTool{
				responseJSON: `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"standalone-pod","namespace":"prod"}}`,
			}

			ctx := investigator.WithFleetOverlay(context.Background(), map[string]tools.Tool{"resources_get": getTool})

			tool := custom.NewNamespacedResourceContextTool(ds, k8s, logr.Discard())
			result, err := tool.Execute(ctx,
				json.RawMessage(`{"kind":"Pod","name":"standalone-pod","namespace":"prod"}`))
			Expect(err).NotTo(HaveOccurred())

			Expect(k8s.ownerChainCalls).To(Equal(0), "hub-bound K8sClient must NOT be used for a fleet-target investigation")
			Expect(k8s.specHashCalls).To(Equal(0))
			Expect(getTool.calls).To(BeNumerically(">=", 1), "the overlay's resources_get tool must be invoked instead")

			var resp map[string]interface{}
			Expect(json.Unmarshal([]byte(result), &resp)).To(Succeed())
			rootOwner := resp["root_owner"].(map[string]interface{})
			Expect(rootOwner["name"]).To(Equal("standalone-pod"))
		})

		It("degrades to a clear 'not available' error (not the hub) when the fleet overlay lacks resources_get", func() {
			k8s := &fakeK8s{chain: []enrichment.OwnerChainEntry{{Kind: "Deployment", Name: "hub-should-not-be-called"}}}
			ds := &fakeDS{history: &enrichment.RemediationHistoryResult{}}

			// Overlay is non-empty (has some other tool) but does not publish resources_get.
			ctx := investigator.WithFleetOverlay(context.Background(), map[string]tools.Tool{"kubectl_list": &fakeGetTool{}})

			tool := custom.NewNamespacedResourceContextTool(ds, k8s, logr.Discard())
			result, err := tool.Execute(ctx,
				json.RawMessage(`{"kind":"Pod","name":"orphan-pod","namespace":"prod"}`))
			Expect(err).NotTo(HaveOccurred(), "the tool call itself must not fail -- owner-chain/spec-hash degrade gracefully")

			Expect(k8s.ownerChainCalls).To(Equal(0), "hub-bound K8sClient must NOT be used as a silent fallback for a fleet-target investigation")
			Expect(k8s.specHashCalls).To(Equal(0))

			var resp map[string]interface{}
			Expect(json.Unmarshal([]byte(result), &resp)).To(Succeed())
			rootOwner := resp["root_owner"].(map[string]interface{})
			Expect(rootOwner["name"]).To(Equal("orphan-pod"), "falls back to the input resource, matching existing GetOwnerChain-error behavior")
		})
	})

	Describe("UT-KA-FLEET-034: fetchRemediationHistory threads the target clusterID — BR-INTEGRATION-1489", func() {
		It("passes audit.ClusterIDFromContext(ctx) to ds.GetRemediationHistory for a fleet-target investigation", func() {
			k8s := &fakeK8s{chain: []enrichment.OwnerChainEntry{{Kind: "Deployment", Name: "api-server", Namespace: "prod"}}}
			ds := &fakeDS{history: &enrichment.RemediationHistoryResult{}}

			ctx := audit.WithClusterID(context.Background(), "spoke-east-1")

			tool := custom.NewNamespacedResourceContextTool(ds, k8s, logr.Discard())
			_, err := tool.Execute(ctx,
				json.RawMessage(`{"kind":"Pod","name":"api-server-abc-xyz","namespace":"prod"}`))
			Expect(err).NotTo(HaveOccurred())

			Expect(ds.capturedClusterID).To(Equal("spoke-east-1"))
		})

		It("passes an empty clusterID for a hub-local investigation — zero regression", func() {
			k8s := &fakeK8s{chain: nil}
			ds := &fakeDS{history: &enrichment.RemediationHistoryResult{}}

			tool := custom.NewClusterResourceContextTool(ds, k8s, logr.Discard())
			_, err := tool.Execute(context.Background(),
				json.RawMessage(`{"kind":"Node","name":"worker-1"}`))
			Expect(err).NotTo(HaveOccurred())

			Expect(ds.capturedClusterID).To(Equal(""))
		})
	})
})

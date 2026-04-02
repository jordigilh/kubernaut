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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/custom"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
)

// fakeK8sClient provides owner chain resolution for resource context tools.
type fakeK8sClient struct{}

func (f *fakeK8sClient) GetOwnerChain(_ context.Context, kind, name, ns string) ([]enrichment.OwnerChainEntry, error) {
	return []enrichment.OwnerChainEntry{
		{Kind: kind, Name: name, Namespace: ns},
		{Kind: "ReplicaSet", Name: "api-server-abc", Namespace: ns},
		{Kind: "Deployment", Name: "api-server", Namespace: ns},
	}, nil
}

// fakeEnrichDS provides remediation history for resource context tools.
type fakeEnrichDS struct{}

func (f *fakeEnrichDS) GetRemediationHistory(_ context.Context, _, _, _, _ string) ([]enrichment.RemediationHistoryEntry, error) {
	return []enrichment.RemediationHistoryEntry{
		{WorkflowID: "oom-recovery", Outcome: "success", Timestamp: "2026-02-15T10:00:00Z"},
	}, nil
}

var _ = Describe("Kubernaut Agent Custom Tools Integration — #433", func() {

	var reg *registry.Registry

	BeforeEach(func() {
		Expect(ogenClient).NotTo(BeNil(), "ogen client must be initialized by SynchronizedBeforeSuite")

		k8s := &fakeK8sClient{}
		dsEnrich := &fakeEnrichDS{}
		reg = registry.New()

		allTools := custom.NewAllTools(ogenClient, k8s, dsEnrich)
		Expect(allTools).To(HaveLen(5), "should create 5 custom tools")
		for _, t := range allTools {
			reg.Register(t)
		}
	})

	Describe("IT-KA-433-033: list_available_actions queries real DataStorage API", func() {
		It("should return action types from the real DataStorage", func() {
			result, err := reg.Execute(context.Background(), "list_available_actions",
				json.RawMessage(`{"severity":"critical","component":"deployment","environment":"production","priority":"P0"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeEmpty())
			// DS bootstrap seeds standard action types — validate at least one comes back
			Expect(result).To(ContainSubstring("actionTypes"))
		})
	})

	Describe("IT-KA-433-034: list_workflows searches real DataStorage with criteria", func() {
		It("should return seeded workflows from real DataStorage", func() {
			result, err := reg.Execute(context.Background(), "list_workflows",
				json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeEmpty())
			Expect(result).To(ContainSubstring("workflows"))
		})
	})

	Describe("IT-KA-433-035: get_workflow retrieves specific workflow from real DataStorage", func() {
		It("should return the seeded workflow definition by UUID", func() {
			Expect(workflowUUIDs).NotTo(BeEmpty(), "workflow UUIDs must be seeded")

			var wfUUID string
			for _, v := range workflowUUIDs {
				wfUUID = v
				break
			}
			Expect(wfUUID).NotTo(BeEmpty())

			result, err := reg.Execute(context.Background(), "get_workflow",
				json.RawMessage(fmt.Sprintf(`{"id":"%s"}`, wfUUID)))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("oom-recovery"))
		})
	})

	Describe("IT-KA-433-036: get_namespaced_resource_context combines K8s owner chain + DS remediation history", func() {
		It("should return combined root owner and remediation history", func() {
			result, err := reg.Execute(context.Background(), "get_namespaced_resource_context",
				json.RawMessage(`{"kind":"Pod","name":"api-server-abc-xyz","namespace":"production"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("Deployment"))
			Expect(result).To(ContainSubstring("oom-recovery"))
		})
	})
})

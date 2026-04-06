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

package registry_test

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/k8s"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	"k8s.io/client-go/kubernetes/fake"
)

type nopResolver struct{}

func (n *nopResolver) Get(_ context.Context, _, _, _ string) (interface{}, error) { return nil, nil }
func (n *nopResolver) List(_ context.Context, _, _ string) (interface{}, error)   { return nil, nil }

var _ k8s.ResourceResolver = (*nopResolver)(nil)

var _ = Describe("Kubernaut Agent Tool Registry — #433", func() {

	Describe("UT-KA-433-029: Baseline K8s tools satisfy Tool interface", func() {
		It("should create 18 baseline tools implementing the Tool interface", func() {
			client := fake.NewSimpleClientset()
			allTools := k8s.NewAllTools(client, &nopResolver{})
			Expect(allTools).NotTo(BeNil(), "NewAllTools should not return nil")
			Expect(allTools).To(HaveLen(19), "should create exactly 19 baseline K8s tools")

			for _, t := range allTools {
				Expect(t.Name()).NotTo(BeEmpty(), "tool Name() should not be empty")
				Expect(t.Description()).NotTo(BeEmpty(), "tool Description() should not be empty")
			}
		})

		It("should have unique names across all tools", func() {
			client := fake.NewSimpleClientset()
			allTools := k8s.NewAllTools(client, &nopResolver{})
			Expect(allTools).NotTo(BeNil())

			names := make(map[string]bool)
			for _, t := range allTools {
				Expect(names).NotTo(HaveKey(t.Name()), "duplicate tool name: "+t.Name())
				names[t.Name()] = true
			}
		})

		It("should match the canonical tool name list", func() {
			client := fake.NewSimpleClientset()
			allTools := k8s.NewAllTools(client, &nopResolver{})
			Expect(allTools).NotTo(BeNil())

			actualNames := make([]string, len(allTools))
			for i, t := range allTools {
				actualNames[i] = t.Name()
			}
			for _, expected := range k8s.AllToolNames {
				Expect(actualNames).To(ContainElement(expected),
					"missing tool: "+expected)
			}
		})
	})

	Describe("UT-KA-433-030: Tool registry registers all tools and resolves by name", func() {
		It("should register and resolve each tool by name", func() {
			reg := registry.New()
			client := fake.NewSimpleClientset()
			allTools := k8s.NewAllTools(client, &nopResolver{})
			Expect(allTools).NotTo(BeNil())

			for _, t := range allTools {
				reg.Register(t)
			}

			all := reg.All()
			Expect(all).To(HaveLen(19), "registry should contain 19 tools")

			for _, expected := range k8s.AllToolNames {
				tool, err := reg.Get(expected)
				Expect(err).NotTo(HaveOccurred(), "should resolve tool: "+expected)
				Expect(tool.Name()).To(Equal(expected))
			}
		})
	})

	Describe("UT-KA-433-031: Registry ToolsForPhase returns correct tool subset", func() {
		It("should return only RCA-phase tools for PhaseRCA", func() {
			reg := registry.New()
			client := fake.NewSimpleClientset()
			for _, t := range k8s.NewAllTools(client, &nopResolver{}) {
				reg.Register(t)
			}

			phaseTools := katypes.PhaseToolMap{
				katypes.PhaseRCA:               {"kubectl_describe", "kubectl_logs"},
				katypes.PhaseWorkflowDiscovery: {},
			}

			rcaTools := reg.ToolsForPhase(katypes.PhaseRCA, phaseTools)
			Expect(rcaTools).To(HaveLen(2))

			names := toolNames(rcaTools)
			Expect(names).To(ContainElement("kubectl_describe"))
			Expect(names).To(ContainElement("kubectl_logs"))
		})
	})

	Describe("UT-KA-433-032: Registry rejects execution of unregistered tool", func() {
		It("should return ErrToolNotFound for unregistered tool name", func() {
			reg := registry.New()
			_, err := reg.Execute(context.Background(), "nonexistent_tool", json.RawMessage(`{}`))
			Expect(err).To(HaveOccurred())
			var notFound *registry.ErrToolNotFound
			Expect(err).To(BeAssignableToTypeOf(notFound))
		})
	})
})

func toolNames(tt []tools.Tool) []string {
	names := make([]string, len(tt))
	for i, t := range tt {
		names[i] = t.Name()
	}
	return names
}

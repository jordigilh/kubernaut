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

package security_test

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
)

// fakeTool implements tools.Tool for testing phase scoping.
type fakeTool struct {
	name string
}

func (t *fakeTool) Name() string               { return t.name }
func (t *fakeTool) Description() string         { return "fake tool for testing" }
func (t *fakeTool) Parameters() json.RawMessage { return json.RawMessage(`{}`) }
func (t *fakeTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	return `{"result":"ok"}`, nil
}

var _ tools.Tool = (*fakeTool)(nil)

var _ = Describe("Kubernaut Agent I4 Phase Scoping Integration — #433", func() {

	var (
		reg      *registry.Registry
		ptm      katypes.PhaseToolMap
	)

	BeforeEach(func() {
		reg = registry.New()
		ptm = investigator.DefaultPhaseToolMap()

		for _, phase := range []katypes.Phase{katypes.PhaseRCA, katypes.PhaseWorkflowDiscovery, katypes.PhaseValidation} {
			for _, name := range ptm[phase] {
				reg.Register(&fakeTool{name: name})
			}
		}
	})

	Describe("IT-KA-433-038: Off-phase tool call rejected with error", func() {
		It("should reject a workflow tool during the RCA phase", func() {
			rcaTools := reg.ToolsForPhase(katypes.PhaseRCA, ptm)
			rcaToolNames := make([]string, len(rcaTools))
			for i, t := range rcaTools {
				rcaToolNames[i] = t.Name()
			}

			By("Workflow discovery tools should NOT be available during RCA phase")
			Expect(rcaToolNames).NotTo(ContainElement("list_workflows"))
			Expect(rcaToolNames).NotTo(ContainElement("get_workflow"))
			Expect(rcaToolNames).NotTo(ContainElement("list_available_actions"))

			By("K8s and Prometheus tools SHOULD be available during RCA phase")
			Expect(rcaToolNames).To(ContainElement("kubectl_describe"))
			Expect(rcaToolNames).To(ContainElement("execute_prometheus_instant_query"))
		})

		It("should reject K8s tools during the WorkflowDiscovery phase", func() {
			wdTools := reg.ToolsForPhase(katypes.PhaseWorkflowDiscovery, ptm)
			wdToolNames := make([]string, len(wdTools))
			for i, t := range wdTools {
				wdToolNames[i] = t.Name()
			}

			By("K8s tools should NOT be available during WorkflowDiscovery phase")
			Expect(wdToolNames).NotTo(ContainElement("kubectl_describe"))
			Expect(wdToolNames).NotTo(ContainElement("kubectl_logs"))

			By("Resource context tools should NOT be in WorkflowDiscovery phase (they are in RCA)")
			Expect(wdToolNames).NotTo(ContainElement("get_namespaced_resource_context"))
			Expect(wdToolNames).NotTo(ContainElement("get_cluster_resource_context"))

			By("Custom tools SHOULD be available during WorkflowDiscovery phase")
			Expect(wdToolNames).To(ContainElement("list_workflows"))
			Expect(wdToolNames).To(ContainElement("get_workflow"))
			Expect(wdToolNames).To(ContainElement("list_available_actions"))
		})

		It("should return only TodoWrite during the Validation phase", func() {
			valTools := reg.ToolsForPhase(katypes.PhaseValidation, ptm)
			Expect(valTools).To(HaveLen(1), "Validation phase should have TodoWrite only")
			Expect(valTools[0].Name()).To(Equal("todo_write"))
		})
	})

	Describe("IT-KA-433-039: Phase transition correctly updates available tool set", func() {
		It("should expose different tool sets as phase progresses from RCA to WorkflowDiscovery to Validation", func() {
			phases := []katypes.Phase{katypes.PhaseRCA, katypes.PhaseWorkflowDiscovery, katypes.PhaseValidation}
			var prevToolNames []string

			for _, phase := range phases {
				phaseTools := reg.ToolsForPhase(phase, ptm)
				toolNames := make([]string, len(phaseTools))
				for i, t := range phaseTools {
					toolNames[i] = t.Name()
				}

				if prevToolNames != nil {
					By("Tool set should change between phases")
					Expect(toolNames).NotTo(Equal(prevToolNames),
						"phase %v should have different tools than previous phase", phase)
				}
				prevToolNames = toolNames
			}
		})

		It("should enforce that RCA has 33 tools, WorkflowDiscovery has 4, Validation has 1", func() {
			rcaTools := reg.ToolsForPhase(katypes.PhaseRCA, ptm)
			Expect(rcaTools).To(HaveLen(33), "RCA should have 19 K8s + 2 metrics + 1 fetch_pod_logs + 8 Prometheus + 2 resource context + 1 TodoWrite = 33 tools")

			wdTools := reg.ToolsForPhase(katypes.PhaseWorkflowDiscovery, ptm)
			Expect(wdTools).To(HaveLen(4), "WorkflowDiscovery should have 3 workflow + 1 TodoWrite = 4 tools")

			valTools := reg.ToolsForPhase(katypes.PhaseValidation, ptm)
			Expect(valTools).To(HaveLen(1), "Validation should have 1 TodoWrite tool")
		})
	})
})

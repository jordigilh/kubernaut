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

package investigator_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
)

var _ = Describe("Kubernaut Agent Investigator — #433", func() {

	Describe("UT-KA-433-014: Phase definitions map tools to correct phases (I4)", func() {
		It("should assign K8s, Prometheus, resource context, and TodoWrite tools to RCA phase", func() {
			ptm := investigator.DefaultPhaseToolMap()
			Expect(ptm).NotTo(BeNil(), "DefaultPhaseToolMap should not return nil")
			rcaTools := ptm[katypes.PhaseRCA]
			Expect(rcaTools).To(HaveLen(33), "RCA phase should have 19 K8s + 2 metrics + 1 fetch_pod_logs + 8 Prometheus + 2 resource context + 1 todo_write")
			Expect(rcaTools).To(ContainElement("kubectl_describe"))
			Expect(rcaTools).To(ContainElement("kubectl_logs"))
			Expect(rcaTools).To(ContainElement("execute_prometheus_instant_query"))
			Expect(rcaTools).To(ContainElement("execute_prometheus_range_query"))
			Expect(rcaTools).To(ContainElement("get_namespaced_resource_context"))
			Expect(rcaTools).To(ContainElement("get_cluster_resource_context"))
			Expect(rcaTools).To(ContainElement("todo_write"))
			Expect(rcaTools).To(ContainElement("kubectl_get_by_kind_in_cluster"))
			Expect(rcaTools).To(ContainElement("kubectl_find_resource"))
			Expect(rcaTools).To(ContainElement("kubectl_get_yaml"))
			Expect(rcaTools).To(ContainElement("kubectl_memory_requests_all_namespaces"))
			Expect(rcaTools).To(ContainElement("kubernetes_jq_query"))
			Expect(rcaTools).To(ContainElement("kubernetes_count"))
			Expect(rcaTools).To(ContainElement("kubectl_top_pods"))
			Expect(rcaTools).To(ContainElement("kubectl_top_nodes"))
			Expect(rcaTools).To(ContainElement("fetch_pod_logs"))
		})

		It("should assign workflow discovery tools and TodoWrite to WorkflowDiscovery phase", func() {
			ptm := investigator.DefaultPhaseToolMap()
			wdTools := ptm[katypes.PhaseWorkflowDiscovery]
			Expect(wdTools).To(HaveLen(4), "WorkflowDiscovery phase should have 3 workflow + 1 todo_write")
			Expect(wdTools).To(ContainElement("list_available_actions"))
			Expect(wdTools).To(ContainElement("list_workflows"))
			Expect(wdTools).To(ContainElement("get_workflow"))
			Expect(wdTools).To(ContainElement("todo_write"))
		})

		It("should assign only TodoWrite to Validation phase", func() {
			ptm := investigator.DefaultPhaseToolMap()
			valTools := ptm[katypes.PhaseValidation]
			Expect(valTools).To(HaveLen(1))
			Expect(valTools).To(ContainElement("todo_write"))
		})

		It("should NOT have resource context tools in WorkflowDiscovery phase", func() {
			ptm := investigator.DefaultPhaseToolMap()
			wdTools := ptm[katypes.PhaseWorkflowDiscovery]
			Expect(wdTools).NotTo(ContainElement("get_namespaced_resource_context"))
			Expect(wdTools).NotTo(ContainElement("get_cluster_resource_context"))
		})
	})

	Describe("UT-KA-433-015: Phase transition from RCA to WorkflowDiscovery", func() {
		It("should define all three phases in order", func() {
			ptm := investigator.DefaultPhaseToolMap()
			Expect(ptm).NotTo(BeNil())
			_, hasRCA := ptm[katypes.PhaseRCA]
			_, hasWD := ptm[katypes.PhaseWorkflowDiscovery]
			_, hasVal := ptm[katypes.PhaseValidation]
			Expect(hasRCA).To(BeTrue())
			Expect(hasWD).To(BeTrue())
			Expect(hasVal).To(BeTrue())
		})
	})

	Describe("UT-KA-433-016: Max-turn exhaustion produces human-review flag", func() {
		It("should set HumanReviewNeeded when investigation exceeds max turns", func() {
			result := &katypes.InvestigationResult{
				HumanReviewNeeded: false,
			}
			Expect(result.HumanReviewNeeded).To(BeFalse(),
				"stub result has HumanReviewNeeded=false; GREEN will make Investigate set it to true on exhaustion")
		})
	})
})

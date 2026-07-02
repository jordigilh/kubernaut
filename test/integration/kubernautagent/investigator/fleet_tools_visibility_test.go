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

package investigator

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// IT-KA-FLEET-001: Fleet tools visible in RCA phase tool map
//
// Business Outcome: When fleet tools are discovered at startup from MCP Gateway,
// they must appear in the PhaseToolMap[PhaseRCA] so the LLM investigator
// can select them during RCA. Without this wiring, remote cluster tools
// remain undiscoverable by the LLM even when registered.
//
// Wiring Manifest Row:
//   Component: AppendFleetToolsToRCA
//   Production Entry Point: cmd/kubernautagent/main.go:231
//   Wiring Code Location: internal/kubernautagent/investigator/types.go
//   IT Test ID: IT-KA-FLEET-001
var _ = Describe("KA Fleet Tool Visibility (BR-INTEGRATION-065)", Label("fleet", "integration"), func() {
	It("IT-KA-FLEET-001: fleet tools should be visible in RCA phase after AppendFleetToolsToRCA", func() {
		ptm := investigator.DefaultPhaseToolMap()

		originalRCALen := len(ptm[katypes.PhaseRCA])

		fleetToolNames := []string{
			"fleet_get_pods_prod-east",
			"fleet_get_deployments_prod-east",
			"fleet_list_nodes_prod-west",
		}

		investigator.AppendFleetToolsToRCA(ptm, fleetToolNames)

		Expect(ptm[katypes.PhaseRCA]).To(HaveLen(originalRCALen + 3),
			"IT-KA-FLEET-001: RCA phase must contain original tools + fleet tools")

		for _, name := range fleetToolNames {
			Expect(ptm[katypes.PhaseRCA]).To(ContainElement(name),
				"IT-KA-FLEET-001: fleet tool %q must be in RCA phase", name)
		}

		Expect(ptm[katypes.PhaseWorkflowDiscovery]).ToNot(ContainElement("fleet_get_pods_prod-east"),
			"IT-KA-FLEET-001: fleet tools must NOT leak into non-RCA phases")
	})

	It("IT-KA-FLEET-002: empty fleet tools should not corrupt the phase map", func() {
		ptm := investigator.DefaultPhaseToolMap()
		originalRCALen := len(ptm[katypes.PhaseRCA])

		investigator.AppendFleetToolsToRCA(ptm, nil)

		Expect(ptm[katypes.PhaseRCA]).To(HaveLen(originalRCALen),
			"IT-KA-FLEET-002: nil fleet tools must not change RCA phase length")

		investigator.AppendFleetToolsToRCA(ptm, []string{})

		Expect(ptm[katypes.PhaseRCA]).To(HaveLen(originalRCALen),
			"IT-KA-FLEET-002: empty fleet tools must not change RCA phase length")
	})
})

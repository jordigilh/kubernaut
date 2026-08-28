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
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// BR-INTEGRATION-1489, DD-FLEET-005, issue #2306: subtractive suppression of
// hub-bound local K8s tools during fleet-target investigations — pure logic
// tier (UT). Proves toolDefinitionsForPhase's own decision of which local
// tools to advertise in isolation, with no LLM or real registry.Execute
// involved. Wiring into Investigate()/RunInteractiveTurn is proven
// separately by IT-KA-FLEET-030/031 (fleet_prescoping_test.go).
var _ = Describe("Fleet-target local K8s tool suppression (BR-INTEGRATION-1489, DD-FLEET-005, #2306)", Label("fleet", "unit"), func() {

	var inv *Investigator

	newTestInvestigator := func() *Investigator {
		reg := registry.New()
		reg.Register(&fleetOverlayFakeTool{name: "kubectl_get_by_name"})
		reg.Register(&fleetOverlayFakeTool{name: "kubectl_logs"})
		reg.Register(&fleetOverlayFakeTool{name: "kubectl_top_pods"})
		reg.Register(&fleetOverlayFakeTool{name: "get_namespaced_resource_context"})
		return New(Config{
			Logger:     logr.Discard(),
			PhaseTools: DefaultPhaseToolMap(),
			Registry:   reg,
		})
	}

	BeforeEach(func() {
		inv = newTestInvestigator()
	})

	Describe("UT-KA-FLEET-029 [AC-6]: local k8s/metrics/node-proxy tools are excluded for a fleet-target investigation", func() {
		It("excludes kubectl_get_by_name, kubectl_logs, and kubectl_top_pods from the RCA schema when a fleet overlay is present", func() {
			overlay := map[string]tools.Tool{
				// An overlay entry for a name distinct from every suppressed
				// local tool -- proves suppression is driven by the local
				// tool's own name being in the suppressed set, not merely by
				// "an overlay exists at all".
				"resources_get": &fleetOverlayFakeTool{name: "resources_get"},
			}
			ctx := WithFleetOverlay(context.Background(), overlay)

			defs := inv.toolDefinitionsForPhase(ctx, katypes.PhaseRCA)

			names := toolNames(defs)
			Expect(names).NotTo(ContainElement("kubectl_get_by_name"),
				"UT-KA-FLEET-029: a hub-bound local k8s tool must not be advertised to the LLM "+
					"during a fleet-target investigation unless the overlay provides a same-named override (AC-6)")
			Expect(names).NotTo(ContainElement("kubectl_logs"))
			Expect(names).NotTo(ContainElement("kubectl_top_pods"))
		})

		It("does not suppress a fleet-agnostic tool like get_namespaced_resource_context", func() {
			overlay := map[string]tools.Tool{
				"resources_get": &fleetOverlayFakeTool{name: "resources_get"},
			}
			ctx := WithFleetOverlay(context.Background(), overlay)

			defs := inv.toolDefinitionsForPhase(ctx, katypes.PhaseRCA)

			Expect(toolNames(defs)).To(ContainElement("get_namespaced_resource_context"),
				"UT-KA-FLEET-029: a DataStorage-backed, fleet-agnostic tool must remain visible "+
					"regardless of fleet targeting -- suppression is scoped to client-go/dynamic-client-backed "+
					"RCA tools only")
		})

		It("does not suppress a local tool that the overlay overrides by exact name", func() {
			overlayTool := &fleetOverlayFakeTool{name: "kubectl_get_by_name"}
			overlay := map[string]tools.Tool{
				"kubectl_get_by_name": overlayTool,
			}
			ctx := WithFleetOverlay(context.Background(), overlay)

			defs := inv.toolDefinitionsForPhase(ctx, katypes.PhaseRCA)

			Expect(toolNames(defs)).To(ContainElement("kubectl_get_by_name"),
				"UT-KA-FLEET-029: a suppressed local tool name must still be advertised when the "+
					"fleet overlay itself supplies a same-named override -- suppression must never hide "+
					"a name the overlay explicitly re-exposes")
		})
	})

	Describe("UT-KA-FLEET-030 [zero regression]: hub-local investigations are unaffected by the suppression set", func() {
		It("includes every local k8s tool in the RCA schema when no fleet overlay is present", func() {
			defs := inv.toolDefinitionsForPhase(context.Background(), katypes.PhaseRCA)

			names := toolNames(defs)
			Expect(names).To(ContainElement("kubectl_get_by_name"),
				"UT-KA-FLEET-030: a hub-local investigation (no overlay in ctx) must see the "+
					"full local tool set unchanged -- suppression must be a strict no-op absent a fleet overlay")
			Expect(names).To(ContainElement("kubectl_logs"))
			Expect(names).To(ContainElement("kubectl_top_pods"))
			Expect(names).To(ContainElement("get_namespaced_resource_context"))
		})
	})
})

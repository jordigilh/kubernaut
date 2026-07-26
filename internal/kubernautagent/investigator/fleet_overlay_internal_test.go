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
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
)

// fleetOverlayFakeTool is a minimal tools.Tool double for the fleet overlay
// unit tests below — no MCP session or gateway involved, this suite only
// proves the pure context-carrier and resolution logic (UT tier of the
// Pyramid Invariant; wiring proof lives in the IT-KA-FLEET-013/015 suites).
type fleetOverlayFakeTool struct{ name string }

func (f *fleetOverlayFakeTool) Name() string                { return f.name }
func (f *fleetOverlayFakeTool) Description() string         { return "fake " + f.name }
func (f *fleetOverlayFakeTool) Parameters() json.RawMessage { return nil }
func (f *fleetOverlayFakeTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	return "fake-result:" + f.name, nil
}

// BR-INTEGRATION-065, DD-FLEET-004: cluster-transparent tool exposure —
// pure logic tier (UT). Proves the overlay context carrier and the
// overlay-vs-registry resolution decision in isolation, with no registry,
// session, or gateway involved. Wiring into Investigate()/toolDefinitionsForPhase/
// executeTool is proven separately by IT-KA-FLEET-013/015.
var _ = Describe("Fleet overlay context carrier and resolution (BR-INTEGRATION-065, DD-FLEET-004)", Label("fleet", "unit"), func() {

	Describe("UT-KA-FLEET-014: fleet overlay context carrier round-trip", func() {
		It("returns the stored overlay and true when present", func() {
			overlay := map[string]tools.Tool{
				"kubectl_get_by_name": &fleetOverlayFakeTool{name: "kubectl_get_by_name"},
			}
			ctx := WithFleetOverlay(context.Background(), overlay)

			got, ok := FleetOverlayFromContext(ctx)
			Expect(ok).To(BeTrue(), "UT-KA-FLEET-014: overlay must be retrievable once stored")
			Expect(got).To(HaveLen(1))
			Expect(got["kubectl_get_by_name"].Name()).To(Equal("kubectl_get_by_name"))
		})

		It("returns not-found for a context with no overlay stored (hub-local investigations)", func() {
			got, ok := FleetOverlayFromContext(context.Background())
			Expect(ok).To(BeFalse(), "UT-KA-FLEET-014: absent context must report not-found, not a nil-but-ok overlay")
			Expect(got).To(BeNil())
		})

		It("returns not-found for an explicitly empty overlay (target cluster published zero tools)", func() {
			ctx := WithFleetOverlay(context.Background(), map[string]tools.Tool{})

			_, ok := FleetOverlayFromContext(ctx)
			Expect(ok).To(BeFalse(),
				"UT-KA-FLEET-014: an empty overlay must behave like no overlay, so callers fall back to the local registry")
		})
	})

	Describe("UT-KA-FLEET-018 [AC-6]: resolveTool overlay-vs-registry name resolution logic", func() {
		It("returns the overlay's tool and true when the name is present in the overlay", func() {
			bridgeStand := &fleetOverlayFakeTool{name: "kubectl_get_by_name"}
			overlay := map[string]tools.Tool{"kubectl_get_by_name": bridgeStand}

			got, found := resolveTool(overlay, "kubectl_get_by_name")
			Expect(found).To(BeTrue())
			Expect(got).To(BeIdenticalTo(bridgeStand))
		})

		It("returns not-found when the name is absent from the overlay (caller falls back to the local registry)", func() {
			overlay := map[string]tools.Tool{"kubectl_get_by_name": &fleetOverlayFakeTool{name: "kubectl_get_by_name"}}

			_, found := resolveTool(overlay, "kubectl_logs")
			Expect(found).To(BeFalse(),
				"UT-KA-FLEET-018: a name absent from the overlay must report not-found, not panic or a zero-value hit")
		})

		It("returns not-found for a nil overlay (hub-local investigations, zero behavior change)", func() {
			_, found := resolveTool(nil, "kubectl_get_by_name")
			Expect(found).To(BeFalse(), "UT-KA-FLEET-018: nil overlay must behave identically to an empty overlay")
		})
	})
})

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
	"errors"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
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

// BR-INTEGRATION-1489, DD-FLEET-004: cluster-transparent tool exposure —
// pure logic tier (UT). Proves the overlay context carrier and the
// overlay-vs-registry resolution decision in isolation, with no registry,
// session, or gateway involved. Wiring into Investigate()/toolDefinitionsForPhase/
// executeTool is proven separately by IT-KA-FLEET-013/015.
var _ = Describe("Fleet overlay context carrier and resolution (BR-INTEGRATION-1489, DD-FLEET-004)", Label("fleet", "unit"), func() {

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

// fleetOverlayRecordingAuditStore is a minimal audit.AuditStore double that
// records every event handed to it, for the observability tests below —
// no DataStorage/DS involved, this suite only proves prescopeFleetOverlay's
// own decision of *whether* to emit, not the audit transport.
type fleetOverlayRecordingAuditStore struct{ events []*audit.AuditEvent }

func (s *fleetOverlayRecordingAuditStore) StoreAudit(_ context.Context, event *audit.AuditEvent) error {
	s.events = append(s.events, event)
	return nil
}

// fleetOverlayErrResolver always fails Overlay, for the pre-existing
// "resolver configured but errored" branch — kept distinct from the "no
// resolver configured at all" branch under test below.
type fleetOverlayErrResolver struct{}

func (fleetOverlayErrResolver) Overlay(_ context.Context, _ string) (map[string]tools.Tool, error) {
	return nil, errors.New("gateway unreachable")
}

// fleetOverlaySuccessResolver always succeeds with a non-empty overlay.
type fleetOverlaySuccessResolver struct{}

func (fleetOverlaySuccessResolver) Overlay(_ context.Context, _ string) (map[string]tools.Tool, error) {
	return map[string]tools.Tool{"kubectl_get_by_name": &fleetOverlayFakeTool{name: "kubectl_get_by_name"}}, nil
}

// fleetOverlayEmptyResolver succeeds (no error) but resolves to zero tools,
// e.g. a target cluster that is fleet-registered but currently publishes no
// tools -- distinct from both fleetOverlayErrResolver (configured, errors)
// and a nil resolver (not configured at all).
type fleetOverlayEmptyResolver struct{}

func (fleetOverlayEmptyResolver) Overlay(_ context.Context, _ string) (map[string]tools.Tool, error) {
	return map[string]tools.Tool{}, nil
}

// Issue #1768 follow-up (Gap D E2E scoping discussion): prescopeFleetOverlay
// previously returned ctx completely unchanged -- no log, no audit event --
// whenever inv.fleetOverlayResolver was nil, for ANY clusterID (including a
// genuinely fleet-targeted one). That is indistinguishable, from the
// outside, from "prescopeFleetOverlay was never reached at all" (e.g. a
// regression removing the call, or RunInteractiveTurn/Investigate silently
// no-op'ing) -- there was no observable signal proving "a fleet-target
// investigation arrived at a KA instance with no fleet capability" versus
// "this investigation never carried a target cluster in the first place."
// This matters operationally (an SRE's fleet-targeted investigation
// silently ran against local/hub tools with zero trace) and for E2E
// testability (no way to assert the pre-scoping call site was reached
// without a real, wired FleetOverlayResolver).
var _ = Describe("UT-KA-FLEET-028 [AU-3, GA Readiness Dim. 12]: prescopeFleetOverlay observability for an unconfigured fleet", Label("fleet", "unit"), func() {

	It("emits EventTypeFleetOverlayUnavailable when a fleet-target investigation hits a nil resolver", func() {
		store := &fleetOverlayRecordingAuditStore{}
		inv := &Investigator{
			logger:               logr.Discard(),
			auditStore:           store,
			fleetOverlayResolver: nil,
		}

		got := inv.prescopeFleetOverlay(context.Background(), "remote-cluster", "corr-unavailable-1")

		_, ok := FleetOverlayFromContext(got)
		Expect(ok).To(BeFalse(), "UT-KA-FLEET-028: ctx must carry no overlay when fleet isn't configured")

		Expect(store.events).To(HaveLen(1),
			"UT-KA-FLEET-028: a fleet-target investigation hitting a nil resolver must be independently "+
				"observable, not silently indistinguishable from prescopeFleetOverlay never having been called")
		evt := store.events[0]
		Expect(evt.EventType).To(Equal(audit.EventTypeFleetOverlayUnavailable))
		Expect(evt.EventAction).To(Equal(audit.ActionFleetOverlayUnavailable))
		Expect(evt.EventOutcome).To(Equal(audit.OutcomeFailure))
		Expect(evt.ClusterID).To(Equal("remote-cluster"))
		Expect(evt.CorrelationID).To(Equal("corr-unavailable-1"))
		Expect(evt.Data["cluster_id"]).To(Equal("remote-cluster"))
	})

	It("emits nothing for a hub-local investigation (clusterID empty) even with a nil resolver", func() {
		store := &fleetOverlayRecordingAuditStore{}
		inv := &Investigator{
			logger:               logr.Discard(),
			auditStore:           store,
			fleetOverlayResolver: nil,
		}

		_ = inv.prescopeFleetOverlay(context.Background(), "", "corr-hub-1")

		Expect(store.events).To(BeEmpty(),
			"UT-KA-FLEET-028: a hub-local investigation (no target cluster) must stay silent -- "+
				"this is the expected, unchanged zero-regression path, not a degraded one")
	})

	It("emits the pre-existing EventTypeFleetOverlayFailed (not Unavailable) when a real resolver errors", func() {
		store := &fleetOverlayRecordingAuditStore{}
		inv := &Investigator{
			logger:               logr.Discard(),
			auditStore:           store,
			fleetOverlayResolver: fleetOverlayErrResolver{},
		}

		_ = inv.prescopeFleetOverlay(context.Background(), "remote-cluster", "corr-err-1")

		Expect(store.events).To(HaveLen(1))
		Expect(store.events[0].EventType).To(Equal(audit.EventTypeFleetOverlayFailed),
			"UT-KA-FLEET-028: a configured-but-erroring resolver is a distinct condition from "+
				"'not configured at all' and must keep using the pre-existing event type")
	})

	It("emits nothing when a real resolver succeeds", func() {
		store := &fleetOverlayRecordingAuditStore{}
		inv := &Investigator{
			logger:               logr.Discard(),
			auditStore:           store,
			fleetOverlayResolver: fleetOverlaySuccessResolver{},
		}

		got := inv.prescopeFleetOverlay(context.Background(), "remote-cluster", "corr-ok-1")

		Expect(store.events).To(BeEmpty(), "UT-KA-FLEET-028: a successful resolution needs no degradation event")
		_, ok := FleetOverlayFromContext(got)
		Expect(ok).To(BeTrue(), "a successful resolver must still populate the overlay as before")
		clusterID, ok := audit.ClusterIDFromContext(got)
		Expect(ok).To(BeTrue(),
			"UT-KA-FLEET-028: a successful resolution must also attach audit.WithClusterID to the "+
				"returned context, so every audit event downstream of this call (e.g. "+
				"alignment.SubmitToolStep's attributionClusterID) carries correct cluster attribution "+
				"even for callers that invoke Investigate()/RunInteractiveTurn directly, without going "+
				"through session.Manager's own attachInvestigationContext")
		Expect(clusterID).To(Equal("remote-cluster"))
	})

	// QE readiness audit follow-up (PR #1799 Finding #2, tracked as #1834): characterizes a
	// currently-untested edge case -- a resolver that IS configured, does
	// NOT error, but resolves to an empty overlay (e.g. the target cluster
	// is fleet-registered but currently publishes zero tools). This is
	// deliberately a documentation/characterization test of the EXISTING
	// behavior, not a behavior change: FleetOverlayFromContext already
	// treats an empty overlay as "no overlay" (see its own len(overlay)==0
	// check and UT-KA-FLEET-014's "explicitly empty overlay" case), so this
	// currently falls through the same path as a hub-local investigation --
	// no log, no audit event. That is the same class of blind spot the nil-
	// resolver case above used to have (a fleet-target investigation ending
	// up with zero remote-cluster tools, indistinguishable from "there was
	// never a target cluster") and is tracked as a follow-up decision, not
	// fixed here.
	It("emits nothing when a real resolver succeeds but returns an empty overlay (tracked follow-up gap, not fixed here)", func() {
		store := &fleetOverlayRecordingAuditStore{}
		inv := &Investigator{
			logger:               logr.Discard(),
			auditStore:           store,
			fleetOverlayResolver: fleetOverlayEmptyResolver{},
		}

		got := inv.prescopeFleetOverlay(context.Background(), "remote-cluster", "corr-empty-1")

		Expect(store.events).To(BeEmpty(),
			"UT-KA-FLEET-028: current behavior -- a successful-but-empty resolution is indistinguishable "+
				"from a hub-local investigation, same as EventTypeFleetOverlayUnavailable's nil-resolver "+
				"case used to be before issue #1768's follow-up. Tracked as a known follow-up gap, not "+
				"fixed by this test.")
		_, ok := FleetOverlayFromContext(got)
		Expect(ok).To(BeFalse(),
			"an empty overlay must behave like no overlay at all, so callers fall back to the local registry")
	})
})

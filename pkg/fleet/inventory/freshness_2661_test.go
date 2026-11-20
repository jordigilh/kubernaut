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

package inventory_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// DD-FLEET-017 D3 / G3: freshness is part of the page contract, and only a
// proof-backed value is allowed. `unknown` is deliberately absent in v1.6
// (D3.5), so an empty or unrecognised state must be rejected rather than
// serialized as a zero value.
var _ = Describe("UT-FMC-INV-2661-007 [BR-API-001]: page freshness contract", func() {
	var asOf time.Time

	BeforeEach(func() {
		asOf = time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	})

	managedResource := func() inventory.Resource {
		identity := scope.ResourceIdentity{
			ClusterID: "prod-east",
			Group:     "apps",
			Version:   "v1",
			Kind:      "Deployment",
			Namespace: "payments",
			Name:      "checkout",
		}
		return inventory.Resource{
			ID:        inventory.CanonicalResourceID(identity),
			ClusterID: identity.ClusterID,
			Group:     identity.Group,
			Version:   identity.Version,
			Kind:      identity.Kind,
			Namespace: identity.Namespace,
			Name:      identity.Name,
			Scope:     inventory.Scope{Included: true, ResourceLabel: inventory.LabelTrue, Source: inventory.ScopeSourceResourceLabel},
		}
	}

	DescribeTable("rejects a page that claims inventory without proof-backed freshness",
		func(page inventory.Page, expected string) {
			Expect(page.Validate()).To(MatchError(ContainSubstring(expected)))
		},
		Entry("success page without a freshness state",
			inventory.Page{
				State:        inventory.StateSuccess,
				Completeness: inventory.CompletenessComplete,
				Freshness:    inventory.Freshness{AsOf: time.Now().UTC()},
				Items:        []inventory.Resource{managedResource()},
			},
			"page freshness state is required",
		),
		Entry("success page with a zero as_of",
			inventory.Page{
				State:        inventory.StateSuccess,
				Completeness: inventory.CompletenessComplete,
				Freshness:    inventory.Freshness{State: inventory.FreshnessFresh},
				Items:        []inventory.Resource{managedResource()},
			},
			"page freshness as_of is required",
		),
		Entry("empty page without a freshness state",
			inventory.Page{State: inventory.StateEmpty, Completeness: inventory.CompletenessComplete},
			"page freshness state is required",
		),
		Entry("empty page with a zero as_of",
			inventory.Page{
				State:        inventory.StateEmpty,
				Completeness: inventory.CompletenessComplete,
				Freshness:    inventory.Freshness{State: inventory.FreshnessStale},
			},
			"page freshness as_of is required",
		),
		Entry("success page with the absent unknown state",
			inventory.Page{
				State:        inventory.StateSuccess,
				Completeness: inventory.CompletenessComplete,
				Freshness:    inventory.Freshness{State: inventory.FreshnessState("unknown"), AsOf: time.Now().UTC()},
				Items:        []inventory.Resource{managedResource()},
			},
			`unsupported page freshness state "unknown"`,
		),
		Entry("empty page with an unrecognised state",
			inventory.Page{
				State:        inventory.StateEmpty,
				Completeness: inventory.CompletenessComplete,
				Freshness:    inventory.Freshness{State: inventory.FreshnessState("fresh-ish"), AsOf: time.Now().UTC()},
			},
			`unsupported page freshness state "fresh-ish"`,
		),
	)

	It("accepts fresh and stale pages that carry the projection instant", func() {
		page := inventory.Page{
			State:        inventory.StateSuccess,
			Completeness: inventory.CompletenessComplete,
			Freshness:    inventory.Freshness{State: inventory.FreshnessFresh, AsOf: asOf},
			Items:        []inventory.Resource{managedResource()},
		}
		Expect(page.Validate()).To(Succeed())

		page.Freshness.State = inventory.FreshnessStale
		Expect(page.Validate()).To(Succeed(), "a proven but ageing generation is still a valid page")

		empty := inventory.Page{
			State:        inventory.StateEmpty,
			Completeness: inventory.CompletenessComplete,
			Freshness:    inventory.Freshness{State: inventory.FreshnessStale, AsOf: asOf},
		}
		Expect(empty.Validate()).To(Succeed())
	})

	It("validates an internal collection result without a freshness claim", func() {
		// DD-FLEET-017 G10/D3.6: a collection result is not a read surface. It
		// has no partition proof, so it must not claim freshness -- and the
		// collection check must still enforce every shape invariant.
		collection := inventory.Page{
			State:        inventory.StateSuccess,
			Completeness: inventory.CompletenessComplete,
			Items:        []inventory.Resource{managedResource()},
		}
		Expect(collection.ValidateCollection()).To(Succeed())
		Expect(collection.Freshness.State).To(BeEmpty())
		Expect(collection.Validate()).To(MatchError(ContainSubstring("page freshness state is required")),
			"the read-surface validator must never accept a collection shape")

		truncated := collection
		truncated.Completeness = inventory.CompletenessPartial
		Expect(truncated.ValidateCollection()).To(MatchError(ContainSubstring("successful page is incomplete")),
			"ValidateCollection is not a weaker check: shape invariants still apply")

		badCursor := collection
		badCursor.NextCursor = "opaque-id"
		Expect(badCursor.ValidateCollection()).To(MatchError(ContainSubstring("next_cursor is not a valid resource ID")))

		emptyCollection := inventory.Page{State: inventory.StateEmpty, Completeness: inventory.CompletenessComplete}
		Expect(emptyCollection.ValidateCollection()).To(Succeed())
		Expect(inventory.StateOnly(inventory.StateUnavailable).ValidateCollection()).To(Succeed())
	})

	It("leaves state-only failures without a freshness claim", func() {
		// A failure envelope proves nothing about the fleet, so it must not be
		// forced to invent freshness.
		for _, state := range []inventory.State{
			inventory.StatePartial, inventory.StateUnavailable, inventory.StateForbidden, inventory.StateNotFound, inventory.StateStale,
		} {
			Expect(inventory.StateOnly(state).Validate()).To(Succeed(), string(state))
		}
	})
})

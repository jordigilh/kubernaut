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

package acm

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
)

// UT-ACM-2593-COLLECTION-003 [BR-SAFETY-001]: DD-FLEET-017 G10.
// A successfully traversed zero-item Search query is an applied observation, so
// the collection result is a complete empty shape -- never the state-only
// failure envelope. Whether the fleet is empty is FMC page assembly's decision.
var _ = Describe("UT-ACM-2593-COLLECTION-003: zero-item collection result", func() {
	It("returns a complete empty observation, not a failure envelope", func() {
		page, err := pageFromSearchResponse(graphQLResponse{
			Data: &searchData{SearchResult: []searchResultItem{{Count: 0}}},
		}, "prod-east")
		Expect(err).ToNot(HaveOccurred())

		Expect(page.State).To(Equal(inventory.StateEmpty))
		Expect(page.Completeness).To(Equal(inventory.CompletenessComplete),
			"a completed traversal that matched nothing is complete, not partial")
		Expect(page.Items).To(BeEmpty())
		Expect(page.NextCursor).To(BeEmpty())
		Expect(page.ObservedAt).ToNot(BeZero())
		Expect(page.Freshness.State).To(BeEmpty(),
			"the collection adapter makes no freshness claim")
		Expect(page.ValidateCollection()).To(Succeed())
		Expect(page.Validate()).To(MatchError(ContainSubstring("page freshness state is required")),
			"the collection shape is not a read-surface page")
	})
})

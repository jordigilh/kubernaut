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

package acm_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/acm"
	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
)

// DD-FLEET-017 G10 / D3.6: pkg/fleet/acm is a collection adapter, not a read
// surface. ACM Search exposes no index-freshness signal, so a collection result
// must carry no freshness envelope at all -- neither `fresh` nor `unknown`.
// The page layer (FMC page assembly) derives freshness from the partition
// proof; borrowing that claim here would publish proof this adapter cannot have.
var _ = Describe("UT-ACM-2593-COLLECTION-002 [BR-API-001]: collection results claim no freshness", func() {
	It("returns an applied collection result without a freshness envelope", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, readErr := io.ReadAll(r.Body)
			Expect(readErr).ToNot(HaveOccurred())
			if respondNamespaceFixture(w, body) {
				return
			}
			_, writeErr := io.WriteString(w, `{"data":{"searchResult":[{"count":1,"items":[{"cluster":"prod-east","apiversion":"v1","kind":"ConfigMap","namespace":"payments","name":"checkout","label":"kubernaut.ai/managed=true","_uid":"prod-east/12345"}]}]}}`)
			Expect(writeErr).ToNot(HaveOccurred())
		}))
		DeferCleanup(server.Close)

		page, err := acm.NewClient(server.URL).List(context.Background(), inventory.Filters{ClusterID: "prod-east"})
		Expect(err).ToNot(HaveOccurred())
		Expect(page.State).To(Equal(inventory.StateSuccess))
		Expect(page.Items).To(HaveLen(1))

		Expect(page.Freshness.State).To(BeEmpty(),
			"the collection result must not claim a freshness state it cannot prove")
		Expect(page.Freshness.AsOf).To(BeZero(),
			"as_of is the projection instant of a read surface, not of a collection result")
		Expect(page.Validate()).To(MatchError(ContainSubstring("page freshness state is required")),
			"a collection result is not a read-surface page")
		Expect(page.ValidateCollection()).To(Succeed())
	})
})

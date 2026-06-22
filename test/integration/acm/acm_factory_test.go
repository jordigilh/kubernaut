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
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// localAlwaysFalse isolates the remote (ACM) path under test by ensuring the
// local scope checker never claims a resource as managed.
type localAlwaysFalse struct{}

func (l *localAlwaysFalse) IsManagedResource(_ context.Context, _ scope.ResourceIdentity) (bool, error) {
	return false, nil
}

// IT-ACM-054-001
//
// Pyramid Invariant: IT proves wiring.
// This test exercises the production factory dispatch path:
//
//	fleet.NewScopeChecker (factory, BackendACM)
//	  -> FederatedScopeChecker (local/remote router)
//	    -> acm.Client (production GraphQL client)
//	      -> real HTTP transport
//	        -> fake ACM Search GraphQL server (httptest)
//
// The fake GraphQL server returns controlled responses so the test can verify
// that the factory correctly wires acm.Client and that the full dispatch path
// produces the correct scope decision.
//
// Wiring Manifest:
//
//	acm.Client               -> pkg/fleet/acm/client.go        -> IT-ACM-054-001
//	Factory BackendACM       -> pkg/fleet/scope_factory.go      -> IT-ACM-054-001
//	FederatedScopeChecker    -> pkg/fleet/federated_checker.go  -> IT-ACM-054-001
var _ = Describe("ACM Search Factory Wiring (BR-INTEGRATION-065)", Ordered, Label("acm", "integration"), func() {
	var (
		graphQLServer *httptest.Server
		fedChecker    scope.ScopeChecker
		nextCount     int
	)

	BeforeAll(func() {
		By("Starting fake ACM Search GraphQL server")
		graphQLServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			Expect(r.Method).To(Equal(http.MethodPost), "ACM Search uses POST for GraphQL")
			Expect(r.URL.Path).To(Equal("/searchapi/graphql"), "endpoint path must match ACM convention")

			w.Header().Set("Content-Type", "application/json")
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"searchResult": []map[string]interface{}{
						{"count": nextCount},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))

		By("Creating FederatedScopeChecker via production factory with BackendACM")
		cfg := fleet.FleetConfig{
			Enabled:  true,
			Backend:  fleet.BackendACM,
			Endpoint: graphQLServer.URL,
		}
		var err error
		fedChecker, err = fleet.NewScopeChecker(&localAlwaysFalse{}, cfg, logr.Discard())
		Expect(err).ToNot(HaveOccurred(),
			"fleet.NewScopeChecker with BackendACM must succeed — factory must wire acm.Client")
	})

	AfterAll(func() {
		if graphQLServer != nil {
			graphQLServer.Close()
		}
	})

	// AC-4 Information Flow Enforcement: proves that the production factory
	// correctly dispatches through acm.Client to the ACM Search backend, and
	// the positive scope decision flows back through FederatedScopeChecker.
	Describe("IT-ACM-054-001 [AC-4]: factory BackendACM dispatches through acm.Client", func() {
		It("Sub-case A: managed resource (count=1) returns managed=true through full factory path", func() {
			nextCount = 1

			managed, err := fedChecker.IsManagedResource(context.Background(), scope.ResourceIdentity{
				ClusterID: "prod-east",
				Group:     "apps",
				Version:   "v1",
				Kind:      "Deployment",
				Namespace: "default",
				Name:      "nginx",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue(),
				"AC-4: managed resource must return true through factory -> "+
					"FederatedScopeChecker -> acm.Client -> GraphQL server path")
		})

		It("Sub-case B: unmanaged resource (count=0) returns managed=false through full factory path", func() {
			nextCount = 0

			managed, err := fedChecker.IsManagedResource(context.Background(), scope.ResourceIdentity{
				ClusterID: "prod-east",
				Group:     "apps",
				Version:   "v1",
				Kind:      "Deployment",
				Namespace: "orphan-ns",
				Name:      "no-such-deploy",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeFalse(),
				"AC-4: absent resource must return false through the full production path")
		})
	})
})

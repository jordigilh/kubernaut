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
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"

	"github.com/jordigilh/kubernaut/pkg/fleet/acm"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

var _ = Describe("ACM Search GraphQL Client (BR-INTEGRATION-065, ADR-068)", func() {
	var (
		server *httptest.Server
		client *acm.Client
	)

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	// AC-4 Information Flow Enforcement: the scope check API is the enforcement
	// point for multi-cluster resource governance. When ACM Search reports that a
	// resource exists on a managed cluster (count > 0), the adapter MUST return
	// managed=true so GW/RO act on it.
	It("UT-ACM-054-001 [AC-4]: returns managed=true when ACM Search reports count > 0", func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"searchResult":[{"count":1}]}}`))
		}))
		client = acm.NewClient(server.URL)

		managed, err := client.IsManagedResource(context.Background(), scope.ResourceIdentity{
			ClusterID: "prod-east", Group: "apps", Version: "v1",
			Kind: "Deployment", Namespace: "default", Name: "nginx",
		})

		Expect(err).ToNot(HaveOccurred())
		Expect(managed).To(BeTrue(),
			"AC-4: resource found by ACM Search (count=1) must be reported as managed "+
				"so the gateway enforces the correct information flow decision")
	})

	// AC-4: When ACM Search reports count=0, the resource is not on the managed
	// cluster. The adapter MUST return managed=false so GW/RO skip it.
	It("UT-ACM-054-002 [AC-4]: returns managed=false when ACM Search reports count == 0", func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"searchResult":[{"count":0}]}}`))
		}))
		client = acm.NewClient(server.URL)

		managed, err := client.IsManagedResource(context.Background(), scope.ResourceIdentity{
			ClusterID: "prod-east", Group: "apps", Version: "v1",
			Kind: "Deployment", Namespace: "default", Name: "nonexistent",
		})

		Expect(err).ToNot(HaveOccurred())
		Expect(managed).To(BeFalse(),
			"AC-4: resource absent from ACM Search (count=0) must be reported as unmanaged "+
				"so the gateway does not act on resources outside its scope")
	})

	// SC-7 Boundary Protection: under failure, the managed-resource boundary must
	// be conservative. Every error condition must fall back to "unmanaged" (false)
	// with nil error, so the caller never propagates a transient failure as a
	// positive scope decision.
	DescribeTable("UT-ACM-054-003..005 [SC-7]: fail-safe returns managed=false — boundary remains conservative under failure",
		func(setupServer func() string) {
			client = acm.NewClient(setupServer())

			managed, err := client.IsManagedResource(context.Background(), scope.ResourceIdentity{
				ClusterID: "prod-east", Kind: "Deployment", Name: "nginx",
			})

			Expect(err).ToNot(HaveOccurred(),
				"SC-7: errors must be absorbed (fail-safe), not propagated — "+
					"the boundary must remain conservative under infrastructure failure")
			Expect(managed).To(BeFalse(),
				"SC-7: boundary protection requires unmanaged default when the "+
					"backend is unreachable or returns an error")
		},
		Entry("connection refused — network failure", func() string {
			return "http://127.0.0.1:1"
		}),
		Entry("HTTP 500 — server error", func() string {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}))
			return server.URL
		}),
		Entry("malformed JSON body — decode failure", func() string {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`not-json`))
			}))
			return server.URL
		}),
	)

	// SC-7: GraphQL-level errors (the API returns 200 but the errors array is
	// non-empty) must also trigger fail-safe. This protects against authorization
	// failures, malformed queries, or backend-internal errors being interpreted
	// as "not managed" vs "error checking".
	It("UT-ACM-054-006 [SC-7]: GraphQL-level error triggers fail-safe — boundary conservative on API error", func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"errors":[{"message":"unauthorized"}],"data":{"searchResult":null}}`))
		}))
		client = acm.NewClient(server.URL)

		managed, err := client.IsManagedResource(context.Background(), scope.ResourceIdentity{
			ClusterID: "prod-east", Kind: "Deployment", Name: "nginx",
		})

		Expect(err).ToNot(HaveOccurred(),
			"SC-7: GraphQL errors must be absorbed, not propagated")
		Expect(managed).To(BeFalse(),
			"SC-7: GraphQL errors must default to unmanaged — the boundary "+
				"cannot widen due to an authorization or query failure")
	})

	// SI-10 Information Input Validation: the adapter must correctly translate
	// ResourceIdentity fields into ACM Search GraphQL filter properties. An
	// incorrect mapping would cause silent false negatives (resource appears
	// unmanaged when it is managed) or false positives.
	It("UT-ACM-054-007 [SI-10]: request body maps ResourceIdentity to correct GraphQL filters", func() {
		var receivedBody []byte
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedBody, _ = io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"searchResult":[{"count":0}]}}`))
		}))
		client = acm.NewClient(server.URL)

		_, _ = client.IsManagedResource(context.Background(), scope.ResourceIdentity{
			ClusterID: "prod-east",
			Group:     "apps",
			Version:   "v1",
			Kind:      "Deployment",
			Namespace: "kube-system",
			Name:      "coredns",
		})

		Expect(receivedBody).ToNot(BeEmpty(), "request must have a body")

		var req map[string]interface{}
		Expect(json.Unmarshal(receivedBody, &req)).To(Succeed())

		Expect(req).To(HaveKey("query"),
			"SI-10: request must contain a GraphQL query string")
		Expect(req).To(HaveKey("variables"),
			"SI-10: request must contain variables with search filters")

		vars, ok := req["variables"].(map[string]interface{})
		Expect(ok).To(BeTrue())
		inputs, ok := vars["input"].([]interface{})
		Expect(ok).To(BeTrue())
		Expect(inputs).To(HaveLen(1))

		input, ok := inputs[0].(map[string]interface{})
		Expect(ok).To(BeTrue())
		filters, ok := input["filters"].([]interface{})
		Expect(ok).To(BeTrue())

		filterMap := make(map[string]string)
		for _, f := range filters {
			fm := f.(map[string]interface{})
			prop := fm["property"].(string)
			vals := fm["values"].([]interface{})
			if len(vals) > 0 {
				filterMap[prop] = vals[0].(string)
			}
		}

		Expect(filterMap).To(HaveKeyWithValue("kind", "Deployment"),
			"SI-10: Kind must map to 'kind' filter property")
		Expect(filterMap).To(HaveKeyWithValue("name", "coredns"),
			"SI-10: Name must map to 'name' filter property")
		Expect(filterMap).To(HaveKeyWithValue("namespace", "kube-system"),
			"SI-10: Namespace must map to 'namespace' filter property")
		Expect(filterMap).To(HaveKeyWithValue("cluster", "prod-east"),
			"SI-10: ClusterID must map to 'cluster' filter property")
	})

	It("UT-ACM-054-008: Client satisfies scope.ScopeChecker interface", func() {
		var checker scope.ScopeChecker = acm.NewClient("https://search-api.example.com:4010")
		Expect(checker).ToNot(BeNil(),
			"acm.Client must implement scope.ScopeChecker to be usable by the factory")
	})

	Describe("Ping [readiness gate Wave 0]", func() {
		It("UT-ACM-054-009: succeeds when ACM Search responds 200 with no GraphQL errors", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"data":{"searchResult":[{"count":0}]}}`))
			}))
			client = acm.NewClient(server.URL)

			Expect(client.Ping(context.Background())).To(Succeed())
		})

		It("UT-ACM-054-010: returns an error (does not swallow it) when the endpoint is unreachable", func() {
			client = acm.NewClient("http://127.0.0.1:1")
			err := client.Ping(context.Background())
			Expect(err).To(HaveOccurred(),
				"unlike IsManagedResource, Ping must surface the transport error for the readiness gate")
		})

		It("UT-ACM-054-011: returns an error when ACM Search responds with a non-200 status", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			client = acm.NewClient(server.URL)

			err := client.Ping(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("503"))
		})

		It("UT-ACM-054-012: returns an error when ACM Search responds with GraphQL errors", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"errors":[{"message":"unauthorized"}]}`))
			}))
			client = acm.NewClient(server.URL)

			err := client.Ping(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unauthorized"))
		})

		// #1556 spike (2026-07-07, live ACM 2.16.2 on OCP): a real ACM Search
		// backend rejects an empty/nil filter set with a GraphQL business error
		// ("query input must contain a filter or keyword") — HTTP 200, but a
		// non-empty errors array — REGARDLESS of authentication success. Ping's
		// query must therefore carry a filter that is always syntactically valid
		// and satisfiable on every real ACM hub (kind=Namespace is always
		// indexed), or the readiness gate would permanently report the ACM
		// backend as NotReady even when it is perfectly healthy.
		It("UT-ACM-054-013 [SC-7]: Ping sends a non-empty filter, not an empty/nil filter set real ACM Search rejects", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				var req map[string]interface{}
				_ = json.Unmarshal(body, &req)
				vars, _ := req["variables"].(map[string]interface{})
				inputs, _ := vars["input"].([]interface{})

				hasFilter := false
				if len(inputs) > 0 {
					if input, ok := inputs[0].(map[string]interface{}); ok {
						if filters, ok := input["filters"].([]interface{}); ok && len(filters) > 0 {
							hasFilter = true
						}
					}
				}

				w.Header().Set("Content-Type", "application/json")
				if !hasFilter {
					// Mirrors real ACM Search v2.16.2's rejection of empty/nil
					// filter sets, confirmed against a live cluster.
					_, _ = w.Write([]byte(`{"errors":[{"message":"query input must contain a filter or keyword"}],"data":{"searchResult":[{"count":null}]}}`))
					return
				}
				_, _ = w.Write([]byte(`{"data":{"searchResult":[{"count":0}]}}`))
			}))
			client = acm.NewClient(server.URL)

			err := client.Ping(context.Background())
			Expect(err).ToNot(HaveOccurred(),
				"SC-7: Ping must send a syntactically valid, always-satisfiable filter "+
					"so a healthy, fully-authenticated ACM Search backend is correctly "+
					"reported as ready, not rejected for an invalid query shape")
		})
	})

	Describe("UT-ACM-TLS [SC-8]: TLS transport security", func() {
		It("UT-ACM-TLS-001 [SC-8]: should connect to TLS server when CA cert is provided via WithHTTPClient", func() {
			server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"data":{"searchResult":[{"count":1}]}}`))
			}))

			client = acm.NewClient(server.URL, acm.WithHTTPClient(server.Client()))

			managed, err := client.IsManagedResource(context.Background(), scope.ResourceIdentity{
				ClusterID: "prod-east", Kind: "Deployment", Name: "nginx",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue(),
				"SC-8: TLS connection with valid CA must succeed")
		})

		It("UT-ACM-TLS-002 [SC-8]: should fail-safe when TLS server cert is untrusted (no InsecureSkipVerify)", func() {
			server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"data":{"searchResult":[{"count":1}]}}`))
			}))

			client = acm.NewClient(server.URL)

			managed, err := client.IsManagedResource(context.Background(), scope.ResourceIdentity{
				ClusterID: "prod-east", Kind: "Deployment", Name: "nginx",
			})
			Expect(err).ToNot(HaveOccurred(),
				"SC-8: TLS failure must be absorbed (fail-safe per SC-7)")
			Expect(managed).To(BeFalse(),
				"SC-8: untrusted TLS cert must not allow managed=true — prevents MITM")
		})

		It("UT-ACM-TLS-003 [SC-8]: WithHTTPClient overrides default transport", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"data":{"searchResult":[{"count":1}]}}`))
			}))

			customClient := &http.Client{Timeout: 5 * time.Second}
			client = acm.NewClient(server.URL, acm.WithHTTPClient(customClient))

			managed, err := client.IsManagedResource(context.Background(), scope.ResourceIdentity{
				ClusterID: "prod-east", Kind: "Deployment", Name: "nginx",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue(),
				"SC-8: WithHTTPClient option must be applied correctly")
		})
	})

	// Contract test: validates the adapter's GraphQL query against the
	// upstream ACM Search SDL schema (vendored from stolostron/search-v2-api
	// release-2.13, OCP 4.18 floor). If the upstream schema ever changes in
	// a way that invalidates our query (renamed types, removed fields, changed
	// signatures), this test fails immediately.
	//
	// Coverage: input types (SearchInput, SearchFilter), query signature
	// (search), and response field selections (SearchResult.count).
	It("UT-ACM-054-009 [AC-4,SC-7]: adapter query validates against ACM Search SDL schema (contract test)", func() {
		schemaSDL, err := os.ReadFile("testdata/acm-search-schema.graphqls")
		Expect(err).ToNot(HaveOccurred(), "vendored schema must be readable")

		schema, parseErr := gqlparser.LoadSchema(&ast.Source{
			Name:  "acm-search-schema.graphqls",
			Input: string(schemaSDL),
		})
		Expect(parseErr).ToNot(HaveOccurred(), "vendored schema must parse without errors")

		_, queryErrs := gqlparser.LoadQuery(schema, acm.SearchQuery)
		Expect(queryErrs).To(BeEmpty(),
			"AC-4/SC-7: adapter SearchQuery must be valid against the ACM Search SDL schema — "+
				"if this fails, the upstream schema has drifted and the adapter's GraphQL "+
				"contract (types, query signature, or response fields) is broken")
	})
})

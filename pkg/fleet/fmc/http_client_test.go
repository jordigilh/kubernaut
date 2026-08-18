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

package fmc_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// flakyRoundTripper fails the first failCount requests with a transport-level
// error (simulating a dropped connection / dial failure), then delegates to
// the real transport. Used to test IsManagedResource's retry-on-transport-error
// path deterministically, without relying on real network flakiness.
type flakyRoundTripper struct {
	failCount int32
	attempts  atomic.Int32
	delegate  http.RoundTripper
}

func (f *flakyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	n := f.attempts.Add(1)
	if n <= f.failCount {
		return nil, fmt.Errorf("simulated transport error (attempt %d)", n)
	}
	return f.delegate.RoundTrip(req)
}

var _ = Describe("FMC HTTP Client (BR-INTEGRATION-065, ADR-068)", func() {
	var (
		server *httptest.Server
		client *fmc.HTTPClient
	)

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	It("UT-FMC-HC-001 [AC-4]: returns managed=true when FMC responds with managed=true", func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"managed":true}`))
		}))
		client = fmc.NewHTTPClient(server.URL)

		managed, err := client.IsManagedResource(context.Background(), scope.ResourceIdentity{
			ClusterID: "prod-east", Group: "apps", Version: "v1",
			Kind: "Deployment", Namespace: "default", Name: "nginx",
		})

		Expect(err).ToNot(HaveOccurred())
		Expect(managed).To(BeTrue())
	})

	It("UT-FMC-HC-002 [AC-4]: returns managed=false when FMC responds with managed=false", func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"managed":false}`))
		}))
		client = fmc.NewHTTPClient(server.URL)

		managed, err := client.IsManagedResource(context.Background(), scope.ResourceIdentity{
			ClusterID: "prod-east", Group: "apps", Version: "v1",
			Kind: "Deployment", Namespace: "default", Name: "missing",
		})

		Expect(err).ToNot(HaveOccurred())
		Expect(managed).To(BeFalse())
	})

	DescribeTable("UT-FMC-HC-003..005 [SC-7]: fail-safe returns managed=false on backend errors",
		func(setupServer func() string) {
			client = fmc.NewHTTPClient(setupServer())

			managed, err := client.IsManagedResource(context.Background(), scope.ResourceIdentity{
				ClusterID: "prod-east", Kind: "Deployment", Name: "nginx",
			})

			Expect(err).ToNot(HaveOccurred(),
				"errors must be absorbed (fail-safe), not propagated")
			Expect(managed).To(BeFalse())
		},
		Entry("connection refused", func() string {
			return "http://127.0.0.1:1"
		}),
		Entry("HTTP 500", func() string {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}))
			return server.URL
		}),
		Entry("malformed JSON body", func() string {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`not-json`))
			}))
			return server.URL
		}),
	)

	It("UT-FMC-HC-006 [SI-10]: query parameters are URL-encoded correctly", func() {
		var receivedQuery string
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedQuery = r.URL.RawQuery
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"managed":false}`))
		}))
		client = fmc.NewHTTPClient(server.URL)

		_, _ = client.IsManagedResource(context.Background(), scope.ResourceIdentity{
			ClusterID: "cluster with spaces",
			Kind:      "Deployment",
			Name:      "name/with/slashes",
			Namespace: "ns",
		})

		Expect(receivedQuery).To(ContainSubstring("cluster=cluster+with+spaces"))
		Expect(receivedQuery).To(ContainSubstring("name=name%2Fwith%2Fslashes"))
	})

	It("UT-FMC-HC-007: HTTPClient satisfies scope.ScopeChecker interface", func() {
		var checker scope.ScopeChecker = fmc.NewHTTPClient("http://localhost:8080")
		Expect(checker).ToNot(BeNil())
	})

	Describe("Ping [readiness gate Wave 0, DD-PLATFORM-010]", func() {
		It("UT-FMC-HC-008: succeeds when /readyz responds 200", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Path).To(Equal(fmc.ReadyzPath),
					"DD-PLATFORM-010: Ping must target the unauthenticated ReadyzPath, not ClustersPath")
				w.WriteHeader(http.StatusOK)
			}))
			client = fmc.NewHTTPClient(server.URL)

			Expect(client.Ping(context.Background())).To(Succeed())
		})

		It("UT-FMC-HC-009: returns an error (does not swallow it) when the endpoint is unreachable", func() {
			client = fmc.NewHTTPClient("http://127.0.0.1:1")
			err := client.Ping(context.Background())
			Expect(err).To(HaveOccurred(),
				"unlike IsManagedResource, Ping must surface the transport error for the readiness gate")
		})

		It("UT-FMC-HC-010: returns an error when /readyz responds with a non-200 status", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			client = fmc.NewHTTPClient(server.URL)

			err := client.Ping(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("503"))
		})
	})

	Describe("WithHTTPClient [#1683, SC-8]", func() {
		// Ping (unlike IsManagedResource) surfaces transport errors directly,
		// so it's the clearest way to prove the *injected* client -- not the
		// package's own 5s-timeout default -- is actually what governs the
		// request: a slow server combined with a much shorter injected
		// timeout must fail fast, well before the default would have.
		It("UT-FMC-HC-011: governs request behavior via the injected client's own timeout, not the 5s default", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				time.Sleep(200 * time.Millisecond)
				w.WriteHeader(http.StatusOK)
			}))

			customClient := &http.Client{Timeout: 10 * time.Millisecond}
			client = fmc.NewHTTPClient(server.URL, fmc.WithHTTPClient(customClient))

			start := time.Now()
			err := client.Ping(context.Background())
			elapsed := time.Since(start)

			Expect(err).To(HaveOccurred(),
				"the injected 10ms timeout must fire, not the package's 5s default")
			Expect(elapsed).To(BeNumerically("<", 150*time.Millisecond),
				"failure must happen fast (per the injected client), well before the 200ms server delay or the 5s default")
		})

		It("UT-FMC-HC-012: NewHTTPClient without options preserves the package's own default client", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"managed":true}`))
			}))
			client = fmc.NewHTTPClient(server.URL)

			managed, err := client.IsManagedResource(context.Background(), scope.ResourceIdentity{
				ClusterID: "prod-east", Kind: "Deployment", Name: "nginx",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue())
		})
	})

	// Issue #54 fleet E2E RCA (CI run 30464667745): FMC's handler now returns
	// 503 for a failed check (transient, e.g. context canceled under load),
	// distinct from a completed check's 200 (managed true or false). These
	// tests lock in a *bounded* retry-with-backoff on the transient case only
	// -- the RCA's follow-up requirement that this must not retry forever.
	Describe("Bounded retry with backoff on transient failures [SC-7, Issue #54]", func() {
		var requestCount int32

		countingHandler := func(fn func(w http.ResponseWriter, r *http.Request, n int32)) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				n := atomic.AddInt32(&requestCount, 1)
				fn(w, r, n)
			}
		}

		BeforeEach(func() {
			requestCount = 0
		})

		It("UT-FMC-HC-013 [SC-7]: retries on 503 and succeeds once FMC recovers, without exhausting all attempts", func() {
			server = httptest.NewServer(countingHandler(func(w http.ResponseWriter, _ *http.Request, n int32) {
				if n < 2 {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"managed":true}`))
			}))
			client = fmc.NewHTTPClient(server.URL)

			managed, err := client.IsManagedResource(context.Background(), scope.ResourceIdentity{
				ClusterID: "prod-east", Kind: "Deployment", Name: "nginx",
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue())
			Expect(atomic.LoadInt32(&requestCount)).To(Equal(int32(2)),
				"must retry exactly once after the transient 503 before succeeding")
		})

		It("UT-FMC-HC-014 [SC-7]: retries on a transport error and succeeds once the connection recovers", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"managed":true}`))
			}))
			flaky := &flakyRoundTripper{failCount: 1, delegate: http.DefaultTransport}
			client = fmc.NewHTTPClient(server.URL, fmc.WithHTTPClient(&http.Client{Transport: flaky}))

			managed, err := client.IsManagedResource(context.Background(), scope.ResourceIdentity{
				ClusterID: "prod-east", Kind: "Deployment", Name: "nginx",
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue())
			Expect(flaky.attempts.Load()).To(Equal(int32(2)),
				"must retry exactly once after the transient transport error before succeeding")
		})

		It("UT-FMC-HC-015 [SC-7]: does NOT retry a definitive 200 managed=false response", func() {
			server = httptest.NewServer(countingHandler(func(w http.ResponseWriter, _ *http.Request, _ int32) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"managed":false}`))
			}))
			client = fmc.NewHTTPClient(server.URL)

			managed, err := client.IsManagedResource(context.Background(), scope.ResourceIdentity{
				ClusterID: "prod-east", Kind: "Deployment", Name: "missing",
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeFalse())
			Expect(atomic.LoadInt32(&requestCount)).To(Equal(int32(1)),
				"a completed check that determined not-managed is a final answer, not a failure to retry")
		})

		It("UT-FMC-HC-016 [SC-7]: does NOT retry a non-5xx, non-200 status (e.g. 400)", func() {
			server = httptest.NewServer(countingHandler(func(w http.ResponseWriter, _ *http.Request, _ int32) {
				w.WriteHeader(http.StatusBadRequest)
			}))
			client = fmc.NewHTTPClient(server.URL)

			managed, err := client.IsManagedResource(context.Background(), scope.ResourceIdentity{
				ClusterID: "prod-east", Kind: "Deployment", Name: "nginx",
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeFalse())
			Expect(atomic.LoadInt32(&requestCount)).To(Equal(int32(1)),
				"a 4xx is a client-side problem, not a transient backend failure -- must fail fast, not retry")
		})

		It("UT-FMC-HC-017 [SC-7]: gives up after a bounded number of attempts when 503 persists -- does not retry forever", func() {
			server = httptest.NewServer(countingHandler(func(w http.ResponseWriter, _ *http.Request, _ int32) {
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			client = fmc.NewHTTPClient(server.URL)

			start := time.Now()
			managed, err := client.IsManagedResource(context.Background(), scope.ResourceIdentity{
				ClusterID: "prod-east", Kind: "Deployment", Name: "nginx",
			})
			elapsed := time.Since(start)

			Expect(err).ToNot(HaveOccurred(),
				"exhausted retries must still fail safe (SC-7), never propagate an error")
			Expect(managed).To(BeFalse())
			finalCount := atomic.LoadInt32(&requestCount)
			Expect(finalCount).To(BeNumerically(">", 1),
				"must have retried at least once")
			Expect(finalCount).To(BeNumerically("<=", 3),
				"must NOT retry forever -- attempts are bounded by a ceiling")
			Expect(elapsed).To(BeNumerically("<", 3*time.Second),
				"the bounded retry budget must keep total added latency small on Gateway's synchronous alert-ingestion path")
		})

		It("UT-FMC-HC-018 [SC-7]: honors a short caller context deadline instead of exhausting its own retry budget", func() {
			server = httptest.NewServer(countingHandler(func(w http.ResponseWriter, _ *http.Request, _ int32) {
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			client = fmc.NewHTTPClient(server.URL)

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer cancel()

			start := time.Now()
			managed, err := client.IsManagedResource(ctx, scope.ResourceIdentity{
				ClusterID: "prod-east", Kind: "Deployment", Name: "nginx",
			})
			elapsed := time.Since(start)

			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeFalse())
			Expect(elapsed).To(BeNumerically("<", 1*time.Second),
				"a short caller-provided context deadline must cut the retry loop short, not run to its own internal ceiling")
		})
	})
})

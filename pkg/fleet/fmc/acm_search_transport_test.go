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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	"github.com/jordigilh/kubernaut/pkg/fleet/observation"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
)

// UT-FMC-SRC-2661-025 (TEST_PLAN §8 Tier 1b): the transport hardening from
// review round 1 — one Search reply is byte-bounded before decoding, and
// chunk failures are classified transport/tool/malformed onto the matching
// observation failure. Every case drives the production ACMHTTPSearch (the
// constructor buildACMSource uses), never a fake, so the classification
// asserted is the one production reports.
var _ = Describe("ACM search transport hardening (DD-FLEET-017 D4.3)", func() {
	var tokenPath string

	BeforeEach(func() {
		tokenPath = filepath.Join(GinkgoT().TempDir(), "token")
		Expect(os.WriteFile(tokenPath, []byte("transport-token"), 0o600)).To(Succeed())
	})

	listPods := func(search fmc.ACMSearch) (observation.ListObservation, error) {
		source, err := fmc.NewACMSource(fmc.ACMSourceConfig{
			Search: search,
			Kinds:  map[string]schema.GroupVersionKind{"Pod": {Version: "v1", Kind: "Pod"}},
		})
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		reader, err := source.ReaderFor(context.Background(), registry.ClusterInfo{ID: acmSourceCluster})
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		return reader.ListObserved(context.Background(), observation.ListRequest{
			APIVersion: "v1",
			Kind:       "Pod",
			Limit:      observation.ObservationPageLimit,
		})
	}

	It("UT-FMC-SRC-2661-025: an oversized but well-formed reply is rejected before decoding", func() {
		// A valid GraphQL reply whose single item carries a 9 MiB filler field:
		// without the byte bound this decodes and the collection succeeds, so
		// only the bound can make this collection fail.
		item := map[string]any{
			"_uid":       "uid-1",
			"cluster":    acmSourceCluster,
			"apiversion": "v1",
			"kind":       "Pod",
			"name":       "oversized",
			"namespace":  "payments",
			"label":      "kubernaut.ai/managed=true",
			"filler":     make([]byte, 9<<20),
		}
		reply, err := json.Marshal(map[string]any{
			"data": map[string]any{
				"searchResult": []map[string]any{{"count": 1, "items": []map[string]any{item}}},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(reply)
		}))
		DeferCleanup(server.Close)

		search, err := fmc.NewACMHTTPSearch(server.URL, tokenPath, "", 5*time.Second)
		Expect(err).NotTo(HaveOccurred())

		observed, listErr := listPods(search)
		ExpectWithOffset(1, listErr).To(HaveOccurred(),
			"a reply above the transport byte bound must fail the collection, not be decoded")
		ExpectWithOffset(1, observed.Complete).To(BeFalse())
		ExpectWithOffset(1, observed.Failure).To(Equal(observation.FailureMalformedContent),
			"the byte bound reports malformed content, not a transport outage")
	})

	It("UT-FMC-SRC-2661-025: chunk failures are classified onto the matching observation failure", func() {
		By("an HTTP error status is a tool failure, never a network outage")
		httpError := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		DeferCleanup(httpError.Close)
		search, err := fmc.NewACMHTTPSearch(httpError.URL, tokenPath, "", 5*time.Second)
		Expect(err).NotTo(HaveOccurred())
		observed, listErr := listPods(search)
		ExpectWithOffset(1, listErr).To(HaveOccurred())
		ExpectWithOffset(1, observed.Failure).To(Equal(observation.FailureTool),
			"the endpoint answered; reporting it as transport would hide an auth or GraphQL problem")

		By("an undecodable body is malformed content")
		garbage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("not-json{"))
		}))
		DeferCleanup(garbage.Close)
		search, err = fmc.NewACMHTTPSearch(garbage.URL, tokenPath, "", 5*time.Second)
		Expect(err).NotTo(HaveOccurred())
		observed, listErr = listPods(search)
		ExpectWithOffset(1, listErr).To(HaveOccurred())
		ExpectWithOffset(1, observed.Failure).To(Equal(observation.FailureMalformedContent))

		By("a TLS failure against an untrusted host is transport")
		untrusted := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"searchResult":[{"count":0,"items":[]}]}}`))
		}))
		DeferCleanup(untrusted.Close)
		// The client trusts only an unrelated CA, so the handshake itself
		// fails before any handler runs. The spec is deliberately
		// self-contained here: it must also compile and run against
		// pre-hardening baselines for its RED replay.
		wrongCA := transportUnrelatedCA(GinkgoT().TempDir())
		search, err = fmc.NewACMHTTPSearch(untrusted.URL, tokenPath, wrongCA, 5*time.Second)
		Expect(err).NotTo(HaveOccurred())
		observed, listErr = listPods(search)
		ExpectWithOffset(1, listErr).To(HaveOccurred())
		ExpectWithOffset(1, observed.Failure).To(Equal(observation.FailureTransport),
			"a handshake failure is a network-level outage, not a source problem")
	})

	It("UT-FMC-SRC-2661-026: a redirect answer is refused, never followed", func() {
		var targetHits int
		redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			targetHits++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"searchResult":[{"count":1,"items":[` +
				`{"_uid":"uid-1","cluster":"` + acmSourceCluster + `","apiversion":"v1","kind":"Pod","name":"web","namespace":"payments","label":"kubernaut.ai/managed=true"}]}]}}`))
		}))
		DeferCleanup(redirectTarget.Close)
		redirecting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, redirectTarget.URL, http.StatusFound)
		}))
		DeferCleanup(redirecting.Close)

		search, err := fmc.NewACMHTTPSearch(redirecting.URL, tokenPath, "", 5*time.Second)
		Expect(err).NotTo(HaveOccurred())
		observed, listErr := listPods(search)
		ExpectWithOffset(1, listErr).To(HaveOccurred(),
			"a redirect answer must fail the collection; following it would let the endpoint route the request — and the bearer — elsewhere")
		ExpectWithOffset(1, observed.Complete).To(BeFalse())
		ExpectWithOffset(1, targetHits).To(BeZero(),
			"the redirect target must never receive the request, so it can never receive the bearer token")
	})

	It("UT-FMC-SRC-2661-026: trailing data after the JSON reply is malformed, never accepted", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"searchResult":[{"count":0,"items":[]}]}} not-json-trailing`))
		}))
		DeferCleanup(server.Close)
		search, err := fmc.NewACMHTTPSearch(server.URL, tokenPath, "", 5*time.Second)
		Expect(err).NotTo(HaveOccurred())
		observed, listErr := listPods(search)
		ExpectWithOffset(1, listErr).To(HaveOccurred(),
			"a reply that is not exactly one JSON value is malformed; accepting its first value would publish a truncated answer as complete")
		ExpectWithOffset(1, observed.Failure).To(Equal(observation.FailureMalformedContent))
	})

	DescribeTable("UT-FMC-SRC-2661-026: a reply without an explicit nonnegative count is a tool failure, never a proven empty collection",
		func(body string) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
			}))
			DeferCleanup(server.Close)
			search, err := fmc.NewACMHTTPSearch(server.URL, tokenPath, "", 5*time.Second)
			Expect(err).NotTo(HaveOccurred())
			observed, listErr := listPods(search)
			ExpectWithOffset(1, listErr).To(HaveOccurred(),
				"only an explicitly provided nonnegative count can prove a traversal; a missing, null or negative count must not publish an empty inventory")
			ExpectWithOffset(1, observed.Failure).To(Equal(observation.FailureTool))
		},
		Entry("count absent", `{"data":{"searchResult":[{"items":[]}]}}`),
		Entry("count null", `{"data":{"searchResult":[{"count":null,"items":[]}]}}`),
		Entry("count negative", `{"data":{"searchResult":[{"count":-1,"items":[]}]}}`),
	)

	It("UT-FMC-SRC-2661-026: a valid explicit zero count with empty items remains a complete empty page", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"searchResult":[{"count":0,"items":[]}]}}`))
		}))
		DeferCleanup(server.Close)
		search, err := fmc.NewACMHTTPSearch(server.URL, tokenPath, "", 5*time.Second)
		Expect(err).NotTo(HaveOccurred())
		observed, listErr := listPods(search)
		ExpectWithOffset(1, listErr).NotTo(HaveOccurred(),
			"an explicit count of zero with no items is the well-formed empty answer")
		Expect(observed.Complete).To(BeTrue())
		Expect(observed.Items).To(BeEmpty())
	})
})

func mustParse(raw string) *url.URL {
	parsed, err := url.Parse(raw)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return parsed
}

// transportUnrelatedCA writes a throwaway self-signed certificate that signs
// nothing the test serves, so a client trusting it fails the TLS handshake.
func transportUnrelatedCA(dir string) string {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "acm-transport-unrelated-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	path := filepath.Join(dir, "unrelated-ca.crt")
	ExpectWithOffset(1, os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600)).To(Succeed())
	return path
}

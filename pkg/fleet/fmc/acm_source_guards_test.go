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
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	"github.com/jordigilh/kubernaut/pkg/fleet/observation"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// These specs exist because the first pass of the collector's own tests could
// pass against a subtly broken implementation: a walk that ignores offsets, one
// that resets uid state per page, or a label parser that turns corruption into
// "absent" all looked green.

var _ = Describe("ACM source collector — guards the first pass could not see (DD-FLEET-017 D4.3)", func() {
	// UT-FMC-SRC-2661-014
	It("UT-FMC-SRC-2661-014: continues by offset, not by re-reading the first page", func() {
		items := make([]map[string]json.RawMessage, 0, 120)
		for index := 0; index < 120; index++ {
			items = append(items, searchItem(fmt.Sprintf("uid-%03d", index), fmt.Sprintf("app-%03d", index), "kubernaut.ai/managed=true"))
		}
		search := &fakeACMSearch{replies: chunked(items)}

		observed, err := readAll(newACMSource(search))

		Expect(err).ToNot(HaveOccurred())
		Expect(observed.Complete).To(BeTrue())
		Expect(observed.Items).To(HaveLen(120))
		offsets := make([]int, 0, len(search.calls))
		for _, call := range search.calls {
			offsets = append(offsets, call.Offset)
		}
		Expect(offsets).To(Equal([]int{0, 50, 100}),
			"each chunk must advance by the previous chunk's size")
	})

	// UT-FMC-SRC-2661-016
	It("UT-FMC-SRC-2661-016: rejects a uid that goes backwards across pages", func() {
		pageOne := make([]map[string]json.RawMessage, 0, 50)
		for index := 0; index < 50; index++ {
			pageOne = append(pageOne, searchItem(fmt.Sprintf("uid-%03d", 100+index), fmt.Sprintf("app-%03d", index), "kubernaut.ai/managed=true"))
		}

		observed, err := readAll(newACMSource(&fakeACMSearch{replies: []fakeACMReply{
			{count: 51, items: pageOne},
			{count: 51, items: []map[string]json.RawMessage{searchItem("uid-050", "late-arrival", "kubernaut.ai/managed=true")}},
		}}))

		Expect(err).To(HaveOccurred(), "uid state must survive a page boundary")
		Expect(observed.Complete).To(BeFalse())
		Expect(observed.Failure).To(Equal(observation.FailureMalformedContent))
	})

	// UT-FMC-SRC-2661-022
	It("UT-FMC-SRC-2661-022: refuses a collection above the partition item ceiling", func() {
		replies := make([]fakeACMReply, 0, 102)
		total := 0
		for page := 0; page < 102; page++ {
			items := make([]map[string]json.RawMessage, 0, 50)
			for index := 0; index < 50; index++ {
				items = append(items, searchItem(fmt.Sprintf("uid-%05d", total), fmt.Sprintf("app-%05d", total), "kubernaut.ai/managed=true"))
				total++
			}
			replies = append(replies, fakeACMReply{count: 5100, items: items})
		}

		observed, err := readAll(newACMSource(&fakeACMSearch{replies: replies}))

		Expect(err).To(HaveOccurred())
		Expect(observed.Complete).To(BeFalse())
		Expect(observed.Failure).To(Equal(observation.FailureBounds),
			"the partition ceiling is a bound, not a truncation point")
	})

	// UT-FMC-SRC-2661-012: the collector-side label-state shapes
	DescribeTable("UT-FMC-SRC-2661-012: malformed projected labels are invalid, never absent and never resolved",
		func(label string, expected scope.LabelState) {
			item := searchItem("uid-1", "api", label)
			observed, err := readAll(newACMSource(&fakeACMSearch{replies: chunked([]map[string]json.RawMessage{item})}))

			Expect(err).ToNot(HaveOccurred(), "collection succeeds; the syncer rejects an invalid label")
			Expect(observed.Items).To(HaveLen(1))
			Expect(observed.Items[0].LabelState).To(Equal(expected))
			Expect(observed.Items[0].LabelsPresent).To(BeTrue(),
				"the object did carry a labels field, even if its value is unusable")
		},
		Entry("unrecognised value", "kubernaut.ai/managed=maybe", scope.LabelStateInvalid),
		Entry("empty value", "kubernaut.ai/managed=", scope.LabelStateInvalid),
		Entry("duplicate managed key", "kubernaut.ai/managed=true;kubernaut.ai/managed=false", scope.LabelStateInvalid),
		Entry("entry without a separator", "kubernaut.ai/managed", scope.LabelStateInvalid),
		Entry("valid true", "kubernaut.ai/managed=true", scope.LabelStateTrue),
		Entry("valid false", "kubernaut.ai/managed=false", scope.LabelStateFalse),
		Entry("other labels only", "app=checkout;tier=web", scope.LabelStateAbsent),
	)

	It("UT-FMC-SRC-2661-012: a null label field is invalid, not absent", func() {
		item := searchItem("uid-1", "api", "")
		item["label"] = json.RawMessage("null")

		observed, err := readAll(newACMSource(&fakeACMSearch{replies: chunked([]map[string]json.RawMessage{item})}))

		Expect(err).ToNot(HaveOccurred())
		Expect(observed.Items).To(HaveLen(1))
		Expect(observed.Items[0].LabelState).To(Equal(scope.LabelStateInvalid))
		Expect(observed.Items[0].LabelsPresent).To(BeTrue())
	})

	DescribeTable("UT-FMC-SRC-2661-012: a per-key label projection is held to the same rule as the semicolon field",
		func(rawValue string) {
			// Legacy projections use a dedicated key instead of the `label` field.
			item := searchItem("uid-1", "api", "")
			item["label.kubernaut.ai/managed"] = json.RawMessage(rawValue)

			observed, err := readAll(newACMSource(&fakeACMSearch{replies: chunked([]map[string]json.RawMessage{item})}))

			Expect(err).ToNot(HaveOccurred())
			Expect(observed.Items).To(HaveLen(1))
			Expect(observed.Items[0].LabelsPresent).To(BeTrue(),
				"the field was present, so claiming it was absent would hide corruption")
			Expect(observed.Items[0].LabelState).To(Equal(scope.LabelStateInvalid))
		},
		Entry("null value", "null"),
		Entry("numeric value", "42"),
		Entry("empty value", `""`),
	)

	It("UT-FMC-SRC-2661-012: two managed-label sources at once is corruption, not a precedence choice", func() {
		// Search answers with either the projected `label` field or the legacy
		// per-key form. Both at once has no defined winner, and silently picking
		// one can turn corruption into a clean managed/unmanaged answer.
		item := searchItem("uid-1", "api", "app=web")
		item["label.kubernaut.ai/managed"] = mustJSON("maybe")

		observed, err := readAll(newACMSource(&fakeACMSearch{replies: chunked([]map[string]json.RawMessage{item})}))

		Expect(err).ToNot(HaveOccurred())
		Expect(observed.Items[0].LabelsPresent).To(BeTrue())
		Expect(observed.Items[0].LabelState).To(Equal(scope.LabelStateInvalid))
	})

	It("UT-FMC-SRC-2661-012: conflicting legacy managed-label spellings are corruption", func() {
		item := searchItem("uid-1", "api", "")
		item["label.kubernaut.ai/managed"] = mustJSON("true")
		item["labels.kubernaut.ai/managed"] = mustJSON("false")

		observed, err := readAll(newACMSource(&fakeACMSearch{replies: chunked([]map[string]json.RawMessage{item})}))

		Expect(err).ToNot(HaveOccurred())
		Expect(observed.Items[0].LabelsPresent).To(BeTrue())
		Expect(observed.Items[0].LabelState).To(Equal(scope.LabelStateInvalid),
			"first-match would invent an answer the source never gave")
	})

	It("UT-FMC-SRC-2661-012: a space-adjacent separator is malformed, matching the Check lane", func() {
		// pkg/fleet/acm/client_test.go pins "kubernaut.ai/managed =true" as
		// ScopeInvalid for the Check lane; the collector must not read the same
		// string as managed, or the two lanes disagree about one fleet.
		item := searchItem("uid-1", "api", "kubernaut.ai/managed =true")

		observed, err := readAll(newACMSource(&fakeACMSearch{replies: chunked([]map[string]json.RawMessage{item})}))

		Expect(err).ToNot(HaveOccurred())
		Expect(observed.Items[0].LabelsPresent).To(BeTrue())
		Expect(observed.Items[0].LabelState).To(Equal(scope.LabelStateInvalid))
	})

	It("UT-FMC-SRC-2661-013: an item without cluster attribution fails the kind", func() {
		item := searchItem("uid-1", "api", "kubernaut.ai/managed=true")
		delete(item, "cluster")

		observed, err := readAll(newACMSource(&fakeACMSearch{replies: chunked([]map[string]json.RawMessage{item})}))

		Expect(err).To(HaveOccurred(), "ownership is never assumed from Search visibility")
		Expect(observed.Complete).To(BeFalse())
	})

	It("UT-FMC-SRC-2661-013: a non-string namespace on a cluster-scoped kind fails the kind", func() {
		item := map[string]json.RawMessage{
			"_uid":       mustJSON("uid-1"),
			"cluster":    mustJSON(acmSourceCluster),
			"apiversion": mustJSON("v1"),
			"kind":       mustJSON("Namespace"),
			"name":       mustJSON("payments"),
			"namespace":  json.RawMessage("42"),
		}
		source := newACMSource(&fakeACMSearch{replies: chunked([]map[string]json.RawMessage{item})}, func(config *fmc.ACMSourceConfig) {
			config.Kinds = map[string]schema.GroupVersionKind{"Namespace": {Version: "v1", Kind: "Namespace"}}
		})

		observed, err := readAll(source)

		Expect(err).To(HaveOccurred())
		Expect(observed.Complete).To(BeFalse())
	})
})

// UT-FMC-SRC-2661-023: the transport contract, over real TLS servers.
var _ = Describe("ACM source transport (DD-FLEET-017 D4.1a rule 6)", func() {
	It("UT-FMC-SRC-2661-023: sends the bearer token from tokenPath, validates TLS against caFile, and honours rotated trust", func() {
		firstAuthority := newTestAuthority()
		secondAuthority := newTestAuthority()

		seen := &tokenRecorder{}
		first := newACMSearchServer(firstAuthority.serverCertificate(), "token-from-file", seen)
		DeferCleanup(first.Close)
		second := newACMSearchServer(secondAuthority.serverCertificate(), "token-from-file", seen)
		DeferCleanup(second.Close)

		dir := GinkgoT().TempDir()
		caPath := filepath.Join(dir, "ca.crt")
		Expect(os.WriteFile(caPath, firstAuthority.caPEM, 0o600)).To(Succeed())
		tokenPath := filepath.Join(dir, "token")
		Expect(os.WriteFile(tokenPath, []byte("token-from-file"), 0o600)).To(Succeed())

		trusted, err := fmc.NewACMHTTPSearch(first.URL, tokenPath, caPath, 5*time.Second)
		Expect(err).ToNot(HaveOccurred())
		page, err := trusted.Search(context.Background(), searchOptionsForTest())
		Expect(err).ToNot(HaveOccurred())
		Expect(page.Count).To(Equal(0))
		Expect(seen.last()).To(Equal("Bearer token-from-file"),
			"the bearer token must come from tokenPath, not from the environment")

		untrusted, err := fmc.NewACMHTTPSearch(second.URL, tokenPath, caPath, 5*time.Second)
		Expect(err).ToNot(HaveOccurred())
		_, err = untrusted.Search(context.Background(), searchOptionsForTest())
		Expect(err).To(HaveOccurred(), "a CA that does not sign the host must fail closed")

		Expect(untrusted.ReloadCA(secondAuthority.caPEM)).To(Succeed())
		_, err = untrusted.Search(context.Background(), searchOptionsForTest())
		Expect(err).ToNot(HaveOccurred(), "rotated trust applies to the same client, without recreation")
	})
})

func searchOptionsForTest() fmc.ACMSearchOptions {
	return fmc.ACMSearchOptions{ClusterID: acmSourceCluster, Version: "v1", Kind: "ConfigMap", OrderBy: "uid asc", Limit: 50}
}

// tokenRecorder captures the Authorization header the server received.
type tokenRecorder struct {
	mu   sync.Mutex
	auth string
}

func (r *tokenRecorder) record(value string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.auth = value
}

func (r *tokenRecorder) last() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.auth
}

// newACMSearchServer serves one empty Search reply and requires the token.
func newACMSearchServer(cert tls.Certificate, expectedToken string, recorder *tokenRecorder) *httptest.Server {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder.record(r.Header.Get("Authorization"))
		if r.Header.Get("Authorization") != "Bearer "+expectedToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"data":{"searchResult":[{"count":0,"items":[]}]}}`))
	})
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	server.StartTLS()
	return server
}

// testAuthority is a throwaway CA that can issue a 127.0.0.1 server certificate.
type testAuthority struct {
	caPEM []byte
	cert  tls.Certificate
}

func newTestAuthority() *testAuthority {
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "acm-source-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())

	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano() + 1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caTemplate, &serverKey.PublicKey, caKey)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())

	certificate, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER}),
		pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(serverKey)}),
	)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())

	return &testAuthority{
		caPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}),
		cert:  certificate,
	}
}

func (a *testAuthority) serverCertificate() tls.Certificate { return a.cert }

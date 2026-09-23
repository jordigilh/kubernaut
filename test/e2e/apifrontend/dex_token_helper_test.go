package e2e_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DEX persona token helper", Label("dex-token-helper"), func() {
	It("UT-AF-E2E-1807-002 retries a DEX request that reaches its per-attempt timeout", func() {
		var requests atomic.Int32
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			requests.Add(1)
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		DeferCleanup(server.Close)

		client := server.Client()
		client.Timeout = 50 * time.Millisecond
		_, err := fetchDEXTokenWithClient(client, server.URL+"/dex", "client", "secret", "user", "password")

		Expect(err).To(HaveOccurred())
		Expect(requests.Load()).To(Equal(int32(dexTokenMaxAttempts)))
	})

	It("UT-AF-E2E-1807-001 reuses a persona JWT for the same DEX client", func() {
		var requests atomic.Int32
		var validRequest atomic.Bool
		validRequest.Store(true)
		var responseWriteError atomic.Bool
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			if r.URL.Path != "/dex/token" || r.ParseForm() != nil || r.FormValue("client_id") != "dex-token-helper-test" {
				validRequest.Store(false)
			}
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write([]byte(`{"id_token":"test-persona-token"}`)); err != nil {
				responseWriteError.Store(true)
			}
		}))
		DeferCleanup(server.Close)

		originalDEXURL, originalClientID, originalClientSecret := dexURL, clientID, clientSecret
		dexURL = server.URL + "/dex"
		clientID = "dex-token-helper-test"
		clientSecret = "dex-token-helper-secret"
		DeferCleanup(func() {
			dexURL, clientID, clientSecret = originalDEXURL, originalClientID, originalClientSecret
		})

		firstToken, err := fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred())
		secondToken, err := fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred())

		Expect(firstToken).To(Equal("test-persona-token"))
		Expect(secondToken).To(Equal(firstToken))
		Expect(requests.Load()).To(Equal(int32(1)), "the same persona/client should only authenticate once per worker")
		Expect(validRequest.Load()).To(BeTrue())
		Expect(responseWriteError.Load()).To(BeFalse())
	})
})

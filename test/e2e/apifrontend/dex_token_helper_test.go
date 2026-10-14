package e2e_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
)

var _ = Describe("Owned audit fixtures [BR-AUDIT-005, BR-PLATFORM-001.6a]", Label("helpers"), func() {
	It("UT-AF-2501-006: unique OIDC identity inherits only the existing SRE password/group fixture", func() {
		config := "issuer: https://dex:5556/dex\nstorage: {type: sqlite3}\nstaticPasswords:\n  - email: sre@kubernaut.ai\n    username: sre-user\n    userID: sre-id\n    hash: existing-hash\n    groups: [sre]\nstaticClients: [{id: existing-client}]\n"
		const actor = "audit-observer-owned@kubernaut.invalid"
		updated, err := addAuditOIDCActor(config, actor)
		Expect(err).NotTo(HaveOccurred())
		var decoded struct {
			Issuer        string `yaml:"issuer"`
			StaticClients []struct {
				ID string `yaml:"id"`
			} `yaml:"staticClients"`
			StaticPasswords []struct {
				Email  string   `yaml:"email"`
				Hash   string   `yaml:"hash"`
				Groups []string `yaml:"groups"`
			} `yaml:"staticPasswords"`
		}
		Expect(yaml.Unmarshal([]byte(updated), &decoded)).To(Succeed())
		Expect(decoded.Issuer).To(Equal("https://dex:5556/dex"))
		Expect(decoded.StaticClients).To(HaveLen(1))
		Expect(decoded.StaticPasswords).To(HaveLen(2))
		Expect(decoded.StaticPasswords[1].Email).To(Equal(actor))
		Expect(decoded.StaticPasswords[1].Hash).To(Equal("existing-hash"))
		Expect(decoded.StaticPasswords[1].Groups).To(ConsistOf("sre"))
		_, err = addAuditOIDCActor(strings.ReplaceAll(config, "sre@kubernaut.ai", "unrelated@kubernaut.ai"), actor)
		Expect(err).To(HaveOccurred(), "never invent another policy/password fixture")
	})

	It("UT-AF-2501-007: parallel failure markers use owned temp storage and return write failures", func() {
		dir := GinkgoT().TempDir()
		GinkgoT().Setenv("TMPDIR", dir)
		Expect(markAFFailure("owned-2501")).To(Succeed())
		body, err := os.ReadFile(afFailureMarkerPath("owned-2501"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(body)).To(Equal("1"))
		Expect(afFailureMarkerPath("owned-2501")).To(HavePrefix(dir + string(os.PathSeparator)))
		blocked := filepath.Join(dir, "not-a-directory")
		Expect(os.WriteFile(blocked, []byte("owned"), 0600)).To(Succeed())
		GinkgoT().Setenv("TMPDIR", blocked)
		Expect(markAFFailure("owned-2501")).To(HaveOccurred())
	})

	DescribeTable("UT-AF-2501-008: audit attribution uses the authenticated DEX subject, never its login email",
		func(payload, subject string, invalid bool) {
			token := "test-header." + base64.RawURLEncoding.EncodeToString([]byte(payload)) + ".test-signature"
			actor, err := auditOIDCSubject(token)
			if invalid {
				Expect(err).To(HaveOccurred())
				Expect(actor).To(BeEmpty())
				return
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(actor).To(Equal(subject))
		}, Entry("distinct opaque subject", `{"sub":"owned-opaque-subject-2501","email":"owned@kubernaut.invalid"}`, "owned-opaque-subject-2501", false),
		Entry("missing subject", `{"email":"owned@kubernaut.invalid"}`, "", true), Entry("invalid claims", `{`, "", true))

	DescribeTable("UT-AF-2501-008: malformed fixture tokens fail without being logged or accepted",
		func(token string) {
			actor, err := auditOIDCSubject(token)
			Expect(err).To(HaveOccurred())
			Expect(actor).To(BeEmpty())
			Expect(err.Error()).ToNot(ContainSubstring(token))
		}, Entry("not a JWT", "synthetic-invalid-2501"), Entry("invalid base64 claims", "h.%synthetic-invalid-2501.s"))
})

var _ = Describe("DEX persona token helper", Label("dex-token-helper"), func() {
	It("UT-AF-E2E-1807-004 allows a slow but responsive DEX token request", func() {
		var requests atomic.Int32
		var responseWriteError atomic.Bool
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if requests.Add(1) == 1 {
				time.Sleep(1500 * time.Millisecond)
				w.Header().Set("Content-Type", "application/json")
				if _, err := w.Write([]byte(`{"id_token":"slow-response-token"}`)); err != nil {
					responseWriteError.Store(true)
				}
				return
			}
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		DeferCleanup(server.Close)

		token, err := fetchDEXToken(server.URL+"/dex", "client", "secret", "user", "password")
		Expect(err).NotTo(HaveOccurred())
		Expect(token).To(Equal("slow-response-token"))
		Expect(requests.Load()).To(Equal(int32(1)))
		Expect(responseWriteError.Load()).To(BeFalse())
	})

	It("UT-AF-E2E-1807-003 distributes prewarmed persona tokens to suite workers", func() {
		var requests atomic.Int32
		var responseWriteError atomic.Bool
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			requests.Add(1)
			if _, err := w.Write([]byte(`{"id_token":"unexpected-network-token"}`)); err != nil {
				responseWriteError.Store(true)
			}
		}))
		DeferCleanup(server.Close)

		originalDEXURL, originalClientID, originalClientSecret := dexURL, clientID, clientSecret
		dexURL = server.URL + "/dex"
		clientID = "suite-setup-token-test"
		clientSecret = "suite-setup-secret"
		DeferCleanup(func() {
			dexURL, clientID, clientSecret = originalDEXURL, originalClientID, originalClientSecret
		})

		setup := afE2ESuiteSetup{PersonaTokens: map[string]string{
			"sre":                         "shared-suite-token",
			structuredDecisionPersonaName: "structured-decision-suite-token",
		}}
		encoded, err := json.Marshal(setup)
		Expect(err).NotTo(HaveOccurred())
		var workerSetup afE2ESuiteSetup
		Expect(json.Unmarshal(encoded, &workerSetup)).To(Succeed())
		Expect(seedDEXPersonaTokenCache(workerSetup.PersonaTokens)).To(Succeed())

		token, err := fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred())
		Expect(token).To(Equal("shared-suite-token"))
		Expect(requests.Load()).To(Equal(int32(0)), "workers should use the synchronized token instead of calling DEX")
		Expect(responseWriteError.Load()).To(BeFalse())

		dedicatedToken, err := fetchDEXTokenForDedicatedPersona(structuredDecisionPersonaName)
		Expect(err).NotTo(HaveOccurred())
		Expect(dedicatedToken).To(Equal("structured-decision-suite-token"))
	})

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

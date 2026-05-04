package auth_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"

	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
)

// k8sStatusError simulates a K8s API 403 error for testing.
type k8sStatusError struct {
	code    int
	reason  string
	message string
}

func (e *k8sStatusError) Error() string { return e.message }
func (e *k8sStatusError) Status() int   { return e.code }

var _ = Describe("Impersonation", func() {
	Context("NewImpersonatedConfig", func() {
		It("UT-AF-055-001: sets impersonate user and groups on REST config", func() {
			baseCfg := &rest.Config{Host: "https://k8s.example.com"}
			cfg, err := auth.NewImpersonatedConfig(baseCfg, "alice", []string{"sre-team", "dev-team"})
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).NotTo(BeNil())

			Expect(cfg.Impersonate.UserName).To(Equal("alice"))
			Expect(cfg.Impersonate.Groups).To(Equal([]string{"sre-team", "dev-team"}))
		})

		It("UT-AF-055-002: exposes host and impersonation fields needed for triage verbs", func() {
			baseCfg := &rest.Config{Host: "https://k8s.example.com"}
			cfg, err := auth.NewImpersonatedConfig(baseCfg, "alice", []string{"sre-team"})
			Expect(err).NotTo(HaveOccurred())

			Expect(cfg.Host).To(Equal("https://k8s.example.com"))
			Expect(cfg.Impersonate.UserName).To(Equal("alice"))
		})

		It("UT-AF-055-009: rejects empty username with ErrEmptyUsername", func() {
			baseCfg := &rest.Config{Host: "https://k8s.example.com"}

			_, err := auth.NewImpersonatedConfig(baseCfg, "", []string{"sre"})
			Expect(errors.Is(err, auth.ErrEmptyUsername)).To(BeTrue())
		})

		It("UT-AF-055-010: deep-copies base REST config without mutating impersonation on the original", func() {
			baseCfg := &rest.Config{
				Host:        "https://k8s.example.com",
				BearerToken: "sa-token",
			}

			cfg, err := auth.NewImpersonatedConfig(baseCfg, "alice", []string{"sre"})
			Expect(err).NotTo(HaveOccurred())

			Expect(baseCfg.Impersonate.UserName).To(Equal(""), "original config must not be mutated")
			Expect(baseCfg.Impersonate.Groups).To(BeNil())
			Expect(cfg.Impersonate.UserName).To(Equal("alice"))
			Expect(cfg.Impersonate.Groups).To(Equal([]string{"sre"}))
			Expect(cfg.BearerToken).To(Equal("sa-token"), "SA token should be preserved in copy")
		})

		It("UT-AF-055-015: builds impersonation from user identity username and groups", func() {
			baseCfg := &rest.Config{Host: "https://k8s.example.com"}
			identity := &auth.UserIdentity{Username: "bob", Groups: []string{"dev", "ops"}}

			cfg, err := auth.NewImpersonatedConfig(baseCfg, identity.Username, identity.Groups)
			Expect(err).NotTo(HaveOccurred())

			Expect(cfg.Impersonate.UserName).To(Equal("bob"))
			Expect(cfg.Impersonate.Groups).To(Equal([]string{"dev", "ops"}))
		})
	})

	Context("ClientFactory with ScopeUserImpersonation", func() {
		It("UT-AF-055-003: constructs a client for owner resolution using impersonation", func() {
			baseCfg := &rest.Config{Host: "https://k8s.example.com"}
			identity := &auth.UserIdentity{Username: "alice", Groups: []string{"sre"}}

			factory, err := auth.NewClientFactory(baseCfg)
			Expect(err).NotTo(HaveOccurred())

			client, err := factory.ClientForScope(auth.ScopeUserImpersonation, identity)
			Expect(err).NotTo(HaveOccurred())
			Expect(client).NotTo(BeNil())
		})

		It("UT-AF-055-004: constructs a client for scope checks using impersonation", func() {
			baseCfg := &rest.Config{Host: "https://k8s.example.com"}
			identity := &auth.UserIdentity{Username: "alice", Groups: []string{"sre"}}

			factory, err := auth.NewClientFactory(baseCfg)
			Expect(err).NotTo(HaveOccurred())

			client, err := factory.ClientForScope(auth.ScopeUserImpersonation, identity)
			Expect(err).NotTo(HaveOccurred())
			Expect(client).NotTo(BeNil())
		})
	})

	Context("ToUserFriendlyError for forbidden responses", func() {
		It("UT-AF-055-005: turns K8s 403 into a clear message without raw Status or API group jargon", func() {
			k8sErr := &k8sStatusError{
				code:    403,
				reason:  "Forbidden",
				message: `pods is forbidden: User "alice" cannot list resource "pods" in API group "" in the namespace "production"`,
			}

			friendlyErr := auth.ToUserFriendlyError(k8sErr)
			Expect(friendlyErr).To(ContainSubstring("lack access"))
			Expect(friendlyErr).NotTo(ContainSubstring("Status"))
			Expect(friendlyErr).NotTo(ContainSubstring("API group"))
		})
	})

	Context("ClientFactory with ScopeServiceAccount", func() {
		It("UT-AF-055-006: constructs a service-account client for RR creation (no user impersonation on client)", func() {
			baseCfg := &rest.Config{Host: "https://k8s.example.com"}
			identity := &auth.UserIdentity{Username: "alice", Groups: []string{"sre"}}

			factory, err := auth.NewClientFactory(baseCfg)
			Expect(err).NotTo(HaveOccurred())

			client, err := factory.ClientForScope(auth.ScopeServiceAccount, identity)
			Expect(err).NotTo(HaveOccurred())
			Expect(client).NotTo(BeNil())
		})

		It("UT-AF-055-007: constructs a service-account client for investigation session creation", func() {
			baseCfg := &rest.Config{Host: "https://k8s.example.com"}
			identity := &auth.UserIdentity{Username: "alice", Groups: []string{"sre"}}

			factory, err := auth.NewClientFactory(baseCfg)
			Expect(err).NotTo(HaveOccurred())

			client, err := factory.ClientForScope(auth.ScopeServiceAccount, identity)
			Expect(err).NotTo(HaveOccurred())
			Expect(client).NotTo(BeNil())
		})

		It("UT-AF-055-008: constructs a service-account client for lease creation", func() {
			baseCfg := &rest.Config{Host: "https://k8s.example.com"}
			identity := &auth.UserIdentity{Username: "alice", Groups: []string{"sre"}}

			factory, err := auth.NewClientFactory(baseCfg)
			Expect(err).NotTo(HaveOccurred())

			saClient, err := factory.ClientForScope(auth.ScopeServiceAccount, identity)
			Expect(err).NotTo(HaveOccurred())
			Expect(saClient).NotTo(BeNil())
		})
	})

	Context("JWTDelegationTransport", func() {
		It("UT-AF-055-011: forwards the original JWT in the Authorization header", func() {
			originalJWT := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.test.signature"

			var capturedAuth string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedAuth = r.Header.Get("Authorization")
				w.WriteHeader(http.StatusOK)
			}))
			DeferCleanup(srv.Close)

			transport := &auth.JWTDelegationTransport{
				Base:  srv.Client().Transport,
				Token: originalJWT,
			}

			client := &http.Client{Transport: transport}
			req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
			Expect(err).NotTo(HaveOccurred())
			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(capturedAuth).To(Equal("Bearer " + originalJWT))
		})

		It("UT-AF-055-012: forwards JWT bytes unchanged including special characters", func() {
			originalJWT := "eyJhbGciOiJSUzI1NiJ9.eyJpc3MiOiJodHRwczovL3Nzby5leGFtcGxlLmNvbSIsInN1YiI6ImFsaWNlIn0.sig-with-special+chars/and=padding"

			var capturedToken string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				authHeader := r.Header.Get("Authorization")
				capturedToken = authHeader[len("Bearer "):]
				w.WriteHeader(http.StatusOK)
			}))
			DeferCleanup(srv.Close)

			transport := &auth.JWTDelegationTransport{
				Base:  srv.Client().Transport,
				Token: originalJWT,
			}

			client := &http.Client{Transport: transport}
			req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
			Expect(err).NotTo(HaveOccurred())
			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			_, err = io.Copy(io.Discard, resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Body.Close()).To(Succeed())

			Expect(capturedToken).To(Equal(originalJWT), "JWT must be forwarded byte-identical")
		})
	})

	Context("Two-tier access model", func() {
		It("UT-AF-055-013: user-scoped queries obtain an impersonation-scoped client", func() {
			baseCfg := &rest.Config{Host: "https://k8s.example.com", BearerToken: "sa-token"}
			identity := &auth.UserIdentity{Username: "alice", Groups: []string{"sre"}}

			factory, err := auth.NewClientFactory(baseCfg)
			Expect(err).NotTo(HaveOccurred())

			_, err = factory.ClientForScope(auth.ScopeUserImpersonation, identity)
			Expect(err).NotTo(HaveOccurred())
		})

		It("UT-AF-055-014: AF-owned resources obtain a service-account-scoped client", func() {
			baseCfg := &rest.Config{Host: "https://k8s.example.com", BearerToken: "sa-token"}
			identity := &auth.UserIdentity{Username: "alice", Groups: []string{"sre"}}

			factory, err := auth.NewClientFactory(baseCfg)
			Expect(err).NotTo(HaveOccurred())

			_, err = factory.ClientForScope(auth.ScopeServiceAccount, identity)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

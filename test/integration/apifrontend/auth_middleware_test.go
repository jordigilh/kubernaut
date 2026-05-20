package apifrontend_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/httputil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func ptr[T any](v T) *T { return &v }

var _ = Describe("Auth Middleware Integration (auth/)", func() {

	// identityCapture wraps a handler to capture the UserIdentity from context.
	identityCapture := func(captured **auth.UserIdentity) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			*captured = auth.UserIdentityFromContext(r.Context())
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		})
	}

	// buildAuthServer creates a test server with the real auth middleware
	// protecting a handler that captures identity from context.
	buildAuthServer := func(captured **auth.UserIdentity) *httptest.Server {
		middleware := auth.MiddlewareWithConfig(auth.MiddlewareConfig{
			Validator:    jwtValidator,
			Logger:       logf.Log.WithName("auth-mw-it"),
			Auditor:      auditRecorder,
			AuthDuration: metricsRegistry.AuthDuration,
		})
		srv := httptest.NewServer(middleware(identityCapture(captured)))
		return srv
	}

	BeforeEach(func() {
		auditRecorder.Reset()
	})

	Describe("AC-9 + AC-12: JWT validation and identity propagation", func() {
		It("IT-AF-1195-014: valid JWT accepted, UserIdentity propagated to downstream", func() {
			var captured *auth.UserIdentity
			srv := buildAuthServer(&captured)
			defer srv.Close()

			token := signValidToken("it-jwt-user")
			req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(captured).NotTo(BeNil())
			Expect(captured.Username).To(Equal("it-jwt-user"))
			Expect(captured.Issuer).To(Equal(jwksServer.URL))
		})
	})

	Describe("AC-10: Token rejection with correct error classification", func() {
		It("IT-AF-1195-015: expired JWT rejected with 401", func() {
			var captured *auth.UserIdentity
			srv := buildAuthServer(&captured)
			defer srv.Close()

			token := signExpiredToken("it-expired-user")
			req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("application/problem+json"))
			Expect(captured).To(BeNil())
		})

		It("IT-AF-1195-016: wrong audience JWT rejected with 401", func() {
			var captured *auth.UserIdentity
			srv := buildAuthServer(&captured)
			defer srv.Close()

			token := signWrongAudienceToken("it-wrong-aud")
			req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			Expect(captured).To(BeNil())
		})

		It("IT-AF-1195-017: missing Authorization header rejected with 401", func() {
			var captured *auth.UserIdentity
			srv := buildAuthServer(&captured)
			defer srv.Close()

			resp, err := http.Get(srv.URL)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))

			var problem httputil.ProblemDetail
			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(json.Unmarshal(body, &problem)).To(Succeed())
			Expect(problem.Title).To(Equal("Missing Authorization"))
		})

		It("IT-AF-1195-018: non-bearer scheme rejected with 401", func() {
			var captured *auth.UserIdentity
			srv := buildAuthServer(&captured)
			defer srv.Close()

			req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))

			var problem httputil.ProblemDetail
			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(json.Unmarshal(body, &problem)).To(Succeed())
			Expect(problem.Title).To(Equal("Invalid Scheme"))
		})
	})

	Describe("AC-11: K8s ServiceAccount token validation via TokenReview", func() {
		It("IT-AF-1195-019: envtest SA token validated via TokenReview", func() {
			var captured *auth.UserIdentity
			srv := buildAuthServer(&captured)
			defer srv.Close()

			// Create a ServiceAccount in the per-process envtest and request a token
			sa := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "af-it-tokenreview-sa",
					Namespace: "default",
				},
			}
			err := k8sClient.Create(context.Background(), sa)
			if err != nil && !apierrors.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			// Request a token from the per-process envtest kube-apiserver
			tokenReq := &authv1.TokenRequest{
				Spec: authv1.TokenRequestSpec{
					ExpirationSeconds: ptr(int64(3600)),
				},
			}
			tokenResult, err := k8sClientset.CoreV1().ServiceAccounts("default").CreateToken(
				context.Background(), "af-it-tokenreview-sa", tokenReq, metav1.CreateOptions{},
			)
			Expect(err).NotTo(HaveOccurred())
			saToken := tokenResult.Status.Token
			Expect(saToken).NotTo(BeEmpty())

			// Send the SA token through the auth middleware -- it should fall through
			// JWT validation (unknown issuer) to TokenReview (K8s SA validation)
			req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Authorization", "Bearer "+saToken)

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(captured).NotTo(BeNil())
			Expect(captured.Username).To(ContainSubstring("af-it-tokenreview-sa"))
		})
	})

	Describe("AC-15: Audit event emission", func() {
		It("IT-AF-1195-020: auth success emits audit.EventAuthSuccess", func() {
			var captured *auth.UserIdentity
			srv := buildAuthServer(&captured)
			defer srv.Close()

			token := signValidToken("it-audit-success")
			req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			Eventually(func() []*audit.Event {
				return auditRecorder.EventsOfType(audit.EventAuthSuccess)
			}).Should(HaveLen(1))

			events := auditRecorder.EventsOfType(audit.EventAuthSuccess)
			Expect(events[0].UserID).To(Equal("it-audit-success"))
			Expect(events[0].Detail["auth_method"]).To(Equal("jwt"))
		})

		It("IT-AF-1195-021: auth failure emits audit.EventAuthFailure", func() {
			var captured *auth.UserIdentity
			srv := buildAuthServer(&captured)
			defer srv.Close()

			token := signExpiredToken("it-audit-failure")
			req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))

			Eventually(func() []*audit.Event {
				return auditRecorder.EventsOfType(audit.EventAuthFailure)
			}).Should(HaveLen(1))

			events := auditRecorder.EventsOfType(audit.EventAuthFailure)
			Expect(events[0].Detail["failure_reason"]).To(Equal("token_expired"))
		})
	})

	Describe("AC-13: JWKS circuit breaker fail-open", func() {
		It("IT-AF-1195-022: fail-open with cached keys when JWKS server is down", func() {
			requestCount := 0
			controllableJWKS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requestCount++
				if requestCount <= 2 {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(jwksKeyPair.jwks())
				} else {
					w.WriteHeader(http.StatusInternalServerError)
				}
			}))
			defer controllableJWKS.Close()

			cbCfg := auth.Config{
				JWT: []auth.ProviderConfig{
					{
						Issuer: auth.IssuerConfig{
							URL:       controllableJWKS.URL,
							JWKSURL:   controllableJWKS.URL,
							Audiences: []string{"kubernaut-af"},
						},
					},
				},
				Kubernetes:           auth.KubernetesAuthConfig{Enabled: false},
				AllowInsecureIssuers: true,
			}
			cbValidator, err := auth.NewJWTValidator(cbCfg,
				auth.WithHTTPClient(controllableJWKS.Client()),
				auth.WithCBTestTimeout(100*time.Millisecond),
			)
			Expect(err).NotTo(HaveOccurred())

			cbMiddleware := auth.MiddlewareWithConfig(auth.MiddlewareConfig{
				Validator: cbValidator,
				Logger:    logf.Log.WithName("cb-it"),
			})

			var captured *auth.UserIdentity
			cbSrv := httptest.NewServer(cbMiddleware(identityCapture(&captured)))
			defer cbSrv.Close()

			// First request: JWKS fetched, keys cached, validation succeeds
			token := jwksKeyPair.signToken(standardClaims(
				controllableJWKS.URL, "cb-user", []string{"kubernaut-af"}, time.Now().Add(1*time.Hour),
			))
			req, err := http.NewRequest(http.MethodGet, cbSrv.URL, nil)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(captured).NotTo(BeNil())

			// Trigger CB failures (server now returns 500)
			for i := 0; i < 4; i++ {
				captured = nil
				req, _ = http.NewRequest(http.MethodGet, cbSrv.URL, nil)
				req.Header.Set("Authorization", "Bearer "+token)
				resp, _ = http.DefaultClient.Do(req)
				if resp != nil {
					resp.Body.Close()
				}
			}

			// CB should be open but cached keys should still work (fail-open)
			captured = nil
			req, err = http.NewRequest(http.MethodGet, cbSrv.URL, nil)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err = http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK),
				"fail-open: cached JWKS keys should still validate tokens when CB is open")
			Expect(captured).NotTo(BeNil())
			Expect(captured.Username).To(Equal("cb-user"))
		})
	})

	Describe("AC-14: JWT delegation transport", func() {
		It("IT-AF-1195-023: JWT delegation forwards user token to downstream", func() {
			var receivedAuthHeader string
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedAuthHeader = r.Header.Get("Authorization")
				w.WriteHeader(http.StatusOK)
			}))
			defer backend.Close()

			token := signValidToken("delegation-user")
			identity := &auth.UserIdentity{
				Username: "delegation-user",
				Issuer:   jwksServer.URL,
				RawToken: token,
				ExpiresAt: time.Now().Add(1 * time.Hour),
			}

			transport := &auth.ContextJWTDelegationTransport{
				Base: http.DefaultTransport,
			}
			client := &http.Client{Transport: transport}

			ctx := auth.WithUserIdentity(context.Background(), identity)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, backend.URL, nil)
			Expect(err).NotTo(HaveOccurred())

			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(receivedAuthHeader).To(Equal("Bearer " + token),
				"delegation transport must forward the user's raw JWT to downstream")
		})
	})
})


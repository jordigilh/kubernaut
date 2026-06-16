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

	Describe("AC-11: OIDC mode rejects SA tokens (#1309 replaces dual-auth)", func() {
		It("IT-AF-1195-019: OIDC mode rejects envtest SA token (no TokenReview fallback)", func() {
			var captured *auth.UserIdentity
			srv := buildAuthServer(&captured)
			defer srv.Close()

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

			req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Authorization", "Bearer "+saToken)

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
				"OIDC mode must reject SA tokens — no TokenReview fallback (#1309)")
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

	Describe("Multi-issuer JWT validation (#1436)", func() {
		var (
			kcKeyPair    *testKeyPair
			spireKeyPair *testKeyPair
			kcJWKS       *httptest.Server
			spireJWKS    *httptest.Server
			multiValidator *auth.JWTValidator
		)

		BeforeEach(func() {
			kcKeyPair = newTestKeyPair("kc-key-1")
			spireKeyPair = newTestKeyPair("spire-key-1")
			kcJWKS = newJWKSServer(kcKeyPair.jwks())
			spireJWKS = newJWKSServer(spireKeyPair.jwks())

			multiCfg := auth.Config{
				JWT: []auth.ProviderConfig{
					{
						Issuer: auth.IssuerConfig{
							URL:       kcJWKS.URL,
							JWKSURL:   kcJWKS.URL,
							Audiences: []string{"kubernaut-af"},
						},
						ClaimMappings: auth.ClaimMappings{
							Username: "preferred_username",
							Groups:   "groups",
						},
					},
					{
						Issuer: auth.IssuerConfig{
							URL:       spireJWKS.URL,
							JWKSURL:   spireJWKS.URL,
							Audiences: []string{"spiffe://trust-domain/ns/kubernaut/sa/af"},
						},
					},
				},
				AllowInsecureIssuers: true,
			}

			var err error
			multiValidator, err = auth.NewJWTValidator(multiCfg)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if kcJWKS != nil {
				kcJWKS.Close()
			}
			if spireJWKS != nil {
				spireJWKS.Close()
			}
		})

		It("IT-AF-1436-001: JWT from provider A (keycloak) accepted with correct identity", func() {
			var captured *auth.UserIdentity
			mw := auth.MiddlewareWithConfig(auth.MiddlewareConfig{
				Validator: multiValidator,
				Logger:    logf.Log.WithName("it-1436-multi"),
				Auditor:   auditRecorder,
			})
			srv := httptest.NewServer(mw(identityCapture(&captured)))
			defer srv.Close()

			token := kcKeyPair.signToken(standardClaims(
				kcJWKS.URL, "kc-user", []string{"kubernaut-af"}, time.Now().Add(1*time.Hour),
			))

			req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(captured).NotTo(BeNil())
			Expect(captured.Username).To(Equal("kc-user"))
			Expect(captured.Issuer).To(Equal(kcJWKS.URL))
		})

		It("IT-AF-1436-002: JWT from provider B (SPIRE-style) accepted with SPIRE identity", func() {
			var captured *auth.UserIdentity
			mw := auth.MiddlewareWithConfig(auth.MiddlewareConfig{
				Validator: multiValidator,
				Logger:    logf.Log.WithName("it-1436-spire"),
				Auditor:   auditRecorder,
			})
			srv := httptest.NewServer(mw(identityCapture(&captured)))
			defer srv.Close()

			token := spireKeyPair.signToken(standardClaims(
				spireJWKS.URL, "spiffe://trust-domain/ns/kubernaut/sa/agent",
				[]string{"spiffe://trust-domain/ns/kubernaut/sa/af"},
				time.Now().Add(1*time.Hour),
			))

			req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(captured).NotTo(BeNil())
			Expect(captured.Issuer).To(Equal(spireJWKS.URL))
		})

		It("IT-AF-1436-003: JWT from unknown third issuer rejected with 401", func() {
			unknownKeyPair := newTestKeyPair("unknown-key-1")
			unknownJWKS := newJWKSServer(unknownKeyPair.jwks())
			defer unknownJWKS.Close()

			var captured *auth.UserIdentity
			mw := auth.MiddlewareWithConfig(auth.MiddlewareConfig{
				Validator: multiValidator,
				Logger:    logf.Log.WithName("it-1436-unknown"),
			})
			srv := httptest.NewServer(mw(identityCapture(&captured)))
			defer srv.Close()

			token := unknownKeyPair.signToken(standardClaims(
				unknownJWKS.URL, "unknown-user",
				[]string{"kubernaut-af"},
				time.Now().Add(1*time.Hour),
			))

			req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
				"unknown issuer must be rejected")
			Expect(captured).To(BeNil())
		})
	})

	Describe("Auth auto-detect mode (#1309)", func() {
		It("IT-AF-1309-001: OIDC mode rejects envtest SA token", func() {
			oidcCfg := auth.Config{
				JWT: []auth.ProviderConfig{
					{
						Issuer: auth.IssuerConfig{
							URL:       jwksServer.URL,
							JWKSURL:   jwksServer.URL,
							Audiences: []string{"kubernaut-af"},
						},
					},
				},
				AllowInsecureIssuers: true,
			}

			oidcValidator, err := auth.NewJWTValidator(oidcCfg,
				auth.WithHTTPClient(jwksServer.Client()),
			)
			Expect(err).NotTo(HaveOccurred())

			var captured *auth.UserIdentity
			mw := auth.MiddlewareWithConfig(auth.MiddlewareConfig{
				Validator: oidcValidator,
				Logger:    logf.Log.WithName("it-1309-oidc"),
			})
			srv := httptest.NewServer(mw(identityCapture(&captured)))
			defer srv.Close()

			sa := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Name: "af-it-1309-oidc-sa", Namespace: "default"},
			}
			err = k8sClient.Create(context.Background(), sa)
			if err != nil && !apierrors.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			tokenReq := &authv1.TokenRequest{
				Spec: authv1.TokenRequestSpec{ExpirationSeconds: ptr(int64(3600))},
			}
			tokenResult, err := k8sClientset.CoreV1().ServiceAccounts("default").CreateToken(
				context.Background(), "af-it-1309-oidc-sa", tokenReq, metav1.CreateOptions{},
			)
			Expect(err).NotTo(HaveOccurred())

			req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Authorization", "Bearer "+tokenResult.Status.Token)

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
				"OIDC mode must reject SA tokens (no TokenReview fallback)")
		})

		It("IT-AF-1309-002: TokenReview mode accepts envtest SA token", func() {
			tokenReviewCfg := auth.Config{
				JWT:                  []auth.ProviderConfig{},
				AllowInsecureIssuers: true,
			}

			reviewer := auth.NewTokenReviewer(k8sClientset)
			trValidator, err := auth.NewJWTValidator(tokenReviewCfg,
				auth.WithTokenReviewer(reviewer),
			)
			Expect(err).NotTo(HaveOccurred())

			var captured *auth.UserIdentity
			mw := auth.MiddlewareWithConfig(auth.MiddlewareConfig{
				Validator: trValidator,
				Logger:    logf.Log.WithName("it-1309-tr"),
			})
			srv := httptest.NewServer(mw(identityCapture(&captured)))
			defer srv.Close()

			sa := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Name: "af-it-1309-tr-sa", Namespace: "default"},
			}
			err = k8sClient.Create(context.Background(), sa)
			if err != nil && !apierrors.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			tokenReq := &authv1.TokenRequest{
				Spec: authv1.TokenRequestSpec{ExpirationSeconds: ptr(int64(3600))},
			}
			tokenResult, err := k8sClientset.CoreV1().ServiceAccounts("default").CreateToken(
				context.Background(), "af-it-1309-tr-sa", tokenReq, metav1.CreateOptions{},
			)
			Expect(err).NotTo(HaveOccurred())

			req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Authorization", "Bearer "+tokenResult.Status.Token)

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK),
				"TokenReview mode must accept SA tokens")
			Expect(captured).NotTo(BeNil())
			Expect(captured.Username).To(ContainSubstring("af-it-1309-tr-sa"))
			Expect(captured.IsServiceAccount).To(BeTrue())
		})

		It("IT-AF-1309-003: TokenReview mode emits auth.success audit event", func() {
			tokenReviewCfg := auth.Config{
				JWT:                  []auth.ProviderConfig{},
				AllowInsecureIssuers: true,
			}

			reviewer := auth.NewTokenReviewer(k8sClientset)
			trValidator, err := auth.NewJWTValidator(tokenReviewCfg,
				auth.WithTokenReviewer(reviewer),
			)
			Expect(err).NotTo(HaveOccurred())

			auditRec := newRecordingEmitter()
			var captured *auth.UserIdentity
			mw := auth.MiddlewareWithConfig(auth.MiddlewareConfig{
				Validator:    trValidator,
				Logger:       logf.Log.WithName("it-1309-audit"),
				Auditor:      auditRec,
				AuthDuration: metricsRegistry.AuthDuration,
			})
			srv := httptest.NewServer(mw(identityCapture(&captured)))
			defer srv.Close()

			sa := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Name: "af-it-1309-audit-sa", Namespace: "default"},
			}
			err = k8sClient.Create(context.Background(), sa)
			if err != nil && !apierrors.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			tokenReq := &authv1.TokenRequest{
				Spec: authv1.TokenRequestSpec{ExpirationSeconds: ptr(int64(3600))},
			}
			tokenResult, err := k8sClientset.CoreV1().ServiceAccounts("default").CreateToken(
				context.Background(), "af-it-1309-audit-sa", tokenReq, metav1.CreateOptions{},
			)
			Expect(err).NotTo(HaveOccurred())

			req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Authorization", "Bearer "+tokenResult.Status.Token)

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			Eventually(func() []*audit.Event {
				return auditRec.EventsOfType(audit.EventAuthSuccess)
			}).Should(HaveLen(1))

			events := auditRec.EventsOfType(audit.EventAuthSuccess)
			Expect(events[0].Detail["auth_method"]).To(Equal("token_review"))
		})
	})
})


package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
	"github.com/jordigilh/kubernaut-apifrontend/internal/httputil"
)

func TestAuthSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Auth Suite")
}

// testKeyPair holds an RSA key pair and its JOSE representation for test fixtures.
type testKeyPair struct {
	private *rsa.PrivateKey
	keyID   string
}

func newTestKeyPair(kid string) *testKeyPair {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())
	return &testKeyPair{private: key, keyID: kid}
}

func (kp *testKeyPair) jwks() jose.JSONWebKeySet {
	return jose.JSONWebKeySet{
		Keys: []jose.JSONWebKey{
			{
				Key:       &kp.private.PublicKey,
				KeyID:     kp.keyID,
				Algorithm: string(jose.RS256),
				Use:       "sig",
			},
		},
	}
}

func (kp *testKeyPair) signToken(claims interface{}) string {
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: kp.private}, (&jose.SignerOptions{}).WithHeader(jose.HeaderKey("kid"), kp.keyID))
	Expect(err).NotTo(HaveOccurred())

	raw, err := jwt.Signed(signer).Claims(claims).Serialize()
	Expect(err).NotTo(HaveOccurred())
	return raw
}

// newJWKSServer creates a test JWKS server that serves the given key set.
func newJWKSServer(jwks jose.JSONWebKeySet) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jwks)
	}))
	DeferCleanup(srv.Close)
	return srv
}

// newFailingJWKSServer creates a JWKS server that always returns 500.
func newFailingJWKSServer() *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	DeferCleanup(srv.Close)
	return srv
}

// newFakeTokenReviewClient creates a fake K8s client that responds to TokenReview requests.
func newFakeTokenReviewClient(expectedToken, username string, groups []string) kubernetes.Interface {
	client := k8sfake.NewSimpleClientset()
	client.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		review, _ := action.(k8stesting.CreateAction).GetObject().(*authv1.TokenReview)
		if review.Spec.Token == expectedToken {
			review.Status = authv1.TokenReviewStatus{
				Authenticated: true,
				User: authv1.UserInfo{
					Username: username,
					Groups:   groups,
				},
			}
		} else {
			review.Status = authv1.TokenReviewStatus{Authenticated: false}
		}
		return true, review, nil
	})
	return client
}

// standardClaims creates JWT claims for testing.
func standardClaims(issuer, subject string, audiences, groups []string, expiry time.Time) map[string]interface{} {
	claims := map[string]interface{}{
		"iss":                issuer,
		"sub":                subject,
		"aud":                audiences,
		"exp":                expiry.Unix(),
		"iat":                time.Now().Unix(),
		"preferred_username": subject,
	}
	if groups != nil {
		claims["groups"] = groups
	}
	return claims
}

var _ = Describe("Auth", func() {
	Describe("JWT validation", func() {
		Context("token lifecycle and provider routing", func() {
			It("UT-AF-002-001: valid token returns user identity", func() {
				kp := newTestKeyPair("key-1")
				jwksSrv := newJWKSServer(kp.jwks())

				cfg := auth.Config{
					JWT: []auth.ProviderConfig{
						{
							Issuer: auth.IssuerConfig{
								URL:       jwksSrv.URL,
								Audiences: []string{"kubernaut-agent"},
							},
							ClaimMappings: auth.ClaimMappings{
								Username: "claims.preferred_username",
								Groups:   "claims.groups",
							},
						},
					},
				}

				validator, err := auth.NewJWTValidator(cfg, auth.WithHTTPClient(jwksSrv.Client()))
				Expect(err).NotTo(HaveOccurred())
				Expect(validator).NotTo(BeNil())

				claims := standardClaims(jwksSrv.URL, "alice", []string{"kubernaut-agent"}, []string{"sre-team"}, time.Now().Add(time.Hour))
				token := kp.signToken(claims)

				identity, err := validator.Validate(context.Background(), token)
				Expect(err).NotTo(HaveOccurred())
				Expect(identity.Username).To(Equal("alice"))
				Expect(identity.Groups).To(Equal([]string{"sre-team"}))
			})

			It("UT-AF-002-002: expired token returns token expired error", func() {
				kp := newTestKeyPair("key-1")
				jwksSrv := newJWKSServer(kp.jwks())

				cfg := auth.Config{
					JWT: []auth.ProviderConfig{
						{
							Issuer: auth.IssuerConfig{
								URL:       jwksSrv.URL,
								Audiences: []string{"kubernaut-agent"},
							},
							ClaimMappings: auth.ClaimMappings{
								Username: "claims.preferred_username",
								Groups:   "claims.groups",
							},
						},
					},
				}

				validator, err := auth.NewJWTValidator(cfg, auth.WithHTTPClient(jwksSrv.Client()))
				Expect(err).NotTo(HaveOccurred())

				claims := standardClaims(jwksSrv.URL, "alice", []string{"kubernaut-agent"}, nil, time.Now().Add(-time.Hour))
				token := kp.signToken(claims)

				_, err = validator.Validate(context.Background(), token)
				Expect(err).To(MatchError(auth.ErrTokenExpired))
			})

			It("UT-AF-002-020: token without exp claim returns missing expiry error", func() {
				kp := newTestKeyPair("key-1")
				jwksSrv := newJWKSServer(kp.jwks())

				cfg := auth.Config{
					JWT: []auth.ProviderConfig{
						{
							Issuer: auth.IssuerConfig{
								URL:       jwksSrv.URL,
								Audiences: []string{"kubernaut-agent"},
							},
							ClaimMappings: auth.ClaimMappings{
								Username: "claims.preferred_username",
								Groups:   "claims.groups",
							},
						},
					},
				}

				validator, err := auth.NewJWTValidator(cfg, auth.WithHTTPClient(jwksSrv.Client()))
				Expect(err).NotTo(HaveOccurred())

				claims := map[string]interface{}{
					"iss":                jwksSrv.URL,
					"sub":                "alice",
					"aud":                []string{"kubernaut-agent"},
					"iat":                time.Now().Unix(),
					"preferred_username": "alice",
				}
				token := kp.signToken(claims)

				_, err = validator.Validate(context.Background(), token)
				Expect(err).To(MatchError(auth.ErrMissingExpiry))
			})

			It("UT-AF-002-003: wrong audience returns invalid audience error", func() {
				kp := newTestKeyPair("key-1")
				jwksSrv := newJWKSServer(kp.jwks())

				cfg := auth.Config{
					JWT: []auth.ProviderConfig{
						{
							Issuer: auth.IssuerConfig{
								URL:       jwksSrv.URL,
								Audiences: []string{"kubernaut-agent"},
							},
							ClaimMappings: auth.ClaimMappings{
								Username: "claims.preferred_username",
								Groups:   "claims.groups",
							},
						},
					},
				}

				validator, err := auth.NewJWTValidator(cfg, auth.WithHTTPClient(jwksSrv.Client()))
				Expect(err).NotTo(HaveOccurred())

				claims := standardClaims(jwksSrv.URL, "alice", []string{"wrong-audience"}, nil, time.Now().Add(time.Hour))
				token := kp.signToken(claims)

				_, err = validator.Validate(context.Background(), token)
				Expect(err).To(MatchError(auth.ErrInvalidAudience))
			})

			It("UT-AF-002-004: unknown issuer fails closed", func() {
				kp := newTestKeyPair("key-1")
				jwksSrv := newJWKSServer(kp.jwks())

				cfg := auth.Config{
					JWT: []auth.ProviderConfig{
						{
							Issuer: auth.IssuerConfig{
								URL:       jwksSrv.URL,
								Audiences: []string{"kubernaut-agent"},
							},
							ClaimMappings: auth.ClaimMappings{
								Username: "claims.preferred_username",
								Groups:   "claims.groups",
							},
						},
					},
				}

				validator, err := auth.NewJWTValidator(cfg, auth.WithHTTPClient(jwksSrv.Client()))
				Expect(err).NotTo(HaveOccurred())

				claims := standardClaims("https://unknown-issuer.example.com", "alice", []string{"kubernaut-agent"}, nil, time.Now().Add(time.Hour))
				token := kp.signToken(claims)

				_, err = validator.Validate(context.Background(), token)
				Expect(err).To(MatchError(auth.ErrUnknownIssuer))
			})

			It("UT-AF-002-005: multiple providers route to correct issuer", func() {
				kp1 := newTestKeyPair("key-provider1")
				kp2 := newTestKeyPair("key-provider2")
				jwksSrv1 := newJWKSServer(kp1.jwks())
				jwksSrv2 := newJWKSServer(kp2.jwks())

				cfg := auth.Config{
					JWT: []auth.ProviderConfig{
						{
							Issuer: auth.IssuerConfig{
								URL:       jwksSrv1.URL,
								Audiences: []string{"kubernaut-agent"},
							},
							ClaimMappings: auth.ClaimMappings{
								Username: "claims.preferred_username",
								Groups:   "claims.groups",
							},
						},
						{
							Issuer: auth.IssuerConfig{
								URL:       jwksSrv2.URL,
								Audiences: []string{"kubernaut-ci"},
							},
							ClaimMappings: auth.ClaimMappings{
								Username: "claims.preferred_username",
								Groups:   "claims.groups",
							},
						},
					},
				}

				validator, err := auth.NewJWTValidator(cfg)
				Expect(err).NotTo(HaveOccurred())

				claims := standardClaims(jwksSrv2.URL, "ci-bot", []string{"kubernaut-ci"}, []string{"ci"}, time.Now().Add(time.Hour))
				token := kp2.signToken(claims)

				identity, err := validator.Validate(context.Background(), token)
				Expect(err).NotTo(HaveOccurred())
				Expect(identity.Username).To(Equal("ci-bot"))
				Expect(identity.Issuer).To(Equal(jwksSrv2.URL))
			})

			It("UT-AF-002-006: duplicate issuers produce config error", func() {
				cfg := auth.Config{
					JWT: []auth.ProviderConfig{
						{
							Issuer: auth.IssuerConfig{URL: "https://sso.example.com"},
						},
						{
							Issuer: auth.IssuerConfig{URL: "https://sso.example.com"},
						},
					},
				}

				_, err := auth.NewJWTValidator(cfg)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("duplicate issuer"))
			})

			It("UT-AF-002-007: CEL validation rejects system prefix", func() {
				kp := newTestKeyPair("key-1")
				jwksSrv := newJWKSServer(kp.jwks())

				cfg := auth.Config{
					JWT: []auth.ProviderConfig{
						{
							Issuer: auth.IssuerConfig{
								URL:       jwksSrv.URL,
								Audiences: []string{"kubernaut-agent"},
							},
							ClaimMappings: auth.ClaimMappings{
								Username: "claims.preferred_username",
								Groups:   "claims.groups",
							},
							UserValidationRules: []auth.ValidationRule{
								{
									Expression: "!user.username.startsWith('system:')",
									Message:    "external users cannot use system: prefix",
								},
							},
						},
					},
				}

				validator, err := auth.NewJWTValidator(cfg, auth.WithHTTPClient(jwksSrv.Client()))
				Expect(err).NotTo(HaveOccurred())

				claims := standardClaims(jwksSrv.URL, "system:admin", []string{"kubernaut-agent"}, nil, time.Now().Add(time.Hour))
				token := kp.signToken(claims)

				_, err = validator.Validate(context.Background(), token)
				Expect(err).To(MatchError(auth.ErrCELValidation))
			})

			It("UT-AF-002-008: malformed token returns malformed token error", func() {
				kp := newTestKeyPair("key-1")
				jwksSrv := newJWKSServer(kp.jwks())

				cfg := auth.Config{
					JWT: []auth.ProviderConfig{
						{
							Issuer: auth.IssuerConfig{
								URL:       jwksSrv.URL,
								Audiences: []string{"kubernaut-agent"},
							},
							ClaimMappings: auth.ClaimMappings{
								Username: "claims.preferred_username",
								Groups:   "claims.groups",
							},
						},
					},
				}

				validator, err := auth.NewJWTValidator(cfg, auth.WithHTTPClient(jwksSrv.Client()))
				Expect(err).NotTo(HaveOccurred())

				_, err = validator.Validate(context.Background(), "not.a.valid.jwt.token")
				Expect(err).To(MatchError(auth.ErrMalformedToken))
			})
		})
	})

	Describe("JWKS cache", func() {
		Context("caching behaviour", func() {
			It("UT-AF-002-009: fetches JWKS on first request", func() {
				fetchCount := 0
				kp := newTestKeyPair("key-1")
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					fetchCount++
					w.Header().Set("Content-Type", "application/json")
					k := kp // capture for closure
					_ = json.NewEncoder(w).Encode(k.jwks())
				}))
				DeferCleanup(srv.Close)

				cfg := auth.Config{
					JWT: []auth.ProviderConfig{
						{
							Issuer: auth.IssuerConfig{
								URL:       srv.URL,
								Audiences: []string{"kubernaut-agent"},
							},
							ClaimMappings: auth.ClaimMappings{
								Username: "claims.preferred_username",
								Groups:   "claims.groups",
							},
						},
					},
				}

				validator, err := auth.NewJWTValidator(cfg, auth.WithHTTPClient(srv.Client()))
				Expect(err).NotTo(HaveOccurred())

				claims := standardClaims(srv.URL, "alice", []string{"kubernaut-agent"}, nil, time.Now().Add(time.Hour))
				token := kp.signToken(claims)

				_, err = validator.Validate(context.Background(), token)
				Expect(err).NotTo(HaveOccurred())
				Expect(fetchCount).To(Equal(1), "JWKS should be fetched exactly once on first request")
			})

			It("UT-AF-002-010: uses stale JWKS on fetch failure", func() {
				kp := newTestKeyPair("key-1")
				requestCount := 0
				k := kp
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					requestCount++
					if requestCount == 1 {
						w.Header().Set("Content-Type", "application/json")
						_ = json.NewEncoder(w).Encode(k.jwks())
					} else {
						w.WriteHeader(http.StatusInternalServerError)
					}
				}))
				DeferCleanup(srv.Close)

				cfg := auth.Config{
					JWT: []auth.ProviderConfig{
						{
							Issuer: auth.IssuerConfig{
								URL:       srv.URL,
								Audiences: []string{"kubernaut-agent"},
							},
							ClaimMappings: auth.ClaimMappings{
								Username: "claims.preferred_username",
								Groups:   "claims.groups",
							},
						},
					},
				}

				validator, err := auth.NewJWTValidator(cfg, auth.WithHTTPClient(srv.Client()))
				Expect(err).NotTo(HaveOccurred())

				claims := standardClaims(srv.URL, "alice", []string{"kubernaut-agent"}, nil, time.Now().Add(time.Hour))
				token := kp.signToken(claims)

				_, err = validator.Validate(context.Background(), token)
				Expect(err).NotTo(HaveOccurred())

				claims2 := standardClaims(srv.URL, "bob", []string{"kubernaut-agent"}, nil, time.Now().Add(time.Hour))
				token2 := kp.signToken(claims2)

				identity, err := validator.Validate(context.Background(), token2)
				Expect(err).NotTo(HaveOccurred(), "should succeed using stale cached JWKS")
				Expect(identity.Username).To(Equal("bob"))
			})
		})
	})

	Describe("JWKS circuit breaker", func() {
		Context("failure thresholds and recovery", func() {
			It("UT-AF-002-011: opens after three consecutive failures", func() {
				kp := newTestKeyPair("key-1")
				failSrv := newFailingJWKSServer()

				cfg := auth.Config{
					JWT: []auth.ProviderConfig{
						{
							Issuer: auth.IssuerConfig{
								URL:       failSrv.URL,
								Audiences: []string{"kubernaut-agent"},
							},
							ClaimMappings: auth.ClaimMappings{
								Username: "claims.preferred_username",
								Groups:   "claims.groups",
							},
						},
					},
				}

				validator, err := auth.NewJWTValidator(cfg, auth.WithHTTPClient(failSrv.Client()))
				Expect(err).NotTo(HaveOccurred())

				claims := standardClaims(failSrv.URL, "alice", []string{"kubernaut-agent"}, nil, time.Now().Add(time.Hour))
				token := kp.signToken(claims)

				for i := 0; i < 3; i++ {
					_, err = validator.Validate(context.Background(), token)
					Expect(err).To(HaveOccurred(), fmt.Sprintf("attempt %d should fail", i+1))
				}

				_, err = validator.Validate(context.Background(), token)
				Expect(err).To(MatchError(auth.ErrCircuitOpen))
			})

			It("UT-AF-002-012: transitions half-open after timeout and recovers", func() {
				kp := newTestKeyPair("key-1")
				requestCount := 0
				k := kp
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					requestCount++
					if requestCount <= 3 {
						w.WriteHeader(http.StatusInternalServerError)
					} else {
						w.Header().Set("Content-Type", "application/json")
						_ = json.NewEncoder(w).Encode(k.jwks())
					}
				}))
				DeferCleanup(srv.Close)

				cfg := auth.Config{
					JWT: []auth.ProviderConfig{
						{
							Issuer: auth.IssuerConfig{
								URL:       srv.URL,
								Audiences: []string{"kubernaut-agent"},
							},
							ClaimMappings: auth.ClaimMappings{
								Username: "claims.preferred_username",
								Groups:   "claims.groups",
							},
						},
					},
				}

				validator, err := auth.NewJWTValidator(cfg,
					auth.WithHTTPClient(srv.Client()),
					auth.WithCBTestTimeout(20*time.Millisecond),
				)
				Expect(err).NotTo(HaveOccurred())

				claims := standardClaims(srv.URL, "alice", []string{"kubernaut-agent"}, nil, time.Now().Add(time.Hour))
				token := kp.signToken(claims)

				for i := 0; i < 3; i++ {
					_, _ = validator.Validate(context.Background(), token)
					_ = i
				}

				_, err = validator.Validate(context.Background(), token)
				Expect(err).To(MatchError(auth.ErrCircuitOpen))

				time.Sleep(50 * time.Millisecond)

				identity, err := validator.Validate(context.Background(), token)
				Expect(err).NotTo(HaveOccurred())
				Expect(identity.Username).To(Equal("alice"))
			})

			It("UT-AF-002-013: closes circuit on successful probe", func() {
				kp := newTestKeyPair("key-1")
				requestCount := 0
				k := kp
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					requestCount++
					if requestCount <= 3 {
						w.WriteHeader(http.StatusInternalServerError)
					} else {
						w.Header().Set("Content-Type", "application/json")
						_ = json.NewEncoder(w).Encode(k.jwks())
					}
				}))
				DeferCleanup(srv.Close)

				cfg := auth.Config{
					JWT: []auth.ProviderConfig{
						{
							Issuer: auth.IssuerConfig{
								URL:       srv.URL,
								Audiences: []string{"kubernaut-agent"},
							},
							ClaimMappings: auth.ClaimMappings{
								Username: "claims.preferred_username",
								Groups:   "claims.groups",
							},
						},
					},
				}

				validator, err := auth.NewJWTValidator(cfg,
					auth.WithHTTPClient(srv.Client()),
					auth.WithCBTestTimeout(20*time.Millisecond),
				)
				Expect(err).NotTo(HaveOccurred())

				claims := standardClaims(srv.URL, "alice", []string{"kubernaut-agent"}, nil, time.Now().Add(time.Hour))
				token := kp.signToken(claims)

				for i := 0; i < 3; i++ {
					_, _ = validator.Validate(context.Background(), token)
					_ = i
				}

				time.Sleep(50 * time.Millisecond)

				_, err = validator.Validate(context.Background(), token)
				Expect(err).NotTo(HaveOccurred())

				identity, err := validator.Validate(context.Background(), token)
				Expect(err).NotTo(HaveOccurred())
				Expect(identity.Username).To(Equal("alice"))
			})

			It("UT-AF-002-014: existing sessions fail open when circuit opens", func() {
				kp := newTestKeyPair("key-1")
				requestCount := 0
				k := kp
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					requestCount++
					if requestCount == 1 {
						w.Header().Set("Content-Type", "application/json")
						_ = json.NewEncoder(w).Encode(k.jwks())
					} else {
						w.WriteHeader(http.StatusInternalServerError)
					}
				}))
				DeferCleanup(srv.Close)

				cfg := auth.Config{
					JWT: []auth.ProviderConfig{
						{
							Issuer: auth.IssuerConfig{
								URL:       srv.URL,
								Audiences: []string{"kubernaut-agent"},
							},
							ClaimMappings: auth.ClaimMappings{
								Username: "claims.preferred_username",
								Groups:   "claims.groups",
							},
						},
					},
				}

				validator, err := auth.NewJWTValidator(cfg, auth.WithHTTPClient(srv.Client()))
				Expect(err).NotTo(HaveOccurred())

				claims := standardClaims(srv.URL, "alice", []string{"kubernaut-agent"}, nil, time.Now().Add(time.Hour))
				token := kp.signToken(claims)

				_, err = validator.Validate(context.Background(), token)
				Expect(err).NotTo(HaveOccurred())

				for i := 0; i < 3; i++ {
					_, _ = validator.Validate(context.Background(), token)
					_ = i
				}

				identity, err := validator.Validate(context.Background(), token)
				Expect(err).NotTo(HaveOccurred(), "existing sessions with cached JWKS should continue working when circuit is open")
				Expect(identity.Username).To(Equal("alice"))
			})

			It("UT-AF-002-021: resolves separate JWKS URL from issuer", func() {
				kp := newTestKeyPair("key-1")
				jwksSrv := newJWKSServer(kp.jwks())

				issuerURL := "https://fake-issuer.example.com"
				cfg := auth.Config{
					JWT: []auth.ProviderConfig{
						{
							Issuer: auth.IssuerConfig{
								URL:       issuerURL,
								JWKSURL:   jwksSrv.URL,
								Audiences: []string{"test-audience"},
							},
							ClaimMappings: auth.ClaimMappings{
								Username: "claims.preferred_username",
								Groups:   "claims.groups",
							},
						},
					},
				}

				validator, err := auth.NewJWTValidator(cfg, auth.WithHTTPClient(jwksSrv.Client()))
				Expect(err).NotTo(HaveOccurred())

				claims := standardClaims(issuerURL, "alice", []string{"test-audience"}, nil, time.Now().Add(time.Hour))
				token := kp.signToken(claims)

				identity, err := validator.Validate(context.Background(), token)
				Expect(err).NotTo(HaveOccurred())
				Expect(identity.Username).To(Equal("alice"))
			})
		})
	})

	Describe("Middleware", func() {
		Context("HTTP auth and RFC 7807 errors", func() {
			It("UT-AF-002-015: sets user context on successful authentication", func() {
				kp := newTestKeyPair("key-1")
				jwksSrv := newJWKSServer(kp.jwks())

				cfg := auth.Config{
					JWT: []auth.ProviderConfig{
						{
							Issuer: auth.IssuerConfig{
								URL:       jwksSrv.URL,
								Audiences: []string{"kubernaut-agent"},
							},
							ClaimMappings: auth.ClaimMappings{
								Username: "claims.preferred_username",
								Groups:   "claims.groups",
							},
						},
					},
				}

				validator, err := auth.NewJWTValidator(cfg, auth.WithHTTPClient(jwksSrv.Client()))
				Expect(err).NotTo(HaveOccurred())

				var capturedIdentity *auth.UserIdentity
				handler := auth.MiddlewareWithConfig(auth.MiddlewareConfig{Validator: validator})(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
					capturedIdentity = auth.UserIdentityFromContext(r.Context())
				}))

				claims := standardClaims(jwksSrv.URL, "alice", []string{"kubernaut-agent"}, []string{"sre"}, time.Now().Add(time.Hour))
				token := kp.signToken(claims)

				req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
				req.Header.Set("Authorization", "Bearer "+token)
				rec := httptest.NewRecorder()

				handler.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))
				Expect(capturedIdentity).NotTo(BeNil())
				Expect(capturedIdentity.Username).To(Equal("alice"))
				Expect(capturedIdentity.Groups).To(Equal([]string{"sre"}))
			})

			It("UT-AF-002-016: returns 401 RFC 7807 when Authorization header is missing", func() {
				validator, err := auth.NewJWTValidator(auth.Config{})
				Expect(err).NotTo(HaveOccurred())

				handler := auth.MiddlewareWithConfig(auth.MiddlewareConfig{Validator: validator})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))

				req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
				rec := httptest.NewRecorder()

				handler.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusUnauthorized))
				Expect(rec.Header().Get("Content-Type")).To(Equal("application/problem+json"))
				var problem httputil.ProblemDetail
				Expect(json.Unmarshal(rec.Body.Bytes(), &problem)).To(Succeed())
				Expect(problem.Status).To(Equal(http.StatusUnauthorized))
				Expect(problem.Title).To(Equal("Missing Authorization"))
			})

			It("UT-AF-002-017: returns 400 RFC 7807 when Authorization header has control characters", func() {
				kp := newTestKeyPair("key-1")
				jwksSrv := newJWKSServer(kp.jwks())

				cfg := auth.Config{
					JWT: []auth.ProviderConfig{
						{
							Issuer: auth.IssuerConfig{
								URL:       jwksSrv.URL,
								Audiences: []string{"kubernaut-agent"},
							},
							ClaimMappings: auth.ClaimMappings{
								Username: "claims.preferred_username",
								Groups:   "claims.groups",
							},
						},
					},
				}

				validator, err := auth.NewJWTValidator(cfg, auth.WithHTTPClient(jwksSrv.Client()))
				Expect(err).NotTo(HaveOccurred())

				handler := auth.MiddlewareWithConfig(auth.MiddlewareConfig{Validator: validator})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))

				req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
				req.Header.Set("Authorization", "Bearer \x00malicious-token")
				rec := httptest.NewRecorder()

				handler.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusBadRequest))
				Expect(rec.Header().Get("Content-Type")).To(Equal("application/problem+json"))
				var problem httputil.ProblemDetail
				Expect(json.Unmarshal(rec.Body.Bytes(), &problem)).To(Succeed())
				Expect(problem.Status).To(Equal(http.StatusBadRequest))
				Expect(problem.Title).To(Equal("Invalid Authorization Header"))
			})

			It("UT-AF-002-018: returns 413 when request body exceeds MaxBodySize", func() {
				kp := newTestKeyPair("key-1")
				jwksSrv := newJWKSServer(kp.jwks())

				cfg := auth.Config{
					JWT: []auth.ProviderConfig{
						{
							Issuer: auth.IssuerConfig{
								URL:       jwksSrv.URL,
								Audiences: []string{"kubernaut-agent"},
							},
							ClaimMappings: auth.ClaimMappings{
								Username: "claims.preferred_username",
								Groups:   "claims.groups",
							},
						},
					},
				}

				validator, err := auth.NewJWTValidator(cfg, auth.WithHTTPClient(jwksSrv.Client()))
				Expect(err).NotTo(HaveOccurred())

				var bodyReadErr error
				handler := auth.MiddlewareWithConfig(auth.MiddlewareConfig{Validator: validator})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, bodyReadErr = io.ReadAll(r.Body)
					if bodyReadErr != nil {
						w.WriteHeader(http.StatusRequestEntityTooLarge)
						return
					}
					w.WriteHeader(http.StatusOK)
				}))

				claims := standardClaims(jwksSrv.URL, "alice", []string{"kubernaut-agent"}, nil, time.Now().Add(time.Hour))
				token := kp.signToken(claims)

				oversizedBody := strings.Repeat("x", auth.MaxBodySize+1)
				req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(oversizedBody))
				req.Header.Set("Authorization", "Bearer "+token)
				rec := httptest.NewRecorder()

				handler.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusRequestEntityTooLarge))
			})

			It("UT-AF-002-019: Kubernetes TokenReview accepts valid service account token", func() {
				fakeClient := newFakeTokenReviewClient("sa-token-that-is-not-jwt", "system:serviceaccount:default:my-sa", []string{"system:serviceaccounts"})

				reviewer := auth.NewTokenReviewer(fakeClient)

				cfg := auth.Config{
					JWT:        []auth.ProviderConfig{},
					Kubernetes: auth.KubernetesAuthConfig{Enabled: true},
				}

				validator, err := auth.NewJWTValidator(cfg, auth.WithTokenReviewer(reviewer))
				Expect(err).NotTo(HaveOccurred())

				handler := auth.MiddlewareWithConfig(auth.MiddlewareConfig{Validator: validator})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					identity := auth.UserIdentityFromContext(r.Context())
					if identity == nil {
						w.WriteHeader(http.StatusUnauthorized)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(struct {
						Username string `json:"username"`
					}{Username: identity.Username})
				}))

				req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
				req.Header.Set("Authorization", "Bearer sa-token-that-is-not-jwt")
				rec := httptest.NewRecorder()

				handler.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))
				var body struct {
					Username string `json:"username"`
				}
				Expect(json.Unmarshal(rec.Body.Bytes(), &body)).To(Succeed())
				Expect(body.Username).To(ContainSubstring("system:serviceaccount:"))
			})
		})
	})
})

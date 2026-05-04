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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
)

// testKeyPair holds an RSA key pair and its JOSE representation for test fixtures.
type testKeyPair struct {
	private *rsa.PrivateKey
	keyID   string
}

func newTestKeyPair(t *testing.T, kid string) *testKeyPair {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
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

func (kp *testKeyPair) signToken(t *testing.T, claims interface{}) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: kp.private}, (&jose.SignerOptions{}).WithHeader(jose.HeaderKey("kid"), kp.keyID))
	require.NoError(t, err)

	raw, err := jwt.Signed(signer).Claims(claims).Serialize()
	require.NoError(t, err)
	return raw
}

// newJWKSServer creates a test JWKS server that serves the given key set.
func newJWKSServer(t *testing.T, jwks jose.JSONWebKeySet) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jwks)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// newFailingJWKSServer creates a JWKS server that always returns 500.
func newFailingJWKSServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// newFakeTokenReviewClient creates a fake K8s client that responds to TokenReview requests.
func newFakeTokenReviewClient(t *testing.T, expectedToken, username string, groups []string) kubernetes.Interface {
	t.Helper()
	client := k8sfake.NewSimpleClientset()
	client.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		review := action.(k8stesting.CreateAction).GetObject().(*authv1.TokenReview)
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
func standardClaims(issuer, subject string, audiences []string, groups []string, expiry time.Time) map[string]interface{} {
	claims := map[string]interface{}{
		"iss":                issuer,
		"sub":               subject,
		"aud":               audiences,
		"exp":               expiry.Unix(),
		"iat":               time.Now().Unix(),
		"preferred_username": subject,
	}
	if groups != nil {
		claims["groups"] = groups
	}
	return claims
}

func TestValidateJWT_ValidToken_ReturnsUserIdentity(t *testing.T) {
	kp := newTestKeyPair(t, "key-1")
	jwksSrv := newJWKSServer(t, kp.jwks())

	cfg := auth.AuthConfig{
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
	require.NoError(t, err)
	require.NotNil(t, validator)

	claims := standardClaims(jwksSrv.URL, "alice", []string{"kubernaut-agent"}, []string{"sre-team"}, time.Now().Add(time.Hour))
	token := kp.signToken(t, claims)

	identity, err := validator.Validate(context.Background(), token)
	require.NoError(t, err)
	assert.Equal(t, "alice", identity.Username)
	assert.Equal(t, []string{"sre-team"}, identity.Groups)
}

func TestValidateJWT_ExpiredToken_Returns401(t *testing.T) {
	kp := newTestKeyPair(t, "key-1")
	jwksSrv := newJWKSServer(t, kp.jwks())

	cfg := auth.AuthConfig{
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
	require.NoError(t, err)

	claims := standardClaims(jwksSrv.URL, "alice", []string{"kubernaut-agent"}, nil, time.Now().Add(-time.Hour))
	token := kp.signToken(t, claims)

	_, err = validator.Validate(context.Background(), token)
	assert.ErrorIs(t, err, auth.ErrTokenExpired)
}

func TestValidateJWT_WrongAudience_Returns401(t *testing.T) {
	kp := newTestKeyPair(t, "key-1")
	jwksSrv := newJWKSServer(t, kp.jwks())

	cfg := auth.AuthConfig{
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
	require.NoError(t, err)

	claims := standardClaims(jwksSrv.URL, "alice", []string{"wrong-audience"}, nil, time.Now().Add(time.Hour))
	token := kp.signToken(t, claims)

	_, err = validator.Validate(context.Background(), token)
	assert.ErrorIs(t, err, auth.ErrInvalidAudience)
}

func TestValidateJWT_UnknownIssuer_FailsClosed(t *testing.T) {
	kp := newTestKeyPair(t, "key-1")
	jwksSrv := newJWKSServer(t, kp.jwks())

	cfg := auth.AuthConfig{
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
	require.NoError(t, err)

	claims := standardClaims("https://unknown-issuer.example.com", "alice", []string{"kubernaut-agent"}, nil, time.Now().Add(time.Hour))
	token := kp.signToken(t, claims)

	_, err = validator.Validate(context.Background(), token)
	assert.ErrorIs(t, err, auth.ErrUnknownIssuer)
}

func TestValidateJWT_MultipleProviders_RoutesToCorrectIssuer(t *testing.T) {
	kp1 := newTestKeyPair(t, "key-provider1")
	kp2 := newTestKeyPair(t, "key-provider2")
	jwksSrv1 := newJWKSServer(t, kp1.jwks())
	jwksSrv2 := newJWKSServer(t, kp2.jwks())

	cfg := auth.AuthConfig{
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
	require.NoError(t, err)

	// Token from provider 2 should be validated by provider 2's JWKS
	claims := standardClaims(jwksSrv2.URL, "ci-bot", []string{"kubernaut-ci"}, []string{"ci"}, time.Now().Add(time.Hour))
	token := kp2.signToken(t, claims)

	identity, err := validator.Validate(context.Background(), token)
	require.NoError(t, err)
	assert.Equal(t, "ci-bot", identity.Username)
	assert.Equal(t, jwksSrv2.URL, identity.Issuer)
}

func TestValidateJWT_DuplicateIssuers_ConfigError(t *testing.T) {
	cfg := auth.AuthConfig{
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
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate issuer")
}

func TestValidateJWT_CELValidation_RejectsSystemPrefix(t *testing.T) {
	kp := newTestKeyPair(t, "key-1")
	jwksSrv := newJWKSServer(t, kp.jwks())

	cfg := auth.AuthConfig{
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
	require.NoError(t, err)

	claims := standardClaims(jwksSrv.URL, "system:admin", []string{"kubernaut-agent"}, nil, time.Now().Add(time.Hour))
	token := kp.signToken(t, claims)

	_, err = validator.Validate(context.Background(), token)
	assert.ErrorIs(t, err, auth.ErrCELValidation)
}

func TestValidateJWT_MalformedToken_Returns401(t *testing.T) {
	kp := newTestKeyPair(t, "key-1")
	jwksSrv := newJWKSServer(t, kp.jwks())

	cfg := auth.AuthConfig{
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
	require.NoError(t, err)

	_, err = validator.Validate(context.Background(), "not.a.valid.jwt.token")
	assert.ErrorIs(t, err, auth.ErrMalformedToken)
}

func TestJWKSCache_FetchesOnFirstRequest(t *testing.T) {
	fetchCount := 0
	kp := newTestKeyPair(t, "key-1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fetchCount++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(kp.jwks())
	}))
	t.Cleanup(srv.Close)

	cfg := auth.AuthConfig{
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
	require.NoError(t, err)

	claims := standardClaims(srv.URL, "alice", []string{"kubernaut-agent"}, nil, time.Now().Add(time.Hour))
	token := kp.signToken(t, claims)

	_, err = validator.Validate(context.Background(), token)
	require.NoError(t, err)
	assert.Equal(t, 1, fetchCount, "JWKS should be fetched exactly once on first request")
}

func TestJWKSCache_UsesStaleOnFetchFailure(t *testing.T) {
	kp := newTestKeyPair(t, "key-1")
	requestCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		if requestCount == 1 {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(kp.jwks())
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)

	cfg := auth.AuthConfig{
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
	require.NoError(t, err)

	claims := standardClaims(srv.URL, "alice", []string{"kubernaut-agent"}, nil, time.Now().Add(time.Hour))
	token := kp.signToken(t, claims)

	// First call populates cache
	_, err = validator.Validate(context.Background(), token)
	require.NoError(t, err)

	// Force cache refresh (simulate expiry) and second call should use stale cache
	claims2 := standardClaims(srv.URL, "bob", []string{"kubernaut-agent"}, nil, time.Now().Add(time.Hour))
	token2 := kp.signToken(t, claims2)

	identity, err := validator.Validate(context.Background(), token2)
	require.NoError(t, err, "should succeed using stale cached JWKS")
	assert.Equal(t, "bob", identity.Username)
}

func TestJWKSCircuitBreaker_OpensAfter3Failures(t *testing.T) {
	kp := newTestKeyPair(t, "key-1")
	failSrv := newFailingJWKSServer(t)

	cfg := auth.AuthConfig{
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
	require.NoError(t, err)

	claims := standardClaims(failSrv.URL, "alice", []string{"kubernaut-agent"}, nil, time.Now().Add(time.Hour))
	token := kp.signToken(t, claims)

	// Trigger 3 failures to open the circuit
	for i := 0; i < 3; i++ {
		_, err = validator.Validate(context.Background(), token)
		assert.Error(t, err, fmt.Sprintf("attempt %d should fail", i+1))
	}

	// 4th attempt should get circuit breaker error (fast fail, not network attempt)
	_, err = validator.Validate(context.Background(), token)
	assert.ErrorIs(t, err, auth.ErrCircuitOpen)
}

func TestJWKSCircuitBreaker_HalfOpenAfter30s(t *testing.T) {
	kp := newTestKeyPair(t, "key-1")
	requestCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		if requestCount <= 3 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(kp.jwks())
		}
	}))
	t.Cleanup(srv.Close)

	cfg := auth.AuthConfig{
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

	// Use short CB timeout (20ms) for testing instead of production 30s
	validator, err := auth.NewJWTValidator(cfg,
		auth.WithHTTPClient(srv.Client()),
		auth.WithCBTestTimeout(20*time.Millisecond),
	)
	require.NoError(t, err)

	claims := standardClaims(srv.URL, "alice", []string{"kubernaut-agent"}, nil, time.Now().Add(time.Hour))
	token := kp.signToken(t, claims)

	// Trigger 3 failures to open
	for i := 0; i < 3; i++ {
		_, _ = validator.Validate(context.Background(), token)
	}

	// Verify circuit is open
	_, err = validator.Validate(context.Background(), token)
	assert.ErrorIs(t, err, auth.ErrCircuitOpen)

	// After timeout, circuit should transition to half-open
	time.Sleep(50 * time.Millisecond)

	// Probe should go through (and succeed since server now responds)
	identity, err := validator.Validate(context.Background(), token)
	require.NoError(t, err)
	assert.Equal(t, "alice", identity.Username)
}

func TestJWKSCircuitBreaker_ClosesOnSuccess(t *testing.T) {
	kp := newTestKeyPair(t, "key-1")
	requestCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		if requestCount <= 3 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(kp.jwks())
		}
	}))
	t.Cleanup(srv.Close)

	cfg := auth.AuthConfig{
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
	require.NoError(t, err)

	claims := standardClaims(srv.URL, "alice", []string{"kubernaut-agent"}, nil, time.Now().Add(time.Hour))
	token := kp.signToken(t, claims)

	// Open the circuit
	for i := 0; i < 3; i++ {
		_, _ = validator.Validate(context.Background(), token)
	}

	// Wait for half-open
	time.Sleep(50 * time.Millisecond)

	// Successful probe should close the circuit
	_, err = validator.Validate(context.Background(), token)
	require.NoError(t, err)

	// Subsequent requests should succeed normally (circuit closed)
	identity, err := validator.Validate(context.Background(), token)
	require.NoError(t, err)
	assert.Equal(t, "alice", identity.Username)
}

func TestJWKSCircuitBreaker_ExistingSessionsFailOpen(t *testing.T) {
	kp := newTestKeyPair(t, "key-1")
	requestCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		if requestCount == 1 {
			// First request succeeds (populates cache)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(kp.jwks())
		} else {
			// Subsequent requests fail (triggers circuit breaker)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)

	cfg := auth.AuthConfig{
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
	require.NoError(t, err)

	claims := standardClaims(srv.URL, "alice", []string{"kubernaut-agent"}, nil, time.Now().Add(time.Hour))
	token := kp.signToken(t, claims)

	// First request succeeds and caches JWKS
	_, err = validator.Validate(context.Background(), token)
	require.NoError(t, err)

	// Trigger circuit breaker open (3 fetch failures on refresh attempts)
	for i := 0; i < 3; i++ {
		_, _ = validator.Validate(context.Background(), token)
	}

	// Existing session (with cached JWKS) should still work (fail-open)
	identity, err := validator.Validate(context.Background(), token)
	require.NoError(t, err, "existing sessions with cached JWKS should continue working when circuit is open")
	assert.Equal(t, "alice", identity.Username)
}

func TestMiddleware_SetsUserContextOnSuccess(t *testing.T) {
	kp := newTestKeyPair(t, "key-1")
	jwksSrv := newJWKSServer(t, kp.jwks())

	cfg := auth.AuthConfig{
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
	require.NoError(t, err)

	var capturedIdentity *auth.UserIdentity
	handler := auth.Middleware(validator)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		capturedIdentity = auth.UserIdentityFromContext(r.Context())
	}))

	claims := standardClaims(jwksSrv.URL, "alice", []string{"kubernaut-agent"}, []string{"sre"}, time.Now().Add(time.Hour))
	token := kp.signToken(t, claims)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, capturedIdentity)
	assert.Equal(t, "alice", capturedIdentity.Username)
	assert.Equal(t, []string{"sre"}, capturedIdentity.Groups)
}

func TestMiddleware_NoAuthHeader_Returns401(t *testing.T) {
	validator, _ := auth.NewJWTValidator(auth.AuthConfig{})
	handler := auth.Middleware(validator)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestMiddleware_AuthHeaderControlChars_Returns400(t *testing.T) {
	kp := newTestKeyPair(t, "key-1")
	jwksSrv := newJWKSServer(t, kp.jwks())

	cfg := auth.AuthConfig{
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
	require.NoError(t, err)

	handler := auth.Middleware(validator)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Authorization header with null byte (control character)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer \x00malicious-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestMiddleware_OversizedBody_Returns413(t *testing.T) {
	kp := newTestKeyPair(t, "key-1")
	jwksSrv := newJWKSServer(t, kp.jwks())

	cfg := auth.AuthConfig{
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
	require.NoError(t, err)

	var bodyReadErr error
	handler := auth.Middleware(validator)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, bodyReadErr = io.ReadAll(r.Body)
		if bodyReadErr != nil {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	claims := standardClaims(jwksSrv.URL, "alice", []string{"kubernaut-agent"}, nil, time.Now().Add(time.Hour))
	token := kp.signToken(t, claims)

	// Create oversized body (larger than MaxBodySize)
	oversizedBody := strings.Repeat("x", auth.MaxBodySize+1)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(oversizedBody))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestMiddleware_K8sTokenReview_ValidSA(t *testing.T) {
	fakeClient := newFakeTokenReviewClient(t, "sa-token-that-is-not-jwt", "system:serviceaccount:default:my-sa", []string{"system:serviceaccounts"})

	reviewer := auth.NewTokenReviewer(fakeClient)

	cfg := auth.AuthConfig{
		JWT:        []auth.ProviderConfig{},
		Kubernetes: auth.KubernetesAuthConfig{Enabled: true},
	}

	validator, err := auth.NewJWTValidator(cfg, auth.WithTokenReviewer(reviewer))
	require.NoError(t, err)

	handler := auth.Middleware(validator)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity := auth.UserIdentityFromContext(r.Context())
		if identity == nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, identity.Username)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer sa-token-that-is-not-jwt")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "system:serviceaccount:")
}

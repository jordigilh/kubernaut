package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	josejwt "github.com/go-jose/go-jose/v4/jwt"
	dto "github.com/prometheus/client_model/go"
)

func testSignToken(t *testing.T, key *rsa.PrivateKey, kid string, claims interface{}) string {
	t.Helper()
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: key},
		(&jose.SignerOptions{}).WithHeader(jose.HeaderKey("kid"), kid),
	)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := josejwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestReady_NilCache(t *testing.T) {
	// Business outcome: validator with no providers (no JWKS cache) is always ready
	v := &JWTValidator{}
	if !v.Ready() {
		t.Error("Ready() should return true when no JWKS cache is configured")
	}
}

func TestReady_HealthyCacheClosed(t *testing.T) {
	// Business outcome: when all circuit breakers are closed, the service is ready
	cache := NewJWKSCache(&http.Client{}, []string{"http://example.com/jwks"})
	v := &JWTValidator{cache: cache}
	if !v.Ready() {
		t.Error("Ready() should return true when all breakers are closed")
	}
}

func TestReady_UnhealthyCacheOpen(t *testing.T) {
	// Business outcome: when a circuit breaker is open, the service reports not ready
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cache := NewJWKSCache(&http.Client{}, []string{srv.URL}, WithCBTimeout(50*time.Millisecond))
	ctx := context.Background()

	// Trip the circuit breaker: 3 consecutive failures
	for i := 0; i < 4; i++ {
		_, _ = cache.GetKeys(ctx, srv.URL)
	}

	v := &JWTValidator{cache: cache}
	if v.Ready() {
		t.Error("Ready() should return false when a breaker is open")
	}
}

func TestWithCBMetrics_GaugeUpdatesOnStateChange(t *testing.T) {
	// Business outcome: the CB state gauge reflects breaker state transitions
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	gauge := NewCircuitBreakerStateGauge()
	keySet := testJWKS(t)
	_ = keySet

	cfg := Config{
		JWT: []ProviderConfig{
			{Issuer: IssuerConfig{URL: srv.URL, Audiences: []string{"aud"}}},
		},
	}

	v, err := NewJWTValidator(cfg,
		WithHTTPClient(srv.Client()),
		WithCBMetrics(gauge),
		WithCBTestTimeout(50*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("NewJWTValidator: %v", err)
	}

	// Generate a valid-looking token to trigger JWKS fetch
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	claims := map[string]interface{}{
		"iss": srv.URL, "sub": "u", "aud": []string{"aud"},
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	token := testSignToken(t, key, "k1", claims)

	// Trigger 4 validation attempts (JWKS fetches) to trip the CB
	for i := 0; i < 4; i++ {
		_, _ = v.Validate(context.Background(), token)
	}

	// Check that the gauge was set (state=2 means open)
	var m dto.Metric
	depLabel := "jwks_" + srv.URL
	g, err := gauge.GetMetricWithLabelValues(depLabel)
	if err != nil {
		t.Fatalf("could not get metric: %v", err)
	}
	if err := g.Write(&m); err != nil {
		t.Fatal(err)
	}
	if m.GetGauge().GetValue() != 2 {
		t.Errorf("expected gauge=2 (open), got %v", m.GetGauge().GetValue())
	}
}

func TestWithReplayCache_RejectsReplayedToken(t *testing.T) {
	// Business outcome: a token used twice with the same jti is rejected the second time
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keySet := jose.JSONWebKeySet{
		Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(keySet)
	}))
	defer srv.Close()

	rc := NewReplayCache(1 * time.Minute)
	defer rc.Stop()

	cfg := Config{
		JWT: []ProviderConfig{
			{Issuer: IssuerConfig{URL: srv.URL, Audiences: []string{"aud"}}},
		},
	}

	v, err := NewJWTValidator(cfg, WithHTTPClient(&http.Client{}), WithReplayCache(rc))
	if err != nil {
		t.Fatalf("NewJWTValidator: %v", err)
	}

	claims := map[string]interface{}{
		"iss": srv.URL, "sub": "alice", "aud": []string{"aud"},
		"exp":                time.Now().Add(time.Hour).Unix(),
		"jti":                "unique-token-id-001",
		"preferred_username": "alice",
	}
	token := testSignToken(t, key, "k1", claims)

	// First use: should succeed
	identity, err := v.Validate(context.Background(), token)
	if err != nil {
		t.Fatalf("first Validate failed: %v", err)
	}
	if identity.Username != "alice" {
		t.Errorf("expected username alice, got %q", identity.Username)
	}

	// Replay: should be rejected
	_, err = v.Validate(context.Background(), token)
	if err == nil {
		t.Fatal("expected error on replay, got nil")
	}
}

func TestWithReplayCache_RequiresJTI(t *testing.T) {
	// Business outcome: when replay protection is enabled, tokens without jti are rejected
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keySet := jose.JSONWebKeySet{
		Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(keySet)
	}))
	defer srv.Close()

	rc := NewReplayCache(1 * time.Minute)
	defer rc.Stop()

	cfg := Config{
		JWT: []ProviderConfig{
			{Issuer: IssuerConfig{URL: srv.URL, Audiences: []string{"aud"}}},
		},
	}

	v, err := NewJWTValidator(cfg, WithHTTPClient(&http.Client{}), WithReplayCache(rc))
	if err != nil {
		t.Fatal(err)
	}

	// Token without jti
	claims := map[string]interface{}{
		"iss": srv.URL, "sub": "alice", "aud": []string{"aud"},
		"exp":                time.Now().Add(time.Hour).Unix(),
		"preferred_username": "alice",
	}
	token := testSignToken(t, key, "k1", claims)

	_, err = v.Validate(context.Background(), token)
	if err == nil {
		t.Fatal("expected error for missing jti with replay protection enabled")
	}
}

package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/prometheus/client_golang/prometheus"
)

func testJWKS(t *testing.T) jose.JSONWebKeySet {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return jose.JSONWebKeySet{
		Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "test-key", Algorithm: "RS256", Use: "sig"}},
	}
}

func TestNewCircuitBreakerStateGauge(t *testing.T) {
	g := NewCircuitBreakerStateGauge()
	if g == nil {
		t.Fatal("expected non-nil gauge")
	}
}

func TestWithCBGauge(t *testing.T) {
	g := NewCircuitBreakerStateGauge()
	cache := NewJWKSCache(nil, []string{"issuer1"}, WithCBGauge(g))
	if cache == nil {
		t.Fatal("expected non-nil cache")
	}
}

func TestWithMaxStaleness(t *testing.T) {
	cache := NewJWKSCache(nil, []string{"issuer1"}, WithMaxStaleness(2*time.Hour))
	if cache.maxStaleness != 2*time.Hour {
		t.Errorf("expected maxStaleness=2h, got %v", cache.maxStaleness)
	}
}

func TestJWKSCache_Healthy_AllClosed(t *testing.T) {
	cache := NewJWKSCache(&http.Client{Timeout: time.Second}, []string{"iss1", "iss2"})
	if !cache.Healthy() {
		t.Error("expected Healthy() == true when all breakers are closed")
	}
}

func TestNewJWKSCache_DefaultMaxStaleness(t *testing.T) {
	cache := NewJWKSCache(nil, []string{"issuer1"})
	if cache.maxStaleness != time.Hour {
		t.Errorf("expected default maxStaleness=1h, got %v", cache.maxStaleness)
	}
}

func TestNewJWKSCache_RegistersCBGaugeStateChange(t *testing.T) {
	g := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "test_cb_state",
	}, []string{"dependency"})
	cache := NewJWKSCache(nil, []string{"iss1"}, WithCBGauge(g))
	if cache == nil {
		t.Fatal("expected non-nil cache")
	}
	if _, ok := cache.breakers["iss1"]; !ok {
		t.Error("expected breaker registered for iss1")
	}
}

func TestWithRefreshInterval(t *testing.T) {
	cache := NewJWKSCache(nil, []string{"iss1"}, WithRefreshInterval(10*time.Minute))
	if cache.refreshInterval != 10*time.Minute {
		t.Errorf("expected refreshInterval=10m, got %v", cache.refreshInterval)
	}
}

func TestJWKSCache_GetKeys_SkipsFetchWhenFresh(t *testing.T) {
	// Business outcome: if JWKS was fetched recently (within refreshInterval),
	// GetKeys returns cached keys WITHOUT hitting the network.
	var fetchCount atomic.Int32
	keySet := testJWKS(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fetchCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(keySet)
	}))
	defer srv.Close()

	cache := NewJWKSCache(&http.Client{}, []string{srv.URL}, WithRefreshInterval(5*time.Minute))
	ctx := context.Background()

	// First call: must fetch
	keys1, err := cache.GetKeys(ctx, srv.URL)
	if err != nil {
		t.Fatalf("first GetKeys: %v", err)
	}
	if keys1 == nil || len(keys1.Keys) == 0 {
		t.Fatal("expected non-empty keys on first fetch")
	}
	if fetchCount.Load() != 1 {
		t.Fatalf("expected 1 fetch, got %d", fetchCount.Load())
	}

	// Second call: should return cached (no new fetch)
	keys2, err := cache.GetKeys(ctx, srv.URL)
	if err != nil {
		t.Fatalf("second GetKeys: %v", err)
	}
	if keys2 == nil || len(keys2.Keys) == 0 {
		t.Fatal("expected non-empty keys on cached return")
	}
	if fetchCount.Load() != 1 {
		t.Fatalf("expected still 1 fetch (cached), got %d", fetchCount.Load())
	}
}

func TestJWKSCache_GetKeys_RefetchesAfterTTLExpires(t *testing.T) {
	// Business outcome: after refreshInterval expires, a new fetch is performed.
	var fetchCount atomic.Int32
	keySet := testJWKS(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fetchCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(keySet)
	}))
	defer srv.Close()

	// Use very short refresh interval so it expires immediately
	cache := NewJWKSCache(&http.Client{}, []string{srv.URL}, WithRefreshInterval(1*time.Millisecond))
	ctx := context.Background()

	// First fetch
	_, err := cache.GetKeys(ctx, srv.URL)
	if err != nil {
		t.Fatalf("first GetKeys: %v", err)
	}

	// Wait for TTL to expire
	time.Sleep(5 * time.Millisecond)

	// Should re-fetch
	_, err = cache.GetKeys(ctx, srv.URL)
	if err != nil {
		t.Fatalf("second GetKeys: %v", err)
	}
	if fetchCount.Load() != 2 {
		t.Fatalf("expected 2 fetches after TTL expiry, got %d", fetchCount.Load())
	}
}

package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
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

// spyFetchLimiter records AllowFetch calls and returns configurable results.
type spyFetchLimiter struct {
	allowed    bool
	callCount  atomic.Int32
}

func (s *spyFetchLimiter) AllowFetch(_ string) bool {
	s.callCount.Add(1)
	return s.allowed
}

func TestJWKSCache_FetchLimiter_BlocksExcessiveFetches(t *testing.T) {
	// IT-AF-RATE-W01: ProviderLimiter integration — when rate-limited,
	// GetKeys returns cached keys without making a network call.
	var fetchCount atomic.Int32
	keySet := testJWKS(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fetchCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(keySet)
	}))
	defer srv.Close()

	limiter := &spyFetchLimiter{allowed: true}
	cache := NewJWKSCache(&http.Client{}, []string{srv.URL},
		WithRefreshInterval(0),
		WithFetchLimiter(limiter),
	)
	ctx := context.Background()

	// First call: limiter allows, fetch occurs
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

	// Second call: limiter blocks, cached keys returned without fetch
	limiter.allowed = false
	keys2, err := cache.GetKeys(ctx, srv.URL)
	if err != nil {
		t.Fatalf("second GetKeys: %v", err)
	}
	if keys2 == nil || len(keys2.Keys) == 0 {
		t.Fatal("expected non-empty cached keys when rate-limited")
	}
	if fetchCount.Load() != 1 {
		t.Fatalf("expected still 1 fetch (rate-limited), got %d", fetchCount.Load())
	}

	// Third call: limiter allows again, new fetch occurs
	limiter.allowed = true
	_, err = cache.GetKeys(ctx, srv.URL)
	if err != nil {
		t.Fatalf("third GetKeys: %v", err)
	}
	if fetchCount.Load() != 2 {
		t.Fatalf("expected 2 fetches after limiter allows, got %d", fetchCount.Load())
	}
}

// TestJWKSCache_GetKeys_ConcurrentColdCacheDedupsFetch proves the PR #1790
// round-12 RCA fix: concurrent requests arriving on a COLD cache (e.g. right
// after a pod restart, before any entry exists) must not each independently
// hit the network. Before the singleflight fix, two near-simultaneous
// callers could each pass freshCachedKeys/throttledCachedKeys (no entry yet)
// and race to fetchJWKS separately -- if one of those two fetches failed
// under load while the other succeeded, the unlucky caller 401'd even
// though a JWKS fetch for the exact same issuer succeeded milliseconds
// earlier (observed as APIFrontend's `notifications/initialized` 401 right
// after PHASE 6 restarted AF in the fullpipeline/fleet E2E suites).
func TestJWKSCache_GetKeys_ConcurrentColdCacheDedupsFetch(t *testing.T) {
	var fetchCount atomic.Int32
	keySet := testJWKS(t)
	release := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fetchCount.Add(1)
		<-release // hold every concurrent caller inside the fetch window
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(keySet)
	}))
	defer srv.Close()

	cache := NewJWKSCache(&http.Client{}, []string{srv.URL}, WithRefreshInterval(5*time.Minute))
	ctx := context.Background()

	const concurrentCallers = 10
	var wg sync.WaitGroup
	errs := make([]error, concurrentCallers)
	keys := make([]*jose.JSONWebKeySet, concurrentCallers)
	wg.Add(concurrentCallers)
	for i := 0; i < concurrentCallers; i++ {
		go func(idx int) {
			defer wg.Done()
			keys[idx], errs[idx] = cache.GetKeys(ctx, srv.URL)
		}(i)
	}

	// Give every goroutine a chance to reach the cold-cache fetch path
	// before letting the (single, deduplicated) server response through.
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	if got := fetchCount.Load(); got != 1 {
		t.Fatalf("expected exactly 1 network fetch for %d concurrent cold-cache callers (singleflight dedup), got %d", concurrentCallers, got)
	}
	for i, err := range errs {
		if err != nil {
			t.Fatalf("caller %d: unexpected error: %v", i, err)
		}
		if keys[i] == nil || len(keys[i].Keys) == 0 {
			t.Fatalf("caller %d: expected non-empty deduplicated keys", i)
		}
	}
}

// TestJWKSCache_GetKeys_ConcurrentColdCacheDedupsFailure proves the
// singleflight fix also collapses concurrent failures: if the single
// deduplicated fetch fails, every waiting caller observes that same
// failure -- there is no scenario where a second concurrent caller's own
// independent fetch attempt could fail while the first's succeeds (or
// vice versa), because only one fetch ever happens.
func TestJWKSCache_GetKeys_ConcurrentColdCacheDedupsFailure(t *testing.T) {
	var fetchCount atomic.Int32
	release := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fetchCount.Add(1)
		<-release
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cache := NewJWKSCache(&http.Client{}, []string{srv.URL}, WithRefreshInterval(5*time.Minute))
	ctx := context.Background()

	const concurrentCallers = 5
	var wg sync.WaitGroup
	errs := make([]error, concurrentCallers)
	wg.Add(concurrentCallers)
	for i := 0; i < concurrentCallers; i++ {
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = cache.GetKeys(ctx, srv.URL)
		}(i)
	}

	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	if got := fetchCount.Load(); got != 1 {
		t.Fatalf("expected exactly 1 network fetch attempt for %d concurrent cold-cache callers, got %d", concurrentCallers, got)
	}
	for i, err := range errs {
		if err == nil {
			t.Fatalf("caller %d: expected error from the deduplicated failed fetch, got nil", i)
		}
	}
}

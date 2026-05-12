package auth

import (
	"net/http"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

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

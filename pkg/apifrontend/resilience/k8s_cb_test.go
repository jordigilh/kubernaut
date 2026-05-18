package resilience

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	gobreaker "github.com/sony/gobreaker/v2"
)

// UT-AF-038-050
func TestK8sCB_OpensAfterNFailures(t *testing.T) {
	cb := NewK8sCircuitBreaker(K8sCBConfig{
		Name:             "test-k8s-cb",
		MaxRequests:      1,
		Interval:         10 * time.Second,
		Timeout:          50 * time.Millisecond,
		FailureThreshold: 3,
	})

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_ = cb.Execute(ctx, func(_ context.Context) error {
			return errors.New("apiserver error")
		})
	}

	if cb.State() != gobreaker.StateOpen {
		t.Errorf("state = %v, want Open", cb.State())
	}

	err := cb.Execute(ctx, func(_ context.Context) error {
		t.Error("function should not be called when CB is open")
		return nil
	})
	if err == nil {
		t.Fatal("expected error when CB is open")
	}
}

// UT-AF-038-051
func TestK8sCB_PassesThroughWhenClosed(t *testing.T) {
	cb := NewK8sCircuitBreaker(K8sCBConfig{
		Name:             "test-k8s-pass",
		MaxRequests:      5,
		Interval:         10 * time.Second,
		Timeout:          30 * time.Second,
		FailureThreshold: 5,
	})

	ctx := context.Background()
	called := false
	err := cb.Execute(ctx, func(_ context.Context) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !called {
		t.Error("function was not called")
	}
	if cb.State() != gobreaker.StateClosed {
		t.Errorf("state = %v, want Closed", cb.State())
	}
}

// UT-AF-038-053
func TestK8sCB_ReportsStateToGauge(t *testing.T) {
	gauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "test_k8s_cb_state",
	}, []string{"dependency"})

	cb := NewK8sCircuitBreaker(K8sCBConfig{
		Name:             "test-k8s-gauge",
		MaxRequests:      1,
		Interval:         10 * time.Second,
		Timeout:          50 * time.Millisecond,
		FailureThreshold: 2,
		StateGauge:       gauge,
		DependencyName:   "k8s",
	})

	ctx := context.Background()
	_ = cb.Execute(ctx, func(_ context.Context) error { return errors.New("fail") })
	_ = cb.Execute(ctx, func(_ context.Context) error { return errors.New("fail") })

	m := &dto.Metric{}
	if err := gauge.WithLabelValues("k8s").Write(m); err != nil {
		t.Fatalf("write gauge: %v", err)
	}
	if m.GetGauge().GetValue() != 2 {
		t.Errorf("gauge = %v, want 2 (open)", m.GetGauge().GetValue())
	}
}

// UT-AF-038-051 additional: Healthy returns true when closed
func TestK8sCB_HealthyWhenClosed(t *testing.T) {
	cb := NewK8sCircuitBreaker(K8sCBConfig{
		Name:             "test-k8s-healthy",
		FailureThreshold: 5,
	})
	if !cb.Healthy() {
		t.Error("Healthy() = false, want true when closed")
	}
}

// UT-AF-038-050 additional: Healthy returns false when open
func TestK8sCB_UnhealthyWhenOpen(t *testing.T) {
	cb := NewK8sCircuitBreaker(K8sCBConfig{
		Name:             "test-k8s-unhealthy",
		FailureThreshold: 1,
		Timeout:          1 * time.Second,
	})
	ctx := context.Background()
	_ = cb.Execute(ctx, func(_ context.Context) error { return errors.New("fail") })
	if cb.Healthy() {
		t.Error("Healthy() = true, want false when open")
	}
}

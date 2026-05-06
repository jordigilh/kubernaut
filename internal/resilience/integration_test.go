package resilience

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	gobreaker "github.com/sony/gobreaker/v2"
)

// IT-AF-038-070: KA 5xx burst opens CB, next call fails fast.
func TestIntegration_BurstOpensCB(t *testing.T) {
	var callCount atomic.Int32
	base := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		callCount.Add(1)
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Body:       io.NopCloser(strings.NewReader("error")),
		}, nil
	})

	retryRT := NewRetryTransport(base, RetryConfig{
		MaxAttempts:       1,
		InitialBackoff:    1 * time.Millisecond,
		MaxBackoff:        5 * time.Millisecond,
		RetryableStatuses: []int{502, 503, 504},
	})

	cbt := NewCircuitBreakerTransport(retryRT, &CircuitBreakerConfig{
		Name:             "it-burst",
		MaxRequests:      1,
		Interval:         10 * time.Second,
		Timeout:          100 * time.Millisecond,
		FailureThreshold: 3,
	})

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.com/test", http.NoBody)

	// Trip it with 3 failures
	for i := 0; i < 3; i++ {
		_, _ = cbt.RoundTrip(req)
	}

	if cbt.State() != gobreaker.StateOpen {
		t.Fatalf("CB state = %v, want Open", cbt.State())
	}

	// Next call should fail fast
	callsBefore := callCount.Load()
	_, err := cbt.RoundTrip(req)
	if err == nil {
		t.Fatal("expected error on open CB")
	}
	if !errors.Is(err, gobreaker.ErrOpenState) {
		t.Errorf("error = %v, want ErrOpenState", err)
	}
	if callCount.Load() != callsBefore {
		t.Error("base transport was called when CB is open")
	}
}

// IT-AF-038-071: KA CB recovers after timeout period.
func TestIntegration_CBRecovers(t *testing.T) {
	var shouldSucceed atomic.Bool
	base := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		if shouldSucceed.Load() {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("ok")),
			}, nil
		}
		return nil, errors.New("connection refused")
	})

	cbt := NewCircuitBreakerTransport(base, &CircuitBreakerConfig{
		Name:             "it-recover",
		MaxRequests:      1,
		Interval:         0,
		Timeout:          50 * time.Millisecond,
		FailureThreshold: 2,
	})

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.com/test", http.NoBody)

	// Trip it
	_, _ = cbt.RoundTrip(req)
	_, _ = cbt.RoundTrip(req)

	if cbt.State() != gobreaker.StateOpen {
		t.Fatalf("CB state = %v, want Open", cbt.State())
	}

	// Wait for half-open
	time.Sleep(100 * time.Millisecond)

	// Allow success
	shouldSucceed.Store(true)
	resp, err := cbt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip in half-open = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	// Should be closed now
	if cbt.State() != gobreaker.StateClosed {
		t.Errorf("CB state after recovery = %v, want Closed", cbt.State())
	}
}

// IT-AF-038-073: Retry succeeds on transient 503 then 200.
func TestIntegration_RetrySucceedsOnTransient(t *testing.T) {
	var attempts atomic.Int32
	base := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		n := attempts.Add(1)
		if n <= 2 {
			return &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Body:       io.NopCloser(strings.NewReader("busy")),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
		}, nil
	})

	retryRT := NewRetryTransport(base, RetryConfig{
		MaxAttempts:       4,
		InitialBackoff:    1 * time.Millisecond,
		MaxBackoff:        10 * time.Millisecond,
		RetryableStatuses: []int{502, 503, 504},
	})

	cbt := NewCircuitBreakerTransport(retryRT, &CircuitBreakerConfig{
		Name:             "it-transient",
		MaxRequests:      5,
		Interval:         10 * time.Second,
		Timeout:          30 * time.Second,
		FailureThreshold: 10,
	})

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.com/test", http.NoBody)
	resp, err := cbt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if attempts.Load() != 3 {
		t.Errorf("attempts = %d, want 3", attempts.Load())
	}
}

// IT-AF-038-074: Retry exhaustion returns last error to caller.
func TestIntegration_RetryExhaustion(t *testing.T) {
	base := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Body:       io.NopCloser(strings.NewReader("busy")),
		}, nil
	})

	retryRT := NewRetryTransport(base, RetryConfig{
		MaxAttempts:       3,
		InitialBackoff:    1 * time.Millisecond,
		MaxBackoff:        5 * time.Millisecond,
		RetryableStatuses: []int{503},
	})

	cbt := NewCircuitBreakerTransport(retryRT, &CircuitBreakerConfig{
		Name:             "it-exhaust",
		MaxRequests:      5,
		Interval:         10 * time.Second,
		Timeout:          30 * time.Second,
		FailureThreshold: 10,
	})

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.com/test", http.NoBody)
	resp, err := cbt.RoundTrip(req)
	// The CB transport gets a 503 from retry exhaust — it's a failure
	// CB sees it as a failure (502/503/504) and wraps it
	// After retry exhaustion, we get the last response (503) back
	if err != nil {
		t.Fatalf("expected resp with status, got error = %v", err)
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", resp.StatusCode)
	}
}

// IT-AF-038-072: DS timeout opens CB, subsequent calls fail fast.
func TestIntegration_TimeoutOpensCB(t *testing.T) {
	gauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "it_ds_cb_state",
	}, []string{"dependency"})

	base := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return nil, errors.New("i/o timeout")
	})

	cbt := NewCircuitBreakerTransport(base, &CircuitBreakerConfig{
		Name:             "it-timeout",
		MaxRequests:      1,
		Interval:         10 * time.Second,
		Timeout:          1 * time.Second,
		FailureThreshold: 2,
		StateGauge:       gauge,
		DependencyName:   "ds",
	})

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.com/test", http.NoBody)

	// Trip with 2 timeouts
	_, _ = cbt.RoundTrip(req)
	_, _ = cbt.RoundTrip(req)

	if cbt.State() != gobreaker.StateOpen {
		t.Fatalf("CB state = %v, want Open", cbt.State())
	}

	// Verify gauge was set
	m := &dto.Metric{}
	if err := gauge.WithLabelValues("ds").Write(m); err != nil {
		t.Fatalf("write gauge: %v", err)
	}
	if m.GetGauge().GetValue() != 2 {
		t.Errorf("gauge = %v, want 2 (Open)", m.GetGauge().GetValue())
	}

	// Next call fails fast
	_, err := cbt.RoundTrip(req)
	if !errors.Is(err, gobreaker.ErrOpenState) {
		t.Errorf("error = %v, want ErrOpenState", err)
	}
}

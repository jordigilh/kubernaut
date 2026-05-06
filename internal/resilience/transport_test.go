package resilience

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	gobreaker "github.com/sony/gobreaker/v2"
)

// roundTripFunc implements http.RoundTripper for testing.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newResponse(status int) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader("")),
	}
}

// UT-AF-038-010
func TestRetryTransport_RetriesOn503AndSucceeds(t *testing.T) {
	var attempts int32
	base := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		n := atomic.AddInt32(&attempts, 1)
		if n == 1 {
			return newResponse(http.StatusServiceUnavailable), nil
		}
		return newResponse(http.StatusOK), nil
	})

	rt := NewRetryTransport(base, RetryConfig{
		MaxAttempts:       3,
		InitialBackoff:    1 * time.Millisecond,
		MaxBackoff:        10 * time.Millisecond,
		RetryableStatuses: []int{502, 503, 504},
	})

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/test", http.NoBody)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
	if atomic.LoadInt32(&attempts) != 2 {
		t.Errorf("attempts = %d, want 2", atomic.LoadInt32(&attempts))
	}
}

// UT-AF-038-011
func TestRetryTransport_GivesUpAfterMaxAttempts(t *testing.T) {
	var attempts int32
	base := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		return newResponse(http.StatusServiceUnavailable), nil
	})

	rt := NewRetryTransport(base, RetryConfig{
		MaxAttempts:       3,
		InitialBackoff:    1 * time.Millisecond,
		MaxBackoff:        10 * time.Millisecond,
		RetryableStatuses: []int{503},
	})

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/test", http.NoBody)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("StatusCode = %d, want 503", resp.StatusCode)
	}
	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("attempts = %d, want 3", atomic.LoadInt32(&attempts))
	}
}

// UT-AF-038-012
func TestRetryTransport_DoesNotRetry400(t *testing.T) {
	var attempts int32
	base := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		return newResponse(http.StatusBadRequest), nil
	})

	rt := NewRetryTransport(base, RetryConfig{
		MaxAttempts:       3,
		InitialBackoff:    1 * time.Millisecond,
		MaxBackoff:        10 * time.Millisecond,
		RetryableStatuses: []int{502, 503, 504},
	})

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/test", http.NoBody)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("StatusCode = %d, want 400", resp.StatusCode)
	}
	if atomic.LoadInt32(&attempts) != 1 {
		t.Errorf("attempts = %d, want 1 (no retry)", atomic.LoadInt32(&attempts))
	}
}

// UT-AF-038-013
func TestRetryTransport_DoesNotRetryNonReplayableBody(t *testing.T) {
	var attempts int32
	base := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		return nil, errors.New("connection reset")
	})

	rt := NewRetryTransport(base, RetryConfig{
		MaxAttempts:       3,
		InitialBackoff:    1 * time.Millisecond,
		MaxBackoff:        10 * time.Millisecond,
		RetryableStatuses: []int{502, 503, 504},
	})

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://example.com/test",
		strings.NewReader("body"))
	// No GetBody set — body is not replayable
	resp, err := rt.RoundTrip(req)
	if err == nil {
		t.Fatalf("RoundTrip() expected error, got resp=%v", resp)
	}
	if atomic.LoadInt32(&attempts) != 1 {
		t.Errorf("attempts = %d, want 1 (no retry for non-replayable body)", atomic.LoadInt32(&attempts))
	}
}

// UT-AF-038-014
func TestRetryTransport_RespectsContextCancellation(t *testing.T) {
	base := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return newResponse(http.StatusServiceUnavailable), nil
	})

	rt := NewRetryTransport(base, RetryConfig{
		MaxAttempts:       5,
		InitialBackoff:    1 * time.Second,
		MaxBackoff:        5 * time.Second,
		RetryableStatuses: []int{503},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.com/test", http.NoBody)
	_, err := rt.RoundTrip(req)
	if err == nil {
		t.Fatal("RoundTrip() expected error on cancelled context")
	}
}

// UT-AF-038-015
func TestRetryTransport_IncrementsRetryMetric(t *testing.T) {
	counter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "test_retry_total",
	}, []string{"dependency", "attempt"})

	var attempts int32
	base := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 3 {
			return newResponse(http.StatusServiceUnavailable), nil
		}
		return newResponse(http.StatusOK), nil
	})

	rt := NewRetryTransport(base, RetryConfig{
		MaxAttempts:       3,
		InitialBackoff:    1 * time.Millisecond,
		MaxBackoff:        10 * time.Millisecond,
		RetryableStatuses: []int{503},
		RetryCounter:      counter,
		DependencyName:    "test",
	})

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/test", http.NoBody)
	_, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}

	// Should have 2 retries recorded (attempts 2 and 3)
	m := &dto.Metric{}
	if err := counter.WithLabelValues("test", "2").Write(m); err != nil {
		t.Fatalf("write metric: %v", err)
	}
	if m.GetCounter().GetValue() != 1 {
		t.Errorf("retry counter attempt=2 = %v, want 1", m.GetCounter().GetValue())
	}
}

// UT-AF-038-016
func TestCBTransport_OpensAfterConsecutiveFailures(t *testing.T) {
	failCount := 5
	var attempts int32
	base := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		return nil, errors.New("server error")
	})

	cbt := NewCircuitBreakerTransport(base, &CircuitBreakerConfig{
		Name:             "test-cb",
		MaxRequests:      1,
		Interval:         10 * time.Second,
		Timeout:          100 * time.Millisecond,
		FailureThreshold: uint32(failCount),
	})

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/test", http.NoBody)

	// Trip the breaker
	for i := 0; i < failCount; i++ {
		_, _ = cbt.RoundTrip(req)
	}

	// Next request should be rejected by open circuit
	_, err := cbt.RoundTrip(req)
	if err == nil {
		t.Fatal("expected error when CB is open")
	}
	if !errors.Is(err, gobreaker.ErrOpenState) {
		t.Errorf("error = %v, want gobreaker.ErrOpenState", err)
	}
}

// UT-AF-038-017
func TestCBTransport_RejectsImmediatelyWhenOpen(t *testing.T) {
	base := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return nil, errors.New("server down")
	})

	cbt := NewCircuitBreakerTransport(base, &CircuitBreakerConfig{
		Name:             "test-cb-reject",
		MaxRequests:      1,
		Interval:         10 * time.Second,
		Timeout:          1 * time.Second,
		FailureThreshold: 2,
	})

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/test", http.NoBody)

	// Trip it
	_, _ = cbt.RoundTrip(req)
	_, _ = cbt.RoundTrip(req)

	// Now should fail fast without calling base
	start := time.Now()
	_, err := cbt.RoundTrip(req)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error when CB is open")
	}
	if elapsed > 50*time.Millisecond {
		t.Errorf("fail-fast took %v, want < 50ms", elapsed)
	}
}

// UT-AF-038-018
func TestCBTransport_TransitionsHalfOpenToClosed(t *testing.T) {
	var shouldSucceed atomic.Bool
	base := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		if shouldSucceed.Load() {
			return newResponse(http.StatusOK), nil
		}
		return nil, errors.New("fail")
	})

	cbt := NewCircuitBreakerTransport(base, &CircuitBreakerConfig{
		Name:             "test-cb-recover",
		MaxRequests:      1,
		Interval:         0,
		Timeout:          50 * time.Millisecond,
		FailureThreshold: 2,
	})

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/test", http.NoBody)

	// Trip it
	_, _ = cbt.RoundTrip(req)
	_, _ = cbt.RoundTrip(req)

	// Wait for half-open
	time.Sleep(100 * time.Millisecond)

	// Now allow success
	shouldSucceed.Store(true)
	resp, err := cbt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip in half-open error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	// Should now be closed — next request should also succeed
	resp, err = cbt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip after recovery error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status after recovery = %d, want 200", resp.StatusCode)
	}
}

// UT-AF-038-019
func TestCBTransport_UpdatesStateGauge(t *testing.T) {
	gauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "test_cb_state",
	}, []string{"dependency"})

	base := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return nil, errors.New("fail")
	})

	cbt := NewCircuitBreakerTransport(base, &CircuitBreakerConfig{
		Name:             "test-cb-gauge",
		MaxRequests:      1,
		Interval:         10 * time.Second,
		Timeout:          50 * time.Millisecond,
		FailureThreshold: 2,
		StateGauge:       gauge,
		DependencyName:   "test",
	})

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/test", http.NoBody)
	_, _ = cbt.RoundTrip(req)
	_, _ = cbt.RoundTrip(req)

	// Check gauge was updated to "open" (value 2)
	m := &dto.Metric{}
	if err := gauge.WithLabelValues("test").Write(m); err != nil {
		t.Fatalf("write gauge: %v", err)
	}
	if m.GetGauge().GetValue() != 2 {
		t.Errorf("gauge = %v, want 2 (open)", m.GetGauge().GetValue())
	}
}

// UT-AF-038-020
func TestCBTransport_RecordsDurationHistogram(t *testing.T) {
	hist := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "test_duration",
		Buckets: prometheus.DefBuckets,
	}, []string{"dependency", "status"})

	base := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return newResponse(http.StatusOK), nil
	})

	cbt := NewCircuitBreakerTransport(base, &CircuitBreakerConfig{
		Name:             "test-cb-hist",
		MaxRequests:      5,
		Interval:         10 * time.Second,
		Timeout:          30 * time.Second,
		FailureThreshold: 5,
		DurationHist:     hist,
		DependencyName:   "test",
	})

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/test", http.NoBody)
	_, err := cbt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}

	// Verify histogram was observed
	m := &dto.Metric{}
	observer, err := hist.GetMetricWithLabelValues("test", "200")
	if err != nil {
		t.Fatalf("get metric: %v", err)
	}
	metric, ok := observer.(prometheus.Metric)
	if !ok {
		t.Fatal("observer does not implement prometheus.Metric")
	}
	if err := metric.Write(m); err != nil {
		t.Fatalf("write hist: %v", err)
	}
	if m.GetHistogram().GetSampleCount() != 1 {
		t.Errorf("histogram sample count = %d, want 1", m.GetHistogram().GetSampleCount())
	}
}

// UT-AF-038-021
func TestFullChain_ConcurrentLoad(t *testing.T) {
	var successCount atomic.Int32
	base := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		successCount.Add(1)
		return newResponse(http.StatusOK), nil
	})

	retryRT := NewRetryTransport(base, RetryConfig{
		MaxAttempts:       2,
		InitialBackoff:    1 * time.Millisecond,
		MaxBackoff:        5 * time.Millisecond,
		RetryableStatuses: []int{503},
	})

	cbt := NewCircuitBreakerTransport(retryRT, &CircuitBreakerConfig{
		Name:             "test-chain",
		MaxRequests:      10,
		Interval:         10 * time.Second,
		Timeout:          30 * time.Second,
		FailureThreshold: 50,
	})

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/test", http.NoBody)
			resp, err := cbt.RoundTrip(req)
			if err != nil {
				t.Errorf("concurrent RoundTrip error = %v", err)
				return
			}
			if resp.StatusCode != http.StatusOK {
				t.Errorf("concurrent status = %d, want 200", resp.StatusCode)
			}
		}()
	}
	wg.Wait()

	if successCount.Load() != 20 {
		t.Errorf("total successes = %d, want 20", successCount.Load())
	}
}

// UT-AF-038-022
func TestRetryTransport_HandlesECONNRESET(t *testing.T) {
	var attempts int32
	base := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		n := atomic.AddInt32(&attempts, 1)
		if n == 1 {
			return nil, syscall.ECONNRESET
		}
		return newResponse(http.StatusOK), nil
	})

	rt := NewRetryTransport(base, RetryConfig{
		MaxAttempts:       3,
		InitialBackoff:    1 * time.Millisecond,
		MaxBackoff:        10 * time.Millisecond,
		RetryableStatuses: []int{502, 503, 504},
	})

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/test", http.NoBody)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if atomic.LoadInt32(&attempts) != 2 {
		t.Errorf("attempts = %d, want 2", atomic.LoadInt32(&attempts))
	}
}

// UT-AF-038-023
func TestRetryTransport_HandlesEOF(t *testing.T) {
	var attempts int32
	base := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		n := atomic.AddInt32(&attempts, 1)
		if n == 1 {
			return nil, io.EOF
		}
		return newResponse(http.StatusOK), nil
	})

	rt := NewRetryTransport(base, RetryConfig{
		MaxAttempts:       3,
		InitialBackoff:    1 * time.Millisecond,
		MaxBackoff:        10 * time.Millisecond,
		RetryableStatuses: []int{502, 503, 504},
	})

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/test", http.NoBody)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

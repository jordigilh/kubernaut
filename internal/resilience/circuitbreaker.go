package resilience

import (
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	gobreaker "github.com/sony/gobreaker/v2"
)

// CircuitBreakerConfig controls the circuit breaker behavior.
type CircuitBreakerConfig struct {
	Name             string
	MaxRequests      uint32
	Interval         time.Duration
	Timeout          time.Duration
	FailureThreshold uint32
	StateGauge       *prometheus.GaugeVec
	DurationHist     *prometheus.HistogramVec
	DependencyName   string
	OnStateChange    func(name string, from, to gobreaker.State)
}

// CircuitBreakerTransport wraps an http.RoundTripper with gobreaker protection.
// When the breaker is open, requests fail immediately with gobreaker.ErrOpenState.
type CircuitBreakerTransport struct {
	next http.RoundTripper
	cb   *gobreaker.CircuitBreaker[*http.Response]
	cfg  CircuitBreakerConfig
}

// NewCircuitBreakerTransport creates a CircuitBreakerTransport.
func NewCircuitBreakerTransport(next http.RoundTripper, cfg *CircuitBreakerConfig) *CircuitBreakerTransport {
	failureThreshold := cfg.FailureThreshold
	if failureThreshold == 0 {
		failureThreshold = 5
	}

	settings := gobreaker.Settings{
		Name:        cfg.Name,
		MaxRequests: cfg.MaxRequests,
		Interval:    cfg.Interval,
		Timeout:     cfg.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= failureThreshold
		},
	}

	settings.OnStateChange = func(name string, from, to gobreaker.State) {
		if cfg.StateGauge != nil {
			cfg.StateGauge.WithLabelValues(cfg.DependencyName).Set(float64(to))
		}
		if cfg.OnStateChange != nil {
			cfg.OnStateChange(name, from, to)
		}
	}

	cb := gobreaker.NewCircuitBreaker[*http.Response](settings)

	return &CircuitBreakerTransport{
		next: next,
		cb:   cb,
		cfg:  *cfg,
	}
}

// RoundTrip executes the HTTP request through the circuit breaker.
func (t *CircuitBreakerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()

	resp, err := t.cb.Execute(func() (*http.Response, error) {
		r, e := t.next.RoundTrip(req)
		if e != nil {
			return nil, e
		}
		if isCircuitBreakerFailure(r.StatusCode) {
			return r, fmt.Errorf("downstream returned %d", r.StatusCode)
		}
		return r, nil
	})

	duration := time.Since(start).Seconds()

	if t.cfg.DurationHist != nil {
		status := "error"
		if resp != nil {
			status = fmt.Sprintf("%d", resp.StatusCode)
		}
		t.cfg.DurationHist.WithLabelValues(t.cfg.DependencyName, status).Observe(duration)
	}

	if err != nil {
		if resp != nil {
			return resp, nil
		}
		return nil, err
	}

	return resp, nil
}

// State returns the current circuit breaker state.
func (t *CircuitBreakerTransport) State() gobreaker.State {
	return t.cb.State()
}

func isCircuitBreakerFailure(code int) bool {
	return code == http.StatusBadGateway ||
		code == http.StatusServiceUnavailable ||
		code == http.StatusGatewayTimeout
}

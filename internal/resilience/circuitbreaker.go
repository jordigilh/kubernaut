package resilience

import (
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
	FailureStatuses  []int
	StateGauge       *prometheus.GaugeVec
	DurationHist     *prometheus.HistogramVec
	DependencyName   string
	OnStateChange    func(name string, from, to gobreaker.State)
	// AuditFunc is called on every state transition for FedRAMP AU-2 compliance.
	// It receives the dependency name, previous state, and new state.
	AuditFunc func(dependency string, from, to gobreaker.State)
}

// CircuitBreakerTransport wraps an http.RoundTripper with gobreaker protection.
// When the breaker is open, requests fail immediately with gobreaker.ErrOpenState.
type CircuitBreakerTransport struct {
	next            http.RoundTripper
	cb              *gobreaker.CircuitBreaker[*http.Response]
	cfg             CircuitBreakerConfig
	failureStatuses map[int]struct{}
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
		if cfg.AuditFunc != nil {
			cfg.AuditFunc(cfg.DependencyName, from, to)
		}
		if cfg.OnStateChange != nil {
			cfg.OnStateChange(name, from, to)
		}
	}

	cb := gobreaker.NewCircuitBreaker[*http.Response](settings)

	if cfg.StateGauge != nil {
		cfg.StateGauge.WithLabelValues(cfg.DependencyName).Set(float64(gobreaker.StateClosed))
	}
	if cfg.DurationHist != nil && cfg.DependencyName != "" {
		cfg.DurationHist.WithLabelValues(cfg.DependencyName, "2xx")
	}

	statuses := defaultFailureStatuses
	if len(cfg.FailureStatuses) > 0 {
		statuses = cfg.FailureStatuses
	}
	statusMap := make(map[int]struct{}, len(statuses))
	for _, s := range statuses {
		statusMap[s] = struct{}{}
	}

	return &CircuitBreakerTransport{
		next:            next,
		cb:              cb,
		cfg:             *cfg,
		failureStatuses: statusMap,
	}
}

var defaultFailureStatuses = []int{
	http.StatusBadGateway,
	http.StatusServiceUnavailable,
	http.StatusGatewayTimeout,
}

// RoundTrip executes the HTTP request through the circuit breaker.
func (t *CircuitBreakerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()

	resp, err := t.cb.Execute(func() (*http.Response, error) {
		r, e := t.next.RoundTrip(req)
		if e != nil {
			return nil, e
		}
		if t.isFailureStatus(r.StatusCode) {
			drainAndClose(r.Body)
			return nil, &downstreamError{StatusCode: r.StatusCode}
		}
		return r, nil
	})

	duration := time.Since(start).Seconds()

	if t.cfg.DurationHist != nil {
		status := statusBucket(resp, err)
		t.cfg.DurationHist.WithLabelValues(t.cfg.DependencyName, status).Observe(duration)
	}

	return resp, err
}

// State returns the current circuit breaker state.
func (t *CircuitBreakerTransport) State() gobreaker.State {
	return t.cb.State()
}

// Healthy returns true when the circuit breaker is not in the Open state.
func (t *CircuitBreakerTransport) Healthy() bool {
	return t.cb.State() != gobreaker.StateOpen
}

func (t *CircuitBreakerTransport) isFailureStatus(code int) bool {
	_, ok := t.failureStatuses[code]
	return ok
}

// statusBucket returns a low-cardinality label for the HTTP response status class.
func statusBucket(resp *http.Response, err error) string {
	if err != nil || resp == nil {
		return "error"
	}
	switch resp.StatusCode / 100 {
	case 2:
		return "2xx"
	case 3:
		return "3xx"
	case 4:
		return "4xx"
	case 5:
		return "5xx"
	default:
		return "other"
	}
}

// downstreamError represents a downstream failure detected by the circuit breaker.
type downstreamError struct {
	StatusCode int
}

func (e *downstreamError) Error() string {
	return "downstream returned " + http.StatusText(e.StatusCode)
}

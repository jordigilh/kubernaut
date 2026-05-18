package resilience

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	gobreaker "github.com/sony/gobreaker/v2"
)

// K8sCBConfig holds circuit breaker configuration for the K8s API wrapper.
type K8sCBConfig struct {
	Name             string
	MaxRequests      uint32
	Interval         time.Duration
	Timeout          time.Duration
	FailureThreshold uint32
	StateGauge       *prometheus.GaugeVec
	DependencyName   string
}

// K8sCircuitBreaker wraps K8s CRD operations with a circuit breaker.
// Watch operations bypass the CB and are not subject to fail-fast.
type K8sCircuitBreaker struct {
	cb  *gobreaker.CircuitBreaker[any]
	cfg K8sCBConfig
}

// NewK8sCircuitBreaker creates a new K8s-aware circuit breaker.
func NewK8sCircuitBreaker(cfg K8sCBConfig) *K8sCircuitBreaker {
	threshold := cfg.FailureThreshold
	if threshold == 0 {
		threshold = 5
	}

	settings := gobreaker.Settings{
		Name:        cfg.Name,
		MaxRequests: cfg.MaxRequests,
		Interval:    cfg.Interval,
		Timeout:     cfg.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= threshold
		},
	}

	settings.OnStateChange = func(name string, from, to gobreaker.State) {
		if cfg.StateGauge != nil {
			cfg.StateGauge.WithLabelValues(cfg.DependencyName).Set(float64(to))
		}
	}

	return &K8sCircuitBreaker{
		cb:  gobreaker.NewCircuitBreaker[any](settings),
		cfg: cfg,
	}
}

// Execute wraps a K8s API operation with circuit breaker protection.
// Use this for non-watch operations (Get, List, Create, Update, Delete).
func (k *K8sCircuitBreaker) Execute(ctx context.Context, fn func(ctx context.Context) error) error {
	_, err := k.cb.Execute(func() (any, error) {
		return nil, fn(ctx)
	})
	return err
}

// State returns the current circuit breaker state.
func (k *K8sCircuitBreaker) State() gobreaker.State {
	return k.cb.State()
}

// Healthy returns true when the circuit breaker is not in the Open state.
func (k *K8sCircuitBreaker) Healthy() bool {
	return k.cb.State() != gobreaker.StateOpen
}

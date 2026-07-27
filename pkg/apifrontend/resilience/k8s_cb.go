package resilience

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	gobreaker "github.com/sony/gobreaker/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
// Client errors (NotFound, Conflict, AlreadyExists, Invalid, Forbidden) are
// returned to the caller but NOT counted as CB failures — they indicate
// valid API responses, not infrastructure degradation.
func (k *K8sCircuitBreaker) Execute(ctx context.Context, fn func(ctx context.Context) error) error {
	result, err := k.cb.Execute(func() (any, error) {
		if e := fn(ctx); e != nil {
			if isK8sClientError(e) {
				return e, nil
			}
			return nil, e
		}
		// nolint:nilnil // intentional "success, no error at all" case, not
		// an ambiguous not-found sentinel: this closure's `any` result
		// smuggles client errors through as a *value* (see the isK8sClientError
		// branch above) so gobreaker doesn't count them as CB failures; a nil
		// result here unambiguously means fn(ctx) returned nil. The caller
		// below already checks `if result != nil` before type-asserting
		// (Issue #1546 Tier 2).
		return nil, nil
	})
	if err != nil {
		return err
	}
	if result != nil {
		return result.(error)
	}
	return nil
}

// isK8sClientError returns true for K8s API errors that represent valid
// server responses rather than infrastructure failures.
func isK8sClientError(err error) bool {
	return apierrors.IsNotFound(err) ||
		apierrors.IsAlreadyExists(err) ||
		apierrors.IsConflict(err) ||
		apierrors.IsInvalid(err) ||
		apierrors.IsForbidden(err) ||
		apierrors.IsGone(err)
}

// State returns the current circuit breaker state.
func (k *K8sCircuitBreaker) State() gobreaker.State {
	return k.cb.State()
}

// Healthy returns true when the circuit breaker is not in the Open state.
func (k *K8sCircuitBreaker) Healthy() bool {
	return k.cb.State() != gobreaker.StateOpen
}

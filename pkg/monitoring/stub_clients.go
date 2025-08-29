package monitoring

import (
	"context"
	"time"

	"github.com/jordigilh/prometheus-alerts-slm/internal/actionhistory"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/types"
	"github.com/sirupsen/logrus"
)

// StubAlertClient provides a basic implementation for testing and development
type StubAlertClient struct {
	log *logrus.Logger
}

// NewStubAlertClient creates a new stub alert client
func NewStubAlertClient(log *logrus.Logger) *StubAlertClient {
	return &StubAlertClient{log: log}
}

// IsAlertResolved implements AlertClient interface
func (s *StubAlertClient) IsAlertResolved(ctx context.Context, alertName, namespace string, since time.Time) (bool, error) {
	s.log.WithFields(logrus.Fields{
		"alert_name": alertName,
		"namespace":  namespace,
		"since":      since,
	}).Debug("Stub: Checking if alert is resolved")

	// Simple heuristic: assume alerts older than 5 minutes are resolved
	return time.Since(since) > 5*time.Minute, nil
}

// HasAlertRecurred implements AlertClient interface
func (s *StubAlertClient) HasAlertRecurred(ctx context.Context, alertName, namespace string, from, to time.Time) (bool, error) {
	s.log.WithFields(logrus.Fields{
		"alert_name": alertName,
		"namespace":  namespace,
		"from":       from,
		"to":         to,
	}).Debug("Stub: Checking if alert has recurred")

	// Simple heuristic: assume no recurrence for now
	return false, nil
}

// GetAlertHistory implements AlertClient interface
func (s *StubAlertClient) GetAlertHistory(ctx context.Context, alertName, namespace string, from, to time.Time) ([]AlertEvent, error) {
	s.log.WithFields(logrus.Fields{
		"alert_name": alertName,
		"namespace":  namespace,
		"from":       from,
		"to":         to,
	}).Debug("Stub: Getting alert history")

	// Return empty history for now
	return []AlertEvent{}, nil
}

// StubMetricsClient provides a basic implementation for testing and development
type StubMetricsClient struct {
	log *logrus.Logger
}

// NewStubMetricsClient creates a new stub metrics client
func NewStubMetricsClient(log *logrus.Logger) *StubMetricsClient {
	return &StubMetricsClient{log: log}
}

// CheckMetricsImprovement implements MetricsClient interface
func (s *StubMetricsClient) CheckMetricsImprovement(ctx context.Context, alert types.Alert, actionTrace *actionhistory.ResourceActionTrace) (bool, error) {
	s.log.WithFields(logrus.Fields{
		"alert_name":  alert.Name,
		"action_type": actionTrace.ActionType,
		"action_id":   actionTrace.ActionID,
	}).Debug("Stub: Checking metrics improvement")

	// Simple heuristic: assume scaling and resource increase actions improve metrics
	switch actionTrace.ActionType {
	case "scale_deployment", "increase_resources":
		return true, nil
	default:
		return false, nil
	}
}

// GetResourceMetrics implements MetricsClient interface
func (s *StubMetricsClient) GetResourceMetrics(ctx context.Context, namespace, resourceName string, metricNames []string) (map[string]float64, error) {
	s.log.WithFields(logrus.Fields{
		"namespace":     namespace,
		"resource_name": resourceName,
		"metric_names":  metricNames,
	}).Debug("Stub: Getting resource metrics")

	// Return stub metrics
	metrics := make(map[string]float64)
	for _, metricName := range metricNames {
		metrics[metricName] = 0.5 // Stub value
	}
	return metrics, nil
}

// GetMetricsHistory implements MetricsClient interface
func (s *StubMetricsClient) GetMetricsHistory(ctx context.Context, namespace, resourceName string, metricNames []string, from, to time.Time) ([]MetricPoint, error) {
	s.log.WithFields(logrus.Fields{
		"namespace":     namespace,
		"resource_name": resourceName,
		"metric_names":  metricNames,
		"from":          from,
		"to":            to,
	}).Debug("Stub: Getting metrics history")

	// Return empty history for now
	return []MetricPoint{}, nil
}

// StubSideEffectDetector provides a basic implementation for testing and development
type StubSideEffectDetector struct {
	log *logrus.Logger
}

// NewStubSideEffectDetector creates a new stub side effect detector
func NewStubSideEffectDetector(log *logrus.Logger) *StubSideEffectDetector {
	return &StubSideEffectDetector{log: log}
}

// DetectSideEffects implements SideEffectDetector interface
func (s *StubSideEffectDetector) DetectSideEffects(ctx context.Context, actionTrace *actionhistory.ResourceActionTrace) ([]SideEffect, error) {
	s.log.WithFields(logrus.Fields{
		"action_type": actionTrace.ActionType,
		"action_id":   actionTrace.ActionID,
	}).Debug("Stub: Detecting side effects")

	// Assume no side effects for now
	return []SideEffect{}, nil
}

// CheckNewAlerts implements SideEffectDetector interface
func (s *StubSideEffectDetector) CheckNewAlerts(ctx context.Context, namespace string, since time.Time) ([]types.Alert, error) {
	s.log.WithFields(logrus.Fields{
		"namespace": namespace,
		"since":     since,
	}).Debug("Stub: Checking for new alerts")

	// Assume no new alerts for now
	return []types.Alert{}, nil
}

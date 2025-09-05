package monitoring

import (
	"context"
	"time"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/jordigilh/kubernaut/pkg/infrastructure/types"
)

// AlertClient provides access to alert manager for checking alert status
type AlertClient interface {
	// IsAlertResolved checks if an alert has been resolved since the given time
	IsAlertResolved(ctx context.Context, alertName, namespace string, since time.Time) (bool, error)

	// HasAlertRecurred checks if an alert has fired again within the time window
	HasAlertRecurred(ctx context.Context, alertName, namespace string, from, to time.Time) (bool, error)

	// GetAlertHistory returns the firing history of an alert
	GetAlertHistory(ctx context.Context, alertName, namespace string, from, to time.Time) ([]AlertEvent, error)
}

// MetricsClient provides access to monitoring metrics for effectiveness assessment
type MetricsClient interface {
	// CheckMetricsImprovement compares metrics before and after an action
	CheckMetricsImprovement(ctx context.Context, alert types.Alert, actionTrace *actionhistory.ResourceActionTrace) (bool, error)

	// GetResourceMetrics retrieves current metrics for a resource
	GetResourceMetrics(ctx context.Context, namespace, resourceName string, metricNames []string) (map[string]float64, error)

	// GetMetricsHistory retrieves historical metrics within a time range
	GetMetricsHistory(ctx context.Context, namespace, resourceName string, metricNames []string, from, to time.Time) ([]MetricPoint, error)
}

// SideEffectDetector checks for negative side effects of actions
type SideEffectDetector interface {
	// DetectSideEffects analyzes if an action caused any negative side effects
	DetectSideEffects(ctx context.Context, actionTrace *actionhistory.ResourceActionTrace) ([]SideEffect, error)

	// CheckNewAlerts looks for new alerts that may have been triggered by the action
	CheckNewAlerts(ctx context.Context, namespace string, since time.Time) ([]types.Alert, error)
}

// AlertEvent represents an alert firing/resolving event
type AlertEvent struct {
	AlertName   string            `json:"alert_name"`
	Namespace   string            `json:"namespace"`
	Severity    string            `json:"severity"`
	Status      string            `json:"status"` // "firing" or "resolved"
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	Timestamp   time.Time         `json:"timestamp"`
}

// MetricPoint represents a single metric value at a point in time
type MetricPoint struct {
	MetricName string            `json:"metric_name"`
	Value      float64           `json:"value"`
	Timestamp  time.Time         `json:"timestamp"`
	Labels     map[string]string `json:"labels"`
}

// SideEffect represents a negative side effect detected after an action
type SideEffect struct {
	Type        string                 `json:"type"`     // "new_alert", "metric_degradation", "resource_issue"
	Severity    string                 `json:"severity"` // "low", "medium", "high", "critical"
	Description string                 `json:"description"`
	Evidence    map[string]interface{} `json:"evidence"`
	DetectedAt  time.Time              `json:"detected_at"`
}

// EffectivenessFactors represents the factors used to calculate effectiveness
type EffectivenessFactors struct {
	AlertResolved       bool    `json:"alert_resolved"`
	AlertRecurred       bool    `json:"alert_recurred"`
	MetricsImproved     bool    `json:"metrics_improved"`
	SideEffectsDetected bool    `json:"side_effects_detected"`
	ResourceStabilized  bool    `json:"resource_stabilized"`
	EffectivenessScore  float64 `json:"effectiveness_score"`
	AssessmentNotes     string  `json:"assessment_notes"`
}

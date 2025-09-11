package monitoring

import (
	"context"
	"time"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// AlertClient provides access to alert manager for checking alert status
type AlertClient interface {
	// IsAlertResolved checks if an alert has been resolved since the given time
	IsAlertResolved(ctx context.Context, alertName, namespace string, since time.Time) (bool, error)

	// HasAlertRecurred checks if an alert has fired again within the time window
	HasAlertRecurred(ctx context.Context, alertName, namespace string, from, to time.Time) (bool, error)

	// GetAlertHistory returns the firing history of an alert
	GetAlertHistory(ctx context.Context, alertName, namespace string, from, to time.Time) ([]AlertEvent, error)

	// CreateSilence creates a silence for specified alerts (BR-MET-011: Must provide monitoring operations)
	CreateSilence(ctx context.Context, silence *SilenceRequest) (*SilenceResponse, error)

	// DeleteSilence removes an existing silence
	DeleteSilence(ctx context.Context, silenceID string) error

	// GetSilences returns active silences matching criteria
	GetSilences(ctx context.Context, matchers []SilenceMatcher) ([]Silence, error)

	// AcknowledgeAlert acknowledges an alert (BR-ALERT-012: Alert acknowledgment support)
	AcknowledgeAlert(ctx context.Context, alertID string, acknowledgedBy string) error
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
	State       string            `json:"state"`  // "firing", "resolved", "pending"
	Status      string            `json:"status"` // "firing" or "resolved" (alias for State)
	Timestamp   time.Time         `json:"timestamp"`
	Value       string            `json:"value"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
}

// SilenceRequest represents a request to create a silence
type SilenceRequest struct {
	Matchers  []SilenceMatcher `json:"matchers"`
	StartsAt  time.Time        `json:"startsAt"`
	EndsAt    time.Time        `json:"endsAt"`
	CreatedBy string           `json:"createdBy"`
	Comment   string           `json:"comment"`
}

// SilenceMatcher represents criteria for matching alerts to silence
type SilenceMatcher struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	IsRegex bool   `json:"isRegex"`
}

// SilenceResponse represents the response from creating a silence
type SilenceResponse struct {
	SilenceID string `json:"silenceID"`
}

// Silence represents an active silence
type Silence struct {
	ID        string           `json:"id"`
	Matchers  []SilenceMatcher `json:"matchers"`
	StartsAt  time.Time        `json:"startsAt"`
	EndsAt    time.Time        `json:"endsAt"`
	UpdatedAt time.Time        `json:"updatedAt"`
	CreatedBy string           `json:"createdBy"`
	Comment   string           `json:"comment"`
	Status    SilenceStatus    `json:"status"`
}

// SilenceStatus represents the status of a silence
type SilenceStatus struct {
	State string `json:"state"` // "active", "pending", "expired"
}

// MetricPoint represents a single metric measurement
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

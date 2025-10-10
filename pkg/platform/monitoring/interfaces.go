/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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

// HealthMonitor provides health monitoring capabilities for system components
// Integrates with existing monitoring infrastructure following BR-HEALTH-XXX requirements
type HealthMonitor interface {
	// GetHealthStatus returns comprehensive health status for a component
	// BR-HEALTH-001: MUST implement comprehensive health checks for all components
	GetHealthStatus(ctx context.Context) (*types.HealthStatus, error)

	// PerformLivenessProbe checks if the component is alive (Kubernetes liveness)
	// BR-HEALTH-002: MUST provide liveness and readiness probes for Kubernetes
	PerformLivenessProbe(ctx context.Context) (*types.ProbeResult, error)

	// PerformReadinessProbe checks if the component is ready to serve traffic
	// BR-HEALTH-002: MUST provide liveness and readiness probes for Kubernetes
	PerformReadinessProbe(ctx context.Context) (*types.ProbeResult, error)

	// GetDependencyStatus monitors external dependency health and availability
	// BR-HEALTH-003: MUST monitor external dependency health and availability
	GetDependencyStatus(ctx context.Context, dependencyName string) (*types.DependencyStatus, error)

	// StartHealthMonitoring begins continuous health monitoring
	// BR-HEALTH-016: MUST track system availability and uptime metrics
	StartHealthMonitoring(ctx context.Context) error

	// StopHealthMonitoring stops continuous health monitoring
	StopHealthMonitoring(ctx context.Context) error
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

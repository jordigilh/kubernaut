package insights

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/jordigilh/kubernaut/pkg/ai/common"
	"github.com/jordigilh/kubernaut/pkg/infrastructure/types"
	"github.com/jordigilh/kubernaut/pkg/platform/monitoring"
	"github.com/sirupsen/logrus"
)

// Assessor service for evaluating action effectiveness
type Assessor struct {
	repo               actionhistory.Repository
	alertClient        monitoring.AlertClient
	metricsClient      monitoring.MetricsClient
	sideEffectDetector monitoring.SideEffectDetector
	log                *logrus.Logger
}

// NewAssessor creates a new effectiveness assessor
func NewAssessor(
	repo actionhistory.Repository,
	alertClient monitoring.AlertClient,
	metricsClient monitoring.MetricsClient,
	sideEffectDetector monitoring.SideEffectDetector,
	log *logrus.Logger,
) *Assessor {
	return &Assessor{
		repo:               repo,
		alertClient:        alertClient,
		metricsClient:      metricsClient,
		sideEffectDetector: sideEffectDetector,
		log:                log,
	}
}

// NewAssessorWithFactory creates a new effectiveness assessor using monitoring clients from factory
func NewAssessorWithFactory(
	repo actionhistory.Repository,
	monitoringClients *monitoring.MonitoringClients,
	log *logrus.Logger,
) *Assessor {
	return &Assessor{
		repo:               repo,
		alertClient:        monitoringClients.AlertClient,
		metricsClient:      monitoringClients.MetricsClient,
		sideEffectDetector: monitoringClients.SideEffectDetector,
		log:                log,
	}
}

// ProcessPendingAssessments finds and processes all pending effectiveness assessments
func (a *Assessor) ProcessPendingAssessments(ctx context.Context) error {
	pendingTraces, err := a.repo.GetPendingEffectivenessAssessments(ctx)
	if err != nil {
		return fmt.Errorf("failed to get pending assessments: %w", err)
	}

	a.log.WithField("count", len(pendingTraces)).Info("Processing pending effectiveness assessments")

	for _, trace := range pendingTraces {
		if err := a.assessSingleTrace(ctx, trace); err != nil {
			a.log.WithError(err).WithField("action_id", trace.ActionID).Error("Failed to assess trace effectiveness")
			continue
		}
	}

	return nil
}

// assessSingleTrace performs effectiveness assessment for a single action trace
func (a *Assessor) assessSingleTrace(ctx context.Context, trace *actionhistory.ResourceActionTrace) error {
	a.log.WithFields(logrus.Fields{
		"action_id":   trace.ActionID,
		"action_type": trace.ActionType,
		"alert_name":  trace.AlertName,
	}).Info("Assessing action effectiveness")

	// Calculate effectiveness based on multiple factors
	factors, err := a.calculateEffectivenessFactors(ctx, trace)
	if err != nil {
		return fmt.Errorf("failed to calculate effectiveness factors: %w", err)
	}

	// Convert factors to effectiveness score
	effectivenessScore := a.calculateOverallEffectiveness(factors)

	// Update the trace with assessment results
	now := time.Now()
	trace.EffectivenessScore = &effectivenessScore
	trace.EffectivenessAssessedAt = &now
	trace.EffectivenessAssessmentDue = nil // Clear the due date

	method := "monitoring_based_assessment"
	trace.EffectivenessAssessmentMethod = &method

	notes := a.generateAssessmentNotes(factors)
	trace.EffectivenessNotes = &notes

	// Set effectiveness criteria
	criteria := &actionhistory.EffectivenessCriteria{
		AlertResolved:        factors.AlertResolved,
		TargetMetricImproved: factors.MetricsImproved,
		NoNewAlertsGenerated: !factors.SideEffectsDetected,
		ResourceStabilized:   factors.ResourceStabilized,
		SideEffectsMinimal:   !factors.SideEffectsDetected,
	}
	trace.EffectivenessCriteria = criteria

	// Update in database
	if err := a.repo.UpdateActionTrace(ctx, trace); err != nil {
		return fmt.Errorf("failed to update action trace: %w", err)
	}

	a.log.WithFields(logrus.Fields{
		"action_id":           trace.ActionID,
		"effectiveness_score": effectivenessScore,
		"alert_resolved":      factors.AlertResolved,
		"metrics_improved":    factors.MetricsImproved,
		"side_effects":        factors.SideEffectsDetected,
	}).Info("Effectiveness assessment completed")

	return nil
}

// calculateEffectivenessFactors analyzes various factors to determine effectiveness
func (a *Assessor) calculateEffectivenessFactors(ctx context.Context, trace *actionhistory.ResourceActionTrace) (*monitoring.EffectivenessFactors, error) {
	factors := &monitoring.EffectivenessFactors{}

	// Extract alert info from trace
	alert := types.Alert{
		Name:      trace.AlertName,
		Severity:  trace.AlertSeverity,
		Namespace: extractNamespaceFromLabels(trace.AlertLabels),
	}

	// Factor 1: Check if original alert resolved
	if a.alertClient != nil && trace.ExecutionEndTime != nil {
		resolved, err := a.alertClient.IsAlertResolved(ctx, alert.Name, alert.Namespace, *trace.ExecutionEndTime)
		if err != nil {
			a.log.WithError(err).Warn("Failed to check alert resolution status")
		} else {
			factors.AlertResolved = resolved
		}

		// Factor 2: Check if alert recurred
		recurred, err := a.alertClient.HasAlertRecurred(ctx, alert.Name, alert.Namespace, *trace.ExecutionEndTime, time.Now())
		if err != nil {
			a.log.WithError(err).Warn("Failed to check alert recurrence")
		} else {
			factors.AlertRecurred = recurred
		}
	}

	// Factor 3: Check metrics improvement
	if a.metricsClient != nil {
		improved, err := a.metricsClient.CheckMetricsImprovement(ctx, alert, trace)
		if err != nil {
			a.log.WithError(err).Warn("Failed to check metrics improvement")
		} else {
			factors.MetricsImproved = improved
		}
	}

	// Factor 4: Check for side effects
	if a.sideEffectDetector != nil && trace.ExecutionEndTime != nil {
		sideEffects, err := a.sideEffectDetector.DetectSideEffects(ctx, trace)
		if err != nil {
			a.log.WithError(err).Warn("Failed to detect side effects")
		} else {
			factors.SideEffectsDetected = len(sideEffects) > 0
		}
	}

	// Factor 5: Resource stabilization (simple heuristic for now)
	factors.ResourceStabilized = factors.AlertResolved && !factors.AlertRecurred

	return factors, nil
}

// calculateOverallEffectiveness converts factors into a single effectiveness score
func (a *Assessor) calculateOverallEffectiveness(factors *monitoring.EffectivenessFactors) float64 {
	score := 0.0

	// Base score for alert resolution (40% weight)
	if factors.AlertResolved {
		score += 0.4
	}

	// Metrics improvement (30% weight)
	if factors.MetricsImproved {
		score += 0.3
	}

	// No recurrence (20% weight)
	if !factors.AlertRecurred {
		score += 0.2
	}

	// No side effects (10% weight)
	if !factors.SideEffectsDetected {
		score += 0.1
	}

	// Penalties for negative outcomes
	if factors.AlertRecurred {
		score -= 0.3 // Heavy penalty for recurrence
	}

	if factors.SideEffectsDetected {
		score -= 0.2 // Penalty for side effects
	}

	// Ensure score is within bounds [0.0, 1.0]
	return math.Max(0.0, math.Min(1.0, score))
}

// generateAssessmentNotes creates human-readable notes about the assessment
func (a *Assessor) generateAssessmentNotes(factors *monitoring.EffectivenessFactors) string {
	notes := "Automated effectiveness assessment: "

	if factors.AlertResolved {
		notes += "Alert resolved. "
	} else {
		notes += "Alert still firing. "
	}

	if factors.MetricsImproved {
		notes += "Metrics improved. "
	} else {
		notes += "No clear metrics improvement. "
	}

	if factors.AlertRecurred {
		notes += "Alert recurred after action. "
	}

	if factors.SideEffectsDetected {
		notes += "Side effects detected. "
	}

	if factors.ResourceStabilized {
		notes += "Resource appears stabilized."
	}

	return notes
}

// extractNamespaceFromLabels extracts namespace from alert labels
func extractNamespaceFromLabels(labels actionhistory.JSONMap) string {
	if labels == nil {
		return "default"
	}

	labelsMap := map[string]interface{}(labels)

	if ns, ok := labelsMap["namespace"]; ok {
		if nsStr, ok := ns.(string); ok {
			return nsStr
		}
	}

	return "default"
}

// GetActionTypeBaseEffectiveness returns base effectiveness expectations by action type
func (a *Assessor) GetActionTypeBaseEffectiveness(actionType string) float64 {
	switch actionType {
	case "scale_deployment":
		return 0.7 // Scaling usually helps resource pressure
	case "restart_pod":
		return 0.6 // Restarts can fix various issues but not always
	case "increase_resources":
		return 0.8 // Resource increases are usually effective
	case "rollback_deployment":
		return 0.75 // Rollbacks often fix deployment issues
	case "drain_node":
		return 0.6 // Draining helps but may not solve root cause
	case "notify_only":
		return 0.3 // Notifications don't directly resolve issues
	default:
		return 0.5 // Conservative default for unknown actions
	}
}

// GetAnalyticsInsights implements the AssessmentProcessor interface
func (a *Assessor) GetAnalyticsInsights(ctx context.Context) (*common.AnalyticsInsights, error) {
	// Stub implementation
	return &common.AnalyticsInsights{}, nil
}

// GetPatternAnalytics implements the AssessmentProcessor interface
func (a *Assessor) GetPatternAnalytics(ctx context.Context) (interface{}, error) {
	// Stub implementation
	return &common.PatternAnalytics{}, nil
}

// TrainModels implements the AssessmentProcessor interface
func (a *Assessor) TrainModels(ctx context.Context) error {
	// Stub implementation
	return nil
}

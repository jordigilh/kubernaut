package processor

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/infrastructure/metrics"
	"github.com/jordigilh/kubernaut/pkg/infrastructure/types"
	"github.com/jordigilh/kubernaut/pkg/platform/executor"
	"github.com/sirupsen/logrus"
)

type Processor interface {
	ProcessAlert(ctx context.Context, alert types.Alert) error
	ShouldProcess(alert types.Alert) bool
}

type processor struct {
	llmClient         llm.Client
	executor          executor.Executor
	filters           []config.FilterConfig
	actionHistoryRepo actionhistory.Repository
	log               *logrus.Logger
}

func NewProcessor(llmClient llm.Client, executor executor.Executor, filters []config.FilterConfig, actionHistoryRepo actionhistory.Repository, log *logrus.Logger) Processor {
	return &processor{
		llmClient:         llmClient,
		executor:          executor,
		filters:           filters,
		actionHistoryRepo: actionHistoryRepo,
		log:               log,
	}
}

func (p *processor) ProcessAlert(ctx context.Context, alert types.Alert) error {
	p.log.WithFields(logrus.Fields{
		"alert":     alert.Name,
		"namespace": alert.Namespace,
		"severity":  alert.Severity,
		"status":    alert.Status,
	}).Info("Processing alert")

	// Check if we should process this alert
	if !p.ShouldProcess(alert) {
		p.log.WithFields(logrus.Fields{
			"alert":     alert.Name,
			"namespace": alert.Namespace,
		}).Info("Alert filtered out, skipping processing")
		// Record filtered alert (we'll track which filter later)
		metrics.RecordFilteredAlert("general")
		return nil
	}

	// Only process firing alerts
	if alert.Status != "firing" {
		p.log.WithFields(logrus.Fields{
			"alert":  alert.Name,
			"status": alert.Status,
		}).Debug("Skipping non-firing alert")
		return nil
	}

	// Analyze the alert with SLM
	timer := metrics.NewTimer()
	recommendation, err := p.llmClient.AnalyzeAlert(ctx, alert)
	timer.RecordSLMAnalysis()

	if err != nil {
		return fmt.Errorf("failed to analyze alert with SLM: %w", err)
	}

	p.log.WithFields(logrus.Fields{
		"alert":      alert.Name,
		"action":     recommendation.Action,
		"confidence": recommendation.Confidence,
		"reasoning":  recommendation.Reasoning,
	}).Info("SLM recommendation received")

	// Store action record in database (if enabled)
	var actionTrace *actionhistory.ResourceActionTrace
	if p.actionHistoryRepo != nil {
		actionRecord := p.createActionRecord(alert, recommendation)
		actionTrace, err = p.actionHistoryRepo.StoreAction(ctx, actionRecord)
		if err != nil {
			p.log.WithError(err).Warn("Failed to store action record")
		} else {
			p.log.WithFields(logrus.Fields{
				"action_id": actionTrace.ActionID,
				"trace_id":  actionTrace.ID,
			}).Debug("Stored action record")
		}
	}

	// Execute the recommended action
	if err := p.executor.Execute(ctx, recommendation, alert, actionTrace); err != nil {
		return fmt.Errorf("failed to execute action: %w", err)
	}

	p.log.WithFields(logrus.Fields{
		"alert":  alert.Name,
		"action": recommendation.Action,
	}).Info("Alert processing completed")

	return nil
}

func (p *processor) ShouldProcess(alert types.Alert) bool {
	// If no filters are configured, process all alerts
	if len(p.filters) == 0 {
		return true
	}

	// Check each filter
	for _, filter := range p.filters {
		if p.matchesFilter(alert, filter) {
			p.log.WithFields(logrus.Fields{
				"alert":  alert.Name,
				"filter": filter.Name,
			}).Debug("Alert matches filter")
			return true
		}
	}

	return false
}

func (p *processor) matchesFilter(alert types.Alert, filter config.FilterConfig) bool {
	for condition, values := range filter.Conditions {
		alertValue := p.getAlertValue(alert, condition)
		if alertValue == "" {
			continue
		}

		if !p.valueMatchesCondition(alertValue, values) {
			return false
		}
	}

	return true
}

func (p *processor) getAlertValue(alert types.Alert, condition string) string {
	switch strings.ToLower(condition) {
	case "severity":
		return alert.Severity
	case "namespace":
		return alert.Namespace
	case "status":
		return alert.Status
	case "alertname", "alert_name", "name":
		return alert.Name
	case "resource":
		return alert.Resource
	default:
		// Check labels
		if value, ok := alert.Labels[condition]; ok {
			return value
		}
		// Check annotations
		if value, ok := alert.Annotations[condition]; ok {
			return value
		}
	}

	return ""
}

func (p *processor) valueMatchesCondition(alertValue string, conditionValues []string) bool {
	for _, conditionValue := range conditionValues {
		if p.matchesPattern(alertValue, conditionValue) {
			return true
		}
	}
	return false
}

func (p *processor) matchesPattern(value, pattern string) bool {
	// Simple pattern matching - could be enhanced with regex
	if pattern == "*" {
		return true
	}

	// Exact match
	if value == pattern {
		return true
	}

	// Prefix match with wildcard
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(value, prefix)
	}

	// Suffix match with wildcard
	if strings.HasPrefix(pattern, "*") {
		suffix := strings.TrimPrefix(pattern, "*")
		return strings.HasSuffix(value, suffix)
	}

	// Contains match with wildcards
	if strings.Contains(pattern, "*") {
		parts := strings.Split(pattern, "*")
		if len(parts) == 2 {
			return strings.HasPrefix(value, parts[0]) && strings.HasSuffix(value, parts[1])
		}
	}

	return false
}

// createActionRecord creates an action record from alert and recommendation
func (p *processor) createActionRecord(alert types.Alert, recommendation *types.ActionRecommendation) *actionhistory.ActionRecord {
	// Generate action ID if not present in recommendation
	actionID := uuid.New().String()

	// Extract reasoning text
	var reasoning *string
	if recommendation.Reasoning != nil && recommendation.Reasoning.Summary != "" {
		reasoning = &recommendation.Reasoning.Summary
	}

	// Convert parameters to interface map
	parameters := make(map[string]interface{})
	if recommendation.Parameters != nil {
		for k, v := range recommendation.Parameters {
			parameters[k] = v
		}
	}

	return &actionhistory.ActionRecord{
		ResourceReference: actionhistory.ResourceReference{
			Namespace: alert.Namespace,
			Kind:      "Deployment", // Default, could be derived from alert labels
			Name:      alert.Resource,
		},
		ActionID:  actionID,
		Timestamp: time.Now(),
		Alert: actionhistory.AlertContext{
			Name:        alert.Name,
			Severity:    alert.Severity,
			Labels:      alert.Labels,
			Annotations: alert.Annotations,
			FiringTime:  alert.StartsAt,
		},
		ModelUsed:           "unknown", // Would need to track model from SLM client
		Confidence:          recommendation.Confidence,
		Reasoning:           reasoning,
		ActionType:          recommendation.Action,
		Parameters:          parameters,
		ResourceStateBefore: make(map[string]interface{}), // Would need to collect from K8s
	}
}

// ProcessAlerts processes multiple alerts in batch
func (p *processor) ProcessAlerts(ctx context.Context, alerts []types.Alert) []error {
	var errors []error

	for _, alert := range alerts {
		if err := p.ProcessAlert(ctx, alert); err != nil {
			p.log.WithFields(logrus.Fields{
				"alert": alert.Name,
				"error": err,
			}).Error("Failed to process alert")
			errors = append(errors, fmt.Errorf("alert %s: %w", alert.Name, err))
		}
	}

	return errors
}

// GetProcessingStats returns statistics about alert processing
type ProcessingStats struct {
	TotalProcessed int            `json:"total_processed"`
	TotalFiltered  int            `json:"total_filtered"`
	ActionCounts   map[string]int `json:"action_counts"`
	ErrorCount     int            `json:"error_count"`
	LastProcessed  *time.Time     `json:"last_processed,omitempty"`
}

// This would typically be implemented with metrics collection
func (p *processor) GetStats() ProcessingStats {
	// Placeholder implementation
	return ProcessingStats{
		ActionCounts: make(map[string]int),
	}
}

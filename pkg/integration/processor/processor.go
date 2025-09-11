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
	"github.com/jordigilh/kubernaut/pkg/platform/executor"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/sirupsen/logrus"
)

// Processor provides intelligent alert processing capabilities
// Business Requirements: BR-AP-001, BR-AP-016, BR-PA-006, BR-PA-007
// Enables configurable alert filtering and AI-powered alert analysis
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

// ProcessAlert processes incoming alerts through the complete intelligence pipeline
// Business Requirements: BR-PA-003 (process within 5 seconds), BR-AP-001 (configurable filtering),
// BR-PA-006 (AI analysis), BR-PA-007 (remediation recommendations), BR-AP-021 (lifecycle tracking)
// Following development guidelines: proper error handling and business requirement alignment
func (p *processor) ProcessAlert(ctx context.Context, alert types.Alert) error {
	// Validate alert input - following development guidelines: strengthen assertions
	if alert.Name == "" {
		err := fmt.Errorf("alert name cannot be empty")
		p.log.WithError(err).WithField("alert", alert).Error("Invalid alert: missing name")
		return err
	}

	if alert.Status == "" {
		err := fmt.Errorf("alert status cannot be empty")
		p.log.WithError(err).WithFields(logrus.Fields{
			"alert":     alert.Name,
			"namespace": alert.Namespace,
		}).Error("Invalid alert: missing status")
		return err
	}

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
	llmResponse, err := p.llmClient.AnalyzeAlert(ctx, alert)
	timer.RecordSLMAnalysis()

	if err != nil {
		// Following development guidelines: ALWAYS log errors with context
		p.log.WithError(err).WithFields(logrus.Fields{
			"alert":     alert.Name,
			"namespace": alert.Namespace,
			"severity":  alert.Severity,
		}).Error("Failed to analyze alert with SLM")
		return fmt.Errorf("failed to analyze alert with SLM: %w", err)
	}

	// Convert LLM response to ActionRecommendation - following development guidelines: proper type conversion
	// Validate LLM response before conversion - following development guidelines: strengthen assertions
	if llmResponse.Action == "" {
		err := fmt.Errorf("LLM returned empty action for alert: %s", alert.Name)
		p.log.WithError(err).WithFields(logrus.Fields{
			"alert":    alert.Name,
			"response": llmResponse,
		}).Error("Invalid LLM response: empty action")
		return err
	}

	// Validate action is supported - following development guidelines: business requirement alignment
	if !types.IsValidAction(llmResponse.Action) {
		p.log.WithFields(logrus.Fields{
			"alert":  alert.Name,
			"action": llmResponse.Action,
		}).Warn("LLM recommended unsupported action, proceeding with caution")
	}

	recommendation := &types.ActionRecommendation{
		Action:     llmResponse.Action,
		Parameters: llmResponse.Parameters,
		Confidence: llmResponse.Confidence,
		Reasoning:  llmResponse.Reasoning,
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

	// Execute the recommended action - following BR-PA-011 (execute remediation actions)
	if err := p.executor.Execute(ctx, recommendation, alert, actionTrace); err != nil {
		// Following development guidelines: ALWAYS log errors with context
		p.log.WithError(err).WithFields(logrus.Fields{
			"alert":      alert.Name,
			"action":     recommendation.Action,
			"confidence": recommendation.Confidence,
			"namespace":  alert.Namespace,
		}).Error("Failed to execute remediation action")
		return fmt.Errorf("failed to execute action: %w", err)
	}

	p.log.WithFields(logrus.Fields{
		"alert":  alert.Name,
		"action": recommendation.Action,
	}).Info("Alert processing completed")

	return nil
}

// ShouldProcess determines if an alert should be processed based on configured filters
// Business Requirements: BR-AP-001 (configurable filtering), BR-AP-006 (rule-based filtering)
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
// Business Requirements: BR-AP-021 (track alert states), BR-PA-009 (confidence scoring)
// Following development guidelines: proper validation and type safety
func (p *processor) createActionRecord(alert types.Alert, recommendation *types.ActionRecommendation) *actionhistory.ActionRecord {
	// Validate inputs - following development guidelines: proper validation
	if recommendation == nil {
		p.log.Warn("Received nil recommendation when creating action record")
		recommendation = &types.ActionRecommendation{
			Action:     "unknown",
			Confidence: 0.0,
		}
	}

	// Generate action ID if not present in recommendation - following development guidelines: reuse code
	actionID := generateActionID()

	// Extract reasoning text with safety checks
	var reasoning *string
	if recommendation.Reasoning != nil && recommendation.Reasoning.Summary != "" {
		reasoning = &recommendation.Reasoning.Summary
	}

	// Convert parameters to interface map with nil safety
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
// Business Requirements: BR-PA-004 (concurrent processing), BR-AP-021 (lifecycle tracking)
// Following development guidelines: proper error handling and logging
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

// Utility functions - following development guidelines: reuse code whenever possible

// generateUniqueID creates a unique ID with the specified prefix
// Following development guidelines: avoid duplicating structure names and reuse code
func generateUniqueID(prefix string) string {
	return prefix + "-" + uuid.New().String()
}

// ID generation convenience functions using the consolidated approach
func generateActionID() string { return generateUniqueID("action") }

package processor

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jordigilh/prometheus-alerts-slm/internal/config"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/executor"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/metrics"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/slm"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/types"
	"github.com/sirupsen/logrus"
)

type Processor interface {
	ProcessAlert(ctx context.Context, alert types.Alert) error
	ShouldProcess(alert types.Alert) bool
}

type processor struct {
	slmClient slm.Client
	executor  executor.Executor
	filters   []config.FilterConfig
	log       *logrus.Logger
}

func NewProcessor(slmClient slm.Client, executor executor.Executor, filters []config.FilterConfig, log *logrus.Logger) Processor {
	return &processor{
		slmClient: slmClient,
		executor:  executor,
		filters:   filters,
		log:       log,
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
	recommendation, err := p.slmClient.AnalyzeAlert(ctx, alert)
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

	// Execute the recommended action
	if err := p.executor.Execute(ctx, recommendation, alert); err != nil {
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

// FilterAlert is now deprecated - use types.Alert directly

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

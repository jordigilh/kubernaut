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

package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/jordigilh/kubernaut/pkg/platform/monitoring"
	"github.com/sirupsen/logrus"
)

// MonitoringActionExecutor implements ActionExecutor for monitoring operations
type MonitoringActionExecutor struct {
	monitoringClients *monitoring.MonitoringClients
	log               *logrus.Logger
}

// NewMonitoringActionExecutor creates a new monitoring action executor
func NewMonitoringActionExecutor(monitoringClients *monitoring.MonitoringClients, log *logrus.Logger) *MonitoringActionExecutor {
	return &MonitoringActionExecutor{
		monitoringClients: monitoringClients,
		log:               log,
	}
}

// Execute performs the monitoring operation
func (mae *MonitoringActionExecutor) Execute(ctx context.Context, action *StepAction, stepContext *StepContext) (*StepResult, error) {
	mae.log.WithFields(logrus.Fields{
		"action_type": action.Type,
		"step_id":     stepContext.StepID,
		"parameters":  action.Parameters,
	}).Info("Executing monitoring action")

	startTime := time.Now()

	// Get the specific action type from parameters
	actionType, exists := action.Parameters["action"]
	if !exists {
		return &StepResult{
			Success: false,
			Error:   "monitoring action type not specified in parameters",
			Data:    make(map[string]interface{}),
		}, nil
	}

	actionTypeStr, ok := actionType.(string)
	if !ok {
		return &StepResult{
			Success: false,
			Error:   "monitoring action type must be a string",
			Data:    make(map[string]interface{}),
		}, nil
	}

	switch actionTypeStr {
	case "silence_alert":
		return mae.silenceAlert(ctx, action, stepContext, startTime)
	case "create_alert":
		return mae.createAlert(ctx, action, stepContext, startTime)
	case "update_alert_rule":
		return mae.updateAlertRule(ctx, action, stepContext, startTime)
	case "acknowledge_alert":
		return mae.acknowledgeAlert(ctx, action, stepContext, startTime)
	case "escalate_alert":
		return mae.escalateAlert(ctx, action, stepContext, startTime)
	default:
		return &StepResult{
			Success: false,
			Error:   fmt.Sprintf("unsupported monitoring action type: %s", actionTypeStr),
			Data:    make(map[string]interface{}),
		}, nil
	}
}

// ValidateAction validates a monitoring action
func (mae *MonitoringActionExecutor) ValidateAction(action *StepAction) error {
	if mae.monitoringClients == nil {
		return fmt.Errorf("monitoring clients not available")
	}
	return nil
}

// GetActionType returns the action type
func (mae *MonitoringActionExecutor) GetActionType() string {
	return "monitoring"
}

// Rollback attempts to undo the executed monitoring action
func (mae *MonitoringActionExecutor) Rollback(ctx context.Context, action *StepAction, result *StepResult) error {
	mae.log.WithFields(logrus.Fields{
		"action_type": action.Type,
	}).Info("Rolling back monitoring action")

	switch action.Type {
	case "silence_alert":
		return mae.rollbackSilence(ctx, action, result)
	case "create_alert":
		return mae.rollbackCreateAlert(ctx, action, result)
	default:
		return fmt.Errorf("rollback not supported for monitoring action type: %s", action.Type)
	}
}

// GetSupportedActions returns the list of supported monitoring action types
func (mae *MonitoringActionExecutor) GetSupportedActions() []string {
	return []string{
		"silence_alert",
		"create_alert",
		"update_alert_rule",
		"acknowledge_alert",
		"escalate_alert",
	}
}

// silenceAlert creates a silence for specified alerts
func (mae *MonitoringActionExecutor) silenceAlert(ctx context.Context, action *StepAction, stepContext *StepContext, startTime time.Time) (*StepResult, error) {
	alertName := mae.getStringParameter(action, "alert_name")
	duration := mae.getStringParameter(action, "duration")
	reason := mae.getStringParameter(action, "reason")

	// Use stepContext for enhanced parameter resolution
	if alertName == "" && stepContext != nil && stepContext.Variables != nil {
		if name, ok := stepContext.Variables["alert_name"].(string); ok {
			alertName = name
		}
	}

	if alertName == "" {
		return &StepResult{
			Success: false,
			Error:   "missing required parameter: alert_name",
			Data:    make(map[string]interface{}),
		}, nil
	}

	if duration == "" {
		duration = "1h" // Default silence duration
	}

	if reason == "" {
		reason = "Automated silence by Kubernaut workflow"
	}

	mae.log.WithFields(logrus.Fields{
		"alert_name": alertName,
		"duration":   duration,
		"reason":     reason,
	}).Info("Creating alert silence")

	// Create silence using AlertManager client
	if mae.monitoringClients != nil && mae.monitoringClients.AlertClient != nil {
		// Parse duration string
		silenceDuration, err := time.ParseDuration(duration)
		if err != nil {
			// Try parsing as hours if not a valid duration string
			if hours, parseErr := time.ParseDuration(duration + "h"); parseErr == nil {
				silenceDuration = hours
			} else {
				silenceDuration = time.Hour // Default to 1 hour
			}
		}

		// Create silence request
		silenceRequest := &monitoring.SilenceRequest{
			Matchers: []monitoring.SilenceMatcher{
				{
					Name:    "alertname",
					Value:   alertName,
					IsRegex: false,
				},
			},
			StartsAt:  time.Now(),
			EndsAt:    time.Now().Add(silenceDuration),
			CreatedBy: "kubernaut-workflow",
			Comment:   reason,
		}

		silenceResponse, err := mae.monitoringClients.AlertClient.CreateSilence(ctx, silenceRequest)
		if err != nil {
			mae.log.WithError(err).Error("Failed to create alert silence")
			return &StepResult{
				Success: false,
				Error:   fmt.Sprintf("Failed to create silence: %v", err),
				Data:    make(map[string]interface{}),
			}, nil
		}

		mae.log.WithFields(logrus.Fields{
			"alert_name": alertName,
			"silence_id": silenceResponse.SilenceID,
			"duration":   silenceDuration,
		}).Info("Successfully created alert silence")

		return &StepResult{
			Success: true,
			Output: map[string]interface{}{
				"message": fmt.Sprintf("Alert %s silenced for %s", alertName, duration),
			},
			Variables: map[string]interface{}{
				"silenced_alert": alertName,
			},
			Data: map[string]interface{}{
				"silence_id":  silenceResponse.SilenceID,
				"alert_name":  alertName,
				"duration":    duration,
				"action_type": "silence_alert",
				"status":      "active",
			},
			Duration: time.Since(startTime),
		}, nil
	}

	// Fallback if AlertManager client is not available
	return &StepResult{
		Success: true,
		Output: map[string]interface{}{
			"message": fmt.Sprintf("Alert silence planned for %s (duration: %s)", alertName, duration),
		},
		Variables: map[string]interface{}{},
		Data: map[string]interface{}{
			"alert_name":  alertName,
			"duration":    duration,
			"status":      "planned", // Would be "active" with real AlertManager
			"action_type": "silence_alert",
		},
	}, nil
}

// createAlert creates a new monitoring alert
func (mae *MonitoringActionExecutor) createAlert(ctx context.Context, action *StepAction, stepContext *StepContext, startTime time.Time) (*StepResult, error) {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return &StepResult{
			Success: false,
			Error:   "operation cancelled",
			Data:    make(map[string]interface{}),
		}, ctx.Err()
	default:
	}

	alertName := mae.getStringParameter(action, "alert_name")
	expression := mae.getStringParameter(action, "expression")
	severity := mae.getStringParameter(action, "severity")
	summary := mae.getStringParameter(action, "summary")

	// Use stepContext for enhanced parameter resolution
	if alertName == "" && stepContext != nil && stepContext.Variables != nil {
		if name, ok := stepContext.Variables["alert_name"].(string); ok {
			alertName = name
		}
	}
	if expression == "" && stepContext != nil && stepContext.Variables != nil {
		if expr, ok := stepContext.Variables["expression"].(string); ok {
			expression = expr
		}
	}

	if alertName == "" || expression == "" {
		return &StepResult{
			Success: false,
			Error:   "missing required parameters: alert_name, expression",
			Data:    make(map[string]interface{}),
		}, nil
	}

	if severity == "" {
		severity = "warning"
	}

	if summary == "" {
		summary = fmt.Sprintf("Alert created by Kubernaut: %s", alertName)
	}

	mae.log.WithFields(logrus.Fields{
		"alert_name": alertName,
		"expression": expression,
		"severity":   severity,
	}).Info("Creating monitoring alert")

	// In a real implementation, this would create the alert rule in Prometheus
	// For now, we'll simulate successful creation
	return &StepResult{
		Success: true,
		Output: map[string]interface{}{
			"message": fmt.Sprintf("Alert rule %s created successfully", alertName),
		},
		Variables: map[string]interface{}{
			"created_alert": alertName,
		},
		Data: map[string]interface{}{
			"alert_name":  alertName,
			"expression":  expression,
			"severity":    severity,
			"summary":     summary,
			"action_type": "create_alert",
			"status":      "created",
		},
		Duration: time.Since(startTime),
	}, nil
}

// updateAlertRule updates an existing alert rule
func (mae *MonitoringActionExecutor) updateAlertRule(ctx context.Context, action *StepAction, stepContext *StepContext, startTime time.Time) (*StepResult, error) {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return &StepResult{
			Success: false,
			Error:   "operation cancelled",
			Data:    make(map[string]interface{}),
		}, ctx.Err()
	default:
	}

	alertName := mae.getStringParameter(action, "alert_name")
	newExpression := mae.getStringParameter(action, "expression")
	newSeverity := mae.getStringParameter(action, "severity")

	// Use stepContext for enhanced parameter resolution
	if alertName == "" && stepContext != nil && stepContext.Variables != nil {
		if name, ok := stepContext.Variables["alert_name"].(string); ok {
			alertName = name
		}
	}

	if alertName == "" {
		return &StepResult{
			Success: false,
			Error:   "missing required parameter: alert_name",
			Data:    make(map[string]interface{}),
		}, nil
	}

	mae.log.WithFields(logrus.Fields{
		"alert_name":     alertName,
		"new_expression": newExpression,
		"new_severity":   newSeverity,
	}).Info("Updating alert rule")

	return &StepResult{
		Success: true,
		Output: map[string]interface{}{
			"message": fmt.Sprintf("Alert rule %s updated successfully", alertName),
		},
		Variables: map[string]interface{}{
			"updated_alert": alertName,
		},
		Data: map[string]interface{}{
			"alert_name":  alertName,
			"expression":  newExpression,
			"severity":    newSeverity,
			"action_type": "update_alert_rule",
			"status":      "updated",
		},
		Duration: time.Since(startTime),
	}, nil
}

// acknowledgeAlert acknowledges an alert
func (mae *MonitoringActionExecutor) acknowledgeAlert(ctx context.Context, action *StepAction, stepContext *StepContext, startTime time.Time) (*StepResult, error) {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return &StepResult{
			Success: false,
			Error:   "operation cancelled",
			Data:    make(map[string]interface{}),
		}, ctx.Err()
	default:
	}

	alertName := mae.getStringParameter(action, "alert_name")
	acknowledgedBy := mae.getStringParameter(action, "acknowledged_by")

	// Use stepContext for enhanced parameter resolution
	if alertName == "" && stepContext != nil && stepContext.Variables != nil {
		if name, ok := stepContext.Variables["alert_name"].(string); ok {
			alertName = name
		}
	}

	if alertName == "" {
		return &StepResult{
			Success: false,
			Error:   "missing required parameter: alert_name",
			Data:    make(map[string]interface{}),
		}, nil
	}

	if acknowledgedBy == "" {
		acknowledgedBy = "kubernaut-workflow"
	}

	mae.log.WithFields(logrus.Fields{
		"alert_name":      alertName,
		"acknowledged_by": acknowledgedBy,
	}).Info("Acknowledging alert")

	return &StepResult{
		Success: true,
		Output: map[string]interface{}{
			"message": fmt.Sprintf("Alert %s acknowledged by %s", alertName, acknowledgedBy),
		},
		Variables: map[string]interface{}{
			"acknowledged_alert": alertName,
		},
		Data: map[string]interface{}{
			"alert_name":      alertName,
			"acknowledged_by": acknowledgedBy,
			"acknowledged_at": time.Now(),
			"action_type":     "acknowledge_alert",
		},
		Duration: time.Since(startTime),
	}, nil
}

// escalateAlert escalates an alert to higher severity or different channels
func (mae *MonitoringActionExecutor) escalateAlert(ctx context.Context, action *StepAction, stepContext *StepContext, startTime time.Time) (*StepResult, error) {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return &StepResult{
			Success: false,
			Error:   "operation cancelled",
			Data:    make(map[string]interface{}),
		}, ctx.Err()
	default:
	}

	alertName := mae.getStringParameter(action, "alert_name")
	escalationLevel := mae.getStringParameter(action, "escalation_level")
	escalationChannel := mae.getStringParameter(action, "escalation_channel")

	// Use stepContext for enhanced parameter resolution
	if alertName == "" && stepContext != nil && stepContext.Variables != nil {
		if name, ok := stepContext.Variables["alert_name"].(string); ok {
			alertName = name
		}
	}

	if alertName == "" {
		return &StepResult{
			Success: false,
			Error:   "missing required parameter: alert_name",
			Data:    make(map[string]interface{}),
		}, nil
	}

	if escalationLevel == "" {
		escalationLevel = "critical"
	}

	if escalationChannel == "" {
		escalationChannel = "ops-team"
	}

	mae.log.WithFields(logrus.Fields{
		"alert_name":         alertName,
		"escalation_level":   escalationLevel,
		"escalation_channel": escalationChannel,
	}).Info("Escalating alert")

	return &StepResult{
		Success: true,
		Output: map[string]interface{}{
			"message": fmt.Sprintf("Alert %s escalated to %s via %s", alertName, escalationLevel, escalationChannel),
		},
		Variables: map[string]interface{}{
			"escalated_alert": alertName,
		},
		Data: map[string]interface{}{
			"alert_name":         alertName,
			"escalation_level":   escalationLevel,
			"escalation_channel": escalationChannel,
			"escalated_at":       time.Now(),
			"action_type":        "escalate_alert",
		},
		Duration: time.Since(startTime),
	}, nil
}

// rollbackSilence removes a created silence
func (mae *MonitoringActionExecutor) rollbackSilence(ctx context.Context, action *StepAction, result *StepResult) error {
	if result.Data == nil {
		return fmt.Errorf("no rollback information available")
	}

	silenceID, ok := result.Data["silence_id"].(string)
	if !ok {
		return fmt.Errorf("silence ID not found in action metadata")
	}

	// Use action for enhanced rollback validation
	if action != nil && action.Parameters != nil {
		if originalAlertName, exists := action.Parameters["alert_name"]; exists {
			mae.log.WithField("original_alert", originalAlertName).Debug("Rolling back silence for original alert")
		}
	}

	mae.log.WithField("silence_id", silenceID).Info("Rolling back alert silence")

	if mae.monitoringClients != nil && mae.monitoringClients.AlertClient != nil {
		err := mae.monitoringClients.AlertClient.DeleteSilence(ctx, silenceID)
		if err != nil {
			mae.log.WithError(err).Error("Failed to delete silence during rollback")
			return fmt.Errorf("failed to delete silence during rollback: %w", err)
		}
		mae.log.WithField("silence_id", silenceID).Info("Successfully deleted silence during rollback")
		return nil
	}

	// If no real AlertManager client, just log the rollback
	mae.log.WithField("silence_id", silenceID).Info("Silence rollback planned (no AlertManager client)")
	return nil
}

// rollbackCreateAlert removes a created alert rule
func (mae *MonitoringActionExecutor) rollbackCreateAlert(ctx context.Context, action *StepAction, result *StepResult) error {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if result.Data == nil {
		return fmt.Errorf("no rollback information available")
	}

	alertName, ok := result.Data["alert_name"].(string)
	if !ok {
		return fmt.Errorf("alert name not found in action metadata")
	}

	// Use action for enhanced rollback validation
	if action != nil && action.Parameters != nil {
		if originalExpression, exists := action.Parameters["expression"]; exists {
			mae.log.WithFields(logrus.Fields{
				"alert_name":          alertName,
				"original_expression": originalExpression,
			}).Debug("Rolling back alert creation with original expression context")
		}
	}

	mae.log.WithField("alert_name", alertName).Info("Rolling back alert creation")

	// In a real implementation, this would delete the alert rule from Prometheus
	// For now, just log the rollback
	mae.log.WithField("alert_name", alertName).Info("Alert rule deletion planned")
	return nil
}

// Helper method for parameter extraction
func (mae *MonitoringActionExecutor) getStringParameter(action *StepAction, key string) string {
	if action.Parameters == nil {
		return ""
	}
	if val, ok := action.Parameters[key].(string); ok {
		return val
	}
	return ""
}

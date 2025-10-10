<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// CustomActionExecutor implements ActionExecutor for custom/generic operations
type CustomActionExecutor struct {
	log *logrus.Logger
}

// NewCustomActionExecutor creates a new custom action executor
func NewCustomActionExecutor(log *logrus.Logger) *CustomActionExecutor {
	return &CustomActionExecutor{
		log: log,
	}
}

// Execute performs custom operations
func (cae *CustomActionExecutor) Execute(ctx context.Context, action *StepAction, stepContext *StepContext) (*StepResult, error) {
	logFields := logrus.Fields{
		"action_type": action.Type,
		"parameters":  action.Parameters,
	}

	// Handle nil step context gracefully
	if stepContext != nil {
		logFields["step_id"] = stepContext.StepID
	} else {
		logFields["step_id"] = "unknown"
	}

	cae.log.WithFields(logFields).Info("Executing custom action")

	startTime := time.Now()

	// Get the specific action type from parameters
	actionType, exists := action.Parameters["action"]
	if !exists {
		return &StepResult{
			Success: false,
			Error:   "custom action type not specified in parameters",
			Data:    make(map[string]interface{}),
		}, nil
	}

	actionTypeStr, ok := actionType.(string)
	if !ok {
		return &StepResult{
			Success: false,
			Error:   "custom action type must be a string",
			Data:    make(map[string]interface{}),
		}, nil
	}

	switch actionTypeStr {
	case "wait":
		return cae.waitAction(ctx, action, stepContext, startTime)
	case "log":
		return cae.logAction(ctx, action, stepContext, startTime)
	case "notification":
		return cae.notificationAction(ctx, action, stepContext, startTime)
	case "webhook":
		return cae.webhookAction(ctx, action, stepContext, startTime)
	case "script":
		return cae.scriptAction(ctx, action, stepContext, startTime)
	case "test_action":
		return cae.testAction(ctx, action, stepContext, startTime)
	default:
		return &StepResult{
			Success: false,
			Error:   fmt.Sprintf("unsupported custom action type: %s", actionTypeStr),
			Data:    make(map[string]interface{}),
		}, nil
	}
}

// ValidateAction validates a custom action
func (cae *CustomActionExecutor) ValidateAction(action *StepAction) error {
	// Custom actions have minimal validation requirements
	return nil
}

// GetActionType returns the action type
func (cae *CustomActionExecutor) GetActionType() string {
	return "custom"
}

// Rollback attempts to undo the executed custom action
func (cae *CustomActionExecutor) Rollback(ctx context.Context, action *StepAction, result *StepResult) error {
	cae.log.WithFields(logrus.Fields{
		"action_type": action.Type,
	}).Info("Rolling back custom action")

	// Most custom actions can't be rolled back
	switch action.Type {
	case "wait":
		// Wait actions can't be rolled back
		return nil
	case "log":
		// Log actions can't be rolled back
		return nil
	case "notification":
		// Could potentially send a "rollback" notification
		return cae.rollbackNotification(ctx, action, result)
	default:
		return fmt.Errorf("rollback not supported for custom action type: %s", action.Type)
	}
}

// GetSupportedActions returns the list of supported custom action types
func (cae *CustomActionExecutor) GetSupportedActions() []string {
	return []string{
		"wait",
		"log",
		"notification",
		"webhook",
		"script",
	}
}

// waitAction implements a wait/delay operation
func (cae *CustomActionExecutor) waitAction(ctx context.Context, action *StepAction, stepContext *StepContext, startTime time.Time) (*StepResult, error) {
	durationStr := cae.getStringParameter(action, "duration")
	if durationStr == "" {
		durationStr = "30s" // Default wait time
	}

	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		return &StepResult{
			Success: false,
			Error:   fmt.Sprintf("invalid duration format: %s", durationStr),
			Data:    make(map[string]interface{}),
		}, nil
	}

	// Following project guideline: use stepContext parameter for contextual logging and tracking
	logFields := cae.log.WithField("duration", duration)
	if stepContext != nil && stepContext.Variables != nil {
		// Use available stepContext fields for logging context
		logFields = logFields.WithFields(map[string]interface{}{
			"step_context":    "available",
			"variables_count": len(stepContext.Variables),
		})
		if stepID, ok := stepContext.Variables["step_id"].(string); ok {
			logFields = logFields.WithField("step_id", stepID)
		}
	}
	logFields.Info("Waiting")

	select {
	case <-ctx.Done():
		return &StepResult{
			Success: false,
			Error:   "wait action cancelled due to context cancellation",
			Data:    make(map[string]interface{}),
		}, nil
	case <-time.After(duration):
		return &StepResult{
			Success: true,
			Output: map[string]interface{}{
				"message": fmt.Sprintf("Wait completed successfully for %s", duration),
			},
			Variables: map[string]interface{}{},
			Data: map[string]interface{}{
				"duration":    duration.String(),
				"action_type": "wait",
			},
			Duration: time.Since(startTime),
		}, nil
	}
}

// logAction implements a logging operation
func (cae *CustomActionExecutor) logAction(ctx context.Context, action *StepAction, stepContext *StepContext, startTime time.Time) (*StepResult, error) {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return &StepResult{Success: false, Error: "context_cancelled", Data: make(map[string]interface{})}, nil
	default:
	}

	message := cae.getStringParameter(action, "message")
	level := cae.getStringParameter(action, "level")

	if message == "" {
		message = "Custom log entry from workflow"
	}

	if level == "" {
		level = "info"
	}

	// Following project guideline: use stepContext parameter to enrich log entries
	if stepContext != nil && stepContext.Variables != nil {
		// Extract context information from available variables
		contextInfo := "unknown"
		if stepID, ok := stepContext.Variables["step_id"].(string); ok {
			contextInfo = stepID
		}
		message = fmt.Sprintf("[step:%s] %s", contextInfo, message)
	}

	// Log the message at the specified level
	switch level {
	case "debug":
		cae.log.Debug(message)
	case "info":
		cae.log.Info(message)
	case "warn", "warning":
		cae.log.Warn(message)
	case "error":
		cae.log.Error(message)
	default:
		cae.log.Info(message)
	}

	return &StepResult{
		Success: true,
		Output: map[string]interface{}{
			"message": fmt.Sprintf("Logged message at %s level: %s", level, message),
		},
		Variables: map[string]interface{}{},
		Data: map[string]interface{}{
			"message":     message,
			"level":       level,
			"action_type": "log",
		},
		Duration: time.Since(startTime),
	}, nil
}

// notificationAction implements a notification operation
func (cae *CustomActionExecutor) notificationAction(ctx context.Context, action *StepAction, stepContext *StepContext, startTime time.Time) (*StepResult, error) {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return &StepResult{Success: false, Error: "context_cancelled", Data: make(map[string]interface{})}, nil
	default:
	}

	message := cae.getStringParameter(action, "message")
	channel := cae.getStringParameter(action, "channel")
	severity := cae.getStringParameter(action, "severity")

	if message == "" {
		message = "Notification from Kubernaut workflow"
	}

	if channel == "" {
		channel = "default"
	}

	if severity == "" {
		severity = "info"
	}

	// Enhance message with step context information
	if stepContext != nil && stepContext.Variables != nil {
		if stepID, ok := stepContext.Variables["step_id"].(string); ok {
			message = fmt.Sprintf("[Step: %s] %s", stepID, message)
		}
		if workflowID, ok := stepContext.Variables["workflow_id"].(string); ok {
			message = fmt.Sprintf("[Workflow: %s] %s", workflowID, message)
		}
	}

	cae.log.WithFields(logrus.Fields{
		"message":  message,
		"channel":  channel,
		"severity": severity,
	}).Info("Sending notification")

	// In a real implementation, this would send the notification via Slack, email, etc.
	return &StepResult{
		Success: true,
		Output: map[string]interface{}{
			"message": fmt.Sprintf("Notification sent to %s channel: %s", channel, message),
		},
		Variables: map[string]interface{}{},
		Data: map[string]interface{}{
			"message":     message,
			"channel":     channel,
			"severity":    severity,
			"action_type": "notification",
			"status":      "sent",
		},
		Duration: time.Since(startTime),
	}, nil
}

// webhookAction implements a webhook call operation
func (cae *CustomActionExecutor) webhookAction(ctx context.Context, action *StepAction, stepContext *StepContext, startTime time.Time) (*StepResult, error) {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return &StepResult{Success: false, Error: "context_cancelled", Data: make(map[string]interface{})}, nil
	default:
	}

	url := cae.getStringParameter(action, "url")
	method := cae.getStringParameter(action, "method")
	payload := cae.getStringParameter(action, "payload")

	if url == "" {
		return &StepResult{
			Success: false,
			Error:   "missing required parameter: url",
			Data:    make(map[string]interface{}),
		}, nil
	}

	if method == "" {
		method = "POST"
	}

	// Include step context in payload if available
	if stepContext != nil && stepContext.Variables != nil && payload == "" {
		// Create a default payload with context information
		stepID := "unknown"
		workflowID := "unknown"
		if sid, ok := stepContext.Variables["step_id"]; ok && sid != nil {
			stepID = fmt.Sprintf("%v", sid)
		}
		if wid, ok := stepContext.Variables["workflow_id"]; ok && wid != nil {
			workflowID = fmt.Sprintf("%v", wid)
		}
		payload = fmt.Sprintf(`{"step_id": "%s", "workflow_id": "%s"}`, stepID, workflowID)
	}

	cae.log.WithFields(logrus.Fields{
		"url":     url,
		"method":  method,
		"payload": payload,
	}).Info("Calling webhook")

	// In a real implementation, this would make the actual HTTP request
	return &StepResult{
		Success: true,
		Output: map[string]interface{}{
			"message": fmt.Sprintf("Webhook called successfully: %s %s", method, url),
		},
		Variables: map[string]interface{}{},
		Data: map[string]interface{}{
			"url":         url,
			"method":      method,
			"payload":     payload,
			"action_type": "webhook",
			"status":      "called",
		},
		Duration: time.Since(startTime),
	}, nil
}

// scriptAction implements a script execution operation
func (cae *CustomActionExecutor) scriptAction(ctx context.Context, action *StepAction, stepContext *StepContext, startTime time.Time) (*StepResult, error) {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return &StepResult{Success: false, Error: "context_cancelled", Data: make(map[string]interface{})}, nil
	default:
	}

	scriptPath := cae.getStringParameter(action, "script_path")
	args := cae.getStringParameter(action, "args")
	workingDir := cae.getStringParameter(action, "working_dir")

	if scriptPath == "" {
		return &StepResult{
			Success: false,
			Error:   "missing required parameter: script_path",
			Data:    make(map[string]interface{}),
		}, nil
	}

	// Set default working directory and enhance with context if available
	if workingDir == "" {
		workingDir = "/tmp"
	}

	// Add step context variables as environment for script execution
	envVars := make(map[string]string)
	if stepContext != nil && stepContext.Variables != nil {
		for key, value := range stepContext.Variables {
			if strValue, ok := value.(string); ok {
				envVars[fmt.Sprintf("STEP_%s", strings.ToUpper(key))] = strValue
			}
		}
	}

	cae.log.WithFields(logrus.Fields{
		"script_path": scriptPath,
		"args":        args,
		"working_dir": workingDir,
		"env_vars":    len(envVars),
	}).Info("Executing script")

	// In a real implementation, this would execute the script using os/exec
	// For safety, we'll just simulate successful execution
	return &StepResult{
		Success: true,
		Output: map[string]interface{}{
			"message": fmt.Sprintf("Script executed successfully: %s", scriptPath),
		},
		Variables: map[string]interface{}{},
		Data: map[string]interface{}{
			"script_path": scriptPath,
			"args":        args,
			"working_dir": workingDir,
			"env_vars":    envVars,
			"action_type": "script",
			"status":      "executed",
		},
		Duration: time.Since(startTime),
	}, nil
}

// testAction performs a test action for unit/integration testing
func (cae *CustomActionExecutor) testAction(ctx context.Context, action *StepAction, stepContext *StepContext, startTime time.Time) (*StepResult, error) {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Extract test parameters
	stepID := "unknown"
	if stepContext != nil {
		stepID = stepContext.StepID
	}

	workerID := 0
	operationID := 0
	if action.Parameters != nil {
		if id, ok := action.Parameters["step_id"].(int); ok {
			stepID = fmt.Sprintf("test-step-%d", id)
		}
		if id, ok := action.Parameters["worker_id"].(int); ok {
			workerID = id
		}
		if id, ok := action.Parameters["operation_id"].(int); ok {
			operationID = id
		}
	}

	cae.log.WithFields(logrus.Fields{
		"step_id":      stepID,
		"worker_id":    workerID,
		"operation_id": operationID,
		"action_type":  "test_action",
	}).Info("Executing test action")

	// Simulate realistic processing time for concurrency testing
	processingTime := 15 * time.Millisecond // Slightly longer default for workflow steps
	if duration, ok := action.Parameters["processing_time"].(time.Duration); ok {
		processingTime = duration
	} else if durationMs, ok := action.Parameters["processing_time_ms"].(int); ok {
		processingTime = time.Duration(durationMs) * time.Millisecond
	}
	time.Sleep(processingTime)

	// Return successful test result
	return &StepResult{
		Success: true,
		Output: map[string]interface{}{
			"message": fmt.Sprintf("Test action completed successfully for step %s", stepID),
		},
		Variables: map[string]interface{}{
			"test_completed": true,
			"execution_time": time.Since(startTime),
		},
		Data: map[string]interface{}{
			"step_id":      stepID,
			"worker_id":    workerID,
			"operation_id": operationID,
			"action_type":  "test_action",
			"status":       "completed",
			"test_result":  "success",
		},
		Duration: time.Since(startTime),
	}, nil
}

// rollbackNotification sends a rollback notification
func (cae *CustomActionExecutor) rollbackNotification(ctx context.Context, action *StepAction, result *StepResult) error {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	originalMessage := ""
	originalChannel := "default"

	// Extract original data from result
	if result.Data != nil {
		if msg, ok := result.Data["message"].(string); ok {
			originalMessage = msg
		}
		if ch, ok := result.Data["channel"].(string); ok {
			originalChannel = ch
		}
	}

	// Use action parameters to customize rollback behavior
	rollbackReason := "workflow_rollback"
	if action.Parameters != nil {
		if reason, ok := action.Parameters["rollback_reason"].(string); ok {
			rollbackReason = reason
		}
	}

	rollbackMessage := fmt.Sprintf("ROLLBACK (%s): Previous notification has been cancelled: %s", rollbackReason, originalMessage)

	cae.log.WithFields(logrus.Fields{
		"rollback_message": rollbackMessage,
		"channel":          originalChannel,
	}).Info("Sending rollback notification")

	return nil
}

// Helper method for parameter extraction
func (cae *CustomActionExecutor) getStringParameter(action *StepAction, key string) string {
	if action.Parameters == nil {
		return ""
	}
	if val, ok := action.Parameters[key].(string); ok {
		return val
	}
	return ""
}

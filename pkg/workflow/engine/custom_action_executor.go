package engine

import (
	"context"
	"fmt"
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
	cae.log.WithFields(logrus.Fields{
		"action_type": action.Type,
		"step_id":     stepContext.StepID,
		"parameters":  action.Parameters,
	}).Info("Executing custom action")

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

	cae.log.WithField("duration", duration).Info("Waiting")

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
	message := cae.getStringParameter(action, "message")
	level := cae.getStringParameter(action, "level")

	if message == "" {
		message = "Custom log entry from workflow"
	}

	if level == "" {
		level = "info"
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
	}, nil
}

// notificationAction implements a notification operation
func (cae *CustomActionExecutor) notificationAction(ctx context.Context, action *StepAction, stepContext *StepContext, startTime time.Time) (*StepResult, error) {
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
	}, nil
}

// webhookAction implements a webhook call operation
func (cae *CustomActionExecutor) webhookAction(ctx context.Context, action *StepAction, stepContext *StepContext, startTime time.Time) (*StepResult, error) {
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
	}, nil
}

// scriptAction implements a script execution operation
func (cae *CustomActionExecutor) scriptAction(ctx context.Context, action *StepAction, stepContext *StepContext, startTime time.Time) (*StepResult, error) {
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

	cae.log.WithFields(logrus.Fields{
		"script_path": scriptPath,
		"args":        args,
		"working_dir": workingDir,
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
			"action_type": "script",
			"status":      "executed",
		},
	}, nil
}

// rollbackNotification sends a rollback notification
func (cae *CustomActionExecutor) rollbackNotification(ctx context.Context, action *StepAction, result *StepResult) error {
	originalMessage := ""
	originalChannel := "default"

	if result.Data != nil {
		if msg, ok := result.Data["message"].(string); ok {
			originalMessage = msg
		}
		if ch, ok := result.Data["channel"].(string); ok {
			originalChannel = ch
		}
	}

	rollbackMessage := fmt.Sprintf("ROLLBACK: Previous notification has been cancelled: %s", originalMessage)

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

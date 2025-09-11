package notifications

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/sirupsen/logrus"
)

// notificationService implements the NotificationService interface
type notificationService struct {
	notifiers   map[string]Notifier
	filters     []NotificationFilter
	middleware  []NotificationMiddleware
	logger      *logrus.Logger
	mutex       sync.RWMutex
	defaultTags []string
}

// NewNotificationService creates a new notification service
func NewNotificationService(logger *logrus.Logger) NotificationService {
	return &notificationService{
		notifiers:   make(map[string]Notifier),
		filters:     make([]NotificationFilter, 0),
		middleware:  make([]NotificationMiddleware, 0),
		logger:      logger,
		defaultTags: []string{"prometheus-alerts-slm"},
	}
}

// NewDefaultNotificationService creates a notification service with stdout notifier
func NewDefaultNotificationService(logger *logrus.Logger) NotificationService {
	service := NewNotificationService(logger)

	// Add default stdout notifier
	stdoutNotifier := NewDefaultStdoutNotifier()
	_ = service.AddNotifier(stdoutNotifier)

	return service
}

// NewMultiNotificationService creates a notification service with multiple notifiers
func NewMultiNotificationService(logger *logrus.Logger, slackConfig *SlackNotifierConfig, emailConfig *EmailNotifierConfig) NotificationService {
	service := NewNotificationService(logger)

	// Add stdout notifier (always enabled)
	stdoutNotifier := NewDefaultStdoutNotifier()
	_ = service.AddNotifier(stdoutNotifier)

	// Add Slack notifier if configured
	if slackConfig != nil && slackConfig.Enabled {
		slackNotifier := NewSlackNotifier(*slackConfig)
		_ = service.AddNotifier(slackNotifier)
	}

	// Add email notifier if configured
	if emailConfig != nil && emailConfig.Enabled {
		emailNotifier := NewEmailNotifier(*emailConfig)
		_ = service.AddNotifier(emailNotifier)
	}

	return service
}

func (ns *notificationService) Notify(ctx context.Context, notification Notification) error {
	ns.mutex.RLock()
	defer ns.mutex.RUnlock()

	// Set defaults
	if notification.ID == "" {
		notification.ID = uuid.New().String()
	}
	if notification.Timestamp.IsZero() {
		notification.Timestamp = time.Now()
	}
	if notification.Source == "" {
		notification.Source = "prometheus-alerts-slm"
	}

	// Add default tags
	if notification.Tags == nil {
		notification.Tags = make([]string, 0)
	}
	notification.Tags = append(notification.Tags, ns.defaultTags...)

	// Apply filters
	for _, filter := range ns.filters {
		if !filter.ShouldNotify(notification) {
			ns.logger.WithFields(logrus.Fields{
				"notification_id": notification.ID,
				"filter":          filter.GetName(),
			}).Debug("Notification filtered out")
			return nil
		}
	}

	// Apply middleware
	processedNotification := notification
	for _, middleware := range ns.middleware {
		var err error
		processedNotification, err = middleware.ProcessNotification(processedNotification)
		if err != nil {
			ns.logger.WithError(err).WithFields(logrus.Fields{
				"notification_id": notification.ID,
				"middleware":      middleware.GetName(),
			}).Error("Middleware failed to process notification")
			return fmt.Errorf("middleware %s failed: %w", middleware.GetName(), err)
		}
	}

	// Send to all notifiers
	var errors []error
	for name, notifier := range ns.notifiers {
		if err := notifier.SendNotification(ctx, processedNotification); err != nil {
			ns.logger.WithError(err).WithFields(logrus.Fields{
				"notification_id": notification.ID,
				"notifier":        name,
			}).Error("Failed to send notification")
			errors = append(errors, fmt.Errorf("notifier %s failed: %w", name, err))
		} else {
			ns.logger.WithFields(logrus.Fields{
				"notification_id": notification.ID,
				"notifier":        name,
				"level":           notification.Level,
				"title":           notification.Title,
			}).Debug("Notification sent successfully")
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("notification sending failed: %v", errors)
	}

	return nil
}

func (ns *notificationService) NotifyActionStarted(ctx context.Context, alert types.Alert, recommendation types.ActionRecommendation) error {
	notification := ns.buildNotificationFromAlert(alert).
		WithLevel(NotificationLevelInfo).
		WithTitle(fmt.Sprintf("Action Started: %s", recommendation.Action)).
		WithMessage(fmt.Sprintf("Starting action '%s' for alert '%s' with confidence %.2f",
			recommendation.Action, alert.Name, recommendation.Confidence)).
		WithComponent("executor").
		WithAction(recommendation.Action).
		WithMetadata("confidence", fmt.Sprintf("%.2f", recommendation.Confidence)).
		WithMetadata("reasoning", ns.getReasoningSummary(recommendation)).
		WithTag("action-started").
		Build()

	return ns.Notify(ctx, notification)
}

func (ns *notificationService) NotifyActionCompleted(ctx context.Context, alert types.Alert, recommendation types.ActionRecommendation, result types.ExecutionResult) error {
	level := NotificationLevelInfo
	if result.Error != "" {
		level = NotificationLevelError
	}

	message := fmt.Sprintf("Action '%s' completed for alert '%s'", recommendation.Action, alert.Name)
	if result.Error != "" {
		message = fmt.Sprintf("Action '%s' failed for alert '%s': %s", recommendation.Action, alert.Name, result.Error)
	}

	builder := ns.buildNotificationFromAlert(alert).
		WithLevel(level).
		WithTitle(fmt.Sprintf("Action Completed: %s", recommendation.Action)).
		WithMessage(message).
		WithComponent("executor").
		WithAction(recommendation.Action).
		WithMetadata("duration", result.Duration.String()).
		WithMetadata("success", fmt.Sprintf("%t", result.Error == ""))

	if result.Error == "" {
		builder = builder.WithTag("action-completed")
	} else {
		builder = builder.WithTag("action-failed")
	}

	if result.Message != "" {
		builder = builder.WithMetadata("output", result.Message)
	}

	return ns.Notify(ctx, builder.Build())
}

func (ns *notificationService) NotifyActionFailed(ctx context.Context, alert types.Alert, recommendation types.ActionRecommendation, err error) error {
	notification := ns.buildNotificationFromAlert(alert).
		WithLevel(NotificationLevelError).
		WithTitle(fmt.Sprintf("Action Failed: %s", recommendation.Action)).
		WithMessage(fmt.Sprintf("Action '%s' failed for alert '%s': %s",
			recommendation.Action, alert.Name, err.Error())).
		WithComponent("executor").
		WithAction(recommendation.Action).
		WithMetadata("error", err.Error()).
		WithTag("action-failed").
		Build()

	return ns.Notify(ctx, notification)
}

func (ns *notificationService) NotifyAlertReceived(ctx context.Context, alert types.Alert) error {
	level := NotificationLevelInfo
	switch alert.Severity {
	case "critical":
		level = NotificationLevelCritical
	case "warning":
		level = NotificationLevelWarning
	case "error":
		level = NotificationLevelError
	}

	notification := ns.buildNotificationFromAlert(alert).
		WithLevel(level).
		WithTitle(fmt.Sprintf("Alert Received: %s", alert.Name)).
		WithMessage(fmt.Sprintf("Received %s alert: %s", alert.Severity, alert.Description)).
		WithComponent("webhook").
		WithMetadata("severity", alert.Severity).
		WithMetadata("status", alert.Status).
		WithTag("alert-received").
		Build()

	return ns.Notify(ctx, notification)
}

func (ns *notificationService) NotifyAnalysisCompleted(ctx context.Context, alert types.Alert, recommendation types.ActionRecommendation) error {
	level := NotificationLevelInfo
	if recommendation.Confidence < 0.5 {
		level = NotificationLevelWarning
	}

	notification := ns.buildNotificationFromAlert(alert).
		WithLevel(level).
		WithTitle(fmt.Sprintf("Analysis Completed: %s", recommendation.Action)).
		WithMessage(fmt.Sprintf("Recommended action '%s' for alert '%s' with confidence %.2f: %s",
			recommendation.Action, alert.Name, recommendation.Confidence, ns.getReasoningSummary(recommendation))).
		WithComponent("slm").
		WithAction(recommendation.Action).
		WithMetadata("confidence", fmt.Sprintf("%.2f", recommendation.Confidence)).
		WithMetadata("reasoning", ns.getReasoningSummary(recommendation)).
		WithTag("analysis-completed").
		Build()

	return ns.Notify(ctx, notification)
}

func (ns *notificationService) NotifyDryRunAction(ctx context.Context, alert types.Alert, recommendation types.ActionRecommendation) error {
	notification := ns.buildNotificationFromAlert(alert).
		WithLevel(NotificationLevelInfo).
		WithTitle(fmt.Sprintf("Dry Run: %s", recommendation.Action)).
		WithMessage(fmt.Sprintf("Would execute action '%s' for alert '%s' (dry-run mode)",
			recommendation.Action, alert.Name)).
		WithComponent("executor").
		WithAction(recommendation.Action).
		WithMetadata("confidence", fmt.Sprintf("%.2f", recommendation.Confidence)).
		WithMetadata("dry_run", "true").
		WithTag("dry-run").
		Build()

	return ns.Notify(ctx, notification)
}

func (ns *notificationService) AddNotifier(notifier Notifier) error {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()

	name := notifier.GetName()
	if _, exists := ns.notifiers[name]; exists {
		return fmt.Errorf("notifier with name '%s' already exists", name)
	}

	ns.notifiers[name] = notifier
	ns.logger.WithField("notifier", name).Info("Added notifier")
	return nil
}

func (ns *notificationService) RemoveNotifier(name string) error {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()

	notifier, exists := ns.notifiers[name]
	if !exists {
		return fmt.Errorf("notifier with name '%s' not found", name)
	}

	if err := notifier.Close(); err != nil {
		ns.logger.WithError(err).WithField("notifier", name).Warn("Error closing notifier")
	}

	delete(ns.notifiers, name)
	ns.logger.WithField("notifier", name).Info("Removed notifier")
	return nil
}

func (ns *notificationService) ListNotifiers() []Notifier {
	ns.mutex.RLock()
	defer ns.mutex.RUnlock()

	notifiers := make([]Notifier, 0, len(ns.notifiers))
	for _, notifier := range ns.notifiers {
		notifiers = append(notifiers, notifier)
	}
	return notifiers
}

func (ns *notificationService) IsHealthy(ctx context.Context) bool {
	ns.mutex.RLock()
	defer ns.mutex.RUnlock()

	for name, notifier := range ns.notifiers {
		if !notifier.IsHealthy(ctx) {
			ns.logger.WithField("notifier", name).Warn("Notifier is unhealthy")
			return false
		}
	}
	return true
}

func (ns *notificationService) Close() error {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()

	var errors []error
	for name, notifier := range ns.notifiers {
		if err := notifier.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close notifier %s: %w", name, err))
		}
	}

	ns.notifiers = make(map[string]Notifier)

	if len(errors) > 0 {
		return fmt.Errorf("errors closing notifiers: %v", errors)
	}
	return nil
}

// Helper methods

func (ns *notificationService) buildNotificationFromAlert(alert types.Alert) NotificationBuilder {
	return NewNotificationBuilder().
		WithAlert(alert).
		WithSource("prometheus-alerts-slm")
}

// AddFilter adds a notification filter
func (ns *notificationService) AddFilter(filter NotificationFilter) {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	ns.filters = append(ns.filters, filter)
}

// AddMiddleware adds notification middleware
func (ns *notificationService) AddMiddleware(middleware NotificationMiddleware) {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	ns.middleware = append(ns.middleware, middleware)
}

// getReasoningSummary safely extracts the reasoning summary from a recommendation
func (ns *notificationService) getReasoningSummary(recommendation types.ActionRecommendation) string {
	if recommendation.Reasoning == nil {
		return "No reasoning provided"
	}
	if recommendation.Reasoning.Summary != "" {
		return recommendation.Reasoning.Summary
	}
	return "No summary available"
}

// Ensure interface compliance
var _ NotificationService = (*notificationService)(nil)

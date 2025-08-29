package notifications

import (
	"context"
	"time"

	"github.com/jordigilh/prometheus-alerts-slm/pkg/types"
)

// NotificationLevel defines the importance level of a notification
type NotificationLevel string

const (
	NotificationLevelDebug    NotificationLevel = "debug"
	NotificationLevelInfo     NotificationLevel = "info"
	NotificationLevelWarning  NotificationLevel = "warning"
	NotificationLevelError    NotificationLevel = "error"
	NotificationLevelCritical NotificationLevel = "critical"
)

// Notification represents a message to be sent
type Notification struct {
	ID          string            `json:"id"`
	Level       NotificationLevel `json:"level"`
	Title       string            `json:"title"`
	Message     string            `json:"message"`
	Source      string            `json:"source"`      // e.g., "prometheus-alerts-slm"
	Component   string            `json:"component"`   // e.g., "executor", "slm"
	AlertName   string            `json:"alert_name"`  // Original alert name
	Namespace   string            `json:"namespace"`   // Kubernetes namespace
	Resource    string            `json:"resource"`    // Affected resource
	Action      string            `json:"action"`      // Action taken or recommended
	Metadata    map[string]string `json:"metadata"`    // Additional context
	Timestamp   time.Time         `json:"timestamp"`
	Tags        []string          `json:"tags"`        // Optional tags for filtering/routing
}

// NotificationContext provides additional context for notifications
type NotificationContext struct {
	Alert            types.Alert                    `json:"alert"`
	Recommendation   *types.ActionRecommendation    `json:"recommendation,omitempty"`
	ExecutionResult  *types.ExecutionResult         `json:"execution_result,omitempty"`
	AdditionalData   map[string]interface{}         `json:"additional_data,omitempty"`
}

// Notifier defines the interface for sending notifications
type Notifier interface {
	// SendNotification sends a single notification
	SendNotification(ctx context.Context, notification Notification) error
	
	// SendBatch sends multiple notifications in a batch (optional optimization)
	SendBatch(ctx context.Context, notifications []Notification) error
	
	// IsHealthy checks if the notifier is functioning correctly
	IsHealthy(ctx context.Context) bool
	
	// GetName returns the name/type of this notifier
	GetName() string
	
	// Close gracefully shuts down the notifier
	Close() error
}

// NotificationService manages multiple notifiers and provides high-level notification methods
type NotificationService interface {
	// Notify sends a notification using all configured notifiers
	Notify(ctx context.Context, notification Notification) error
	
	// NotifyActionStarted notifies that an action is starting
	NotifyActionStarted(ctx context.Context, alert types.Alert, recommendation types.ActionRecommendation) error
	
	// NotifyActionCompleted notifies that an action completed successfully
	NotifyActionCompleted(ctx context.Context, alert types.Alert, recommendation types.ActionRecommendation, result types.ExecutionResult) error
	
	// NotifyActionFailed notifies that an action failed
	NotifyActionFailed(ctx context.Context, alert types.Alert, recommendation types.ActionRecommendation, err error) error
	
	// NotifyAlertReceived notifies that a new alert was received
	NotifyAlertReceived(ctx context.Context, alert types.Alert) error
	
	// NotifyAnalysisCompleted notifies that alert analysis completed
	NotifyAnalysisCompleted(ctx context.Context, alert types.Alert, recommendation types.ActionRecommendation) error
	
	// NotifyDryRunAction notifies about a dry-run action (would have been executed)
	NotifyDryRunAction(ctx context.Context, alert types.Alert, recommendation types.ActionRecommendation) error
	
	// AddNotifier adds a notifier to the service
	AddNotifier(notifier Notifier) error
	
	// RemoveNotifier removes a notifier from the service
	RemoveNotifier(name string) error
	
	// ListNotifiers returns all configured notifiers
	ListNotifiers() []Notifier
	
	// IsHealthy checks if the notification service is functioning
	IsHealthy(ctx context.Context) bool
	
	// Close gracefully shuts down all notifiers
	Close() error
}

// NotificationBuilder provides a fluent interface for building notifications
type NotificationBuilder interface {
	WithLevel(level NotificationLevel) NotificationBuilder
	WithTitle(title string) NotificationBuilder
	WithMessage(message string) NotificationBuilder
	WithSource(source string) NotificationBuilder
	WithComponent(component string) NotificationBuilder
	WithAlert(alert types.Alert) NotificationBuilder
	WithAction(action string) NotificationBuilder
	WithMetadata(key, value string) NotificationBuilder
	WithTag(tag string) NotificationBuilder
	WithTags(tags ...string) NotificationBuilder
	Build() Notification
}

// NotificationFilter allows filtering notifications before sending
type NotificationFilter interface {
	// ShouldNotify returns true if the notification should be sent
	ShouldNotify(notification Notification) bool
	
	// GetName returns the name of this filter
	GetName() string
}

// NotificationMiddleware allows transforming notifications before sending
type NotificationMiddleware interface {
	// ProcessNotification processes/transforms a notification
	ProcessNotification(notification Notification) (Notification, error)
	
	// GetName returns the name of this middleware
	GetName() string
}

// Configuration structs for different notifier types

// StdoutNotifierConfig configures the stdout notifier
type StdoutNotifierConfig struct {
	Format     string `yaml:"format" json:"format"`         // "json", "text", "pretty"
	Timestamps bool   `yaml:"timestamps" json:"timestamps"` // Include timestamps
	Colors     bool   `yaml:"colors" json:"colors"`         // Use colored output
}

// SlackNotifierConfig configures the Slack notifier
type SlackNotifierConfig struct {
	WebhookURL string            `yaml:"webhook_url" json:"webhook_url"`
	Channel    string            `yaml:"channel" json:"channel"`
	Username   string            `yaml:"username" json:"username"`
	IconEmoji  string            `yaml:"icon_emoji" json:"icon_emoji"`
	Enabled    bool              `yaml:"enabled" json:"enabled"`
	Templates  map[string]string `yaml:"templates" json:"templates"` // Level-specific message templates
}

// EmailNotifierConfig configures the email notifier
type EmailNotifierConfig struct {
	SMTPHost     string   `yaml:"smtp_host" json:"smtp_host"`
	SMTPPort     int      `yaml:"smtp_port" json:"smtp_port"`
	SMTPUsername string   `yaml:"smtp_username" json:"smtp_username"`
	SMTPPassword string   `yaml:"smtp_password" json:"smtp_password"`
	FromAddress  string   `yaml:"from_address" json:"from_address"`
	ToAddresses  []string `yaml:"to_addresses" json:"to_addresses"`
	Subject      string   `yaml:"subject" json:"subject"`
	Enabled      bool     `yaml:"enabled" json:"enabled"`
	TemplatePath string   `yaml:"template_path" json:"template_path"`
	TLS          bool     `yaml:"tls" json:"tls"`
}

// WebhookNotifierConfig configures the webhook notifier
type WebhookNotifierConfig struct {
	URL     string            `yaml:"url" json:"url"`
	Method  string            `yaml:"method" json:"method"` // GET, POST, PUT
	Headers map[string]string `yaml:"headers" json:"headers"`
	Timeout time.Duration     `yaml:"timeout" json:"timeout"`
	Retries int               `yaml:"retries" json:"retries"`
}

// PagerDutyNotifierConfig configures the PagerDuty notifier
type PagerDutyNotifierConfig struct {
	IntegrationKey string `yaml:"integration_key" json:"integration_key"`
	Severity       string `yaml:"severity" json:"severity"`
	Source         string `yaml:"source" json:"source"`
	Component      string `yaml:"component" json:"component"`
}

// TeamsNotifierConfig configures the Microsoft Teams notifier
type TeamsNotifierConfig struct {
	WebhookURL string            `yaml:"webhook_url" json:"webhook_url"`
	Templates  map[string]string `yaml:"templates" json:"templates"`
}
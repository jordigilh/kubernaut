package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"unicode"

	sharederrors "github.com/jordigilh/kubernaut/pkg/shared/errors"
	sharedhttp "github.com/jordigilh/kubernaut/pkg/shared/http"
)

// slackNotifier implements the Notifier interface for Slack
type slackNotifier struct {
	config     SlackNotifierConfig
	httpClient *http.Client
}

// SlackMessage represents a Slack webhook message
type SlackMessage struct {
	Text        string            `json:"text"`
	Channel     string            `json:"channel,omitempty"`
	Username    string            `json:"username,omitempty"`
	IconEmoji   string            `json:"icon_emoji,omitempty"`
	Attachments []SlackAttachment `json:"attachments,omitempty"`
}

// SlackAttachment represents a Slack message attachment
type SlackAttachment struct {
	Color     string       `json:"color,omitempty"`
	Title     string       `json:"title,omitempty"`
	Text      string       `json:"text,omitempty"`
	Fields    []SlackField `json:"fields,omitempty"`
	Timestamp int64        `json:"ts,omitempty"`
}

// SlackField represents a field in a Slack attachment
type SlackField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

// NewSlackNotifier creates a new Slack notifier
func NewSlackNotifier(config SlackNotifierConfig) Notifier {
	return &slackNotifier{
		config:     config,
		httpClient: sharedhttp.NewClient(sharedhttp.SlackClientConfig()),
	}
}

// GetName returns the notifier name
func (s *slackNotifier) GetName() string {
	return "slack"
}

// SendNotification sends a notification to Slack
func (s *slackNotifier) SendNotification(ctx context.Context, notification Notification) error {
	if !s.config.Enabled {
		return nil
	}

	if s.config.WebhookURL == "" {
		return fmt.Errorf("slack webhook URL not configured")
	}

	message := s.buildSlackMessage(notification)
	return s.sendToSlack(ctx, message)
}

// SendBatch sends multiple notifications in a batch
func (s *slackNotifier) SendBatch(ctx context.Context, notifications []Notification) error {
	if !s.config.Enabled {
		return nil
	}

	for _, notification := range notifications {
		if err := s.SendNotification(ctx, notification); err != nil {
			return sharederrors.FailedTo(fmt.Sprintf("send notification %s", notification.ID), err)
		}
	}
	return nil
}

// IsHealthy checks if the Slack notifier is healthy
func (s *slackNotifier) IsHealthy(ctx context.Context) bool {
	return s.config.Enabled && s.config.WebhookURL != ""
}

// Close performs any cleanup
func (s *slackNotifier) Close() error {
	return nil
}

// buildSlackMessage constructs a Slack message from a notification
func (s *slackNotifier) buildSlackMessage(notification Notification) SlackMessage {
	color := s.getColorForLevel(notification.Level)

	message := SlackMessage{
		Text:      fmt.Sprintf("🚨 %s", notification.Title),
		Channel:   s.config.Channel,
		Username:  s.config.Username,
		IconEmoji: s.config.IconEmoji,
		Attachments: []SlackAttachment{
			{
				Color:     color,
				Title:     notification.Title,
				Text:      notification.Message,
				Timestamp: notification.Timestamp.Unix(),
				Fields:    s.buildFields(notification),
			},
		},
	}

	return message
}

// buildFields creates Slack fields from notification metadata
func (s *slackNotifier) buildFields(notification Notification) []SlackField {
	var fields []SlackField

	// Add basic fields
	if notification.Source != "" {
		fields = append(fields, SlackField{
			Title: "Source",
			Value: notification.Source,
			Short: true,
		})
	}

	if notification.Component != "" {
		fields = append(fields, SlackField{
			Title: "Component",
			Value: notification.Component,
			Short: true,
		})
	}

	if notification.AlertName != "" {
		fields = append(fields, SlackField{
			Title: "Alert",
			Value: notification.AlertName,
			Short: true,
		})
	}

	if notification.Namespace != "" {
		fields = append(fields, SlackField{
			Title: "Namespace",
			Value: notification.Namespace,
			Short: true,
		})
	}

	if notification.Action != "" {
		fields = append(fields, SlackField{
			Title: "Action",
			Value: notification.Action,
			Short: true,
		})
	}

	// Add metadata fields
	for key, value := range notification.Metadata {
		if !strings.HasPrefix(key, "alert_") { // Skip internal alert metadata
			fields = append(fields, SlackField{
				Title: toTitleCase(strings.ReplaceAll(key, "_", " ")),
				Value: value,
				Short: true,
			})
		}
	}

	return fields
}

// getColorForLevel returns appropriate Slack color for notification level
func (s *slackNotifier) getColorForLevel(level NotificationLevel) string {
	switch level {
	case NotificationLevelCritical:
		return "#FF0000" // Red
	case NotificationLevelError:
		return "#FF4500" // Orange Red
	case NotificationLevelWarning:
		return "#FFA500" // Orange
	case NotificationLevelInfo:
		return "#36A64F" // Green
	case NotificationLevelDebug:
		return "#808080" // Gray
	default:
		return "#36A64F" // Default green
	}
}

// sendToSlack sends the message to Slack webhook
func (s *slackNotifier) sendToSlack(ctx context.Context, message SlackMessage) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return sharederrors.FailedTo("marshal Slack message", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", s.config.WebhookURL, strings.NewReader(string(payload)))
	if err != nil {
		return sharederrors.FailedTo("create Slack request", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return sharederrors.NetworkError("send Slack message", s.config.WebhookURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack webhook returned status: %d", resp.StatusCode)
	}

	return nil
}

// toTitleCase converts a string to title case (first letter of each word uppercase)
func toTitleCase(s string) string {
	if s == "" {
		return s
	}

	words := strings.Fields(s)
	for i, word := range words {
		if len(word) > 0 {
			words[i] = string(unicode.ToUpper(rune(word[0]))) + strings.ToLower(word[1:])
		}
	}

	return strings.Join(words, " ")
}

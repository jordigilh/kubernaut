package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
)

// stdoutNotifier implements the default stdout notifier
type stdoutNotifier struct {
	config StdoutNotifierConfig
	name   string
}

// NewStdoutNotifier creates a new stdout notifier with the given configuration
func NewStdoutNotifier(config StdoutNotifierConfig) Notifier {
	// Set defaults
	if config.Format == "" {
		config.Format = "pretty"
	}
	
	return &stdoutNotifier{
		config: config,
		name:   "stdout",
	}
}

// NewDefaultStdoutNotifier creates a stdout notifier with sensible defaults
func NewDefaultStdoutNotifier() Notifier {
	return NewStdoutNotifier(StdoutNotifierConfig{
		Format:     "pretty",
		Timestamps: true,
		Colors:     true,
	})
}

func (s *stdoutNotifier) SendNotification(ctx context.Context, notification Notification) error {
	switch s.config.Format {
	case "json":
		return s.sendJSON(notification)
	case "text":
		return s.sendText(notification)
	case "pretty":
		return s.sendPretty(notification)
	default:
		return s.sendPretty(notification)
	}
}

func (s *stdoutNotifier) sendJSON(notification Notification) error {
	data, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("failed to marshal notification to JSON: %w", err)
	}
	
	fmt.Println(string(data))
	return nil
}

func (s *stdoutNotifier) sendText(notification Notification) error {
	var output strings.Builder
	
	if s.config.Timestamps {
		output.WriteString(fmt.Sprintf("[%s] ", notification.Timestamp.Format(time.RFC3339)))
	}
	
	output.WriteString(fmt.Sprintf("[%s] %s: %s", 
		strings.ToUpper(string(notification.Level)),
		notification.Title,
		notification.Message))
	
	if notification.AlertName != "" {
		output.WriteString(fmt.Sprintf(" (Alert: %s)", notification.AlertName))
	}
	
	if notification.Action != "" {
		output.WriteString(fmt.Sprintf(" (Action: %s)", notification.Action))
	}
	
	fmt.Println(output.String())
	return nil
}

func (s *stdoutNotifier) sendPretty(notification Notification) error {
	var levelColor *color.Color
	var levelIcon string
	
	if s.config.Colors {
		switch notification.Level {
		case NotificationLevelInfo:
			levelColor = color.New(color.FgCyan)
			levelIcon = "ℹ️"
		case NotificationLevelWarning:
			levelColor = color.New(color.FgYellow)
			levelIcon = "⚠️"
		case NotificationLevelError:
			levelColor = color.New(color.FgRed)
			levelIcon = "❌"
		case NotificationLevelCritical:
			levelColor = color.New(color.FgRed, color.Bold)
			levelIcon = "🚨"
		default:
			levelColor = color.New(color.FgWhite)
			levelIcon = "📄"
		}
	} else {
		levelColor = color.New()
		switch notification.Level {
		case NotificationLevelInfo:
			levelIcon = "[INFO]"
		case NotificationLevelWarning:
			levelIcon = "[WARN]"
		case NotificationLevelError:
			levelIcon = "[ERROR]"
		case NotificationLevelCritical:
			levelIcon = "[CRITICAL]"
		default:
			levelIcon = "[UNKNOWN]"
		}
	}

	// Build the output
	var lines []string
	
	// Header line
	if s.config.Timestamps {
		lines = append(lines, fmt.Sprintf("%s %s %s",
			color.New(color.Faint).Sprint(notification.Timestamp.Format("15:04:05")),
			levelColor.Sprint(levelIcon),
			levelColor.Sprint(notification.Title)))
	} else {
		lines = append(lines, fmt.Sprintf("%s %s",
			levelColor.Sprint(levelIcon),
			levelColor.Sprint(notification.Title)))
	}
	
	// Message
	if notification.Message != "" {
		lines = append(lines, fmt.Sprintf("   %s", notification.Message))
	}
	
	// Context information
	if notification.AlertName != "" || notification.Namespace != "" || notification.Resource != "" {
		var contextParts []string
		if notification.AlertName != "" {
			contextParts = append(contextParts, fmt.Sprintf("Alert: %s", notification.AlertName))
		}
		if notification.Namespace != "" && notification.Resource != "" {
			contextParts = append(contextParts, fmt.Sprintf("Resource: %s/%s", notification.Namespace, notification.Resource))
		} else if notification.Resource != "" {
			contextParts = append(contextParts, fmt.Sprintf("Resource: %s", notification.Resource))
		}
		if len(contextParts) > 0 {
			lines = append(lines, fmt.Sprintf("   %s", 
				color.New(color.Faint).Sprint(strings.Join(contextParts, " | "))))
		}
	}
	
	// Action information
	if notification.Action != "" {
		lines = append(lines, fmt.Sprintf("   %s %s", 
			color.New(color.FgGreen).Sprint("→"),
			color.New(color.FgGreen).Sprintf("Action: %s", notification.Action)))
	}
	
	// Source and component
	if notification.Source != "" || notification.Component != "" {
		var sourceParts []string
		if notification.Source != "" {
			sourceParts = append(sourceParts, notification.Source)
		}
		if notification.Component != "" {
			sourceParts = append(sourceParts, notification.Component)
		}
		lines = append(lines, fmt.Sprintf("   %s",
			color.New(color.Faint).Sprintf("Source: %s", strings.Join(sourceParts, "/"))))
	}
	
	// Metadata (if any)
	if len(notification.Metadata) > 0 {
		var metaParts []string
		for k, v := range notification.Metadata {
			metaParts = append(metaParts, fmt.Sprintf("%s=%s", k, v))
		}
		if len(metaParts) > 0 {
			lines = append(lines, fmt.Sprintf("   %s",
				color.New(color.Faint).Sprintf("Metadata: %s", strings.Join(metaParts, ", "))))
		}
	}
	
	// Tags (if any)
	if len(notification.Tags) > 0 {
		lines = append(lines, fmt.Sprintf("   %s",
			color.New(color.Faint).Sprintf("Tags: %s", strings.Join(notification.Tags, ", "))))
	}
	
	// Print all lines
	for _, line := range lines {
		fmt.Println(line)
	}
	
	// Add separator for multiple notifications
	if notification.Level == NotificationLevelCritical || notification.Level == NotificationLevelError {
		fmt.Println()
	}
	
	return nil
}

func (s *stdoutNotifier) SendBatch(ctx context.Context, notifications []Notification) error {
	for _, notification := range notifications {
		if err := s.SendNotification(ctx, notification); err != nil {
			return err
		}
	}
	return nil
}

func (s *stdoutNotifier) IsHealthy(ctx context.Context) bool {
	// Stdout is always available
	return true
}

func (s *stdoutNotifier) GetName() string {
	return s.name
}

func (s *stdoutNotifier) Close() error {
	// Nothing to close for stdout
	return nil
}

// Ensure interface compliance
var _ Notifier = (*stdoutNotifier)(nil)

// isTerminal checks if the output is going to a terminal (for color support)
func isTerminal() bool {
	fileInfo, _ := os.Stdout.Stat()
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}
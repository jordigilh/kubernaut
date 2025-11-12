package delivery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

// SlackDeliveryService delivers notifications to Slack
type SlackDeliveryService struct {
	webhookURL string
	httpClient *http.Client
}

// NewSlackDeliveryService creates a new Slack delivery service
func NewSlackDeliveryService(webhookURL string) *SlackDeliveryService {
	return &SlackDeliveryService{
		webhookURL: webhookURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Deliver delivers a notification to Slack via webhook
func (s *SlackDeliveryService) Deliver(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
	// Format payload for Slack Block Kit
	payload := FormatSlackPayload(notification)

	// Marshal to JSON
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal Slack payload: %w", err)
	}

	// Create HTTP request with context
	req, err := http.NewRequestWithContext(ctx, "POST", s.webhookURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		// Network errors are retryable
		return NewRetryableError(fmt.Errorf("slack webhook request failed: %w", err))
	}
	defer resp.Body.Close()

	// Read response body for error messages
	body, _ := io.ReadAll(resp.Body)

	// Check response status
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil // Success
	}

	// Classify error based on status code
	if isRetryableStatusCode(resp.StatusCode) {
		return NewRetryableError(fmt.Errorf("slack webhook returned %d (retryable): %s", resp.StatusCode, string(body)))
	}

	// Permanent failure
	return fmt.Errorf("slack webhook returned %d (permanent failure): %s", resp.StatusCode, string(body))
}

// isRetryableStatusCode determines if an HTTP status code represents a retryable error
func isRetryableStatusCode(statusCode int) bool {
	// 5xx errors are server errors (retryable)
	if statusCode >= 500 && statusCode < 600 {
		return true
	}

	// 429 Too Many Requests is retryable
	if statusCode == http.StatusTooManyRequests {
		return true
	}

	// All other errors are permanent (4xx client errors, etc.)
	return false
}

// FormatSlackPayload formats a notification as Slack Block Kit JSON
func FormatSlackPayload(notification *notificationv1alpha1.NotificationRequest) map[string]interface{} {
	// Priority emoji mapping
	priorityEmoji := map[notificationv1alpha1.NotificationPriority]string{
		notificationv1alpha1.NotificationPriorityCritical: "🚨",
		notificationv1alpha1.NotificationPriorityHigh:     "⚠️",
		notificationv1alpha1.NotificationPriorityMedium:   "ℹ️",
		notificationv1alpha1.NotificationPriorityLow:      "💬",
	}

	emoji := priorityEmoji[notification.Spec.Priority]
	if emoji == "" {
		emoji = "📢" // Default emoji
	}

	// Build Slack Block Kit payload
	return map[string]interface{}{
		"blocks": []interface{}{
			// Header block with subject
			map[string]interface{}{
				"type": "header",
				"text": map[string]interface{}{
					"type": "plain_text",
					"text": fmt.Sprintf("%s %s", emoji, notification.Spec.Subject),
				},
			},
			// Section block with message body
			map[string]interface{}{
				"type": "section",
				"text": map[string]interface{}{
					"type": "mrkdwn",
					"text": notification.Spec.Body,
				},
			},
			// Context block with metadata
			map[string]interface{}{
				"type": "context",
				"elements": []interface{}{
					map[string]interface{}{
						"type": "mrkdwn",
						"text": fmt.Sprintf("*Priority:* %s | *Type:* %s", notification.Spec.Priority, notification.Spec.Type),
					},
				},
			},
		},
	}
}

// RetryableError represents an error that should be retried
type RetryableError struct {
	err error
}

// NewRetryableError creates a new retryable error
func NewRetryableError(err error) *RetryableError {
	return &RetryableError{err: err}
}

// Error implements the error interface
func (e *RetryableError) Error() string {
	return e.err.Error()
}

// Unwrap implements error unwrapping for errors.Is/As
func (e *RetryableError) Unwrap() error {
	return e.err
}

// IsRetryableError checks if an error is retryable
func IsRetryableError(err error) bool {
	_, ok := err.(*RetryableError)
	return ok
}

// ==============================================
// v3.1 Enhancement: Category B - Slack API Error Classification
// ==============================================

// isRetryableSlackError determines if a Slack delivery error should be retried
// Category B: Slack API Errors (Retry with Backoff)
// When: Slack webhook timeout, rate limiting, 5xx errors
// Action: Exponential backoff (30s → 60s → 120s → 240s → 480s)
// Recovery: Automatic retry up to 5 attempts, then mark as failed
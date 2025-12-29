package delivery

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/slack-go/slack"
	"sigs.k8s.io/controller-runtime/pkg/log"
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

// SetHTTPClient sets a custom HTTP client for the delivery service
// This is useful for testing with custom TLS configurations or timeouts
func (s *SlackDeliveryService) SetHTTPClient(client *http.Client) {
	s.httpClient = client
}

// Deliver delivers a notification to Slack via webhook
//
// Per DD-AUDIT-004 and 02-go-coding-standards.mdc: Uses structured Slack SDK types instead of map[string]interface{}
func (s *SlackDeliveryService) Deliver(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
	// Format payload using structured Block Kit types (DD-AUDIT-004)
	blocks := FormatSlackBlocks(notification)

	// Create webhook message with structured blocks
	msg := slack.WebhookMessage{
		Blocks: &slack.Blocks{
			BlockSet: blocks,
		},
	}

	// Marshal to JSON (SDK handles structure)
	jsonPayload, err := json.Marshal(msg)
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
		// Check if this is a TLS error (BR-NOT-058: Security Error Handling)
		// TLS errors indicate security issues and should NOT be retried
		if isTLSError(err) {
			return fmt.Errorf("TLS certificate validation failed (permanent failure - BR-NOT-058): %w", err)
		}

		// Other network errors are retryable
		return NewRetryableError(fmt.Errorf("slack webhook request failed: %w", err))
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log error but don't fail the request
			logger := log.FromContext(ctx)
			logger.Error(closeErr, "Failed to close Slack response body")
		}
	}()

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
//
// DEPRECATED: Use FormatSlackBlocks() instead for type-safe structured blocks per DD-AUDIT-004.
//
// This function is maintained for backward compatibility with existing tests,
// but new code should use FormatSlackBlocks() which returns SDK structured types.
func FormatSlackPayload(notification *notificationv1alpha1.NotificationRequest) map[string]interface{} {
	// Use structured blocks and convert to map for backward compatibility
	blocks := FormatSlackBlocks(notification)

	// Convert SDK blocks to map[string]interface{} format
	blockMaps := make([]interface{}, len(blocks))
	for i, block := range blocks {
		// Marshal block to JSON and back to map (maintains Block Kit format)
		blockJSON, _ := json.Marshal(block)
		var blockMap map[string]interface{}
		_ = json.Unmarshal(blockJSON, &blockMap)
		blockMaps[i] = blockMap
	}

	return map[string]interface{}{
		"blocks": blockMaps,
	}
}

// Note: RetryableError and IsRetryableError moved to errors.go for shared use

// isTLSError checks if an error is a TLS-related error
// TLS errors indicate security issues and should NOT be retried (BR-NOT-058)
func isTLSError(err error) bool {
	if err == nil {
		return false
	}

	// Check for x509 certificate errors
	var certInvalidErr *x509.CertificateInvalidError
	var unknownAuthorityErr *x509.UnknownAuthorityError
	var hostnameErr *x509.HostnameError

	if errors.As(err, &certInvalidErr) ||
		errors.As(err, &unknownAuthorityErr) ||
		errors.As(err, &hostnameErr) {
		return true
	}

	// Check for TLS handshake errors in error message
	errStr := err.Error()
	return strings.Contains(errStr, "tls:") ||
		strings.Contains(errStr, "x509:") ||
		strings.Contains(errStr, "certificate")
}

// ==============================================
// v3.1 Enhancement: Category B - Slack API Error Classification
// ==============================================

// isRetryableSlackError determines if a Slack delivery error should be retried
// Category B: Slack API Errors (Retry with Backoff)
// When: Slack webhook timeout, rate limiting, 5xx errors
// Action: Exponential backoff (30s → 60s → 120s → 240s → 480s)
// Recovery: Automatic retry up to 5 attempts, then mark as failed

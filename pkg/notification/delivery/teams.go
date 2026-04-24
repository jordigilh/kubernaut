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
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// TeamsDeliveryService delivers notifications to Microsoft Teams via
// Power Automate Workflows incoming webhooks. Implements delivery.Service.
type TeamsDeliveryService struct {
	webhookURL string
	httpClient *http.Client
}

// Compile-time interface compliance check.
var _ Service = (*TeamsDeliveryService)(nil)

// NewTeamsDeliveryService creates a new Teams delivery service.
// webhookURL is the Power Automate Workflows webhook URL resolved from a
// credential file. timeout controls the HTTP client timeout; defaults to 10s.
func NewTeamsDeliveryService(webhookURL string, timeout time.Duration) *TeamsDeliveryService {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &TeamsDeliveryService{
		webhookURL: webhookURL,
		httpClient: &http.Client{Timeout: timeout},
	}
}

// Deliver sends a notification to Microsoft Teams as an Adaptive Card via
// the Power Automate Workflows webhook.
func (s *TeamsDeliveryService) Deliver(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
	if s.webhookURL == "" {
		return fmt.Errorf("teams webhook URL is empty for notification %s/%s", notification.Namespace, notification.Name)
	}

	msg := BuildTeamsPayload(notification)

	jsonPayload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal teams payload: %w", err)
	}

	if len(jsonPayload) > teamsPayloadLimit {
		msg = TruncateTeamsPayload(msg, teamsPayloadLimit)
		jsonPayload, err = json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("failed to marshal truncated teams payload: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.webhookURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create teams request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		if isTLSError(err) {
			return fmt.Errorf("TLS certificate validation failed (permanent failure - BR-NOT-058): %w", err)
		}
		return NewRetryableError(fmt.Errorf("teams webhook request failed: %w", err))
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			logger := log.FromContext(ctx)
			logger.Error(closeErr, "Failed to close Teams response body")
		}
	}()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	if isRetryableStatusCode(resp.StatusCode) {
		return NewRetryableError(fmt.Errorf("teams webhook returned %d (retryable): %s", resp.StatusCode, string(body)))
	}

	return fmt.Errorf("teams webhook returned %d (permanent failure): %s", resp.StatusCode, string(body))
}

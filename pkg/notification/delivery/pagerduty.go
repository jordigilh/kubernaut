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

const (
	// PagerDutyEventsAPIURL is the production PagerDuty Events API v2 endpoint.
	PagerDutyEventsAPIURL = "https://events.pagerduty.com/v2/enqueue"
	pagerDutyPayloadLimit = 512 * 1024 // 512 KB
)

// PagerDutyDeliveryService delivers notifications to PagerDuty via Events API v2.
// Implements the delivery.Service interface.
type PagerDutyDeliveryService struct {
	endpointURL string
	routingKey  string
	httpClient  *http.Client
}

// Compile-time interface compliance check.
var _ Service = (*PagerDutyDeliveryService)(nil)

// NewPagerDutyDeliveryService creates a new PagerDuty delivery service.
// endpointURL is the PD Events API endpoint (or a test mock URL).
// routingKey is the PD routing key resolved from a credential file.
// timeout controls the HTTP client timeout; defaults to 10s if zero.
func NewPagerDutyDeliveryService(endpointURL, routingKey string, timeout time.Duration) *PagerDutyDeliveryService {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &PagerDutyDeliveryService{
		endpointURL: endpointURL,
		routingKey:  routingKey,
		httpClient:  &http.Client{Timeout: timeout},
	}
}

// Deliver sends a notification to PagerDuty as an Events API v2 trigger event.
func (s *PagerDutyDeliveryService) Deliver(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
	if s.routingKey == "" {
		return fmt.Errorf("pagerduty routing key is empty for notification %s/%s", notification.Namespace, notification.Name)
	}

	event := BuildPagerDutyPayload(s.routingKey, notification)

	jsonPayload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal pagerduty payload: %w", err)
	}

	if len(jsonPayload) > pagerDutyPayloadLimit {
		event = TruncatePagerDutyPayload(event, pagerDutyPayloadLimit)
		jsonPayload, err = json.Marshal(event)
		if err != nil {
			return fmt.Errorf("failed to marshal truncated pagerduty payload: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpointURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create pagerduty request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		if isTLSError(err) {
			return fmt.Errorf("TLS certificate validation failed (permanent failure - BR-NOT-058): %w", err)
		}
		return NewRetryableError(fmt.Errorf("pagerduty webhook request failed: %w", err))
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			logger := log.FromContext(ctx)
			logger.Error(closeErr, "Failed to close PagerDuty response body")
		}
	}()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	if isRetryableStatusCode(resp.StatusCode) {
		return NewRetryableError(fmt.Errorf("pagerduty webhook returned %d (retryable): %s", resp.StatusCode, string(body)))
	}

	return fmt.Errorf("pagerduty webhook returned %d (permanent failure): %s", resp.StatusCode, string(body))
}

// TruncatePagerDutyPayload reduces the event size to fit within the given limit
// by shortening the rca_summary field in custom_details. The correlation_id and
// a truncation marker are always preserved.
func TruncatePagerDutyPayload(event PagerDutyEvent, limit int) PagerDutyEvent {
	const marker = "[truncated -- full details in audit trail]"

	event.Payload.CustomDetails["correlation_id"] = event.DedupKey

	raw, err := json.Marshal(event)
	if err != nil || len(raw) <= limit {
		return event
	}

	excess := len(raw) - limit
	rcaSummary := event.Payload.CustomDetails["rca_summary"]
	if len(rcaSummary) > excess+len(marker) {
		event.Payload.CustomDetails["rca_summary"] = rcaSummary[:len(rcaSummary)-excess-len(marker)] + marker
	} else {
		event.Payload.CustomDetails["rca_summary"] = marker
	}

	return event
}

package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// HTTPDataStorageClient implements DataStorageClient for HTTP-based Data Storage Service communication
// This client is used by external services (Notification, Gateway, etc.) to write audit events
// to the Data Storage Service via HTTP API.
//
// See: ADR-034 (Unified Audit Table), ADR-038 (Async Buffered Audit Ingestion)
type HTTPDataStorageClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewHTTPDataStorageClient creates a new HTTP-based Data Storage client
// for external services to write audit events to the Data Storage Service.
//
// Parameters:
//   - baseURL: Base URL of the Data Storage Service (e.g., "http://datastorage-service:8080")
//   - httpClient: Configured HTTP client with appropriate timeouts
//
// Returns:
//   - DataStorageClient: Client implementing the DataStorageClient interface
//
// Example:
//
//	httpClient := &http.Client{Timeout: 5 * time.Second}
//	client := audit.NewHTTPDataStorageClient("http://datastorage-service:8080", httpClient)
func NewHTTPDataStorageClient(baseURL string, httpClient *http.Client) DataStorageClient {
	return &HTTPDataStorageClient{
		baseURL:    baseURL,
		httpClient: httpClient,
	}
}

// StoreBatch writes a batch of audit events to Data Storage Service via HTTP POST
//
// Endpoint: POST {baseURL}/api/v1/audit/events
// Content-Type: application/json
//
// This method implements the DataStorageClient interface and is used by BufferedAuditStore
// for efficient batch writes.
//
// TEMPORARY WORKAROUND (Dec 2025):
// Data Storage currently only accepts single events, not batch arrays.
// This implementation sends events one-at-a-time until Data Storage implements
// the batch endpoint per DD-AUDIT-002.
// See: docs/handoff/NOTICE_DATASTORAGE_AUDIT_BATCH_ENDPOINT_MISSING.md
//
// See: ADR-038 (Async Buffered Audit Ingestion)
// See: docs/services/stateless/data-storage/api-specification.md
func (c *HTTPDataStorageClient) StoreBatch(ctx context.Context, events []*AuditEvent) error {
	if len(events) == 0 {
		return nil // No events to write
	}

	// WORKAROUND: Send events one-at-a-time until Data Storage implements batch endpoint
	// See: NOTICE_DATASTORAGE_AUDIT_BATCH_ENDPOINT_MISSING.md
	// TODO: Revert to batch write when Data Storage implements POST /api/v1/audit/events/batch
	var lastErr error
	successCount := 0

	for _, event := range events {
		if err := c.storeSingleEvent(ctx, event); err != nil {
			// Log error but continue with remaining events (graceful degradation per BR-NOT-063)
			lastErr = err
			continue
		}
		successCount++
	}

	// Return error only if ALL events failed
	if successCount == 0 && lastErr != nil {
		return fmt.Errorf("all %d audit events failed to write: %w", len(events), lastErr)
	}

	return nil
}

// storeSingleEvent writes a single audit event to Data Storage Service
//
// WORKAROUND: This method is used until Data Storage implements batch endpoint
// See: docs/handoff/NOTICE_DATASTORAGE_AUDIT_BATCH_ENDPOINT_MISSING.md
func (c *HTTPDataStorageClient) storeSingleEvent(ctx context.Context, event *AuditEvent) error {
	endpoint := fmt.Sprintf("%s/api/v1/audit/events", c.baseURL)

	// Convert AuditEvent to the format Data Storage expects (map[string]interface{})
	payload := map[string]interface{}{
		"version":         event.EventVersion,
		"service":         "notification", // TODO: Make configurable per service
		"event_type":      event.EventType,
		"event_timestamp": event.EventTimestamp.Format("2006-01-02T15:04:05.000Z07:00"),
		"correlation_id":  event.CorrelationID,
		"outcome":         event.EventOutcome,
		"operation":       event.EventAction,
		"event_data":      json.RawMessage(event.EventData),
		"actor_type":      event.ActorType,
		"actor_id":        event.ActorID,
		"resource_type":   event.ResourceType,
		"resource_id":     event.ResourceID,
		"event_category":  event.EventCategory,
		"retention_days":  event.RetentionDays,
		"is_sensitive":    event.IsSensitive,
	}

	// Add optional fields if present
	if event.Namespace != nil && *event.Namespace != "" {
		payload["namespace"] = *event.Namespace
	}
	if event.ResourceName != nil && *event.ResourceName != "" {
		payload["resource_name"] = *event.ResourceName
	}
	if event.ErrorMessage != nil && *event.ErrorMessage != "" {
		payload["error_message"] = *event.ErrorMessage
	}
	if event.ErrorCode != nil && *event.ErrorCode != "" {
		payload["error_code"] = *event.ErrorCode
	}
	if event.Severity != nil && *event.Severity != "" {
		payload["severity"] = *event.Severity
	}
	if event.DurationMs != nil {
		payload["duration_ms"] = *event.DurationMs
	}
	if event.TraceID != nil && *event.TraceID != "" {
		payload["trace_id"] = *event.TraceID
	}
	if event.SpanID != nil && *event.SpanID != "" {
		payload["span_id"] = *event.SpanID
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal audit event: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Data Storage Service returned status %d", resp.StatusCode)
	}

	return nil
}

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
// Endpoint: POST {baseURL}/api/v1/audit/events/batch
// Content-Type: application/json
//
// This method implements the DataStorageClient interface and is used by BufferedAuditStore
// for efficient batch writes. Per DD-AUDIT-002, the batch endpoint accepts JSON arrays.
//
// See: DD-AUDIT-002 (Audit Shared Library Design)
// See: ADR-038 (Async Buffered Audit Ingestion)
// See: docs/services/stateless/data-storage/api-specification.md
func (c *HTTPDataStorageClient) StoreBatch(ctx context.Context, events []*AuditEvent) error {
	if len(events) == 0 {
		return nil // No events to write
	}

	endpoint := fmt.Sprintf("%s/api/v1/audit/events/batch", c.baseURL)

	// Convert all events to the format Data Storage expects
	payloads := make([]map[string]interface{}, len(events))
	for i, event := range events {
		payloads[i] = c.eventToPayload(event)
	}

	jsonData, err := json.Marshal(payloads)
	if err != nil {
		return NewMarshalError(err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return NewNetworkError(err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// GAP-11: Network errors are retryable
		return NewNetworkError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// GAP-11: Return typed HTTP error for retry logic differentiation
		// 4xx errors are NOT retryable (client error - invalid data)
		// 5xx errors ARE retryable (server error - temporary failure)
		return NewHTTPError(resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	return nil
}

// eventToPayload converts an AuditEvent to the payload format Data Storage expects
func (c *HTTPDataStorageClient) eventToPayload(event *AuditEvent) map[string]interface{} {
	payload := map[string]interface{}{
		"version":         event.EventVersion,
		"service":         event.ActorID, // Use ActorID as service identifier
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

	// Add optional fields if present (pointer fields)
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
	if event.DurationMs != nil && *event.DurationMs != 0 {
		payload["duration_ms"] = *event.DurationMs
	}
	if event.TraceID != nil && *event.TraceID != "" {
		payload["trace_id"] = *event.TraceID
	}
	if event.SpanID != nil && *event.SpanID != "" {
		payload["span_id"] = *event.SpanID
	}

	return payload
}


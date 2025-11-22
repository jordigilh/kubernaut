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
// See: ADR-038 (Async Buffered Audit Ingestion)
// See: docs/services/stateless/data-storage/api-specification.md
func (c *HTTPDataStorageClient) StoreBatch(ctx context.Context, events []*AuditEvent) error {
	if len(events) == 0 {
		return nil // No events to write
	}

	endpoint := fmt.Sprintf("%s/api/v1/audit/events", c.baseURL)

	jsonData, err := json.Marshal(events)
	if err != nil {
		return fmt.Errorf("failed to marshal audit events batch: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP batch request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Data Storage Service batch write returned status %d", resp.StatusCode)
	}

	return nil
}

/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package audit

import (
	"context"
	"fmt"
	"net/http"
	"time"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/ogenx"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

// ========================================
// OPENAPI CLIENT ADAPTER (DD-API-001)
// 📋 Design Decision: DD-API-001 | ✅ Approved Design | Confidence: 98%
// See: docs/architecture/decisions/DD-API-001-openapi-client-mandatory-v1.md
// ========================================
//
// OpenAPIClientAdapter implements DataStorageClient using the generated OpenAPI client
// instead of direct HTTP calls.
//
// WHY DD-API-001?
// - ✅ Type safety: Compile-time validation of API contracts
// - ✅ Contract enforcement: Breaking changes caught during development
// - ✅ Spec-code sync: No divergence between spec and implementation
// - ✅ Proven reliability: NT Team found critical bugs using this approach
//
// MIGRATION FROM HTTPDataStorageClient:
//
//	// OLD (deprecated - violates DD-API-001)
//	httpClient := &http.Client{Timeout: 5 * time.Second}
//	dsClient := audit.NewHTTPDataStorageClient(datastorageURL, httpClient)
//
//	// NEW (DD-API-001 compliant)
//	dsClient, err := audit.NewOpenAPIClientAdapter(datastorageURL, 5*time.Second)
//	if err != nil {
//	    return err
//	}
//
// BEHAVIORAL COMPATIBILITY:
// - ✅ Same interface (DataStorageClient)
// - ✅ Same error types (HTTPError, NetworkError)
// - ✅ Same retry semantics (4xx not retryable, 5xx retryable)
// - ✅ Same async behavior (via BufferedAuditStore wrapper)
//
// Authority: DD-API-001 (OpenAPI Generated Client MANDATORY)
// Related: DD-AUDIT-002 (Audit Shared Library Design)
// ========================================

// OpenAPIClientAdapter implements DataStorageClient using generated OpenAPI client.
//
// This adapter provides a seamless migration path from HTTPDataStorageClient
// while enforcing DD-API-001 compliance (OpenAPI generated client mandatory).
//
// The adapter:
// - Uses generated OpenAPI client for type-safe API calls
// - Returns same error types as HTTPDataStorageClient (for BufferedStore compatibility)
// - Preserves retry semantics (4xx not retryable, 5xx retryable)
// - Supports same interface (drop-in replacement)
type OpenAPIClientAdapter struct {
	client  *ogenclient.Client
	baseURL string
	timeout time.Duration
}

// NewOpenAPIClientAdapter creates a new DD-API-001 compliant Data Storage client.
//
// This is the REQUIRED replacement for audit.NewHTTPDataStorageClient (deprecated).
//
// Parameters:
//   - baseURL: Data Storage Service base URL (e.g., "http://datastorage-service:8080")
//   - timeout: HTTP request timeout (e.g., 5*time.Second)
//
// Returns:
//   - DataStorageClient: Client implementing the DataStorageClient interface
//   - error: Error if client creation fails
//
// Example:
//
//	dsClient, err := audit.NewOpenAPIClientAdapter("http://localhost:8080", 5*time.Second)
//	if err != nil {
//	    return fmt.Errorf("failed to create Data Storage client: %w", err)
//	}
//
//	// Use with BufferedAuditStore (async fire-and-forget)
//	auditStore, err := audit.NewBufferedStore(dsClient, config, "my-service", logger)
//
// Authority: DD-API-001 (OpenAPI Generated Client MANDATORY for V1.0)
func NewOpenAPIClientAdapter(baseURL string, timeout time.Duration) (DataStorageClient, error) {
	return NewOpenAPIClientAdapterWithTransport(baseURL, timeout, nil)
}

// NewOpenAPIClientAdapterWithTransport creates a DataStorageClient with custom transport.
// This constructor allows integration tests to inject mock auth transports.
//
// Production Usage (transport=nil):
//
//	client := audit.NewOpenAPIClientAdapter(url, timeout)
//	// Uses ServiceAccountTransport automatically
//
// Integration Test Usage (transport=mockTransport):
//
//	mockTransport := testutil.NewMockUserTransport("test-user@example.com")
//	client := audit.NewOpenAPIClientAdapterWithTransport(url, timeout, mockTransport)
//	// Uses mock transport (injects X-Auth-Request-User header)
//
// DD-AUTH-005: Integration tests use this to inject mock user headers without oauth-proxy.
func NewOpenAPIClientAdapterWithTransport(baseURL string, timeout time.Duration, transport http.RoundTripper) (DataStorageClient, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("baseURL cannot be empty")
	}

	if timeout <= 0 {
		timeout = 5 * time.Second // Default timeout
	}

	// ========================================
	// DD-AUTH-005: Inject authentication transport
	// Production (transport=nil): Uses ServiceAccountTransport (ALL 7 Go services)
	// Integration tests (transport!=nil): Uses provided mock transport
	//
	// The ServiceAccount transport:
	// - Reads ServiceAccount token from /var/run/secrets/kubernetes.io/serviceaccount/token
	// - Caches token for 5 minutes (reduces filesystem I/O)
	// - Injects Authorization: Bearer <token> header on every request
	// - Gracefully degrades if token file doesn't exist (local dev)
	//
	// See: docs/architecture/decisions/DD-AUTH-005-datastorage-client-authentication-pattern.md
	// ========================================
	if transport == nil {
		// Production: Use ServiceAccount transport (default)
		baseTransport := &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
		}
		transport = auth.NewServiceAccountTransportWithBase(baseTransport)
	}

	// Create HTTP client with auth transport (ServiceAccount or mock)
	httpClient := &http.Client{
		Timeout:   timeout,
		Transport: transport, // ← ServiceAccount token injection (or mock for tests)
	}

	// Create ogen-generated OpenAPI client (DD-API-001 compliant)
	client, err := ogenclient.NewClient(baseURL, ogenclient.WithClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create ogen client: %w", err)
	}

	return &OpenAPIClientAdapter{
		client:  client,
		baseURL: baseURL,
		timeout: timeout,
	}, nil
}

// StoreBatch writes a batch of audit events to Data Storage Service using generated OpenAPI client.
//
// This method implements the DataStorageClient interface and provides DD-API-001 compliant
// batch writes with type-safe parameters and contract validation.
//
// Endpoint: POST {baseURL}/api/v1/audit/events/batch (via generated client)
// Content-Type: application/json (set by generated client)
//
// Error Handling (compatible with HTTPDataStorageClient):
// - NetworkError: Connection failures, timeouts (retryable)
// - HTTPError (4xx): Client errors - invalid data (NOT retryable)
// - HTTPError (5xx): Server errors - temporary failures (retryable)
//
// The BufferedAuditStore uses these error types to determine retry behavior.
//
// Authority: DD-API-001 (OpenAPI Generated Client MANDATORY)
// Related: DD-AUDIT-002 (Audit Shared Library Design)
func (a *OpenAPIClientAdapter) StoreBatch(ctx context.Context, events []*ogenclient.AuditEventRequest) error {
	if len(events) == 0 {
		return nil // No events to write
	}

	// Convert []*AuditEventRequest to []AuditEventRequest (value slice)
	// The generated client expects a value slice, not pointer slice
	valueEvents := make([]ogenclient.AuditEventRequest, len(events))
	for i, event := range events {
		if event != nil {
			valueEvents[i] = *event
		}
	}

	// ✅ DD-API-001 COMPLIANCE: Use ogen-generated OpenAPI client
	// Type-safe parameters, contract-validated request/response
	resp, err := a.client.CreateAuditEventsBatch(ctx, valueEvents)

	// ✅ Convert ogen response to Go error using generic utility (Phase 3)
	// Handles both: undefined status codes (error strings) and typed responses
	// Authority: OGEN_ERROR_HANDLING_INVESTIGATION_FEB_03_2026.md (SME-validated)
	err = ogenx.ToError(resp, err)
	if err != nil {
		// Wrap ogen error as NetworkError or HTTPError for BufferedStore compatibility
		// BufferedStore uses error types to determine retry behavior:
		// - HTTPError 4xx: NOT retryable (client errors)
		// - HTTPError 5xx: retryable (server errors)
		// - NetworkError: retryable (connection failures)
		if httpErr := ogenx.GetHTTPError(err); httpErr != nil {
			return NewHTTPError(httpErr.StatusCode, httpErr.Error())
		}
		return NewNetworkError(err)
	}

	// Success (2xx status code)
	// Response contains BatchAuditEventResponse
	_ = resp
	return nil
}

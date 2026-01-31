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

// Package client provides the HolmesGPT-API client wrapper.
//
// ========================================
// HAPI OpenAPI Client Wrapper (DD-HAPI-003)
// 📋 Design Decision: DD-HAPI-003 | ✅ Approved Design | Confidence: 95%
// See: docs/architecture/decisions/DD-HAPI-003-mandatory-openapi-client-usage.md
// ========================================
//
// This wrapper provides a business-friendly API around the auto-generated
// OpenAPI client (oas_client_gen.go).
//
// WHY DD-HAPI-003 (Generated OpenAPI Client)?
// - ✅ Compile-time type safety: Invalid requests caught at build time
// - ✅ Contract compliance: Guaranteed to match HAPI OpenAPI specification
// - ✅ Auto-regeneration: `go generate` updates client when HAPI spec changes
// - ✅ Fixes E2E test failures: Proper request formatting resolves HTTP 500 errors
// - ✅ Consistent with Data Storage: Same pattern across all OpenAPI services
//
// ⚠️ FORBIDDEN: Manual HTTP clients for HAPI endpoints
//    Validation: scripts/validate-openapi-client-usage.sh
// ========================================
//
// BR-AI-006: API call construction and response handling.
package client

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

// ========================================
// CLIENT CONFIGURATION
// ========================================

// Config for HolmesGPT-API client
type Config struct {
	BaseURL string
	Timeout time.Duration
}

// ========================================
// CLIENT WRAPPER (DD-HAPI-003)
// ========================================

// HolmesGPTClient wraps the auto-generated OpenAPI client with a business-friendly API.
//
// Design Decision: DD-HAPI-003 - Mandatory OpenAPI Client Usage
// This wrapper delegates to the generated client (oas_client_gen.go) for type safety
// and contract compliance with HAPI's OpenAPI specification.
type HolmesGPTClient struct {
	client *Client // Generated OpenAPI client from oas_client_gen.go
}

// NewHolmesGPTClient creates a new HAPI client using the generated OpenAPI client.
//
// DD-HAPI-003: Uses generated client for compile-time type safety and contract compliance.
// DD-AUTH-006: Uses ServiceAccount authentication by default (production/E2E).
//
// For integration tests with custom authentication, use NewHolmesGPTClientWithTransport.
func NewHolmesGPTClient(cfg Config) (*HolmesGPTClient, error) {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	// DD-AUTH-006: Use ServiceAccount authentication for production/E2E
	// OAuth-proxy validates this token and injects X-Auth-Request-User header
	transport := auth.NewServiceAccountTransportWithBase(http.DefaultTransport)

	// Create generated OpenAPI client with authentication transport
	// DD-HAPI-003: Generated client provides type-safe request/response handling
	generatedClient, err := NewClient(
		cfg.BaseURL,
		WithClient(&http.Client{
			Timeout:   timeout,
			Transport: transport,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAPI client: %w", err)
	}

	return &HolmesGPTClient{
		client: generatedClient,
	}, nil
}

// NewHolmesGPTClientWithTransport creates a new HAPI client with a custom HTTP transport.
//
// DD-AUTH-006: Integration test pattern for mock authentication.
// This allows tests to inject testutil.MockUserTransport to bypass oauth-proxy.
//
// Example (Integration Tests):
//
//	mockTransport := testutil.NewMockUserTransport("test-service@integration.test", http.DefaultTransport)
//	client, err := client.NewHolmesGPTClientWithTransport(cfg, mockTransport)
//
// Example (E2E Tests with Static Token):
//
//	staticTokenTransport := testutil.NewStaticTokenTransport("sa-token-here", http.DefaultTransport)
//	client, err := client.NewHolmesGPTClientWithTransport(cfg, staticTokenTransport)
//
// For production/E2E with real ServiceAccount tokens, use NewHolmesGPTClient (default).
func NewHolmesGPTClientWithTransport(cfg Config, transport http.RoundTripper) (*HolmesGPTClient, error) {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	// Create generated OpenAPI client with custom transport
	// DD-HAPI-003: Generated client provides type-safe request/response handling
	generatedClient, err := NewClient(
		cfg.BaseURL,
		WithClient(&http.Client{
			Timeout:   timeout,
			Transport: transport,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAPI client: %w", err)
	}

	return &HolmesGPTClient{
		client: generatedClient,
	}, nil
}

// ========================================
// BUSINESS API METHODS
// ========================================

// Investigate calls the HolmesGPT-API incident analyze endpoint.
//
// BR-AI-006: POST /api/v1/incident/analyze
// DD-HAPI-003: Uses generated OpenAPI client for type safety and contract compliance.
//
// Example:
//
//	req := &client.IncidentRequest{
//	    IncidentID: "incident-123",
//	    // ... other fields
//	}
//	resp, err := hapiClient.Investigate(ctx, req)
//
// Returns:
//   - *IncidentResponse: Successful response with AI analysis
//   - *APIError: HTTP error (4xx, 5xx)
func (c *HolmesGPTClient) Investigate(ctx context.Context, req *IncidentRequest) (*IncidentResponse, error) {
	// DD-HAPI-003: Use generated client method for compile-time type safety
	res, err := c.client.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePost(ctx, req)
	if err != nil {
		// Extract status code from ogen error message (format: "unexpected status code: NNN")
		statusCode := 0
		errMsg := err.Error()
		if _, scanErr := fmt.Sscanf(errMsg, "decode response: unexpected status code: %d", &statusCode); scanErr == nil {
			// Successfully extracted status code from ogen error
			return nil, &APIError{
				StatusCode: statusCode,
				Message:    fmt.Sprintf("HolmesGPT-API returned HTTP %d: %v", statusCode, err),
			}
		}
		// True network error (no HTTP response)
		return nil, &APIError{
			StatusCode: 0,
			Message:    fmt.Sprintf("HolmesGPT-API call failed: %v", err),
		}
	}

	// DD-HAPI-003: Type-assert response interface to concrete type
	// DD-AUTH-013: Handle all HTTP status codes (200, 400, 401, 403, 422, 500)
	switch v := res.(type) {
	case *IncidentResponse:
		// 200 OK - Success
		return v, nil
	case *IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostBadRequest:
		// 400 Bad Request - Validation error (RFC7807)
		// Per commit 12bdd7f7d: HAPI returns 400 for Pydantic validation errors
		return nil, &APIError{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("HAPI validation error: %+v", v),
		}
	case *IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostUnauthorized:
		// 401 Unauthorized - Authentication failed (ose-oauth-proxy)
		return nil, &APIError{
			StatusCode: http.StatusUnauthorized,
			Message:    "Authentication failed: invalid or missing Bearer token",
		}
	case *IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostForbidden:
		// 403 Forbidden - Authorization failed (ose-oauth-proxy SAR denied)
		return nil, &APIError{
			StatusCode: http.StatusForbidden,
			Message:    "Authorization failed: ServiceAccount lacks 'get' permission on holmesgpt-api resource",
		}
	case *IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostUnprocessableEntity:
		// 422 Unprocessable Entity - Validation error (Deprecated: HAPI now uses 400)
		return nil, &APIError{
			StatusCode: http.StatusUnprocessableEntity,
			Message:    fmt.Sprintf("HAPI validation error: %+v", v),
		}
	case *IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostInternalServerError:
		// 500 Internal Server Error - HAPI application error
		return nil, &APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "HAPI internal server error",
		}
	default:
		// Unexpected response type
		return nil, &APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("unexpected response type from HAPI: %T", res),
		}
	}
}

// InvestigateRecovery calls the HolmesGPT-API recovery analyze endpoint.
//
// BR-AI-082: POST /api/v1/recovery/analyze
// DD-RECOVERY-002: Direct recovery flow implementation
// DD-HAPI-003: Uses generated OpenAPI client for type safety and contract compliance.
//
// Example:
//
//	req := &client.RecoveryRequest{
//	    IncidentID: "incident-123",
//	    // ... other fields
//	}
//	resp, err := hapiClient.InvestigateRecovery(ctx, req)
//
// Returns:
//   - *RecoveryResponse: Successful response with recovery strategies
//   - *APIError: HTTP error (4xx, 5xx)
func (c *HolmesGPTClient) InvestigateRecovery(ctx context.Context, req *RecoveryRequest) (*RecoveryResponse, error) {
	// DEBUG: Log what we're sending (BR-HAPI-197 investigation)
	log.Printf("🔍 DEBUG: Sending recovery request to HAPI - IncidentID=%s, SignalType.Set=%v, SignalType.Value=%s, IsRecoveryAttempt=%v, requestPointer=%p",
		req.IncidentID, req.SignalType.Set, req.SignalType.Value, req.IsRecoveryAttempt.Value, req)

	// DD-HAPI-003: Use generated client method for compile-time type safety
	res, err := c.client.RecoveryAnalyzeEndpointAPIV1RecoveryAnalyzePost(ctx, req)
	if err != nil {
		// Extract status code from ogen error message (format: "unexpected status code: NNN")
		statusCode := 0
		errMsg := err.Error()
		if _, scanErr := fmt.Sscanf(errMsg, "decode response: unexpected status code: %d", &statusCode); scanErr == nil {
			// Successfully extracted status code from ogen error
			return nil, &APIError{
				StatusCode: statusCode,
				Message:    fmt.Sprintf("HolmesGPT-API recovery returned HTTP %d: %v", statusCode, err),
			}
		}
		// True network error (no HTTP response)
		return nil, &APIError{
			StatusCode: 0,
			Message:    fmt.Sprintf("HolmesGPT-API recovery call failed: %v", err),
		}
	}

	// DD-HAPI-003: Type-assert response interface to concrete type
	// DD-AUTH-013: Handle all HTTP status codes (200, 400, 401, 403, 422, 500)
	switch v := res.(type) {
	case *RecoveryResponse:
		// 200 OK - Success
		return v, nil
	case *RecoveryAnalyzeEndpointAPIV1RecoveryAnalyzePostBadRequest:
		// 400 Bad Request - Validation error (RFC7807)
		// Per commit 12bdd7f7d: HAPI returns 400 for Pydantic validation errors
		return nil, &APIError{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("HAPI recovery validation error: %+v", v),
		}
	case *RecoveryAnalyzeEndpointAPIV1RecoveryAnalyzePostUnauthorized:
		// 401 Unauthorized - Authentication failed (ose-oauth-proxy)
		return nil, &APIError{
			StatusCode: http.StatusUnauthorized,
			Message:    "Authentication failed: invalid or missing Bearer token",
		}
	case *RecoveryAnalyzeEndpointAPIV1RecoveryAnalyzePostForbidden:
		// 403 Forbidden - Authorization failed (ose-oauth-proxy SAR denied)
		return nil, &APIError{
			StatusCode: http.StatusForbidden,
			Message:    "Authorization failed: ServiceAccount lacks 'get' permission on holmesgpt-api resource",
		}
	case *RecoveryAnalyzeEndpointAPIV1RecoveryAnalyzePostUnprocessableEntity:
		// 422 Unprocessable Entity - Validation error (Deprecated: HAPI now uses 400)
		return nil, &APIError{
			StatusCode: http.StatusUnprocessableEntity,
			Message:    fmt.Sprintf("HAPI recovery validation error: %+v", v),
		}
	case *RecoveryAnalyzeEndpointAPIV1RecoveryAnalyzePostInternalServerError:
		// 500 Internal Server Error - HAPI application error
		return nil, &APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "HAPI recovery internal server error",
		}
	default:
		// Unexpected response type
		return nil, &APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("unexpected response type from HAPI recovery endpoint: %T", res),
		}
	}
}

// ========================================
// ERROR TYPES
// ========================================

// APIError represents an HTTP error from HolmesGPT-API.
//
// This error type wraps both network errors (no status code) and HTTP errors (4xx, 5xx).
type APIError struct {
	StatusCode int    // HTTP status code (0 for network errors)
	Message    string // Human-readable error message
}

// Error implements the error interface.
func (e *APIError) Error() string {
	if e.StatusCode == 0 {
		return fmt.Sprintf("HolmesGPT-API network error: %s", e.Message)
	}
	return fmt.Sprintf("HolmesGPT-API error (HTTP %d): %s", e.StatusCode, e.Message)
}




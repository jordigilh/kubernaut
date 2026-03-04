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
//
//	Validation: scripts/validate-openapi-client-usage.sh
//
// ========================================
//
// BR-AI-006: API call construction and response handling.
package client

import (
	"context"
	"encoding/json"
	"fmt"
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
// All methods delegate to the generated client (oas_client_gen.go) for type safety,
// OTel tracing, and contract compliance with HAPI's OpenAPI specification.
type HolmesGPTClient struct {
	client *Client // Generated OpenAPI client from oas_client_gen.go
}

// newClientWithHTTP is the shared constructor that builds a HolmesGPTClient
// from an already-configured *http.Client. Both public constructors delegate here.
func newClientWithHTTP(cfg Config, httpClient *http.Client) (*HolmesGPTClient, error) {
	generatedClient, err := NewClient(
		cfg.BaseURL,
		WithClient(httpClient),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAPI client: %w", err)
	}

	return &HolmesGPTClient{
		client: generatedClient,
	}, nil
}

// defaultTimeout returns cfg.Timeout or 60s if zero.
func defaultTimeout(cfg Config) time.Duration {
	if cfg.Timeout > 0 {
		return cfg.Timeout
	}
	return 60 * time.Second
}

// NewHolmesGPTClient creates a new HAPI client using the generated OpenAPI client.
//
// DD-HAPI-003: Uses generated client for compile-time type safety and contract compliance.
// DD-AUTH-006: Uses ServiceAccount authentication by default (production/E2E).
//
// For integration tests with custom authentication, use NewHolmesGPTClientWithTransport.
func NewHolmesGPTClient(cfg Config) (*HolmesGPTClient, error) {
	// DD-AUTH-006: Use ServiceAccount authentication for production/E2E
	// OAuth-proxy validates this token and injects X-Auth-Request-User header
	transport := auth.NewServiceAccountTransportWithBase(http.DefaultTransport)

	return newClientWithHTTP(cfg, &http.Client{
		Timeout:   defaultTimeout(cfg),
		Transport: transport,
	})
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
	return newClientWithHTTP(cfg, &http.Client{
		Timeout:   defaultTimeout(cfg),
		Transport: transport,
	})
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
	// BR-AA-HAPI-064: HAPI endpoints are async-only (202 Accepted).
	// This sync wrapper internally does submit -> poll -> get result,
	// providing backward-compatible API for callers that don't need
	// explicit session management (e.g., integration tests, one-shot callers).
	// The AA controller uses WithSessionMode() and explicit session methods instead.
	sessionID, err := c.SubmitInvestigation(ctx, req)
	if err != nil {
		return nil, err
	}

	// Poll until session completes (1s interval, bounded by ctx deadline)
	if err := c.awaitSession(ctx, sessionID); err != nil {
		return nil, err
	}

	return c.GetSessionResult(ctx, sessionID)
}

// awaitSession polls a HAPI session until it reaches a terminal state ("completed" or "failed").
// Used by the sync wrapper Investigate() to block until the async investigation finishes.
// The poll interval is 1s, bounded by the ctx deadline.
//
// BR-AA-HAPI-064: Internal helper for sync-over-async wrapping.
func (c *HolmesGPTClient) awaitSession(ctx context.Context, sessionID string) error {
	for {
		status, err := c.PollSession(ctx, sessionID)
		if err != nil {
			return err
		}

		switch status.Status {
		case "completed":
			return nil
		case "failed":
			return &APIError{
				StatusCode: http.StatusInternalServerError,
				Message:    fmt.Sprintf("HAPI session failed: %s", status.Error),
			}
		default:
			// "pending" or "investigating" -- wait and retry
			select {
			case <-ctx.Done():
				return &APIError{
					StatusCode: 0,
					Message:    fmt.Sprintf("context cancelled while polling session %s: %v", sessionID, ctx.Err()),
				}
			case <-time.After(1 * time.Second):
				// continue polling
			}
		}
	}
}

// ========================================
// SESSION TYPES (BR-AA-HAPI-064)
// ========================================

// SessionStatus represents the status of a HAPI investigation session.
// Returned by PollSession when querying session progress.
type SessionStatus struct {
	// Status of the session: "pending", "investigating", "completed", "failed"
	Status string `json:"status"`
	// Error message when status is "failed"
	Error string `json:"error,omitempty"`
	// Progress description for operator visibility
	Progress string `json:"progress,omitempty"`
}

// ========================================
// ASYNC SESSION METHODS (BR-AA-HAPI-064)
// DD-HAPI-003: All session methods now use the generated OpenAPI client.
// ========================================

// SubmitInvestigation submits an incident investigation request and returns a session ID.
// BR-AA-HAPI-064.1: POST /api/v1/incident/analyze returns 202 with session_id
// DD-HAPI-003: Delegates to generated client for OTel tracing and type-safe dispatch.
func (c *HolmesGPTClient) SubmitInvestigation(ctx context.Context, req *IncidentRequest) (string, error) {
	res, err := c.client.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePost(ctx, req)
	if err != nil {
		return "", &APIError{StatusCode: 0, Message: fmt.Sprintf("submit investigation failed: %v", err)}
	}

	accepted, ok := res.(*IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostAcceptedApplicationJSON)
	if !ok {
		return "", &APIError{StatusCode: 0, Message: fmt.Sprintf("unexpected response type (expected 202 Accepted): %T", res)}
	}

	var parsed struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal([]byte(*accepted), &parsed); err != nil {
		return "", &APIError{StatusCode: http.StatusAccepted, Message: fmt.Sprintf("failed to decode session response: %v", err)}
	}
	return parsed.SessionID, nil
}

// PollSession polls the status of an investigation session.
// BR-AA-HAPI-064.2: GET /api/v1/incident/session/{id}
// Returns *APIError{StatusCode: 404} when session not found (BR-AA-HAPI-064.5 regeneration trigger).
func (c *HolmesGPTClient) PollSession(ctx context.Context, sessionID string) (*SessionStatus, error) {
	res, err := c.client.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGet(ctx,
		IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetParams{SessionID: sessionID})
	if err != nil {
		return nil, &APIError{StatusCode: 0, Message: fmt.Sprintf("poll session failed: %v", err)}
	}

	switch v := res.(type) {
	case *IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetOKApplicationJSON:
		var status SessionStatus
		if err := json.Unmarshal([]byte(*v), &status); err != nil {
			return nil, &APIError{StatusCode: 0, Message: fmt.Sprintf("failed to decode session status: %v", err)}
		}
		return &status, nil
	case *IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetNotFound:
		return nil, &APIError{StatusCode: http.StatusNotFound, Message: fmt.Sprintf("session %s not found", sessionID)}
	case *HTTPValidationError:
		return nil, &APIError{StatusCode: http.StatusUnprocessableEntity, Message: "validation error"}
	default:
		return nil, &APIError{StatusCode: 0, Message: fmt.Sprintf("unexpected response type: %T", res)}
	}
}

// GetSessionResult retrieves the result of a completed incident investigation session.
// BR-AA-HAPI-064.3: GET /api/v1/incident/session/{id}/result
func (c *HolmesGPTClient) GetSessionResult(ctx context.Context, sessionID string) (*IncidentResponse, error) {
	res, err := c.client.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGet(ctx,
		IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetParams{SessionID: sessionID})
	if err != nil {
		return nil, &APIError{StatusCode: 0, Message: fmt.Sprintf("get session result failed: %v", err)}
	}

	switch v := res.(type) {
	case *IncidentResponse:
		return v, nil
	case *IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetNotFound:
		return nil, &APIError{StatusCode: http.StatusNotFound, Message: fmt.Sprintf("session %s not found", sessionID)}
	case *IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetConflict:
		return nil, &APIError{StatusCode: http.StatusConflict, Message: fmt.Sprintf("session %s not yet completed", sessionID)}
	case *HTTPValidationError:
		return nil, &APIError{StatusCode: http.StatusUnprocessableEntity, Message: "validation error"}
	default:
		return nil, &APIError{StatusCode: 0, Message: fmt.Sprintf("unexpected response type: %T", res)}
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

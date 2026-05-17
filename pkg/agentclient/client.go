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

// Package agentclient provides the Kubernaut Agent OpenAPI client wrapper.
//
// DD-HAPI-003: Mandatory OpenAPI Client Usage
// This wrapper provides a business-friendly API around the auto-generated
// OpenAPI client (oas_client_gen.go).
//
// BR-AI-006: API call construction and response handling.
package agentclient

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/pkg/shared/auth"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
)

// ========================================
// CLIENT CONFIGURATION
// ========================================

// Config for Kubernaut Agent client
type Config struct {
	BaseURL string
	Timeout time.Duration
}

// KubernautAgentClient wraps the auto-generated OpenAPI client with a business-friendly API.
// DD-HAPI-003: All methods delegate to the generated client (oas_client_gen.go).
type KubernautAgentClient struct {
	client *Client // Generated OpenAPI client from oas_client_gen.go
}

// newClientWithHTTP is the shared constructor that builds a KubernautAgentClient
// from an already-configured *http.Client. Both public constructors delegate here.
func newClientWithHTTP(cfg Config, httpClient *http.Client) (*KubernautAgentClient, error) {
	generatedClient, err := NewClient(
		cfg.BaseURL,
		WithClient(httpClient),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAPI client: %w", err)
	}

	return &KubernautAgentClient{
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

// NewKubernautAgentClient creates a new KA client using the generated OpenAPI client.
//
// DD-HAPI-003: Uses generated client for compile-time type safety and contract compliance.
// DD-AUTH-006: Uses ServiceAccount authentication by default (production/E2E).
//
// For integration tests with custom authentication, use NewAgentClientWithTransport.
func NewKubernautAgentClient(cfg Config) (*KubernautAgentClient, error) {
	// Issue #750: TLS_CA_FILE honoured for inter-service HTTPS.
	// Issue #853: Wrapped with RetryTransport for transient failure resilience.
	baseTransport, err := sharedtls.DefaultBaseTransportWithRetry()
	if err != nil {
		return nil, fmt.Errorf("failed to create TLS-aware base transport: %w", err)
	}
	transport := auth.NewAuthTransport(auth.NewDefaultTokenSource(), baseTransport)

	return newClientWithHTTP(cfg, &http.Client{
		Timeout:   defaultTimeout(cfg),
		Transport: transport,
	})
}

// NewKubernautAgentClientWithTransport creates a new agent client with a custom HTTP transport.
// DD-AUTH-006: Integration test pattern for mock authentication.
func NewKubernautAgentClientWithTransport(cfg Config, transport http.RoundTripper) (*KubernautAgentClient, error) {
	return newClientWithHTTP(cfg, &http.Client{
		Timeout:   defaultTimeout(cfg),
		Transport: transport,
	})
}

// ========================================
// BUSINESS API METHODS
// ========================================

// Investigate calls the incident analyze endpoint.
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
//	resp, err := kaClient.Investigate(ctx, req)
//
// Returns:
//   - *IncidentResponse: Successful response with AI analysis
//   - *APIError: HTTP error (4xx, 5xx)
func (c *KubernautAgentClient) Investigate(ctx context.Context, req *IncidentRequest) (*IncidentResponse, error) {
	// BR-AA-HAPI-064: KA endpoints are async-only (202 Accepted).
	// This sync wrapper internally does submit -> poll -> get result,
	// providing backward-compatible API for callers that don't need
	// explicit session management (e.g., integration tests, one-shot callers).
	// The AA controller uses WithSessionMode() and explicit session methods instead.
	sessionID, err := c.SubmitInvestigation(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := c.awaitSession(ctx, sessionID); err != nil {
		return nil, err
	}

	return c.getSessionResultWithRetry(ctx, sessionID)
}

// awaitSession polls a KA session until it reaches a terminal state ("completed" or "failed").
// Used by the sync wrapper Investigate() to block until the async investigation finishes.
// The poll interval is 1s, bounded by the ctx deadline. Transient 429 (rate-limited)
// responses are retried with a 2s back-off rather than failing immediately.
//
// BR-AA-HAPI-064: Internal helper for sync-over-async wrapping.
func (c *KubernautAgentClient) awaitSession(ctx context.Context, sessionID string) error {
	for {
		result, err := c.PollSession(ctx, sessionID)
		if err != nil {
			if isRateLimited(err) {
				if waitErr := sleepCtx(ctx, 2*time.Second); waitErr != nil {
					return &APIError{StatusCode: 0, Message: fmt.Sprintf("context cancelled while rate-limited polling session %s: %v", sessionID, ctx.Err())}
				}
				continue
			}
			return err
		}

		switch result.Status {
		case "completed":
			return nil
		case "failed":
			return &APIError{
				StatusCode: http.StatusInternalServerError,
				Message:    fmt.Sprintf("agent session failed: %s", result.Error),
			}
		default:
			if waitErr := sleepCtx(ctx, 1*time.Second); waitErr != nil {
				return &APIError{StatusCode: 0, Message: fmt.Sprintf("context cancelled while polling session %s: %v", sessionID, ctx.Err())}
			}
		}
	}
}

// ========================================
// SESSION TYPES (BR-AA-HAPI-064)
// ========================================

// SessionStatusResult represents the status of an investigation session.
// Returned by PollSession when querying session progress.
type SessionStatusResult struct {
	// SessionID is the KA-assigned session identifier
	SessionID string `json:"session_id,omitempty"`
	// Status of the session: "pending", "investigating", "completed", "failed"
	Status string `json:"status"`
	// Error message when status is "failed"
	Error string `json:"error,omitempty"`
	// Progress description for operator visibility
	Progress string `json:"progress,omitempty"`
	// ActingUser is the resolved identity currently driving the investigation
	// (populated when status is "user_driving", BR-INTERACTIVE-001, #774)
	ActingUser string `json:"acting_user,omitempty"`
	// ActingUserGroups are the groups of the user driving the investigation
	// (populated when status is "user_driving", BR-INTERACTIVE-001, #774)
	ActingUserGroups []string `json:"acting_user_groups,omitempty"`
}

// ========================================
// ASYNC SESSION METHODS (BR-AA-HAPI-064)
// DD-HAPI-003: All session methods now use the generated OpenAPI client.
// ========================================

// SubmitInvestigation submits an incident investigation request and returns a session ID.
// BR-AA-HAPI-064.1: POST /api/v1/incident/analyze returns 202 with session_id
// DD-HAPI-003: Delegates to generated client for OTel tracing and type-safe dispatch.
func (c *KubernautAgentClient) SubmitInvestigation(ctx context.Context, req *IncidentRequest) (string, error) {
	res, err := c.client.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePost(ctx, req)
	if err != nil {
		return "", &APIError{StatusCode: 0, Message: fmt.Sprintf("submit investigation failed: %v", err)}
	}

	switch v := res.(type) {
	case *AnalyzeAccepted:
		return v.SessionID.String(), nil
	case *IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostApplicationJSONBadRequest:
		return "", &APIError{StatusCode: http.StatusBadRequest, Message: fmt.Sprintf("bad request: %s", HTTPError(*v).Detail)}
	case *IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostBadRequestApplicationProblemJSON:
		return "", &APIError{StatusCode: http.StatusBadRequest, Message: v.Detail}
	case *IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostApplicationJSONUnprocessableEntity:
		return "", &APIError{StatusCode: http.StatusUnprocessableEntity, Message: fmt.Sprintf("validation error: %s", HTTPError(*v).Detail)}
	case *IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostUnprocessableEntityApplicationProblemJSON:
		return "", &APIError{StatusCode: http.StatusUnprocessableEntity, Message: v.Detail}
	case *IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostApplicationJSONUnauthorized:
		return "", &APIError{StatusCode: http.StatusUnauthorized, Message: "unauthorized"}
	case *IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostApplicationJSONForbidden:
		return "", &APIError{StatusCode: http.StatusForbidden, Message: "forbidden"}
	case *IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostApplicationJSONInternalServerError:
		return "", &APIError{StatusCode: http.StatusInternalServerError, Message: fmt.Sprintf("server error: %s", HTTPError(*v).Detail)}
	default:
		return "", &APIError{StatusCode: 0, Message: fmt.Sprintf("unexpected response type (expected 202 Accepted): %T", res)}
	}
}

// PollSession polls the status of an investigation session.
// BR-AA-HAPI-064.2: GET /api/v1/incident/session/{id}
// Returns *APIError{StatusCode: 404} when session not found (BR-AA-HAPI-064.5 regeneration trigger).
func (c *KubernautAgentClient) PollSession(ctx context.Context, sessionID string) (*SessionStatusResult, error) {
	res, err := c.client.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGet(ctx,
		IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetParams{SessionID: sessionID})
	if err != nil {
		return nil, &APIError{StatusCode: 0, Message: fmt.Sprintf("poll session failed: %v", err)}
	}

	switch v := res.(type) {
	case *SessionStatus:
		result := &SessionStatusResult{
			SessionID:        v.SessionID,
			Status:           v.Status,
			Error:            v.Error.Value,
			ActingUser:       v.ActingUser.Value,
			ActingUserGroups: v.ActingUserGroups,
		}
		return result, nil
	case *IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetNotFound:
		return nil, &APIError{StatusCode: http.StatusNotFound, Message: fmt.Sprintf("session %s not found: %s", sessionID, v.Detail)}
	case *IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetInternalServerError:
		return nil, &APIError{StatusCode: http.StatusInternalServerError, Message: fmt.Sprintf("server error: %s", v.Detail)}
	case *HTTPValidationError:
		return nil, &APIError{StatusCode: http.StatusUnprocessableEntity, Message: "validation error"}
	default:
		return nil, &APIError{StatusCode: 0, Message: fmt.Sprintf("unexpected response type: %T", res)}
	}
}

// GetSessionResult retrieves the result of a completed incident investigation session.
// BR-AA-HAPI-064.3: GET /api/v1/incident/session/{id}/result
func (c *KubernautAgentClient) GetSessionResult(ctx context.Context, sessionID string) (*IncidentResponse, error) {
	res, err := c.client.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGet(ctx,
		IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetParams{SessionID: sessionID})
	if err != nil {
		return nil, &APIError{StatusCode: 0, Message: fmt.Sprintf("get session result failed: %v", err)}
	}

	switch v := res.(type) {
	case *IncidentResponse:
		return v, nil
	case *IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetNotFound:
		return nil, &APIError{StatusCode: http.StatusNotFound, Message: fmt.Sprintf("session %s not found: %s", sessionID, v.Detail)}
	case *IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetConflict:
		return nil, &APIError{StatusCode: http.StatusConflict, Message: fmt.Sprintf("session %s not yet completed: %s", sessionID, v.Detail)}
	case *HTTPValidationError:
		return nil, &APIError{StatusCode: http.StatusUnprocessableEntity, Message: "validation error"}
	default:
		return nil, &APIError{StatusCode: 0, Message: fmt.Sprintf("unexpected response type: %T", res)}
	}
}

// ========================================
// ERROR TYPES
// ========================================

// APIError represents an HTTP error from the Kubernaut Agent.
type APIError struct {
	StatusCode int    // HTTP status code (0 for network errors)
	Message    string // Human-readable error message
}

// Error implements the error interface.
func (e *APIError) Error() string {
	if e.StatusCode == 0 {
		return fmt.Sprintf("agent network error: %s", e.Message)
	}
	return fmt.Sprintf("agent error (HTTP %d): %s", e.StatusCode, e.Message)
}

// isRateLimited returns true if the error indicates an HTTP 429 response.
// The ogen client does not model 429 as a typed response, so we detect it
// from the error string ("unexpected status code: 429").
func isRateLimited(err error) bool {
	return err != nil && strings.Contains(err.Error(), "429")
}

// sleepCtx blocks for d or until ctx is cancelled.
func sleepCtx(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

const maxResultRetries = 3

// getSessionResultWithRetry wraps GetSessionResult with retry on 429.
func (c *KubernautAgentClient) getSessionResultWithRetry(ctx context.Context, sessionID string) (*IncidentResponse, error) {
	var lastErr error
	for i := 0; i < maxResultRetries; i++ {
		resp, err := c.GetSessionResult(ctx, sessionID)
		if err == nil {
			return resp, nil
		}
		if !isRateLimited(err) {
			return nil, err
		}
		lastErr = err
		if waitErr := sleepCtx(ctx, 2*time.Second); waitErr != nil {
			return nil, &APIError{StatusCode: 0, Message: fmt.Sprintf("context cancelled while retrying result for session %s: %v", sessionID, ctx.Err())}
		}
	}
	return nil, lastErr
}

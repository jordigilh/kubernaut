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
// BR-AI-006: API call construction and response handling.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Config for HolmesGPT-API client
type Config struct {
	BaseURL string
	Timeout time.Duration
}

// HolmesGPTClient wraps HolmesGPT-API calls
type HolmesGPTClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewHolmesGPTClient creates a new client
func NewHolmesGPTClient(cfg Config) *HolmesGPTClient {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	return &HolmesGPTClient{
		baseURL: cfg.BaseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// IncidentRequest represents request to /api/v1/incident/analyze
type IncidentRequest struct {
	Context        string                 `json:"context"`
	DetectedLabels map[string]interface{} `json:"detected_labels,omitempty"`
	CustomLabels   map[string][]string    `json:"custom_labels,omitempty"`
	OwnerChain     []OwnerChainEntry      `json:"owner_chain,omitempty"`
}

// OwnerChainEntry represents a resource in the owner chain
type OwnerChainEntry struct {
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
}

// IncidentResponse represents response from HolmesGPT-API /api/v1/incident/analyze
// Per HolmesGPT-API team (Dec 5, 2025): This endpoint returns ALL analysis results
// including selected_workflow and alternative_workflows in a single call.
// BR-HAPI-197 (Dec 6, 2025): Added NeedsHumanReview and HumanReviewReason fields.
type IncidentResponse struct {
	// Incident identifier from request
	IncidentID string `json:"incident_id"`
	// Natural language analysis from LLM
	Analysis string `json:"analysis"`
	// Structured RCA with summary, severity, contributing_factors
	RootCauseAnalysis *RootCauseAnalysis `json:"root_cause_analysis,omitempty"`
	// Selected workflow for execution (DD-CONTRACT-002)
	SelectedWorkflow *SelectedWorkflow `json:"selected_workflow,omitempty"`
	// Alternative workflows considered but not selected
	// INFORMATIONAL ONLY - NOT for automatic execution
	// Per HolmesGPT-API team: Alternatives are for CONTEXT, not EXECUTION
	AlternativeWorkflows []AlternativeWorkflow `json:"alternative_workflows,omitempty"`
	// Overall confidence in analysis (0.0-1.0)
	Confidence float64 `json:"confidence"`
	// ISO timestamp of analysis completion
	Timestamp string `json:"timestamp"`
	// Whether RCA-identified target resource was found in OwnerChain
	// If false, DetectedLabels may be from different scope than affected resource
	TargetInOwnerChain bool `json:"target_in_owner_chain"`
	// Non-fatal warnings (e.g., OwnerChain validation issues, low confidence)
	Warnings []string `json:"warnings,omitempty"`
	// BR-HAPI-197: True when AI cannot produce reliable result and human must intervene
	// When true, automatic remediation MUST NOT proceed
	NeedsHumanReview bool `json:"needs_human_review"`
	// BR-HAPI-197: Structured reason for NeedsHumanReview=true
	// Enum: workflow_not_found, image_mismatch, parameter_validation_failed,
	//       no_matching_workflows, low_confidence, llm_parsing_error
	// Use this for reliable SubReason mapping instead of parsing warnings
	HumanReviewReason *string `json:"human_review_reason,omitempty"`
	// DD-HAPI-002 v1.4: Complete history of all validation attempts
	// HAPI retries up to 3 times with LLM self-correction
	// Provides audit trail for operator notifications and debugging
	ValidationAttemptsHistory []ValidationAttempt `json:"validation_attempts_history,omitempty"`
}

// ValidationAttempt contains details of a single HAPI validation attempt
// Per DD-HAPI-002 v1.4: Each attempt feeds validation errors back to the LLM
type ValidationAttempt struct {
	// Attempt number (1, 2, or 3)
	Attempt int `json:"attempt"`
	// WorkflowID that the LLM tried in this attempt
	WorkflowID string `json:"workflow_id"`
	// Whether validation passed (always false for failed attempts in history)
	IsValid bool `json:"is_valid"`
	// Validation errors encountered
	Errors []string `json:"errors,omitempty"`
	// When this attempt occurred (ISO timestamp)
	Timestamp string `json:"timestamp"`
}

// RootCauseAnalysis contains structured RCA results from HolmesGPT-API
type RootCauseAnalysis struct {
	// Brief summary of root cause
	Summary string `json:"summary"`
	// Severity determined by RCA: critical, high, medium, low
	Severity string `json:"severity"`
	// Contributing factors that led to the issue
	ContributingFactors []string `json:"contributing_factors,omitempty"`
}

// SelectedWorkflow contains the AI-selected workflow for execution
type SelectedWorkflow struct {
	// Workflow identifier (catalog lookup key)
	WorkflowID string `json:"workflow_id"`
	// Workflow version
	Version string `json:"version,omitempty"`
	// Container image (OCI bundle) - resolved by HolmesGPT-API
	ContainerImage string `json:"containerImage"`
	// Container digest for audit trail
	ContainerDigest string `json:"containerDigest,omitempty"`
	// Confidence score (0.0-1.0)
	Confidence float64 `json:"confidence"`
	// Workflow parameters (UPPER_SNAKE_CASE keys per DD-WORKFLOW-003)
	Parameters map[string]string `json:"parameters,omitempty"`
	// Rationale explaining why this workflow was selected
	Rationale string `json:"rationale"`
}

// AlternativeWorkflow contains alternative workflows considered but not selected
// INFORMATIONAL ONLY - NOT for automatic execution
type AlternativeWorkflow struct {
	// Workflow identifier
	WorkflowID string `json:"workflow_id"`
	// Container image (OCI bundle)
	ContainerImage string `json:"containerImage,omitempty"`
	// Confidence score (0.0-1.0) - shows why it wasn't selected
	Confidence float64 `json:"confidence"`
	// Rationale explaining why this workflow was considered
	Rationale string `json:"rationale"`
}

// Investigate calls the HolmesGPT-API incident analyze endpoint
// BR-AI-006: API call construction
func (c *HolmesGPTClient) Investigate(ctx context.Context, req *IncidentRequest) (*IncidentResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/v1/incident/analyze", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("API returned status %d", resp.StatusCode),
		}
	}

	var result IncidentResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// APIError represents an API error
// BR-AI-009: Error classification for retry logic
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API error (status %d): %s", e.StatusCode, e.Message)
}

// IsTransient returns true if the error is retry-able
// BR-AI-009: Transient errors (429, 502, 503, 504) should be retried
// BR-AI-010: Permanent errors (400, 401, 403, 404) should not be retried
func (e *APIError) IsTransient() bool {
	switch e.StatusCode {
	case http.StatusTooManyRequests, // 429
		http.StatusBadGateway,         // 502
		http.StatusServiceUnavailable, // 503
		http.StatusGatewayTimeout:     // 504
		return true
	default:
		return false
	}
}

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

// Package client provides HolmesGPT-API client integration for AIAnalysis.
// It wraps the ogen-generated client with error classification and type mapping.
//
// Design Decision: DD-CONTRACT-002 - Self-contained CRD pattern
// Business Requirements: BR-AI-006 (API integration), BR-AI-009 (retry logic)
package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/ogen-go/ogen/validate"

	holmesgpt "github.com/jordigilh/kubernaut/pkg/clients/holmesgpt"
)

// ========================================
// CONFIG (BR-AI-006)
// ========================================

// Config configures the HolmesGPT-API client.
type Config struct {
	// BaseURL is the HolmesGPT-API service URL (e.g., "http://holmesgpt-api:8080")
	// Internal service - no API key needed (use K8s NetworkPolicies for access control)
	BaseURL string
}

// ========================================
// CLIENT INTERFACE (BR-AI-006)
// ========================================

// HolmesGPTClient defines the interface for HolmesGPT-API calls.
// This interface allows for mocking in unit tests.
type HolmesGPTClient interface {
	// Investigate calls the incident analysis endpoint.
	Investigate(ctx context.Context, req *IncidentRequest) (*IncidentResponse, error)
}

// ========================================
// REQUEST/RESPONSE TYPES (BR-AI-007)
// ========================================

// IncidentRequest is the simplified request structure for the Investigate method.
// It maps to holmesgpt.IncidentRequest for the API call.
type IncidentRequest struct {
	IncidentID        string
	RemediationID     string
	SignalType        string
	Severity          string
	SignalSource      string
	ResourceNamespace string
	ResourceKind      string
	ResourceName      string
	ErrorMessage      string
	Environment       string
	Priority          string
	RiskTolerance     string
	BusinessCategory  string
	ClusterName       string

	// Enrichment data
	DetectedLabels map[string]interface{}
	CustomLabels   map[string][]string
	OwnerChain     []OwnerChainEntry
}

// OwnerChainEntry represents a resource in the owner chain.
type OwnerChainEntry struct {
	Namespace string
	Kind      string
	Name      string
}

// IncidentResponse is the simplified response structure from the Investigate method.
type IncidentResponse struct {
	IncidentID         string
	Analysis           string
	RootCauseAnalysis  *RootCauseAnalysis
	SelectedWorkflow   *SelectedWorkflow
	Confidence         float64
	Timestamp          string
	TargetInOwnerChain bool
	Warnings           []string
}

// RootCauseAnalysis contains structured RCA information.
type RootCauseAnalysis struct {
	Summary             string
	Severity            string
	SignalType          string
	ContributingFactors []string
}

// SelectedWorkflow contains the AI-selected workflow.
type SelectedWorkflow struct {
	WorkflowID      string
	Version         string
	ContainerImage  string
	ContainerDigest string
	Confidence      float64
	Parameters      map[string]string
	Rationale       string
}

// ========================================
// CLIENT IMPLEMENTATION (BR-AI-006)
// ========================================

// Client wraps the ogen-generated HolmesGPT-API client with error classification.
type Client struct {
	ogenClient *holmesgpt.Client
	baseURL    string
}

// NewClient creates a new HolmesGPT-API client.
func NewClient(cfg Config) (*Client, error) {
	ogenClient, err := holmesgpt.NewClient(cfg.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create ogen client: %w", err)
	}

	return &Client{
		ogenClient: ogenClient,
		baseURL:    cfg.BaseURL,
	}, nil
}

// Investigate calls the HolmesGPT-API incident analysis endpoint.
// It maps our internal request type to the ogen-generated type and handles errors.
func (c *Client) Investigate(ctx context.Context, req *IncidentRequest) (*IncidentResponse, error) {
	// Build the ogen request
	ogenReq := c.buildOgenRequest(req)

	// Call the API
	resp, err := c.ogenClient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePost(ctx, ogenReq)
	if err != nil {
		return nil, c.classifyError(err)
	}

	// Handle response type
	switch r := resp.(type) {
	case *holmesgpt.IncidentResponse:
		return c.mapResponse(r), nil
	default:
		return nil, NewAPIError(http.StatusInternalServerError, "unexpected response type")
	}
}

// buildOgenRequest maps our request type to the ogen-generated type.
func (c *Client) buildOgenRequest(req *IncidentRequest) *holmesgpt.IncidentRequest {
	return &holmesgpt.IncidentRequest{
		IncidentID:        req.IncidentID,
		RemediationID:     req.RemediationID,
		SignalType:        req.SignalType,
		Severity:          req.Severity,
		SignalSource:      req.SignalSource,
		ResourceNamespace: req.ResourceNamespace,
		ResourceKind:      req.ResourceKind,
		ResourceName:      req.ResourceName,
		ErrorMessage:      req.ErrorMessage,
		Environment:       req.Environment,
		Priority:          req.Priority,
		RiskTolerance:     req.RiskTolerance,
		BusinessCategory:  req.BusinessCategory,
		ClusterName:       req.ClusterName,
	}
}

// mapResponse maps the ogen response to our internal type.
func (c *Client) mapResponse(resp *holmesgpt.IncidentResponse) *IncidentResponse {
	result := &IncidentResponse{
		IncidentID: resp.IncidentID,
		Analysis:   resp.Analysis,
		Confidence: resp.Confidence,
		Timestamp:  resp.Timestamp,
		Warnings:   resp.Warnings,
	}

	// Map TargetInOwnerChain (optional field)
	if resp.TargetInOwnerChain.IsSet() {
		result.TargetInOwnerChain = resp.TargetInOwnerChain.Value
	}

	return result
}

// classifyError determines if an error is transient or permanent.
// Uses ogen's typed error to extract HTTP status code directly.
func (c *Client) classifyError(err error) error {
	// Check for ogen's UnexpectedStatusCodeError which contains the actual status code
	var statusErr *validate.UnexpectedStatusCodeError
	if errors.As(err, &statusErr) {
		return NewAPIError(statusErr.StatusCode, err.Error())
	}

	// Fallback for other error types (network errors, etc.)
	// Treat as transient since we can't determine the cause
	return NewAPIError(http.StatusServiceUnavailable, err.Error())
}

// ========================================
// ERROR TYPES (BR-AI-009, BR-AI-010)
// ========================================

// APIError represents an error from the HolmesGPT-API.
type APIError struct {
	StatusCode int
	Message    string
}

// NewAPIError creates a new API error.
func NewAPIError(statusCode int, message string) *APIError {
	return &APIError{
		StatusCode: statusCode,
		Message:    message,
	}
}

func (e *APIError) Error() string {
	return fmt.Sprintf("HolmesGPT-API error (status %d): %s", e.StatusCode, e.Message)
}

// IsTransient returns true if the error is retry-able.
// Per APPENDIX_B: 429, 502, 503, 504 are transient.
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

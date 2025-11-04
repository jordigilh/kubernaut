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

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	contextapierrors "github.com/jordigilh/kubernaut/pkg/contextapi/errors"
)

// Config holds the configuration for the Data Storage HTTP client
type Config struct {
	// BaseURL is the Data Storage Service base URL (e.g., "http://localhost:8080")
	BaseURL string

	// Timeout is the HTTP request timeout (default: 5s)
	Timeout time.Duration

	// MaxConnections is the maximum number of concurrent connections (default: 100)
	MaxConnections int

	// HTTPClient allows injecting a custom HTTP client (for testing)
	HTTPClient *http.Client
}

// DataStorageClient is a high-level wrapper around the auto-generated OpenAPI client
// It provides a cleaner API with parsed responses and proper error handling
type DataStorageClient struct {
	client ClientInterface
	config Config
}

// NewDataStorageClient creates a new Data Storage HTTP client
func NewDataStorageClient(cfg Config) *DataStorageClient {
	// Validate and set defaults
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Second
	}
	if cfg.MaxConnections == 0 {
		cfg.MaxConnections = 100
	}

	// Create HTTP client with configured timeout and connection pool
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: cfg.Timeout,
			Transport: &http.Transport{
				MaxIdleConns:        cfg.MaxConnections,
				MaxIdleConnsPerHost: cfg.MaxConnections,
				IdleConnTimeout:     90 * time.Second,
			},
		}
	}

	// Create the auto-generated client
	generatedClient, err := NewClientWithResponses(cfg.BaseURL, WithHTTPClient(httpClient))
	if err != nil {
		panic(fmt.Sprintf("failed to create Data Storage client: %v", err))
	}

	return &DataStorageClient{
		client: generatedClient,
		config: cfg,
	}
}

// IncidentsResult holds incidents with pagination metadata
type IncidentsResult struct {
	Incidents []Incident
	Total     int
}

// ListIncidents retrieves a list of incidents from the Data Storage Service
// filters can include: signal_name, severity, action_type, limit, offset
func (c *DataStorageClient) ListIncidents(ctx context.Context, filters map[string]string) (*IncidentsResult, error) {
	// Build query parameters from filters
	params := &ListIncidentsParams{}

	if signalName, ok := filters["signal_name"]; ok {
		params.SignalName = &signalName
	}
	if severity, ok := filters["severity"]; ok {
		sev := ListIncidentsParamsSeverity(severity)
		params.Severity = &sev
	}
	if actionType, ok := filters["action_type"]; ok {
		params.ActionType = &actionType
	}
	if limitStr, ok := filters["limit"]; ok {
		if limitInt, err := strconv.Atoi(limitStr); err == nil {
			params.Limit = &limitInt
		}
	}
	if offsetStr, ok := filters["offset"]; ok {
		if offsetInt, err := strconv.Atoi(offsetStr); err == nil {
			params.Offset = &offsetInt
		}
	}
	if namespace, ok := filters["namespace"]; ok {
		params.Namespace = &namespace
	}

	// Add request ID for tracing
	requestID := uuid.New().String()
	reqEditor := func(ctx context.Context, req *http.Request) error {
		req.Header.Set("X-Request-ID", requestID)
		req.Header.Set("User-Agent", "kubernaut-context-api/1.0")
		return nil
	}

	// Make the request
	resp, err := c.client.ListIncidents(ctx, params, reqEditor)
	if err != nil {
		return nil, fmt.Errorf("failed to list incidents: %w", err)
	}
	defer resp.Body.Close()

	// Check for errors
	if resp.StatusCode >= 400 {
		return nil, c.parseError(resp)
	}

	// Parse response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var listResp IncidentListResponse
	if err := json.Unmarshal(body, &listResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Extract total from pagination
	total := int(listResp.Pagination.Total)

	return &IncidentsResult{
		Incidents: listResp.Data,
		Total:     total,
	}, nil
}

// GetIncidentByID retrieves a single incident by ID from the Data Storage Service
func (c *DataStorageClient) GetIncidentByID(ctx context.Context, id int) (*Incident, error) {
	// Add request ID for tracing
	requestID := uuid.New().String()
	reqEditor := func(ctx context.Context, req *http.Request) error {
		req.Header.Set("X-Request-ID", requestID)
		req.Header.Set("User-Agent", "kubernaut-context-api/1.0")
		return nil
	}

	// Make the request
	resp, err := c.client.GetIncidentByID(ctx, int64(id), reqEditor)
	if err != nil {
		return nil, fmt.Errorf("failed to get incident: %w", err)
	}
	defer resp.Body.Close()

	// Check for errors
	if resp.StatusCode == 404 {
		return nil, nil // Not found, return nil incident
	}
	if resp.StatusCode >= 400 {
		return nil, c.parseError(resp)
	}

	// Parse response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var incident Incident
	if err := json.Unmarshal(body, &incident); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &incident, nil
}

// parseError parses an RFC 7807 error response
func (c *DataStorageClient) parseError(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("HTTP %d: failed to read error body: %w", resp.StatusCode, err)
	}

	// Try to parse as RFC 7807 error
	var rfc7807Err RFC7807Error
	if err := json.Unmarshal(body, &rfc7807Err); err == nil && rfc7807Err.Title != "" {
		// Return structured RFC 7807 error (Context API errors package)
		// This preserves all error fields for consumers, not just the message
		return &contextapierrors.RFC7807Error{
			Type:     rfc7807Err.Type,
			Title:    rfc7807Err.Title,
			Detail:   stringValue(rfc7807Err.Detail),
			Status:   resp.StatusCode,
			Instance: stringValue(rfc7807Err.Instance),
		}
	}

	// Fallback to generic error
	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
}

// stringValue safely dereferences a string pointer
func stringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.BaseURL == "" {
		return fmt.Errorf("base URL required")
	}
	if c.Timeout < 0 {
		return fmt.Errorf("invalid timeout: must be >= 0")
	}
	if c.MaxConnections < 0 {
		return fmt.Errorf("invalid max connections: must be >= 0")
	}
	return nil
}

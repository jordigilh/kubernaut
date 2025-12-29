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
	"fmt"
	"net/http"
	"time"
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
// It provides a cleaner API with structured types and proper error handling
type DataStorageClient struct {
	client ClientWithResponsesInterface
	config Config
}

// NewDataStorageClient creates a new Data Storage HTTP client
func NewDataStorageClient(cfg Config) (*DataStorageClient, error) {
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
		return nil, fmt.Errorf("failed to create Data Storage client: %w", err)
	}

	return &DataStorageClient{
		client: generatedClient,
		config: cfg,
	}, nil
}

// CreateAuditEventResult holds the result of creating an audit event
// Commented out as CreateAuditEvent method is not implemented
/*
type CreateAuditEventResult struct {
	EventID   uuid.UUID
	CreatedAt time.Time
}
*/

// CreateAuditEvent creates a single audit event using structured types
//
// V1.0: Commented out due to type mismatches (use batch endpoint instead)
// Temporarily commented out to unblock development. Use batch endpoint instead via:
// pkg/datastorage/audit.NewOpenAPIAuditClient which uses PostApiV1AuditEventsBatchWithResponse
/*
func (c *DataStorageClient) CreateAuditEvent(ctx context.Context, event *audit.AuditEvent) (*CreateAuditEventResult, error) {
	// ... implementation commented out due to compilation errors ...
	return nil, fmt.Errorf("not implemented - use audit batch endpoint instead")
}
*/

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

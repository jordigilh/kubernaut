package datastorage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	dsmodels "github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"go.uber.org/zap"
)

// ========================================
// BR-INTEGRATION-008, BR-INTEGRATION-009, BR-INTEGRATION-010
// Data Storage HTTP Client
// ========================================
//
// Client provides HTTP REST API access to Data Storage Service
//
// ADR-033: Context API aggregates from Data Storage Service REST API
// ADR-032: No direct PostgreSQL access from Context API
//
// TDD GREEN Phase: Minimal implementation to pass unit tests
// TDD REFACTOR Phase: Extract constants, improve error handling
// ========================================

// Retry configuration constants
const (
	maxRetries        = 3
	initialRetryDelay = 100 * time.Millisecond
	retryBackoffFactor = 2
)

// Client is an HTTP client for Data Storage Service REST API
type Client struct {
	baseURL    string
	httpClient *http.Client
	logger     *zap.Logger
}

// NewClient creates a new Data Storage HTTP client
func NewClient(baseURL string, timeout time.Duration, logger *zap.Logger) *Client {
	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		logger: logger,
	}
}

// ========================================
// BR-INTEGRATION-008: Incident-Type Success Rate API
// ========================================

// GetSuccessRateByIncidentType retrieves success rate for a specific incident type
func (c *Client) GetSuccessRateByIncidentType(
	ctx context.Context,
	incidentType string,
	timeRange string,
	minSamples int,
) (*dsmodels.IncidentTypeSuccessRateResponse, error) {
	// Validation
	if incidentType == "" {
		return nil, fmt.Errorf("incident_type cannot be empty")
	}

	// Build URL with query parameters
	endpoint := fmt.Sprintf("%s/api/v1/success-rate/incident-type", c.baseURL)
	params := url.Values{}
	params.Add("incident_type", incidentType)
	params.Add("time_range", timeRange)
	params.Add("min_samples", strconv.Itoa(minSamples))

	fullURL := fmt.Sprintf("%s?%s", endpoint, params.Encode())

	c.logger.Debug("GetSuccessRateByIncidentType",
		zap.String("url", fullURL),
		zap.String("incident_type", incidentType),
		zap.String("time_range", timeRange),
		zap.Int("min_samples", minSamples))

	// Make HTTP request with retry logic
	var response dsmodels.IncidentTypeSuccessRateResponse
	err := c.doRequestWithRetry(ctx, http.MethodGet, fullURL, &response)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

// ========================================
// BR-INTEGRATION-009: Playbook Success Rate API
// ========================================

// GetSuccessRateByPlaybook retrieves success rate for a specific playbook
func (c *Client) GetSuccessRateByPlaybook(
	ctx context.Context,
	playbookID string,
	playbookVersion string,
	timeRange string,
	minSamples int,
) (*dsmodels.PlaybookSuccessRateResponse, error) {
	// Validation
	if playbookID == "" {
		return nil, fmt.Errorf("playbook_id cannot be empty")
	}

	// Build URL with query parameters
	endpoint := fmt.Sprintf("%s/api/v1/success-rate/playbook", c.baseURL)
	params := url.Values{}
	params.Add("playbook_id", playbookID)
	if playbookVersion != "" {
		params.Add("playbook_version", playbookVersion)
	}
	params.Add("time_range", timeRange)
	params.Add("min_samples", strconv.Itoa(minSamples))

	fullURL := fmt.Sprintf("%s?%s", endpoint, params.Encode())

	c.logger.Debug("GetSuccessRateByPlaybook",
		zap.String("url", fullURL),
		zap.String("playbook_id", playbookID),
		zap.String("playbook_version", playbookVersion))

	// Make HTTP request with retry logic
	var response dsmodels.PlaybookSuccessRateResponse
	err := c.doRequestWithRetry(ctx, http.MethodGet, fullURL, &response)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

// ========================================
// BR-INTEGRATION-010: Multi-Dimensional Success Rate API
// ========================================

// MultiDimensionalQuery represents query parameters for multi-dimensional aggregation
type MultiDimensionalQuery struct {
	IncidentType    string
	PlaybookID      string
	PlaybookVersion string
	ActionType      string
	TimeRange       string
	MinSamples      int
}

// GetSuccessRateMultiDimensional retrieves success rate across multiple dimensions
func (c *Client) GetSuccessRateMultiDimensional(
	ctx context.Context,
	query *MultiDimensionalQuery,
) (*dsmodels.MultiDimensionalSuccessRateResponse, error) {
	// Validation: at least one dimension must be specified
	if query.IncidentType == "" && query.PlaybookID == "" && query.ActionType == "" {
		return nil, fmt.Errorf("at least one dimension (incident_type, playbook_id, or action_type) must be specified")
	}

	// Build URL with query parameters
	endpoint := fmt.Sprintf("%s/api/v1/success-rate/multi-dimensional", c.baseURL)
	params := url.Values{}

	if query.IncidentType != "" {
		params.Add("incident_type", query.IncidentType)
	}
	if query.PlaybookID != "" {
		params.Add("playbook_id", query.PlaybookID)
	}
	if query.PlaybookVersion != "" {
		params.Add("playbook_version", query.PlaybookVersion)
	}
	if query.ActionType != "" {
		params.Add("action_type", query.ActionType)
	}
	params.Add("time_range", query.TimeRange)
	params.Add("min_samples", strconv.Itoa(query.MinSamples))

	fullURL := fmt.Sprintf("%s?%s", endpoint, params.Encode())

	c.logger.Debug("GetSuccessRateMultiDimensional",
		zap.String("url", fullURL),
		zap.String("incident_type", query.IncidentType),
		zap.String("playbook_id", query.PlaybookID),
		zap.String("action_type", query.ActionType))

	// Make HTTP request with retry logic
	var response dsmodels.MultiDimensionalSuccessRateResponse
	err := c.doRequestWithRetry(ctx, http.MethodGet, fullURL, &response)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

// ========================================
// HTTP Request Helper with Retry Logic
// ========================================

// doRequestWithRetry makes HTTP request with retry logic for 503 errors
// TDD REFACTOR: Use constants for retry configuration
func (c *Client) doRequestWithRetry(ctx context.Context, method, url string, result interface{}) error {
	retryDelay := initialRetryDelay

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := c.doRequest(ctx, method, url, result)
		if err == nil {
			return nil
		}

		lastErr = err

		// Retry only on 503 Service Unavailable
		if c.isRetryableError(err) && attempt < maxRetries {
			c.logger.Warn("Retrying request after retryable error",
				zap.String("url", url),
				zap.String("method", method),
				zap.Int("attempt", attempt),
				zap.Int("max_retries", maxRetries),
				zap.Duration("delay", retryDelay),
				zap.Error(err))
			time.Sleep(retryDelay)
			retryDelay *= retryBackoffFactor // Exponential backoff
			continue
		}

		// Don't retry for other errors
		c.logger.Error("Non-retryable error",
			zap.String("url", url),
			zap.String("method", method),
			zap.Int("attempt", attempt),
			zap.Error(err))
		return err
	}

	c.logger.Error("Max retries exceeded",
		zap.String("url", url),
		zap.String("method", method),
		zap.Int("max_retries", maxRetries),
		zap.Error(lastErr))
	return lastErr
}

// isRetryableError determines if an error should trigger a retry
// TDD REFACTOR: Extracted retry logic
func (c *Client) isRetryableError(err error) bool {
	return strings.Contains(err.Error(), "503")
}

// doRequest makes a single HTTP request
// TDD REFACTOR: Extracted error parsing to helper function
func (c *Client) doRequest(ctx context.Context, method, url string, result interface{}) error {
	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Check if context was cancelled
		if ctx.Err() != nil {
			c.logger.Warn("Request cancelled",
				zap.String("url", url),
				zap.String("method", method),
				zap.Error(ctx.Err()))
			return fmt.Errorf("request cancelled: %w", ctx.Err())
		}
		c.logger.Error("HTTP request failed",
			zap.String("url", url),
			zap.String("method", method),
			zap.Error(err))
		return fmt.Errorf("HTTP request failed (connection error): %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logger.Error("Failed to read response body",
			zap.String("url", url),
			zap.Int("status_code", resp.StatusCode),
			zap.Error(err))
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Check HTTP status code
	if resp.StatusCode != http.StatusOK {
		return c.parseErrorResponse(resp.StatusCode, body, url)
	}

	// Parse JSON response
	if err := json.Unmarshal(body, result); err != nil {
		c.logger.Error("Failed to parse JSON response",
			zap.String("url", url),
			zap.Int("status_code", resp.StatusCode),
			zap.Error(err))
		return fmt.Errorf("failed to parse JSON response: %w", err)
	}

	return nil
}

// parseErrorResponse parses RFC 7807 error responses
// TDD REFACTOR: Extracted RFC 7807 error parsing
func (c *Client) parseErrorResponse(statusCode int, body []byte, url string) error {
	// Try to parse RFC 7807 error response
	var problemDetails map[string]interface{}
	if json.Unmarshal(body, &problemDetails) == nil {
		if problemType, ok := problemDetails["type"].(string); ok {
			detail := problemDetails["detail"]
			c.logger.Error("Data Storage API error (RFC 7807)",
				zap.String("url", url),
				zap.Int("status_code", statusCode),
				zap.String("problem_type", problemType),
				zap.Any("detail", detail))
			return fmt.Errorf("Data Storage API error (HTTP %d): %s - %v",
				statusCode, problemType, detail)
		}
	}

	// Fallback to plain error message
	c.logger.Error("Data Storage API error",
		zap.String("url", url),
		zap.Int("status_code", statusCode),
		zap.String("body", string(body)))
	return fmt.Errorf("Data Storage API error (HTTP %d): %s", statusCode, string(body))
}


package health

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// HTTPHealthChecker performs HTTP-based health checks on service endpoints
// Business Requirement: BR-TOOLSET-012 - Health validation
//
// Design: Shared health check logic extracted from individual detectors
// to ensure consistent timeout, retry, and success criteria across all HTTP-based
// service health checks.
type HTTPHealthChecker struct {
	httpClient *http.Client
	timeout    time.Duration
	retries    int
	retryDelay time.Duration
}

// HTTPHealthCheckConfig configures the HTTP health checker behavior
type HTTPHealthCheckConfig struct {
	// Timeout for each individual health check attempt (default: 5s)
	Timeout time.Duration

	// Retries is the number of retry attempts for failed health checks (default: 3)
	Retries int

	// RetryDelay is the delay between retry attempts (default: 1s)
	RetryDelay time.Duration
}

// NewHTTPHealthChecker creates a new HTTP health checker with default configuration
func NewHTTPHealthChecker() *HTTPHealthChecker {
	return NewHTTPHealthCheckerWithConfig(HTTPHealthCheckConfig{
		Timeout:    5 * time.Second,
		Retries:    3,
		RetryDelay: 1 * time.Second,
	})
}

// NewHTTPHealthCheckerWithConfig creates a new HTTP health checker with custom configuration
func NewHTTPHealthCheckerWithConfig(config HTTPHealthCheckConfig) *HTTPHealthChecker {
	// Apply defaults
	if config.Timeout == 0 {
		config.Timeout = 5 * time.Second
	}
	if config.Retries == 0 {
		config.Retries = 3
	}
	if config.RetryDelay == 0 {
		config.RetryDelay = 1 * time.Second
	}

	return &HTTPHealthChecker{
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		timeout:    config.Timeout,
		retries:    config.Retries,
		retryDelay: config.RetryDelay,
	}
}

// Check performs an HTTP health check on the given endpoint with the specified path
// BR-TOOLSET-012: Implements health validation with retry logic
//
// Success criteria: HTTP 200 OK or 204 No Content
// Failure: Any other status code or network error
//
// Retries: Configured number of retries with exponential backoff
func (h *HTTPHealthChecker) Check(ctx context.Context, baseURL, healthPath string) error {
	healthURL := fmt.Sprintf("%s%s", baseURL, healthPath)

	var lastErr error
	for attempt := 0; attempt <= h.retries; attempt++ {
		if attempt > 0 {
			// Wait before retrying (exponential backoff)
			delay := h.retryDelay * time.Duration(attempt)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return fmt.Errorf("health check canceled: %w", ctx.Err())
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		if err != nil {
			lastErr = fmt.Errorf("failed to create health check request: %w", err)
			continue
		}

		resp, err := h.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("health check request failed (attempt %d/%d): %w", attempt+1, h.retries+1, err)
			continue
		}
		defer resp.Body.Close()

		// Success: HTTP 200 OK or 204 No Content
		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
			return nil
		}

		lastErr = fmt.Errorf("service unhealthy: status code %d (attempt %d/%d)", resp.StatusCode, attempt+1, h.retries+1)
	}

	return lastErr
}

// CheckSimple performs a simple health check without retries
// Useful for quick health checks where retries are not desired
func (h *HTTPHealthChecker) CheckSimple(ctx context.Context, baseURL, healthPath string) error {
	healthURL := fmt.Sprintf("%s%s", baseURL, healthPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("health check request failed: %w", err)
	}
	defer resp.Body.Close()

	// Success: HTTP 200 OK or 204 No Content
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
		return nil
	}

	return fmt.Errorf("service unhealthy: status code %d", resp.StatusCode)
}

// NewHTTPHealthCheckerWithClient creates a new HTTP health checker with a custom HTTP client
// This is primarily for testing purposes to inject mock HTTP clients
func NewHTTPHealthCheckerWithClient(client *http.Client) *HTTPHealthChecker {
	return &HTTPHealthChecker{
		httpClient: client,
		timeout:    1 * time.Millisecond,
		retries:    0,
		retryDelay: 0,
	}
}

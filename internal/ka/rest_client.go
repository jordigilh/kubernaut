package ka

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sony/gobreaker/v2"

	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
)

// Client is a REST client for the Kubernaut Agent API.
type Client struct {
	baseURL    string
	httpClient *http.Client
	cb         *gobreaker.CircuitBreaker[*http.Response]
}

// NewClient creates a new KA REST client with circuit breaker protection.
// The Token field provides a static fallback JWT; per-request identity delegation
// (extracting tokens from context) is wired in PR5 via the HTTP middleware layer.
//
//nolint:gocritic // hugeParam: value copy intentional; called once at startup
func NewClient(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	transport := http.DefaultTransport
	if cfg.Token != "" {
		transport = &auth.JWTDelegationTransport{
			Base:  http.DefaultTransport,
			Token: cfg.Token,
		}
	}

	cbMaxReqs := cfg.CBMaxRequests
	if cbMaxReqs == 0 {
		cbMaxReqs = 5
	}
	cbInterval := cfg.CBInterval
	if cbInterval == 0 {
		cbInterval = 60 * time.Second
	}
	cbTimeout := cfg.CBTimeout
	if cbTimeout == 0 {
		cbTimeout = 30 * time.Second
	}

	cb := gobreaker.NewCircuitBreaker[*http.Response](gobreaker.Settings{
		Name:        "ka-rest",
		MaxRequests: cbMaxReqs,
		Interval:    cbInterval,
		Timeout:     cbTimeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			threshold := cfg.CBFailureThreshold
			if threshold == 0 {
				threshold = 5
			}
			return counts.ConsecutiveFailures >= threshold
		},
	})

	return &Client{
		baseURL: cfg.BaseURL,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   timeout,
		},
		cb: cb,
	}
}

// Analyze starts an investigation via POST /api/v1/incident/analyze.
func (c *Client) Analyze(ctx context.Context, req AnalyzeRequest) (string, error) {
	resp, err := c.doJSON(ctx, http.MethodPost, "/api/v1/incident/analyze", req)
	if err != nil {
		return "", kaToUserFriendlyError(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return "", kaToUserFriendlyError(fmt.Errorf("KA analyze returned %d", resp.StatusCode))
	}

	var result struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding analyze response: %w", err)
	}
	return result.SessionID, nil
}

// Status polls investigation status via GET /api/v1/incident/session/{id}.
func (c *Client) Status(ctx context.Context, sessionID string) (*SessionStatus, error) {
	resp, err := c.doGet(ctx, fmt.Sprintf("/api/v1/incident/session/%s", sessionID))
	if err != nil {
		return nil, kaToUserFriendlyError(err)
	}
	defer func() { _ = resp.Body.Close() }()

	var status SessionStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("decoding status response: %w", err)
	}
	return &status, nil
}

// Result retrieves investigation results via GET /api/v1/incident/session/{id}/result.
func (c *Client) Result(ctx context.Context, sessionID string) (*IncidentResponse, error) {
	resp, err := c.doGet(ctx, fmt.Sprintf("/api/v1/incident/session/%s/result", sessionID))
	if err != nil {
		return nil, kaToUserFriendlyError(err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result IncidentResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding result response: %w", err)
	}
	return &result, nil
}

// Cancel cancels an investigation via POST /api/v1/incident/session/{id}/cancel.
func (c *Client) Cancel(ctx context.Context, sessionID string) error {
	resp, err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/api/v1/incident/session/%s/cancel", sessionID), nil)
	if err != nil {
		return kaToUserFriendlyError(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return kaToUserFriendlyError(fmt.Errorf("KA cancel returned %d", resp.StatusCode))
	}
	return nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		pr, pw := io.Pipe()
		go func() {
			pw.CloseWithError(json.NewEncoder(pw).Encode(body))
		}()
		bodyReader = pr
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	return c.cb.Execute(func() (*http.Response, error) {
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= 500 {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("KA returned server error %d", resp.StatusCode)
		}
		return resp, nil
	})
}

func (c *Client) doGet(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	return c.cb.Execute(func() (*http.Response, error) {
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= 500 {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("KA returned server error %d", resp.StatusCode)
		}
		return resp, nil
	})
}

// kaToUserFriendlyError sanitizes KA-originated errors for user presentation.
// Raw HTTP status codes, connection details, and internal messages are replaced
// with generic user-friendly alternatives.
func kaToUserFriendlyError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "connection refused") || strings.Contains(msg, "no such host") || strings.Contains(msg, "circuit breaker is open"):
		return fmt.Errorf("investigation service temporarily unavailable — please retry shortly")
	case strings.Contains(msg, "returned 403") || strings.Contains(msg, "Forbidden"):
		return fmt.Errorf("access denied: you do not have permission to perform this investigation")
	case strings.Contains(msg, "returned 404") || strings.Contains(msg, "Not Found"):
		return fmt.Errorf("not found: the requested investigation session does not exist")
	case strings.Contains(msg, "returned 5") || strings.Contains(msg, "server error"):
		return fmt.Errorf("internal error in investigation service — please retry or contact support")
	default:
		return fmt.Errorf("investigation service error — please retry or contact support")
	}
}

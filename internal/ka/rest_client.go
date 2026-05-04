package ka

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
)

// Client is a REST client for the Kubernaut Agent API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new KA REST client.
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

	return &Client{
		baseURL: cfg.BaseURL,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   timeout,
		},
	}
}

// Analyze starts an investigation via POST /api/v1/incident/analyze.
func (c *Client) Analyze(ctx context.Context, req AnalyzeRequest) (string, error) {
	resp, err := c.doJSON(ctx, http.MethodPost, "/api/v1/incident/analyze", req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("KA analyze returned %d", resp.StatusCode)
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
		return nil, err
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
		return nil, err
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
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("KA cancel returned %d", resp.StatusCode)
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

	return c.httpClient.Do(req)
}

func (c *Client) doGet(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	return c.httpClient.Do(req)
}

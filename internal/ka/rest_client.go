package ka

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	gobreaker "github.com/sony/gobreaker/v2"

	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
	"github.com/jordigilh/kubernaut-apifrontend/internal/requestid"
	"github.com/jordigilh/kubernaut-apifrontend/internal/resilience"
)

// Client is a REST client for the Kubernaut Agent API.
type Client struct {
	baseURL     string
	httpClient  *http.Client
	cbTransport *resilience.CircuitBreakerTransport
}

// ClientMetrics holds Prometheus collectors for the KA client.
type ClientMetrics struct {
	StateGauge   *prometheus.GaugeVec
	DurationHist *prometheus.HistogramVec
	RetryCounter *prometheus.CounterVec
}

// NewClient creates a new KA REST client with circuit breaker and retry protection.
// metrics is optional and may be nil if metrics are not needed.
//
//nolint:gocritic // hugeParam: called once at startup; value copy is acceptable
func NewClient(cfg Config, metrics ...*ClientMetrics) *Client {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	underlying := cfg.BaseTransport
	if underlying == nil {
		underlying = http.DefaultTransport
	}
	var baseTransport http.RoundTripper = &requestid.Transport{Base: underlying}
	if cfg.Token != "" {
		baseTransport = &auth.JWTDelegationTransport{
			Base:  baseTransport,
			Token: cfg.Token,
		}
	}

	// Build the resilience transport chain: CB -> Retry -> Auth/Base
	var retryCounter *prometheus.CounterVec
	var stateGauge *prometheus.GaugeVec
	var durationHist *prometheus.HistogramVec
	if len(metrics) > 0 && metrics[0] != nil {
		retryCounter = metrics[0].RetryCounter
		stateGauge = metrics[0].StateGauge
		durationHist = metrics[0].DurationHist
	}

	retryRT := resilience.NewRetryTransport(baseTransport, &resilience.RetryConfig{
		MaxAttempts:       cfg.RetryMax + 1,
		InitialBackoff:    cfg.RetryInitBackoff,
		MaxBackoff:        cfg.RetryMaxBackoff,
		RetryableStatuses: cfg.RetryableStatuses,
		RetryCounter:      retryCounter,
		DependencyName:    "ka",
		IdempotentOnly:    true,
	})

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
	cbFailureThreshold := cfg.CBFailureThreshold
	if cbFailureThreshold == 0 {
		cbFailureThreshold = 5
	}

	cbt := resilience.NewCircuitBreakerTransport(retryRT, &resilience.CircuitBreakerConfig{
		Name:             "ka-rest",
		MaxRequests:      cbMaxReqs,
		Interval:         cbInterval,
		Timeout:          cbTimeout,
		FailureThreshold: cbFailureThreshold,
		StateGauge:       stateGauge,
		DurationHist:     durationHist,
		DependencyName:   "ka",
	})

	return &Client{
		baseURL: cfg.BaseURL,
		httpClient: &http.Client{
			Transport: cbt,
			Timeout:   timeout,
		},
		cbTransport: cbt,
	}
}

// State returns the current circuit breaker state.
func (c *Client) State() gobreaker.State {
	return c.cbTransport.State()
}

// Healthy returns true when the circuit breaker is not in the Open state.
func (c *Client) Healthy() bool {
	return c.cbTransport.State() != gobreaker.StateOpen
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
	var getBody func() (io.ReadCloser, error)

	if body != nil {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return nil, fmt.Errorf("encoding request body: %w", err)
		}
		bodyBytes := buf.Bytes()
		bodyReader = bytes.NewReader(bodyBytes)
		getBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(bodyBytes)), nil
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if getBody != nil {
		req.GetBody = getBody
	}

	return c.httpClient.Do(req)
}

func (c *Client) doGet(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	return c.httpClient.Do(req)
}

// kaToUserFriendlyError sanitizes KA-originated errors for user presentation.
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

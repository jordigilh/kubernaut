package prometheus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/jordigilh/kubernaut-apifrontend/internal/security"
)

func redactErr(origErr error) error {
	return errors.New(security.RedactError(origErr))
}

const maxResponseBodyBytes = 10 * 1024 * 1024 // 10 MB

// Client defines the interface for querying Prometheus APIs.
type Client interface {
	GetAlerts(ctx context.Context) ([]Alert, error)
	GetRules(ctx context.Context) ([]RuleGroup, error)
	InstantQuery(ctx context.Context, query string) (*QueryResult, error)
}

type httpClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewHTTPClient creates a Prometheus HTTP client. The caller provides a
// pre-configured *http.Client for TLS, bearer token, and timeout composition.
func NewHTTPClient(baseURL string, client *http.Client) Client {
	return &httpClient{
		baseURL:    baseURL,
		httpClient: client,
	}
}

// GetAlerts queries /api/v1/alerts and returns all alerts.
func (c *httpClient) GetAlerts(ctx context.Context) ([]Alert, error) {
	reqURL := fmt.Sprintf("%s/api/v1/alerts", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, redactErr(fmt.Errorf("creating alerts request: %w", err))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, redactErr(fmt.Errorf("querying alerts: %w", err))
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, redactErr(fmt.Errorf("querying alerts: HTTP %d", resp.StatusCode))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
	if err != nil {
		return nil, redactErr(fmt.Errorf("reading alerts response: %w", err))
	}

	var apiResp alertsAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("parsing alerts response: %w", err)
	}

	if apiResp.Status != "success" {
		return nil, redactErr(fmt.Errorf("alerts API error: %s", apiResp.Error))
	}

	alerts := make([]Alert, 0, len(apiResp.Data.Alerts))
	for _, a := range apiResp.Data.Alerts {
		alert := Alert{
			Labels:      a.Labels,
			Annotations: a.Annotations,
			State:       a.State,
		}
		if a.ActiveAt != "" {
			if t, parseErr := time.Parse(time.RFC3339, a.ActiveAt); parseErr == nil {
				alert.ActiveAt = t
			}
		}
		alerts = append(alerts, alert)
	}
	return alerts, nil
}

// GetRules queries /api/v1/rules and returns all rule groups.
func (c *httpClient) GetRules(ctx context.Context) ([]RuleGroup, error) {
	reqURL := fmt.Sprintf("%s/api/v1/rules", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, redactErr(fmt.Errorf("creating rules request: %w", err))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, redactErr(fmt.Errorf("querying rules: %w", err))
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, redactErr(fmt.Errorf("querying rules: HTTP %d", resp.StatusCode))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
	if err != nil {
		return nil, redactErr(fmt.Errorf("reading rules response: %w", err))
	}

	var apiResp rulesAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("parsing rules response: %w", err)
	}

	if apiResp.Status != "success" {
		return nil, redactErr(fmt.Errorf("rules API error: %s", apiResp.Error))
	}

	groups := make([]RuleGroup, 0, len(apiResp.Data.Groups))
	for _, g := range apiResp.Data.Groups {
		rg := RuleGroup{Name: g.Name, File: g.File}
		for _, r := range g.Rules {
			rg.Rules = append(rg.Rules, Rule{
				Name:        r.Alert,
				Query:       r.Expr,
				Duration:    r.Duration,
				Labels:      r.Labels,
				Annotations: r.Annotations,
				State:       r.State,
				Type:        r.Type,
			})
		}
		groups = append(groups, rg)
	}
	return groups, nil
}

// InstantQuery evaluates a PromQL expression via /api/v1/query.
func (c *httpClient) InstantQuery(ctx context.Context, query string) (*QueryResult, error) {
	params := url.Values{}
	params.Set("query", query)

	reqURL := fmt.Sprintf("%s/api/v1/query?%s", c.baseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, redactErr(fmt.Errorf("creating query request: %w", err))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, redactErr(fmt.Errorf("executing query: %w", err))
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, redactErr(fmt.Errorf("executing query: HTTP %d", resp.StatusCode))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
	if err != nil {
		return nil, redactErr(fmt.Errorf("reading query response: %w", err))
	}

	var apiResp queryAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("parsing query response: %w", err)
	}

	if apiResp.Status != "success" {
		return nil, redactErr(fmt.Errorf("query API error: %s", apiResp.Error))
	}

	result := &QueryResult{Samples: make([]Sample, 0)}
	for _, raw := range apiResp.Data.Result {
		var vr vectorResult
		if err := json.Unmarshal(raw, &vr); err != nil {
			continue
		}
		sample := Sample{Metric: vr.Metric}
		if len(vr.Value) == 2 {
			if ts, ok := vr.Value[0].(float64); ok {
				sample.Timestamp = time.Unix(int64(ts), 0)
			}
			switch v := vr.Value[1].(type) {
			case string:
				if val, parseErr := strconv.ParseFloat(v, 64); parseErr == nil {
					sample.Value = val
				}
			case float64:
				sample.Value = v
			}
		}
		result.Samples = append(result.Samples, sample)
	}
	return result, nil
}

// --- API response types ---

type alertsAPIResponse struct {
	Status string         `json:"status"`
	Data   alertsAPIData  `json:"data"`
	Error  string         `json:"error,omitempty"`
}

type alertsAPIData struct {
	Alerts []apiAlert `json:"alerts"`
}

type apiAlert struct {
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	State       string            `json:"state"`
	ActiveAt    string            `json:"activeAt"`
}

type rulesAPIResponse struct {
	Status string        `json:"status"`
	Data   rulesAPIData  `json:"data"`
	Error  string        `json:"error,omitempty"`
}

type rulesAPIData struct {
	Groups []apiRuleGroup `json:"groups"`
}

type apiRuleGroup struct {
	Name  string    `json:"name"`
	File  string    `json:"file"`
	Rules []apiRule `json:"rules"`
}

type apiRule struct {
	Alert       string            `json:"alert"`
	Expr        string            `json:"expr"`
	Duration    float64           `json:"duration"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	State       string            `json:"state"`
	Type        string            `json:"type"`
}

type queryAPIResponse struct {
	Status string       `json:"status"`
	Data   queryAPIData `json:"data"`
	Error  string       `json:"error,omitempty"`
}

type queryAPIData struct {
	ResultType string            `json:"resultType"`
	Result     []json.RawMessage `json:"result"`
}

type vectorResult struct {
	Metric map[string]string `json:"metric"`
	Value  []interface{}     `json:"value"`
}

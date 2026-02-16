/*
Copyright 2026 Jordi Gil.

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
	"net/url"
	"strings"
	"time"
)

// alertManagerHTTPClient implements AlertManagerClient via the AlertManager HTTP API.
// Used by the EM controller for alert resolution scoring (BR-EM-002).
//
// Integration tests: connects to httptest.NewServer mock (ephemeral port)
// E2E tests: connects to real AlertManager container (NodePort 30193)
type alertManagerHTTPClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewAlertManagerHTTPClient creates an AlertManagerClient that connects to an AlertManager HTTP API.
func NewAlertManagerHTTPClient(baseURL string, timeout time.Duration) AlertManagerClient {
	return &alertManagerHTTPClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// GetAlerts retrieves active alerts matching the given filters.
func (c *alertManagerHTTPClient) GetAlerts(ctx context.Context, filters AlertFilters) ([]Alert, error) {
	params := url.Values{}
	for _, matcher := range filters.Matchers {
		params.Add("filter", matcher)
	}

	reqURL := fmt.Sprintf("%s/api/v2/alerts?%s", c.baseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating AlertManager alerts request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing AlertManager alerts request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("AlertManager alerts returned status %d: %s", resp.StatusCode, string(body))
	}

	return parseAMAlerts(resp.Body)
}

// Ready checks if AlertManager is ready to accept requests.
func (c *alertManagerHTTPClient) Ready(ctx context.Context) error {
	reqURL := fmt.Sprintf("%s/-/ready", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("creating AlertManager ready request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("AlertManager ready check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("AlertManager not ready: status %d", resp.StatusCode)
	}
	return nil
}

// amAPIAlert represents an alert from the AlertManager v2 API.
type amAPIAlert struct {
	Labels   map[string]string `json:"labels"`
	Status   amAlertStatus     `json:"status"`
	StartsAt string            `json:"startsAt"`
	EndsAt   string            `json:"endsAt"`
}

type amAlertStatus struct {
	State string `json:"state"` // active, suppressed, unprocessed
}

// parseAMAlerts parses the AlertManager /api/v2/alerts response.
func parseAMAlerts(body io.Reader) ([]Alert, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("reading AlertManager response body: %w", err)
	}

	var apiAlerts []amAPIAlert
	if err := json.Unmarshal(data, &apiAlerts); err != nil {
		return nil, fmt.Errorf("parsing AlertManager response: %w", err)
	}

	alerts := make([]Alert, 0, len(apiAlerts))
	for _, aa := range apiAlerts {
		alert := Alert{
			Labels: aa.Labels,
			State:  aa.Status.State,
		}

		// Parse timestamps
		if aa.StartsAt != "" {
			if t, err := time.Parse(time.RFC3339, strings.TrimSpace(aa.StartsAt)); err == nil {
				alert.StartsAt = t
			}
		}
		if aa.EndsAt != "" {
			if t, err := time.Parse(time.RFC3339, strings.TrimSpace(aa.EndsAt)); err == nil {
				alert.EndsAt = t
			}
		}

		alerts = append(alerts, alert)
	}

	return alerts, nil
}

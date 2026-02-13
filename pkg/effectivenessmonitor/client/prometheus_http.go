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
	"strconv"
	"time"
)

// prometheusHTTPClient implements PrometheusQuerier via the Prometheus HTTP API.
// Used by the EM controller for metric comparison scoring (BR-EM-003).
//
// Integration tests: connects to httptest.NewServer mock (ephemeral port)
// E2E tests: connects to real Prometheus container (NodePort 30190)
type prometheusHTTPClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewPrometheusHTTPClient creates a PrometheusQuerier that connects to a Prometheus HTTP API.
func NewPrometheusHTTPClient(baseURL string, timeout time.Duration) PrometheusQuerier {
	return &prometheusHTTPClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// Query executes an instant PromQL query.
func (c *prometheusHTTPClient) Query(ctx context.Context, query string, ts time.Time) (*QueryResult, error) {
	params := url.Values{}
	params.Set("query", query)
	if !ts.IsZero() {
		params.Set("time", strconv.FormatFloat(float64(ts.Unix()), 'f', -1, 64))
	}

	reqURL := fmt.Sprintf("%s/api/v1/query?%s", c.baseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating Prometheus query request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing Prometheus query: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Prometheus query returned status %d: %s", resp.StatusCode, string(body))
	}

	return parsePromResponse(resp.Body)
}

// QueryRange executes a range PromQL query.
func (c *prometheusHTTPClient) QueryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) (*QueryResult, error) {
	params := url.Values{}
	params.Set("query", query)
	params.Set("start", strconv.FormatFloat(float64(start.Unix()), 'f', -1, 64))
	params.Set("end", strconv.FormatFloat(float64(end.Unix()), 'f', -1, 64))
	params.Set("step", step.String())

	reqURL := fmt.Sprintf("%s/api/v1/query_range?%s", c.baseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating Prometheus query_range request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing Prometheus query_range: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Prometheus query_range returned status %d: %s", resp.StatusCode, string(body))
	}

	return parsePromResponse(resp.Body)
}

// Ready checks if Prometheus is ready to accept queries.
func (c *prometheusHTTPClient) Ready(ctx context.Context) error {
	reqURL := fmt.Sprintf("%s/-/ready", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("creating Prometheus ready request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("Prometheus ready check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Prometheus not ready: status %d", resp.StatusCode)
	}
	return nil
}

// promAPIResponse represents the standard Prometheus API response.
type promAPIResponse struct {
	Status string          `json:"status"`
	Data   promAPIData     `json:"data"`
	Error  string          `json:"error,omitempty"`
}

type promAPIData struct {
	ResultType string            `json:"resultType"`
	Result     []json.RawMessage `json:"result"`
}

type promVectorResult struct {
	Metric map[string]string `json:"metric"`
	Value  [2]interface{}    `json:"value"` // [timestamp, value_string]
}

type promMatrixResult struct {
	Metric map[string]string `json:"metric"`
	Values [][2]interface{}  `json:"values"` // [[timestamp, value_string], ...]
}

// parsePromResponse parses a Prometheus API response into a QueryResult.
func parsePromResponse(body io.Reader) (*QueryResult, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("reading Prometheus response body: %w", err)
	}

	var apiResp promAPIResponse
	if err := json.Unmarshal(data, &apiResp); err != nil {
		return nil, fmt.Errorf("parsing Prometheus response: %w", err)
	}

	if apiResp.Status != "success" {
		return nil, fmt.Errorf("Prometheus API error: %s", apiResp.Error)
	}

	result := &QueryResult{
		Samples: make([]Sample, 0),
	}

	switch apiResp.Data.ResultType {
	case "vector":
		for _, raw := range apiResp.Data.Result {
			var vr promVectorResult
			if err := json.Unmarshal(raw, &vr); err != nil {
				continue
			}
			sample, err := parseVectorSample(vr)
			if err != nil {
				continue
			}
			result.Samples = append(result.Samples, sample)
		}
	case "matrix":
		for _, raw := range apiResp.Data.Result {
			var mr promMatrixResult
			if err := json.Unmarshal(raw, &mr); err != nil {
				continue
			}
			// Return ALL data points from the matrix result.
			// QueryRange returns time series with multiple [timestamp, value] pairs;
			// callers need the full set to compare earliest vs latest (pre/post remediation).
			for _, valuePair := range mr.Values {
				sample, err := parseValuePair(mr.Metric, valuePair)
				if err != nil {
					continue
				}
				result.Samples = append(result.Samples, sample)
			}
		}
	}

	return result, nil
}

func parseVectorSample(vr promVectorResult) (Sample, error) {
	return parseValuePair(vr.Metric, vr.Value)
}

func parseValuePair(metric map[string]string, valuePair [2]interface{}) (Sample, error) {
	var sample Sample
	sample.Metric = metric

	// Parse timestamp
	switch ts := valuePair[0].(type) {
	case float64:
		sample.Timestamp = time.Unix(int64(ts), 0)
	}

	// Parse value
	switch v := valuePair[1].(type) {
	case string:
		val, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return sample, fmt.Errorf("parsing sample value %q: %w", v, err)
		}
		sample.Value = val
	case float64:
		sample.Value = v
	}

	return sample, nil
}

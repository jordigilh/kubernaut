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

package executor

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// AWXHTTPClient implements AWXClient using the AWX REST API.
type AWXHTTPClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewAWXHTTPClient creates a new AWX REST API client.
func NewAWXHTTPClient(baseURL, token string, insecure bool) *AWXHTTPClient {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if insecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}
	return &AWXHTTPClient{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
	}
}

func (c *AWXHTTPClient) LaunchJobTemplate(ctx context.Context, templateID int, extraVars map[string]interface{}) (int, error) {
	url := fmt.Sprintf("%s/api/v2/job_templates/%d/launch/", c.baseURL, templateID)

	payload := map[string]interface{}{}
	if len(extraVars) > 0 {
		payload["extra_vars"] = extraVars
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("marshal launch payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("create launch request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("AWX launch request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("AWX launch returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decode launch response: %w", err)
	}

	return result.ID, nil
}

func (c *AWXHTTPClient) GetJobStatus(ctx context.Context, jobID int) (*AWXJobStatus, error) {
	url := fmt.Sprintf("%s/api/v2/jobs/%d/", c.baseURL, jobID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create status request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("AWX status request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("AWX status returned %d: %s", resp.StatusCode, string(respBody))
	}

	var status AWXJobStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("decode status response: %w", err)
	}

	return &status, nil
}

func (c *AWXHTTPClient) CancelJob(ctx context.Context, jobID int) error {
	url := fmt.Sprintf("%s/api/v2/jobs/%d/cancel/", c.baseURL, jobID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("create cancel request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("AWX cancel request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("AWX cancel returned %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (c *AWXHTTPClient) FindJobTemplateByName(ctx context.Context, name string) (int, error) {
	url := fmt.Sprintf("%s/api/v2/job_templates/?name=%s", c.baseURL, name)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("create template search request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("AWX template search request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("AWX template search returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Count   int `json:"count"`
		Results []struct {
			ID int `json:"id"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decode template search response: %w", err)
	}

	if result.Count == 0 {
		return 0, fmt.Errorf("AWX job template %q not found", name)
	}

	return result.Results[0].ID, nil
}

func (c *AWXHTTPClient) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
}

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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
)

// AWXHTTPClient implements AWXClient using the AWX REST API.
type AWXHTTPClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// AWXAPIError preserves the HTTP status returned by AWX so callers can make
// idempotency decisions without parsing human-readable error strings.
type AWXAPIError struct {
	Operation  string
	StatusCode int
	Body       string
}

func (e *AWXAPIError) Error() string {
	return fmt.Sprintf("AWX %s returned %d: %s", e.Operation, e.StatusCode, e.Body)
}

// NewAWXHTTPClient creates a new AWX REST API client.
// Issue #902: Uses sharedtls.DefaultBaseTransport() to honour TLS_CA_FILE.
func NewAWXHTTPClient(baseURL, token string) (*AWXHTTPClient, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("baseURL must not be empty")
	}
	if token == "" {
		return nil, fmt.Errorf("token must not be empty")
	}

	rt, err := sharedtls.DefaultBaseTransport()
	if err != nil {
		return nil, fmt.Errorf("initialize TLS transport: %w", err)
	}

	return &AWXHTTPClient{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: rt,
		},
	}, nil
}

func (c *AWXHTTPClient) LaunchJobTemplate(ctx context.Context, templateID int, extraVars map[string]interface{}) (int, error) {
	return c.launchJobTemplate(ctx, templateID, extraVars, nil, nil)
}

func (c *AWXHTTPClient) LaunchJobTemplateWithLabels(ctx context.Context, templateID int, extraVars map[string]interface{}, credentialIDs []int, labels []string) (int, error) {
	return c.launchJobTemplate(ctx, templateID, extraVars, credentialIDs, labels)
}

func (c *AWXHTTPClient) launchJobTemplate(ctx context.Context, templateID int, extraVars map[string]interface{}, credentialIDs []int, labels []string) (int, error) {
	launchURL := fmt.Sprintf("%s/api/v2/job_templates/%d/launch/", c.baseURL, templateID)

	payload := map[string]interface{}{}
	if len(extraVars) > 0 {
		payload["extra_vars"] = extraVars
	}
	if len(credentialIDs) > 0 {
		payload["credentials"] = credentialIDs
	}
	if len(labels) > 0 {
		payload["labels"] = labels
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("marshal launch payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, launchURL, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("create launch request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("%w: AWX launch request: %w", ErrAWXLaunchRecoveryPending, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		apiErr := &AWXAPIError{Operation: "launch", StatusCode: resp.StatusCode, Body: string(respBody)}
		if resp.StatusCode >= http.StatusInternalServerError {
			return 0, fmt.Errorf("%w: %w", ErrAWXLaunchRecoveryPending, apiErr)
		}
		return 0, apiErr
	}

	var result struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("%w: decode launch response: %w", ErrAWXLaunchRecoveryPending, err)
	}
	if result.ID <= 0 {
		return 0, fmt.Errorf("%w: AWX launch response did not include a valid job ID", ErrAWXLaunchRecoveryPending)
	}

	return result.ID, nil
}

func (c *AWXHTTPClient) GetJobStatus(ctx context.Context, jobID int) (*AWXJobStatus, error) {
	body, err := c.getAWXResource(ctx, fmt.Sprintf("%s/api/v2/jobs/%d/", c.baseURL, jobID), "status")
	if err != nil {
		return nil, err
	}

	var status AWXJobStatus
	if err := json.Unmarshal(body, &status); err != nil {
		return nil, fmt.Errorf("decode status response: %w", err)
	}
	return &status, nil
}

func (c *AWXHTTPClient) GetJobTemplate(ctx context.Context, templateID int) (*AWXJobTemplate, error) {
	body, err := c.getAWXResource(ctx, fmt.Sprintf("%s/api/v2/job_templates/%d/", c.baseURL, templateID), "job template")
	if err != nil {
		return nil, err
	}

	var template AWXJobTemplate
	if err := json.Unmarshal(body, &template); err != nil {
		return nil, fmt.Errorf("decode AWX job template response: %w", err)
	}
	return &template, nil
}

func (c *AWXHTTPClient) getAWXResource(ctx context.Context, requestURL, operation string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create %s request: %w", operation, err)
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("AWX %s request: %w", operation, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, &AWXAPIError{Operation: operation, StatusCode: resp.StatusCode, Body: string(respBody)}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s response: %w", operation, err)
	}
	return body, nil
}

func (c *AWXHTTPClient) FindJobsByLabel(ctx context.Context, label string) ([]AWXJobStatus, error) {
	query := url.Values{}
	query.Set("labels__name", label)
	listURL := fmt.Sprintf("%s/api/v2/jobs/?%s", c.baseURL, query.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create AWX job label search request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("AWX job label search request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, &AWXAPIError{Operation: "job label search", StatusCode: resp.StatusCode, Body: string(respBody)}
	}

	var result struct {
		Count   int            `json:"count"`
		Results []AWXJobStatus `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode AWX job label search response: %w", err)
	}
	if result.Count > len(result.Results) {
		return nil, fmt.Errorf("AWX job label search returned %d matches but only %d results; refusing ambiguous recovery", result.Count, len(result.Results))
	}
	return result.Results, nil
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
		return &AWXAPIError{Operation: "cancel", StatusCode: resp.StatusCode, Body: string(respBody)}
	}

	return nil
}

// findFirstIDByListQuery performs a GET request against an AWX list endpoint
// filtered by a query string, decodes a `{count, results: [{id}]}` envelope,
// and returns the first matching result's ID (dupl: shared by
// FindJobTemplateByName/FindCredentialTypeByName, which were byte-for-byte
// identical apart from the URL path and the entity name used in error
// messages).
func (c *AWXHTTPClient) findFirstIDByListQuery(ctx context.Context, listURL, entity, notFoundValue string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return 0, fmt.Errorf("create %s search request: %w", entity, err)
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("AWX %s search request: %w", entity, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("AWX %s search returned %d: %s", entity, resp.StatusCode, string(respBody))
	}

	var result struct {
		Count   int `json:"count"`
		Results []struct {
			ID int `json:"id"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decode %s search response: %w", entity, err)
	}

	if result.Count == 0 {
		return 0, fmt.Errorf("AWX %s %q not found", entity, notFoundValue)
	}

	return result.Results[0].ID, nil
}

func (c *AWXHTTPClient) FindJobTemplateByName(ctx context.Context, name string) (int, error) {
	url := fmt.Sprintf("%s/api/v2/job_templates/?name=%s", c.baseURL, name)
	return c.findFirstIDByListQuery(ctx, url, "job template", name)
}

func (c *AWXHTTPClient) CreateCredentialType(ctx context.Context, name string, inputs CredentialTypeInputs, injectors CredentialTypeInjectors) (int, error) {
	url := fmt.Sprintf("%s/api/v2/credential_types/", c.baseURL)

	payload := map[string]interface{}{
		"name":      name,
		"kind":      "cloud",
		"inputs":    inputs,
		"injectors": injectors,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("marshal credential type payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("create credential type request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("AWX credential type request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("AWX create credential type returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decode credential type response: %w", err)
	}

	return result.ID, nil
}

func (c *AWXHTTPClient) FindCredentialTypeByName(ctx context.Context, name string) (int, error) {
	url := fmt.Sprintf("%s/api/v2/credential_types/?name=%s", c.baseURL, name)
	return c.findFirstIDByListQuery(ctx, url, "credential type", name)
}

func (c *AWXHTTPClient) FindCredentialTypeByKind(ctx context.Context, kind string, managed bool) (int, error) {
	url := fmt.Sprintf("%s/api/v2/credential_types/?kind=%s&managed=%t", c.baseURL, kind, managed)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("create credential type kind search request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("AWX credential type kind search request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("AWX credential type kind search returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Count   int `json:"count"`
		Results []struct {
			ID int `json:"id"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decode credential type kind search response: %w", err)
	}

	if result.Count == 0 {
		return 0, fmt.Errorf("AWX credential type with kind=%q managed=%t not found", kind, managed)
	}

	return result.Results[0].ID, nil
}

func (c *AWXHTTPClient) CreateCredential(ctx context.Context, name string, credTypeID, orgID int, inputs map[string]string) (int, error) {
	url := fmt.Sprintf("%s/api/v2/credentials/", c.baseURL)

	typedInputs := make(map[string]interface{}, len(inputs))
	for k, v := range inputs {
		typedInputs[k] = v
	}

	payload := map[string]interface{}{
		"name":            name,
		"credential_type": credTypeID,
		"organization":    orgID,
		"inputs":          typedInputs,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("marshal credential payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("create credential request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("AWX credential request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("AWX create credential returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decode credential response: %w", err)
	}

	return result.ID, nil
}

func (c *AWXHTTPClient) DeleteCredential(ctx context.Context, credentialID int) error {
	url := fmt.Sprintf("%s/api/v2/credentials/%d/", c.baseURL, credentialID)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("create delete credential request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("AWX delete credential request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("AWX delete credential returned %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (c *AWXHTTPClient) LaunchJobTemplateWithCreds(ctx context.Context, templateID int, extraVars map[string]interface{}, credentialIDs []int) (int, error) {
	return c.launchJobTemplate(ctx, templateID, extraVars, credentialIDs, nil)
}

func (c *AWXHTTPClient) GetJobTemplateCredentials(ctx context.Context, templateID int) ([]int, error) {
	url := fmt.Sprintf("%s/api/v2/job_templates/%d/credentials/", c.baseURL, templateID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create credentials request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("AWX credentials request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("AWX credentials returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Results []struct {
			ID int `json:"id"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode credentials response: %w", err)
	}

	ids := make([]int, len(result.Results))
	for i, r := range result.Results {
		ids[i] = r.ID
	}
	return ids, nil
}

func (c *AWXHTTPClient) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
}

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
	"strings"
	"time"

	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
	sigsyaml "sigs.k8s.io/yaml"
)

// AWXHTTPClient implements AWXClient using the AWX REST API.
type AWXHTTPClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
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

// FindJobByExecutionIdentity searches recent AWX jobs for the exact WFE
// identity injected into extra_vars. AWX does not permit filtering jobs by
// extra_vars on all supported versions, so the bounded recent-job scan is
// intentional; an ambiguous match fails closed rather than adopting either
// job (DD-2582).
func (c *AWXHTTPClient) FindJobByExecutionIdentity(ctx context.Context, wfeName, wfeNamespace, wfeUID string) (int, error) {
	var matches []int
	pageURL := fmt.Sprintf("%s/api/v2/jobs/?order_by=-id&page_size=100", c.baseURL)
	visited := make(map[string]struct{})
	for pageURL != "" {
		if _, seen := visited[pageURL]; seen {
			return 0, fmt.Errorf("AWX execution identity search pagination repeated %q", pageURL)
		}
		visited[pageURL] = struct{}{}
		nextURL, jobs, err := c.fetchAWXJobPage(ctx, pageURL)
		if err != nil {
			return 0, err
		}
		for _, job := range jobs {
			vars, err := decodeAWXExtraVars(job.ExtraVars)
			if err != nil {
				// AWX permits legacy/YAML/empty extra_vars values. An unrelated
				// record that cannot be decoded is not evidence of this WFE and
				// must not prevent recovery of an exact later match.
				continue
			}
			if awxExecutionIdentityMatches(vars, wfeName, wfeNamespace, wfeUID) {
				matches = append(matches, job.ID)
			}
		}
		pageURL = c.normalizeAWXNextURL(nextURL)
	}
	if len(matches) > 1 {
		return 0, fmt.Errorf("multiple AWX jobs match WFE identity %s/%s", wfeNamespace, wfeName)
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	return 0, nil
}

type awxJobPageRecord struct {
	ID        int             `json:"id"`
	ExtraVars json.RawMessage `json:"extra_vars"`
}

func (c *AWXHTTPClient) fetchAWXJobPage(ctx context.Context, pageURL string) (string, []awxJobPageRecord, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return "", nil, fmt.Errorf("create AWX execution identity search request: %w", err)
	}
	c.setHeaders(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("AWX execution identity search request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", nil, fmt.Errorf("AWX execution identity search returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Next    string             `json:"next"`
		Results []awxJobPageRecord `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", nil, fmt.Errorf("decode AWX execution identity search response: %w", err)
	}
	return result.Next, result.Results, nil
}

func (c *AWXHTTPClient) normalizeAWXNextURL(nextURL string) string {
	if strings.HasPrefix(nextURL, "/") {
		return c.baseURL + nextURL
	}
	return nextURL
}

func decodeAWXExtraVars(raw json.RawMessage) (map[string]interface{}, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]interface{}{}, nil
	}
	var vars map[string]interface{}
	if err := json.Unmarshal(raw, &vars); err == nil {
		return vars, nil
	}
	if err := sigsyaml.Unmarshal(raw, &vars); err == nil {
		return vars, nil
	}
	var encoded string
	if err := json.Unmarshal(raw, &encoded); err != nil {
		return nil, fmt.Errorf("decode extra_vars value: %w", err)
	}
	if err := json.Unmarshal([]byte(encoded), &vars); err != nil {
		if err := sigsyaml.Unmarshal([]byte(encoded), &vars); err != nil {
			return nil, fmt.Errorf("decode encoded extra_vars: %w", err)
		}
	}
	return vars, nil
}

func awxExecutionIdentityMatches(vars map[string]interface{}, wfeName, wfeNamespace, wfeUID string) bool {
	if vars == nil || vars["WFE_NAME"] != wfeName || vars["WFE_NAMESPACE"] != wfeNamespace {
		return false
	}
	if wfeUID == "" {
		return true
	}
	return vars["WFE_UID"] == wfeUID
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
	url := fmt.Sprintf("%s/api/v2/job_templates/%d/launch/", c.baseURL, templateID)

	payload := map[string]interface{}{}
	if len(extraVars) > 0 {
		payload["extra_vars"] = extraVars
	}
	if len(credentialIDs) > 0 {
		payload["credentials"] = credentialIDs
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

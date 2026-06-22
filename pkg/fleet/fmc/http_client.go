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

package fmc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// HTTPClient is a scope.ScopeChecker that calls the FMC REST API over HTTP.
// ADR-068: GW/RO use this client to resolve federated scope instead of connecting
// to Valkey directly. All failures are fail-safe (return unmanaged).
type HTTPClient struct {
	baseURL    string
	httpClient *http.Client
}

var _ scope.ScopeChecker = (*HTTPClient)(nil)

// NewHTTPClient creates an FMC HTTP client targeting the given base URL.
func NewHTTPClient(baseURL string) *HTTPClient {
	return &HTTPClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// IsManagedResource checks whether a resource is in-scope by calling FMC's
// /api/v1/scope/check endpoint. Returns false on any error (fail-safe per SC-7).
func (c *HTTPClient) IsManagedResource(ctx context.Context, r scope.ResourceIdentity) (bool, error) {
	params := url.Values{}
	params.Set("cluster", r.ClusterID)
	params.Set("group", r.Group)
	params.Set("version", r.Version)
	params.Set("kind", r.Kind)
	params.Set("namespace", r.Namespace)
	params.Set("name", r.Name)

	reqURL := c.baseURL + ScopeCheckPath + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return false, nil
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, nil
	}

	var result ScopeCheckResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, nil
	}

	return result.Managed, nil
}

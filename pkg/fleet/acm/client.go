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

// Package acm provides a scope.ScopeChecker implementation that queries
// ACM Search's GraphQL API to determine whether a resource is managed
// on a remote cluster. See ADR-068 for the contract specification.
package acm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

const graphQLPath = "/searchapi/graphql"

// Client is a scope.ScopeChecker that queries the ACM Search GraphQL API.
// ADR-068: used in non-FMC environments where ACM/OCM is the fleet control plane.
// All failures are fail-safe (return unmanaged) per SC-7.
type Client struct {
	endpoint   string
	httpClient *http.Client
}

var _ scope.ScopeChecker = (*Client)(nil)

// ClientOption configures the ACM Search client.
type ClientOption func(*Client)

// WithHTTPClient overrides the default http.Client used for ACM Search queries.
// Use this to inject a client with proper TLS configuration (e.g., via
// sharedtls.CAReloader) instead of relying on InsecureSkipVerify.
func WithHTTPClient(c *http.Client) ClientOption {
	return func(client *Client) {
		client.httpClient = c
	}
}

// NewClient creates an ACM Search client targeting the given GraphQL endpoint.
// The endpoint should be the base URL without the path (e.g. "https://search-api:4010").
// By default, the client uses a plain http.Client with no TLS verification.
// Use WithHTTPClient to provide a client configured with the cluster's CA cert.
func NewClient(endpoint string, opts ...ClientOption) *Client {
	c := &Client{
		endpoint: endpoint,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// IsManagedResource checks whether a resource exists on a managed cluster
// by querying ACM Search's GraphQL API.
// Returns false on any error (fail-safe per SC-7).
func (c *Client) IsManagedResource(ctx context.Context, r scope.ResourceIdentity) (bool, error) {
	filters := buildFilters(r)
	reqBody := graphQLRequest{
		Query: SearchQuery,
		Variables: map[string]interface{}{
			"input": []searchInput{{Filters: filters}},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return false, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+graphQLPath, bytes.NewReader(body))
	if err != nil {
		return false, nil
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return false, nil
	}

	var gqlResp graphQLResponse
	if err := json.NewDecoder(resp.Body).Decode(&gqlResp); err != nil {
		return false, nil
	}

	if len(gqlResp.Errors) > 0 {
		return false, nil
	}

	if gqlResp.Data == nil || len(gqlResp.Data.SearchResult) == 0 {
		return false, nil
	}

	return gqlResp.Data.SearchResult[0].Count > 0, nil
}

// Ping checks connectivity to the ACM Search GraphQL API by issuing the
// same SearchQuery used by IsManagedResource, filtered on kind=Namespace.
// ACM Search rejects an empty/nil filter set with a GraphQL business error
// ("query input must contain a filter or keyword") even when the backend is
// healthy and fully authenticated (confirmed against a live ACM 2.16.2
// cluster during the #1556 spike) — so Ping needs a filter that is always
// syntactically valid and satisfiable on every real ACM hub. Namespace is
// always indexed (the hub cluster itself always has namespaces), so this
// filter never depends on Kubernaut-specific fleet state.
// Unlike IsManagedResource, Ping does NOT swallow errors: the readiness
// gate (pkg/fleet/readiness.ScopeCheckerProber) needs the real
// transport/status/GraphQL error to correctly flip /readyz to NotReady
// when ACM Search is unreachable.
func (c *Client) Ping(ctx context.Context) error {
	reqBody := graphQLRequest{
		Query: SearchQuery,
		Variables: map[string]interface{}{
			"input": []searchInput{{Filters: []searchFilter{{Property: "kind", Values: []string{"Namespace"}}}}},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("build ACM Search ping request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+graphQLPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build ACM Search ping request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ACM Search unreachable: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ACM Search returned status %d", resp.StatusCode)
	}

	var gqlResp graphQLResponse
	if err := json.NewDecoder(resp.Body).Decode(&gqlResp); err != nil {
		return fmt.Errorf("decode ACM Search ping response: %w", err)
	}
	if len(gqlResp.Errors) > 0 {
		return fmt.Errorf("ACM Search returned GraphQL errors: %s", gqlResp.Errors[0].Message)
	}
	return nil
}

func buildFilters(r scope.ResourceIdentity) []searchFilter {
	filters := []searchFilter{
		{Property: "kind", Values: []string{r.Kind}},
	}
	if r.Name != "" {
		filters = append(filters, searchFilter{Property: "name", Values: []string{r.Name}})
	}
	if r.Namespace != "" {
		filters = append(filters, searchFilter{Property: "namespace", Values: []string{r.Namespace}})
	}
	if r.ClusterID != "" {
		filters = append(filters, searchFilter{Property: "cluster", Values: []string{r.ClusterID}})
	}
	if r.Group != "" {
		filters = append(filters, searchFilter{Property: "apigroup", Values: []string{r.Group}})
	}
	return filters
}

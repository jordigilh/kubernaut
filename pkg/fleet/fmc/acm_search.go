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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/pkg/shared/auth"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
)

// paginatedSearchQuery is the collector's own GraphQL document (DD-FLEET-017
// D4.2). It is deliberately NOT acm.SearchQuery: that constant is shared by
// Check and Ping, and the vendored SDL has no orderBy/offset fields, so the
// paginated shape must be a separate, provisioned document rather than a
// widening of the request-time one.
const paginatedSearchQuery = `query($input: [SearchInput]) { searchResult: search(input: $input) { count items } }`

const acmGraphQLPath = "/searchapi/graphql"

// maxACMResponseBytes bounds one Search reply before it is decoded. The logical
// page-size guard runs after decoding, so without this a hostile or broken
// Search could allocate far beyond the configured chunk.
const maxACMResponseBytes = 8 << 20

// ACMSearchFailure classifies why a chunk request failed, so the collector can
// report the right observation failure instead of collapsing everything into a
// transport error.
type ACMSearchFailure string

const (
	// ACMSearchFailureTransport is a network, timeout or TLS failure.
	ACMSearchFailureTransport ACMSearchFailure = "transport"
	// ACMSearchFailureTool is an answer the endpoint gave: an HTTP error status,
	// a GraphQL error, or an empty searchResult.
	ACMSearchFailureTool ACMSearchFailure = "tool"
	// ACMSearchFailureMalformed is an unreadable or over-budget body.
	ACMSearchFailureMalformed ACMSearchFailure = "malformed"
)

// ACMSearchError carries the classified failure while retaining the cause.
type ACMSearchError struct {
	Failure ACMSearchFailure
	Err     error
}

func (e *ACMSearchError) Error() string {
	if e.Err == nil {
		return string(e.Failure)
	}
	return fmt.Sprintf("ACM search %s: %v", e.Failure, e.Err)
}

func (e *ACMSearchError) Unwrap() error { return e.Err }

func searchFailure(failure ACMSearchFailure, err error) error {
	return &ACMSearchError{Failure: failure, Err: err}
}

// ACMSearchOptions is one chunk of an ACM Search traversal.
type ACMSearchOptions struct {
	ClusterID string
	Group     string
	Version   string
	Kind      string
	// OrderBy is always "uid asc" for the collector: the spike proved an
	// invalid or absent order is accepted silently, so the request cannot be
	// trusted to imply an order.
	OrderBy string
	Offset  int
	Limit   int
}

// ACMSearchPage is one raw Search chunk. Items stay typed as JSON because the
// projection decides what it needs; nothing here widens the payload.
type ACMSearchPage struct {
	Count int
	Items []map[string]json.RawMessage
}

// ACMSearch is the bounded chunk primitive the collector depends on. Specs
// substitute a fake so traversal guards are proven without a Search instance.
type ACMSearch interface {
	Search(ctx context.Context, options ACMSearchOptions) (ACMSearchPage, error)
}

// ACMHTTPSearch is the production ACM Search chunk reader. Credentials and
// trust reuse the existing mechanisms the rest of the fleet lane uses: the file
// token source and the CA reloader (DD-FLEET-017 D4.1a rule 6), so rotation
// needs no new machinery and no token ever enters this package. Redirects are
// refused outright: an endpoint that answers with one must not be able to
// route the request — and the bearer — anywhere else.
type ACMHTTPSearch struct {
	endpoint *url.URL
	client   *http.Client
	// reloader is retained so CA rotation has an owner: a watcher (or a caller)
	// can hand it new PEM without rebuilding the client (D4.1a rule 6).
	reloader *sharedtls.CAReloader
}

// ReloadCA installs rotated CA material on the live client. A no-op when the
// deployment uses system trust.
//
// Boundary (TEST_PLAN §13): no production watcher is wired to this method
// yet, so a CA rotation currently requires a process restart; until a watcher
// exists, handshakes fail closed after rotation (incomplete observations, the
// previous proof ages out) rather than trusting the new CA automatically.
func (s *ACMHTTPSearch) ReloadCA(pemData []byte) error {
	if s.reloader == nil {
		return nil
	}
	return s.reloader.ReloadCallback(string(pemData))
}

// NewACMHTTPSearch builds the chunk reader for one ACM Search endpoint.
func NewACMHTTPSearch(endpoint, tokenPath, caFile string, timeout time.Duration) (*ACMHTTPSearch, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("acm search endpoint is required")
	}
	if tokenPath == "" {
		return nil, fmt.Errorf("acm search token path is required")
	}
	// The request route is derived from the endpoint's path so a trailing
	// slash or an embedded query cannot alter it. A query or fragment in the
	// configured endpoint is a configuration error, not a request feature.
	parsed, err := url.Parse(endpoint)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawFragment != "" {
		return nil, fmt.Errorf("acm search endpoint must be an absolute URL without a query or fragment, got %q", endpoint)
	}
	transport := http.DefaultTransport
	var reloader *sharedtls.CAReloader
	if caFile != "" {
		loaded, err := sharedtls.NewCAReloaderFromFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("load ACM search CA: %w", err)
		}
		reloader = loaded
		transport = loaded
	}
	if timeout <= 0 {
		timeout = defaultACMRequestCap
	}
	return &ACMHTTPSearch{
		endpoint: parsed,
		reloader: reloader,
		client: &http.Client{
			Timeout:   timeout,
			Transport: auth.NewAuthTransport(auth.NewTokenSource(tokenPath), transport),
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return fmt.Errorf("acm search endpoint answered with a redirect; redirects are not supported")
			},
		},
	}, nil
}

// Search performs one GraphQL chunk request.
func (s *ACMHTTPSearch) Search(ctx context.Context, options ACMSearchOptions) (ACMSearchPage, error) {
	body, err := json.Marshal(acmGraphQLRequest{
		Query:     paginatedSearchQuery,
		Variables: acmGraphQLVariables{Input: []acmSearchInput{acmSearchInputFor(options)}},
	})
	if err != nil {
		return ACMSearchPage{}, searchFailure(ACMSearchFailureTool, fmt.Errorf("build ACM search request: %w", err))
	}
	requestURL := *s.endpoint
	// TrimRight, not TrimSuffix: an endpoint like https://search.example//
	// carries more than one trailing slash, and any count of them must yield
	// the same single-slash route (#2691 review).
	requestURL.Path = strings.TrimRight(s.endpoint.Path, "/") + acmGraphQLPath
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL.String(), bytes.NewReader(body))
	if err != nil {
		return ACMSearchPage{}, searchFailure(ACMSearchFailureTool, fmt.Errorf("build ACM search request: %w", err))
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return ACMSearchPage{}, searchFailure(ACMSearchFailureTransport, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return ACMSearchPage{}, searchFailure(ACMSearchFailureTool,
			fmt.Errorf("acm search returned status %d", response.StatusCode))
	}
	if response.ContentLength > maxACMResponseBytes {
		return ACMSearchPage{}, searchFailure(ACMSearchFailureMalformed,
			fmt.Errorf("acm search response is %d bytes, above the %d byte bound", response.ContentLength, maxACMResponseBytes))
	}
	limited := io.LimitReader(response.Body, maxACMResponseBytes+1)
	// The whole payload is read and decoded as exactly one JSON value, so a
	// reply with valid data followed by trailing garbage is malformed rather
	// than silently truncated-and-accepted.
	payload, err := io.ReadAll(limited)
	if err != nil {
		return ACMSearchPage{}, searchFailure(ACMSearchFailureMalformed,
			fmt.Errorf("read ACM search response: %w", err))
	}
	if len(payload) > maxACMResponseBytes {
		return ACMSearchPage{}, searchFailure(ACMSearchFailureMalformed,
			fmt.Errorf("acm search response is above the %d byte bound", maxACMResponseBytes))
	}
	var decoded acmGraphQLResponse
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return ACMSearchPage{}, searchFailure(ACMSearchFailureMalformed,
			fmt.Errorf("decode ACM search response: %w", err))
	}
	if len(decoded.Errors) > 0 || decoded.Data == nil || len(decoded.Data.SearchResult) != 1 {
		return ACMSearchPage{}, searchFailure(ACMSearchFailureTool, fmt.Errorf("acm search query failed"))
	}
	result := decoded.Data.SearchResult[0]
	// An explicit, nonnegative count is the only proof a traversal can be
	// checked against; a missing, null or negative count must never publish
	// an empty-looking inventory (DD-FLEET-017 D4.4, G10).
	if result.Count == nil || *result.Count < 0 {
		return ACMSearchPage{}, searchFailure(ACMSearchFailureTool,
			fmt.Errorf("acm search reply has no explicit nonnegative count"))
	}
	return ACMSearchPage{Count: *result.Count, Items: result.Items}, nil
}

// acmGraphQLRequest is the POST body sent to ACM Search's GraphQL endpoint.
type acmGraphQLRequest struct {
	Query     string              `json:"query"`
	Variables acmGraphQLVariables `json:"variables"`
}

// acmGraphQLVariables is the typed variables envelope: one fixed field, so a
// misspelled key cannot compile and defer the error to Search.
type acmGraphQLVariables struct {
	Input []acmSearchInput `json:"input"`
}

type acmGraphQLResponse struct {
	Data   *acmSearchData    `json:"data"`
	Errors []acmGraphQLError `json:"errors,omitempty"`
}

type acmSearchData struct {
	SearchResult []acmSearchResultItem `json:"searchResult"`
}

type acmSearchResultItem struct {
	// Count is a pointer so an absent or null count is distinguishable from
	// an explicit zero: only a provided, nonnegative count proves a traversal.
	Count *int                         `json:"count"`
	Items []map[string]json.RawMessage `json:"items"`
}

type acmGraphQLError struct {
	Message string `json:"message"`
}

type acmSearchFilter struct {
	Property string   `json:"property"`
	Values   []string `json:"values"`
}

type acmSearchInput struct {
	Filters []acmSearchFilter `json:"filters"`
	Limit   int               `json:"limit,omitempty"`
	Offset  int               `json:"offset,omitempty"`
	OrderBy string            `json:"orderBy,omitempty"`
}

func acmSearchInputFor(options ACMSearchOptions) acmSearchInput {
	filters := make([]acmSearchFilter, 0, 5)
	for _, filter := range []acmSearchFilter{
		{Property: "cluster", Values: []string{options.ClusterID}},
		{Property: "apigroup", Values: []string{options.Group}},
		{Property: "apiversion", Values: []string{options.Version}},
		{Property: "kind", Values: []string{options.Kind}},
	} {
		if filter.Values[0] == "" {
			continue
		}
		filters = append(filters, filter)
	}
	return acmSearchInput{
		Filters: filters,
		Limit:   options.Limit,
		Offset:  options.Offset,
		OrderBy: options.OrderBy,
	}
}

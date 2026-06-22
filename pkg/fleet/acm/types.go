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

package acm

// graphQLRequest is the POST body sent to ACM Search's /searchapi/graphql endpoint.
type graphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

// graphQLResponse is the envelope returned by ACM Search.
type graphQLResponse struct {
	Data   *searchData    `json:"data"`
	Errors []graphQLError `json:"errors,omitempty"`
}

type searchData struct {
	SearchResult []searchResultItem `json:"searchResult"`
}

type searchResultItem struct {
	Count int `json:"count"`
}

type graphQLError struct {
	Message string `json:"message"`
}

// searchFilter maps a single filter property and its values for ACM Search.
type searchFilter struct {
	Property string   `json:"property"`
	Values   []string `json:"values"`
}

// searchInput wraps filters for a single search query.
type searchInput struct {
	Filters []searchFilter `json:"filters"`
}

// SearchQuery is the GraphQL query sent to ACM Search. Exported so the
// contract test (UT-ACM-054-009) can validate it against the vendored SDL schema.
const SearchQuery = `query($input: [SearchInput]) { searchResult: search(input: $input) { count } }`

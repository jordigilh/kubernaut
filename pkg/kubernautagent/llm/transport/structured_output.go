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

package transport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// StructuredOutputTransport implements http.RoundTripper to inject Anthropic's
// output_config.format into Messages API requests. This enables structured JSON
// output (constrained decoding) without forking the LangChainGo vendor.
//
// The transport only modifies requests that look like Anthropic Messages API
// calls (identified by the presence of a "messages" key in the JSON body).
// Non-matching requests pass through unmodified. If the request already
// contains an "output_config" key, the transport does not overwrite it.
//
// Thread Safety: Safe for concurrent use — clones the request before mutation.
//
// Authority: BR-TESTING-001
type StructuredOutputTransport struct {
	base   http.RoundTripper
	schema json.RawMessage
}

// NewStructuredOutputTransport creates a transport that injects
// output_config.format into Anthropic Messages API requests.
// The schema parameter is a JSON Schema object that defines the expected
// response format (e.g., the InvestigationResult schema).
// If base is nil, http.DefaultTransport is used.
func NewStructuredOutputTransport(schema json.RawMessage, base http.RoundTripper) *StructuredOutputTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &StructuredOutputTransport{
		base:   base,
		schema: schema,
	}
}

// RoundTrip intercepts outgoing requests, and if the body looks like an
// Anthropic Messages API payload (has a "messages" key), injects the
// output_config.format field for structured JSON output.
func (t *StructuredOutputTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body == nil || req.Method != http.MethodPost {
		return t.base.RoundTrip(req)
	}

	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("structured output transport: read body: %w", err)
	}
	_ = req.Body.Close()

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		reqClone := req.Clone(req.Context())
		reqClone.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		reqClone.ContentLength = int64(len(bodyBytes))
		return t.base.RoundTrip(reqClone)
	}

	_, hasMessages := payload["messages"]
	_, hasOutputConfig := payload["output_config"]

	if !hasMessages || hasOutputConfig {
		reqClone := req.Clone(req.Context())
		reqClone.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		reqClone.ContentLength = int64(len(bodyBytes))
		return t.base.RoundTrip(reqClone)
	}

	outputConfig := map[string]interface{}{
		"format": map[string]interface{}{
			"type":   "json_schema",
			"schema": json.RawMessage(t.schema),
		},
	}

	configBytes, err := json.Marshal(outputConfig)
	if err != nil {
		return nil, fmt.Errorf("structured output transport: marshal output_config: %w", err)
	}
	payload["output_config"] = configBytes

	newBody, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("structured output transport: marshal payload: %w", err)
	}

	reqClone := req.Clone(req.Context())
	reqClone.Body = io.NopCloser(bytes.NewReader(newBody))
	reqClone.ContentLength = int64(len(newBody))

	return t.base.RoundTrip(reqClone)
}

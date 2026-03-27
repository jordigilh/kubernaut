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

package llm

import (
	"log/slog"
	"net/http"

	"github.com/jordigilh/kubernaut/pkg/kapi/config"
	"github.com/jordigilh/kubernaut/pkg/kapi/llm/transport"
)

// NewLLMClient creates an http.Client with custom authentication headers
// injected via the AuthHeadersTransport. If headers is nil or empty, the
// transport is still installed but operates as a no-op pass-through.
//
// Authority: Issue #417 — Support custom authentication headers for LLM proxy endpoints
func NewLLMClient(baseURL string, headers []config.HeaderDefinition) (*http.Client, error) {
	rt := transport.NewAuthHeadersTransport(headers, http.DefaultTransport)
	return &http.Client{Transport: rt}, nil
}

// NewLLMClientWithLogger creates an http.Client with custom authentication headers
// and structured logging. Header injection events are logged with sensitive values
// redacted per DD-HAPI-019-003 (G4: Credential Scrubbing).
func NewLLMClientWithLogger(baseURL string, headers []config.HeaderDefinition, logger *slog.Logger) (*http.Client, error) {
	rt := transport.NewAuthHeadersTransportWithLogger(headers, http.DefaultTransport, logger)
	return &http.Client{Transport: rt}, nil
}

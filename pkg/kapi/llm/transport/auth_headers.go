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
	"fmt"
	"log/slog"
	"net/http"

	"github.com/jordigilh/kubernaut/pkg/kapi/config"
)

// AuthHeadersTransport implements http.RoundTripper to inject custom
// authentication headers into outbound LLM API requests.
//
// Thread Safety: Safe for concurrent use — clones the request before mutation.
//
// Authority: Issue #417, DD-HAPI-019-003 (G4: Credential Scrubbing)
type AuthHeadersTransport struct {
	base    http.RoundTripper
	headers []config.HeaderDefinition
	logger  *slog.Logger
}

// NewAuthHeadersTransport wraps a base transport, injecting the given
// headers into every outbound request. If base is nil, http.DefaultTransport is used.
func NewAuthHeadersTransport(headers []config.HeaderDefinition, base http.RoundTripper) *AuthHeadersTransport {
	return NewAuthHeadersTransportWithLogger(headers, base, nil)
}

// NewAuthHeadersTransportWithLogger wraps a base transport with structured logging.
// Header values are redacted in log output per DD-HAPI-019-003 (G4).
func NewAuthHeadersTransportWithLogger(headers []config.HeaderDefinition, base http.RoundTripper, logger *slog.Logger) *AuthHeadersTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &AuthHeadersTransport{
		base:    base,
		headers: headers,
		logger:  logger,
	}
}

// RoundTrip clones the request, resolves header values from their configured sources,
// injects them, and delegates to the inner transport.
//
// Per http.RoundTripper contract: the original *http.Request is NOT modified.
func (t *AuthHeadersTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	reqClone := req.Clone(req.Context())

	for _, h := range t.headers {
		val, err := resolveHeader(h)
		if err != nil {
			return nil, fmt.Errorf("auth header transport: header %q: %w", h.Name, err)
		}
		reqClone.Header.Set(h.Name, val)

		if t.logger != nil {
			t.logger.Debug("injected auth header",
				"header", h.Name,
				"value", RedactHeaderValue(val, IsSensitiveSource(h)),
			)
		}
	}

	return t.base.RoundTrip(reqClone)
}

func resolveHeader(def config.HeaderDefinition) (string, error) {
	switch {
	case def.Value != "":
		return ResolveValue(def.Value), nil
	case def.SecretKeyRef != "":
		return ResolveSecretKeyRef(def.SecretKeyRef)
	case def.FilePath != "":
		return ResolveFilePath(def.FilePath)
	default:
		return "", fmt.Errorf("no value source configured")
	}
}

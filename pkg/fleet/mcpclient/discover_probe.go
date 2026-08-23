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

package mcpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/go-logr/logr"
)

// methodServerDiscover is go-sdk's SEP-2575 stateless discovery RPC method
// name (the SDK's own `methodDiscover` constant is unexported; this is the
// literal wire value it sends).
const methodServerDiscover = "server/discover"

// discoverProbeRoundTripper bounds the go-sdk v1.7.0+ "server/discover"
// probe (SEP-2575) with its own short sub-timeout, independent of the
// caller's request context deadline.
//
// go-sdk's Client.Connect falls back to the legacy "initialize" handshake
// on ANY error from server/discover -- see mcp/client.go: "Per the spec,
// fall back to the legacy initialize handshake on any non-modern error
// from server/discover" -- and that fallback reuses the *original* Connect
// context, not one derived from the discover attempt. Both steps,
// nonetheless, share one context by default, so a gateway that hangs
// (rather than erroring) on server/discover silently consumes the entire
// shared ConnectTimeout budget before the fallback ever gets a chance to
// run (issue #2262). Wrapping just this one HTTP request with its own
// deadline lets a hung probe fail fast, leaving the rest of that budget for
// the fallback.
type discoverProbeRoundTripper struct {
	next    http.RoundTripper
	timeout time.Duration
	logger  logr.Logger
}

// RoundTrip peeks the outgoing JSON-RPC request's "method" field. Request
// bodies here are small, fully-buffered control messages (go-sdk's
// streamable client transport always sends a bytes.Reader, never a
// streaming body), so reading and restoring the body is cheap and safe.
// Only "server/discover" requests get the sub-timeout; every other method
// (initialize, notifications/*, tools/*, etc.) passes through unmodified.
func (t *discoverProbeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	next := t.next
	if next == nil {
		next = http.DefaultTransport
	}

	if req.Body == nil || req.Method != http.MethodPost {
		return next.RoundTrip(req)
	}

	body, readErr := io.ReadAll(req.Body)
	_ = req.Body.Close()
	req.Body = io.NopCloser(bytes.NewReader(body))
	if readErr != nil {
		return next.RoundTrip(req)
	}

	var probe struct {
		Method string `json:"method"`
	}
	if err := json.Unmarshal(body, &probe); err != nil || probe.Method != methodServerDiscover {
		return next.RoundTrip(req)
	}

	subCtx, cancel := context.WithTimeout(req.Context(), t.timeout)
	defer cancel()

	start := time.Now()
	resp, err := next.RoundTrip(req.WithContext(subCtx))
	if err != nil && subCtx.Err() != nil {
		t.logger.Info("mcpclient: server/discover probe did not respond within its sub-timeout, falling back to legacy initialize handshake",
			"method", methodServerDiscover,
			"endpoint", req.URL.String(),
			"discoverProbeTimeout", t.timeout,
			"elapsed", time.Since(start),
		)
	}
	return resp, err
}

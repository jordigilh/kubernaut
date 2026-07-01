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
	"context"
	"net/http"
	"time"
)

// Option configures an MCPResourceClient.
type Option func(*clientConfig)

type clientConfig struct {
	httpClient *http.Client
	timeout    time.Duration
	clusterID  string
	toolPrefix string
	reconnect  func(context.Context) error
}

// WithClusterID binds the client to a specific remote cluster. The cluster ID
// is injected as a tool-name prefix on every MCP call (e.g. "{clusterID}__get_resource"),
// keeping the per-call API symmetric with K8s client.Reader.
func WithClusterID(id string) Option {
	return func(cfg *clientConfig) {
		cfg.clusterID = id
	}
}

// WithToolPrefix sets the gateway-specific tool prefix used when constructing
// MCP tool call names. When set, tool names are resolved as "{prefix}{tool}"
// via ClusterToolWithPrefix instead of the default EAIGW "{clusterID}__{tool}"
// convention. This enables Kuadrant and other gateways that use different
// prefix schemes.
func WithToolPrefix(prefix string) Option {
	return func(cfg *clientConfig) {
		cfg.toolPrefix = prefix
	}
}

// WithHTTPClient sets a custom HTTP client for the MCP transport.
// Use this to inject OAuth2 auth transports or custom TLS configurations.
func WithHTTPClient(c *http.Client) Option {
	return func(cfg *clientConfig) {
		cfg.httpClient = c
	}
}

// WithReconnect binds a reconnection callback to a session-provider Client
// (created via NewFromSessionProvider). When a tool call fails with a
// retryable session error (connection closed, session missing, etc.), the
// Client invokes reconnect once and retries the call against the refreshed
// session returned by the SessionProvider.
//
// Without this option, a session-provider Client that observes a dead
// session has no way to trigger recovery: SessionProvider() only reads the
// currently stored session, it does not repair it. ResilientClient.Reconnect
// is the intended callback for per-cluster readers built from
// ResilientClient.SessionProvider().
func WithReconnect(reconnect func(context.Context) error) Option {
	return func(cfg *clientConfig) {
		cfg.reconnect = reconnect
	}
}

// WithTimeout sets the HTTP client timeout in seconds.
// Creates a new HTTP client with the given timeout if no custom client is set.
func WithTimeout(seconds int) Option {
	return func(cfg *clientConfig) {
		cfg.timeout = time.Duration(seconds) * time.Second
		if cfg.httpClient == nil {
			cfg.httpClient = &http.Client{Timeout: cfg.timeout}
		} else {
			cfg.httpClient.Timeout = cfg.timeout
		}
	}
}

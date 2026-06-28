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

package alertmanager

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const defaultSizeLimit = 30000
const defaultTimeout = 30 * time.Second

// ClientConfig holds Alertmanager client settings.
type ClientConfig struct {
	URL       string            `yaml:"url"`
	Headers   map[string]string `yaml:"headers"`
	Timeout   time.Duration     `yaml:"timeout"`
	SizeLimit int               `yaml:"sizeLimit"`
	TLSCaFile string            `yaml:"tlsCaFile"`

	// Transport overrides the http.Client's RoundTripper. When set, it is used
	// as the underlying transport for all Alertmanager API requests. Enables
	// SA bearer token injection and custom TLS trust. When nil, http.DefaultTransport is used.
	Transport http.RoundTripper `yaml:"-"`
}

// Client wraps net/http for Alertmanager API access.
type Client struct {
	config     ClientConfig
	httpClient *http.Client
}

// NewClient creates an Alertmanager client from config.
func NewClient(cfg ClientConfig) (*Client, error) {
	if cfg.SizeLimit <= 0 {
		cfg.SizeLimit = defaultSizeLimit
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	return &Client{
		config: cfg,
		httpClient: &http.Client{
			Timeout:   cfg.Timeout,
			Transport: cfg.Transport,
		},
	}, nil
}

// Config returns the client's configuration.
func (c *Client) Config() ClientConfig {
	return c.config
}

// DoGet performs an HTTP GET to the given Alertmanager API path with query params.
func (c *Client) DoGet(ctx context.Context, apiPath string, params map[string][]string) (string, error) {
	u, err := url.Parse(c.config.URL + apiPath)
	if err != nil {
		return "", fmt.Errorf("building URL: %w", err)
	}
	if params != nil {
		u.RawQuery = url.Values(params).Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	for k, v := range c.config.Headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("alertmanager request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(c.config.SizeLimit)+1))
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := string(body)
		if len(snippet) > 256 {
			snippet = snippet[:256]
		}
		return "", fmt.Errorf("alertmanager returned HTTP %d: %s", resp.StatusCode, snippet)
	}

	result := string(body)
	return truncateWithHint(result, c.config.SizeLimit), nil
}

func truncateWithHint(text string, sizeLimit int) string {
	if len(text) <= sizeLimit {
		return text
	}
	return text[:sizeLimit] + "\n... [TRUNCATED] Response exceeded limit. Use more specific filters to narrow results."
}

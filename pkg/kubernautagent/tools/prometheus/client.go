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

package prometheus

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
const defaultMetadataLimit = 100
const defaultMetadataTimeWindowHrs = 1

// ClientConfig holds Prometheus client settings.
type ClientConfig struct {
	URL                   string            `yaml:"url"`
	Headers               map[string]string `yaml:"headers"`
	Timeout               time.Duration     `yaml:"timeout"`
	SizeLimit             int               `yaml:"size_limit"`
	MetadataLimit         int               `yaml:"metadata_limit"`
	MetadataTimeWindowHrs int               `yaml:"metadata_time_window_hrs"`
}

// DefaultClientConfig returns production defaults.
func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		Timeout:               defaultTimeout,
		SizeLimit:             defaultSizeLimit,
		MetadataLimit:         defaultMetadataLimit,
		MetadataTimeWindowHrs: defaultMetadataTimeWindowHrs,
	}
}

// Client wraps net/http for Prometheus API access.
type Client struct {
	config     ClientConfig
	httpClient *http.Client
}

// NewClient creates a Prometheus client from config.
func NewClient(cfg ClientConfig) (*Client, error) {
	if cfg.SizeLimit <= 0 {
		cfg.SizeLimit = defaultSizeLimit
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.MetadataLimit <= 0 {
		cfg.MetadataLimit = defaultMetadataLimit
	}
	if cfg.MetadataTimeWindowHrs <= 0 {
		cfg.MetadataTimeWindowHrs = defaultMetadataTimeWindowHrs
	}
	return &Client{
		config: cfg,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}, nil
}

// Config returns the client's configuration.
func (c *Client) Config() ClientConfig {
	return c.config
}

// doGet performs an HTTP GET to the given Prometheus API path with query params.
func (c *Client) doGet(ctx context.Context, apiPath string, params url.Values) (string, error) {
	u, err := url.Parse(c.config.URL + apiPath)
	if err != nil {
		return "", fmt.Errorf("building URL: %w", err)
	}
	if params != nil {
		u.RawQuery = params.Encode()
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
		return "", fmt.Errorf("prometheus request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	result := string(body)
	return TruncateWithHint(result, c.config.SizeLimit), nil
}

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
	"fmt"
	"net/http"
	"os"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// OAuth2Config holds the parameters for client credentials OAuth2 authentication
// against the MCP Gateway's token endpoint (e.g., DEX, Keycloak, or Authorino).
type OAuth2Config struct {
	TokenURL     string
	ClientID     string
	ClientSecret string
	Scopes       []string
}

// NewOAuth2Transport creates an http.RoundTripper that automatically acquires,
// caches, and refreshes OAuth2 tokens using the client credentials grant.
// The token is injected as Authorization: Bearer on every outbound request.
//
// If base is nil, http.DefaultTransport is used.
func NewOAuth2Transport(cfg OAuth2Config, base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	ccConfig := &clientcredentials.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		TokenURL:     cfg.TokenURL,
		Scopes:       cfg.Scopes,
	}
	return &oauth2.Transport{
		Source: ccConfig.TokenSource(context.Background()),
		Base:   base,
	}
}

// LoadOAuth2ConfigFromFiles reads OAuth2 credentials from file-mounted Secret paths.
// This is the recommended pattern for Kubernetes deployments where credentials
// are mounted as files (not environment variables) for rotation safety.
//
// Expected file layout (e.g., from a Secret volume mount):
//
//	/etc/fleet/oauth2/token-url
//	/etc/fleet/oauth2/client-id
//	/etc/fleet/oauth2/client-secret
func LoadOAuth2ConfigFromFiles(tokenURLPath, clientIDPath, clientSecretPath string) (OAuth2Config, error) {
	tokenURL, err := readTrimmedFile(tokenURLPath)
	if err != nil {
		return OAuth2Config{}, fmt.Errorf("read token URL from %s: %w", tokenURLPath, err)
	}
	clientID, err := readTrimmedFile(clientIDPath)
	if err != nil {
		return OAuth2Config{}, fmt.Errorf("read client ID from %s: %w", clientIDPath, err)
	}
	clientSecret, err := readTrimmedFile(clientSecretPath)
	if err != nil {
		return OAuth2Config{}, fmt.Errorf("read client secret from %s: %w", clientSecretPath, err)
	}
	return OAuth2Config{
		TokenURL:     tokenURL,
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}, nil
}

// DefaultFleetScopes returns the provided scopes if non-empty, or the
// minimal default scopes ["openid", "groups"] required for service identity
// tokens. The "groups" scope is needed for DEX to include role-bearing claims
// (mcp-read, mcp-write) used by the gateway's CEL authorization rules.
func DefaultFleetScopes(scopes []string) []string {
	if len(scopes) > 0 {
		return scopes
	}
	return []string{"openid", "groups"}
}

func readTrimmedFile(path string) (string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // paths are from trusted config, not user input
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// WithOAuth2Transport is an Option that configures the MCPResourceClient with
// OAuth2 client credentials authentication for the MCP Gateway.
func WithOAuth2Transport(cfg OAuth2Config) Option {
	return func(c *clientConfig) {
		base := http.DefaultTransport
		if c.httpClient != nil && c.httpClient.Transport != nil {
			base = c.httpClient.Transport
		}
		transport := NewOAuth2Transport(cfg, base)
		if c.httpClient == nil {
			c.httpClient = &http.Client{Transport: transport}
		} else {
			c.httpClient.Transport = transport
		}
	}
}

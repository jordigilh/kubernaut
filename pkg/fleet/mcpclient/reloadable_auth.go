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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"
)

// ReloadableOAuth2Transport is an http.RoundTripper that supports live reloading
// of OAuth2 credentials when the underlying Secret files change. It uses
// hotreload.FileWatcher to monitor the credential directory and atomically swaps
// the oauth2.TokenSource when credentials are rotated.
//
// This addresses concern #1 from Phase 6: "OAuth2 secret rotation not live-reloaded".
type ReloadableOAuth2Transport struct {
	mu          sync.RWMutex
	tokenSource oauth2.TokenSource
	base        http.RoundTripper
	logger      logr.Logger

	watchers []*hotreload.FileWatcher

	tokenURL         string
	clientIDPath     string
	clientSecretPath string
	scopes           []string
	tokenTimeout     time.Duration
	tlsCaFile        string
}

// ReloadableOAuth2Config holds paths for the file-watched OAuth2 credentials.
type ReloadableOAuth2Config struct {
	TokenURL         string
	ClientIDPath     string
	ClientSecretPath string
	Scopes           []string
	TokenTimeout     time.Duration
	TlsCaFile        string
}

// NewReloadableOAuth2Transport creates a transport that watches credential files
// and rebuilds the TokenSource on change. The initial credentials must be loadable
// or an error is returned.
func NewReloadableOAuth2Transport(cfg ReloadableOAuth2Config, base http.RoundTripper, logger logr.Logger) (*ReloadableOAuth2Transport, error) {
	if base == nil {
		base = http.DefaultTransport
	}
	if cfg.TokenTimeout == 0 {
		cfg.TokenTimeout = 10 * time.Second
	}

	t := &ReloadableOAuth2Transport{
		base:             base,
		logger:           logger.WithName("reloadable-oauth2"),
		tokenURL:         cfg.TokenURL,
		clientIDPath:     cfg.ClientIDPath,
		clientSecretPath: cfg.ClientSecretPath,
		scopes:           cfg.Scopes,
		tokenTimeout:     cfg.TokenTimeout,
		tlsCaFile:        cfg.TlsCaFile,
	}

	if err := t.rebuildTokenSource(); err != nil {
		return nil, err
	}

	return t, nil
}

// StartWatching begins file watchers on the credential paths.
// Must be called after construction. Call Stop() to clean up watchers.
func (t *ReloadableOAuth2Transport) StartWatching(ctx context.Context) error {
	paths := []string{t.clientIDPath, t.clientSecretPath}
	for _, path := range paths {
		w, err := hotreload.NewFileWatcher(path, func(_ string) error {
			return t.rebuildTokenSource()
		}, t.logger)
		if err != nil {
			t.Stop()
			return err
		}
		if err := w.Start(ctx); err != nil {
			t.Stop()
			return err
		}
		t.watchers = append(t.watchers, w)
	}
	t.logger.Info("OAuth2 credential file watchers started",
		"clientID", t.clientIDPath,
		"clientSecret", t.clientSecretPath)
	return nil
}

// Stop terminates all file watchers.
func (t *ReloadableOAuth2Transport) Stop() {
	for _, w := range t.watchers {
		w.Stop()
	}
	t.watchers = nil
}

// RoundTrip implements http.RoundTripper, injecting the current OAuth2 token.
func (t *ReloadableOAuth2Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.RLock()
	ts := t.tokenSource
	t.mu.RUnlock()

	token, err := ts.Token()
	if err != nil {
		return nil, err
	}

	reqClone := req.Clone(req.Context())
	token.SetAuthHeader(reqClone)
	return t.base.RoundTrip(reqClone)
}

// InvalidateToken forces a new token acquisition on the next request.
// Used by the retry wrapper after receiving a 401 response.
func (t *ReloadableOAuth2Transport) InvalidateToken() {
	if err := t.rebuildTokenSource(); err != nil {
		t.logger.Error(err, "Failed to rebuild token source during invalidation")
	}
}

func (t *ReloadableOAuth2Transport) rebuildTokenSource() error {
	clientID, err := readTrimmedFile(t.clientIDPath)
	if err != nil {
		return err
	}
	clientSecret, err := readTrimmedFile(t.clientSecretPath)
	if err != nil {
		return err
	}

	ccConfig := &clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     t.tokenURL,
		Scopes:       t.scopes,
	}

	ctx := context.Background()
	httpClient := &http.Client{Timeout: t.tokenTimeout}
	if t.tlsCaFile != "" {
		caPEM, caErr := os.ReadFile(t.tlsCaFile)
		if caErr != nil {
			return fmt.Errorf("failed to read TLS CA file %s: %w", t.tlsCaFile, caErr)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return fmt.Errorf("no valid PEM certificates found in %s", t.tlsCaFile)
		}
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    pool,
				MinVersion: tls.VersionTLS12,
			},
		}
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, httpClient)

	t.mu.Lock()
	t.tokenSource = ccConfig.TokenSource(ctx)
	t.mu.Unlock()

	t.logger.Info("OAuth2 TokenSource rebuilt",
		"clientID", clientID,
		"tokenURL", t.tokenURL)
	return nil
}

// WithReloadableOAuth2Transport is an Option that configures the MCPResourceClient with
// a hot-reloadable OAuth2 transport. The transport watches credential files for changes
// and atomically swaps the token source when secrets are rotated.
func WithReloadableOAuth2Transport(cfg ReloadableOAuth2Config, logger logr.Logger) Option {
	return func(c *clientConfig) {
		base := http.DefaultTransport
		if c.httpClient != nil && c.httpClient.Transport != nil {
			base = c.httpClient.Transport
		}
		transport, err := NewReloadableOAuth2Transport(cfg, base, logger)
		if err != nil {
			logger.Error(err, "Failed to create reloadable OAuth2 transport, falling back to static")
			return
		}
		if c.httpClient == nil {
			c.httpClient = &http.Client{Transport: transport}
		} else {
			c.httpClient.Transport = transport
		}
	}
}

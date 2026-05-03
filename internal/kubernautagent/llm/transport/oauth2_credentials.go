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
	"context"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
)

// NewOAuth2ClientCredentialsTransport creates an http.RoundTripper that
// automatically acquires, caches, and refreshes OAuth2 tokens using the
// client credentials grant (RFC 6749 s4.4). The token is injected as
// an Authorization: Bearer header into every outbound request.
//
// Token lifecycle is fully managed by golang.org/x/oauth2:
//   - Acquisition via client_credentials grant against cfg.TokenURL
//   - In-memory caching via oauth2.ReuseTokenSource
//   - Automatic refresh before expiry
//   - Thread-safe concurrent access
//
// If base is nil, http.DefaultTransport is used.
//
// Authority: Issue #417 — Enterprise LLM gateway OAuth2 authentication
func NewOAuth2ClientCredentialsTransport(cfg kaconfig.OAuth2Config, base http.RoundTripper) http.RoundTripper {
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

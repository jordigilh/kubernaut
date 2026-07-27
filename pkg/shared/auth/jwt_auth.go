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

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/lestrrat-go/httprc/v3"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jws"
	"github.com/lestrrat-go/jwx/v3/jwt"

	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
)

// ErrIssuerNotFound indicates the JWT's issuer claim does not match any
// configured provider. Distinct from ErrTokenInvalid: the token may be
// well-formed but KA has no JWKS endpoint registered for this issuer.
var ErrIssuerNotFound = errors.New("issuer not found in configured providers")

// ErrTokenMalformed indicates the token could not be parsed as a JWT at all.
// Distinct from ErrTokenInvalid: the token is not a JWT (e.g., opaque or garbage).
// CompositeAuthenticator uses this to fall back to K8sAuthenticator.
var ErrTokenMalformed = errors.New("token is not a valid JWT")

// JWTProviderEntry defines a trusted JWT issuer for Pattern B authentication.
// Maps 1:1 to config.JWTProviderConfig but lives in the auth package to avoid
// import cycles.
type JWTProviderEntry struct {
	Issuer        string
	JWKSURL       string
	Audience      string
	UsernameClaim string
	GroupsClaim   string
	// TLSCAFile is an optional path to a PEM-encoded CA bundle used to verify
	// the JWKSURL's TLS certificate when it is signed by a private/internal
	// CA (e.g. an in-cluster inter-service CA) rather than a public CA.
	// When empty, the process default HTTP client (system trust store) is used.
	TLSCAFile string
}

type jwtProvider struct {
	entry    JWTProviderEntry
	jwksCache *jwk.Cache
}

// JWTAuthenticator validates JWTs against configured OIDC providers using JWKS.
// Implements the Authenticator interface for Pattern B (DD-AUTH-MCP-001 v2.0).
//
// Multi-issuer support: tokens are routed to the correct JWKS endpoint by
// extracting the unverified `iss` claim, then verified against that provider's keys.
//
// Call Close() to stop background JWKS refresh goroutines when the authenticator
// is no longer needed (e.g., during graceful shutdown).
type JWTAuthenticator struct {
	providers map[string]*jwtProvider
	cancel    context.CancelFunc
	logger    logr.Logger
}

// jwksPreWarmTimeout is the maximum time to wait for the initial JWKS fetch
// during construction. Prevents indefinite blocking if the OIDC provider is
// slow to respond.
const jwksPreWarmTimeout = 15 * time.Second

// NewJWTAuthenticator creates a JWTAuthenticator from a list of trusted providers.
// Each provider's JWKS endpoint is configured for automatic background refresh
// via jwk.Cache. A synchronous pre-warm fetch is performed for each provider
// to ensure keys are available before the first token validation request.
// Returns an error if any provider's cache cannot be initialized or pre-warmed.
func NewJWTAuthenticator(entries []JWTProviderEntry, logger logr.Logger) (*JWTAuthenticator, error) {
	ctx, cancel := context.WithCancel(context.Background())
	providers := make(map[string]*jwtProvider, len(entries))
	for i := range entries {
		e := entries[i]
		c, err := jwk.NewCache(ctx, httprc.NewClient())
		if err != nil {
			cancel()
			return nil, fmt.Errorf("creating JWKS cache for provider %q: %w", e.Issuer, err)
		}

		var registerOpts []jwk.RegisterOption
		if e.TLSCAFile != "" {
			transport, tlsErr := sharedtls.NewTLSTransport(e.TLSCAFile)
			if tlsErr != nil {
				cancel()
				return nil, fmt.Errorf("loading TLS CA for provider %q (%s): %w", e.Issuer, e.TLSCAFile, tlsErr)
			}
			registerOpts = append(registerOpts, jwk.WithHTTPClient(&http.Client{
				Transport: transport,
				Timeout:   jwksPreWarmTimeout,
			}))
		}

		// Bound the registration's initial fetch-and-wait with the same
		// timeout used for the warm-up Lookup below. httprc's Controller.Add
		// (called by Register, WithWaitReady defaults to true) blocks on
		// Resource.Ready(ctx) until the first successful fetch -- and that
		// wait does NOT time out unless the passed context has a deadline
		// (see lestrrat-go/httprc Controller.Add godoc). Without this bound,
		// a permanently unreachable or TLS-untrusted JWKS endpoint hangs
		// kubernautagent's startup forever instead of failing fast, since
		// every retry attempt fails identically and the resource never
		// becomes ready.
		warmCtx, warmCancel := context.WithTimeout(ctx, jwksPreWarmTimeout)
		err = c.Register(warmCtx, e.JWKSURL, registerOpts...)
		warmCancel()
		if err != nil {
			cancel()
			return nil, fmt.Errorf("registering JWKS URL %q for provider %q: %w", e.JWKSURL, e.Issuer, err)
		}

		lookupCtx, lookupCancel := context.WithTimeout(ctx, jwksPreWarmTimeout)
		_, err = c.Lookup(lookupCtx, e.JWKSURL)
		lookupCancel()
		if err != nil {
			cancel()
			return nil, fmt.Errorf("JWKS pre-warm failed for provider %q (%s): %w", e.Issuer, e.JWKSURL, err)
		}

		providers[e.Issuer] = &jwtProvider{
			entry:     e,
			jwksCache: c,
		}
	}
	return &JWTAuthenticator{providers: providers, cancel: cancel, logger: logger}, nil
}

// Close stops all background JWKS refresh goroutines. Safe to call multiple times.
func (a *JWTAuthenticator) Close() {
	if a.cancel != nil {
		a.cancel()
	}
}

// ValidateToken implements Authenticator.ValidateToken by delegating to
// ValidateTokenFull and returning just the username.
func (a *JWTAuthenticator) ValidateToken(ctx context.Context, token string) (string, error) {
	info, err := a.ValidateTokenFull(ctx, token)
	if err != nil {
		return "", err
	}
	return info.Username, nil
}

// ValidateTokenFull implements Authenticator.ValidateTokenFull.
// It parses the JWT without verification to extract the issuer, routes to the
// correct provider's JWKS, verifies signature + standard claims, then extracts
// username and groups from the configured claim paths.
func (a *JWTAuthenticator) ValidateTokenFull(ctx context.Context, rawToken string) (UserInfo, error) {
	issuer, err := extractUnverifiedIssuer(rawToken)
	if err != nil {
		return UserInfo{}, fmt.Errorf("%w: %w", ErrTokenMalformed, err)
	}

	provider, ok := a.providers[issuer]
	if !ok {
		return UserInfo{}, fmt.Errorf("%w: %s", ErrIssuerNotFound, issuer)
	}

	keyset, err := provider.jwksCache.Lookup(ctx, provider.entry.JWKSURL)
	if err != nil {
		return UserInfo{}, fmt.Errorf("fetching JWKS for issuer %q: %w", issuer, err)
	}

	verifiedToken, err := jwt.Parse(
		[]byte(rawToken),
		jwt.WithKeySet(keyset, jws.WithInferAlgorithmFromKey(true)),
		jwt.WithIssuer(provider.entry.Issuer),
		jwt.WithAudience(provider.entry.Audience),
		jwt.WithValidate(true),
	)
	if err != nil {
		return UserInfo{}, fmt.Errorf("%w: %w", ErrTokenInvalid, err)
	}

	claims, err := tokenToClaimsMap(verifiedToken)
	if err != nil {
		return UserInfo{}, fmt.Errorf("extracting claims: %w", err)
	}

	username, err := ExtractStringClaim(claims, provider.entry.UsernameClaim)
	if err != nil {
		return UserInfo{}, fmt.Errorf("extracting username from claim %q: %w", provider.entry.UsernameClaim, err)
	}

	groups, err := ExtractGroupsClaim(claims, provider.entry.GroupsClaim)
	if err != nil {
		a.logger.V(1).Info("groups claim extraction failed; user will have no group memberships",
			"username", username, "groupsClaim", provider.entry.GroupsClaim, "error", err)
		groups = []string{}
	}

	return UserInfo{
		Username:     username,
		Groups:       groups,
		ProviderType: "jwt:" + issuer,
	}, nil
}

// extractUnverifiedIssuer parses the JWT payload without signature verification
// to extract the `iss` claim for provider routing. This is safe because the
// actual signature verification happens after routing to the correct JWKS.
func extractUnverifiedIssuer(rawToken string) (string, error) {
	token, err := jwt.Parse([]byte(rawToken), jwt.WithVerify(false), jwt.WithValidate(false))
	if err != nil {
		return "", fmt.Errorf("parsing JWT header: %w", err)
	}
	iss, ok := token.Issuer()
	if !ok || iss == "" {
		return "", fmt.Errorf("JWT missing iss claim")
	}
	return iss, nil
}

// tokenToClaimsMap serializes a jwt.Token to a generic map for claim extraction.
// This allows the claims.go extraction functions to navigate nested structures
// (e.g., realm_access.roles) uniformly.
func tokenToClaimsMap(token jwt.Token) (map[string]interface{}, error) {
	data, err := json.Marshal(token)
	if err != nil {
		return nil, err
	}
	var claims map[string]interface{}
	if err := json.Unmarshal(data, &claims); err != nil {
		return nil, err
	}
	return claims, nil
}

// Compile-time interface compliance check.
var _ Authenticator = (*JWTAuthenticator)(nil)

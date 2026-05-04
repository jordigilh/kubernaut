package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-jose/go-jose/v4"
	josejwt "github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/cel-go/cel"

	"github.com/jordigilh/kubernaut-apifrontend/internal/security"
)

// ErrUnknownIssuer is returned when a token's issuer doesn't match any configured provider.
var ErrUnknownIssuer = errors.New("unknown token issuer")

// ErrTokenExpired is returned when a token's exp claim is in the past.
var ErrTokenExpired = errors.New("token expired")

// ErrInvalidAudience is returned when a token's audience doesn't match configured audiences.
var ErrInvalidAudience = errors.New("invalid token audience")

// ErrMalformedToken is returned when a token cannot be parsed.
var ErrMalformedToken = errors.New("malformed token")

// ErrCELValidation is returned when a CEL validation rule rejects the user.
var ErrCELValidation = errors.New("user validation rule failed")

// ErrCircuitOpen is returned when the JWKS circuit breaker is open and no cached keys exist.
var ErrCircuitOpen = errors.New("JWKS circuit breaker open: authentication unavailable")

type compiledRule struct {
	program cel.Program
	message string
}

type providerRuntime struct {
	config   ProviderConfig
	celRules []compiledRule
}

// JWTValidator validates JWT tokens against configured OIDC providers.
// It implements issuer-based deterministic routing (KEP-3331 pattern),
// CEL-based claim validation, and JWKS caching with circuit breaker.
//
// Known limitation (v1.5): no token replay protection. Tokens are validated
// for signature, expiry, audience, and CEL rules, but the "jti" (JWT ID) claim
// is not tracked. A stolen token can be replayed until it expires. This is
// acceptable for the current threat model where tokens are short-lived and
// transmitted over TLS. Future versions may add jti-based replay detection
// backed by a distributed cache.
type JWTValidator struct {
	providers  map[string]*providerRuntime
	cache      *JWKSCache
	reviewer   *TokenReviewer
	k8sEnabled bool
	httpClient *http.Client
	cbTimeout  time.Duration
}

// JWTValidatorOption is a functional option for configuring JWTValidator.
type JWTValidatorOption func(*JWTValidator)

// WithHTTPClient sets the HTTP client used for JWKS fetches.
func WithHTTPClient(client *http.Client) JWTValidatorOption {
	return func(v *JWTValidator) { v.httpClient = client }
}

// WithTokenReviewer sets the K8s TokenReviewer for ServiceAccount token validation.
func WithTokenReviewer(reviewer *TokenReviewer) JWTValidatorOption {
	return func(v *JWTValidator) { v.reviewer = reviewer }
}

// WithCBTestTimeout overrides the circuit breaker timeout (default 30s).
// Intended for testing with short timeouts.
func WithCBTestTimeout(d time.Duration) JWTValidatorOption {
	return func(v *JWTValidator) { v.cbTimeout = d }
}

// NewJWTValidator creates a new JWTValidator from the given config.
// Returns an error if the config is invalid (e.g., duplicate issuer URLs)
// or if any CEL expression fails to compile.
func NewJWTValidator(cfg AuthConfig, opts ...JWTValidatorOption) (*JWTValidator, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	providers := make(map[string]*providerRuntime, len(cfg.JWT))
	issuers := make([]string, 0, len(cfg.JWT))

	for _, pc := range cfg.JWT {
		rt := &providerRuntime{config: pc}

		for _, rule := range pc.UserValidationRules {
			program, err := compileCELRule(rule.Expression)
			if err != nil {
				return nil, fmt.Errorf("compile CEL rule for issuer %s: %w", pc.Issuer.URL, err)
			}
			rt.celRules = append(rt.celRules, compiledRule{program: program, message: rule.Message})
		}

		providers[pc.Issuer.URL] = rt
		issuers = append(issuers, pc.Issuer.URL)
	}

	v := &JWTValidator{
		providers:  providers,
		k8sEnabled: cfg.Kubernetes.Enabled,
		cbTimeout:  30 * time.Second,
	}

	for _, opt := range opts {
		opt(v)
	}

	var cacheOpts []JWKSCacheOption
	if v.cbTimeout != 30*time.Second {
		cacheOpts = append(cacheOpts, WithCBTimeout(v.cbTimeout))
	}
	v.cache = NewJWKSCache(v.httpClient, issuers, cacheOpts...)

	return v, nil
}

// Validate validates a raw JWT token string and returns the authenticated UserIdentity.
// It routes to the correct provider based on the token's issuer claim (deterministic, no fallthrough).
// If the token is not a valid JWT and K8s auth is enabled, it falls through to TokenReview.
func (v *JWTValidator) Validate(ctx context.Context, rawToken string) (*UserIdentity, error) {
	token, err := josejwt.ParseSigned(rawToken, []jose.SignatureAlgorithm{jose.RS256, jose.ES256})
	if err != nil {
		return v.fallbackToTokenReview(ctx, rawToken, fmt.Errorf("%w: %v", ErrMalformedToken, err))
	}

	issuer, err := extractIssuerUnsafe(token)
	if err != nil {
		return nil, fmt.Errorf("%w: cannot read issuer", ErrMalformedToken)
	}

	provider, ok := v.providers[issuer]
	if !ok {
		return v.fallbackToTokenReview(ctx, rawToken, ErrUnknownIssuer)
	}

	keySet, err := v.cache.GetKeys(ctx, issuer)
	if err != nil {
		if errors.Is(err, ErrCircuitOpen) {
			return nil, ErrCircuitOpen
		}
		return nil, fmt.Errorf("JWKS fetch failed: %w", err)
	}

	claims, err := verifySignature(token, keySet)
	if err != nil {
		return nil, err
	}

	if err := validateExpiry(claims); err != nil {
		return nil, err
	}

	if err := validateAudience(claims, provider.config.Issuer.Audiences); err != nil {
		return nil, err
	}

	identity := buildIdentity(claims, issuer, rawToken)

	for _, rule := range provider.celRules {
		if err := evaluateCELRule(rule, identity); err != nil {
			return nil, err
		}
	}

	return identity, nil
}

func (v *JWTValidator) fallbackToTokenReview(ctx context.Context, rawToken string, origErr error) (*UserIdentity, error) {
	if !v.k8sEnabled || v.reviewer == nil {
		return nil, origErr
	}
	return v.reviewer.Validate(ctx, rawToken)
}

// extractIssuerUnsafe reads the unverified "iss" claim for provider routing.
// This is safe because it is used solely to select which JWKS key set to use
// for signature verification; the token is NOT trusted until verifySignature
// succeeds with the selected provider's keys. An attacker who spoofs the issuer
// will fail signature verification against the wrong key set.
func extractIssuerUnsafe(token *josejwt.JSONWebToken) (string, error) {
	var unverified struct {
		Issuer string `json:"iss"`
	}
	if err := token.UnsafeClaimsWithoutVerification(&unverified); err != nil {
		return "", err
	}
	return unverified.Issuer, nil
}

func verifySignature(token *josejwt.JSONWebToken, keySet *jose.JSONWebKeySet) (map[string]interface{}, error) {
	for _, key := range keySet.Keys {
		raw := json.RawMessage{}
		if err := token.Claims(key.Key, &raw); err != nil {
			continue
		}

		var claims map[string]interface{}
		if err := json.Unmarshal(raw, &claims); err != nil {
			continue
		}
		return claims, nil
	}
	return nil, fmt.Errorf("%w: signature verification failed", ErrMalformedToken)
}

func validateExpiry(claims map[string]interface{}) error {
	exp, ok := claims["exp"].(float64)
	if !ok {
		return nil
	}
	if time.Unix(int64(exp), 0).Before(time.Now()) {
		return ErrTokenExpired
	}
	return nil
}

func validateAudience(claims map[string]interface{}, expectedAudiences []string) error {
	tokenAudiences := extractAudiences(claims)
	for _, expected := range expectedAudiences {
		for _, actual := range tokenAudiences {
			if actual == expected {
				return nil
			}
		}
	}
	return ErrInvalidAudience
}

func extractAudiences(claims map[string]interface{}) []string {
	switch v := claims["aud"].(type) {
	case string:
		return []string{v}
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, a := range v {
			if s, ok := a.(string); ok {
				result = append(result, s)
			}
		}
		return result
	default:
		return nil
	}
}

func buildIdentity(claims map[string]interface{}, issuer, rawToken string) *UserIdentity {
	username := extractStringClaim(claims, "preferred_username")
	if username == "" {
		username = extractStringClaim(claims, "sub")
	}
	return &UserIdentity{
		Username: security.SanitizeClaimValue(username),
		Groups:   sanitizeGroups(extractGroupsClaim(claims)),
		Issuer:   issuer,
		RawToken: rawToken,
	}
}

func sanitizeGroups(groups []string) []string {
	if groups == nil {
		return nil
	}
	sanitized := make([]string, len(groups))
	for i, g := range groups {
		sanitized[i] = security.SanitizeClaimValue(g)
	}
	return sanitized
}

func extractStringClaim(claims map[string]interface{}, key string) string {
	v, ok := claims[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

func extractGroupsClaim(claims map[string]interface{}) []string {
	v, ok := claims["groups"]
	if !ok {
		return nil
	}
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	groups := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			groups = append(groups, s)
		}
	}
	return groups
}

func compileCELRule(expression string) (cel.Program, error) {
	env, err := cel.NewEnv(
		cel.Variable("user", cel.MapType(cel.StringType, cel.AnyType)),
	)
	if err != nil {
		return nil, err
	}

	ast, issues := env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return nil, issues.Err()
	}

	return env.Program(ast)
}

func evaluateCELRule(rule compiledRule, identity *UserIdentity) error {
	userMap := map[string]interface{}{
		"username": identity.Username,
		"groups":   identity.Groups,
	}

	out, _, err := rule.program.Eval(map[string]interface{}{
		"user": userMap,
	})
	if err != nil {
		return fmt.Errorf("%w: evaluation error: %v", ErrCELValidation, err)
	}

	if out.Value() != true {
		return fmt.Errorf("%w: %s", ErrCELValidation, rule.message)
	}

	return nil
}

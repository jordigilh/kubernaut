package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// ErrEmptyUsername is returned when impersonation is requested with an empty username.
var ErrEmptyUsername = errors.New("impersonation: username must not be empty")

// ErrForbidden wraps a 403 Forbidden error from the K8s API.
var ErrForbidden = errors.New("access denied")

// ResourceScope categorizes K8s resources by their access pattern.
type ResourceScope int

const (
	// ScopeUserImpersonation means the resource is accessed with user identity.
	ScopeUserImpersonation ResourceScope = iota
	// ScopeServiceAccount means the resource is accessed with AF's own SA.
	ScopeServiceAccount
)

// NewImpersonatedConfig creates a deep copy of the base rest.Config with K8s
// impersonation configured for the specified user and groups.
// The original config is never mutated. Returns ErrEmptyUsername if username is empty.
func NewImpersonatedConfig(baseCfg *rest.Config, username string, groups []string) (*rest.Config, error) {
	if username == "" {
		return nil, ErrEmptyUsername
	}

	cfg := rest.CopyConfig(baseCfg)
	cfg.Impersonate = rest.ImpersonationConfig{
		UserName: username,
		Groups:   groups,
	}

	return cfg, nil
}

// ClientFactory creates K8s clients appropriate for the resource scope.
// User-scoped queries use impersonation; AF-owned resources use the base SA config.
type ClientFactory struct {
	baseCfg *rest.Config
}

// NewClientFactory creates a ClientFactory from the given base config (AF's SA).
func NewClientFactory(baseCfg *rest.Config) (*ClientFactory, error) {
	if baseCfg == nil {
		return nil, fmt.Errorf("base config must not be nil")
	}
	return &ClientFactory{baseCfg: baseCfg}, nil
}

// ClientForScope returns a K8s client appropriate for the resource scope.
// ScopeUserImpersonation creates an impersonated client using the identity.
// ScopeServiceAccount returns a client using AF's own SA credentials.
func (f *ClientFactory) ClientForScope(scope ResourceScope, identity *UserIdentity) (kubernetes.Interface, error) {
	switch scope {
	case ScopeUserImpersonation:
		if identity == nil {
			return nil, fmt.Errorf("identity required for user impersonation scope")
		}
		cfg, err := NewImpersonatedConfig(f.baseCfg, identity.Username, identity.Groups)
		if err != nil {
			return nil, err
		}
		return kubernetes.NewForConfig(cfg)

	case ScopeServiceAccount:
		return kubernetes.NewForConfig(f.baseCfg)

	default:
		return nil, fmt.Errorf("unknown resource scope: %d", scope)
	}
}

// JWTDelegationTransport wraps an http.RoundTripper to inject the user's original JWT
// as an Authorization: Bearer header when making requests to KA.
// The token is forwarded byte-identical (no re-signing or modification).
type JWTDelegationTransport struct {
	Base  http.RoundTripper
	Token string
}

// RoundTrip injects the Authorization header with the original JWT.
func (t *JWTDelegationTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	reqClone := req.Clone(req.Context())
	reqClone.Header.Set("Authorization", "Bearer "+t.Token)

	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(reqClone)
}

// ToUserFriendlyError translates K8s API errors into user-friendly messages.
// Uses errors.As for typed error detection (#49, #50 from 100-go-mistakes)
// rather than string matching where possible.
func ToUserFriendlyError(err error) string {
	if err == nil {
		return ""
	}

	// Use typed error detection (preferred over string matching)
	var statusErr *k8serrors.StatusError
	if errors.As(err, &statusErr) {
		if statusErr.ErrStatus.Code == http.StatusForbidden {
			return buildForbiddenMessage(statusErr.ErrStatus.Message)
		}
		return statusErr.ErrStatus.Message
	}

	// Fallback: string-based detection for non-K8s errors that still indicate 403
	msg := err.Error()
	if strings.Contains(msg, "forbidden") || strings.Contains(msg, "Forbidden") {
		return buildForbiddenMessage(msg)
	}

	return msg
}

func buildForbiddenMessage(msg string) string {
	parts := strings.SplitN(msg, "cannot", 2)
	if len(parts) == 2 {
		action := strings.TrimSpace(parts[1])
		if idx := strings.Index(action, "in API group"); idx > 0 {
			action = strings.TrimSpace(action[:idx])
		}
		return fmt.Sprintf("You lack access to %s. Contact your cluster administrator for RBAC permissions.", action)
	}
	return "You lack access to this resource. Contact your cluster administrator for RBAC permissions."
}

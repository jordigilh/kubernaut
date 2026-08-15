package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ToolAuthorizer checks whether a user is authorized to invoke a specific tool.
// Implementations must be safe for concurrent use.
type ToolAuthorizer interface {
	Check(ctx context.Context, user string, groups []string, toolName string) (bool, error)
}

// ConsoleAuthorizer checks whether a user is authorized to access the
// kubernaut console/chat UI at all -- a coarse-grained gate independent of
// (and enforced in addition to) any per-tool ToolAuthorizer.Check result
// (#1919, AC-3/AC-6). Implementations must be safe for concurrent use.
//
// This is an optional capability: callers holding a ToolAuthorizer type-assert
// for ConsoleAuthorizer and skip the console check when the concrete
// authorizer doesn't implement it, so existing ToolAuthorizer-only test
// doubles need no changes (see checkRBAC in mcp_bridge.go and newRBACGuard in
// agent/root.go).
type ConsoleAuthorizer interface {
	CheckConsoleAccess(ctx context.Context, user string, groups []string) (bool, error)
}

type cacheEntry struct {
	allowed   bool
	expiresAt time.Time
}

// SARChecker implements ToolAuthorizer using Kubernetes SubjectAccessReview.
// Results are cached with a configurable TTL to reduce API server load.
//
// Cache eviction: a background goroutine sweeps expired entries every 2*cacheTTL
// (minimum 60s) to prevent unbounded growth from departed users. Each entry is
// ~100 bytes, so even at 1000 users x 21 tools the peak between sweeps is ~2MB.
type SARChecker struct {
	client   kubernetes.Interface
	cacheTTL time.Duration
	logger   logr.Logger
	mu       sync.RWMutex
	cache    map[string]cacheEntry
}

// NewSARChecker creates a SARChecker that performs SubjectAccessReview calls
// against the Kubernetes API server with results cached for the given TTL.
// A background sweep goroutine evicts expired entries periodically.
func NewSARChecker(client kubernetes.Interface, cacheTTL time.Duration, logger logr.Logger) *SARChecker {
	s := &SARChecker{
		client:   client,
		cacheTTL: cacheTTL,
		logger:   logger,
		cache:    make(map[string]cacheEntry, 64),
	}

	sweepInterval := 2 * cacheTTL
	if sweepInterval < 60*time.Second {
		sweepInterval = 60 * time.Second
	}
	go s.evictExpired(sweepInterval)

	return s
}

// evictExpired periodically removes cache entries whose TTL has expired.
func (s *SARChecker) evictExpired(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		s.mu.Lock()
		for k, v := range s.cache {
			if now.After(v.expiresAt) {
				delete(s.cache, k)
			}
		}
		s.mu.Unlock()
	}
}

// Check verifies whether the given user (with groups) is authorized to invoke toolName
// by performing a Kubernetes SubjectAccessReview against the kubernaut.ai/tools resource.
// Results are cached for the configured TTL. Errors are never cached (retried on next call).
func (s *SARChecker) Check(ctx context.Context, user string, groups []string, toolName string) (bool, error) {
	if toolName == "" {
		return false, fmt.Errorf("tool name must not be empty")
	}
	return s.checkSAR(ctx, user, groups, "tools", toolName)
}

// CheckConsoleAccess verifies whether the given user (with groups) holds the
// coarse-grained, unnamed "console" grant (#1919, AC-3/AC-6) -- a separate,
// independently-auditable authorization step from any per-tool grant. It
// performs a Kubernetes SubjectAccessReview against the kubernaut.ai/console
// resource (verb=use, no resourceName). Results are cached for the
// configured TTL, in the same cache but keyed distinctly from Check's
// "tools" entries so the two never collide.
func (s *SARChecker) CheckConsoleAccess(ctx context.Context, user string, groups []string) (bool, error) {
	return s.checkSAR(ctx, user, groups, "console", "")
}

// checkSAR is the shared SAR-call-plus-cache implementation behind both
// Check (resource="tools", name=toolName) and CheckConsoleAccess
// (resource="console", name=""). Fail-closed: empty user or any API error
// returns (false, err).
func (s *SARChecker) checkSAR(ctx context.Context, user string, groups []string, resource, name string) (bool, error) {
	if user == "" {
		return false, fmt.Errorf("user must not be empty")
	}

	key := cacheKey(user, groups, resource, name)

	s.mu.RLock()
	if entry, ok := s.cache[key]; ok && time.Now().Before(entry.expiresAt) {
		s.mu.RUnlock()
		return entry.allowed, nil
	}
	s.mu.RUnlock()

	sar := &authorizationv1.SubjectAccessReview{
		Spec: authorizationv1.SubjectAccessReviewSpec{
			User:   user,
			Groups: groups,
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Verb:     "use",
				Group:    "kubernaut.ai",
				Resource: resource,
				Name:     name,
			},
		},
	}

	result, err := s.client.AuthorizationV1().SubjectAccessReviews().Create(ctx, sar, metav1.CreateOptions{})
	if err != nil {
		s.logger.Error(err, "SAR API call failed", "user", user, "resource", resource, "name", name)
		return false, fmt.Errorf("SAR authorization check failed: %w", err)
	}

	allowed := result.Status.Allowed

	s.mu.Lock()
	s.cache[key] = cacheEntry{
		allowed:   allowed,
		expiresAt: time.Now().Add(s.cacheTTL),
	}
	s.mu.Unlock()

	if !allowed {
		s.logger.V(1).Info("SAR denied access", "user", user, "resource", resource, "name", name, "groups", groups)
	}

	return allowed, nil
}

// ConsoleAccessAuthorizationCheckGate wraps a *SARChecker to make the
// coarse-grained console gate authentication-only (#2148): CheckConsoleAccess
// no longer performs its authorization check (a SAR call), it just requires
// a non-empty user. Check (per-tool authorization, the actual security
// boundary) is inherited unchanged via embedding -- this type never defines
// its own Check, so that invariant is provably untouched by construction,
// not merely by convention.
//
// This is the default (#2150) so a fresh install's console works with zero
// RBAC configuration -- production installs that configure real
// personas/consoleAccessGroups should set
// RBACConfig.ConsoleAccessAuthorizationCheckEnabled to true so
// ConsoleAccessGroups takes precedence and the console gate's authorization
// check runs too.
type ConsoleAccessAuthorizationCheckGate struct {
	*SARChecker
}

// NewConsoleAccessAuthorizationCheckGate wraps checker so that
// CheckConsoleAccess bypasses its authorization check and grants access to
// any non-empty authenticated user. Check (per-tool authorization) delegates
// to checker unchanged.
func NewConsoleAccessAuthorizationCheckGate(checker *SARChecker) *ConsoleAccessAuthorizationCheckGate {
	return &ConsoleAccessAuthorizationCheckGate{SARChecker: checker}
}

// CheckConsoleAccess grants console access to any non-empty authenticated
// user without running the authorization check. Authentication remains
// mandatory: an empty user still fails closed.
func (g *ConsoleAccessAuthorizationCheckGate) CheckConsoleAccess(_ context.Context, user string, _ []string) (bool, error) {
	if user == "" {
		return false, fmt.Errorf("user must not be empty")
	}
	return true, nil
}

var _ ToolAuthorizer = (*ConsoleAccessAuthorizationCheckGate)(nil)
var _ ConsoleAuthorizer = (*ConsoleAccessAuthorizationCheckGate)(nil)

func cacheKey(user string, groups []string, resource, name string) string {
	sorted := make([]string, len(groups))
	copy(sorted, groups)
	sort.Strings(sorted)
	raw := user + "\x00" + strings.Join(sorted, "\x00") + "\x00" + resource + "\x00" + name
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

var _ ToolAuthorizer = (*SARChecker)(nil)
var _ ConsoleAuthorizer = (*SARChecker)(nil)

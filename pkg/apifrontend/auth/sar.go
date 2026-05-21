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
	if user == "" {
		return false, fmt.Errorf("user must not be empty")
	}
	if toolName == "" {
		return false, fmt.Errorf("tool name must not be empty")
	}

	key := cacheKey(user, groups, toolName)

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
				Resource: "tools",
				Name:     toolName,
			},
		},
	}

	result, err := s.client.AuthorizationV1().SubjectAccessReviews().Create(ctx, sar, metav1.CreateOptions{})
	if err != nil {
		s.logger.Error(err, "SAR API call failed", "user", user, "tool", toolName)
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
		s.logger.V(1).Info("SAR denied tool access", "user", user, "tool", toolName, "groups", groups)
	}

	return allowed, nil
}

func cacheKey(user string, groups []string, toolName string) string {
	sorted := make([]string, len(groups))
	copy(sorted, groups)
	sort.Strings(sorted)
	raw := user + "\x00" + strings.Join(sorted, "\x00") + "\x00" + toolName
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

var _ ToolAuthorizer = (*SARChecker)(nil)

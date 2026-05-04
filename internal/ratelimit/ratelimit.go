package ratelimit

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/time/rate"

	"github.com/jordigilh/kubernaut-apifrontend/internal/audit"
	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
	"github.com/jordigilh/kubernaut-apifrontend/internal/httputil"
)

// IPLimiter provides per-IP token bucket rate limiting (pre-authentication tier).
type IPLimiter struct {
	mu       sync.Mutex
	limiters map[string]*ipEntry
	cfg      PerIPConfig
	stopCh   chan struct{}
	stopOnce sync.Once
}

type ipEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewIPLimiter creates a per-IP rate limiter and starts background eviction.
func NewIPLimiter(cfg PerIPConfig) *IPLimiter {
	if cfg.CleanupInterval <= 0 {
		cfg.CleanupInterval = 5 * time.Minute
	}
	if cfg.MaxAge <= 0 {
		cfg.MaxAge = 10 * time.Minute
	}
	l := &IPLimiter{
		limiters: make(map[string]*ipEntry),
		cfg:      cfg,
		stopCh:   make(chan struct{}),
	}
	go l.cleanup()
	return l
}

// Allow checks if the given IP is within its rate limit.
func (l *IPLimiter) Allow(ip string) bool {
	l.mu.Lock()
	entry, ok := l.limiters[ip]
	if !ok {
		entry = &ipEntry{
			limiter: rate.NewLimiter(rate.Limit(l.cfg.RequestsPerSecond), l.cfg.Burst),
		}
		l.limiters[ip] = entry
	}
	entry.lastSeen = time.Now()
	l.mu.Unlock()

	return entry.limiter.Allow()
}

// Stop halts the background cleanup goroutine.
func (l *IPLimiter) Stop() {
	l.stopOnce.Do(func() { close(l.stopCh) })
}

func (l *IPLimiter) cleanup() {
	ticker := time.NewTicker(l.cfg.CleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-l.stopCh:
			return
		case <-ticker.C:
			l.mu.Lock()
			cutoff := time.Now().Add(-l.cfg.MaxAge)
			for ip, entry := range l.limiters {
				if entry.lastSeen.Before(cutoff) {
					delete(l.limiters, ip)
				}
			}
			l.mu.Unlock()
		}
	}
}

// UserLimiter provides per-user rate limiting (post-authentication tier).
type UserLimiter struct {
	cfg      PerUserConfig
	mu       sync.Mutex
	requests map[string]*rate.Limiter
	sessions map[string]*atomic.Int32
	tools    map[string]*rate.Limiter
}

// NewUserLimiter creates a per-user rate limiter.
func NewUserLimiter(cfg PerUserConfig) *UserLimiter {
	return &UserLimiter{
		cfg:      cfg,
		requests: make(map[string]*rate.Limiter),
		sessions: make(map[string]*atomic.Int32),
		tools:    make(map[string]*rate.Limiter),
	}
}

// AllowRequest checks if the user is within their per-minute request rate limit.
func (l *UserLimiter) AllowRequest(username string) bool {
	l.mu.Lock()
	lim, ok := l.requests[username]
	if !ok {
		lim = rate.NewLimiter(rate.Limit(float64(l.cfg.RequestsPerMinute)/60.0), l.cfg.RequestsPerMinute)
		l.requests[username] = lim
	}
	l.mu.Unlock()

	return lim.Allow()
}

// AcquireSession attempts to acquire a concurrent session slot for the user.
func (l *UserLimiter) AcquireSession(username string) bool {
	l.mu.Lock()
	counter, ok := l.sessions[username]
	if !ok {
		counter = &atomic.Int32{}
		l.sessions[username] = counter
	}
	l.mu.Unlock()

	for {
		current := counter.Load()
		if int(current) >= l.cfg.MaxConcurrentSessions {
			return false
		}
		if counter.CompareAndSwap(current, current+1) {
			return true
		}
	}
}

// ReleaseSession releases a concurrent session slot for the user.
func (l *UserLimiter) ReleaseSession(username string) {
	l.mu.Lock()
	counter, ok := l.sessions[username]
	l.mu.Unlock()

	if ok {
		counter.Add(-1)
	}
}

// AllowToolCall checks if the user is within their per-minute tool call rate limit.
func (l *UserLimiter) AllowToolCall(username string) bool {
	l.mu.Lock()
	lim, ok := l.tools[username]
	if !ok {
		lim = rate.NewLimiter(rate.Limit(float64(l.cfg.ToolCallsPerMinute)/60.0), l.cfg.ToolCallsPerMinute)
		l.tools[username] = lim
	}
	l.mu.Unlock()

	return lim.Allow()
}

// ProviderLimiter limits JWKS fetch rate per OIDC provider.
type ProviderLimiter struct {
	mu       sync.Mutex
	lastFetch map[string]time.Time
	interval time.Duration
}

// NewProviderLimiter creates a per-provider rate limiter.
func NewProviderLimiter(cfg PerProviderConfig) *ProviderLimiter {
	return &ProviderLimiter{
		lastFetch: make(map[string]time.Time),
		interval:  time.Duration(cfg.FetchIntervalSeconds) * time.Second,
	}
}

// AllowFetch checks if a JWKS fetch is allowed for the provider.
func (l *ProviderLimiter) AllowFetch(provider string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	last, ok := l.lastFetch[provider]
	if ok && time.Since(last) < l.interval {
		return false
	}
	l.lastFetch[provider] = time.Now()
	return true
}

// LLMSemaphore limits global LLM concurrency.
type LLMSemaphore struct {
	max     int32
	current atomic.Int32
}

// NewLLMSemaphore creates a semaphore with the given max concurrent slots.
func NewLLMSemaphore(max int) *LLMSemaphore {
	return &LLMSemaphore{max: int32(max)}
}

// Acquire attempts to acquire a semaphore slot. Returns false if at capacity.
func (s *LLMSemaphore) Acquire() bool {
	for {
		current := s.current.Load()
		if current >= s.max {
			return false
		}
		if s.current.CompareAndSwap(current, current+1) {
			return true
		}
	}
}

// Release releases a semaphore slot.
func (s *LLMSemaphore) Release() {
	s.current.Add(-1)
}

// NewRateLimitDeniedTotal creates a fresh rate limit denied counter.
// Call this from the metrics registry to avoid package-level state.
func NewRateLimitDeniedTotal() *prometheus.CounterVec {
	return prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "kubernaut_apifrontend",
		Name:      "ratelimit_denied_total",
		Help:      "Total rate limit denials by tier and reason.",
	}, []string{"tier", "reason"})
}

// PreAuthMiddlewareConfig holds dependencies for pre-auth rate limiting.
type PreAuthMiddlewareConfig struct {
	Limiter *IPLimiter
	Auditor audit.Emitter
	Metrics *prometheus.CounterVec
}

// PreAuthMiddleware returns middleware for pre-authentication rate limiting (per-IP).
// Returns RFC 7807 429 with Retry-After header when rate limit is exceeded.
func PreAuthMiddleware(limiter *IPLimiter) func(http.Handler) http.Handler {
	return PreAuthMiddlewareWithConfig(PreAuthMiddlewareConfig{Limiter: limiter})
}

// PreAuthMiddlewareWithConfig returns pre-auth rate limiting middleware with audit support.
func PreAuthMiddlewareWithConfig(cfg PreAuthMiddlewareConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := extractIP(r)
			if !cfg.Limiter.Allow(ip) {
				retryAfter := int(1.0 / cfg.Limiter.cfg.RequestsPerSecond)
				if retryAfter < 1 {
					retryAfter = 1
				}
				if cfg.Metrics != nil {
					cfg.Metrics.WithLabelValues("ip", "burst_exceeded").Inc()
				}
				if cfg.Auditor != nil {
					cfg.Auditor.Emit(r.Context(), audit.Event{
						Type:     audit.EventRateLimitDenied,
						SourceIP: ip,
						Detail:   map[string]string{"tier": "ip"},
					})
				}
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				httputil.WriteProblem(w, http.StatusTooManyRequests,
					"Rate Limit Exceeded", "Too many requests from your IP address. Please retry later.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// PostAuthMiddlewareConfig holds dependencies for post-auth rate limiting.
type PostAuthMiddlewareConfig struct {
	Limiter *UserLimiter
	Auditor audit.Emitter
	Metrics *prometheus.CounterVec
}

// PostAuthMiddleware returns middleware for post-authentication rate limiting (per-user).
// Reads UserIdentity from context (set by JWT middleware).
func PostAuthMiddleware(limiter *UserLimiter) func(http.Handler) http.Handler {
	return PostAuthMiddlewareWithConfig(PostAuthMiddlewareConfig{Limiter: limiter})
}

// PostAuthMiddlewareWithConfig returns post-auth rate limiting middleware with audit support.
func PostAuthMiddlewareWithConfig(cfg PostAuthMiddlewareConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity := auth.UserIdentityFromContext(r.Context())
			if identity == nil {
				next.ServeHTTP(w, r)
				return
			}

			if !cfg.Limiter.AllowRequest(identity.Username) {
				if cfg.Metrics != nil {
					cfg.Metrics.WithLabelValues("user", "request_rate").Inc()
				}
				if cfg.Auditor != nil {
					cfg.Auditor.Emit(r.Context(), audit.Event{
						Type:     audit.EventRateLimitDenied,
						UserID:   identity.Username,
						SourceIP: extractIP(r),
						Detail:   map[string]string{"tier": "user"},
					})
				}
				w.Header().Set("Retry-After", "1")
				httputil.WriteProblem(w, http.StatusTooManyRequests,
					"Rate Limit Exceeded", "You have exceeded your request rate limit. Please retry later.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func extractIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.Index(xff, ","); idx != -1 {
			xff = strings.TrimSpace(xff[:idx])
		}
		if ip, _, err := net.SplitHostPort(xff); err == nil {
			return ip
		}
		return xff
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

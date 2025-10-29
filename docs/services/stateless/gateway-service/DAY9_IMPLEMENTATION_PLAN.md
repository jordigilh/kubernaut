# 🎯 Day 9: Metrics + Observability - Implementation Plan

**Date**: 2025-10-26
**Status**: 🚀 **READY TO START**
**Estimated Time**: 13 hours
**Priority**: HIGH (Required for debugging integration test failures)

---

## 📋 **Prerequisites - COMPLETE** ✅

- ✅ Authentication infrastructure working (Kind-only)
- ✅ Structured logging migrated to `zap`
- ✅ Integration test baseline established (37% pass rate)
- ✅ 58 test failures documented with root causes
- ✅ Metrics temporarily disabled (nil checks in place)

---

## 🎯 **Business Requirements**

### **Primary Goals**
1. **BR-GATEWAY-010**: Implement comprehensive Prometheus metrics
2. **BR-GATEWAY-011**: Health and readiness endpoints
3. **BR-GATEWAY-012**: Observability for debugging 503/OOM issues
4. **BR-GATEWAY-013**: TokenReview/SubjectAccessReview timeout tracking
5. **BR-GATEWAY-014**: K8s API latency monitoring

### **Success Criteria**
- ✅ `/health` endpoint returns 200 when healthy
- ✅ `/ready` endpoint returns 200 when ready
- ✅ `/metrics` endpoint exposes Prometheus metrics
- ✅ All middleware instrumented with metrics
- ✅ Redis connection health tracked
- ✅ K8s API latency tracked
- ✅ TokenReview/SubjectAccessReview timeouts tracked
- ✅ 100% unit test coverage for metrics
- ✅ Integration tests validate metrics collection

---

## 📊 **Phase 1: Health Endpoints (2 hours)**

### **Phase 1.1: Health Endpoint Implementation (1h)**

**File**: `pkg/gateway/server/health.go` (NEW)

**Implementation**:
```go
package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// HealthStatus represents the health status of the Gateway
type HealthStatus struct {
	Status    string            `json:"status"`    // "healthy" or "unhealthy"
	Timestamp time.Time         `json:"timestamp"` // Current time
	Checks    map[string]string `json:"checks"`    // Component health checks
}

// HealthHandler returns the health status of the Gateway
// BR-GATEWAY-011: Health endpoint for liveness probe
func (s *Server) HealthHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	checks := make(map[string]string)
	allHealthy := true

	// Check 1: Redis connectivity
	if s.redisClient != nil {
		if err := s.redisClient.Ping(ctx).Err(); err != nil {
			checks["redis"] = "unhealthy: " + err.Error()
			allHealthy = false
		} else {
			checks["redis"] = "healthy"
		}
	} else {
		checks["redis"] = "not configured"
	}

	// Check 2: K8s API connectivity
	if s.k8sClientset != nil {
		if _, err := s.k8sClientset.Discovery().ServerVersion(); err != nil {
			checks["kubernetes"] = "unhealthy: " + err.Error()
			allHealthy = false
		} else {
			checks["kubernetes"] = "healthy"
		}
	} else {
		checks["kubernetes"] = "not configured"
	}

	status := "healthy"
	statusCode := http.StatusOK
	if !allHealthy {
		status = "unhealthy"
		statusCode = http.StatusServiceUnavailable
	}

	health := HealthStatus{
		Status:    status,
		Timestamp: time.Now(),
		Checks:    checks,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(health); err != nil {
		s.logger.Error("Failed to encode health response", zap.Error(err))
	}
}

// ReadinessHandler returns the readiness status of the Gateway
// BR-GATEWAY-011: Readiness endpoint for readiness probe
func (s *Server) ReadinessHandler(w http.ResponseWriter, r *http.Request) {
	// For now, readiness is same as health
	// In future, could add more sophisticated checks (e.g., warming up caches)
	s.HealthHandler(w, r)
}
```

**Tests**: `test/unit/gateway/server/health_test.go` (NEW)
- Test health endpoint returns 200 when all checks pass
- Test health endpoint returns 503 when Redis unhealthy
- Test health endpoint returns 503 when K8s API unhealthy
- Test health endpoint respects 5s timeout
- Test readiness endpoint mirrors health endpoint

---

### **Phase 1.2: Wire Health Endpoints to Router (30min)**

**File**: `pkg/gateway/server/server.go`

**Changes**:
```go
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()

	// ... existing middleware ...

	// Health endpoints (no authentication required)
	r.Get("/health", s.HealthHandler)
	r.Get("/ready", s.ReadinessHandler)

	// Webhook endpoints (authentication required)
	r.Post("/webhook/prometheus", s.PrometheusWebhookHandler)
	r.Post("/webhook/kubernetes", s.KubernetesEventWebhookHandler)

	return r
}
```

**Tests**: `test/integration/gateway/health_integration_test.go` (NEW)
- Test `/health` endpoint with real Redis
- Test `/health` endpoint with real K8s API
- Test `/health` endpoint when Redis unavailable
- Test `/health` endpoint when K8s API unavailable
- Test `/ready` endpoint mirrors `/health`

---

### **Phase 1.3: Update K8s Manifests (30min)**

**File**: `deploy/gateway/deployment.yaml`

**Changes**:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
  namespace: kubernaut-system
spec:
  template:
    spec:
      containers:
      - name: gateway
        image: gateway:latest
        ports:
        - containerPort: 8080
          name: http
        livenessProbe:
          httpGet:
            path: /health
            port: http
          initialDelaySeconds: 10
          periodSeconds: 10
          timeoutSeconds: 5
          failureThreshold: 3
        readinessProbe:
          httpGet:
            path: /ready
            port: http
          initialDelaySeconds: 5
          periodSeconds: 5
          timeoutSeconds: 5
          failureThreshold: 3
```

---

## 📊 **Phase 2: Prometheus Metrics Integration (4.5 hours)**

### **Phase 2.1: Metrics Registry Setup (30min)**

**File**: `pkg/gateway/metrics/registry.go` (NEW)

**Implementation**:
```go
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Gateway Prometheus metrics
type Metrics struct {
	// HTTP Metrics
	HTTPRequestsTotal      *prometheus.CounterVec
	HTTPRequestDuration    *prometheus.HistogramVec
	HTTPRequestsInFlight   prometheus.Gauge
	HTTPRequestSize        *prometheus.HistogramVec
	HTTPResponseSize       *prometheus.HistogramVec

	// Authentication Metrics
	TokenReviewTotal       *prometheus.CounterVec
	TokenReviewDuration    *prometheus.HistogramVec
	TokenReviewTimeouts    prometheus.Counter
	SubjectAccessReviewTotal    *prometheus.CounterVec
	SubjectAccessReviewDuration *prometheus.HistogramVec
	SubjectAccessReviewTimeouts prometheus.Counter

	// K8s API Metrics
	K8sAPIRequestsTotal    *prometheus.CounterVec
	K8sAPIRequestDuration  *prometheus.HistogramVec
	K8sAPILatency          *prometheus.HistogramVec

	// Redis Metrics
	RedisCommandsTotal     *prometheus.CounterVec
	RedisCommandDuration   *prometheus.HistogramVec
	RedisConnectionsActive prometheus.Gauge
	RedisErrorsTotal       *prometheus.CounterVec

	// Business Logic Metrics
	DeduplicationTotal     *prometheus.CounterVec
	StormDetectionTotal    *prometheus.CounterVec
	CRDCreationTotal       *prometheus.CounterVec
	CRDCreationDuration    *prometheus.HistogramVec

	// Rate Limiting Metrics
	RateLimitExceeded      *prometheus.CounterVec
}

// NewMetrics creates a new Metrics instance with all metrics registered
func NewMetrics() *Metrics {
	return &Metrics{
		HTTPRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gateway_http_requests_total",
				Help: "Total number of HTTP requests",
			},
			[]string{"method", "path", "status"},
		),
		HTTPRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "gateway_http_request_duration_seconds",
				Help:    "HTTP request duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"method", "path", "status"},
		),
		HTTPRequestsInFlight: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "gateway_http_requests_in_flight",
				Help: "Current number of HTTP requests being processed",
			},
		),
		TokenReviewTimeouts: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "gateway_token_review_timeouts_total",
				Help: "Total number of TokenReview timeouts (BR-GATEWAY-013)",
			},
		),
		SubjectAccessReviewTimeouts: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "gateway_subject_access_review_timeouts_total",
				Help: "Total number of SubjectAccessReview timeouts (BR-GATEWAY-013)",
			},
		),
		K8sAPILatency: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "gateway_k8s_api_latency_seconds",
				Help:    "K8s API latency in seconds (BR-GATEWAY-014)",
				Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0, 5.0},
			},
			[]string{"operation"},
		),
		// ... other metrics ...
	}
}
```

**Tests**: `test/unit/gateway/metrics/registry_test.go` (NEW)
- Test all metrics are registered
- Test metrics can be incremented
- Test metrics can be observed
- Test metrics are exposed via Prometheus registry

---

### **Phase 2.2: Instrument Middleware (2h)**

**Files to Update**:
1. `pkg/gateway/middleware/auth.go` - Add TokenReview metrics
2. `pkg/gateway/middleware/authz.go` - Add SubjectAccessReview metrics
3. `pkg/gateway/middleware/rate_limit.go` - Add rate limit metrics
4. `pkg/gateway/middleware/logging.go` - Add HTTP request metrics

**Example** (`auth.go`):
```go
func TokenReviewAuth(k8sClient kubernetes.Interface, metrics *gatewayMetrics.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// ... existing auth logic ...

			// Record metrics
			duration := time.Since(start).Seconds()
			if metrics != nil {
				metrics.TokenReviewDuration.WithLabelValues("success").Observe(duration)
				metrics.TokenReviewTotal.WithLabelValues("success").Inc()

				// Track timeouts (BR-GATEWAY-013)
				if duration > 5.0 {
					metrics.TokenReviewTimeouts.Inc()
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}
```

---

### **Phase 2.3: Instrument Services (2h)**

**Files to Update**:
1. `pkg/gateway/processing/deduplication.go` - Add deduplication metrics
2. `pkg/gateway/processing/storm_detection.go` - Add storm detection metrics
3. `pkg/gateway/processing/crd_creator.go` - Add CRD creation metrics
4. `pkg/gateway/server/handlers.go` - Add HTTP handler metrics

**Example** (`deduplication.go`):
```go
func (d *DeduplicationService) IsDuplicate(ctx context.Context, fingerprint string) (bool, error) {
	start := time.Now()

	// ... existing logic ...

	// Record metrics
	if d.metrics != nil {
		duration := time.Since(start).Seconds()
		d.metrics.RedisCommandDuration.WithLabelValues("get").Observe(duration)
		d.metrics.DeduplicationTotal.WithLabelValues("checked").Inc()

		if isDuplicate {
			d.metrics.DeduplicationTotal.WithLabelValues("duplicate").Inc()
		} else {
			d.metrics.DeduplicationTotal.WithLabelValues("unique").Inc()
		}
	}

	return isDuplicate, nil
}
```

---

## 📊 **Phase 3: /metrics Endpoint (30 minutes)**

### **Phase 3.1: Add Prometheus Handler (15min)**

**File**: `pkg/gateway/server/server.go`

**Changes**:
```go
import (
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()

	// ... existing middleware ...

	// Health endpoints (no authentication required)
	r.Get("/health", s.HealthHandler)
	r.Get("/ready", s.ReadinessHandler)
	r.Get("/metrics", promhttp.Handler().ServeHTTP)

	// Webhook endpoints (authentication required)
	r.Post("/webhook/prometheus", s.PrometheusWebhookHandler)
	r.Post("/webhook/kubernetes", s.KubernetesEventWebhookHandler)

	return r
}
```

---

### **Phase 3.2: Test /metrics Endpoint (15min)**

**File**: `test/integration/gateway/metrics_integration_test.go` (NEW)

**Tests**:
- Test `/metrics` endpoint returns Prometheus format
- Test `/metrics` endpoint includes all registered metrics
- Test metrics are updated after webhook requests
- Test metrics persist across requests

---

## 📊 **Phase 4: Additional Metrics (2 hours)**

### **Phase 4.1: HTTP Latency Histogram (30min)**

**Implementation**:
- Add histogram buckets: [0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0, 5.0]
- Track latency per endpoint
- Track latency per status code

---

### **Phase 4.2: In-Flight Requests Gauge (30min)**

**Implementation**:
- Increment on request start
- Decrement on request end
- Track per endpoint

---

### **Phase 4.3: Redis Connection Gauge (30min)**

**Implementation**:
- Track active Redis connections
- Update on connection open/close
- Alert if connections exceed threshold

---

### **Phase 4.4: K8s API Latency Histogram (30min)**

**Implementation**:
- Track latency for TokenReview
- Track latency for SubjectAccessReview
- Track latency for CRD creation
- Track latency for CRD updates

---

## 📊 **Phase 5: Structured Logging Completion (1 hour)**

### **Phase 5.1: Audit Log Levels (30min)**

**Review all log statements**:
- `Debug`: Development-only details
- `Info`: Normal operations
- `Warn`: Recoverable errors
- `Error`: Unrecoverable errors
- `Fatal`: System shutdown

---

### **Phase 5.2: Add Structured Fields (30min)**

**Ensure all logs include**:
- `request_id`: Unique request identifier
- `namespace`: Kubernetes namespace
- `alert_name`: Alert name (if applicable)
- `duration_ms`: Operation duration
- `error`: Error message (if applicable)

---

## 📊 **Phase 6: Tests (3 hours)**

### **Phase 6.1: Unit Tests (1.5h)**

**Files**:
1. `test/unit/gateway/server/health_test.go` (5 tests)
2. `test/unit/gateway/metrics/registry_test.go` (5 tests)
3. `test/unit/gateway/metrics/instrumentation_test.go` (10 tests)

**Total**: 20 unit tests

---

### **Phase 6.2: Integration Tests (1.5h)**

**Files**:
1. `test/integration/gateway/health_integration_test.go` (5 tests)
2. `test/integration/gateway/metrics_integration_test.go` (5 tests)

**Total**: 10 integration tests

---

## 📋 **Deliverables**

### **Code**
- ✅ `pkg/gateway/server/health.go` (NEW)
- ✅ `pkg/gateway/metrics/registry.go` (NEW)
- ✅ Updated middleware with metrics
- ✅ Updated services with metrics
- ✅ Updated server with `/metrics` endpoint

### **Tests**
- ✅ 20 unit tests
- ✅ 10 integration tests
- ✅ 100% coverage for metrics code

### **Documentation**
- ✅ Metrics documentation
- ✅ Health endpoint documentation
- ✅ Observability guide

### **Deployment**
- ✅ Updated K8s manifests with health probes
- ✅ Prometheus ServiceMonitor (if using Prometheus Operator)

---

## 🎯 **Success Metrics**

| Metric | Target | Validation |
|--------|--------|------------|
| **Health Endpoint** | 200 OK when healthy | Manual test |
| **Readiness Endpoint** | 200 OK when ready | Manual test |
| **Metrics Endpoint** | Prometheus format | Manual test |
| **Unit Test Coverage** | 100% | `go test -cover` |
| **Integration Tests** | 10 passing | `go test` |
| **TokenReview Timeouts** | Tracked | Check `/metrics` |
| **SubjectAccessReview Timeouts** | Tracked | Check `/metrics` |
| **K8s API Latency** | Tracked | Check `/metrics` |

---

## 📊 **Timeline**

| Phase | Duration | Status |
|-------|----------|--------|
| **Phase 1: Health Endpoints** | 2h | 🟡 Pending |
| **Phase 2: Metrics Integration** | 4.5h | 🟡 Pending |
| **Phase 3: /metrics Endpoint** | 30min | 🟡 Pending |
| **Phase 4: Additional Metrics** | 2h | 🟡 Pending |
| **Phase 5: Logging Completion** | 1h | 🟡 Pending |
| **Phase 6: Tests** | 3h | 🟡 Pending |
| **Total** | **13h** | 🟡 Pending |

---

## 🔗 **Related Documents**

- `KIND_AUTH_COMPLETE.md` - Authentication infrastructure summary
- `REMAINING_FAILURES_ACTION_PLAN.md` - Integration test fix plan
- `CURRENT_STATUS_AND_RECOMMENDATION.md` - Decision rationale
- `DAY8_COMPLETE_SUMMARY.md` - Previous day summary

---

**Date**: 2025-10-26
**Author**: AI Assistant
**Status**: 🚀 **READY TO START**
**Confidence**: 95%

**Justification**: Clear requirements, well-defined phases, realistic timeline, strong foundation (authentication working, logging migrated, test infrastructure solid).



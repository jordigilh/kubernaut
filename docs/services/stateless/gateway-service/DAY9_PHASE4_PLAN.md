# Day 9 Phase 4: Additional Metrics - APDC Plan

**Date**: 2025-10-26
**Estimated Duration**: 2 hours
**Status**: ⏳ IN PROGRESS

---

## 🎯 **APDC Analysis**

### **Business Requirements**

**BR-GATEWAY-071**: HTTP request observability for performance monitoring
**BR-GATEWAY-072**: In-flight request tracking for capacity planning
**BR-GATEWAY-073**: Redis connection pool monitoring for resource management

**Business Value**:
- **HTTP Latency**: Identify slow endpoints and optimize performance
- **In-Flight Requests**: Detect capacity issues and prevent overload
- **Redis Pool**: Monitor connection exhaustion and prevent failures

### **Current State**

**Existing Metrics** (Phase 2):
- ✅ `gateway_processing_duration_seconds` - Signal processing time
- ✅ `gateway_k8s_api_latency_seconds` - K8s API call latency
- ❌ No HTTP request latency (overall request time)
- ❌ No in-flight request tracking
- ❌ No Redis connection pool metrics

**Gap Analysis**:
1. **HTTP Latency**: Need end-to-end request duration (not just processing)
2. **In-Flight**: Need concurrent request counter for capacity planning
3. **Redis Pool**: Need connection pool stats for resource monitoring

---

## 📋 **APDC Plan**

### **Phase 4.1: HTTP Request Latency Histogram** (45 min)

**Metric**: `gateway_http_request_duration_seconds`

**Business Value**: Track end-to-end HTTP request performance

**Implementation**:
```go
// pkg/gateway/metrics/metrics.go
HTTPRequestDuration *prometheus.HistogramVec // labels: method, path, status_code
```

**Middleware Integration**:
```go
// pkg/gateway/middleware/metrics.go (NEW FILE)
func HTTPMetrics(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()

            // Wrap response writer to capture status code
            ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

            next.ServeHTTP(ww, r)

            duration := time.Since(start).Seconds()
            metrics.HTTPRequestDuration.WithLabelValues(
                r.Method,
                r.URL.Path,
                strconv.Itoa(ww.Status()),
            ).Observe(duration)
        })
    }
}
```

**Success Criteria**:
- ✅ Histogram tracks request duration by method, path, status
- ✅ Middleware added to server middleware chain
- ✅ Metrics exposed via `/metrics` endpoint

---

### **Phase 4.2: In-Flight Requests Gauge** (30 min)

**Metric**: `gateway_http_requests_in_flight`

**Business Value**: Monitor concurrent request load for capacity planning

**Implementation**:
```go
// pkg/gateway/metrics/metrics.go
HTTPRequestsInFlight prometheus.Gauge
```

**Middleware Integration**:
```go
// pkg/gateway/middleware/metrics.go
func InFlightRequests(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            metrics.HTTPRequestsInFlight.Inc()
            defer metrics.HTTPRequestsInFlight.Dec()

            next.ServeHTTP(w, r)
        })
    }
}
```

**Success Criteria**:
- ✅ Gauge increments on request start
- ✅ Gauge decrements on request end (defer)
- ✅ Accurate concurrent request count

---

### **Phase 4.3: Redis Connection Pool Metrics** (45 min)

**Metrics**:
- `gateway_redis_pool_connections_total` - Total connections
- `gateway_redis_pool_connections_idle` - Idle connections
- `gateway_redis_pool_connections_active` - Active connections
- `gateway_redis_pool_hits_total` - Connection reuse hits
- `gateway_redis_pool_misses_total` - New connection creations
- `gateway_redis_pool_timeouts_total` - Connection acquisition timeouts

**Business Value**: Detect connection pool exhaustion before failures

**Implementation**:
```go
// pkg/gateway/metrics/metrics.go
RedisPoolConnectionsTotal  prometheus.Gauge
RedisPoolConnectionsIdle   prometheus.Gauge
RedisPoolConnectionsActive prometheus.Gauge
RedisPoolHitsTotal         prometheus.Counter
RedisPoolMissesTotal       prometheus.Counter
RedisPoolTimeoutsTotal     prometheus.Counter
```

**Collection Strategy**:
```go
// pkg/gateway/server/server.go
func (s *Server) collectRedisPoolMetrics() {
    // Use Redis client PoolStats() method
    stats := s.redisClient.PoolStats()

    s.metrics.RedisPoolConnectionsTotal.Set(float64(stats.TotalConns))
    s.metrics.RedisPoolConnectionsIdle.Set(float64(stats.IdleConns))
    s.metrics.RedisPoolConnectionsActive.Set(float64(stats.TotalConns - stats.IdleConns))
    s.metrics.RedisPoolHitsTotal.Add(float64(stats.Hits))
    s.metrics.RedisPoolMissesTotal.Add(float64(stats.Misses))
    s.metrics.RedisPoolTimeoutsTotal.Add(float64(stats.Timeouts))
}
```

**Periodic Collection**:
```go
// Start background goroutine to collect pool stats every 10 seconds
go func() {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            s.collectRedisPoolMetrics()
        }
    }
}()
```

**Success Criteria**:
- ✅ Pool stats collected every 10 seconds
- ✅ Metrics reflect current pool state
- ✅ Goroutine stops on server shutdown

---

## 🧪 **TDD Compliance**

### **Classification: REFACTOR Phase** ✅

**Justification**:
1. ✅ **Existing Tests**: Integration tests will verify metrics work
2. ✅ **Standard Patterns**: Using Prometheus standard middleware patterns
3. ✅ **No New Business Logic**: Just observability instrumentation
4. ✅ **Phase 6 Tests**: Dedicated metrics tests planned

**TDD Cycle**:
- ✅ **RED**: Integration tests (Phase 6) will verify metrics
- ✅ **GREEN**: Add metrics middleware (this phase)
- ✅ **REFACTOR**: Already in REFACTOR phase (enhancing observability)

---

## 📊 **Implementation Steps**

### **Step 1: Add Metrics to Centralized Struct** (15 min)

**File**: `pkg/gateway/metrics/metrics.go`

```go
type Metrics struct {
    // ... existing metrics ...

    // HTTP request metrics (Phase 4)
    HTTPRequestDuration   *prometheus.HistogramVec
    HTTPRequestsInFlight  prometheus.Gauge

    // Redis connection pool metrics (Phase 4)
    RedisPoolConnectionsTotal  prometheus.Gauge
    RedisPoolConnectionsIdle   prometheus.Gauge
    RedisPoolConnectionsActive prometheus.Gauge
    RedisPoolHitsTotal         prometheus.Counter
    RedisPoolMissesTotal       prometheus.Counter
    RedisPoolTimeoutsTotal     prometheus.Counter
}

func NewMetrics() *Metrics {
    m := &Metrics{
        // ... existing metrics ...

        // HTTP request metrics
        HTTPRequestDuration: prometheus.NewHistogramVec(
            prometheus.HistogramOpts{
                Name:    "gateway_http_request_duration_seconds",
                Help:    "HTTP request duration in seconds",
                Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
            },
            []string{"method", "path", "status_code"},
        ),

        HTTPRequestsInFlight: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_http_requests_in_flight",
            Help: "Current number of HTTP requests being processed",
        }),

        // Redis pool metrics
        RedisPoolConnectionsTotal: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_redis_pool_connections_total",
            Help: "Total number of connections in the pool",
        }),

        RedisPoolConnectionsIdle: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_redis_pool_connections_idle",
            Help: "Number of idle connections in the pool",
        }),

        RedisPoolConnectionsActive: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_redis_pool_connections_active",
            Help: "Number of active connections in the pool",
        }),

        RedisPoolHitsTotal: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "gateway_redis_pool_hits_total",
            Help: "Total number of times a connection was reused from the pool",
        }),

        RedisPoolMissesTotal: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "gateway_redis_pool_misses_total",
            Help: "Total number of times a new connection was created",
        }),

        RedisPoolTimeoutsTotal: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "gateway_redis_pool_timeouts_total",
            Help: "Total number of connection acquisition timeouts",
        }),
    }

    return m
}

func (m *Metrics) Register(registry *prometheus.Registry) error {
    // ... existing registrations ...

    // HTTP metrics
    registry.MustRegister(m.HTTPRequestDuration)
    registry.MustRegister(m.HTTPRequestsInFlight)

    // Redis pool metrics
    registry.MustRegister(m.RedisPoolConnectionsTotal)
    registry.MustRegister(m.RedisPoolConnectionsIdle)
    registry.MustRegister(m.RedisPoolConnectionsActive)
    registry.MustRegister(m.RedisPoolHitsTotal)
    registry.MustRegister(m.RedisPoolMissesTotal)
    registry.MustRegister(m.RedisPoolTimeoutsTotal)

    return nil
}
```

---

### **Step 2: Create HTTP Metrics Middleware** (30 min)

**File**: `pkg/gateway/middleware/http_metrics.go` (NEW)

```go
package middleware

import (
    "net/http"
    "strconv"
    "time"

    "github.com/go-chi/chi/v5/middleware"
    gatewayMetrics "github.com/jordigilh/kubernaut/pkg/gateway/metrics"
)

// HTTPMetrics tracks HTTP request duration
// BR-GATEWAY-071: HTTP request observability
func HTTPMetrics(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if metrics == nil {
                next.ServeHTTP(w, r)
                return
            }

            start := time.Now()
            ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

            next.ServeHTTP(ww, r)

            duration := time.Since(start).Seconds()
            metrics.HTTPRequestDuration.WithLabelValues(
                r.Method,
                r.URL.Path,
                strconv.Itoa(ww.Status()),
            ).Observe(duration)
        })
    }
}

// InFlightRequests tracks concurrent request count
// BR-GATEWAY-072: In-flight request tracking
func InFlightRequests(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if metrics == nil {
                next.ServeHTTP(w, r)
                return
            }

            metrics.HTTPRequestsInFlight.Inc()
            defer metrics.HTTPRequestsInFlight.Dec()

            next.ServeHTTP(w, r)
        })
    }
}
```

---

### **Step 3: Add Middleware to Server** (15 min)

**File**: `pkg/gateway/server/server.go`

```go
func (s *Server) Handler() http.Handler {
    r := chi.NewRouter()

    // ... existing middleware ...

    // HTTP metrics middleware (Phase 4)
    r.Use(middleware.InFlightRequests(s.metrics))
    r.Use(middleware.HTTPMetrics(s.metrics))

    // ... rest of middleware chain ...

    s.setupRoutes(r)
    return r
}
```

---

### **Step 4: Add Redis Pool Metrics Collection** (30 min)

**File**: `pkg/gateway/server/server.go`

```go
// collectRedisPoolMetrics collects Redis connection pool statistics
// BR-GATEWAY-073: Redis connection pool monitoring
func (s *Server) collectRedisPoolMetrics() {
    if s.metrics == nil || s.redisClient == nil {
        return
    }

    stats := s.redisClient.PoolStats()

    s.metrics.RedisPoolConnectionsTotal.Set(float64(stats.TotalConns))
    s.metrics.RedisPoolConnectionsIdle.Set(float64(stats.IdleConns))
    s.metrics.RedisPoolConnectionsActive.Set(float64(stats.TotalConns - stats.IdleConns))

    // Counters are cumulative, so we need to track deltas
    s.metrics.RedisPoolHitsTotal.Add(float64(stats.Hits))
    s.metrics.RedisPoolMissesTotal.Add(float64(stats.Misses))
    s.metrics.RedisPoolTimeoutsTotal.Add(float64(stats.Timeouts))
}

// Start starts the HTTP server (blocking)
func (s *Server) Start(ctx context.Context) error {
    s.httpServer.Handler = s.Handler()

    // Start Redis health monitoring (existing)
    go s.healthMonitor.Start(ctx)

    // Start Redis pool metrics collection (NEW - Phase 4)
    go s.startRedisPoolMetricsCollection(ctx)

    s.logger.Info("Starting Gateway HTTP server",
        zap.String("addr", s.httpServer.Addr))

    return s.httpServer.ListenAndServe()
}

// startRedisPoolMetricsCollection collects Redis pool stats every 10 seconds
// BR-GATEWAY-073: Redis connection pool monitoring
func (s *Server) startRedisPoolMetricsCollection(ctx context.Context) {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    s.logger.Info("Starting Redis pool metrics collection",
        zap.Duration("interval", 10*time.Second))

    for {
        select {
        case <-ctx.Done():
            s.logger.Info("Stopping Redis pool metrics collection")
            return
        case <-ticker.C:
            s.collectRedisPoolMetrics()
        }
    }
}
```

---

## ✅ **Success Criteria**

### **Functional Requirements**
- [ ] HTTP request duration histogram tracks all requests
- [ ] In-flight requests gauge increments/decrements correctly
- [ ] Redis pool metrics collected every 10 seconds
- [ ] All metrics exposed via `/metrics` endpoint
- [ ] Nil-safe implementation (no panics when metrics disabled)

### **Quality Requirements**
- [ ] Code compiles successfully
- [ ] No new lint errors
- [ ] Middleware follows existing patterns
- [ ] Goroutines stop on server shutdown

### **TDD Compliance**
- [ ] REFACTOR phase (enhancing observability)
- [ ] Integration tests planned for Phase 6
- [ ] No new business logic requiring RED-GREEN

---

## 🎯 **Confidence Assessment**

### **Implementation Confidence: 90%**

**High Confidence Factors**:
- ✅ Standard Prometheus patterns (histogram, gauge, counter)
- ✅ Chi middleware integration is straightforward
- ✅ Redis client provides `PoolStats()` method
- ✅ Similar patterns already exist in codebase

**Minor Risks** (10%):
- ⚠️ Redis pool counter deltas might need adjustment (Hits/Misses are cumulative)
- ⚠️ HTTP path cardinality explosion (need to normalize paths)
- ⚠️ Goroutine cleanup on shutdown needs verification

**Mitigation**:
- Use path patterns instead of raw paths (e.g., `/webhook/:type`)
- Test goroutine shutdown in integration tests
- Monitor Redis counter behavior in Phase 6 tests

---

## 📋 **Phase 4 Checklist**

- [ ] Add 8 new metrics to `pkg/gateway/metrics/metrics.go`
- [ ] Create `pkg/gateway/middleware/http_metrics.go`
- [ ] Add HTTP metrics middleware to server middleware chain
- [ ] Implement `collectRedisPoolMetrics()` function
- [ ] Add background goroutine for pool metrics collection
- [ ] Verify code compiles
- [ ] Verify metrics appear in `/metrics` endpoint
- [ ] Update documentation with new metrics

---

**Estimated Time**: 2 hours
**Complexity**: Medium (middleware + background collection)
**Risk**: Low-Medium (10% - path cardinality, counter deltas)
**Next Phase**: Phase 5 - Structured Logging (1h)



**Date**: 2025-10-26
**Estimated Duration**: 2 hours
**Status**: ⏳ IN PROGRESS

---

## 🎯 **APDC Analysis**

### **Business Requirements**

**BR-GATEWAY-071**: HTTP request observability for performance monitoring
**BR-GATEWAY-072**: In-flight request tracking for capacity planning
**BR-GATEWAY-073**: Redis connection pool monitoring for resource management

**Business Value**:
- **HTTP Latency**: Identify slow endpoints and optimize performance
- **In-Flight Requests**: Detect capacity issues and prevent overload
- **Redis Pool**: Monitor connection exhaustion and prevent failures

### **Current State**

**Existing Metrics** (Phase 2):
- ✅ `gateway_processing_duration_seconds` - Signal processing time
- ✅ `gateway_k8s_api_latency_seconds` - K8s API call latency
- ❌ No HTTP request latency (overall request time)
- ❌ No in-flight request tracking
- ❌ No Redis connection pool metrics

**Gap Analysis**:
1. **HTTP Latency**: Need end-to-end request duration (not just processing)
2. **In-Flight**: Need concurrent request counter for capacity planning
3. **Redis Pool**: Need connection pool stats for resource monitoring

---

## 📋 **APDC Plan**

### **Phase 4.1: HTTP Request Latency Histogram** (45 min)

**Metric**: `gateway_http_request_duration_seconds`

**Business Value**: Track end-to-end HTTP request performance

**Implementation**:
```go
// pkg/gateway/metrics/metrics.go
HTTPRequestDuration *prometheus.HistogramVec // labels: method, path, status_code
```

**Middleware Integration**:
```go
// pkg/gateway/middleware/metrics.go (NEW FILE)
func HTTPMetrics(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()

            // Wrap response writer to capture status code
            ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

            next.ServeHTTP(ww, r)

            duration := time.Since(start).Seconds()
            metrics.HTTPRequestDuration.WithLabelValues(
                r.Method,
                r.URL.Path,
                strconv.Itoa(ww.Status()),
            ).Observe(duration)
        })
    }
}
```

**Success Criteria**:
- ✅ Histogram tracks request duration by method, path, status
- ✅ Middleware added to server middleware chain
- ✅ Metrics exposed via `/metrics` endpoint

---

### **Phase 4.2: In-Flight Requests Gauge** (30 min)

**Metric**: `gateway_http_requests_in_flight`

**Business Value**: Monitor concurrent request load for capacity planning

**Implementation**:
```go
// pkg/gateway/metrics/metrics.go
HTTPRequestsInFlight prometheus.Gauge
```

**Middleware Integration**:
```go
// pkg/gateway/middleware/metrics.go
func InFlightRequests(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            metrics.HTTPRequestsInFlight.Inc()
            defer metrics.HTTPRequestsInFlight.Dec()

            next.ServeHTTP(w, r)
        })
    }
}
```

**Success Criteria**:
- ✅ Gauge increments on request start
- ✅ Gauge decrements on request end (defer)
- ✅ Accurate concurrent request count

---

### **Phase 4.3: Redis Connection Pool Metrics** (45 min)

**Metrics**:
- `gateway_redis_pool_connections_total` - Total connections
- `gateway_redis_pool_connections_idle` - Idle connections
- `gateway_redis_pool_connections_active` - Active connections
- `gateway_redis_pool_hits_total` - Connection reuse hits
- `gateway_redis_pool_misses_total` - New connection creations
- `gateway_redis_pool_timeouts_total` - Connection acquisition timeouts

**Business Value**: Detect connection pool exhaustion before failures

**Implementation**:
```go
// pkg/gateway/metrics/metrics.go
RedisPoolConnectionsTotal  prometheus.Gauge
RedisPoolConnectionsIdle   prometheus.Gauge
RedisPoolConnectionsActive prometheus.Gauge
RedisPoolHitsTotal         prometheus.Counter
RedisPoolMissesTotal       prometheus.Counter
RedisPoolTimeoutsTotal     prometheus.Counter
```

**Collection Strategy**:
```go
// pkg/gateway/server/server.go
func (s *Server) collectRedisPoolMetrics() {
    // Use Redis client PoolStats() method
    stats := s.redisClient.PoolStats()

    s.metrics.RedisPoolConnectionsTotal.Set(float64(stats.TotalConns))
    s.metrics.RedisPoolConnectionsIdle.Set(float64(stats.IdleConns))
    s.metrics.RedisPoolConnectionsActive.Set(float64(stats.TotalConns - stats.IdleConns))
    s.metrics.RedisPoolHitsTotal.Add(float64(stats.Hits))
    s.metrics.RedisPoolMissesTotal.Add(float64(stats.Misses))
    s.metrics.RedisPoolTimeoutsTotal.Add(float64(stats.Timeouts))
}
```

**Periodic Collection**:
```go
// Start background goroutine to collect pool stats every 10 seconds
go func() {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            s.collectRedisPoolMetrics()
        }
    }
}()
```

**Success Criteria**:
- ✅ Pool stats collected every 10 seconds
- ✅ Metrics reflect current pool state
- ✅ Goroutine stops on server shutdown

---

## 🧪 **TDD Compliance**

### **Classification: REFACTOR Phase** ✅

**Justification**:
1. ✅ **Existing Tests**: Integration tests will verify metrics work
2. ✅ **Standard Patterns**: Using Prometheus standard middleware patterns
3. ✅ **No New Business Logic**: Just observability instrumentation
4. ✅ **Phase 6 Tests**: Dedicated metrics tests planned

**TDD Cycle**:
- ✅ **RED**: Integration tests (Phase 6) will verify metrics
- ✅ **GREEN**: Add metrics middleware (this phase)
- ✅ **REFACTOR**: Already in REFACTOR phase (enhancing observability)

---

## 📊 **Implementation Steps**

### **Step 1: Add Metrics to Centralized Struct** (15 min)

**File**: `pkg/gateway/metrics/metrics.go`

```go
type Metrics struct {
    // ... existing metrics ...

    // HTTP request metrics (Phase 4)
    HTTPRequestDuration   *prometheus.HistogramVec
    HTTPRequestsInFlight  prometheus.Gauge

    // Redis connection pool metrics (Phase 4)
    RedisPoolConnectionsTotal  prometheus.Gauge
    RedisPoolConnectionsIdle   prometheus.Gauge
    RedisPoolConnectionsActive prometheus.Gauge
    RedisPoolHitsTotal         prometheus.Counter
    RedisPoolMissesTotal       prometheus.Counter
    RedisPoolTimeoutsTotal     prometheus.Counter
}

func NewMetrics() *Metrics {
    m := &Metrics{
        // ... existing metrics ...

        // HTTP request metrics
        HTTPRequestDuration: prometheus.NewHistogramVec(
            prometheus.HistogramOpts{
                Name:    "gateway_http_request_duration_seconds",
                Help:    "HTTP request duration in seconds",
                Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
            },
            []string{"method", "path", "status_code"},
        ),

        HTTPRequestsInFlight: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_http_requests_in_flight",
            Help: "Current number of HTTP requests being processed",
        }),

        // Redis pool metrics
        RedisPoolConnectionsTotal: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_redis_pool_connections_total",
            Help: "Total number of connections in the pool",
        }),

        RedisPoolConnectionsIdle: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_redis_pool_connections_idle",
            Help: "Number of idle connections in the pool",
        }),

        RedisPoolConnectionsActive: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_redis_pool_connections_active",
            Help: "Number of active connections in the pool",
        }),

        RedisPoolHitsTotal: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "gateway_redis_pool_hits_total",
            Help: "Total number of times a connection was reused from the pool",
        }),

        RedisPoolMissesTotal: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "gateway_redis_pool_misses_total",
            Help: "Total number of times a new connection was created",
        }),

        RedisPoolTimeoutsTotal: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "gateway_redis_pool_timeouts_total",
            Help: "Total number of connection acquisition timeouts",
        }),
    }

    return m
}

func (m *Metrics) Register(registry *prometheus.Registry) error {
    // ... existing registrations ...

    // HTTP metrics
    registry.MustRegister(m.HTTPRequestDuration)
    registry.MustRegister(m.HTTPRequestsInFlight)

    // Redis pool metrics
    registry.MustRegister(m.RedisPoolConnectionsTotal)
    registry.MustRegister(m.RedisPoolConnectionsIdle)
    registry.MustRegister(m.RedisPoolConnectionsActive)
    registry.MustRegister(m.RedisPoolHitsTotal)
    registry.MustRegister(m.RedisPoolMissesTotal)
    registry.MustRegister(m.RedisPoolTimeoutsTotal)

    return nil
}
```

---

### **Step 2: Create HTTP Metrics Middleware** (30 min)

**File**: `pkg/gateway/middleware/http_metrics.go` (NEW)

```go
package middleware

import (
    "net/http"
    "strconv"
    "time"

    "github.com/go-chi/chi/v5/middleware"
    gatewayMetrics "github.com/jordigilh/kubernaut/pkg/gateway/metrics"
)

// HTTPMetrics tracks HTTP request duration
// BR-GATEWAY-071: HTTP request observability
func HTTPMetrics(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if metrics == nil {
                next.ServeHTTP(w, r)
                return
            }

            start := time.Now()
            ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

            next.ServeHTTP(ww, r)

            duration := time.Since(start).Seconds()
            metrics.HTTPRequestDuration.WithLabelValues(
                r.Method,
                r.URL.Path,
                strconv.Itoa(ww.Status()),
            ).Observe(duration)
        })
    }
}

// InFlightRequests tracks concurrent request count
// BR-GATEWAY-072: In-flight request tracking
func InFlightRequests(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if metrics == nil {
                next.ServeHTTP(w, r)
                return
            }

            metrics.HTTPRequestsInFlight.Inc()
            defer metrics.HTTPRequestsInFlight.Dec()

            next.ServeHTTP(w, r)
        })
    }
}
```

---

### **Step 3: Add Middleware to Server** (15 min)

**File**: `pkg/gateway/server/server.go`

```go
func (s *Server) Handler() http.Handler {
    r := chi.NewRouter()

    // ... existing middleware ...

    // HTTP metrics middleware (Phase 4)
    r.Use(middleware.InFlightRequests(s.metrics))
    r.Use(middleware.HTTPMetrics(s.metrics))

    // ... rest of middleware chain ...

    s.setupRoutes(r)
    return r
}
```

---

### **Step 4: Add Redis Pool Metrics Collection** (30 min)

**File**: `pkg/gateway/server/server.go`

```go
// collectRedisPoolMetrics collects Redis connection pool statistics
// BR-GATEWAY-073: Redis connection pool monitoring
func (s *Server) collectRedisPoolMetrics() {
    if s.metrics == nil || s.redisClient == nil {
        return
    }

    stats := s.redisClient.PoolStats()

    s.metrics.RedisPoolConnectionsTotal.Set(float64(stats.TotalConns))
    s.metrics.RedisPoolConnectionsIdle.Set(float64(stats.IdleConns))
    s.metrics.RedisPoolConnectionsActive.Set(float64(stats.TotalConns - stats.IdleConns))

    // Counters are cumulative, so we need to track deltas
    s.metrics.RedisPoolHitsTotal.Add(float64(stats.Hits))
    s.metrics.RedisPoolMissesTotal.Add(float64(stats.Misses))
    s.metrics.RedisPoolTimeoutsTotal.Add(float64(stats.Timeouts))
}

// Start starts the HTTP server (blocking)
func (s *Server) Start(ctx context.Context) error {
    s.httpServer.Handler = s.Handler()

    // Start Redis health monitoring (existing)
    go s.healthMonitor.Start(ctx)

    // Start Redis pool metrics collection (NEW - Phase 4)
    go s.startRedisPoolMetricsCollection(ctx)

    s.logger.Info("Starting Gateway HTTP server",
        zap.String("addr", s.httpServer.Addr))

    return s.httpServer.ListenAndServe()
}

// startRedisPoolMetricsCollection collects Redis pool stats every 10 seconds
// BR-GATEWAY-073: Redis connection pool monitoring
func (s *Server) startRedisPoolMetricsCollection(ctx context.Context) {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    s.logger.Info("Starting Redis pool metrics collection",
        zap.Duration("interval", 10*time.Second))

    for {
        select {
        case <-ctx.Done():
            s.logger.Info("Stopping Redis pool metrics collection")
            return
        case <-ticker.C:
            s.collectRedisPoolMetrics()
        }
    }
}
```

---

## ✅ **Success Criteria**

### **Functional Requirements**
- [ ] HTTP request duration histogram tracks all requests
- [ ] In-flight requests gauge increments/decrements correctly
- [ ] Redis pool metrics collected every 10 seconds
- [ ] All metrics exposed via `/metrics` endpoint
- [ ] Nil-safe implementation (no panics when metrics disabled)

### **Quality Requirements**
- [ ] Code compiles successfully
- [ ] No new lint errors
- [ ] Middleware follows existing patterns
- [ ] Goroutines stop on server shutdown

### **TDD Compliance**
- [ ] REFACTOR phase (enhancing observability)
- [ ] Integration tests planned for Phase 6
- [ ] No new business logic requiring RED-GREEN

---

## 🎯 **Confidence Assessment**

### **Implementation Confidence: 90%**

**High Confidence Factors**:
- ✅ Standard Prometheus patterns (histogram, gauge, counter)
- ✅ Chi middleware integration is straightforward
- ✅ Redis client provides `PoolStats()` method
- ✅ Similar patterns already exist in codebase

**Minor Risks** (10%):
- ⚠️ Redis pool counter deltas might need adjustment (Hits/Misses are cumulative)
- ⚠️ HTTP path cardinality explosion (need to normalize paths)
- ⚠️ Goroutine cleanup on shutdown needs verification

**Mitigation**:
- Use path patterns instead of raw paths (e.g., `/webhook/:type`)
- Test goroutine shutdown in integration tests
- Monitor Redis counter behavior in Phase 6 tests

---

## 📋 **Phase 4 Checklist**

- [ ] Add 8 new metrics to `pkg/gateway/metrics/metrics.go`
- [ ] Create `pkg/gateway/middleware/http_metrics.go`
- [ ] Add HTTP metrics middleware to server middleware chain
- [ ] Implement `collectRedisPoolMetrics()` function
- [ ] Add background goroutine for pool metrics collection
- [ ] Verify code compiles
- [ ] Verify metrics appear in `/metrics` endpoint
- [ ] Update documentation with new metrics

---

**Estimated Time**: 2 hours
**Complexity**: Medium (middleware + background collection)
**Risk**: Low-Medium (10% - path cardinality, counter deltas)
**Next Phase**: Phase 5 - Structured Logging (1h)

# Day 9 Phase 4: Additional Metrics - APDC Plan

**Date**: 2025-10-26
**Estimated Duration**: 2 hours
**Status**: ⏳ IN PROGRESS

---

## 🎯 **APDC Analysis**

### **Business Requirements**

**BR-GATEWAY-071**: HTTP request observability for performance monitoring
**BR-GATEWAY-072**: In-flight request tracking for capacity planning
**BR-GATEWAY-073**: Redis connection pool monitoring for resource management

**Business Value**:
- **HTTP Latency**: Identify slow endpoints and optimize performance
- **In-Flight Requests**: Detect capacity issues and prevent overload
- **Redis Pool**: Monitor connection exhaustion and prevent failures

### **Current State**

**Existing Metrics** (Phase 2):
- ✅ `gateway_processing_duration_seconds` - Signal processing time
- ✅ `gateway_k8s_api_latency_seconds` - K8s API call latency
- ❌ No HTTP request latency (overall request time)
- ❌ No in-flight request tracking
- ❌ No Redis connection pool metrics

**Gap Analysis**:
1. **HTTP Latency**: Need end-to-end request duration (not just processing)
2. **In-Flight**: Need concurrent request counter for capacity planning
3. **Redis Pool**: Need connection pool stats for resource monitoring

---

## 📋 **APDC Plan**

### **Phase 4.1: HTTP Request Latency Histogram** (45 min)

**Metric**: `gateway_http_request_duration_seconds`

**Business Value**: Track end-to-end HTTP request performance

**Implementation**:
```go
// pkg/gateway/metrics/metrics.go
HTTPRequestDuration *prometheus.HistogramVec // labels: method, path, status_code
```

**Middleware Integration**:
```go
// pkg/gateway/middleware/metrics.go (NEW FILE)
func HTTPMetrics(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()

            // Wrap response writer to capture status code
            ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

            next.ServeHTTP(ww, r)

            duration := time.Since(start).Seconds()
            metrics.HTTPRequestDuration.WithLabelValues(
                r.Method,
                r.URL.Path,
                strconv.Itoa(ww.Status()),
            ).Observe(duration)
        })
    }
}
```

**Success Criteria**:
- ✅ Histogram tracks request duration by method, path, status
- ✅ Middleware added to server middleware chain
- ✅ Metrics exposed via `/metrics` endpoint

---

### **Phase 4.2: In-Flight Requests Gauge** (30 min)

**Metric**: `gateway_http_requests_in_flight`

**Business Value**: Monitor concurrent request load for capacity planning

**Implementation**:
```go
// pkg/gateway/metrics/metrics.go
HTTPRequestsInFlight prometheus.Gauge
```

**Middleware Integration**:
```go
// pkg/gateway/middleware/metrics.go
func InFlightRequests(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            metrics.HTTPRequestsInFlight.Inc()
            defer metrics.HTTPRequestsInFlight.Dec()

            next.ServeHTTP(w, r)
        })
    }
}
```

**Success Criteria**:
- ✅ Gauge increments on request start
- ✅ Gauge decrements on request end (defer)
- ✅ Accurate concurrent request count

---

### **Phase 4.3: Redis Connection Pool Metrics** (45 min)

**Metrics**:
- `gateway_redis_pool_connections_total` - Total connections
- `gateway_redis_pool_connections_idle` - Idle connections
- `gateway_redis_pool_connections_active` - Active connections
- `gateway_redis_pool_hits_total` - Connection reuse hits
- `gateway_redis_pool_misses_total` - New connection creations
- `gateway_redis_pool_timeouts_total` - Connection acquisition timeouts

**Business Value**: Detect connection pool exhaustion before failures

**Implementation**:
```go
// pkg/gateway/metrics/metrics.go
RedisPoolConnectionsTotal  prometheus.Gauge
RedisPoolConnectionsIdle   prometheus.Gauge
RedisPoolConnectionsActive prometheus.Gauge
RedisPoolHitsTotal         prometheus.Counter
RedisPoolMissesTotal       prometheus.Counter
RedisPoolTimeoutsTotal     prometheus.Counter
```

**Collection Strategy**:
```go
// pkg/gateway/server/server.go
func (s *Server) collectRedisPoolMetrics() {
    // Use Redis client PoolStats() method
    stats := s.redisClient.PoolStats()

    s.metrics.RedisPoolConnectionsTotal.Set(float64(stats.TotalConns))
    s.metrics.RedisPoolConnectionsIdle.Set(float64(stats.IdleConns))
    s.metrics.RedisPoolConnectionsActive.Set(float64(stats.TotalConns - stats.IdleConns))
    s.metrics.RedisPoolHitsTotal.Add(float64(stats.Hits))
    s.metrics.RedisPoolMissesTotal.Add(float64(stats.Misses))
    s.metrics.RedisPoolTimeoutsTotal.Add(float64(stats.Timeouts))
}
```

**Periodic Collection**:
```go
// Start background goroutine to collect pool stats every 10 seconds
go func() {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            s.collectRedisPoolMetrics()
        }
    }
}()
```

**Success Criteria**:
- ✅ Pool stats collected every 10 seconds
- ✅ Metrics reflect current pool state
- ✅ Goroutine stops on server shutdown

---

## 🧪 **TDD Compliance**

### **Classification: REFACTOR Phase** ✅

**Justification**:
1. ✅ **Existing Tests**: Integration tests will verify metrics work
2. ✅ **Standard Patterns**: Using Prometheus standard middleware patterns
3. ✅ **No New Business Logic**: Just observability instrumentation
4. ✅ **Phase 6 Tests**: Dedicated metrics tests planned

**TDD Cycle**:
- ✅ **RED**: Integration tests (Phase 6) will verify metrics
- ✅ **GREEN**: Add metrics middleware (this phase)
- ✅ **REFACTOR**: Already in REFACTOR phase (enhancing observability)

---

## 📊 **Implementation Steps**

### **Step 1: Add Metrics to Centralized Struct** (15 min)

**File**: `pkg/gateway/metrics/metrics.go`

```go
type Metrics struct {
    // ... existing metrics ...

    // HTTP request metrics (Phase 4)
    HTTPRequestDuration   *prometheus.HistogramVec
    HTTPRequestsInFlight  prometheus.Gauge

    // Redis connection pool metrics (Phase 4)
    RedisPoolConnectionsTotal  prometheus.Gauge
    RedisPoolConnectionsIdle   prometheus.Gauge
    RedisPoolConnectionsActive prometheus.Gauge
    RedisPoolHitsTotal         prometheus.Counter
    RedisPoolMissesTotal       prometheus.Counter
    RedisPoolTimeoutsTotal     prometheus.Counter
}

func NewMetrics() *Metrics {
    m := &Metrics{
        // ... existing metrics ...

        // HTTP request metrics
        HTTPRequestDuration: prometheus.NewHistogramVec(
            prometheus.HistogramOpts{
                Name:    "gateway_http_request_duration_seconds",
                Help:    "HTTP request duration in seconds",
                Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
            },
            []string{"method", "path", "status_code"},
        ),

        HTTPRequestsInFlight: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_http_requests_in_flight",
            Help: "Current number of HTTP requests being processed",
        }),

        // Redis pool metrics
        RedisPoolConnectionsTotal: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_redis_pool_connections_total",
            Help: "Total number of connections in the pool",
        }),

        RedisPoolConnectionsIdle: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_redis_pool_connections_idle",
            Help: "Number of idle connections in the pool",
        }),

        RedisPoolConnectionsActive: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_redis_pool_connections_active",
            Help: "Number of active connections in the pool",
        }),

        RedisPoolHitsTotal: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "gateway_redis_pool_hits_total",
            Help: "Total number of times a connection was reused from the pool",
        }),

        RedisPoolMissesTotal: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "gateway_redis_pool_misses_total",
            Help: "Total number of times a new connection was created",
        }),

        RedisPoolTimeoutsTotal: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "gateway_redis_pool_timeouts_total",
            Help: "Total number of connection acquisition timeouts",
        }),
    }

    return m
}

func (m *Metrics) Register(registry *prometheus.Registry) error {
    // ... existing registrations ...

    // HTTP metrics
    registry.MustRegister(m.HTTPRequestDuration)
    registry.MustRegister(m.HTTPRequestsInFlight)

    // Redis pool metrics
    registry.MustRegister(m.RedisPoolConnectionsTotal)
    registry.MustRegister(m.RedisPoolConnectionsIdle)
    registry.MustRegister(m.RedisPoolConnectionsActive)
    registry.MustRegister(m.RedisPoolHitsTotal)
    registry.MustRegister(m.RedisPoolMissesTotal)
    registry.MustRegister(m.RedisPoolTimeoutsTotal)

    return nil
}
```

---

### **Step 2: Create HTTP Metrics Middleware** (30 min)

**File**: `pkg/gateway/middleware/http_metrics.go` (NEW)

```go
package middleware

import (
    "net/http"
    "strconv"
    "time"

    "github.com/go-chi/chi/v5/middleware"
    gatewayMetrics "github.com/jordigilh/kubernaut/pkg/gateway/metrics"
)

// HTTPMetrics tracks HTTP request duration
// BR-GATEWAY-071: HTTP request observability
func HTTPMetrics(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if metrics == nil {
                next.ServeHTTP(w, r)
                return
            }

            start := time.Now()
            ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

            next.ServeHTTP(ww, r)

            duration := time.Since(start).Seconds()
            metrics.HTTPRequestDuration.WithLabelValues(
                r.Method,
                r.URL.Path,
                strconv.Itoa(ww.Status()),
            ).Observe(duration)
        })
    }
}

// InFlightRequests tracks concurrent request count
// BR-GATEWAY-072: In-flight request tracking
func InFlightRequests(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if metrics == nil {
                next.ServeHTTP(w, r)
                return
            }

            metrics.HTTPRequestsInFlight.Inc()
            defer metrics.HTTPRequestsInFlight.Dec()

            next.ServeHTTP(w, r)
        })
    }
}
```

---

### **Step 3: Add Middleware to Server** (15 min)

**File**: `pkg/gateway/server/server.go`

```go
func (s *Server) Handler() http.Handler {
    r := chi.NewRouter()

    // ... existing middleware ...

    // HTTP metrics middleware (Phase 4)
    r.Use(middleware.InFlightRequests(s.metrics))
    r.Use(middleware.HTTPMetrics(s.metrics))

    // ... rest of middleware chain ...

    s.setupRoutes(r)
    return r
}
```

---

### **Step 4: Add Redis Pool Metrics Collection** (30 min)

**File**: `pkg/gateway/server/server.go`

```go
// collectRedisPoolMetrics collects Redis connection pool statistics
// BR-GATEWAY-073: Redis connection pool monitoring
func (s *Server) collectRedisPoolMetrics() {
    if s.metrics == nil || s.redisClient == nil {
        return
    }

    stats := s.redisClient.PoolStats()

    s.metrics.RedisPoolConnectionsTotal.Set(float64(stats.TotalConns))
    s.metrics.RedisPoolConnectionsIdle.Set(float64(stats.IdleConns))
    s.metrics.RedisPoolConnectionsActive.Set(float64(stats.TotalConns - stats.IdleConns))

    // Counters are cumulative, so we need to track deltas
    s.metrics.RedisPoolHitsTotal.Add(float64(stats.Hits))
    s.metrics.RedisPoolMissesTotal.Add(float64(stats.Misses))
    s.metrics.RedisPoolTimeoutsTotal.Add(float64(stats.Timeouts))
}

// Start starts the HTTP server (blocking)
func (s *Server) Start(ctx context.Context) error {
    s.httpServer.Handler = s.Handler()

    // Start Redis health monitoring (existing)
    go s.healthMonitor.Start(ctx)

    // Start Redis pool metrics collection (NEW - Phase 4)
    go s.startRedisPoolMetricsCollection(ctx)

    s.logger.Info("Starting Gateway HTTP server",
        zap.String("addr", s.httpServer.Addr))

    return s.httpServer.ListenAndServe()
}

// startRedisPoolMetricsCollection collects Redis pool stats every 10 seconds
// BR-GATEWAY-073: Redis connection pool monitoring
func (s *Server) startRedisPoolMetricsCollection(ctx context.Context) {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    s.logger.Info("Starting Redis pool metrics collection",
        zap.Duration("interval", 10*time.Second))

    for {
        select {
        case <-ctx.Done():
            s.logger.Info("Stopping Redis pool metrics collection")
            return
        case <-ticker.C:
            s.collectRedisPoolMetrics()
        }
    }
}
```

---

## ✅ **Success Criteria**

### **Functional Requirements**
- [ ] HTTP request duration histogram tracks all requests
- [ ] In-flight requests gauge increments/decrements correctly
- [ ] Redis pool metrics collected every 10 seconds
- [ ] All metrics exposed via `/metrics` endpoint
- [ ] Nil-safe implementation (no panics when metrics disabled)

### **Quality Requirements**
- [ ] Code compiles successfully
- [ ] No new lint errors
- [ ] Middleware follows existing patterns
- [ ] Goroutines stop on server shutdown

### **TDD Compliance**
- [ ] REFACTOR phase (enhancing observability)
- [ ] Integration tests planned for Phase 6
- [ ] No new business logic requiring RED-GREEN

---

## 🎯 **Confidence Assessment**

### **Implementation Confidence: 90%**

**High Confidence Factors**:
- ✅ Standard Prometheus patterns (histogram, gauge, counter)
- ✅ Chi middleware integration is straightforward
- ✅ Redis client provides `PoolStats()` method
- ✅ Similar patterns already exist in codebase

**Minor Risks** (10%):
- ⚠️ Redis pool counter deltas might need adjustment (Hits/Misses are cumulative)
- ⚠️ HTTP path cardinality explosion (need to normalize paths)
- ⚠️ Goroutine cleanup on shutdown needs verification

**Mitigation**:
- Use path patterns instead of raw paths (e.g., `/webhook/:type`)
- Test goroutine shutdown in integration tests
- Monitor Redis counter behavior in Phase 6 tests

---

## 📋 **Phase 4 Checklist**

- [ ] Add 8 new metrics to `pkg/gateway/metrics/metrics.go`
- [ ] Create `pkg/gateway/middleware/http_metrics.go`
- [ ] Add HTTP metrics middleware to server middleware chain
- [ ] Implement `collectRedisPoolMetrics()` function
- [ ] Add background goroutine for pool metrics collection
- [ ] Verify code compiles
- [ ] Verify metrics appear in `/metrics` endpoint
- [ ] Update documentation with new metrics

---

**Estimated Time**: 2 hours
**Complexity**: Medium (middleware + background collection)
**Risk**: Low-Medium (10% - path cardinality, counter deltas)
**Next Phase**: Phase 5 - Structured Logging (1h)

# Day 9 Phase 4: Additional Metrics - APDC Plan

**Date**: 2025-10-26
**Estimated Duration**: 2 hours
**Status**: ⏳ IN PROGRESS

---

## 🎯 **APDC Analysis**

### **Business Requirements**

**BR-GATEWAY-071**: HTTP request observability for performance monitoring
**BR-GATEWAY-072**: In-flight request tracking for capacity planning
**BR-GATEWAY-073**: Redis connection pool monitoring for resource management

**Business Value**:
- **HTTP Latency**: Identify slow endpoints and optimize performance
- **In-Flight Requests**: Detect capacity issues and prevent overload
- **Redis Pool**: Monitor connection exhaustion and prevent failures

### **Current State**

**Existing Metrics** (Phase 2):
- ✅ `gateway_processing_duration_seconds` - Signal processing time
- ✅ `gateway_k8s_api_latency_seconds` - K8s API call latency
- ❌ No HTTP request latency (overall request time)
- ❌ No in-flight request tracking
- ❌ No Redis connection pool metrics

**Gap Analysis**:
1. **HTTP Latency**: Need end-to-end request duration (not just processing)
2. **In-Flight**: Need concurrent request counter for capacity planning
3. **Redis Pool**: Need connection pool stats for resource monitoring

---

## 📋 **APDC Plan**

### **Phase 4.1: HTTP Request Latency Histogram** (45 min)

**Metric**: `gateway_http_request_duration_seconds`

**Business Value**: Track end-to-end HTTP request performance

**Implementation**:
```go
// pkg/gateway/metrics/metrics.go
HTTPRequestDuration *prometheus.HistogramVec // labels: method, path, status_code
```

**Middleware Integration**:
```go
// pkg/gateway/middleware/metrics.go (NEW FILE)
func HTTPMetrics(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()

            // Wrap response writer to capture status code
            ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

            next.ServeHTTP(ww, r)

            duration := time.Since(start).Seconds()
            metrics.HTTPRequestDuration.WithLabelValues(
                r.Method,
                r.URL.Path,
                strconv.Itoa(ww.Status()),
            ).Observe(duration)
        })
    }
}
```

**Success Criteria**:
- ✅ Histogram tracks request duration by method, path, status
- ✅ Middleware added to server middleware chain
- ✅ Metrics exposed via `/metrics` endpoint

---

### **Phase 4.2: In-Flight Requests Gauge** (30 min)

**Metric**: `gateway_http_requests_in_flight`

**Business Value**: Monitor concurrent request load for capacity planning

**Implementation**:
```go
// pkg/gateway/metrics/metrics.go
HTTPRequestsInFlight prometheus.Gauge
```

**Middleware Integration**:
```go
// pkg/gateway/middleware/metrics.go
func InFlightRequests(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            metrics.HTTPRequestsInFlight.Inc()
            defer metrics.HTTPRequestsInFlight.Dec()

            next.ServeHTTP(w, r)
        })
    }
}
```

**Success Criteria**:
- ✅ Gauge increments on request start
- ✅ Gauge decrements on request end (defer)
- ✅ Accurate concurrent request count

---

### **Phase 4.3: Redis Connection Pool Metrics** (45 min)

**Metrics**:
- `gateway_redis_pool_connections_total` - Total connections
- `gateway_redis_pool_connections_idle` - Idle connections
- `gateway_redis_pool_connections_active` - Active connections
- `gateway_redis_pool_hits_total` - Connection reuse hits
- `gateway_redis_pool_misses_total` - New connection creations
- `gateway_redis_pool_timeouts_total` - Connection acquisition timeouts

**Business Value**: Detect connection pool exhaustion before failures

**Implementation**:
```go
// pkg/gateway/metrics/metrics.go
RedisPoolConnectionsTotal  prometheus.Gauge
RedisPoolConnectionsIdle   prometheus.Gauge
RedisPoolConnectionsActive prometheus.Gauge
RedisPoolHitsTotal         prometheus.Counter
RedisPoolMissesTotal       prometheus.Counter
RedisPoolTimeoutsTotal     prometheus.Counter
```

**Collection Strategy**:
```go
// pkg/gateway/server/server.go
func (s *Server) collectRedisPoolMetrics() {
    // Use Redis client PoolStats() method
    stats := s.redisClient.PoolStats()

    s.metrics.RedisPoolConnectionsTotal.Set(float64(stats.TotalConns))
    s.metrics.RedisPoolConnectionsIdle.Set(float64(stats.IdleConns))
    s.metrics.RedisPoolConnectionsActive.Set(float64(stats.TotalConns - stats.IdleConns))
    s.metrics.RedisPoolHitsTotal.Add(float64(stats.Hits))
    s.metrics.RedisPoolMissesTotal.Add(float64(stats.Misses))
    s.metrics.RedisPoolTimeoutsTotal.Add(float64(stats.Timeouts))
}
```

**Periodic Collection**:
```go
// Start background goroutine to collect pool stats every 10 seconds
go func() {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            s.collectRedisPoolMetrics()
        }
    }
}()
```

**Success Criteria**:
- ✅ Pool stats collected every 10 seconds
- ✅ Metrics reflect current pool state
- ✅ Goroutine stops on server shutdown

---

## 🧪 **TDD Compliance**

### **Classification: REFACTOR Phase** ✅

**Justification**:
1. ✅ **Existing Tests**: Integration tests will verify metrics work
2. ✅ **Standard Patterns**: Using Prometheus standard middleware patterns
3. ✅ **No New Business Logic**: Just observability instrumentation
4. ✅ **Phase 6 Tests**: Dedicated metrics tests planned

**TDD Cycle**:
- ✅ **RED**: Integration tests (Phase 6) will verify metrics
- ✅ **GREEN**: Add metrics middleware (this phase)
- ✅ **REFACTOR**: Already in REFACTOR phase (enhancing observability)

---

## 📊 **Implementation Steps**

### **Step 1: Add Metrics to Centralized Struct** (15 min)

**File**: `pkg/gateway/metrics/metrics.go`

```go
type Metrics struct {
    // ... existing metrics ...

    // HTTP request metrics (Phase 4)
    HTTPRequestDuration   *prometheus.HistogramVec
    HTTPRequestsInFlight  prometheus.Gauge

    // Redis connection pool metrics (Phase 4)
    RedisPoolConnectionsTotal  prometheus.Gauge
    RedisPoolConnectionsIdle   prometheus.Gauge
    RedisPoolConnectionsActive prometheus.Gauge
    RedisPoolHitsTotal         prometheus.Counter
    RedisPoolMissesTotal       prometheus.Counter
    RedisPoolTimeoutsTotal     prometheus.Counter
}

func NewMetrics() *Metrics {
    m := &Metrics{
        // ... existing metrics ...

        // HTTP request metrics
        HTTPRequestDuration: prometheus.NewHistogramVec(
            prometheus.HistogramOpts{
                Name:    "gateway_http_request_duration_seconds",
                Help:    "HTTP request duration in seconds",
                Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
            },
            []string{"method", "path", "status_code"},
        ),

        HTTPRequestsInFlight: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_http_requests_in_flight",
            Help: "Current number of HTTP requests being processed",
        }),

        // Redis pool metrics
        RedisPoolConnectionsTotal: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_redis_pool_connections_total",
            Help: "Total number of connections in the pool",
        }),

        RedisPoolConnectionsIdle: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_redis_pool_connections_idle",
            Help: "Number of idle connections in the pool",
        }),

        RedisPoolConnectionsActive: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_redis_pool_connections_active",
            Help: "Number of active connections in the pool",
        }),

        RedisPoolHitsTotal: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "gateway_redis_pool_hits_total",
            Help: "Total number of times a connection was reused from the pool",
        }),

        RedisPoolMissesTotal: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "gateway_redis_pool_misses_total",
            Help: "Total number of times a new connection was created",
        }),

        RedisPoolTimeoutsTotal: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "gateway_redis_pool_timeouts_total",
            Help: "Total number of connection acquisition timeouts",
        }),
    }

    return m
}

func (m *Metrics) Register(registry *prometheus.Registry) error {
    // ... existing registrations ...

    // HTTP metrics
    registry.MustRegister(m.HTTPRequestDuration)
    registry.MustRegister(m.HTTPRequestsInFlight)

    // Redis pool metrics
    registry.MustRegister(m.RedisPoolConnectionsTotal)
    registry.MustRegister(m.RedisPoolConnectionsIdle)
    registry.MustRegister(m.RedisPoolConnectionsActive)
    registry.MustRegister(m.RedisPoolHitsTotal)
    registry.MustRegister(m.RedisPoolMissesTotal)
    registry.MustRegister(m.RedisPoolTimeoutsTotal)

    return nil
}
```

---

### **Step 2: Create HTTP Metrics Middleware** (30 min)

**File**: `pkg/gateway/middleware/http_metrics.go` (NEW)

```go
package middleware

import (
    "net/http"
    "strconv"
    "time"

    "github.com/go-chi/chi/v5/middleware"
    gatewayMetrics "github.com/jordigilh/kubernaut/pkg/gateway/metrics"
)

// HTTPMetrics tracks HTTP request duration
// BR-GATEWAY-071: HTTP request observability
func HTTPMetrics(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if metrics == nil {
                next.ServeHTTP(w, r)
                return
            }

            start := time.Now()
            ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

            next.ServeHTTP(ww, r)

            duration := time.Since(start).Seconds()
            metrics.HTTPRequestDuration.WithLabelValues(
                r.Method,
                r.URL.Path,
                strconv.Itoa(ww.Status()),
            ).Observe(duration)
        })
    }
}

// InFlightRequests tracks concurrent request count
// BR-GATEWAY-072: In-flight request tracking
func InFlightRequests(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if metrics == nil {
                next.ServeHTTP(w, r)
                return
            }

            metrics.HTTPRequestsInFlight.Inc()
            defer metrics.HTTPRequestsInFlight.Dec()

            next.ServeHTTP(w, r)
        })
    }
}
```

---

### **Step 3: Add Middleware to Server** (15 min)

**File**: `pkg/gateway/server/server.go`

```go
func (s *Server) Handler() http.Handler {
    r := chi.NewRouter()

    // ... existing middleware ...

    // HTTP metrics middleware (Phase 4)
    r.Use(middleware.InFlightRequests(s.metrics))
    r.Use(middleware.HTTPMetrics(s.metrics))

    // ... rest of middleware chain ...

    s.setupRoutes(r)
    return r
}
```

---

### **Step 4: Add Redis Pool Metrics Collection** (30 min)

**File**: `pkg/gateway/server/server.go`

```go
// collectRedisPoolMetrics collects Redis connection pool statistics
// BR-GATEWAY-073: Redis connection pool monitoring
func (s *Server) collectRedisPoolMetrics() {
    if s.metrics == nil || s.redisClient == nil {
        return
    }

    stats := s.redisClient.PoolStats()

    s.metrics.RedisPoolConnectionsTotal.Set(float64(stats.TotalConns))
    s.metrics.RedisPoolConnectionsIdle.Set(float64(stats.IdleConns))
    s.metrics.RedisPoolConnectionsActive.Set(float64(stats.TotalConns - stats.IdleConns))

    // Counters are cumulative, so we need to track deltas
    s.metrics.RedisPoolHitsTotal.Add(float64(stats.Hits))
    s.metrics.RedisPoolMissesTotal.Add(float64(stats.Misses))
    s.metrics.RedisPoolTimeoutsTotal.Add(float64(stats.Timeouts))
}

// Start starts the HTTP server (blocking)
func (s *Server) Start(ctx context.Context) error {
    s.httpServer.Handler = s.Handler()

    // Start Redis health monitoring (existing)
    go s.healthMonitor.Start(ctx)

    // Start Redis pool metrics collection (NEW - Phase 4)
    go s.startRedisPoolMetricsCollection(ctx)

    s.logger.Info("Starting Gateway HTTP server",
        zap.String("addr", s.httpServer.Addr))

    return s.httpServer.ListenAndServe()
}

// startRedisPoolMetricsCollection collects Redis pool stats every 10 seconds
// BR-GATEWAY-073: Redis connection pool monitoring
func (s *Server) startRedisPoolMetricsCollection(ctx context.Context) {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    s.logger.Info("Starting Redis pool metrics collection",
        zap.Duration("interval", 10*time.Second))

    for {
        select {
        case <-ctx.Done():
            s.logger.Info("Stopping Redis pool metrics collection")
            return
        case <-ticker.C:
            s.collectRedisPoolMetrics()
        }
    }
}
```

---

## ✅ **Success Criteria**

### **Functional Requirements**
- [ ] HTTP request duration histogram tracks all requests
- [ ] In-flight requests gauge increments/decrements correctly
- [ ] Redis pool metrics collected every 10 seconds
- [ ] All metrics exposed via `/metrics` endpoint
- [ ] Nil-safe implementation (no panics when metrics disabled)

### **Quality Requirements**
- [ ] Code compiles successfully
- [ ] No new lint errors
- [ ] Middleware follows existing patterns
- [ ] Goroutines stop on server shutdown

### **TDD Compliance**
- [ ] REFACTOR phase (enhancing observability)
- [ ] Integration tests planned for Phase 6
- [ ] No new business logic requiring RED-GREEN

---

## 🎯 **Confidence Assessment**

### **Implementation Confidence: 90%**

**High Confidence Factors**:
- ✅ Standard Prometheus patterns (histogram, gauge, counter)
- ✅ Chi middleware integration is straightforward
- ✅ Redis client provides `PoolStats()` method
- ✅ Similar patterns already exist in codebase

**Minor Risks** (10%):
- ⚠️ Redis pool counter deltas might need adjustment (Hits/Misses are cumulative)
- ⚠️ HTTP path cardinality explosion (need to normalize paths)
- ⚠️ Goroutine cleanup on shutdown needs verification

**Mitigation**:
- Use path patterns instead of raw paths (e.g., `/webhook/:type`)
- Test goroutine shutdown in integration tests
- Monitor Redis counter behavior in Phase 6 tests

---

## 📋 **Phase 4 Checklist**

- [ ] Add 8 new metrics to `pkg/gateway/metrics/metrics.go`
- [ ] Create `pkg/gateway/middleware/http_metrics.go`
- [ ] Add HTTP metrics middleware to server middleware chain
- [ ] Implement `collectRedisPoolMetrics()` function
- [ ] Add background goroutine for pool metrics collection
- [ ] Verify code compiles
- [ ] Verify metrics appear in `/metrics` endpoint
- [ ] Update documentation with new metrics

---

**Estimated Time**: 2 hours
**Complexity**: Medium (middleware + background collection)
**Risk**: Low-Medium (10% - path cardinality, counter deltas)
**Next Phase**: Phase 5 - Structured Logging (1h)



**Date**: 2025-10-26
**Estimated Duration**: 2 hours
**Status**: ⏳ IN PROGRESS

---

## 🎯 **APDC Analysis**

### **Business Requirements**

**BR-GATEWAY-071**: HTTP request observability for performance monitoring
**BR-GATEWAY-072**: In-flight request tracking for capacity planning
**BR-GATEWAY-073**: Redis connection pool monitoring for resource management

**Business Value**:
- **HTTP Latency**: Identify slow endpoints and optimize performance
- **In-Flight Requests**: Detect capacity issues and prevent overload
- **Redis Pool**: Monitor connection exhaustion and prevent failures

### **Current State**

**Existing Metrics** (Phase 2):
- ✅ `gateway_processing_duration_seconds` - Signal processing time
- ✅ `gateway_k8s_api_latency_seconds` - K8s API call latency
- ❌ No HTTP request latency (overall request time)
- ❌ No in-flight request tracking
- ❌ No Redis connection pool metrics

**Gap Analysis**:
1. **HTTP Latency**: Need end-to-end request duration (not just processing)
2. **In-Flight**: Need concurrent request counter for capacity planning
3. **Redis Pool**: Need connection pool stats for resource monitoring

---

## 📋 **APDC Plan**

### **Phase 4.1: HTTP Request Latency Histogram** (45 min)

**Metric**: `gateway_http_request_duration_seconds`

**Business Value**: Track end-to-end HTTP request performance

**Implementation**:
```go
// pkg/gateway/metrics/metrics.go
HTTPRequestDuration *prometheus.HistogramVec // labels: method, path, status_code
```

**Middleware Integration**:
```go
// pkg/gateway/middleware/metrics.go (NEW FILE)
func HTTPMetrics(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()

            // Wrap response writer to capture status code
            ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

            next.ServeHTTP(ww, r)

            duration := time.Since(start).Seconds()
            metrics.HTTPRequestDuration.WithLabelValues(
                r.Method,
                r.URL.Path,
                strconv.Itoa(ww.Status()),
            ).Observe(duration)
        })
    }
}
```

**Success Criteria**:
- ✅ Histogram tracks request duration by method, path, status
- ✅ Middleware added to server middleware chain
- ✅ Metrics exposed via `/metrics` endpoint

---

### **Phase 4.2: In-Flight Requests Gauge** (30 min)

**Metric**: `gateway_http_requests_in_flight`

**Business Value**: Monitor concurrent request load for capacity planning

**Implementation**:
```go
// pkg/gateway/metrics/metrics.go
HTTPRequestsInFlight prometheus.Gauge
```

**Middleware Integration**:
```go
// pkg/gateway/middleware/metrics.go
func InFlightRequests(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            metrics.HTTPRequestsInFlight.Inc()
            defer metrics.HTTPRequestsInFlight.Dec()

            next.ServeHTTP(w, r)
        })
    }
}
```

**Success Criteria**:
- ✅ Gauge increments on request start
- ✅ Gauge decrements on request end (defer)
- ✅ Accurate concurrent request count

---

### **Phase 4.3: Redis Connection Pool Metrics** (45 min)

**Metrics**:
- `gateway_redis_pool_connections_total` - Total connections
- `gateway_redis_pool_connections_idle` - Idle connections
- `gateway_redis_pool_connections_active` - Active connections
- `gateway_redis_pool_hits_total` - Connection reuse hits
- `gateway_redis_pool_misses_total` - New connection creations
- `gateway_redis_pool_timeouts_total` - Connection acquisition timeouts

**Business Value**: Detect connection pool exhaustion before failures

**Implementation**:
```go
// pkg/gateway/metrics/metrics.go
RedisPoolConnectionsTotal  prometheus.Gauge
RedisPoolConnectionsIdle   prometheus.Gauge
RedisPoolConnectionsActive prometheus.Gauge
RedisPoolHitsTotal         prometheus.Counter
RedisPoolMissesTotal       prometheus.Counter
RedisPoolTimeoutsTotal     prometheus.Counter
```

**Collection Strategy**:
```go
// pkg/gateway/server/server.go
func (s *Server) collectRedisPoolMetrics() {
    // Use Redis client PoolStats() method
    stats := s.redisClient.PoolStats()

    s.metrics.RedisPoolConnectionsTotal.Set(float64(stats.TotalConns))
    s.metrics.RedisPoolConnectionsIdle.Set(float64(stats.IdleConns))
    s.metrics.RedisPoolConnectionsActive.Set(float64(stats.TotalConns - stats.IdleConns))
    s.metrics.RedisPoolHitsTotal.Add(float64(stats.Hits))
    s.metrics.RedisPoolMissesTotal.Add(float64(stats.Misses))
    s.metrics.RedisPoolTimeoutsTotal.Add(float64(stats.Timeouts))
}
```

**Periodic Collection**:
```go
// Start background goroutine to collect pool stats every 10 seconds
go func() {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            s.collectRedisPoolMetrics()
        }
    }
}()
```

**Success Criteria**:
- ✅ Pool stats collected every 10 seconds
- ✅ Metrics reflect current pool state
- ✅ Goroutine stops on server shutdown

---

## 🧪 **TDD Compliance**

### **Classification: REFACTOR Phase** ✅

**Justification**:
1. ✅ **Existing Tests**: Integration tests will verify metrics work
2. ✅ **Standard Patterns**: Using Prometheus standard middleware patterns
3. ✅ **No New Business Logic**: Just observability instrumentation
4. ✅ **Phase 6 Tests**: Dedicated metrics tests planned

**TDD Cycle**:
- ✅ **RED**: Integration tests (Phase 6) will verify metrics
- ✅ **GREEN**: Add metrics middleware (this phase)
- ✅ **REFACTOR**: Already in REFACTOR phase (enhancing observability)

---

## 📊 **Implementation Steps**

### **Step 1: Add Metrics to Centralized Struct** (15 min)

**File**: `pkg/gateway/metrics/metrics.go`

```go
type Metrics struct {
    // ... existing metrics ...

    // HTTP request metrics (Phase 4)
    HTTPRequestDuration   *prometheus.HistogramVec
    HTTPRequestsInFlight  prometheus.Gauge

    // Redis connection pool metrics (Phase 4)
    RedisPoolConnectionsTotal  prometheus.Gauge
    RedisPoolConnectionsIdle   prometheus.Gauge
    RedisPoolConnectionsActive prometheus.Gauge
    RedisPoolHitsTotal         prometheus.Counter
    RedisPoolMissesTotal       prometheus.Counter
    RedisPoolTimeoutsTotal     prometheus.Counter
}

func NewMetrics() *Metrics {
    m := &Metrics{
        // ... existing metrics ...

        // HTTP request metrics
        HTTPRequestDuration: prometheus.NewHistogramVec(
            prometheus.HistogramOpts{
                Name:    "gateway_http_request_duration_seconds",
                Help:    "HTTP request duration in seconds",
                Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
            },
            []string{"method", "path", "status_code"},
        ),

        HTTPRequestsInFlight: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_http_requests_in_flight",
            Help: "Current number of HTTP requests being processed",
        }),

        // Redis pool metrics
        RedisPoolConnectionsTotal: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_redis_pool_connections_total",
            Help: "Total number of connections in the pool",
        }),

        RedisPoolConnectionsIdle: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_redis_pool_connections_idle",
            Help: "Number of idle connections in the pool",
        }),

        RedisPoolConnectionsActive: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_redis_pool_connections_active",
            Help: "Number of active connections in the pool",
        }),

        RedisPoolHitsTotal: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "gateway_redis_pool_hits_total",
            Help: "Total number of times a connection was reused from the pool",
        }),

        RedisPoolMissesTotal: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "gateway_redis_pool_misses_total",
            Help: "Total number of times a new connection was created",
        }),

        RedisPoolTimeoutsTotal: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "gateway_redis_pool_timeouts_total",
            Help: "Total number of connection acquisition timeouts",
        }),
    }

    return m
}

func (m *Metrics) Register(registry *prometheus.Registry) error {
    // ... existing registrations ...

    // HTTP metrics
    registry.MustRegister(m.HTTPRequestDuration)
    registry.MustRegister(m.HTTPRequestsInFlight)

    // Redis pool metrics
    registry.MustRegister(m.RedisPoolConnectionsTotal)
    registry.MustRegister(m.RedisPoolConnectionsIdle)
    registry.MustRegister(m.RedisPoolConnectionsActive)
    registry.MustRegister(m.RedisPoolHitsTotal)
    registry.MustRegister(m.RedisPoolMissesTotal)
    registry.MustRegister(m.RedisPoolTimeoutsTotal)

    return nil
}
```

---

### **Step 2: Create HTTP Metrics Middleware** (30 min)

**File**: `pkg/gateway/middleware/http_metrics.go` (NEW)

```go
package middleware

import (
    "net/http"
    "strconv"
    "time"

    "github.com/go-chi/chi/v5/middleware"
    gatewayMetrics "github.com/jordigilh/kubernaut/pkg/gateway/metrics"
)

// HTTPMetrics tracks HTTP request duration
// BR-GATEWAY-071: HTTP request observability
func HTTPMetrics(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if metrics == nil {
                next.ServeHTTP(w, r)
                return
            }

            start := time.Now()
            ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

            next.ServeHTTP(ww, r)

            duration := time.Since(start).Seconds()
            metrics.HTTPRequestDuration.WithLabelValues(
                r.Method,
                r.URL.Path,
                strconv.Itoa(ww.Status()),
            ).Observe(duration)
        })
    }
}

// InFlightRequests tracks concurrent request count
// BR-GATEWAY-072: In-flight request tracking
func InFlightRequests(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if metrics == nil {
                next.ServeHTTP(w, r)
                return
            }

            metrics.HTTPRequestsInFlight.Inc()
            defer metrics.HTTPRequestsInFlight.Dec()

            next.ServeHTTP(w, r)
        })
    }
}
```

---

### **Step 3: Add Middleware to Server** (15 min)

**File**: `pkg/gateway/server/server.go`

```go
func (s *Server) Handler() http.Handler {
    r := chi.NewRouter()

    // ... existing middleware ...

    // HTTP metrics middleware (Phase 4)
    r.Use(middleware.InFlightRequests(s.metrics))
    r.Use(middleware.HTTPMetrics(s.metrics))

    // ... rest of middleware chain ...

    s.setupRoutes(r)
    return r
}
```

---

### **Step 4: Add Redis Pool Metrics Collection** (30 min)

**File**: `pkg/gateway/server/server.go`

```go
// collectRedisPoolMetrics collects Redis connection pool statistics
// BR-GATEWAY-073: Redis connection pool monitoring
func (s *Server) collectRedisPoolMetrics() {
    if s.metrics == nil || s.redisClient == nil {
        return
    }

    stats := s.redisClient.PoolStats()

    s.metrics.RedisPoolConnectionsTotal.Set(float64(stats.TotalConns))
    s.metrics.RedisPoolConnectionsIdle.Set(float64(stats.IdleConns))
    s.metrics.RedisPoolConnectionsActive.Set(float64(stats.TotalConns - stats.IdleConns))

    // Counters are cumulative, so we need to track deltas
    s.metrics.RedisPoolHitsTotal.Add(float64(stats.Hits))
    s.metrics.RedisPoolMissesTotal.Add(float64(stats.Misses))
    s.metrics.RedisPoolTimeoutsTotal.Add(float64(stats.Timeouts))
}

// Start starts the HTTP server (blocking)
func (s *Server) Start(ctx context.Context) error {
    s.httpServer.Handler = s.Handler()

    // Start Redis health monitoring (existing)
    go s.healthMonitor.Start(ctx)

    // Start Redis pool metrics collection (NEW - Phase 4)
    go s.startRedisPoolMetricsCollection(ctx)

    s.logger.Info("Starting Gateway HTTP server",
        zap.String("addr", s.httpServer.Addr))

    return s.httpServer.ListenAndServe()
}

// startRedisPoolMetricsCollection collects Redis pool stats every 10 seconds
// BR-GATEWAY-073: Redis connection pool monitoring
func (s *Server) startRedisPoolMetricsCollection(ctx context.Context) {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    s.logger.Info("Starting Redis pool metrics collection",
        zap.Duration("interval", 10*time.Second))

    for {
        select {
        case <-ctx.Done():
            s.logger.Info("Stopping Redis pool metrics collection")
            return
        case <-ticker.C:
            s.collectRedisPoolMetrics()
        }
    }
}
```

---

## ✅ **Success Criteria**

### **Functional Requirements**
- [ ] HTTP request duration histogram tracks all requests
- [ ] In-flight requests gauge increments/decrements correctly
- [ ] Redis pool metrics collected every 10 seconds
- [ ] All metrics exposed via `/metrics` endpoint
- [ ] Nil-safe implementation (no panics when metrics disabled)

### **Quality Requirements**
- [ ] Code compiles successfully
- [ ] No new lint errors
- [ ] Middleware follows existing patterns
- [ ] Goroutines stop on server shutdown

### **TDD Compliance**
- [ ] REFACTOR phase (enhancing observability)
- [ ] Integration tests planned for Phase 6
- [ ] No new business logic requiring RED-GREEN

---

## 🎯 **Confidence Assessment**

### **Implementation Confidence: 90%**

**High Confidence Factors**:
- ✅ Standard Prometheus patterns (histogram, gauge, counter)
- ✅ Chi middleware integration is straightforward
- ✅ Redis client provides `PoolStats()` method
- ✅ Similar patterns already exist in codebase

**Minor Risks** (10%):
- ⚠️ Redis pool counter deltas might need adjustment (Hits/Misses are cumulative)
- ⚠️ HTTP path cardinality explosion (need to normalize paths)
- ⚠️ Goroutine cleanup on shutdown needs verification

**Mitigation**:
- Use path patterns instead of raw paths (e.g., `/webhook/:type`)
- Test goroutine shutdown in integration tests
- Monitor Redis counter behavior in Phase 6 tests

---

## 📋 **Phase 4 Checklist**

- [ ] Add 8 new metrics to `pkg/gateway/metrics/metrics.go`
- [ ] Create `pkg/gateway/middleware/http_metrics.go`
- [ ] Add HTTP metrics middleware to server middleware chain
- [ ] Implement `collectRedisPoolMetrics()` function
- [ ] Add background goroutine for pool metrics collection
- [ ] Verify code compiles
- [ ] Verify metrics appear in `/metrics` endpoint
- [ ] Update documentation with new metrics

---

**Estimated Time**: 2 hours
**Complexity**: Medium (middleware + background collection)
**Risk**: Low-Medium (10% - path cardinality, counter deltas)
**Next Phase**: Phase 5 - Structured Logging (1h)

# Day 9 Phase 4: Additional Metrics - APDC Plan

**Date**: 2025-10-26
**Estimated Duration**: 2 hours
**Status**: ⏳ IN PROGRESS

---

## 🎯 **APDC Analysis**

### **Business Requirements**

**BR-GATEWAY-071**: HTTP request observability for performance monitoring
**BR-GATEWAY-072**: In-flight request tracking for capacity planning
**BR-GATEWAY-073**: Redis connection pool monitoring for resource management

**Business Value**:
- **HTTP Latency**: Identify slow endpoints and optimize performance
- **In-Flight Requests**: Detect capacity issues and prevent overload
- **Redis Pool**: Monitor connection exhaustion and prevent failures

### **Current State**

**Existing Metrics** (Phase 2):
- ✅ `gateway_processing_duration_seconds` - Signal processing time
- ✅ `gateway_k8s_api_latency_seconds` - K8s API call latency
- ❌ No HTTP request latency (overall request time)
- ❌ No in-flight request tracking
- ❌ No Redis connection pool metrics

**Gap Analysis**:
1. **HTTP Latency**: Need end-to-end request duration (not just processing)
2. **In-Flight**: Need concurrent request counter for capacity planning
3. **Redis Pool**: Need connection pool stats for resource monitoring

---

## 📋 **APDC Plan**

### **Phase 4.1: HTTP Request Latency Histogram** (45 min)

**Metric**: `gateway_http_request_duration_seconds`

**Business Value**: Track end-to-end HTTP request performance

**Implementation**:
```go
// pkg/gateway/metrics/metrics.go
HTTPRequestDuration *prometheus.HistogramVec // labels: method, path, status_code
```

**Middleware Integration**:
```go
// pkg/gateway/middleware/metrics.go (NEW FILE)
func HTTPMetrics(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()

            // Wrap response writer to capture status code
            ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

            next.ServeHTTP(ww, r)

            duration := time.Since(start).Seconds()
            metrics.HTTPRequestDuration.WithLabelValues(
                r.Method,
                r.URL.Path,
                strconv.Itoa(ww.Status()),
            ).Observe(duration)
        })
    }
}
```

**Success Criteria**:
- ✅ Histogram tracks request duration by method, path, status
- ✅ Middleware added to server middleware chain
- ✅ Metrics exposed via `/metrics` endpoint

---

### **Phase 4.2: In-Flight Requests Gauge** (30 min)

**Metric**: `gateway_http_requests_in_flight`

**Business Value**: Monitor concurrent request load for capacity planning

**Implementation**:
```go
// pkg/gateway/metrics/metrics.go
HTTPRequestsInFlight prometheus.Gauge
```

**Middleware Integration**:
```go
// pkg/gateway/middleware/metrics.go
func InFlightRequests(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            metrics.HTTPRequestsInFlight.Inc()
            defer metrics.HTTPRequestsInFlight.Dec()

            next.ServeHTTP(w, r)
        })
    }
}
```

**Success Criteria**:
- ✅ Gauge increments on request start
- ✅ Gauge decrements on request end (defer)
- ✅ Accurate concurrent request count

---

### **Phase 4.3: Redis Connection Pool Metrics** (45 min)

**Metrics**:
- `gateway_redis_pool_connections_total` - Total connections
- `gateway_redis_pool_connections_idle` - Idle connections
- `gateway_redis_pool_connections_active` - Active connections
- `gateway_redis_pool_hits_total` - Connection reuse hits
- `gateway_redis_pool_misses_total` - New connection creations
- `gateway_redis_pool_timeouts_total` - Connection acquisition timeouts

**Business Value**: Detect connection pool exhaustion before failures

**Implementation**:
```go
// pkg/gateway/metrics/metrics.go
RedisPoolConnectionsTotal  prometheus.Gauge
RedisPoolConnectionsIdle   prometheus.Gauge
RedisPoolConnectionsActive prometheus.Gauge
RedisPoolHitsTotal         prometheus.Counter
RedisPoolMissesTotal       prometheus.Counter
RedisPoolTimeoutsTotal     prometheus.Counter
```

**Collection Strategy**:
```go
// pkg/gateway/server/server.go
func (s *Server) collectRedisPoolMetrics() {
    // Use Redis client PoolStats() method
    stats := s.redisClient.PoolStats()

    s.metrics.RedisPoolConnectionsTotal.Set(float64(stats.TotalConns))
    s.metrics.RedisPoolConnectionsIdle.Set(float64(stats.IdleConns))
    s.metrics.RedisPoolConnectionsActive.Set(float64(stats.TotalConns - stats.IdleConns))
    s.metrics.RedisPoolHitsTotal.Add(float64(stats.Hits))
    s.metrics.RedisPoolMissesTotal.Add(float64(stats.Misses))
    s.metrics.RedisPoolTimeoutsTotal.Add(float64(stats.Timeouts))
}
```

**Periodic Collection**:
```go
// Start background goroutine to collect pool stats every 10 seconds
go func() {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            s.collectRedisPoolMetrics()
        }
    }
}()
```

**Success Criteria**:
- ✅ Pool stats collected every 10 seconds
- ✅ Metrics reflect current pool state
- ✅ Goroutine stops on server shutdown

---

## 🧪 **TDD Compliance**

### **Classification: REFACTOR Phase** ✅

**Justification**:
1. ✅ **Existing Tests**: Integration tests will verify metrics work
2. ✅ **Standard Patterns**: Using Prometheus standard middleware patterns
3. ✅ **No New Business Logic**: Just observability instrumentation
4. ✅ **Phase 6 Tests**: Dedicated metrics tests planned

**TDD Cycle**:
- ✅ **RED**: Integration tests (Phase 6) will verify metrics
- ✅ **GREEN**: Add metrics middleware (this phase)
- ✅ **REFACTOR**: Already in REFACTOR phase (enhancing observability)

---

## 📊 **Implementation Steps**

### **Step 1: Add Metrics to Centralized Struct** (15 min)

**File**: `pkg/gateway/metrics/metrics.go`

```go
type Metrics struct {
    // ... existing metrics ...

    // HTTP request metrics (Phase 4)
    HTTPRequestDuration   *prometheus.HistogramVec
    HTTPRequestsInFlight  prometheus.Gauge

    // Redis connection pool metrics (Phase 4)
    RedisPoolConnectionsTotal  prometheus.Gauge
    RedisPoolConnectionsIdle   prometheus.Gauge
    RedisPoolConnectionsActive prometheus.Gauge
    RedisPoolHitsTotal         prometheus.Counter
    RedisPoolMissesTotal       prometheus.Counter
    RedisPoolTimeoutsTotal     prometheus.Counter
}

func NewMetrics() *Metrics {
    m := &Metrics{
        // ... existing metrics ...

        // HTTP request metrics
        HTTPRequestDuration: prometheus.NewHistogramVec(
            prometheus.HistogramOpts{
                Name:    "gateway_http_request_duration_seconds",
                Help:    "HTTP request duration in seconds",
                Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
            },
            []string{"method", "path", "status_code"},
        ),

        HTTPRequestsInFlight: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_http_requests_in_flight",
            Help: "Current number of HTTP requests being processed",
        }),

        // Redis pool metrics
        RedisPoolConnectionsTotal: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_redis_pool_connections_total",
            Help: "Total number of connections in the pool",
        }),

        RedisPoolConnectionsIdle: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_redis_pool_connections_idle",
            Help: "Number of idle connections in the pool",
        }),

        RedisPoolConnectionsActive: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_redis_pool_connections_active",
            Help: "Number of active connections in the pool",
        }),

        RedisPoolHitsTotal: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "gateway_redis_pool_hits_total",
            Help: "Total number of times a connection was reused from the pool",
        }),

        RedisPoolMissesTotal: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "gateway_redis_pool_misses_total",
            Help: "Total number of times a new connection was created",
        }),

        RedisPoolTimeoutsTotal: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "gateway_redis_pool_timeouts_total",
            Help: "Total number of connection acquisition timeouts",
        }),
    }

    return m
}

func (m *Metrics) Register(registry *prometheus.Registry) error {
    // ... existing registrations ...

    // HTTP metrics
    registry.MustRegister(m.HTTPRequestDuration)
    registry.MustRegister(m.HTTPRequestsInFlight)

    // Redis pool metrics
    registry.MustRegister(m.RedisPoolConnectionsTotal)
    registry.MustRegister(m.RedisPoolConnectionsIdle)
    registry.MustRegister(m.RedisPoolConnectionsActive)
    registry.MustRegister(m.RedisPoolHitsTotal)
    registry.MustRegister(m.RedisPoolMissesTotal)
    registry.MustRegister(m.RedisPoolTimeoutsTotal)

    return nil
}
```

---

### **Step 2: Create HTTP Metrics Middleware** (30 min)

**File**: `pkg/gateway/middleware/http_metrics.go` (NEW)

```go
package middleware

import (
    "net/http"
    "strconv"
    "time"

    "github.com/go-chi/chi/v5/middleware"
    gatewayMetrics "github.com/jordigilh/kubernaut/pkg/gateway/metrics"
)

// HTTPMetrics tracks HTTP request duration
// BR-GATEWAY-071: HTTP request observability
func HTTPMetrics(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if metrics == nil {
                next.ServeHTTP(w, r)
                return
            }

            start := time.Now()
            ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

            next.ServeHTTP(ww, r)

            duration := time.Since(start).Seconds()
            metrics.HTTPRequestDuration.WithLabelValues(
                r.Method,
                r.URL.Path,
                strconv.Itoa(ww.Status()),
            ).Observe(duration)
        })
    }
}

// InFlightRequests tracks concurrent request count
// BR-GATEWAY-072: In-flight request tracking
func InFlightRequests(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if metrics == nil {
                next.ServeHTTP(w, r)
                return
            }

            metrics.HTTPRequestsInFlight.Inc()
            defer metrics.HTTPRequestsInFlight.Dec()

            next.ServeHTTP(w, r)
        })
    }
}
```

---

### **Step 3: Add Middleware to Server** (15 min)

**File**: `pkg/gateway/server/server.go`

```go
func (s *Server) Handler() http.Handler {
    r := chi.NewRouter()

    // ... existing middleware ...

    // HTTP metrics middleware (Phase 4)
    r.Use(middleware.InFlightRequests(s.metrics))
    r.Use(middleware.HTTPMetrics(s.metrics))

    // ... rest of middleware chain ...

    s.setupRoutes(r)
    return r
}
```

---

### **Step 4: Add Redis Pool Metrics Collection** (30 min)

**File**: `pkg/gateway/server/server.go`

```go
// collectRedisPoolMetrics collects Redis connection pool statistics
// BR-GATEWAY-073: Redis connection pool monitoring
func (s *Server) collectRedisPoolMetrics() {
    if s.metrics == nil || s.redisClient == nil {
        return
    }

    stats := s.redisClient.PoolStats()

    s.metrics.RedisPoolConnectionsTotal.Set(float64(stats.TotalConns))
    s.metrics.RedisPoolConnectionsIdle.Set(float64(stats.IdleConns))
    s.metrics.RedisPoolConnectionsActive.Set(float64(stats.TotalConns - stats.IdleConns))

    // Counters are cumulative, so we need to track deltas
    s.metrics.RedisPoolHitsTotal.Add(float64(stats.Hits))
    s.metrics.RedisPoolMissesTotal.Add(float64(stats.Misses))
    s.metrics.RedisPoolTimeoutsTotal.Add(float64(stats.Timeouts))
}

// Start starts the HTTP server (blocking)
func (s *Server) Start(ctx context.Context) error {
    s.httpServer.Handler = s.Handler()

    // Start Redis health monitoring (existing)
    go s.healthMonitor.Start(ctx)

    // Start Redis pool metrics collection (NEW - Phase 4)
    go s.startRedisPoolMetricsCollection(ctx)

    s.logger.Info("Starting Gateway HTTP server",
        zap.String("addr", s.httpServer.Addr))

    return s.httpServer.ListenAndServe()
}

// startRedisPoolMetricsCollection collects Redis pool stats every 10 seconds
// BR-GATEWAY-073: Redis connection pool monitoring
func (s *Server) startRedisPoolMetricsCollection(ctx context.Context) {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    s.logger.Info("Starting Redis pool metrics collection",
        zap.Duration("interval", 10*time.Second))

    for {
        select {
        case <-ctx.Done():
            s.logger.Info("Stopping Redis pool metrics collection")
            return
        case <-ticker.C:
            s.collectRedisPoolMetrics()
        }
    }
}
```

---

## ✅ **Success Criteria**

### **Functional Requirements**
- [ ] HTTP request duration histogram tracks all requests
- [ ] In-flight requests gauge increments/decrements correctly
- [ ] Redis pool metrics collected every 10 seconds
- [ ] All metrics exposed via `/metrics` endpoint
- [ ] Nil-safe implementation (no panics when metrics disabled)

### **Quality Requirements**
- [ ] Code compiles successfully
- [ ] No new lint errors
- [ ] Middleware follows existing patterns
- [ ] Goroutines stop on server shutdown

### **TDD Compliance**
- [ ] REFACTOR phase (enhancing observability)
- [ ] Integration tests planned for Phase 6
- [ ] No new business logic requiring RED-GREEN

---

## 🎯 **Confidence Assessment**

### **Implementation Confidence: 90%**

**High Confidence Factors**:
- ✅ Standard Prometheus patterns (histogram, gauge, counter)
- ✅ Chi middleware integration is straightforward
- ✅ Redis client provides `PoolStats()` method
- ✅ Similar patterns already exist in codebase

**Minor Risks** (10%):
- ⚠️ Redis pool counter deltas might need adjustment (Hits/Misses are cumulative)
- ⚠️ HTTP path cardinality explosion (need to normalize paths)
- ⚠️ Goroutine cleanup on shutdown needs verification

**Mitigation**:
- Use path patterns instead of raw paths (e.g., `/webhook/:type`)
- Test goroutine shutdown in integration tests
- Monitor Redis counter behavior in Phase 6 tests

---

## 📋 **Phase 4 Checklist**

- [ ] Add 8 new metrics to `pkg/gateway/metrics/metrics.go`
- [ ] Create `pkg/gateway/middleware/http_metrics.go`
- [ ] Add HTTP metrics middleware to server middleware chain
- [ ] Implement `collectRedisPoolMetrics()` function
- [ ] Add background goroutine for pool metrics collection
- [ ] Verify code compiles
- [ ] Verify metrics appear in `/metrics` endpoint
- [ ] Update documentation with new metrics

---

**Estimated Time**: 2 hours
**Complexity**: Medium (middleware + background collection)
**Risk**: Low-Medium (10% - path cardinality, counter deltas)
**Next Phase**: Phase 5 - Structured Logging (1h)

# Day 9 Phase 4: Additional Metrics - APDC Plan

**Date**: 2025-10-26
**Estimated Duration**: 2 hours
**Status**: ⏳ IN PROGRESS

---

## 🎯 **APDC Analysis**

### **Business Requirements**

**BR-GATEWAY-071**: HTTP request observability for performance monitoring
**BR-GATEWAY-072**: In-flight request tracking for capacity planning
**BR-GATEWAY-073**: Redis connection pool monitoring for resource management

**Business Value**:
- **HTTP Latency**: Identify slow endpoints and optimize performance
- **In-Flight Requests**: Detect capacity issues and prevent overload
- **Redis Pool**: Monitor connection exhaustion and prevent failures

### **Current State**

**Existing Metrics** (Phase 2):
- ✅ `gateway_processing_duration_seconds` - Signal processing time
- ✅ `gateway_k8s_api_latency_seconds` - K8s API call latency
- ❌ No HTTP request latency (overall request time)
- ❌ No in-flight request tracking
- ❌ No Redis connection pool metrics

**Gap Analysis**:
1. **HTTP Latency**: Need end-to-end request duration (not just processing)
2. **In-Flight**: Need concurrent request counter for capacity planning
3. **Redis Pool**: Need connection pool stats for resource monitoring

---

## 📋 **APDC Plan**

### **Phase 4.1: HTTP Request Latency Histogram** (45 min)

**Metric**: `gateway_http_request_duration_seconds`

**Business Value**: Track end-to-end HTTP request performance

**Implementation**:
```go
// pkg/gateway/metrics/metrics.go
HTTPRequestDuration *prometheus.HistogramVec // labels: method, path, status_code
```

**Middleware Integration**:
```go
// pkg/gateway/middleware/metrics.go (NEW FILE)
func HTTPMetrics(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()

            // Wrap response writer to capture status code
            ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

            next.ServeHTTP(ww, r)

            duration := time.Since(start).Seconds()
            metrics.HTTPRequestDuration.WithLabelValues(
                r.Method,
                r.URL.Path,
                strconv.Itoa(ww.Status()),
            ).Observe(duration)
        })
    }
}
```

**Success Criteria**:
- ✅ Histogram tracks request duration by method, path, status
- ✅ Middleware added to server middleware chain
- ✅ Metrics exposed via `/metrics` endpoint

---

### **Phase 4.2: In-Flight Requests Gauge** (30 min)

**Metric**: `gateway_http_requests_in_flight`

**Business Value**: Monitor concurrent request load for capacity planning

**Implementation**:
```go
// pkg/gateway/metrics/metrics.go
HTTPRequestsInFlight prometheus.Gauge
```

**Middleware Integration**:
```go
// pkg/gateway/middleware/metrics.go
func InFlightRequests(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            metrics.HTTPRequestsInFlight.Inc()
            defer metrics.HTTPRequestsInFlight.Dec()

            next.ServeHTTP(w, r)
        })
    }
}
```

**Success Criteria**:
- ✅ Gauge increments on request start
- ✅ Gauge decrements on request end (defer)
- ✅ Accurate concurrent request count

---

### **Phase 4.3: Redis Connection Pool Metrics** (45 min)

**Metrics**:
- `gateway_redis_pool_connections_total` - Total connections
- `gateway_redis_pool_connections_idle` - Idle connections
- `gateway_redis_pool_connections_active` - Active connections
- `gateway_redis_pool_hits_total` - Connection reuse hits
- `gateway_redis_pool_misses_total` - New connection creations
- `gateway_redis_pool_timeouts_total` - Connection acquisition timeouts

**Business Value**: Detect connection pool exhaustion before failures

**Implementation**:
```go
// pkg/gateway/metrics/metrics.go
RedisPoolConnectionsTotal  prometheus.Gauge
RedisPoolConnectionsIdle   prometheus.Gauge
RedisPoolConnectionsActive prometheus.Gauge
RedisPoolHitsTotal         prometheus.Counter
RedisPoolMissesTotal       prometheus.Counter
RedisPoolTimeoutsTotal     prometheus.Counter
```

**Collection Strategy**:
```go
// pkg/gateway/server/server.go
func (s *Server) collectRedisPoolMetrics() {
    // Use Redis client PoolStats() method
    stats := s.redisClient.PoolStats()

    s.metrics.RedisPoolConnectionsTotal.Set(float64(stats.TotalConns))
    s.metrics.RedisPoolConnectionsIdle.Set(float64(stats.IdleConns))
    s.metrics.RedisPoolConnectionsActive.Set(float64(stats.TotalConns - stats.IdleConns))
    s.metrics.RedisPoolHitsTotal.Add(float64(stats.Hits))
    s.metrics.RedisPoolMissesTotal.Add(float64(stats.Misses))
    s.metrics.RedisPoolTimeoutsTotal.Add(float64(stats.Timeouts))
}
```

**Periodic Collection**:
```go
// Start background goroutine to collect pool stats every 10 seconds
go func() {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            s.collectRedisPoolMetrics()
        }
    }
}()
```

**Success Criteria**:
- ✅ Pool stats collected every 10 seconds
- ✅ Metrics reflect current pool state
- ✅ Goroutine stops on server shutdown

---

## 🧪 **TDD Compliance**

### **Classification: REFACTOR Phase** ✅

**Justification**:
1. ✅ **Existing Tests**: Integration tests will verify metrics work
2. ✅ **Standard Patterns**: Using Prometheus standard middleware patterns
3. ✅ **No New Business Logic**: Just observability instrumentation
4. ✅ **Phase 6 Tests**: Dedicated metrics tests planned

**TDD Cycle**:
- ✅ **RED**: Integration tests (Phase 6) will verify metrics
- ✅ **GREEN**: Add metrics middleware (this phase)
- ✅ **REFACTOR**: Already in REFACTOR phase (enhancing observability)

---

## 📊 **Implementation Steps**

### **Step 1: Add Metrics to Centralized Struct** (15 min)

**File**: `pkg/gateway/metrics/metrics.go`

```go
type Metrics struct {
    // ... existing metrics ...

    // HTTP request metrics (Phase 4)
    HTTPRequestDuration   *prometheus.HistogramVec
    HTTPRequestsInFlight  prometheus.Gauge

    // Redis connection pool metrics (Phase 4)
    RedisPoolConnectionsTotal  prometheus.Gauge
    RedisPoolConnectionsIdle   prometheus.Gauge
    RedisPoolConnectionsActive prometheus.Gauge
    RedisPoolHitsTotal         prometheus.Counter
    RedisPoolMissesTotal       prometheus.Counter
    RedisPoolTimeoutsTotal     prometheus.Counter
}

func NewMetrics() *Metrics {
    m := &Metrics{
        // ... existing metrics ...

        // HTTP request metrics
        HTTPRequestDuration: prometheus.NewHistogramVec(
            prometheus.HistogramOpts{
                Name:    "gateway_http_request_duration_seconds",
                Help:    "HTTP request duration in seconds",
                Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
            },
            []string{"method", "path", "status_code"},
        ),

        HTTPRequestsInFlight: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_http_requests_in_flight",
            Help: "Current number of HTTP requests being processed",
        }),

        // Redis pool metrics
        RedisPoolConnectionsTotal: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_redis_pool_connections_total",
            Help: "Total number of connections in the pool",
        }),

        RedisPoolConnectionsIdle: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_redis_pool_connections_idle",
            Help: "Number of idle connections in the pool",
        }),

        RedisPoolConnectionsActive: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_redis_pool_connections_active",
            Help: "Number of active connections in the pool",
        }),

        RedisPoolHitsTotal: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "gateway_redis_pool_hits_total",
            Help: "Total number of times a connection was reused from the pool",
        }),

        RedisPoolMissesTotal: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "gateway_redis_pool_misses_total",
            Help: "Total number of times a new connection was created",
        }),

        RedisPoolTimeoutsTotal: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "gateway_redis_pool_timeouts_total",
            Help: "Total number of connection acquisition timeouts",
        }),
    }

    return m
}

func (m *Metrics) Register(registry *prometheus.Registry) error {
    // ... existing registrations ...

    // HTTP metrics
    registry.MustRegister(m.HTTPRequestDuration)
    registry.MustRegister(m.HTTPRequestsInFlight)

    // Redis pool metrics
    registry.MustRegister(m.RedisPoolConnectionsTotal)
    registry.MustRegister(m.RedisPoolConnectionsIdle)
    registry.MustRegister(m.RedisPoolConnectionsActive)
    registry.MustRegister(m.RedisPoolHitsTotal)
    registry.MustRegister(m.RedisPoolMissesTotal)
    registry.MustRegister(m.RedisPoolTimeoutsTotal)

    return nil
}
```

---

### **Step 2: Create HTTP Metrics Middleware** (30 min)

**File**: `pkg/gateway/middleware/http_metrics.go` (NEW)

```go
package middleware

import (
    "net/http"
    "strconv"
    "time"

    "github.com/go-chi/chi/v5/middleware"
    gatewayMetrics "github.com/jordigilh/kubernaut/pkg/gateway/metrics"
)

// HTTPMetrics tracks HTTP request duration
// BR-GATEWAY-071: HTTP request observability
func HTTPMetrics(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if metrics == nil {
                next.ServeHTTP(w, r)
                return
            }

            start := time.Now()
            ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

            next.ServeHTTP(ww, r)

            duration := time.Since(start).Seconds()
            metrics.HTTPRequestDuration.WithLabelValues(
                r.Method,
                r.URL.Path,
                strconv.Itoa(ww.Status()),
            ).Observe(duration)
        })
    }
}

// InFlightRequests tracks concurrent request count
// BR-GATEWAY-072: In-flight request tracking
func InFlightRequests(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if metrics == nil {
                next.ServeHTTP(w, r)
                return
            }

            metrics.HTTPRequestsInFlight.Inc()
            defer metrics.HTTPRequestsInFlight.Dec()

            next.ServeHTTP(w, r)
        })
    }
}
```

---

### **Step 3: Add Middleware to Server** (15 min)

**File**: `pkg/gateway/server/server.go`

```go
func (s *Server) Handler() http.Handler {
    r := chi.NewRouter()

    // ... existing middleware ...

    // HTTP metrics middleware (Phase 4)
    r.Use(middleware.InFlightRequests(s.metrics))
    r.Use(middleware.HTTPMetrics(s.metrics))

    // ... rest of middleware chain ...

    s.setupRoutes(r)
    return r
}
```

---

### **Step 4: Add Redis Pool Metrics Collection** (30 min)

**File**: `pkg/gateway/server/server.go`

```go
// collectRedisPoolMetrics collects Redis connection pool statistics
// BR-GATEWAY-073: Redis connection pool monitoring
func (s *Server) collectRedisPoolMetrics() {
    if s.metrics == nil || s.redisClient == nil {
        return
    }

    stats := s.redisClient.PoolStats()

    s.metrics.RedisPoolConnectionsTotal.Set(float64(stats.TotalConns))
    s.metrics.RedisPoolConnectionsIdle.Set(float64(stats.IdleConns))
    s.metrics.RedisPoolConnectionsActive.Set(float64(stats.TotalConns - stats.IdleConns))

    // Counters are cumulative, so we need to track deltas
    s.metrics.RedisPoolHitsTotal.Add(float64(stats.Hits))
    s.metrics.RedisPoolMissesTotal.Add(float64(stats.Misses))
    s.metrics.RedisPoolTimeoutsTotal.Add(float64(stats.Timeouts))
}

// Start starts the HTTP server (blocking)
func (s *Server) Start(ctx context.Context) error {
    s.httpServer.Handler = s.Handler()

    // Start Redis health monitoring (existing)
    go s.healthMonitor.Start(ctx)

    // Start Redis pool metrics collection (NEW - Phase 4)
    go s.startRedisPoolMetricsCollection(ctx)

    s.logger.Info("Starting Gateway HTTP server",
        zap.String("addr", s.httpServer.Addr))

    return s.httpServer.ListenAndServe()
}

// startRedisPoolMetricsCollection collects Redis pool stats every 10 seconds
// BR-GATEWAY-073: Redis connection pool monitoring
func (s *Server) startRedisPoolMetricsCollection(ctx context.Context) {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    s.logger.Info("Starting Redis pool metrics collection",
        zap.Duration("interval", 10*time.Second))

    for {
        select {
        case <-ctx.Done():
            s.logger.Info("Stopping Redis pool metrics collection")
            return
        case <-ticker.C:
            s.collectRedisPoolMetrics()
        }
    }
}
```

---

## ✅ **Success Criteria**

### **Functional Requirements**
- [ ] HTTP request duration histogram tracks all requests
- [ ] In-flight requests gauge increments/decrements correctly
- [ ] Redis pool metrics collected every 10 seconds
- [ ] All metrics exposed via `/metrics` endpoint
- [ ] Nil-safe implementation (no panics when metrics disabled)

### **Quality Requirements**
- [ ] Code compiles successfully
- [ ] No new lint errors
- [ ] Middleware follows existing patterns
- [ ] Goroutines stop on server shutdown

### **TDD Compliance**
- [ ] REFACTOR phase (enhancing observability)
- [ ] Integration tests planned for Phase 6
- [ ] No new business logic requiring RED-GREEN

---

## 🎯 **Confidence Assessment**

### **Implementation Confidence: 90%**

**High Confidence Factors**:
- ✅ Standard Prometheus patterns (histogram, gauge, counter)
- ✅ Chi middleware integration is straightforward
- ✅ Redis client provides `PoolStats()` method
- ✅ Similar patterns already exist in codebase

**Minor Risks** (10%):
- ⚠️ Redis pool counter deltas might need adjustment (Hits/Misses are cumulative)
- ⚠️ HTTP path cardinality explosion (need to normalize paths)
- ⚠️ Goroutine cleanup on shutdown needs verification

**Mitigation**:
- Use path patterns instead of raw paths (e.g., `/webhook/:type`)
- Test goroutine shutdown in integration tests
- Monitor Redis counter behavior in Phase 6 tests

---

## 📋 **Phase 4 Checklist**

- [ ] Add 8 new metrics to `pkg/gateway/metrics/metrics.go`
- [ ] Create `pkg/gateway/middleware/http_metrics.go`
- [ ] Add HTTP metrics middleware to server middleware chain
- [ ] Implement `collectRedisPoolMetrics()` function
- [ ] Add background goroutine for pool metrics collection
- [ ] Verify code compiles
- [ ] Verify metrics appear in `/metrics` endpoint
- [ ] Update documentation with new metrics

---

**Estimated Time**: 2 hours
**Complexity**: Medium (middleware + background collection)
**Risk**: Low-Medium (10% - path cardinality, counter deltas)
**Next Phase**: Phase 5 - Structured Logging (1h)



**Date**: 2025-10-26
**Estimated Duration**: 2 hours
**Status**: ⏳ IN PROGRESS

---

## 🎯 **APDC Analysis**

### **Business Requirements**

**BR-GATEWAY-071**: HTTP request observability for performance monitoring
**BR-GATEWAY-072**: In-flight request tracking for capacity planning
**BR-GATEWAY-073**: Redis connection pool monitoring for resource management

**Business Value**:
- **HTTP Latency**: Identify slow endpoints and optimize performance
- **In-Flight Requests**: Detect capacity issues and prevent overload
- **Redis Pool**: Monitor connection exhaustion and prevent failures

### **Current State**

**Existing Metrics** (Phase 2):
- ✅ `gateway_processing_duration_seconds` - Signal processing time
- ✅ `gateway_k8s_api_latency_seconds` - K8s API call latency
- ❌ No HTTP request latency (overall request time)
- ❌ No in-flight request tracking
- ❌ No Redis connection pool metrics

**Gap Analysis**:
1. **HTTP Latency**: Need end-to-end request duration (not just processing)
2. **In-Flight**: Need concurrent request counter for capacity planning
3. **Redis Pool**: Need connection pool stats for resource monitoring

---

## 📋 **APDC Plan**

### **Phase 4.1: HTTP Request Latency Histogram** (45 min)

**Metric**: `gateway_http_request_duration_seconds`

**Business Value**: Track end-to-end HTTP request performance

**Implementation**:
```go
// pkg/gateway/metrics/metrics.go
HTTPRequestDuration *prometheus.HistogramVec // labels: method, path, status_code
```

**Middleware Integration**:
```go
// pkg/gateway/middleware/metrics.go (NEW FILE)
func HTTPMetrics(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()

            // Wrap response writer to capture status code
            ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

            next.ServeHTTP(ww, r)

            duration := time.Since(start).Seconds()
            metrics.HTTPRequestDuration.WithLabelValues(
                r.Method,
                r.URL.Path,
                strconv.Itoa(ww.Status()),
            ).Observe(duration)
        })
    }
}
```

**Success Criteria**:
- ✅ Histogram tracks request duration by method, path, status
- ✅ Middleware added to server middleware chain
- ✅ Metrics exposed via `/metrics` endpoint

---

### **Phase 4.2: In-Flight Requests Gauge** (30 min)

**Metric**: `gateway_http_requests_in_flight`

**Business Value**: Monitor concurrent request load for capacity planning

**Implementation**:
```go
// pkg/gateway/metrics/metrics.go
HTTPRequestsInFlight prometheus.Gauge
```

**Middleware Integration**:
```go
// pkg/gateway/middleware/metrics.go
func InFlightRequests(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            metrics.HTTPRequestsInFlight.Inc()
            defer metrics.HTTPRequestsInFlight.Dec()

            next.ServeHTTP(w, r)
        })
    }
}
```

**Success Criteria**:
- ✅ Gauge increments on request start
- ✅ Gauge decrements on request end (defer)
- ✅ Accurate concurrent request count

---

### **Phase 4.3: Redis Connection Pool Metrics** (45 min)

**Metrics**:
- `gateway_redis_pool_connections_total` - Total connections
- `gateway_redis_pool_connections_idle` - Idle connections
- `gateway_redis_pool_connections_active` - Active connections
- `gateway_redis_pool_hits_total` - Connection reuse hits
- `gateway_redis_pool_misses_total` - New connection creations
- `gateway_redis_pool_timeouts_total` - Connection acquisition timeouts

**Business Value**: Detect connection pool exhaustion before failures

**Implementation**:
```go
// pkg/gateway/metrics/metrics.go
RedisPoolConnectionsTotal  prometheus.Gauge
RedisPoolConnectionsIdle   prometheus.Gauge
RedisPoolConnectionsActive prometheus.Gauge
RedisPoolHitsTotal         prometheus.Counter
RedisPoolMissesTotal       prometheus.Counter
RedisPoolTimeoutsTotal     prometheus.Counter
```

**Collection Strategy**:
```go
// pkg/gateway/server/server.go
func (s *Server) collectRedisPoolMetrics() {
    // Use Redis client PoolStats() method
    stats := s.redisClient.PoolStats()

    s.metrics.RedisPoolConnectionsTotal.Set(float64(stats.TotalConns))
    s.metrics.RedisPoolConnectionsIdle.Set(float64(stats.IdleConns))
    s.metrics.RedisPoolConnectionsActive.Set(float64(stats.TotalConns - stats.IdleConns))
    s.metrics.RedisPoolHitsTotal.Add(float64(stats.Hits))
    s.metrics.RedisPoolMissesTotal.Add(float64(stats.Misses))
    s.metrics.RedisPoolTimeoutsTotal.Add(float64(stats.Timeouts))
}
```

**Periodic Collection**:
```go
// Start background goroutine to collect pool stats every 10 seconds
go func() {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            s.collectRedisPoolMetrics()
        }
    }
}()
```

**Success Criteria**:
- ✅ Pool stats collected every 10 seconds
- ✅ Metrics reflect current pool state
- ✅ Goroutine stops on server shutdown

---

## 🧪 **TDD Compliance**

### **Classification: REFACTOR Phase** ✅

**Justification**:
1. ✅ **Existing Tests**: Integration tests will verify metrics work
2. ✅ **Standard Patterns**: Using Prometheus standard middleware patterns
3. ✅ **No New Business Logic**: Just observability instrumentation
4. ✅ **Phase 6 Tests**: Dedicated metrics tests planned

**TDD Cycle**:
- ✅ **RED**: Integration tests (Phase 6) will verify metrics
- ✅ **GREEN**: Add metrics middleware (this phase)
- ✅ **REFACTOR**: Already in REFACTOR phase (enhancing observability)

---

## 📊 **Implementation Steps**

### **Step 1: Add Metrics to Centralized Struct** (15 min)

**File**: `pkg/gateway/metrics/metrics.go`

```go
type Metrics struct {
    // ... existing metrics ...

    // HTTP request metrics (Phase 4)
    HTTPRequestDuration   *prometheus.HistogramVec
    HTTPRequestsInFlight  prometheus.Gauge

    // Redis connection pool metrics (Phase 4)
    RedisPoolConnectionsTotal  prometheus.Gauge
    RedisPoolConnectionsIdle   prometheus.Gauge
    RedisPoolConnectionsActive prometheus.Gauge
    RedisPoolHitsTotal         prometheus.Counter
    RedisPoolMissesTotal       prometheus.Counter
    RedisPoolTimeoutsTotal     prometheus.Counter
}

func NewMetrics() *Metrics {
    m := &Metrics{
        // ... existing metrics ...

        // HTTP request metrics
        HTTPRequestDuration: prometheus.NewHistogramVec(
            prometheus.HistogramOpts{
                Name:    "gateway_http_request_duration_seconds",
                Help:    "HTTP request duration in seconds",
                Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
            },
            []string{"method", "path", "status_code"},
        ),

        HTTPRequestsInFlight: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_http_requests_in_flight",
            Help: "Current number of HTTP requests being processed",
        }),

        // Redis pool metrics
        RedisPoolConnectionsTotal: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_redis_pool_connections_total",
            Help: "Total number of connections in the pool",
        }),

        RedisPoolConnectionsIdle: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_redis_pool_connections_idle",
            Help: "Number of idle connections in the pool",
        }),

        RedisPoolConnectionsActive: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_redis_pool_connections_active",
            Help: "Number of active connections in the pool",
        }),

        RedisPoolHitsTotal: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "gateway_redis_pool_hits_total",
            Help: "Total number of times a connection was reused from the pool",
        }),

        RedisPoolMissesTotal: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "gateway_redis_pool_misses_total",
            Help: "Total number of times a new connection was created",
        }),

        RedisPoolTimeoutsTotal: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "gateway_redis_pool_timeouts_total",
            Help: "Total number of connection acquisition timeouts",
        }),
    }

    return m
}

func (m *Metrics) Register(registry *prometheus.Registry) error {
    // ... existing registrations ...

    // HTTP metrics
    registry.MustRegister(m.HTTPRequestDuration)
    registry.MustRegister(m.HTTPRequestsInFlight)

    // Redis pool metrics
    registry.MustRegister(m.RedisPoolConnectionsTotal)
    registry.MustRegister(m.RedisPoolConnectionsIdle)
    registry.MustRegister(m.RedisPoolConnectionsActive)
    registry.MustRegister(m.RedisPoolHitsTotal)
    registry.MustRegister(m.RedisPoolMissesTotal)
    registry.MustRegister(m.RedisPoolTimeoutsTotal)

    return nil
}
```

---

### **Step 2: Create HTTP Metrics Middleware** (30 min)

**File**: `pkg/gateway/middleware/http_metrics.go` (NEW)

```go
package middleware

import (
    "net/http"
    "strconv"
    "time"

    "github.com/go-chi/chi/v5/middleware"
    gatewayMetrics "github.com/jordigilh/kubernaut/pkg/gateway/metrics"
)

// HTTPMetrics tracks HTTP request duration
// BR-GATEWAY-071: HTTP request observability
func HTTPMetrics(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if metrics == nil {
                next.ServeHTTP(w, r)
                return
            }

            start := time.Now()
            ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

            next.ServeHTTP(ww, r)

            duration := time.Since(start).Seconds()
            metrics.HTTPRequestDuration.WithLabelValues(
                r.Method,
                r.URL.Path,
                strconv.Itoa(ww.Status()),
            ).Observe(duration)
        })
    }
}

// InFlightRequests tracks concurrent request count
// BR-GATEWAY-072: In-flight request tracking
func InFlightRequests(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if metrics == nil {
                next.ServeHTTP(w, r)
                return
            }

            metrics.HTTPRequestsInFlight.Inc()
            defer metrics.HTTPRequestsInFlight.Dec()

            next.ServeHTTP(w, r)
        })
    }
}
```

---

### **Step 3: Add Middleware to Server** (15 min)

**File**: `pkg/gateway/server/server.go`

```go
func (s *Server) Handler() http.Handler {
    r := chi.NewRouter()

    // ... existing middleware ...

    // HTTP metrics middleware (Phase 4)
    r.Use(middleware.InFlightRequests(s.metrics))
    r.Use(middleware.HTTPMetrics(s.metrics))

    // ... rest of middleware chain ...

    s.setupRoutes(r)
    return r
}
```

---

### **Step 4: Add Redis Pool Metrics Collection** (30 min)

**File**: `pkg/gateway/server/server.go`

```go
// collectRedisPoolMetrics collects Redis connection pool statistics
// BR-GATEWAY-073: Redis connection pool monitoring
func (s *Server) collectRedisPoolMetrics() {
    if s.metrics == nil || s.redisClient == nil {
        return
    }

    stats := s.redisClient.PoolStats()

    s.metrics.RedisPoolConnectionsTotal.Set(float64(stats.TotalConns))
    s.metrics.RedisPoolConnectionsIdle.Set(float64(stats.IdleConns))
    s.metrics.RedisPoolConnectionsActive.Set(float64(stats.TotalConns - stats.IdleConns))

    // Counters are cumulative, so we need to track deltas
    s.metrics.RedisPoolHitsTotal.Add(float64(stats.Hits))
    s.metrics.RedisPoolMissesTotal.Add(float64(stats.Misses))
    s.metrics.RedisPoolTimeoutsTotal.Add(float64(stats.Timeouts))
}

// Start starts the HTTP server (blocking)
func (s *Server) Start(ctx context.Context) error {
    s.httpServer.Handler = s.Handler()

    // Start Redis health monitoring (existing)
    go s.healthMonitor.Start(ctx)

    // Start Redis pool metrics collection (NEW - Phase 4)
    go s.startRedisPoolMetricsCollection(ctx)

    s.logger.Info("Starting Gateway HTTP server",
        zap.String("addr", s.httpServer.Addr))

    return s.httpServer.ListenAndServe()
}

// startRedisPoolMetricsCollection collects Redis pool stats every 10 seconds
// BR-GATEWAY-073: Redis connection pool monitoring
func (s *Server) startRedisPoolMetricsCollection(ctx context.Context) {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    s.logger.Info("Starting Redis pool metrics collection",
        zap.Duration("interval", 10*time.Second))

    for {
        select {
        case <-ctx.Done():
            s.logger.Info("Stopping Redis pool metrics collection")
            return
        case <-ticker.C:
            s.collectRedisPoolMetrics()
        }
    }
}
```

---

## ✅ **Success Criteria**

### **Functional Requirements**
- [ ] HTTP request duration histogram tracks all requests
- [ ] In-flight requests gauge increments/decrements correctly
- [ ] Redis pool metrics collected every 10 seconds
- [ ] All metrics exposed via `/metrics` endpoint
- [ ] Nil-safe implementation (no panics when metrics disabled)

### **Quality Requirements**
- [ ] Code compiles successfully
- [ ] No new lint errors
- [ ] Middleware follows existing patterns
- [ ] Goroutines stop on server shutdown

### **TDD Compliance**
- [ ] REFACTOR phase (enhancing observability)
- [ ] Integration tests planned for Phase 6
- [ ] No new business logic requiring RED-GREEN

---

## 🎯 **Confidence Assessment**

### **Implementation Confidence: 90%**

**High Confidence Factors**:
- ✅ Standard Prometheus patterns (histogram, gauge, counter)
- ✅ Chi middleware integration is straightforward
- ✅ Redis client provides `PoolStats()` method
- ✅ Similar patterns already exist in codebase

**Minor Risks** (10%):
- ⚠️ Redis pool counter deltas might need adjustment (Hits/Misses are cumulative)
- ⚠️ HTTP path cardinality explosion (need to normalize paths)
- ⚠️ Goroutine cleanup on shutdown needs verification

**Mitigation**:
- Use path patterns instead of raw paths (e.g., `/webhook/:type`)
- Test goroutine shutdown in integration tests
- Monitor Redis counter behavior in Phase 6 tests

---

## 📋 **Phase 4 Checklist**

- [ ] Add 8 new metrics to `pkg/gateway/metrics/metrics.go`
- [ ] Create `pkg/gateway/middleware/http_metrics.go`
- [ ] Add HTTP metrics middleware to server middleware chain
- [ ] Implement `collectRedisPoolMetrics()` function
- [ ] Add background goroutine for pool metrics collection
- [ ] Verify code compiles
- [ ] Verify metrics appear in `/metrics` endpoint
- [ ] Update documentation with new metrics

---

**Estimated Time**: 2 hours
**Complexity**: Medium (middleware + background collection)
**Risk**: Low-Medium (10% - path cardinality, counter deltas)
**Next Phase**: Phase 5 - Structured Logging (1h)

# Day 9 Phase 4: Additional Metrics - APDC Plan

**Date**: 2025-10-26
**Estimated Duration**: 2 hours
**Status**: ⏳ IN PROGRESS

---

## 🎯 **APDC Analysis**

### **Business Requirements**

**BR-GATEWAY-071**: HTTP request observability for performance monitoring
**BR-GATEWAY-072**: In-flight request tracking for capacity planning
**BR-GATEWAY-073**: Redis connection pool monitoring for resource management

**Business Value**:
- **HTTP Latency**: Identify slow endpoints and optimize performance
- **In-Flight Requests**: Detect capacity issues and prevent overload
- **Redis Pool**: Monitor connection exhaustion and prevent failures

### **Current State**

**Existing Metrics** (Phase 2):
- ✅ `gateway_processing_duration_seconds` - Signal processing time
- ✅ `gateway_k8s_api_latency_seconds` - K8s API call latency
- ❌ No HTTP request latency (overall request time)
- ❌ No in-flight request tracking
- ❌ No Redis connection pool metrics

**Gap Analysis**:
1. **HTTP Latency**: Need end-to-end request duration (not just processing)
2. **In-Flight**: Need concurrent request counter for capacity planning
3. **Redis Pool**: Need connection pool stats for resource monitoring

---

## 📋 **APDC Plan**

### **Phase 4.1: HTTP Request Latency Histogram** (45 min)

**Metric**: `gateway_http_request_duration_seconds`

**Business Value**: Track end-to-end HTTP request performance

**Implementation**:
```go
// pkg/gateway/metrics/metrics.go
HTTPRequestDuration *prometheus.HistogramVec // labels: method, path, status_code
```

**Middleware Integration**:
```go
// pkg/gateway/middleware/metrics.go (NEW FILE)
func HTTPMetrics(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()

            // Wrap response writer to capture status code
            ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

            next.ServeHTTP(ww, r)

            duration := time.Since(start).Seconds()
            metrics.HTTPRequestDuration.WithLabelValues(
                r.Method,
                r.URL.Path,
                strconv.Itoa(ww.Status()),
            ).Observe(duration)
        })
    }
}
```

**Success Criteria**:
- ✅ Histogram tracks request duration by method, path, status
- ✅ Middleware added to server middleware chain
- ✅ Metrics exposed via `/metrics` endpoint

---

### **Phase 4.2: In-Flight Requests Gauge** (30 min)

**Metric**: `gateway_http_requests_in_flight`

**Business Value**: Monitor concurrent request load for capacity planning

**Implementation**:
```go
// pkg/gateway/metrics/metrics.go
HTTPRequestsInFlight prometheus.Gauge
```

**Middleware Integration**:
```go
// pkg/gateway/middleware/metrics.go
func InFlightRequests(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            metrics.HTTPRequestsInFlight.Inc()
            defer metrics.HTTPRequestsInFlight.Dec()

            next.ServeHTTP(w, r)
        })
    }
}
```

**Success Criteria**:
- ✅ Gauge increments on request start
- ✅ Gauge decrements on request end (defer)
- ✅ Accurate concurrent request count

---

### **Phase 4.3: Redis Connection Pool Metrics** (45 min)

**Metrics**:
- `gateway_redis_pool_connections_total` - Total connections
- `gateway_redis_pool_connections_idle` - Idle connections
- `gateway_redis_pool_connections_active` - Active connections
- `gateway_redis_pool_hits_total` - Connection reuse hits
- `gateway_redis_pool_misses_total` - New connection creations
- `gateway_redis_pool_timeouts_total` - Connection acquisition timeouts

**Business Value**: Detect connection pool exhaustion before failures

**Implementation**:
```go
// pkg/gateway/metrics/metrics.go
RedisPoolConnectionsTotal  prometheus.Gauge
RedisPoolConnectionsIdle   prometheus.Gauge
RedisPoolConnectionsActive prometheus.Gauge
RedisPoolHitsTotal         prometheus.Counter
RedisPoolMissesTotal       prometheus.Counter
RedisPoolTimeoutsTotal     prometheus.Counter
```

**Collection Strategy**:
```go
// pkg/gateway/server/server.go
func (s *Server) collectRedisPoolMetrics() {
    // Use Redis client PoolStats() method
    stats := s.redisClient.PoolStats()

    s.metrics.RedisPoolConnectionsTotal.Set(float64(stats.TotalConns))
    s.metrics.RedisPoolConnectionsIdle.Set(float64(stats.IdleConns))
    s.metrics.RedisPoolConnectionsActive.Set(float64(stats.TotalConns - stats.IdleConns))
    s.metrics.RedisPoolHitsTotal.Add(float64(stats.Hits))
    s.metrics.RedisPoolMissesTotal.Add(float64(stats.Misses))
    s.metrics.RedisPoolTimeoutsTotal.Add(float64(stats.Timeouts))
}
```

**Periodic Collection**:
```go
// Start background goroutine to collect pool stats every 10 seconds
go func() {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            s.collectRedisPoolMetrics()
        }
    }
}()
```

**Success Criteria**:
- ✅ Pool stats collected every 10 seconds
- ✅ Metrics reflect current pool state
- ✅ Goroutine stops on server shutdown

---

## 🧪 **TDD Compliance**

### **Classification: REFACTOR Phase** ✅

**Justification**:
1. ✅ **Existing Tests**: Integration tests will verify metrics work
2. ✅ **Standard Patterns**: Using Prometheus standard middleware patterns
3. ✅ **No New Business Logic**: Just observability instrumentation
4. ✅ **Phase 6 Tests**: Dedicated metrics tests planned

**TDD Cycle**:
- ✅ **RED**: Integration tests (Phase 6) will verify metrics
- ✅ **GREEN**: Add metrics middleware (this phase)
- ✅ **REFACTOR**: Already in REFACTOR phase (enhancing observability)

---

## 📊 **Implementation Steps**

### **Step 1: Add Metrics to Centralized Struct** (15 min)

**File**: `pkg/gateway/metrics/metrics.go`

```go
type Metrics struct {
    // ... existing metrics ...

    // HTTP request metrics (Phase 4)
    HTTPRequestDuration   *prometheus.HistogramVec
    HTTPRequestsInFlight  prometheus.Gauge

    // Redis connection pool metrics (Phase 4)
    RedisPoolConnectionsTotal  prometheus.Gauge
    RedisPoolConnectionsIdle   prometheus.Gauge
    RedisPoolConnectionsActive prometheus.Gauge
    RedisPoolHitsTotal         prometheus.Counter
    RedisPoolMissesTotal       prometheus.Counter
    RedisPoolTimeoutsTotal     prometheus.Counter
}

func NewMetrics() *Metrics {
    m := &Metrics{
        // ... existing metrics ...

        // HTTP request metrics
        HTTPRequestDuration: prometheus.NewHistogramVec(
            prometheus.HistogramOpts{
                Name:    "gateway_http_request_duration_seconds",
                Help:    "HTTP request duration in seconds",
                Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
            },
            []string{"method", "path", "status_code"},
        ),

        HTTPRequestsInFlight: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_http_requests_in_flight",
            Help: "Current number of HTTP requests being processed",
        }),

        // Redis pool metrics
        RedisPoolConnectionsTotal: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_redis_pool_connections_total",
            Help: "Total number of connections in the pool",
        }),

        RedisPoolConnectionsIdle: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_redis_pool_connections_idle",
            Help: "Number of idle connections in the pool",
        }),

        RedisPoolConnectionsActive: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "gateway_redis_pool_connections_active",
            Help: "Number of active connections in the pool",
        }),

        RedisPoolHitsTotal: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "gateway_redis_pool_hits_total",
            Help: "Total number of times a connection was reused from the pool",
        }),

        RedisPoolMissesTotal: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "gateway_redis_pool_misses_total",
            Help: "Total number of times a new connection was created",
        }),

        RedisPoolTimeoutsTotal: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "gateway_redis_pool_timeouts_total",
            Help: "Total number of connection acquisition timeouts",
        }),
    }

    return m
}

func (m *Metrics) Register(registry *prometheus.Registry) error {
    // ... existing registrations ...

    // HTTP metrics
    registry.MustRegister(m.HTTPRequestDuration)
    registry.MustRegister(m.HTTPRequestsInFlight)

    // Redis pool metrics
    registry.MustRegister(m.RedisPoolConnectionsTotal)
    registry.MustRegister(m.RedisPoolConnectionsIdle)
    registry.MustRegister(m.RedisPoolConnectionsActive)
    registry.MustRegister(m.RedisPoolHitsTotal)
    registry.MustRegister(m.RedisPoolMissesTotal)
    registry.MustRegister(m.RedisPoolTimeoutsTotal)

    return nil
}
```

---

### **Step 2: Create HTTP Metrics Middleware** (30 min)

**File**: `pkg/gateway/middleware/http_metrics.go` (NEW)

```go
package middleware

import (
    "net/http"
    "strconv"
    "time"

    "github.com/go-chi/chi/v5/middleware"
    gatewayMetrics "github.com/jordigilh/kubernaut/pkg/gateway/metrics"
)

// HTTPMetrics tracks HTTP request duration
// BR-GATEWAY-071: HTTP request observability
func HTTPMetrics(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if metrics == nil {
                next.ServeHTTP(w, r)
                return
            }

            start := time.Now()
            ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

            next.ServeHTTP(ww, r)

            duration := time.Since(start).Seconds()
            metrics.HTTPRequestDuration.WithLabelValues(
                r.Method,
                r.URL.Path,
                strconv.Itoa(ww.Status()),
            ).Observe(duration)
        })
    }
}

// InFlightRequests tracks concurrent request count
// BR-GATEWAY-072: In-flight request tracking
func InFlightRequests(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if metrics == nil {
                next.ServeHTTP(w, r)
                return
            }

            metrics.HTTPRequestsInFlight.Inc()
            defer metrics.HTTPRequestsInFlight.Dec()

            next.ServeHTTP(w, r)
        })
    }
}
```

---

### **Step 3: Add Middleware to Server** (15 min)

**File**: `pkg/gateway/server/server.go`

```go
func (s *Server) Handler() http.Handler {
    r := chi.NewRouter()

    // ... existing middleware ...

    // HTTP metrics middleware (Phase 4)
    r.Use(middleware.InFlightRequests(s.metrics))
    r.Use(middleware.HTTPMetrics(s.metrics))

    // ... rest of middleware chain ...

    s.setupRoutes(r)
    return r
}
```

---

### **Step 4: Add Redis Pool Metrics Collection** (30 min)

**File**: `pkg/gateway/server/server.go`

```go
// collectRedisPoolMetrics collects Redis connection pool statistics
// BR-GATEWAY-073: Redis connection pool monitoring
func (s *Server) collectRedisPoolMetrics() {
    if s.metrics == nil || s.redisClient == nil {
        return
    }

    stats := s.redisClient.PoolStats()

    s.metrics.RedisPoolConnectionsTotal.Set(float64(stats.TotalConns))
    s.metrics.RedisPoolConnectionsIdle.Set(float64(stats.IdleConns))
    s.metrics.RedisPoolConnectionsActive.Set(float64(stats.TotalConns - stats.IdleConns))

    // Counters are cumulative, so we need to track deltas
    s.metrics.RedisPoolHitsTotal.Add(float64(stats.Hits))
    s.metrics.RedisPoolMissesTotal.Add(float64(stats.Misses))
    s.metrics.RedisPoolTimeoutsTotal.Add(float64(stats.Timeouts))
}

// Start starts the HTTP server (blocking)
func (s *Server) Start(ctx context.Context) error {
    s.httpServer.Handler = s.Handler()

    // Start Redis health monitoring (existing)
    go s.healthMonitor.Start(ctx)

    // Start Redis pool metrics collection (NEW - Phase 4)
    go s.startRedisPoolMetricsCollection(ctx)

    s.logger.Info("Starting Gateway HTTP server",
        zap.String("addr", s.httpServer.Addr))

    return s.httpServer.ListenAndServe()
}

// startRedisPoolMetricsCollection collects Redis pool stats every 10 seconds
// BR-GATEWAY-073: Redis connection pool monitoring
func (s *Server) startRedisPoolMetricsCollection(ctx context.Context) {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    s.logger.Info("Starting Redis pool metrics collection",
        zap.Duration("interval", 10*time.Second))

    for {
        select {
        case <-ctx.Done():
            s.logger.Info("Stopping Redis pool metrics collection")
            return
        case <-ticker.C:
            s.collectRedisPoolMetrics()
        }
    }
}
```

---

## ✅ **Success Criteria**

### **Functional Requirements**
- [ ] HTTP request duration histogram tracks all requests
- [ ] In-flight requests gauge increments/decrements correctly
- [ ] Redis pool metrics collected every 10 seconds
- [ ] All metrics exposed via `/metrics` endpoint
- [ ] Nil-safe implementation (no panics when metrics disabled)

### **Quality Requirements**
- [ ] Code compiles successfully
- [ ] No new lint errors
- [ ] Middleware follows existing patterns
- [ ] Goroutines stop on server shutdown

### **TDD Compliance**
- [ ] REFACTOR phase (enhancing observability)
- [ ] Integration tests planned for Phase 6
- [ ] No new business logic requiring RED-GREEN

---

## 🎯 **Confidence Assessment**

### **Implementation Confidence: 90%**

**High Confidence Factors**:
- ✅ Standard Prometheus patterns (histogram, gauge, counter)
- ✅ Chi middleware integration is straightforward
- ✅ Redis client provides `PoolStats()` method
- ✅ Similar patterns already exist in codebase

**Minor Risks** (10%):
- ⚠️ Redis pool counter deltas might need adjustment (Hits/Misses are cumulative)
- ⚠️ HTTP path cardinality explosion (need to normalize paths)
- ⚠️ Goroutine cleanup on shutdown needs verification

**Mitigation**:
- Use path patterns instead of raw paths (e.g., `/webhook/:type`)
- Test goroutine shutdown in integration tests
- Monitor Redis counter behavior in Phase 6 tests

---

## 📋 **Phase 4 Checklist**

- [ ] Add 8 new metrics to `pkg/gateway/metrics/metrics.go`
- [ ] Create `pkg/gateway/middleware/http_metrics.go`
- [ ] Add HTTP metrics middleware to server middleware chain
- [ ] Implement `collectRedisPoolMetrics()` function
- [ ] Add background goroutine for pool metrics collection
- [ ] Verify code compiles
- [ ] Verify metrics appear in `/metrics` endpoint
- [ ] Update documentation with new metrics

---

**Estimated Time**: 2 hours
**Complexity**: Medium (middleware + background collection)
**Risk**: Low-Medium (10% - path cardinality, counter deltas)
**Next Phase**: Phase 5 - Structured Logging (1h)





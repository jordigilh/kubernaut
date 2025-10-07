# Operational Standards - Kubernaut Services

**Version**: v1.0
**Last Updated**: October 6, 2025
**Status**: ✅ **STANDARDIZED**
**Scope**: All 6 Stateless HTTP Services + 5 CRD Controllers

---

## 📋 **Table of Contents**

1. [Timeout Configuration](#timeout-configuration)
2. [Graceful Shutdown](#graceful-shutdown)
3. [Comprehensive Metrics](#comprehensive-metrics)
4. [Circuit Breakers](#circuit-breakers)
5. [Caching TTL Strategy](#caching-ttl-strategy)

---

## ⏱️ **1. Timeout Configuration**

### **Standard Timeout Values**

| Operation Type | Timeout | Rationale |
|----------------|---------|-----------|
| **HTTP Request** | 30s | Standard REST API timeout |
| **Database Query** | 10s | Prevent slow query cascade |
| **External API** | 30s | LLM, Webhooks, etc. |
| **Connection Establishment** | 5s | Fail fast on connection issues |
| **Health Check** | 5s | Quick liveness/readiness response |

---

### **Go Implementation**

```go
// pkg/config/timeouts.go
package config

import "time"

const (
    HTTPRequestTimeout        = 30 * time.Second
    DatabaseQueryTimeout      = 10 * time.Second
    ExternalAPITimeout        = 30 * time.Second
    ConnectionTimeout         = 5 * time.Second
    HealthCheckTimeout        = 5 * time.Second

    // Service-specific timeouts
    LLMInvestigationTimeout   = 60 * time.Second  // HolmesGPT investigations
    VectorSearchTimeout       = 15 * time.Second  // Context API vector search
    WebhookDeliveryTimeout    = 10 * time.Second  // Notification webhooks
)
```

---

### **HTTP Client Configuration**

```go
// pkg/http/client.go
package http

import (
    "net"
    "net/http"
    "time"

    "github.com/jordigilh/kubernaut/pkg/config"
)

func NewHTTPClient() *http.Client {
    return &http.Client{
        Timeout: config.HTTPRequestTimeout,
        Transport: &http.Transport{
            DialContext: (&net.Dialer{
                Timeout:   config.ConnectionTimeout,
                KeepAlive: 30 * time.Second,
            }).DialContext,
            MaxIdleConns:          100,
            MaxIdleConnsPerHost:   10,
            IdleConnTimeout:       90 * time.Second,
            TLSHandshakeTimeout:   10 * time.Second,
            ExpectContinueTimeout: 1 * time.Second,
        },
    }
}
```

---

### **Database Configuration**

```go
// pkg/database/postgres.go
package database

import (
    "database/sql"
    "time"

    "github.com/jordigilh/kubernaut/pkg/config"
)

func NewPostgreSQLClient(dsn string) (*sql.DB, error) {
    db, err := sql.Open("postgres", dsn)
    if err != nil {
        return nil, err
    }

    // Connection pool settings
    db.SetMaxOpenConns(100)
    db.SetMaxIdleConns(10)
    db.SetConnMaxLifetime(time.Hour)
    db.SetConnMaxIdleTime(10 * time.Minute)

    // Set query timeout (application-level)
    // Use context.WithTimeout() for each query

    return db, nil
}

// Example query with timeout
func QueryWithTimeout(ctx context.Context, db *sql.DB, query string) (*sql.Rows, error) {
    ctx, cancel := context.WithTimeout(ctx, config.DatabaseQueryTimeout)
    defer cancel()

    return db.QueryContext(ctx, query)
}
```

---

### **Timeout by Service**

| Service | HTTP | Database | External API | Notes |
|---------|------|----------|--------------|-------|
| **Gateway** | 30s | 5s (Redis) | 10s (Rego) | Fast response critical |
| **Context API** | 30s | 10s (PG) | 15s (Vector) | Complex queries |
| **Data Storage** | 30s | 10s (PG) | 30s (LLM embedding) | Write operations |
| **HolmesGPT API** | 60s | N/A | 60s (LLM) | Long-running investigations |
| **Notification** | 30s | N/A | 10s (Webhooks) | Multiple channel attempts |
| **Dynamic Toolset** | 30s | N/A | 5s (Service health) | Discovery operations |

---

## 🔄 **2. Graceful Shutdown**

### **Standard Graceful Shutdown Pattern**

**SIGTERM Handler**: 30-second drain period

```go
// pkg/server/graceful.go
package server

import (
    "context"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "go.uber.org/zap"
)

type GracefulServer struct {
    server *http.Server
    logger *zap.Logger
}

func NewGracefulServer(addr string, handler http.Handler, logger *zap.Logger) *GracefulServer {
    return &GracefulServer{
        server: &http.Server{
            Addr:    addr,
            Handler: handler,
        },
        logger: logger,
    }
}

func (s *GracefulServer) Start() error {
    // Start server in goroutine
    go func() {
        s.logger.Info("Starting HTTP server", zap.String("addr", s.server.Addr))
        if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            s.logger.Fatal("Server failed", zap.Error(err))
        }
    }()

    // Wait for SIGTERM or SIGINT
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
    <-quit

    s.logger.Info("Shutting down server...")

    // 30-second graceful shutdown
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    // Stop accepting new connections
    // Complete in-flight requests
    if err := s.server.Shutdown(ctx); err != nil {
        s.logger.Error("Server forced to shutdown", zap.Error(err))
        return err
    }

    s.logger.Info("Server exited gracefully")
    return nil
}
```

---

### **Graceful Shutdown Checklist**

**During Shutdown** (30-second window):

1. ✅ **Stop accepting new requests**: HTTP server stops listening
2. ✅ **Complete in-flight requests**: Wait for active requests to finish
3. ✅ **Close database connections**: Drain connection pool
4. ✅ **Flush metrics**: Send final metrics to Prometheus
5. ✅ **Close log buffers**: Flush structured logs
6. ✅ **Cleanup resources**: Close file handles, network connections

---

### **Kubernetes Configuration**

```yaml
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      terminationGracePeriodSeconds: 30  # Match application shutdown
      containers:
      - name: service
        lifecycle:
          preStop:
            exec:
              command: ["/bin/sh", "-c", "sleep 5"]  # Delay for deregistration
```

---

### **Database Connection Cleanup**

```go
func (s *GracefulServer) shutdownDatabase(ctx context.Context, db *sql.DB) error {
    s.logger.Info("Closing database connections...")

    // Set shorter timeout for remaining queries
    db.SetConnMaxLifetime(5 * time.Second)

    // Close database (waits for in-flight queries)
    if err := db.Close(); err != nil {
        s.logger.Error("Database close error", zap.Error(err))
        return err
    }

    s.logger.Info("Database connections closed")
    return nil
}
```

---

## 📊 **3. Comprehensive Metrics**

### **Standard Metrics** (All Services)

#### **HTTP Metrics**

```go
// HTTP request counter
{service}_http_requests_total{
    method="GET",
    path="/api/v1/context",
    code="200",
    client="ai-analysis-sa"
} 1500

// HTTP request duration (histogram)
{service}_http_request_duration_seconds{
    method="GET",
    path="/api/v1/context"
} histogram

// HTTP request size (histogram)
{service}_http_request_size_bytes{
    method="POST",
    path="/api/v1/signals"
} histogram

// HTTP response size (histogram)
{service}_http_response_size_bytes{
    path="/api/v1/context"
} histogram
```

---

#### **Dependency Metrics**

```go
// Database query duration
{service}_database_query_duration_seconds{
    operation="SELECT",
    table="incident_embeddings"
} histogram

// Database connection pool
{service}_database_connections{
    state="active"
} 15
{service}_database_connections{
    state="idle"
} 5

// Cache hit/miss ratio
{service}_cache_requests_total{
    result="hit"
} 850
{service}_cache_requests_total{
    result="miss"
} 150

// External API calls
{service}_external_api_requests_total{
    endpoint="llm_provider",
    code="200"
} 100
```

---

#### **Business Logic Metrics**

```go
// Gateway Service
gateway_signals_ingested_total{
    type="prometheus",
    priority="P0"
} 500
gateway_signals_deduplicated_total{
    type="prometheus"
} 200
gateway_crd_created_total{} 300

// Context API
contextapi_vector_search_duration_seconds{} histogram
contextapi_embeddings_cached_total{} 750

// HolmesGPT API
holmesgpt_investigations_total{
    model="gpt-4",
    status="success"
} 50
holmesgpt_llm_tokens_total{
    model="gpt-4",
    type="input"
} 150000
holmesgpt_llm_cost_usd_total{} 12.50

// Notification Service
notification_sent_total{
    channel="slack",
    status="success"
} 100
```

---

### **Prometheus Instrumentation**

```go
// pkg/metrics/prometheus.go
package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    HTTPRequestsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "{service}_http_requests_total",
            Help: "Total HTTP requests",
        },
        []string{"method", "path", "code", "client"},
    )

    HTTPRequestDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "{service}_http_request_duration_seconds",
            Help:    "HTTP request duration",
            Buckets: prometheus.DefBuckets,
        },
        []string{"method", "path"},
    )

    DatabaseQueryDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "{service}_database_query_duration_seconds",
            Help:    "Database query duration",
            Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0, 5.0, 10.0},
        },
        []string{"operation", "table"},
    )
)
```

---

### **Sample PromQL Queries**

```promql
# Request rate
rate({service}_http_requests_total[5m])

# Error rate
rate({service}_http_requests_total{code=~"5.."}[5m])

# P95 latency
histogram_quantile(0.95, rate({service}_http_request_duration_seconds_bucket[5m]))

# Cache hit ratio
rate({service}_cache_requests_total{result="hit"}[5m]) / rate({service}_cache_requests_total[5m])
```

---

## 🛡️ **4. Circuit Breakers**

### **Circuit Breaker Pattern**

**Library**: `github.com/sony/gobreaker`

```go
// pkg/circuitbreaker/breaker.go
package circuitbreaker

import (
    "time"

    "github.com/sony/gobreaker"
    "go.uber.org/zap"
)

type Settings struct {
    Name          string
    MaxRequests   uint32        // Half-open: max requests to test
    Interval      time.Duration // Closed->Open: time window for failures
    Timeout       time.Duration // Open->Half-open: time before retry
    ErrorThreshold float64      // Failure rate to open (0.5 = 50%)
}

func New(settings Settings, logger *zap.Logger) *gobreaker.CircuitBreaker {
    cbSettings := gobreaker.Settings{
        Name:        settings.Name,
        MaxRequests: settings.MaxRequests,
        Interval:    settings.Interval,
        Timeout:     settings.Timeout,
        ReadyToTrip: func(counts gobreaker.Counts) bool {
            failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
            return counts.Requests >= 10 && failureRatio >= settings.ErrorThreshold
        },
        OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
            logger.Warn("Circuit breaker state change",
                zap.String("breaker", name),
                zap.String("from", from.String()),
                zap.String("to", to.String()),
            )
        },
    }

    return gobreaker.NewCircuitBreaker(cbSettings)
}
```

---

### **Circuit Breaker by Service**

| Service | Dependency | Error Threshold | Open Duration | Max Requests |
|---------|------------|-----------------|---------------|--------------|
| **Gateway** | Redis | 50% | 30s | 5 |
| **Gateway** | K8s API | 30% | 60s | 3 |
| **Context API** | PostgreSQL | 50% | 30s | 5 |
| **Context API** | Vector DB | 50% | 30s | 5 |
| **Data Storage** | PostgreSQL | 50% | 30s | 5 |
| **Data Storage** | LLM API | 30% | 60s | 3 |
| **HolmesGPT API** | LLM Provider | 20% | 120s | 2 |
| **Notification** | Slack API | 50% | 60s | 3 |
| **Notification** | Teams API | 50% | 60s | 3 |

---

### **Usage Example**

```go
// Context API: PostgreSQL circuit breaker
pgBreaker := circuitbreaker.New(circuitbreaker.Settings{
    Name:           "postgresql",
    MaxRequests:    5,
    Interval:       60 * time.Second,
    Timeout:        30 * time.Second,
    ErrorThreshold: 0.5,  // 50% failure rate
}, logger)

func (s *Service) QueryDatabase(ctx context.Context, query string) ([]Row, error) {
    result, err := pgBreaker.Execute(func() (interface{}, error) {
        return s.db.QueryContext(ctx, query)
    })

    if err != nil {
        // Circuit breaker is open or query failed
        return nil, err
    }

    return result.([]Row), nil
}
```

---

### **Circuit Breaker Metrics**

```go
{service}_circuit_breaker_state{
    breaker="postgresql"
} 0  # 0=Closed, 1=Open, 2=Half-Open

{service}_circuit_breaker_requests_total{
    breaker="postgresql",
    result="success"
} 1000

{service}_circuit_breaker_requests_total{
    breaker="postgresql",
    result="failure"
} 50
```

---

## 💾 **5. Caching TTL Strategy**

### **Cache Layers**

1. **In-Memory Cache**: Fast, limited size, per-replica
2. **Redis Cache**: Shared across replicas, distributed
3. **CDN/Edge Cache**: Static assets, API responses

---

### **Standard TTL Values**

| Cache Type | TTL | Use Case | Invalidation |
|------------|-----|----------|--------------|
| **Short-term** | 5 min | Frequent updates | Time-based |
| **Medium-term** | 30 min | Stable data | Time or event |
| **Long-term** | 24 hours | Rarely changes | Event-based |
| **Eternal** | No expiry | Immutable data | Never |

---

### **TTL by Service**

#### **Gateway Service**

```go
// Fingerprint deduplication (Redis)
cache.Set("fingerprint:{hash}", signal, 5*time.Minute)  // Short-term

// Storm detection window (Redis)
cache.ZAdd("storm:{key}", timestamp, 10*time.Minute)    // Short-term
```

---

#### **Context API Service**

```go
// Environment context (Redis)
cache.Set("env:{namespace}", context, 5*time.Minute)    // Short-term

// Historical patterns (Redis)
cache.Set("patterns:{key}", patterns, 30*time.Minute)  // Medium-term

// Success rates (Redis)
cache.Set("success:{workflow}", rate, 30*time.Minute)  // Medium-term

// Vector embeddings (immutable, Redis)
cache.Set("embedding:{id}", embedding, 24*time.Hour)   // Long-term
```

---

#### **Data Storage Service**

```go
// Embedding cache (Redis)
cache.Set("embedding:{text_hash}", vector, 5*time.Minute)  // Short-term (frequently regenerated)

// Audit trail metadata (Redis)
cache.Set("audit_meta:{id}", metadata, 30*time.Minute)    // Medium-term
```

---

#### **HolmesGPT API**

```go
// Investigation results (Redis)
cache.Set("investigation:{hash}", result, 30*time.Minute)  // Medium-term

// Toolset configuration (in-memory, reloaded from ConfigMap)
// File-based polling: 60s check interval
```

---

#### **Notification Service**

```go
// Channel availability (in-memory)
cache.Set("channel:{slack}:available", true, 1*time.Minute)  // Short-term

// Template cache (in-memory)
cache.Set("template:{name}", compiled, 24*time.Hour)         // Long-term
```

---

### **Cache Implementation**

```go
// pkg/cache/redis.go
package cache

import (
    "context"
    "time"

    "github.com/go-redis/redis/v8"
    "go.uber.org/zap"
)

type RedisCache struct {
    client *redis.Client
    logger *zap.Logger
}

func NewRedisCache(client *redis.Client, logger *zap.Logger) *RedisCache {
    return &RedisCache{
        client: client,
        logger: logger,
    }
}

func (c *RedisCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
    return c.client.Set(ctx, key, value, ttl).Err()
}

func (c *RedisCache) Get(ctx context.Context, key string) (string, error) {
    return c.client.Get(ctx, key).Result()
}

func (c *RedisCache) Delete(ctx context.Context, key string) error {
    return c.client.Del(ctx, key).Err()
}
```

---

### **Cache Invalidation Strategies**

#### **1. Time-Based (TTL)**
```go
// Automatic expiration
cache.Set(key, value, 5*time.Minute)
```

#### **2. Event-Based**
```go
// Invalidate on CRD update
func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) {
    // ... reconciliation logic ...

    // Invalidate cache
    cache.Delete(ctx, "workflow:"+req.Name)
}
```

#### **3. Write-Through**
```go
// Update cache and database atomically
func (s *Service) UpdateWorkflow(ctx context.Context, workflow *Workflow) error {
    // Write to database
    if err := s.db.Update(ctx, workflow); err != nil {
        return err
    }

    // Update cache
    return cache.Set(ctx, "workflow:"+workflow.ID, workflow, 30*time.Minute)
}
```

---

## ✅ **Operational Standards Checklist**

### **For Each Service**:

1. ✅ **Timeouts configured**: HTTP, database, external APIs
2. ✅ **Graceful shutdown**: SIGTERM handler, 30s drain
3. ✅ **Metrics instrumented**: HTTP, database, business logic
4. ✅ **Circuit breakers**: All external dependencies protected
5. ✅ **Caching strategy**: TTL values documented and implemented
6. ✅ **Monitoring**: Prometheus alerts for timeouts, shutdowns, circuit breakers
7. ✅ **Documentation**: All standards referenced in service specs

---

## 📚 **Related Documentation**

- [RATE_LIMITING_STANDARD.md](./RATE_LIMITING_STANDARD.md) - Rate limiting patterns
- [ERROR_RESPONSE_STANDARD.md](./ERROR_RESPONSE_STANDARD.md) - Error handling
- [HEALTH_CHECK_STANDARD.md](./HEALTH_CHECK_STANDARD.md) - Health checks
- [PROMETHEUS_SERVICEMONITOR_PATTERN.md](./PROMETHEUS_SERVICEMONITOR_PATTERN.md) - Metrics collection

---

**Document Status**: ✅ Complete
**Compliance**: All services covered
**Last Updated**: October 6, 2025
**Version**: 1.0

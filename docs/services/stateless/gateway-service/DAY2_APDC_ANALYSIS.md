# Day 2: HTTP Server Implementation - APDC Analysis Phase

**Date**: 2025-10-22
**Duration**: 45 minutes
**Objective**: Analyze existing HTTP server patterns for Gateway implementation

---

## 🔍 **Existing Patterns Discovered**

### **Primary Reference: Context API Server** (`pkg/contextapi/server/server.go`)

**Architecture Pattern**:
```go
type Server struct {
    // HTTP infrastructure
    router     *chi.Mux      // Chi router for routing
    httpServer *http.Server  // Standard HTTP server

    // Business components
    cachedExecutor *query.CachedExecutor // Business logic
    dbClient       client.Client         // Database
    cacheManager   cache.CacheManager    // Cache

    // Observability
    metrics *metrics.Metrics  // Prometheus metrics
    logger  *zap.Logger       // Structured logging
}
```

### **Middleware Stack** (Context API Pattern)
```go
func (s *Server) Handler() http.Handler {
    r := chi.NewRouter()

    // Standard middleware (chi built-ins)
    r.Use(middleware.RequestID)   // Request tracing
    r.Use(middleware.RealIP)      // Real IP extraction
    r.Use(s.loggingMiddleware)    // Custom: Logging + metrics
    r.Use(middleware.Recoverer)   // Panic recovery
    r.Use(cors.Handler(...))      // CORS configuration

    return r
}
```

### **Logging Middleware Pattern**
```go
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()

        // Wrap response writer to capture status code
        ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

        next.ServeHTTP(ww, r)

        // Record metrics AFTER request completes
        duration := time.Since(start).Seconds()
        s.metrics.RecordHTTPRequest(r.Method, r.URL.Path, strconv.Itoa(ww.Status()), duration)

        // Log request
        s.logger.Info("HTTP request",
            zap.String("method", r.Method),
            zap.String("path", r.URL.Path),
            zap.Int("status", ww.Status()),
            zap.Duration("duration", time.Since(start)),
        )
    })
}
```

### **Health Check Pattern**
```go
// Simple liveness (always returns 200 OK)
r.Get("/health/live", s.handleLiveness)

// Readiness with dependency checks
func (s *Server) handleReadiness(w http.ResponseWriter, r *http.Request) {
    // Check database
    dbHealthy := s.dbClient.Ping(r.Context()) == nil

    // Check cache
    cacheHealthy := true // TODO: cache.Ping()

    // Return 503 if any dependency unhealthy
    if !dbHealthy || !cacheHealthy {
        s.respondJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
            "database": dbReady,
            "cache": cacheReady,
        })
        return
    }

    s.respondJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
```

### **Metrics Endpoint**
```go
r.Handle("/metrics", promhttp.Handler()) // Standard Prometheus handler
```

### **Server Lifecycle**
```go
// Start (blocking)
func (s *Server) Start() error {
    s.httpServer.Handler = s.Handler() // Assign chi router
    return s.httpServer.ListenAndServe()
}

// Shutdown (graceful)
func (s *Server) Shutdown(ctx context.Context) error {
    return s.httpServer.Shutdown(ctx)
}

// Handler (for testing with httptest.NewServer)
func (s *Server) Handler() http.Handler {
    return r // Chi router
}
```

---

## 🎯 **Gateway-Specific Requirements**

### **Webhook Endpoints** (Gateway-specific)
```go
// Prometheus AlertManager webhook
POST /webhook/prometheus
Content-Type: application/json

// Kubernetes Event webhook
POST /webhook/k8s-event
Content-Type: application/json
```

### **Expected HTTP Status Codes**
- **201 Created**: CRD created successfully
- **202 Accepted**: Duplicate signal (deduplicated)
- **400 Bad Request**: Invalid webhook payload (JSON parse error, missing fields)
- **500 Internal Server Error**: CRD creation failure, Redis error, K8s API error

### **Response Format**
```go
type WebhookResponse struct {
    Status      string `json:"status"`       // "created", "duplicate", "error"
    RequestID   string `json:"request_id"`   // From middleware.RequestID
    Fingerprint string `json:"fingerprint"`  // Signal fingerprint (for dedup tracking)
    Message     string `json:"message"`      // Human-readable status
}
```

---

## 📊 **Technical Context**

### **Dependencies Already Available**
- ✅ `github.com/go-chi/chi/v5` - Router (go.mod confirmed)
- ✅ `github.com/go-chi/chi/v5/middleware` - Built-in middleware
- ✅ `github.com/prometheus/client_golang/prometheus/promhttp` - Metrics endpoint
- ✅ `github.com/sirupsen/logrus` - Structured logging (Day 1 choice)
- ✅ `sigs.k8s.io/controller-runtime/pkg/client` - K8s client (Day 1 integration)

### **Integration Points**
1. **Adapters** (`pkg/gateway/adapters/`): Parse webhook payloads
2. **Processing Pipeline** (`pkg/gateway/processing/`): Classify, assign priority, detect storms
3. **CRD Creator** (`pkg/gateway/processing/crd_creator.go`): Create RemediationRequest CRDs
4. **Metrics** (to be created): Record webhook counts, processing latency, CRD creation success/failure

---

## 🚨 **Business Requirements Coverage (Day 2 Focus)**

### **Primary BRs**
- **BR-GATEWAY-017**: HTTP webhook endpoints for Prometheus and K8s Events
- **BR-GATEWAY-018**: Request validation (JSON parsing, required fields)
- **BR-GATEWAY-019**: Error handling with appropriate HTTP status codes
- **BR-GATEWAY-023**: Request logging with tracing (RequestID)
- **BR-GATEWAY-024**: Health endpoints for Kubernetes probes

### **Deferred BRs** (Days 3-6)
- BR-GATEWAY-066 to BR-GATEWAY-075: Authentication/Authorization (Day 6)
- BR-GATEWAY-016: Metrics (Day 7, basic counter today)

---

## 📋 **Analysis Deliverables Checklist**

- [x] **Business Context**: HTTP server enables webhook ingestion (BR-GATEWAY-017 through BR-GATEWAY-024)
- [x] **Existing Patterns**: Context API server provides proven architecture
- [x] **Integration Points**: Adapters, processing pipeline, CRD creator integration clear
- [x] **Complexity Assessment**: SIMPLE (following established Context API patterns)
- [x] **Risk Level**: LOW (proven patterns, no novel architecture)

---

## ✅ **ANALYSIS PHASE COMPLETE**

**Confidence**: 95%

**Justification**:
- ✅ Context API server provides complete reference implementation
- ✅ All dependencies available in go.mod
- ✅ Integration points with Day 1 components clear
- ✅ HTTP patterns well-understood (chi router, middleware, handlers)
- ⚠️ Minor risk: Webhook-specific error handling (5% uncertainty)

**Recommended Approach**: Enhance Context API pattern with Gateway-specific webhook handlers

---

**Next Phase**: PLAN (Design TDD strategy, handler architecture, test scenarios)




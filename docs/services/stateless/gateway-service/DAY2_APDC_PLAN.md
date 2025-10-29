# Day 2: HTTP Server Implementation - APDC Plan Phase

**Date**: 2025-10-22
**Duration**: 60 minutes
**Objective**: Design HTTP server architecture with comprehensive TDD strategy

---

## 🎯 **Implementation Strategy**

### **TDD Approach: RED → GREEN → REFACTOR**

**Phase Breakdown**:
1. **DO-RED** (2 hours): Write 20-25 unit tests for handlers, middleware, server lifecycle
2. **DO-GREEN** (3 hours): Minimal implementation to pass tests (basic routing, simple handlers)
3. **DO-REFACTOR** (2 hours): Enhanced error handling, metrics integration, structured responses

**Target Coverage**: 85%+ test coverage, 90%+ confidence

---

## 🏗️ **Server Architecture Design**

### **Server Struct** (`pkg/gateway/server/server.go`)

```go
package server

import (
    "context"
    "net/http"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/sirupsen/logrus"
    "sigs.k8s.io/controller-runtime/pkg/client"

    "github.com/jordigilh/kubernaut/pkg/gateway/adapters"
    "github.com/jordigilh/kubernaut/pkg/gateway/processing"
)

// Server is the Gateway HTTP server
// BR-GATEWAY-017: Webhook HTTP endpoints
type Server struct {
    // HTTP infrastructure
    router     *chi.Mux      // Chi router for routing
    httpServer *http.Server  // Standard HTTP server

    // Gateway components (from Day 1)
    adapterRegistry  *adapters.AdapterRegistry      // Adapter management
    classifier       *processing.EnvironmentClassifier  // Environment classification
    priorityEngine   *processing.PriorityEngine     // Priority assignment
    pathDecider      *processing.RemediationPathDecider // Path selection
    crdCreator       *processing.CRDCreator         // CRD creation

    // Observability
    logger *logrus.Logger // Structured logging

    // Metrics (basic for Day 2, enhanced in Day 7)
    webhookRequestsTotal    prometheus.Counter   // Total webhook requests
    webhookErrorsTotal      prometheus.Counter   // Total webhook errors
    crdCreationTotal        prometheus.Counter   // Total CRDs created
    webhookProcessingSeconds prometheus.Histogram // Webhook processing latency
}

// Config contains server configuration
type Config struct {
    Port         int
    ReadTimeout  time.Duration
    WriteTimeout time.Duration
}
```

### **Constructor** (`NewServer`)

```go
// NewServer creates a new Gateway HTTP server
// BR-GATEWAY-017, BR-GATEWAY-023, BR-GATEWAY-024
func NewServer(
    adapterRegistry *adapters.AdapterRegistry,
    classifier *processing.EnvironmentClassifier,
    priorityEngine *processing.PriorityEngine,
    pathDecider *processing.RemediationPathDecider,
    crdCreator *processing.CRDCreator,
    logger *logrus.Logger,
    cfg *Config,
) *Server {
    s := &Server{
        adapterRegistry: adapterRegistry,
        classifier:      classifier,
        priorityEngine:  priorityEngine,
        pathDecider:     pathDecider,
        crdCreator:      crdCreator,
        logger:          logger,
        httpServer: &http.Server{
            Addr:         fmt.Sprintf(":%d", cfg.Port),
            ReadTimeout:  cfg.ReadTimeout,
            WriteTimeout: cfg.WriteTimeout,
        },
    }

    // Initialize metrics
    s.initMetrics()

    return s
}

// initMetrics creates Prometheus metrics
func (s *Server) initMetrics() {
    s.webhookRequestsTotal = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "gateway_webhook_requests_total",
        Help: "Total number of webhook requests received",
    })
    prometheus.MustRegister(s.webhookRequestsTotal)

    // ... other metrics
}
```

---

## 🛣️ **Route Design**

### **Webhook Endpoints**

```go
func (s *Server) setupRoutes(r chi.Router) {
    // Health checks (BR-GATEWAY-024)
    r.Get("/health", s.handleHealth)
    r.Get("/health/ready", s.handleReadiness)
    r.Get("/health/live", s.handleLiveness)

    // Metrics endpoint (BR-GATEWAY-016 - basic)
    r.Handle("/metrics", promhttp.Handler())

    // Webhook API (BR-GATEWAY-017)
    r.Route("/webhook", func(r chi.Router) {
        // Prometheus AlertManager webhook
        r.Post("/prometheus", s.handlePrometheusWebhook)

        // Kubernetes Event webhook
        r.Post("/k8s-event", s.handleKubernetesEventWebhook)
    })
}
```

### **Middleware Stack**

```go
func (s *Server) Handler() http.Handler {
    r := chi.NewRouter()

    // Standard chi middleware
    r.Use(middleware.RequestID)      // BR-GATEWAY-023: Request tracing
    r.Use(middleware.RealIP)         // Real IP extraction
    r.Use(s.loggingMiddleware)       // Custom: Logging + metrics
    r.Use(middleware.Recoverer)      // Panic recovery (BR-GATEWAY-019)
    r.Use(middleware.Timeout(60 * time.Second)) // Request timeout

    // Setup routes
    s.setupRoutes(r)

    return r
}
```

---

## 📝 **Handler Implementation Design**

### **Prometheus Webhook Handler**

```go
// handlePrometheusWebhook processes Prometheus AlertManager webhooks
// BR-GATEWAY-001, BR-GATEWAY-017, BR-GATEWAY-018, BR-GATEWAY-019
func (s *Server) handlePrometheusWebhook(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    requestID := middleware.GetReqID(ctx)

    // Increment request counter
    s.webhookRequestsTotal.Inc()

    // Read request body
    body, err := io.ReadAll(r.Body)
    if err != nil {
        s.respondError(w, http.StatusBadRequest, "failed to read request body", requestID, err)
        return
    }
    defer r.Body.Close()

    // Get Prometheus adapter
    adapter, err := s.adapterRegistry.Get("prometheus-adapter")
    if err != nil {
        s.respondError(w, http.StatusInternalServerError, "adapter not found", requestID, err)
        return
    }

    // Parse webhook (BR-GATEWAY-001)
    signal, err := adapter.Parse(ctx, body)
    if err != nil {
        // BR-GATEWAY-018: Validation failure → 400 Bad Request
        s.respondError(w, http.StatusBadRequest, "invalid webhook payload", requestID, err)
        return
    }

    // Process signal through pipeline
    environment, _ := s.classifier.Classify(ctx, signal)
    priority := s.priorityEngine.AssignPriority(ctx, signal, environment)

    signalCtx := &processing.SignalContext{
        Signal:      signal,
        Environment: environment,
        Priority:    priority,
    }
    remediationPath := s.pathDecider.DeterminePath(ctx, signalCtx)

    // Create RemediationRequest CRD (BR-GATEWAY-015)
    rr, err := s.crdCreator.Create(ctx, signal, environment, priority, remediationPath)
    if err != nil {
        // BR-GATEWAY-019: CRD creation failure → 500 Internal Server Error
        s.respondError(w, http.StatusInternalServerError, "failed to create remediation request", requestID, err)
        s.webhookErrorsTotal.Inc()
        return
    }

    // Success: CRD created
    s.crdCreationTotal.Inc()
    s.respondJSON(w, http.StatusCreated, map[string]interface{}{
        "status":      "created",
        "request_id":  requestID,
        "fingerprint": signal.Fingerprint,
        "crd_name":    rr.Name,
        "namespace":   rr.Namespace,
        "priority":    priority,
        "environment": environment,
        "message":     "RemediationRequest CRD created successfully",
    })
}
```

### **Kubernetes Event Handler**

```go
// handleKubernetesEventWebhook processes Kubernetes Event webhooks
// BR-GATEWAY-002, BR-GATEWAY-017, BR-GATEWAY-018, BR-GATEWAY-019
func (s *Server) handleKubernetesEventWebhook(w http.ResponseWriter, r *http.Request) {
    // Similar structure to handlePrometheusWebhook
    // Uses "kubernetes-event-adapter" instead
}
```

---

## 📋 **Response Format Design**

### **Success Response** (201 Created)
```json
{
  "status": "created",
  "request_id": "req-abc123",
  "fingerprint": "sha256-xyz789...",
  "crd_name": "rr-xyz789ab",
  "namespace": "production",
  "priority": "P0",
  "environment": "production",
  "message": "RemediationRequest CRD created successfully"
}
```

### **Duplicate Response** (202 Accepted) - Day 3
```json
{
  "status": "duplicate",
  "request_id": "req-abc123",
  "fingerprint": "sha256-xyz789...",
  "message": "Signal already processed (deduplicated)"
}
```

### **Error Response** (400 Bad Request)
```json
{
  "status": "error",
  "request_id": "req-abc123",
  "error": "invalid webhook payload",
  "details": "missing required field: alertname",
  "code": "VALIDATION_ERROR"
}
```

### **Error Response** (500 Internal Server Error)
```json
{
  "status": "error",
  "request_id": "req-abc123",
  "error": "failed to create remediation request",
  "details": "kubernetes API error: connection refused",
  "code": "CRD_CREATION_ERROR"
}
```

---

## 🧪 **TDD Test Strategy**

### **Test Categories**

#### **1. Handler Tests** (`test/unit/gateway/server/handlers_test.go`)
**Coverage**: 15-20 tests

**Prometheus Webhook Tests**:
- ✅ Valid Prometheus webhook → 201 Created
- ✅ Invalid JSON → 400 Bad Request
- ✅ Missing required fields → 400 Bad Request
- ✅ Adapter not found → 500 Internal Server Error
- ✅ CRD creation failure → 500 Internal Server Error
- ✅ Response includes request_id, fingerprint, CRD name
- ✅ Metrics incremented correctly

**Kubernetes Event Webhook Tests**:
- ✅ Valid K8s Event → 201 Created
- ✅ Normal event filtered → 400 Bad Request
- ✅ Missing involvedObject → 400 Bad Request
- ✅ Response includes correct fields

#### **2. Middleware Tests** (`test/unit/gateway/server/middleware_test.go`)
**Coverage**: 8-10 tests

- ✅ Request ID added to context
- ✅ Logging middleware logs request details
- ✅ Logging middleware records metrics
- ✅ Panic recovery catches panics → 500
- ✅ Timeout middleware enforces 60s limit

#### **3. Server Lifecycle Tests** (`test/unit/gateway/server/server_test.go`)
**Coverage**: 5-8 tests

- ✅ Server starts successfully
- ✅ Server shutdown graceful
- ✅ Health endpoint returns 200 OK
- ✅ Readiness endpoint checks dependencies
- ✅ Metrics endpoint returns Prometheus format

---

## 📦 **File Structure Plan**

```
pkg/gateway/
├── server/
│   ├── server.go          # Server struct, lifecycle (Start, Shutdown)
│   ├── handlers.go        # Webhook handlers
│   ├── middleware.go      # Logging middleware
│   ├── responses.go       # JSON response helpers
│   └── health.go          # Health check handlers

test/unit/gateway/server/
├── suite_test.go          # Ginkgo suite setup
├── server_test.go         # Server lifecycle tests (5-8 tests)
├── handlers_test.go       # Webhook handler tests (15-20 tests)
├── middleware_test.go     # Middleware tests (8-10 tests)
└── health_test.go         # Health check tests (3-5 tests)
```

---

## ✅ **Success Criteria**

### **Functional Requirements**
- [x] HTTP server starts on configured port
- [x] Webhooks accepted at `/webhook/prometheus` and `/webhook/k8s-event`
- [x] Valid webhooks create RemediationRequest CRDs
- [x] Invalid webhooks return 400 Bad Request
- [x] CRD creation failures return 500 Internal Server Error
- [x] Health endpoints responsive
- [x] Metrics endpoint exports Prometheus metrics

### **Quality Requirements**
- [x] 85%+ test coverage
- [x] Zero linter errors
- [x] All tests pass
- [x] TDD methodology followed (RED → GREEN → REFACTOR)
- [x] BR references in code comments

### **Integration Requirements**
- [x] Integrates with Day 1 adapters
- [x] Integrates with Day 1 processing pipeline
- [x] Integrates with Day 1 CRD creator
- [x] Request ID propagates through entire pipeline

---

## 📊 **Estimated Effort**

| Phase | Duration | Deliverables |
|-------|----------|--------------|
| **DO-RED** | 2 hours | 25-30 unit tests written (failing) |
| **DO-GREEN** | 3 hours | Minimal implementation (tests passing) |
| **DO-REFACTOR** | 2 hours | Enhanced error handling, metrics, logging |
| **Validation** | 1 hour | Build, lint, test verification |
| **TOTAL** | **8 hours** | HTTP server complete |

---

## 🎯 **Risk Assessment**

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| K8s client mocking in tests | Medium | Medium | Use fake client from Day 1 tests |
| Panic recovery testing | Low | Low | Ginkgo has panic testing support |
| Metrics registration conflicts | Low | Medium | Unique metric names with `gateway_` prefix |
| Request body reading errors | Low | Low | Standard Go io.ReadAll pattern |

**Overall Risk**: LOW (90% confidence)

---

## ✅ **PLAN PHASE COMPLETE**

**Confidence**: 90%

**Justification**:
- ✅ Architecture follows proven Context API pattern
- ✅ TDD strategy clear with 25-30 test scenarios
- ✅ Integration points with Day 1 components well-defined
- ✅ Response formats standardized
- ✅ Error handling strategy comprehensive
- ⚠️ Minor risk: Fake K8s client setup in tests (10% uncertainty)

**Approved Approach**: Enhance Context API server pattern with Gateway-specific webhook handlers

---

**Next Phase**: DO-RED (Write 25-30 unit tests for server, handlers, middleware)




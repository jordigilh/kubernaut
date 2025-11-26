# DD-HTTP-001: HTTP Router Strategy for Kubernaut Services

## Status
**✅ APPROVED** (2025-11-22)
**Last Reviewed**: 2025-11-22
**Confidence**: 95%

---

## Context & Problem

Kubernaut consists of multiple service types with different HTTP routing needs:
1. **REST API Services** (Data Storage, future services) - Full REST APIs with CRUD operations, URL parameters, route grouping
2. **Webhook Receivers** (Gateway) - Simple webhook endpoints with minimal routing
3. **CRD Controllers** (Toolset, Notification, etc.) - Only health/metrics endpoints

**Problem**: Without a documented router strategy, services were inconsistent:
- Data Storage used `chi` (external dependency)
- Gateway and Toolset used `http.NewServeMux()` (stdlib)
- No clear guidance on which to use for new services

**Key Requirements**:
- REST API services need URL parameters (`/incidents/{id}`)
- REST API services need route grouping (`/api/v1/*`)
- REST API services have 20+ endpoints
- Webhook receivers need simple handlers
- CRD controllers only need health/metrics endpoints
- All services need minimal dependencies

---

## Alternatives Considered

### Alternative 1: `http.NewServeMux()` for All Services

**Approach**: Use Go standard library for all HTTP routing

**Pros**:
- ✅ Zero external dependencies
- ✅ Stable across Go versions
- ✅ Simple for webhook receivers and CRD controllers

**Cons**:
- ❌ Too verbose for REST APIs with 20+ endpoints
- ❌ No built-in URL parameter extraction
- ❌ No route grouping - manual prefix handling
- ❌ Poor developer experience for complex REST APIs

**Example Pain Point**:
```go
// Manual URL parameter parsing
mux.HandleFunc("/incidents/", func(w http.ResponseWriter, r *http.Request) {
    parts := strings.Split(r.URL.Path, "/")
    if len(parts) < 4 {
        http.Error(w, "Not found", 404)
        return
    }
    id := parts[3] // Manual extraction
    // ... handle request
})
```

**Confidence**: 30% (rejected for REST APIs - suitable only for simple services)

---

### Alternative 2: `chi` for All Services

**Approach**: Use `chi` router for all Kubernaut services

**Pros**:
- ✅ Consistent across all services
- ✅ URL parameters and route grouping
- ✅ Lightweight (15KB)

**Cons**:
- ❌ Overkill for CRD controllers (only 2-3 endpoints)
- ❌ Adds unnecessary dependency to simple services

**Confidence**: 40% (rejected - unnecessary for CRD controllers with only health/metrics)

---

### Alternative 3: Service-Specific Router Strategy ⭐ **RECOMMENDED**

**Approach**: Choose router based on service type and complexity

**Router Selection Matrix**:

| Service Type | Router | Rationale |
|--------------|--------|-----------|
| **REST API Services** | `chi` | Purpose-built for REST, URL params, route grouping, 10+ endpoints |
| **Webhook API Services** | `chi` | Multiple webhooks, route grouping, middleware per group, 5+ endpoints |
| **CRD Controllers** | `http.NewServeMux()` | Only health/metrics endpoints (2-3 total) |

**Pros**:
- ✅ Right tool for each use case
- ✅ Minimal dependencies where possible
- ✅ Excellent DX for complex REST APIs and webhook APIs
- ✅ Simple stdlib for simple services (health/metrics only)
- ✅ Clear decision criteria for new services
- ✅ Route grouping for webhook APIs with multiple adapters

**Cons**:
- ⚠️ Two patterns to learn (but well-documented)
- ⚠️ One external dependency for API services (justified by 15KB size)

**Confidence**: 95% (approved)

---

## Decision

**APPROVED: Alternative 3** - Service-Specific Router Strategy

### Router Selection Rules

#### Go Services

##### Use `chi` (go-chi/chi/v5) When:
- ✅ Service is a REST API with CRUD operations
- ✅ Service is a webhook API with multiple adapters/sources
- ✅ Service has 5+ HTTP endpoints
- ✅ Service needs URL parameters (`/resource/{id}`)
- ✅ Service needs route grouping (`/api/v1/*`)
- ✅ Service has OpenAPI specification (ADR-031)
- ✅ Service needs middleware per route group

**Examples:**
- Data Storage Service (20+ REST endpoints)
- Gateway Service (8+ webhook endpoints planned)
- Future REST API or webhook API services

##### Use `http.NewServeMux()` (stdlib) When:
- ✅ Service is a CRD controller (health/metrics only)
- ✅ Service has simple routing needs (2-3 endpoints)
- ✅ No URL parameters needed
- ✅ No route grouping needed

**Examples:**
- Toolset Service (CRD controller)
- Notification Service (CRD controller)

#### Python Services

##### Use `FastAPI` (Python Best Practice)
- ✅ All Python REST API services
- ✅ Automatic OpenAPI generation
- ✅ Built-in validation and serialization
- ✅ Async support
- ✅ Industry standard for Python APIs

**Examples:**
- HolmesGPT API Service (REST API)
- Future Python microservices (e.g., Embedding Service)

---

## Implementation

### REST API Service Pattern (chi)

**File**: `pkg/{service}/server/server.go`

```go
package server

import (
    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"
    "github.com/go-chi/cors"
)

func (s *Server) Handler() http.Handler {
    r := chi.NewRouter()

    // Middleware
    r.Use(middleware.RequestID)
    r.Use(middleware.RealIP)
    r.Use(s.loggingMiddleware)
    r.Use(cors.Handler(cors.Options{
        AllowedOrigins: []string{"*"},
        AllowedMethods: []string{"GET", "POST", "OPTIONS"},
    }))

    // Health endpoints
    r.Get("/health", s.handleHealth)
    r.Get("/health/ready", s.handleReadiness)
    r.Get("/health/live", s.handleLiveness)

    // Metrics
    r.Handle("/metrics", promhttp.Handler())

    // REST API routes (chi features: route grouping, URL params)
    r.Route("/api/v1", func(r chi.Router) {
        r.Get("/resources", s.handler.ListResources)
        r.Get("/resources/{id}", s.handler.GetResource)      // URL parameter
        r.Post("/resources", s.handler.CreateResource)
        r.Put("/resources/{id}", s.handler.UpdateResource)   // URL parameter
        r.Delete("/resources/{id}", s.handler.DeleteResource) // URL parameter
    })

    return r
}

// Handler with URL parameter extraction
func (h *Handler) GetResource(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id") // Built-in parameter extraction
    // ... handle request
}
```

---

### Webhook API Service Pattern (chi)

**File**: `pkg/{service}/server/server.go`

```go
package server

import (
    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"
)

func (s *Server) setupRoutes() chi.Router {
    r := chi.NewRouter()

    // Middleware
    r.Use(middleware.RequestID)
    r.Use(middleware.RealIP)
    r.Use(s.loggingMiddleware)

    // Health endpoints
    r.Get("/health", s.healthHandler)
    r.Get("/ready", s.readinessHandler)

    // Metrics
    r.Handle("/metrics", promhttp.Handler())

    // Webhook routes (chi features: route grouping, middleware per group)
    r.Route("/api/v1/signals", func(r chi.Router) {
        // Middleware applied to all webhook endpoints
        r.Use(middleware.ValidateContentType)
        r.Use(s.webhookMetricsMiddleware)

        // Dynamic adapter registration
        r.Post("/prometheus", s.prometheusHandler)
        r.Post("/kubernetes-event", s.k8sEventHandler)
        r.Post("/grafana", s.grafanaHandler)
        // ... more adapters
    })

    return r
}

// RegisterAdapter dynamically adds webhook routes
func (s *Server) RegisterAdapter(adapter Adapter) error {
    // Routes are registered in setupRoutes() or dynamically via:
    // s.router.Post(adapter.GetRoute(), s.createAdapterHandler(adapter))
    return nil
}
```

---

### CRD Controller Pattern (stdlib)

**File**: `pkg/{service}/server/server.go`

```go
package server

import (
    "net/http"
)

func (s *Server) setupRoutes() *http.ServeMux {
    mux := http.NewServeMux()

    // Health endpoints
    mux.HandleFunc("/health", s.healthHandler)
    mux.HandleFunc("/ready", s.readinessHandler)

    // Metrics
    mux.Handle("/metrics", promhttp.Handler())

    return mux
}

// Wrap with custom middleware
func (s *Server) wrapWithMiddleware(handler http.Handler) http.Handler {
    handler = s.loggingMiddleware(handler)
    handler = middleware.RequestIDMiddleware(s.logger)(handler)
    return handler
}
```

---

## Consequences

### Positive

- ✅ **Clear decision criteria** for new services
- ✅ **Optimal DX** for REST APIs (chi) and simple services (stdlib)
- ✅ **Minimal dependencies** where possible
- ✅ **Consistent patterns** within service types
- ✅ **Industry-standard** approach (chi is widely used for Go REST APIs)
- ✅ **Matches Python pattern** (HolmesGPT uses FastAPI for REST)

### Negative

- ⚠️ **Two patterns to learn** - but well-documented with clear rules
  - **Mitigation**: This document provides clear selection criteria
- ⚠️ **One external dependency** for REST services (chi)
  - **Mitigation**: chi is lightweight (15KB), well-maintained (18k+ stars), and stdlib-compatible

### Neutral

- 🔄 **Existing services unchanged** - Gateway/Toolset keep stdlib, Data Storage keeps chi
- 🔄 **Future services** follow documented rules

---

## Validation Results

### Current Service Alignment

| Service | Type | Language | Router | Status |
|---------|------|----------|--------|--------|
| Data Storage | REST API | Go | `chi` | ✅ Correct |
| HolmesGPT API | REST API | Python | `FastAPI` | ✅ Correct |
| Gateway | Webhook API | Go | `http.NewServeMux()` | ⚠️ Should migrate to `chi` |
| Toolset | CRD Controller | Go | `http.NewServeMux()` | ✅ Correct |
| Notification | CRD Controller | Go | `http.NewServeMux()` | ✅ Correct |

**Result**: Gateway needs migration to `chi` for webhook API pattern consistency

---

## Related Decisions

- **ADR-031**: OpenAPI Specification Standard (REST APIs use OpenAPI)
- **ADR-036**: Authentication Strategy (network policies, not router-level auth)
- **DD-HOLMESGPT-012**: Minimal Internal Service Architecture (REST API design)

---

## Review & Evolution

### When to Revisit

- If a new service type emerges with different routing needs
- If `chi` is archived or unmaintained
- If Go stdlib adds URL parameter support to `http.NewServeMux()`
- If REST API services consistently need features chi doesn't provide

### Success Metrics

- New services follow documented router selection rules
- No confusion about which router to use
- Minimal dependency footprint across all services
- Excellent developer experience for REST API development

---

## Quick Reference

### Decision Tree for New Services

```
What language is the service?
├─ Python → Use FastAPI (Python best practice)
│  └─ Examples: HolmesGPT API, Embedding Service
└─ Go → What type of service?
   ├─ REST API (5+ endpoints) → Use chi
   │  └─ Examples: Data Storage
   ├─ Webhook API (5+ endpoints) → Use chi
   │  └─ Examples: Gateway
   └─ CRD Controller (health/metrics only) → Use http.NewServeMux()
      └─ Examples: Toolset, Notification
```

### Key Insight

**chi is the right tool for Go REST APIs and webhook APIs** - it's purpose-built for HTTP APIs, lightweight (15KB), and provides essential features (URL parameters, route grouping, middleware per group) that would require significant boilerplate with stdlib. For simple CRD controllers (health/metrics only), stdlib is sufficient and preferred.

**FastAPI is the right tool for Python APIs** - it's the industry standard for Python REST APIs with automatic OpenAPI generation, built-in validation, and async support.

---

## References

- [chi GitHub](https://github.com/go-chi/chi) - 18k+ stars, active maintenance
- [FastAPI](https://fastapi.tiangolo.com/) - Python equivalent (used by HolmesGPT API)
- [Go http.ServeMux](https://pkg.go.dev/net/http#ServeMux) - Standard library router


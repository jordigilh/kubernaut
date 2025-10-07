# Dynamic Toolset Service - REST API Specification

**Version**: v1.0
**Last Updated**: October 6, 2025
**Base URL**: `http://dynamic-toolset.kubernaut-system:8080`
**Authentication**: Bearer Token (Kubernetes ServiceAccount)

---

## Table of Contents

1. [API Overview](#api-overview)
2. [Toolset Management API](#toolset-management-api)
3. [Service Discovery API](#service-discovery-api)
4. [Health & Metrics](#health--metrics)
5. [Error Responses](#error-responses)

---

## API Overview

### Base URL
```
http://dynamic-toolset.kubernaut-system:8080
```

### API Version
All endpoints are prefixed with `/api/v1/`

### Content Type
```
Content-Type: application/json
```

### Rate Limiting
- **Per Service**: 100 requests/second
- **Burst**: 150 requests
- **Response Header**: `X-RateLimit-Remaining: 99`

---

## Toolset Management API

### List Discovered Toolsets

**Purpose**: Retrieve all currently configured HolmesGPT toolsets

#### Request

```
GET /api/v1/toolsets
```

#### Query Parameters

| Parameter | Type | Required | Description | Example |
|-----------|------|----------|-------------|---------|
| `enabled` | boolean | No | Filter by enabled status | `true` |
| `healthy` | boolean | No | Filter by health status | `true` |

#### Example Request

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "http://dynamic-toolset:8080/api/v1/toolsets?enabled=true&healthy=true"
```

#### Response (200 OK)

```json
{
  "toolsets": [
    {
      "name": "kubernetes",
      "type": "kubernetes",
      "enabled": true,
      "healthy": true,
      "config": {
        "incluster": true,
        "namespaces": ["*"]
      },
      "discoveredAt": "2025-10-06T10:00:00Z",
      "lastHealthCheck": "2025-10-06T10:15:00Z"
    },
    {
      "name": "prometheus",
      "type": "prometheus",
      "enabled": true,
      "healthy": true,
      "config": {
        "url": "http://prometheus.monitoring:9090",
        "timeout": "30s"
      },
      "serviceEndpoint": "prometheus.monitoring:9090",
      "discoveredAt": "2025-10-06T10:00:00Z",
      "lastHealthCheck": "2025-10-06T10:15:00Z"
    },
    {
      "name": "grafana",
      "type": "grafana",
      "enabled": true,
      "healthy": true,
      "config": {
        "url": "http://grafana.monitoring:3000",
        "apiKey": "${GRAFANA_API_KEY}"
      },
      "serviceEndpoint": "grafana.monitoring:3000",
      "discoveredAt": "2025-10-06T10:00:00Z",
      "lastHealthCheck": "2025-10-06T10:15:00Z"
    }
  ],
  "total": 3,
  "configMapVersion": "12345",
  "lastDiscovery": "2025-10-06T10:00:00Z"
}
```

#### Go Implementation

```go
// pkg/dynamictoolset/handlers/toolsets.go
package handlers

import (
    "encoding/json"
    "net/http"
    "strconv"

    "go.uber.org/zap"

    "github.com/jordigilh/kubernaut/pkg/dynamictoolset/models"
    "github.com/jordigilh/kubernaut/pkg/dynamictoolset/services"
    "github.com/jordigilh/kubernaut/pkg/correlation"
)

type ToolsetHandler struct {
    toolsetService *services.ToolsetService
    logger         *zap.Logger
}

func (h *ToolsetHandler) ListToolsets(w http.ResponseWriter, r *http.Request) {
    correlationID := correlation.FromContext(r.Context())
    log := h.logger.With(
        zap.String("correlation_id", correlationID),
        zap.String("handler", "ListToolsets"),
    )

    // Parse query parameters
    var enabledFilter *bool
    if enabledStr := r.URL.Query().Get("enabled"); enabledStr != "" {
        enabled, err := strconv.ParseBool(enabledStr)
        if err != nil {
            log.Warn("Invalid enabled parameter", zap.Error(err))
            http.Error(w, "Invalid enabled parameter", http.StatusBadRequest)
            return
        }
        enabledFilter = &enabled
    }

    var healthyFilter *bool
    if healthyStr := r.URL.Query().Get("healthy"); healthyStr != "" {
        healthy, err := strconv.ParseBool(healthyStr)
        if err != nil {
            log.Warn("Invalid healthy parameter", zap.Error(err))
            http.Error(w, "Invalid healthy parameter", http.StatusBadRequest)
            return
        }
        healthyFilter = &healthy
    }

    log.Info("Fetching toolsets",
        zap.Any("enabled_filter", enabledFilter),
        zap.Any("healthy_filter", healthyFilter),
    )

    // Get toolsets from service
    toolsets, err := h.toolsetService.ListToolsets(r.Context(), enabledFilter, healthyFilter)
    if err != nil {
        log.Error("Failed to list toolsets", zap.Error(err))
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }

    // Return JSON response
    w.Header().Set("Content-Type", "application/json")
    w.Header().Set("X-Correlation-ID", correlationID)
    json.NewEncoder(w).Encode(toolsets)

    log.Info("Toolsets returned",
        zap.Int("count", len(toolsets.Toolsets)),
    )
}
```

---

### Get Specific Toolset

**Purpose**: Retrieve configuration for a specific toolset

#### Request

```
GET /api/v1/toolsets/{name}
```

#### Response (200 OK)

```json
{
  "name": "prometheus",
  "type": "prometheus",
  "enabled": true,
  "healthy": true,
  "config": {
    "url": "http://prometheus.monitoring:9090",
    "timeout": "30s"
  },
  "serviceEndpoint": "prometheus.monitoring:9090",
  "discoveredAt": "2025-10-06T10:00:00Z",
  "lastHealthCheck": "2025-10-06T10:15:00Z",
  "healthHistory": [
    {
      "timestamp": "2025-10-06T10:15:00Z",
      "status": "healthy",
      "responseTime": "45ms"
    }
  ]
}
```

---

## Service Discovery API

### List Discovered Services

**Purpose**: Retrieve all services discovered in the cluster

#### Request

```
GET /api/v1/services
```

#### Query Parameters

| Parameter | Type | Required | Description | Example |
|-----------|------|----------|-------------|---------|
| `namespace` | string | No | Filter by namespace | `monitoring` |
| `type` | string | No | Filter by service type | `prometheus` |

#### Response (200 OK)

```json
{
  "services": [
    {
      "name": "prometheus",
      "namespace": "monitoring",
      "type": "prometheus",
      "endpoint": "http://prometheus.monitoring:9090",
      "healthy": true,
      "labels": {
        "app": "prometheus"
      },
      "annotations": {
        "prometheus.io/scrape": "true"
      },
      "discoveredAt": "2025-10-06T10:00:00Z"
    },
    {
      "name": "grafana",
      "namespace": "monitoring",
      "type": "grafana",
      "endpoint": "http://grafana.monitoring:3000",
      "healthy": true,
      "labels": {
        "app": "grafana"
      },
      "discoveredAt": "2025-10-06T10:00:00Z"
    }
  ],
  "total": 2,
  "lastDiscovery": "2025-10-06T10:00:00Z"
}
```

---

### Trigger Manual Discovery

**Purpose**: Manually trigger service discovery (bypasses 5-minute interval)

#### Request

```
POST /api/v1/discover
```

#### Request Body (Optional)

```json
{
  "namespaces": ["monitoring", "logging"],
  "serviceTypes": ["prometheus", "grafana"],
  "force": true
}
```

#### Response (202 Accepted)

```json
{
  "message": "Discovery triggered",
  "jobId": "discover-abc123",
  "estimatedCompletion": "2025-10-06T10:01:00Z"
}
```

#### Go Implementation

```go
// pkg/dynamictoolset/handlers/discovery.go
package handlers

import (
    "encoding/json"
    "net/http"
    "time"

    "go.uber.org/zap"

    "github.com/jordigilh/kubernaut/pkg/dynamictoolset/models"
    "github.com/jordigilh/kubernaut/pkg/dynamictoolset/services"
    "github.com/jordigilh/kubernaut/pkg/correlation"
)

type DiscoveryHandler struct {
    discoveryService *services.DiscoveryService
    logger           *zap.Logger
}

func (h *DiscoveryHandler) TriggerDiscovery(w http.ResponseWriter, r *http.Request) {
    correlationID := correlation.FromContext(r.Context())
    log := h.logger.With(
        zap.String("correlation_id", correlationID),
        zap.String("handler", "TriggerDiscovery"),
    )

    // Parse request body
    var req models.DiscoveryRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        log.Warn("Invalid JSON", zap.Error(err))
        http.Error(w, "Invalid JSON", http.StatusBadRequest)
        return
    }

    log.Info("Triggering manual discovery",
        zap.Strings("namespaces", req.Namespaces),
        zap.Strings("serviceTypes", req.ServiceTypes),
        zap.Bool("force", req.Force),
    )

    // Trigger discovery (async)
    jobID, err := h.discoveryService.TriggerDiscovery(r.Context(), &req)
    if err != nil {
        log.Error("Failed to trigger discovery", zap.Error(err))
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }

    // Return 202 Accepted
    response := models.DiscoveryResponse{
        Message:             "Discovery triggered",
        JobID:               jobID,
        EstimatedCompletion: time.Now().Add(1 * time.Minute),
    }

    w.Header().Set("Content-Type", "application/json")
    w.Header().Set("X-Correlation-ID", correlationID)
    w.WriteHeader(http.StatusAccepted)
    json.NewEncoder(w).Encode(response)

    log.Info("Discovery triggered",
        zap.String("job_id", jobID),
    )
}
```

---

## Health & Metrics

### Health Check

```
GET /healthz
```

**Response**: 200 OK if healthy

```json
{
  "status": "healthy",
  "timestamp": "2025-10-06T10:15:30Z",
  "dependencies": {
    "kubernetes_api": "healthy",
    "configmap": "healthy"
  },
  "discovery": {
    "last_run": "2025-10-06T10:10:00Z",
    "next_run": "2025-10-06T10:15:00Z"
  }
}
```

---

### Readiness Check

```
GET /readyz
```

**Response**: 200 OK if ready

**Ready Criteria**:
- ✅ Kubernetes API reachable
- ✅ ConfigMap accessible
- ✅ Initial discovery completed

---

### Metrics

```
GET /metrics
```

**Format**: Prometheus text format
**Authentication**: Required (TokenReviewer)

**Key Metrics**:
```
# Service discovery
dynamictoolset_services_discovered_total{type="prometheus"} 1
dynamictoolset_services_discovered_total{type="grafana"} 1
dynamictoolset_services_discovered_total{type="jaeger"} 1

# Toolset health
dynamictoolset_toolset_healthy{name="prometheus"} 1
dynamictoolset_toolset_healthy{name="grafana"} 1

# Discovery performance
dynamictoolset_discovery_duration_seconds{phase="detection"} 2.3
dynamictoolset_discovery_duration_seconds{phase="health_check"} 1.5
dynamictoolset_discovery_duration_seconds{phase="generation"} 0.2

# ConfigMap reconciliation
dynamictoolset_configmap_reconcile_total 42
dynamictoolset_configmap_drift_detected_total 3

# API usage
dynamictoolset_http_requests_total{endpoint="/api/v1/toolsets",code="200"} 150
dynamictoolset_http_request_duration_seconds{endpoint="/api/v1/toolsets"} 0.05
```

---

## Error Responses

### Standard Error Format

```json
{
  "error": {
    "code": "DISCOVERY_FAILED",
    "message": "Failed to discover services",
    "details": {
      "namespace": "monitoring",
      "reason": "Permission denied"
    }
  },
  "timestamp": "2025-10-06T10:15:30Z",
  "path": "/api/v1/discover",
  "correlationId": "req-2025-10-06-abc123"
}
```

### HTTP Status Codes

| Code | Meaning | Example |
|------|---------|---------|
| 200 | Success | Toolsets retrieved |
| 202 | Accepted | Discovery triggered (async) |
| 400 | Bad Request | Invalid namespace filter |
| 401 | Unauthorized | Invalid token |
| 404 | Not Found | Toolset not found |
| 429 | Too Many Requests | Rate limit exceeded |
| 500 | Internal Server Error | Kubernetes API error |
| 503 | Service Unavailable | Kubernetes API unavailable |

---

### Common Error Codes

| Error Code | Description | Resolution |
|------------|-------------|------------|
| `DISCOVERY_FAILED` | Service discovery failed | Check Kubernetes API connectivity |
| `CONFIGMAP_ERROR` | ConfigMap operation failed | Check RBAC permissions |
| `HEALTH_CHECK_FAILED` | Service health check failed | Verify service endpoint |
| `TOOLSET_NOT_FOUND` | Requested toolset not found | Check toolset name spelling |

---

## ConfigMap Schema

### Toolset Configuration Format

**File**: Each toolset is a separate YAML file in ConfigMap data

**Example**:
```yaml
# kubernetes-toolset.yaml
toolset: kubernetes
enabled: true
config:
  incluster: true
  namespaces:
    - "*"

# prometheus-toolset.yaml
toolset: prometheus
enabled: true
config:
  url: "http://prometheus.monitoring:9090"
  timeout: "30s"

# grafana-toolset.yaml
toolset: grafana
enabled: true
config:
  url: "http://grafana.monitoring:3000"
  apiKey: "${GRAFANA_API_KEY}"  # From Secret

# overrides.yaml (manual admin edits)
custom-service:
  enabled: true
  config:
    url: "http://custom-service:8080"
```

---

## Request Validation Rules

### Common Rules

| Field | Type | Required | Validation |
|-------|------|----------|------------|
| `enabled` | boolean | No | true or false |
| `healthy` | boolean | No | true or false |
| `namespace` | string | No | Valid K8s namespace name |
| `type` | string | No | Enum: 'prometheus', 'grafana', 'jaeger', 'elasticsearch', 'custom' |

---

## Performance Considerations

### Discovery Performance

| Operation | Target Latency | Notes |
|-----------|----------------|-------|
| Service detection | < 5s | Kubernetes API list operation |
| Health check (per service) | < 5s | HTTP request with 5s timeout |
| ConfigMap write | < 1s | Kubernetes API write |
| Total discovery cycle | < 30s | For 5-10 services |

---

### API Performance

| Endpoint | Target Latency (p95) | Notes |
|----------|---------------------|-------|
| GET /api/v1/toolsets | < 50ms | In-memory data |
| GET /api/v1/services | < 100ms | In-memory + Kubernetes cache |
| POST /api/v1/discover | < 200ms | Async trigger |

---

**Document Status**: ✅ Complete
**Last Updated**: October 6, 2025
**Version**: 1.0

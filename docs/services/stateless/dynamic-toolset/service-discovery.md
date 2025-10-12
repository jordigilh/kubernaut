# Dynamic Toolset Service - Service Discovery Deep Dive

**Version**: v1.0
**Last Updated**: October 10, 2025
**Status**: ✅ Design Complete

---

## Overview

The Service Discovery engine is the core component of the Dynamic Toolset Service. It automatically discovers monitoring and observability services in a Kubernetes cluster and makes them available for HolmesGPT investigations.

---

## Discovery Architecture

### Component Relationships

```
┌─────────────────────────────────────────────────────────────┐
│           Service Discovery Engine                          │
│                                                             │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  ServiceDiscoverer (Orchestrator)                    │  │
│  │  - Manages discovery loop (5-minute interval)        │  │
│  │  - Coordinates multiple detectors                    │  │
│  │  - Aggregates results                                │  │
│  └────────────┬─────────────────────────────────────────┘  │
│               │                                             │
│               │ Delegates to registered detectors          │
│               ▼                                             │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  ServiceDetector Interface                           │  │
│  │  - Detect(services) → DiscoveredService[]            │  │
│  │  - ServiceType() → string                            │  │
│  │  - HealthCheck(endpoint) → error                     │  │
│  └─────┬────────┬─────────┬─────────┬─────────┬─────────┘  │
│        │        │         │         │         │             │
│        ▼        ▼         ▼         ▼         ▼             │
│   Prometheus Grafana  Jaeger  Elasticsearch Custom          │
│    Detector  Detector Detector  Detector  Detector          │
└─────────────────────────────────────────────────────────────┘
                │
                │ Discovers services from
                ▼
        ┌───────────────────┐
        │ Kubernetes API    │
        │ (List Services)   │
        └───────────────────┘
```

---

## Discovery Flow

### Step-by-Step Process

#### 1. **List All Services**
```go
services, err := k8sClient.CoreV1().Services("").List(ctx, metav1.ListOptions{})
```

**What**: Retrieve all Kubernetes Services from all namespaces
**Why**: Detectors need complete service list to find matches
**Performance**: Single K8s API call, fast (<100ms typical)

---

#### 2. **Run Each Detector**
```go
for _, detector := range d.detectors {
    detectedServices, err := detector.Detect(ctx, services.Items)
    discovered = append(discovered, detectedServices...)
}
```

**What**: Each detector evaluates all services to find matches
**Why**: Different services have different detection criteria
**Performance**: In-memory matching, fast (<10ms per detector)

---

#### 3. **Health Check Validation**
```go
for _, svc := range discovered {
    if err := detector.HealthCheck(ctx, svc.Endpoint); err != nil {
        logger.Warn("Service health check failed, skipping",
            zap.String("service", svc.Name), zap.Error(err))
        continue
    }
    validServices = append(validServices, svc)
}
```

**What**: HTTP GET to each service's health endpoint
**Why**: Only include operational services in toolsets
**Performance**: Network I/O bound (5s timeout per service)

---

#### 4. **Update Cache**
```go
d.discoveryCache = make(map[string]DiscoveredService)
for _, svc := range validServices {
    key := fmt.Sprintf("%s/%s", svc.Type, svc.Name)
    d.discoveryCache[key] = svc
}
```

**What**: Store discovered services in-memory cache
**Why**: Fast access for API queries
**Performance**: In-memory map, instant

---

## Detector Implementations

### Prometheus Detector

**Detection Criteria**:
1. **Label Match**: `app=prometheus` or `app.kubernetes.io/name=prometheus`
2. **Name Match**: Service name contains "prometheus"
3. **Port Match**: Port 9090 named "web"

**Health Check**: `GET http://<endpoint>/-/healthy`

**Example Service**:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: prometheus-server
  namespace: monitoring
  labels:
    app: prometheus
spec:
  ports:
  - name: web
    port: 9090
  selector:
    app: prometheus
```

**Detection Code**:
```go
import (
    "strings"
    
    corev1 "k8s.io/api/core/v1"
)

func (d *PrometheusDetector) isPrometheus(svc corev1.Service) bool {
    // Strategy 1: Label match (highest confidence)
    if app, ok := svc.Labels["app"]; ok && app == "prometheus" {
        return true
    }
    
    // Strategy 2: Name match (medium confidence)
    if strings.Contains(svc.Name, "prometheus") {
        return true
    }
    
    // Strategy 3: Port match (lowest confidence, may have false positives)
    for _, port := range svc.Spec.Ports {
        if port.Name == "web" && port.Port == 9090 {
            return true
        }
    }
    
    return false
}
```

---

### Grafana Detector

**Detection Criteria**:
1. **Label Match**: `app=grafana` or `app.kubernetes.io/name=grafana`
2. **Name Match**: Service name equals "grafana"
3. **Port Match**: Port 3000 named "service"

**Health Check**: `GET http://<endpoint>/api/health`

**Example Service**:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: grafana
  namespace: monitoring
  labels:
    app: grafana
spec:
  ports:
  - name: service
    port: 3000
  selector:
    app: grafana
```

---

### Custom Service Detector

**Detection Criteria**: Annotation-based discovery

**Required Annotations**:
```yaml
annotations:
  kubernaut.io/toolset: "true"
  kubernaut.io/toolset-type: "custom"
  kubernaut.io/toolset-name: "my-service"
  kubernaut.io/health-endpoint: "/health"
```

**Example**:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-observability-service
  namespace: monitoring
  annotations:
    kubernaut.io/toolset: "true"
    kubernaut.io/toolset-type: "custom"
    kubernaut.io/toolset-name: "my-service"
    kubernaut.io/health-endpoint: "/health"
spec:
  ports:
  - name: http
    port: 8080
```

---

## Health Check Strategy

### Health Check Configuration

```go
type HealthCheckConfig struct {
    Enabled       bool          `yaml:"enabled"`        // Default: true
    Timeout       time.Duration `yaml:"timeout"`        // Default: 5s
    RetryAttempts int           `yaml:"retry_attempts"` // Default: 3
    RetryInterval time.Duration `yaml:"retry_interval"` // Default: 1s
}
```

### Health Check Flow

```
┌──────────────────────────────────────────────────────────┐
│ Health Check with Retry                                  │
└──────────────────────────────────────────────────────────┘

Attempt 1:  GET /health  →  Timeout (5s)
            ↓
            [Wait 1s]
            ↓
Attempt 2:  GET /health  →  503 Service Unavailable
            ↓
            [Wait 1s]
            ↓
Attempt 3:  GET /health  →  200 OK  ✓

Result: Service is healthy
```

### Health Check Endpoints by Service Type

| Service | Health Endpoint | Success Criteria |
|---------|----------------|------------------|
| **Prometheus** | `/-/healthy` | 200 OK |
| **Grafana** | `/api/health` | 200 OK |
| **Jaeger** | `/` | 200 OK |
| **Elasticsearch** | `/` | 200 OK |
| **Custom** | Annotation-defined | 200 OK |

---

## Discovery Loop

### Timing Configuration

**Discovery Interval**: 5 minutes
**Rationale**: Services are added/removed infrequently, 5 minutes is acceptable

**Discovery Duration**: < 10 seconds (p95)
**Breakdown**:
- List services: 100ms
- Run detectors: 50ms (5 detectors × 10ms)
- Health checks: 9s (3 services × 3s avg)

### Loop Implementation

```go
func (d *ServiceDiscoverer) Start(ctx context.Context) error {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()

    // Run discovery immediately on start
    if _, err := d.DiscoverServices(ctx); err != nil {
        logger.Error("Initial discovery failed", zap.Error(err))
        return err
    }

    for {
        select {
        case <-ticker.C:
            if _, err := d.DiscoverServices(ctx); err != nil {
                logger.Error("Discovery failed", zap.Error(err))
                // Continue running even if discovery fails
            }
        case <-d.stopCh:
            logger.Info("Stopping discovery loop")
            return nil
        case <-ctx.Done():
            return ctx.Err()
        }
    }
}
```

---

## Error Handling

### Detector Failure

**Strategy**: Continue with other detectors if one fails

```go
for _, detector := range d.detectors {
    detectedServices, err := detector.Detect(ctx, services.Items)
    if err != nil {
        logger.Warn("Detector failed",
            zap.String("detector", detector.ServiceType()),
            zap.Error(err))
        continue // Don't fail entire discovery
    }
    discovered = append(discovered, detectedServices...)
}
```

**Result**: Partial discovery success (e.g., Prometheus found, Grafana detector failed)

### Health Check Failure

**Strategy**: Log warning and exclude service

```go
if err := detector.HealthCheck(ctx, svc.Endpoint); err != nil {
    logger.Warn("Service health check failed, skipping",
        zap.String("service_type", svc.Type),
        zap.String("service_name", svc.Name),
        zap.Error(err))
    continue
}
```

**Result**: Only healthy services included in toolsets

### Kubernetes API Failure

**Strategy**: Fail discovery, retry on next interval

```go
services, err := k8sClient.CoreV1().Services("").List(ctx, metav1.ListOptions{})
if err != nil {
    logger.Error("Failed to list services", zap.Error(err))
    return nil, fmt.Errorf("failed to list services: %w", err)
}
```

**Result**: No services discovered, retry in 5 minutes

---

## Performance Optimization

### Caching Strategy

**In-Memory Cache**: Store discovered services for API queries

```go
import (
    "sync"
    "time"
    
    "github.com/jordigilh/kubernaut/pkg/toolset"
)

type ServiceCache struct {
    mu       sync.RWMutex
    services map[string]toolset.DiscoveredService
    ttl      time.Duration
}
```

**Benefits**:
- Fast API responses (<1ms)
- Reduced K8s API load
- Consistent results during discovery interval

### Concurrent Health Checks

**Parallel Execution**: Health check multiple services concurrently

```go
import (
    "context"
    "sync"
    
    "github.com/jordigilh/kubernaut/pkg/toolset"
)

var wg sync.WaitGroup
healthChan := make(chan toolset.DiscoveredService, len(discovered))

for _, svc := range discovered {
    wg.Add(1)
    go func(s toolset.DiscoveredService) {
        defer wg.Done()
        if err := detector.HealthCheck(ctx, s.Endpoint); err == nil {
            healthChan <- s
        }
    }(svc)
}

wg.Wait()
close(healthChan)
```

**Benefit**: Reduce total health check time from 30s (10 services × 3s) to 3s (parallel)

---

## Monitoring & Metrics

### Discovery Metrics

```go
// Services discovered by type
servicesDiscoveredTotal.WithLabelValues("prometheus", "monitoring").Inc()

// Discovery duration
discoveryDuration.WithLabelValues("success").Observe(duration.Seconds())

// Health check failures
healthChecksTotal.WithLabelValues("grafana", "unhealthy").Inc()
```

### Logging

```go
logger.Info("Service discovery complete",
    zap.Int("discovered_count", len(discovered)),
    zap.Duration("duration", duration),
    zap.Int("prometheus_count", countByType(discovered, "prometheus")),
    zap.Int("grafana_count", countByType(discovered, "grafana")))
```

---

**Document Status**: ✅ Service Discovery Deep Dive Complete
**Last Updated**: October 10, 2025
**Related**: [implementation.md](./implementation.md), [design/01-detector-interface-design.md](./implementation/design/01-detector-interface-design.md)


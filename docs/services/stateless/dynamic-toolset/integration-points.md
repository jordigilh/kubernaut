# Dynamic Toolset Service - Integration Points

**Version**: v1.0
**Last Updated**: October 6, 2025
**Service Type**: Stateless HTTP API + Kubernetes Controller
**Port**: 8080 (REST API + Health), 9090 (Metrics)

---

## Table of Contents

1. [Integration Overview](#integration-overview)
2. [Upstream Services (Toolset Consumers)](#upstream-services-toolset-consumers)
3. [Downstream Services (Discovered Services)](#downstream-services-discovered-services)
4. [Integration Patterns](#integration-patterns)
5. [Error Handling](#error-handling)
6. [Data Flow Diagrams](#data-flow-diagrams)

---

## Integration Overview

### **Service Position in Architecture**

Dynamic Toolset Service acts as the **intelligent service discovery engine** in the Kubernaut architecture:

```
┌─────────────────────────────────────────────────────────────┐
│                    Upstream Services                        │
│  (Poll toolset configuration)                               │
│                                                             │
│  • HolmesGPT API Service                                    │
└────────────────────┬────────────────────────────────────────┘
                     │
                     │ ConfigMap Poll (holmesgpt-toolset)
                     │ or GET /api/v1/toolset
                     ▼
┌─────────────────────────────────────────────────────────────┐
│        Dynamic Toolset Service (Port 8080)                  │
│                                                             │
│  1. Watch Kubernetes services (all namespaces)              │
│  2. Detect service types (Prometheus, Grafana, etc.)        │
│  3. Validate service health                                 │
│  4. Generate toolset configuration                          │
│  5. Write to ConfigMap (holmesgpt-toolset)                  │
│  6. Reconcile ConfigMap (prevent deletion)                  │
└────────────────────┬────────────────────────────────────────┘
                     │
                     │ Kubernetes Service Watch
                     │ Health Check HTTP Calls
                     │ ConfigMap Create/Update
                     ▼
┌─────────────────────────────────────────────────────────────┐
│                 Downstream Services                         │
│  (Discovered and health-checked)                            │
│                                                             │
│  • Prometheus (monitoring namespace)                        │
│  • Grafana (monitoring namespace)                           │
│  • Jaeger (tracing namespace)                               │
│  • Elasticsearch (logging namespace)                        │
└─────────────────────────────────────────────────────────────┘
```

---

## Upstream Services (Toolset Consumers)

### **1. HolmesGPT API Service**

**Purpose**: Loads toolset configuration for AI-powered investigations

**Integration Pattern**: ConfigMap polling (primary) + HTTP GET (fallback)
**Authentication**: ServiceAccount (in-cluster) for ConfigMap, Bearer Token for HTTP
**Refresh Interval**: 5 minutes (configurable)

#### **ConfigMap Polling (Primary)**

```python
# In HolmesGPT API Service (Python)
from kubernetes import client, config, watch
import yaml
import logging

logger = logging.getLogger(__name__)

class ToolsetLoader:
    """Load toolset from ConfigMap with hot-reload."""

    def __init__(self, namespace: str = "kubernaut-system", configmap_name: str = "holmesgpt-toolset"):
        try:
            config.load_incluster_config()
        except config.ConfigException:
            config.load_kube_config()

        self.core_api = client.CoreV1Api()
        self.namespace = namespace
        self.configmap_name = configmap_name
        self.toolset = None

    def load_toolset(self) -> Dict:
        """Load toolset from ConfigMap."""
        try:
            configmap = self.core_api.read_namespaced_config_map(
                name=self.configmap_name,
                namespace=self.namespace
            )

            toolset_yaml = configmap.data.get("toolset.yaml", "{}")
            self.toolset = yaml.safe_load(toolset_yaml)

            logger.info(f"Toolset loaded: {len(self.toolset.get('tools', []))} tools")
            return self.toolset

        except Exception as e:
            logger.error(f"Failed to load toolset: {e}")
            return {}

    def watch_toolset_changes(self, callback):
        """Watch ConfigMap for toolset changes (hot-reload)."""
        w = watch.Watch()

        for event in w.stream(
            self.core_api.list_namespaced_config_map,
            namespace=self.namespace,
            field_selector=f"metadata.name={self.configmap_name}"
        ):
            if event["type"] in ["ADDED", "MODIFIED"]:
                logger.info("Toolset ConfigMap changed, reloading...")
                new_toolset = self.load_toolset()
                callback(new_toolset)
```

#### **HTTP GET (Fallback)**

```python
import httpx

class DynamicToolsetClient:
    """HTTP client for Dynamic Toolset Service."""

    def __init__(self, base_url: str, token: str):
        self.base_url = base_url
        self.token = token
        self.client = httpx.AsyncClient(timeout=10.0)

    async def get_current_toolset(self) -> Dict:
        """Get current toolset via HTTP API."""
        headers = {
            "Authorization": f"Bearer {self.token}",
            "Content-Type": "application/json"
        }

        try:
            response = await self.client.get(
                f"{self.base_url}/api/v1/toolset",
                headers=headers
            )

            if response.status_code == 200:
                return response.json()
            else:
                logger.warning(f"Dynamic Toolset API returned {response.status_code}")
                return {}

        except Exception as e:
            logger.error(f"Failed to get toolset: {e}")
            return {}
```

---

## Downstream Services (Discovered Services)

### **1. Prometheus (Monitoring)**

**Purpose**: Metrics querying for HolmesGPT investigations

**Integration Pattern**: Service discovery → Health check → Toolset inclusion
**Health Check**: `GET http://<endpoint>/-/healthy`
**Expected Port**: 9090

#### **Service Discovery**

```go
package discovery

import (
    corev1 "k8s.io/api/core/v1"
)

type PrometheusDetector struct{}

func (d *PrometheusDetector) Detect(service *corev1.Service) bool {
    // Check labels
    if service.Labels["app"] == "prometheus" ||
       service.Labels["app.kubernetes.io/name"] == "prometheus" {
        return true
    }

    // Check port 9090
    for _, port := range service.Spec.Ports {
        if port.Port == 9090 {
            return true
        }
    }

    return false
}

func (d *PrometheusDetector) GetEndpoint(service *corev1.Service) string {
    return fmt.Sprintf("http://%s.%s.svc.cluster.local:9090",
        service.Name,
        service.Namespace,
    )
}
```

#### **Health Check**

```go
package discovery

import (
    "context"
    "fmt"
    "net/http"
    "time"
)

func (hc *ServiceHealthChecker) CheckPrometheusHealth(ctx context.Context, endpoint string) (bool, error) {
    req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/-/healthy", endpoint), nil)
    if err != nil {
        return false, err
    }

    client := &http.Client{Timeout: 5 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        return false, err
    }
    defer resp.Body.Close()

    return resp.StatusCode == http.StatusOK, nil
}
```

---

### **2. Grafana (Monitoring)**

**Purpose**: Dashboard access for visualization

**Integration Pattern**: Service discovery → Health check → Toolset inclusion
**Health Check**: `GET http://<endpoint>/api/health`
**Expected Port**: 3000

#### **Service Discovery**

```go
package discovery

import (
    corev1 "k8s.io/api/core/v1"
)

type GrafanaDetector struct{}

func (d *GrafanaDetector) Detect(service *corev1.Service) bool {
    // Check labels
    if service.Labels["app"] == "grafana" ||
       service.Labels["app.kubernetes.io/name"] == "grafana" {
        return true
    }

    // Check port 3000
    for _, port := range service.Spec.Ports {
        if port.Port == 3000 {
            return true
        }
    }

    return false
}
```

---

### **3. Jaeger (Tracing)**

**Purpose**: Distributed tracing for investigations

**Integration Pattern**: Service discovery → Health check → Toolset inclusion
**Health Check**: `GET http://<endpoint>/`
**Expected Port**: 16686 (UI), 14250 (gRPC)

---

### **4. Elasticsearch (Logging)**

**Purpose**: Log aggregation and search

**Integration Pattern**: Service discovery → Health check → Toolset inclusion
**Health Check**: `GET http://<endpoint>/_cluster/health`
**Expected Port**: 9200

---

## Integration Patterns

### **Pattern 1: Periodic Service Discovery**

**Trigger**: Every 5 minutes (configurable)
**Process**: List all services → Detect types → Validate health → Update ConfigMap

```go
package discovery

import (
    "context"
    "time"

    "go.uber.org/zap"
)

func (d *DiscoveryService) PeriodicDiscovery(ctx context.Context, interval time.Duration) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            d.logger.Info("Starting periodic service discovery")

            // Discover services
            services, err := d.DiscoverServices(ctx)
            if err != nil {
                d.logger.Error("Service discovery failed", zap.Error(err))
                continue
            }

            // Validate health
            for i := range services {
                healthy, err := d.healthChecker.CheckHealth(ctx, services[i].Type, services[i].Endpoint)
                if err != nil {
                    d.logger.Warn("Health check failed",
                        zap.String("service", services[i].Name),
                        zap.Error(err),
                    )
                }
                services[i].Healthy = healthy
            }

            // Generate toolset
            toolset, err := d.generator.GenerateToolset(services)
            if err != nil {
                d.logger.Error("Toolset generation failed", zap.Error(err))
                continue
            }

            // Update ConfigMap
            err = d.configMapWriter.WriteToolset(ctx, "kubernaut-system", "holmesgpt-toolset", toolset)
            if err != nil {
                d.logger.Error("ConfigMap update failed", zap.Error(err))
                continue
            }

            d.logger.Info("Service discovery completed",
                zap.Int("services_discovered", len(services)),
                zap.Int("healthy_services", countHealthy(services)),
            )

        case <-ctx.Done():
            return
        }
    }
}
```

---

### **Pattern 2: Event-Driven Discovery (Service Watch)**

**Trigger**: Kubernetes Service add/delete/modify events
**Process**: Immediate discovery and ConfigMap update

```go
package discovery

import (
    "context"

    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/watch"
    "go.uber.org/zap"
)

func (d *DiscoveryService) WatchServices(ctx context.Context) {
    watcher, err := d.k8sClient.CoreV1().Services("").Watch(ctx, metav1.ListOptions{})
    if err != nil {
        d.logger.Error("Failed to start service watcher", zap.Error(err))
        return
    }
    defer watcher.Stop()

    for event := range watcher.ResultChan() {
        service, ok := event.Object.(*corev1.Service)
        if !ok {
            continue
        }

        switch event.Type {
        case watch.Added:
            d.logger.Info("Service added",
                zap.String("name", service.Name),
                zap.String("namespace", service.Namespace),
            )
            d.handleServiceAdded(ctx, service)

        case watch.Deleted:
            d.logger.Info("Service deleted",
                zap.String("name", service.Name),
                zap.String("namespace", service.Namespace),
            )
            d.handleServiceDeleted(ctx, service)

        case watch.Modified:
            d.logger.Info("Service modified",
                zap.String("name", service.Name),
                zap.String("namespace", service.Namespace),
            )
            d.handleServiceModified(ctx, service)
        }
    }
}
```

---

### **Pattern 3: ConfigMap Reconciliation**

**Trigger**: ConfigMap delete or modification events
**Process**: Recreate ConfigMap with latest discovered services

```go
package controllers

import (
    "context"
    "time"

    corev1 "k8s.io/api/core/v1"
    "k8s.io/apimachinery/pkg/api/errors"
    "sigs.k8s.io/controller-runtime/pkg/reconcile"
    "go.uber.org/zap"
)

func (r *ConfigMapReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
    r.logger.Info("Reconciling ConfigMap",
        zap.String("name", req.Name),
        zap.String("namespace", req.Namespace),
    )

    // Check if ConfigMap exists
    configMap := &corev1.ConfigMap{}
    err := r.client.Get(ctx, req.NamespacedName, configMap)
    if err != nil {
        if errors.IsNotFound(err) {
            // ConfigMap was deleted, recreate it
            r.logger.Warn("ConfigMap not found, recreating",
                zap.String("name", req.Name),
            )
            return r.recreateConfigMap(ctx, req)
        }
        return reconcile.Result{}, err
    }

    // ConfigMap exists, verify it has correct structure
    if _, ok := configMap.Data["toolset.yaml"]; !ok {
        r.logger.Warn("ConfigMap missing toolset.yaml, fixing",
            zap.String("name", req.Name),
        )
        return r.fixConfigMap(ctx, configMap)
    }

    // Reconciliation successful
    return reconcile.Result{RequeueAfter: 5 * time.Minute}, nil
}
```

---

## Error Handling

### **Service Discovery Errors**

```go
package discovery

import (
    "context"
    "fmt"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "go.uber.org/zap"
)

func (d *DiscoveryService) DiscoverServices(ctx context.Context) ([]DiscoveredService, error) {
    services, err := d.k8sClient.CoreV1().Services("").List(ctx, metav1.ListOptions{})
    if err != nil {
        d.logger.Error("Failed to list services", zap.Error(err))
        // Return cached services if available
        if len(d.cachedServices) > 0 {
            d.logger.Info("Returning cached services",
                zap.Int("count", len(d.cachedServices)),
            )
            return d.cachedServices, nil
        }
        return nil, fmt.Errorf("service discovery failed: %w", err)
    }

    discovered := []DiscoveredService{}
    for _, svc := range services.Items {
        serviceType := d.detector.DetectServiceType(&svc)
        if serviceType != "" {
            discovered = append(discovered, DiscoveredService{
                Type:      serviceType,
                Name:      svc.Name,
                Namespace: svc.Namespace,
                Endpoint:  d.detector.GetEndpoint(&svc),
            })
        }
    }

    // Cache discovered services
    d.cachedServices = discovered

    return discovered, nil
}
```

### **Health Check Errors**

```go
package discovery

import (
    "context"
    "fmt"
    "net/http"
    "time"

    "go.uber.org/zap"
)

func (hc *ServiceHealthChecker) CheckHealth(ctx context.Context, serviceType, endpoint string) (bool, error) {
    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()

    healthPath := hc.getHealthPath(serviceType)
    req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s%s", endpoint, healthPath), nil)
    if err != nil {
        return false, err
    }

    client := &http.Client{Timeout: 5 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        hc.logger.Warn("Health check failed",
            zap.String("service_type", serviceType),
            zap.String("endpoint", endpoint),
            zap.Error(err),
        )
        return false, nil // Return false, no error (service unhealthy)
    }
    defer resp.Body.Close()

    return resp.StatusCode == http.StatusOK, nil
}
```

---

## Data Flow Diagrams

### **Complete Service Discovery Flow**

```
┌──────────────────────────────────────────────────────────────────┐
│ Step 1: Periodic Discovery Trigger (every 5 minutes)            │
│   - Timer fires                                                  │
│   - Start discovery process                                      │
└─────────────────────────┬────────────────────────────────────────┘
                          │
                          ▼
┌──────────────────────────────────────────────────────────────────┐
│ Step 2: List All Kubernetes Services                            │
│   - Call k8s API: GET /api/v1/services                          │
│   - Filter by namespaces (if configured)                         │
└─────────────────────────┬────────────────────────────────────────┘
                          │
                          ▼
┌──────────────────────────────────────────────────────────────────┐
│ Step 3: Detect Service Types                                    │
│   - Iterate through services                                     │
│   - Check labels for known service types                         │
│   - Check ports for known service types                          │
│   - Identify: Prometheus, Grafana, Jaeger, Elasticsearch        │
└─────────────────────────┬────────────────────────────────────────┘
                          │
                          ▼
┌──────────────────────────────────────────────────────────────────┐
│ Step 4: Validate Service Health                                 │
│   - For each discovered service:                                 │
│     - GET <endpoint>/-/healthy (Prometheus)                      │
│     - GET <endpoint>/api/health (Grafana)                        │
│     - GET <endpoint>/ (Jaeger)                                   │
│     - GET <endpoint>/_cluster/health (Elasticsearch)             │
│   - Mark service as healthy or unhealthy                         │
└─────────────────────────┬────────────────────────────────────────┘
                          │
                          ▼
┌──────────────────────────────────────────────────────────────────┐
│ Step 5: Generate Toolset Configuration                          │
│   - Filter out unhealthy services                                │
│   - Format as HolmesGPT toolset YAML                             │
│   - Preserve manual overrides (if any)                           │
└─────────────────────────┬────────────────────────────────────────┘
                          │
                          ▼
┌──────────────────────────────────────────────────────────────────┐
│ Step 6: Write to ConfigMap                                      │
│   - Create/Update holmesgpt-toolset ConfigMap                    │
│   - Write toolset.yaml data                                      │
│   - Set owner references                                         │
└─────────────────────────┬────────────────────────────────────────┘
                          │
                          ▼
┌──────────────────────────────────────────────────────────────────┐
│ Step 7: HolmesGPT API Detects Change                            │
│   - ConfigMap watch detects modification                         │
│   - Trigger toolset reload                                       │
│   - Update HolmesGPT SDK configuration                           │
└──────────────────────────────────────────────────────────────────┘
```

---

## Reference Documentation

- **API Specification**: `docs/services/stateless/dynamic-toolset/api-specification.md`
- **Security Configuration**: `docs/services/stateless/dynamic-toolset/security-configuration.md`
- **Testing Strategy**: `docs/services/stateless/dynamic-toolset/testing-strategy.md`
- **TokenReviewer Auth**: `docs/architecture/KUBERNETES_TOKENREVIEWER_AUTH.md`
- **Service Dependency Map**: `docs/architecture/SERVICE_DEPENDENCY_MAP.md`

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: October 6, 2025
**Integration Status**: Design complete, implementation pending


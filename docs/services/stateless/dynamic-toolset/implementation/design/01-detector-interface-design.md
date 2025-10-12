# Dynamic Toolset Service - Detector Interface Design

**Version**: v1.0
**Created**: October 10, 2025
**Status**: ✅ Design Complete

---

## Design Decision: Pluggable Detector Pattern

### Problem Statement

The Dynamic Toolset Service needs to discover multiple service types (Prometheus, Grafana, Jaeger, Elasticsearch) in a Kubernetes cluster. Each service type has different detection criteria (labels, ports, names).

**Requirements**:
1. **Extensibility**: Easy to add new service detectors without modifying core discovery logic
2. **Testability**: Each detector should be independently testable
3. **Consistency**: All detectors should follow the same pattern
4. **Health Validation**: Each detector must validate service health before including in toolsets

### Decision: Interface-Based Detector Pattern

**Design**: Use a pluggable `ServiceDetector` interface with concrete implementations for each service type.

```go
// ServiceDetector detects a specific type of service (Prometheus, Grafana, etc.)
type ServiceDetector interface {
    // Detect searches for services of this type
    Detect(ctx context.Context, services []corev1.Service) ([]toolset.DiscoveredService, error)

    // ServiceType returns the type identifier (e.g., "prometheus", "grafana")
    ServiceType() string

    // HealthCheck validates the service is actually operational
    HealthCheck(ctx context.Context, endpoint string) error
}
```

---

## Detector Implementation Pattern

### Standard Detection Strategy

Each detector follows a 3-step pattern:

#### Step 1: Service Matching
**Question**: Does this Kubernetes Service match the target service type?

**Detection Criteria** (multiple strategies):
1. **Label matching**: Check for specific labels (e.g., `app=prometheus`)
2. **Service name matching**: Check service name (e.g., `prometheus-server`)
3. **Port matching**: Check for expected ports (e.g., port 9090 named "web")

**Example (Prometheus)**:
```go
func (d *PrometheusDetector) isPrometheus(svc corev1.Service) bool {
    // Strategy 1: Check labels
    if app, ok := svc.Labels["app"]; ok && app == "prometheus" {
        return true
    }

    // Strategy 2: Check service name
    if svc.Name == "prometheus" || svc.Name == "prometheus-server" {
        return true
    }

    // Strategy 3: Check for prometheus port
    for _, port := range svc.Spec.Ports {
        if port.Name == "web" && port.Port == 9090 {
            return true
        }
    }

    return false
}
```

#### Step 2: Endpoint Construction
**Question**: What is the service's internal endpoint?

**Pattern**: `http://<service-name>.<namespace>.svc.cluster.local:<port>`

**Example**:
```go
func (d *PrometheusDetector) buildEndpoint(svc corev1.Service) string {
    port := d.getPrometheusPort(svc) // e.g., "9090"
    return fmt.Sprintf("http://%s.%s.svc.cluster.local:%s",
        svc.Name, svc.Namespace, port)
}
```

#### Step 3: Health Validation
**Question**: Is the service actually healthy?

**Pattern**: HTTP GET to service-specific health endpoint

**Example (Prometheus)**:
```go
func (d *PrometheusDetector) HealthCheck(ctx context.Context, endpoint string) error {
    healthURL := fmt.Sprintf("%s/-/healthy", endpoint)

    req, err := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
    if err != nil {
        return fmt.Errorf("failed to create health check request: %w", err)
    }

    resp, err := d.httpClient.Do(req)
    if err != nil {
        return fmt.Errorf("health check failed: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("unhealthy: status %d", resp.StatusCode)
    }

    return nil
}
```

---

## Service Type Registry

### Supported Services (V1)

| Service Type | Detection Label | Default Port | Health Endpoint |
|--------------|-----------------|--------------|-----------------|
| **Kubernetes** | Built-in (always included) | N/A | N/A |
| **Prometheus** | `app=prometheus` | 9090 | `/-/healthy` |
| **Grafana** | `app=grafana` | 3000 | `/api/health` |
| **Jaeger** | `app=jaeger` | 16686 | `/` |
| **Elasticsearch** | `app=elasticsearch` | 9200 | `/` |

### Custom Service Detection (V1)

**Annotation-Based Detection**:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-custom-service
  annotations:
    kubernaut.io/toolset: "true"
    kubernaut.io/toolset-type: "custom"
    kubernaut.io/toolset-name: "my-service"
    kubernaut.io/health-endpoint: "/health"
```

**Custom Detector Implementation**:
```go
func (d *CustomDetector) Detect(ctx context.Context, services []corev1.Service) ([]toolset.DiscoveredService, error) {
    var discovered []toolset.DiscoveredService

    for _, svc := range services {
        // Check for kubernaut.io/toolset annotation
        if enabled, ok := svc.Annotations["kubernaut.io/toolset"]; !ok || enabled != "true" {
            continue
        }

        serviceType := svc.Annotations["kubernaut.io/toolset-type"]
        serviceName := svc.Annotations["kubernaut.io/toolset-name"]
        healthEndpoint := svc.Annotations["kubernaut.io/health-endpoint"]

        endpoint := d.buildEndpoint(svc)

        discovered = append(discovered, toolset.DiscoveredService{
            Name:      serviceName,
            Namespace: svc.Namespace,
            Type:      serviceType,
            Endpoint:  endpoint,
            Labels:    svc.Labels,
            Metadata: map[string]string{
                "health_endpoint": healthEndpoint,
            },
        })
    }

    return discovered, nil
}
```

---

## Detector Registration

### Discovery Service Registration Pattern

```go
// main.go
func main() {
    // Create service discoverer
    discoverer := discovery.NewServiceDiscoverer(k8sClient, logger)

    // Register built-in detectors
    discoverer.RegisterDetector(discovery.NewPrometheusDetector(logger))
    discoverer.RegisterDetector(discovery.NewGrafanaDetector(logger))
    discoverer.RegisterDetector(discovery.NewJaegerDetector(logger))
    discoverer.RegisterDetector(discovery.NewElasticsearchDetector(logger))

    // Register custom detector (for annotation-based services)
    discoverer.RegisterDetector(discovery.NewCustomDetector(logger))

    // Start discovery loop
    discoverer.Start(ctx)
}
```

---

## Design Rationale

### Why Interface-Based Pattern?

**Advantages**:
1. **✅ Extensibility**: New detectors added without modifying `ServiceDiscoverer`
2. **✅ Testability**: Mock detectors for unit testing
3. **✅ Consistency**: All detectors follow same contract
4. **✅ Pluggability**: Enable/disable detectors via configuration

**Alternatives Considered**:
- **❌ Hardcoded detection logic**: Difficult to extend, large switch statement
- **❌ Reflection-based registration**: Complex, difficult to debug
- **✅ Interface-based (chosen)**: Simple, explicit, testable

### Why Separate Health Check Method?

**Rationale**: Health checks are I/O-bound and may fail transiently. Separating them from detection allows:
1. **Retry logic**: Retry health checks without re-running detection
2. **Timeout control**: Configure health check timeouts independently
3. **Optional health checks**: Allow skipping health checks via configuration
4. **Testability**: Mock health checks independently from detection

---

## Error Handling Strategy

### Detection Errors
**Strategy**: Continue processing other services if one detector fails

```go
for _, detector := range d.detectors {
    detectedServices, err := detector.Detect(ctx, services)
    if err != nil {
        d.logger.Warn("Detector failed",
            zap.String("detector", detector.ServiceType()),
            zap.Error(err))
        continue // Don't fail entire discovery
    }

    discovered = append(discovered, detectedServices...)
}
```

### Health Check Errors
**Strategy**: Log warning and exclude service from toolsets

```go
for _, svc := range detectedServices {
    if err := detector.HealthCheck(ctx, svc.Endpoint); err != nil {
        d.logger.Warn("Service health check failed, skipping",
            zap.String("service_type", svc.Type),
            zap.String("service_name", svc.Name),
            zap.Error(err))
        continue // Skip unhealthy service
    }

    discovered = append(discovered, svc)
}
```

---

## Configuration Options

### Health Check Configuration

```go
type HealthCheckConfig struct {
    Enabled        bool          `yaml:"enabled"`         // Default: true
    Timeout        time.Duration `yaml:"timeout"`         // Default: 5s
    RetryAttempts  int           `yaml:"retry_attempts"`  // Default: 3
    RetryInterval  time.Duration `yaml:"retry_interval"`  // Default: 1s
}
```

**Example**:
```yaml
# config.yaml
health_checks:
  enabled: true
  timeout: 5s
  retry_attempts: 3
  retry_interval: 1s
```

---

## Future Enhancements (V2+)

### Dynamic Detector Plugins
**Concept**: Load detectors from external plugins

```go
// Load detector plugin from shared library
plugin, err := plugin.Open("./detectors/datadog-detector.so")
detector := plugin.Lookup("NewDatadogDetector")
discoverer.RegisterDetector(detector)
```

### Multi-Cluster Discovery
**Concept**: Discover services across multiple clusters

```go
type MultiClusterDiscoverer struct {
    clusters []ClusterConfig
}

func (d *MultiClusterDiscoverer) DiscoverServices(ctx context.Context) ([]toolset.DiscoveredService, error) {
    var allServices []toolset.DiscoveredService

    for _, cluster := range d.clusters {
        clusterServices, err := d.discoverInCluster(ctx, cluster)
        if err != nil {
            continue
        }
        allServices = append(allServices, clusterServices...)
    }

    return allServices, nil
}
```

---

**Document Status**: ✅ Detector Interface Design Complete
**Last Updated**: October 10, 2025
**Confidence**: 95% (Very High)


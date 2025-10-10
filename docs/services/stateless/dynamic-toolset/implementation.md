# Dynamic Toolset Service - Implementation Details

**Version**: v1.0
**Last Updated**: October 10, 2025
**Status**: ✅ Design Complete, Ready for Implementation

---

## Table of Contents

1. [Package Structure](#package-structure)
2. [Service Discovery Pattern](#service-discovery-pattern)
3. [HTTP Server Implementation](#http-server-implementation)
4. [ConfigMap Management](#configmap-management)
5. [Health Check Implementation](#health-check-implementation)
6. [Reconciliation Controller](#reconciliation-controller)
7. [Error Handling](#error-handling)

---

## Package Structure

### Directory Layout

Following Go idioms and Kubernaut patterns:

```
cmd/dynamic-toolset/                      # Main application entry point
  └── main.go                             # Server initialization, dependency injection

pkg/toolset/                              # Business logic (PUBLIC API)
  ├── service.go                          # DynamicToolsetService interface
  ├── server.go                           # HTTP server implementation
  ├── discovery/
  │   ├── discoverer.go                   # ServiceDiscoverer interface
  │   ├── detector.go                     # ServiceDetector interface
  │   ├── prometheus_detector.go          # Prometheus service detector
  │   ├── grafana_detector.go             # Grafana service detector
  │   ├── jaeger_detector.go              # Jaeger service detector
  │   ├── elasticsearch_detector.go       # Elasticsearch service detector
  │   └── custom_detector.go              # Custom annotation-based detector
  ├── generator/
  │   ├── generator.go                    # ConfigMap generator interface
  │   ├── kubernetes_toolset.go           # Kubernetes toolset generator
  │   ├── prometheus_toolset.go           # Prometheus toolset generator
  │   ├── grafana_toolset.go              # Grafana toolset generator
  │   └── override_merger.go              # Override preservation logic
  ├── reconciler/
  │   ├── reconciler.go                   # ConfigMap reconciliation controller
  │   ├── drift_detector.go               # Detect ConfigMap drift
  │   └── writer.go                       # ConfigMap write operations
  ├── health/
  │   ├── validator.go                    # Health check validator interface
  │   └── http_checker.go                 # HTTP health check implementation
  ├── types.go                            # DiscoveredService, ToolsetConfig, etc.
  └── handlers.go                         # HTTP request handlers

internal/toolset/                         # Internal implementation details
  ├── k8s/
  │   └── client.go                       # Kubernetes client wrapper
  ├── cache/
  │   └── service_cache.go                # Discovered services cache
  └── metrics/
      └── collector.go                    # Prometheus metrics collector

test/unit/toolset/                        # Unit tests (70%+ coverage)
  ├── suite_test.go                       # Ginkgo test suite
  ├── prometheus_detector_test.go
  ├── grafana_detector_test.go
  ├── configmap_generator_test.go
  ├── reconciler_test.go
  ├── health_validator_test.go
  └── override_merger_test.go

test/integration/toolset/                 # Integration tests (>50% coverage)
  ├── suite_test.go
  ├── kind_discovery_test.go              # Kind cluster service discovery
  ├── configmap_reconciliation_test.go    # ConfigMap reconciliation
  └── end_to_end_discovery_test.go        # Complete discovery flow

test/e2e/toolset/                         # E2E tests (<10% coverage)
  ├── suite_test.go
  └── holmesgpt_integration_test.go       # Verify HolmesGPT API picks up toolsets
```

---

## Service Discovery Pattern

### ServiceDiscoverer Interface

**Location**: `pkg/toolset/discovery/discoverer.go`

```go
package discovery

import (
    "context"
    "time"

    "github.com/jordigilh/kubernaut/pkg/toolset"
    "go.uber.org/zap"
)

// ServiceDiscoverer discovers available Kubernetes services and generates toolset configurations
type ServiceDiscoverer interface {
    // DiscoverServices finds all detectable services in the cluster
    DiscoverServices(ctx context.Context) ([]toolset.DiscoveredService, error)

    // RegisterDetector adds a new service detector to the discovery pipeline
    RegisterDetector(detector ServiceDetector)

    // Start begins the discovery loop (every 5 minutes)
    Start(ctx context.Context) error

    // Stop gracefully shuts down the discovery loop
    Stop() error
}

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

### ServiceDiscoverer Implementation

**Location**: `pkg/toolset/discovery/discoverer.go`

```go
// ServiceDiscovererImpl implements service discovery with pluggable detectors
type ServiceDiscovererImpl struct {
    k8sClient      *kubernetes.Clientset
    detectors      []ServiceDetector
    logger         *zap.Logger
    discoveryCache map[string]toolset.DiscoveredService
    cacheTTL       time.Duration
    stopCh         chan struct{}
}

func NewServiceDiscoverer(
    k8sClient *kubernetes.Clientset,
    logger *zap.Logger,
) *ServiceDiscovererImpl {
    return &ServiceDiscovererImpl{
        k8sClient:      k8sClient,
        detectors:      []ServiceDetector{},
        logger:         logger,
        discoveryCache: make(map[string]toolset.DiscoveredService),
        cacheTTL:       5 * time.Minute,
        stopCh:         make(chan struct{}),
    }
}

func (d *ServiceDiscovererImpl) RegisterDetector(detector ServiceDetector) {
    d.logger.Info("Registering service detector",
        zap.String("service_type", detector.ServiceType()))
    d.detectors = append(d.detectors, detector)
}

func (d *ServiceDiscovererImpl) DiscoverServices(ctx context.Context) ([]toolset.DiscoveredService, error) {
    d.logger.Info("Starting service discovery")
    startTime := time.Now()

    // List all services in all namespaces
    services, err := d.k8sClient.CoreV1().Services("").List(ctx, metav1.ListOptions{})
    if err != nil {
        d.logger.Error("Failed to list services", zap.Error(err))
        return nil, fmt.Errorf("failed to list services: %w", err)
    }

    d.logger.Info("Retrieved services from Kubernetes",
        zap.Int("count", len(services.Items)))

    var discovered []toolset.DiscoveredService

    // Run each detector
    for _, detector := range d.detectors {
        detectorCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
        defer cancel()

        d.logger.Debug("Running service detector",
            zap.String("detector", detector.ServiceType()))

        detectedServices, err := detector.Detect(detectorCtx, services.Items)
        if err != nil {
            d.logger.Warn("Detector failed",
                zap.String("detector", detector.ServiceType()),
                zap.Error(err))
            continue // Don't fail entire discovery if one detector fails
        }

        // Validate health for each detected service
        for _, svc := range detectedServices {
            if err := detector.HealthCheck(detectorCtx, svc.Endpoint); err != nil {
                d.logger.Warn("Service health check failed, skipping",
                    zap.String("service_type", svc.Type),
                    zap.String("service_name", svc.Name),
                    zap.String("endpoint", svc.Endpoint),
                    zap.Error(err))
                continue
            }

            discovered = append(discovered, svc)
        }
    }

    duration := time.Since(startTime)
    d.logger.Info("Service discovery complete",
        zap.Int("discovered_count", len(discovered)),
        zap.Duration("duration", duration))

    // Update cache
    d.updateCache(discovered)

    // Record metrics
    recordDiscoveryMetrics(len(discovered), duration)

    return discovered, nil
}

func (d *ServiceDiscovererImpl) Start(ctx context.Context) error {
    d.logger.Info("Starting service discovery loop", zap.Duration("interval", 5*time.Minute))

    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()

    // Run discovery immediately on start
    if _, err := d.DiscoverServices(ctx); err != nil {
        d.logger.Error("Initial service discovery failed", zap.Error(err))
        return err
    }

    for {
        select {
        case <-ticker.C:
            if _, err := d.DiscoverServices(ctx); err != nil {
                d.logger.Error("Service discovery failed", zap.Error(err))
                // Continue running even if discovery fails
            }
        case <-d.stopCh:
            d.logger.Info("Stopping service discovery loop")
            return nil
        case <-ctx.Done():
            d.logger.Info("Service discovery loop context canceled")
            return ctx.Err()
        }
    }
}

func (d *ServiceDiscovererImpl) Stop() error {
    close(d.stopCh)
    return nil
}

func (d *ServiceDiscovererImpl) updateCache(services []toolset.DiscoveredService) {
    // Clear old cache
    d.discoveryCache = make(map[string]toolset.DiscoveredService)

    // Populate new cache
    for _, svc := range services {
        key := fmt.Sprintf("%s/%s", svc.Type, svc.Name)
        d.discoveryCache[key] = svc
    }
}
```

---

## Prometheus Detector Implementation

**Location**: `pkg/toolset/discovery/prometheus_detector.go`

```go
package discovery

import (
    "context"
    "fmt"
    "net/http"
    "time"

    corev1 "k8s.io/api/core/v1"
    "github.com/jordigilh/kubernaut/pkg/toolset"
    "go.uber.org/zap"
)

type PrometheusDetector struct {
    httpClient *http.Client
    logger     *zap.Logger
}

func NewPrometheusDetector(logger *zap.Logger) *PrometheusDetector {
    return &PrometheusDetector{
        httpClient: &http.Client{
            Timeout: 5 * time.Second,
        },
        logger: logger,
    }
}

func (d *PrometheusDetector) ServiceType() string {
    return "prometheus"
}

func (d *PrometheusDetector) Detect(
    ctx context.Context,
    services []corev1.Service,
) ([]toolset.DiscoveredService, error) {
    var discovered []toolset.DiscoveredService

    for _, svc := range services {
        // Check if service matches Prometheus patterns
        if !d.isPrometheus(svc) {
            continue
        }

        // Build endpoint URL
        endpoint := d.buildEndpoint(svc)

        discovered = append(discovered, toolset.DiscoveredService{
            Name:      svc.Name,
            Namespace: svc.Namespace,
            Type:      "prometheus",
            Endpoint:  endpoint,
            Labels:    svc.Labels,
            Metadata: map[string]string{
                "cluster_name": svc.ClusterName,
                "service_port": d.getPrometheusPort(svc),
            },
        })

        d.logger.Info("Detected Prometheus service",
            zap.String("name", svc.Name),
            zap.String("namespace", svc.Namespace),
            zap.String("endpoint", endpoint))
    }

    return discovered, nil
}

func (d *PrometheusDetector) isPrometheus(svc corev1.Service) bool {
    // Detection Strategy 1: Check labels
    if app, ok := svc.Labels["app"]; ok && app == "prometheus" {
        return true
    }

    if app, ok := svc.Labels["app.kubernetes.io/name"]; ok && app == "prometheus" {
        return true
    }

    // Detection Strategy 2: Check service name
    if svc.Name == "prometheus" || svc.Name == "prometheus-server" {
        return true
    }

    // Detection Strategy 3: Check for prometheus port
    for _, port := range svc.Spec.Ports {
        if port.Name == "web" && port.Port == 9090 {
            return true
        }
    }

    return false
}

func (d *PrometheusDetector) buildEndpoint(svc corev1.Service) string {
    port := d.getPrometheusPort(svc)
    return fmt.Sprintf("http://%s.%s.svc.cluster.local:%s", svc.Name, svc.Namespace, port)
}

func (d *PrometheusDetector) getPrometheusPort(svc corev1.Service) string {
    for _, port := range svc.Spec.Ports {
        if port.Name == "web" {
            return fmt.Sprintf("%d", port.Port)
        }
    }

    // Default to 9090
    return "9090"
}

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

    d.logger.Debug("Prometheus health check passed", zap.String("endpoint", endpoint))
    return nil
}
```

---

## Grafana Detector Implementation

**Location**: `pkg/toolset/discovery/grafana_detector.go`

```go
package discovery

import (
    "context"
    "fmt"
    "net/http"
    "time"

    corev1 "k8s.io/api/core/v1"
    "github.com/jordigilh/kubernaut/pkg/toolset"
    "go.uber.org/zap"
)

type GrafanaDetector struct {
    httpClient *http.Client
    logger     *zap.Logger
}

func NewGrafanaDetector(logger *zap.Logger) *GrafanaDetector {
    return &GrafanaDetector{
        httpClient: &http.Client{
            Timeout: 5 * time.Second,
        },
        logger: logger,
    }
}

func (d *GrafanaDetector) ServiceType() string {
    return "grafana"
}

func (d *GrafanaDetector) Detect(
    ctx context.Context,
    services []corev1.Service,
) ([]toolset.DiscoveredService, error) {
    var discovered []toolset.DiscoveredService

    for _, svc := range services {
        if !d.isGrafana(svc) {
            continue
        }

        endpoint := d.buildEndpoint(svc)

        discovered = append(discovered, toolset.DiscoveredService{
            Name:      svc.Name,
            Namespace: svc.Namespace,
            Type:      "grafana",
            Endpoint:  endpoint,
            Labels:    svc.Labels,
            Metadata: map[string]string{
                "service_port": d.getGrafanaPort(svc),
            },
        })

        d.logger.Info("Detected Grafana service",
            zap.String("name", svc.Name),
            zap.String("namespace", svc.Namespace),
            zap.String("endpoint", endpoint))
    }

    return discovered, nil
}

func (d *GrafanaDetector) isGrafana(svc corev1.Service) bool {
    // Detection Strategy 1: Check labels
    if app, ok := svc.Labels["app"]; ok && app == "grafana" {
        return true
    }

    if app, ok := svc.Labels["app.kubernetes.io/name"]; ok && app == "grafana" {
        return true
    }

    // Detection Strategy 2: Check service name
    if svc.Name == "grafana" {
        return true
    }

    // Detection Strategy 3: Check for grafana port
    for _, port := range svc.Spec.Ports {
        if port.Name == "service" && port.Port == 3000 {
            return true
        }
    }

    return false
}

func (d *GrafanaDetector) buildEndpoint(svc corev1.Service) string {
    port := d.getGrafanaPort(svc)
    return fmt.Sprintf("http://%s.%s.svc.cluster.local:%s", svc.Name, svc.Namespace, port)
}

func (d *GrafanaDetector) getGrafanaPort(svc corev1.Service) string {
    for _, port := range svc.Spec.Ports {
        if port.Name == "service" {
            return fmt.Sprintf("%d", port.Port)
        }
    }

    // Default to 3000
    return "3000"
}

func (d *GrafanaDetector) HealthCheck(ctx context.Context, endpoint string) error {
    healthURL := fmt.Sprintf("%s/api/health", endpoint)

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

    d.logger.Debug("Grafana health check passed", zap.String("endpoint", endpoint))
    return nil
}
```

---

## ConfigMap Management

### ConfigMap Generator Interface

**Location**: `pkg/toolset/generator/generator.go`

```go
package generator

import (
    "context"

    "github.com/jordigilh/kubernaut/pkg/toolset"
    corev1 "k8s.io/api/core/v1"
)

// ConfigMapGenerator generates toolset configuration for a specific service type
type ConfigMapGenerator interface {
    // Generate creates toolset YAML configuration for the service
    Generate(ctx context.Context, service toolset.DiscoveredService) (string, error)

    // ServiceType returns the service type this generator handles
    ServiceType() string
}

// ToolsetConfigMapBuilder builds the complete ConfigMap from discovered services
type ToolsetConfigMapBuilder struct {
    generators map[string]ConfigMapGenerator
}

func NewToolsetConfigMapBuilder() *ToolsetConfigMapBuilder {
    return &ToolsetConfigMapBuilder{
        generators: make(map[string]ConfigMapGenerator),
    }
}

func (b *ToolsetConfigMapBuilder) RegisterGenerator(gen ConfigMapGenerator) {
    b.generators[gen.ServiceType()] = gen
}

func (b *ToolsetConfigMapBuilder) BuildConfigMap(
    ctx context.Context,
    services []toolset.DiscoveredService,
    overrides map[string]string,
) (*corev1.ConfigMap, error) {
    configMapData := make(map[string]string)

    // Always include Kubernetes toolset (built-in)
    configMapData["kubernetes-toolset.yaml"] = generateKubernetesToolset()

    // Generate toolset configs for discovered services
    for _, svc := range services {
        generator, ok := b.generators[svc.Type]
        if !ok {
            continue // Skip services without generators
        }

        config, err := generator.Generate(ctx, svc)
        if err != nil {
            return nil, fmt.Errorf("failed to generate %s toolset: %w", svc.Type, err)
        }

        key := fmt.Sprintf("%s-toolset.yaml", svc.Type)
        configMapData[key] = config
    }

    // Merge overrides (admin-configured toolsets)
    for key, value := range overrides {
        if key == "overrides.yaml" {
            configMapData[key] = value // Preserve admin overrides
        }
    }

    // Build ConfigMap
    cm := &corev1.ConfigMap{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "kubernaut-toolset-config",
            Namespace: "kubernaut-system",
        },
        Data: configMapData,
    }

    return cm, nil
}

func generateKubernetesToolset() string {
    return `toolset: kubernetes
enabled: true
config:
  incluster: true
  namespaces: ["*"]
`
}
```

### Prometheus Toolset Generator

**Location**: `pkg/toolset/generator/prometheus_toolset.go`

```go
package generator

import (
    "context"
    "fmt"

    "github.com/jordigilh/kubernaut/pkg/toolset"
)

type PrometheusToolsetGenerator struct{}

func NewPrometheusToolsetGenerator() *PrometheusToolsetGenerator {
    return &PrometheusToolsetGenerator{}
}

func (g *PrometheusToolsetGenerator) ServiceType() string {
    return "prometheus"
}

func (g *PrometheusToolsetGenerator) Generate(
    ctx context.Context,
    service toolset.DiscoveredService,
) (string, error) {
    if service.Type != "prometheus" {
        return "", fmt.Errorf("invalid service type: %s", service.Type)
    }

    config := fmt.Sprintf(`toolset: prometheus
enabled: true
config:
  url: "%s"
  timeout: "30s"
  # Prometheus API queries will target this endpoint
  # Example queries: up{}, rate(http_requests_total[5m])
`, service.Endpoint)

    return config, nil
}
```

### Grafana Toolset Generator

**Location**: `pkg/toolset/generator/grafana_toolset.go`

```go
package generator

import (
    "context"
    "fmt"

    "github.com/jordigilh/kubernaut/pkg/toolset"
)

type GrafanaToolsetGenerator struct{}

func NewGrafanaToolsetGenerator() *GrafanaToolsetGenerator {
    return &GrafanaToolsetGenerator{}
}

func (g *GrafanaToolsetGenerator) ServiceType() string {
    return "grafana"
}

func (g *GrafanaToolsetGenerator) Generate(
    ctx context.Context,
    service toolset.DiscoveredService,
) (string, error) {
    if service.Type != "grafana" {
        return "", fmt.Errorf("invalid service type: %s", service.Type)
    }

    config := fmt.Sprintf(`toolset: grafana
enabled: true
config:
  url: "%s"
  apiKey: "${GRAFANA_API_KEY}"  # From Kubernetes Secret
  # Grafana API access for dashboard and panel queries
  # Requires GRAFANA_API_KEY environment variable
`, service.Endpoint)

    return config, nil
}
```

---

## Reconciliation Controller

### Reconciler Implementation

**Location**: `pkg/toolset/reconciler/reconciler.go`

```go
package reconciler

import (
    "context"
    "fmt"
    "time"

    corev1 "k8s.io/api/core/v1"
    "k8s.io/apimachinery/pkg/api/errors"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/client-go/kubernetes"
    "go.uber.org/zap"

    "github.com/jordigilh/kubernaut/pkg/toolset"
)

// ConfigMapReconciler ensures ConfigMap matches desired state
type ConfigMapReconciler struct {
    k8sClient     *kubernetes.Clientset
    logger        *zap.Logger
    configMapName string
    namespace     string
    stopCh        chan struct{}
}

func NewConfigMapReconciler(
    k8sClient *kubernetes.Clientset,
    logger *zap.Logger,
) *ConfigMapReconciler {
    return &ConfigMapReconciler{
        k8sClient:     k8sClient,
        logger:        logger,
        configMapName: "kubernaut-toolset-config",
        namespace:     "kubernaut-system",
        stopCh:        make(chan struct{}),
    }
}

func (r *ConfigMapReconciler) Start(ctx context.Context, desiredState *corev1.ConfigMap) error {
    r.logger.Info("Starting ConfigMap reconciliation loop",
        zap.String("configmap", r.configMapName),
        zap.String("namespace", r.namespace),
        zap.Duration("interval", 30*time.Second))

    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()

    // Run reconciliation immediately on start
    if err := r.Reconcile(ctx, desiredState); err != nil {
        r.logger.Error("Initial reconciliation failed", zap.Error(err))
        return err
    }

    for {
        select {
        case <-ticker.C:
            if err := r.Reconcile(ctx, desiredState); err != nil {
                r.logger.Error("Reconciliation failed", zap.Error(err))
                // Continue running even if reconciliation fails
            }
        case <-r.stopCh:
            r.logger.Info("Stopping reconciliation loop")
            return nil
        case <-ctx.Done():
            r.logger.Info("Reconciliation loop context canceled")
            return ctx.Err()
        }
    }
}

func (r *ConfigMapReconciler) Stop() error {
    close(r.stopCh)
    return nil
}

func (r *ConfigMapReconciler) Reconcile(ctx context.Context, desiredState *corev1.ConfigMap) error {
    r.logger.Debug("Reconciling ConfigMap")

    // Get current ConfigMap
    currentCM, err := r.k8sClient.CoreV1().ConfigMaps(r.namespace).
        Get(ctx, r.configMapName, metav1.GetOptions{})

    if errors.IsNotFound(err) {
        // ConfigMap deleted → recreate
        r.logger.Warn("ConfigMap not found, recreating",
            zap.String("configmap", r.configMapName))
        return r.createConfigMap(ctx, desiredState)
    }

    if err != nil {
        return fmt.Errorf("failed to get ConfigMap: %w", err)
    }

    // Detect drift
    hasDrift, driftDetails := r.detectDrift(currentCM, desiredState)

    if !hasDrift {
        r.logger.Debug("ConfigMap matches desired state, no reconciliation needed")
        return nil
    }

    r.logger.Info("ConfigMap drift detected, reconciling",
        zap.Strings("drift_keys", driftDetails))

    // Merge admin overrides
    merged := r.mergeOverrides(currentCM, desiredState)

    // Update ConfigMap
    return r.updateConfigMap(ctx, merged)
}

func (r *ConfigMapReconciler) detectDrift(current, desired *corev1.ConfigMap) (bool, []string) {
    var driftKeys []string

    // Check for missing keys in current
    for key := range desired.Data {
        if key == "overrides.yaml" {
            continue // Skip overrides, they're admin-managed
        }

        currentValue, ok := current.Data[key]
        if !ok {
            driftKeys = append(driftKeys, fmt.Sprintf("missing:%s", key))
            continue
        }

        if currentValue != desired.Data[key] {
            driftKeys = append(driftKeys, fmt.Sprintf("modified:%s", key))
        }
    }

    return len(driftKeys) > 0, driftKeys
}

func (r *ConfigMapReconciler) mergeOverrides(current, desired *corev1.ConfigMap) *corev1.ConfigMap {
    merged := desired.DeepCopy()

    // Preserve admin overrides from current ConfigMap
    if overrides, ok := current.Data["overrides.yaml"]; ok {
        merged.Data["overrides.yaml"] = overrides
        r.logger.Debug("Preserved admin overrides")
    }

    return merged
}

func (r *ConfigMapReconciler) createConfigMap(ctx context.Context, cm *corev1.ConfigMap) error {
    r.logger.Info("Creating ConfigMap", zap.String("configmap", cm.Name))

    _, err := r.k8sClient.CoreV1().ConfigMaps(r.namespace).
        Create(ctx, cm, metav1.CreateOptions{})

    if err != nil {
        return fmt.Errorf("failed to create ConfigMap: %w", err)
    }

    r.logger.Info("ConfigMap created successfully")
    return nil
}

func (r *ConfigMapReconciler) updateConfigMap(ctx context.Context, cm *corev1.ConfigMap) error {
    r.logger.Info("Updating ConfigMap", zap.String("configmap", cm.Name))

    _, err := r.k8sClient.CoreV1().ConfigMaps(r.namespace).
        Update(ctx, cm, metav1.UpdateOptions{})

    if err != nil {
        return fmt.Errorf("failed to update ConfigMap: %w", err)
    }

    r.logger.Info("ConfigMap updated successfully")
    return nil
}
```

---

## HTTP Server Implementation

### Server Setup

**Location**: `pkg/toolset/server.go`

```go
package toolset

import (
    "context"
    "fmt"
    "net/http"
    "time"

    "github.com/gorilla/mux"
    "go.uber.org/zap"
)

type Server struct {
    router     *mux.Router
    httpServer *http.Server
    logger     *zap.Logger
    discoverer ServiceDiscoverer
}

func NewServer(
    port int,
    logger *zap.Logger,
    discoverer ServiceDiscoverer,
) *Server {
    router := mux.NewRouter()

    server := &Server{
        router:     router,
        logger:     logger,
        discoverer: discoverer,
    }

    server.httpServer = &http.Server{
        Addr:         fmt.Sprintf(":%d", port),
        Handler:      router,
        ReadTimeout:  10 * time.Second,
        WriteTimeout: 10 * time.Second,
        IdleTimeout:  60 * time.Second,
    }

    server.registerRoutes()

    return server
}

func (s *Server) registerRoutes() {
    // Health checks (no auth required)
    s.router.HandleFunc("/health", s.healthHandler).Methods("GET")
    s.router.HandleFunc("/ready", s.readyHandler).Methods("GET")

    // API endpoints (auth required)
    api := s.router.PathPrefix("/api/v1").Subrouter()
    api.Use(s.authMiddleware)

    api.HandleFunc("/toolsets", s.listToolsetsHandler).Methods("GET")
    api.HandleFunc("/services", s.listServicesHandler).Methods("GET")
    api.HandleFunc("/discover", s.manualDiscoveryHandler).Methods("POST")

    // Metrics endpoint (auth required)
    s.router.Handle("/metrics", promhttp.Handler())
}

func (s *Server) Start() error {
    s.logger.Info("Starting HTTP server", zap.String("addr", s.httpServer.Addr))
    return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
    s.logger.Info("Shutting down HTTP server")
    return s.httpServer.Shutdown(ctx)
}
```

### API Handlers

**Location**: `pkg/toolset/handlers.go`

```go
package toolset

import (
    "encoding/json"
    "net/http"

    "go.uber.org/zap"
)

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

func (s *Server) readyHandler(w http.ResponseWriter, r *http.Request) {
    // Check if service is ready (discovery running, etc.)
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

func (s *Server) listToolsetsHandler(w http.ResponseWriter, r *http.Request) {
    // Return list of discovered toolsets
    services, err := s.discoverer.DiscoverServices(r.Context())
    if err != nil {
        s.logger.Error("Failed to discover services", zap.Error(err))
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }

    toolsets := make([]string, 0, len(services)+1)
    toolsets = append(toolsets, "kubernetes") // Always include Kubernetes toolset

    for _, svc := range services {
        toolsets = append(toolsets, svc.Type)
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{
        "toolsets": toolsets,
        "count":    len(toolsets),
    })
}

func (s *Server) listServicesHandler(w http.ResponseWriter, r *http.Request) {
    // Return detailed list of discovered services
    services, err := s.discoverer.DiscoverServices(r.Context())
    if err != nil {
        s.logger.Error("Failed to discover services", zap.Error(err))
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{
        "services": services,
        "count":    len(services),
    })
}

func (s *Server) manualDiscoveryHandler(w http.ResponseWriter, r *http.Request) {
    // Trigger manual service discovery
    s.logger.Info("Manual discovery triggered")

    services, err := s.discoverer.DiscoverServices(r.Context())
    if err != nil {
        s.logger.Error("Manual discovery failed", zap.Error(err))
        http.Error(w, "Discovery failed", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{
        "status":   "success",
        "services": services,
        "count":    len(services),
    })
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // TODO: Implement Kubernetes TokenReviewer authentication
        // For now, accept all requests
        next.ServeHTTP(w, r)
    })
}
```

---

## Error Handling

### Error Types

**Location**: `pkg/toolset/errors.go`

```go
package toolset

import (
    "errors"
    "fmt"
)

var (
    // ErrServiceNotFound indicates a service was not found
    ErrServiceNotFound = errors.New("service not found")

    // ErrHealthCheckFailed indicates a service health check failed
    ErrHealthCheckFailed = errors.New("health check failed")

    // ErrConfigMapNotFound indicates the ConfigMap was not found
    ErrConfigMapNotFound = errors.New("ConfigMap not found")

    // ErrInvalidServiceType indicates an unsupported service type
    ErrInvalidServiceType = errors.New("invalid service type")
)

// DetectionError wraps errors from service detection
type DetectionError struct {
    ServiceType string
    Err         error
}

func (e *DetectionError) Error() string {
    return fmt.Sprintf("detection failed for %s: %v", e.ServiceType, e.Err)
}

func (e *DetectionError) Unwrap() error {
    return e.Err
}

// ReconciliationError wraps errors from ConfigMap reconciliation
type ReconciliationError struct {
    Operation string
    Err       error
}

func (e *ReconciliationError) Error() string {
    return fmt.Sprintf("reconciliation %s failed: %v", e.Operation, e.Err)
}

func (e *ReconciliationError) Unwrap() error {
    return e.Err
}
```

### Error Handling Patterns

```go
// Example: Service detection with error wrapping
func (d *PrometheusDetector) Detect(ctx context.Context, services []corev1.Service) ([]DiscoveredService, error) {
    discovered, err := d.detectServices(ctx, services)
    if err != nil {
        return nil, &DetectionError{
            ServiceType: "prometheus",
            Err:         err,
        }
    }
    return discovered, nil
}

// Example: Health check with proper error handling
func (d *PrometheusDetector) HealthCheck(ctx context.Context, endpoint string) error {
    resp, err := d.httpClient.Get(endpoint + "/-/healthy")
    if err != nil {
        return fmt.Errorf("%w: %v", ErrHealthCheckFailed, err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("%w: status %d", ErrHealthCheckFailed, resp.StatusCode)
    }

    return nil
}

// Example: Reconciliation with recovery
func (r *ConfigMapReconciler) Reconcile(ctx context.Context, desired *corev1.ConfigMap) error {
    if err := r.reconcileConfigMap(ctx, desired); err != nil {
        r.logger.Error("Reconciliation failed, will retry", zap.Error(err))
        return &ReconciliationError{
            Operation: "update",
            Err:       err,
        }
    }
    return nil
}
```

---

## Main Application Entry Point

**Location**: `cmd/dynamic-toolset/main.go`

```go
package main

import (
    "context"
    "os"
    "os/signal"
    "syscall"

    "go.uber.org/zap"
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/rest"

    "github.com/jordigilh/kubernaut/pkg/toolset"
    "github.com/jordigilh/kubernaut/pkg/toolset/discovery"
    "github.com/jordigilh/kubernaut/pkg/toolset/generator"
    "github.com/jordigilh/kubernaut/pkg/toolset/reconciler"
)

func main() {
    // Initialize logger
    logger, _ := zap.NewProduction()
    defer logger.Sync()

    logger.Info("Starting Dynamic Toolset Service")

    // Create Kubernetes client (in-cluster)
    config, err := rest.InClusterConfig()
    if err != nil {
        logger.Fatal("Failed to create Kubernetes config", zap.Error(err))
    }

    k8sClient, err := kubernetes.NewForConfig(config)
    if err != nil {
        logger.Fatal("Failed to create Kubernetes client", zap.Error(err))
    }

    // Create service discoverer
    discoverer := discovery.NewServiceDiscoverer(k8sClient, logger)

    // Register service detectors
    discoverer.RegisterDetector(discovery.NewPrometheusDetector(logger))
    discoverer.RegisterDetector(discovery.NewGrafanaDetector(logger))
    discoverer.RegisterDetector(discovery.NewJaegerDetector(logger))
    discoverer.RegisterDetector(discovery.NewElasticsearchDetector(logger))

    // Create ConfigMap builder
    builder := generator.NewToolsetConfigMapBuilder()
    builder.RegisterGenerator(generator.NewPrometheusToolsetGenerator())
    builder.RegisterGenerator(generator.NewGrafanaToolsetGenerator())
    builder.RegisterGenerator(generator.NewJaegerToolsetGenerator())

    // Create reconciler
    reconcilerCtrl := reconciler.NewConfigMapReconciler(k8sClient, logger)

    // Create HTTP server
    server := toolset.NewServer(8080, logger, discoverer)

    // Start components
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Start discovery loop
    go func() {
        if err := discoverer.Start(ctx); err != nil {
            logger.Fatal("Service discovery failed", zap.Error(err))
        }
    }()

    // Start reconciliation loop
    go func() {
        // Build initial desired state
        services, _ := discoverer.DiscoverServices(ctx)
        desiredCM, _ := builder.BuildConfigMap(ctx, services, nil)

        if err := reconcilerCtrl.Start(ctx, desiredCM); err != nil {
            logger.Fatal("Reconciliation failed", zap.Error(err))
        }
    }()

    // Start HTTP server
    go func() {
        if err := server.Start(); err != nil {
            logger.Fatal("HTTP server failed", zap.Error(err))
        }
    }()

    // Wait for termination signal
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    <-sigCh

    logger.Info("Shutting down Dynamic Toolset Service")

    // Graceful shutdown
    discoverer.Stop()
    reconcilerCtrl.Stop()
    server.Shutdown(ctx)
}
```

---

## Type Definitions

**Location**: `pkg/toolset/types.go`

```go
package toolset

import "time"

// DiscoveredService represents a service discovered in the cluster
type DiscoveredService struct {
    Name      string            `json:"name"`
    Namespace string            `json:"namespace"`
    Type      string            `json:"type"` // "prometheus", "grafana", "jaeger", etc.
    Endpoint  string            `json:"endpoint"`
    Labels    map[string]string `json:"labels"`
    Metadata  map[string]string `json:"metadata"`
    Healthy   bool              `json:"healthy"`
    LastCheck time.Time         `json:"last_check"`
}

// ToolsetConfig represents a generated toolset configuration
type ToolsetConfig struct {
    Toolset string                 `yaml:"toolset"`
    Enabled bool                   `yaml:"enabled"`
    Config  map[string]interface{} `yaml:"config"`
}
```

---

**Document Status**: ✅ Complete Implementation Guide
**Last Updated**: October 10, 2025
**Next Steps**: Begin Phase 0 implementation following this guide


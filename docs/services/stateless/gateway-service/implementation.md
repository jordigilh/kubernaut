# Gateway Service - Implementation Details

**Version**: v1.0
**Last Updated**: October 4, 2025
**Status**: ✅ Design Complete

---

## Table of Contents

1. [Package Structure](#package-structure)
2. [Alert Adapter Pattern](#alert-adapter-pattern)
3. [HTTP Server Implementation](#http-server-implementation)
4. [Alert Processing Pipeline](#alert-processing-pipeline)
5. [Environment Classification](#environment-classification)
6. [Priority Assignment](#priority-assignment)
7. [Error Handling](#error-handling)

---

## Package Structure

### Directory Layout

Following Go idioms and Kubernaut patterns:

```
cmd/gateway/                         # Main application entry point
  └── main.go                        # Server initialization, dependency injection

pkg/gateway/                         # Business logic (PUBLIC API)
  ├── service.go                     # GatewayService interface
  ├── server.go                      # HTTP server implementation
  ├── adapters/
  │   ├── adapter.go                 # AlertAdapter interface (extracted)
  │   ├── prometheus_adapter.go     # Prometheus AlertManager adapter
  │   └── kubernetes_adapter.go     # Kubernetes Event watcher adapter
  ├── processing/
  │   ├── deduplication.go           # Fingerprint generation, Redis checks
  │   ├── storm_detection.go         # Rate + pattern detection
  │   ├── classification.go          # Environment classification
  │   └── priority.go                # Rego priority assignment
  ├── types.go                       # NormalizedSignal, ResourceIdentifier, etc.
  └── handlers.go                    # HTTP request handlers

internal/gateway/                    # Internal implementation details
  ├── redis/
  │   └── client.go                  # Redis connection pool, operations
  ├── cache/
  │   └── namespace_cache.go         # Environment classification cache
  └── rego/
      └── evaluator.go               # Rego policy evaluator

test/unit/gateway/                   # Unit tests (70%+ coverage)
  ├── suite_test.go                  # Ginkgo test suite
  ├── prometheus_adapter_test.go
  ├── kubernetes_adapter_test.go
  ├── deduplication_test.go
  ├── storm_detection_test.go
  ├── classification_test.go
  └── priority_test.go

test/integration/gateway/            # Integration tests (>50% coverage)
  ├── suite_test.go
  ├── redis_integration_test.go
  ├── crd_creation_test.go
  └── webhook_flow_test.go

test/e2e/gateway/                    # E2E tests (<10% coverage)
  ├── suite_test.go
  └── prometheus_to_remediation_test.go
```

---

## Signal Adapter Pattern

### Adapter-Specific Self-Registered Endpoints

**Decision**: Each adapter registers its own HTTP route for explicit, secure routing.

**Architecture**: Adapter-specific endpoints with configuration-driven registration
- Adapters implement `RoutableAdapter` interface
- Each adapter defines its route (e.g., `/api/v1/signals/prometheus`)
- Routes registered at server startup from enabled adapters
- No detection logic needed - HTTP router handles dispatch

**Benefits**:
- ✅ **Superior Security**: No source spoofing, explicit routing
- ✅ **Better Performance**: Direct routing (~50-100μs faster than detection)
- ✅ **Simpler Implementation**: ~60% less code (no detection logic)
- ✅ **Industry Standard**: Follows REST/HTTP best practices
- ✅ **Better Operations**: Clear errors, simple troubleshooting
- ✅ **Configuration-Driven**: Enable/disable adapters via config files

**See**:
- `ADAPTER_ENDPOINT_DESIGN_COMPARISON.md` - Design rationale (90% confidence)
- `CONFIGURATION_DRIVEN_ADAPTERS.md` - Configuration patterns

### Phase 1: Concrete Prometheus Adapter (3-4h)

**Location**: `pkg/gateway/adapters/prometheus_adapter.go`

```go
package adapters

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    "github.com/jordigilh/kubernaut/pkg/gateway"
)

// PrometheusAdapter handles Prometheus AlertManager webhook format
//
// Updated 2026-02-09: Now accepts an optional OwnerResolver for owner-chain-based
// fingerprinting. See BR-GATEWAY-004 for the fingerprint strategy.
type PrometheusAdapter struct {
    ownerResolver OwnerResolver // Optional: resolves top-level owner for fingerprinting
}

// OwnerResolver resolves the top-level controller owner of a Kubernetes resource.
// Used to fingerprint at the Deployment/StatefulSet level instead of the Pod level.
type OwnerResolver interface {
    ResolveTopLevelOwner(ctx context.Context, namespace, kind, name string) (ownerKind, ownerName string, err error)
}

func NewPrometheusAdapter(ownerResolver ...OwnerResolver) *PrometheusAdapter {
    adapter := &PrometheusAdapter{}
    if len(ownerResolver) > 0 && ownerResolver[0] != nil {
        adapter.ownerResolver = ownerResolver[0]
    }
    return adapter
}

// Parse converts Prometheus webhook payload to NormalizedSignal
func (a *PrometheusAdapter) Parse(ctx context.Context, rawData []byte) (*gateway.NormalizedSignal, error) {
    var webhook PrometheusWebhook
    if err := json.Unmarshal(rawData, &webhook); err != nil {
        return nil, fmt.Errorf("failed to parse Prometheus webhook: %w", err)
    }

    // Prometheus sends array of alerts
    if len(webhook.Alerts) == 0 {
        return nil, fmt.Errorf("no alerts in Prometheus webhook")
    }

    // Process first alert (Gateway handles one at a time)
    alert := webhook.Alerts[0]

    // Extract resource identifier from labels
    resource := gateway.ResourceIdentifier{
        Kind:      getResourceKind(alert.Labels),
        Name:      alert.Labels["pod"], // or "deployment", "node", etc.
        Namespace: alert.Labels["namespace"],
    }

    // Generate fingerprint for deduplication (adapter-specific strategy per BR-GATEWAY-004)
    // alertname is EXCLUDED from the fingerprint - LLM investigates resource state, not signal type
    var fingerprint string
    if a.ownerResolver != nil {
        // Owner-chain-based fingerprinting: resolve top-level owner
        ownerKind, ownerName, err := a.ownerResolver.ResolveTopLevelOwner(
            ctx, resource.Namespace, resource.Kind, resource.Name)
        if err == nil {
            // Fingerprint at owner level (e.g., Deployment)
            fingerprint = types.CalculateOwnerFingerprint(gateway.ResourceIdentifier{
                Namespace: resource.Namespace, Kind: ownerKind, Name: ownerName,
            })
        } else {
            // Fallback: resource without alertname
            fingerprint = types.CalculateOwnerFingerprint(resource)
        }
    } else {
        // No OwnerResolver: fingerprint resource directly (without alertname)
        fingerprint = types.CalculateOwnerFingerprint(resource)
    }

    internalAlert := &gateway.NormalizedSignal{
        Fingerprint:  fingerprint,
        AlertName:    alert.Labels["alertname"],
        Severity:     getSeverity(alert.Labels),
        Namespace:    alert.Labels["namespace"],
        Resource:     resource,
        Labels:       alert.Labels,
        Annotations:  alert.Annotations,
        FiringTime:   alert.StartsAt,
        ReceivedTime: time.Now(),
        SourceType:   "prometheus",
        RawPayload:   json.RawMessage(rawData),
    }

    return internalAlert, nil
}

// Validate checks if alert meets minimum requirements
func (a *PrometheusAdapter) Validate(alert *gateway.NormalizedSignal) error {
    if alert.AlertName == "" {
        return fmt.Errorf("missing alertname")
    }
    if alert.Namespace == "" {
        return fmt.Errorf("missing namespace")
    }
    if alert.Resource.Kind == "" || alert.Resource.Name == "" {
        return fmt.Errorf("missing resource identifier")
    }
    return nil
}

// SourceType returns the alert source identifier
func (a *PrometheusAdapter) SourceType() string {
    return "prometheus"
}

// PrometheusWebhook represents Alertmanager webhook payload
type PrometheusWebhook struct {
    Version  string            `json:"version"`
    GroupKey string            `json:"groupKey"`
    Alerts   []PrometheusAlert `json:"alerts"`
}

type PrometheusAlert struct {
    Status       string            `json:"status"` // "firing" or "resolved"
    Labels       map[string]string `json:"labels"`
    Annotations  map[string]string `json:"annotations"`
    StartsAt     time.Time         `json:"startsAt"`
    EndsAt       time.Time         `json:"endsAt"`
    GeneratorURL string            `json:"generatorURL"`
}

// Helper functions

func getResourceKind(labels map[string]string) string {
    // Prometheus labels follow convention: pod, deployment, node, etc.
    // Check in priority order (most specific first)
    switch {
    case labels["pod"] != "":
        return "Pod"
    case labels["deployment"] != "":
        return "Deployment"
    case labels["statefulset"] != "":
        return "StatefulSet"
    case labels["daemonset"] != "":
        return "DaemonSet"
    case labels["node"] != "":
        return "Node"
    default:
        return "Unknown"
    }
}

func getSeverity(labels map[string]string) string {
    // Prometheus standard severity label
    if severity, ok := labels["severity"]; ok {
        return severity // "critical", "warning", "info"
    }
    return "info" // default fallback
}

// Note: generateFingerprint() removed - fingerprinting now uses CalculateOwnerFingerprint()
// from pkg/gateway/types/fingerprint.go, which excludes alertname from the fingerprint.
```

**Unit Test** (`test/unit/gateway/prometheus_adapter_test.go`):

```go
package gateway_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "context"
    "time"

    "github.com/jordigilh/kubernaut/pkg/gateway/adapters"
)

var _ = Describe("BR-GATEWAY-001: Prometheus Adapter", func() {
    var (
        adapter *adapters.PrometheusAdapter
        ctx     context.Context
    )

    BeforeEach(func() {
        adapter = adapters.NewPrometheusAdapter()
        ctx = context.Background()
    })

    Context("when parsing valid Prometheus webhook", func() {
        It("should convert to NormalizedSignal", func() {
            payload := []byte(`{
                "version": "4",
                "groupKey": "{}:{alertname=\"HighMemoryUsage\"}",
                "alerts": [{
                    "status": "firing",
                    "labels": {
                        "alertname": "HighMemoryUsage",
                        "severity": "critical",
                        "namespace": "prod-payment-service",
                        "pod": "payment-api-789"
                    },
                    "annotations": {
                        "description": "Pod using 95% memory"
                    },
                    "startsAt": "2025-10-04T10:00:00Z"
                }]
            }`)

            alert, err := adapter.Parse(ctx, payload)

            Expect(err).ToNot(HaveOccurred())
            Expect(alert.AlertName).To(Equal("HighMemoryUsage"))
            Expect(alert.Severity).To(Equal("critical"))
            Expect(alert.Namespace).To(Equal("prod-payment-service"))
            Expect(alert.Resource.Kind).To(Equal("Pod"))
            Expect(alert.Resource.Name).To(Equal("payment-api-789"))
            Expect(alert.SourceType).To(Equal("prometheus"))
            Expect(alert.Fingerprint).ToNot(BeEmpty())
        })
    })

    Context("BR-GATEWAY-002: when alert missing required fields", func() {
        It("should fail validation", func() {
            payload := []byte(`{
                "version": "4",
                "alerts": [{
                    "status": "firing",
                    "labels": {
                        "alertname": "MissingNamespace"
                    }
                }]
            }`)

            alert, err := adapter.Parse(ctx, payload)
            Expect(err).ToNot(HaveOccurred())

            err = adapter.Validate(alert)
            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("missing namespace"))
        })
    })
})
```

---

### Phase 2: Concrete Kubernetes Events Adapter (4-5h)

**Location**: `pkg/gateway/adapters/kubernetes_adapter.go`

```go
package adapters

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    "github.com/jordigilh/kubernaut/pkg/gateway"
    corev1 "k8s.io/api/core/v1"
)

// KubernetesEventAdapter handles Kubernetes Event API format
//
// Updated 2026-02-09: Now accepts an optional OwnerResolver for owner-chain-based
// fingerprinting. See BR-GATEWAY-004 for the fingerprint strategy.
type KubernetesEventAdapter struct {
    ownerResolver OwnerResolver // Optional: resolves top-level owner for fingerprinting
}

// OwnerResolver resolves the top-level controller owner of a Kubernetes resource.
// Used to fingerprint at the Deployment/StatefulSet level instead of the Pod level.
type OwnerResolver interface {
    ResolveTopLevelOwner(ctx context.Context, namespace, kind, name string) (ownerKind, ownerName string, err error)
}

func NewKubernetesEventAdapter(ownerResolver ...OwnerResolver) *KubernetesEventAdapter {
    adapter := &KubernetesEventAdapter{}
    if len(ownerResolver) > 0 && ownerResolver[0] != nil {
        adapter.ownerResolver = ownerResolver[0]
    }
    return adapter
}

// Parse converts Kubernetes Event to NormalizedSignal
//
// Fingerprint strategy (updated 2026-02-09):
// - If OwnerResolver is configured: resolves owner chain (Pod → RS → Deployment)
//   and fingerprints as SHA256(namespace:ownerKind:ownerName), excluding event reason.
// - If OwnerResolver is nil or resolution fails: falls back to
//   SHA256(namespace:involvedObjectKind:involvedObjectName), still excluding reason.
// - Legacy behavior (no OwnerResolver): SHA256(reason:namespace:kind:name).
func (a *KubernetesEventAdapter) Parse(ctx context.Context, rawData []byte) (*gateway.NormalizedSignal, error) {
    var event corev1.Event
    if err := json.Unmarshal(rawData, &event); err != nil {
        return nil, fmt.Errorf("failed to parse Kubernetes event: %w", err)
    }

    // Only process Warning/Error events
    if event.Type != corev1.EventTypeWarning {
        return nil, fmt.Errorf("ignoring event type: %s (only Warning events)", event.Type)
    }

    // Extract resource from InvolvedObject
    resource := gateway.ResourceIdentifier{
        Kind:      event.InvolvedObject.Kind,
        Name:      event.InvolvedObject.Name,
        Namespace: event.InvolvedObject.Namespace,
    }

    // Generate fingerprint (adapter-specific strategy per BR-GATEWAY-004)
    var fingerprint string
    if a.ownerResolver != nil {
        // Owner-chain-based fingerprinting: resolve top-level owner
        ownerKind, ownerName, err := a.ownerResolver.ResolveTopLevelOwner(
            ctx, resource.Namespace, resource.Kind, resource.Name)
        if err == nil {
            // Fingerprint at owner level (e.g., Deployment)
            fingerprint = types.CalculateOwnerFingerprint(gateway.ResourceIdentifier{
                Namespace: resource.Namespace, Kind: ownerKind, Name: ownerName,
            })
        } else {
            // Fallback: involvedObject without reason
            fingerprint = types.CalculateOwnerFingerprint(resource)
        }
    } else {
        // Legacy: includes reason in fingerprint
        fingerprint = types.CalculateFingerprint(event.Reason, resource)
    }

    severity := mapEventReasonToSeverity(event.Reason)

    internalAlert := &gateway.NormalizedSignal{
        Fingerprint:  fingerprint,
        AlertName:    event.Reason,
        Severity:     severity,
        Namespace:    event.InvolvedObject.Namespace,
        Resource:     resource,
        Labels: map[string]string{
            "reason":    event.Reason,
            "component": event.Source.Component,
            "host":      event.Source.Host,
        },
        Annotations: map[string]string{
            "message": event.Message,
        },
        FiringTime:   event.LastTimestamp.Time, // Prefer lastTimestamp for freshness
        ReceivedTime: time.Now(),
        SourceType:   "kubernetes-event",
        RawPayload:   json.RawMessage(rawData),
    }

    return internalAlert, nil
}

// Validate checks if event meets requirements
func (a *KubernetesEventAdapter) Validate(alert *gateway.NormalizedSignal) error {
    if alert.AlertName == "" {
        return fmt.Errorf("missing alert name")
    }
    if alert.Resource.Kind == "" || alert.Resource.Name == "" {
        return fmt.Errorf("missing resource identifier")
    }
    return nil
}

// SourceType returns the alert source identifier
func (a *KubernetesEventAdapter) SourceType() string {
    return "kubernetes-event"
}

// mapEventReasonToAlertName converts K8s Event reasons to alert names
func mapEventReasonToAlertName(reason string) string {
    // Map common K8s event reasons to standardized alert names
    mapping := map[string]string{
        "OOMKilled":          "PodOOMKilled",
        "CrashLoopBackOff":   "PodCrashLoopBackOff",
        "ImagePullBackOff":   "ImagePullBackOff",
        "ErrImagePull":       "ImagePullError",
        "FailedScheduling":   "PodSchedulingFailed",
        "Evicted":            "PodEvicted",
        "BackOff":            "PodBackOff",
        "FailedMount":        "VolumeMountFailed",
        "FailedAttachVolume": "VolumeAttachFailed",
    }

    if alertName, ok := mapping[reason]; ok {
        return alertName
    }
    return reason // fallback to event reason
}

// mapEventReasonToSeverity assigns severity based on event reason
func mapEventReasonToSeverity(reason string) string {
    // Critical: Pod/Container failures
    criticalReasons := []string{
        "OOMKilled",
        "CrashLoopBackOff",
        "Evicted",
    }

    // Warning: Configuration/resource issues
    warningReasons := []string{
        "ImagePullBackOff",
        "FailedScheduling",
        "FailedMount",
        "BackOff",
    }

    for _, r := range criticalReasons {
        if r == reason {
            return "critical"
        }
    }

    for _, r := range warningReasons {
        if r == reason {
            return "warning"
        }
    }

    return "info" // default
}
```

**Event Watcher** (`pkg/gateway/adapters/kubernetes_watcher.go`):

```go
package adapters

import (
    "context"
    "fmt"
    "time"

    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/watch"
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/tools/cache"
    "sigs.k8s.io/controller-runtime/pkg/log"
)

// EventWatcher watches Kubernetes Events and sends them to processing pipeline
type EventWatcher struct {
    clientset    *kubernetes.Clientset
    adapter      *KubernetesEventAdapter
    eventHandler func(context.Context, *corev1.Event) error
}

func NewEventWatcher(clientset *kubernetes.Clientset, adapter *KubernetesEventAdapter, handler func(context.Context, *corev1.Event) error) *EventWatcher {
    return &EventWatcher{
        clientset:    clientset,
        adapter:      adapter,
        eventHandler: handler,
    }
}

// Start begins watching Kubernetes Events (Warning type only)
func (w *EventWatcher) Start(ctx context.Context) error {
    log := log.FromContext(ctx)
    log.Info("Starting Kubernetes Event watcher")

    // Create informer for Warning events
    listWatcher := cache.NewFilteredListWatchFromClient(
        w.clientset.CoreV1().RESTClient(),
        "events",
        metav1.NamespaceAll,
        func(options *metav1.ListOptions) {
            options.FieldSelector = "type=Warning"
        },
    )

    _, controller := cache.NewInformer(
        listWatcher,
        &corev1.Event{},
        time.Second*30, // resync period
        cache.ResourceEventHandlerFuncs{
            AddFunc: func(obj interface{}) {
                event := obj.(*corev1.Event)
                if err := w.eventHandler(ctx, event); err != nil {
                    log.Error(err, "Failed to process event",
                        "namespace", event.Namespace,
                        "name", event.Name,
                        "reason", event.Reason)
                }
            },
        },
    )

    // Run controller
    stopCh := make(chan struct{})
    go controller.Run(stopCh)

    // Wait for context cancellation
    <-ctx.Done()
    close(stopCh)
    return nil
}
```

---

### Phase 3: RoutableAdapter Interface with Self-Registration (2-3h)

**Location**: `pkg/gateway/adapters/adapter.go`

Adapters implement signal processing AND route registration:

```go
package adapters

import (
    "context"
    "github.com/jordigilh/kubernaut/pkg/gateway"
)

// SignalAdapter converts source-specific signal formats to internal format
type SignalAdapter interface {
    // Name returns the adapter identifier (e.g., "prometheus", "kubernetes-event")
    Name() string

    // Parse converts source-specific signal format to InternalSignal
    Parse(ctx context.Context, rawData []byte) (*gateway.InternalSignal, error)

    // Validate checks if signal meets minimum requirements
    Validate(signal *gateway.InternalSignal) error

    // GetMetadata returns adapter information for observability
    GetMetadata() AdapterMetadata
}

// RoutableAdapter extends SignalAdapter with HTTP route registration
// ALL adapters MUST implement this to register their endpoints
type RoutableAdapter interface {
    SignalAdapter

    // GetRoute returns the HTTP route path for this adapter
    // Example: "/api/v1/signals/prometheus"
    // This route is registered at server startup
    GetRoute() string
}

// AdapterMetadata provides adapter information
type AdapterMetadata struct {
    Name        string
    Version     string
    Description string
    SupportedContentTypes []string
    RequiredHeaders []string
}

// AdapterRegistry manages adapter registration (simplified - no detection logic)
type AdapterRegistry struct {
    adapters map[string]RoutableAdapter
    mu       sync.RWMutex
    log      *logrus.Logger
}

func NewAdapterRegistry(log *logrus.Logger) *AdapterRegistry {
    return &AdapterRegistry{
        adapters: make(map[string]RoutableAdapter),
        log:      log,
    }
}

func (r *AdapterRegistry) Register(adapter RoutableAdapter) error {
    r.mu.Lock()
    defer r.mu.Unlock()

    name := adapter.Name()
    if _, exists := r.adapters[name]; exists {
        return fmt.Errorf("adapter '%s' already registered", name)
    }

    r.adapters[name] = adapter
    r.log.WithFields(logrus.Fields{
        "adapter": name,
        "route":   adapter.GetRoute(),
    }).Info("Adapter registered")

    return nil
}

func (r *AdapterRegistry) GetAllAdapters() []RoutableAdapter {
    r.mu.RLock()
    defer r.mu.RUnlock()

    adapters := make([]RoutableAdapter, 0, len(r.adapters))
    for _, adapter := range r.adapters {
        adapters = append(adapters, adapter)
    }
    return adapters
}
```

**Key Simplifications**:
- ❌ **Removed**: `CanHandle()` method (no detection logic)
- ❌ **Removed**: `Priority()` method (HTTP routing handles priority)
- ❌ **Removed**: `DetectAndSelect()` method (direct routing)
- ✅ **Added**: `GetRoute()` method (defines HTTP endpoint)
- ✅ **Simpler**: ~60% less code than detection-based approach

**Each Adapter Implementation**:
```go
func (a *PrometheusAdapter) Name() string {
    return "prometheus"
}

func (a *PrometheusAdapter) GetRoute() string {
    return "/api/v1/signals/prometheus"
}

func (a *PrometheusAdapter) GetMetadata() AdapterMetadata {
    return AdapterMetadata{
        Name:        "prometheus",
        Version:     "1.0",
        Description: "Handles Prometheus AlertManager webhook notifications",
        SupportedContentTypes: []string{"application/json"},
    }
}
```

---

## HTTP Server Implementation

### Server Structure

**Location**: `pkg/gateway/server.go`

```go
package gateway

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "github.com/jordigilh/kubernaut/pkg/gateway/adapters"
    "github.com/jordigilh/kubernaut/pkg/gateway/processing"
    "github.com/prometheus/client_golang/prometheus"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/log"
)

// Server is the main Gateway HTTP server with dynamic adapter routing
type Server struct {
    // HTTP configuration
    httpAddr    string
    metricsAddr string

    // Kubernetes client for CRD operations
    k8sClient client.Client

    // Adapter registry (configuration-driven, no hardcoded adapters)
    adapterRegistry *adapters.AdapterRegistry

    // Processing components
    deduplication  *processing.DeduplicationService
    stormDetector  *processing.StormDetector
    classifier     *processing.EnvironmentClassifier
    priorityEngine *processing.PriorityEngine

    // Middleware
    authMiddleware      func(http.Handler) http.Handler
    rateLimitMiddleware func(http.Handler) http.Handler
}

func NewServer(
    httpAddr, metricsAddr string,
    k8sClient client.Client,
    adapterRegistry *adapters.AdapterRegistry,  // Passed from main (config-driven)
    redisClient *redis.Client,
) (*Server, error) {
    server := &Server{
        httpAddr:        httpAddr,
        metricsAddr:     metricsAddr,
        k8sClient:       k8sClient,
        adapterRegistry: adapterRegistry,  // No hardcoded adapters!

        // Initialize processing components
        deduplication:  processing.NewDeduplicationService(redisClient),
        stormDetector:  processing.NewStormDetector(redisClient),
        classifier:     processing.NewEnvironmentClassifier(k8sClient),
        priorityEngine: processing.NewPriorityEngine(),
    }

    // Initialize middleware
    server.authMiddleware = server.createAuthMiddleware()
    server.rateLimitMiddleware = server.createRateLimitMiddleware()

    return server, nil
}

// Start begins serving HTTP requests
func (s *Server) Start(ctx context.Context) error {
    log := log.FromContext(ctx)

    // Start port 8080: Health and application endpoints
    httpMux := http.NewServeMux()
    httpMux.HandleFunc("/health", s.handleHealth)
    httpMux.HandleFunc("/ready", s.handleReady)

    // ✅ ADAPTER-SPECIFIC ENDPOINTS: Register routes from enabled adapters
    // Each adapter defines its own route (e.g., /api/v1/signals/prometheus)
    // No generic /api/v1/signals endpoint - explicit routing only
    s.registerAdapterRoutes(httpMux)

    // Apply middleware
    httpHandler := s.rateLimitMiddleware(s.authMiddleware(httpMux))

    httpServer := &http.Server{
        Addr:         s.httpAddr,
        Handler:      httpHandler,
        ReadTimeout:  10 * time.Second,
        WriteTimeout: 10 * time.Second,
    }

    // Start port 9090: Metrics (with auth)
    metricsMux := http.NewServeMux()
    metricsMux.Handle("/metrics", promhttp.Handler())

    metricsServer := &http.Server{
        Addr:    s.metricsAddr,
        Handler: metricsMux,
    }

    // Start HTTP server
    go func() {
        log.Info("Starting HTTP server", "addr", s.httpAddr)
        if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Error(err, "HTTP server failed")
        }
    }()

    // Start metrics server
    go func() {
        log.Info("Starting metrics server", "addr", s.metricsAddr)
        if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Error(err, "Metrics server failed")
        }
    }()

    // Wait for context cancellation
    <-ctx.Done()

    // Graceful shutdown
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    if err := httpServer.Shutdown(shutdownCtx); err != nil {
        return fmt.Errorf("HTTP server shutdown failed: %w", err)
    }

    if err := metricsServer.Shutdown(shutdownCtx); err != nil {
        return fmt.Errorf("Metrics server shutdown failed: %w", err)
    }

    return nil
}

// handleHealth returns health status (no auth)
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("OK"))
}

// handleReady returns readiness status (no auth)
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
    // Check Redis connection
    if err := s.deduplication.HealthCheck(r.Context()); err != nil {
        http.Error(w, "Redis unavailable", http.StatusServiceUnavailable)
        return
    }

    w.WriteHeader(http.StatusOK)
    w.Write([]byte("READY"))
}
```

---

## Alert Processing Pipeline

### Main Handler Logic

**Location**: `pkg/gateway/handlers.go`

```go
package gateway

import (
    "context"
    "encoding/json"
    "io"
    "net/http"
    "time"

    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
    "sigs.k8s.io/controller-runtime/pkg/log"
)

// registerAdapterRoutes registers HTTP routes for all enabled adapters
// Each adapter defines its own route (e.g., /api/v1/signals/prometheus)
func (s *Server) registerAdapterRoutes(mux *http.ServeMux) {
    log := log.FromContext(context.Background())

    adapters := s.adapterRegistry.GetAllAdapters()
    if len(adapters) == 0 {
        log.Warn("No adapters registered - Gateway will have no signal endpoints")
        return
    }

    for _, adapter := range adapters {
        route := adapter.GetRoute()
        handler := s.makeAdapterHandler(adapter)

        mux.HandleFunc(route, handler)

        log.Info("Registered adapter route",
            "adapter", adapter.Name(),
            "route", route,
        )
    }

    log.Info("All adapter routes registered", "count", len(adapters))
}

// makeAdapterHandler creates an HTTP handler for a specific adapter
// This replaces the generic handleSignalIngestion with adapter-specific routing
func (s *Server) makeAdapterHandler(adapter adapters.RoutableAdapter) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        log := log.FromContext(ctx)
        startTime := time.Now()
        adapterName := adapter.Name()

        // Read request body
        body, err := io.ReadAll(r.Body)
        if err != nil {
            log.Error(err, "Failed to read request body", "adapter", adapterName)
            http.Error(w, "Invalid request body", http.StatusBadRequest)
            recordMetrics(adapterName, "parse_error", time.Since(startTime))
            return
        }

        // ✅ DIRECT ADAPTER USAGE - No detection needed!
        // HTTP router already selected the correct adapter based on URL path

        // Parse signal using this adapter
        signal, err := adapter.Parse(ctx, body)
        if err != nil {
            log.Error(err, "Signal parsing failed", "adapter", adapterName)
            http.Error(w, fmt.Sprintf("Invalid %s signal format: %v", adapterName, err), http.StatusBadRequest)
            recordMetrics(adapterName, "parse_error", time.Since(startTime))
            return
        }

        // Validate signal using adapter's validation logic
        if err := adapter.Validate(signal); err != nil {
            log.Error(err, "Signal validation failed", "adapter", adapterName)
            http.Error(w, fmt.Sprintf("Invalid %s signal data: %v", adapterName, err), http.StatusBadRequest)
            recordMetrics(adapterName, "validation_error", time.Since(startTime))
            return
        }

        // Process signal through pipeline (same for all sources)
        response, err := s.processSignal(ctx, signal)
        if err != nil {
            log.Error(err, "Failed to process signal", "fingerprint", signal.Fingerprint, "adapter", adapterName)
            http.Error(w, "Internal server error", http.StatusInternalServerError)
            recordMetrics(adapterName, "processing_error", time.Since(startTime))
            return
        }

        // Return success response
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusAccepted)
        json.NewEncoder(w).Encode(response)
        recordMetrics(adapterName, "success", time.Since(startTime))
    }
}

// ❌ REMOVED: handleSignalIngestion() - generic endpoint with detection
// ❌ REMOVED: detectSignalSource() - hardcoded detection logic
// ❌ REMOVED: getAdapterForSource() - hardcoded adapter selection
// ❌ REMOVED: AdapterRegistry.DetectAndSelect() - confidence-based detection
// ❌ REMOVED: adapter.CanHandle() - detection method
//
// ✅ REPLACED WITH: Direct HTTP routing to adapter-specific endpoints
//
// Benefits:
//   - ~60% less code (no detection logic)
//   - Better security (no source spoofing)
//   - Better performance (~50-100μs faster)
//   - Simpler troubleshooting (404 = wrong endpoint)
//   - Industry standard (follows REST patterns)

// ❌ NO BACKWARD COMPATIBILITY
// No legacy endpoints - clean break to adapter-specific routing
// Clients MUST use adapter-specific endpoints:
//   - POST /api/v1/signals/prometheus
//   - POST /api/v1/signals/kubernetes-event
//   - etc.

// processSignal executes the complete signal processing pipeline
// (works for both Prometheus alerts and Kubernetes events)
func (s *Server) processSignal(ctx context.Context, signal *NormalizedSignal) (*SignalResponse, error) {
    log := log.FromContext(ctx)

    // Step 1: Deduplication check
    isDuplicate, metadata, err := s.deduplication.Check(ctx, signal)
    if err != nil {
        return nil, fmt.Errorf("deduplication check failed: %w", err)
    }

    if isDuplicate {
        log.Info("Signal deduplicated",
            "fingerprint", signal.Fingerprint,
            "count", metadata.Count,
            "remediationRef", metadata.RemediationRequestRef)

        return &SignalResponse{
            Status:                  "deduplicated",
            Fingerprint:             signal.Fingerprint,
            Count:                   metadata.Count,
            RemediationRequestRef:   metadata.RemediationRequestRef,
        }, nil
    }

    // Step 2: Storm detection
    isStorm, stormMetadata, err := s.stormDetector.Check(ctx, signal)
    if err != nil {
        log.Warn("Storm detection failed", "error", err)
        // Non-fatal: continue without storm metadata
    }

    // Step 3: Environment classification
    environment, err := s.classifier.Classify(ctx, signal)
    if err != nil {
        log.Warn("Environment classification failed", "error", err)
        environment = "unknown" // fallback
    }
    signal.Environment = environment

    // Step 4: Priority assignment
    priority, err := s.priorityEngine.Assign(ctx, signal)
    if err != nil {
        log.Warn("Priority assignment failed", "error", err)
        priority = "P2" // safe fallback
    }
    signal.Priority = priority

    // Step 5: Create RemediationRequest CRD
    cr, err := s.createRemediationRequestCRD(ctx, signal, isStorm, stormMetadata)
    if err != nil {
        return nil, fmt.Errorf("failed to create RemediationRequest CRD: %w", err)
    }

    // Step 6: Store deduplication metadata
    if err := s.deduplication.Store(ctx, signal, cr.Name); err != nil {
        log.Warn("Failed to store deduplication metadata", "error", err)
        // Non-fatal: CRD created successfully
    }

    log.Info("Signal processed successfully",
        "fingerprint", signal.Fingerprint,
        "environment", environment,
        "priority", priority,
        "remediationRef", cr.Name,
        "isStorm", isStorm)

    return &SignalResponse{
        Status:                "accepted",
        Fingerprint:           signal.Fingerprint,
        RemediationRequestRef: cr.Name,
        Environment:           environment,
        Priority:              priority,
        IsStorm:               isStorm,
    }, nil
}

// SignalResponse is the HTTP response format (for all signal types)
type SignalResponse struct {
    Status                string `json:"status"` // "accepted" or "deduplicated"
    Fingerprint           string `json:"fingerprint"`
    Count                 int    `json:"count,omitempty"` // for duplicates
    RemediationRequestRef string `json:"remediationRequestRef"`
    Environment           string `json:"environment,omitempty"`
    Priority              string `json:"priority,omitempty"`
    IsStorm               bool   `json:"isStorm,omitempty"`
}
```

---

## Environment Classification

**Location**: `pkg/gateway/processing/classification.go`

```go
package processing

import (
    "context"
    "fmt"
    "sync"
    "time"

    "github.com/jordigilh/kubernaut/pkg/gateway"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/log"
)

// EnvironmentClassifier determines environment (any label value) for alerts
// Supports dynamic configuration: organizations define their own environment taxonomy
// Examples: "prod", "staging", "dev", "canary", "qa-eu", "prod-west", "blue", "green"
type EnvironmentClassifier struct {
    k8sClient client.Client

    // Cache namespace labels (5-minute TTL)
    cache      map[string]string // namespace -> environment (any non-empty string)
    cacheMutex sync.RWMutex
    cacheTTL   time.Duration

    // ConfigMap override (loaded at startup)
    configMapOverride map[string]string
}

func NewEnvironmentClassifier(k8sClient client.Client) *EnvironmentClassifier {
    classifier := &EnvironmentClassifier{
        k8sClient: k8sClient,
        cache:     make(map[string]string),
        cacheTTL:  5 * time.Minute,
    }

    // Load ConfigMap overrides
    classifier.loadConfigMapOverrides()

    // Start background cache refresh
    go classifier.refreshCacheLoop()

    return classifier
}

// Classify determines environment for alert (priority order)
func (c *EnvironmentClassifier) Classify(ctx context.Context, alert *gateway.NormalizedSignal) (string, error) {
    log := log.FromContext(ctx)

    // Priority 1: Check namespace labels (K8s API with cache)
    if env := c.getNamespaceLabelEnv(ctx, alert.Namespace); env != "" {
        environmentClassificationTotal.WithLabelValues("label", env).Inc()
        return env, nil
    }

    // Priority 2: Check ConfigMap override
    if env, ok := c.configMapOverride[alert.Namespace]; ok {
        environmentClassificationTotal.WithLabelValues("configmap", env).Inc()
        return env, nil
    }

    // Priority 3: Check alert labels (Prometheus alerts only)
    if alert.SourceType == "prometheus" {
        if env, ok := alert.Labels["environment"]; ok && env != "" {
            environmentClassificationTotal.WithLabelValues("alert_label", env).Inc()
            return env, nil
        }
    }

    // Priority 4: Default fallback
    log.Warn("Environment classification failed, using default",
        "namespace", alert.Namespace,
        "alertName", alert.AlertName)
    environmentClassificationTotal.WithLabelValues("unknown", "unknown").Inc()
    return "unknown", nil
}

// getNamespaceLabelEnv checks namespace labels (with cache)
func (c *EnvironmentClassifier) getNamespaceLabelEnv(ctx context.Context, namespace string) string {
    // Check cache first
    c.cacheMutex.RLock()
    if env, ok := c.cache[namespace]; ok {
        c.cacheMutex.RUnlock()
        environmentClassificationCacheHitRate.Set(1.0)
        return env
    }
    c.cacheMutex.RUnlock()
    environmentClassificationCacheHitRate.Set(0.0)

    // Fetch from K8s API
    var ns corev1.Namespace
    if err := c.k8sClient.Get(ctx, client.ObjectKey{Name: namespace}, &ns); err != nil {
        return ""
    }

    // Check standard label
    env := ns.Labels["environment"]

    // Update cache
    c.cacheMutex.Lock()
    c.cache[namespace] = env
    c.cacheMutex.Unlock()

    return env
}

// loadConfigMapOverrides loads environment overrides from ConfigMap
func (c *EnvironmentClassifier) loadConfigMapOverrides() {
    ctx := context.Background()
    var cm corev1.ConfigMap

    if err := c.k8sClient.Get(ctx, client.ObjectKey{
        Name:      "gateway-environment-classification",
        Namespace: "kubernaut-system",
    }, &cm); err != nil {
        // ConfigMap not found: use empty overrides
        c.configMapOverride = make(map[string]string)
        return
    }

    // Parse ConfigMap data
    c.configMapOverride = cm.Data
}

// refreshCacheLoop periodically refreshes namespace cache
func (c *EnvironmentClassifier) refreshCacheLoop() {
    ticker := time.NewTicker(2 * time.Minute)
    defer ticker.Stop()

    for range ticker.C {
        c.cacheMutex.Lock()
        // Clear cache (will be rebuilt on next requests)
        c.cache = make(map[string]string)
        c.cacheMutex.Unlock()
    }
}
```

---

## Priority Assignment

**Location**: `pkg/gateway/processing/priority.go`

```go
package processing

import (
    "context"
    "fmt"

    "github.com/jordigilh/kubernaut/pkg/gateway"
    "github.com/open-policy-agent/opa/rego"
    "sigs.k8s.io/controller-runtime/pkg/log"
)

// PriorityEngine assigns priority to alerts using Rego policies
type PriorityEngine struct {
    regoQuery *rego.PreparedEvalQuery
}

func NewPriorityEngine() *PriorityEngine {
    // Load Rego policy from ConfigMap (simplified for example)
    query, err := rego.New(
        rego.Query("data.kubernaut.priority.priority"),
        rego.Load([]string{"policy.rego"}, nil),
    ).PrepareForEval(context.Background())

    if err != nil {
        // Fallback: no Rego policy, use severity+environment matrix
        return &PriorityEngine{regoQuery: nil}
    }

    return &PriorityEngine{regoQuery: &query}
}

// Assign determines priority for alert
func (e *PriorityEngine) Assign(ctx context.Context, alert *gateway.NormalizedSignal) (string, error) {
    log := log.FromContext(ctx)

    // Try Rego evaluation if policy loaded
    if e.regoQuery != nil {
        priority, err := e.evaluateRego(ctx, alert)
        if err == nil {
            return priority, nil
        }
        log.Warn("Rego evaluation failed, falling back to severity+environment", "error", err)
    }

    // Fallback: severity + environment matrix
    return e.mapSeverityEnvironmentToPriority(alert), nil
}

// evaluateRego evaluates Rego policy for priority
func (e *PriorityEngine) evaluateRego(ctx context.Context, alert *gateway.NormalizedSignal) (string, error) {
    input := map[string]interface{}{
        "alertName":   alert.AlertName,
        "severity":    alert.Severity,
        "environment": alert.Environment,
        "namespace":   alert.Namespace,
        "resource": map[string]string{
            "kind":      alert.Resource.Kind,
            "name":      alert.Resource.Name,
            "namespace": alert.Resource.Namespace,
        },
        "labels": alert.Labels,
    }

    results, err := e.regoQuery.Eval(ctx, rego.EvalInput(input))
    if err != nil {
        return "", fmt.Errorf("rego evaluation failed: %w", err)
    }

    if len(results) == 0 || len(results[0].Expressions) == 0 {
        return "", fmt.Errorf("no rego results")
    }

    priority, ok := results[0].Expressions[0].Value.(string)
    if !ok {
        return "", fmt.Errorf("invalid rego result type")
    }

    return priority, nil
}

// mapSeverityEnvironmentToPriority fallback priority assignment
func (e *PriorityEngine) mapSeverityEnvironmentToPriority(alert *gateway.NormalizedSignal) string {
    switch {
    case alert.Severity == "critical" && alert.Environment == "prod":
        return "P0"
    case alert.Severity == "critical" && (alert.Environment == "staging" || alert.Environment == "prod"):
        return "P1"
    case alert.Severity == "warning" && alert.Environment == "prod":
        return "P1"
    case alert.Severity == "warning" && alert.Environment == "staging":
        return "P2"
    default:
        return "P2"
    }
}
```

---

## Error Handling

### HTTP Status Code Strategy

Following Decision 7 (Synchronous Error Handling):

| Status Code | Condition | Alertmanager Action |
|-------------|-----------|---------------------|
| **202 Accepted** | Alert accepted (CRD created or deduplicated) | No retry |
| **400 Bad Request** | Invalid alert format, missing fields | No retry (permanent error) |
| **429 Too Many Requests** | Rate limit exceeded | Retry with backoff |
| **500 Internal Server Error** | Transient error (Redis, K8s API) | Retry (Alertmanager retry logic) |
| **503 Service Unavailable** | Gateway not ready (Redis down) | Retry |

### Error Response Format

```go
type ErrorResponse struct {
    Error   string `json:"error"`
    Code    string `json:"code"` // "INVALID_FORMAT", "RATE_LIMITED", "INTERNAL_ERROR"
    Details string `json:"details,omitempty"`
}
```

### Retry Strategy

**Alertmanager Retry** (configured in Alertmanager):
```yaml
receivers:
  - name: kubernaut-gateway
    webhook_configs:
      - url: http://gateway-service.kubernaut-system:8080/api/v1/alerts/prometheus
        send_resolved: true
        http_config:
          bearer_token_file: /var/run/secrets/kubernetes.io/serviceaccount/token
        # Retry configuration
        max_alerts: 0 # no limit
```

Alertmanager automatically retries on 5xx responses with exponential backoff.

---

## Summary

Gateway Service implementation follows proven patterns:

1. **Value-First Adapters** - Concrete implementations first, interface extraction after validation
2. **Clean HTTP Handlers** - Standard library with middleware (auth, rate limiting)
3. **Separation of Concerns** - Adapters → Processing → CRD creation
4. **Environment Classification** - Namespace labels (cache) → ConfigMap → Alert labels
5. **Priority Assignment** - Rego policies with fallback matrix
6. **Synchronous Error Handling** - HTTP status codes, Alertmanager retry

**Confidence**: 85% (moderate complexity for adapters and Rego integration)

**Next**: [Deduplication Details](./deduplication.md)


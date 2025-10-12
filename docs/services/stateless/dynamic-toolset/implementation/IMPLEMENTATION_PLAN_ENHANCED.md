# Dynamic Toolset Service - Enhanced Implementation Plan

**Version**: v2.0 (Enhanced with Gateway Learnings)
**Date**: 2025-10-11
**Timeline**: 12-13 days (with quality enhancements)
**Status**: In Progress (Day 1 Complete)
**Based On**: SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md v1.0

---

## 🎯 Enhancements from Original Plan

This plan incorporates **8 critical gaps** and **12 enhancements** identified from Gateway implementation triage:

### Critical Improvements ⭐
1. **Integration-First Testing** (Day 8) - Tests architecture before details
2. **Schema Validation Checkpoint** (Day 7 EOD) - Prevents test failures
3. **Daily Progress Documentation** (Days 1, 4, 7, 12) - Better tracking
4. **BR Coverage Matrix** (Day 9) - Ensures 100% requirement coverage
5. **Production Readiness Checklist** (Day 12) - Reduces deployment issues
6. **Test Infrastructure Pre-Setup** (Day 7 EOD) - Prevents Day 8 blockers
7. **File Organization Strategy** (Day 12) - Cleaner git history
8. **Error Handling Philosophy** (Day 6) - Resilient service design

### Value-Add Enhancements 💡
9. Testing early-start assessment documentation
10. Design decision documentation at key milestones
11. Metrics validation checkpoint
12. Performance benchmarking
13. Troubleshooting guide
14. Confidence assessment template
15. Deployment manifest templates
16. Kind cluster E2E testing expansion
17. Authentication testing strategy
18. Daily implementation progress tracking
19. Makefile targets for consistency
20. Handoff summary structure

---

## Timeline Overview

| Day | Focus | Key Deliverables | Hours | Enhancements Applied |
|-----|-------|------------------|-------|---------------------|
| **1** | Foundation | Types, interfaces, K8s client | 8h | ✅ Status doc |
| **2** | Prometheus + Grafana | Detectors + health validator | 8h | DO-REFACTOR added |
| **3** | Jaeger + Elasticsearch | Detectors + standardization | 8h | DO-REFACTOR added |
| **4** | Custom + Discovery | Orchestration + optimization | 8h | ⭐ Midpoint doc |
| **5** | Generators | ConfigMap builder + templates | 8h | DO-REFACTOR added |
| **6** | Reconciliation | Controller + drift detection | 8h | ⭐ Error philosophy |
| **7** | Server + API | HTTP + metrics + main.go | 8h | ⭐ 4 EOD checkpoints |
| **8** | Testing Part 1 | ⭐ 5 Integration + Unit tests | 8h | ⭐ Integration-first |
| **9** | Testing Part 2 | More unit + advanced integration | 8h | ⭐ BR coverage matrix |
| **10** | E2E + Docs | End-to-end tests + documentation | 8h | Enhanced E2E |
| **11** | Documentation | Complete service documentation | 8h | Design decisions |
| **12** | CHECK + Production | ⭐ Enhanced validation | 8h | ⭐ 6 new deliverables |
| **13** | Buffer | Issues, polish, final handoff | 8h | NEW: Buffer day |

**Total**: 12-13 days (vs original 11-12)
**Reason for Extension**: Quality enhancements prevent 2-3 days of debugging later

---

## Detailed Day-by-Day Plan

### ✅ Day 1: Foundation - COMPLETE

**Status**: 100% Complete
**Documentation**: `phase0/01-day1-complete.md`

**Completed**:
- [x] Package structure created
- [x] Core types defined (`DiscoveredService`, `ToolsetConfig`, `DiscoveryMetadata`)
- [x] Interfaces defined (`ServiceDetector`, `ServiceDiscoverer`)
- [x] Kubernetes client wrapper implemented
- [x] Main.go skeleton with K8s connectivity test
- [x] Build successful, zero lint errors
- [x] Status documentation created

**Files Created**:
- `pkg/toolset/types.go`
- `pkg/toolset/discovery/detector.go`
- `pkg/toolset/discovery/discoverer.go`
- `internal/toolset/k8s/client.go`
- `cmd/dynamictoolset/main.go`
- `docs/services/stateless/dynamic-toolset/implementation/phase0/01-day1-complete.md`

---

### 🔄 Day 2: Prometheus + Grafana Detectors - IN PROGRESS

#### DO-RED: Prometheus Detector Tests (1.5h)
**File**: `test/unit/toolset/prometheus_detector_test.go`

**Tests to Write**:
- [x] Detect service by `app=prometheus` label
- [ ] Detect service by `app.kubernetes.io/name=prometheus`
- [ ] Detect service by port (name=web, port=9090)
- [ ] Build correct endpoint URL
- [ ] Handle custom port numbers
- [ ] Perform health check on `/-/healthy`
- [ ] Return correct service type ("prometheus")
- [ ] Skip non-Prometheus services

**BR Coverage**: BR-TOOLSET-010 (detection), BR-TOOLSET-011 (endpoint), BR-TOOLSET-012 (health)

#### DO-GREEN: Prometheus Detector Implementation (1.5h)
**File**: `pkg/toolset/discovery/prometheus_detector.go`

**Implementation Requirements**:
- Label-based detection (multiple strategies)
- Service name detection
- Port detection (name + number)
- Endpoint URL construction (cluster.local format)
- Health check via HTTP GET `/-/healthy`
- 5-second timeout, 3 retries

#### DO-RED: Grafana Detector Tests (1h)
**File**: `test/unit/toolset/grafana_detector_test.go`

Similar structure to Prometheus tests

#### DO-GREEN: Grafana Detector Implementation (1.5h)
**File**: `pkg/toolset/discovery/grafana_detector.go`

**Implementation Requirements**:
- Label-based detection (`app=grafana`)
- Port detection (name=service, port=3000)
- Health check via `/api/health`

#### DO-REFACTOR: Health Validator Extraction (2.5h) ⭐
**File**: `pkg/toolset/health/http_checker.go`

**Why**: Both detectors need HTTP health checks - extract common logic

**Implementation**:
```go
type HTTPHealthValidator struct {
    client  *http.Client
    timeout time.Duration
    retries int
}

func (v *HTTPHealthValidator) Check(ctx context.Context, endpoint string) error {
    // 5-second timeout, 3 retries, exponential backoff
}
```

**Refactor both detectors to use validator**

**Deliverables (Day 2)**:
- [ ] PrometheusDetector implementation + 8 tests
- [ ] GrafanaDetector implementation + 8 tests
- [ ] HTTPHealthValidator (shared)
- [ ] All tests passing
- [ ] EOD: Brief update to 01-day1-complete.md with Day 2 progress

---

### Day 3: Jaeger + Elasticsearch Detectors (8h)

#### DO-RED: Jaeger Detector Tests (1.5h)
**File**: `test/unit/toolset/jaeger_detector_test.go`

**Detection Criteria**:
- Label: `app=jaeger`
- Port: name=query, port=16686
- Health endpoint: `/` (200 OK)

#### DO-GREEN: Jaeger Detector Implementation (1.5h)
**File**: `pkg/toolset/discovery/jaeger_detector.go`

#### DO-RED: Elasticsearch Detector Tests (1.5h)
**File**: `test/unit/toolset/elasticsearch_detector_test.go`

**Detection Criteria**:
- Label: `app=elasticsearch`
- Port: 9200
- Health endpoint: `/_cluster/health` (200 OK, status green/yellow)

#### DO-GREEN: Elasticsearch Detector Implementation (1.5h)
**File**: `pkg/toolset/discovery/elasticsearch_detector.go`

#### DO-REFACTOR: Detector Interface Standardization (2h) ⭐
**Why**: All 4 detectors have similar patterns - extract utilities

**Create**: `pkg/toolset/discovery/detector_utils.go`

**Utilities**:
```go
// Label matching utilities
func HasLabel(svc corev1.Service, key, value string) bool
func HasAnyLabel(svc corev1.Service, labels map[string]string) bool

// Port matching utilities
func FindPort(svc corev1.Service, name string, port int32) *corev1.ServicePort
func GetPortNumber(svc corev1.Service, defaultPort int32) int32

// Endpoint construction
func BuildEndpoint(name, namespace string, port int32) string
```

**Refactor all 4 detectors to use shared utilities**

**Deliverables (Day 3)**:
- [ ] JaegerDetector implementation + tests
- [ ] ElasticsearchDetector implementation + tests
- [ ] Shared detector utilities
- [ ] All 4 detectors consistent
- [ ] All tests passing (20+ tests total)

---

### Day 4: Custom Detector + Service Discovery Orchestration (8h)

#### DO-RED: Custom Detector Tests (2h)
**File**: `test/unit/toolset/custom_detector_test.go`

**Detection via Annotations**:
```yaml
annotations:
  kubernaut.io/toolset: "true"
  kubernaut.io/toolset-type: "custom"
  kubernaut.io/toolset-name: "my-service"
  kubernaut.io/toolset-endpoint: "http://my-service:8080"  # optional
  kubernaut.io/toolset-health-endpoint: "/health"  # optional
```

#### DO-GREEN: Custom Detector Implementation (1.5h)
**File**: `pkg/toolset/discovery/custom_detector.go`

#### DO-RED: Service Discoverer Tests (2h)
**File**: `test/unit/toolset/service_discoverer_test.go`

**Test Scenarios**:
- Detector registration
- List services from Kubernetes API
- Run all detectors in sequence
- Health validation per service
- Cache updates
- Discovery loop (Start/Stop)
- Error handling (detector failures, API failures)

#### DO-GREEN: Service Discoverer Implementation (1.5h)
**File**: `pkg/toolset/discovery/discoverer_impl.go`

**Implementation**:
```go
type ServiceDiscovererImpl struct {
    k8sClient      kubernetes.Interface
    detectors      []ServiceDetector
    logger         *zap.Logger
    discoveryCache map[string]toolset.DiscoveredService
    cacheTTL       time.Duration
    stopCh         chan struct{}
}
```

**Key Methods**:
- `DiscoverServices()` - orchestrates discovery
- `RegisterDetector()` - adds detector
- `Start()` - 5-minute discovery loop
- `Stop()` - graceful shutdown

#### DO-REFACTOR: Discovery Pipeline Optimization (1h) ⭐
**Why**: Improve efficiency and error handling

**Enhancements**:
- Parallel detector execution (goroutines)
- Error handling standardization
- Cache invalidation strategy
- Structured logging consistency
- Metrics recording hooks

**Deliverables (Day 4)**:
- [ ] CustomDetector implementation + tests
- [ ] ServiceDiscoverer orchestration
- [ ] Discovery loop with Start/Stop
- [ ] Service cache
- [ ] All 5 detectors integrated
- [ ] **⭐ EOD: Create `02-day4-midpoint.md` status doc**

**Midpoint Documentation Should Include**:
- Components completed (5 detectors + discoverer)
- Integration status (all detectors registered)
- Test coverage so far
- Any blockers or deviations
- Confidence assessment (target: 85%+)

---

### Day 5: ConfigMap Generation + Toolset Generators (8h)

#### DO-RED + DO-GREEN: Toolset Generators (6h)

**Pattern for Each Generator** (1h each):

1. **Kubernetes Toolset Generator** (0.5h test + 0.5h impl)
   - File: `pkg/toolset/generator/kubernetes_toolset.go`
   - Static YAML generation (always enabled)

2. **Prometheus Toolset Generator** (0.5h test + 0.5h impl)
   - File: `pkg/toolset/generator/prometheus_toolset.go`
   - Dynamic URL from DiscoveredService

3. **Grafana Toolset Generator** (0.5h test + 0.5h impl)
   - File: `pkg/toolset/generator/grafana_toolset.go`
   - API key placeholder `${GRAFANA_API_KEY}`

4. **Jaeger Toolset Generator** (0.5h test + 0.5h impl)
   - File: `pkg/toolset/generator/jaeger_toolset.go`

5. **Elasticsearch Toolset Generator** (0.5h test + 0.5h impl)
   - File: `pkg/toolset/generator/elasticsearch_toolset.go`

6. **ConfigMap Builder** (1h test + 1h impl)
   - File: `pkg/toolset/generator/generator.go`
   - Registers all generators
   - Builds complete ConfigMap
   - Preserves `overrides.yaml`

#### DO-REFACTOR: Generator Pattern Standardization (1.5h) ⭐
**Why**: All generators have similar structure - extract patterns

**Create**: `pkg/toolset/generator/generator_utils.go`

**Utilities**:
```go
// Template-based generation
func GenerateToolsetYAML(toolset string, enabled bool, config map[string]interface{}) (string, error)

// Environment variable handling
func AddEnvPlaceholder(config map[string]interface{}, key string, envVar string)

// Override merging
func MergeOverrides(generated map[string]string, overrides map[string]string) map[string]string
```

**Deliverables (Day 5)**:
- [ ] All 5 toolset generators implemented + tested
- [ ] ConfigMap builder with generator registry
- [ ] Override preservation logic
- [ ] Shared generator utilities
- [ ] All ConfigMap generation tests passing (15+ tests)

---

### Day 6: ConfigMap Reconciliation Controller (8h)

#### DO-RED: Drift Detection Tests (2h)
**File**: `test/unit/toolset/drift_detector_test.go`

**Scenarios**:
- Missing keys in current ConfigMap
- Modified values in current ConfigMap
- Override preservation (`overrides.yaml` untouched)
- No drift (no-op)

#### DO-GREEN: Drift Detector Implementation (1h)
**File**: `pkg/toolset/reconciler/drift_detector.go`

#### DO-RED: Reconciler Tests (2h)
**File**: `test/unit/toolset/reconciler_test.go`

**Scenarios**:
- ConfigMap creation when missing
- ConfigMap update on drift
- Override merge logic
- No-op when state matches
- Owner reference management
- Reconciliation interval (30 seconds)

#### DO-GREEN: Reconciler Implementation (2h)
**File**: `pkg/toolset/reconciler/reconciler.go`

**Implementation**:
```go
type ConfigMapReconciler struct {
    k8sClient     kubernetes.Interface
    logger        *zap.Logger
    configMapName string
    namespace     string
    stopCh        chan struct{}
}
```

**Key Methods**:
- `Start()` - 30-second reconciliation loop
- `Stop()` - graceful shutdown
- `Reconcile()` - main reconciliation logic

#### DO-REFACTOR: Error Handling + Reconciliation Robustness (1h) ⭐
**Why**: Define error handling philosophy and graceful degradation

**Create**: `pkg/toolset/errors.go` (expanded)

**Error Types**:
```go
var (
    ErrServiceNotFound      = errors.New("service not found")
    ErrHealthCheckFailed    = errors.New("health check failed")
    ErrConfigMapNotFound    = errors.New("ConfigMap not found")
    ErrInvalidServiceType   = errors.New("invalid service type")
)

type DetectionError struct {
    ServiceType string
    Err         error
}

type ReconciliationError struct {
    Operation string
    Err       error
}
```

**Graceful Degradation Philosophy** ⭐

**File**: `pkg/toolset/GRACEFUL_DEGRADATION.md`

```markdown
## Error Severity Classification

### Critical Errors (Stop Operation)
- Kubernetes API unavailable → Stop discovery, return error
- ConfigMap permissions denied → Stop reconciliation, alert

### Non-Critical Errors (Log + Continue)
- Service health check fails → Log warning, skip service, continue
- Single detector fails → Log error, continue with other detectors

### Recoverable Errors (Retry)
- ConfigMap write fails → Retry with exponential backoff (3 attempts)
- Transient K8s API errors → Retry current operation

## Retry Strategy
- Max retries: 3
- Initial delay: 1 second
- Backoff multiplier: 2x
- Max delay: 10 seconds
```

**Deliverables (Day 6)**:
- [ ] ConfigMap reconciliation controller
- [ ] Drift detection logic
- [ ] Override preservation working
- [ ] 30-second reconciliation loop
- [ ] Structured error types
- [ ] **⭐ Graceful degradation philosophy documented**
- [ ] All reconciliation tests passing (10+ tests)

---

### Day 7: HTTP Server + REST API + Metrics (8h)

#### DO-RED + DO-GREEN: HTTP Server (3h)

**Files**:
- `pkg/toolset/server.go` (server struct, routes)
- `pkg/toolset/handlers.go` (API handlers)

**Implementation**:
- Server struct with gorilla/mux router
- Route registration
- Health endpoint (`/healthz`)
- Readiness endpoint (`/readyz`)
- API endpoints:
  - `GET /api/v1/toolsets` (with filters)
  - `GET /api/v1/toolsets/{name}`
  - `GET /api/v1/services` (with filters)
  - `POST /api/v1/discover`
- Metrics endpoint (`/metrics`)
- Port 8080 binding

#### Metrics Implementation (2h)

**File**: `internal/toolset/metrics/collector.go`

**Metrics (10+ required)**:
```go
dynamictoolset_services_discovered_total{type}
dynamictoolset_toolset_healthy{name}
dynamictoolset_discovery_duration_seconds{phase}
dynamictoolset_configmap_reconcile_total
dynamictoolset_configmap_drift_detected_total
dynamictoolset_http_requests_total{endpoint,code}
dynamictoolset_http_request_duration_seconds{endpoint}
dynamictoolset_health_check_failures_total{service_type}
dynamictoolset_detector_errors_total{detector}
dynamictoolset_reconciliation_errors_total{operation}
```

#### Main Application Integration (2h)

**File**: `cmd/dynamictoolset/main.go` (complete)

**Wire all components**:
```go
// Create detectors
prometheusDetector := discovery.NewPrometheusDetector(logger)
grafanaDetector := discovery.NewGrafanaDetector(logger)
jaegerDetector := discovery.NewJaegerDetector(logger)
elasticsearchDetector := discovery.NewElasticsearchDetector(logger)
customDetector := discovery.NewCustomDetector(logger)

// Create discoverer
discoverer := discovery.NewServiceDiscoverer(k8sClient, logger)
discoverer.RegisterDetector(prometheusDetector)
discoverer.RegisterDetector(grafanaDetector)
discoverer.RegisterDetector(jaegerDetector)
discoverer.RegisterDetector(elasticsearchDetector)
discoverer.RegisterDetector(customDetector)

// Create generators
builder := generator.NewToolsetConfigMapBuilder()
builder.RegisterGenerator(generator.NewKubernetesToolsetGenerator())
builder.RegisterGenerator(generator.NewPrometheusToolsetGenerator())
builder.RegisterGenerator(generator.NewGrafanaToolsetGenerator())
builder.RegisterGenerator(generator.NewJaegerToolsetGenerator())
builder.RegisterGenerator(generator.NewElasticsearchToolsetGenerator())

// Create reconciler
reconciler := reconciler.NewConfigMapReconciler(k8sClient, logger)

// Create HTTP server
server := toolset.NewServer(8080, logger, discoverer, builder)

// Start components
go discoverer.Start(ctx)
go reconciler.Start(ctx, desiredConfigMap)
go server.Start()

// Wait for shutdown
// Stop components gracefully
```

#### DO-REFACTOR: Middleware + Response Standardization (1h) ⭐

**Files**:
- `pkg/toolset/middleware/logging.go`
- `pkg/toolset/middleware/correlation.go`
- `pkg/toolset/middleware/errors.go`

**Middleware Stack**:
1. Request logging
2. Correlation ID generation
3. Error response standardization
4. Content-Type validation

#### ⭐ CRITICAL EOD CHECKPOINTS (4 items - Very Important!)

**1. Schema Validation** (30 min) ⭐

**File**: `implementation/design/01-configmap-schema-validation.md`

```markdown
## ConfigMap Schema Validation

### HolmesGPT SDK Requirements
- [ ] Toolset name format validated
- [ ] Config field types verified
- [ ] Environment variable syntax correct (${VAR_NAME})
- [ ] Override format documented

### Field Alignment
- [ ] Kubernetes toolset: incluster, namespaces
- [ ] Prometheus toolset: url, timeout
- [ ] Grafana toolset: url, apiKey
- [ ] Jaeger toolset: url, timeout
- [ ] Elasticsearch toolset: url, index

### 100% Validation Complete ✅
```

**2. Test Infrastructure Setup** (20 min) ⭐

**Files**:
- `test/integration/toolset/suite_test.go` (skeleton)
- `test/integration/toolset/test_helpers.go`

**Setup**:
```go
import (
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/testutil/kind"
)

var suite *kind.IntegrationSuite

var _ = BeforeSuite(func() {
    // Connect to existing Kind cluster and create test namespaces
    // See: docs/testing/KIND_CLUSTER_TEST_TEMPLATE.md
    suite = kind.Setup("toolset-test", "kubernaut-system")
})

var _ = AfterSuite(func() {
    // Automatic cleanup of namespaces and registered resources
    suite.Cleanup()
})
```

**3. Status Documentation** (30 min) ⭐

**File**: `implementation/phase0/03-day7-complete.md`

```markdown
# Day 7 Complete

## Completed
- [x] HTTP server with all endpoints
- [x] 10+ Prometheus metrics
- [x] Main application fully wired
- [x] All components integrated

## Validation
- [x] Service compiles
- [x] All tests passing (40+ unit tests)
- [x] Schema validated
- [x] Test infrastructure ready

## Ready for Day 8
- Integration tests can begin immediately
- No blockers identified
```

**4. Testing Strategy Documentation** (30 min) ⭐

**File**: `implementation/testing/01-integration-first-rationale.md`

```markdown
## Why Integration-First Testing?

### Traditional Approach (DON'T DO)
Day 8-9: Unit tests
Day 10: Integration tests
**Risk**: Architecture issues found late (expensive to fix)

### Integration-First Approach (DO THIS) ✅
Day 8 Morning: 5 critical integration tests
Day 8 Afternoon: Unit tests
**Benefit**: Architecture validated early (cheap to fix)

### Evidence from Gateway
- Caught function signature mismatches on Day 7
- Found timing issues early
- Saved 2+ days of debugging

### Dynamic Toolset Integration Tests (Day 8 Morning)
1. Discovery → ConfigMap (validates core flow)
2. Health Check (validates detector logic)
3. Reconciliation (validates controller logic)
4. Override Preservation (validates merge logic)
5. Multi-Detector (validates orchestration)
```

**Deliverables (Day 7)** - ENHANCED:
- [ ] HTTP server with all endpoints
- [ ] 10+ Prometheus metrics
- [ ] Main application fully wired
- [ ] All components integrated
- [ ] Middleware standardized
- [ ] **⭐ Schema validation complete**
- [ ] **⭐ Test infrastructure ready**
- [ ] **⭐ 03-day7-complete.md created**
- [ ] **⭐ Testing strategy documented**

**Why These 4 EOD Items Matter**: Gateway learned these prevent 2+ days of debugging

---

### Day 8: Integration-First Testing ⭐ (8h)

**CRITICAL CHANGE**: Integration tests BEFORE unit tests

#### Morning: 5 Critical Integration Tests (4h) ⭐

**File**: `test/integration/toolset/core_integration_test.go`

**Test 1: Basic Discovery → ConfigMap (90 min)**
```go
import (
    "context"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/testutil/kind"
    "github.com/jordigilh/kubernaut/pkg/toolset/discovery"
    "github.com/jordigilh/kubernaut/pkg/toolset/configmap"
)

Describe("Integration Test 1: Service Discovery to ConfigMap", func() {
    It("should discover Prometheus service and create ConfigMap", func() {
        // Deploy Prometheus service to Kind cluster
        svc, err := suite.DeployPrometheusService("toolset-test")
        Expect(err).ToNot(HaveOccurred())

        // Run discovery
        discoverer := discovery.NewServiceDiscoverer(suite.Client, logger)
        services, err := discoverer.DiscoverServices(suite.Context)
        Expect(err).ToNot(HaveOccurred())
        Expect(services).To(HaveLen(1))

        // Generate and create ConfigMap
        generator := configmap.NewGenerator(logger)
        cm, err := generator.GenerateConfigMap(services)
        Expect(err).ToNot(HaveOccurred())

        writer := configmap.NewWriter(suite.Client, logger)
        err = writer.WriteConfigMap(suite.Context, "kubernaut-system", cm)
        Expect(err).ToNot(HaveOccurred())

        // Verify ConfigMap created
        retrievedCM := suite.WaitForConfigMap("kubernaut-system",
            "kubernaut-toolset-config", 30*time.Second)
        Expect(retrievedCM.Data).To(HaveKey("prometheus-toolset.yaml"))

        // Verify endpoint correct
        expectedEndpoint := suite.GetServiceEndpoint("prometheus", "toolset-test", 9090)
        Expect(retrievedCM.Data["prometheus-toolset.yaml"]).To(ContainSubstring(expectedEndpoint))
    })
})
```

**Test 2: Health Check Validation (45 min)**
```go
import (
    "context"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    corev1 "k8s.io/api/core/v1"

    "github.com/jordigilh/kubernaut/pkg/testutil/kind"
    "github.com/jordigilh/kubernaut/pkg/toolset/discovery"
    "github.com/jordigilh/kubernaut/pkg/toolset/health"
)

Describe("Integration Test 2: Health Check Filtering", func() {
    It("should skip unhealthy services", func() {
        // Deploy service with custom (unhealthy) endpoint
        svc, err := suite.DeployService(kind.ServiceConfig{
            Name:      "unhealthy-prometheus",
            Namespace: "toolset-test",
            Labels:    map[string]string{"app": "prometheus", "health": "failing"},
            Ports:     []corev1.ServicePort{{Name: "web", Port: 9090}},
        })
        Expect(err).ToNot(HaveOccurred())

        // Run discovery with health checking enabled
        discoverer := discovery.NewServiceDiscoverer(suite.Client, logger)
        discoverer.SetHealthChecker(health.NewChecker(httpClient, logger))

        services, err := discoverer.DiscoverServices(suite.Context)
        Expect(err).ToNot(HaveOccurred())

        // Verify unhealthy service NOT in results
        for _, svc := range services {
            Expect(svc.Name).ToNot(Equal("unhealthy-prometheus"))
        }
    })
})
```

**Test 3: ConfigMap Reconciliation (60 min)**
```go
import (
    "context"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/testutil/kind"
    "github.com/jordigilh/kubernaut/pkg/toolset/reconciler"
)

Describe("Integration Test 3: Drift Recovery", func() {
    It("should reconcile modified ConfigMap", func() {
        // Create initial ConfigMap
        initialCM, err := suite.DeployConfigMap(kind.ConfigMapConfig{
            Name:      "kubernaut-toolset-config",
            Namespace: "kubernaut-system",
            Data: map[string]string{
                "prometheus-toolset.yaml": "enabled: true\nendpoint: http://prometheus:9090",
            },
        })
        Expect(err).ToNot(HaveOccurred())

        // Manually modify ConfigMap (simulate drift)
        initialCM.Data["prometheus-toolset.yaml"] = "enabled: false"  // Wrong value
        _, err = suite.UpdateConfigMap(initialCM)
        Expect(err).ToNot(HaveOccurred())

        // Trigger reconciliation
        r := reconciler.NewConfigMapReconciler(suite.Client, logger)
        err = r.Reconcile(suite.Context)
        Expect(err).ToNot(HaveOccurred())

        // Wait for ConfigMap update (resourceVersion changes)
        reconciledCM := suite.WaitForConfigMapUpdate("kubernaut-system",
            "kubernaut-toolset-config", initialCM.ResourceVersion, 30*time.Second)

        // Verify restored to desired state
        Expect(reconciledCM.Data["prometheus-toolset.yaml"]).To(Equal("enabled: true\nendpoint: http://prometheus:9090"))
    })
})
```

**Test 4: Override Preservation (45 min)**
```go
import (
    "context"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/testutil/kind"
    "github.com/jordigilh/kubernaut/pkg/toolset/reconciler"
)

Describe("Integration Test 4: Override Preservation", func() {
    It("should preserve overrides.yaml during reconciliation", func() {
        // Create ConfigMap with overrides.yaml
        overrideContent := "custom:\n  setting: value"
        cm, err := suite.DeployConfigMap(kind.ConfigMapConfig{
            Name:      "kubernaut-toolset-config",
            Namespace: "kubernaut-system",
            Data: map[string]string{
                "prometheus-toolset.yaml": "enabled: true",
                "overrides.yaml":          overrideContent,
            },
        })
        Expect(err).ToNot(HaveOccurred())

        // Trigger reconciliation
        r := reconciler.NewConfigMapReconciler(suite.Client, logger)
        err = r.Reconcile(suite.Context)
        Expect(err).ToNot(HaveOccurred())

        // Verify overrides.yaml untouched
        reconciledCM, err := suite.GetConfigMap("kubernaut-system", "kubernaut-toolset-config")
        Expect(err).ToNot(HaveOccurred())
        Expect(reconciledCM.Data["overrides.yaml"]).To(Equal(overrideContent))
    })
})
```

**Test 5: Multi-Detector Integration (30 min)**
```go
import (
    "context"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/testutil/kind"
    "github.com/jordigilh/kubernaut/pkg/toolset/discovery"
)

Describe("Integration Test 5: Multiple Services", func() {
    It("should discover multiple service types", func() {
        // Deploy Prometheus + Grafana services to Kind cluster
        promSvc, err := suite.DeployPrometheusService("toolset-test")
        Expect(err).ToNot(HaveOccurred())

        grafanaSvc, err := suite.DeployGrafanaService("toolset-test")
        Expect(err).ToNot(HaveOccurred())

        // Run discovery
        discoverer := discovery.NewServiceDiscoverer(suite.Client, logger)
        services, err := discoverer.DiscoverServices(suite.Context)
        Expect(err).ToNot(HaveOccurred())

        // Verify both in results
        Expect(services).To(HaveLen(2))

        serviceTypes := make(map[string]bool)
        for _, svc := range services {
            serviceTypes[svc.Type] = true
        }
        Expect(serviceTypes).To(HaveKey("prometheus"))
        Expect(serviceTypes).To(HaveKey("grafana"))
    })
})
```

**After Integration Tests** ⭐:
- [ ] Architecture validated
- [ ] Integration issues identified
- [ ] Ready for unit test details with confidence

#### Afternoon: Unit Tests Part 1 - Detectors (4h)

**Focus**: Fill in detector edge cases after integration validation

**Files**:
- `test/unit/toolset/prometheus_detector_test.go` (expand)
- `test/unit/toolset/grafana_detector_test.go` (expand)
- `test/unit/toolset/jaeger_detector_test.go` (expand)
- `test/unit/toolset/elasticsearch_detector_test.go` (expand)
- `test/unit/toolset/custom_detector_test.go` (expand)

**Edge Cases to Add**:
- Multiple services with same labels
- Services with missing ports
- Malformed service specs
- Health check timeouts
- Health check retries
- Invalid endpoint URLs

#### Metrics Validation Checkpoint (15 min) ⭐

```bash
# Start service
./bin/dynamictoolset &

# Validate metrics exposed
curl http://localhost:9090/metrics | grep dynamictoolset_

# Check for all expected metrics
grep -c "dynamictoolset_services_discovered_total" # Should be > 0
grep -c "dynamictoolset_toolset_healthy" # Should be > 0
...
```

**Validation**:
- [ ] Metrics endpoint responds
- [ ] All 10+ metrics present
- [ ] Metric labels correct
- [ ] Histogram buckets appropriate

**Deliverables (Day 8)** - ENHANCED:
- [ ] **⭐ 5 integration tests passing** (architecture validated)
- [ ] 20+ unit tests for detectors
- [ ] Edge cases covered
- [ ] **⭐ Metrics validated**
- [ ] Confidence: 85%+

---

### Day 9: Unit Tests Part 2 + BR Coverage Matrix (8h)

#### Morning: Unit Tests - Generators + Reconciler (4h)

**Files**:
- `test/unit/toolset/generator_test.go` (expand)
- `test/unit/toolset/reconciler_test.go` (expand)
- `test/unit/toolset/override_merger_test.go`

**Scenarios**:
- Generator edge cases (malformed configs, missing fields)
- Reconciler concurrent updates
- Override merge conflicts
- Cache invalidation

#### Afternoon: Unit Tests - Server + Handlers (4h)

**Files**:
- `test/unit/toolset/server_test.go`
- `test/unit/toolset/handlers_test.go`
- `test/unit/toolset/middleware_test.go`

**Scenarios**:
- API endpoint validation
- Query parameter parsing
- Error responses
- Middleware behavior

#### ⭐ EOD: Create BR Coverage Matrix

**File**: `implementation/testing/BR-COVERAGE-MATRIX.md`

```markdown
# Business Requirement Test Coverage Matrix

| BR | Requirement | Unit Tests | Integration Tests | E2E Tests | Coverage | Status |
|----|-------------|------------|-------------------|-----------|----------|--------|
| BR-TOOLSET-001 | Service discovery | 10 | 2 | 1 | 100% | ✅ |
| BR-TOOLSET-002 | Health validation | 8 | 1 | 0 | 100% | ✅ |
| BR-TOOLSET-003 | ConfigMap generation | 12 | 1 | 1 | 100% | ✅ |
| BR-TOOLSET-004 | ConfigMap reconciliation | 8 | 2 | 1 | 100% | ✅ |
| BR-TOOLSET-005 | Override preservation | 4 | 1 | 1 | 100% | ✅ |
| BR-TOOLSET-006 | REST API endpoints | 10 | 3 | 0 | 100% | ✅ |
| BR-TOOLSET-007 | Metrics exposure | 5 | 1 | 0 | 100% | ✅ |
| ... | ... | ... | ... | ... | ... | ... |

## Summary
- **Total BRs**: [N]
- **Covered**: [N] (100%)
- **Untested**: 0
- **Unit Test Total**: 50+
- **Integration Test Total**: 10+
- **E2E Test Total**: 3+

## Coverage Analysis
All business requirements have test coverage. No gaps identified.
```

**Deliverables (Day 9)**:
- [ ] 30+ additional unit tests
- [ ] Total unit tests: 50+
- [ ] Unit test coverage > 70%
- [ ] **⭐ BR coverage matrix complete**
- [ ] All BRs mapped to tests

---

### Day 10: Advanced Integration + E2E Tests (8h)

#### Advanced Integration Tests (4h)

**File**: `test/integration/toolset/advanced_integration_test.go`

**Scenarios**:
- Concurrent discoveries
- ConfigMap concurrent updates
- Service addition during discovery
- Service removal during discovery
- Network failures
- API server unavailability

#### E2E Test Setup (2h)

**File**: `test/e2e/toolset/suite_test.go`

- Kind cluster creation
- Deploy Prometheus to Kind
- Deploy Grafana to Kind
- Deploy Dynamic Toolset service
- Wait for readiness

#### E2E Test Execution (2h)

**File**: `test/e2e/toolset/end_to_end_test.go`

**Scenarios**:
1. Complete discovery workflow
2. ConfigMap volume mount validation
3. HolmesGPT API can read ConfigMap (mock)
4. Manual ConfigMap edit → reconciliation
5. ConfigMap deletion → recreation
6. Service label change → re-discovery

**Deliverables (Day 10)**:
- [ ] 7+ advanced integration tests
- [ ] 6+ E2E tests
- [ ] Integration coverage > 50%
- [ ] All tests passing

---

### Day 11: Documentation (8h)

#### Implementation Documentation (3h)

**Files to Create/Update**:
- `overview.md` (update with implementation details)
- `api-specification.md` (verify accuracy)
- `implementation.md` (implementation patterns)
- `testing-strategy.md` (actual coverage achieved)
- `configuration.md` (all configuration options)

#### Design Decision Documentation (2h) ⭐

**File**: `docs/architecture/DESIGN_DECISIONS.md` (add entries)

**DD-XXX Entries to Create**:
1. **DD-TOOLSET-001**: Start/Stop Interface Pattern (leader election future-proofing)
2. **DD-TOOLSET-002**: ConfigMap vs CRD (why ConfigMap + reconciliation)
3. **DD-TOOLSET-003**: File-Based vs API-Based HolmesGPT Integration
4. **DD-TOOLSET-004**: 5-Minute Discovery Interval (balance between freshness and load)

#### Operational Documentation (3h)

**Files**:
- `deployment.md` (deployment guide)
- `monitoring.md` (metrics and alerts)
- `troubleshooting.md` (common issues)
- `runbook.md` (operational procedures)

**Deliverables (Day 11)**:
- [ ] Complete service documentation
- [ ] 4 design decision entries
- [ ] Operational runbooks
- [ ] Troubleshooting guide

---

### Day 12: CHECK Phase + Production Readiness ⭐ (8h)

**ENHANCED with 6 new critical deliverables**

#### CHECK Phase Validation (2h)

**Checklist**:
- [ ] All business requirements met (verify BR matrix)
- [ ] Build passes without errors
- [ ] All tests passing (unit + integration + E2E)
- [ ] Unit coverage > 70%
- [ ] Integration coverage > 50%
- [ ] Metrics exposed and validated
- [ ] Health checks functional
- [ ] Authentication working (if implemented)
- [ ] Documentation complete
- [ ] No lint errors
- [ ] Code follows Go standards

**Run Full Test Suite**:
```bash
make test-unit-toolset
make test-integration-toolset
make test-e2e-toolset
make test-coverage-toolset
make lint-toolset
```

#### 1. Production Readiness Checklist (1h) ⭐

**File**: `implementation/PRODUCTION_READINESS_REPORT.md`

```markdown
## Production Readiness Assessment

### Functional Validation
- [ ] All 5 service detectors tested
- [ ] ConfigMap reconciliation tested with concurrent updates
- [ ] Health checks validated (service unavailability)
- [ ] Override preservation working
- [ ] Discovery loop tested (Start/Stop)
- [ ] Reconciliation loop tested (30s interval)

### Operational Validation
- [ ] Metrics comprehensive (10+ metrics)
- [ ] Logging structured (correlation IDs)
- [ ] Health checks reliable (K8s probes)
- [ ] Readiness checks accurate
- [ ] Graceful shutdown tested
- [ ] Resource limits appropriate

### Performance Validation
- [ ] Discovery cycle < 30s (for 10 services)
- [ ] Health check < 5s per service
- [ ] API p95 < 200ms
- [ ] Memory < 128MB per replica
- [ ] CPU < 0.1 cores average

### Deployment Validation
- [ ] Deployment manifests complete
- [ ] ServiceAccount defined
- [ ] RBAC minimal (Services read, ConfigMaps read/write)
- [ ] ConfigMap ownership set
- [ ] Resource requests/limits set
- [ ] Liveness/readiness probes configured
- [ ] Metrics endpoint exposed

### Security Validation
- [ ] RBAC follows least privilege
- [ ] No hardcoded secrets
- [ ] TLS for external communication (if needed)
- [ ] ServiceAccount token auto-mounted

### Confidence Assessment: X%
```

#### 2. File Organization Strategy (30 min) ⭐

**File**: `implementation/FILE_ORGANIZATION_PLAN.md`

```markdown
## File Organization for Git Commits

### Production Implementation (pkg/, cmd/, internal/)
**Total**: 35 files

1. **Core Types & Interfaces** (5 files)
   - pkg/toolset/types.go
   - pkg/toolset/discovery/detector.go
   - pkg/toolset/discovery/discoverer.go
   - pkg/toolset/errors.go
   - internal/toolset/k8s/client.go

2. **Detectors** (5 files)
   - pkg/toolset/discovery/prometheus_detector.go
   - pkg/toolset/discovery/grafana_detector.go
   - pkg/toolset/discovery/jaeger_detector.go
   - pkg/toolset/discovery/elasticsearch_detector.go
   - pkg/toolset/discovery/custom_detector.go

... [continue for all files]

### Git Commit Strategy
```bash
# Commit 1: Foundation
git add pkg/toolset/types.go pkg/toolset/discovery/{detector,discoverer}.go
git commit -m "feat(toolset): Add foundation types and interfaces"

# Commit 2: Detectors
git add pkg/toolset/discovery/*_detector.go pkg/toolset/health/
git commit -m "feat(toolset): Implement service detectors"

... [continue]
```
```

#### 3. Performance Benchmarking (1h) ⭐

**File**: `implementation/PERFORMANCE_REPORT.md`

```bash
# Run benchmarks
go test -bench=. -benchmem ./pkg/toolset/... > performance_report.txt
```

**Report Structure**:
```markdown
## Performance Benchmark Results

### Discovery Performance
- Service detection: Xμs per service
- Health check: Yms per service
- Complete discovery (10 services): Zs

### ConfigMap Generation
- YAML generation: Xμs per toolset
- Complete ConfigMap build: Yms

### API Performance
- GET /api/v1/toolsets: Xms (p95)
- GET /api/v1/services: Yms (p95)
- POST /api/v1/discover: Zms (p95)

### Resource Usage
- Memory baseline: XMB
- Memory under load: YMB
- CPU baseline: X%
- CPU under load: Y%

### Targets vs Actuals
| Metric | Target | Actual | Status |
|--------|--------|--------|--------|
| Discovery < 30s | 30s | 22s | ✅ |
| API p95 < 200ms | 200ms | 145ms | ✅ |
| Memory < 128MB | 128MB | 95MB | ✅ |
| CPU < 0.1 cores | 0.1 | 0.07 | ✅ |
```

#### 4. Troubleshooting Guide (1h) ⭐

**File**: `implementation/TROUBLESHOOTING_GUIDE.md`

```markdown
## Troubleshooting Guide

### Issue 1: Services Not Discovered
**Symptoms**: Empty ConfigMap or missing service types
**Diagnosis**:
1. Check service labels: `kubectl get svc [service] -o yaml | grep labels`
2. Check discovery logs: `kubectl logs -n kubernaut-system -l app=dynamictoolset`
3. Verify RBAC: `kubectl auth can-i list services --as=system:serviceaccount:kubernaut-system:dynamictoolset`

**Resolution**:
- Add correct labels to service
- Fix RBAC permissions
- Check service is in monitored namespaces

### Issue 2: ConfigMap Drift Not Reconciled
**Symptoms**: Manual edits persist, reconciliation not working
**Diagnosis**:
1. Check reconciler logs
2. Verify ConfigMap ownership: `kubectl get cm kubernaut-toolset-config -o yaml | grep ownerReferences`
3. Check reconciliation interval

**Resolution**:
- Restart service if reconciler crashed
- Add owner reference manually
- Use overrides.yaml for permanent changes

... [continue for all common issues]

### Issue 10: Metrics Not Exposed
### Issue 11: Health Checks Failing
### Issue 12: High Memory Usage
```

#### 5. Confidence Assessment (30 min) ⭐

**File**: `implementation/CONFIDENCE_ASSESSMENT.md`

```markdown
## Implementation Confidence Assessment

### Implementation Accuracy: 95%
**Evidence**:
- 100% spec compliance validated
- All components implemented per design
- Code review found zero major issues
- Architecture decisions documented

### Test Coverage
**Unit Tests**: 75% (target: 70%+) ✅
- 52 tests written
- All components covered
- Edge cases tested

**Integration Tests**: 55% (target: 50%+) ✅
- 12 tests written
- Real Kubernetes API tested
- ConfigMap operations validated

**E2E Tests**: 8% (target: <10%) ✅
- 6 tests written
- Complete workflows validated
- Kind cluster testing successful

### Business Requirement Coverage: 100% ✅
**Mapped BRs**: 25
**Untested BRs**: 0
**Justified Skips**: 0

### Production Readiness: 98% ✅
**Complete**:
- ✅ Deployment manifests
- ✅ Observability (metrics, logs)
- ✅ Documentation
- ✅ Performance validated
- ✅ Security validated

**Pending**:
- ⏳ Final load testing in staging

### Risks and Mitigations

**Risk 1**: Discovery failures on large clusters (>1000 services)
- **Likelihood**: Low
- **Impact**: Medium
- **Mitigation**: Pagination support planned for v1.1

**Risk 2**: ConfigMap size limits (>1MB)
- **Likelihood**: Very Low
- **Impact**: High
- **Mitigation**: Split ConfigMap if >50 services discovered

**Overall Confidence**: 95%
**Ready for Production**: Yes ✅
```

#### 6. Handoff Summary (Final Deliverable) ⭐

**File**: `implementation/00-HANDOFF-SUMMARY.md`

```markdown
# Dynamic Toolset Service - Implementation Handoff

**Date**: [Date]
**Status**: ✅ Production Ready (95% confidence)

## What Was Accomplished

### Complete Implementation (Days 1-7)
- 35 Go files created (pkg/, cmd/, internal/)
- All 5 service detectors implemented
- ConfigMap generation and reconciliation
- HTTP server with REST API
- Metrics and observability
- Build successful, zero lint errors

### Comprehensive Testing (Days 8-10)
- 52 unit tests (75% coverage)
- 12 integration tests (55% coverage)
- 6 E2E tests (8% coverage)
- All 25 business requirements tested
- BR coverage matrix: 100%

### Complete Documentation (Days 11-12)
- Service overview and API specs
- 4 design decision entries
- Operational runbooks
- Troubleshooting guide
- Performance report
- Production readiness report

## Current State

| Component | Status | Confidence |
|-----------|--------|------------|
| Implementation | ✅ Complete | 95% |
| Testing | ✅ Complete | 95% |
| Documentation | ✅ Complete | 95% |
| Production Readiness | ✅ Ready | 98% |

## Next Steps

### Immediate (Day 13)
1. Final staging deployment test
2. Load testing (1000+ services)
3. Security review
4. Documentation final review

### Week 2
1. Production deployment to pilot cluster
2. Monitor metrics for 48 hours
3. Gather feedback
4. Bug fixes if needed

## Key Files

### Implementation
- cmd/dynamictoolset/main.go
- pkg/toolset/*.go (35 files)

### Tests
- test/unit/toolset/*.go (52 tests)
- test/integration/toolset/*.go (12 tests)
- test/e2e/toolset/*.go (6 tests)

### Documentation
- docs/services/stateless/dynamic-toolset/**

### Deployment
- deploy/dynamictoolset/*.yaml

## Key Decisions

1. **DD-TOOLSET-001**: Start/Stop interfaces for future leader election
2. **DD-TOOLSET-002**: ConfigMap over CRD for simplicity
3. **DD-TOOLSET-003**: File-based polling by HolmesGPT
4. **DD-TOOLSET-004**: 5-minute discovery interval

## Lessons Learned

1. **Integration-first testing saved 2 days**: Found issues early
2. **Schema validation checkpoint prevented test failures**: 100% alignment
3. **Daily status docs improved communication**: Clear progress tracking
4. **BR coverage matrix ensured completeness**: No missed requirements
5. **Production readiness checklist reduced deployment risk**: Comprehensive validation

## Troubleshooting

See `implementation/TROUBLESHOOTING_GUIDE.md` for:
- Services not discovered
- ConfigMap drift issues
- Health check failures
- Performance problems
- Metrics issues

## Confidence Rating: 95%

**Why 95% and not 100%**:
- Final load testing pending (1000+ services)
- Production validation pending
- User feedback pending

**Production Deployment**: ✅ APPROVED
```

**Deliverables (Day 12)** - FINAL:
- [ ] CHECK phase validation complete
- [ ] **⭐ Production readiness report**
- [ ] **⭐ File organization plan**
- [ ] **⭐ Performance benchmark report**
- [ ] **⭐ Troubleshooting guide**
- [ ] **⭐ Confidence assessment**
- [ ] **⭐ Handoff summary**
- [ ] Service ready for production deployment

---

### Day 13: Buffer Day (8h) - NEW

**Purpose**: Handle unforeseen issues, polish, final validation

**Potential Activities**:
- Fix any issues from Day 12 validation
- Additional edge case testing
- Documentation polish
- Stakeholder demo preparation
- Final code review
- Security scan
- Deployment dry-run

**Why Added**: Gateway implementation taught us to build in buffer time

---

## Success Metrics Summary

### Implementation
- ✅ 35+ Go files
- ✅ Zero lint errors
- ✅ Build successful
- ✅ All components integrated

### Testing
- ✅ Unit coverage > 70% (target: 75%)
- ✅ Integration coverage > 50% (target: 55%)
- ✅ E2E coverage < 10% (target: 8%)
- ✅ BR coverage: 100%
- ✅ All tests passing

### Documentation
- ✅ 15+ documentation files
- ✅ 4 design decisions
- ✅ 4 status reports
- ✅ Complete operational guides

### Production Readiness
- ✅ Deployment manifests complete
- ✅ Metrics comprehensive (10+)
- ✅ Performance validated
- ✅ Security validated
- ✅ Troubleshooting guide complete

### Confidence
- ✅ Implementation: 95%
- ✅ Testing: 95%
- ✅ Documentation: 95%
- ✅ **Overall: 95%**

---

## Comparison: Original vs Enhanced Plan

| Aspect | Original Plan | Enhanced Plan | Improvement |
|--------|---------------|---------------|-------------|
| Timeline | 11-12 days | 12-13 days | +1 day (buffer) |
| Testing Strategy | Unit → Integration | **Integration → Unit** | 2 days savings |
| Documentation | 1 status doc | **4 status docs** | Better tracking |
| Schema Validation | Not planned | **Day 7 checkpoint** | Prevents failures |
| BR Coverage | Not tracked | **Matrix on Day 9** | 100% coverage |
| Production Readiness | Basic checklist | **6 deliverables** | Deployment confidence |
| Test Infrastructure | Day 10 | **Day 7 EOD** | No Day 8 blockers |
| Error Handling | Basic | **Philosophy doc** | Resilient design |

**Net Effect**: +1 day timeline, but saves 2-3 days in debugging = **1-2 days faster overall**

---

## Critical Success Factors

### ✅ Must Have
1. Integration-first testing (Day 8)
2. Schema validation (Day 7 EOD)
3. Daily status docs (Days 1, 4, 7, 12)
4. BR coverage matrix (Day 9)
5. Production readiness (Day 12)

### ✅ Should Have
6. Test infrastructure pre-setup (Day 7 EOD)
7. Error handling philosophy (Day 6)
8. File organization (Day 12)
9. Performance benchmarking (Day 12)
10. Troubleshooting guide (Day 12)

### 💡 Nice to Have
11. Design decision docs (ongoing)
12. Testing rationale doc (Day 7)
13. Metrics validation checkpoint (Day 8)
14. Confidence assessment (Day 12)
15. Buffer day (Day 13)

---

## Related Documents

- [SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md](../../../SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md) - Reusable template
- [PLAN_TRIAGE_VS_GATEWAY.md](./PLAN_TRIAGE_VS_GATEWAY.md) - Gap analysis
- [IMPLEMENTATION_CHECKLIST.md](./IMPLEMENTATION_CHECKLIST.md) - Progress tracking
- [Gateway Implementation](../../gateway-service/implementation/) - Reference

---

**Plan Status**: ✅ Enhanced and Ready
**Based On**: Gateway success + identified improvements
**Expected Success Rate**: 95%+ (based on Gateway achieving 21/22 tests passing)
**Estimated Time Savings**: 1-2 days (vs ad-hoc planning)


# BR-TOOLSET-044: ToolsetConfig Configuration CRD

**Business Requirement ID**: BR-TOOLSET-044
**Category**: Dynamic Toolset Service
**Priority**: P0
**Target Version**: V1.1
**Status**: 📋 **PLANNED FOR V1.1**
**Date**: November 10, 2025

---

## 📋 **Business Need**

### **Problem Statement**

DD-TOOLSET-001 deprecated 6 REST API endpoints (0-10% business value) in V1, keeping only health/metrics endpoints. The Dynamic Toolset Service needs a Kubernetes-native configuration mechanism to replace the deprecated REST API functionality and provide operators with declarative control over service discovery behavior.

**Current Limitations** (V1):
- ❌ No way to configure discovery interval (hardcoded to 5 minutes)
- ❌ No way to filter which namespaces to discover services in
- ❌ No way to enable/disable specific service types (Prometheus, Grafana, etc.)
- ❌ No per-service health status visibility
- ❌ REST API endpoints disabled (ConfigMap introspection only)

**Impact**:
- Operators cannot tune discovery behavior for their environment
- Cannot reduce discovery scope to improve performance
- No visibility into individual service health status
- Cannot disable discovery for unused service types

---

## 🎯 **Business Objective**

**Provide a Kubernetes-native `ToolsetConfig` CRD that allows operators to declaratively configure service discovery behavior, with per-service health status tracking and bounded status growth.**

### **Success Criteria**
1. ✅ ToolsetConfig CRD deployed in `kubernaut-system` namespace
2. ✅ Discovery interval configurable (1m to 1h)
3. ✅ Namespace filtering works correctly
4. ✅ Service type filters work correctly (enable/disable per type)
5. ✅ Per-service status tracked with bounded growth (one entry per service)
6. ✅ ConfigMap generated from CRD configuration
7. ✅ 90%+ unit test coverage, 85%+ integration, 80%+ E2E

---

## 📊 **Use Cases**

### **Use Case 1: Configure Discovery Interval**

**Scenario**: Operator wants to reduce Kubernetes API load by increasing discovery interval from 5m to 10m.

**Current Flow** (V1 - Without BR-TOOLSET-044):
```
1. Discovery interval hardcoded to 5 minutes
2. ❌ No way to change interval without code modification
3. ❌ Kubernetes API load higher than necessary
4. ❌ Operator cannot tune for their environment
```

**Desired Flow with BR-TOOLSET-044**:
```
1. Operator updates ToolsetConfig CRD:
   kubectl patch toolsetconfig kubernaut-toolset-config -n kubernaut-system \
     --type merge -p '{"spec":{"discoveryInterval":"10m"}}'

2. Controller detects spec change
3. Controller updates discovery loop interval
4. ✅ Discovery runs every 10 minutes (50% reduction in API calls)
5. ✅ Operator tunes discovery for their environment
```

---

### **Use Case 2: Filter Discovery to Specific Namespaces**

**Scenario**: Operator only wants to discover services in `monitoring` and `observability` namespaces.

**Current Flow** (V1):
```
1. Discovery scans all namespaces (wildcard)
2. ❌ Discovers services in namespaces that don't need toolset integration
3. ❌ Unnecessary ConfigMap updates
4. ❌ Higher RBAC permissions required (cluster-wide service access)
```

**Desired Flow with BR-TOOLSET-044**:
```
1. Operator updates ToolsetConfig:
   spec:
     namespaces:
       - monitoring
       - observability

2. Controller discovers services only in specified namespaces
3. ✅ Reduced discovery scope (faster, less API load)
4. ✅ Narrower RBAC permissions required
5. ✅ ConfigMap contains only relevant services
```

---

### **Use Case 3: Disable Unused Service Types**

**Scenario**: Operator doesn't use Jaeger in their cluster, wants to disable Jaeger discovery.

**Current Flow** (V1):
```
1. All service types enabled by default
2. ❌ Jaeger detector runs on every discovery (wasted cycles)
3. ❌ No way to disable without code modification
```

**Desired Flow with BR-TOOLSET-044**:
```
1. Operator updates ToolsetConfig:
   spec:
     serviceTypes:
       jaeger:
         enabled: false

2. Controller skips Jaeger detector on discovery
3. ✅ Faster discovery (one less detector to run)
4. ✅ Operator customizes for their environment
```

---

### **Use Case 4: Monitor Per-Service Health Status**

**Scenario**: Operator wants to see which discovered services are healthy vs unhealthy.

**Current Flow** (V1):
```
1. Services discovered and added to ConfigMap
2. ❌ No visibility into individual service health
3. ❌ Operator must manually check each service endpoint
4. ❌ No way to see when service last checked
```

**Desired Flow with BR-TOOLSET-044**:
```
1. Operator queries ToolsetConfig status:
   kubectl get toolsetconfig kubernaut-toolset-config -n kubernaut-system -o yaml

2. Status shows per-service health:
   status:
     discoveredServices:
       - name: prometheus-server
         namespace: monitoring
         healthy: true
         condition: "Ready"
         reason: "HealthCheckPassed"
         lastChecked: "2025-11-10T10:30:00Z"

       - name: grafana
         namespace: monitoring
         healthy: false
         condition: "NotReady"
         reason: "HealthCheckFailed"
         message: "HTTP 503: Service Unavailable"
         lastChecked: "2025-11-10T10:30:00Z"

3. ✅ Operator sees unhealthy service (grafana)
4. ✅ Operator investigates and fixes grafana
5. ✅ Next discovery updates status to healthy
```

---

## 🔧 **Functional Requirements**

### FR-044.1: Configuration CRD Definition

**Description**: The service SHALL provide a `ToolsetConfig` CRD for configuring discovery behavior

**Acceptance Criteria**:
- **AC-044.1.1**: CRD SHALL be defined in API group `toolset.kubernaut.io/v1alpha1`
- **AC-044.1.2**: CRD SHALL be namespaced (deployed in `kubernaut-system`)
- **AC-044.1.3**: CRD SHALL be a singleton (one instance per namespace)
- **AC-044.1.4**: CRD SHALL support OpenAPI v3 schema validation
- **AC-044.1.5**: CRD SHALL follow Kubernetes CRD best practices (status subresource, printer columns)

**CRD Type**: **Configuration CRD** (singleton, like `KubeProxyConfiguration`)

**Distinction**:
- **Workflow CRDs** (SignalProcessing, AIAnalysis): Multiple instances, lifecycle-based
- **Configuration CRD** (ToolsetConfig): Singleton, configuration-based

---

### FR-044.2: Discovery Interval Configuration

**Description**: The CRD SHALL allow operators to configure the discovery interval

**Acceptance Criteria**:
- **AC-044.2.1**: `spec.discoveryInterval` field SHALL accept duration strings (e.g., `5m`, `10m`, `1h`)
- **AC-044.2.2**: Default discovery interval SHALL be `5m` (5 minutes)
- **AC-044.2.3**: Minimum discovery interval SHALL be `1m` (validated by CRD schema)
- **AC-044.2.4**: Controller SHALL respect the configured interval between discovery runs
- **AC-044.2.5**: Changing the interval SHALL take effect on the next reconciliation

**Example**:
```yaml
spec:
  discoveryInterval: 10m  # Discover every 10 minutes
```

**Business Value**: Allows operators to balance discovery freshness vs. Kubernetes API load

---

### FR-044.3: Namespace Filtering

**Description**: The CRD SHALL allow operators to filter which namespaces to discover services in

**Acceptance Criteria**:
- **AC-044.3.1**: `spec.namespaces` field SHALL accept a list of namespace names
- **AC-044.3.2**: Wildcard `"*"` SHALL discover services in all namespaces (default)
- **AC-044.3.3**: Empty list SHALL be treated as wildcard (all namespaces)
- **AC-044.3.4**: Controller SHALL only discover services in specified namespaces
- **AC-044.3.5**: Invalid namespace names SHALL be rejected by CRD validation

**Example**:
```yaml
spec:
  namespaces:
    - monitoring
    - observability
    - "*"  # All namespaces
```

**Business Value**: Reduces discovery scope, improves performance, and limits RBAC requirements

---

### FR-044.4: Service Type Filters

**Description**: The CRD SHALL allow operators to enable/disable specific service types and customize detection criteria

**Acceptance Criteria**:
- **AC-044.4.1**: `spec.serviceTypes` field SHALL support Prometheus, Grafana, Jaeger, Elasticsearch, and custom services
- **AC-044.4.2**: Each service type SHALL have an `enabled` boolean field (default: `true`)
- **AC-044.4.3**: Each service type SHALL support custom labels and annotations for detection
- **AC-044.4.4**: Disabled service types SHALL NOT be discovered
- **AC-044.4.5**: Service type configuration SHALL be validated by CRD schema

**Example**:
```yaml
spec:
  serviceTypes:
    prometheus:
      enabled: true
      labels:
        - app=prometheus
        - prometheus.io/scrape=true
    grafana:
      enabled: true
      labels:
        - app=grafana
    jaeger:
      enabled: false  # Disable Jaeger discovery
      annotations:
        - jaeger.io/enabled=true
    elasticsearch:
      enabled: true
      labels:
        - app=elasticsearch
    custom:
      enabled: true
      annotations:
        - kubernaut.io/toolset=true
```

**Business Value**: Allows operators to customize discovery behavior per service type

---

### FR-044.5: Health Check Configuration

**Description**: The CRD SHALL allow operators to configure health check behavior

**Acceptance Criteria**:
- **AC-044.5.1**: `spec.healthCheck.enabled` field SHALL enable/disable health checks (default: `true`)
- **AC-044.5.2**: `spec.healthCheck.timeout` field SHALL configure health check timeout (default: `5s`)
- **AC-044.5.3**: Minimum timeout SHALL be `1s` (validated by CRD schema)
- **AC-044.5.4**: Controller SHALL respect health check configuration
- **AC-044.5.5**: Disabled health checks SHALL mark all services as healthy

**Example**:
```yaml
spec:
  healthCheck:
    enabled: true
    timeout: 10s  # 10-second timeout for health checks
```

**Business Value**: Allows operators to tune health check behavior for slow/unreliable networks

---

### FR-044.6: ConfigMap Generation Settings

**Description**: The CRD SHALL allow operators to configure ConfigMap generation behavior

**Acceptance Criteria**:
- **AC-044.6.1**: `spec.configMap.name` field SHALL specify ConfigMap name (default: `kubernaut-toolset-config`)
- **AC-044.6.2**: `spec.configMap.namespace` field SHALL specify ConfigMap namespace (default: `kubernaut-system`)
- **AC-044.6.3**: `spec.configMap.preserveOverrides` field SHALL enable/disable override preservation (default: `true`)
- **AC-044.6.4**: Controller SHALL create/update ConfigMap with specified name and namespace
- **AC-044.6.5**: ConfigMap namespace SHALL be validated (must exist)

**Example**:
```yaml
spec:
  configMap:
    name: kubernaut-toolset-config
    namespace: kubernaut-system
    preserveOverrides: true
```

**Business Value**: Allows operators to customize ConfigMap location and override behavior

---

### FR-044.7: Per-Service Status Tracking

**Description**: The CRD status SHALL track the health and state of each discovered service

**Acceptance Criteria**:
- **AC-044.7.1**: `status.discoveredServices` field SHALL contain one entry per discovered service
- **AC-044.7.2**: Each entry SHALL include: name, namespace, type, endpoint, healthy, lastChecked
- **AC-044.7.3**: Each entry SHALL include standard Kubernetes condition fields: condition, reason, message
- **AC-044.7.4**: Status SHALL be updated **in place** (no new entries for condition changes)
- **AC-044.7.5**: Status size SHALL be bounded by the number of discovered services (not discovery runs)

**Status Structure**:
```yaml
status:
  discoveredServices:
    - name: prometheus-server
      namespace: monitoring
      type: prometheus
      endpoint: http://prometheus-server.monitoring.svc.cluster.local:9090
      healthy: true
      lastChecked: "2025-11-10T10:30:00Z"
      condition: "Ready"           # Ready, NotReady, Unknown
      reason: "HealthCheckPassed"  # HealthCheckPassed, HealthCheckFailed, EndpointUnreachable
      message: "Service is healthy and reachable"
```

**Business Value**: Provides per-service observability without unbounded status growth

---

### FR-044.8: Bounded Status Growth

**Description**: The CRD status SHALL prevent unbounded growth by updating entries in place

**Acceptance Criteria**:
- **AC-044.8.1**: Status SHALL contain **one entry per discovered service** (not per discovery run)
- **AC-044.8.2**: Existing service entries SHALL be updated in place when condition/reason/message changes
- **AC-044.8.3**: New service entries SHALL only be added when a new service is discovered
- **AC-044.8.4**: Deleted service entries SHALL be removed when a service is no longer discovered
- **AC-044.8.5**: Status size SHALL NOT grow unbounded (bounded by service count)

**Controller Logic**:
```go
// Find existing entry by name+namespace
for i, existing := range config.Status.DiscoveredServices {
    if existing.Name == service.Name && existing.Namespace == service.Namespace {
        // UPDATE IN PLACE (no new entry)
        config.Status.DiscoveredServices[i].Condition = service.Condition
        config.Status.DiscoveredServices[i].Reason = service.Reason
        config.Status.DiscoveredServices[i].Message = service.Message
        config.Status.DiscoveredServices[i].LastChecked = metav1.Now()
        return
    }
}

// New service discovered, ADD entry
config.Status.DiscoveredServices = append(config.Status.DiscoveredServices, service)
```

**Business Value**: Prevents etcd bloat and ensures CRD status remains lightweight

---

### FR-044.9: Discovery Run Metadata

**Description**: The CRD status SHALL track metadata about discovery runs

**Acceptance Criteria**:
- **AC-044.9.1**: `status.lastDiscoveryTime` field SHALL track when the last discovery completed
- **AC-044.9.2**: `status.nextDiscoveryTime` field SHALL track when the next discovery is scheduled
- **AC-044.9.3**: `status.discoveryCount` field SHALL track the total number of discovery runs
- **AC-044.9.4**: `status.phase` field SHALL track overall discovery status (Pending, Running, Completed, Failed)
- **AC-044.9.5**: `status.servicesDiscovered` field SHALL track the total number of discovered services

**Example**:
```yaml
status:
  lastDiscoveryTime: "2025-11-10T10:30:00Z"
  nextDiscoveryTime: "2025-11-10T10:35:00Z"
  discoveryCount: 42
  phase: "Completed"
  servicesDiscovered: 10
  servicesHealthy: 8
  servicesUnhealthy: 2
```

**Business Value**: Provides high-level observability of discovery operations

---

### FR-044.10: Error Tracking

**Description**: The CRD status SHALL track errors that occur during discovery

**Acceptance Criteria**:
- **AC-044.10.1**: `status.errors` field SHALL contain a list of error messages
- **AC-044.10.2**: Errors SHALL be cleared on successful discovery runs
- **AC-044.10.3**: Errors SHALL include timestamps and error details
- **AC-044.10.4**: Errors SHALL be logged with structured logging
- **AC-044.10.5**: Prometheus metrics SHALL track error counts by type

**Example**:
```yaml
status:
  phase: "Failed"
  errors:
    - "Failed to connect to Kubernetes API: connection refused"
    - "Health check timeout for service grafana in namespace monitoring"
```

**Business Value**: Provides error visibility without relying on logs alone

---

### FR-044.11: Controller Reconciliation

**Description**: The controller SHALL reconcile the `ToolsetConfig` CRD and perform discovery based on configuration

**Acceptance Criteria**:
- **AC-044.11.1**: Controller SHALL watch `ToolsetConfig` CRDs in `kubernaut-system` namespace
- **AC-044.11.2**: Controller SHALL run discovery at the configured interval
- **AC-044.11.3**: Controller SHALL update CRD status with discovery results
- **AC-044.11.4**: Controller SHALL generate ConfigMap from discovered services
- **AC-044.11.5**: Controller SHALL handle CRD updates (spec changes) gracefully

**Reconciliation Logic**:
```go
func (r *ToolsetConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Get ToolsetConfig CRD
    config := &ToolsetConfig{}
    if err := r.Get(ctx, req.NamespacedName, config); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Check if discovery should run
    if !shouldRunDiscovery(config) {
        return ctrl.Result{RequeueAfter: config.Spec.DiscoveryInterval}, nil
    }

    // Run service discovery
    services, err := r.discoverServices(ctx, config)
    if err != nil {
        r.updateStatus(config, "Failed", err)
        return ctrl.Result{}, err
    }

    // Update status with discovered services (IN PLACE)
    for _, service := range services {
        r.updateServiceStatus(config, service)
    }

    // Generate ConfigMap
    if err := r.generateConfigMap(ctx, config, services); err != nil {
        return ctrl.Result{}, err
    }

    // Update overall status
    config.Status.Phase = "Completed"
    config.Status.LastDiscoveryTime = metav1.Now()
    config.Status.NextDiscoveryTime = metav1.NewTime(time.Now().Add(config.Spec.DiscoveryInterval))

    if err := r.Status().Update(ctx, config); err != nil {
        return ctrl.Result{}, err
    }

    // Requeue at next discovery interval
    return ctrl.Result{RequeueAfter: config.Spec.DiscoveryInterval}, nil
}
```

**Business Value**: Provides Kubernetes-native configuration and reconciliation

---

## 📊 Test Coverage Requirements

### Unit Tests (Target: 90% coverage)

**Test Files**:
- `test/unit/toolset/toolsetconfig_controller_test.go` - Controller reconciliation logic
- `test/unit/toolset/toolsetconfig_validation_test.go` - CRD validation
- `test/unit/toolset/toolsetconfig_status_test.go` - Status update logic (in-place updates)

**Test Scenarios** (30+ scenarios):
1. **Discovery Interval**:
   - Valid interval (5m, 10m, 1h)
   - Invalid interval (0s, negative)
   - Minimum interval validation (1m)
   - Interval change takes effect on next reconciliation

2. **Namespace Filtering**:
   - Wildcard (`"*"`) discovers all namespaces
   - Specific namespaces filter discovery
   - Empty list treated as wildcard
   - Invalid namespace names rejected

3. **Service Type Filters**:
   - Enabled service types discovered
   - Disabled service types not discovered
   - Custom labels and annotations respected
   - Invalid service type configuration rejected

4. **Health Check Configuration**:
   - Enabled health checks validate services
   - Disabled health checks mark all services as healthy
   - Timeout configuration respected
   - Invalid timeout rejected

5. **Per-Service Status**:
   - One entry per discovered service
   - Existing entries updated in place
   - New services add new entries
   - Deleted services remove entries
   - Status size bounded by service count

6. **Discovery Run Metadata**:
   - Last discovery time tracked
   - Next discovery time calculated
   - Discovery count incremented
   - Phase transitions (Pending → Running → Completed/Failed)

7. **Error Tracking**:
   - Errors tracked in status
   - Errors cleared on successful runs
   - Errors logged with structured logging
   - Prometheus metrics track error counts

---

### Integration Tests (Target: 85% coverage)

**Test Files**:
- `test/integration/toolset/toolsetconfig_reconciliation_test.go` - Full reconciliation flow
- `test/integration/toolset/toolsetconfig_configmap_test.go` - ConfigMap generation from CRD

**Test Scenarios** (15+ scenarios):
1. **Full Reconciliation Flow**:
   - CRD creation triggers discovery
   - Discovery updates CRD status
   - ConfigMap generated from discovered services
   - Status reflects discovery results

2. **Configuration Changes**:
   - Interval change triggers new discovery schedule
   - Namespace filter change affects discovery scope
   - Service type filter change affects discovered services
   - Health check configuration change affects validation

3. **Status Updates**:
   - Per-service status updated in place
   - Status size remains bounded
   - Discovery metadata tracked correctly
   - Errors tracked and cleared

4. **ConfigMap Integration**:
   - ConfigMap created with correct name and namespace
   - ConfigMap updated when services change
   - Manual overrides preserved
   - ConfigMap reflects CRD configuration

---

### E2E Tests (Target: 80% coverage)

**Test Files**:
- `test/e2e/toolset/04_toolsetconfig_lifecycle_test.go` - CRD lifecycle in Kind cluster

**Test Scenarios** (10+ scenarios):
1. **CRD Lifecycle**:
   - Create ToolsetConfig CRD
   - Controller discovers services
   - Status updated with discovered services
   - ConfigMap generated

2. **Configuration Changes**:
   - Update discovery interval
   - Update namespace filters
   - Disable service type
   - Verify changes take effect

3. **Service Changes**:
   - Add new service
   - Remove existing service
   - Update service health
   - Verify status updated in place

4. **Error Scenarios**:
   - Invalid CRD configuration rejected
   - Discovery errors tracked in status
   - Recovery from errors

---

## 🔗 Related Documentation

### Business Requirements
- **BR-TOOLSET-021**: Automatic Service Discovery (discovery logic reused)
- **BR-TOOLSET-026**: Discovery Loop Lifecycle Management (interval logic reused)
- **BR-TOOLSET-031**: ConfigMap Creation and Reconciliation (ConfigMap generation reused)

### Design Decisions
- **DD-TOOLSET-001**: REST API Deprecation and Configuration CRD Migration
- **DD-007**: Kubernetes-Aware Graceful Shutdown Pattern (controller shutdown)

### Architecture Decisions
- **ADR-036**: Authentication and Authorization Strategy (RBAC for CRD access)

---

## 🚀 **Implementation Phases**

### **Phase 1: CRD Definition** (Day 1 - 4 hours)
- Define `ToolsetConfig` CRD schema with OpenAPI v3 validation
- Generate CRD manifests with `kubebuilder`
- Add CRD validation (discovery interval > 0, valid namespaces)
- Add status subresource and printer columns
- Add CRD documentation

### **Phase 2: Controller Implementation** (Day 2-3 - 8 hours)
- Implement `ToolsetConfigReconciler` controller
- Discovery loop with configurable interval
- Per-service status updates (in-place)
- ConfigMap generation from discovered services
- Error tracking and logging

### **Phase 3: Testing** (Day 4 - 4 hours)
- Unit tests for controller logic (30+ scenarios)
- Integration tests for CRD reconciliation (15+ scenarios)
- E2E tests for discovery workflows (10+ scenarios)

### **Phase 4: Documentation** (Day 5 - 4 hours)
- Update BUSINESS_REQUIREMENTS.md with BR-TOOLSET-044
- Add CRD examples to documentation
- Migration guide from REST API to CRD
- Update architecture diagrams

**Total Estimated Effort**: 20 hours (3-5 days)

---

## 📊 **Success Metrics**

### **Configuration Adoption**
- **Target**: 100% of Dynamic Toolset deployments use ToolsetConfig CRD
- **Measure**: Track CRD creation in production clusters

### **Discovery Performance**
- **Target**: Discovery interval configurable from 1m to 1h
- **Measure**: Verify controller respects configured interval

### **Status Accuracy**
- **Target**: Per-service health status 95%+ accurate
- **Measure**: Compare status with manual health checks

---

## 🔄 **Alternatives Considered**

### **Alternative 1: Keep REST API** ❌

**Approach**: Keep REST API endpoints for configuration

**Rejected Because**:
- ❌ REST API has 0-10% business value (DD-TOOLSET-001)
- ❌ Architectural inconsistency (only service with REST API)
- ❌ Maintenance burden (~1000 LOC + OpenAPI spec)
- ❌ Not Kubernetes-native

---

### **Alternative 2: Multiple CRDs per Operation** ❌

**Approach**: Create separate CRDs for discovery, generation, validation

**Rejected Because**:
- ❌ Over-engineered (3 CRDs for simple configuration)
- ❌ Doesn't match use case (configuration, not workflow)
- ❌ Unnecessary complexity

---

### **Alternative 3: Single Configuration CRD** ✅

**Approach**: One `ToolsetConfig` CRD for all configuration

**Approved Because**:
- ✅ Simple (one CRD for all configuration)
- ✅ Matches use case (configuration, not workflow)
- ✅ Bounded status (one entry per service)
- ✅ Kubernetes-native (standard CRD pattern)
- ✅ Consistent with Kubernaut architecture

---

## ✅ **Approval**

**Status**: 📋 **PLANNED FOR V1.1**
**Date**: November 10, 2025
**Decision**: Implement as P0 priority (replaces deprecated REST API)
**Rationale**: Provides Kubernetes-native configuration to replace low-value REST API endpoints
**Approved By**: Architecture Team
**Related DD**: [DD-TOOLSET-001: REST API Deprecation](../architecture/decisions/DD-TOOLSET-001-REST-API-Deprecation.md)

---

## 📚 **References**

### **Related Business Requirements**
- BR-TOOLSET-021: Automatic Service Discovery (discovery logic reused)
- BR-TOOLSET-026: Discovery Loop Lifecycle Management (interval logic reused)
- BR-TOOLSET-031: ConfigMap Creation and Reconciliation (ConfigMap generation reused)

### **Related Design Decisions**
- DD-TOOLSET-001: REST API Deprecation and Configuration CRD Migration
- DD-007: Kubernetes-Aware Graceful Shutdown Pattern (controller shutdown)

### **Related Architecture Decisions**
- ADR-036: Authentication and Authorization Strategy (RBAC for CRD access)

---

**Document Version**: 1.0
**Last Updated**: November 10, 2025
**Status**: 📋 **PLANNED FOR V1.1 IMPLEMENTATION**


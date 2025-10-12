# Dynamic Toolset Service - V2 Business Requirements

**Version**: 2.0
**Status**: 📋 Planned (Post-V1)
**Last Updated**: 2025-10-11
**Prerequisites**: V1 Complete (Days 1-10)

---

## 🎯 V2 Enhancement Goals

**Focus**: Operational flexibility, configurability, and advanced features based on V1 deployment feedback.

---

## 📋 V2 Business Requirements

### BR-TOOLSET-037: Configurable Detector Management

**Priority**: High
**Effort**: 4-6 hours
**Confidence**: 85%
**Status**: Approved for V1 (Post-Day 10 implementation)

#### Description
Enable dynamic detector configuration to allow operators to enable/disable service detectors without code changes.

#### Business Value
- **Resource Optimization**: Disable unused detectors to save CPU/memory
- **Environment Flexibility**: Different detector sets for dev/staging/prod
- **Gradual Rollout**: Enable new detectors incrementally
- **Operational Control**: No rebuild required for detector changes

#### Acceptance Criteria

**AC-037-01**: Configuration File Support
```yaml
# config/detector-config.yaml
detectors:
  prometheus:
    enabled: true
    priority: 1
  grafana:
    enabled: true
    priority: 2
  jaeger:
    enabled: false  # Can disable unused detectors
    priority: 3
  elasticsearch:
    enabled: false
    priority: 4
  custom:
    enabled: true
    priority: 5
```

**AC-037-02**: Default Configuration
- MUST provide default configuration that enables all detectors (backward compatible)
- MUST gracefully fallback to defaults if config file missing
- MUST log which detectors are enabled at startup

**AC-037-03**: Priority-Based Detection
- MUST execute detectors in priority order (lower number = higher priority)
- MUST allow priority customization per detector
- MUST skip detection after first match (existing behavior)

**AC-037-04**: Kubernetes ConfigMap Integration
- MUST support loading configuration from Kubernetes ConfigMap
- MUST support file-based configuration
- MUST validate configuration on startup

#### Implementation Components

**New Files**:
```
pkg/toolset/config/
├── detector_config.go              # Configuration structures
└── loader.go                       # Config loading logic

pkg/toolset/discovery/
└── detector_factory.go             # Factory pattern for detectors

test/unit/toolset/
├── detector_config_test.go         # Config tests
└── detector_factory_test.go        # Factory tests

config/
└── detector-config.yaml            # Default configuration
```

**Modified Files**:
```
pkg/toolset/server/server.go        # Use factory for registration
cmd/dynamictoolset/main.go          # Load config
```

#### Technical Specifications

**Configuration Structure**:
```go
type DetectorConfig struct {
    Detectors map[string]DetectorSettings `yaml:"detectors"`
}

type DetectorSettings struct {
    Enabled  bool `yaml:"enabled"`
    Priority int  `yaml:"priority"`
}
```

**Detector Factory**:
```go
type DetectorFactory interface {
    CreateDetectors(config *DetectorConfig) []ServiceDetector
    RegisterCustomDetector(name string, factory func() ServiceDetector)
}
```

#### Testing Requirements
- Unit tests for config loading (valid/invalid YAML)
- Unit tests for factory pattern
- Unit tests for priority sorting
- Integration tests with different detector combinations
- Test graceful fallback to defaults

#### Deployment Considerations
- ConfigMap example for Kubernetes deployment
- Documentation for config file format
- Migration guide from V1 (no changes required)
- Validation errors must be clear and actionable

---

### BR-TOOLSET-038: Per-Detector Configuration

**Priority**: Medium
**Effort**: 3-4 hours
**Confidence**: 80%
**Status**: Planned for V2

#### Description
Allow per-detector configuration for health check timeouts, paths, and behavior customization.

#### Business Value
- **Flexibility**: Accommodate services with non-standard health endpoints
- **Performance Tuning**: Adjust timeouts per service type
- **Custom Behavior**: Configure detection logic per detector

#### Acceptance Criteria

**AC-038-01**: Health Check Customization
```yaml
detectors:
  prometheus:
    enabled: true
    priority: 1
    health_check:
      path: "/-/healthy"
      timeout: 5s
      retries: 3
  custom:
    enabled: true
    priority: 5
    required_annotations:
      - "kubernaut.io/toolset"
      - "kubernaut.io/toolset-type"
    namespaces:
      - "default"
      - "production"
```

**AC-038-02**: Namespace Filtering
- MUST support namespace inclusion/exclusion per detector
- MUST support wildcard namespace patterns
- MUST validate namespace filters at startup

**AC-038-03**: Annotation Requirements
- MUST support custom annotation requirements per detector
- MUST support AND/OR logic for multiple annotations
- MUST validate annotation patterns

#### Technical Specifications

**Extended Configuration**:
```go
type DetectorSettings struct {
    Enabled            bool                    `yaml:"enabled"`
    Priority           int                     `yaml:"priority"`
    HealthCheck        *HealthCheckSettings    `yaml:"health_check,omitempty"`
    RequiredAnnotations []string               `yaml:"required_annotations,omitempty"`
    Namespaces         []string                `yaml:"namespaces,omitempty"`
}

type HealthCheckSettings struct {
    Path    string        `yaml:"path"`
    Timeout time.Duration `yaml:"timeout"`
    Retries int           `yaml:"retries"`
}
```

#### Dependencies
- BR-TOOLSET-037 (must be implemented first)

---

### BR-TOOLSET-039: Hot Configuration Reload

**Priority**: Low
**Effort**: 6-8 hours
**Confidence**: 70%
**Status**: Planned for V2.1

#### Description
Support reloading detector configuration without service restart.

#### Business Value
- **Zero Downtime**: Change detector config without pod restart
- **Faster Iteration**: Test detector configurations quickly
- **Operational Efficiency**: No disruption to running service

#### Acceptance Criteria

**AC-039-01**: File Watch Support
- MUST watch configuration file for changes
- MUST validate new configuration before applying
- MUST log configuration reload events
- MUST handle reload failures gracefully

**AC-039-02**: API Endpoint for Reload
```bash
POST /api/v1/config/reload
Authorization: Bearer <token>

Response:
{
  "status": "success",
  "detectors_enabled": 3,
  "detectors_disabled": 2,
  "reload_time": "2025-10-11T13:30:00Z"
}
```

**AC-039-03**: Graceful Transition
- MUST complete in-flight discoveries before reload
- MUST not drop service detections during reload
- MUST maintain consistent state

#### Technical Challenges
- **State Management**: Need to handle in-flight discoveries
- **Thread Safety**: Concurrent access during reload
- **Validation**: Ensure new config is valid before applying

#### Dependencies
- BR-TOOLSET-037 (configuration foundation)
- BR-TOOLSET-038 (extended config options)

---

### BR-TOOLSET-040: Detector Plugins

**Priority**: Low
**Effort**: 12-16 hours
**Confidence**: 60%
**Status**: Planned for V2.2

#### Description
Support loading external detector implementations as plugins without recompilation.

#### Business Value
- **Extensibility**: Users can add custom detectors
- **Third-Party Integration**: Community can contribute detectors
- **Rapid Development**: Test new detectors without core changes

#### Acceptance Criteria

**AC-040-01**: Plugin Interface
```go
type DetectorPlugin interface {
    ServiceDetector
    Name() string
    Version() string
    Description() string
}
```

**AC-040-02**: Plugin Loading
- MUST support loading Go plugins (.so files)
- MUST validate plugin interface compatibility
- MUST handle plugin load failures gracefully
- MUST log plugin registration

**AC-040-03**: Plugin Configuration
```yaml
detectors:
  prometheus:
    enabled: true
    priority: 1
  custom-plugin:
    enabled: true
    priority: 10
    plugin:
      path: "/plugins/custom-detector.so"
      config:
        endpoint_pattern: "*.custom.com"
```

#### Technical Challenges
- **Go Plugin Limitations**: Same Go version requirement, platform-specific
- **Security**: Sandboxing and validation of external code
- **Dependency Management**: Plugin dependencies may conflict

#### Alternative Approach
- **WebAssembly (WASM)**: More portable, safer sandboxing
- **gRPC Plugins**: Run as separate processes, language-agnostic

---

### BR-TOOLSET-041: Discovery Metrics Dashboard

**Priority**: Medium
**Effort**: 4-6 hours
**Confidence**: 85%
**Status**: Planned for V2

#### Description
Provide built-in dashboard or enhanced metrics for discovery monitoring.

#### Business Value
- **Observability**: Better visibility into discovery performance
- **Troubleshooting**: Identify detector issues quickly
- **Capacity Planning**: Understand discovery resource usage

#### Acceptance Criteria

**AC-041-01**: Enhanced Metrics
```
# Per-detector metrics
dynamic_toolset_detector_duration_seconds{detector="prometheus"}
dynamic_toolset_detector_services_found{detector="prometheus"}
dynamic_toolset_detector_errors_total{detector="prometheus",error_type="timeout"}

# Discovery statistics
dynamic_toolset_discovery_services_total{namespace="monitoring"}
dynamic_toolset_discovery_skipped_total{reason="no_match"}
```

**AC-041-02**: Grafana Dashboard
- MUST provide Grafana dashboard JSON
- MUST include discovery performance panels
- MUST include detector success/failure rates
- MUST include health check failure breakdown

**AC-041-03**: API Endpoint for Statistics
```bash
GET /api/v1/statistics
Authorization: Bearer <token>

Response:
{
  "discoveries_total": 1234,
  "services_discovered": 45,
  "detectors": {
    "prometheus": {"enabled": true, "services": 12, "success_rate": 0.95},
    "grafana": {"enabled": true, "services": 8, "success_rate": 1.0}
  },
  "last_discovery": "2025-10-11T13:30:00Z"
}
```

---

### BR-TOOLSET-042: ConfigMap Reconciliation Loop

**Priority**: High
**Effort**: 6-8 hours
**Confidence**: 80%
**Status**: Planned for V1.1

#### Description
Automatically create and update ConfigMaps in Kubernetes cluster based on discovered services.

#### Business Value
- **Automation**: No manual ConfigMap management
- **Consistency**: ConfigMaps always reflect current services
- **HolmesGPT Integration**: Seamless toolset updates

#### Acceptance Criteria

**AC-042-01**: Automatic ConfigMap Creation
- MUST create ConfigMap if it doesn't exist
- MUST update ConfigMap when services change
- MUST preserve manual overrides (BR-TOOLSET-030)
- MUST detect and handle drift (BR-TOOLSET-031)

**AC-042-02**: Reconciliation Loop
- MUST reconcile every 5 minutes (configurable)
- MUST use Kubernetes Watch API for efficiency
- MUST handle API errors gracefully
- MUST retry with exponential backoff

**AC-042-03**: Metrics
```
dynamic_toolset_configmap_reconciliations_total{result="success"}
dynamic_toolset_configmap_drift_detections_total
dynamic_toolset_configmap_update_errors_total{error_type="api_error"}
```

#### Technical Specifications

**Reconciler Interface**:
```go
type ConfigMapReconciler interface {
    Start(ctx context.Context) error
    Stop() error
    ReconcileNow(ctx context.Context) error
}
```

#### Dependencies
- BR-TOOLSET-029 (ConfigMap builder)
- BR-TOOLSET-030 (Override preservation)
- BR-TOOLSET-031 (Drift detection)

---

## 📊 V2 Implementation Priority Matrix

| Requirement | Priority | Effort | Value | Risk | V1.x Target |
|-------------|----------|--------|-------|------|-------------|
| BR-TOOLSET-037 (Configurable Detectors) | High | 6h | High | Low | V1 (Post-Day 10) |
| BR-TOOLSET-042 (ConfigMap Reconciliation) | High | 8h | High | Medium | V1.1 |
| BR-TOOLSET-038 (Per-Detector Config) | Medium | 4h | Medium | Low | V2.0 |
| BR-TOOLSET-041 (Metrics Dashboard) | Medium | 6h | Medium | Low | V2.0 |
| BR-TOOLSET-039 (Hot Reload) | Low | 8h | Medium | High | V2.1 |
| BR-TOOLSET-040 (Detector Plugins) | Low | 16h | Low | High | V2.2 |

---

## 🎯 Recommended Implementation Order

### V1 (Post-Day 10)
1. **BR-TOOLSET-037**: Configurable Detectors (6 hours)
   - Foundation for all other features
   - High value, low risk
   - Approved for immediate implementation

### V1.1 (Post-V1 Deployment)
2. **BR-TOOLSET-042**: ConfigMap Reconciliation Loop (8 hours)
   - Critical for HolmesGPT integration
   - Completes V1 automation story
   - Leverages existing ConfigMap builder

### V2.0 (Based on User Feedback)
3. **BR-TOOLSET-038**: Per-Detector Configuration (4 hours)
   - Builds on BR-TOOLSET-037
   - Addresses operational flexibility needs

4. **BR-TOOLSET-041**: Metrics Dashboard (6 hours)
   - Enhances observability
   - Supports troubleshooting

### V2.1 (Advanced Features)
5. **BR-TOOLSET-039**: Hot Configuration Reload (8 hours)
   - Nice-to-have, not critical
   - Requires careful state management

### V2.2 (Extensibility)
6. **BR-TOOLSET-040**: Detector Plugins (16 hours)
   - Most complex, highest risk
   - Consider alternative approaches (WASM, gRPC)

---

## 📝 Success Metrics

### V1 Success Criteria
- ✅ All 5 standard detectors configurable
- ✅ Backward compatible (all enabled by default)
- ✅ ConfigMap-based configuration supported
- ✅ Zero breaking changes from V1

### V2 Success Criteria
- ✅ Per-detector customization working
- ✅ Grafana dashboard available
- ✅ 90%+ user satisfaction with configurability
- ✅ < 5% performance overhead from configuration

---

## 🔗 Related Documentation

- **V1 Implementation**: `docs/services/stateless/dynamic-toolset/implementation/`
- **API Specification**: `docs/services/stateless/dynamic-toolset/api-specification.md`
- **Architecture**: `docs/architecture/APPROVED_MICROSERVICES_ARCHITECTURE.md`
- **Testing Strategy**: `docs/testing/TESTING_FRAMEWORK.md`

---

**Document Status**: ✅ Complete
**Approval Status**: ✅ Approved for V1 Post-Day 10
**Next Review**: After V1 deployment feedback

---

*Created: 2025-10-11*
*Approved By: User*
*Implementation Target: V1 (BR-TOOLSET-037), V1.1 (BR-TOOLSET-042)*


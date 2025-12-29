# ADR-050: Configuration Validation Strategy

## Status
**✅ Approved** (2025-12-16)
**Last Reviewed**: 2025-12-16
**Confidence**: 95%

---

## Context & Problem

**Problem**: Multiple Kubernaut services load configuration from various sources (Rego policies, YAML ConfigMaps, JSON settings, environment variables, certificates). Configuration errors discovered at runtime lead to:

1. **Delayed Error Discovery**: Invalid config crashes services during first use in production
2. **Cascading Failures**: Bad config propagates through systems before detection
3. **Silent Degradation**: Services fall back to defaults without alerting operators
4. **Performance Overhead**: Re-parsing/compiling configuration on every operation
5. **Difficult Rollbacks**: Bad config may not be discovered until after successful deployment

**Specific Example** (AIAnalysis Rego Policy):
```go
// CURRENT (WRONG): Runtime compilation on every reconciliation
func (e *Evaluator) Evaluate(ctx context.Context, ...) {
    policyContent, err := os.ReadFile(e.policyPath)  // ❌ File I/O every call
    query, err := rego.New(...).PrepareForEval(ctx)  // ❌ Compile every call (2-5ms)
    if err != nil {
        return &PolicyResult{Degraded: true}, nil     // ❌ Silent degradation
    }
}
```

**Impact**: Syntax errors in Rego policies are only discovered during first reconciliation in production, not during deployment.

**Why This Matters**:
- **Operational Safety**: Invalid config should prevent deployment (fail-fast), not degrade service in production
- **Developer Experience**: Configuration errors should be caught in CI/CD, not discovered in production
- **Performance**: Validation cost should be paid once at startup, not on every operation
- **Consistency**: All services should follow the same validation pattern

**Scope**: All configuration types across all services:
- **Rego Policies**: SignalProcessing, AIAnalysis, WorkflowExecution
- **YAML ConfigMaps**: Gateway (rate limits), WorkflowExecution (playbooks)
- **JSON Settings**: HolmesGPT-API (LLM configurations)
- **Environment Variables**: All services (ports, timeouts, feature flags)
- **Certificate Files**: Data Storage (TLS certificates)

---

## Decision

### Core Principle
> **Fail-fast on startup, gracefully degrade at runtime**
>
> - **Startup**: ALL configuration MUST be validated. Invalid config = pod fails to start (exit 1)
> - **Runtime**: Hot-reloaded config MAY gracefully degrade (keep old config on validation failure)

### Validation Strategy by Configuration Type

| Configuration Type | Validation Method | Startup Behavior | Runtime Reload Behavior |
|---|---|---|---|
| **Rego Policies** | Compile + test query execution | Fatal error | Graceful degradation |
| **YAML ConfigMaps** | Parse + schema validation | Fatal error | Graceful degradation |
| **JSON Settings** | Parse + struct unmarshal + constraints | Fatal error | Graceful degradation |
| **Environment Variables** | Parse + range checks + required fields | Fatal error | N/A (no reload) |
| **Certificate Files** | Load + verify expiry + chain validation | Fatal error | Graceful degradation |

### Implementation Pattern

#### Pattern 1: File-Based Configuration (Rego, YAML, JSON, Certs)

**Use**: `pkg/shared/hotreload/FileWatcher` (per DD-INFRA-001)

**Startup Validation**:
```go
// Step 1: Create FileWatcher with validation callback
watcher, err := hotreload.NewFileWatcher(
    configPath,
    func(content string) error {
        // Validate and load new configuration
        if err := validateAndLoadConfig(content); err != nil {
            return fmt.Errorf("config validation failed: %w", err)
        }
        return nil
    },
    logger,
)
if err != nil {
    logger.Error(err, "failed to create config watcher")
    os.Exit(1)  // ✅ Fatal error at startup
}

// Step 2: Start watcher (loads initial config + validates via callback)
if err := watcher.Start(ctx); err != nil {
    logger.Error(err, "failed to load initial configuration")
    os.Exit(1)  // ✅ Fatal error at startup
}
```

**Runtime Reload** (Graceful Degradation):
```go
func (s *Service) validateAndLoadConfig(content string) error {
    // Try to compile/parse new configuration
    newConfig, err := parseConfig(content)
    if err != nil {
        // ✅ Graceful degradation: keep old config, log error
        return fmt.Errorf("invalid config: %w", err)
    }

    // Apply new configuration
    s.mu.Lock()
    s.config = newConfig
    s.mu.Unlock()

    logger.Info("configuration reloaded successfully")
    return nil
}
```

#### Pattern 2: Environment Variables

**Startup Validation**:
```go
// Validate ALL required environment variables at startup
func validateEnvironment() error {
    port := os.Getenv("SERVICE_PORT")
    if port == "" {
        return fmt.Errorf("SERVICE_PORT is required")
    }

    portNum, err := strconv.Atoi(port)
    if err != nil || portNum < 1024 || portNum > 65535 {
        return fmt.Errorf("SERVICE_PORT must be 1024-65535, got: %s", port)
    }

    // ... validate other required vars
    return nil
}

func main() {
    if err := validateEnvironment(); err != nil {
        setupLog.Error(err, "environment validation failed")
        os.Exit(1)  // ✅ Fatal error at startup
    }
}
```

#### Pattern 3: Rego Policies (Specific Implementation)

**Validation Requirements**:
1. **Syntax Check**: Policy compiles without errors
2. **Query Execution**: Test query returns expected result structure
3. **Caching**: Compiled policy cached for runtime use (eliminate I/O + compilation overhead)

**Reference Implementation**: `pkg/signalprocessing/rego/engine.go`

```go
type Engine struct {
    compiledQuery rego.PreparedEvalQuery  // ✅ Cached compiled policy
    fileWatcher   *hotreload.FileWatcher
    mu            sync.RWMutex
}

// LoadPolicy validates and caches compiled policy
func (e *Engine) LoadPolicy(policyContent string) error {
    // ✅ Validate: Compile policy to check syntax
    query, err := rego.New(
        rego.Query("data.service.policy"),
        rego.Module("policy.rego", policyContent),
    ).PrepareForEval(context.Background())
    if err != nil {
        return fmt.Errorf("policy compilation failed: %w", err)
    }

    // ✅ Cache compiled policy
    e.mu.Lock()
    e.compiledQuery = query
    e.mu.Unlock()

    return nil
}

// StartHotReload initializes with startup validation
func (e *Engine) StartHotReload(ctx context.Context) error {
    e.fileWatcher, err = hotreload.NewFileWatcher(
        e.policyPath,
        func(content string) error {
            return e.LoadPolicy(content)  // Validates via compilation
        },
        e.logger,
    )
    if err != nil {
        return err
    }

    // ✅ Start() loads initial config and validates
    return e.fileWatcher.Start(ctx)
}
```

**Main Entry Point**:
```go
func main() {
    regoEngine := rego.NewEngine(logger, policyPath)

    // ✅ Startup validation: fails fast on invalid policy
    if err := regoEngine.StartHotReload(ctx); err != nil {
        setupLog.Error(err, "failed to load Rego policy")
        os.Exit(1)  // ✅ Fatal error at startup
    }

    setupLog.Info("Rego policy loaded successfully",
        "policyHash", regoEngine.GetPolicyHash())

    // ... rest of service initialization
}
```

---

## Rationale

### Why Fail-Fast at Startup?

**Benefit 1: Kubernetes Deployment Safety**
```
Invalid config deployed → Pod fails to start → Deployment rollback → Previous version preserved
```

**Benefit 2: CI/CD Feedback Loop**
```
Developer pushes bad config → CI deploys to staging → Pod fails → Developer notified immediately
```

**Benefit 3: Prevents Cascading Failures**
```
Without startup validation: Bad config → Service starts → Processes requests → Fails at runtime → Cascades to dependents
With startup validation:    Bad config → Service fails → Kubernetes retries → Old version still running
```

### Why Graceful Degradation at Runtime?

**Hot-reload updates should NOT crash running services**:
- ConfigMap updates happen via Kubernetes `kubelet` (~60s propagation)
- Invalid hot-reload should be non-fatal (log error, keep old config)
- Service continues with last-known-good configuration
- Operators alerted via metrics/logs to fix ConfigMap

**Trade-off**: Startup validation is strict (fail-fast), runtime updates are permissive (graceful degradation).

### Performance Benefits

**Rego Policy Example**:
| Metric | Without Caching | With Caching | Improvement |
|---|---|---|---|
| File I/O per call | ~0.5ms | 0ms | 100% |
| Compilation per call | 2-5ms | 0ms | 100% |
| Evaluation per call | 1-2ms | 1-2ms | 0% |
| **Total per call** | **3.5-7.5ms** | **1-2ms** | **71-83%** |

**Workload**: 100 reconciliations/min = **250-550ms saved per minute** per controller instance.

---

## Compliance Checklist

All services MUST meet these requirements:

### Startup Validation
- [ ] **All configuration validated during startup** (before accepting traffic)
- [ ] **Invalid configuration causes pod to exit** (exit code 1)
- [ ] **Validation errors logged with actionable details** (file path, line number, error message)
- [ ] **Startup validation tested** (unit tests verify exit on invalid config)

### Runtime Hot-Reload (if applicable)
- [ ] **Uses `pkg/shared/hotreload/FileWatcher`** (per DD-INFRA-001)
- [ ] **Invalid runtime updates gracefully degrade** (keep old config, log error)
- [ ] **Metrics track reload success/failure** (e.g., `config_reload_total{status="success|failure"}`)
- [ ] **Compilation/parsing results cached** (eliminate runtime overhead)

### Testing Requirements
- [ ] **Unit tests verify startup validation failures** (invalid config → service exits)
- [ ] **Integration tests verify graceful degradation** (invalid hot-reload → old config preserved)
- [ ] **E2E tests use production configuration files** (not mocks)

---

## Service-Specific Compliance Status

### ✅ Compliant Services
| Service | Config Type | Startup Validation | Hot-Reload | Caching | Notes |
|---|---|---|---|---|---|
| **SignalProcessing** | Rego (4 policies) | ✅ | ✅ | ✅ | **All policies MANDATORY** (2025-12-20) |
| **Data Storage** | TLS certs | ✅ | ❌ | N/A | Restarts on cert rotation |

> **SignalProcessing Update (2025-12-20)**: All Rego policies (environment, priority, business, customlabels) are now **MANDATORY**. Missing policy files cause fatal startup errors. Go-level fallbacks have been removed. Operators define defaults using Rego `default` keyword.
>
> - BR-SP-071: DEPRECATED (Go priority fallback removed)
> - BR-SP-052: DEPRECATED (Go ConfigMap fallback removed)
> - BR-SP-053: DEPRECATED (Go "unknown" default removed)

### ⚠️ Partial Compliance
| Service | Config Type | Startup Validation | Hot-Reload | Caching | Notes |
|---|---|---|---|---|---|
| **HolmesGPT-API** | YAML (LLM config) | ✅ | ✅ | ❌ | Python FileWatcher (DD-HAPI-004) |

### ❌ Non-Compliant Services (V1.0 Fix Required)
| Service | Config Type | Startup Validation | Hot-Reload | Caching | Gap |
|---|---|---|---|---|---|
| **AIAnalysis** | Rego (approval.rego) | ❌ | ❌ | ❌ | **Loads policy at runtime (every call)** |

**Action Required**: AIAnalysis V1.0 must implement DD-AIANALYSIS-002 (Rego Policy Startup Validation).

---

## Migration Guide

### For Services Using Rego Policies

**Before** (Runtime Loading):
```go
func (e *Evaluator) Evaluate(ctx context.Context, ...) {
    policyContent, err := os.ReadFile(e.policyPath)  // ❌
    query, err := rego.New(...).PrepareForEval(ctx)  // ❌
    // ... evaluation
}
```

**After** (Startup Validation + Caching):
```go
type Evaluator struct {
    compiledQuery rego.PreparedEvalQuery  // ✅ Cached
    fileWatcher   *hotreload.FileWatcher
    mu            sync.RWMutex
}

func (e *Evaluator) StartHotReload(ctx context.Context) error {
    e.fileWatcher, err = hotreload.NewFileWatcher(
        e.policyPath,
        func(content string) error {
            return e.LoadPolicy(content)  // Validates + caches
        },
        e.logger,
    )
    return e.fileWatcher.Start(ctx)  // ✅ Fails fast on invalid policy
}

func (e *Evaluator) Evaluate(ctx context.Context, ...) {
    e.mu.RLock()
    query := e.compiledQuery  // ✅ Use cached query
    e.mu.RUnlock()

    results, err := query.Eval(ctx, rego.EvalInput(input))
    // ... evaluation
}
```

### For Services Using YAML/JSON ConfigMaps

**Before** (No Validation):
```go
func loadConfig() (*Config, error) {
    data, _ := os.ReadFile(configPath)
    var cfg Config
    yaml.Unmarshal(data, &cfg)  // ❌ No error check
    return &cfg, nil
}
```

**After** (Startup Validation):
```go
func main() {
    watcher, err := hotreload.NewFileWatcher(
        configPath,
        func(content string) error {
            var cfg Config
            if err := yaml.Unmarshal([]byte(content), &cfg); err != nil {
                return fmt.Errorf("YAML parse failed: %w", err)
            }
            if err := cfg.Validate(); err != nil {  // ✅ Schema validation
                return fmt.Errorf("invalid config: %w", err)
            }
            applyConfig(&cfg)
            return nil
        },
        logger,
    )

    if err := watcher.Start(ctx); err != nil {
        setupLog.Error(err, "failed to load configuration")
        os.Exit(1)  // ✅ Fatal at startup
    }
}
```

---

## Alternatives Considered

### Alternative 1: Runtime-Only Validation (Current AIAnalysis Approach)
**Approach**: Load and validate configuration on first use

**Pros**:
- ✅ Simple implementation (no extra startup logic)
- ✅ Service always starts (even with bad config)

**Cons**:
- ❌ **Delayed error discovery** (first request in production fails)
- ❌ **Silent degradation** (bad config may cause fallback behavior)
- ❌ **Performance overhead** (validation cost paid on every operation)
- ❌ **Difficult rollback** (bad config may pass deployment checks)

**Confidence**: 10% (rejected - operational safety and performance concerns)

---

### Alternative 2: Pre-Deployment Validation (CI/CD Only) ⚠️
**Approach**: Validate configuration files in CI/CD pipeline before deployment

**Pros**:
- ✅ Early error detection (before deployment)
- ✅ No runtime overhead

**Cons**:
- ❌ **Doesn't catch environment-specific issues** (file permissions, missing files)
- ❌ **ConfigMap hot-reload bypasses CI** (manual `kubectl apply` not validated)
- ❌ **Drift between validation and runtime** (different parsers in CI vs service)

**Confidence**: 40% (insufficient alone - MUST be combined with startup validation)

**Decision**: Use BOTH CI/CD validation AND startup validation for defense-in-depth.

---

### Alternative 3: Startup Validation + Runtime Hot-Reload ⭐ **SELECTED**
**Approach**: Validate all config at startup (fail-fast), gracefully degrade on hot-reload

**Pros**:
- ✅ **Fail-fast on deployment** (bad config prevents pod startup)
- ✅ **Kubernetes-native rollback** (failed deployment preserves old version)
- ✅ **Performance optimization** (validation + compilation cached)
- ✅ **Graceful hot-reload** (invalid updates don't crash service)
- ✅ **Operational visibility** (metrics track reload success/failure)

**Cons**:
- ⚠️ **Slightly more complex** (requires FileWatcher integration)
- ⚠️ **Hot-reload errors silent** (operators must monitor metrics)

**Confidence**: 95% (selected - best trade-off of safety, performance, and operability)

---

## Impact Analysis

### Breaking Changes
**None** - This is a new standard for future services and V1.0+ fixes.

**Existing Services**:
- SignalProcessing: Already compliant (no changes needed)
- AIAnalysis: Requires V1.0 fix (DD-AIANALYSIS-002)
- HolmesGPT-API: Python implementation compliant (DD-HAPI-004)

### Risk Assessment
| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| **Service fails to start with valid config** | Low | High | Comprehensive unit tests for validation logic |
| **Hot-reload regression** | Low | Medium | Integration tests verify graceful degradation |
| **Performance regression** | Very Low | Low | Caching eliminates repeated validation overhead |
| **Increased startup time** | Low | Low | Validation adds <100ms to startup (acceptable) |

### Rollout Plan
1. **Phase 1 (Immediate)**: Fix AIAnalysis V1.0 (DD-AIANALYSIS-002)
2. **Phase 2 (V1.1)**: Audit all services for compliance
3. **Phase 3 (V1.2)**: Mandate compliance for all new services

---

## Related Decisions

### Parent Decisions
- **DD-INFRA-001**: ConfigMap Hot-Reload Pattern (defines FileWatcher implementation)

### Child Decisions (Service-Specific)
- **DD-AIANALYSIS-002**: Rego Policy Startup Validation (AIAnalysis implementation)
- **DD-HAPI-004**: ConfigMap Hot-Reload (HolmesGPT-API Python implementation)
- **DD-SP-XXX**: (Implicit - SignalProcessing already compliant)

### Related Decisions
- **ADR-031**: OpenAPI Specification Standard (validates API contract files)
- **ADR-046**: Struct Validation Standard (validates Go struct constraints)

---

## Success Metrics

### Service-Level Metrics
```prometheus
# Track hot-reload success/failure
config_reload_total{service="aianalysis",status="success"} 42
config_reload_total{service="aianalysis",status="failure"} 2

# Track validation duration
config_validation_duration_seconds{service="aianalysis"} 0.023
```

### Target SLOs
- **Startup validation success rate**: >99.9% (for valid configuration)
- **Hot-reload graceful degradation**: 100% (invalid updates never crash service)
- **Performance overhead reduction**: >70% (via caching)

### Validation Checklist (Per Service)
```bash
# Verify startup validation
go test ./pkg/service -run TestStartupValidation_InvalidConfig

# Verify hot-reload graceful degradation
go test ./pkg/service -run TestHotReload_InvalidUpdate

# Verify performance improvement
go test ./pkg/service -bench BenchmarkEvaluate
```

---

## Appendix: Reference Implementations

### Go Service (Rego Policy)
- **Reference**: `pkg/signalprocessing/rego/engine.go`
- **Library**: `pkg/shared/hotreload/file_watcher.go`
- **Main Entry**: `cmd/signalprocessing/main.go:240-252`

### Python Service (YAML Config)
- **Reference**: `holmesgpt-api/src/config/hot_reload.py`
- **Library**: `watchdog` (Python equivalent of fsnotify)
- **Design**: DD-HAPI-004

### Test Examples
- **Unit Tests**: `pkg/signalprocessing/rego/engine_test.go`
- **Integration Tests**: `test/integration/signalprocessing/rego_integration_test.go`

---

**Prepared By**: AI Assistant (Cursor)
**Approved By**: Architecture Team
**Effective Date**: 2025-12-16
**Review Date**: 2026-03-16 (quarterly review)



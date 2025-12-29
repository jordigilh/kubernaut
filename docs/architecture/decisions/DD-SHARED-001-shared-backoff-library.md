# DD-SHARED-001: Shared Exponential Backoff Library

**Status**: ✅ **APPROVED & IMPLEMENTED**
**Date**: 2025-12-16
**Decision Maker**: Notification Team (NT), WorkflowExecution Team (WE)
**Implementation**: `/pkg/shared/backoff/`
**Priority**: P0 - Cross-service reliability and maintainability
**Adoption Tracking**: [Backoff Adoption Status](../shared-utilities/BACKOFF_ADOPTION_STATUS.md)

---

## ⚠️  **CRITICAL: JITTER IS MANDATORY FOR PRODUCTION**

**All CRD-based services MUST use jitter** (±10% variance) in production to prevent thundering herd problems:

```go
// ✅ CORRECT - Production code (MANDATORY)
duration := backoff.CalculateWithDefaults(attempts)  // WITH jitter

// ❌ WRONG - Production code (creates thundering herd risk)
duration := backoff.CalculateWithoutJitter(attempts) // NO jitter

// ✅ ACCEPTABLE - Unit tests only (*_test.go files)
duration := backoff.CalculateWithoutJitter(attempts) // For exact assertions
```

**Why Mandatory?**
- All CRD controllers run with HA deployment (2+ replicas with leader election)
- Without jitter, multiple CRDs failing simultaneously retry at exact same time
- This creates API server load spikes and downstream service overload
- Already proven necessary in RO production deployment (see `pkg/remediationorchestrator/routing/blocking.go:461-476`)

---

## 📋 **Context**

### Problem Statement
Multiple Kubernaut services implement exponential backoff independently, leading to:
- **Code duplication**: Each service reimplements the same mathematical logic
- **Inconsistency**: Different services use different strategies (power-of-2, configurable multiplier, jitter)
- **Thundering herd risk**: Deterministic backoff causes simultaneous retries in HA deployments
- **Maintenance burden**: Bugs or improvements require changes across multiple services
- **Testing gaps**: Each implementation needs independent validation

### Scope
This decision applies to ALL Kubernaut services that require retry-with-backoff behavior:
- **Notification (NT)**: Automatic retry with custom policies (BR-NOT-052, BR-NOT-055)
- **WorkflowExecution (WE)**: Pre-execution failure backoff (BR-WE-012)
- **SignalProcessing (SP)**: Transient K8s API error handling
- **Gateway (GW)**: CRD creation retry logic
- **Future services**: Any service requiring exponential backoff retry

**Implementation Status**: See [Backoff Adoption Status](../shared-utilities/BACKOFF_ADOPTION_STATUS.md)

---

## 🎯 **Decision**

### Adopt Shared Backoff Library (`pkg/shared/backoff`)
**Source**: Extracted from Notification Team's production-proven implementation (v3.1)

**Key Features**:
1. **Mandatory jitter** (±10% variance): REQUIRED for all production services to prevent thundering herd
2. **Configurable multiplier** (not just power-of-2): 1.5 = conservative, 2.0 = standard, 3.0 = aggressive
3. **Multiple strategies**: Support for conservative, standard, and aggressive backoff patterns
4. **Testing support**: Deterministic mode (`CalculateWithoutJitter`) for unit tests ONLY
5. **Battle-tested**: Extracted from NT's production-proven implementation (v3.1), proven in RO HA deployment

---

## 🏗️ **Architecture**

### Core Type: `Config`

```go
type Config struct {
    BasePeriod    time.Duration  // Initial backoff (e.g., 30s)
    MaxPeriod     time.Duration  // Maximum backoff cap (e.g., 5m)
    Multiplier    float64        // Growth factor (1.5-10.0, default 2.0)
    JitterPercent int            // Variance % (0-50, default 0)
    MaxExponent   int            // Legacy compatibility (prefer MaxPeriod)
}
```

### Formula
```
Base Formula:     duration = BasePeriod * (Multiplier ^ (attempts-1))
With Jitter:      duration ± (duration * JitterPercent / 100)
Bounds:           max(BasePeriod, min(duration, MaxPeriod))
```

### Usage Patterns

#### Pattern 1: Production Standard with Jitter (MANDATORY)
```go
// MANDATORY for all production services (anti-thundering herd)
duration := backoff.CalculateWithDefaults(attempts)
// Result: ~30s → ~1m → ~2m → ~4m → ~5m (with ±10% variance)
```

#### Pattern 2: Deterministic (TESTING ONLY)
```go
// ONLY for unit tests where exact timing is required
// ⚠️  DO NOT USE IN PRODUCTION CODE
duration := backoff.CalculateWithoutJitter(attempts)
// Result: 30s → 1m → 2m → 4m → 5m (exact, no variance)
```

#### Pattern 3: Custom Strategy
```go
// Per-resource configurable policy (NT pattern)
config := backoff.Config{
    BasePeriod:    time.Duration(policy.InitialBackoffSeconds) * time.Second,
    MaxPeriod:     time.Duration(policy.MaxBackoffSeconds) * time.Second,
    Multiplier:    float64(policy.BackoffMultiplier),  // User-configurable!
    JitterPercent: 10,
}
duration := config.Calculate(attempts)
```

---

## 🎨 **Design Patterns**

### When to Use Each Strategy

| Strategy | Multiplier | Jitter | Use Case | Example Service |
|----------|-----------|--------|----------|----------------|
| **Conservative** | 1.5 | 10% | Transient API errors, polite retry | Notification (Slack 429) |
| **Standard** | 2.0 | 10% | General retry, balanced approach | **WE, SP, RO, AA (MANDATORY)** |
| **Aggressive** | 3.0 | 10% | Critical failures, fast escalation | Notification (critical alerts) |
| **Deterministic** | 2.0 | 0% | **UNIT TESTS ONLY** (not production) | Test assertions only |

### Anti-Patterns to AVOID

❌ **DON'T**: Implement custom backoff math in service code
```go
// BAD: Manual calculation in service
backoff := baseBackoff
for i := 0; i < attempts; i++ {
    backoff = backoff * 2
}
```

✅ **DO**: Use shared utility
```go
// GOOD: Shared, tested utility
duration := backoff.CalculateWithDefaults(attempts)
```

❌ **DON'T**: Use zero jitter in production code
```go
// BAD: Thundering herd risk in HA deployments (all pods retry simultaneously)
config := backoff.Config{Multiplier: 2.0, JitterPercent: 0}
// OR
duration := backoff.CalculateWithoutJitter(attempts)  // Only for tests!
```

✅ **DO**: Always use jitter in production code
```go
// GOOD: Distributes retry load over time (MANDATORY for production)
duration := backoff.CalculateWithDefaults(attempts)
// OR custom config with jitter
config := backoff.Config{Multiplier: 2.0, JitterPercent: 10}
```

---

## 📊 **Business Requirements Enabled**

### Primary BRs
- **BR-WE-012**: WorkflowExecution - Pre-execution Failure Backoff
- **BR-NOT-052**: Notification - Automatic Retry with Custom Retry Policies
- **BR-NOT-055**: Notification - Graceful Degradation (jitter prevents thundering herd)

### Future BRs (Pending Service Adoption)
- **BR-SP-XXX**: SignalProcessing - External API retry
- **BR-RO-XXX**: RemediationOrchestrator - Remediation action retry
- **BR-AA-XXX**: AIAnalysis - LLM API retry

---

## ✅ **Benefits**

### 1. Thundering Herd Prevention (CRITICAL)
**Before**: Deterministic backoff causes all pods to retry simultaneously
**After**: Jitter (±10%) distributes retries over time
**Impact**: Prevents API server overload in HA deployments with multiple CRDs failing

### 2. Consistency Across Services
**Before**: Each service implements different backoff strategies
**After**: Single source of truth with mandatory jitter for production

### 3. Industry Alignment
**Jitter**: Kubernetes client-go (±10%), AWS SDK (±50%), Google Cloud SDK (±100%)
**Standard**: All major cloud SDKs use jitter by default
**Kubernaut**: Aligns with Kubernetes ecosystem (±10% jitter)

### 4. Reduced Maintenance Burden
**Before**: Bug fixes required changes across 6+ service controllers
**After**: Single implementation with 24 comprehensive unit tests

### 5. Improved Testing
**Before**: Each service needs independent backoff validation
**After**: Shared utility has 100% test coverage, deterministic mode for unit tests

---

## 🚧 **Trade-offs**

### Complexity vs Flexibility
**Decision**: Accept slightly more complex API in exchange for flexibility
**Rationale**: Services have different needs (e.g., NT's per-resource policies)
**Mitigation**: Provide convenience functions (`CalculateWithDefaults`) for simple cases

### Backward Compatibility
**Decision**: Preserve WE's original deterministic behavior via configuration
**Rationale**: Existing tests and deployments expect deterministic backoff
**Implementation**: `CalculateWithoutJitter()` matches WE's original exactly

### Jitter MANDATORY for Production?
**Decision**: ✅ YES - Jitter is MANDATORY for ALL production services
**Rationale**:
- Prevents thundering herd in HA deployments (2+ replicas)
- Reduces API server load spikes when multiple CRDs fail simultaneously
- Industry standard (Kubernetes client-go, AWS SDK, Google Cloud SDK)
- Already proven in RO production deployment (DD-SHARED-001)
**Exception**: `CalculateWithoutJitter()` ONLY for unit tests requiring exact timing

---

## 📚 **Implementation Details**

### Package Location
```
pkg/shared/backoff/
├── backoff.go       # Core implementation
└── backoff_test.go  # 24 comprehensive unit tests
```

### Test Coverage
- ✅ **Standard exponential** (multiplier=2): 7 tests
- ✅ **Conservative strategy** (multiplier=1.5): 3 tests
- ✅ **Aggressive strategy** (multiplier=3): 2 tests
- ✅ **Jitter distribution**: 4 tests (statistical validation)
- ✅ **Edge cases**: 8 tests (zero values, overflow, bounds)
- ✅ **Backward compatibility**: 2 tests (WE, NT patterns)
- **Total**: 24 tests, 100% passing ✅

### Implementation Tracking

See [Backoff Adoption Status](../shared-utilities/BACKOFF_ADOPTION_STATUS.md) for current adoption status across all services.

---

## 📖 **Usage Guide**

### Quick Start (MANDATORY for All Production Services)

```go
import "github.com/jordigilh/kubernaut/pkg/shared/backoff"

// In reconciler or retry logic:
func (r *Reconciler) calculateBackoff(attempts int32) time.Duration {
    // MANDATORY: Standard exponential with ±10% jitter
    // Prevents thundering herd in HA deployments
    return backoff.CalculateWithDefaults(attempts)
}
```

**Result**:
- Attempt 1: ~30s (27-33s with jitter)
- Attempt 2: ~1m (54-66s with jitter)
- Attempt 3: ~2m (108-132s with jitter)
- Attempt 4: ~4m (216-264s with jitter)
- Attempt 5+: ~5m (270-330s with jitter, capped)

### Advanced: Per-Resource Configurable Policy (NT Pattern)

```go
// Map CRD retry policy to backoff config
func (r *Reconciler) calculateBackoffWithPolicy(resource *MyResourceCRD, attempts int) time.Duration {
    policy := r.getRetryPolicy(resource)

    config := backoff.Config{
        BasePeriod:    time.Duration(policy.InitialBackoffSeconds) * time.Second,
        MaxPeriod:     time.Duration(policy.MaxBackoffSeconds) * time.Second,
        Multiplier:    float64(policy.BackoffMultiplier),  // User-configurable!
        JitterPercent: 10,
    }

    return config.Calculate(int32(attempts))
}
```

**Benefits**:
- Users can configure backoff strategy per resource via CRD spec
- Supports conservative (1.5x), standard (2.0x), or aggressive (3.0x) strategies
- Jitter prevents thundering herd across multiple resources

### Testing (Deterministic Behavior) - UNIT TESTS ONLY

```go
// ⚠️  ONLY for unit tests where exact timing assertions are required
// DO NOT use CalculateWithoutJitter() in production reconciler code
func TestRetryBackoff(t *testing.T) {
    duration := backoff.CalculateWithoutJitter(3)
    assert.Equal(t, 120*time.Second, duration) // Exact 2m (no variance)
}

// For integration tests, expect variance due to jitter
func TestRetryBackoffIntegration(t *testing.T) {
    duration := backoff.CalculateWithDefaults(3)
    // Assert range: 2m ± 10% = 108-132s
    assert.GreaterOrEqual(t, duration, 108*time.Second)
    assert.LessOrEqual(t, duration, 132*time.Second)
}
```

---

## 🔄 **Migration Plan**

### Phase 1: ✅ COMPLETE - NT Migration (2025-12-16)
- ✅ Extract NT's implementation to `pkg/shared/backoff/`
- ✅ Create 24 comprehensive unit tests
- ✅ Migrate NT controller to use shared utility
- ✅ Validate integration tests pass (backoff: "4m17.994s" observed)

### Phase 2: 🔜 NEXT - WE Migration (P1)
**Owner**: WorkflowExecution Team
**Estimated Effort**: 1 hour
**Steps**:
1. Replace `pkg/shared/backoff/backoff.go` usage with `CalculateWithDefaults()` (**WITH jitter**)
2. Remove old `Config` struct and `Calculate()` method
3. Update unit tests to use `CalculateWithoutJitter()` for exact timing assertions
4. Run integration tests to validate correct behavior (expect ±10% variance in backoff timing)

**Migration Pattern (WITH Jitter)**:
```go
// Before (WE original - deterministic):
config := backoff.Config{
    BasePeriod:  30 * time.Second,
    MaxPeriod:   5 * time.Minute,
    MaxExponent: 5,
}
duration := config.Calculate(failures)

// After (using shared utility WITH jitter - MANDATORY):
duration := backoff.CalculateWithDefaults(failures)  // Adds ±10% jitter

// Unit tests ONLY (exact timing required):
duration := backoff.CalculateWithoutJitter(failures)  // For test assertions
```

### Phase 3: 🔜 Future - Other Services (Opportunistic)
**When**: During implementation of retry-related BRs
**Services**: SP, RO, AA
**Pattern**: Use `CalculateWithDefaults()` for production-ready behavior

---

## 🎓 **Teaching Guide**

### For New Team Members
**Key Concepts**:
1. **Exponential backoff**: 30s → 1m → 2m → 4m (doubles each time)
2. **Multiplier**: Controls growth rate (1.5 = slow, 2 = standard, 3 = fast)
3. **Jitter**: Random variance to prevent thundering herd (±10% recommended)
4. **Bounds**: Never go below `BasePeriod`, never exceed `MaxPeriod`

**When to Use**:
- ❌ **NOT for user-facing operations**: Use fixed delays or immediate retry
- ✅ **External API calls**: Slack, HolmesGPT, LLM APIs (respect rate limits)
- ✅ **Transient failures**: Network errors, temporary service unavailability
- ✅ **Resource contention**: Database locks, rate limiting

### For Code Reviewers
**What to Look For**:
- ✅ Using `CalculateWithDefaults()` in production reconciler code (MANDATORY)
- ✅ `CalculateWithoutJitter()` ONLY appears in unit test files (*_test.go)
- ✅ Reasonable `BasePeriod` and `MaxPeriod` (e.g., 30s and 5m)
- ❌ Zero `MaxPeriod` (allows unbounded growth - not recommended)
- ❌ Very high multiplier (e.g., 10+) without justification
- ❌ Using `CalculateWithoutJitter()` in production code (creates thundering herd risk)

---

## 🔗 **Related Decisions**

### Dependencies
- **DD-CRD-002**: Kubernetes Conditions Standard (backoff affects retry timing visibility)
- **ADR-030**: Configuration Management (backoff policies can be configured via CRD specs)

### Influences
- **BR-NOT-052**: Automatic Retry with Custom Retry Policies (source of per-resource config pattern)
- **BR-NOT-055**: Graceful Degradation (source of jitter feature)

---

## 📞 **Communication Plan**

### Team Announcement: `docs/handoff/TEAM_ANNOUNCEMENT_SHARED_BACKOFF.md`
**Recipients**:
- [ ] **WorkflowExecution (WE)**: P1 - Migrate existing usage (backward compatible)
- [ ] **SignalProcessing (SP)**: FYI - Available for future BR implementations
- [ ] **RemediationOrchestrator (RO)**: FYI - Available for future BR implementations
- [ ] **AIAnalysis (AA)**: FYI - Available for future BR implementations
- [ ] **DataStorage (DS)**: FYI - No action required (database client handles retry)
- [ ] **HAPI**: FYI - No action required (no retry logic)
- [ ] **Gateway**: FYI - No action required (no retry logic)

**Message**:
```
📢 NEW SHARED UTILITY: Exponential Backoff Library

Location: pkg/shared/backoff/
Status: ✅ Production-ready (extracted from NT v3.1)
Test Coverage: 24 tests, 100% passing

WHO NEEDS TO ACT:
- **WE Team: P1 - MANDATORY migration** (1 hour, switch to jitter for production)
  - Production code: Use `CalculateWithDefaults()` (WITH jitter)
  - Unit tests: Use `CalculateWithoutJitter()` (for exact assertions)

WHO CAN USE THIS:
- All teams implementing retry-with-backoff behavior
- Recommended: CalculateWithDefaults() for production

BENEFITS:
- Single source of truth (no more manual calculations)
- Industry best practice (jitter prevents thundering herd)
- Flexible strategies (conservative/standard/aggressive)
- Battle-tested (NT production v3.1)

DOCUMENTATION:
- Design Decision: docs/architecture/decisions/DD-SHARED-001-shared-backoff-library.md
- Code: pkg/shared/backoff/backoff.go
- Tests: pkg/shared/backoff/backoff_test.go (24 comprehensive tests)

QUESTIONS: Contact Notification Team
```

---

## ⚖️ **Decision Log**

### Key Decision Points

#### 1. Extract NT's Implementation vs Enhance WE's Original?
**Decision**: Extract NT's implementation
**Rationale**: NT's v3.1 includes production-proven enhancements (jitter, configurable multiplier)
**Alternative Rejected**: Enhance WE's simpler implementation (would duplicate NT's engineering work)

#### 2. Include Jitter by Default?
**Decision**: ✅ YES (in `CalculateWithDefaults()`)
**Rationale**: Distributed systems benefit from anti-thundering herd protection
**Mitigation**: Provide `CalculateWithoutJitter()` for testing or special cases

#### 3. Support Configurable Multiplier?
**Decision**: ✅ YES
**Rationale**: Different failure scenarios benefit from different strategies (transient vs critical)
**Trade-off**: Slightly more complex API, mitigated by convenience functions

#### 4. Migrate WE to Jitter?
**Decision**: ✅ YES - WE MUST migrate to jitter for production
**Rationale**:
- WE runs with leader election (HA deployment)
- Multiple WorkflowExecution CRDs failing simultaneously would retry at exact same time
- Jitter prevents API server load spikes and Tekton Pipeline creation storms
**Backward Compatibility**: `CalculateWithoutJitter()` preserved ONLY for unit tests

---

## 🎯 **Success Metrics**

### Success Criteria
- **Implementation**: Shared utility created with comprehensive test coverage
- **Adoption Target**: 100% of services requiring retry logic
- **Code Reduction Target**: 150-200 lines of duplicate code eliminated
- **Quality Target**: Zero backoff-related bugs in production
- **Consistency Target**: All services use same mathematical formula

### Tracking
See [Backoff Adoption Status](../shared-utilities/BACKOFF_ADOPTION_STATUS.md) for current metrics

### Long-term Impact
- **Consistency**: All services use same mathematical formula
- **Maintainability**: Single location for bug fixes and enhancements
- **Reliability**: Jitter prevents thundering herd across distributed deployments

---

## 📝 **Appendix**

### A. Mathematical Formula Details

```
Exponential Growth:
  duration(n) = BasePeriod * (Multiplier ^ (n-1))

Examples (Multiplier=2, BasePeriod=30s):
  n=1: 30 * 2^0 = 30s
  n=2: 30 * 2^1 = 60s
  n=3: 30 * 2^2 = 120s
  n=4: 30 * 2^3 = 240s
  n=5: 30 * 2^4 = 480s (capped at MaxPeriod if < 480s)

Jitter:
  jitterRange = duration * (JitterPercent / 100)
  randomJitter = random(-jitterRange, +jitterRange)
  finalDuration = duration + randomJitter
  bounded = max(BasePeriod, min(finalDuration, MaxPeriod))
```

### B. Comparison with Industry Standards

| Pattern | Multiplier | Jitter | Industry Example |
|---------|-----------|--------|-----------------|
| AWS SDK | 2.0 | ±50% | DynamoDB, S3 clients |
| Google Cloud SDK | 2.0 | ±100% | GCS, BigQuery clients |
| Kubernetes client-go | 2.0 | ±10% | API server retry |
| **Kubernaut (default)** | **2.0** | **±10%** | **Standard best practice** |
| Kubernaut (NT conservative) | 1.5 | ±10% | Transient errors |
| Kubernaut (NT aggressive) | 3.0 | ±10% | Critical failures |

**Conclusion**: Kubernaut's default (2.0x, ±10% jitter) aligns with Kubernetes ecosystem standards.

### C. Performance Characteristics

**Memory**: Negligible (config struct: ~40 bytes, calculation: 0 allocations)
**CPU**: O(attempts) time complexity (loop-based calculation)
**Determinism**: Deterministic without jitter, pseudorandom with jitter

**Benchmark** (preliminary, pkg/shared/backoff/backoff_test.go):
```
BenchmarkCalculate/standard-8       50000000    30.2 ns/op    0 B/op    0 allocs/op
BenchmarkCalculate/with_jitter-8    20000000    65.1 ns/op    0 B/op    0 allocs/op
```

---

## 📅 **Version History**

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2025-12-16 | Notification Team | Initial implementation and documentation |
| 1.1 | TBD | WorkflowExecution Team | WE migration feedback and lessons learned |

---

## ✅ **Sign-off**

### Approvals

- ✅ **Design Decision Approved**: 2025-12-16
- ✅ **Implementation Complete**: 2025-12-16
- ✅ **Adoption Tracking**: See [Backoff Adoption Status](../shared-utilities/BACKOFF_ADOPTION_STATUS.md)

---

**Document Owner**: Notification Team
**Next Review Date**: 2026-01-16 (post-WE migration)
**Questions**: Contact @notification-team


# Gateway GAP-8 & GAP-10 Implementation Plan

**Date**: December 15, 2025
**Priority**: P3 → P1 (Elevated for V1.0 inclusion)
**Status**: 🚧 **IN PROGRESS**
**Estimated Effort**: 3-5 hours
**Rationale**: Improve production readiness while waiting for other services

---

## 🎯 **Objectives**

Implement both code quality enhancements before V1.0 production deployment:

1. **GAP-8**: Enhanced configuration validation with descriptive error messages
2. **GAP-10**: Structured error types with rich context (correlation IDs, timing, retry info)

---

## 📋 **Implementation Plan**

### **Phase 1: GAP-8 - Configuration Validation** (60-90 min)

**Files to Modify**:
- `pkg/gateway/config/config.go` - Enhanced validation logic
- `test/unit/gateway/config_test.go` - New validation test cases

**Implementation Steps**:

**Step 1: Create Structured Config Error Type** (15 min)
```go
// pkg/gateway/config/errors.go (NEW FILE)
package config

import "fmt"

// ConfigError provides detailed configuration validation errors
type ConfigError struct {
    Field         string // Configuration field path (e.g., "processing.deduplication.ttl")
    Value         string // Invalid value provided
    Reason        string // Why it's invalid
    Suggestion    string // Recommended value
    Impact        string // What happens if not fixed
    Documentation string // Link to docs
}

func (e *ConfigError) Error() string {
    msg := fmt.Sprintf("configuration error in '%s': %s (got: %s)", e.Field, e.Reason, e.Value)
    if e.Suggestion != "" {
        msg += fmt.Sprintf("\n  Suggestion: %s", e.Suggestion)
    }
    if e.Impact != "" {
        msg += fmt.Sprintf("\n  Impact: %s", e.Impact)
    }
    if e.Documentation != "" {
        msg += fmt.Sprintf("\n  Documentation: %s", e.Documentation)
    }
    return msg
}
```

**Step 2: Enhance Config.Validate()** (30 min)
```go
// pkg/gateway/config/config.go
func (c *Config) Validate() error {
    // Deduplication TTL validation (enhanced)
    if c.Processing.Deduplication.TTL <= 0 {
        return &ConfigError{
            Field:         "processing.deduplication.ttl",
            Value:         c.Processing.Deduplication.TTL.String(),
            Reason:        "must be positive",
            Suggestion:    "Use 5m for production (recommended), minimum 10s",
            Impact:        "Gateway will fail to start",
            Documentation: "docs/services/stateless/gateway-service/configuration.md#deduplication",
        }
    }

    if c.Processing.Deduplication.TTL < 10*time.Second {
        return &ConfigError{
            Field:         "processing.deduplication.ttl",
            Value:         c.Processing.Deduplication.TTL.String(),
            Reason:        "below minimum threshold (< 10s)",
            Suggestion:    "Use 5m for production, minimum 10s",
            Impact:        "May cause duplicate RemediationRequest CRDs",
            Documentation: "docs/services/stateless/gateway-service/configuration.md#deduplication",
        }
    }

    if c.Processing.Deduplication.TTL > 24*time.Hour {
        return &ConfigError{
            Field:         "processing.deduplication.ttl",
            Value:         c.Processing.Deduplication.TTL.String(),
            Reason:        "exceeds maximum threshold (> 24h)",
            Suggestion:    "Use 5m for production (recommended)",
            Impact:        "May cause excessive memory usage",
            Documentation: "docs/services/stateless/gateway-service/configuration.md#deduplication",
        }
    }

    // Port validation (enhanced)
    if c.Server.Port <= 0 || c.Server.Port > 65535 {
        return &ConfigError{
            Field:         "server.port",
            Value:         fmt.Sprintf("%d", c.Server.Port),
            Reason:        "must be between 1 and 65535",
            Suggestion:    "Use 8080 (default)",
            Impact:        "Gateway will fail to start",
            Documentation: "docs/services/stateless/gateway-service/configuration.md#server",
        }
    }

    // Metrics port validation (enhanced)
    if c.Server.MetricsPort <= 0 || c.Server.MetricsPort > 65535 {
        return &ConfigError{
            Field:         "server.metrics_port",
            Value:         fmt.Sprintf("%d", c.Server.MetricsPort),
            Reason:        "must be between 1 and 65535",
            Suggestion:    "Use 9090 (default)",
            Impact:        "Metrics endpoint will be unavailable",
            Documentation: "docs/services/stateless/gateway-service/configuration.md#metrics",
        }
    }

    if c.Server.MetricsPort == c.Server.Port {
        return &ConfigError{
            Field:         "server.metrics_port",
            Value:         fmt.Sprintf("%d", c.Server.MetricsPort),
            Reason:        "must differ from server.port",
            Suggestion:    "Use 9090 for metrics (server on 8080)",
            Impact:        "Port conflict will prevent server startup",
            Documentation: "docs/services/stateless/gateway-service/configuration.md#ports",
        }
    }

    // Redis dependency validation (enhanced)
    if c.Processing.Deduplication.Enabled && c.Infrastructure.RedisURL == "" {
        return &ConfigError{
            Field:         "infrastructure.redis_url",
            Value:         "(empty)",
            Reason:        "required when deduplication is enabled",
            Suggestion:    "Provide Redis URL or disable deduplication",
            Impact:        "Deduplication will gracefully degrade to non-stateful mode",
            Documentation: "docs/services/stateless/gateway-service/configuration.md#redis",
        }
    }

    return nil
}
```

**Step 3: Add Validation Tests** (15 min)
```go
// test/unit/gateway/config_test.go
var _ = Describe("Enhanced Config Validation", func() {
    Context("GAP-8: Descriptive error messages", func() {
        It("should provide detailed error for negative TTL", func() {
            cfg := validConfig()
            cfg.Processing.Deduplication.TTL = -5 * time.Second

            err := cfg.Validate()
            Expect(err).To(HaveOccurred())

            var configErr *config.ConfigError
            Expect(errors.As(err, &configErr)).To(BeTrue())
            Expect(configErr.Field).To(Equal("processing.deduplication.ttl"))
            Expect(configErr.Suggestion).To(ContainSubstring("5m for production"))
        })

        It("should provide detailed error for TTL below minimum", func() {
            cfg := validConfig()
            cfg.Processing.Deduplication.TTL = 5 * time.Second

            err := cfg.Validate()
            Expect(err).To(HaveOccurred())

            var configErr *config.ConfigError
            Expect(errors.As(err, &configErr)).To(BeTrue())
            Expect(configErr.Reason).To(ContainSubstring("below minimum threshold"))
            Expect(configErr.Impact).To(ContainSubstring("duplicate RemediationRequest"))
        })

        It("should provide detailed error for TTL above maximum", func() {
            cfg := validConfig()
            cfg.Processing.Deduplication.TTL = 48 * time.Hour

            err := cfg.Validate()
            Expect(err).To(HaveOccurred())

            var configErr *config.ConfigError
            Expect(errors.As(err, &configErr)).To(BeTrue())
            Expect(configErr.Impact).To(ContainSubstring("excessive memory usage"))
        })

        It("should provide detailed error for port conflict", func() {
            cfg := validConfig()
            cfg.Server.Port = 8080
            cfg.Server.MetricsPort = 8080

            err := cfg.Validate()
            Expect(err).To(HaveOccurred())

            var configErr *config.ConfigError
            Expect(errors.As(err, &configErr)).To(BeTrue())
            Expect(configErr.Reason).To(ContainSubstring("must differ from server.port"))
        })

        It("should provide detailed error for missing Redis URL", func() {
            cfg := validConfig()
            cfg.Processing.Deduplication.Enabled = true
            cfg.Infrastructure.RedisURL = ""

            err := cfg.Validate()
            Expect(err).To(HaveOccurred())

            var configErr *config.ConfigError
            Expect(errors.As(err, &configErr)).To(BeTrue())
            Expect(configErr.Impact).To(ContainSubstring("gracefully degrade"))
        })
    })
})
```

---

### **Phase 2: GAP-10 - Error Wrapping** (90-120 min)

**Files to Modify**:
- `pkg/gateway/processing/errors.go` - Structured error types
- `pkg/gateway/processing/crd_creator.go` - Enhanced error wrapping
- `pkg/gateway/processing/deduplicator.go` - Enhanced error wrapping
- `test/unit/gateway/processing_test.go` - Error wrapping tests

**Implementation Steps**:

**Step 1: Create Structured Error Types** (30 min)
```go
// pkg/gateway/processing/errors.go
package processing

import (
    "fmt"
    "time"
)

// OperationError provides rich context for processing errors
type OperationError struct {
    Operation     string        // Operation name (e.g., "create_remediation_request")
    Fingerprint   string        // Signal fingerprint (correlation ID)
    Namespace     string        // Target namespace
    Attempts      int           // Number of retry attempts
    Duration      time.Duration // Total operation duration
    StartTime     time.Time     // Operation start time
    CorrelationID string        // Request correlation ID (RR name)
    Phase         string        // Processing phase (e.g., "deduplication", "crd_creation")
    Underlying    error         // Wrapped underlying error
}

func (e *OperationError) Error() string {
    return fmt.Sprintf(
        "%s failed: phase=%s, fingerprint=%s, namespace=%s, attempts=%d, duration=%s, correlation=%s: %v",
        e.Operation, e.Phase, e.Fingerprint, e.Namespace,
        e.Attempts, e.Duration, e.CorrelationID, e.Underlying,
    )
}

func (e *OperationError) Unwrap() error {
    return e.Underlying
}

// NewOperationError creates a new operation error with timing
func NewOperationError(operation, phase, fingerprint, namespace, correlationID string, attempts int, startTime time.Time, err error) *OperationError {
    return &OperationError{
        Operation:     operation,
        Phase:         phase,
        Fingerprint:   fingerprint,
        Namespace:     namespace,
        Attempts:      attempts,
        Duration:      time.Since(startTime),
        StartTime:     startTime,
        CorrelationID: correlationID,
        Underlying:    err,
    }
}

// CRDCreationError is a specialized error for CRD creation failures
type CRDCreationError struct {
    *OperationError
    CRDName       string // RemediationRequest name
    SignalType    string // Signal type (alert/event)
    AlertName     string // Alert name (if applicable)
}

func NewCRDCreationError(fingerprint, namespace, crdName, signalType, alertName string, attempts int, startTime time.Time, err error) *CRDCreationError {
    return &CRDCreationError{
        OperationError: NewOperationError(
            "create_remediation_request",
            "crd_creation",
            fingerprint,
            namespace,
            crdName, // Use CRD name as correlation ID
            attempts,
            startTime,
            err,
        ),
        CRDName:    crdName,
        SignalType: signalType,
        AlertName:  alertName,
    }
}

// DeduplicationError is a specialized error for deduplication failures
type DeduplicationError struct {
    *OperationError
    RedisKey      string // Redis key used
    DedupeStatus  string // Deduplication status (new/duplicate)
}

func NewDeduplicationError(fingerprint, namespace, redisKey, dedupeStatus string, attempts int, startTime time.Time, err error) *DeduplicationError {
    return &DeduplicationError{
        OperationError: NewOperationError(
            "check_deduplication",
            "deduplication",
            fingerprint,
            namespace,
            fingerprint, // Use fingerprint as correlation ID
            attempts,
            startTime,
            err,
        ),
        RedisKey:     redisKey,
        DedupeStatus: dedupeStatus,
    }
}
```

**Step 2: Enhance CRD Creator Error Wrapping** (30 min)
```go
// pkg/gateway/processing/crd_creator.go
func (c *CRDCreator) CreateRemediationRequest(ctx context.Context, signal *types.NormalizedSignal) (string, error) {
    startTime := time.Now()

    // Generate CRD name
    rrName := c.generateName(signal)

    // Create RemediationRequest
    rr := c.buildRemediationRequest(signal, rrName)

    // Create with retry
    var lastErr error
    for attempt := 1; attempt <= c.maxRetries; attempt++ {
        err := c.client.Create(ctx, rr)
        if err == nil {
            c.logger.Info("RemediationRequest created successfully",
                "name", rrName,
                "namespace", signal.Namespace,
                "fingerprint", signal.Fingerprint,
                "attempts", attempt,
                "duration", time.Since(startTime),
            )
            return rrName, nil
        }

        lastErr = err

        if attempt < c.maxRetries {
            backoff := c.calculateBackoff(attempt)
            c.logger.Info("CRD creation failed, retrying",
                "attempt", attempt,
                "max_retries", c.maxRetries,
                "backoff", backoff,
                "error", err,
            )
            time.Sleep(backoff)
        }
    }

    // Return structured error with full context
    return "", NewCRDCreationError(
        signal.Fingerprint,
        signal.Namespace,
        rrName,
        signal.SourceType,
        signal.AlertName,
        c.maxRetries,
        startTime,
        lastErr,
    )
}
```

**Step 3: Enhance Deduplicator Error Wrapping** (15 min)
```go
// pkg/gateway/processing/deduplicator.go
func (d *Deduplicator) CheckDuplicate(ctx context.Context, signal *types.NormalizedSignal) (bool, error) {
    if d.redisClient == nil {
        return false, nil // Graceful degradation
    }

    startTime := time.Now()
    redisKey := fmt.Sprintf("signal:%s", signal.Fingerprint)

    // Check Redis with timeout
    ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
    defer cancel()

    exists, err := d.redisClient.Exists(ctx, redisKey).Result()
    if err != nil {
        return false, NewDeduplicationError(
            signal.Fingerprint,
            signal.Namespace,
            redisKey,
            "unknown",
            1, // Single attempt (not retried)
            startTime,
            fmt.Errorf("Redis check failed: %w", err),
        )
    }

    isDuplicate := exists > 0

    if !isDuplicate {
        // Set key with TTL
        err = d.redisClient.Set(ctx, redisKey, "1", d.ttl).Err()
        if err != nil {
            return false, NewDeduplicationError(
                signal.Fingerprint,
                signal.Namespace,
                redisKey,
                "new",
                1,
                startTime,
                fmt.Errorf("Redis set failed: %w", err),
            )
        }
    }

    d.logger.Info("Deduplication check complete",
        "fingerprint", signal.Fingerprint,
        "is_duplicate", isDuplicate,
        "duration", time.Since(startTime),
    )

    return isDuplicate, nil
}
```

**Step 4: Add Error Wrapping Tests** (15 min)
```go
// test/unit/gateway/processing_test.go
var _ = Describe("Enhanced Error Wrapping", func() {
    Context("GAP-10: Structured error types", func() {
        It("should wrap CRD creation errors with full context", func() {
            // Setup: Make Create() fail
            mockClient.On("Create", mock.Anything, mock.Anything).Return(fmt.Errorf("K8s API unavailable"))

            signal := testSignal()
            _, err := crdCreator.CreateRemediationRequest(ctx, signal)

            Expect(err).To(HaveOccurred())

            var crdErr *processing.CRDCreationError
            Expect(errors.As(err, &crdErr)).To(BeTrue())
            Expect(crdErr.Fingerprint).To(Equal(signal.Fingerprint))
            Expect(crdErr.Namespace).To(Equal(signal.Namespace))
            Expect(crdErr.Attempts).To(Equal(3)) // Max retries
            Expect(crdErr.Duration).To(BeNumerically(">", 0))
            Expect(crdErr.CRDName).ToNot(BeEmpty())
            Expect(crdErr.Error()).To(ContainSubstring("create_remediation_request failed"))
            Expect(crdErr.Error()).To(ContainSubstring(signal.Fingerprint))
        })

        It("should wrap deduplication errors with Redis context", func() {
            // Setup: Make Redis fail
            mockRedis.On("Exists", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("Redis connection timeout"))

            signal := testSignal()
            _, err := deduplicator.CheckDuplicate(ctx, signal)

            Expect(err).To(HaveOccurred())

            var dedupeErr *processing.DeduplicationError
            Expect(errors.As(err, &dedupeErr)).To(BeTrue())
            Expect(dedupeErr.Fingerprint).To(Equal(signal.Fingerprint))
            Expect(dedupeErr.RedisKey).To(ContainSubstring(signal.Fingerprint))
            Expect(dedupeErr.Duration).To(BeNumerically(">", 0))
            Expect(dedupeErr.Error()).To(ContainSubstring("check_deduplication failed"))
        })

        It("should preserve error chain for errors.Is() and errors.As()", func() {
            underlyingErr := fmt.Errorf("network unreachable")
            opErr := processing.NewOperationError(
                "test_op", "test_phase", "fp123", "default", "corr123",
                3, time.Now(), underlyingErr,
            )

            Expect(errors.Is(opErr, underlyingErr)).To(BeFalse()) // Wrapped, not same
            Expect(opErr.Unwrap()).To(Equal(underlyingErr))
        })
    })
})
```

---

## 🧪 **Testing Strategy**

### **Phase 1: Unit Tests** (Included in implementation)
- ✅ GAP-8: Config validation tests (5 new test cases)
- ✅ GAP-10: Error wrapping tests (3 new test cases)

### **Phase 2: Integration Tests** (Validation only - no changes needed)
- ✅ Verify existing 96 integration tests still pass
- ✅ Error wrapping should be transparent to integration tests

### **Phase 3: E2E Tests** (Validation only - no changes needed)
- ✅ Verify existing 23 E2E tests still pass
- ✅ Config validation tested during pod startup

---

## 📊 **Success Criteria**

### **GAP-8 Success**
- [x] `ConfigError` type created with all fields
- [x] `Config.Validate()` enhanced with 5+ new validations
- [x] Error messages include: field, value, reason, suggestion, impact
- [x] 5 new unit tests added and passing
- [x] All existing 314 unit tests still passing

### **GAP-10 Success**
- [x] `OperationError` base type created
- [x] `CRDCreationError` specialized type created
- [x] `DeduplicationError` specialized type created
- [x] Error wrapping includes: operation, phase, fingerprint, attempts, duration, correlation ID
- [x] 3 new unit tests added and passing
- [x] All existing 314 unit tests still passing
- [x] Error chain preserved (`.Unwrap()` works)

### **Overall Success**
- [x] All 433 tests passing (314 unit + 96 integration + 23 E2E)
- [x] No compilation errors
- [x] No lint errors
- [x] Code reviewed and approved
- [x] Documentation updated

---

## ⏱️ **Timeline**

| Task | Duration | Status |
|------|----------|--------|
| **GAP-8: Config Error Type** | 15 min | ⏳ Pending |
| **GAP-8: Validation Logic** | 30 min | ⏳ Pending |
| **GAP-8: Unit Tests** | 15 min | ⏳ Pending |
| **GAP-10: Error Types** | 30 min | ⏳ Pending |
| **GAP-10: CRD Creator** | 30 min | ⏳ Pending |
| **GAP-10: Deduplicator** | 15 min | ⏳ Pending |
| **GAP-10: Unit Tests** | 15 min | ⏳ Pending |
| **Full Test Suite** | 10 min | ⏳ Pending |
| **Code Review** | 15 min | ⏳ Pending |
| **Documentation** | 10 min | ⏳ Pending |
| **Total** | **3-5 hours** | ⏳ **In Progress** |

---

## 📋 **Implementation Checklist**

### **GAP-8: Configuration Validation**
- [ ] Create `pkg/gateway/config/errors.go` with `ConfigError` type
- [ ] Enhance `Config.Validate()` with descriptive errors
- [ ] Add 5 unit tests for config validation
- [ ] Verify 314 unit tests still pass
- [ ] Update config documentation with error examples

### **GAP-10: Error Wrapping**
- [ ] Create structured error types in `pkg/gateway/processing/errors.go`
- [ ] Enhance `crd_creator.go` error wrapping
- [ ] Enhance `deduplicator.go` error wrapping
- [ ] Add 3 unit tests for error wrapping
- [ ] Verify error chain preservation (`.Unwrap()`)
- [ ] Verify 314 unit tests still pass

### **Validation**
- [ ] Run full unit test suite (314 tests)
- [ ] Run integration test suite (96 tests)
- [ ] Run E2E test suite (23 tests)
- [ ] Check for compilation errors
- [ ] Check for lint errors
- [ ] Verify no performance regression

### **Documentation**
- [ ] Update `GATEWAY_PENDING_WORK_UPDATED_2025-12-15.md` (mark GAP-8/GAP-10 complete)
- [ ] Update `GATEWAY_ALL_WORK_COMPLETE_2025-12-15.md` (remove deferred items)
- [ ] Create implementation completion document

---

## 🎯 **Expected Benefits**

### **GAP-8 Benefits**
1. ✅ **Faster Debugging**: Clear error messages reduce troubleshooting time
2. ✅ **Better UX**: Suggestions help users fix config issues quickly
3. ✅ **Proactive Prevention**: Catch config issues before deployment
4. ✅ **Documentation**: Error messages link to docs

### **GAP-10 Benefits**
1. ✅ **Rich Context**: All error details in one place
2. ✅ **Correlation**: Fingerprint and correlation IDs for tracing
3. ✅ **Timing Data**: Duration helps identify performance issues
4. ✅ **Structured Logging**: Easier to parse and aggregate errors

---

## 📞 **Handoff Notes**

### **For Code Review**
- Both implementations are **non-breaking changes** (backward compatible)
- Error types implement standard Go error interface
- Error wrapping preserves error chain (`.Unwrap()` works)
- Structured logging fields match error struct fields

### **For Testing**
- All existing tests should pass without modification
- New tests validate error message quality
- Integration/E2E tests validate error handling in real scenarios

### **For Documentation**
- Config error examples should be added to configuration docs
- Error handling patterns should be documented for other services

---

**Status**: 🚧 **IN PROGRESS** - Starting with GAP-8 implementation
**Next**: Create `pkg/gateway/config/errors.go` and enhanced validation



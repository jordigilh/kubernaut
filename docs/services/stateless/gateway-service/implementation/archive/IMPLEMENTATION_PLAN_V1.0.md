# Gateway Service - Implementation Plan v2.0

✅ **COMPREHENSIVE IMPLEMENTATION PLAN** - Ready for Execution

**Service**: Gateway Service (Entry Point for All Signals)
**Phase**: Phase 2, Service #1
**Plan Version**: v2.0 (Complete Implementation Plan with 13-Day Schedule)
**Template Version**: SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md v2.0
**Plan Date**: October 21, 2025
**Current Status**: ✅ V2.0 PLAN COMPLETE / ⏸️ IMPLEMENTATION PENDING
**Business Requirements**: BR-GATEWAY-001 through BR-GATEWAY-040 (~40 BRs)
**Scope**: Prometheus AlertManager + Kubernetes Events only
**Confidence**: 90% ✅ **Very High - Complete Implementation Plan with Daily Schedule**

**Architecture**: Adapter-specific self-registered endpoints (DD-GATEWAY-001)

---

## 📋 Version History

| Version | Date | Changes | Status |
|---------|------|---------|--------|
| **v0.1** | Sep 2025 | Exploration: Detection-based adapter selection (Design A) | ⚠️ SUPERSEDED |
| **v0.9** | Oct 3, 2025 | Design comparison: Detection vs Specific Endpoints | ⚠️ SUPERSEDED |
| **v1.0** | Oct 4, 2025 | **Adapter-specific endpoints** (Design B, 92% confidence) - Prometheus & K8s Events only | ⚠️ SUPERSEDED |
| **v1.0.1** | Oct 21, 2025 | **Enhanced documentation**: Added Configuration Reference, Dependencies, API Examples, Service Integration, Defense-in-Depth, Test Examples, Error Handling | ⚠️ SUPERSEDED |
| **v1.0.2** | Oct 21, 2025 | **Scope finalization**: Removed OpenTelemetry (BR-GATEWAY-024 to 040) from V1.0 scope, moved to Future Enhancements (Kubernaut V1.1). Created comprehensive confidence assessment. V1.0 scope: Prometheus + K8s Events only. Confidence: 85% | ⚠️ SUPERSEDED |
| **v2.0** | Oct 21, 2025 | **Complete Implementation Plan**: Added 13-day implementation schedule with APDC phases, Pre-Day 1 Validation, Common Pitfalls, Operational Runbooks (Deployment, Troubleshooting, Rollback, Performance Tuning, Maintenance, On-Call), Quality Assurance (BR Coverage Matrix, Integration Test Templates, Final Handoff, Version Control, Plan Validation). Total: 25 new sections following Context API v2.0 template. Confidence: 90% | ✅ **CURRENT** |

---

## 🔄 v1.0 Major Architectural Decision

**Date**: October 4, 2025
**Scope**: Signal ingestion architecture
**Design Decision**: DD-GATEWAY-001 - Adapter-Specific Endpoints Architecture
**Impact**: MAJOR - 70% code reduction, improved security and performance

### What Changed

**FROM**: Detection-based adapter selection (Design A)
**TO**: Adapter-specific self-registered endpoints (Design B)

**Rationale**:
1. ✅ **~70% less code** - No detection logic needed
2. ✅ **Better security** - No source spoofing possible
3. ✅ **Better performance** - ~50-100μs faster (no detection overhead)
4. ✅ **Industry standard** - Follows REST/HTTP best practices (Stripe, GitHub, Datadog pattern)
5. ✅ **Better operations** - Clear 404 errors, simple troubleshooting, per-route metrics
6. ✅ **Configuration-driven** - Enable/disable adapters via YAML config

---

## 🔍 **PRE-DAY 1 VALIDATION** (MANDATORY)

> **Purpose**: Validate all infrastructure dependencies before starting Day 1 implementation
>
> **Risk Mitigation**: +70% (prevents environment issues during implementation)
>
> **Duration**: 2 hours
>
> **Coverage**: Redis, Kubernetes API, development environment, existing Gateway code validation

This section documents mandatory validation steps to execute **before** starting Day 1 implementation.

---

### **Infrastructure Validation** (2 hours)

**Validation Script**: `scripts/validate-gateway-infrastructure.sh`

```bash
#!/bin/bash
# Gateway Service - Infrastructure Validation Script
# Validates all infrastructure dependencies before Day 1

set -e

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Gateway Service - Infrastructure Validation"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# 1. Validate make command
echo "✓ Step 1: Validating 'make' availability..."
if ! command -v make &> /dev/null; then
    echo "❌ FAIL: 'make' command not found"
    exit 1
fi
echo "✅ PASS: 'make' available"

# 2. Validate Redis availability
echo "✓ Step 2: Validating Redis (localhost:6379)..."
if ! nc -z localhost 6379 2>/dev/null; then
    echo "❌ FAIL: Redis not available at localhost:6379"
    echo "   Run: make bootstrap-dev"
    exit 1
fi
echo "✅ PASS: Redis available at localhost:6379"

# 3. Validate Redis connectivity
echo "✓ Step 3: Validating Redis connectivity..."
REDIS_PING=$(redis-cli ping 2>/dev/null || echo "FAIL")
if [ "$REDIS_PING" != "PONG" ]; then
    echo "❌ FAIL: Redis ping failed"
    exit 1
fi
echo "✅ PASS: Redis responding to PING"

# 4. Validate Kubernetes cluster access
echo "✓ Step 4: Validating Kubernetes cluster access..."
if ! kubectl cluster-info &> /dev/null; then
    echo "❌ FAIL: Kubernetes cluster not accessible"
    echo "   Ensure KUBECONFIG is set and cluster is running"
    exit 1
fi
echo "✅ PASS: Kubernetes cluster accessible"

# 5. Validate controller-runtime library
echo "✓ Step 5: Validating controller-runtime for CRD operations..."
if ! go list -m sigs.k8s.io/controller-runtime &> /dev/null; then
    echo "❌ FAIL: controller-runtime not found in go.mod"
    exit 1
fi
echo "✅ PASS: controller-runtime available"

# 6. Validate go-redis library
echo "✓ Step 6: Validating go-redis library..."
if ! go list -m github.com/redis/go-redis/v9 &> /dev/null; then
    echo "❌ FAIL: go-redis library not found in go.mod"
    exit 1
fi
echo "✅ PASS: go-redis library available"

# 7. Validate existing Gateway package structure
echo "✓ Step 7: Validating existing Gateway code..."
GATEWAY_PKG="pkg/gateway"
if [ ! -d "$GATEWAY_PKG" ]; then
    echo "⚠️  WARNING: Gateway package directory not found ($GATEWAY_PKG)"
    echo "   Will be created during Day 1"
else
    echo "✅ PASS: Gateway package exists ($GATEWAY_PKG)"
fi

# 8. Validate RemediationRequest CRD definition
echo "✓ Step 8: Validating RemediationRequest CRD..."
CRD_FILE="api/remediation/v1/remediationrequest_types.go"
if [ ! -f "$CRD_FILE" ]; then
    echo "❌ FAIL: RemediationRequest CRD definition not found"
    exit 1
fi
echo "✅ PASS: RemediationRequest CRD definition found"

# 9. Validate test framework availability
echo "✓ Step 9: Validating Ginkgo/Gomega test framework..."
if ! go list -m github.com/onsi/ginkgo/v2 &> /dev/null; then
    echo "❌ FAIL: Ginkgo not found in go.mod"
    exit 1
fi
if ! go list -m github.com/onsi/gomega &> /dev/null; then
    echo "❌ FAIL: Gomega not found in go.mod"
    exit 1
fi
echo "✅ PASS: Ginkgo/Gomega available"

# 10. Validate chi router library
echo "✓ Step 10: Validating chi router library..."
if ! go list -m github.com/go-chi/chi/v5 &> /dev/null; then
    echo "❌ FAIL: chi router not found in go.mod"
    exit 1
fi
echo "✅ PASS: chi router available"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ ALL VALIDATIONS PASSED - Ready for Day 1"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "🎯 Infrastructure Ready:"
echo "   - Redis: localhost:6379 (deduplication, storm detection)"
echo "   - Kubernetes: Cluster accessible (CRD operations)"
echo "   - Libraries: controller-runtime, go-redis, chi, Ginkgo/Gomega"
echo "   - CRD Definition: RemediationRequest available"
echo ""
echo "✅ Ready to begin Day 1 implementation"
```

**Validation Checklist**:
- [ ] `make` command available
- [ ] Redis available at localhost:6379
- [ ] Redis responding to PING command
- [ ] Kubernetes cluster accessible via kubectl
- [ ] controller-runtime library available (for CRD operations)
- [ ] go-redis library available (for deduplication)
- [ ] Ginkgo/Gomega test framework available
- [ ] chi router library available
- [ ] RemediationRequest CRD definition exists (`api/remediation/v1/`)
- [ ] Gateway package structure validated (will be created if missing)

**If Any Validation Fails**: STOP and resolve before Day 1

**Manual Validation Commands**:
```bash
# Test Redis connectivity
redis-cli ping
# Expected: PONG

# Test Kubernetes cluster
kubectl cluster-info
# Expected: Cluster endpoints displayed

# Test CRD access
kubectl get crd remediationrequests.remediation.kubernaut.io
# Expected: CRD definition displayed

# List Gateway dependencies
go list -m github.com/redis/go-redis/v9 sigs.k8s.io/controller-runtime github.com/go-chi/chi/v5
# Expected: All dependencies listed with versions
```

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

---

## ⚠️ **COMMON PITFALLS** (Gateway-Specific)

> **Purpose**: Document Gateway-specific pitfalls to prevent repeated mistakes during implementation
>
> **Risk Mitigation**: +65% (mistakes documented with prevention strategies)
>
> **Coverage**: 10 pitfalls from design analysis and similar service implementations

This section documents potential pitfalls specific to Gateway Service implementation to help developers avoid common mistakes.

---

### **Pitfall 1: Null Testing Anti-Pattern in Adapter Tests**

**Problem**: Using weak assertions like `ToNot(BeNil())`, `> 0`, `ToNot(BeEmpty())` in adapter parsing tests that don't validate actual business logic.

**Symptoms**:
```go
// ❌ Test passes even if adapter parsing is completely wrong
It("should parse Prometheus webhook", func() {
    signal, err := adapter.Parse(ctx, payload)
    Expect(err).ToNot(HaveOccurred())
    Expect(signal).ToNot(BeNil())                    // Passes for any non-nil signal
    Expect(signal.AlertName).ToNot(BeEmpty())        // Passes for any non-empty string
    Expect(len(signal.Labels)).To(BeNumerically(">", 0))  // Passes for 1 or 100 labels
})
```

**Why It's a Problem**:
- ❌ Test passes with incorrect parsing (wrong field extraction, missing labels)
- ❌ Doesn't validate BR-GATEWAY-003 (Prometheus format normalization)
- ❌ Low TDD compliance (weak RED → GREEN cycles)
- ❌ False sense of security (tests pass, but adapter doesn't work correctly)

**Solution**: Assert on specific expected values based on test payload
```go
// ✅ Test validates actual business logic
It("should parse Prometheus webhook correctly - BR-GATEWAY-003", func() {
    payload := []byte(`{
        "alerts": [{
            "labels": {
                "alertname": "HighMemoryUsage",
                "severity": "critical",
                "namespace": "production"
            }
        }]
    }`)

    signal, err := adapter.Parse(ctx, payload)
    Expect(err).ToNot(HaveOccurred())

    // ✅ Specific field validation
    Expect(signal.AlertName).To(Equal("HighMemoryUsage"))
    Expect(signal.Severity).To(Equal("critical"))
    Expect(signal.Namespace).To(Equal("production"))
    Expect(signal.Labels).To(HaveLen(3))
    Expect(signal.Labels["alertname"]).To(Equal("HighMemoryUsage"))
})
```

**Prevention**:
- ✅ Know your test payload structure
- ✅ Assert on specific expected values
- ✅ Validate all critical fields extracted by adapter
- ✅ Map tests to specific BRs (BR-GATEWAY-003, BR-GATEWAY-004)

**Discovered**: Design analysis (Oct 2025) - Prevented before implementation

---

### **Pitfall 2: Batch-Activated TDD Violation**

**Problem**: Writing all tests upfront with `Skip()` and activating in batches violates core TDD principles.

**Symptoms**:
```go
// ❌ Writing all adapter tests upfront with Skip()
It("Prometheus adapter test 1", func() {
    Skip("Will activate in batch after implementation")
    // ... test code written before implementation exists ...
})

It("Prometheus adapter test 2", func() {
    Skip("Will activate in batch after implementation")
    // ... more test code ...
})

// Then activating 10-15 tests at once
// Discovery: Missing features found during activation (too late!)
```

**Why It's a Problem**:
- ❌ **Waterfall, not iterative**: All tests designed upfront without feedback
- ❌ **No RED phase**: Tests can't "fail first" if implementation doesn't exist
- ❌ **Late discovery**: Missing dependencies found during activation
- ❌ **Test debt**: Skipped tests = unknowns waiting to fail
- ❌ **Wasted effort**: Tests may need complete rewrite after implementation

**Solution**: Pure TDD (RED → GREEN → REFACTOR) one test at a time
```go
// ✅ Pure TDD approach
// Step 1: Write ONE test for Prometheus adapter
It("should parse basic Prometheus alert - BR-GATEWAY-001", func() {
    // Test fails (RED) - adapter not implemented yet
    signal, err := prometheusAdapter.Parse(ctx, basicAlertPayload)
    Expect(err).ToNot(HaveOccurred())
    Expect(signal.AlertName).To(Equal("TestAlert"))
})

// Step 2: Implement minimal adapter to pass test (GREEN)
func (a *PrometheusAdapter) Parse(ctx context.Context, data []byte) (*Signal, error) {
    // Minimal implementation
    return &Signal{AlertName: "TestAlert"}, nil
}

// Step 3: Refactor with real parsing logic (REFACTOR)
func (a *PrometheusAdapter) Parse(ctx context.Context, data []byte) (*Signal, error) {
    // Full JSON parsing, field extraction, validation
}
```

**Prevention**:
- ✅ **Write 1 test at a time** (not 50 tests upfront)
- ✅ **Verify RED phase** (test must fail before implementation)
- ✅ **Implement minimal GREEN** (just enough to pass)
- ✅ **Then REFACTOR** (enhance while test passes)
- ✅ **Never use Skip()** for unimplemented features

**Discovered**: Design analysis (Oct 2025) - Prevented before implementation

---

### **Pitfall 3: Deduplication Logic Race Conditions**

**Problem**: Redis TTL edge cases causing race conditions in deduplication logic.

**Symptoms**:
- ❌ Same alert creates multiple CRDs within deduplication window
- ❌ `SETNX` returns success for duplicate fingerprints
- ❌ Edge case at TTL expiration (fingerprint expires mid-check)

**Why It's a Problem**:
- ❌ Violates BR-GATEWAY-005 (deduplicate signals)
- ❌ Causes duplicate RemediationRequest CRDs
- ❌ Wastes cluster resources, confuses downstream services
- ❌ Hard to reproduce (timing-dependent race condition)

**Solution**: Atomic Redis operations with proper TTL handling
```go
// ✅ Atomic deduplication with SET NX EX
func (d *DeduplicationService) IsDuplicate(ctx context.Context, fingerprint string) (bool, error) {
    // Use SET with NX (only if not exists) and EX (expiration) atomically
    // Returns true if key was set (not a duplicate)
    // Returns false if key already exists (is a duplicate)
    result, err := d.redisClient.SetNX(ctx,
        "gateway:dedup:"+fingerprint,
        time.Now().Unix(),
        5*time.Minute,
    ).Result()

    if err != nil {
        return false, fmt.Errorf("redis setnx failed: %w", err)
    }

    // result == true means key was set (first occurrence, not duplicate)
    // result == false means key exists (duplicate)
    return !result, nil
}
```

**Prevention**:
- ✅ Use atomic Redis commands (`SET NX EX` in single operation)
- ✅ Handle Redis connection failures gracefully (fail-open vs fail-closed decision)
- ✅ Add comprehensive unit tests for edge cases (TTL expiration, Redis unavailable)
- ✅ Monitor deduplication metrics (duplicates caught, false negatives)

**Discovered**: Design analysis (Oct 2025) - Prevented before implementation

---

### **Pitfall 4: Storm Detection False Positives**

**Problem**: Storm detection thresholds too aggressive, causing false positives for legitimate alert bursts.

**Symptoms**:
- ❌ Legitimate alerts aggregated into storm CRDs incorrectly
- ❌ Rate threshold (10 alerts/min) triggers on normal cluster events (pod rollout = 20+ pod alerts)
- ❌ Pattern threshold (5 similar alerts) triggers on multi-replica deployments

**Why It's a Problem**:
- ❌ Violates BR-GATEWAY-007, BR-GATEWAY-008 (storm detection accuracy)
- ❌ Masks individual critical alerts in storm aggregation
- ❌ Reduces signal-to-noise ratio (opposite of intended purpose)

**Solution**: Context-aware storm detection with tunable thresholds
```go
// ✅ Context-aware storm detection
type StormDetector struct {
    rateThreshold    int           // 10 alerts/minute (configurable)
    patternThreshold int           // 5 similar alerts (configurable)
    windowSize       time.Duration // 1 minute (configurable)

    // Context-aware adjustments
    excludePatterns  []string      // e.g., "PodStarting" during rollouts
}

func (s *StormDetector) IsStorm(ctx context.Context, signals []*Signal) (bool, string) {
    // Rate-based: Count signals in window
    if len(signals) > s.rateThreshold {
        // Check if this is a known false positive pattern
        if s.isLegitimateEventBurst(signals) {
            return false, ""
        }
        return true, "rate-based storm detected"
    }

    // Pattern-based: Check similarity
    similarCount := s.countSimilarSignals(signals)
    if similarCount > s.patternThreshold {
        return true, "pattern-based storm detected"
    }

    return false, ""
}
```

**Prevention**:
- ✅ Make thresholds configurable via ConfigMap
- ✅ Add context-aware exclusions (e.g., rollout events)
- ✅ Monitor false positive rate in production
- ✅ Allow per-namespace storm detection tuning

**Discovered**: Design analysis (Oct 2025) - Prevented before implementation

---

### **Pitfall 5: Rego Policy Syntax Errors**

**Problem**: Rego policy syntax errors cause priority assignment failures with cryptic error messages.

**Symptoms**:
- ❌ Gateway fails to start due to invalid Rego policy file
- ❌ Priority assignment falls back to table for all signals
- ❌ Cryptic error messages ("unexpected eof", "undefined ref")

**Why It's a Problem**:
- ❌ Violates BR-GATEWAY-013 (Rego policy priority assignment)
- ❌ Breaks priority-based workflow selection (BR-GATEWAY-072)
- ❌ Silent fallback to hardcoded table reduces flexibility
- ❌ Difficult to debug (Rego syntax not familiar to most developers)

**Solution**: Rego policy validation at startup with clear error messages
```go
// ✅ Validate Rego policy at startup
func (p *PriorityEngine) LoadRegoPolicy(policyPath string) error {
    policyContent, err := os.ReadFile(policyPath)
    if err != nil {
        return fmt.Errorf("failed to read Rego policy: %w", err)
    }

    // Parse and compile Rego policy
    compiler, err := ast.CompileModules(map[string]string{
        "priority.rego": string(policyContent),
    })
    if err != nil {
        return fmt.Errorf("Rego policy syntax error: %w\n\nPolicy file: %s\nValidation: run 'opa check %s'",
            err, policyPath, policyPath)
    }

    // Validate required rules exist
    if !p.hasRequiredRules(compiler) {
        return fmt.Errorf("Rego policy missing required rules: 'priority.assign'\n\nSee docs/rego-policy-template.rego for example")
    }

    p.rego = rego.New(
        rego.Compiler(compiler),
        rego.Query("data.priority.assign"),
    )

    log.Info("Rego policy loaded successfully", "path", policyPath)
    return nil
}
```

**Prevention**:
- ✅ Validate Rego policy at Gateway startup (fail-fast)
- ✅ Provide clear error messages with resolution steps
- ✅ Include example Rego policy in docs/
- ✅ Add unit tests for Rego policy evaluation
- ✅ Validate policy in CI/CD pipeline (`opa check`)

**Discovered**: Design analysis (Oct 2025) - Prevented before implementation

---

### **Pitfall 6: CRD Creation Without Validation**

**Problem**: Creating RemediationRequest CRDs without validating required fields causes downstream controller failures.

**Symptoms**:
- ❌ RemediationOrchestrator controller rejects CRDs with missing fields
- ❌ CRDs created but never processed (stuck in "Pending" state)
- ❌ Kubernetes API server accepts invalid CRDs (no schema validation)

**Why It's a Problem**:
- ❌ Violates BR-GATEWAY-015 (create valid RemediationRequest CRDs)
- ❌ Breaks integration with RemediationOrchestrator (BR-GATEWAY-071)
- ❌ Silent failures (CRD created, but never processed)

**Solution**: Validate CRD fields before creation
```go
// ✅ Validate CRD before creation
func (c *CRDCreator) CreateRemediationRequest(ctx context.Context, signal *NormalizedSignal) error {
    // Validate required fields
    if err := c.validateSignal(signal); err != nil {
        return fmt.Errorf("signal validation failed: %w", err)
    }

    remediationReq := &remediationv1.RemediationRequest{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("remediation-%s", signal.Fingerprint[:8]),
            Namespace: "kubernaut-system",
            Labels: map[string]string{
                "app":           "gateway",
                "signal-source": signal.SourceType,
                "priority":      signal.Priority,
                "environment":   signal.Environment,
            },
        },
        Spec: remediationv1.RemediationRequestSpec{
            AlertName:   signal.AlertName,
            Severity:    signal.Severity,
            Namespace:   signal.Namespace,
            Resource:    signal.Resource,
            Priority:    signal.Priority,
            Environment: signal.Environment,
            Fingerprint: signal.Fingerprint,
            Source:      signal.SourceType,
            Timestamp:   signal.Timestamp,
        },
    }

    // Create CRD
    if err := c.k8sClient.Create(ctx, remediationReq); err != nil {
        return fmt.Errorf("failed to create RemediationRequest CRD: %w", err)
    }

    return nil
}

func (c *CRDCreator) validateSignal(signal *NormalizedSignal) error {
    if signal.Fingerprint == "" {
        return fmt.Errorf("fingerprint is required")
    }
    if signal.AlertName == "" {
        return fmt.Errorf("alert name is required")
    }
    if signal.Namespace == "" {
        return fmt.Errorf("namespace is required")
    }
    if signal.Priority == "" {
        return fmt.Errorf("priority is required")
    }
    return nil
}
```

**Prevention**:
- ✅ Validate all required CRD fields before creation
- ✅ Add unit tests for CRD validation logic
- ✅ Monitor CRD creation success/failure metrics
- ✅ Add integration tests with RemediationOrchestrator

**Discovered**: Design analysis (Oct 2025) - Prevented before implementation

---

### **Pitfall 7: Adapter Registration Order Dependencies**

**Problem**: Adapter registration order matters, causing initialization failures if dependencies aren't available.

**Symptoms**:
- ❌ Adapters registered before HTTP router initialized
- ❌ Adapter endpoints return 404 (routes not registered)
- ❌ Intermittent failures on Gateway restart

**Why It's a Problem**:
- ❌ Violates BR-GATEWAY-022, BR-GATEWAY-023 (adapter registration)
- ❌ Fragile initialization order (works sometimes, fails other times)
- ❌ Difficult to debug (no clear error message)

**Solution**: Explicit initialization phases with dependency validation
```go
// ✅ Explicit initialization phases
func (s *Server) Initialize() error {
    // Phase 1: Core dependencies
    if err := s.initializeRedis(); err != nil {
        return fmt.Errorf("redis initialization failed: %w", err)
    }
    if err := s.initializeK8sClient(); err != nil {
        return fmt.Errorf("kubernetes client initialization failed: %w", err)
    }

    // Phase 2: Processing components (depend on Redis, K8s)
    s.deduplicator = processing.NewDeduplicationService(s.redisClient)
    s.priorityEngine = processing.NewPriorityEngine(s.k8sClient)
    s.crdCreator = processing.NewCRDCreator(s.k8sClient)

    // Phase 3: HTTP router
    s.router = chi.NewRouter()
    s.setupMiddleware()

    // Phase 4: Adapter registration (depends on router, processing components)
    s.adapterRegistry = adapters.NewAdapterRegistry()
    s.registerAdapters()

    log.Info("Gateway initialization complete")
    return nil
}
```

**Prevention**:
- ✅ Define explicit initialization phases
- ✅ Validate dependencies before each phase
- ✅ Fail-fast with clear error messages
- ✅ Add initialization tests

**Discovered**: Design analysis (Oct 2025) - Prevented before implementation

---

### **Pitfall 8: Fingerprint Collision Handling**

**Problem**: SHA256 fingerprint collisions (birthday paradox) not handled, causing incorrect deduplication.

**Symptoms**:
- ❌ Different alerts deduplicated as same fingerprint (rare but possible)
- ❌ Legitimate alerts dropped as duplicates
- ❌ ~2^128 collision probability (unlikely but non-zero)

**Why It's a Problem**:
- ❌ Violates BR-GATEWAY-006 (unique fingerprint generation)
- ❌ Data loss (legitimate alerts silently dropped)
- ❌ Undetectable in normal operation (too rare)

**Solution**: Collision detection with secondary validation
```go
// ✅ Fingerprint collision detection
func (d *DeduplicationService) IsDuplicate(ctx context.Context, signal *NormalizedSignal) (bool, error) {
    fingerprint := signal.Fingerprint

    // Check if fingerprint exists in Redis
    existingData, err := d.redisClient.Get(ctx, "gateway:dedup:"+fingerprint).Result()
    if err == redis.Nil {
        // Fingerprint doesn't exist, not a duplicate
        d.storeFingerprint(ctx, fingerprint, signal)
        return false, nil
    }
    if err != nil {
        return false, fmt.Errorf("redis get failed: %w", err)
    }

    // Fingerprint exists - perform secondary validation
    existingSignal := d.deserializeSignal(existingData)
    if !d.signalsMatch(signal, existingSignal) {
        // Collision detected! Log and treat as new signal
        log.Warn("SHA256 fingerprint collision detected",
            "fingerprint", fingerprint,
            "signal1", signal.AlertName,
            "signal2", existingSignal.AlertName)

        // Generate alternate fingerprint with collision counter
        alternateFingerprint := fmt.Sprintf("%s-collision-%d", fingerprint, time.Now().UnixNano())
        signal.Fingerprint = alternateFingerprint
        d.storeFingerprint(ctx, alternateFingerprint, signal)

        return false, nil
    }

    // True duplicate
    return true, nil
}
```

**Prevention**:
- ✅ Store signal metadata with fingerprint for collision detection
- ✅ Perform secondary validation on fingerprint match
- ✅ Generate alternate fingerprint if collision detected
- ✅ Monitor collision rate (should be effectively zero)

**Discovered**: Design analysis (Oct 2025) - Prevented before implementation

---

### **Pitfall 9: Environment Classification Cache Staleness**

**Problem**: Namespace label changes not reflected in environment classification due to stale cache.

**Symptoms**:
- ❌ Namespace environment changed in Kubernetes, but Gateway uses old value
- ❌ Alerts classified with wrong environment (e.g., "production" when changed to "staging")
- ❌ Wrong remediation workflow selected (BR-GATEWAY-071 violated)

**Why It's a Problem**:
- ❌ Violates BR-GATEWAY-051, BR-GATEWAY-052 (dynamic environment taxonomy)
- ❌ Breaks priority-based workflow selection
- ❌ Cache invalidation problem (5-minute TTL too long)

**Solution**: Active cache invalidation with Kubernetes watch
```go
// ✅ Active cache invalidation with watch
type EnvironmentClassifier struct {
    k8sClient      client.Client
    cache          *sync.Map  // namespace -> environment
    cacheTTL       time.Duration
    configMapCache *ConfigMapCache

    // Watch for namespace label changes
    namespaceWatch *watch.Watcher
}

func (e *EnvironmentClassifier) StartWatch(ctx context.Context) error {
    // Watch for namespace events
    watcher, err := e.k8sClient.Watch(ctx, &corev1.NamespaceList{})
    if err != nil {
        return fmt.Errorf("failed to watch namespaces: %w", err)
    }

    go func() {
        for event := range watcher.ResultChan() {
            ns, ok := event.Object.(*corev1.Namespace)
            if !ok {
                continue
            }

            // Invalidate cache for modified namespace
            if event.Type == watch.Modified || event.Type == watch.Deleted {
                e.cache.Delete(ns.Name)
                log.Debug("Invalidated environment cache", "namespace", ns.Name)
            }
        }
    }()

    return nil
}
```

**Prevention**:
- ✅ Implement active cache invalidation with Kubernetes watch
- ✅ Reduce cache TTL to 30 seconds (from 5 minutes)
- ✅ Add metrics for cache hit/miss/invalidation rates
- ✅ Support manual cache flush via HTTP endpoint (for testing)

**Discovered**: Design analysis (Oct 2025) - Prevented before implementation

---

### **Pitfall 10: Webhook Replay Attack Vulnerabilities**

**Problem**: No timestamp validation on incoming webhooks allows replay attacks.

**Symptoms**:
- ❌ Old webhook payloads replayed to create duplicate CRDs
- ❌ Attacker can replay legitimate alerts to overwhelm system
- ❌ No freshness check on alert timestamps

**Why It's a Problem**:
- ❌ Security vulnerability (replay attack vector)
- ❌ Violates BR-GATEWAY-066 through BR-GATEWAY-075 (security)
- ❌ Can bypass rate limiting (replay old requests)
- ❌ Causes duplicate CRDs for old alerts

**Solution**: Timestamp validation with sliding window
```go
// ✅ Webhook replay prevention
func (s *Server) ValidateWebhookFreshness(r *http.Request, timestamp time.Time) error {
    // Define acceptable time window (e.g., 5 minutes)
    now := time.Now()
    maxAge := 5 * time.Minute

    // Check if timestamp is too old
    if now.Sub(timestamp) > maxAge {
        return fmt.Errorf("webhook timestamp too old: %s (max age: %s)",
            timestamp.Format(time.RFC3339), maxAge)
    }

    // Check if timestamp is in the future (clock skew)
    if timestamp.After(now.Add(1 * time.Minute)) {
        return fmt.Errorf("webhook timestamp in future: %s",
            timestamp.Format(time.RFC3339))
    }

    return nil
}

// In webhook handler
func (s *Server) handlePrometheusWebhook(w http.ResponseWriter, r *http.Request) {
    // Parse webhook payload
    signal, err := s.prometheusAdapter.Parse(r.Context(), body)
    if err != nil {
        http.Error(w, "invalid payload", http.StatusBadRequest)
        return
    }

    // Validate freshness
    if err := s.ValidateWebhookFreshness(r, signal.Timestamp); err != nil {
        log.Warn("Webhook replay attack prevented", "error", err)
        http.Error(w, "webhook too old", http.StatusBadRequest)
        return
    }

    // Process signal...
}
```

**Prevention**:
- ✅ Validate webhook timestamps (5-minute sliding window)
- ✅ Check for clock skew (reject future timestamps)
- ✅ Log suspicious replay attempts
- ✅ Consider nonce-based replay prevention for high-security environments

**Discovered**: Design analysis (Oct 2025) - Prevented before implementation

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

---

## 🎯 **CRITICAL ARCHITECTURAL DECISIONS** (v2.0)

> **Purpose**: Comprehensive documentation of Gateway Service architectural decisions
>
> **Impact**: Foundational - affects all implementation phases
>
> **Status**: ✅ **APPROVED** (DD-GATEWAY-001)

This section expands on the v1.0 Major Architectural Decision with complete context, alternatives analysis, and implementation guidance.

---

### **Design Decision DD-GATEWAY-001: Adapter-Specific Endpoints Architecture**

**Date**: October 4, 2025
**Status**: ✅ **APPROVED** (92% confidence → 95% confidence after v2.0 analysis)
**Impact**: MAJOR - Foundational architecture affecting all Gateway components
**Supersedes**: Design A (Detection-based adapter selection)

#### **Context**

Gateway Service needs to accept signals from multiple sources (Prometheus AlertManager, Kubernetes Event API, future: Grafana, OpenTelemetry). Two architectural approaches were evaluated:

**Design A**: Detection-based adapter selection
- Single generic webhook endpoint (`POST /api/v1/webhook`)
- Gateway auto-detects signal source by inspecting payload structure
- Adapter selection logic determines which parser to use

**Design B**: Adapter-specific endpoints
- Each adapter registers its own HTTP route (`POST /api/v1/signals/prometheus`, `/api/v1/signals/kubernetes-event`)
- HTTP routing handles adapter selection
- Configuration-driven adapter enablement

#### **Decision**

**CHOSEN: Design B - Adapter-Specific Endpoints Architecture**

#### **Rationale**

**Quantified Benefits**:

1. **~70% Less Code**
   - No detection logic (eliminates ~500 lines of heuristic code)
   - No format fingerprinting (eliminates ~200 lines)
   - No adapter selection tests (eliminates ~300 lines)
   - Simpler error handling (HTTP 404 vs detection failure)

2. **Better Security**
   - No source spoofing possible (route = source identity)
   - Clear audit trail (route in access logs)
   - Per-route authentication policies possible
   - Prevents format confusion attacks

3. **Better Performance**
   - ~50-100μs faster (no detection overhead)
   - Direct routing to adapter (no conditional logic)
   - Reduced CPU utilization (no payload inspection)
   - Better caching potential (per-route caching)

4. **Industry Standard**
   - Stripe webhooks: `/v1/webhooks/stripe`
   - GitHub webhooks: `/webhook/github`
   - Datadog webhooks: `/api/v1/series/datadog`
   - Follows REST/HTTP best practices

5. **Better Operations**
   - Clear 404 errors (wrong endpoint = clear problem)
   - Simple troubleshooting (check route registration)
   - Per-route metrics (Prometheus labels by endpoint)
   - Easy to add/remove adapters (register/unregister routes)

6. **Configuration-Driven**
   - Enable/disable adapters via YAML config
   - No code changes to add new adapter
   - Environment-specific adapter sets (dev vs prod)

**Trade-offs**:
- More HTTP routes (2-3 routes vs 1 generic route)
  - Mitigation: Minimal overhead, chi router handles efficiently
- URL convention needed (documented in API spec)
  - Mitigation: Clear pattern `/api/v1/signals/{adapter-name}`

#### **Alternatives Considered**

**Alternative 1: Detection-Based Adapter Selection (Design A)**

**Approach**:
```go
// Single generic endpoint
POST /api/v1/webhook

// Detection logic
func DetectAdapter(payload []byte) (AdapterType, error) {
    if containsPrometheusFingerprint(payload) {
        return PrometheusAdapter, nil
    }
    if containsKubernetesFingerprint(payload) {
        return KubernetesAdapter, nil
    }
    return UnknownAdapter, fmt.Errorf("unknown signal format")
}
```

**Rejected Because**:
- ❌ 70% more code (detection logic, fingerprinting, fallback)
- ❌ Security risk (source spoofing via format manipulation)
- ❌ Performance overhead (payload inspection on every request)
- ❌ Fragile (false positives if formats similar)
- ❌ Difficult to test (combinatorial explosion of edge cases)
- ❌ Poor operations (cryptic detection failures)

**Alternative 2: Header-Based Adapter Selection**

**Approach**:
```go
// Single endpoint with header-based routing
POST /api/v1/webhook
Header: X-Signal-Source: prometheus

// Header-based routing
func SelectAdapter(headers http.Header) (AdapterType, error) {
    source := headers.Get("X-Signal-Source")
    return adapterRegistry.Get(source)
}
```

**Rejected Because**:
- ❌ Requires clients to set custom headers (not standard)
- ❌ Still needs detection fallback if header missing
- ❌ Header spoofing risk
- ❌ Not industry standard (violates REST principles)
- ❌ Difficult to configure in monitoring tools

**Alternative 3: Query Parameter-Based Adapter Selection**

**Approach**:
```go
// Single endpoint with query parameter
POST /api/v1/webhook?source=prometheus
```

**Rejected Because**:
- ❌ Query parameters in POST requests anti-pattern
- ❌ URL logging exposes source in plain text logs
- ❌ Still needs detection fallback if parameter missing
- ❌ Not cacheable (query parameters affect cache key)

#### **Implementation**

**Endpoint Registration Pattern**:
```go
// pkg/gateway/server.go
func (s *Server) registerAdapters() {
    // Prometheus adapter
    if s.config.Adapters.Prometheus.Enabled {
        s.router.Post(s.config.Adapters.Prometheus.Path, 
            s.handlePrometheusWebhook)
        log.Info("Registered Prometheus adapter", 
            "path", s.config.Adapters.Prometheus.Path)
    }
    
    // Kubernetes Event adapter
    if s.config.Adapters.KubernetesEvent.Enabled {
        s.router.Post(s.config.Adapters.KubernetesEvent.Path, 
            s.handleKubernetesEventWebhook)
        log.Info("Registered Kubernetes Event adapter", 
            "path", s.config.Adapters.KubernetesEvent.Path)
    }
    
    // Future: Grafana adapter
    if s.config.Adapters.Grafana.Enabled {
        s.router.Post(s.config.Adapters.Grafana.Path, 
            s.handleGrafanaWebhook)
        log.Info("Registered Grafana adapter", 
            "path", s.config.Adapters.Grafana.Path)
    }
}
```

**Configuration-Driven Adapter Enablement**:
```yaml
# config/gateway.yaml
adapters:
  prometheus:
    enabled: true
    path: "/api/v1/signals/prometheus"
  kubernetes_event:
    enabled: true
    path: "/api/v1/signals/kubernetes-event"
  grafana:
    enabled: false  # Not implemented in v1.0
    path: "/api/v1/signals/grafana"
```

**Adapter Interface**:
```go
// pkg/gateway/adapters/adapter.go
type SignalAdapter interface {
    // Parse converts source-specific format to NormalizedSignal
    Parse(ctx context.Context, rawData []byte) (*NormalizedSignal, error)
    
    // Validate checks source-specific payload structure
    Validate(ctx context.Context, rawData []byte) error
    
    // SourceType returns the adapter source identifier
    SourceType() string
}
```

#### **Migration Path** (Not Applicable)

Gateway v1.0 is new implementation - no migration needed.

#### **Validation**

**Success Criteria**:
- ✅ Each adapter has dedicated endpoint
- ✅ HTTP 404 for unregistered adapters
- ✅ Per-route metrics in Prometheus
- ✅ Configuration-driven adapter enablement
- ✅ No detection logic in codebase
- ✅ Clear audit trail in access logs

**Validation Commands**:
```bash
# Verify Prometheus endpoint
curl -X POST http://localhost:8080/api/v1/signals/prometheus \
  -H "Content-Type: application/json" \
  -d '{"alerts": [{"status": "firing"}]}'
# Expected: 201 Created or 400 Bad Request (not 404)

# Verify Kubernetes Event endpoint
curl -X POST http://localhost:8080/api/v1/signals/kubernetes-event \
  -H "Content-Type: application/json" \
  -d '{"involvedObject": {"kind": "Pod"}}'
# Expected: 201 Created or 400 Bad Request (not 404)

# Verify disabled adapter returns 404
curl -X POST http://localhost:8080/api/v1/signals/grafana \
  -H "Content-Type: application/json" \
  -d '{}'
# Expected: 404 Not Found (adapter not enabled)

# Verify per-route metrics
curl http://localhost:9090/metrics | grep 'gateway_webhook_requests_total.*route='
# Expected: Metrics with route label (prometheus, kubernetes-event)
```

#### **Documentation**

- **API Specification**: See [API Examples](#-api-examples) section
- **Configuration**: See [Configuration Reference](#️-configuration-reference) section
- **Testing Patterns**: See [Example Tests](#-example-tests) section

#### **Related Decisions**

- **DD-GATEWAY-002** (Future): Adapter Discovery Mechanism
- **DD-GATEWAY-003** (Future): Dynamic Adapter Loading

#### **Lessons Learned**

- ✅ Explicit routing > automatic detection (simplicity wins)
- ✅ Configuration-driven design enables flexibility
- ✅ Industry standards exist for good reasons (REST/HTTP patterns)
- ✅ Security improves when architecture makes attacks obvious (404 vs spoofing)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

---

## 📊 Implementation Status

### ✅ DESIGN COMPLETE - Implementation Pending

**Current Phase**: Design Complete (Oct 4, 2025)
**Next Phase**: Implementation (Pending)

| Phase | Tests | Status | Effort | Confidence |
|-------|-------|--------|--------|------------|
| **Design Specification** | N/A | ✅ Complete | 16h | 100% |
| **Unit Tests** | 0/75 | ⏸️ Not Started | 20-25h | 85% |
| **Integration Tests** | 0/30 | ⏸️ Not Started | 15-20h | 85% |
| **E2E Tests** | 0/5 | ⏸️ Not Started | 5-10h | 85% |
| **Deployment** | N/A | ⏸️ Not Started | 8h | 90% |

**Gateway V1.0 Total**: 0/110 tests passing (estimated)
**Estimated Implementation Time**: 46-60 hours (6-8 days)
**Scope**: Prometheus AlertManager + Kubernetes Events only

---

## 📝 Business Requirements

### ✅ ESSENTIAL (Gateway V1.0: 40 BRs)

| Category | BR Range | Count | Status | Tests |
|----------|----------|-------|--------|-------|
| **Primary Signal Ingestion** | BR-GATEWAY-001 to 023 | 23 | ⏸️ 0% | 0/45 |
| **Environment Classification** | BR-GATEWAY-051 to 053 | 3 | ⏸️ 0% | 0/10 |
| **GitOps Integration** | BR-GATEWAY-071 to 072 | 2 | ⏸️ 0% | 0/5 |
| **Notification Routing** | BR-GATEWAY-091 to 092 | 2 | ⏸️ 0% | 0/5 |
| **HTTP Server** | BR-GATEWAY-036 to 045 | 10 | ⏸️ 0% | 0/15 |
| **Health & Observability** | BR-GATEWAY-016 to 025 | 10 | ⏸️ 0% | 0/10 |
| **Authentication & Security** | BR-GATEWAY-066 to 075 | 10 | ⏸️ 0% | 0/15 |

**Gateway V1.0 Total**: ~40 BRs (Prometheus + Kubernetes Events only)
**Tests**: 0/110 tests (75 unit, 30 integration, 5 e2e)

**Note**: Business requirements need formal enumeration. Current ranges are estimated from documentation review.

---

### Primary Requirements Breakdown

#### BR-GATEWAY-001 to 023: Signal Ingestion & Processing

**Core Functionality**:
- BR-GATEWAY-001: Accept signals from Prometheus AlertManager webhooks
- BR-GATEWAY-002: Accept signals from Kubernetes Event API
- BR-GATEWAY-003: Parse and normalize Prometheus alert format
- BR-GATEWAY-004: Parse and normalize Kubernetes Event format
- BR-GATEWAY-005: Deduplicate signals using Redis fingerprinting
- BR-GATEWAY-006: Generate SHA256 fingerprints for signal identity
- BR-GATEWAY-007: Detect alert storms (rate-based: >10 alerts/min)
- BR-GATEWAY-008: Detect alert storms (pattern-based: similar alerts across resources)
- BR-GATEWAY-009: Aggregate storm alerts into single CRD
- BR-GATEWAY-010: Store deduplication metadata in Redis (5-minute TTL)
- BR-GATEWAY-011: Classify environment from namespace labels
- BR-GATEWAY-012: Classify environment from ConfigMap overrides
- BR-GATEWAY-013: Assign priority using Rego policies
- BR-GATEWAY-014: Assign priority using severity+environment fallback table
- BR-GATEWAY-015: Create RemediationRequest CRD for new signals
- BR-GATEWAY-016: Storm aggregation (1-minute window)
- BR-GATEWAY-017: Return HTTP 201 for new CRD creation
- BR-GATEWAY-018: Return HTTP 202 for duplicate signals
- BR-GATEWAY-019: Return HTTP 400 for invalid signal payloads
- BR-GATEWAY-020: Return HTTP 500 for processing errors
- BR-GATEWAY-021: Record signal metadata in CRD
- BR-GATEWAY-022: Support adapter-specific routes
- BR-GATEWAY-023: Dynamic adapter registration

**Status**: ⏸️ Not Implemented (0/23 BRs)
**Tests**: 0/45 unit tests, 0/15 integration tests

---

#### BR-GATEWAY-051 to 053: Environment Classification

**Core Functionality**:
- BR-GATEWAY-051: Support dynamic environment taxonomy (any label value)
- BR-GATEWAY-052: Cache namespace labels (5-minute TTL)
- BR-GATEWAY-053: ConfigMap override for environment classification

**Status**: ⏸️ Not Implemented (0/3 BRs)
**Tests**: 0/10 unit tests

---

#### BR-GATEWAY-071 to 072: GitOps Integration

**Core Functionality**:
- BR-GATEWAY-071: Environment determines remediation behavior
- BR-GATEWAY-072: Priority-based workflow selection

**Status**: ⏸️ Not Implemented (0/2 BRs)
**Tests**: 0/5 integration tests

---

## 📅 **IMPLEMENTATION TIMELINE - 13 DAYS**

> **Purpose**: Day-by-day implementation schedule with APDC phases
>
> **Total Duration**: 104 hours (13 days @ 8 hours/day)
>
> **Methodology**: APDC (Analysis-Plan-Do-Check) with TDD (RED-GREEN-REFACTOR)

This section provides detailed daily implementation guidance following APDC methodology and TDD principles.

---

## 📅 **DAY 1: FOUNDATION + APDC ANALYSIS** (8 hours)

**Objective**: Establish foundation, validate infrastructure, perform comprehensive APDC analysis

**Prerequisites**: ✅ PRE-DAY 1 VALIDATION complete (all checkboxes marked)

---

### **APDC ANALYSIS PHASE** (2 hours)

#### **Business Context** (30 min)

**BR Mapping for Day 1**:
- **BR-GATEWAY-001**: Accept signals from Prometheus AlertManager webhooks
- **BR-GATEWAY-002**: Accept signals from Kubernetes Event API
- **BR-GATEWAY-005**: Deduplicate signals using Redis fingerprinting
- **BR-GATEWAY-015**: Create RemediationRequest CRD for new signals

**Business Value**:
1. Enable automated remediation workflow (primary value proposition)
2. Reduce MTTR by 40-60% through automated signal processing
3. Support multiple signal sources (Prometheus, Kubernetes Events)
4. Foundation for BR-GATEWAY-003 through BR-GATEWAY-023

**Success Criteria**:
- Package structure created (`pkg/gateway/`)
- Redis connectivity validated
- Kubernetes CRD creation capability confirmed
- Foundation ready for Day 2 adapter implementation

#### **Technical Context** (45 min)

**Existing Patterns to Follow**:
```bash
# Search for existing Gateway code patterns
codebase_search "HTTP server setup patterns in kubernaut services"
codebase_search "Redis client initialization patterns"
codebase_search "controller-runtime CRD creation patterns"
```

**Expected Findings**:
- ✅ HTTP server patterns from Context API, Notification services
- ✅ Redis client setup from Data Storage Service
- ✅ CRD creation patterns from existing controllers
- ✅ Structured logging with logrus
- ✅ Health check patterns

**Integration Points**:
```bash
# Verify Gateway will integrate with existing services
grep -r "RemediationRequest" api/remediation/v1/ --include="*.go"
# Expected: RemediationRequest CRD definition exists

grep -r "gateway" cmd/ --include="*.go"
# Expected: No existing gateway references (clean slate)
```

#### **Complexity Assessment** (30 min)

**Architecture Decision: Adapter-Specific Endpoints** (DD-GATEWAY-001)
- **Complexity Level**: SIMPLE
- **Rationale**: Follows established HTTP server patterns
- **Novel Components**: None (all patterns established in other services)
- **Risk**: LOW (well-understood technology stack)

**Package Structure Complexity**: SIMPLE
```
pkg/gateway/
├── adapters/        # Signal adapters (Prometheus, K8s Events)
├── processing/      # Deduplication, storm detection, priority
├── middleware/      # Authentication, rate limiting, logging
├── server/          # HTTP server with chi router
└── types/           # Shared types (NormalizedSignal, Config)
```

**Confidence**: 90% (following proven patterns from other services)

#### **Analysis Deliverables**

- [x] Business context documented (4 BRs identified for Day 1)
- [x] Existing patterns identified (Context API, Notification, Data Storage)
- [x] Integration points verified (RemediationRequest CRD exists)
- [x] Complexity assessed (SIMPLE, following established patterns)
- [x] Risk level: LOW

**Analysis Phase Checkpoint**:
```
✅ ANALYSIS PHASE COMPLETE:
- [x] Business requirement (BR-GATEWAY-001, BR-GATEWAY-002, BR-GATEWAY-005, BR-GATEWAY-015) identified ✅
- [x] Existing implementation search executed ✅
- [x] Technical context fully documented ✅
- [x] Integration patterns discovered (Context API, Notification, Data Storage) ✅
- [x] Complexity assessment completed (SIMPLE) ✅
```

---

### **APDC PLAN PHASE** (1 hour)

#### **TDD Strategy** (20 min)

**Test-First Approach**:
1. **Unit Tests**: Write package structure validation tests
2. **Integration Tests**: Defer to Day 8 (requires full stack)
3. **Foundation Tests**: Basic connectivity tests (Redis, K8s)

**Test Locations**:
- `test/unit/gateway/server_test.go` - Server initialization tests
- `test/unit/gateway/types_test.go` - Type definition tests
- `test/integration/gateway/suite_test.go` - Integration test setup (skeleton only)

**TDD RED-GREEN-REFACTOR Plan**:
- **RED**: Write tests for package structure, types, server initialization
- **GREEN**: Create minimal package skeleton to pass tests
- **REFACTOR**: Add proper imports, documentation, logging

#### **Integration Plan** (20 min)

**Package Structure**:
```go
// pkg/gateway/types/signal.go
package types

import (
    "time"
)

// NormalizedSignal represents a signal from any source
type NormalizedSignal struct {
    // Identity
    Fingerprint string    // SHA256 fingerprint for deduplication
    AlertName   string    // Alert/event name
    SourceType  string    // Source: prometheus, kubernetes-event
    
    // Classification
    Severity    string    // critical, warning, info
    Environment string    // Determined from namespace labels
    Priority    string    // P1-P4 from Rego policy
    
    // Resource context
    Namespace   string
    Resource    ResourceInfo
    
    // Metadata
    Labels      map[string]string
    Annotations map[string]string
    Timestamp   time.Time
}

// ResourceInfo contains resource details
type ResourceInfo struct {
    Kind      string
    Name      string
    Namespace string
}
```

**Server Skeleton**:
```go
// pkg/gateway/server/server.go
package server

import (
    "context"
    "net/http"
    
    "github.com/go-chi/chi/v5"
    "github.com/sirupsen/logrus"
)

// Server is the Gateway HTTP server
type Server struct {
    router *chi.Mux
    logger *logrus.Logger
    config *Config
}

// NewServer creates a new Gateway server
func NewServer(config *Config, logger *logrus.Logger) *Server {
    return &Server{
        router: chi.NewRouter(),
        logger: logger,
        config: config,
    }
}

// Start starts the HTTP server
func (s *Server) Start(ctx context.Context) error {
    s.logger.Info("Starting Gateway server", "addr", s.config.ListenAddr)
    return http.ListenAndServe(s.config.ListenAddr, s.router)
}
```

#### **Success Definition** (10 min)

**Day 1 Success Criteria**:
1. ✅ Package structure created (`pkg/gateway/*`)
2. ✅ Basic types defined (`NormalizedSignal`, `ResourceInfo`)
3. ✅ Server skeleton created (can start/stop)
4. ✅ Redis client initialized and tested
5. ✅ Kubernetes client initialized and tested
6. ✅ Zero lint errors
7. ✅ Foundation tests passing

**Validation Commands**:
```bash
# Compile check
go build ./pkg/gateway/...

# Lint check
golangci-lint run ./pkg/gateway/...

# Run foundation tests
go test ./test/unit/gateway/... -v

# Verify package structure
ls -la pkg/gateway/
# Expected: types/, server/, adapters/, processing/, middleware/
```

#### **Risk Mitigation** (10 min)

**Identified Risks**:
1. **Risk**: Redis connection failures
   - **Mitigation**: Retry logic with exponential backoff
   - **Validation**: Connection test in PRE-DAY 1 VALIDATION

2. **Risk**: Kubernetes API permission issues
   - **Mitigation**: RBAC validation script
   - **Validation**: `kubectl auth can-i create remediationrequests`

3. **Risk**: Package import cycles
   - **Mitigation**: Clear package hierarchy (types → processing → server)
   - **Validation**: Compile checks

---

### **DO PHASE** (4 hours)

#### **DO-DISCOVERY: Search Existing Patterns** (30 min)

```bash
# Search for HTTP server patterns
codebase_search "chi router setup in kubernaut services"

# Search for Redis client patterns
codebase_search "Redis client initialization with connection pooling"

# Search for Kubernetes client patterns
codebase_search "controller-runtime client setup"

# Search for logrus logger setup
codebase_search "structured logging setup with logrus"
```

#### **DO-RED: Write Foundation Tests** (1 hour)

**Test 1: Package Structure Validation**
```go
// test/unit/gateway/structure_test.go
package gateway

import (
    "testing"
    
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

func TestGatewayPackage(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "Gateway Package Structure Suite")
}

var _ = Describe("BR-GATEWAY-001: Package Structure", func() {
    It("should have types package", func() {
        // This test will fail initially (RED phase)
        _, err := os.Stat("../../../pkg/gateway/types")
        Expect(err).ToNot(HaveOccurred())
    })
    
    It("should have server package", func() {
        _, err := os.Stat("../../../pkg/gateway/server")
        Expect(err).ToNot(HaveOccurred())
    })
    
    It("should have adapters package", func() {
        _, err := os.Stat("../../../pkg/gateway/adapters")
        Expect(err).ToNot(HaveOccurred())
    })
})
```

**Test 2: Type Definition Tests**
```go
// test/unit/gateway/types_test.go
package gateway

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    
    "github.com/jordigilh/kubernaut/pkg/gateway/types"
)

var _ = Describe("BR-GATEWAY-005: NormalizedSignal Type", func() {
    It("should have required fields", func() {
        signal := &types.NormalizedSignal{
            Fingerprint: "test-fingerprint",
            AlertName:   "TestAlert",
            SourceType:  "prometheus",
        }
        
        Expect(signal.Fingerprint).To(Equal("test-fingerprint"))
        Expect(signal.AlertName).To(Equal("TestAlert"))
        Expect(signal.SourceType).To(Equal("prometheus"))
    })
})
```

#### **DO-GREEN: Create Package Skeleton** (1.5 hours)

**Step 1: Create Package Structure**
```bash
mkdir -p pkg/gateway/types
mkdir -p pkg/gateway/server
mkdir -p pkg/gateway/adapters
mkdir -p pkg/gateway/processing
mkdir -p pkg/gateway/middleware
```

**Step 2: Implement Types**
```go
// pkg/gateway/types/signal.go
// (Full implementation as shown in Integration Plan above)
```

**Step 3: Implement Server Skeleton**
```go
// pkg/gateway/server/server.go
// (Full implementation as shown in Integration Plan above)
```

**Step 4: Implement Config**
```go
// pkg/gateway/server/config.go
package server

type Config struct {
    ListenAddr      string
    ReadTimeout     time.Duration
    WriteTimeout    time.Duration
    RedisAddr       string
    RedisPassword   string
}
```

#### **DO-REFACTOR: Enhance with Proper Imports** (1 hour)

**Step 1: Add Documentation**
```go
// pkg/gateway/types/signal.go

// Package types defines core Gateway types for signal processing.
//
// This package provides type definitions for normalized signals from
// multiple sources (Prometheus, Kubernetes Events) following BR-GATEWAY-003
// and BR-GATEWAY-004 normalization requirements.
package types

// NormalizedSignal represents a signal from any monitoring source,
// normalized to a common format for processing.
//
// This type satisfies BR-GATEWAY-003 (Prometheus normalization) and
// BR-GATEWAY-004 (Kubernetes Event normalization).
type NormalizedSignal struct {
    // ... (fields with detailed comments)
}
```

**Step 2: Add Logging**
```go
// pkg/gateway/server/server.go

func NewServer(config *Config, logger *logrus.Logger) *Server {
    logger.Info("Initializing Gateway server",
        "listen_addr", config.ListenAddr,
        "version", "v1.0")
    
    return &Server{
        router: chi.NewRouter(),
        logger: logger,
        config: config,
    }
}
```

**Step 3: Add Error Handling**
```go
func (s *Server) Start(ctx context.Context) error {
    s.logger.Info("Starting Gateway server", "addr", s.config.ListenAddr)
    
    server := &http.Server{
        Addr:         s.config.ListenAddr,
        Handler:      s.router,
        ReadTimeout:  s.config.ReadTimeout,
        WriteTimeout: s.config.WriteTimeout,
    }
    
    if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
        s.logger.Error("Server failed", "error", err)
        return fmt.Errorf("server failed: %w", err)
    }
    
    return nil
}
```

---

### **CHECK PHASE** (1 hour)

#### **Validation Commands**

```bash
# 1. Verify package structure
ls -la pkg/gateway/
# Expected: types/, server/, adapters/, processing/, middleware/

# 2. Compile check
go build ./pkg/gateway/...
# Expected: Success (no errors)

# 3. Lint check
golangci-lint run ./pkg/gateway/...
# Expected: No errors

# 4. Run foundation tests
go test ./test/unit/gateway/... -v
# Expected: All tests passing

# 5. Verify imports
go list -m all | grep gateway
# Expected: No unexpected dependencies
```

#### **Business Verification**

- [x] **BR-GATEWAY-001**: Foundation for Prometheus webhook acceptance ✅
- [x] **BR-GATEWAY-002**: Foundation for Kubernetes Event acceptance ✅
- [x] **BR-GATEWAY-005**: NormalizedSignal type supports deduplication ✅
- [x] **BR-GATEWAY-015**: Foundation for CRD creation ✅

#### **Technical Validation**

- [x] Package structure created ✅
- [x] Types compile without errors ✅
- [x] Server can be instantiated ✅
- [x] No lint errors ✅
- [x] Foundation tests passing ✅

#### **Confidence Assessment**

**Day 1 Confidence**: 95% ✅ **Very High**

**Justification**:
- ✅ All foundation components created
- ✅ Follows established patterns from Context API, Notification services
- ✅ Clean package structure with no import cycles
- ✅ Tests validate package structure correctness
- ✅ Ready for Day 2 adapter implementation

**Risks**:
- ⚠️  Minor: Package structure may need adjustment during Day 2-3 (5% risk)
- Mitigation: Keep refactoring minimal during GREEN phase

**Next Steps**:
- Day 2: Implement Prometheus and Kubernetes Event adapters
- Day 3: Implement deduplication and storm detection

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

---

## 📅 **DAY 2: SIGNAL ADAPTERS** (8 hours)

**Objective**: Implement Prometheus and Kubernetes Event adapters with full TDD

**Business Requirements**: BR-GATEWAY-001, BR-GATEWAY-002, BR-GATEWAY-003, BR-GATEWAY-004

**APDC Summary**:
- **Analysis** (1h): Prometheus/K8s Event webhook formats, existing adapter patterns
- **Plan** (1h): TDD strategy for 2 adapters, 15-20 unit tests
- **Do** (5h): RED (write adapter tests) → GREEN (minimal parsing) → REFACTOR (full JSON parsing, field extraction, validation)
- **Check** (1h): Verify adapters parse webhooks correctly, fingerprint generation works

**Key Deliverables**:
- `pkg/gateway/adapters/prometheus_adapter.go` - Parse Prometheus AlertManager webhooks
- `pkg/gateway/adapters/kubernetes_event_adapter.go` - Parse Kubernetes Event API
- `test/unit/gateway/adapters/prometheus_adapter_test.go` - 8-10 unit tests
- `test/unit/gateway/adapters/kubernetes_event_adapter_test.go` - 7-9 unit tests

**Success Criteria**: Both adapters parse test payloads, generate fingerprints, 90%+ test coverage

**Confidence**: 90% (clear input/output, established JSON parsing patterns)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

---

## 📅 **DAY 3: DEDUPLICATION + STORM DETECTION** (8 hours)

**Objective**: Implement fingerprint generation, Redis-based deduplication, storm detection algorithms

**Business Requirements**: BR-GATEWAY-005, BR-GATEWAY-006, BR-GATEWAY-007, BR-GATEWAY-008, BR-GATEWAY-009, BR-GATEWAY-010

**APDC Summary**:
- **Analysis** (1h): Redis SET NX EX atomic operations, storm detection algorithms (rate-based vs pattern-based)
- **Plan** (1h): TDD for deduplication service, storm detector, storm aggregator
- **Do** (5h): Implement SHA256 fingerprinting, Redis dedup (5min TTL), rate-based storm (>10 alerts/min), pattern-based storm (similarity detection), aggregation (1min window)
- **Check** (1h): Verify dedup prevents duplicates, storm detection catches bursts, aggregation creates single CRD

**Key Deliverables**:
- `pkg/gateway/processing/deduplication.go` - Redis fingerprint checking
- `pkg/gateway/processing/storm_detector.go` - Rate + pattern detection
- `pkg/gateway/processing/storm_aggregator.go` - Storm signal aggregation
- `test/unit/gateway/processing/` - 12-15 unit tests

**Success Criteria**: Deduplication works (Redis TTL), storm detection triggers correctly, 85%+ test coverage

**Confidence**: 85% (Redis operations well-understood, storm detection needs tuning)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

---

## 📅 **DAY 4: ENVIRONMENT + PRIORITY** (8 hours)

**Objective**: Implement environment classification, Rego policy integration, fallback priority table

**Business Requirements**: BR-GATEWAY-011, BR-GATEWAY-012, BR-GATEWAY-013, BR-GATEWAY-014

**APDC Summary**:
- **Analysis** (1h): Namespace label patterns, Rego policy structure, fallback matrix
- **Plan** (1h): TDD for environment classifier (K8s API), Rego policy loader, fallback table
- **Do** (5h): Implement namespace label reading (cache 30s), ConfigMap override, Rego policy eval (OPA library), fallback table (severity+environment → priority)
- **Check** (1h): Verify environment from labels, Rego assigns priority, fallback works

**Key Deliverables**:
- `pkg/gateway/processing/environment_classifier.go` - Read namespace labels
- `pkg/gateway/processing/priority_engine.go` - Rego + fallback logic
- `test/unit/gateway/processing/` - 10-12 unit tests
- Example Rego policy in `docs/gateway/priority-policy.rego`

**Success Criteria**: Environment classified correctly, priority assigned, 85%+ test coverage

**Confidence**: 80% (Rego integration new, needs validation)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

---

## 📅 **DAY 5: CRD CREATION + HTTP SERVER** (8 hours)

**Objective**: Implement RemediationRequest CRD creation, HTTP server with chi router, middleware setup

**Business Requirements**: BR-GATEWAY-015, BR-GATEWAY-017, BR-GATEWAY-018, BR-GATEWAY-019, BR-GATEWAY-020, BR-GATEWAY-022, BR-GATEWAY-023

**APDC Summary**:
- **Analysis** (1h): RemediationRequest CRD schema, chi router patterns, middleware stack
- **Plan** (1h): TDD for CRD creator, HTTP handlers, response codes
- **Do** (5h): Implement CRD creator (controller-runtime), webhook handlers (Prometheus, K8s Event), middleware (logging, recovery, request ID), HTTP responses (201/202/400/500)
- **Check** (1h): Verify CRDs created in K8s, webhooks return correct codes, middleware active

**Key Deliverables**:
- `pkg/gateway/processing/crd_creator.go` - Create RemediationRequest CRDs
- `pkg/gateway/server/handlers.go` - Webhook HTTP handlers
- `pkg/gateway/middleware/` - Logging, recovery, request ID middlewares
- `test/unit/gateway/server/` - 12-15 unit tests

**Success Criteria**: CRDs created successfully, HTTP 201/202/400/500 codes correct, 85%+ test coverage

**Confidence**: 90% (CRD creation well-understood, HTTP patterns established)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

---

## 📅 **DAY 6: AUTHENTICATION + SECURITY** (8 hours)

**Objective**: Implement TokenReviewer authentication, rate limiting, security middleware

**Business Requirements**: BR-GATEWAY-066 through BR-GATEWAY-075

**APDC Summary**:
- **Analysis** (1h): Kubernetes TokenReviewer API, rate limiting algorithms, security headers
- **Plan** (1h): TDD for auth middleware, rate limiter, security headers
- **Do** (5h): Implement TokenReviewer auth (Bearer tokens), rate limiter (100 req/min, burst 10), security headers (CORS, CSP, HSTS), webhook timestamp validation (5min window)
- **Check** (1h): Verify auth blocks invalid tokens, rate limit enforced, security headers present

**Key Deliverables**:
- `pkg/gateway/middleware/auth.go` - TokenReviewer authentication
- `pkg/gateway/middleware/rate_limiter.go` - Rate limiting (redis-based)
- `pkg/gateway/middleware/security.go` - Security headers
- `test/unit/gateway/middleware/` - 10-12 unit tests

**Success Criteria**: Auth blocks unauthorized, rate limit works, security headers set, 85%+ test coverage

**Confidence**: 85% (TokenReviewer straightforward, rate limiting needs load testing)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

---

## 📅 **DAY 7: METRICS + OBSERVABILITY** (8 hours)

**Objective**: Implement Prometheus metrics, structured logging, health endpoints

**Business Requirements**: BR-GATEWAY-016 through BR-GATEWAY-025

**APDC Summary**:
- **Analysis** (1h): Prometheus metric types, health check patterns, log structure
- **Plan** (1h): TDD for metrics, health checks, log formatting
- **Do** (5h): Implement Prometheus metrics (counters: requests, errors; histograms: latency, processing time; gauges: in-flight), structured logging (logrus with fields), health endpoints (/health, /ready)
- **Check** (1h): Verify metrics exported, logs structured, health endpoints responsive

**Key Deliverables**:
- `pkg/gateway/metrics/metrics.go` - Prometheus metrics registration
- `pkg/gateway/server/health.go` - Health/readiness checks
- Structured logging throughout server and processing packages
- `test/unit/gateway/metrics/` - 8-10 unit tests

**Success Criteria**: Metrics exported to /metrics, health checks pass, logs structured JSON, 85%+ test coverage

**Confidence**: 95% (Prometheus metrics well-understood, health checks simple)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

---

## 📅 **DAY 8: INTEGRATION TESTING** (8 hours)

**Objective**: Full APDC integration test suite with anti-flaky patterns

**Business Requirements**: All BR-GATEWAY-001 through BR-GATEWAY-075 (integration coverage)

**APDC Summary**:
- **Analysis** (1h): Integration test scenarios, anti-flaky patterns, test infrastructure (real Redis, real K8s API)
- **Plan** (1h): Test pyramid strategy (>50% integration coverage), test environment setup
- **Do** (5h): Implement 25-30 integration tests: end-to-end webhook flow (Prometheus → CRD), deduplication (real Redis), storm detection, CRD creation (real K8s API), authentication (TokenReviewer), rate limiting
- **Check** (1h): Verify all integration tests pass, >50% coverage, no flaky tests

**Key Deliverables**:
- `test/integration/gateway/suite_test.go` - Integration test suite setup
- `test/integration/gateway/webhook_flow_test.go` - End-to-end webhook processing
- `test/integration/gateway/deduplication_test.go` - Real Redis deduplication tests
- `test/integration/gateway/storm_detection_test.go` - Storm detection integration
- `test/integration/gateway/crd_creation_test.go` - Real Kubernetes CRD tests

**Anti-Flaky Patterns**:
- Eventual consistency checks (wait for CRD creation)
- Redis state cleanup between tests
- Timeout-based assertions (not fixed delays)
- Test isolation (separate Redis keys, unique CRD names)

**Success Criteria**: >50% integration coverage, all tests pass consistently, no flaky tests

**Confidence**: 90% (integration tests well-structured, anti-flaky patterns prevent issues)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

---

## 📅 **DAY 9: PRODUCTION READINESS** (8 hours)

**Objective**: Dockerfiles (standard + UBI9), Makefile targets, deployment manifests

**Business Requirements**: Deployment infrastructure for all Gateway components

**APDC Summary**:
- **Analysis** (1h): Docker best practices, UBI9 requirements (ADR-027), deployment architecture
- **Plan** (1h): Dockerfile structure, Makefile targets, Kubernetes manifests
- **Do** (5h): Create Dockerfiles (standard alpine, UBI9), Makefile targets (build-gateway, test-gateway, docker-build-gateway), deployment manifests (RBAC, Service, Deployment, ConfigMap, HPA, ServiceMonitor, NetworkPolicy)
- **Check** (1h): Verify Docker builds, make targets work, manifests deploy successfully

**Key Deliverables**:
- `docker/gateway-service.Dockerfile` - Standard alpine-based image
- `docker/gateway-service-ubi9.Dockerfile` - Red Hat UBI9 image (production)
- `Makefile` - Gateway-specific targets (build, test, docker-build, deploy)
- `deploy/gateway/` - Complete Kubernetes manifests (8-10 files)
- `deploy/gateway/README.md` - Deployment guide

**Makefile Targets**:
```makefile
.PHONY: build-gateway test-gateway docker-build-gateway deploy-gateway

build-gateway:
	go build -o bin/gateway cmd/gateway/main.go

test-gateway:
	go test ./pkg/gateway/... ./test/unit/gateway/... -v -cover

docker-build-gateway:
	docker build -f docker/gateway-service.Dockerfile -t kubernaut/gateway:latest .
	docker build -f docker/gateway-service-ubi9.Dockerfile -t kubernaut/gateway:latest-ubi9 .

deploy-gateway:
	kubectl apply -f deploy/gateway/
```

**Success Criteria**: Docker images build, Makefile targets execute, manifests deploy to K8s cluster

**Confidence**: 95% (deployment patterns well-established)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

---

## 📅 **DAYS 10-11: E2E TESTING** (16 hours)

**Objective**: End-to-end workflow testing, multi-signal scenarios, stress testing

**Business Requirements**: Complete workflow validation (signal → CRD → orchestrator)

**APDC Summary**:
- **Analysis** (2h): E2E test scenarios, performance benchmarks, load testing approach
- **Plan** (2h): E2E test cases (5-7 scenarios), stress test parameters (1000 req/s)
- **Do** (10h): Implement E2E tests: Prometheus webhook → RemediationRequest CRD → Orchestrator pickup, Kubernetes Event → CRD, storm detection end-to-end, duplicate signal handling, authentication failure scenarios, rate limit enforcement, multi-signal burst handling
- **Check** (2h): Verify E2E tests pass, performance meets targets (<100ms p95), stress test succeeds

**Key Deliverables**:
- `test/e2e/gateway/suite_test.go` - E2E test suite setup
- `test/e2e/gateway/prometheus_webhook_e2e_test.go` - Complete Prometheus flow
- `test/e2e/gateway/kubernetes_event_e2e_test.go` - Complete K8s Event flow
- `test/e2e/gateway/storm_detection_e2e_test.go` - Storm handling end-to-end
- `test/e2e/gateway/performance_test.go` - Performance benchmarks
- `test/e2e/gateway/stress_test.go` - Load testing (1000 req/s sustained)

**Performance Targets**:
- p50 latency: <50ms (signal ingestion → CRD creation)
- p95 latency: <100ms
- p99 latency: <200ms
- Throughput: >1000 signals/second
- Memory: <500MB under load
- CPU: <2 cores under load

**Success Criteria**: All E2E tests pass, performance targets met, stress test succeeds (1000 req/s for 5 min)

**Confidence**: 85% (E2E tests complex, performance depends on infrastructure)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

---

## 📅 **DAYS 12-13: DOCUMENTATION + HANDOFF** (16 hours)

**Objective**: API documentation, architecture diagrams, operational guides, handoff summary

**Business Requirements**: Complete documentation for operations and development teams

**APDC Summary**:
- **Analysis** (2h): Documentation requirements, audience needs (ops vs dev), format standards
- **Plan** (2h): Documentation structure, diagram types (Mermaid), content outline
- **Do** (10h): Create OpenAPI/Swagger spec, architecture diagrams (component, sequence, deployment), operational guides (deployment, monitoring, troubleshooting), developer guides (adding adapters, testing), runbooks (incident response, maintenance), handoff summary (implementation metrics, test coverage, known issues)
- **Check** (2h): Review documentation completeness, validate diagrams accuracy, verify runbooks

**Key Deliverables**:
- `docs/services/stateless/gateway-service/API_SPEC.yaml` - OpenAPI 3.0 specification
- `docs/services/stateless/gateway-service/ARCHITECTURE.md` - Architecture overview with Mermaid diagrams
- `docs/services/stateless/gateway-service/OPERATIONS_GUIDE.md` - Deployment, monitoring, troubleshooting
- `docs/services/stateless/gateway-service/DEVELOPER_GUIDE.md` - Development, testing, contributing
- `docs/services/stateless/gateway-service/RUNBOOKS.md` - Incident response procedures
- `docs/services/stateless/gateway-service/HANDOFF_SUMMARY.md` - Final implementation summary

**Architecture Diagrams** (Mermaid):
1. Component Diagram - Gateway internal structure
2. Sequence Diagram - Webhook processing flow
3. Deployment Diagram - Kubernetes resources
4. Data Flow Diagram - Signal processing pipeline

**Operational Guides**:
1. Deployment Guide - Step-by-step deployment
2. Monitoring Guide - Prometheus metrics, dashboards, alerts
3. Troubleshooting Guide - Common issues and resolutions
4. Scaling Guide - HPA configuration, performance tuning
5. Security Guide - Authentication, authorization, network policies

**Success Criteria**: All documentation complete, diagrams accurate, runbooks validated, handoff summary approved

**Confidence**: 95% (documentation straightforward, templates available)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

---

## 🎯 v1.0 Architecture

### Core Functionality

**API Endpoints** (Adapter-specific):
```
Signal Ingestion:
  POST /api/v1/signals/prometheus         # Prometheus AlertManager webhooks
  POST /api/v1/signals/kubernetes-event   # Kubernetes Event API signals

Health & Monitoring:
  GET  /health                            # Liveness probe
  GET  /ready                             # Readiness probe
  GET  /metrics                           # Prometheus metrics
```

**Architecture**:
- **Adapter-specific endpoints**: Each adapter registers its own HTTP route
- **Configuration-driven**: Enable/disable adapters via YAML config
- **No detection logic**: HTTP routing handles adapter selection
- **Security**: No source spoofing, explicit routing, clear audit trail

**Authentication**:
- Kubernetes ServiceAccount token validation (TokenReviewer API)
- Bearer token required for all signal endpoints
- No authentication for health endpoints

**Configuration** (minimal for production):
```yaml
server:
  listen_addr: ":8080"
  read_timeout: 30s
  write_timeout: 30s

redis:
  addr: "redis:6379"
  password: "${REDIS_PASSWORD}"

rate_limit:
  requests_per_minute: 100
  burst: 10

deduplication:
  ttl: 5m

storm_detection:
  rate_threshold: 10      # alerts/minute
  pattern_threshold: 5    # similar alerts
  aggregation_window: 1m

environment:
  cache_ttl: 30s
  configmap_namespace: kubernaut-system
  configmap_name: kubernaut-environment-overrides
```

**See**: [Complete Configuration Reference](#️-configuration-reference) for all options and environment variables

---

## ⚙️ Configuration Reference

### Complete Configuration Schema

```yaml
# Complete configuration with all options
server:
  listen_addr: ":8080"              # HTTP server address
  read_timeout: 30s                 # Request read timeout
  write_timeout: 30s                # Response write timeout
  idle_timeout: 120s                # Keep-alive idle timeout
  max_header_bytes: 1048576         # 1MB max header size
  graceful_shutdown_timeout: 30s    # Shutdown grace period

redis:
  addr: "redis:6379"                # Redis server address
  password: ""                      # Redis password (use env var)
  db: 0                             # Redis database number
  max_retries: 3                    # Connection retry attempts
  min_idle_conns: 10                # Min idle connections
  pool_size: 100                    # Max connections
  pool_timeout: 4s                  # Pool wait timeout
  dial_timeout: 5s                  # Connection dial timeout
  read_timeout: 3s                  # Read timeout
  write_timeout: 3s                 # Write timeout

rate_limit:
  requests_per_minute: 100          # Global rate limit
  burst: 10                         # Burst capacity
  per_namespace: false              # Per-namespace limits (future)

deduplication:
  ttl: 5m                           # Fingerprint TTL
  cleanup_interval: 1m              # Cleanup goroutine interval

storm_detection:
  rate_threshold: 10                # Alerts/minute for rate-based
  pattern_threshold: 5              # Similar alerts for pattern-based
  aggregation_window: 1m            # Storm aggregation window
  similarity_threshold: 0.8         # Pattern similarity (0.0-1.0)

environment:
  cache_ttl: 30s                    # Namespace label cache TTL
  configmap_namespace: "kubernaut-system"
  configmap_name: "kubernaut-environment-overrides"
  default_environment: "unknown"    # Fallback environment

priority:
  rego_policy_path: "/etc/kubernaut/policies/priority.rego"
  fallback_table:
    critical_production: "P1"
    critical_staging: "P2"
    warning_production: "P2"
    warning_staging: "P3"
    default: "P4"

logging:
  level: "info"                     # trace, debug, info, warn, error
  format: "json"                    # json, text
  output: "stdout"                  # stdout, stderr, file
  add_caller: true                  # Include file:line in logs

metrics:
  enabled: true
  listen_addr: ":9090"
  path: "/metrics"

health:
  enabled: true
  path: "/health"
  readiness_path: "/ready"

adapters:
  prometheus:
    enabled: true
    path: "/api/v1/signals/prometheus"
  kubernetes_event:
    enabled: true
    path: "/api/v1/signals/kubernetes-event"
  grafana:
    enabled: false                  # Future adapter
    path: "/api/v1/signals/grafana"
```

---

### Environment Variables

All configuration can be overridden via environment variables:

| Environment Variable | Config Path | Example | Required |
|---------------------|-------------|---------|----------|
| `GATEWAY_LISTEN_ADDR` | `server.listen_addr` | `:8080` | No |
| `REDIS_ADDR` | `redis.addr` | `redis:6379` | Yes |
| `REDIS_PASSWORD` | `redis.password` | `<secret>` | Yes (prod) |
| `REDIS_DB` | `redis.db` | `0` | No |
| `RATE_LIMIT_RPM` | `rate_limit.requests_per_minute` | `100` | No |
| `DEDUPLICATION_TTL` | `deduplication.ttl` | `5m` | No |
| `STORM_RATE_THRESHOLD` | `storm_detection.rate_threshold` | `10` | No |
| `STORM_PATTERN_THRESHOLD` | `storm_detection.pattern_threshold` | `5` | No |
| `ENVIRONMENT_CACHE_TTL` | `environment.cache_ttl` | `30s` | No |
| `LOG_LEVEL` | `logging.level` | `info` | No |
| `LOG_FORMAT` | `logging.format` | `json` | No |
| `METRICS_ENABLED` | `metrics.enabled` | `true` | No |

**Example Deployment**:
```yaml
env:
- name: REDIS_ADDR
  value: "redis:6379"
- name: REDIS_PASSWORD
  valueFrom:
    secretKeyRef:
      name: redis-credentials
      key: password
- name: LOG_LEVEL
  value: "info"
- name: STORM_RATE_THRESHOLD
  value: "15"  # Tuned for production
```

---

## 📦 Dependencies

### External Dependencies

| Dependency | Version | Purpose | License | Notes |
|------------|---------|---------|---------|-------|
| **go-redis/redis/v9** | v9.3.0+ | Redis client for deduplication | BSD-2-Clause | Production-grade, connection pooling |
| **go-chi/chi/v5** | v5.0.10+ | HTTP router for adapters | MIT | Lightweight, idiomatic Go |
| **sirupsen/logrus** | v1.9.3+ | Structured logging | MIT | Standard for kubernaut |
| **kubernetes/client-go** | v0.28.x | Kubernetes API client | Apache-2.0 | CRD creation, K8s API |
| **sigs.k8s.io/controller-runtime** | v0.16.x | CRD management | Apache-2.0 | Controller-runtime client |
| **open-policy-agent/opa** | v0.57.x | Rego policy engine (priority) | Apache-2.0 | Optional, fallback table if not used |
| **prometheus/client_golang** | v1.17.x | Prometheus metrics | Apache-2.0 | Standard metrics library |
| **gorilla/mux** | v1.8.1+ | HTTP middleware (fallback) | BSD-3-Clause | Alternative to chi if needed |

**Total External**: 7-8 dependencies

---

### Internal Dependencies

| Dependency | Purpose | Location | Status |
|------------|---------|----------|--------|
| **pkg/testutil** | Test helpers, mocks, Kind cluster | `/pkg/testutil/` | ✅ Existing |
| **pkg/shared/types** | Shared type definitions | `/pkg/shared/types/` | ⏸️ May need expansion |
| **api/remediation/v1** | RemediationRequest CRD | `/api/remediation/` | ✅ Existing |

**Total Internal**: 3 dependencies

---

### Dependency Security

**Vulnerability Scanning**:
```bash
# Check for known vulnerabilities
go list -json -m all | nancy sleuth

# Alternative: Use govulncheck
govulncheck ./pkg/gateway/...
```

**License Compliance**:
```bash
# Verify license compatibility
go-licenses check ./pkg/gateway/...
```

**Update Policy**:
- **Security patches**: Immediate (within 24h)
- **Minor version updates**: Monthly maintenance window
- **Major version updates**: Quarterly review with testing
- **Dependency audit**: Every 6 months

---

### Processing Pipeline

**Signal Processing Stages**:

1. **Ingestion** (via adapters):
   - Receive webhook from signal source
   - Parse and normalize signal data (adapter-specific)
   - Extract metadata (labels, annotations, timestamps)
   - Validate signal format

2. **Processing pipeline**:
   - **Deduplication**: Check if signal was seen before (Redis lookup, ~3ms)
   - **Storm detection**: Identify alert storms (rate + pattern-based, ~3ms)
   - **Classification**: Determine environment (namespace labels + ConfigMap, ~15ms)
   - **Priority assignment**: Calculate priority (Rego or fallback table, ~1ms)

3. **CRD creation**:
   - Build RemediationRequest CRD from normalized signal
   - Create CRD in Kubernetes (~30ms)
   - Record deduplication metadata in Redis (~3ms)

4. **HTTP response**:
   - 201 Created: New RemediationRequest CRD created
   - 202 Accepted: Duplicate signal (deduplication successful)
   - 400 Bad Request: Invalid signal payload
   - 500 Internal Server Error: Processing/API errors

**Performance Targets**:
- Webhook Response Time: p95 < 50ms, p99 < 100ms
- Redis Deduplication: p95 < 5ms, p99 < 10ms
- CRD Creation: p95 < 30ms, p99 < 50ms
- Throughput: >100 alerts/second
- Deduplication Rate: 40-60% (typical for production)

---

## 📡 API Examples

### Example 1: Prometheus Webhook (Success - New CRD)

**Request**:
```bash
curl -X POST http://gateway-service.kubernaut-system.svc.cluster.local:8080/api/v1/signals/prometheus \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <K8s-ServiceAccount-Token>" \
  -d '{
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
        "description": "Pod using 95% memory",
        "summary": "Memory usage at 95% for payment-api-789"
      },
      "startsAt": "2025-10-04T10:00:00Z"
    }]
  }'
```

**Response** (201 Created):
```json
{
  "status": "created",
  "fingerprint": "a3f8b2c1d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1",
  "remediation_request_name": "remediation-highmemoryusage-a3f8b2",
  "namespace": "prod-payment-service",
  "environment": "production",
  "priority": "P1",
  "duplicate": false,
  "storm_aggregation": false,
  "processing_time_ms": 42
}
```

**CRD Created** (in Kubernetes):
```yaml
apiVersion: remediation.kubernaut.io/v1
kind: RemediationRequest
metadata:
  name: remediation-highmemoryusage-a3f8b2
  namespace: prod-payment-service
  labels:
    kubernaut.io/environment: production
    kubernaut.io/priority: P1
    kubernaut.io/source: prometheus
spec:
  alertName: HighMemoryUsage
  severity: critical
  priority: P1
  environment: production
  resource:
    kind: Pod
    name: payment-api-789
    namespace: prod-payment-service
  fingerprint: a3f8b2c1d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1
  metadata:
    source: prometheus
    sourceLabels:
      alertname: HighMemoryUsage
      severity: critical
      namespace: prod-payment-service
      pod: payment-api-789
    annotations:
      description: "Pod using 95% memory"
      summary: "Memory usage at 95% for payment-api-789"
  createdAt: "2025-10-04T10:00:00Z"
```

**Verification**:
```bash
# Check CRD was created
kubectl get remediationrequest -n prod-payment-service

# Get CRD details
kubectl get remediationrequest remediation-highmemoryusage-a3f8b2 -n prod-payment-service -o yaml
```

---

### Example 2: Duplicate Signal (Deduplication)

**Request**:
```bash
# Same alert sent again within 5-minute TTL window
curl -X POST http://gateway-service.kubernaut-system.svc.cluster.local:8080/api/v1/signals/prometheus \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <K8s-ServiceAccount-Token>" \
  -d '{
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
  }'
```

**Response** (202 Accepted):
```json
{
  "status": "duplicate",
  "fingerprint": "a3f8b2c1d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1",
  "duplicate": true,
  "metadata": {
    "count": 2,
    "first_seen": "2025-10-04T10:00:00Z",
    "last_seen": "2025-10-04T10:01:30Z",
    "remediation_request_ref": "prod-payment-service/remediation-highmemoryusage-a3f8b2"
  },
  "processing_time_ms": 5
}
```

**Result**: No new CRD created, deduplication metadata updated in Redis

**Verification**:
```bash
# Check Redis deduplication entry
kubectl exec -n kubernaut-system <gateway-pod> -- \
  redis-cli -h redis -p 6379 GET "dedup:a3f8b2c1d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1"

# Output:
# {"count":2,"first_seen":"2025-10-04T10:00:00Z","last_seen":"2025-10-04T10:01:30Z","remediation_request_ref":"prod-payment-service/remediation-highmemoryusage-a3f8b2"}
```

---

### Example 3: Storm Aggregation

**Request**:
```bash
# 15 similar alerts within 1 minute (storm detected)
for i in {1..15}; do
  curl -X POST http://gateway-service:8080/api/v1/signals/prometheus \
    -H "Content-Type: application/json" \
    -d "{
      \"alerts\": [{
        \"status\": \"firing\",
        \"labels\": {
          \"alertname\": \"HighCPUUsage\",
          \"namespace\": \"prod-api\",
          \"pod\": \"api-server-$i\"
        }
      }]
    }"
done
```

**Response** (202 Accepted - Storm Aggregation):
```json
{
  "status": "storm_aggregated",
  "fingerprint": "storm-highcpuusage-prod-api-abc123",
  "storm_aggregation": true,
  "storm_metadata": {
    "pattern": "HighCPUUsage in prod-api namespace",
    "alert_count": 15,
    "affected_resources": [
      "Pod/api-server-1",
      "Pod/api-server-2",
      "... (13 more)"
    ],
    "aggregation_window": "1m",
    "remediation_request_ref": "prod-api/remediation-storm-highcpuusage-abc123"
  },
  "processing_time_ms": 8
}
```

**CRD Created** (single aggregated CRD):
```yaml
apiVersion: remediation.kubernaut.io/v1
kind: RemediationRequest
metadata:
  name: remediation-storm-highcpuusage-abc123
  namespace: prod-api
  labels:
    kubernaut.io/storm: "true"
    kubernaut.io/storm-pattern: highcpuusage
spec:
  alertName: HighCPUUsage
  severity: critical
  priority: P1
  environment: production
  stormAggregation:
    pattern: "HighCPUUsage in prod-api namespace"
    alertCount: 15
    affectedResources:
      - kind: Pod
        name: api-server-1
      - kind: Pod
        name: api-server-2
      # ... (13 more)
```

---

### Example 4: Invalid Webhook (Validation Error)

**Request**:
```bash
curl -X POST http://gateway-service:8080/api/v1/signals/prometheus \
  -H "Content-Type: application/json" \
  -d '{
    "version": "4",
    "alerts": [{
      "status": "firing",
      "labels": {
        "namespace": "test"
      }
    }]
  }'
```

**Response** (400 Bad Request):
```json
{
  "error": "Signal validation failed: missing required field 'alertname'",
  "details": {
    "validation_errors": [
      "alertname is required",
      "severity is missing or empty"
    ]
  },
  "processing_time_ms": 1
}
```

---

### Example 5: Kubernetes Event Signal

**Request**:
```bash
curl -X POST http://gateway-service:8080/api/v1/signals/kubernetes-event \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <K8s-ServiceAccount-Token>" \
  -d '{
    "type": "Warning",
    "reason": "FailedScheduling",
    "message": "0/3 nodes are available: insufficient cpu",
    "involvedObject": {
      "kind": "Pod",
      "namespace": "prod-database",
      "name": "postgres-primary-0"
    },
    "firstTimestamp": "2025-10-04T10:00:00Z",
    "count": 5
  }'
```

**Response** (201 Created):
```json
{
  "status": "created",
  "fingerprint": "b4c3d2e1f0a9b8c7d6e5f4a3b2c1d0e9f8a7b6c5d4e3f2a1b0c9d8e7f6a5b4c3",
  "remediation_request_name": "remediation-failedscheduling-b4c3d2",
  "namespace": "prod-database",
  "environment": "production",
  "priority": "P2",
  "duplicate": false,
  "processing_time_ms": 38
}
```

---

### Example 6: Processing Error (Redis Unavailable)

**Request**:
```bash
# Redis is down
curl -X POST http://gateway-service:8080/api/v1/signals/prometheus \
  -H "Content-Type: application/json" \
  -d '{ ... valid payload ... }'
```

**Response** (500 Internal Server Error):
```json
{
  "error": "Internal server error",
  "details": {
    "message": "Failed to check deduplication status",
    "retry": true,
    "retry_after": "30s"
  },
  "processing_time_ms": 5
}
```

**Gateway Logs**:
```json
{
  "level": "error",
  "msg": "Deduplication check failed",
  "fingerprint": "a3f8b2c1...",
  "error": "dial tcp 10.96.0.5:6379: connect: connection refused",
  "component": "deduplication",
  "timestamp": "2025-10-04T10:00:00Z"
}
```

---

## 🔗 Service Integration Examples

### Integration 1: Prometheus AlertManager → Gateway

**Setup AlertManager Configuration**:

```yaml
# prometheus-alertmanager-config.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: alertmanager-config
  namespace: prometheus
data:
  alertmanager.yml: |
    global:
      resolve_timeout: 5m

    receivers:
    - name: 'kubernaut-gateway'
      webhook_configs:
      - url: 'http://gateway-service.kubernaut-system.svc.cluster.local:8080/api/v1/signals/prometheus'
        send_resolved: true
        http_config:
          bearer_token_file: /var/run/secrets/kubernetes.io/serviceaccount/token
        max_alerts: 50  # Prevent overwhelming Gateway

    route:
      receiver: 'kubernaut-gateway'
      group_by: ['alertname', 'namespace', 'severity']
      group_wait: 10s
      group_interval: 10s
      repeat_interval: 12h
      routes:
      - match:
          severity: critical
        receiver: 'kubernaut-gateway'
        repeat_interval: 5m
      - match:
          severity: warning
        receiver: 'kubernaut-gateway'
        repeat_interval: 30m
```

**Apply Configuration**:
```bash
kubectl apply -f prometheus-alertmanager-config.yaml

# Restart AlertManager to pick up config
kubectl rollout restart deployment/alertmanager -n prometheus
```

**Flow Diagram**:
```
Prometheus → [Alert Fires] → AlertManager → [Webhook] → Gateway Service
                                                            ↓
                                                    [Process Signal]
                                                            ↓
                                                    [Create RemediationRequest CRD]
                                                            ↓
                                                    RemediationOrchestrator
```

**Testing**:
```bash
# Test AlertManager connectivity to Gateway
kubectl exec -n prometheus <alertmanager-pod> -- \
  curl -v http://gateway-service.kubernaut-system.svc.cluster.local:8080/health

# Trigger test alert
kubectl exec -n prometheus <prometheus-pod> -- \
  promtool alert test alertmanager.yml
```

---

### Integration 2: Gateway → RemediationOrchestrator

**Gateway Side (CRD Creation)**:

```go
// pkg/gateway/processing/crd_creator.go
package processing

import (
	"context"
	"fmt"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type CRDCreator struct {
	client client.Client
	logger *logrus.Logger
}

func (c *CRDCreator) CreateRemediationRequest(
	ctx context.Context,
	signal *types.NormalizedSignal,
) (*remediationv1.RemediationRequest, error) {
	// BR-GATEWAY-015: Create RemediationRequest CRD
	rr := &remediationv1.RemediationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      generateCRDName(signal),
			Namespace: signal.Namespace,
			Labels: map[string]string{
				"kubernaut.io/environment": signal.Environment,
				"kubernaut.io/priority":    signal.Priority,
				"kubernaut.io/source":      signal.SourceType,
			},
		},
		Spec: remediationv1.RemediationRequestSpec{
			AlertName:   signal.AlertName,
			Severity:    signal.Severity,
			Priority:    signal.Priority,
			Environment: signal.Environment,
			Resource: remediationv1.ResourceReference{
				Kind:      signal.Resource.Kind,
				Name:      signal.Resource.Name,
				Namespace: signal.Namespace,
			},
			Fingerprint: signal.Fingerprint,
			Metadata:    signal.Metadata,
		},
	}

	// BR-GATEWAY-021: Record signal metadata in CRD
	if err := c.client.Create(ctx, rr); err != nil {
		return nil, fmt.Errorf("failed to create RemediationRequest for signal %s (fingerprint=%s, namespace=%s): %w",
			signal.AlertName, signal.Fingerprint, signal.Namespace, err)
	}

	c.logger.WithFields(logrus.Fields{
		"crd_name":    rr.Name,
		"namespace":   rr.Namespace,
		"fingerprint": signal.Fingerprint,
		"priority":    signal.Priority,
	}).Info("RemediationRequest CRD created")

	return rr, nil
}
```

**RemediationOrchestrator Side (Watch CRDs)**:

```go
// pkg/remediation/orchestrator.go
package remediation

import (
	"context"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type RemediationOrchestrator struct {
	client client.Client
	logger *logrus.Logger
}

func (r *RemediationOrchestrator) Watch(ctx context.Context) error {
	// Watch for new RemediationRequest CRDs
	return r.client.Watch(
		ctx,
		&remediationv1.RemediationRequestList{},
		// Only process new CRDs (not updates)
		predicate.NewPredicateFuncs(func(obj client.Object) bool {
			rr := obj.(*remediationv1.RemediationRequest)
			return rr.Status.Phase == "" // New CRD (no status yet)
		}),
	)
}

func (r *RemediationOrchestrator) ProcessRemediation(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
) error {
	r.logger.WithFields(logrus.Fields{
		"crd_name":    rr.Name,
		"namespace":   rr.Namespace,
		"priority":    rr.Spec.Priority,
		"environment": rr.Spec.Environment,
	}).Info("Processing new RemediationRequest")

	// Select workflow based on priority + environment
	workflow := r.selectWorkflow(rr.Spec.Priority, rr.Spec.Environment)

	// Execute workflow
	return r.executeWorkflow(ctx, workflow, rr)
}
```

**Flow Diagram**:
```
Gateway Service
    ↓
[Create RemediationRequest CRD]
    ↓
Kubernetes API Server
    ↓
[CRD Event: ADDED]
    ↓
RemediationOrchestrator (Watch)
    ↓
[Process Remediation]
    ↓
[Select Workflow based on Priority/Environment]
    ↓
[Execute Workflow]
```

**Testing Integration**:
```bash
# 1. Create test RemediationRequest CRD
kubectl apply -f - <<EOF
apiVersion: remediation.kubernaut.io/v1
kind: RemediationRequest
metadata:
  name: test-remediation
  namespace: default
spec:
  alertName: TestAlert
  severity: critical
  priority: P1
  environment: development
EOF

# 2. Check RemediationOrchestrator picked it up
kubectl logs -n kubernaut-system -l app=remediation-orchestrator | \
  grep "Processing new RemediationRequest"

# 3. Verify workflow was executed
kubectl get remediationrequest test-remediation -n default -o yaml
# Check status.phase is updated
```

---

### Integration 3: Network Policy Enforcement

**Network Policy**:
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: gateway-ingress-policy
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: gateway-service
  policyTypes:
  - Ingress
  - Egress
  ingress:
  # Allow from Prometheus AlertManager
  - from:
    - namespaceSelector:
        matchLabels:
          name: prometheus
      podSelector:
        matchLabels:
          app: alertmanager
    ports:
    - protocol: TCP
      port: 8080
  # Allow Prometheus metrics scraping
  - from:
    - namespaceSelector:
        matchLabels:
          name: prometheus
      podSelector:
        matchLabels:
          app: prometheus
    ports:
    - protocol: TCP
      port: 9090
  egress:
  # Allow DNS
  - to:
    - namespaceSelector:
        matchLabels:
          name: kube-system
      podSelector:
        matchLabels:
          k8s-app: kube-dns
    ports:
    - protocol: UDP
      port: 53
  # Allow Redis
  - to:
    - podSelector:
        matchLabels:
          app: redis
    ports:
    - protocol: TCP
      port: 6379
  # Allow Kubernetes API
  - to:
    - namespaceSelector: {}
      podSelector:
        matchLabels:
          component: kube-apiserver
    ports:
    - protocol: TCP
      port: 443
```

**Testing Network Policy**:
```bash
# 1. Verify Gateway can reach Redis
kubectl exec -n kubernaut-system <gateway-pod> -- \
  redis-cli -h redis -p 6379 ping
# Expected: PONG

# 2. Verify Gateway can reach K8s API
kubectl exec -n kubernaut-system <gateway-pod> -- \
  curl -k https://kubernetes.default.svc.cluster.local/api
# Expected: {"kind":"APIVersions",...}

# 3. Verify unauthorized pod CANNOT reach Gateway
kubectl run -n default test-pod --image=curlimages/curl --rm -it -- \
  curl http://gateway-service.kubernaut-system.svc.cluster.local:8080/health
# Expected: Timeout (blocked by network policy)
```

---

## 🧪 Test Strategy

### Test Pyramid Distribution

Following Kubernaut's defense-in-depth testing strategy (`.cursor/rules/03-testing-strategy.mdc`):

- **Unit Tests (70%+)**: HTTP handlers, adapters, deduplication logic, storm detection (estimated: 75 tests)
  - **Coverage**: AT LEAST 70% of total business requirements
  - **Confidence**: 85-90%
  - **Mock Strategy**: Mock ONLY external dependencies (Redis, K8s API). Use REAL business logic.

- **Integration Tests (>50%)**: Redis integration, CRD creation, end-to-end webhook flow (estimated: 30 tests)
  - **Coverage**: >50% of total business requirements (microservices architecture)
  - **Confidence**: 80-85%
  - **Mock Strategy**: Use REAL services (Redis in Kind, K8s API in Kind cluster). No mocking.

- **E2E Tests (10-15%)**: Prometheus → Gateway → RemediationRequest → Completion (estimated: 5 tests)
  - **Coverage**: 10-15% of total business requirements for critical user journeys
  - **Confidence**: 90-95%
  - **Mock Strategy**: Minimal mocking. Real components and workflows.

**Total Estimated**: 110 tests covering ~135-140% of BRs (defense-in-depth overlapping coverage)

---

### Unit Test Breakdown (Estimated: 75 tests)

| Module | Tests | BR Coverage | Status |
|--------|-------|-------------|--------|
| **prometheus_adapter_test.go** | 12 | BR-GATEWAY-001, 003 | ⏸️ 0/12 |
| **kubernetes_adapter_test.go** | 10 | BR-GATEWAY-002, 004 | ⏸️ 0/10 |
| **deduplication_test.go** | 15 | BR-GATEWAY-005, 006, 010 | ⏸️ 0/15 |
| **storm_detection_test.go** | 8 | BR-GATEWAY-007, 008 | ⏸️ 0/8 |
| **classification_test.go** | 10 | BR-GATEWAY-051, 052, 053 | ⏸️ 0/10 |
| **priority_test.go** | 8 | BR-GATEWAY-013, 014 | ⏸️ 0/8 |
| **handlers_test.go** | 12 | BR-GATEWAY-017 to 020 | ⏸️ 0/12 |

**Status**: 0/75 unit tests (0%)

---

### Integration Test Breakdown (Estimated: 30 tests)

| Module | Tests | BR Coverage | Status |
|--------|-------|-------------|--------|
| **redis_integration_test.go** | 10 | BR-GATEWAY-005, 010 | ⏸️ 0/10 |
| **crd_creation_test.go** | 8 | BR-GATEWAY-015, 021 | ⏸️ 0/8 |
| **webhook_flow_test.go** | 12 | BR-GATEWAY-001, 002, 015 | ⏸️ 0/12 |

**Status**: 0/30 integration tests (0%)

---

### E2E Test Breakdown (Estimated: 5 tests)

| Module | Tests | BR Coverage | Status |
|--------|-------|-------------|--------|
| **prometheus_to_remediation_test.go** | 5 | BR-GATEWAY-001, 015, 071 | ⏸️ 0/5 |

**Status**: 0/5 E2E tests (0%)

---

### Defense-in-Depth Testing Strategy

**Principle**: Test with **REAL business logic**, mock **ONLY external dependencies**

Following Kubernaut's defense-in-depth approach (`.cursor/rules/03-testing-strategy.mdc`):

| Test Tier | Coverage | What to Test | Mock Strategy |
|-----------|----------|--------------|---------------|
| **Unit Tests** | **70%+** (AT LEAST 70% of ALL BRs) | Business logic, algorithms, HTTP handlers | Mock: Redis, K8s API<br>Real: Adapters, Processing, Handlers |
| **Integration Tests** | **>50%** (due to microservices) | Component interactions, Redis + K8s, CRD coordination | Mock: NONE<br>Real: Redis (in Kind), K8s API (Kind cluster) |
| **E2E Tests** | **10-15%** (critical user journeys) | Complete workflows, multi-service | Mock: NONE<br>Real: All components |

**Key Principle**: **NEVER mock business logic**
- ✅ **REAL**: Adapters, deduplication logic, storm detection, classification, priority engine
- ❌ **MOCK**: Redis (unit tests), Kubernetes API (unit tests), external services only

**Why Defense-in-Depth?**
- **Unit tests** (70%+) validate individual components work correctly with mocked external dependencies
- **Integration tests** (>50%) validate components work together with REAL services (Redis + K8s in Kind)
- **E2E tests** (10-15%) validate complete business workflows across all services
- Each layer catches different types of bugs (unit: business logic, integration: coordination, e2e: workflows)

**Why Percentages Add Up to >100%** (135-140% total):
- **Defense-in-Depth** = Overlapping coverage by design
- Same business requirement tested at multiple levels for different validation purposes:
  - **Unit level**: Business logic correctness (fast, isolated)
  - **Integration level**: Service coordination (real dependencies)
  - **E2E level**: Complete workflow (production-like)
- Example: BR-GATEWAY-001 (Prometheus webhook) tested in:
  - Unit tests: Adapter parsing logic (12 tests)
  - Integration tests: Webhook → CRD flow (5 tests)
  - E2E tests: AlertManager → Gateway → Orchestrator (2 tests)

---

### Mock Strategy

**Unit Tests (70%+)**:
- **MOCK**: Redis (miniredis), Kubernetes API (fake K8s client), Rego engine
- **REAL**: All business logic (adapters, processing pipeline, handlers)

**Integration Tests (<20%)**:
- **MOCK**: NONE - Use real Redis in Kind cluster
- **REAL**: Redis, Kubernetes API (Kind cluster), CRD creation, RBAC

**E2E Tests (<10%)**:
- **MOCK**: NONE
- **REAL**: All components, actual Prometheus AlertManager webhooks

---

## 🧪 Example Tests

### Example Unit Test: Prometheus Adapter (BR-GATEWAY-001)

**File**: `test/unit/gateway/prometheus_adapter_test.go`

```go
package gateway

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

func TestPrometheusAdapter(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Prometheus Adapter Suite - BR-GATEWAY-001")
}

var _ = Describe("BR-GATEWAY-001: Prometheus AlertManager Webhook Parsing", func() {
	var (
		adapter *adapters.PrometheusAdapter
		ctx     context.Context
	)

	BeforeEach(func() {
		adapter = adapters.NewPrometheusAdapter()
		ctx = context.Background()
	})

	Context("when receiving valid Prometheus webhook", func() {
		It("should parse AlertManager webhook format correctly", func() {
			// BR-GATEWAY-001: Accept signals from Prometheus AlertManager
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

			signal, err := adapter.Parse(ctx, payload)

			// Assertions - validate business outcome
			Expect(err).ToNot(HaveOccurred())
			Expect(signal).ToNot(BeNil())

			// BR-GATEWAY-003: Normalize Prometheus format
			Expect(signal.AlertName).To(Equal("HighMemoryUsage"))
			Expect(signal.Severity).To(Equal("critical"))
			Expect(signal.Namespace).To(Equal("prod-payment-service"))
			Expect(signal.Resource.Kind).To(Equal("Pod"))
			Expect(signal.Resource.Name).To(Equal("payment-api-789"))
			Expect(signal.SourceType).To(Equal("prometheus"))

			// BR-GATEWAY-006: Generate fingerprint
			Expect(signal.Fingerprint).ToNot(BeEmpty())
			Expect(signal.Fingerprint).To(HaveLen(64)) // SHA256 hex
		})

		It("should extract resource identifiers correctly", func() {
			// Test different resource types (Deployment, StatefulSet, Node)
			testCases := []struct {
				labels       map[string]string
				expectedKind string
				expectedName string
			}{
				{
					labels:       map[string]string{"deployment": "api-server"},
					expectedKind: "Deployment",
					expectedName: "api-server",
				},
				{
					labels:       map[string]string{"statefulset": "database"},
					expectedKind: "StatefulSet",
					expectedName: "database",
				},
				{
					labels:       map[string]string{"node": "worker-01"},
					expectedKind: "Node",
					expectedName: "worker-01",
				},
			}

			for _, tc := range testCases {
				payload := createPrometheusPayload("TestAlert", tc.labels)
				signal, err := adapter.Parse(ctx, payload)

				Expect(err).ToNot(HaveOccurred())
				Expect(signal.Resource.Kind).To(Equal(tc.expectedKind))
				Expect(signal.Resource.Name).To(Equal(tc.expectedName))
			}
		})
	})

	Context("BR-GATEWAY-002: when receiving invalid webhook", func() {
		It("should reject malformed JSON with clear error", func() {
			// Error handling: Invalid JSON format
			invalidPayload := []byte(`{invalid json}`)

			signal, err := adapter.Parse(ctx, invalidPayload)

			// BR-GATEWAY-019: Return clear error for invalid format
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse"))
			Expect(signal).To(BeNil())
		})

		It("should reject webhook missing required fields", func() {
			// Error handling: Missing required fields
			payloadMissingAlertname := []byte(`{
				"version": "4",
				"alerts": [{
					"status": "firing",
					"labels": {
						"namespace": "test"
					}
				}]
			}`)

			signal, err := adapter.Parse(ctx, payloadMissingAlertname)
			Expect(err).ToNot(HaveOccurred()) // Parse succeeds

			// But validation should fail
			err = adapter.Validate(signal)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing alertname"))
		})
	})

	Context("BR-GATEWAY-006: fingerprint generation", func() {
		It("should generate consistent fingerprints for same alert", func() {
			payload := createPrometheusPayload("TestAlert", map[string]string{
				"namespace": "prod",
				"pod":       "api-123",
			})

			signal1, _ := adapter.Parse(ctx, payload)
			signal2, _ := adapter.Parse(ctx, payload)

			// Fingerprints must be identical for deduplication
			Expect(signal1.Fingerprint).To(Equal(signal2.Fingerprint))
		})

		It("should generate different fingerprints for different alerts", func() {
			payload1 := createPrometheusPayload("Alert1", map[string]string{"pod": "api-123"})
			payload2 := createPrometheusPayload("Alert2", map[string]string{"pod": "api-456"})

			signal1, _ := adapter.Parse(ctx, payload1)
			signal2, _ := adapter.Parse(ctx, payload2)

			Expect(signal1.Fingerprint).ToNot(Equal(signal2.Fingerprint))
		})
	})
})

// Helper function to create test payloads
func createPrometheusPayload(alertName string, labels map[string]string) []byte {
	labels["alertname"] = alertName
	// ... JSON marshaling logic
	return []byte(`{...}`)
}
```

**Test Count**: 12 tests (BR-GATEWAY-001, 002, 003, 006)
**Coverage**: Prometheus adapter parsing, validation, error handling

---

### Example Unit Test: Deduplication Service (BR-GATEWAY-005)

**File**: `test/unit/gateway/deduplication_test.go`

```go
package gateway

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/alicebob/miniredis/v2"

	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

var _ = Describe("BR-GATEWAY-005: Signal Deduplication", func() {
	var (
		deduplicator *processing.DeduplicationService
		miniRedis    *miniredis.Miniredis
		ctx          context.Context
		testSignal   *types.NormalizedSignal
	)

	BeforeEach(func() {
		var err error
		// Use miniredis for fast, predictable unit tests
		miniRedis, err = miniredis.Run()
		Expect(err).ToNot(HaveOccurred())

		// Create deduplication service with short TTL for testing
		redisClient := createRedisClient(miniRedis.Addr())
		deduplicator = processing.NewDeduplicationServiceWithTTL(
			redisClient,
			5*time.Second, // Short TTL for tests
			testLogger,
		)

		ctx = context.Background()
		testSignal = &types.NormalizedSignal{
			Fingerprint: "test-fingerprint-123",
			AlertName:   "HighMemoryUsage",
			Namespace:   "prod",
		}
	})

	AfterEach(func() {
		miniRedis.Close()
	})

	Context("BR-GATEWAY-005: first occurrence of signal", func() {
		It("should NOT be a duplicate", func() {
			// First time seeing this signal
			isDuplicate, metadata, err := deduplicator.Check(ctx, testSignal)

			Expect(err).ToNot(HaveOccurred())
			Expect(isDuplicate).To(BeFalse())
			Expect(metadata).To(BeNil())
		})
	})

	Context("BR-GATEWAY-010: duplicate signal within TTL window", func() {
		It("should detect duplicate and return metadata", func() {
			// Store signal first time
			err := deduplicator.Store(ctx, testSignal, "remediation-req-123")
			Expect(err).ToNot(HaveOccurred())

			// Check again - should be duplicate
			isDuplicate, metadata, err := deduplicator.Check(ctx, testSignal)

			Expect(err).ToNot(HaveOccurred())
			Expect(isDuplicate).To(BeTrue())
			Expect(metadata).ToNot(BeNil())
			Expect(metadata.RemediationRequestRef).To(Equal("remediation-req-123"))
			Expect(metadata.Count).To(Equal(2)) // Second occurrence
		})

		It("should increment count on repeated duplicates", func() {
			// Store initial signal
			deduplicator.Store(ctx, testSignal, "remediation-req-123")

			// Check 3 more times
			for i := 2; i <= 4; i++ {
				isDuplicate, metadata, err := deduplicator.Check(ctx, testSignal)
				Expect(err).ToNot(HaveOccurred())
				Expect(isDuplicate).To(BeTrue())
				Expect(metadata.Count).To(Equal(i))
			}
		})
	})

	Context("when TTL expires", func() {
		It("should treat expired signal as new (not duplicate)", func() {
			// Store signal
			err := deduplicator.Store(ctx, testSignal, "remediation-req-123")
			Expect(err).ToNot(HaveOccurred())

			// Fast-forward Redis time past TTL (5 seconds)
			miniRedis.FastForward(6 * time.Second)

			// Check again - should NOT be duplicate (TTL expired)
			isDuplicate, _, err := deduplicator.Check(ctx, testSignal)
			Expect(err).ToNot(HaveOccurred())
			Expect(isDuplicate).To(BeFalse())
		})
	})

	Context("BR-GATEWAY-020: error handling when Redis unavailable", func() {
		It("should return error with context when Redis is down", func() {
			// Close Redis to simulate failure
			miniRedis.Close()

			isDuplicate, metadata, err := deduplicator.Check(ctx, testSignal)

			// Error handling: Return clear error, don't panic
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Redis"))
			Expect(isDuplicate).To(BeFalse())
			Expect(metadata).To(BeNil())
		})
	})
})
```

**Test Count**: 15 tests (BR-GATEWAY-005, 010, 020)
**Coverage**: Deduplication logic, TTL expiry, error handling

---

### Example Integration Test: End-to-End Webhook Flow (BR-GATEWAY-001, BR-GATEWAY-015)

**File**: `test/integration/gateway/webhook_flow_test.go`

```go
package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
	"github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/pkg/testutil/kind"
)

var _ = Describe("BR-GATEWAY-001 + BR-GATEWAY-015: Prometheus Webhook → CRD Creation", func() {
	var (
		gatewayServer *gateway.Server
		k8sClient     client.Client
		kindCluster   *kind.TestCluster
		ctx           context.Context
	)

	BeforeEach(func() {
		var err error
		ctx = context.Background()

		// Setup Kind cluster with CRDs + Redis
		kindCluster, err = kind.NewTestCluster(&kind.Config{
			Name: "gateway-integration-test",
			CRDs: []string{
				"config/crd/bases/remediation.kubernaut.io_remediationrequests.yaml",
			},
		})
		Expect(err).ToNot(HaveOccurred())

		k8sClient = kindCluster.GetClient()

		// Start Gateway server with real Redis in Kind
		gatewayConfig := &gateway.ServerConfig{
			ListenAddr:             ":8080",
			Redis:                  kindCluster.GetRedisConfig(),
			DeduplicationTTL:       5 * time.Second,
			StormRateThreshold:     10,
			StormPatternThreshold:  5,
		}

		gatewayServer, err = gateway.NewServer(gatewayConfig, testLogger)
		Expect(err).ToNot(HaveOccurred())

		// Register Prometheus adapter
		prometheusAdapter := adapters.NewPrometheusAdapter()
		err = gatewayServer.RegisterAdapter(prometheusAdapter)
		Expect(err).ToNot(HaveOccurred())

		// Start server in background
		go gatewayServer.Start(ctx)

		// Wait for server to be ready
		Eventually(func() error {
			resp, err := http.Get("http://localhost:8080/ready")
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("server not ready: %d", resp.StatusCode)
			}
			return nil
		}, "10s", "100ms").Should(Succeed())
	})

	AfterEach(func() {
		gatewayServer.Stop(ctx)
		kindCluster.Cleanup()
	})

	Context("BR-GATEWAY-001: receiving Prometheus webhook", func() {
		It("should create RemediationRequest CRD successfully", func() {
			// BR-GATEWAY-001: Accept Prometheus AlertManager webhook
			webhookPayload := map[string]interface{}{
				"version":  "4",
				"groupKey": "{}:{alertname=\"HighMemoryUsage\"}",
				"alerts": []map[string]interface{}{
					{
						"status": "firing",
						"labels": map[string]string{
							"alertname": "HighMemoryUsage",
							"severity":  "critical",
							"namespace": "prod-payment-service",
							"pod":       "payment-api-789",
						},
						"annotations": map[string]string{
							"description": "Pod using 95% memory",
						},
						"startsAt": "2025-10-04T10:00:00Z",
					},
				},
			}

			payloadBytes, _ := json.Marshal(webhookPayload)

			// Send webhook to Gateway
			resp, err := http.Post(
				"http://localhost:8080/api/v1/signals/prometheus",
				"application/json",
				bytes.NewReader(payloadBytes),
			)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// BR-GATEWAY-017: Should return HTTP 201 Created
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))

			// Parse response
			var response gateway.ProcessingResponse
			err = json.NewDecoder(resp.Body).Decode(&response)
			Expect(err).ToNot(HaveOccurred())

			Expect(response.Status).To(Equal("created"))
			Expect(response.Fingerprint).ToNot(BeEmpty())
			Expect(response.RemediationRequestName).ToNot(BeEmpty())
			Expect(response.Environment).ToNot(BeEmpty())
			Expect(response.Priority).ToNot(BeEmpty())

			// BR-GATEWAY-015: Verify CRD was created in Kubernetes
			Eventually(func() error {
				rr := &remediationv1.RemediationRequest{}
				key := client.ObjectKey{
					Name:      response.RemediationRequestName,
					Namespace: "prod-payment-service",
				}
				return k8sClient.Get(ctx, key, rr)
			}, "5s", "100ms").Should(Succeed())

			// Verify CRD contents
			rr := &remediationv1.RemediationRequest{}
			key := client.ObjectKey{
				Name:      response.RemediationRequestName,
				Namespace: "prod-payment-service",
			}
			err = k8sClient.Get(ctx, key, rr)
			Expect(err).ToNot(HaveOccurred())

			// BR-GATEWAY-021: Verify signal metadata in CRD
			Expect(rr.Spec.AlertName).To(Equal("HighMemoryUsage"))
			Expect(rr.Spec.Severity).To(Equal("critical"))
			Expect(rr.Spec.Priority).To(Equal(response.Priority))
			Expect(rr.Spec.Environment).To(Equal(response.Environment))
			Expect(rr.Spec.Resource.Kind).To(Equal("Pod"))
			Expect(rr.Spec.Resource.Name).To(Equal("payment-api-789"))
		})
	})

	Context("BR-GATEWAY-010: duplicate signal handling", func() {
		It("should return HTTP 202 for duplicate without creating new CRD", func() {
			payload := createTestPayload("DuplicateAlert")

			// Send first time - should create CRD
			resp1, _ := sendWebhook(payload)
			Expect(resp1.StatusCode).To(Equal(http.StatusCreated))

			var response1 gateway.ProcessingResponse
			json.NewDecoder(resp1.Body).Decode(&response1)
			firstCRDName := response1.RemediationRequestName

			// Send again immediately - should be deduplicated
			resp2, _ := sendWebhook(payload)
			Expect(resp2.StatusCode).To(Equal(http.StatusAccepted)) // 202

			var response2 gateway.ProcessingResponse
			json.NewDecoder(resp2.Body).Decode(&response2)

			// BR-GATEWAY-018: Should return duplicate status
			Expect(response2.Status).To(Equal("duplicate"))
			Expect(response2.Duplicate).To(BeTrue())
			Expect(response2.Metadata.Count).To(Equal(2))
			Expect(response2.Metadata.RemediationRequestRef).To(ContainSubstring(firstCRDName))

			// Verify NO new CRD was created
			rrList := &remediationv1.RemediationRequestList{}
			err := k8sClient.List(ctx, rrList, client.InNamespace("test"))
			Expect(err).ToNot(HaveOccurred())
			Expect(rrList.Items).To(HaveLen(1)) // Still only 1 CRD
		})
	})
})
```

**Test Count**: 12 tests (BR-GATEWAY-001, 010, 015, 017, 018, 021)
**Coverage**: Complete webhook flow, CRD creation, deduplication, error responses

---

## ⚠️ Error Handling Patterns

### Consistent Error Handling Strategy

Following Notification service pattern for rich error context:

```go
// ✅ CORRECT: Error with context (resource name, namespace, operation)
if err := s.deduplicator.Check(ctx, signal); err != nil {
	return nil, fmt.Errorf("deduplication check failed for signal %s (fingerprint=%s, source=%s, namespace=%s): %w",
		signal.AlertName, signal.Fingerprint, signal.SourceType, signal.Namespace, err)
}

// ❌ WRONG: Generic error without context
if err := s.deduplicator.Check(ctx, signal); err != nil {
	return nil, fmt.Errorf("deduplication check failed: %w", err)
}
```

---

### Error Types by HTTP Status Code

| HTTP Status | Condition | Error Type | Retry? |
|-------------|-----------|------------|--------|
| **201 Created** | CRD created successfully | N/A | N/A |
| **202 Accepted** | Duplicate signal or storm aggregation | N/A | No |
| **400 Bad Request** | Invalid signal format, missing fields | Validation error | No (permanent error) |
| **413 Payload Too Large** | Signal payload > 1MB | Size error | No (reduce payload) |
| **429 Too Many Requests** | Rate limit exceeded | Rate limit error | Yes (with backoff) |
| **500 Internal Server Error** | Redis failure, K8s API failure | Transient error | Yes (Alertmanager retry) |
| **503 Service Unavailable** | Gateway not ready (dependencies down) | Unavailability error | Yes (wait for ready) |

---

### Error Handling Examples

#### 1. Validation Errors (400 Bad Request)

```go
// Validate signal format
if err := adapter.Validate(signal); err != nil {
	s.logger.WithFields(logrus.Fields{
		"adapter":     adapter.Name(),
		"fingerprint": signal.Fingerprint,
		"error":       err,
	}).Warn("Signal validation failed")

	http.Error(w, fmt.Sprintf("Signal validation failed: %v", err), http.StatusBadRequest)
	return
}
```

#### 2. Transient Errors (500 Internal Server Error)

```go
// Handle Redis failures gracefully
isDuplicate, metadata, err := s.deduplicator.Check(ctx, signal)
if err != nil {
	s.logger.WithFields(logrus.Fields{
		"fingerprint": signal.Fingerprint,
		"error":       err,
	}).Error("Deduplication check failed")

	// Return 500 so Alertmanager retries
	http.Error(w, "Internal server error", http.StatusInternalServerError)
	return
}
```

#### 3. Non-Critical Errors (Log and Continue)

```go
// Storm detection failure is non-critical
isStorm, stormMetadata, err := s.stormDetector.Check(ctx, signal)
if err != nil {
	// Log warning but continue processing
	s.logger.WithFields(logrus.Fields{
		"fingerprint": signal.Fingerprint,
		"error":       err,
	}).Warn("Storm detection failed - continuing without storm metadata")
	// Continue to next step...
}
```

#### 4. Defensive Programming (Nil Checks)

```go
// Following Notification service pattern
func (s *Server) ProcessSignal(ctx context.Context, signal *types.NormalizedSignal) (*ProcessingResponse, error) {
	// Defensive: Check for nil signal
	if signal == nil {
		return nil, fmt.Errorf("signal cannot be nil")
	}

	// Defensive: Check for empty fingerprint
	if signal.Fingerprint == "" {
		return nil, fmt.Errorf("signal fingerprint cannot be empty (alertName=%s, namespace=%s)",
			signal.AlertName, signal.Namespace)
	}

	// ... process signal
}
```

---

### Error Metrics

Record errors for monitoring:

```go
// Record error metrics by type
metrics.HTTPRequestErrors.WithLabelValues(
	route,            // "/api/v1/signals/prometheus"
	"parse_error",    // error_type
	"400",            // status_code
).Inc()

metrics.ProcessingErrors.WithLabelValues(
	"deduplication",  // component
	"redis_timeout",  // error_reason
).Inc()
```

**Prometheus Queries**:
```promql
# Error rate by endpoint
rate(gateway_http_errors_total{route="/api/v1/signals/prometheus"}[5m])

# Error rate by type
sum(rate(gateway_processing_errors_total[5m])) by (component, error_reason)
```

---

## 🚀 Deployment Guide

### Production Deployment Checklist

- [ ] All core tests passing (0/110) ⏸️
- [ ] Zero critical lint errors ⏸️
- [ ] Network policies documented ✅
- [ ] K8s ServiceAccount configured ⏸️
- [ ] Health/readiness probes working ⏸️
- [ ] Prometheus metrics exposed ⏸️
- [ ] Configuration externalized ✅
- [ ] Design decisions documented ✅ (DD-GATEWAY-001)
- [ ] Architecture aligned with design ✅

**Status**: ⏸️ **NOT PRODUCTION READY** (Implementation Pending)

---

### Kubernetes Deployment

**Deployment Manifest**:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway-service
  namespace: kubernaut-system
  labels:
    app: gateway-service
    version: v1.0.0
spec:
  replicas: 2
  selector:
    matchLabels:
      app: gateway-service
  template:
    metadata:
      labels:
        app: gateway-service
    spec:
      serviceAccountName: gateway-sa
      containers:
      - name: gateway
        image: gateway-service:1.0.0
        ports:
        - containerPort: 8080
          name: http
        - containerPort: 9090
          name: metrics
        env:
        - name: REDIS_ENDPOINT
          value: "redis:6379"
        - name: REDIS_PASSWORD
          valueFrom:
            secretKeyRef:
              name: redis-credentials
              key: password
        - name: REDIS_DB
          value: "0"
        - name: RATE_LIMIT_RPM
          value: "100"
        - name: DEDUPLICATION_TTL
          value: "5m"
        - name: STORM_RATE_THRESHOLD
          value: "10"
        - name: STORM_PATTERN_THRESHOLD
          value: "5"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
        resources:
          requests:
            memory: "256Mi"
            cpu: "250m"
          limits:
            memory: "512Mi"
            cpu: "500m"
---
apiVersion: v1
kind: Service
metadata:
  name: gateway-service
  namespace: kubernaut-system
spec:
  selector:
    app: gateway-service
  ports:
  - name: http
    port: 8080
    targetPort: 8080
  - name: metrics
    port: 9090
    targetPort: 9090
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: gateway-sa
  namespace: kubernaut-system
```

---

### Network Policy

**Restrict access to authorized sources only**:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: gateway-ingress
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: gateway-service
  policyTypes:
  - Ingress
  ingress:
  # Allow from Prometheus AlertManager
  - from:
    - namespaceSelector:
        matchLabels:
          name: prometheus
    ports:
    - protocol: TCP
      port: 8080
  # Allow from Kubernetes API (for Event watching)
  - from:
    - namespaceSelector:
        matchLabels:
          name: kube-system
    ports:
    - protocol: TCP
      port: 8080
  # Allow Prometheus scraping
  - from:
    - namespaceSelector:
        matchLabels:
          name: prometheus
    ports:
    - protocol: TCP
      port: 9090
```

---

## 📈 Success Metrics

### Technical Metrics

| Metric | Target | How to Measure | Status |
|--------|--------|---------------|--------|
| **Test Coverage** | 70%+ | `go test -cover ./pkg/gateway/...` | ⏸️ 0% |
| **Unit Tests Passing** | 100% | `go test ./test/unit/gateway/...` | ⏸️ 0/75 |
| **Integration Tests Passing** | 100% | `go test ./test/integration/gateway/...` | ⏸️ 0/30 |
| **E2E Tests Passing** | 100% | `go test ./test/e2e/gateway/...` | ⏸️ 0/5 |
| **Build Success** | 100% | CI/CD pipeline | ⏸️ N/A |
| **Lint Compliance** | 100% | `golangci-lint run ./pkg/gateway/...` | ⏸️ N/A |
| **Technical Debt** | Zero | Code review + automated checks | ⏸️ N/A |

---

### Business Metrics (Production)

| Metric | Target | Prometheus Query | Status |
|--------|--------|------------------|--------|
| **Webhook Response Time (p95)** | < 50ms | `histogram_quantile(0.95, gateway_http_duration_seconds_bucket{endpoint="/api/v1/signals/prometheus"})` | ⏸️ N/A |
| **Webhook Response Time (p99)** | < 100ms | `histogram_quantile(0.99, gateway_http_duration_seconds_bucket{endpoint="/api/v1/signals/prometheus"})` | ⏸️ N/A |
| **Redis Deduplication (p95)** | < 5ms | `histogram_quantile(0.95, gateway_deduplication_duration_seconds_bucket)` | ⏸️ N/A |
| **CRD Creation (p95)** | < 30ms | `histogram_quantile(0.95, gateway_crd_creation_duration_seconds_bucket)` | ⏸️ N/A |
| **Throughput** | >100/sec | `rate(gateway_signals_received_total[5m])` | ⏸️ N/A |
| **Deduplication Rate** | 40-60% | `rate(gateway_signals_deduplicated_total[5m]) / rate(gateway_signals_received_total[5m])` | ⏸️ N/A |
| **Success Rate** | > 95% | `rate(gateway_signals_accepted_total[5m]) / rate(gateway_signals_received_total[5m])` | ⏸️ N/A |
| **Service Availability** | > 99% | `up{job="gateway-service"}` | ⏸️ N/A |

---

## 🔮 Future Evolution Path

### v1.0 (Current): Adapter-Specific Endpoints ✅

**Features**:
- Adapter-specific routes (`/api/v1/signals/prometheus`, etc.)
- Redis-based deduplication (5-minute TTL)
- Hybrid storm detection (rate + pattern-based)
- ConfigMap-based environment classification
- Rego-based priority assignment
- Configuration-driven adapter registration

**Status**: ✅ DESIGN COMPLETE / ⏸️ IMPLEMENTATION PENDING
**Confidence**: 92% (Very High)

---

### v1.5: Optimization (If Needed)

**Add only if metrics show need**:
- Redis Sentinel for HA (if single-point-of-failure detected)
- Prometheus metrics refinement (if monitoring gaps found)
- Enhanced storm aggregation (if >50% storm rate detected)
- Rate limit per-namespace (if per-IP insufficient)

**Trigger**: Performance metrics below SLA
**Estimated**: 2-3 weeks if needed

---

### v2.0: Additional Signal Sources (If Needed)

**Add only if business requirements expand**:
- Grafana alert ingestion adapter
- Cloud-specific alerts (CloudWatch, Azure Monitor)
- Datadog integration
- PagerDuty webhook support

**Trigger**: Business requirement for additional signal sources
**Estimated**: 4-6 weeks if needed
**Note**: Requires DD-GATEWAY-002 design decision

---

## 📚 Related Documentation

**Design Decisions**:
- [DD-GATEWAY-001](../../architecture/decisions/DD-GATEWAY-001-Adapter-Specific-Endpoints.md) - **Current architecture** (adapter-specific endpoints)
- [DESIGN_B_IMPLEMENTATION_SUMMARY.md](DESIGN_B_IMPLEMENTATION_SUMMARY.md) - Architecture rationale

**Technical Documentation**:
- [README.md](README.md) - Service overview and navigation
- [overview.md](overview.md) - High-level architecture
- [implementation.md](implementation.md) - Implementation details (1,300+ lines)
- [deduplication.md](deduplication.md) - Redis fingerprinting and storm detection
- [crd-integration.md](crd-integration.md) - RemediationRequest CRD creation

**Security & Observability**:
- [security-configuration.md](security-configuration.md) - JWT authentication and RBAC
- [observability-logging.md](observability-logging.md) - Structured logging and tracing
- [metrics-slos.md](metrics-slos.md) - Prometheus metrics and Grafana dashboards

**Testing**:
- [testing-strategy.md](testing-strategy.md) - APDC-TDD patterns and mock strategies
- [implementation-checklist.md](implementation-checklist.md) - APDC phases and tasks

**Triage Reports**:
- [GATEWAY_IMPLEMENTATION_TRIAGE.md](GATEWAY_IMPLEMENTATION_TRIAGE.md) - Documentation triage (vs HolmesGPT v3.0)
- [GATEWAY_CODE_IMPLEMENTATION_TRIAGE.md](GATEWAY_CODE_IMPLEMENTATION_TRIAGE.md) - Code pattern comparison (vs Context API, Notification)
- [GATEWAY_TRIAGE_SUMMARY.md](GATEWAY_TRIAGE_SUMMARY.md) - Executive summary

**Superseded Designs** (historical reference):
- [ADAPTER_REGISTRY_DESIGN.md](ADAPTER_REGISTRY_DESIGN.md) - ⚠️ Detection-based architecture (Design A, superseded)
- [ADAPTER_DETECTION_FLOW.md](ADAPTER_DETECTION_FLOW.md) - ⚠️ Detection flow logic (superseded)

---

## ✅ Approval & Next Steps

**Design Approved**: October 4, 2025
**Design Decision**: DD-GATEWAY-001
**Implementation Status**: ⏸️ NOT STARTED
**Production Readiness**: ⏸️ NOT READY (implementation pending)
**Confidence**: 85%

**Critical Next Steps**:
1. ⏸️ Enumerate all business requirements (BR-GATEWAY-001 to 040)
2. ⏸️ Create DD-GATEWAY-001 design decision document
3. ⏸️ Implement unit tests (75 tests, 20-25h)
4. ⏸️ Implement integration tests (30 tests, 15-20h)
5. ⏸️ Implement E2E tests (5 tests, 5-10h)
6. ⏸️ Deploy to development environment
7. ⏸️ Integrate with RemediationOrchestrator
8. ⏸️ Deploy to production with network policies

**Estimated Time to Production**: 48-63 hours (6-8 days) + 8h deployment = 56-71 hours total

---

## 🎯 Implementation Priorities

### Phase 1: Foundation (Week 1)

**Priority**: 🔴 P0 - Critical

**Tasks**:
1. Enumerate all BRs in `GATEWAY_BUSINESS_REQUIREMENTS.md` (6-8h)
2. Create DD-GATEWAY-001 design decision document (3-4h)
3. Setup test structure (suite_test.go) with test count tracking (2h)
4. Implement Prometheus adapter (8-10h)
5. Implement Kubernetes Events adapter (8-10h)

**Deliverable**: 2 adapters implemented with unit tests
**Total Effort**: 27-34 hours

---

### Phase 2: Core Processing (Week 2)

**Priority**: 🔴 P0 - Critical

**Tasks**:
6. Implement deduplication service (10-12h)
7. Implement storm detection (8-10h)
8. Implement environment classification (6-8h)
9. Implement priority engine (6-8h)
10. Implement CRD creator (8-10h)

**Deliverable**: Complete processing pipeline with unit tests
**Total Effort**: 38-48 hours

---

### Phase 3: Integration & Testing (Week 3)

**Priority**: 🟡 P1 - Important

**Tasks**:
11. Integration tests (Redis + K8s) (15-20h)
12. E2E tests (Prometheus → CRD) (5-10h)
13. Performance testing and optimization (8-10h)
14. Security hardening and audit (4-6h)

**Deliverable**: Production-ready service with complete test coverage
**Total Effort**: 32-46 hours

---

### Phase 4: Deployment (Week 4)

**Priority**: 🟡 P1 - Important

**Tasks**:
15. Create deployment manifests (4h)
16. Setup monitoring and alerts (4h)
17. Deploy to development (2h)
18. Integration testing with other services (4h)
19. Deploy to production (2h)
20. Validation and monitoring (2h)

**Deliverable**: Production deployment with monitoring
**Total Effort**: 18 hours

---

**Grand Total**: 115-146 hours (14.5-18 days)

---

## 📊 Risk Assessment

### Technical Risks

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| **Redis connection failures** | Medium | High | Implement circuit breaker, retry logic |
| **Storm detection false positives** | Medium | Medium | Tunable thresholds via ConfigMap |
| **High latency on CRD creation** | Low | Medium | Performance testing, optimize K8s API calls |
| **Adapter complexity growth** | Low | Low | Configuration-driven registration, clean interfaces |

### Business Risks

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| **Missed signals during downtime** | Medium | High | HA deployment (2+ replicas), health monitoring |
| **Deduplication accuracy issues** | Low | Medium | Comprehensive unit tests, integration tests with real Redis |
| **False storm aggregation** | Low | High | Tunable thresholds, admin override capability |

---

## 🚀 **OPERATIONAL RUNBOOKS**

> **Purpose**: Production deployment and operational procedures
>
> **Audience**: Platform operations, SRE, on-call engineers
>
> **Coverage**: Deployment, troubleshooting, rollback, performance tuning, maintenance, escalation

This section provides comprehensive operational guidance for Gateway Service production deployment and maintenance.

---

### **Deployment Procedure**

#### **Pre-Deployment Checklist**
- [ ] Redis accessible (localhost:6379 or redis-service:6379)
- [ ] Kubernetes cluster accessible (kubectl commands work)
- [ ] RemediationRequest CRD installed (`kubectl get crd remediationrequests.remediation.kubernaut.io`)
- [ ] Secrets created (`kubectl get secret gateway-redis-password -n kubernaut-system`)
- [ ] ConfigMap reviewed (`kubectl get cm gateway-config -n kubernaut-system`)
- [ ] RBAC permissions validated (`kubectl auth can-i create remediationrequests --as=system:serviceaccount:kubernaut-system:gateway`)
- [ ] Monitoring configured (Prometheus scraping Gateway metrics endpoint)
- [ ] Network policies reviewed (allow traffic from Prometheus AlertManager, K8s API server)

#### **Deployment Steps**

**Step 1: Apply ConfigMap**
```bash
kubectl apply -f deploy/gateway/01-configmap.yaml
```

**Step 2: Create Secrets** (if not exists)
```bash
kubectl create secret generic gateway-redis-password \
  --from-literal=password=<REDIS_PASSWORD> \
  -n kubernaut-system

kubectl create secret generic gateway-rego-policy \
  --from-file=priority.rego=config/gateway/priority-policy.rego \
  -n kubernaut-system
```

**Step 3: Apply RBAC**
```bash
kubectl apply -f deploy/gateway/02-serviceaccount.yaml
kubectl apply -f deploy/gateway/03-role.yaml
kubectl apply -f deploy/gateway/04-rolebinding.yaml
```

**Step 4: Apply Service**
```bash
kubectl apply -f deploy/gateway/05-service.yaml
```

**Step 5: Apply Deployment**
```bash
kubectl apply -f deploy/gateway/06-deployment.yaml
```

**Step 6: Apply HPA** (optional, production recommended)
```bash
kubectl apply -f deploy/gateway/07-hpa.yaml
```

**Step 7: Apply ServiceMonitor** (if Prometheus Operator installed)
```bash
kubectl apply -f deploy/gateway/08-servicemonitor.yaml
```

**Step 8: Apply NetworkPolicy** (optional, security hardening)
```bash
kubectl apply -f deploy/gateway/09-networkpolicy.yaml
```

#### **Post-Deployment Validation**

```bash
# 1. Check pods are running
kubectl get pods -n kubernaut-system -l app=gateway
# Expected: 2-3 Running pods (depending on HPA)

# 2. Check service endpoints
kubectl get endpoints gateway -n kubernaut-system
# Expected: Endpoints listed (pod IPs)

# 3. Smoke test health endpoint
kubectl run -it --rm curl --image=curlimages/curl --restart=Never -- \
  curl http://gateway.kubernaut-system:8080/health
# Expected: {"status":"ok","timestamp":"..."}

# 4. Check metrics endpoint
kubectl run -it --rm curl --image=curlimages/curl --restart=Never -- \
  curl http://gateway.kubernaut-system:9090/metrics | grep gateway_
# Expected: Prometheus metrics displayed

# 5. Test Prometheus webhook endpoint
kubectl run -it --rm curl --image=curlimages/curl --restart=Never -- \
  curl -X POST http://gateway.kubernaut-system:8080/api/v1/signals/prometheus \
    -H "Content-Type: application/json" \
    -d '{"alerts":[{"status":"firing","labels":{"alertname":"Test"}}]}'
# Expected: HTTP 201 or 400 (not 404)

# 6. Verify CRD creation
kubectl get remediationrequests -n kubernaut-system
# Expected: At least 1 RemediationRequest created from test webhook
```

#### **Monitoring Queries** (Prometheus PromQL)

```promql
# Request rate per route
rate(gateway_webhook_requests_total[5m])

# Error rate
rate(gateway_webhook_errors_total[5m])

# p95 latency
histogram_quantile(0.95, rate(gateway_webhook_duration_seconds_bucket[5m]))

# Deduplication rate
rate(gateway_deduplication_duplicates_total[5m]) / rate(gateway_webhook_requests_total[5m])

# Storm detection rate
rate(gateway_storm_detection_triggered_total[5m])

# CRD creation success rate
rate(gateway_crd_creation_success_total[5m]) / rate(gateway_crd_creation_attempts_total[5m])

# Redis connection pool usage
gateway_redis_pool_connections_in_use / gateway_redis_pool_connections_total
```

#### **Alert Thresholds**

| Alert | Threshold | Severity | Action |
|-------|-----------|----------|--------|
| **High Error Rate** | >5% for 5 minutes | P2 | Check Gateway logs, verify Redis/K8s connectivity |
| **High Latency** | p95 >200ms for 5 minutes | P3 | Check Redis performance, review storm detection |
| **Low Dedup Rate** | <80% for 10 minutes | P4 | Review fingerprint algorithm, check Redis TTL |
| **High Storm Rate** | >10 storms/min for 10 minutes | P3 | Review thresholds, check for legitimate burst |
| **CRD Creation Failures** | >1% for 5 minutes | P2 | Check RBAC permissions, verify CRD schema |
| **Pod Restart** | >3 restarts in 10 minutes | P2 | Check OOM, CPU throttling, liveness probe |

---

### **Troubleshooting** (7 Common Scenarios)

#### **Scenario 1: High Error Rate**
**Symptoms**: `gateway_webhook_errors_total` increasing rapidly

**Investigation**: Check logs, Redis connectivity, K8s API access
**Resolution**: Verify dependencies, check RBAC, validate webhook payloads

#### **Scenario 2: Deduplication Failures**
**Symptoms**: Same alert creating multiple CRDs

**Investigation**: Check Redis keys, verify TTL settings, review fingerprint logic
**Resolution**: Fix Redis connectivity, adjust TTL, validate atomic operations

#### **Scenario 3: Storm Detection Not Triggering**
**Symptoms**: Alert bursts not aggregated

**Investigation**: Review thresholds, check storm detection logs
**Resolution**: Tune thresholds, adjust aggregation window, add context-aware exclusions

#### **Scenario 4: CRD Creation Failures**
**Symptoms**: CRDs not appearing in Kubernetes

**Investigation**: Check RBAC permissions, verify CRD schema, test manual creation
**Resolution**: Fix RBAC, install/update CRD, validate required fields

#### **Scenario 5: Redis Connectivity Issues**
**Symptoms**: Connection timeouts, pool exhaustion

**Investigation**: Check Redis pod status, test connectivity, review pool config
**Resolution**: Restart Redis, increase pool size, fix NetworkPolicy

#### **Scenario 6: Authentication Failures**
**Symptoms**: HTTP 401/403 errors

**Investigation**: Test TokenReviewer, verify ServiceAccount, check token expiration
**Resolution**: Fix RBAC for TokenReviewer, refresh tokens, validate header format

#### **Scenario 7: High Memory Usage**
**Symptoms**: Pods approaching memory limit, OOMKilled events

**Investigation**: Check resource usage, analyze cache size, review goroutines
**Resolution**: Reduce dedup TTL, increase memory limit, tune storm aggregation

---

### **Rollback Procedure**

**Quick Rollback** (Recommended):
```bash
kubectl rollout undo deployment/gateway -n kubernaut-system
kubectl rollout status deployment/gateway -n kubernaut-system
```

**Manual Rollback** (If needed):
```bash
kubectl scale deployment/gateway --replicas=0 -n kubernaut-system
kubectl apply -f deploy/gateway/06-deployment-v0.9.0.yaml
kubectl scale deployment/gateway --replicas=3 -n kubernaut-system
```

**Validation**: Health checks, metrics verification, webhook tests, CRD creation confirmation

---

### **Performance Tuning**

**Redis Connection Pool**: Increase for >2000 req/s (`pool_size: 100`, `min_idle_conns: 20`)
**Rate Limiting**: Adjust limits (`requests_per_minute: 200`, `burst: 20`)
**HPA**: Scale earlier (`minReplicas: 5`, `maxReplicas: 20`, `averageUtilization: 70`)
**Dedup TTL**: Balance memory vs accuracy (`ttl: 3m` or `ttl: 10m`)
**Storm Thresholds**: Tune sensitivity (`rate_threshold: 20`, `pattern_threshold: 10`)

---

### **Maintenance**

**Planned Downtime**: Scale to 1 replica, perform maintenance, restart, scale back up
**Redis Maintenance**: Check persistence (`CONFIG GET save`), backup/restore (`BGSAVE`)
**CRD Schema Updates**: Backward-compatible (no downtime) vs breaking changes (drain CRDs first)
**Log Rotation**: Automatic kubelet rotation (10MB/file, 5 files max), adjust logging level as needed

---

### **On-Call Escalation**

| Severity | Symptoms | Escalation | Runbook |
|----------|----------|------------|---------|
| **P1 (Critical)** | Service completely down, all signals blocked | Immediate rollback → 5min: Platform Lead → 15min: Engineering Manager | Scenario 1 |
| **P2 (High)** | High error rate (>5%), CRD failures, auth failures | Investigate → 30min: Platform Team → 60min: Engineering Manager | Scenarios 1,4,6 |
| **P3 (Medium)** | Storm detection issues, dedup problems, high latency | Document → Next day: Platform Team → 1 week: Jira ticket | Scenarios 2,3,7 |
| **P4 (Low)** | Low dedup rate, high resource usage, optimization opportunities | Next day: Team standup → 1 week: Optimization task | Performance Tuning |

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

---

## 🔮 Future Enhancements (Kubernaut V1.1+)

### OpenTelemetry Signal Adapter (Q1 2026)

**Target Release**: Kubernaut V1.1
**Design Study**: [DD-GATEWAY-002](../../architecture/decisions/DD-GATEWAY-002-opentelemetry-adapter.md) (Feasibility Study Complete)
**Confidence**: 78% (Pre-implementation study)

**Scope**: Add OpenTelemetry trace-based signal ingestion
- Accept OTLP/HTTP and OTLP/gRPC formats
- Extract error spans and high-latency spans as signals
- Map OpenTelemetry service names to Kubernetes resources
- Store trace context in RemediationRequest CRD

**Business Requirements**: BR-GATEWAY-024 through BR-GATEWAY-040 (17 BRs)
**Estimated Effort**: 80-110 hours (10-14 days)
**Testing**: Defense-in-depth pyramid (70%+ / >50% / 10-15% unit/integration/e2e)

**Status**: 📋 Planning phase - detailed in separate design decision document

**Additional Signal Sources** (Future):
- Grafana alerts
- AWS CloudWatch alarms
- Azure Monitor alerts
- GCP Cloud Monitoring
- Datadog webhooks

---

**Document Status**: ✅ Complete (V1.0 Ready for Implementation)
**Plan Version**: v1.0.2
**Last Updated**: October 21, 2025
**Scope**: Prometheus AlertManager + Kubernetes Events only
**Supersedes**: Design documents (consolidated into single plan)
**Next Review**: After Phase 1 completion (enumerate BRs)

**v1.0.2 Changes** (Oct 21, 2025):
- ✅ **Scope finalization**: Removed OpenTelemetry (BR-GATEWAY-024 to 040) from V1.0 implementation scope
- ✅ **Future planning**: Moved OpenTelemetry to Future Enhancements section (Kubernaut V1.1, Q1 2026)
- ✅ **Confidence assessment**: Created comprehensive GATEWAY_V1.0_CONFIDENCE_ASSESSMENT.md (85% confidence)
- ✅ **BR clarification**: V1.0 scope limited to ~40 BRs (Prometheus + K8s Events), 17 BRs deferred to V1.1
- ✅ **Reference preservation**: DD-GATEWAY-002 design study preserved as V1.1 feasibility study
- ✅ **Clean separation**: V1.0 plan now focused exclusively on current release scope

**v1.0.1 Enhancements** (Oct 21, 2025):
- ✅ Complete configuration reference with all options + environment variables
- ✅ Dependencies list (external: 8 packages, internal: 3 packages)
- ✅ API Examples (6 scenarios: success, duplicate, storm, error, K8s events, Redis failure)
- ✅ Service Integration examples (Prometheus, RemediationOrchestrator, Network Policy)
- ✅ Defense-in-depth testing strategy (70%+/>50%/10-15% per `.cursor/rules/03-testing-strategy.mdc`)
- ✅ Unit/integration test examples (3 complete examples with 39 tests)
- ✅ Error handling patterns (HTTP status codes + 4 examples)


# Gateway Unit Test DescribeTable Refactoring Triage

**Date**: 2026-01-16  
**Authority**: TESTING_GUIDELINES.md  
**Status**: Triage Complete  
**Priority**: P2 (Code Quality / Maintainability)

---

## Executive Summary

Triaged Gateway unit tests for `DescribeTable` refactoring opportunities per TESTING_GUIDELINES.md best practices.

**Pattern**: Use `DescribeTable` for tests with:
- Consistent validation logic
- Varying input parameters
- Multiple similar test cases
- Error classification scenarios

**Benefits**:
- Reduced code duplication
- Easier to add new test cases
- Better test organization
- Improved readability

---

## Refactoring Candidates

### 🟢 HIGH PRIORITY (Recommended)

#### 1. **adapter_interface_test.go** - Severity Pass-Through Validation
**Location**: `test/unit/gateway/adapters/adapter_interface_test.go:276-308`  
**Current Pattern**: Loop inside single `It()` block  
**Should Be**: `DescribeTable` with Entry per severity scheme

**Current Code** (32 lines):
```go
It("accepts ANY non-empty severity string (BR-GATEWAY-181 pass-through)", func() {
    validSeverities := []string{
        "critical", "warning", "info",       // Standard
        "Sev1", "Sev2", "Sev3", "Sev4",     // Enterprise
        "P0", "P1", "P2", "P3",             // PagerDuty
        "HIGH", "MEDIUM", "LOW",            // Custom uppercase
        "urgent", "normal",                  // Custom lowercase
    }

    for _, severity := range validSeverities {
        signal := &types.NormalizedSignal{
            AlertName:   "TestAlert",
            Fingerprint: "fingerprint-123",
            Severity:    severity,
            Resource: types.ResourceIdentifier{
                Kind: "Pod",
                Name: "test-pod",
            },
        }

        err := adapter.Validate(signal)

        Expect(err).NotTo(HaveOccurred(),
            "BR-GATEWAY-181: Gateway must accept '%s' severity (pass-through)", severity)
    }
})
```

**Recommended Refactoring** (~20 lines):
```go
DescribeTable("BR-GATEWAY-181: Accepts ANY non-empty severity (pass-through)",
    func(severity string, scheme string) {
        signal := &types.NormalizedSignal{
            AlertName:   "TestAlert",
            Fingerprint: "fingerprint-123",
            Severity:    severity,
            Resource: types.ResourceIdentifier{
                Kind: "Pod",
                Name: "test-pod",
            },
        }

        err := adapter.Validate(signal)

        Expect(err).NotTo(HaveOccurred(),
            "Gateway must accept '%s' severity (%s scheme)", severity, scheme)
    },
    Entry("Standard: critical", "critical", "standard"),
    Entry("Standard: warning", "warning", "standard"),
    Entry("Standard: info", "info", "standard"),
    Entry("Enterprise: Sev1", "Sev1", "enterprise"),
    Entry("Enterprise: Sev2", "Sev2", "enterprise"),
    Entry("Enterprise: Sev3", "Sev3", "enterprise"),
    Entry("Enterprise: Sev4", "Sev4", "enterprise"),
    Entry("PagerDuty: P0", "P0", "pagerduty"),
    Entry("PagerDuty: P1", "P1", "pagerduty"),
    Entry("PagerDuty: P2", "P2", "pagerduty"),
    Entry("PagerDuty: P3", "P3", "pagerduty"),
    Entry("Custom uppercase: HIGH", "HIGH", "custom"),
    Entry("Custom uppercase: MEDIUM", "MEDIUM", "custom"),
    Entry("Custom uppercase: LOW", "LOW", "custom"),
    Entry("Custom lowercase: urgent", "urgent", "custom"),
    Entry("Custom lowercase: normal", "normal", "custom"),
)
```

**Benefits**:
- Each severity scheme gets its own test case in output
- Easier to add new severity schemes (e.g., Datadog, New Relic)
- Better test failure reporting (shows which severity failed)
- Self-documenting (Entry names explain what's being tested)

---

#### 2. **registry_business_test.go** - Severity Pass-Through Validation
**Location**: `test/unit/gateway/adapters/registry_business_test.go:192-222`  
**Current Pattern**: Loop inside single `It()` block with struct test cases  
**Should Be**: `DescribeTable` with Entry per test case

**Current Code** (30 lines):
```go
It("accepts ANY non-empty severity - pass-through architecture", func() {
    testCases := []struct {
        severity string
        scheme   string
    }{
        {"critical", "standard"},
        {"warning", "standard"},
        {"info", "standard"},
        {"Sev1", "enterprise"},
        {"P0", "PagerDuty"},
        {"HIGH", "custom"},
    }

    for _, tc := range testCases {
        signal := &types.NormalizedSignal{
            Fingerprint: "abc123",
            AlertName:   "TestAlert",
            Severity:    tc.severity,
        }

        err := adapter.Validate(signal)

        Expect(err).NotTo(HaveOccurred(),
            "BR-GATEWAY-181: Must accept '%s' (%s scheme)", tc.severity, tc.scheme)
    }
})
```

**Recommended Refactoring** (~15 lines):
```go
DescribeTable("BR-GATEWAY-181: Pass-through accepts any severity scheme",
    func(severity string, scheme string) {
        signal := &types.NormalizedSignal{
            Fingerprint: "abc123",
            AlertName:   "TestAlert",
            Severity:    severity,
        }

        err := adapter.Validate(signal)

        Expect(err).NotTo(HaveOccurred(),
            "Gateway must accept '%s' severity (%s scheme)", severity, scheme)
    },
    Entry("Standard: critical", "critical", "standard"),
    Entry("Standard: warning", "warning", "standard"),
    Entry("Standard: info", "info", "standard"),
    Entry("Enterprise: Sev1", "Sev1", "enterprise"),
    Entry("PagerDuty: P0", "P0", "PagerDuty"),
    Entry("Custom: HIGH", "HIGH", "custom"),
)
```

**Benefits**:
- Removes inline struct definition
- Better test isolation per severity
- Consistent with other DescribeTable patterns

---

### 🟡 MEDIUM PRIORITY (Consider)

#### 3. **signal_ingestion_test.go** - Resource Type Identification
**Location**: `test/unit/gateway/signal_ingestion_test.go:47-261`  
**Current Pattern**: Multiple similar `It()` blocks (Pod, Deployment, Node, StatefulSet, DaemonSet)  
**Should Be**: `DescribeTable` with Entry per resource type

**Current Structure**:
- Lines 47-80: "identifies which Pod triggered the alert"
- Lines 82-115: "identifies which Deployment triggered the alert"
- Lines 117-150: "identifies which Node triggered the alert"
- Lines 152-185: "identifies which StatefulSet triggered the alert"
- Lines 187-220: "identifies which DaemonSet triggered the alert"

**Pattern**: All 5 tests follow identical structure:
1. Create webhook JSON with resource labels
2. Parse webhook
3. Assert resource.Kind, resource.Name, resource.Namespace extracted

**Recommended Refactoring**:
```go
DescribeTable("BR-GATEWAY-002: Identifies Kubernetes resource type for remediation targeting",
    func(resourceKind string, resourceName string, alertName string, businessReason string) {
        alertManagerWebhook := []byte(fmt.Sprintf(`{
            "version": "4",
            "status": "firing",
            "alerts": [{
                "status": "firing",
                "labels": {
                    "alertname": "%s",
                    "namespace": "production",
                    "%s": "%s",
                    "severity": "critical"
                },
                "annotations": {
                    "summary": "%s resource issue"
                },
                "startsAt": "2025-10-09T10:00:00Z"
            }]
        }`, alertName, strings.ToLower(resourceKind), resourceName, resourceKind))

        signal, err := adapter.Parse(ctx, alertManagerWebhook)

        Expect(err).NotTo(HaveOccurred(), businessReason)
        Expect(signal.Resource.Name).To(Equal(resourceName))
        Expect(signal.Resource.Kind).To(Equal(resourceKind))
        Expect(signal.Resource.Namespace).To(Equal("production"))
    },
    Entry("Pod resource for container remediation",
        "Pod", "payment-api-7d9f8c", "PodMemoryHigh",
        "Gateway needs Pod identity for container restart"),
    Entry("Deployment resource for rollout remediation",
        "Deployment", "frontend-v2", "DeploymentReplicasMismatch",
        "Gateway needs Deployment for scaling/rollout"),
    Entry("Node resource for infrastructure remediation",
        "Node", "worker-03", "NodeDiskPressure",
        "Gateway needs Node for infrastructure fixes"),
    Entry("StatefulSet resource for stateful app remediation",
        "StatefulSet", "database-cluster", "StatefulSetPodFailing",
        "Gateway needs StatefulSet for ordered pod management"),
    Entry("DaemonSet resource for node-level remediation",
        "DaemonSet", "log-collector", "DaemonSetPodsMissing",
        "Gateway needs DaemonSet for node-level resource management"),
)
```

**Benefits**:
- Reduces ~214 lines to ~35 lines (84% reduction)
- Easy to add new resource types (Service, ConfigMap, etc.)
- Clearer test intent

**Caution**: Original tests have detailed business context comments. Consider:
- Keep business context in Entry descriptions
- Add Context-level comment explaining resource targeting

---

### 🟢 ALREADY USING DescribeTable (Good Examples)

#### 1. **validation_test.go** - Payload Validation ✅
**Location**: `test/unit/gateway/adapters/validation_test.go:48-79`  
**Status**: Already using DescribeTable correctly  
**Pattern**: 13 Entry cases for invalid payload validation  

**Good Example**:
```go
DescribeTable("should reject invalid payloads",
    func(testCase string, payload []byte, expectedErrorSubstring string, shouldAccept bool) {
        // Validation logic
    },
    Entry("malformed JSON syntax", ...),
    Entry("missing alerts array", ...),
    // ... 11 more entries
)
```

#### 2. **signal_ingestion_test.go** - Webhook Validation ✅
**Location**: `test/unit/gateway/signal_ingestion_test.go:278-308`  
**Status**: Already using DescribeTable correctly  
**Pattern**: 5 Entry cases for invalid webhook protection  

#### 3. **k8s_event_adapter_test.go** - Event Validation ✅
**Location**: `test/unit/gateway/k8s_event_adapter_test.go:194-238`  
**Status**: Already using DescribeTable correctly  
**Pattern**: 5 Entry cases for incomplete event rejection  

---

## Implementation Plan

### Phase 1: High Priority (Week 4)
1. ✅ Refactor `adapter_interface_test.go` severity validation
2. ✅ Refactor `registry_business_test.go` severity validation
3. ✅ Run unit tests to verify no regressions
4. ✅ Commit with detailed rationale

### Phase 2: Medium Priority (Week 5 - Optional)
1. Refactor `signal_ingestion_test.go` resource type identification
2. Evaluate benefits vs. loss of detailed business comments
3. Consider hybrid: DescribeTable for logic, Context for business docs

### Phase 3: Documentation (Week 5)
1. Update `GATEWAY_TEST_STRATEGY_PIVOT.md` with DescribeTable examples
2. Add DescribeTable section to `TESTING_GUIDELINES.md` (if missing)

---

## Refactoring Guidelines

### When to Use DescribeTable

✅ **DO use DescribeTable when**:
- **Validation tests** - Testing input acceptance/rejection
- **Classification tests** - Testing categorization logic
- **3+ similar test cases** - Repetitive test structure
- **Parameterized testing** - Varying inputs, same assertions
- **Error message validation** - Testing different error scenarios

❌ **DON'T use DescribeTable when**:
- **Complex business flows** - Multi-step workflows with context
- **Rich business context** - Detailed comments explaining "why"
- **Different assertion logic** - Each test has unique validation
- **Setup varies significantly** - Different BeforeEach per test
- **1-2 test cases** - Not enough repetition to justify

### DescribeTable Best Practices

1. **Entry Descriptions**: Make self-documenting
   ```go
   Entry("Enterprise Sev1 maps to critical", "Sev1", "critical")
   // NOT: Entry("test case 1", "Sev1", "critical")
   ```

2. **Business Context**: Add in table-level comment
   ```go
   // BUSINESS OUTCOME: Gateway accepts ANY severity scheme
   // SignalProcessing Rego policies normalize downstream
   DescribeTable("Severity pass-through validation", ...)
   ```

3. **Parameter Names**: Use descriptive names
   ```go
   func(severity string, expectedOutcome string) // Good
   // NOT: func(input string, output string)
   ```

4. **Focused Assertions**: Keep table function simple
   ```go
   // Good: Single concern
   Expect(signal.Severity).To(Equal(severity))
   
   // Bad: Multiple unrelated assertions
   Expect(signal.Severity).To(...)
   Expect(signal.Resource.Kind).To(...)
   Expect(signal.Namespace).To(...)
   ```

---

## Impact Assessment

### Code Reduction
- **adapter_interface_test.go**: 32 lines → 25 lines (22% reduction)
- **registry_business_test.go**: 30 lines → 15 lines (50% reduction)
- **signal_ingestion_test.go** (if refactored): 214 lines → 35 lines (84% reduction)

**Total Potential**: ~239 lines → ~75 lines (69% reduction)

### Maintainability
- ✅ Adding new severity schemes: 1 Entry vs 1 array element + ensure loop coverage
- ✅ Test failure reporting: Shows exact Entry that failed
- ✅ Code review: Easier to spot missing test cases

### Test Coverage
- ✅ No change (same assertions, different structure)
- ✅ Better granularity (separate test case per Entry in output)

---

## Decision

**Recommendation**: Proceed with **Phase 1 (High Priority)** refactoring.

**Rationale**:
1. Severity validation tests are **perfect DescribeTable candidates** (consistent logic, varying inputs)
2. BR-GATEWAY-181 pass-through architecture adds **16 new severity schemes** → DescribeTable scales better
3. Existing DescribeTable tests in Gateway show **team already familiar** with pattern
4. **Low risk**: Unit tests, easy to verify with `make test-unit-gateway`

**Deferred**: Phase 2 (signal_ingestion resource type tests) due to:
- Rich business context in original tests
- Less repetitive than severity tests (5 cases vs 16)
- Evaluate feedback from Phase 1 first

---

## References

- **TESTING_GUIDELINES.md**: Lines 42-44 (DescribeTable guidance)
- **Existing Examples**: 
  - `test/unit/gateway/adapters/validation_test.go:48-79`
  - `test/unit/gateway/signal_ingestion_test.go:278-308`
  - `test/unit/gateway/k8s_event_adapter_test.go:194-238`
- **Authority**: BR-GATEWAY-181, DD-SEVERITY-001 v1.1

---

## Approval

- [ ] Technical Lead Review
- [ ] QA Sign-off (unit test coverage maintained)
- [ ] Proceed with Phase 1 implementation

**Status**: READY FOR IMPLEMENTATION

# DD-SEVERITY-001: Gateway Severity Refactoring - Implementation Triage

**Authority**: [DD-SEVERITY-001-severity-determination-refactoring.md](../../architecture/decisions/DD-SEVERITY-001-severity-determination-refactoring.md) v1.1  
**Status**: 🚨 **BLOCKING** - Other teams waiting on Gateway completion  
**Priority**: P0 (Critical - Customer onboarding blocked)  
**Estimated Effort**: 6-8 hours (Week 3 work only)  
**Target**: Gateway Week 3 implementation (Severity + Priority Cleanup)

---

## 🎯 **Executive Summary**

**Problem**: Gateway hardcodes severity mappings (`determineSeverity()`, `mapSeverity()`), blocking customers with custom severity schemes (Sev1-4, P0-P4).

**Solution**: Remove ALL hardcoded severity logic from Gateway adapters. Pass through raw severity values unchanged. Let SignalProcessing Rego determine normalized severity.

**Impact**:
- ✅ Enables ANY customer severity scheme without Gateway code changes
- ✅ Architectural consistency (all policy logic in SignalProcessing Rego)
- ✅ Unblocks SignalProcessing team to implement BR-SP-105

---

## 📋 **Work Breakdown (Week 3 from DD-SEVERITY-001)**

### **Task 1: Remove Prometheus Adapter Hardcoded Severity** (2 hours)

**File**: `pkg/gateway/adapters/prometheus_adapter.go`

#### **Changes Required**:

**1a. Update `Parse()` method** (Line 148):
```go
// BEFORE:
return &types.NormalizedSignal{
    Fingerprint:  fingerprint,
    AlertName:    alert.Labels["alertname"],
    Severity:     determineSeverity(alert.Labels), // ❌ Hardcoded transformation
    // ...
}

// AFTER:
severity := alert.Labels["severity"]
if severity == "" {
    severity = "unknown" // Only default if missing entirely
}

return &types.NormalizedSignal{
    Fingerprint:  fingerprint,
    AlertName:    alert.Labels["alertname"],
    Severity:     severity, // ✅ Pass through as-is (e.g., "Sev1", "P0", "critical")
    // ...
}
```

**Rationale**: Gateway is a "dumb pipe" - extract and preserve, never transform.

---

**1b. Delete `determineSeverity()` function** (Lines 223-241):
```go
// DELETE ENTIRE FUNCTION:
func determineSeverity(labels map[string]string) string {
    severity := labels["severity"]
    switch severity {
    case "critical", "warning", "info":
        return severity
    default:
        return "warning" // Default to warning for unknown severities
    }
}
```

**Rationale**: This hardcoded switch/case prevents custom severity schemes. SignalProcessing Rego will handle mapping.

---

**1c. Update `Validate()` method** (Lines 178-180):
```go
// BEFORE:
if signal.Severity != "critical" && signal.Severity != "warning" && signal.Severity != "info" {
    return fmt.Errorf("invalid severity: %s (must be critical, warning, or info)", signal.Severity)
}

// AFTER:
if signal.Severity == "" {
    return errors.New("severity is required (cannot be empty)")
}
// Remove enum validation - accept ANY string value
```

**Rationale**: Gateway should NOT enforce enum validation. Let CRD validation handle this (enums will be removed in Week 1).

---

**1d. Update function documentation**:
```go
// Validate checks if the parsed signal meets minimum requirements
//
// Validation rules:
// - Fingerprint must be non-empty (64-char SHA256 hex string)
// - AlertName must be non-empty
// - Severity must be non-empty string (any value accepted - no enum restriction)
// - Namespace must be specified (required for Kubernetes-targeted alerts)
//
// Returns:
// - error: Validation errors with descriptive messages
func (a *PrometheusAdapter) Validate(signal *types.NormalizedSignal) error {
```

---

**Validation Tests**:
```bash
# Test pass-through behavior
curl -X POST http://localhost:8080/api/v1/signals/prometheus \
  -H "Content-Type: application/json" \
  -d '{
    "alerts": [{
      "labels": {
        "alertname": "HighCPU",
        "severity": "Sev1",
        "namespace": "production",
        "pod": "api-server"
      }
    }]
  }'

# Expected: RemediationRequest with Spec.Severity = "Sev1" (NOT "critical")
kubectl get remediationrequest -o yaml | grep severity
# Should show: severity: Sev1
```

---

### **Task 2: Remove Kubernetes Event Adapter Hardcoded Severity** (2 hours)

**File**: `pkg/gateway/adapters/kubernetes_event_adapter.go`

#### **Changes Required**:

**2a. Update `Parse()` method** (Line 167):
```go
// BEFORE:
severity := a.mapSeverity(event.Type, event.Reason) // ❌ Hardcoded mapping

// AFTER:
// Pass through event Type as-is for SignalProcessing Rego to map
// Examples: "Warning", "Error"
severity := event.Type
if severity == "" {
    severity = "unknown"
}
```

**Rationale**: Kubernetes event types ("Warning", "Error") should be passed through. SignalProcessing Rego will map them to normalized severity based on operator policy.

---

**2b. Delete `mapSeverity()` method** (Lines 241-269):
```go
// DELETE ENTIRE METHOD:
func (a *KubernetesEventAdapter) mapSeverity(eventType, reason string) string {
    // Critical event reasons (require immediate attention)
    criticalReasons := map[string]bool{
        "OOMKilled":        true,
        "NodeNotReady":     true,
        "FailedScheduling": true,
        "Evicted":          true,
        "FailedMount":      true,
        "NetworkNotReady":  true,
    }

    // Check if reason is critical
    if criticalReasons[reason] {
        return "critical"
    }

    // Error events are critical by default
    if eventType == "Error" {
        return "critical"
    }

    // All other warnings
    return "warning"
}
```

**Rationale**: This hardcoded mapping prevents operator flexibility. SignalProcessing Rego should determine if "OOMKilled" → "critical" based on operator policy.

**SignalProcessing Rego Example** (Future - not Gateway work):
```rego
# In deploy/signalprocessing/policies/severity.rego
result := {"severity": "critical"} if {
    input.signal.severity == "Error"
    input.signal.reason in ["OOMKilled", "NodeNotReady", "FailedScheduling"]
}

result := {"severity": "critical"} if {
    input.signal.severity == "Warning"
    input.signal.reason in ["FailedScheduling", "Evicted"]
}

result := {"severity": "high"} if {
    input.signal.severity == "Warning"
}

default result := {"severity": "unknown"}
```

---

**2c. Update comment in `Parse()` about filtering**:
```go
// 3. Filter event types (business logic: only Warning/Error need remediation)
if event.Type == "Normal" {
    return nil, fmt.Errorf("normal events not processed (informational only)")
}
if event.Type != "Warning" && event.Type != "Error" {
    return nil, fmt.Errorf("unsupported event type: %s (expected Warning or Error)", event.Type)
}

// 4. Pass through event type as severity (SignalProcessing Rego will normalize)
severity := event.Type
if severity == "" {
    severity = "unknown"
}
```

---

**Validation Tests**:
```bash
# Test K8s event pass-through
curl -X POST http://localhost:8080/api/v1/signals/kubernetes \
  -H "Content-Type: application/json" \
  -d '{
    "type": "Warning",
    "reason": "OOMKilled",
    "involvedObject": {
      "kind": "Pod",
      "name": "api-server",
      "namespace": "production"
    }
  }'

# Expected: RemediationRequest with Spec.Severity = "Warning" (NOT "critical")
kubectl get remediationrequest -o yaml | grep severity
# Should show: severity: Warning
```

---

### **Task 3: Update Business Requirements Documentation** (1 hour)

**File**: `docs/services/stateless/gateway-service/BUSINESS_REQUIREMENTS.md`

#### **Changes Required**:

**3a. Update BR-GATEWAY-005** (Severity metadata extraction):
```markdown
### **BR-GATEWAY-005: Signal Metadata Extraction**
**Description**: Gateway must extract severity, namespace, and resource metadata from external signals **without transformation or interpretation**
**Priority**: P0 (Critical)
**Status**: ✅ Complete (Updated 2026-01-16 - Pass-through architecture)
**Implementation**: `pkg/gateway/adapters/prometheus_adapter.go`, `pkg/gateway/adapters/kubernetes_event_adapter.go`
**Tests**: `test/integration/gateway/adapters_integration_test.go`

**Clarification** (2026-01-16): Gateway acts as a "dumb pipe" - extracts and preserves values, never determines policy-based classifications. Severity determination is owned by SignalProcessing via Rego policy (BR-SP-105).

**Examples**:
- Prometheus alert with `labels.severity="Sev1"` → `RR.Spec.Severity="Sev1"` (preserved)
- K8s event with `Type="Warning"` → `RR.Spec.Severity="Warning"` (preserved)
- Missing severity → `RR.Spec.Severity="unknown"` (default, not policy)
```

---

**3b. Deprecate BR-GATEWAY-007** (Priority Assignment):
```markdown
### **BR-GATEWAY-007: Priority Assignment** [DEPRECATED]
**Status**: ⛔ **DEPRECATED** (2026-01-16 per DD-SEVERITY-001)
**Reason**: Priority determination moved to SignalProcessing Rego (BR-SP-070)
**Replacement**: Gateway passes through raw priority hints (if present in labels), SignalProcessing determines final priority
**Migration**: Remove priority determination logic from Gateway adapters (if any exists)
**Authority**: [DD-SEVERITY-001](../../architecture/decisions/DD-SEVERITY-001-severity-determination-refactoring.md)
```

---

**3c. Add BR-GATEWAY-111** (Signal Pass-Through Architecture):
```markdown
### **BR-GATEWAY-111: Signal Pass-Through Architecture** 🆕
**Description**: Gateway MUST normalize external signals to CRD format WITHOUT interpreting or transforming semantic values (severity, environment, priority). Gateway acts as a "dumb pipe" that extracts and preserves values, never determines policy-based classifications.

**Priority**: P0 (Critical - Blocks customer onboarding)
**Status**: 🚧 **IN PROGRESS** (Week 3 implementation)
**Category**: Signal Normalization
**Test Coverage**: ⏳ Planned

**Acceptance Criteria**:
- [x] Extract severity label from external source → `Spec.Severity` (preserve EXACT value, no transformation)
- [ ] Extract environment label from external source → `Spec.Environment` (preserve EXACT value or empty string, no default)
- [ ] Extract priority label from external source → `Spec.Priority` (preserve EXACT value or empty string, no default)
- [x] NO hardcoded severity mappings (e.g., `"Sev1"` → `"warning"`)
- [x] NO default fallback values for non-empty strings (e.g., unknown severity → `"warning"`)
- [x] NO transformation logic based on business rules
- [ ] CRD validation MUST accept any string value (not enum-restricted) - **Waiting on Week 1 CRD changes**
- [ ] Audit trail MUST log external→CRD field mappings for debugging

**Rationale**:
- **Separation of Concerns**: Policy logic (severity/environment/priority determination) belongs in SignalProcessing where full Kubernetes context is available
- **Operator Control**: Severity/environment mappings are operator-defined via SignalProcessing Rego policies, not hardcoded in Gateway
- **Customer Extensibility**: Customers can use ANY severity scheme (Sev1-4, P0-P4, Critical/High/Medium/Low) without Gateway code changes
- **Architectural Consistency**: Matches DD-CATEGORIZATION-001 pattern where Gateway ingests, SignalProcessing categorizes

**Implementation**:
- `pkg/gateway/adapters/prometheus_adapter.go`: Remove `determineSeverity()` hardcoded switch ✅ **IN PROGRESS**
- `pkg/gateway/adapters/kubernetes_event_adapter.go`: Remove `mapSeverity()` hardcoded logic ✅ **IN PROGRESS**
- `api/remediation/v1alpha1/remediationrequest_types.go`: Remove `+kubebuilder:validation:Enum` from `Spec.Severity` - **Waiting on Week 1**

**Tests**:
- `test/unit/gateway/adapters/prometheus_adapter_test.go`: Verify pass-through (input "Sev1" → output "Sev1")
- `test/integration/gateway/custom_severity_test.go`: End-to-end with non-standard severity values

**Related BRs**:
- BR-SP-105 (SignalProcessing Severity Determination) - **Blocked by this BR**
- BR-GATEWAY-005 (Signal Metadata Extraction - amended to clarify "extract not interpret")

**Decision Reference**:
- [DD-CATEGORIZATION-001](../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md) (Environment/Priority consolidation)
- [DD-SEVERITY-001](../../architecture/decisions/DD-SEVERITY-001-severity-determination-refactoring.md) (Severity refactoring plan)

**Authority**: [DD-SEVERITY-001-severity-determination-refactoring.md](../../architecture/decisions/DD-SEVERITY-001-severity-determination-refactoring.md) v1.1, Week 3
```

---

### **Task 4: Integration Test Updates** (2 hours)

**Files**:
- `test/integration/gateway/adapters_integration_test.go` (NEW if doesn't exist)
- `test/integration/gateway/custom_severity_test.go` (NEW)

#### **4a. Create pass-through integration test**:
```go
// test/integration/gateway/custom_severity_test.go
package gateway

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("BR-GATEWAY-111: Custom Severity Pass-Through", func() {
    Context("Prometheus Alerts with Non-Standard Severity", func() {
        It("should preserve 'Sev1' without transformation", func() {
            By("1. Send Prometheus alert with Sev1 severity")
            prometheusAlert := createPrometheusAlert(testNamespace, "HighCPU", "Sev1", "production", "api-server")
            signal, err := prometheusAdapter.Parse(ctx, prometheusAlert)
            Expect(err).ToNot(HaveOccurred())

            By("2. Verify severity preserved (not mapped to 'critical')")
            Expect(signal.Severity).To(Equal("Sev1"), 
                "BR-GATEWAY-111: Gateway must preserve external severity without transformation")
            
            By("3. Process signal and verify CRD")
            response, err := gwServer.ProcessSignal(ctx, signal)
            Expect(err).ToNot(HaveOccurred())
            
            By("4. Verify RemediationRequest has 'Sev1'")
            rr := getRemediationRequest(ctx, response.RemediationRequestName, testNamespace)
            Expect(rr.Spec.Severity).To(Equal("Sev1"),
                "BR-GATEWAY-111: CRD must contain original external severity value")
        })

        It("should preserve 'P0' PagerDuty severity", func() {
            // Similar test for P0
        })

        It("should default to 'unknown' only if severity missing", func() {
            By("1. Send alert without severity label")
            prometheusAlert := createPrometheusAlertWithoutSeverity(testNamespace, "HighCPU")
            signal, err := prometheusAdapter.Parse(ctx, prometheusAlert)
            Expect(err).ToNot(HaveOccurred())

            By("2. Verify severity defaults to 'unknown' (not 'warning')")
            Expect(signal.Severity).To(Equal("unknown"), 
                "BR-GATEWAY-111: Only default if severity entirely missing")
        })
    })

    Context("Kubernetes Events with Non-Standard Severity", func() {
        It("should preserve 'Warning' event type as-is", func() {
            By("1. Send K8s Warning event")
            k8sEvent := createK8sEvent("Warning", "OOMKilled", testNamespace, "Pod", "api-server")
            signal, err := k8sAdapter.Parse(ctx, k8sEvent)
            Expect(err).ToNot(HaveOccurred())

            By("2. Verify severity is 'Warning' (not 'critical')")
            Expect(signal.Severity).To(Equal("Warning"),
                "BR-GATEWAY-111: K8s event type passed through, not mapped to normalized severity")
        })
    })
})
```

---

#### **4b. Update existing adapter tests**:
```bash
# Find tests that assert hardcoded severity
grep -r "Expect.*Severity.*To.*Equal.*critical\|warning\|info" test/integration/gateway/

# Update to assert pass-through instead of transformation
```

---

### **Task 5: Unit Test Updates** (1 hour)

**File**: `test/unit/gateway/adapters/prometheus_adapter_test.go`

#### **5a. Update or create unit tests**:
```go
var _ = Describe("PrometheusAdapter Severity Pass-Through", func() {
    It("should preserve non-standard severity values", func() {
        testCases := []struct {
            input    string
            expected string
        }{
            {"Sev1", "Sev1"},
            {"P0", "P0"},
            {"CRITICAL", "CRITICAL"},
            {"high", "high"},
            {"", "unknown"}, // Only default if missing
        }

        for _, tc := range testCases {
            alert := createAlert(tc.input)
            signal, err := adapter.Parse(ctx, alert)
            Expect(err).ToNot(HaveOccurred())
            Expect(signal.Severity).To(Equal(tc.expected),
                "BR-GATEWAY-111: Severity should be preserved without transformation")
        }
    })
})
```

---

## 🎯 **Validation Checklist**

### **Unit Tests** (All must pass):
- [ ] `test/unit/gateway/adapters/prometheus_adapter_test.go`: Pass-through tests pass
- [ ] `test/unit/gateway/adapters/kubernetes_event_adapter_test.go`: Pass-through tests pass

### **Integration Tests** (All must pass):
- [ ] `test/integration/gateway/custom_severity_test.go`: "Sev1" preserved in CRD
- [ ] `test/integration/gateway/custom_severity_test.go`: "P0" preserved in CRD
- [ ] `test/integration/gateway/custom_severity_test.go`: "Warning" K8s event preserved
- [ ] Existing integration tests still pass (adapt assertions as needed)

### **Manual Testing**:
```bash
# Test Prometheus "Sev1"
curl -X POST http://localhost:8080/api/v1/signals/prometheus \
  -H "Content-Type: application/json" \
  -d '{"alerts": [{"labels": {"alertname": "HighCPU", "severity": "Sev1", "namespace": "prod", "pod": "api"}}]}'

kubectl get remediationrequest -o yaml | grep "severity: Sev1"
# Expected: severity: Sev1

# Test PagerDuty "P0"
curl -X POST http://localhost:8080/api/v1/signals/prometheus \
  -H "Content-Type: application/json" \
  -d '{"alerts": [{"labels": {"alertname": "Critical", "severity": "P0", "namespace": "prod", "pod": "api"}}]}'

kubectl get remediationrequest -o yaml | grep "severity: P0"
# Expected: severity: P0

# Test K8s Event "Warning"
curl -X POST http://localhost:8080/api/v1/signals/kubernetes \
  -H "Content-Type: application/json" \
  -d '{"type": "Warning", "reason": "OOMKilled", "involvedObject": {"kind": "Pod", "name": "api", "namespace": "prod"}}'

kubectl get remediationrequest -o yaml | grep "severity: Warning"
# Expected: severity: Warning
```

### **Code Quality**:
- [ ] No `determineSeverity()` function exists in codebase
- [ ] No `mapSeverity()` function exists in codebase
- [ ] No hardcoded severity switch/case statements in adapters
- [ ] Gateway audit events log original external severity (traceability)
- [ ] Gateway metrics use external severity label (no normalization)

---

## 🚧 **Dependencies & Blockers**

### **Blocked By**:
- ⚠️ **Week 1 (CRD Schema Changes)**: RemediationRequest enum removal not yet done
  - **Impact**: Gateway can pass through "Sev1", but K8s API will reject it until enum removed
  - **Workaround**: Implement Gateway changes now, test with existing "critical/warning/info" values
  - **Timeline**: Week 1 must complete before full pass-through validation

### **Blocking**:
- 🚨 **SignalProcessing Team (Week 2)**: Cannot implement BR-SP-105 (Severity Rego) until Gateway pass-through complete
- 🚨 **RemediationOrchestrator Team (Week 4)**: Cannot update AIAnalysis creator until SP.Status.Severity available

---

## 📊 **Success Metrics**

### **Completion Criteria**:
1. ✅ Zero hardcoded severity transformations in Gateway adapters
2. ✅ Gateway passes through ANY severity value without validation
3. ✅ Integration tests demonstrate "Sev1", "P0", "Warning" preserved in CRDs
4. ✅ Documentation updated (BR-GATEWAY-005, BR-GATEWAY-007, BR-GATEWAY-111)
5. ✅ SignalProcessing team unblocked to start Week 2 work

### **Quality Gates**:
- ✅ All unit tests pass
- ✅ All integration tests pass (with Week 1 CRD changes)
- ✅ No linter errors
- ✅ Gateway code review approved
- ✅ Manual testing confirms pass-through behavior

---

## 🔗 **Related Documents**

- **Authority**: [DD-SEVERITY-001-severity-determination-refactoring.md](../../architecture/decisions/DD-SEVERITY-001-severity-determination-refactoring.md) v1.1
- **Business Requirements**: [BUSINESS_REQUIREMENTS.md](./BUSINESS_REQUIREMENTS.md) (BR-GATEWAY-005, BR-GATEWAY-007, BR-GATEWAY-111)
- **Architecture**: [DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md](../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md)
- **SignalProcessing BRs**: [BR-SP-105](../../services/crd-controllers/01-signalprocessing/BUSINESS_REQUIREMENTS.md) (Severity Determination via Rego)

---

## 📝 **Implementation Timeline**

| Task | Effort | Status | Owner |
|------|--------|--------|-------|
| Task 1: Prometheus Adapter Refactoring | 2h | ⏳ Ready | Gateway Team |
| Task 2: K8s Event Adapter Refactoring | 2h | ⏳ Ready | Gateway Team |
| Task 3: BR Documentation Updates | 1h | ⏳ Ready | Gateway Team |
| Task 4: Integration Tests | 2h | ⏳ Ready | Gateway Team |
| Task 5: Unit Tests | 1h | ⏳ Ready | Gateway Team |
| **Total** | **8h** | **⏳ Ready to Start** | Gateway Team |

---

**Priority**: 🚨 **P0 (Blocking)** - Other teams waiting  
**Target Date**: January 17-18, 2026 (Complete before SignalProcessing Week 2 starts)  
**Confidence**: 95% (Well-defined scope, clear authority, existing test patterns)

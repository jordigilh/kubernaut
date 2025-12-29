# BR-SP-071: Priority Fallback Matrix

## ⚠️ DEPRECATED (2025-12-20)

**This business requirement has been DEPRECATED.**

### Deprecation Reason

Go-level fallback logic creates **silent behavior mismatch** when operator-defined Rego policies don't match the hardcoded fallback values. Operators should have **full control** over priority assignment, including default/catch-all behavior.

### Replacement

- **Rego `default` keyword**: Operators define their own catch-all defaults directly in their Rego policies
- **Mandatory policies**: All classification policies (priority, environment, business) are now MANDATORY
- **No Go fallbacks**: Go code returns errors if Rego policy evaluation fails

### Migration

Replace hardcoded Go fallbacks with Rego `default` rules:

```rego
# Old approach (Go code fallback):
# if rego fails → return P2 (hardcoded in Go)

# New approach (Rego default):
default result := {"priority": "P2", "policy_name": "operator-default"}

# Operators can customize to their needs:
default result := {"priority": "P3", "policy_name": "low-priority-default"}
default result := {"priority": "", "policy_name": "unclassified"}  # Empty = no priority
```

### See Also

- `deploy/signalprocessing/policies/priority.rego` - Default rule example
- `cmd/signalprocessing/main.go` - Mandatory policy enforcement

---

## Original Document (Archived)

**Document Version**: 1.0 (DEPRECATED)
**Date**: December 20, 2025
**Status**: ❌ DEPRECATED - See above
**Category**: Priority Assignment
**Priority**: P1 - High
**Service**: SignalProcessing

---

## 1. Business Purpose

### 1.1 Problem Statement
The SignalProcessing controller relies on Rego policies to assign priorities to incoming signals. However, Rego policy evaluation can fail for multiple reasons:
- **Policy File Missing**: ConfigMap not mounted or policy file not present
- **Policy Syntax Errors**: Invalid Rego syntax in policy file
- **Policy Evaluation Timeout**: Evaluation exceeds 100ms SLA (per BR-SP-070)
- **Policy Runtime Errors**: Evaluation fails with runtime exceptions

Without a fallback mechanism, these failures would block signal processing, preventing remediation workflows from being initiated.

### 1.2 Business Value
A severity-based fallback provides:
- **Operational Resilience**: Signal processing continues despite policy failures
- **Production Safety**: Critical signals (severity=critical) get assigned P1 priority automatically
- **Graceful Degradation**: Reduced confidence but valid priority assignment
- **Zero Downtime**: No service interruption during policy hot-reload or configuration errors

### 1.3 Design Rationale
**Why Severity-Only Fallback?**

When Rego policy fails, the system cannot reliably determine the environment classification or apply complex business rules. Using only signal severity provides:
- ✅ **Predictability**: Severity is always present in signal data
- ✅ **Conservative Escalation**: Critical signals get P1 (high but not highest) without full context
- ✅ **No Compounding Uncertainty**: Avoids combining unreliable environment detection with fallback logic
- ✅ **Simple Debugging**: Clear distinction between policy-driven and fallback assignments

**Trade-off Accepted**: Fallback does NOT consider environment (production vs. staging) or custom business rules. This is intentional to avoid incorrect assumptions when policy is unavailable.

---

## 2. Mandatory Requirements

### 2.1 Fallback Trigger Conditions

#### BR-SP-071.1: Policy File Missing
**MUST** trigger fallback when priority policy file does not exist:

**Trigger Conditions**:
- ConfigMap not mounted to controller pod
- Policy file path does not exist on filesystem
- Detected via `os.ErrNotExist` or equivalent

**Behavior**:
- Log: `priority policy file not mounted, will use severity fallback (BR-SP-071)`
- Use severity-based fallback matrix (see BR-SP-071.5)
- Set `priority_assignment.source = "fallback-severity"`
- Set `priority_assignment.confidence = 0.6`

**Rationale**: Missing policy file is an **acceptable operational state** during initial deployment or when custom policies are not configured. This is distinct from policy syntax errors, which are fatal (per ADR-050 Configuration Validation Strategy).

---

#### BR-SP-071.2: Policy Evaluation Timeout
**MUST** trigger fallback when Rego evaluation exceeds 100ms:

**Trigger Conditions**:
- Rego evaluation duration > 100ms (per BR-SP-070 P95 latency SLA)
- Context deadline exceeded or timeout error

**Behavior**:
- Log: `Rego policy evaluation timed out (>100ms), using severity fallback`
- Use severity-based fallback matrix (see BR-SP-071.5)
- Set `priority_assignment.source = "fallback-timeout"`
- Set `priority_assignment.confidence = 0.6`
- Emit metric: `priority_assignment_fallback_total{reason="timeout"}`

**Rationale**: Performance degradation should not block signal processing. Fallback ensures sub-second reconciliation even with policy issues.

---

#### BR-SP-071.3: Policy Runtime Error
**MUST** trigger fallback when Rego evaluation fails with runtime error:

**Trigger Conditions**:
- Rego `query.Eval()` returns error
- Invalid Rego query results structure
- Policy returns non-standard output format

**Behavior**:
- Log: `Rego policy evaluation failed: <error>, using severity fallback`
- Use severity-based fallback matrix (see BR-SP-071.5)
- Set `priority_assignment.source = "fallback-error"`
- Set `priority_assignment.confidence = 0.6`
- Emit metric: `priority_assignment_fallback_total{reason="error"}`

**Rationale**: Policy bugs or incompatible changes should degrade gracefully rather than fail signal processing.

---

#### BR-SP-071.4: Fallback Logging Requirements
**MUST** log fallback triggers with diagnostic context:

**Required Log Fields**:
- `event`: "Using severity-based fallback"
- `severity`: Signal severity value
- `priority`: Assigned fallback priority
- `reason`: Fallback trigger reason (missing_policy | timeout | error)
- `confidence`: Assigned confidence (0.6)
- `signal_id`: Signal identifier for correlation

**Log Level**: `INFO` (not ERROR - fallback is expected behavior)

**Example**:
```json
{
  "level": "info",
  "timestamp": "2025-12-20T10:30:45Z",
  "event": "Using severity-based fallback",
  "severity": "critical",
  "priority": "P1",
  "reason": "missing_policy",
  "confidence": 0.6,
  "signal_id": "sig-prod-001"
}
```

**Rationale**: Diagnostic logging enables operators to detect policy configuration issues and understand fallback behavior.

---

### 2.2 Fallback Priority Matrix

#### BR-SP-071.5: Severity-Based Priority Mapping
**MUST** use this matrix for fallback priority assignment:

| Severity | Priority | Confidence | Rationale |
|----------|----------|------------|-----------|
| `critical` | `P1` | 0.6 | Conservative - high but not highest without environment context |
| `warning` | `P2` | 0.6 | Standard priority for warnings without business rules |
| `info` | `P3` | 0.6 | Lowest priority for informational signals |
| `unknown` | `P2` | 0.6 | Default when severity is also unknown or invalid |

**Key Principles**:
1. **Conservative Escalation**: `critical` → `P1` (not `P0`) because production context is unknown
2. **No Environment Factor**: Production signals get same priority as staging when using fallback
3. **Fixed Confidence**: 0.6 indicates degraded confidence compared to policy-driven 0.95
4. **Case Insensitive**: Severity matching is case-insensitive (e.g., `Critical` = `critical`)

**Example Behavior**:
```
Signal: severity=critical, environment=production
Policy Available: P0 (confidence: 0.95)  ← Full context available
Policy Missing:   P1 (confidence: 0.6)   ← Conservative fallback
```

---

### 2.3 Non-Functional Requirements

#### BR-SP-071.6: Never Fail
**MUST** always return a valid priority assignment:

**Acceptance Criteria**:
- Fallback function MUST NOT return errors
- Invalid severity values default to `P2` priority
- Empty/nil severity values default to `P2` priority
- Function signature: `func fallbackBySeverity(severity string) *PriorityAssignment` (no error return)

**Rationale**: Signal processing MUST NOT fail due to priority assignment. Degraded operation is acceptable; blocking operation is not.

---

#### BR-SP-071.7: Performance Requirements
**MUST** execute fallback in <1ms:

**Acceptance Criteria**:
- Fallback logic is simple string matching (no external calls)
- P99 latency < 1ms
- Zero allocations beyond return struct
- No network I/O or disk I/O

**Rationale**: Fallback should not introduce latency that triggers timeouts.

---

## 3. Implementation Guidance

### 3.1 Code Location
- **File**: `pkg/signalprocessing/priority/engine.go`
- **Function**: `func (p *PriorityEngine) fallbackBySeverity(severity string) *PriorityAssignment`

### 3.2 Reference Implementation
```go
// fallbackBySeverity returns priority based on severity only (BR-SP-071)
// Used when Rego fails - environment is NOT considered in fallback
func (p *PriorityEngine) fallbackBySeverity(severity string) *PriorityAssignment {
    var priority string
    switch strings.ToLower(severity) {
    case "critical":
        priority = "P1" // Conservative - high but not highest without context
    case "warning":
        priority = "P2"
    case "info":
        priority = "P3"
    default:
        priority = "P2" // Default when severity unknown
    }

    p.logger.Info("Using severity-based fallback",
        "severity", severity,
        "priority", priority,
        "confidence", 0.6,
    )

    return &PriorityAssignment{
        Priority:   priority,
        Confidence: 0.6, // Reduced confidence for fallback
        Source:     "fallback-severity",
    }
}
```

### 3.3 Integration Points
**Callsites**:
1. `cmd/signalprocessing/main.go` - Startup validation when policy file missing
2. `pkg/signalprocessing/priority/engine.go:Assign()` - When Rego eval fails or times out

**Metrics**:
- `priority_assignment_fallback_total{reason="missing_policy|timeout|error"}`
- `priority_assignment_confidence_bucket{source="fallback-severity"}`

---

## 4. Success Criteria

### 4.1 Functional Validation
- ✅ Fallback triggers on missing policy file (startup validation)
- ✅ Fallback triggers on Rego timeout (>100ms)
- ✅ Fallback triggers on Rego runtime errors
- ✅ All severity values map to correct priorities per matrix
- ✅ Unknown/invalid severities default to P2
- ✅ Never returns error or fails signal processing

### 4.2 Operational Validation
- ✅ Logs clearly indicate fallback usage and reason
- ✅ Metrics track fallback usage rate
- ✅ Confidence score accurately reflects degraded mode (0.6 vs 0.95)
- ✅ Performance <1ms P99 latency

### 4.3 Testing Coverage
- ✅ Unit Tests: `priority_engine_test.go`
  - Test each severity mapping
  - Test unknown severity handling
  - Test logging output
  - Test performance <1ms
- ✅ Integration Tests:
  - Test policy file missing scenario
  - Test Rego timeout scenario
  - Test Rego error scenario

---

## 5. Related Requirements & Decisions

### 5.1 Business Requirements
- **BR-SP-070**: Priority Assignment (Rego) - Parent requirement
- **BR-SP-072**: Rego Hot-Reload - Triggers fallback during reload failures

### 5.2 Design Decisions
- **DD-SIGNALPROCESSING-001**: Priority Assignment Architecture (to be created if doesn't exist)

### 5.3 Architecture Decisions
- **ADR-050**: Configuration Validation Strategy - Distinguishes missing file (acceptable) from syntax error (fatal)

### 5.4 Implementation References
- Implemented in: `pkg/signalprocessing/priority/engine.go`
- Used in: `cmd/signalprocessing/main.go` (startup validation)
- Referenced in: Multiple implementation plan versions (v1.18-v1.31)

---

## 6. References

### Internal Documentation
- Implementation Plan: `docs/services/crd-controllers/01-signalprocessing/IMPLEMENTATION_PLAN_V1.31.md`
- Business Requirements: `docs/services/crd-controllers/01-signalprocessing/BUSINESS_REQUIREMENTS.md` (section 278-301)
- Configuration Validation: `docs/architecture/decisions/ADR-050-configuration-validation-strategy.md`

### Code References
- Main Entry Point: `cmd/signalprocessing/main.go:229-232`
- Priority Engine: `pkg/signalprocessing/priority/engine.go`
- Unit Tests: `pkg/signalprocessing/priority/priority_engine_test.go`

---

**Document Status**: ✅ **APPROVED - IMPLEMENTED IN V1.0**
**Priority**: **P1 - High**
**Target Version**: **Kubernaut v1.0**
**Implementation Status**: **✅ COMPLETE** (v1.0)


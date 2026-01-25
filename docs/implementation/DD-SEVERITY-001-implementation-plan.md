# DD-SEVERITY-001: Implementation Plan

**Related DD**: [DD-SEVERITY-001](../architecture/decisions/DD-SEVERITY-001-severity-determination-refactoring.md)
**Test Plan**: [DD_SEVERITY_001_TEST_PLAN_JAN11_2026.md](../handoff/DD_SEVERITY_001_TEST_PLAN_JAN11_2026.md)
**E2E Scenarios**: [DD-SEVERITY-001-E2E-SCENARIOS.md](../testing/test-plans/DD-SEVERITY-001-E2E-SCENARIOS.md)
**Version**: 1.0
**Status**: 🟡 85% Complete
**Target Completion**: Sprint N+1 (Week 5)

---

## 📋 **Executive Summary**

**Scope**: 4 services, 7 files modified, 2 new files created
**Duration**: 4 weeks + 1 week buffer (5 weeks total)
**Current Sprint**: Week 4 (Consumer updates - Complete)
**Next Sprint**: Week 5 (E2E testing)

**Progress**: 85% complete (code 100%, unit tests 100%, integration tests 90%, E2E 0%)

---

## 🎯 **Service Impact Matrix**

| Service | Files Modified | Status | Owner | Blocker |
|---------|---------------|--------|-------|---------|
| **Gateway** | 2 files | 🟡 95% | GW team | Tests 005 & 006 (remove PIt/Skip) |
| **SignalProcessing** | 4 files | ✅ 100% | SP team | None |
| **RemediationOrchestrator** | 1 file | ✅ 100% | RO team | None |
| **AIAnalysis** | 1 file (enum update) | ✅ 100% | AA team | None |
| **DataStorage** | 0 files (triaged) | ✅ 100% | DS team | None |

---

## 📅 **Week-by-Week Implementation Plan**

### **Week 1: CRD Schema Changes** ✅ COMPLETE

**Dates**: Jan 9-13, 2026
**Status**: ✅ All deliverables complete
**Sprint**: Sprint N-3

#### Tasks

| Task ID | Task | Status | Files | Tests |
|---------|------|--------|-------|-------|
| IMPL-001 | Remove RR.Spec.Severity enum | ✅ | `api/remediation/v1alpha1/remediationrequest_types.go:234` | CRD validation |
| IMPL-002 | Remove SP.Spec.Signal.Severity enum | ✅ | `api/signalprocessing/v1alpha1/signalprocessing_types.go` | CRD validation |
| IMPL-003 | Add SP.Status.Severity field | ✅ | `api/signalprocessing/v1alpha1/signalprocessing_types.go:235` | CRD validation |
| IMPL-004 | Update AA.SignalContext enum to v1.1 values | ✅ | `api/aianalysis/v1alpha1/aianalysis_types.go:121,485` | CRD validation |

#### Code Changes

**1. RemediationRequest (Remove Enum)**:
```go
// api/remediation/v1alpha1/remediationrequest_types.go:234
// BEFORE:
// +kubebuilder:validation:Enum=critical;warning;info
Severity string `json:"severity"`

// AFTER:
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=50
Severity string `json:"severity"` // Accepts ANY severity (Sev1, P0, critical, etc.)
```

**2. SignalProcessing (Remove Spec Enum, Add Status Field)**:
```go
// api/signalprocessing/v1alpha1/signalprocessing_types.go
// Spec: Remove enum
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=50
Severity string `json:"severity"` // External severity (pass-through from RR)

// Status: Add normalized severity field
type SignalProcessingStatus struct {
    // ... existing fields ...
    
    // Normalized severity determined by Rego policy (DD-SEVERITY-001 v1.1)
    // Valid values: critical, high, medium, low, unknown
    // +optional
    Severity string `json:"severity,omitempty"`
}
```

**3. AIAnalysis (Update Enum to v1.1 values)**:
```go
// api/aianalysis/v1alpha1/aianalysis_types.go:121,485
// BEFORE:
// +kubebuilder:validation:Enum=critical;warning;info

// AFTER:
// +kubebuilder:validation:Enum=critical;high;medium;low;unknown
Severity string `json:"severity"`
```

#### Deliverables

- ✅ Updated CRD manifests in `deploy/`
- ✅ Updated Go types in `api/*/v1alpha1/`
- ✅ CRD validation tests passing
- ✅ `make generate && make manifests` successful

#### Dependencies

- None (foundational changes)

#### Validation

```bash
# Verify RemediationRequest accepts any severity
kubectl apply -f - <<EOF
apiVersion: remediation.kubernaut.ai/v1alpha1
kind: RemediationRequest
metadata:
  name: test-sev1
spec:
  severity: "Sev1"  # Should be accepted
EOF

# Expected: Created successfully (no CRD validation error)
```

---

### **Week 2: SignalProcessing Rego Implementation** ✅ COMPLETE

**Dates**: Jan 13-17, 2026
**Status**: ✅ All deliverables complete
**Sprint**: Sprint N-2

#### Tasks

| Task ID | Task | Status | Files | Tests |
|---------|------|--------|-------|-------|
| IMPL-005 | Create default severity.rego | ✅ | `config/severity-policy-example.rego` | Unit |
| IMPL-006 | Implement SeverityClassifier | ✅ | `pkg/signalprocessing/classifier/severity.go` | Unit |
| IMPL-007 | Wire controller integration | ✅ | `internal/controller/signalprocessing/reconciler.go` | Integration |
| IMPL-008 | Update audit client | ✅ | `pkg/signalprocessing/audit/client.go:84,325` | Integration |

#### Code Changes

**1. Default Rego Policy** (`config/severity-policy-example.rego`):
```rego
package signalprocessing.severity

import rego.v1

# 1:1 mapping for standard severity values (backward compatibility)
result := {"severity": "critical", "source": "rego-policy"} if {
    lower(input.signal.severity) == "critical"
}

result := {"severity": "high", "source": "rego-policy"} if {
    lower(input.signal.severity) == "high"
}

result := {"severity": "medium", "source": "rego-policy"} if {
    lower(input.signal.severity) == "medium"
}

result := {"severity": "low", "source": "rego-policy"} if {
    lower(input.signal.severity) == "low"
}

# Fallback: unmapped severity → unknown
default result := {"severity": "unknown", "source": "fallback"}
```

**2. Severity Classifier** (`pkg/signalprocessing/classifier/severity.go`):
- Loads Rego policy from ConfigMap
- Evaluates Rego with `input.signal.severity` (external value)
- Returns normalized severity (`critical`, `high`, `medium`, `low`, `unknown`)
- Handles Rego evaluation errors gracefully (fallback to "unknown")

**3. Controller Integration** (`internal/controller/signalprocessing/reconciler.go`):
```go
// During reconcileClassifying phase
severityResult, err := r.severityClassifier.ClassifySeverity(ctx, sp)
if err != nil {
    return ctrl.Result{}, fmt.Errorf("severity classification failed: %w", err)
}

// Write to Status
sp.Status.Severity = severityResult.Severity

// Emit metrics
severityDeterminationTotal.WithLabelValues(
    sp.Spec.Signal.Severity, // external
    severityResult.Severity,  // normalized
    severityResult.Source,    // rego-policy/fallback
).Inc()
```

**4. Audit Client Update** (`pkg/signalprocessing/audit/client.go`):
```go
// Line 84: RecordSignalProcessed
// BEFORE:
payload.Severity.SetTo(toSignalProcessingAuditPayloadSeverity(sp.Spec.Signal.Severity)) // ❌ External

// AFTER:
payload.Severity.SetTo(toSignalProcessingAuditPayloadSeverity(sp.Status.Severity)) // ✅ Normalized

// Line 325: RecordBusinessClassification
// Same change (use sp.Status.Severity)
```

#### Deliverables

- ✅ Default Rego policy with 1:1 mapping
- ✅ SeverityClassifier with OPA integration
- ✅ Controller calls Rego during classification phase
- ✅ Audit events emit normalized severity
- ✅ Test fixtures created (`test/fixtures/severity/`)
- ✅ 10 unit tests + 8 integration tests passing

#### Dependencies

- IMPL-003 (SP.Status.Severity field must exist)

#### Validation

```bash
# Run unit tests
go test ./pkg/signalprocessing/classifier/ -v

# Run integration tests
go test ./test/integration/signalprocessing/ -v -run Severity

# Verify Rego evaluation
echo '{"signal": {"severity": "Sev1"}}' | opa eval -d config/severity-policy-example.rego 'data.signalprocessing.severity.result'
# Expected: {"severity": "unknown", "source": "fallback"} (not in default policy)
```

---

### **Week 3: Gateway Refactoring** 🟡 95% COMPLETE

**Dates**: Jan 15-16, 2026
**Status**: 🟡 Code complete, 2 tests pending (remove PIt/Skip)
**Sprint**: Sprint N-1
**Blocker**: Tests GW-INT-SEV-005 & 006 use `PIt()`/`Skip()` (TESTING_GUIDELINES violation)

#### Tasks

| Task ID | Task | Status | Files | Tests |
|---------|------|--------|-------|-------|
| IMPL-009 | Remove determineSeverity() hardcoding | ✅ | `pkg/gateway/adapters/prometheus_adapter.go` | Unit |
| IMPL-010 | Pass-through severity logic | ✅ | `pkg/gateway/adapters/*.go` | Unit + Integration |
| IMPL-011 | Deprecate BR-GATEWAY-007 | ✅ | `docs/services/stateless/gateway-service/BUSINESS_REQUIREMENTS.md` | Documentation |
| IMPL-012 | Enable tests 005 & 006 (remove PIt/Skip) | ⏸️ **BLOCKER** | `test/integration/gateway/custom_severity_integration_test.go` | Integration |

#### Code Changes

**1. Remove Severity Hardcoding** (`pkg/gateway/adapters/prometheus_adapter.go`):
```go
// BEFORE (Lines 234-241): DELETED
func determineSeverity(labels map[string]string) string {
    severity := labels["severity"]
    switch severity {
    case "critical", "warning", "info":
        return severity
    default:
        return "warning" // Default to warning for unknown severities
    }
}

// AFTER: Function removed entirely

// Update alert processing
// BEFORE:
severity := determineSeverity(alert.Labels)

// AFTER:
severity := alert.Labels["severity"] // Pass through as-is (e.g., "Sev1")
if severity == "" {
    severity = "unknown" // Only default if missing entirely
}
```

**2. Kubernetes Event Adapter** (`pkg/gateway/adapters/kubernetes_event_adapter.go`):
```go
// BEFORE: Hardcoded mapping removed
// K8s Event Type/Reason passed through as-is
// SignalProcessing Rego will map event types to severity
```

**3. Business Requirements Update**:
```markdown
### BR-GATEWAY-007: Priority Assignment [DEPRECATED]
**Status**: ⛔ DEPRECATED (2026-01-09)
**Reason**: Priority determination moved to SignalProcessing Rego (BR-SP-070)
**Replacement**: Gateway passes through raw priority hints
**Migration**: Remove priority determination logic from Gateway adapters
```

#### Deliverables

- ✅ Gateway code cleaned of hardcoded severity logic
- ✅ BR-GATEWAY-007 marked DEPRECATED
- ✅ 8/10 integration tests passing
- ⏸️ **BLOCKER**: Tests 005 & 006 pending (remove `PIt()`/`Skip()`)

#### Dependencies

- IMPL-001 (RR must accept any string)

#### Blocker Resolution

**File**: `test/integration/gateway/custom_severity_integration_test.go`

**Current (VIOLATION)**:
```go
// Lines ~180-220
PIt("[GW-INT-SEV-005] should preserve 'Sev1' enterprise severity...", func() {
    Skip("Pending CRD enum removal")
    // ... test code ...
})
```

**Required Fix** (1 hour):
```go
// Remove PIt() and Skip()
It("[GW-INT-SEV-005] should preserve 'Sev1' enterprise severity...", func() {
    // Update test to use actual Sev1 value
    severity := "Sev1"  // Was using placeholder
    // ... rest of test ...
})
```

#### Validation

```bash
# Verify pass-through behavior
go test ./test/integration/gateway/ -v -run GW-INT-SEV

# Expected: 10/10 tests passing (after fixing 005 & 006)
```

---

### **Week 4: Consumer Updates + DataStorage Triage** ✅ COMPLETE

**Dates**: Jan 15-16, 2026
**Status**: ✅ All deliverables complete
**Sprint**: Sprint N (current)

#### Tasks

| Task ID | Task | Status | Files | Tests |
|---------|------|--------|-------|-------|
| IMPL-013 | Update AIAnalysis creator | ✅ | `pkg/remediationorchestrator/creator/aianalysis.go:172` | Unit |
| IMPL-014 | Verify Notification (no change) | ✅ | `pkg/remediationorchestrator/creator/notification.go` | N/A |
| IMPL-015 | Verify WE handler (no change) | ✅ | `pkg/remediationorchestrator/handler/workflowexecution.go` | N/A |
| IMPL-016 | DataStorage triage | ✅ | `docs/services/stateless/data-storage/BUSINESS_REQUIREMENTS.md` | Documentation |
| IMPL-017 | RO integration tests | ✅ | `test/integration/remediationorchestrator/severity_normalization_integration_test.go` | Integration |

#### Code Changes

**1. AIAnalysis Creator** (`pkg/remediationorchestrator/creator/aianalysis.go:172`):
```go
// BEFORE:
return aianalysisv1.SignalContextInput{
    Fingerprint:      rr.Spec.SignalFingerprint,
    Severity:         rr.Spec.Severity, // ❌ External "Sev1"
    SignalType:       rr.Spec.SignalType,
    Environment:      environment,
    BusinessPriority: priority,
}

// AFTER:
return aianalysisv1.SignalContextInput{
    Fingerprint:      rr.Spec.SignalFingerprint,
    Severity:         sp.Status.Severity, // ✅ Normalized "critical" from Rego
    SignalType:       rr.Spec.SignalType,
    Environment:      environment,
    BusinessPriority: priority,
}
```

**2. Notification Creator (NO CHANGE)**:
```go
// pkg/remediationorchestrator/creator/notification.go:110,127,224,559
// KEEP: rr.Spec.Severity (external "Sev1")
// Rationale: Operators want to see their own severity values in notifications
```

**3. WorkflowExecution Handler (NO CHANGE)**:
```go
// pkg/remediationorchestrator/handler/workflowexecution.go:447
// KEEP: rr.Spec.Severity (external "Sev1")
// Rationale: Operators want to see their own severity values in failure messages
```

**4. DataStorage Triage Decision**:
- Workflow severity is a **separate domain** (workflow catalog)
- Signal severity (this DD) is for **incident classification**
- No integration needed (decision documented)

#### Deliverables

- ✅ RO uses `sp.Status.Severity` (normalized) for AIAnalysis
- ✅ Notifications preserve `rr.Spec.Severity` (external)
- ✅ 1 unit test passing (`test/unit/remediationorchestrator/aianalysis_creator_test.go:200-237`)
- ✅ 5 RO integration tests created and passing:
  - `[RO-INT-SEV-001]` Sev1 → critical normalization
  - `[RO-INT-SEV-002]` Sev2 → high normalization
  - `[RO-INT-SEV-003]` P0 → critical normalization
  - `[RO-INT-SEV-004]` P3 → medium normalization
  - `[RO-INT-SEV-005]` critical → critical (1:1)
- ✅ DataStorage decision documented

#### Dependencies

- IMPL-003 (SP.Status.Severity field)
- IMPL-007 (SP controller populates Status.Severity)

#### Validation

```bash
# Run RO unit tests
go test ./test/unit/remediationorchestrator/ -v -run "normalized severity"

# Run RO integration tests
go test ./test/integration/remediationorchestrator/ -v -run Severity

# Expected: 6 tests passing (1 unit + 5 integration)
```

---

### **Week 5: E2E Testing** ⏸️ NEXT SPRINT

**Dates**: Sprint N+1
**Status**: ⏸️ Pending Gateway P1 completion
**Sprint**: Sprint N+1 (next)
**Blocker**: Gateway tests 005 & 006 (IMPL-012)

#### Tasks

| Task ID | Task | Status | Effort | Priority |
|---------|------|--------|--------|----------|
| IMPL-018 | Enable Gateway tests 005 & 006 | ⏸️ | 1 hour | P1 |
| IMPL-019 | Run Gateway integration tests | ⏸️ | 30 min | P1 |
| IMPL-020 | Create E2E test suite file | ⏸️ | 2 hours | P2 |
| IMPL-021 | Implement Scenario 1 (Sev1) | ⏸️ | 3 hours | P2 |
| IMPL-022 | Implement Scenario 2 (P0) | ⏸️ | 2 hours | P2 |
| IMPL-023 | Implement Scenario 3 (Hot-reload) | ⏸️ | 4 hours | P3 |
| IMPL-024 | Implement Scenario 4 (Multi-scheme) | ⏸️ | 3 hours | P3 |

**Total Effort**: ~15 hours (~2 days)

#### Deliverables

- ⏸️ Gateway integration tests 100% (10/10 passing)
- ⏸️ E2E test suite created (`test/e2e/severity/`)
- ⏸️ 4 E2E pipeline scenarios implemented
- ⏸️ Test fixtures validated in E2E environment
- ⏸️ DD-SEVERITY-001 marked 100% complete

#### Dependencies

- IMPL-012 (Gateway tests 005 & 006 must be enabled)
- IMPL-017 (RO integration tests must pass) ✅ **COMPLETE**

#### Test Scenarios

See [DD-SEVERITY-001-E2E-SCENARIOS.md](../testing/test-plans/DD-SEVERITY-001-E2E-SCENARIOS.md) for detailed E2E test specifications.

---

## 📊 **Progress Tracking**

### **Overall Progress: 85% Complete**

| Week | Component | Code | Unit Tests | Integration Tests | E2E Tests | Status |
|------|-----------|------|------------|-------------------|-----------|--------|
| **Week 1** | CRD Schema | ✅ 100% | ✅ 100% | N/A | N/A | ✅ 100% |
| **Week 2** | SignalProcessing | ✅ 100% | ✅ 100% | ✅ 100% | N/A | ✅ 100% |
| **Week 3** | Gateway | ✅ 100% | ✅ 100% | 🟡 80% (8/10) | N/A | 🟡 95% |
| **Week 4** | RO + DataStorage | ✅ 100% | ✅ 100% | ✅ 100% (5/5) | N/A | ✅ 100% |
| **Week 5** | E2E Pipeline | N/A | N/A | N/A | ⏸️ 0% (0/4) | ⏸️ 0% |

### **File Changes Summary**

| Service | Files Modified | Files Created | Lines Changed |
|---------|---------------|---------------|---------------|
| Gateway | 2 | 0 | ~50 (deletions) |
| SignalProcessing | 3 | 2 | ~350 |
| RemediationOrchestrator | 1 | 1 (tests) | ~450 |
| AIAnalysis | 1 | 0 | ~10 (enum update) |
| Test Fixtures | 0 | 5 | ~200 |
| **Total** | **7** | **8** | **~1,060** |

### **Test Coverage Summary**

| Test Tier | Total Tests | Passing | Pending | Success Rate |
|-----------|-------------|---------|---------|--------------|
| **Unit** | 16 | 16 | 0 | 100% |
| **Integration** | 23 | 21 | 2 | 91% |
| **E2E** | 4 | 0 | 4 | 0% (next sprint) |
| **Total** | **43** | **37** | **6** | **86%** |

---

## 🚧 **Blockers & Resolutions**

| Blocker ID | Description | Impact | Resolution | Owner | ETA |
|------------|-------------|--------|------------|-------|-----|
| BLOCK-001 | Gateway tests 005 & 006 use `PIt()`/`Skip()` | Blocks E2E (P1) | Remove `PIt()`/`Skip()`, update test values to "Sev1"/"P0" | GW team | 1 hour |
| BLOCK-002 | RO integration tests missing | Blocked confidence | ✅ **RESOLVED** Jan 16, 2026 - 5 tests created | RO team | Complete |

---

## 🎯 **Critical Path**

```
Week 1 (CRD) ──────────┬──────> Week 2 (SP Rego) ──────────┬──────> Week 5 (E2E)
                       │                                     │
                       └──────> Week 3 (Gateway) ───────────┘
                       │
                       └──────> Week 4 (RO Consumers) ──────┘
```

**Critical Dependencies**:
1. ✅ Week 1 blocks all (CRD schema foundational)
2. ✅ Week 2 blocks Week 4 (SP.Status.Severity must exist)
3. ⏸️ Week 3 blocks Week 5 (Gateway tests must pass) ← **CURRENT BLOCKER**
4. ✅ Week 4 resolved (RO integration tests complete)

---

## 🔍 **Risk Assessment**

| Risk | Probability | Impact | Mitigation | Status |
|------|------------|--------|------------|--------|
| Gateway tests not enabled in time | Medium | High (blocks E2E) | Escalate to GW team, 1-hour fix with clear instructions | ⏸️ Pending |
| E2E infrastructure issues | Low | Medium | Test fixtures validated, Kind cluster infrastructure tested | ✅ Mitigated |
| Hot-reload fails in E2E | Low | Low | Integration tests validate hot-reload mechanism | ✅ Mitigated |
| RO integration test gaps | Low | Medium | ✅ **RESOLVED** - 5 comprehensive tests created | ✅ Resolved |

---

## ✅ **Sign-Off**

| Milestone | Planned Date | Actual Date | Approver | Status |
|-----------|-------------|-------------|----------|--------|
| Week 1 Complete | Jan 13, 2026 | Jan 13, 2026 | Tech Lead | ✅ |
| Week 2 Complete | Jan 17, 2026 | Jan 17, 2026 | Tech Lead | ✅ |
| Week 3 Complete | Jan 17, 2026 | Jan 16, 2026 | Tech Lead | 🟡 95% |
| Week 4 Complete | Jan 20, 2026 | Jan 16, 2026 | Tech Lead | ✅ |
| Week 5 Ready | Jan 27, 2026 | Pending | Tech Lead | ⏸️ |

---

## 🔗 **Cross-References**

- **Design Decision**: [DD-SEVERITY-001](../architecture/decisions/DD-SEVERITY-001-severity-determination-refactoring.md) - WHY (architecture, rationale, consequences)
- **Test Plan**: [DD_SEVERITY_001_TEST_PLAN_JAN11_2026.md](../handoff/DD_SEVERITY_001_TEST_PLAN_JAN11_2026.md) - WHAT (comprehensive test coverage)
- **E2E Scenarios**: [DD-SEVERITY-001-E2E-SCENARIOS.md](../testing/test-plans/DD-SEVERITY-001-E2E-SCENARIOS.md) - WHEN (next sprint E2E focus)
- **Test Fixtures**: [test/fixtures/severity/README.md](../../test/fixtures/severity/README.md) - Test data for E2E scenarios
- **Business Requirements**:
  - [BR-GATEWAY-111](../services/stateless/gateway-service/BUSINESS_REQUIREMENTS.md) - Gateway Signal Pass-Through
  - [BR-SP-105](../services/crd-controllers/01-signalprocessing/BUSINESS_REQUIREMENTS.md) - Severity Determination via Rego

---

## 📝 **Changelog**

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-01-16 | AI Assistant | Initial implementation plan extracted from DD-SEVERITY-001 |

---

**Document Type**: Implementation Plan (HOW + WHEN)
**Related DD**: DD-SEVERITY-001 v1.1
**Next Review**: After Week 5 E2E completion

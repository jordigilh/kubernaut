# DD-SEVERITY-001 v1.1 Test Compliance Triage - Jan 15, 2026

## 📋 **Executive Summary**

**Issue**: DD-SEVERITY-001 v1.1 updated normalized severity values from `critical/warning/info/unknown` to `critical/high/medium/low/unknown`, but SignalProcessing tests still use old values.

**Impact**:
- ❌ **123 test files** reference obsolete `warning`/`info` severity values
- ❌ **Rego policies** in integration/e2e tests output old values
- ✅ **CRD enum** already updated correctly in `signalprocessing_types.go` (line 192)

**Priority**: **P0** - Blocks DD-SEVERITY-001 v1.1 compliance

---

## 🔍 **Triage Results**

### **1. CRD Definition - ✅ COMPLIANT**

**File**: `api/signalprocessing/v1alpha1/signalprocessing_types.go:192`

```go
// +kubebuilder:validation:Enum=critical;high;medium;low;unknown
Severity string `json:"severity,omitempty"`
```

**Status**: ✅ Already updated to v1.1 values

---

### **2. Unit Tests - ❌ NON-COMPLIANT (23 occurrences)**

**File**: `test/unit/signalprocessing/audit_client_test.go`
- Line 322: `Severity: "warning"` → Should be `"medium"`

**File**: `test/unit/signalprocessing/controller_reconciliation_test.go`
- Line 672: `Severity: "warning"` → Should be `"medium"`

**File**: `test/unit/signalprocessing/priority_engine_test.go`
- Lines 91-122: **Rego policy** uses `warning`/`info` → Should be `high`/`medium`/`low`
- Lines 166-568: **Test cases** use `"warning"`/`"info"` → Should be `"high"`/`"medium"`/`"low"`

**Impact**: Unit tests will fail CRD validation when creating SignalProcessing objects with `warning`/`info`.

---

### **3. Integration Tests - ❌ NON-COMPLIANT (74 occurrences)**

**File**: `test/integration/signalprocessing/component_integration_test.go` (27 occurrences)
- Lines 109, 149, 209, 262, 300, 382, 419, 541, 584, 618, 699, 751, 793, 873, 918: `Severity: "warning"` → `"medium"` or `"high"`
- Lines 973, 1022, 1062, 1108, 1152, 1200, 1247, 1298: `Severity: "info"` → `"low"`

**File**: `test/integration/signalprocessing/suite_test.go` (12 occurrences - **REGO POLICIES**)
- Lines 379, 389, 399, 410: Rego conditions check for `"warning"` → Should be `"high"` or `"medium"`
- Lines 484-490: Rego outputs `"warning"` and `"info"` → Should be `"high"`, `"medium"`, `"low"`
- Lines 797, 807, 817, 828: Rego conditions check for `"warning"` → Should be `"high"` or `"medium"`

**File**: `test/integration/signalprocessing/severity_integration_test.go` (9 occurrences)
- Lines 117, 283, 361, 363, 567, 620: Assertions expect `"warning"`/`"info"` → Should expect `"high"`, `"medium"`, `"low"`

**File**: `test/integration/signalprocessing/audit_integration_test.go` (6 occurrences)
- Lines 299, 306, 501, 508, 593, 600: Test severities use `"warning"`/`"info"` → Should be `"medium"`/`"low"`

**File**: `test/integration/signalprocessing/reconciler_integration_test.go` (26 occurrences)
- Multiple lines: `Severity: "warning"` and `"info"` → Should be `"medium"`/`"high"`/`"low"`

**Impact**: Integration tests will fail CRD validation when creating SignalProcessing CRs.

---

### **4. E2E Tests - ❌ NON-COMPLIANT (26 occurrences)**

**File**: `test/e2e/signalprocessing/40_severity_determination_test.go`
- Line 226: **Rego policy** outputs `"warning"` → Should be `"high"` or `"medium"`
- Lines 277-279, 324-325: Assertions expect `"warning"` → Should expect `"high"` or `"medium"`

**File**: `test/e2e/signalprocessing/business_requirements_test.go` (20 occurrences)
- Lines 138, 195, 333, 450, 524, 583, 911, 1345, 1549, 1668, 1926, 2046, 2391: `Severity: "warning"` → `"high"` or `"medium"`
- Lines 450, 2483: `Severity: "info"` → `"low"`
- Lines 316-317, 2357-2405, 2449-2497: Test descriptions reference `warning`/`info` → Update to `high`/`medium`/`low`

**Impact**: E2E tests will fail CRD validation when creating SignalProcessing CRs.

---

## 🎯 **Severity Value Mapping Strategy**

### **Semantic Mapping**

Per DD-SEVERITY-001 v1.1, the 5-level granularity provides clearer distinctions:

| Old Value (v1.0) | New Value (v1.1) | Semantic Meaning | Prometheus Alert Mapping |
|-----------------|-----------------|-----------------|------------------------|
| `critical` | `critical` | Immediate action required | P0, Sev1, Critical |
| `warning` | **`high`** | Urgent, requires attention | P1-P2, Sev2, High |
| ❌ N/A | **`medium`** | Important, not urgent | P3, Sev3, Medium |
| `info` | **`low`** | Informational, FYI | P4, Sev4, Low, Info |
| `unknown` | `unknown` | Unmapped/fallback | (fallback) |

### **Recommended Replacements**

#### **Context-Dependent Mapping**

**For Production Environment Tests**:
- `"warning"` in **production + high-priority** context → `"high"`
- `"warning"` in **staging/development** context → `"medium"`

**For Development/Staging Tests**:
- `"warning"` in **low-priority** context → `"medium"`
- `"info"` → `"low"`

**For Rego Policies**:
- Replace **all** `"warning"` outputs with `"high"` (enterprise Sev2, PagerDuty P2)
- Replace **all** `"info"` outputs with `"low"` (enterprise Sev3-4, PagerDuty P3-4)

---

## 📝 **Implementation Plan**

### **Phase 1: Rego Policy Updates (Blocking)**

**Files**:
1. `test/integration/signalprocessing/suite_test.go` (lines 474-499)
2. `test/e2e/signalprocessing/40_severity_determination_test.go` (line 226)
3. `test/unit/signalprocessing/priority_engine_test.go` (lines 91-122)

**Changes**:
```rego
# BEFORE:
determine_severity := "warning" if {
    input.signal.severity == "sev2"
}
determine_severity := "info" if {
    input.signal.severity == "sev3"
}

# AFTER (DD-SEVERITY-001 v1.1):
determine_severity := "high" if {
    input.signal.severity == "sev2"
}
determine_severity := "medium" if {
    input.signal.severity == "sev3"
}
determine_severity := "low" if {
    input.signal.severity == "sev4"
}
```

**Priority**: **P0** - Blocking for all test execution

---

### **Phase 2: Unit Test Updates**

**Files**:
1. `test/unit/signalprocessing/audit_client_test.go` (1 occurrence)
2. `test/unit/signalprocessing/controller_reconciliation_test.go` (1 occurrence)
3. `test/unit/signalprocessing/priority_engine_test.go` (20 occurrences)

**Strategy**:
- Replace `"warning"` with `"high"` or `"medium"` based on context
- Replace `"info"` with `"low"`
- Update test descriptions to reference new values

---

### **Phase 3: Integration Test Updates**

**Files**:
1. `test/integration/signalprocessing/component_integration_test.go` (27 occurrences)
2. `test/integration/signalprocessing/severity_integration_test.go` (9 occurrences)
3. `test/integration/signalprocessing/audit_integration_test.go` (6 occurrences)
4. `test/integration/signalprocessing/reconciler_integration_test.go` (26 occurrences)
5. `test/integration/signalprocessing/metrics_integration_test.go` (3 occurrences)
6. `test/integration/signalprocessing/hot_reloader_test.go` (6 occurrences)
7. `test/integration/signalprocessing/rego_integration_test.go` (9 occurrences)
8. `test/integration/signalprocessing/setup_verification_test.go` (1 occurrence)

**Strategy**:
- **Production + critical context**: Keep `"critical"`
- **Production + warning context**: Replace with `"high"`
- **Staging/development + warning context**: Replace with `"medium"`
- **All `"info"` contexts**: Replace with `"low"`

---

### **Phase 4: E2E Test Updates**

**Files**:
1. `test/e2e/signalprocessing/40_severity_determination_test.go` (6 occurrences)
2. `test/e2e/signalprocessing/business_requirements_test.go` (20 occurrences)

**Strategy**:
- Update Rego policy outputs to use `high`/`medium`/`low`
- Update test assertions to expect new values
- Update test descriptions and comments

---

## ✅ **Validation Criteria**

### **Success Metrics**

1. ✅ **Zero occurrences** of `"warning"` or `"info"` in SignalProcessing test severity fields
2. ✅ **All Rego policies** output only `critical/high/medium/low/unknown`
3. ✅ **All test assertions** validate against v1.1 enum values
4. ✅ **100% test pass rate** after updates
5. ✅ **No CRD validation errors** for severity field

### **Validation Commands**

```bash
# Check for remaining old values
grep -r "\"warning\"" test/unit/signalprocessing test/integration/signalprocessing test/e2e/signalprocessing | grep -i severity
grep -r "\"info\"" test/unit/signalprocessing test/integration/signalprocessing test/e2e/signalprocessing | grep -i severity

# Run tests
make test-unit-signalprocessing
make test-integration-signalprocessing
make test-e2e-signalprocessing
```

---

## 🔗 **Related Documentation**

- **[DD-SEVERITY-001](../architecture/decisions/DD-SEVERITY-001-severity-determination-refactoring.md)** (v1.1) - Authoritative severity value specification
- **[AA_TEAM_DD_SEVERITY_001_WEEK4_HANDOFF_JAN15_2026.md](AA_TEAM_DD_SEVERITY_001_WEEK4_HANDOFF_JAN15_2026.md)** - AIAnalysis team handoff

---

## 📊 **Summary Statistics**

| Test Type | Total Occurrences | Files Affected | Priority |
|-----------|------------------|---------------|----------|
| **Unit** | 23 | 3 | P0 |
| **Integration** | 74 | 8 | P0 |
| **E2E** | 26 | 2 | P0 |
| **TOTAL** | **123** | **13** | **P0** |

**Estimated Fix Duration**: 45-60 minutes (systematic find-replace with semantic validation)

---

**Document Status**: ✅ Complete
**Created**: 2026-01-15
**Author**: AI Assistant (Triage)
**Priority**: P0 - Blocks DD-SEVERITY-001 v1.1 compliance

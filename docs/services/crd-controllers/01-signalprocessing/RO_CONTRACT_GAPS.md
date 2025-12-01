# RO Contract Gaps - SignalProcessing Team

**From**: Remediation Orchestrator Team
**To**: SignalProcessing Team
**Date**: December 1, 2025
**Status**: 🔴 ACTION REQUIRED

---

## Summary

The following gaps affect the `SignalProcessing` CRD schema. Most are **bug fixes** where enum constraints should be free-text (Rego policy values).

| Gap ID | Issue | Severity | Action Required |
|--------|-------|----------|-----------------|
| GAP-C1-01 | Environment enum constraint | 🔴 Critical | Change to free-text |
| GAP-C1-02 | Priority enum constraint | 🟠 High | Change to free-text |
| GAP-C1-04 | Deduplication type | ✅ Resolved | Already completed |
| GAP-C1-05 | StormType missing | 🟡 Medium | Add field or confirm not needed |
| GAP-C1-06 | StormWindow missing | 🟡 Medium | Add field or confirm not needed |

---

## GAP-C1-01: Environment Enum Constraint (BUG FIX) 🔴 CRITICAL

**File**: `api/signalprocessing/v1alpha1/signalprocessing_types.go:59-60`

**Current** (incorrect):
```go
// +kubebuilder:validation:Enum=prod;staging;dev  // ❌ WRONG
Environment string `json:"environment"`
```

**Required** (correct):
```go
// Environment value provided by Rego policies - no enum enforcement
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=63
Environment string `json:"environment"`
```

**Reason**: Environment values are defined by Rego policies (operator-customizable), not hardcoded enums. The current enum blocks valid values like "production", "qa-eu", "canary", etc.

---

## GAP-C1-02: Priority Enum Constraint (BUG FIX)

**File**: `api/signalprocessing/v1alpha1/signalprocessing_types.go:63-65`

**Current** (incorrect):
```go
// +kubebuilder:validation:Enum=P0;P1;P2  // ❌ WRONG - missing P3 and should be free-text
Priority string `json:"priority"`
```

**Required** (correct):
```go
// Priority value provided by Rego policies - no enum enforcement
// Best practice examples: P0 (critical), P1 (high), P2 (normal), P3 (low)
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=63
Priority string `json:"priority"`
```

**Reason**: Priority values are defined by Rego policies (operator-customizable), not hardcoded enums.

---

## GAP-C1-04: Deduplication Shared Type ✅ RESOLVED

Already completed - thank you!

---

## GAP-C1-05 & GAP-C1-06: Storm Fields (QUESTION)

**RemediationRequest has these fields that are missing in SignalProcessing.spec**:

| Field | Type | Description |
|-------|------|-------------|
| `stormType` | string | "rate" or "pattern" |
| `stormWindow` | string | e.g., "5m" |

**Question**: Does SignalProcessing need these fields for enrichment logic?

**Options**:
- [ ] **A) Add fields** - SP needs them for storm-aware enrichment
- [ ] **B) Skip fields** - SP doesn't use them, RO can omit when creating SP

---

## Response Template

Please respond below or create a new section:

---

## ✅ SignalProcessing Team Response

**Date**: December 1, 2025
**Respondent**: SignalProcessing Team
**Status**: ✅ **ALL GAPS RESOLVED**

---

### GAP-C1-01 (Environment) ✅ FIXED

- [x] Accepted - changed to free-text

**Changes Made** (`api/signalprocessing/v1alpha1/signalprocessing_types.go`):

**Before**:
```go
// Environment: "prod", "staging", "dev"
// +kubebuilder:validation:Enum=prod;staging;dev
Environment string `json:"environment"`
```

**After**:
```go
// Environment value provided by Rego policies - no enum enforcement
// Examples: "production", "staging", "development", "qa-eu", "canary"
// GAP-C1-01 FIX: Changed from Enum=prod;staging;dev to free-text
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=63
Environment string `json:"environment"`
```

**Rationale**: Rego policies define environment values dynamically. Operators can customize environment names per their organization (e.g., "production" vs "prod", regional variants like "qa-eu").

---

### GAP-C1-02 (Priority) ✅ FIXED

- [x] Accepted - changed to free-text

**Note**: Also removed Pattern constraint `^P[0-2]$` which was not mentioned in the gap but was blocking P3.

**Changes Made** (`api/signalprocessing/v1alpha1/signalprocessing_types.go`):

**Before**:
```go
// Priority assigned by Gateway (P0=critical, P1=high, P2=normal)
// +kubebuilder:validation:Enum=P0;P1;P2
// +kubebuilder:validation:Pattern="^P[0-2]$"
Priority string `json:"priority"`
```

**After**:
```go
// Priority value provided by Rego policies - no enum enforcement
// Best practice examples: P0 (critical), P1 (high), P2 (normal), P3 (low)
// GAP-C1-02 FIX: Changed from Enum=P0;P1;P2 + Pattern to free-text
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=63
Priority string `json:"priority"`
```

**Rationale**: 
1. Rego policies define priority values dynamically
2. DD-WORKFLOW-001 v1.8 specifies P0-P3 as valid priorities
3. Operators may use custom priority schemes (e.g., "critical", "high", "medium", "low")

---

### GAP-C1-05/06 (Storm Fields) ✅ FIXED

- [x] Option A - Added stormType and stormWindow fields

**Changes Made** (`api/signalprocessing/v1alpha1/signalprocessing_types.go`):

**New Fields Added**:
```go
// Storm type classification
// GAP-C1-05 FIX: Added field for contract alignment with RemediationRequest
// Values: "rate" (frequency-based storm) or "pattern" (similar alerts storm)
// +kubebuilder:validation:MaxLength=63
StormType string `json:"stormType,omitempty"`

// Time window used for storm detection
// GAP-C1-06 FIX: Added field for contract alignment with RemediationRequest
// Format: duration string (e.g., "5m", "1h")
// +kubebuilder:validation:MaxLength=63
StormWindow string `json:"stormWindow,omitempty"`
```

**Rationale**:
1. **Self-contained CRD pattern**: SignalProcessing should have ALL data from RemediationRequest
2. **LLM context**: Storm type provides context for AI analysis (rate vs pattern storms may need different handling)
3. **Contract alignment**: RO expects to copy all fields without data loss
4. **Future enrichment**: Storm metadata may influence enrichment logic (e.g., aggregate context for pattern storms)

---

### SignalProcessing Storm Fields (Complete List)

| Field | Type | Description | Status |
|-------|------|-------------|--------|
| `isStorm` | bool | Whether signal is part of a storm | ✅ Already existed |
| `stormAlertCount` | int | Number of alerts in the storm | ✅ Already existed |
| `stormType` | string | "rate" or "pattern" | ✅ **Added** |
| `stormWindow` | string | Duration (e.g., "5m") | ✅ **Added** |

---

### Verification

```bash
# Commands executed:
make generate && make manifests
go build ./api/signalprocessing/...

# Results:
✅ CRD manifests regenerated successfully
✅ API package builds without errors
✅ deepcopy functions regenerated
```

---

### Files Modified

| File | Changes |
|------|---------|
| `api/signalprocessing/v1alpha1/signalprocessing_types.go` | Removed enums, added storm fields |
| `api/signalprocessing/v1alpha1/zz_generated.deepcopy.go` | Auto-regenerated |
| `config/crd/bases/signalprocessing.kubernaut.ai_signalprocessings.yaml` | Updated schema |

---

### CRD Schema Changes Summary

| Field | Before | After |
|-------|--------|-------|
| `environment` | `enum: [prod, staging, dev]` | `minLength: 1, maxLength: 63` |
| `priority` | `enum: [P0, P1, P2]` + `pattern: ^P[0-2]$` | `minLength: 1, maxLength: 63` |
| `stormType` | ❌ Missing | ✅ `maxLength: 63` |
| `stormWindow` | ❌ Missing | ✅ `maxLength: 63` |

---

**Completion Date**: December 1, 2025
**Ready for RO Team Review**: ✅ YES

---

## After Completion

Run:
```bash
make generate && make manifests
```

Notify RO team when complete.


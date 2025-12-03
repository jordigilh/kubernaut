# RO Contract Gaps - SignalProcessing Team

**From**: Remediation Orchestrator Team
**To**: SignalProcessing Team
**Date**: December 1, 2025
**Status**: ✅ **ALL GAPS RESOLVED**

---

## Summary

The following gaps affect the `SignalProcessing` CRD schema. Most are **bug fixes** where enum constraints should be free-text (Rego policy values).

| Gap ID | Issue | Severity | Status |
|--------|-------|----------|--------|
| GAP-C1-01 | Environment enum constraint | 🔴 Critical | ✅ Changed to free-text |
| GAP-C1-02 | Priority enum constraint | 🟠 High | ✅ Changed to free-text |
| GAP-C1-04 | Deduplication type | ✅ Resolved | Already completed |
| GAP-C1-05 | StormType missing | 🟡 Medium | ✅ Field added |
| GAP-C1-06 | StormWindow missing | 🟡 Medium | ✅ Field added |

---

## GAP-C1-01: Environment Enum Constraint (BUG FIX) 🔴 CRITICAL

**File**: `api/signalprocessing/v1alpha1/signalprocessing_types.go:59-60`

**Before** (incorrect):
```go
// +kubebuilder:validation:Enum=prod;staging;dev  // ❌ WRONG
Environment string `json:"environment"`
```

**After** (correct):
```go
// Environment value provided by Rego policies - no enum enforcement
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=63
Environment string `json:"environment"`
```

**Reason**: Environment values are defined by Rego policies (operator-customizable), not hardcoded enums.

---

## GAP-C1-02: Priority Enum Constraint (BUG FIX)

**Before** (incorrect):
```go
// +kubebuilder:validation:Enum=P0;P1;P2  // ❌ WRONG - missing P3 and should be free-text
Priority string `json:"priority"`
```

**After** (correct):
```go
// Priority value provided by Rego policies - no enum enforcement
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=63
Priority string `json:"priority"`
```

---

## GAP-C1-05 & GAP-C1-06: Storm Fields ✅ RESOLVED

**New Fields Added**:
```go
// Storm type classification
// Values: "rate" (frequency-based storm) or "pattern" (similar alerts storm)
// +kubebuilder:validation:MaxLength=63
StormType string `json:"stormType,omitempty"`

// Time window used for storm detection
// Format: duration string (e.g., "5m", "1h")
// +kubebuilder:validation:MaxLength=63
StormWindow string `json:"stormWindow,omitempty"`
```

---

## ✅ SignalProcessing Team Response

**Date**: December 1, 2025
**Respondent**: SignalProcessing Team
**Status**: ✅ **ALL GAPS RESOLVED**

### GAP-C1-01 (Environment) ✅ FIXED
- [x] Accepted - changed to free-text
- Removed `Enum=prod;staging;dev` validation
- Added `MinLength=1`, `MaxLength=63`

### GAP-C1-02 (Priority) ✅ FIXED
- [x] Accepted - changed to free-text
- Also removed Pattern constraint `^P[0-2]$`

### GAP-C1-05/06 (Storm Fields) ✅ FIXED
- [x] Option A - Added stormType and stormWindow fields

### SignalProcessing Storm Fields (Complete List)

| Field | Type | Description | Status |
|-------|------|-------------|--------|
| `isStorm` | bool | Whether signal is part of a storm | ✅ Already existed |
| `stormAlertCount` | int | Number of alerts in the storm | ✅ Already existed |
| `stormType` | string | "rate" or "pattern" | ✅ **Added** |
| `stormWindow` | string | Duration (e.g., "5m") | ✅ **Added** |

### CRD Schema Changes Summary

| Field | Before | After |
|-------|--------|-------|
| `environment` | `enum: [prod, staging, dev]` | `minLength: 1, maxLength: 63` |
| `priority` | `enum: [P0, P1, P2]` + `pattern: ^P[0-2]$` | `minLength: 1, maxLength: 63` |
| `stormType` | ❌ Missing | ✅ `maxLength: 63` |
| `stormWindow` | ❌ Missing | ✅ `maxLength: 63` |

### Verification

```bash
make generate && make manifests
go build ./api/signalprocessing/...
# ✅ All commands successful
```

---

**Document Version**: 1.1
**Last Updated**: December 2, 2025
**Migrated From**: `docs/services/crd-controllers/01-signalprocessing/RO_CONTRACT_GAPS.md`
**Changelog**:
- v1.1: Migrated to `docs/handoff/` as authoritative Q&A directory
- v1.0: Initial document with all gaps resolved



# CRD "Alert" Prefix Triage - Signal-Agnostic Naming

**Date**: October 8, 2025
**Purpose**: Identify and fix "Alert" prefix usage in CRD schemas for signal-agnostic terminology
**Context**: Kubernaut now supports multiple signal types (Prometheus alerts, Kubernetes events, AWS CloudWatch, etc.)
**Scope**: CRD schemas and recent triage/action plan documents

---

## 🎯 **TRIAGE SUMMARY**

**Status**: ⚠️ **ALERT PREFIX FOUND** - 3 field names need renaming

**Impact**: **LOW** - Only field names, no CRD names or major structural issues

**Reason for Change**: Kubernaut processes multiple signal types:
- ✅ Prometheus alerts
- ✅ Kubernetes events
- ⏸️ AWS CloudWatch alarms (V2)
- ⏸️ Azure Monitor alerts (V2)
- ⏸️ Datadog monitors (V2)
- ⏸️ Custom webhooks (V2)

Using "Alert" prefix is misleading and limits conceptual scope.

---

## 📊 **FINDINGS**

### **✅ NO ISSUES - CRD Names Are Correct**

All CRD names already use signal-agnostic terminology:
- ✅ `RemediationRequest` (not AlertRemediation)
- ✅ `RemediationProcessing` (not AlertProcessing)
- ✅ `AIAnalysis` (signal-agnostic)
- ✅ `WorkflowExecution` (signal-agnostic)
- ✅ `KubernetesExecution` (signal-agnostic)

**Conclusion**: CRD names follow correct pattern per ADR-015.

---

### **❌ ISSUES FOUND - Field Names Use "Alert" Prefix**

**3 fields** in RemediationRequest spec use "Alert" prefix:

| Current Name | Occurrences | Should Be | Reason |
|--------------|-------------|-----------|--------|
| `alertFingerprint` | 7 in CRD_SCHEMAS.md | `signalFingerprint` | Works for all signal types |
| `alertName` | 7 in CRD_SCHEMAS.md | `signalName` | Works for all signal types |
| `alertLabels` | 0 in schemas (proposed) | `signalLabels` | Proposed new field |
| `alertAnnotations` | 0 in schemas (proposed) | `signalAnnotations` | Proposed new field |

**Total**: 4 field names need renaming.

---

## 🔍 **DETAILED FINDINGS BY DOCUMENT**

### **Document 1: CRD_SCHEMAS.md** (Authoritative)

**File**: `docs/architecture/CRD_SCHEMAS.md`

**Line 95-98**: RemediationRequest spec definition
```go
// ❌ CURRENT (Alert prefix)
AlertFingerprint string `json:"alertFingerprint"`
AlertName string `json:"alertName"`

// ✅ SHOULD BE (Signal-agnostic)
SignalFingerprint string `json:"signalFingerprint"`
SignalName string `json:"signalName"`
```

**Line 590-591**: Field table
```markdown
| Field | Type | Description |
|-------|------|-------------|
| `alertFingerprint` | string | Unique fingerprint for deduplication |  ❌
| `alertName` | string | Human-readable signal name |              ❌

# SHOULD BE:
| `signalFingerprint` | string | Unique fingerprint for deduplication | ✅
| `signalName` | string | Human-readable signal name |             ✅
```

**Line 625-626**: Cross-service field usage
```markdown
| Field | Gateway | RemediationProcessor | AIAnalysis | WorkflowExecution |
| `alertFingerprint` | ✅ Creates | ... | ... | ... |  ❌

# SHOULD BE:
| `signalFingerprint` | ✅ Creates | ... | ... | ... | ✅
```

**Line 650, 661**: Code examples
```go
// ❌ CURRENT
type RemediationRequestSpec struct {
    AlertFingerprint string

// ✅ SHOULD BE
type RemediationRequestSpec struct {
    SignalFingerprint string
```

**Total in CRD_SCHEMAS.md**: 7 occurrences need fixing

---

### **Document 2: CRD_SCHEMA_UPDATE_ACTION_PLAN.md** (Recent triage)

**File**: `docs/analysis/CRD_SCHEMA_UPDATE_ACTION_PLAN.md`

**Occurrences**: 19 lines

**Issues**:
1. Line 22: `alertFingerprint` in current state description
2. Lines 53-54: Proposed `AlertLabels` and `AlertAnnotations` (NEW fields)
3. Lines 81-82: Proposed spec with `AlertFingerprint` and `AlertName`
4. Lines 189-190: Code example using Alert prefix
5. Lines 202-203: Code example using Alert prefix
6. Lines 278-279: Code example using Alert prefix
7. Lines 307-308: Field mapping table
8. Lines 312-313: Field mapping table (NEW fields)
9. Lines 337-338: Checklist items
10. Lines 390-391: Validation checklist

**All 19 occurrences** need renaming.

---

### **Document 3: CRD_DATA_FLOW_TRIAGE_REVISED.md** (Recent triage)

**File**: `docs/analysis/CRD_DATA_FLOW_TRIAGE_REVISED.md`

**Occurrences**: 20 lines

**Issues**:
1. Lines 54-55: Data needs analysis
2. Lines 110-111: Field assessment table
3. Lines 152-153: Proposed spec code
4. Lines 259-260: Field mapping
5. Lines 326-327: Proposed new fields
6. Lines 466-467: Code example
7. Lines 560-561: Proposed new fields (repeated)
8. Lines 579-580: Spec structure
9. Lines 628-629: Priority list
10. Lines 662, 727: Action items

**All 20 occurrences** need renaming.

---

## 📋 **PROPOSED FIELD RENAMINGS**

### **Renaming Table**:

| Current Name | New Name | JSON Tag | Rationale |
|--------------|----------|----------|-----------|
| `AlertFingerprint` | `SignalFingerprint` | `signalFingerprint` | Generic for all signal types |
| `AlertName` | `SignalName` | `signalName` | Generic for all signal types |
| `AlertLabels` | `SignalLabels` | `signalLabels` | Generic for all signal types |
| `AlertAnnotations` | `SignalAnnotations` | `signalAnnotations` | Generic for all signal types |

**Note**: JSON tags also need updating for consistency.

---

## 🔄 **IMPACT ANALYSIS**

### **Breaking Change Assessment**:

**CRD Field Changes**: ✅ **NO IMPACT** - Pre-release, no backward compatibility needed

**Affected Components**:
1. ✅ **CRD Schemas** (authoritative documentation)
2. ✅ **Action Plan Document** (recent triage - not yet implemented)
3. ✅ **Data Flow Triage** (recent triage - not yet implemented)
4. ⏸️ **Implementation** (future - not yet built)

**Conclusion**: Safe to rename now before implementation begins.

---

### **Consistency Check**:

**Already Signal-Agnostic** ✅:
- `severity` (not "alertSeverity")
- `environment` (not "alertEnvironment")
- `priority` (not "alertPriority")
- `signalType` (correct - already signal-agnostic)
- `signalSource` (correct - already signal-agnostic)
- `firingTime` (not "alertFiringTime")
- `receivedTime` (not "alertReceivedTime")

**Pattern**: Only `AlertFingerprint`, `AlertName`, `AlertLabels`, `AlertAnnotations` use Alert prefix.

---

## ✅ **RECOMMENDED CHANGES**

### **Change 1: CRD_SCHEMAS.md** (Authoritative)

**File**: `docs/architecture/CRD_SCHEMAS.md`
**Priority**: **P0 - CRITICAL** (Authoritative schema)
**Occurrences**: 7 lines

**Changes**:
```go
// RemediationRequestSpec
type RemediationRequestSpec struct {
    // Core Signal Identification
    // ❌ AlertFingerprint string `json:"alertFingerprint"`
    // ❌ AlertName string `json:"alertName"`

    // ✅ SignalFingerprint string `json:"signalFingerprint"`
    // ✅ SignalName string `json:"signalName"`

    // ... other fields remain unchanged ...
}
```

**Field Table Update**:
```markdown
| Field | Type | Description |
|-------|------|-------------|
| `signalFingerprint` | string | Unique fingerprint for deduplication | ✅
| `signalName` | string | Human-readable signal name |             ✅
```

**Cross-Service Usage Table**:
```markdown
| Field | Gateway | RemediationProcessor | AIAnalysis | WorkflowExecution |
| `signalFingerprint` | ✅ Creates | ✅ Uses | ✅ Uses | ✅ Uses | ✅
```

---

### **Change 2: CRD_SCHEMA_UPDATE_ACTION_PLAN.md**

**File**: `docs/analysis/CRD_SCHEMA_UPDATE_ACTION_PLAN.md`
**Priority**: **P1 - HIGH** (Recent triage document)
**Occurrences**: 19 lines

**Changes**:
```go
// Proposed NEW fields in RemediationRequest
type RemediationRequestSpec struct {
    // ❌ AlertLabels      map[string]string `json:"alertLabels,omitempty"`
    // ❌ AlertAnnotations map[string]string `json:"alertAnnotations,omitempty"`

    // ✅ SignalLabels      map[string]string `json:"signalLabels,omitempty"`
    // ✅ SignalAnnotations map[string]string `json:"signalAnnotations,omitempty"`
}

// Proposed RemediationProcessing spec
type RemediationProcessingSpec struct {
    // ❌ AlertFingerprint string `json:"alertFingerprint"`
    // ❌ AlertName        string `json:"alertName"`

    // ✅ SignalFingerprint string `json:"signalFingerprint"`
    // ✅ SignalName        string `json:"signalName"`
}
```

**Field Mapping Table**:
```markdown
| RemediationRequest | RemediationProcessing | Priority | Notes |
| `signalFingerprint` | `signalFingerprint` | **P0** | Direct copy | ✅
| `signalName` | `signalName` | **P0** | Direct copy |               ✅
| `signalLabels` | `labels` | **P0** | Direct copy (NEW field) |    ✅
| `signalAnnotations` | `annotations` | **P0** | Direct copy (NEW field) | ✅
```

---

### **Change 3: CRD_DATA_FLOW_TRIAGE_REVISED.md**

**File**: `docs/analysis/CRD_DATA_FLOW_TRIAGE_REVISED.md`
**Priority**: **P1 - HIGH** (Recent triage document)
**Occurrences**: 20 lines

**Changes**: Same pattern as Change 2 (replace all Alert prefix occurrences).

---

## 📊 **CHANGE SUMMARY**

### **Total Changes Required**:

| Document | Occurrences | Priority | Effort |
|----------|-------------|----------|--------|
| `CRD_SCHEMAS.md` | 7 | **P0** | 15 min |
| `CRD_SCHEMA_UPDATE_ACTION_PLAN.md` | 19 | **P1** | 20 min |
| `CRD_DATA_FLOW_TRIAGE_REVISED.md` | 20 | **P1** | 20 min |
| **Total** | **46** | - | **55 min** |

**Estimated Total Effort**: ~1 hour

---

## 🎯 **SEARCH & REPLACE PATTERNS**

### **Pattern 1: Go Struct Fields**
```regex
Search:  AlertFingerprint
Replace: SignalFingerprint

Search:  AlertName
Replace: SignalName

Search:  AlertLabels
Replace: SignalLabels

Search:  AlertAnnotations
Replace: SignalAnnotations
```

### **Pattern 2: JSON Tags**
```regex
Search:  alertFingerprint
Replace: signalFingerprint

Search:  alertName
Replace: signalName

Search:  alertLabels
Replace: signalLabels

Search:  alertAnnotations
Replace: signalAnnotations
```

### **Pattern 3: Markdown Tables and Text**
```regex
Search:  `alertFingerprint`
Replace: `signalFingerprint`

Search:  `alertName`
Replace: `signalName`

Search:  `alertLabels`
Replace: `signalLabels`

Search:  `alertAnnotations`
Replace: `signalAnnotations`
```

---

## ✅ **VALIDATION CHECKLIST**

After changes, verify:

### **Field Names**:
- [ ] No `AlertFingerprint` in any document
- [ ] No `AlertName` in any document
- [ ] No `AlertLabels` in any document
- [ ] No `AlertAnnotations` in any document
- [ ] All replaced with `Signal*` variants

### **JSON Tags**:
- [ ] No `alertFingerprint` in JSON tags
- [ ] No `alertName` in JSON tags
- [ ] No `alertLabels` in JSON tags
- [ ] No `alertAnnotations` in JSON tags
- [ ] All replaced with `signal*` variants

### **Consistency**:
- [ ] `signalType` remains unchanged (already correct)
- [ ] `signalSource` remains unchanged (already correct)
- [ ] Other fields remain unchanged

### **Documentation**:
- [ ] Field tables updated
- [ ] Code examples updated
- [ ] Mapping tables updated
- [ ] Comments updated

---

## 🔗 **RELATED DOCUMENTS**

**Architecture Decisions**:
- `docs/architecture/decisions/ADR-015-alert-to-signal-naming-migration.md` - Signal naming rationale

**CRD Schemas**:
- `docs/architecture/CRD_SCHEMAS.md` - Authoritative schema (**P0** - fix first)

**Recent Triage Documents**:
- `docs/analysis/CRD_SCHEMA_UPDATE_ACTION_PLAN.md` - Action plan (**P1** - fix)
- `docs/analysis/CRD_DATA_FLOW_TRIAGE_REVISED.md` - Data flow triage (**P1** - fix)

**Service Documentation** (verify after implementation):
- `docs/services/stateless/gateway-service/crd-integration.md`
- `docs/services/crd-controllers/01-remediationprocessor/crd-schema.md`
- `docs/services/crd-controllers/05-remediationorchestrator/integration-points.md`

---

## 🎯 **EXECUTION PLAN**

### **Step 1: Fix Authoritative Schema** (15 min)
- [ ] Update `docs/architecture/CRD_SCHEMAS.md`
- [ ] Replace all 7 occurrences
- [ ] Verify JSON tags updated

### **Step 2: Fix Action Plan** (20 min)
- [ ] Update `docs/analysis/CRD_SCHEMA_UPDATE_ACTION_PLAN.md`
- [ ] Replace all 19 occurrences
- [ ] Update field mapping tables
- [ ] Update code examples

### **Step 3: Fix Data Flow Triage** (20 min)
- [ ] Update `docs/analysis/CRD_DATA_FLOW_TRIAGE_REVISED.md`
- [ ] Replace all 20 occurrences
- [ ] Update field assessment tables
- [ ] Update code examples

### **Step 4: Validation** (5 min)
- [ ] Run grep to verify no `Alert[A-Z]` in field names
- [ ] Run grep to verify no `alert[A-Z]` in JSON tags
- [ ] Review changes for consistency

### **Step 5: Commit** (5 min)
- [ ] Git add changed files
- [ ] Commit with descriptive message
- [ ] Reference ADR-015 in commit message

**Total Time**: ~1 hour

---

## 📝 **COMMIT MESSAGE TEMPLATE**

```
docs(crd): Rename Alert prefix fields to Signal prefix (ADR-015)

Updated CRD schema field names to use signal-agnostic terminology per
ADR-015. Kubernaut now supports multiple signal types beyond Prometheus
alerts (K8s events, AWS CloudWatch, Azure Monitor, Datadog, custom webhooks).

FIELD RENAMINGS (4 fields):

1. AlertFingerprint → SignalFingerprint
   - JSON: alertFingerprint → signalFingerprint
   - Rationale: Works for all signal types

2. AlertName → SignalName
   - JSON: alertName → signalName
   - Rationale: Generic signal identifier

3. AlertLabels → SignalLabels (proposed new field)
   - JSON: alertLabels → signalLabels
   - Rationale: Applies to all signal types

4. AlertAnnotations → SignalAnnotations (proposed new field)
   - JSON: alertAnnotations → signalAnnotations
   - Rationale: Applies to all signal types

FILES UPDATED (3):
- docs/architecture/CRD_SCHEMAS.md (7 occurrences)
- docs/analysis/CRD_SCHEMA_UPDATE_ACTION_PLAN.md (19 occurrences)
- docs/analysis/CRD_DATA_FLOW_TRIAGE_REVISED.md (20 occurrences)

Total: 46 occurrences replaced

VALIDATION:
✅ No Alert* field names remaining
✅ No alert* JSON tags remaining
✅ CRD names already signal-agnostic (no changes needed)
✅ Consistent with existing signal-agnostic fields (signalType, signalSource)

IMPACT:
✅ Pre-release - no backward compatibility concerns
✅ Documentation only - implementation not yet started

Reference: ADR-015 (Alert to Signal Naming Migration)
```

---

## 🎯 **FINAL RECOMMENDATION**

**Status**: ⚠️ **ALERT PREFIX FOUND** - 4 field names need renaming
**Action**: Execute search & replace across 3 documents
**Priority**: **P0 - HIGH** (Before implementation begins)
**Effort**: ~1 hour
**Impact**: **LOW** (Documentation only, no code changes yet)

**Recommendation**: **PROCEED IMMEDIATELY** with renaming to ensure consistency before implementation.

---

**Triage Complete**: October 8, 2025
**Next Step**: Execute Step 1 (Fix authoritative schema)


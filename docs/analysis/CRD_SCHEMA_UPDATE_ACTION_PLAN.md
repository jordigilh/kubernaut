# CRD Schema Update Action Plan - Self-Contained RemediationProcessing

**Date**: October 8, 2025
**Purpose**: Action plan for updating CRD schemas to implement self-contained RemediationProcessing pattern
**Context**: No backward compatibility constraints - schemas can be freely updated

---

## 🎯 **GOAL**

Update CRD schemas to ensure RemediationProcessing contains **all data it needs** from Gateway without requiring cross-CRD reads during reconciliation.

**Pattern**: Self-contained CRDs for performance, reliability, and isolation.

---

## 📊 **CURRENT STATE vs TARGET STATE**

### **Current State** ❌

RemediationProcessing has minimal data (4 fields):
- `signalFingerprint`, `severity`
- `namespace` (derived)
- `labels`, `annotations`

**Missing**: 14 of 18 Gateway fields (78% data loss)

### **Target State** ✅

RemediationProcessing has **all 18 required fields**:
- 7 HIGH priority (critical)
- 8 MEDIUM priority (important)
- 3 LOW priority (nice to have)

**Result**: 100% data availability for RemediationProcessor

---

## 📋 **REQUIRED SCHEMA CHANGES**

### **Change 1: RemediationRequest Schema** (Gateway Service)

**File**: `docs/architecture/CRD_SCHEMAS.md`
**Priority**: **P0 - CRITICAL**
**Effort**: 2-3 hours

**Add Top-Level Fields**:
```go
type RemediationRequestSpec struct {
    // ... existing fields ...

    // ✅ ADD: Structured signal metadata (no payload parsing needed)
    SignalLabels      map[string]string `json:"signalLabels,omitempty"`
    SignalAnnotations map[string]string `json:"signalAnnotations,omitempty"`
}
```

**Gateway Implementation**: Extract labels/annotations from webhook payload and populate as structured fields.

**Benefit**: RemediationProcessor gets structured data without parsing `originalPayload`.

---

### **Change 2: RemediationProcessing Schema** (RemediationProcessor Service)

**File**: `docs/services/crd-controllers/01-remediationprocessor/crd-schema.md`
**Priority**: **P0 - CRITICAL**
**Effort**: 3-4 hours

**Complete Spec Redesign**:
```go
type RemediationProcessingSpec struct {
    // ========================================
    // PARENT REFERENCE (Always Required)
    // ========================================
    RemediationRequestRef corev1.ObjectReference `json:"remediationRequestRef"`

    // ========================================
    // SIGNAL IDENTIFICATION (HIGH PRIORITY)
    // ========================================
    SignalFingerprint string `json:"signalFingerprint"` // ✅ Correlation
    SignalName        string `json:"signalName"`        // ✅ Validation
    Severity          string `json:"severity"`          // ✅ Classification

    // ========================================
    // RESOURCE TARGETING (CRITICAL)
    // ========================================
    TargetResource ResourceIdentifier `json:"targetResource"` // ✅ NEW
    ProviderData   json.RawMessage    `json:"providerData"`   // ✅ K8s URLs

    // ========================================
    // SIGNAL METADATA (HIGH PRIORITY)
    // ========================================
    Labels       map[string]string `json:"labels"`       // ✅ NEW
    Annotations  map[string]string `json:"annotations"`  // ✅ NEW
    FiringTime   metav1.Time       `json:"firingTime"`   // ✅ NEW
    ReceivedTime metav1.Time       `json:"receivedTime"` // ✅ NEW

    // ========================================
    // BUSINESS CONTEXT (MEDIUM PRIORITY)
    // ========================================
    Priority        string `json:"priority"`        // ✅ NEW - P0/P1/P2
    EnvironmentHint string `json:"environmentHint"` // ✅ NEW - Gateway hint

    // ========================================
    // CORRELATION CONTEXT (MEDIUM PRIORITY)
    // ========================================
    Deduplication DeduplicationContext `json:"deduplication"` // ✅ NEW
    IsStorm         bool                `json:"isStorm"`       // ✅ NEW
    StormAlertCount int                 `json:"stormAlertCount"` // ✅ NEW

    // ========================================
    // SIGNAL CLASSIFICATION (LOW PRIORITY)
    // ========================================
    SignalType string `json:"signalType"` // ✅ NEW
    TargetType string `json:"targetType"` // ✅ NEW

    // ========================================
    // FALLBACK DATA (LOW PRIORITY)
    // ========================================
    OriginalPayload []byte `json:"originalPayload,omitempty"` // ✅ NEW

    // ========================================
    // PROCESSING CONFIGURATION (EXISTING)
    // ========================================
    EnrichmentConfig              EnrichmentConfig              `json:"enrichmentConfig,omitempty"`
    EnvironmentClassification     EnvironmentClassificationConfig `json:"environmentClassification,omitempty"`
}

// ✅ NEW: Explicit resource targeting
type ResourceIdentifier struct {
    Kind      string `json:"kind"`      // Pod, Deployment, StatefulSet, etc.
    Name      string `json:"name"`      // Resource name
    Namespace string `json:"namespace"` // Resource namespace
}

// ✅ NEW: Deduplication context
type DeduplicationContext struct {
    OccurrenceCount int         `json:"occurrenceCount"` // How many times seen
    FirstSeen       metav1.Time `json:"firstSeen"`       // First occurrence
    LastSeen        metav1.Time `json:"lastSeen"`        // Latest occurrence
}
```

**Changes Summary**:
- ✅ Add 15 new fields
- ✅ Add 2 new types (ResourceIdentifier, DeduplicationContext)
- ✅ Keep existing configuration fields

---

### **Change 3: RemediationOrchestrator Mapping Logic** (RemediationOrchestrator Service)

**File**: `docs/services/crd-controllers/05-remediationorchestrator/integration-points.md`
**Priority**: **P1 - HIGH**
**Effort**: 2-3 hours

**Document Complete Mapping**:
```go
func (r *RemediationRequestReconciler) createRemediationProcessing(
    ctx context.Context,
    remReq *remediationv1.RemediationRequest,
) (*remediationprocessingv1.RemediationProcessing, error) {

    // Parse provider data for target resource
    var k8sData KubernetesProviderData
    json.Unmarshal(remReq.Spec.ProviderData, &k8sData)

    remProc := &remediationprocessingv1.RemediationProcessing{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-processing", remReq.Name),
            Namespace: remReq.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(remReq, remediationv1.GroupVersion.WithKind("RemediationRequest")),
            },
        },
        Spec: remediationprocessingv1.RemediationProcessingSpec{
            // Parent reference
            RemediationRequestRef: corev1.ObjectReference{
                Name:      remReq.Name,
                Namespace: remReq.Namespace,
            },

            // ========================================
            // COPY ALL 18 FIELDS
            // ========================================

            // Signal identification
            SignalFingerprint: remReq.Spec.SignalFingerprint,
            SignalName:        remReq.Spec.SignalName,
            Severity:          remReq.Spec.Severity,

            // Resource targeting
            TargetResource: remediationprocessingv1.ResourceIdentifier{
                Kind:      k8sData.Resource.Kind,
                Name:      k8sData.Resource.Name,
                Namespace: k8sData.Resource.Namespace,
            },
            ProviderData: remReq.Spec.ProviderData,

            // Signal metadata
            Labels:       remReq.Spec.SignalLabels,      // ✅ From new field
            Annotations:  remReq.Spec.SignalAnnotations, // ✅ From new field
            FiringTime:   remReq.Spec.FiringTime,
            ReceivedTime: remReq.Spec.ReceivedTime,

            // Business context
            Priority:        remReq.Spec.Priority,
            EnvironmentHint: remReq.Spec.Environment,

            // Correlation context
            Deduplication: remediationprocessingv1.DeduplicationContext{
                OccurrenceCount: remReq.Spec.Deduplication.OccurrenceCount,
                FirstSeen:       remReq.Spec.Deduplication.FirstSeen,
                LastSeen:        remReq.Spec.Deduplication.LastSeen,
            },
            IsStorm:         remReq.Spec.IsStorm,
            StormAlertCount: remReq.Spec.StormAlertCount,

            // Signal classification
            SignalType: remReq.Spec.SignalType,
            TargetType: remReq.Spec.TargetType,

            // Fallback data
            OriginalPayload: remReq.Spec.OriginalPayload,

            // Configuration
            EnrichmentConfig: remediationprocessingv1.EnrichmentConfig{
                ContextSources:     []string{"kubernetes"},
                ContextDepth:       determineEnrichmentDepth(remReq), // Adaptive
                HistoricalLookback: "24h",
            },
            EnvironmentClassification: remediationprocessingv1.EnvironmentClassificationConfig{
                ClassificationSources: []string{"labels", "annotations", "patterns"},
                ConfidenceThreshold:   0.8,
            },
        },
    }

    return remProc, r.Create(ctx, remProc)
}

// Adaptive enrichment depth based on priority/storm
func determineEnrichmentDepth(remReq *remediationv1.RemediationRequest) string {
    if remReq.Spec.Priority == "P0" {
        return "comprehensive" // Full enrichment for critical
    }
    if remReq.Spec.IsStorm {
        return "basic" // Lighter for storm alerts
    }
    return "detailed" // Normal enrichment
}
```

---

### **Change 4: Gateway CRD Integration** (Gateway Service)

**File**: `docs/services/stateless/gateway-service/crd-integration.md`
**Priority**: **P1 - HIGH**
**Effort**: 1-2 hours

**Update CRD Creation Logic**:
```go
func (s *Server) createRemediationRequestCRD(
    ctx context.Context,
    signal *NormalizedSignal,
    isStorm bool,
    stormMetadata *processing.StormMetadata,
) (*remediationv1.RemediationRequest, error) {

    cr := &remediationv1.RemediationRequest{
        // ... existing metadata ...
        Spec: remediationv1.RemediationRequestSpec{
            // ... existing fields ...

            // ✅ ADD: Structured signal metadata
            SignalLabels:      extractLabels(signal.RawPayload),
            SignalAnnotations: extractAnnotations(signal.RawPayload),
        },
    }

    return cr, s.k8sClient.Create(ctx, cr)
}

// Extract structured labels from webhook payload
func extractLabels(payload []byte) map[string]string {
    var alert PrometheusAlert
    json.Unmarshal(payload, &alert)
    return alert.Labels
}

// Extract structured annotations from webhook payload
func extractAnnotations(payload []byte) map[string]string {
    var alert PrometheusAlert
    json.Unmarshal(payload, &alert)
    return alert.Annotations
}
```

---

## 📊 **FIELD MAPPING MATRIX**

| RemediationRequest | RemediationProcessing | Priority | Notes |
|--------------------|----------------------|----------|-------|
| `signalFingerprint` | `signalFingerprint` | **P0** | Direct copy |
| `signalName` | `signalName` | **P0** | Direct copy |
| `severity` | `severity` | **P0** | Direct copy |
| `providerData` | `providerData` | **P0** | Direct copy (full JSON) |
| `providerData.resource` | `targetResource` | **P0** | Parsed extraction |
| `signalLabels` | `labels` | **P0** | Direct copy (NEW field) |
| `signalAnnotations` | `annotations` | **P0** | Direct copy (NEW field) |
| `priority` | `priority` | **P1** | Direct copy |
| `firingTime` | `firingTime` | **P1** | Direct copy |
| `receivedTime` | `receivedTime` | **P1** | Direct copy |
| `deduplication.*` | `deduplication.*` | **P1** | Struct copy (3 fields) |
| `isStorm` | `isStorm` | **P1** | Direct copy |
| `stormAlertCount` | `stormAlertCount` | **P1** | Direct copy |
| `environment` | `environmentHint` | **P2** | Direct copy (renamed) |
| `signalType` | `signalType` | **P2** | Direct copy |
| `targetType` | `targetType` | **P2** | Direct copy |
| `originalPayload` | `originalPayload` | **P3** | Direct copy |

**Total**: 18 fields mapped

---

## 🎯 **IMPLEMENTATION PLAN**

### **Phase 1: Schema Documentation Updates** (1 week)

**Week 1**:

**Day 1-2: Gateway Schema** (Owner: Gateway team)
- [ ] Update `docs/architecture/CRD_SCHEMAS.md`
- [ ] Add `signalLabels` field
- [ ] Add `signalAnnotations` field
- [ ] Update examples

**Day 3-4: RemediationProcessing Schema** (Owner: RemediationProcessor team)
- [ ] Update `docs/services/crd-controllers/01-remediationprocessor/crd-schema.md`
- [ ] Add 15 new fields
- [ ] Add ResourceIdentifier type
- [ ] Add DeduplicationContext type
- [ ] Update examples

**Day 5: RemediationOrchestrator Documentation** (Owner: RemediationOrchestrator team)
- [ ] Update `docs/services/crd-controllers/05-remediationorchestrator/integration-points.md`
- [ ] Document complete field mapping
- [ ] Add code examples

---

### **Phase 2: Implementation** (Future - When Services Built)

**Gateway Service**:
- [ ] Update CRD creation logic
- [ ] Extract labels/annotations from webhook
- [ ] Populate new fields

**RemediationOrchestrator Service**:
- [ ] Implement field mapping logic
- [ ] Parse providerData for targetResource
- [ ] Copy all 18 fields to RemediationProcessing

**RemediationProcessor Service**:
- [ ] Read new fields from spec
- [ ] Use targetResource for enrichment
- [ ] Use priority for adaptive behavior

---

### **Phase 3: Data Flow Documentation** (1-2 days)

**Day 1-2**: Create comprehensive data flow diagram
- [ ] Create `docs/architecture/CRD_DATA_FLOW.md`
- [ ] Document all field mappings
- [ ] Show self-contained pattern
- [ ] Add sequence diagrams

---

## ✅ **VALIDATION CHECKLIST**

### **Schema Validation**:
- [ ] All 18 fields present in RemediationProcessing spec
- [ ] ResourceIdentifier type defined
- [ ] DeduplicationContext type defined
- [ ] signalLabels field in RemediationRequest
- [ ] signalAnnotations field in RemediationRequest

### **Documentation Validation**:
- [ ] Mapping logic documented
- [ ] Code examples provided
- [ ] Field priority levels clear
- [ ] Data flow diagram created

### **Implementation Validation** (Future):
- [ ] Gateway populates new fields
- [ ] RemediationOrchestrator copies all fields
- [ ] RemediationProcessor reads all fields
- [ ] No cross-CRD reads during reconciliation

---

## 📈 **EFFORT SUMMARY**

**Total Estimated Effort**: 10-15 hours (documentation only)

**Breakdown**:
1. Gateway schema update: 2-3 hours
2. RemediationProcessing schema update: 3-4 hours
3. RemediationOrchestrator documentation: 2-3 hours
4. Data flow diagram: 1-2 hours
5. Review and validation: 2-3 hours

**Timeline**: 1 week (documentation phase)

---

## 🔗 **AFFECTED DOCUMENTS**

**Schema Definitions**:
1. ✅ `docs/architecture/CRD_SCHEMAS.md` - RemediationRequest schema
2. ✅ `docs/services/crd-controllers/01-remediationprocessor/crd-schema.md` - RemediationProcessing schema

**Service Integration**:
3. ✅ `docs/services/stateless/gateway-service/crd-integration.md` - Gateway CRD creation
4. ✅ `docs/services/crd-controllers/05-remediationorchestrator/integration-points.md` - Orchestrator mapping

**Architecture**:
5. ✅ `docs/architecture/CRD_DATA_FLOW.md` - NEW - Complete data flow diagram

---

## 🎯 **SUCCESS CRITERIA**

### **Documentation Phase** ✅
- [ ] All schema documents updated
- [ ] Field mapping logic documented
- [ ] Code examples provided
- [ ] Data flow diagram created

### **Implementation Phase** (Future) ⏳
- [ ] Gateway creates CRDs with new fields
- [ ] RemediationOrchestrator copies all 18 fields
- [ ] RemediationProcessor operates self-contained
- [ ] No performance degradation

---

## 📝 **NOTES**

**No Backward Compatibility Concerns**: Schemas can be freely updated without versioning constraints.

**Self-Contained Pattern Benefits**:
- ✅ Faster reconciliation (no cross-CRD reads)
- ✅ Better reliability (works if parent deleted)
- ✅ Clearer isolation (each controller independent)
- ✅ Simpler testing (all data in spec)

**Priority Focus**: Start with P0/P1 fields (15 fields), add P2/P3 later if needed.

---

**Action Plan Complete**: October 8, 2025
**Next Step**: Execute Phase 1 (Schema Documentation Updates)
**Owner**: Architecture team to coordinate with service teams


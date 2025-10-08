# CRD Data Flow Triage - REVISED: Self-Contained CRD Pattern

**Date**: October 8, 2025  
**Purpose**: Triage RemediationProcessing CRD to ensure it contains all data needed for self-contained operation  
**Scope**: Gateway Service → RemediationRequest → RemediationProcessing CRD  
**Architecture Pattern**: **Self-Contained CRDs** (no cross-CRD reads during reconciliation)

---

## 🎯 **REVISED UNDERSTANDING**

**Correct Pattern**: RemediationProcessing CRD MUST be **self-contained** with all data it needs copied from RemediationRequest.

**Rationale**:
1. ✅ **No Cross-CRD Reads**: Controllers don't fetch parent CRDs during reconciliation
2. ✅ **Performance**: Faster reconciliation (all data in spec)
3. ✅ **Reliability**: Works even if parent CRD deleted/unavailable
4. ✅ **Isolation**: Each controller operates independently

**Key Principle**: RemediationOrchestrator (RemediationRequest controller) copies **exactly what RemediationProcessor needs** from RemediationRequest → RemediationProcessing spec.

---

## 📊 **REMEDIATIONPROCESSOR CONTROLLER RESPONSIBILITIES**

Based on `docs/services/crd-controllers/01-remediationprocessor/overview.md`:

### **Core Responsibilities**:
1. **Enrich alerts with Kubernetes context** (pods, deployments, nodes) → ~8KB context
2. **Classify environment tier** (production, staging, development) with business criticality
3. **Validate alert completeness** and readiness for AI analysis
4. **Update status** for RemediationRequest controller to trigger next phase

### **V1 Scope**:
- Single enrichment provider: Context Service
- Environment classification with fallback heuristics
- Basic alert validation
- **Targeting data ONLY** (namespace, resource kind/name, Kubernetes context)
- **NO log/metric storage** (HolmesGPT fetches dynamically)

### **Key Decisions**:
- **Single-phase synchronous processing** (~3 seconds)
- **Degraded mode** when Context Service unavailable
- **Does NOT create AIAnalysis CRD** (RemediationOrchestrator responsibility)
- **No duplicate detection** (Gateway responsibility)

---

## 🔍 **DATA NEEDS ANALYSIS**

### **What RemediationProcessor NEEDS to Do Its Job**:

#### **1. Core Signal Identification** (For validation and targeting)
- ✅ `alertFingerprint` - Unique identifier for correlation
- ✅ `alertName` - Human-readable name
- ✅ `severity` - critical/warning/info (for validation)

#### **2. Resource Targeting** (For Kubernetes context enrichment)
- ✅ `namespace` - Target namespace for context lookup
- ✅ `resource` - Target resource (kind, name, namespace) for enrichment
- ⚠️ **Currently**: Derived from providerData
- ✅ **Needed**: Explicit resource targeting fields

#### **3. Environment Classification Input**
- ⚠️ `environment` - Gateway provides classification, but RemediationProcessor may override
- ✅ `namespace` - Used for label-based classification
- ✅ `labels` - For classification rules
- ✅ `annotations` - For classification rules

#### **4. Alert Metadata** (For validation and correlation)
- ✅ `labels` - Alert labels from Prometheus
- ✅ `annotations` - Alert annotations
- ⚠️ `startsAt`, `endsAt` - Alert timing (from originalPayload)

#### **5. Provider Context** (For enrichment)
- ✅ `providerData` - Kubernetes-specific context (namespace, resource, URLs)
- ❓ `alertmanagerURL` - For reference in enrichment
- ❓ `grafanaURL` - For reference in enrichment
- ❓ `prometheusQuery` - For context

#### **6. Signal Classification** (For adaptive behavior)
- ⚠️ `signalType` - prometheus/kubernetes-event (may affect enrichment strategy)
- ⚠️ `targetType` - kubernetes/aws/azure (V1: kubernetes only)

#### **7. Temporal Data** (For timeout handling - BR-AP-062)
- ⚠️ `firingTime` - When signal started (for age calculation)
- ⚠️ `receivedTime` - When Gateway received (for latency tracking)

#### **8. Deduplication Context** (For correlation - BR-AP-061)
- ⚠️ `deduplication.occurrenceCount` - How many times seen
- ⚠️ `deduplication.firstSeen` - First occurrence timestamp
- ⚠️ `deduplication.lastSeen` - Latest occurrence timestamp

#### **9. Storm Detection** (For prioritization)
- ⚠️ `isStorm` - Part of alert storm (may affect enrichment depth)
- ⚠️ `stormAlertCount` - Number of alerts in storm

#### **10. Priority** (For business criticality classification)
- ⚠️ `priority` - Gateway's Rego-assigned priority (P0/P1/P2)

#### **11. Original Payload** (For audit and fallback)
- ❓ `originalPayload` - Raw webhook payload (for degraded mode fallback)

---

## 📋 **FIELD-BY-FIELD NEEDS ASSESSMENT**

| Gateway Field | RemediationProcessor Needs? | Why? | Priority |
|---------------|----------------------------|------|----------|
| `alertFingerprint` | ✅ **YES** | Validation, correlation | **HIGH** |
| `alertName` | ✅ **YES** | Validation, human context | **HIGH** |
| `severity` | ✅ **YES** | Validation, classification | **HIGH** |
| `environment` | ⚠️ **INPUT** | Classification input (may override) | **MEDIUM** |
| `priority` | ✅ **YES** | Business criticality context | **MEDIUM** |
| `signalType` | ⚠️ **MAYBE** | Adaptive enrichment strategy | **LOW** |
| `signalSource` | ❌ **NO** | Not needed for enrichment | **NONE** |
| `targetType` | ⚠️ **MAYBE** | V1: Always kubernetes | **LOW** |
| `firingTime` | ✅ **YES** | Timeout handling (BR-AP-062) | **MEDIUM** |
| `receivedTime` | ✅ **YES** | Latency tracking | **MEDIUM** |
| `deduplication.occurrenceCount` | ✅ **YES** | Correlation (BR-AP-061) | **MEDIUM** |
| `deduplication.firstSeen` | ✅ **YES** | Pattern detection | **LOW** |
| `deduplication.lastSeen` | ✅ **YES** | Recency tracking | **LOW** |
| `deduplication.isDuplicate` | ❌ **NO** | Gateway already filtered | **NONE** |
| `deduplication.previousRef` | ❌ **NO** | Not needed for processing | **NONE** |
| `isStorm` | ✅ **YES** | Enrichment depth adjustment | **MEDIUM** |
| `stormType` | ⚠️ **MAYBE** | Context for analysis | **LOW** |
| `stormWindow` | ❌ **NO** | Not needed for processing | **NONE** |
| `stormAlertCount` | ✅ **YES** | Prioritization context | **LOW** |
| `providerData` | ✅ **YES** | Kubernetes context (namespace, resource, URLs) | **CRITICAL** |
| `originalPayload` | ⚠️ **MAYBE** | Degraded mode fallback | **LOW** |
| `timeoutConfig` | ❌ **NO** | RemediationRequest handles timeouts | **NONE** |

---

## ✅ **RECOMMENDED REMEDIATIONPROCESSING SPEC (COMPLETE)**

### **Redesigned Spec Structure**:

```go
// RemediationProcessingSpec defines the desired state of RemediationProcessing
type RemediationProcessingSpec struct {
    // ========================================
    // PARENT REFERENCE (Always Required)
    // ========================================
    RemediationRequestRef corev1.ObjectReference `json:"remediationRequestRef"`
    
    // ========================================
    // SIGNAL IDENTIFICATION (From Gateway)
    // ========================================
    
    // Core identifiers
    AlertFingerprint string `json:"alertFingerprint"` // ✅ HIGH - Correlation
    AlertName        string `json:"alertName"`        // ✅ HIGH - Human context
    Severity         string `json:"severity"`         // ✅ HIGH - Validation
    
    // ========================================
    // RESOURCE TARGETING (From Gateway)
    // ========================================
    
    // Target resource for enrichment
    TargetResource ResourceIdentifier `json:"targetResource"` // ✅ CRITICAL
    
    // Provider-specific context (Kubernetes URLs, etc.)
    ProviderData json.RawMessage `json:"providerData"` // ✅ CRITICAL
    
    // ========================================
    // SIGNAL METADATA (From Gateway)
    // ========================================
    
    // Alert labels and annotations (for classification)
    Labels      map[string]string `json:"labels"`      // ✅ HIGH
    Annotations map[string]string `json:"annotations"` // ✅ HIGH
    
    // Temporal data
    FiringTime   metav1.Time `json:"firingTime"`   // ✅ MEDIUM - Timeout handling
    ReceivedTime metav1.Time `json:"receivedTime"` // ✅ MEDIUM - Latency tracking
    
    // ========================================
    // BUSINESS CONTEXT (From Gateway)
    // ========================================
    
    // Gateway-assigned priority (Rego policy)
    Priority string `json:"priority"` // ✅ MEDIUM - Business criticality (P0/P1/P2)
    
    // Gateway-assigned environment (may be overridden by RemediationProcessor)
    EnvironmentHint string `json:"environmentHint,omitempty"` // ⚠️ MEDIUM - Input for classification
    
    // ========================================
    // CORRELATION CONTEXT (From Gateway)
    // ========================================
    
    // Deduplication context for correlation (BR-AP-061)
    Deduplication DeduplicationContext `json:"deduplication"` // ✅ MEDIUM
    
    // Storm detection context
    IsStorm          bool `json:"isStorm,omitempty"`          // ✅ MEDIUM - Enrichment depth
    StormAlertCount  int  `json:"stormAlertCount,omitempty"`  // ✅ LOW - Prioritization
    
    // ========================================
    // SIGNAL CLASSIFICATION (From Gateway)
    // ========================================
    
    // Signal type (for adaptive behavior)
    SignalType string `json:"signalType"` // ⚠️ LOW - prometheus/kubernetes-event
    
    // Target platform type (V1: always kubernetes)
    TargetType string `json:"targetType"` // ⚠️ LOW - kubernetes/aws/azure
    
    // ========================================
    // FALLBACK DATA (From Gateway)
    // ========================================
    
    // Original payload for degraded mode fallback
    OriginalPayload []byte `json:"originalPayload,omitempty"` // ⚠️ LOW - Degraded mode
    
    // ========================================
    // PROCESSING CONFIGURATION
    // ========================================
    
    // Enrichment configuration
    EnrichmentConfig EnrichmentConfig `json:"enrichmentConfig,omitempty"`
    
    // Environment classification configuration
    EnvironmentClassification EnvironmentClassificationConfig `json:"environmentClassification,omitempty"`
}

// ResourceIdentifier identifies the target Kubernetes resource for enrichment
type ResourceIdentifier struct {
    Kind      string `json:"kind"`      // Pod, Deployment, StatefulSet, etc.
    Name      string `json:"name"`      // Resource name
    Namespace string `json:"namespace"` // Resource namespace
}

// DeduplicationContext provides correlation information
type DeduplicationContext struct {
    OccurrenceCount int         `json:"occurrenceCount"` // How many times seen
    FirstSeen       metav1.Time `json:"firstSeen"`       // First occurrence
    LastSeen        metav1.Time `json:"lastSeen"`        // Latest occurrence
}

// KubernetesProviderData (parsed from providerData JSON)
type KubernetesProviderData struct {
    Namespace       string             `json:"namespace"`
    Resource        ResourceIdentifier `json:"resource"`
    AlertmanagerURL string             `json:"alertmanagerURL,omitempty"`
    GrafanaURL      string             `json:"grafanaURL,omitempty"`
    PrometheusQuery string             `json:"prometheusQuery,omitempty"`
}
```

---

## 📊 **FIELD MAPPING: RemediationRequest → RemediationProcessing**

### **HIGH PRIORITY** (Critical for core functionality):

| RemediationRequest.spec | RemediationProcessing.spec | Mapping |
|------------------------|---------------------------|---------|
| `alertFingerprint` | `alertFingerprint` | ✅ Direct copy |
| `alertName` | `alertName` | ✅ Direct copy |
| `severity` | `severity` | ✅ Direct copy |
| `providerData` | `providerData` | ✅ Direct copy (json.RawMessage) |
| `labels` (from originalPayload) | `labels` | ⚠️ Parse from payload |
| `annotations` (from originalPayload) | `annotations` | ⚠️ Parse from payload |

**Action**: Gateway must expose `labels` and `annotations` as top-level fields in RemediationRequest OR RemediationOrchestrator parses from `originalPayload`.

---

### **MEDIUM PRIORITY** (Important for business logic):

| RemediationRequest.spec | RemediationProcessing.spec | Mapping |
|------------------------|---------------------------|---------|
| `priority` | `priority` | ✅ Direct copy |
| `environment` | `environmentHint` | ✅ Direct copy (renamed for clarity) |
| `firingTime` | `firingTime` | ✅ Direct copy |
| `receivedTime` | `receivedTime` | ✅ Direct copy |
| `deduplication.occurrenceCount` | `deduplication.occurrenceCount` | ✅ Direct copy |
| `deduplication.firstSeen` | `deduplication.firstSeen` | ✅ Direct copy |
| `deduplication.lastSeen` | `deduplication.lastSeen` | ✅ Direct copy |
| `isStorm` | `isStorm` | ✅ Direct copy |
| `stormAlertCount` | `stormAlertCount` | ✅ Direct copy |

---

### **LOW PRIORITY** (Nice to have, not critical):

| RemediationRequest.spec | RemediationProcessing.spec | Mapping |
|------------------------|---------------------------|---------|
| `signalType` | `signalType` | ✅ Direct copy |
| `targetType` | `targetType` | ✅ Direct copy |
| `originalPayload` | `originalPayload` | ✅ Direct copy (for degraded mode) |

---

### **NOT NEEDED** (Excluded from RemediationProcessing):

| RemediationRequest.spec | Reason NOT Needed |
|------------------------|-------------------|
| `signalSource` | Not used in enrichment logic |
| `deduplication.isDuplicate` | Gateway already filtered duplicates |
| `deduplication.previousRemediationRequestRef` | Not needed for processing |
| `stormType` | Context info, not actionable |
| `stormWindow` | Not needed for processing |
| `timeoutConfig` | RemediationRequest controller handles timeouts |

---

## 🚨 **MISSING DATA IN CURRENT DESIGN**

### **Issue 1: Labels and Annotations Not Top-Level** 🔥 HIGH SEVERITY

**Problem**: `labels` and `annotations` are inside `originalPayload` ([]byte), not accessible as structured fields.

**Impact**: RemediationProcessor cannot easily access labels/annotations for environment classification without parsing raw payload.

**Solution Options**:

#### **Option A: Gateway Exposes Labels/Annotations** ✅ RECOMMENDED
```go
// RemediationRequestSpec (ENHANCED)
type RemediationRequestSpec struct {
    // ... existing fields ...
    
    // ✅ ADD: Alert labels and annotations as top-level fields
    AlertLabels      map[string]string `json:"alertLabels,omitempty"`
    AlertAnnotations map[string]string `json:"alertAnnotations,omitempty"`
}
```

**Benefits**:
- ✅ RemediationProcessor gets structured data
- ✅ No payload parsing needed
- ✅ Consistent with Gateway's data collection role

---

#### **Option B: RemediationOrchestrator Parses Payload** ⚠️ ALTERNATIVE
```go
// RemediationOrchestrator creates RemediationProcessing
func (r *RemediationRequestReconciler) createRemediationProcessing(remReq *RemediationRequest) {
    // Parse originalPayload to extract labels/annotations
    labels, annotations := parsePrometheusPayload(remReq.Spec.OriginalPayload)
    
    remProc := &RemediationProcessing{
        Spec: RemediationProcessingSpec{
            Labels:      labels,
            Annotations: annotations,
            // ... other fields
        },
    }
}
```

**Drawbacks**:
- ❌ Duplicates Gateway parsing logic
- ❌ RemediationOrchestrator needs Prometheus payload knowledge

**Verdict**: **Option A preferred** - Gateway should expose structured fields.

---

### **Issue 2: Resource Targeting Not Explicit** ⚠️ MEDIUM SEVERITY

**Problem**: Target resource (kind, name, namespace) is inside `providerData` JSON, requiring parsing.

**Current**:
```go
type RemediationProcessingSpec struct {
    Alert struct {
        Namespace string // ⚠️ Derived, not explicit
    }
    ProviderData json.RawMessage // Contains resource info
}
```

**Solution**: Add explicit `targetResource` field:

```go
type RemediationProcessingSpec struct {
    TargetResource ResourceIdentifier `json:"targetResource"` // ✅ Explicit
    ProviderData   json.RawMessage    `json:"providerData"`   // Still available for URLs
}

type ResourceIdentifier struct {
    Kind      string `json:"kind"`      // Pod, Deployment, etc.
    Name      string `json:"name"`
    Namespace string `json:"namespace"`
}
```

**Mapping**:
```go
// RemediationOrchestrator extracts from providerData
var k8sData KubernetesProviderData
json.Unmarshal(remReq.Spec.ProviderData, &k8sData)

remProc := &RemediationProcessing{
    Spec: RemediationProcessingSpec{
        TargetResource: ResourceIdentifier{
            Kind:      k8sData.Resource.Kind,
            Name:      k8sData.Resource.Name,
            Namespace: k8sData.Resource.Namespace,
        },
        ProviderData: remReq.Spec.ProviderData, // Still pass full JSON
    },
}
```

---

### **Issue 3: Priority Not in Current Spec** ⚠️ MEDIUM SEVERITY

**Problem**: Gateway assigns priority via Rego policies (P0/P1/P2), but RemediationProcessing spec doesn't include it.

**Impact**: RemediationProcessor cannot use business priority for enrichment depth decisions.

**Solution**: Add `priority` field:
```go
type RemediationProcessingSpec struct {
    Priority string `json:"priority"` // P0/P1/P2 from Gateway
}
```

---

## ✅ **RECOMMENDED REMEDIATIONORCHESTRATOR MAPPING LOGIC**

### **Complete Data Copy Implementation**:

```go
// pkg/remediationorchestrator/orchestrator.go
func (r *RemediationRequestReconciler) createRemediationProcessing(
    ctx context.Context,
    remReq *remediationv1.RemediationRequest,
) (*remediationprocessingv1.RemediationProcessing, error) {
    
    // Parse provider data to extract target resource
    var k8sData KubernetesProviderData
    if err := json.Unmarshal(remReq.Spec.ProviderData, &k8sData); err != nil {
        return nil, fmt.Errorf("failed to parse provider data: %w", err)
    }
    
    // Parse original payload for labels/annotations
    // (IF Gateway doesn't expose them as top-level fields)
    labels, annotations := parseAlertLabels(remReq.Spec.OriginalPayload)
    
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
            // SIGNAL IDENTIFICATION (HIGH PRIORITY)
            // ========================================
            AlertFingerprint: remReq.Spec.AlertFingerprint,
            AlertName:        remReq.Spec.AlertName,
            Severity:         remReq.Spec.Severity,
            
            // ========================================
            // RESOURCE TARGETING (CRITICAL)
            // ========================================
            TargetResource: remediationprocessingv1.ResourceIdentifier{
                Kind:      k8sData.Resource.Kind,
                Name:      k8sData.Resource.Name,
                Namespace: k8sData.Resource.Namespace,
            },
            ProviderData: remReq.Spec.ProviderData, // Full JSON for URLs
            
            // ========================================
            // SIGNAL METADATA (HIGH PRIORITY)
            // ========================================
            Labels:       labels,
            Annotations:  annotations,
            FiringTime:   remReq.Spec.FiringTime,
            ReceivedTime: remReq.Spec.ReceivedTime,
            
            // ========================================
            // BUSINESS CONTEXT (MEDIUM PRIORITY)
            // ========================================
            Priority:        remReq.Spec.Priority,
            EnvironmentHint: remReq.Spec.Environment, // Gateway's classification as hint
            
            // ========================================
            // CORRELATION CONTEXT (MEDIUM PRIORITY)
            // ========================================
            Deduplication: remediationprocessingv1.DeduplicationContext{
                OccurrenceCount: remReq.Spec.Deduplication.OccurrenceCount,
                FirstSeen:       remReq.Spec.Deduplication.FirstSeen,
                LastSeen:        remReq.Spec.Deduplication.LastSeen,
            },
            IsStorm:         remReq.Spec.IsStorm,
            StormAlertCount: remReq.Spec.StormAlertCount,
            
            // ========================================
            // SIGNAL CLASSIFICATION (LOW PRIORITY)
            // ========================================
            SignalType: remReq.Spec.SignalType,
            TargetType: remReq.Spec.TargetType,
            
            // ========================================
            // FALLBACK DATA (LOW PRIORITY)
            // ========================================
            OriginalPayload: remReq.Spec.OriginalPayload,
            
            // ========================================
            // PROCESSING CONFIGURATION
            // ========================================
            EnrichmentConfig: remediationprocessingv1.EnrichmentConfig{
                ContextSources:     []string{"kubernetes"},
                ContextDepth:       determineEnrichmentDepth(remReq),
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

// determineEnrichmentDepth adjusts enrichment based on priority/storm
func determineEnrichmentDepth(remReq *remediationv1.RemediationRequest) string {
    if remReq.Spec.Priority == "P0" {
        return "comprehensive"
    }
    if remReq.Spec.IsStorm {
        return "basic" // Lighter enrichment for storm alerts
    }
    return "detailed"
}
```

---

## 📊 **SUMMARY OF REQUIRED CHANGES**

### **1. RemediationRequest Schema Enhancement** (Gateway Service)

**File**: `docs/architecture/CRD_SCHEMAS.md`

**Changes**:
```go
type RemediationRequestSpec struct {
    // ... existing fields ...
    
    // ✅ ADD: Alert labels and annotations (HIGH PRIORITY)
    AlertLabels      map[string]string `json:"alertLabels,omitempty"`
    AlertAnnotations map[string]string `json:"alertAnnotations,omitempty"`
}
```

**Impact**: Gateway must populate these fields from webhook payload.

---

### **2. RemediationProcessing Schema Enhancement**

**File**: `docs/services/crd-controllers/01-remediationprocessor/crd-schema.md`

**Changes**:
```go
type RemediationProcessingSpec struct {
    // ✅ ADD: All fields from "RECOMMENDED SPEC" above
    
    // Core identification
    AlertFingerprint string
    AlertName        string
    Severity         string
    
    // Resource targeting (CRITICAL)
    TargetResource ResourceIdentifier
    ProviderData   json.RawMessage
    
    // Signal metadata
    Labels       map[string]string
    Annotations  map[string]string
    FiringTime   metav1.Time
    ReceivedTime metav1.Time
    
    // Business context
    Priority        string
    EnvironmentHint string
    
    // Correlation context
    Deduplication DeduplicationContext
    IsStorm         bool
    StormAlertCount int
    
    // Signal classification
    SignalType string
    TargetType string
    
    // Fallback data
    OriginalPayload []byte
    
    // ✅ KEEP: Configuration fields
    EnrichmentConfig              EnrichmentConfig
    EnvironmentClassification     EnvironmentClassificationConfig
}
```

---

### **3. RemediationOrchestrator Mapping Logic**

**File**: `docs/services/crd-controllers/05-remediationorchestrator/integration-points.md`

**Changes**: Document complete field mapping logic (see "RECOMMENDED MAPPING LOGIC" above).

---

## 🎯 **PRIORITY LEVELS FOR IMPLEMENTATION**

### **P0 - CRITICAL** (Blocks core functionality):
1. ✅ `alertFingerprint` - Correlation
2. ✅ `alertName` - Validation
3. ✅ `severity` - Classification
4. ✅ `targetResource` - Enrichment targeting
5. ✅ `providerData` - Kubernetes context
6. ✅ `labels` - Classification
7. ✅ `annotations` - Classification

### **P1 - HIGH** (Important for business logic):
8. ✅ `priority` - Business criticality
9. ✅ `firingTime` - Timeout handling
10. ✅ `receivedTime` - Latency tracking
11. ✅ `deduplication.occurrenceCount` - Correlation
12. ✅ `isStorm` - Enrichment depth

### **P2 - MEDIUM** (Nice to have):
13. ⚠️ `environmentHint` - Classification input
14. ⚠️ `deduplication.firstSeen/lastSeen` - Pattern detection
15. ⚠️ `stormAlertCount` - Prioritization

### **P3 - LOW** (Optional):
16. ⚠️ `signalType` - Adaptive behavior
17. ⚠️ `targetType` - V1: Always kubernetes
18. ⚠️ `originalPayload` - Degraded mode fallback

---

## ✅ **RECOMMENDED ACTION ITEMS**

### **HIGH PRIORITY**:

1. **Add Labels/Annotations to RemediationRequest** (2-3 hours)
   - File: `docs/architecture/CRD_SCHEMAS.md`
   - File: `docs/services/stateless/gateway-service/crd-integration.md`
   - Action: Add `alertLabels` and `alertAnnotations` fields
   - Owner: Gateway Service team

2. **Update RemediationProcessing Spec** (3-4 hours)
   - File: `docs/services/crd-controllers/01-remediationprocessor/crd-schema.md`
   - Action: Add all fields from recommended spec
   - Owner: RemediationProcessor team

3. **Document Mapping Logic** (2-3 hours)
   - File: `docs/services/crd-controllers/05-remediationorchestrator/integration-points.md`
   - Action: Add complete field mapping example
   - Owner: RemediationOrchestrator team

### **MEDIUM PRIORITY**:

4. **Create Field Mapping Matrix** (1-2 hours)
   - File: `docs/architecture/CRD_DATA_FLOW.md` (NEW)
   - Action: Document all field mappings Gateway → RemediationRequest → RemediationProcessing
   - Owner: Architecture team

### **LOW PRIORITY**:

5. **Add Validation Tests** (2-3 hours)
   - File: Test suite
   - Action: Validate all required fields are present
   - Owner: QA team

---

## 📈 **ESTIMATED EFFORT**

**Total**: 10-15 hours

**Breakdown**:
- Gateway schema updates: 2-3 hours
- RemediationProcessing schema updates: 3-4 hours
- Documentation: 3-4 hours
- Testing: 2-3 hours

---

## 🔗 **RELATED DOCUMENTS**

**CRD Schemas**:
- `docs/architecture/CRD_SCHEMAS.md` - RemediationRequest schema (needs update)
- `docs/services/crd-controllers/01-remediationprocessor/crd-schema.md` - RemediationProcessing schema (needs update)

**Service Specifications**:
- `docs/services/stateless/gateway-service/crd-integration.md` - Gateway CRD creation (needs update)
- `docs/services/crd-controllers/01-remediationprocessor/overview.md` - RemediationProcessor responsibilities
- `docs/services/crd-controllers/05-remediationorchestrator/integration-points.md` - Orchestrator mapping (needs update)

---

## 🎯 **FINAL RECOMMENDATION**

**Status**: ⚠️ **SCHEMA ENHANCEMENTS NEEDED**

**Action**: Implement self-contained RemediationProcessing spec with all required Gateway data

**Priority**: **P1 - HIGH** (Not critical, but important for full functionality)

**Confidence**: **100%** - Self-contained CRD pattern is correct, schema needs enhancement

**Next Steps**:
1. Add `alertLabels` and `alertAnnotations` to RemediationRequest
2. Update RemediationProcessing spec with all recommended fields
3. Document complete field mapping logic
4. Create data flow diagram

---

**Triage Complete**: October 8, 2025


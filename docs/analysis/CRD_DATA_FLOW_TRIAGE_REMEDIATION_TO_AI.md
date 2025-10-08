# CRD Data Flow Triage: RemediationProcessor → AIAnalysis

**Date**: October 8, 2025  
**Purpose**: Triage RemediationProcessing CRD status to ensure it provides all data AIAnalysis needs  
**Scope**: RemediationOrchestrator creates AIAnalysis with data snapshot from RemediationProcessing.status  
**Architecture Pattern**: **Self-Contained CRDs** (no cross-CRD reads during reconciliation)

---

## Executive Summary

**Status**: 🔴 **CRITICAL DATA FLOW ISSUE DETECTED**

**Problem**: RemediationOrchestrator creates AIAnalysis CRD with a data snapshot from RemediationProcessing.status, but the current `RemediationProcessingStatus` schema **does NOT include most of the fields AIAnalysis requires**.

**Impact**:
- **AIAnalysis cannot operate** without complete enriched data
- **HolmesGPT investigations will fail** due to missing context
- **Remediation flow is blocked** at AI analysis phase

**Root Cause**: RemediationProcessing.status schema is incomplete - it doesn't expose the enriched alert data that AIAnalysis needs.

---

## 🔍 Data Flow Pattern

```
Gateway Service
    ↓ (creates RemediationRequest CRD)
RemediationOrchestrator
    ↓ (creates RemediationProcessing CRD with data from RemediationRequest)
RemediationProcessor Controller
    ↓ (enriches alert, updates RemediationProcessing.status)
RemediationOrchestrator (watches RemediationProcessing.status.phase == "completed")
    ↓ (SNAPSHOT: copies data from RemediationProcessing.status to AIAnalysis.spec)
AIAnalysis CRD (self-contained)
    ↓
AIAnalysis Controller (operates on AIAnalysis.spec - NO cross-CRD reads)
```

**Key Pattern**: AIAnalysis.spec is a **data snapshot** from RemediationProcessing.status at creation time.

---

## 📋 AIAnalysis Data Requirements

### What AIAnalysis Needs (from `docs/services/crd-controllers/02-aianalysis/crd-schema.md`)

AIAnalysis.spec expects:

```yaml
spec:
  analysisRequest:
    alertContext:
      # Basic identifiers
      fingerprint: "abc123def456"
      severity: critical
      environment: production
      businessPriority: p0
      
      # COMPLETE enriched payload (the main data requirement)
      enrichedPayload:
        originalAlert:
          labels: {...}
          annotations: {...}
        
        kubernetesContext:
          podDetails: {...}
          deploymentDetails: {...}
          nodeDetails: {...}
        
        monitoringContext:
          relatedAlerts: [...]
          metrics: [...]
          logs: [...]
        
        businessContext:
          serviceOwner: "..."
          criticality: "..."
          sla: "..."
```

**Key Requirement**: `enrichedPayload` must contain:
1. Original alert (labels, annotations)
2. Kubernetes context (pods, deployments, nodes, services, ingresses, configmaps)
3. Monitoring context (related alerts, metrics, logs)
4. Business context (ownership, SLA, criticality)

---

## 📊 Current RemediationProcessing.status Schema

From `docs/services/crd-controllers/01-remediationprocessor/crd-schema.md`:

```go
type RemediationProcessingStatus struct {
    // Phase tracking
    Phase string `json:"phase"` // "enriching", "classifying", "routing", "completed"
    
    // EnrichmentResults contains context data gathered
    EnrichmentResults EnrichmentResults `json:"enrichmentResults,omitempty"`
    
    // EnvironmentClassification result with confidence
    EnvironmentClassification EnvironmentClassification `json:"environmentClassification,omitempty"`
    
    // RoutingDecision for next service
    RoutingDecision RoutingDecision `json:"routingDecision,omitempty"`
    
    // ProcessingTime duration for metrics
    ProcessingTime string `json:"processingTime,omitempty"`
    
    // Conditions for status tracking
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type EnrichmentResults struct {
    KubernetesContext *KubernetesContext `json:"kubernetesContext,omitempty"`
    HistoricalContext *HistoricalContext `json:"historicalContext,omitempty"`
    EnrichmentQuality float64            `json:"enrichmentQuality,omitempty"` // 0.0-1.0
}
```

**Observation**: `EnrichmentResults` has `KubernetesContext` and `HistoricalContext`, which is **partially correct**, but:
1. ❌ **Missing**: Original alert (labels, annotations)
2. ❌ **Missing**: Monitoring context (related alerts, metrics, logs)
3. ❌ **Missing**: Business context (ownership, SLA, criticality)
4. ⚠️ **Incomplete**: `KubernetesContext` exists but may not match AIAnalysis expectations

---

## 🔬 Detailed Field-by-Field Analysis

### AIAnalysis Requirements vs RemediationProcessing.status

| AIAnalysis Field | Priority | Available in RemediationProcessing.status? | Gap Severity |
|---|---|---|---|
| **alertContext.fingerprint** | HIGH | ❌ NOT in status | 🔴 CRITICAL |
| **alertContext.severity** | HIGH | ❌ NOT in status | 🔴 CRITICAL |
| **alertContext.environment** | HIGH | ✅ YES (`status.environmentClassification.environment`) | ✅ OK |
| **alertContext.businessPriority** | HIGH | ✅ YES (`status.environmentClassification.businessPriority`) | ✅ OK |
| **enrichedPayload.originalAlert.labels** | CRITICAL | ❌ NOT in status | 🔴 CRITICAL |
| **enrichedPayload.originalAlert.annotations** | CRITICAL | ❌ NOT in status | 🔴 CRITICAL |
| **enrichedPayload.kubernetesContext** | CRITICAL | ✅ PARTIAL (`status.enrichmentResults.kubernetesContext`) | ⚠️ VERIFY |
| **enrichedPayload.monitoringContext** | MEDIUM | ❌ NOT in status | 🟠 HIGH |
| **enrichedPayload.businessContext** | MEDIUM | ❌ NOT in status | 🟠 HIGH |

---

## 🚨 CRITICAL GAPS IDENTIFIED

### Gap 1: Missing Core Signal Identifiers in Status (P0 - CRITICAL)

**Problem**: AIAnalysis needs `fingerprint` and `severity`, but they're **not in RemediationProcessing.status**.

**Current State**:
- `spec.alert.fingerprint` exists (input data)
- `spec.alert.severity` exists (input data)
- ❌ `status` **does NOT re-export these fields**

**Why This is Critical**:
- RemediationOrchestrator copies from `status` (not `spec`)
- AIAnalysis would receive `null` for fingerprint/severity
- HolmesGPT investigation **requires** fingerprint for correlation

**Solution Required**:
```go
type RemediationProcessingStatus struct {
    // ... existing fields ...
    
    // ✅ ADD: Signal identification (re-export from spec for snapshot pattern)
    SignalFingerprint string `json:"signalFingerprint"`
    SignalName        string `json:"signalName"`
    Severity          string `json:"severity"`
}
```

---

### Gap 2: Missing Original Alert Payload in Status (P0 - CRITICAL)

**Problem**: AIAnalysis needs `originalAlert.labels` and `originalAlert.annotations`, but they're **not in RemediationProcessing.status**.

**Current State**:
- `spec.alert.labels` exists (input data)
- `spec.alert.annotations` exists (input data)
- ❌ `status.enrichmentResults` **does NOT include original alert**

**Why This is Critical**:
- HolmesGPT needs original alert labels for context
- Alert annotations contain human-readable descriptions
- AIAnalysis **cannot function** without original alert data

**Solution Required**:
```go
type EnrichmentResults struct {
    // ✅ ADD: Original alert payload
    OriginalAlert *OriginalAlert `json:"originalAlert"`
    
    // ... existing fields ...
    KubernetesContext *KubernetesContext `json:"kubernetesContext,omitempty"`
    HistoricalContext *HistoricalContext `json:"historicalContext,omitempty"`
    EnrichmentQuality float64            `json:"enrichmentQuality,omitempty"`
}

// ✅ ADD: New type
type OriginalAlert struct {
    Labels      map[string]string `json:"labels"`
    Annotations map[string]string `json:"annotations"`
    FiringTime  metav1.Time       `json:"firingTime,omitempty"`
}
```

---

### Gap 3: Missing Monitoring Context in Status (P1 - HIGH)

**Problem**: AIAnalysis expects `monitoringContext` (related alerts, metrics, logs), but RemediationProcessing.status **does not provide this**.

**Current State**:
- RemediationProcessor enriches with Kubernetes context only
- ❌ No monitoring data enrichment in V1 scope

**Why This is High Priority**:
- HolmesGPT can use related alerts for correlation
- Metrics/logs provide additional investigation context
- AIAnalysis expects this field (even if empty)

**Solution Options**:

**Option A: Add to RemediationProcessing.status (Recommended)**
```go
type EnrichmentResults struct {
    OriginalAlert     *OriginalAlert     `json:"originalAlert"`
    KubernetesContext *KubernetesContext `json:"kubernetesContext,omitempty"`
    HistoricalContext *HistoricalContext `json:"historicalContext,omitempty"`
    
    // ✅ ADD: Monitoring context
    MonitoringContext *MonitoringContext `json:"monitoringContext,omitempty"`
    
    EnrichmentQuality float64            `json:"enrichmentQuality,omitempty"`
}

// ✅ ADD: New type
type MonitoringContext struct {
    RelatedAlerts []RelatedAlert `json:"relatedAlerts,omitempty"`
    Metrics       []MetricSample `json:"metrics,omitempty"`
    Logs          []LogEntry     `json:"logs,omitempty"`
}
```

**Option B: Defer to V2 (if HolmesGPT can fetch dynamically)**
- If HolmesGPT toolsets can fetch metrics/logs via Context API, this can be empty in V1
- AIAnalysis would pass empty `monitoringContext` to HolmesGPT

---

### Gap 4: Missing Business Context in Status (P1 - HIGH)

**Problem**: AIAnalysis expects `businessContext` (service ownership, SLA, criticality), but RemediationProcessing.status **does not provide this**.

**Current State**:
- `status.environmentClassification.businessPriority` exists (P0/P1/P2)
- ❌ No service ownership or SLA data

**Why This is High Priority**:
- AIAnalysis approval policies use business context
- Criticality affects auto-approve thresholds
- SLA requirements influence urgency

**Solution Required**:
```go
type EnrichmentResults struct {
    // ... other fields ...
    
    // ✅ ADD: Business context
    BusinessContext *BusinessContext `json:"businessContext,omitempty"`
}

// ✅ ADD: New type
type BusinessContext struct {
    ServiceOwner string `json:"serviceOwner,omitempty"` // From labels/annotations
    Criticality  string `json:"criticality,omitempty"`  // "high", "medium", "low"
    SLA          string `json:"sla,omitempty"`          // e.g., "99.9%"
    CostCenter   string `json:"costCenter,omitempty"`   // From labels
}
```

---

## 📈 KubernetesContext Compatibility Check

### AIAnalysis Expectation (from crd-schema.md)

```yaml
kubernetesContext:
  podDetails:
    name: web-app-789
    namespace: production
    containers:
    - name: app
      memoryLimit: "512Mi"
      memoryUsage: "498Mi"
  deploymentDetails:
    name: web-app
    replicas: 3
  nodeDetails:
    name: node-1
    capacity: {...}
```

### RemediationProcessing.status Provides (from crd-schema.md)

```go
type KubernetesContext struct {
    Namespace       string            `json:"namespace"`
    NamespaceLabels map[string]string `json:"namespaceLabels,omitempty"`
    
    PodDetails        *PodDetails        `json:"podDetails,omitempty"`
    DeploymentDetails *DeploymentDetails `json:"deploymentDetails,omitempty"`
    NodeDetails       *NodeDetails       `json:"nodeDetails,omitempty"`
    
    RelatedServices   []ServiceSummary   `json:"relatedServices,omitempty"`
    RelatedIngresses  []IngressSummary   `json:"relatedIngresses,omitempty"`
    RelatedConfigMaps []ConfigMapSummary `json:"relatedConfigMaps,omitempty"`
}
```

**Compatibility Assessment**: ✅ **MOSTLY COMPATIBLE**

RemediationProcessing provides:
- ✅ `podDetails` (matches AIAnalysis expectation)
- ✅ `deploymentDetails` (matches AIAnalysis expectation)
- ✅ `nodeDetails` (matches AIAnalysis expectation)
- ✅ **BONUS**: Related services, ingresses, configmaps (richer than AIAnalysis expects)

**Minor Gap**: RemediationProcessing.PodDetails has:
- ✅ `name`, `phase`, `labels`, `annotations`, `containers`, `restartCount`
- ❌ **Missing**: `memoryUsage` (AIAnalysis example shows this)

**Assessment**: This is a **minor documentation inconsistency**, not a critical gap. If `memoryUsage` is needed, it should be added to `ContainerStatus`.

---

## 🔧 Recommended Schema Updates

### P0 - CRITICAL: Add Signal Identifiers to Status

**File**: `docs/services/crd-controllers/01-remediationprocessor/crd-schema.md`

```go
type RemediationProcessingStatus struct {
    // Phase tracking
    Phase string `json:"phase"`
    
    // ✅ ADD: Signal identification (re-exported from spec for snapshot pattern)
    // These fields are copied from spec to status to enable data snapshot pattern
    // (RemediationOrchestrator copies from status, not spec)
    SignalFingerprint string `json:"signalFingerprint"`
    SignalName        string `json:"signalName"`
    Severity          string `json:"severity"`
    
    // EnrichmentResults contains context data gathered
    EnrichmentResults EnrichmentResults `json:"enrichmentResults,omitempty"`
    
    // ... rest of fields unchanged ...
}
```

---

### P0 - CRITICAL: Add OriginalAlert to EnrichmentResults

**File**: `docs/services/crd-controllers/01-remediationprocessor/crd-schema.md`

```go
type EnrichmentResults struct {
    // ✅ ADD: Original alert payload (re-exported from spec)
    // Required by AIAnalysis for HolmesGPT investigation
    OriginalAlert *OriginalAlert `json:"originalAlert"`
    
    // Existing fields
    KubernetesContext *KubernetesContext `json:"kubernetesContext,omitempty"`
    HistoricalContext *HistoricalContext `json:"historicalContext,omitempty"`
    EnrichmentQuality float64            `json:"enrichmentQuality,omitempty"`
}

// ✅ ADD: New type
// OriginalAlert contains the original signal data from the provider
type OriginalAlert struct {
    Labels      map[string]string `json:"labels"`
    Annotations map[string]string `json:"annotations"`
    FiringTime  metav1.Time       `json:"firingTime,omitempty"`
    ReceivedTime metav1.Time      `json:"receivedTime,omitempty"`
}
```

---

### P1 - HIGH: Add MonitoringContext to EnrichmentResults

**File**: `docs/services/crd-controllers/01-remediationprocessor/crd-schema.md`

```go
type EnrichmentResults struct {
    OriginalAlert     *OriginalAlert     `json:"originalAlert"`
    KubernetesContext *KubernetesContext `json:"kubernetesContext,omitempty"`
    HistoricalContext *HistoricalContext `json:"historicalContext,omitempty"`
    
    // ✅ ADD: Monitoring context (optional in V1, required in V2)
    MonitoringContext *MonitoringContext `json:"monitoringContext,omitempty"`
    
    EnrichmentQuality float64            `json:"enrichmentQuality,omitempty"`
}

// ✅ ADD: New type
// MonitoringContext contains related monitoring data for correlation
// V1: May be empty if HolmesGPT fetches dynamically via Context API
// V2: Should be populated by RemediationProcessor
type MonitoringContext struct {
    // Related alerts for correlation
    RelatedAlerts []RelatedAlert `json:"relatedAlerts,omitempty"`
    
    // Metric samples for analysis
    Metrics []MetricSample `json:"metrics,omitempty"`
    
    // Recent log entries for investigation
    Logs []LogEntry `json:"logs,omitempty"`
}

// ✅ ADD: Supporting types
type RelatedAlert struct {
    Fingerprint string            `json:"fingerprint"`
    Name        string            `json:"name"`
    Severity    string            `json:"severity"`
    FiringTime  metav1.Time       `json:"firingTime"`
    Labels      map[string]string `json:"labels,omitempty"`
}

type MetricSample struct {
    MetricName string      `json:"metricName"`
    Value      float64     `json:"value"`
    Timestamp  metav1.Time `json:"timestamp"`
    Labels     map[string]string `json:"labels,omitempty"`
}

type LogEntry struct {
    Timestamp metav1.Time `json:"timestamp"`
    Level     string      `json:"level"` // "error", "warn", "info"
    Message   string      `json:"message"`
    Source    string      `json:"source"` // pod/container name
}
```

---

### P1 - HIGH: Add BusinessContext to EnrichmentResults

**File**: `docs/services/crd-controllers/01-remediationprocessor/crd-schema.md`

```go
type EnrichmentResults struct {
    OriginalAlert     *OriginalAlert     `json:"originalAlert"`
    KubernetesContext *KubernetesContext `json:"kubernetesContext,omitempty"`
    HistoricalContext *HistoricalContext `json:"historicalContext,omitempty"`
    MonitoringContext *MonitoringContext `json:"monitoringContext,omitempty"`
    
    // ✅ ADD: Business context (extracted from namespace labels/annotations)
    BusinessContext *BusinessContext `json:"businessContext,omitempty"`
    
    EnrichmentQuality float64            `json:"enrichmentQuality,omitempty"`
}

// ✅ ADD: New type
// BusinessContext contains business metadata for policy decisions
// Extracted from namespace labels, annotations, or ConfigMaps
type BusinessContext struct {
    // Service ownership information
    ServiceOwner string `json:"serviceOwner,omitempty"` // From label "owner" or "team"
    
    // Service criticality level
    Criticality string `json:"criticality,omitempty"` // "high", "medium", "low"
    
    // SLA requirement
    SLA string `json:"sla,omitempty"` // e.g., "99.9%", "99.95%"
    
    // Cost center for billing
    CostCenter string `json:"costCenter,omitempty"` // From label "cost-center"
    
    // Additional business metadata
    ProjectName string `json:"projectName,omitempty"`
    ContactInfo string `json:"contactInfo,omitempty"` // Slack channel, email, etc.
}
```

---

## 📝 RemediationOrchestrator Mapping Code

### How RemediationOrchestrator Creates AIAnalysis

**File**: `docs/services/crd-controllers/05-remediationorchestrator/integration-points.md`

```go
// When RemediationProcessing.status.phase == "completed"
func (r *RemediationOrchestratorReconciler) createAIAnalysis(
    ctx context.Context,
    remReq *remediationv1.RemediationRequest,
    remProc *remediationprocessingv1.RemediationProcessing,
) error {
    
    // Validate RemediationProcessing status has required data
    if remProc.Status.SignalFingerprint == "" {
        return fmt.Errorf("RemediationProcessing.status.signalFingerprint is required")
    }
    if remProc.Status.EnrichmentResults.OriginalAlert == nil {
        return fmt.Errorf("RemediationProcessing.status.enrichmentResults.originalAlert is required")
    }
    
    aiAnalysis := &aianalysisv1.AIAnalysis{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-analysis", remReq.Name),
            Namespace: remReq.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(remReq, remediationv1.GroupVersion.WithKind("RemediationRequest")),
            },
        },
        Spec: aianalysisv1.AIAnalysisSpec{
            RemediationRequestRef: aianalysisv1.RemediationRequestReference{
                Name:      remReq.Name,
                Namespace: remReq.Namespace,
            },
            
            AnalysisRequest: aianalysisv1.AnalysisRequest{
                AlertContext: aianalysisv1.AlertContext{
                    // ✅ FROM: remProc.Status.SignalFingerprint
                    Fingerprint: remProc.Status.SignalFingerprint,
                    
                    // ✅ FROM: remProc.Status.Severity
                    Severity: remProc.Status.Severity,
                    
                    // ✅ FROM: remProc.Status.EnvironmentClassification
                    Environment:      remProc.Status.EnvironmentClassification.Environment,
                    BusinessPriority: remProc.Status.EnvironmentClassification.BusinessPriority,
                    
                    // ✅ FROM: remProc.Status.EnrichmentResults
                    EnrichedPayload: aianalysisv1.EnrichedPayload{
                        // ✅ FROM: remProc.Status.EnrichmentResults.OriginalAlert
                        OriginalAlert: aianalysisv1.OriginalAlertPayload{
                            Labels:      remProc.Status.EnrichmentResults.OriginalAlert.Labels,
                            Annotations: remProc.Status.EnrichmentResults.OriginalAlert.Annotations,
                        },
                        
                        // ✅ FROM: remProc.Status.EnrichmentResults.KubernetesContext
                        KubernetesContext: convertKubernetesContext(
                            remProc.Status.EnrichmentResults.KubernetesContext,
                        ),
                        
                        // ✅ FROM: remProc.Status.EnrichmentResults.MonitoringContext
                        MonitoringContext: convertMonitoringContext(
                            remProc.Status.EnrichmentResults.MonitoringContext,
                        ),
                        
                        // ✅ FROM: remProc.Status.EnrichmentResults.BusinessContext
                        BusinessContext: convertBusinessContext(
                            remProc.Status.EnrichmentResults.BusinessContext,
                        ),
                    },
                },
                
                AnalysisTypes: []string{"investigation", "root-cause", "recovery-analysis"},
                
                InvestigationScope: aianalysisv1.InvestigationScope{
                    TimeWindow: "24h",
                    ResourceScope: []aianalysisv1.ResourceScopeItem{
                        {
                            Kind:      remProc.Spec.TargetResource.Kind,
                            Namespace: remProc.Spec.TargetResource.Namespace,
                            Name:      remProc.Spec.TargetResource.Name,
                        },
                    },
                    CorrelationDepth:          "detailed",
                    IncludeHistoricalPatterns: true,
                },
            },
        },
    }
    
    return r.Create(ctx, aiAnalysis)
}
```

---

## ✅ Validation Checklist

### Schema Update Checklist

- [ ] **P0-1**: Add `signalFingerprint` to `RemediationProcessingStatus`
- [ ] **P0-2**: Add `signalName` to `RemediationProcessingStatus`
- [ ] **P0-3**: Add `severity` to `RemediationProcessingStatus`
- [ ] **P0-4**: Add `OriginalAlert` type definition
- [ ] **P0-5**: Add `originalAlert` field to `EnrichmentResults`
- [ ] **P1-1**: Add `MonitoringContext` type definition
- [ ] **P1-2**: Add `RelatedAlert`, `MetricSample`, `LogEntry` types
- [ ] **P1-3**: Add `monitoringContext` field to `EnrichmentResults`
- [ ] **P1-4**: Add `BusinessContext` type definition
- [ ] **P1-5**: Add `businessContext` field to `EnrichmentResults`

### RemediationProcessor Controller Update Checklist

- [ ] **P0-6**: Update controller to copy `signalFingerprint` from spec to status
- [ ] **P0-7**: Update controller to copy `signalName` from spec to status
- [ ] **P0-8**: Update controller to copy `severity` from spec to status
- [ ] **P0-9**: Update controller to copy `labels` and `annotations` to `status.enrichmentResults.originalAlert`
- [ ] **P1-6**: Implement monitoring context enrichment (optional in V1)
- [ ] **P1-7**: Implement business context extraction from namespace labels

### RemediationOrchestrator Update Checklist

- [ ] **P0-10**: Update `createAIAnalysis()` to map from `remProc.Status.SignalFingerprint`
- [ ] **P0-11**: Update `createAIAnalysis()` to map from `remProc.Status.EnrichmentResults.OriginalAlert`
- [ ] **P1-8**: Update `createAIAnalysis()` to map from `remProc.Status.EnrichmentResults.MonitoringContext`
- [ ] **P1-9**: Update `createAIAnalysis()` to map from `remProc.Status.EnrichmentResults.BusinessContext`
- [ ] **P0-12**: Add validation checks for required fields before creating AIAnalysis

### AIAnalysis CRD Update Checklist

- [ ] **P2-1**: Verify `AIAnalysis.spec.analysisRequest.alertContext` schema matches expectations
- [ ] **P2-2**: Verify `EnrichedPayload` schema is compatible with RemediationProcessing types

---

## 🎯 Summary

### Critical Issues (P0 - Blocking)

1. ❌ **RemediationProcessing.status missing signal identifiers** (`signalFingerprint`, `signalName`, `severity`)
   - **Impact**: AIAnalysis cannot identify the signal
   - **Fix**: Add 3 fields to `RemediationProcessingStatus`

2. ❌ **RemediationProcessing.status missing original alert payload**
   - **Impact**: HolmesGPT cannot investigate without alert labels/annotations
   - **Fix**: Add `OriginalAlert` type and field to `EnrichmentResults`

### High Priority Issues (P1 - Recommended for V1)

3. ⚠️ **RemediationProcessing.status missing monitoring context**
   - **Impact**: AIAnalysis cannot correlate with related alerts/metrics
   - **Fix**: Add `MonitoringContext` type and field to `EnrichmentResults`

4. ⚠️ **RemediationProcessing.status missing business context**
   - **Impact**: AIAnalysis approval policies lack business metadata
   - **Fix**: Add `BusinessContext` type and field to `EnrichmentResults`

### Compatibility Check

5. ✅ **KubernetesContext schema is mostly compatible**
   - RemediationProcessing provides richer context than AIAnalysis expects
   - Minor gap: `memoryUsage` field not documented in RemediationProcessing

---

## 📅 Execution Plan

### Phase 1: P0 Fixes (Estimated: 2 hours)

1. Update `docs/services/crd-controllers/01-remediationprocessor/crd-schema.md`
   - Add `signalFingerprint`, `signalName`, `severity` to `RemediationProcessingStatus`
   - Add `OriginalAlert` type definition
   - Add `originalAlert` field to `EnrichmentResults`

2. Update `docs/architecture/CRD_SCHEMAS.md` (if applicable)
   - Ensure consistency with `01-remediationprocessor/crd-schema.md`

3. Update `docs/services/crd-controllers/05-remediationorchestrator/integration-points.md`
   - Add validation checks for required fields
   - Update `createAIAnalysis()` mapping code

### Phase 2: P1 Additions (Estimated: 3 hours)

1. Add `MonitoringContext`, `BusinessContext` types to RemediationProcessing CRD
2. Update RemediationOrchestrator mapping code to handle new fields
3. Document implementation requirements for RemediationProcessor controller

### Phase 3: Validation (Estimated: 1 hour)

1. Verify all checklists complete
2. Cross-reference with AIAnalysis service specs
3. Create unit tests for RemediationOrchestrator mapping logic

---

## 🔗 Related Documents

- [docs/services/crd-controllers/01-remediationprocessor/crd-schema.md](mdc:docs/services/crd-controllers/01-remediationprocessor/crd-schema.md)
- [docs/services/crd-controllers/02-aianalysis/crd-schema.md](mdc:docs/services/crd-controllers/02-aianalysis/crd-schema.md)
- [docs/services/crd-controllers/05-remediationorchestrator/integration-points.md](mdc:docs/services/crd-controllers/05-remediationorchestrator/integration-points.md)
- [docs/architecture/CRD_SCHEMAS.md](mdc:docs/architecture/CRD_SCHEMAS.md)
- [docs/analysis/CRD_DATA_FLOW_TRIAGE_REVISED.md](mdc:docs/analysis/CRD_DATA_FLOW_TRIAGE_REVISED.md) (Gateway → RemediationProcessor)

---

**Confidence Assessment**: 95%

**Justification**: This triage is based on authoritative service specifications and CRD schemas. The gaps are objectively identified by comparing AIAnalysis requirements with RemediationProcessing status schema. The recommendations follow the established self-contained CRD pattern. Risk: AIAnalysis schema may have additional undocumented requirements discovered during implementation.


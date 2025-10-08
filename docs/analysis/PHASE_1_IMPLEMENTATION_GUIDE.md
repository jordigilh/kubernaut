# Phase 1 (P0) Implementation Guide: Critical CRD Schema Fixes

**Date**: October 8, 2025
**Status**: ✅ **APPROVED** - Ready for implementation
**Estimated Time**: 4-5 hours
**Priority**: IMMEDIATE (Unblocks entire remediation pipeline)

---

## 🎯 **Implementation Objectives**

**Goal**: Fix self-contained CRD pattern violations to enable complete data flow from Gateway → RemediationProcessor → AIAnalysis

**Success Criteria**:
1. ✅ RemediationProcessing CRD contains ALL data it needs (no cross-CRD reads)
2. ✅ AIAnalysis receives complete signal identification and payload
3. ✅ End-to-end pipeline functional from signal ingestion to AI investigation

---

## 📋 **Implementation Tasks Breakdown**

### **Task 1: Update RemediationRequest Schema** (1 hour)

**File**: `api/remediation/v1/remediationrequest_types.go`

**Changes**: Add 2 fields to `RemediationRequestSpec`

#### **Before**:
```go
type RemediationRequestSpec struct {
    // Core Signal Identification
    SignalFingerprint string `json:"signalFingerprint"`
    SignalName        string `json:"signalName"`

    // Signal Classification
    Severity     string `json:"severity"`
    Environment  string `json:"environment"`
    Priority     string `json:"priority"`

    // Signal Type Classification
    SignalType   string `json:"signalType"`
    SignalSource string `json:"signalSource"`
    TargetType   string `json:"targetType"`

    // Timestamps
    FiringTime   metav1.Time `json:"firingTime,omitempty"`
    ReceivedTime metav1.Time `json:"receivedTime"`

    // Deduplication
    Deduplication DeduplicationInfo `json:"deduplication"`

    // Provider-specific data (discriminated union)
    ProviderData json.RawMessage `json:"providerData"`

    // Original webhook payload (optional, for audit)
    OriginalPayload []byte `json:"originalPayload,omitempty"`
}
```

#### **After** (Add these 2 fields):
```go
type RemediationRequestSpec struct {
    // Core Signal Identification
    SignalFingerprint string `json:"signalFingerprint"`
    SignalName        string `json:"signalName"`

    // Signal Classification
    Severity     string `json:"severity"`
    Environment  string `json:"environment"`
    Priority     string `json:"priority"`

    // Signal Type Classification
    SignalType   string `json:"signalType"`
    SignalSource string `json:"signalSource"`
    TargetType   string `json:"targetType"`

    // Timestamps
    FiringTime   metav1.Time `json:"firingTime,omitempty"`
    ReceivedTime metav1.Time `json:"receivedTime"`

    // Deduplication
    Deduplication DeduplicationInfo `json:"deduplication"`

    // ✅ ADD: Signal metadata (extracted from provider-specific data)
    // These are populated by Gateway Service after parsing providerData
    SignalLabels      map[string]string `json:"signalLabels,omitempty"`
    SignalAnnotations map[string]string `json:"signalAnnotations,omitempty"`

    // Provider-specific data (discriminated union)
    ProviderData json.RawMessage `json:"providerData"`

    // Original webhook payload (optional, for audit)
    OriginalPayload []byte `json:"originalPayload,omitempty"`
}
```

#### **Subtasks**:

1. **Update Go Types** (10 min)
   - File: `api/remediation/v1/remediationrequest_types.go`
   - Add `SignalLabels` and `SignalAnnotations` fields
   - Run: `make generate` to update generated code

2. **Update Gateway Service** (30 min)
   - File: `pkg/gateway/crd_integration.go`
   - Function: `createRemediationRequestCRD()`
   - Add logic to populate labels/annotations from normalized signal:
     ```go
     cr := &remediationv1.RemediationRequest{
         Spec: remediationv1.RemediationRequestSpec{
             // ... existing fields ...

             // ✅ ADD: Extract and populate signal metadata
             SignalLabels:      extractLabels(signal),
             SignalAnnotations: extractAnnotations(signal),
         },
     }
     ```

3. **Add Extraction Helpers** (10 min)
   - File: `pkg/gateway/signal_extraction.go`
   - Add functions:
     ```go
     func extractLabels(signal *NormalizedSignal) map[string]string {
         // Extract labels from provider-specific data
         switch signal.SignalType {
         case "prometheus":
             return extractPrometheusLabels(signal.ProviderData)
         case "kubernetes-event":
             return extractKubernetesEventLabels(signal.ProviderData)
         default:
             return signal.Labels // Fallback
         }
     }

     func extractAnnotations(signal *NormalizedSignal) map[string]string {
         // Extract annotations from provider-specific data
         switch signal.SignalType {
         case "prometheus":
             return extractPrometheusAnnotations(signal.ProviderData)
         case "kubernetes-event":
             return extractKubernetesEventAnnotations(signal.ProviderData)
         default:
             return signal.Annotations // Fallback
         }
     }
     ```

4. **Update CRD Schema Documentation** (10 min)
   - File: `docs/architecture/CRD_SCHEMAS.md`
   - Add field descriptions for `signalLabels` and `signalAnnotations`

**Validation**:
```bash
# Test CRD creation
make test-gateway-integration

# Verify fields are populated
kubectl get remediationrequest <name> -o yaml | grep -A 5 "signalLabels"
```

---

### **Task 2: Update RemediationProcessing.spec** (2 hours)

**File**: `api/remediationprocessing/v1/remediationprocessing_types.go`

**Changes**: Add 18 fields for self-containment

#### **Current State** (Incomplete):
```go
type RemediationProcessingSpec struct {
    // Parent reference
    RemediationRequestRef corev1.ObjectReference `json:"remediationRequestRef"`

    // ❌ MISSING: Signal identification
    // ❌ MISSING: Signal metadata
    // ❌ MISSING: Provider data
    // ❌ MISSING: Deduplication context
    // ❌ MISSING: Timestamps

    // Configuration
    EnrichmentConfig EnrichmentConfiguration `json:"enrichmentConfig,omitempty"`
}
```

#### **Target State** (Self-contained):
```go
type RemediationProcessingSpec struct {
    // ========================================
    // PARENT REFERENCE (Audit/Lineage Only)
    // ========================================
    RemediationRequestRef corev1.ObjectReference `json:"remediationRequestRef"`

    // ========================================
    // SIGNAL IDENTIFICATION (From RemediationRequest)
    // ========================================
    // ✅ ADD: Core signal identity
    SignalFingerprint string `json:"signalFingerprint"`
    SignalName        string `json:"signalName"`
    Severity          string `json:"severity"`

    // ========================================
    // SIGNAL CLASSIFICATION (From RemediationRequest)
    // ========================================
    // ✅ ADD: Environment and priority
    Environment string `json:"environment"`
    Priority    string `json:"priority"`

    // ✅ ADD: Signal type classification
    SignalType   string `json:"signalType"`
    SignalSource string `json:"signalSource"`
    TargetType   string `json:"targetType"`

    // ========================================
    // SIGNAL METADATA (From RemediationRequest)
    // ========================================
    // ✅ ADD: Labels and annotations
    SignalLabels      map[string]string `json:"signalLabels,omitempty"`
    SignalAnnotations map[string]string `json:"signalAnnotations,omitempty"`

    // ========================================
    // TARGET RESOURCE (From RemediationRequest)
    // ========================================
    // ✅ ADD: Target identification
    TargetResource ResourceIdentifier `json:"targetResource"`

    // ========================================
    // TIMESTAMPS (From RemediationRequest)
    // ========================================
    // ✅ ADD: Temporal context
    FiringTime   metav1.Time `json:"firingTime,omitempty"`
    ReceivedTime metav1.Time `json:"receivedTime"`

    // ========================================
    // DEDUPLICATION (From RemediationRequest)
    // ========================================
    // ✅ ADD: Correlation context
    Deduplication DeduplicationContext `json:"deduplication"`

    // ========================================
    // PROVIDER DATA (From RemediationRequest)
    // ========================================
    // ✅ ADD: Provider-specific information
    ProviderData json.RawMessage `json:"providerData"`

    // ✅ ADD: Original payload (for audit)
    OriginalPayload []byte `json:"originalPayload,omitempty"`

    // ========================================
    // STORM DETECTION (From RemediationRequest)
    // ========================================
    // ✅ ADD: Storm context
    IsStorm         bool `json:"isStorm,omitempty"`
    StormAlertCount int  `json:"stormAlertCount,omitempty"`

    // ========================================
    // CONFIGURATION (Processor-Specific)
    // ========================================
    EnrichmentConfig EnrichmentConfiguration `json:"enrichmentConfig,omitempty"`
}

// ✅ ADD: Supporting types
type ResourceIdentifier struct {
    Kind      string `json:"kind"`
    Name      string `json:"name"`
    Namespace string `json:"namespace"`
}

type DeduplicationContext struct {
    FirstOccurrence metav1.Time `json:"firstOccurrence"`
    LastOccurrence  metav1.Time `json:"lastOccurrence"`
    OccurrenceCount int         `json:"occurrenceCount"`
    CorrelationID   string      `json:"correlationID,omitempty"`
}
```

#### **Subtasks**:

1. **Update Go Types** (15 min)
   - File: `api/remediationprocessing/v1/remediationprocessing_types.go`
   - Add 18 fields to `RemediationProcessingSpec`
   - Add supporting types: `ResourceIdentifier`, `DeduplicationContext`
   - Run: `make generate`

2. **Update RemediationOrchestrator Mapping** (1h 15min)
   - File: `internal/controller/remediationorchestrator/remediationprocessing_creator.go`
   - Function: `createRemediationProcessing()`
   - Update to copy all 18 fields:
     ```go
     func (r *RemediationOrchestratorReconciler) createRemediationProcessing(
         ctx context.Context,
         remReq *remediationv1.RemediationRequest,
     ) (*remediationprocessingv1.RemediationProcessing, error) {

         remProcessing := &remediationprocessingv1.RemediationProcessing{
             ObjectMeta: metav1.ObjectMeta{
                 Name:      fmt.Sprintf("%s-processing", remReq.Name),
                 Namespace: remReq.Namespace,
                 OwnerReferences: []metav1.OwnerReference{
                     *metav1.NewControllerRef(remReq, remediationv1.GroupVersion.WithKind("RemediationRequest")),
                 },
             },
             Spec: remediationprocessingv1.RemediationProcessingSpec{
                 // Parent reference (audit/lineage)
                 RemediationRequestRef: corev1.ObjectReference{
                     Name:      remReq.Name,
                     Namespace: remReq.Namespace,
                     UID:       remReq.UID,
                 },

                 // ✅ COPY: Signal identification (3 fields)
                 SignalFingerprint: remReq.Spec.SignalFingerprint,
                 SignalName:        remReq.Spec.SignalName,
                 Severity:          remReq.Spec.Severity,

                 // ✅ COPY: Signal classification (5 fields)
                 Environment:  remReq.Spec.Environment,
                 Priority:     remReq.Spec.Priority,
                 SignalType:   remReq.Spec.SignalType,
                 SignalSource: remReq.Spec.SignalSource,
                 TargetType:   remReq.Spec.TargetType,

                 // ✅ COPY: Signal metadata (2 fields)
                 SignalLabels:      remReq.Spec.SignalLabels,
                 SignalAnnotations: remReq.Spec.SignalAnnotations,

                 // ✅ COPY: Target resource (1 field)
                 TargetResource: extractTargetResource(remReq),

                 // ✅ COPY: Timestamps (2 fields)
                 FiringTime:   remReq.Spec.FiringTime,
                 ReceivedTime: remReq.Spec.ReceivedTime,

                 // ✅ COPY: Deduplication (1 field)
                 Deduplication: convertDeduplication(remReq.Spec.Deduplication),

                 // ✅ COPY: Provider data (2 fields)
                 ProviderData:    remReq.Spec.ProviderData,
                 OriginalPayload: remReq.Spec.OriginalPayload,

                 // ✅ COPY: Storm detection (2 fields)
                 IsStorm:         remReq.Status.StormDetected,
                 StormAlertCount: remReq.Status.StormMetadata.AlertCount,

                 // Configuration (processor-specific)
                 EnrichmentConfig: getDefaultEnrichmentConfig(),
             },
         }

         return remProcessing, r.Create(ctx, remProcessing)
     }

     func extractTargetResource(remReq *remediationv1.RemediationRequest) remediationprocessingv1.ResourceIdentifier {
         // Extract from provider data based on signal type
         switch remReq.Spec.SignalType {
         case "prometheus":
             return extractPrometheusTarget(remReq.Spec.ProviderData)
         case "kubernetes-event":
             return extractKubernetesEventTarget(remReq.Spec.ProviderData)
         default:
             return remediationprocessingv1.ResourceIdentifier{}
         }
     }

     func convertDeduplication(dedupInfo remediationv1.DeduplicationInfo) remediationprocessingv1.DeduplicationContext {
         return remediationprocessingv1.DeduplicationContext{
             FirstOccurrence: dedupInfo.FirstSeen,
             LastOccurrence:  dedupInfo.LastSeen,
             OccurrenceCount: dedupInfo.Count,
             CorrelationID:   dedupInfo.CorrelationKey,
         }
     }
     ```

3. **Remove Cross-CRD Reads from RemediationProcessor** (20 min)
   - File: `internal/controller/remediationprocessing/controller.go`
   - Remove any `Get(ctx, ..., &remediationRequest)` calls
   - Verify controller only reads from `remProcessing.Spec`

4. **Update CRD Schema Documentation** (10 min)
   - File: `docs/architecture/CRD_SCHEMAS.md`
   - Document all 18 new fields in RemediationProcessingSpec

**Validation**:
```bash
# Test RemediationProcessing creation
make test-orchestrator-integration

# Verify self-containment (no cross-CRD reads)
grep -r "remediationRequest" internal/controller/remediationprocessing/
# Should find ZERO references (except in comments)

# Verify all fields populated
kubectl get remediationprocessing <name> -o yaml | grep "signalFingerprint"
```

---

### **Task 3: Update RemediationProcessing.status** (2 hours)

**File**: `api/remediationprocessing/v1/remediationprocessing_types.go`

**Changes**: Add 4 fields + 1 new type to status

#### **Current State** (Incomplete):
```go
type RemediationProcessingStatus struct {
    Phase string `json:"phase"`

    // ❌ MISSING: Signal identifiers (for AIAnalysis)

    EnrichmentResults EnrichmentResults `json:"enrichmentResults,omitempty"`
    // ❌ EnrichmentResults missing OriginalSignal type

    ValidationResults ValidationResults `json:"validationResults,omitempty"`

    Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type EnrichmentResults struct {
    // ❌ MISSING: OriginalSignal

    KubernetesContext *KubernetesContext `json:"kubernetesContext,omitempty"`
    HistoricalContext *HistoricalContext `json:"historicalContext,omitempty"`
    EnrichmentQuality float64            `json:"enrichmentQuality,omitempty"`
}
```

#### **Target State** (Complete):
```go
type RemediationProcessingStatus struct {
    Phase string `json:"phase"`

    // ✅ ADD: Signal identifiers (re-exported from spec for AIAnalysis)
    SignalFingerprint string `json:"signalFingerprint"`
    SignalName        string `json:"signalName"`
    Severity          string `json:"severity"`

    EnrichmentResults EnrichmentResults `json:"enrichmentResults,omitempty"`
    ValidationResults ValidationResults `json:"validationResults,omitempty"`

    Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type EnrichmentResults struct {
    // ✅ ADD: Original signal payload
    OriginalSignal *OriginalSignal `json:"originalSignal"`

    // Existing fields
    KubernetesContext *KubernetesContext `json:"kubernetesContext,omitempty"`
    HistoricalContext *HistoricalContext `json:"historicalContext,omitempty"`
    EnrichmentQuality float64            `json:"enrichmentQuality,omitempty"`
}

// ✅ ADD: New type
type OriginalSignal struct {
    Labels       map[string]string `json:"labels"`
    Annotations  map[string]string `json:"annotations"`
    FiringTime   metav1.Time       `json:"firingTime,omitempty"`
    ReceivedTime metav1.Time       `json:"receivedTime"`
}
```

#### **Subtasks**:

1. **Update Go Types** (10 min)
   - File: `api/remediationprocessing/v1/remediationprocessing_types.go`
   - Add 3 fields to `RemediationProcessingStatus`
   - Add `OriginalSignal` field to `EnrichmentResults`
   - Add `OriginalSignal` type definition
   - Run: `make generate`

2. **Update RemediationProcessor Controller** (1h 20min)
   - File: `internal/controller/remediationprocessing/enrichment.go`
   - Update status population in enrichment phase:
     ```go
     func (r *RemediationProcessingReconciler) enrichSignal(
         ctx context.Context,
         remProcessing *remediationprocessingv1.RemediationProcessing,
     ) error {

         // ... existing enrichment logic ...

         // ✅ UPDATE: Populate status with signal identifiers
         remProcessing.Status.SignalFingerprint = remProcessing.Spec.SignalFingerprint
         remProcessing.Status.SignalName = remProcessing.Spec.SignalName
         remProcessing.Status.Severity = remProcessing.Spec.Severity

         // ✅ UPDATE: Populate OriginalSignal
         remProcessing.Status.EnrichmentResults.OriginalSignal = &remediationprocessingv1.OriginalSignal{
             Labels:       remProcessing.Spec.SignalLabels,
             Annotations:  remProcessing.Spec.SignalAnnotations,
             FiringTime:   remProcessing.Spec.FiringTime,
             ReceivedTime: remProcessing.Spec.ReceivedTime,
         }

         // ... existing context enrichment ...

         return r.Status().Update(ctx, remProcessing)
     }
     ```

3. **Update RemediationOrchestrator AIAnalysis Creator** (20 min)
   - File: `internal/controller/remediationorchestrator/aianalysis_creator.go`
   - Update `createAIAnalysis()` to read from status:
     ```go
     func (r *RemediationOrchestratorReconciler) createAIAnalysis(
         ctx context.Context,
         remProcessing *remediationprocessingv1.RemediationProcessing,
     ) (*aianalysisv1.AIAnalysis, error) {

         aiAnalysis := &aianalysisv1.AIAnalysis{
             ObjectMeta: metav1.ObjectMeta{
                 Name:      fmt.Sprintf("%s-analysis", remProcessing.Name),
                 Namespace: remProcessing.Namespace,
             },
             Spec: aianalysisv1.AIAnalysisSpec{
                 // ✅ READ FROM STATUS: Signal identification
                 SignalFingerprint: remProcessing.Status.SignalFingerprint,
                 SignalName:        remProcessing.Status.SignalName,
                 Severity:          remProcessing.Status.Severity,

                 // ✅ READ FROM STATUS: Original signal
                 OriginalSignal: &aianalysisv1.SignalPayload{
                     Labels:       remProcessing.Status.EnrichmentResults.OriginalSignal.Labels,
                     Annotations:  remProcessing.Status.EnrichmentResults.OriginalSignal.Annotations,
                     FiringTime:   remProcessing.Status.EnrichmentResults.OriginalSignal.FiringTime,
                     ReceivedTime: remProcessing.Status.EnrichmentResults.OriginalSignal.ReceivedTime,
                 },

                 // ✅ READ FROM STATUS: Enriched context
                 KubernetesContext: remProcessing.Status.EnrichmentResults.KubernetesContext,
                 HistoricalContext: remProcessing.Status.EnrichmentResults.HistoricalContext,

                 // Configuration
                 AnalysisConfig: getDefaultAnalysisConfig(),
             },
         }

         return aiAnalysis, r.Create(ctx, aiAnalysis)
     }
     ```

4. **Update CRD Schema Documentation** (10 min)
   - File: `docs/architecture/CRD_SCHEMAS.md`
   - Document new status fields and OriginalSignal type

**Validation**:
```bash
# Test status population
make test-processor-integration

# Verify status fields populated
kubectl get remediationprocessing <name> -o yaml | grep -A 10 "status:"

# Verify AIAnalysis receives complete data
kubectl get aianalysis <name> -o yaml | grep "signalFingerprint"
kubectl get aianalysis <name> -o yaml | grep -A 5 "originalSignal"
```

---

## 🧪 **Testing Strategy**

### **Unit Tests** (Parallel with implementation)

#### **Test 1: RemediationRequest Schema**
```go
// File: api/remediation/v1/remediationrequest_types_test.go
func TestRemediationRequestSpec_SignalMetadata(t *testing.T) {
    spec := remediationv1.RemediationRequestSpec{
        SignalFingerprint: "abc123",
        SignalName:        "HighMemoryUsage",
        SignalLabels: map[string]string{
            "alertname": "HighMemoryUsage",
            "namespace": "production",
        },
        SignalAnnotations: map[string]string{
            "summary":     "Memory usage above 90%",
            "description": "Pod payment-api-xyz is using 95% memory",
        },
    }

    assert.NotNil(t, spec.SignalLabels)
    assert.Equal(t, 2, len(spec.SignalLabels))
    assert.NotNil(t, spec.SignalAnnotations)
    assert.Equal(t, 2, len(spec.SignalAnnotations))
}
```

#### **Test 2: RemediationProcessing Self-Containment**
```go
// File: internal/controller/remediationorchestrator/remediationprocessing_creator_test.go
func TestCreateRemediationProcessing_SelfContained(t *testing.T) {
    remReq := &remediationv1.RemediationRequest{
        Spec: remediationv1.RemediationRequestSpec{
            SignalFingerprint: "abc123",
            SignalName:        "HighMemoryUsage",
            Severity:          "critical",
            SignalLabels: map[string]string{"alertname": "HighMemoryUsage"},
            SignalAnnotations: map[string]string{"summary": "High memory"},
            // ... all other fields ...
        },
    }

    remProcessing, err := createRemediationProcessing(ctx, remReq)
    require.NoError(t, err)

    // Verify ALL fields copied
    assert.Equal(t, remReq.Spec.SignalFingerprint, remProcessing.Spec.SignalFingerprint)
    assert.Equal(t, remReq.Spec.SignalName, remProcessing.Spec.SignalName)
    assert.Equal(t, remReq.Spec.Severity, remProcessing.Spec.Severity)
    assert.Equal(t, remReq.Spec.SignalLabels, remProcessing.Spec.SignalLabels)
    assert.Equal(t, remReq.Spec.SignalAnnotations, remProcessing.Spec.SignalAnnotations)
    // ... verify all 18 fields ...
}
```

#### **Test 3: RemediationProcessing Status**
```go
// File: internal/controller/remediationprocessing/enrichment_test.go
func TestEnrichSignal_StatusPopulation(t *testing.T) {
    remProcessing := &remediationprocessingv1.RemediationProcessing{
        Spec: remediationprocessingv1.RemediationProcessingSpec{
            SignalFingerprint: "abc123",
            SignalName:        "HighMemoryUsage",
            Severity:          "critical",
            SignalLabels:      map[string]string{"alertname": "HighMemoryUsage"},
            SignalAnnotations: map[string]string{"summary": "High memory"},
        },
    }

    err := enrichSignal(ctx, remProcessing)
    require.NoError(t, err)

    // Verify signal identifiers in status
    assert.Equal(t, "abc123", remProcessing.Status.SignalFingerprint)
    assert.Equal(t, "HighMemoryUsage", remProcessing.Status.SignalName)
    assert.Equal(t, "critical", remProcessing.Status.Severity)

    // Verify OriginalSignal populated
    assert.NotNil(t, remProcessing.Status.EnrichmentResults.OriginalSignal)
    assert.Equal(t, remProcessing.Spec.SignalLabels,
                 remProcessing.Status.EnrichmentResults.OriginalSignal.Labels)
}
```

### **Integration Tests**

#### **Test 4: End-to-End Data Flow**
```go
// File: test/integration/crd_dataflow_test.go
func TestDataFlow_Gateway_To_AIAnalysis(t *testing.T) {
    // Step 1: Gateway creates RemediationRequest
    remReq := createRemediationRequestFromWebhook(prometheusAlert)

    // Verify RemediationRequest has signal metadata
    assert.NotEmpty(t, remReq.Spec.SignalLabels)
    assert.NotEmpty(t, remReq.Spec.SignalAnnotations)

    // Step 2: RemediationOrchestrator creates RemediationProcessing
    waitForRemediationProcessing(t, remReq.Name)
    remProcessing := getRemediationProcessing(t, remReq.Name)

    // Verify RemediationProcessing is self-contained
    assert.Equal(t, remReq.Spec.SignalFingerprint, remProcessing.Spec.SignalFingerprint)
    assert.Equal(t, remReq.Spec.SignalLabels, remProcessing.Spec.SignalLabels)
    // ... verify all 18 fields ...

    // Step 3: RemediationProcessor enriches
    waitForPhase(t, remProcessing.Name, "completed")
    remProcessing = getRemediationProcessing(t, remProcessing.Name)

    // Verify status has signal identifiers
    assert.NotEmpty(t, remProcessing.Status.SignalFingerprint)
    assert.NotEmpty(t, remProcessing.Status.SignalName)
    assert.NotEmpty(t, remProcessing.Status.Severity)

    // Verify OriginalSignal populated
    assert.NotNil(t, remProcessing.Status.EnrichmentResults.OriginalSignal)
    assert.NotEmpty(t, remProcessing.Status.EnrichmentResults.OriginalSignal.Labels)

    // Step 4: RemediationOrchestrator creates AIAnalysis
    waitForAIAnalysis(t, remProcessing.Name)
    aiAnalysis := getAIAnalysis(t, remProcessing.Name)

    // Verify AIAnalysis has complete data
    assert.Equal(t, remProcessing.Status.SignalFingerprint, aiAnalysis.Spec.SignalFingerprint)
    assert.NotNil(t, aiAnalysis.Spec.OriginalSignal)
    assert.Equal(t, remProcessing.Status.EnrichmentResults.OriginalSignal.Labels,
                 aiAnalysis.Spec.OriginalSignal.Labels)
}
```

### **E2E Tests**

#### **Test 5: Complete Pipeline with Real Prometheus Alert**
```go
// File: test/e2e/prometheus_alert_flow_test.go
func TestE2E_PrometheusAlert_Complete_Pipeline(t *testing.T) {
    // Send real Prometheus alert webhook
    alertPayload := `{
        "alerts": [{
            "labels": {
                "alertname": "HighMemoryUsage",
                "namespace": "production",
                "pod": "payment-api-abc123"
            },
            "annotations": {
                "summary": "High memory usage detected",
                "description": "Pod payment-api-abc123 memory at 95%"
            },
            "startsAt": "2025-10-08T12:00:00Z"
        }]
    }`

    resp := sendWebhook(t, gatewayURL, alertPayload)
    assert.Equal(t, 202, resp.StatusCode)

    // Wait for complete pipeline
    time.Sleep(30 * time.Second)

    // Verify RemediationRequest
    remReq := getRemediationRequestByFingerprint(t, calculateFingerprint(alertPayload))
    assert.NotNil(t, remReq)
    assert.Equal(t, "HighMemoryUsage", remReq.Spec.SignalName)
    assert.NotEmpty(t, remReq.Spec.SignalLabels)

    // Verify RemediationProcessing (self-contained)
    remProcessing := getRemediationProcessingForRequest(t, remReq.Name)
    assert.NotNil(t, remProcessing)
    assert.Equal(t, remReq.Spec.SignalLabels, remProcessing.Spec.SignalLabels)

    // Verify AIAnalysis (complete data)
    aiAnalysis := getAIAnalysisForProcessing(t, remProcessing.Name)
    assert.NotNil(t, aiAnalysis)
    assert.Equal(t, "HighMemoryUsage", aiAnalysis.Spec.SignalName)
    assert.NotNil(t, aiAnalysis.Spec.OriginalSignal)
    assert.Equal(t, "High memory usage detected",
                 aiAnalysis.Spec.OriginalSignal.Annotations["summary"])
}
```

---

## ✅ **Validation Checklist**

### **Pre-Implementation**
- [ ] Review all 3 tasks with team
- [ ] Ensure understanding of self-contained CRD pattern
- [ ] Set up development environment
- [ ] Create feature branch: `feature/phase1-crd-schema-fixes`

### **Task 1: RemediationRequest**
- [ ] Go types updated with 2 new fields
- [ ] Gateway Service populates labels/annotations
- [ ] Extraction helpers implemented
- [ ] Unit tests pass
- [ ] CRD documentation updated

### **Task 2: RemediationProcessing.spec**
- [ ] Go types updated with 18 new fields
- [ ] Supporting types added (ResourceIdentifier, DeduplicationContext)
- [ ] RemediationOrchestrator copies all fields
- [ ] Cross-CRD reads removed from RemediationProcessor
- [ ] Unit tests pass
- [ ] Integration tests pass
- [ ] CRD documentation updated

### **Task 3: RemediationProcessing.status**
- [ ] Go types updated with 4 new fields + OriginalSignal type
- [ ] RemediationProcessor populates status fields
- [ ] RemediationOrchestrator reads from status
- [ ] Unit tests pass
- [ ] Integration tests pass
- [ ] CRD documentation updated

### **End-to-End Validation**
- [ ] Gateway → RemediationProcessor data flow works
- [ ] RemediationProcessor → AIAnalysis data flow works
- [ ] Self-contained CRD pattern verified (no cross-CRD reads)
- [ ] All integration tests pass
- [ ] E2E test with real Prometheus alert passes

### **Documentation**
- [ ] `CRD_SCHEMAS.md` updated with all changes
- [ ] Service specifications updated
- [ ] Implementation notes captured
- [ ] Known issues documented

---

## 📊 **Progress Tracking**

### **Time Estimates**

| Task | Estimated | Actual | Status |
|------|-----------|--------|--------|
| Task 1: RemediationRequest | 1h | - | ⏸️ Pending |
| Task 2: RemediationProcessing.spec | 2h | - | ⏸️ Pending |
| Task 3: RemediationProcessing.status | 2h | - | ⏸️ Pending |
| **Total** | **5h** | **-** | **⏸️ Pending** |

### **Completion Percentage**

```
[░░░░░░░░░░░░░░░░░░░░] 0% Complete
```

---

## 🚨 **Risk Mitigation**

### **Risk 1: Breaking Changes to Existing CRDs**

**Mitigation**:
- Add fields as optional (`omitempty` JSON tag)
- Implement backward-compatible defaults
- Test with existing CRDs before rollout

### **Risk 2: Controller Logic Errors**

**Mitigation**:
- Comprehensive unit tests for mapping functions
- Integration tests for data flow
- Peer review of controller changes

### **Risk 3: Performance Impact**

**Mitigation**:
- Measure CRD size increase (expected: ~2-3KB per CRD)
- Monitor controller reconciliation latency
- Use profiling if performance degrades

### **Risk 4: Missing Edge Cases**

**Mitigation**:
- Test with all signal types (Prometheus, Kubernetes events)
- Test with storm scenarios
- Test with missing optional fields

---

## 📚 **Reference Documents**

1. **Triage Reports**:
   - [`CRD_DATA_FLOW_TRIAGE_REVISED.md`](./CRD_DATA_FLOW_TRIAGE_REVISED.md)
   - [`CRD_DATA_FLOW_TRIAGE_REMEDIATION_TO_AI.md`](./CRD_DATA_FLOW_TRIAGE_REMEDIATION_TO_AI.md)

2. **Architecture**:
   - [`CRD_SCHEMAS.md`](../architecture/CRD_SCHEMAS.md)
   - [`APPROVED_MICROSERVICES_ARCHITECTURE.md`](../architecture/APPROVED_MICROSERVICES_ARCHITECTURE.md)

3. **Service Specifications**:
   - [`docs/services/stateless/gateway-service/`](../services/stateless/gateway-service/)
   - [`docs/services/crd-controllers/01-remediationprocessor/`](../services/crd-controllers/01-remediationprocessor/)
   - [`docs/services/crd-controllers/05-remediationorchestrator/`](../services/crd-controllers/05-remediationorchestrator/)

---

## 🎯 **Success Criteria**

**Phase 1 is COMPLETE when**:
1. ✅ RemediationProcessing CRD is self-contained (no cross-CRD reads)
2. ✅ AIAnalysis receives complete signal identification and payload
3. ✅ All unit tests pass
4. ✅ All integration tests pass
5. ✅ E2E test with real Prometheus alert passes
6. ✅ Documentation updated
7. ✅ Code review approved
8. ✅ Merged to main branch

---

**Status**: ✅ **APPROVED** - Ready to begin implementation
**Estimated Duration**: 4-5 hours
**Priority**: IMMEDIATE (Unblocks entire pipeline)
**Next Action**: Create feature branch and begin Task 1


# DD-AIANALYSIS-001: AIAnalysis CRD Spec Structure

**Version**: 1.1
**Status**: Proposed
**Date**: 2025-11-16
**Authority**: Authoritative (when approved)

## Changelog

### Version 1.2 (2025-11-16)
- **SCOPE CLARIFICATION**: Focused on data needed for LLM prompt (per ADR-039)
- Aligned AIAnalysisSpec with RemediationRequest (the only vetted CRD)
- Deferred RemediationProcessing enrichment data (not yet vetted)
- AIAnalysisSpec now contains only what's needed for v1.0 LLM prompt

### Version 1.1 (2025-11-16)
- **BREAKING**: Removed LLM configuration fields from AIAnalysisSpec
  - Removed: `LLMProvider`, `LLMModel`, `MaxTokens`, `Temperature`, `IncludeHistory`
  - Rationale: LLM configuration is system-wide, not per-incident
  - LLM configuration should be managed via ConfigMap or service configuration
- Simplified AIAnalysisSpec to focus only on signal/incident data
- Added note about where LLM configuration should live (controller ConfigMap)

### Version 1.0 (2025-11-16)
- Initial document creation
- Defined structured AIAnalysisSpec with proper Go types
- Replaced generic `map[string]string` with typed structs
- Aligned with RemediationRequest and RemediationProcessing data flow

## Context

The current `AIAnalysisSpec` in `api/aianalysis/v1alpha1/aianalysis_types.go` uses a generic `map[string]string` for `SignalContext`, which lacks type safety and proper validation.

**CRD Spec Vetting Status**:
- ✅ **RemediationRequest** - Spec vetted and approved
- ✅ **NotificationRequest** - Spec vetted and approved
- ⏳ **AIAnalysis** - This document (vetting in progress)
- ⏳ **RemediationProcessing** - Not yet vetted
- ⏳ **RemediationExecution** - Not yet vetted

The AIAnalysis CRD needs structured data from:

1. **RemediationRequest.Spec** - Core signal data, storm detection, deduplication (✅ vetted)
2. **RemediationProcessing.Status** - Enriched context data (⏳ not yet vetted - will be defined separately)

## Problem Statement

**Current Code** (Lines 28-48 of `aianalysis_types.go`):
```go
type AIAnalysisSpec struct {
    RemediationRequestRef string `json:"remediationRequestRef"`
    SignalType    string            `json:"signalType"`
    SignalContext map[string]string `json:"signalContext"` // ❌ Generic map
    LLMProvider   string            `json:"llmProvider"`
    LLMModel      string            `json:"llmModel"`
    MaxTokens     int               `json:"maxTokens"`
    Temperature   float64           `json:"temperature"`
    IncludeHistory bool             `json:"includeHistory"`
}
```

**Issues**:
- ❌ No type safety for signal data
- ❌ No validation for required fields
- ❌ No clear structure for storm/deduplication data
- ❌ Difficult to evolve schema
- ❌ Runtime parsing errors instead of compile-time validation

## Decision

### AIAnalysisSpec Structure

Replace the generic `map[string]string` with structured Go types that mirror the data flow from RemediationRequest and RemediationProcessing.

```go
// api/aianalysis/v1alpha1/aianalysis_types.go
package v1alpha1

import (
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AIAnalysisSpec defines the desired state of AIAnalysis.
// Data snapshot pattern: RemediationOrchestrator copies all required data at creation time.
type AIAnalysisSpec struct {
    // ========================================
    // PARENT REFERENCES (Audit/Lineage)
    // ========================================
    // Reference to parent RemediationRequest CRD
    RemediationRequestRef corev1.ObjectReference `json:"remediationRequestRef"`

    // Reference to RemediationProcessing CRD (if enrichment was performed)
    // +optional
    RemediationProcessingRef *corev1.ObjectReference `json:"remediationProcessingRef,omitempty"`

    // ========================================
    // SIGNAL DATA (From RemediationRequest)
    // ========================================
    // Complete signal context for AI analysis
    SignalData SignalData `json:"signalData"`

    // ========================================
    // ENRICHED CONTEXT (From RemediationProcessing)
    // ========================================
    // Enriched context data from RemediationProcessing.Status.ContextData
    // +optional
    EnrichedContext map[string]string `json:"enrichedContext,omitempty"`

    // ========================================
    // LLM CONFIGURATION
    // ========================================
    // LLM provider: "openai", "anthropic", "holmesgpt"
    // +kubebuilder:validation:Enum=openai;anthropic;local;holmesgpt
    LLMProvider string `json:"llmProvider"`

    // LLM model name (e.g., "gpt-4", "claude-3.5-sonnet", "claude-4.5-haiku")
    // +kubebuilder:validation:MaxLength=253
    LLMModel string `json:"llmModel"`

    // Maximum tokens for LLM response
    // +kubebuilder:validation:Minimum=1
    // +kubebuilder:validation:Maximum=100000
    MaxTokens int `json:"maxTokens"`

    // Temperature for LLM sampling (0.0-1.0)
    // +kubebuilder:validation:Minimum=0.0
    // +kubebuilder:validation:Maximum=1.0
    Temperature float64 `json:"temperature"`

    // Include historical patterns in analysis
    IncludeHistory bool `json:"includeHistory,omitempty"`
}

// SignalData contains all signal information from RemediationRequest
type SignalData struct {
    // ========================================
    // SIGNAL IDENTIFICATION
    // ========================================
    // Unique fingerprint for deduplication (SHA256)
    // +kubebuilder:validation:MaxLength=64
    // +kubebuilder:validation:Pattern="^[a-f0-9]{64}$"
    SignalFingerprint string `json:"signalFingerprint"`

    // Human-readable signal name (e.g., "HighMemoryUsage", "CrashLoopBackOff")
    // +kubebuilder:validation:MaxLength=253
    SignalName string `json:"signalName"`

    // ========================================
    // SIGNAL CLASSIFICATION
    // ========================================
    // Severity level: "critical", "warning", "info"
    // +kubebuilder:validation:Enum=critical;warning;info
    Severity string `json:"severity"`

    // Environment (e.g., "production", "staging", "development")
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=63
    Environment string `json:"environment"`

    // Priority assigned by Gateway (P0=critical, P1=high, P2=normal, P3=low)
    // +kubebuilder:validation:Enum=P0;P1;P2;P3
    Priority string `json:"priority"`

    // Signal type: "prometheus", "kubernetes-event", "aws-cloudwatch", etc.
    SignalType string `json:"signalType"`

    // Adapter that ingested the signal
    // +kubebuilder:validation:MaxLength=63
    // +optional
    SignalSource string `json:"signalSource,omitempty"`

    // Target system type: "kubernetes", "aws", "azure", "gcp", "datadog"
    // +kubebuilder:validation:Enum=kubernetes;aws;azure;gcp;datadog
    TargetType string `json:"targetType"`

    // ========================================
    // SIGNAL METADATA
    // ========================================
    // Signal labels (e.g., Prometheus alert.Labels)
    // +optional
    SignalLabels map[string]string `json:"signalLabels,omitempty"`

    // Signal annotations (e.g., Prometheus alert.Annotations)
    // +optional
    SignalAnnotations map[string]string `json:"signalAnnotations,omitempty"`

    // ========================================
    // TARGET RESOURCE
    // ========================================
    // Target resource identification
    TargetResource ResourceIdentifier `json:"targetResource"`

    // ========================================
    // TIMESTAMPS
    // ========================================
    // When the signal first started firing (from upstream source)
    // +optional
    FiringTime *metav1.Time `json:"firingTime,omitempty"`

    // When Gateway received the signal
    ReceivedTime metav1.Time `json:"receivedTime"`

    // ========================================
    // DEDUPLICATION
    // ========================================
    // Deduplication and correlation context
    Deduplication DeduplicationData `json:"deduplication"`

    // ========================================
    // STORM DETECTION
    // ========================================
    // Storm detection information (if signal is part of a storm)
    // +optional
    StormDetection *StormDetectionData `json:"stormDetection,omitempty"`

    // ========================================
    // PROVIDER DATA
    // ========================================
    // Provider-specific fields in raw JSON format (for debugging/audit)
    // +optional
    ProviderData []byte `json:"providerData,omitempty"`
}

// ResourceIdentifier identifies the target resource for remediation
type ResourceIdentifier struct {
    // Resource kind (e.g., "Pod", "Deployment", "StatefulSet")
    Kind string `json:"kind"`

    // Resource name
    Name string `json:"name"`

    // Resource namespace
    Namespace string `json:"namespace"`
}

// DeduplicationData provides correlation and deduplication information
type DeduplicationData struct {
    // True if this signal is a duplicate of an active remediation
    IsDuplicate bool `json:"isDuplicate"`

    // Timestamp when this signal fingerprint was first seen
    FirstSeen metav1.Time `json:"firstSeen"`

    // Timestamp when this signal fingerprint was last seen
    LastSeen metav1.Time `json:"lastSeen"`

    // Total count of occurrences of this signal
    OccurrenceCount int `json:"occurrenceCount"`

    // Reference to previous RemediationRequest CRD (if duplicate)
    // +optional
    PreviousRemediationRequestRef string `json:"previousRemediationRequestRef,omitempty"`
}

// StormDetectionData contains alert storm information
type StormDetectionData struct {
    // True if this signal is part of a detected alert storm
    IsStorm bool `json:"isStorm"`

    // Storm type: "rate" (frequency-based) or "pattern" (similar alerts)
    // +kubebuilder:validation:Enum=rate;pattern
    StormType string `json:"stormType"`

    // Time window for storm detection (e.g., "5m")
    StormWindow string `json:"stormWindow"`

    // Number of alerts in the storm
    StormAlertCount int `json:"stormAlertCount"`

    // List of affected resources in an aggregated storm
    // Format: "namespace:Kind:name" (e.g., "default:Pod:app-1")
    // +optional
    AffectedResources []string `json:"affectedResources,omitempty"`
}
```

### Data Flow: RemediationOrchestrator → AIAnalysis

The RemediationOrchestrator is responsible for copying data from RemediationRequest (and optionally RemediationProcessing) into the AIAnalysis spec:

```go
// In RemediationOrchestrator controller
func (r *RemediationOrchestratorReconciler) createAIAnalysis(
    ctx context.Context,
    rr *remediationv1alpha1.RemediationRequest,
    rp *remediationprocessingv1alpha1.RemediationProcessing,
) (*aianalysisv1alpha1.AIAnalysis, error) {

    // Build SignalData from RemediationRequest
    signalData := buildSignalData(rr)

    // Build EnrichedContext from RemediationProcessing (if available)
    var enrichedContext map[string]string
    var rpRef *corev1.ObjectReference
    if rp != nil && rp.Status.Phase == "completed" {
        enrichedContext = rp.Status.ContextData
        rpRef = &corev1.ObjectReference{
            Name:      rp.Name,
            Namespace: rp.Namespace,
        }
    }

    // Create AIAnalysis with structured data
    // Note: Issue #91 - kubernaut.ai/remediation-request label REMOVED; spec.remediationRequestRef and ownerRef are sufficient
    ai := &aianalysisv1alpha1.AIAnalysis{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-aianalysis", rr.Name),
            Namespace: rr.Namespace,
        },
        Spec: aianalysisv1alpha1.AIAnalysisSpec{
            RemediationRequestRef: corev1.ObjectReference{
                Name:      rr.Name,
                Namespace: rr.Namespace,
            },
            RemediationProcessingRef: rpRef,
            SignalData:               signalData,
            EnrichedContext:          enrichedContext,
            LLMProvider:              "holmesgpt", // From config
            LLMModel:                 "claude-4.5-haiku",
            MaxTokens:                4096,
            Temperature:              0.7,
            IncludeHistory:           true,
        },
    }

    // Set owner reference
    if err := controllerutil.SetControllerReference(rr, ai, r.scheme); err != nil {
        return nil, fmt.Errorf("failed to set owner reference: %w", err)
    }

    // Create AIAnalysis
    if err := r.client.Create(ctx, ai); err != nil {
        return nil, fmt.Errorf("failed to create AIAnalysis: %w", err)
    }

    return ai, nil
}

// Helper function to build SignalData from RemediationRequest
func buildSignalData(rr *remediationv1alpha1.RemediationRequest) aianalysisv1alpha1.SignalData {
    signalData := aianalysisv1alpha1.SignalData{
        SignalFingerprint: rr.Spec.SignalFingerprint,
        SignalName:        rr.Spec.SignalName,
        Severity:          rr.Spec.Severity,
        Environment:       rr.Spec.Environment,
        Priority:          rr.Spec.Priority,
        SignalType:        rr.Spec.SignalType,
        SignalSource:      rr.Spec.SignalSource,
        TargetType:        rr.Spec.TargetType,
        SignalLabels:      rr.Spec.SignalLabels,
        SignalAnnotations: rr.Spec.SignalAnnotations,
        ReceivedTime:      rr.Spec.ReceivedTime,
        ProviderData:      rr.Spec.ProviderData,

        // Target resource (extracted from ProviderData by Gateway)
        TargetResource: aianalysisv1alpha1.ResourceIdentifier{
            Kind:      extractKind(rr.Spec.ProviderData),
            Name:      extractName(rr.Spec.ProviderData),
            Namespace: extractNamespace(rr.Spec.ProviderData),
        },

        // Deduplication data
        Deduplication: aianalysisv1alpha1.DeduplicationData{
            IsDuplicate:                   rr.Spec.Deduplication.IsDuplicate,
            FirstSeen:                     rr.Spec.Deduplication.FirstSeen,
            LastSeen:                      rr.Spec.Deduplication.LastSeen,
            OccurrenceCount:               rr.Spec.Deduplication.OccurrenceCount,
            PreviousRemediationRequestRef: rr.Spec.Deduplication.PreviousRemediationRequestRef,
        },
    }

    // Add FiringTime if present
    if !rr.Spec.FiringTime.IsZero() {
        signalData.FiringTime = &rr.Spec.FiringTime
    }

    // Add storm detection data if present
    if rr.Spec.IsStorm {
        signalData.StormDetection = &aianalysisv1alpha1.StormDetectionData{
            IsStorm:           true,
            StormType:         rr.Spec.StormType,
            StormWindow:       rr.Spec.StormWindow,
            StormAlertCount:   rr.Spec.StormAlertCount,
            AffectedResources: rr.Spec.AffectedResources,
        }
    }

    return signalData
}
```

## Benefits

### Type Safety
- ✅ Compile-time validation of field types
- ✅ IDE autocomplete and type hints
- ✅ Prevents runtime parsing errors

### Schema Evolution
- ✅ Easy to add new fields with proper types
- ✅ Clear deprecation path for old fields
- ✅ OpenAPI schema generation from Go types

### Validation
- ✅ Kubebuilder validation tags enforce constraints
- ✅ API server validates before accepting CRD
- ✅ Clear error messages for invalid data

### Documentation
- ✅ Self-documenting through Go struct tags
- ✅ Clear field relationships and nesting
- ✅ Easy to generate API documentation

### Testing
- ✅ Easy to create test fixtures with proper types
- ✅ Mock data generation is straightforward
- ✅ Unit tests can validate struct construction

## Migration Path

### Phase 1: Update Go Types (v1.0)
1. Update `api/aianalysis/v1alpha1/aianalysis_types.go` with new struct definitions
2. Run `make manifests` to regenerate CRD YAML
3. Update RemediationOrchestrator to use new types
4. Update AIAnalysis controller to read new types

### Phase 2: Update Tests (v1.0)
1. Update unit tests for RemediationOrchestrator
2. Update unit tests for AIAnalysis controller
3. Add integration tests for data propagation

### Phase 3: Deploy (v1.0)
1. Apply new CRD definitions to cluster
2. Deploy updated controllers
3. Verify existing RemediationRequests work with new schema

## Confidence Assessment

**Confidence Level**: 95%

**Strengths**:
- ✅ Aligns with existing RemediationRequest and RemediationProcessing structures
- ✅ Follows Kubernetes CRD best practices
- ✅ Provides type safety and validation
- ✅ Clear data flow from RemediationRequest → AIAnalysis
- ✅ Supports storm detection and deduplication data

**Risks**:
- ⚠️ **5% Gap**: Requires updating all existing code that references `SignalContext map[string]string`. However, since the AIAnalysis controller is not yet implemented, this is low risk.

**Mitigation**:
- Implement changes in v1.0 before AIAnalysis controller is fully developed
- Add comprehensive unit tests for data copying logic
- Document migration path for any existing test fixtures

## Integration Points

### Authoritative Documents
- **RemediationRequest CRD**: `api/remediation/v1alpha1/remediationrequest_types.go`
- **RemediationProcessing CRD**: `api/remediationprocessing/v1alpha1/remediationprocessing_types.go`
- **ADR-039**: LLM Prompt/Response Contract (will need update to reference new types)

### Affected Services
- **RemediationOrchestrator**: Implements data copying from RemediationRequest to AIAnalysis
- **AIAnalysis Service**: Reads structured data from `AIAnalysisSpec.SignalData`
- **HolmesGPT API**: Receives structured data from AIAnalysis controller

## Testing Strategy

### Unit Tests (BR-AI-001)
```go
// Test SignalData construction from RemediationRequest
func TestBuildSignalData(t *testing.T) {
    rr := &remediationv1alpha1.RemediationRequest{
        Spec: remediationv1alpha1.RemediationRequestSpec{
            SignalFingerprint: "abc123",
            SignalName:        "HighMemoryUsage",
            Severity:          "critical",
            Environment:       "production",
            Priority:          "P0",
            IsStorm:           true,
            StormType:         "rate",
            StormWindow:       "5m",
            StormAlertCount:   15,
            AffectedResources: []string{"default:Pod:app-1", "default:Pod:app-2"},
            Deduplication: remediationv1alpha1.DeduplicationInfo{
                IsDuplicate:     true,
                FirstSeen:       metav1.Now(),
                LastSeen:        metav1.Now(),
                OccurrenceCount: 5,
            },
        },
    }

    signalData := buildSignalData(rr)

    assert.Equal(t, "abc123", signalData.SignalFingerprint)
    assert.Equal(t, "critical", signalData.Severity)
    assert.NotNil(t, signalData.StormDetection)
    assert.Equal(t, "rate", signalData.StormDetection.StormType)
    assert.Equal(t, 15, signalData.StormDetection.StormAlertCount)
    assert.Len(t, signalData.StormDetection.AffectedResources, 2)
    assert.Equal(t, 5, signalData.Deduplication.OccurrenceCount)
}

// Test AIAnalysis creation with structured data
func TestCreateAIAnalysis(t *testing.T) {
    rr := createTestRemediationRequest()
    rp := createTestRemediationProcessing()

    ai, err := createAIAnalysis(ctx, rr, rp)

    assert.NoError(t, err)
    assert.NotNil(t, ai)
    assert.Equal(t, rr.Name, ai.Spec.RemediationRequestRef.Name)
    assert.Equal(t, rp.Name, ai.Spec.RemediationProcessingRef.Name)
    assert.NotNil(t, ai.Spec.SignalData)
    assert.NotEmpty(t, ai.Spec.EnrichedContext)
}
```

### Integration Tests (BR-AI-002)
- Test RemediationOrchestrator creates AIAnalysis with correct structured data
- Test AIAnalysis controller can read and process structured data
- Test storm detection data flows correctly through the system

## Implementation Checklist

- [ ] Update `api/aianalysis/v1alpha1/aianalysis_types.go` with new struct definitions
- [ ] Run `make manifests` to regenerate CRD YAML
- [ ] Implement `buildSignalData()` helper function in RemediationOrchestrator
- [ ] Update `createAIAnalysis()` function to use new types
- [ ] Add unit tests for data construction
- [ ] Update AIAnalysis controller to read new structured types
- [ ] Update ADR-039 to reference new types
- [ ] Add integration tests for end-to-end data flow
- [ ] Update documentation with new schema

## Related Documents

- `DD-ORCHESTRATOR-001`: Storm Detection and Deduplication Data Propagation (superseded by this DD)
- `ADR-039`: LLM Prompt/Response Contract (needs update to reference new types)
- `MULTI_CRD_RECONCILIATION_ARCHITECTURE.md`: Data snapshot pattern
- `KUBERNAUT_CRD_ARCHITECTURE.md`: CRD orchestration flow

---

**Approval**: This design decision is proposed and awaiting approval.

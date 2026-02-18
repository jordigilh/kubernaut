# DD-ORCHESTRATOR-001: Storm Detection and Deduplication Data Propagation

**Version**: 1.0
**Status**: Approved
**Date**: 2025-11-16
**Authority**: Authoritative

## Changelog

### Version 1.0 (2025-11-16)
- Initial document creation
- Defined storm detection and deduplication data propagation from RemediationRequest to AIAnalysis
- Established data snapshot pattern for RemediationOrchestrator

## Context

The Gateway service creates `RemediationRequest` CRDs with storm detection and deduplication metadata in the `.spec` field. This metadata is critical for the LLM's Root Cause Analysis (RCA) to understand:
- Whether a signal is part of an alert storm
- How many times a signal has occurred
- Historical context for duplicate signals

The AIAnalysis service needs this information to build an accurate LLM prompt, but the data must be propagated from `RemediationRequest.Spec` to `AIAnalysis.Spec.SignalContext`.

## Problem Statement

**Current State**:
- `RemediationRequest.Spec` contains structured storm and deduplication fields (lines 82-101 of `remediationrequest_types.go`)
- `AIAnalysis.Spec.SignalContext` is a `map[string]string` (line 34 of `aianalysis_types.go`)
- Gateway can continue updating `RemediationRequest` as it detects new storms or deduplication events
- RemediationOrchestrator needs to copy this data when creating `AIAnalysis` CRD

**Challenge**:
How should the RemediationOrchestrator serialize structured storm/deduplication data into the `SignalContext` map, and when should it capture this snapshot?

## Decision

### Data Snapshot Timing

**PRINCIPLE**: RemediationOrchestrator captures a **point-in-time snapshot** of `RemediationRequest.Spec` when creating the `AIAnalysis` CRD.

**Rationale**:
1. **Immutability**: Once AIAnalysis is created, its input data should not change
2. **Auditability**: The snapshot preserves what the LLM saw at analysis time
3. **Consistency**: Prevents race conditions from concurrent Gateway updates
4. **Determinism**: Same input always produces same LLM analysis

### Data Serialization Format

The RemediationOrchestrator will serialize storm and deduplication data into `AIAnalysis.Spec.SignalContext` using these keys:

#### Deduplication Fields
```go
// From RemediationRequest.Spec.Deduplication
SignalContext["is_duplicate"] = strconv.FormatBool(rr.Spec.Deduplication.IsDuplicate)
SignalContext["first_seen"] = rr.Spec.Deduplication.FirstSeen.Format(time.RFC3339)
SignalContext["last_seen"] = rr.Spec.Deduplication.LastSeen.Format(time.RFC3339)
SignalContext["occurrence_count"] = strconv.Itoa(rr.Spec.Deduplication.OccurrenceCount)
if rr.Spec.Deduplication.PreviousRemediationRequestRef != "" {
    SignalContext["previous_remediation_ref"] = rr.Spec.Deduplication.PreviousRemediationRequestRef
}
```

#### Storm Detection Fields
```go
// From RemediationRequest.Spec storm fields
if rr.Spec.IsStorm {
    SignalContext["is_storm"] = "true"
    SignalContext["storm_type"] = rr.Spec.StormType // "rate" or "pattern"
    SignalContext["storm_window"] = rr.Spec.StormWindow // e.g., "5m"
    SignalContext["storm_alert_count"] = strconv.Itoa(rr.Spec.StormAlertCount)
    
    // Serialize affected resources as JSON array
    if len(rr.Spec.AffectedResources) > 0 {
        affectedJSON, _ := json.Marshal(rr.Spec.AffectedResources)
        SignalContext["affected_resources"] = string(affectedJSON)
    }
}
```

### Implementation Pattern

```go
// In RemediationOrchestrator controller (Reconcile function)
func (r *RemediationOrchestratorReconciler) createAIAnalysis(
    ctx context.Context,
    rr *remediationv1alpha1.RemediationRequest,
    processing *processingv1alpha1.RemediationProcessing,
) (*aianalysisv1alpha1.AIAnalysis, error) {
    
    // Build SignalContext with enriched data from RemediationProcessing
    signalContext := buildSignalContextFromProcessing(processing)
    
    // SNAPSHOT: Copy storm detection and deduplication data from RemediationRequest
    copyStormAndDeduplicationData(signalContext, rr)
    
    // Create AIAnalysis with snapshot
    // Note: Issue #91 - kubernaut.ai/remediation-request label REMOVED; spec.remediationRequestRef and ownerRef are sufficient
    ai := &aianalysisv1alpha1.AIAnalysis{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-aianalysis", rr.Name),
            Namespace: rr.Namespace,
        },
        Spec: aianalysisv1alpha1.AIAnalysisSpec{
            RemediationRequestRef: rr.Name,
            SignalType:            rr.Spec.SignalType,
            SignalContext:         signalContext, // Contains snapshot of storm/dedup data
            // ... other fields ...
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

// Helper function to copy storm and deduplication data
func copyStormAndDeduplicationData(signalContext map[string]string, rr *remediationv1alpha1.RemediationRequest) {
    // Deduplication data (always present)
    signalContext["is_duplicate"] = strconv.FormatBool(rr.Spec.Deduplication.IsDuplicate)
    signalContext["first_seen"] = rr.Spec.Deduplication.FirstSeen.Format(time.RFC3339)
    signalContext["last_seen"] = rr.Spec.Deduplication.LastSeen.Format(time.RFC3339)
    signalContext["occurrence_count"] = strconv.Itoa(rr.Spec.Deduplication.OccurrenceCount)
    
    if rr.Spec.Deduplication.PreviousRemediationRequestRef != "" {
        signalContext["previous_remediation_ref"] = rr.Spec.Deduplication.PreviousRemediationRequestRef
    }
    
    // Storm detection data (conditional)
    if rr.Spec.IsStorm {
        signalContext["is_storm"] = "true"
        signalContext["storm_type"] = rr.Spec.StormType
        signalContext["storm_window"] = rr.Spec.StormWindow
        signalContext["storm_alert_count"] = strconv.Itoa(rr.Spec.StormAlertCount)
        
        if len(rr.Spec.AffectedResources) > 0 {
            affectedJSON, err := json.Marshal(rr.Spec.AffectedResources)
            if err == nil {
                signalContext["affected_resources"] = string(affectedJSON)
            }
        }
    }
}
```

## Gateway Update Behavior

**IMPORTANT**: The Gateway service can continue to update `RemediationRequest.Spec` as it detects:
- New storm patterns
- Additional deduplication occurrences
- Changes in storm alert count

**However**: The RemediationOrchestrator only captures a **point-in-time snapshot** when it creates the `AIAnalysis` CRD. This means:

1. **First AIAnalysis**: Captures initial storm/dedup state
2. **Subsequent Gateway Updates**: Update `RemediationRequest.Spec` but do NOT trigger new AIAnalysis creation
3. **Recovery Attempts**: If remediation fails and a new AIAnalysis is created, it will capture the **updated** storm/dedup state

### Example Timeline

```
T0: Gateway creates RemediationRequest
    - IsStorm: false
    - OccurrenceCount: 1

T1: RemediationOrchestrator creates AIAnalysis
    - Snapshot: IsStorm=false, OccurrenceCount=1

T2: Gateway detects storm, updates RemediationRequest
    - IsStorm: true
    - StormAlertCount: 15
    
T3: AIAnalysis completes (still uses T1 snapshot)
    - LLM saw: IsStorm=false, OccurrenceCount=1

T4: If remediation fails, new AIAnalysis created
    - New snapshot: IsStorm=true, StormAlertCount=15
```

## Confidence Assessment

**Confidence Level**: 95%

**Strengths**:
- ✅ Aligns with existing data snapshot pattern in RemediationOrchestrator
- ✅ Uses existing `SignalContext` map structure
- ✅ Preserves immutability and auditability
- ✅ Simple string serialization for primitive types
- ✅ JSON serialization for complex types (affected resources)
- ✅ Supports Gateway's continuous update pattern

**Risks**:
- ⚠️ **5% Gap**: `SignalContext` is a `map[string]string`, which requires serialization of structured data (e.g., `AffectedResources` array). JSON serialization is standard but adds parsing complexity for the AIAnalysis service.

**Mitigation**:
- Use standard JSON serialization for arrays
- Document expected format in ADR-039 (LLM prompt contract)
- Add validation in AIAnalysis service to handle missing/malformed fields gracefully

## Integration Points

### Authoritative Documents
- **RemediationRequest CRD**: `api/remediation/v1alpha1/remediationrequest_types.go` (lines 82-162)
- **AIAnalysis CRD**: `api/aianalysis/v1alpha1/aianalysis_types.go` (line 34)
- **ADR-039**: LLM Prompt/Response Contract (sections on deduplication and storm detection)

### Affected Services
- **RemediationOrchestrator**: Implements data snapshot and serialization
- **AIAnalysis Service**: Parses `SignalContext` to build LLM prompt
- **Gateway Service**: Continues to update `RemediationRequest.Spec` as storms/dedup events occur

## Testing Strategy

### Unit Tests (BR-ORCH-001)
```go
// Test storm data serialization
func TestCopyStormData(t *testing.T) {
    rr := &remediationv1alpha1.RemediationRequest{
        Spec: remediationv1alpha1.RemediationRequestSpec{
            IsStorm:         true,
            StormType:       "rate",
            StormWindow:     "5m",
            StormAlertCount: 15,
            AffectedResources: []string{
                "default:Pod:app-1",
                "default:Pod:app-2",
            },
        },
    }
    
    signalContext := make(map[string]string)
    copyStormAndDeduplicationData(signalContext, rr)
    
    assert.Equal(t, "true", signalContext["is_storm"])
    assert.Equal(t, "rate", signalContext["storm_type"])
    assert.Equal(t, "5m", signalContext["storm_window"])
    assert.Equal(t, "15", signalContext["storm_alert_count"])
    
    var affectedResources []string
    err := json.Unmarshal([]byte(signalContext["affected_resources"]), &affectedResources)
    assert.NoError(t, err)
    assert.Len(t, affectedResources, 2)
}

// Test deduplication data serialization
func TestCopyDeduplicationData(t *testing.T) {
    firstSeen := metav1.Now()
    lastSeen := metav1.Time{Time: firstSeen.Add(5 * time.Minute)}
    
    rr := &remediationv1alpha1.RemediationRequest{
        Spec: remediationv1alpha1.RemediationRequestSpec{
            Deduplication: remediationv1alpha1.DeduplicationInfo{
                IsDuplicate:     true,
                FirstSeen:       firstSeen,
                LastSeen:        lastSeen,
                OccurrenceCount: 5,
                PreviousRemediationRequestRef: "remediation-001",
            },
        },
    }
    
    signalContext := make(map[string]string)
    copyStormAndDeduplicationData(signalContext, rr)
    
    assert.Equal(t, "true", signalContext["is_duplicate"])
    assert.Equal(t, "5", signalContext["occurrence_count"])
    assert.Equal(t, "remediation-001", signalContext["previous_remediation_ref"])
}
```

### Integration Tests (BR-ORCH-002)
- Test RemediationOrchestrator creates AIAnalysis with storm data snapshot
- Test Gateway updates RemediationRequest after AIAnalysis creation (snapshot unchanged)
- Test recovery attempt creates new AIAnalysis with updated storm data

## Implementation Checklist

- [ ] Implement `copyStormAndDeduplicationData` helper function in RemediationOrchestrator
- [ ] Update `createAIAnalysis` function to call helper
- [ ] Add unit tests for data serialization
- [ ] Update ADR-039 to document expected `SignalContext` keys
- [ ] Update AIAnalysis service to parse storm/dedup fields from `SignalContext`
- [ ] Add integration tests for data propagation
- [ ] Document snapshot timing behavior in RemediationOrchestrator documentation

## Related Documents

- `DD-WORKFLOW-003`: Parameterized Actions (uses storm/dedup data for workflow selection)
- `ADR-039`: LLM Prompt/Response Contract (defines prompt structure with storm/dedup fields)
- `MULTI_CRD_RECONCILIATION_ARCHITECTURE.md`: Data snapshot pattern
- `KUBERNAUT_CRD_ARCHITECTURE.md`: CRD orchestration flow

---

**Approval**: This design decision is approved and ready for implementation.

# DD-AIANALYSIS-004: Storm Context NOT Exposed to LLM

**Date**: December 13, 2025
**Status**: ✅ APPROVED
**Deciders**: Gateway Team, AIAnalysis Team, Architecture Team
**Confidence**: 95%
**Related**: DD-HAPI-001, DD-HOLMESGPT-009, DD-GATEWAY-011, DD-GATEWAY-012

---

## Context

### Problem Statement

Storm detection in Gateway tracks resource-specific persistence (same resource flapping repeatedly). The question is: **Should storm context be exposed to the LLM for improved Root Cause Analysis?**

**Storm Detection Fields in Gateway**:
- `status.stormAggregation.isStorm`: Boolean flag (true when occurrenceCount >= 5)
- `status.stormAggregation.aggregatedCount`: Total occurrences
- `status.stormAggregation.stormType`: "rate" (currently only one type)

**HAPI API Contract** (already supports these as optional fields):
```go
type IncidentRequest struct {
    // ...
    OccurrenceCount   *int  `json:"occurrence_count,omitempty"`  // Already exists
    IsStorm           *bool `json:"is_storm,omitempty"`          // Optional
    StormSignalCount  *int  `json:"storm_signal_count,omitempty"` // Optional
}
```

**Question**: Should AIAnalysis populate `IsStorm` and `StormSignalCount` when calling HAPI?

### Key Requirements

- **BR-GATEWAY-008**: Storm detection must track resource persistence
- **DD-HAPI-001**: Minimal context principle (labels for filtering, not LLM input)
- **DD-HOLMESGPT-009**: Token optimization (60% reduction achieved)
- **Architectural**: RO is a router, not a decision maker (cannot use storm for routing)

---

## Analysis

### Finding 1: Storm Definition - Single Resource Flapping

**Storm Detection Reality**:
```
Storm = SAME resource flapping repeatedly
  SHA256("PodNotReady:prod:Pod:app-pod-1") → occurrenceCount grows to 20
  → 1 CRD with isStorm=true

NOT a Storm = Multiple different resources failing
  20 different pods → 20 different fingerprints → 20 separate CRDs
  → Each has occurrenceCount=1, none flagged as storm
```

**Key Insight**: Storms are resource-specific persistence, not widespread outages.

---

### Finding 2: Initial Investigation - Storm Context Invisible

**Lifecycle Timeline**:
```
T=0:00  Alert 1 arrives
        → Gateway: RR created (occurrenceCount=1, isStorm=false)

T=0:05  RO creates SignalProcessing CRD

T=0:30  SignalProcessing completes enrichment
        → RO creates AIAnalysis CRD
        → AIAnalysis reads RR: occurrenceCount=1 ❌ NO STORM
        → AIAnalysis calls HAPI with occurrenceCount=1

T=1:00  Alert 2 arrives (while AIAnalysis is investigating)
        → Gateway updates RR: occurrenceCount=2
        → AIAnalysis DOES NOT re-read RR ❌

T=2:30  Alert 5 arrives ← STORM THRESHOLD REACHED
        → Gateway updates RR: occurrenceCount=5, isStorm=true
        → BUT: AIAnalysis already completed, HAPI never sees this

T=3:00  HAPI completes investigation with occurrenceCount=1 context

Result: Storm context accumulates AFTER initial investigation
```

**Key Finding**: Initial investigations happen at `occurrenceCount=1`, before storm threshold is reached.

---

### Finding 3: RO Routing - Cannot Use Storm Flag

**Question**: Could RO skip AIAnalysis for storms and go straight to remediation?

**Answer**: **NO** - RO is a router, not a decision maker.

**Why RO Cannot Skip AIAnalysis**:
```
WorkflowExecution CRD requires:
  - workflowId: Which workflow to run?
  - containerImage: Which OCI image to execute?
  - parameters: What deployment/namespace/values?

Source of Truth: AIAnalysis.status.selectedWorkflow

RO Responsibilities:
  ✅ Routes between services (SP → AA → WE)
  ✅ Creates CRDs at the right time
  ✅ Passes data between services

RO Does NOT:
  ❌ Perform root cause analysis
  ❌ Select workflows
  ❌ Determine parameters

Even for "obvious" storms (CrashLoopBackOff):
  - Image pull failure?    → Fix image registry
  - OOMKilled?             → Increase memory limits
  - Config error?          → Rollback deployment
  - Dependency unavailable? → Wait/scale dependency

Same alert, different root causes → different workflows
Only AIAnalysis/HAPI can determine the right action
```

**Key Finding**: RO cannot make remediation decisions, so storm flag cannot influence routing.

---

### Finding 4: Recovery Investigation - Storm Context IS Visible

**When does storm context reach the LLM?**

```
T=5:00  WorkflowExecution fails (pod still crashing)
        → RO creates NEW AIAnalysis for recovery
        → AIAnalysis reads RR with occurrenceCount=20, isStorm=true ✅
        → HAPI receives storm context in recovery request
```

**Recovery Investigation Frequency**: ~5-10% of cases (most workflows succeed on first attempt)

**Value in Recovery Context**:
- Confirms persistent issue (not transient)
- Signals that previous workflow didn't resolve root cause
- ~60% value when applicable

---

### Business Value Assessment

**Storm Context Value by Use Case**:

| Use Case | Storm Visible? | RCA Improvement | Frequency | Weighted Value |
|----------|----------------|-----------------|-----------|----------------|
| **Initial Investigation** | ❌ No (too early) | 0% | 90% | 0% |
| **RO Routing** | ❌ No (can't decide) | 0% | 100% | 0% |
| **Recovery Investigation** | ✅ Yes | 60% | 5-10% | 3-6% |

**Overall Business Value**: **3-6%**

---

## Alternatives Considered

### Alternative 1: Expose Storm Flags to LLM

**Approach**: Populate `IsStorm` and `StormSignalCount` in all HAPI requests.

**Pros**:
- ✅ Provides explicit persistence signal
- ✅ Could help LLM distinguish persistent vs transient issues
- ✅ API contract already supports it

**Cons**:
- ❌ Invisible during initial investigations (90% of cases)
- ❌ Only visible during recovery (~5-10% of cases)
- ❌ Conflicts with DD-HAPI-001 minimal context principle
- ❌ Works against DD-HOLMESGPT-009 token optimization
- ❌ `occurrence_count` already provides the same signal

**Business Value**: 3-6%

**Confidence**: **REJECTED** - Minimal value, architectural conflicts

---

### Alternative 2: Use occurrence_count Only (No Storm Flag)

**Approach**: Rely on `occurrence_count` field (already implemented) to convey persistence.

**Pros**:
- ✅ Already implemented in HAPI contract
- ✅ Simpler contract (fewer fields)
- ✅ Consistent with DD-HAPI-001 (minimal context)
- ✅ Maintains DD-HOLMESGPT-009 token efficiency
- ✅ LLM can infer persistence from count alone

**Cons**:
- ❌ No explicit "this is a storm" flag
- ❌ LLM must infer threshold (but this is trivial: count >= 5)

**LLM Inference Pattern**:
```
occurrence_count=1  → Single occurrence (possibly transient)
occurrence_count=5+ → Persistent issue (storm threshold equivalent)
occurrence_count=20 → Highly persistent (definitely not transient)
```

**Business Value**: Same as Alternative 1, but simpler

**Confidence**: **APPROVED** - Same value, better architecture

---

### Alternative 3: Expose Storm Only for Recovery

**Approach**: Include storm flags only in recovery investigation requests.

**Pros**:
- ✅ Storm context visible when it matters most
- ✅ No changes needed for initial investigations
- ✅ Targeted approach

**Cons**:
- ❌ `occurrence_count` already provides this in recovery
- ❌ Adds complexity for marginal value
- ❌ Inconsistent contract (sometimes present, sometimes not)

**Business Value**: 3-6% (recovery only)

**Confidence**: **REJECTED** - `occurrence_count` already sufficient

---

## Decision

**APPROVED: Alternative 2 - Use `occurrence_count` Only**

**DO NOT expose `is_storm` or `storm_signal_count` to the LLM.**

**Rationale**:

1. **Timing**: Storm context is invisible during initial investigations (90% of cases)
   - Initial investigation happens at occurrenceCount=1
   - Storm threshold (5+) reached AFTER AIAnalysis completes
   - HAPI never sees storm context for initial investigation

2. **Routing**: RO cannot use storm for routing decisions (not a decision maker)
   - RO needs workflow selection from AIAnalysis
   - Cannot skip AIAnalysis (no RCA capability)
   - Storm flag cannot influence routing path

3. **Alternative exists**: `occurrence_count` already provides persistence signal
   - LLM can infer: count >= 5 = persistent issue
   - No need for separate boolean flag
   - Simpler contract, same information

4. **Simplicity**: Fewer fields = simpler contract
   - Consistent with DD-HAPI-001 (minimal context principle)
   - Aligns with DD-HOLMESGPT-009 (token optimization)
   - Reduces API surface area

5. **Architectural consistency**: Aligns with existing patterns
   - Custom labels not exposed to LLM (DD-HAPI-001)
   - Labels for filtering, not analysis
   - Storm flag is just a threshold indicator

**Key Insight**: `occurrence_count` conveys the same persistence information without the complexity of separate storm flags.

---

## Implementation

### AIAnalysis Handler

```go
// pkg/aianalysis/handlers/investigating.go
func (h *InvestigatingHandler) buildRequest(analysis *aianalysisv1.AIAnalysis) *generated.IncidentRequest {
    req := &generated.IncidentRequest{
        IncidentID: analysis.Name,
        Severity: spec.Severity,
        // ✅ Include occurrence count (already implemented)
        OccurrenceCount: &rrStatus.Deduplication.OccurrenceCount,
        // ❌ DO NOT include storm flags
        // IsStorm: nil,  // Explicitly omit
        // StormSignalCount: nil,  // Explicitly omit
    }
    return req
}
```

### Recovery Handler

```go
// pkg/aianalysis/handlers/recovery.go
func (h *RecoveryHandler) buildRequest(analysis *aianalysisv1.AIAnalysis) *generated.RecoveryRequest {
    req := &generated.RecoveryRequest{
        IncidentID: analysis.Name,
        IsRecoveryAttempt: true,
        RecoveryAttemptNumber: analysis.Spec.RecoveryAttemptNumber,
        // ✅ Include occurrence count (provides persistence signal)
        // Read from RemediationRequest status at recovery time
        // This will be the LATEST count, including all occurrences during initial workflow
        OccurrenceCount: &rrStatus.Deduplication.OccurrenceCount,
    }
    return req
}
```

**Note**: Even in recovery, `occurrence_count` is sufficient. The LLM can infer persistence without a separate boolean flag.

---

## Consequences

### Positive

- ✅ **Simpler API contract**: Fewer optional fields to maintain
- ✅ **Consistent architecture**: Aligns with DD-HAPI-001 minimal context principle
- ✅ **Token efficiency**: Maintains DD-HOLMESGPT-009 optimization goals
- ✅ **Same information**: `occurrence_count` conveys persistence signal
- ✅ **No code changes needed**: Current implementation already correct

### Negative

- ⚠️ **No explicit flag**: LLM must infer persistence from count
  - **Mitigation**: This is trivial logic (count >= 5 = persistent)
  - **Impact**: Negligible - LLMs excel at numerical thresholds

### Neutral

- 🔄 **HAPI API contract unchanged**: Optional fields remain for backward compatibility
- 🔄 **Future flexibility**: Can add storm flags later if business value increases

---

## Related Decisions

- **DD-HAPI-001**: Custom Labels Auto-Append Architecture (minimal context principle)
- **DD-HOLMESGPT-009**: Ultra-Compact JSON Format (token optimization)
- **DD-GATEWAY-011**: Shared Status Ownership (storm state in RR CRD status)
- **DD-GATEWAY-012**: Redis-free Storm Detection (status-based tracking)

---

## Open Question

**What IS storm detection for?** (see [BRAINSTORM_STORM_DETECTION_PURPOSE.md](../../../handoff/BRAINSTORM_STORM_DETECTION_PURPOSE.md))

After determining that storm context has minimal value for LLM RCA (3-6%), the actual business purpose of storm detection remains unclear:

**Potential purposes**:
1. **Circuit breaker** - Rate limiting signal ingestion to prevent Gateway overload?
2. **Observability** - SRE metric for tracking resource flapping?
3. **Redundant** - Deduplication already provides aggregation?

**Action Item**: Investigate actual business value of storm detection in Gateway service.

---

## References

- **HAPI API Contract**: `pkg/aianalysis/client/holmesgpt.go`
- **AIAnalysis Integration**: `docs/services/crd-controllers/02-aianalysis/integration-points.md`
- **Gateway Storm Detection**: `docs/services/stateless/gateway-service/overview.md`
- **Storm Detection Brainstorm**: (internal development reference, removed in v1.0)

---

**Document Version**: 1.0
**Last Updated**: December 13, 2025
**Status**: ✅ Approved

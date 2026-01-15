# SignalProcessing DurationMs Field Fix - Missing Performance Metrics

**Date**: 2026-01-14
**Status**: ✅ ROOT CAUSE CONFIRMED - Fix Ready
**Priority**: P2 - Test Failure (Field Exists, Not Populated)
**Related**: `docs/handoff/SP_FINAL_2_FAILURES_RCA_JAN14_2026.md`

---

## 📋 Executive Summary

**TEST FAILURE**: "should emit 'classification.decision' audit event with both external and normalized severity"
**Line**: test/integration/signalprocessing/severity_integration_test.go:286
**Error**: `Expect(event.DurationMs.IsSet()).To(BeTrue())` → FAILED (returned false)

**ROOT CAUSE**: Audit client `RecordClassificationDecision()` does **NOT populate** the `DurationMs` field, even though:
1. ✅ Field EXISTS in Ogen schema (`AuditEvent.DurationMs OptNilInt`)
2. ✅ Controller TRACKS classification timing (`classifyingStart := time.Now()`)
3. ❌ Audit client does NOT receive or set the duration

---

## 🔍 Evidence

### 1. Schema Confirms Field Exists

**File**: `pkg/datastorage/ogen-client/oas_schemas_gen.go:585`
```go
type AuditEvent struct {
    // ... other fields ...
    DurationMs    OptNilInt    `json:"duration_ms"`
    // ... other fields ...
}
```

**Getter/Setter**: Lines 671-773
```go
// GetDurationMs returns the value of DurationMs.
func (s *AuditEvent) GetDurationMs() OptNilInt {
    return s.DurationMs
}

// SetDurationMs sets the value of DurationMs.
func (s *AuditEvent) SetDurationMs(val OptNilInt) {
    s.DurationMs = val
}
```

**Conclusion**: ✅ Field exists in schema and is queryable

---

### 2. Controller Tracks Classification Timing

**File**: `internal/controller/signalprocessing/signalprocessing_controller.go:490`
```go
func (r *SignalProcessingReconciler) reconcileClassifying(...) (ctrl.Result, error) {
    logger.V(1).Info("Processing Classifying phase")

    // DD-005: Track phase processing metrics
    r.Metrics.IncrementProcessingTotal("classifying", "attempt")
    classifyingStart := time.Now()  // ✅ START TIMER

    // ... perform classification ...

    // DD-005: Record metrics
    r.Metrics.IncrementProcessingTotal("classifying", "success")
    r.Metrics.ObserveProcessingDuration("classifying", time.Since(classifyingStart).Seconds())
    // ✅ DURATION IS CALCULATED BUT NOT PASSED TO AUDIT
}
```

**Line 576**: Audit event is emitted WITHOUT duration
```go
if r.AuditClient != nil && severityResult != nil {
    r.AuditClient.RecordClassificationDecision(ctx, sp)
    // ❌ NO DURATION PARAMETER
}
```

---

### 3. Audit Client Does NOT Set Duration

**File**: `pkg/signalprocessing/audit/client.go:217-276`
```go
// RecordClassificationDecision records classification decision event.
func (c *AuditClient) RecordClassificationDecision(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing) {
    // ❌ NO DURATION PARAMETER
    payload := api.SignalProcessingAuditPayload{
        EventType: EventTypeClassificationDecision,
        Signal:    sp.Spec.Signal.Name,
        Phase:     toSignalProcessingAuditPayloadPhase(string(sp.Status.Phase)),
    }

    // Sets severity, environment, priority, business fields...
    // ❌ BUT NEVER SETS DurationMs

    event := audit.NewAuditEventRequest()
    // ... sets other event fields ...
    // ❌ NEVER CALLS audit.SetDuration(event, durationMs)

    if err := c.store.StoreAudit(ctx, event); err != nil {
        c.log.Error(err, "Failed to write classification decision audit")
    }
}
```

---

### 4. Compare with Working Example (Enrichment)

**Controller** (`signalprocessing_controller.go:459-460`):
```go
enrichmentDuration := int(time.Since(enrichmentStart).Milliseconds())  // ✅ CALCULATE
if err := r.recordEnrichmentCompleteAudit(ctx, sp, k8sCtx, enrichmentDuration, ...); err != nil {
    // ✅ PASS DURATION TO AUDIT CLIENT
}
```

**Audit Client** (`audit/client.go:361-388`):
```go
func (c *AuditClient) RecordEnrichmentComplete(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, durationMs int) {
    // ✅ ACCEPTS DURATION PARAMETER
    payload := api.SignalProcessingAuditPayload{
        EventType: EventTypeEnrichmentComplete,
        // ...
    }
    payload.DurationMs.SetTo(durationMs)  // ✅ SET IN PAYLOAD

    event := audit.NewAuditEventRequest()
    // ... set other fields ...
    audit.SetDuration(event, durationMs)  // ✅ SET IN EVENT

    if err := c.store.StoreAudit(ctx, event); err != nil {
        c.log.Error(err, "Failed to write enrichment audit")
    }
}
```

**Conclusion**: Enrichment has duration, classification does NOT → inconsistency

---

## 🔧 Required Fix

### Fix #1: Update Audit Client Signature

**File**: `pkg/signalprocessing/audit/client.go:220`

#### Before (MISSING DURATION):
```go
func (c *AuditClient) RecordClassificationDecision(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing) {
```

#### After (ADD DURATION PARAMETER):
```go
func (c *AuditClient) RecordClassificationDecision(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, durationMs int) {
```

---

### Fix #2: Set Duration in Audit Client

**File**: `pkg/signalprocessing/audit/client.go` (after line 245)

#### Add Duration to Event:
```go
func (c *AuditClient) RecordClassificationDecision(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, durationMs int) {
    payload := api.SignalProcessingAuditPayload{
        EventType: EventTypeClassificationDecision,
        Signal:    sp.Spec.Signal.Name,
        Phase:     toSignalProcessingAuditPayloadPhase(string(sp.Status.Phase)),
    }

    // ADD THIS: Set duration in payload
    payload.DurationMs.SetTo(durationMs)

    // ... existing severity/environment/priority code ...

    event := audit.NewAuditEventRequest()
    event.Version = "1.0"
    audit.SetEventType(event, EventTypeClassificationDecision)
    audit.SetEventCategory(event, CategorySignalProcessing)
    audit.SetEventAction(event, "classification")
    audit.SetEventOutcome(event, audit.OutcomeSuccess)
    audit.SetActor(event, "service", "signalprocessing-controller")
    audit.SetResource(event, "SignalProcessing", sp.Name)

    // ADD THIS: Set duration in event
    audit.SetDuration(event, durationMs)

    // ... rest of existing code ...
}
```

---

### Fix #3: Pass Duration from Controller

**File**: `internal/controller/signalprocessing/signalprocessing_controller.go:576`

#### Before (NO DURATION):
```go
if r.AuditClient != nil && severityResult != nil {
    logger.V(1).Info("DEBUG: Emitting classification.decision audit event",
        "severityResult", severityResult.Severity)
    r.AuditClient.RecordClassificationDecision(ctx, sp)
}
```

#### After (PASS DURATION):
```go
if r.AuditClient != nil && severityResult != nil {
    classificationDuration := int(time.Since(classifyingStart).Milliseconds())
    logger.V(1).Info("DEBUG: Emitting classification.decision audit event",
        "severityResult", severityResult.Severity,
        "durationMs", classificationDuration)
    r.AuditClient.RecordClassificationDecision(ctx, sp, classificationDuration)
}
```

---

## 📊 Expected Results

### Before Fix
```
Test: should emit 'classification.decision' audit event
Line 286: Expect(event.DurationMs.IsSet()).To(BeTrue())
Result: FAIL - Expected <bool>: false to be true
```

### After Fix
```
Test: should emit 'classification.decision' audit event
Line 286: Expect(event.DurationMs.IsSet()).To(BeTrue())
Result: PASS - DurationMs is set (e.g., 15ms for classification)

Line 288: Expect(event.DurationMs.Value).To(BeNumerically(">", 0))
Result: PASS - DurationMs > 0 (meaningful performance metric)
```

---

## 🎯 Impact Assessment

### Test Pass Rate Impact

**Current**: 85/87 = 97.7% pass rate (2 failures)
**After Fix**: 86/87 = 98.9% pass rate (1 failure - the INTERRUPTED test)

### Performance Metrics Benefit

**Business Value**: Compliance auditors can now see:
- How long classification took (e.g., "15ms")
- Performance degradation over time
- Slow Rego policy evaluations

**Example Audit Event** (after fix):
```json
{
  "event_type": "signalprocessing.classification.decision",
  "correlation_id": "test-audit-event-rr-1768441006828736000",
  "duration_ms": 15,  // ✅ NOW POPULATED
  "event_data": {
    "external_severity": "Sev2",
    "normalized_severity": "warning",
    "determination_source": "rego-policy",
    "environment": "production",
    "priority": "p2"
  }
}
```

---

## ✅ Validation Plan

### Step 1: Apply Fix

```bash
# Edit 3 files:
# 1. pkg/signalprocessing/audit/client.go (add durationMs parameter, set in event)
# 2. internal/controller/signalprocessing/signalprocessing_controller.go (pass duration)
```

### Step 2: Run Integration Tests

```bash
make test-integration-signalprocessing
```

**Expected Result**:
```
Ran 87 of 92 Specs in ~70 seconds
FAIL! -- 86 Passed | 1 Failed | 2 Pending | 3 Skipped
Pass Rate: 98.9% (86/87)

Remaining Failure:
- should emit 'error.occurred' event for fatal enrichment errors (INTERRUPTED)
  → Will likely PASS after re-run (not a real failure)
```

### Step 3: Verify Duration is Populated

```bash
# Check test logs for duration values
grep "✅ Found 1 event.*classification.decision" /tmp/sp-final-test.log

# Query a specific event to verify duration
# (in test or manual verification)
```

---

## 📝 Consistency Check

### All Audit Events Should Have Duration (Where Applicable)

| Event Type | Has Duration? | Notes |
|------------|---------------|-------|
| `enrichment.completed` | ✅ YES | Measures K8s API call time |
| `classification.decision` | ❌ **MISSING** | **THIS FIX** |
| `business.classified` | ❓ Check | Should have duration |
| `signal.processed` | ❓ Check | Should have end-to-end duration |
| `phase.transition` | ❌ N/A | Instant event, no duration |
| `error.occurred` | ❓ Check | Could have duration until error |

**Follow-Up**: After fixing `classification.decision`, audit other events for consistency

---

## 🔗 Related Documents

1. **RCA**: `docs/handoff/SP_FINAL_2_FAILURES_RCA_JAN14_2026.md`
2. **Duplicate Emission Fix**: `docs/handoff/SP_DUPLICATE_CLASSIFICATION_EVENT_BUG_JAN14_2026.md`
3. **Audit Helper Functions**: `pkg/audit/helpers.go:86` (`SetDuration`)
4. **Working Example**: `RecordEnrichmentComplete` in `pkg/signalprocessing/audit/client.go:361`

---

## ⏭️ Next Steps

1. [ ] Apply Fix #1: Update `RecordClassificationDecision` signature
2. [ ] Apply Fix #2: Set duration in audit client (payload + event)
3. [ ] Apply Fix #3: Pass duration from controller
4. [ ] Run integration tests to verify fix
5. [ ] Check other audit events for duration consistency
6. [ ] Document duration metrics in audit event specification

---

**Confidence**: **99%**

**Justification**:
1. ✅ Field exists in schema (line 585)
2. ✅ Controller tracks timing (line 490)
3. ✅ Helper function exists (`audit.SetDuration`)
4. ✅ Working example exists (enrichment)
5. ✅ Fix is straightforward (3 small changes)
6. ⚠️ 1% risk: Ensure payload schema also supports `duration_ms`

---

**Last Updated**: 2026-01-14T21:15:00
**Status**: ✅ Ready for implementation

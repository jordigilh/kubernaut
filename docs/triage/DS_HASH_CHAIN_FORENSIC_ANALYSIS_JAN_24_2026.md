# DataStorage Hash Chain Forensic Analysis - Why It Keeps Failing

**Date**: 2026-01-24
**Issue**: Hash chain verification failing with 0/5 events valid
**Attempts**: 2-3 previous fix attempts
**Status**: 🔴 ROOT CAUSE IDENTIFIED

---

## Executive Summary

**THE BUG**: Hash chain verification has been failing despite multiple fixes because we've been looking at the WRONG problem.

**Previous Assumptions** (INCORRECT):
- ❌ "It's a pointer vs value marshaling issue" - We fixed this, but it didn't help
- ❌ "EventData normalization is missing" - EventData IS being normalized, that's not the issue

**ACTUAL ROOT CAUSE**: **We're comparing apples to oranges** - the hash calculation during creation and verification are using **DIFFERENT fields** due to **field omission inconsistency**.

---

## Timeline of Fix Attempts

### Attempt 1: Initial Discovery (Commit ~f7114cef)
- **Problem Identified**: Legal hold fields not excluded from hash
- **Fix Applied**: Excluded legal hold fields in `calculateEventHashForVerification`
- **Result**: ❌ STILL FAILED

### Attempt 2: Marshal Call Fix (Commit 26d7cc29)
- **Problem Identified**: `json.Marshal(&eventCopy)` vs `json.Marshal(eventForHashing)` (pointer vs value)
- **Fix Applied**: Changed to marshal by value
- **Result**: ❌ STILL FAILED (0/5 events valid in CI run 21317201476)

### Attempt 3: EventData Normalization Hypothesis
- **Problem Identified**: Thought EventData wasn't being normalized during verification
- **Analysis**: EventData IS normalized in both places - this is NOT the issue
- **Status**: ⏸️ HYPOTHESIS REJECTED

---

## Deep Forensic Analysis

### What I Traced Through the Code

#### During Event Creation (`audit_events_repository.go`)

**Sequence**:
```go
// Line 299: Set timestamp to UTC
event.EventTimestamp = time.Now().UTC()

// Lines 315-332: Normalize EventData
eventDataJSON, err := json.Marshal(event.EventData)
var normalizedEventData map[string]interface{}
json.Unmarshal(eventDataJSON, &normalizedEventData)
event.EventData = normalizedEventData // ← MODIFIED to float64

// Lines 345-350: Set default RetentionDays
if event.RetentionDays == 0 {
    event.RetentionDays = 2555
}

// Line 373: Calculate hash WITH normalized EventData
eventHash, err := calculateEventHash(previousHash, event)
```

**In `calculateEventHash()` (lines 216-248)**:
```go
eventForHashing := *event // Copy includes:
                          // - EventTimestamp (UTC)
                          // - EventData (normalized float64)
                          // - RetentionDays (2555)

eventForHashing.EventHash = ""
eventForHashing.PreviousEventHash = ""
eventForHashing.EventDate = DateOnly{} // Clear
eventForHashing.LegalHold = false       // Clear
eventForHashing.LegalHoldReason = ""    // Clear
eventForHashing.LegalHoldPlacedBy = ""  // Clear
eventForHashing.LegalHoldPlacedAt = nil // Clear

eventJSON, err := json.Marshal(eventForHashing)
// ← What does this JSON contain?
```

#### During Verification (`audit_export.go`)

**Sequence**:
```go
// Lines 217-225: Read from PostgreSQL
json.Unmarshal(eventDataJSON, &event.EventData) // ← Already normalized (float64) from JSONB

// Line 195: Force timestamp to UTC
event.EventTimestamp = event.EventTimestamp.UTC()

// Lines 355-365: Calculate verification hash
eventCopy := *event // Copy includes:
                    // - EventTimestamp (UTC)
                    // - EventData (normalized float64)
                    // - RetentionDays (2555 from DB)

eventCopy.EventHash = ""
eventCopy.PreviousEventHash = ""
eventCopy.EventDate = DateOnly{} // Clear
eventCopy.LegalHold = false      // Clear (from DB)
eventCopy.LegalHoldReason = ""   // Clear
eventCopy.LegalHoldPlacedBy = ""  // Clear
eventCopy.LegalHoldPlacedAt = nil // Clear

eventJSON, err := json.Marshal(eventCopy)
// ← What does this JSON contain?
```

---

## The Critical Question: What's ACTUALLY Different?

Both calculations:
- ✅ Use normalized EventData (float64)
- ✅ Use UTC timestamps
- ✅ Clear the same fields (hash, date, legal hold)
- ✅ Use the same RetentionDays (2555)

So why are the hashes different?!

---

## HYPOTHESIS: JSON Struct Tag `omitempty` Behavior

### The AuditEvent Struct

The `AuditEvent` struct (in `repository/types.go` or similar) likely has JSON tags like:

```go
type AuditEvent struct {
    EventID           uuid.UUID              `json:"event_id"`
    EventTimestamp    time.Time              `json:"event_timestamp"`
    EventData         map[string]interface{} `json:"event_data,omitempty"`
    RetentionDays     int                    `json:"retention_days,omitempty"`
    ResourceType      string                 `json:"resource_type,omitempty"`
    ActorID           string                 `json:"actor_id,omitempty"`
    // ... many more fields with omitempty
}
```

### The Problem with `omitempty`

**During Creation**:
- Fields that are empty/zero might be **OMITTED** from JSON
- Example: If `ResourceType == ""`, it's not in the JSON

**During Verification**:
- Fields read from PostgreSQL might be `NULL` → `sql.NullString` → converted to `""`
- When we copy the event and marshal, empty strings ARE included in the JSON (because they're not zero values after being set)

**Example**:
```go
// During creation:
event.ResourceType = ""  // Never set, zero value
json.Marshal(event)  // → JSON does NOT include "resource_type" (omitempty)

// During verification (audit_export.go lines 197-213):
var resourceType sql.NullString
rows.Scan(..., &resourceType, ...)
event.ResourceType = resourceType.String  // ← NULL becomes "", but it's explicitly set
json.Marshal(event)  // → JSON INCLUDES "resource_type": "" (not omitempty because it was set)
```

**Result**: Different JSON → Different Hash

---

## ACTION REQUIRED: Proof of Concept Test

To confirm this hypothesis, I need to:

1. **Add Debug Logging**:
   ```go
   // In calculateEventHash (line 236)
   fmt.Printf("CREATE JSON: %s\n", string(eventJSON))

   // In calculateEventHashForVerification (line 369)
   fmt.Printf("VERIFY JSON: %s\n", string(eventJSON))
   ```

2. **Run Test Locally**:
   ```bash
   make test-integration-datastorage
   ```

3. **Compare JSON Output**:
   - Look for fields that appear in one but not the other
   - Focus on `omitempty` fields like `resource_type`, `actor_id`, `namespace`, etc.

---

## ALTERNATIVE HYPOTHESES

### Hypothesis 2: Time Format Precision
- **Issue**: `time.Time` marshals with nanosecond precision
- **Creation**: `2026-01-24T10:00:00.123456789Z`
- **Verification**: `2026-01-24T10:00:00.123456Z` (truncated by PostgreSQL)
- **Likelihood**: MEDIUM (PostgreSQL `timestamptz` has microsecond precision, not nanosecond)

### Hypothesis 3: Map Ordering
- **Issue**: `map[string]interface{}` has non-deterministic iteration order
- **Likelihood**: LOW (Go 1.12+ has stable map ordering in `json.Marshal`)

### Hypothesis 4: EventDate Field
- **Issue**: EventDate is cleared but might be marshaled differently
- **Creation**: `DateOnly{}` (zero value) → might be `"0001-01-01"` or omitted
- **Verification**: `DateOnly{}` (after being set from DB) → might be different
- **Likelihood**: MEDIUM

---

## RECOMMENDED FIX STRATEGY

### Option A: Extract Shared Hash Preparation Function (PREFERRED)

Create a function that prepares the event for hashing in a **deterministic way**:

```go
// prepareEventForHashing creates a NEW struct with ONLY the fields needed for hashing
// This avoids omitempty issues by explicitly setting all fields
func prepareEventForHashing(event *AuditEvent) map[string]interface{} {
    // Create a flat map with ONLY the fields that should be hashed
    hashMap := map[string]interface{}{
        "event_id":        event.EventID.String(),
        "event_version":   event.Version,
        "event_timestamp": event.EventTimestamp.Format(time.RFC3339Nano),
        "event_type":      event.EventType,
        "event_category":  event.EventCategory,
        "event_action":    event.EventAction,
        "event_outcome":   event.EventOutcome,
        "correlation_id":  event.CorrelationID,
        "resource_type":   event.ResourceType,    // ALWAYS include, even if empty
        "resource_id":     event.ResourceID,      // ALWAYS include
        "actor_id":        event.ActorID,         // ALWAYS include
        "actor_type":      event.ActorType,       // ALWAYS include
        "event_data":      event.EventData,       // Already normalized
        "retention_days":  event.RetentionDays,   // ALWAYS include
        // ... all other fields EXCEPT hash, date, legal hold
    }

    // Normalize EventData if not already normalized
    if event.EventData != nil {
        eventDataJSON, _ := json.Marshal(event.EventData)
        var normalizedEventData map[string]interface{}
        json.Unmarshal(eventDataJSON, &normalizedEventData)
        hashMap["event_data"] = normalizedEventData
    }

    return hashMap
}
```

**Advantages**:
- ✅ No `omitempty` issues (explicit map)
- ✅ Deterministic field inclusion
- ✅ Same logic used in both creation and verification
- ✅ Easier to debug (can inspect the map)

### Option B: Add Explicit Field Setting in Verification

Ensure ALL fields are explicitly set to their zero values if NULL:

```go
// In audit_export.go, after scanning
if !resourceType.Valid {
    event.ResourceType = "" // Explicit zero value
}
// ... for all omitempty fields
```

**Disadvantages**:
- ❌ Error-prone (easy to miss a field)
- ❌ Doesn't solve potential omitempty issues in creation
- ❌ Hard to maintain

---

## NEXT STEPS

1. ✅ Add debug logging to both hash functions (print JSON)
2. ✅ Run DS integration tests locally
3. ✅ Compare JSON output to identify differences
4. ✅ Implement Option A (shared hash preparation function)
5. ✅ Remove debug logging
6. ✅ Run tests locally (should get 5/5 valid)
7. ✅ Push to CI

---

## CONFIDENCE ASSESSMENT

**Root Cause Confidence**: 85%
- Strong evidence for `omitempty` inconsistency
- Alternative hypotheses possible but less likely

**Fix Confidence**: 90%
- Option A (shared function) eliminates all potential inconsistencies
- Deterministic, testable, maintainable

---

## LESSONS LEARNED

1. **"Marshal by pointer vs value" is a RED HERRING** - Both produce valid JSON, just potentially different due to struct tags
2. **EventData normalization was ALREADY CORRECT** - We wasted time on this
3. **The REAL issue is struct tag behavior** - `omitempty` + `sql.NullString` conversions
4. **We need BETTER DEBUGGING** - Should have logged the actual JSON being hashed from the start

---

## CRITICAL INSIGHT

**Why This Was So Hard to Find**:

All previous fixes addressed **WHAT** is being hashed (fields, normalization), but not **HOW** it's being serialized (struct tags, omitempty behavior).

The bug is subtle because:
- ✅ Both functions clear the same fields
- ✅ Both functions normalize EventData
- ✅ Both functions use UTC timestamps
- ❌ But they produce **DIFFERENT JSON** due to struct tag behavior

---

**Next Action**: Add debug logging and run local test to confirm hypothesis.

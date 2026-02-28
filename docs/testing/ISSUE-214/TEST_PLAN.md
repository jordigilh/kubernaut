# Test Plan: Issue #214 -- Consecutive Ineffective Remediation Detection

**Authority**: BR-ORCH-042, DD-RO-002-ADDENDUM  
**Service**: Remediation Orchestrator (RO)  
**Component**: `pkg/remediationorchestrator/routing` (RoutingEngine)  
**Date**: 2026-02-28  
**Status**: Draft

---

## Overview

Issue #214 adds a three-layer detection algorithm to `CheckPostAnalysisConditions`
that detects consecutive ineffective remediations using DataStorage audit traces.
When the chain length exceeds the configured threshold, the RR is blocked with
`BlockReasonIneffectiveChain` and escalated to human review via `NotificationRequest`.

### Detection Layers

1. **Hash chain match** (Layer 1): Walk `Tier1.Chain` entries backwards. If consecutive
   entries have `PostRemediationSpecHash == preRemediationSpecHash` of the current RR,
   the remediation had no lasting effect (resource spec reverted).

2. **Spec drift / regression** (Layer 2): If `HashMatch == "preRemediation"`, the
   current resource spec matches the pre-remediation state of a previous entry,
   indicating the resource was reverted by an external actor.

3. **Safety net** (Layer 3): Count total DS entries for this target within the
   `IneffectiveTimeWindow`. If count >= `RecurrenceCountThreshold`, escalate even
   without conclusive hash data (e.g., hash fields missing from older entries).

### Configuration

| Field                       | Default | Description                                      |
|-----------------------------|---------|--------------------------------------------------|
| IneffectiveChainThreshold   | 3       | Consecutive ineffective entries to trigger block  |
| RecurrenceCountThreshold    | 5       | Total entries in window for safety-net trigger    |
| IneffectiveTimeWindow       | 4h      | Lookback window for both chain and safety-net     |

### Error Handling

- DataStorage query errors: **fail-open** (log error, return nil, do not block)
- `CapturePreRemediationHash` error (`hashErr != nil`): **terminal** (RR -> Failed)
- `CapturePreRemediationHash` empty hash (no error): skip hash-based checks, proceed

---

## Unit Tests (RO Routing Engine -- mock DS client)

Test file: `test/unit/remediationorchestrator/routing/blocking_test.go`

### Layer 1+2: Hash chain + spec_drift (DS audit traces)

| ID            | Description                                                            | Expected Result                  |
|---------------|------------------------------------------------------------------------|----------------------------------|
| UT-RO-214-001 | Hash chain matches across 3 consecutive entries within 4h window       | `BlockReasonIneffectiveChain`    |
| UT-RO-214-002 | `HashMatch == "preRemediation"` (regression) for 3 consecutive entries | `BlockReasonIneffectiveChain`    |
| UT-RO-214-003 | Entry without regression flag, hash chain breaks                       | `nil` (chain broken)             |
| UT-RO-214-004 | DS entry missing pre/post hash data                                    | `nil` (insufficient data)        |
| UT-RO-214-005 | 2 ineffective entries, threshold = 3                                   | `nil` (below threshold)          |

### Layer 3: Safety net (count + time window)

| ID            | Description                                               | Expected Result                  |
|---------------|-----------------------------------------------------------|----------------------------------|
| UT-RO-214-006 | 5 entries within 4h, no hash data                         | `BlockReasonIneffectiveChain`    |
| UT-RO-214-007 | 5 entries but all completed >4h ago                       | `nil` (outside window)           |

### Cross-layer

| ID            | Description                                                              | Expected Result                  |
|---------------|--------------------------------------------------------------------------|----------------------------------|
| UT-RO-214-008 | Pre-analysis `CheckConsecutiveFailures` unchanged (regression guard)      | Existing behavior preserved      |
| UT-RO-214-009 | Mixed: Failed + ineffective Completed in same fingerprint chain          | Both checks coexist correctly    |

### Additional: CapturePreRemediationHash failure

| ID            | Description                                                  | Expected Result           |
|---------------|--------------------------------------------------------------|---------------------------|
| UT-RO-214-010 | `CapturePreRemediationHash` returns `hashErr != nil`         | RR transitions to Failed  |

---

## Acceptance Criteria

1. `CheckIneffectiveRemediationChain` is called LAST in `CheckPostAnalysisConditions` (after `CheckExponentialBackoff`)
2. DS query failures are fail-open (log + nil return)
3. `handleBlocked` creates `NotificationRequest` BEFORE status update for `BlockReasonIneffectiveChain`
4. Status update sets `Outcome = "ManualReviewRequired"` and `RequiresManualReview = true` in a single API call
5. `RequeueAfter` = `IneffectiveTimeWindow` for `BlockReasonIneffectiveChain` blocks
6. `CapturePreRemediationHash` error (`hashErr != nil`) is terminal (RR -> Failed)
7. Empty `preRemediationSpecHash` (no error) skips hash-based checks but allows RR to proceed
8. All 9 existing `CheckConsecutiveFailures` / routing tests continue to pass (regression)

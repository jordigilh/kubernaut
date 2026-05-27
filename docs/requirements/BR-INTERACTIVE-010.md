# BR-INTERACTIVE-010: IS CRD as Universal Interactive Signal

**Business Requirement ID**: BR-INTERACTIVE-010
**Priority**: P0
**Status**: Implementation Complete
**Target Version**: v1.5
**Date**: May 26, 2026
**Related BRs**: BR-INTERACTIVE-001, BR-INTERACTIVE-004, BR-INTERACTIVE-008
**Related DDs**: DD-INTERACTIVE-002 (Dynamic Takeover Model)
**GitHub Issue**: [#1293](https://github.com/jordigilh/kubernaut/issues/1293)

---

## Business Need

The platform requires a single, unified mechanism to signal interactive investigation intent across all pipeline components (AF, RO, AA, KA). The signal must:
- Be detectable by AA before submitting to KA (pre-emptive interactive mode)
- Be detectable by AA during a running investigation (dynamic takeover)
- ~~Allow RO to bypass cooldown blocking for human-initiated re-investigations~~ (CANCELLED — AF inherently bypasses RO cooldown)
- Prevent ServiceAccount abuse of cooldown bypass
- Support signal withdrawal (cancellation via IS deletion)

The InvestigationSession CRD already exists and links to the RR via `spec.remediationRequestRef`. Its existence becomes the universal interactive signal — no schema changes to RR or AIAnalysis CRDs are required.

---

## Success Criteria

### SC-1: IS CRD as Interactive Signal (AA Detection)

1. AA registers a field index on `spec.remediationRequestRef.name` for InvestigationSession CRDs
2. Before submitting to KA, AA queries IS by field selector — if an Active IS exists for the RR, `interactive=true` is set on the IncidentRequest
3. AA watches InvestigationSession CRDs; creation of a new IS for an RR with an active KA session triggers cancel + re-submit with `interactive=true`
4. AA watches InvestigationSession CRDs; deletion of an IS for an RR with an active KA session triggers cancel + AIAnalysis transition to `PhaseFailed` with `ReasonInteractiveCancelled`

### SC-2: KA Interactive Session Lifecycle

1. KA's `IncidentRequest` schema includes an optional `interactive` boolean field
2. When `interactive=true`, KA creates the session in `pending` state without launching the `Investigate()` goroutine
3. KA returns 202 + `session_id` for interactive submissions
4. KA MCP `action=start` detects a pending (awaiting) session, triggers context reconstruction from DS audit trail, and launches `Investigate()`
5. After RCA completes with `signal.Interactive=true`, KA skips Phase 2 (re-enrichment) and Phase 3 (workflow selection), returning `InteractiveHold=true`
6. Session manager sets `StatusUserDriving` when result has `InteractiveHold=true`

### SC-3: Context Reconstruction from Audit Trail

1. On interactive submit, KA queries DS for the last session's audit events by `remediation_id` (using `correlation_id` parameter)
2. If RCA result is available from the prior session, it is used as summary context
3. If RCA is incomplete, KA reconstructs from whatever audit traces exist (chained sessions — latest contains all prior context)
4. Reconstructed messages are fed as initial prompt to the LLM
5. No external payload carries prior context (prevents prompt hijacking — KA retrieves from trusted DS only)

### SC-4: RO Cooldown Bypass for IS-Backed RRs — CANCELLED

**Status**: CANCELLED BY DESIGN

**Rationale**: AF inherently bypasses RO cooldown by design. When a human creates an
InvestigationSession via AF, the resulting RR creation path does not pass through
RO's cooldown/backoff logic. The cooldown is an RO-internal mechanism for autonomous
re-investigations; interactive investigations initiated via AF operate on a separate
path that is not subject to cooldown. No RO changes are required.

~~1. RO registers a field index on `spec.remediationRequestRef.name` for InvestigationSession CRDs~~
~~2. In cooldown check logic (`CheckConsecutiveFailures`, `CheckExponentialBackoff`), RO queries IS for the RR~~
~~3. If an Active IS exists, cooldown is bypassed (investigation proceeds)~~
~~4. Dedup (`CheckDuplicateInProgress`) remains unaffected — two concurrent active investigations for the same alert are still prevented~~
~~5. This enables "resume after terminal" scenarios: user creates new RR + IS for a completed/failed alert~~

### SC-5: Human-Only IS Creation

1. AF's `UserIdentity` includes an `IsServiceAccount` boolean, set during TokenReview for SA tokens
2. `MaterializeCRD` rejects IS creation when the caller is a ServiceAccount
3. The single-driver guard in `MaterializeCRD` is migrated from label-based lookup to field selector (`spec.remediationRequestRef.name`)

### SC-6: MCP discover_workflows Phase 2 Enrichment Fix

1. MCP `discover_workflows` performs Phase 2 re-enrichment (`ResolveEnrichmentTarget` + `enricher.Enrich`) before Phase 3 workflow selection
2. This ensures consistent workflow recommendations between autonomous and interactive paths

### SC-7: AA Poll Status Handling

1. AA's investigating handler explicitly handles `"cancelled"` poll status (currently falls to `default` → treated as pending)
2. On `"cancelled"` with IS still Active: re-submit with `interactive=true` (session was cancelled for takeover)
3. On `"cancelled"` with IS deleted: transition AIAnalysis to `PhaseFailed` with `ReasonInteractiveCancelled`

### SC-8: AF Interactive Flow Orchestration

1. AF prompt enforces sequential tool usage: `af_create_rr` THEN wait for KA session THEN connect
2. AF watches `AIAnalysis.status.kaSession.id` to detect KA session readiness
3. AF connects to KA via MCP `action=start` once session exists
4. AF readiness tool self-timeouts (5-10 min) since no ADK framework timeout exists
5. Human chat defaults to interactive mode; SA callers default to autonomous

---

## Edge Cases

| Case | Behavior |
|------|----------|
| Pending session abandoned (MCP start never arrives) | AA's 25m cap on `pending` status fails the investigation |
| IS deleted during active investigation | AA cancels KA session, AIAnalysis → `PhaseFailed` + `ReasonInteractiveCancelled` |
| IS created mid-flight (dynamic takeover) | AA cancels existing session, re-submits with `interactive=true` |
| IS for terminal RR (resume) | New RR + IS created; AF path bypasses RO cooldown by design |
| Multiple IS for same RR by same user | Allowed (reconnection); different user blocked (single-driver guard) |
| SA attempts IS creation | Rejected by AF's `MaterializeCRD` |
| RR cancelled while IS active | Pre-existing gap: KA session continues until GC (follow-up) |
| Chained sessions (multiple prior sessions) | Latest session used for reconstruction (contains all prior context) |

---

## Out of Scope (v1.6+)

- On IS deletion, restart investigation in autonomous mode with a clean slate (no prior context — previous data may be tainted)
- `maxUserDrivingDuration` for AA to cap indefinite user_driving polling
- RR cancellation propagation to KA session
- Multi-replica session affinity for KA

---

## Traceability

| SC | Component | Test Tier | Test IDs |
|----|-----------|-----------|----------|
| SC-1 | AA | Unit + Integration | UT-AA-1293-001..008, IT-AA-1293-001..004 |
| SC-2 | KA | Unit | UT-KA-1293-001..007, UT-KA-1293-012 |
| SC-3 | KA | Unit | UT-KA-1293-008..010 |
| SC-4 | ~~RO~~ | ~~CANCELLED~~ | CANCELLED BY DESIGN — AF bypasses RO cooldown |
| SC-5 | AF | Unit | UT-AF-1293-001..005 |
| SC-6 | KA | Unit | UT-KA-1293-011 |
| SC-7 | AA | Unit | UT-AA-1293-004..005 |
| SC-8 | AF | Unit + E2E | E2E-1293-001..006 |

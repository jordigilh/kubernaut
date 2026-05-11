# Test Plan: Shadow Agent Alignment Fixes (#1076, #1077, #1078, C-1)

**Version**: 1.0
**Created**: 2026-05-10
**Status**: Active
**Service**: Kubernaut Agent (KA) / AI Analysis (AA) / Remediation Orchestrator (RO)
**Service Type**: [x] CRD Controller | [x] Stateless HTTP API
**Issues**: #1076, #1077, #1078, C-1
**Business Requirements**: BR-AI-601.2, BR-SAFETY-1076
**Compliance**: SOC2 CC7.4 (audit trail), NIST AU-2/AU-3 (audit events), NIST SI-4 (system monitoring)

---

## 1. Scope

This test plan covers four related fixes to the shadow agent alignment subsystem:

| Fix | Issue | Description | Risk |
|-----|-------|-------------|------|
| 1 | #1077 | VerdictClean "clean" -> "aligned" to match OpenAPI spec | Low |
| 2 | #1078 (H-1) | Session manager panic recovery | Medium |
| 3 | #1078 (H-3) | Two-tier TTL cleanup with MaxSessionAge | Medium |
| 4 | #1078 (H-2) | AA-side investigation timeout | Medium |
| 5 | C-1 | LLMProxy bypass when Swappable pins client | High |
| 6 | #1076 | Circuit breaker: cancel investigation on suspicious step | High |
| 7 | -- | AlignmentVerdict schema extension (KA OpenAPI, AA CRD) | Medium |
| 8 | -- | RO notification alignment verdict rendering | Medium |

---

## 2. Test Scenario Naming Convention

**Format**: `{TIER}-{SERVICE}-{ISSUE}-{SEQUENCE}`

- `UT-SA-1077-*` -- verdict enum fix (shadow agent)
- `UT-KA-1078-*` -- session resilience fixes (KA session manager)
- `UT-KA-1078-TTL-*` -- two-tier TTL eviction
- `UT-AA-1078-TOUT-*` -- AA-side investigation timeout
- `UT-SA-C1-*` -- LLMProxy bypass fix
- `IT-SA-C1-*` -- integration: LLM reasoning reaches observer
- `UT-SA-1076-*` -- circuit breaker
- `UT-SA-SCHEMA-*` -- alignment verdict schema (wrapper)
- `UT-KA-SCHEMA-*` -- alignment verdict schema (KA handler)
- `UT-AA-SCHEMA-*` -- alignment verdict schema (AA response processor)
- `UT-RO-NOTIF-*` -- RO notification rendering

---

## 3. Features Not to be Tested

- **E2E tests for circuit breaker**: The mock-LLM service does not yet support mid-investigation interception. E2E tests are deferred to a follow-up when mock-LLM supports circuit breaker scenarios.
- **Formal injection benchmarking**: (#602, v1.5) Curated attack datasets and scoring.
- **Grafana dashboard updates**: Deferred; dashboards will be updated in a separate operational PR.

---

## 4. Test Scenarios

### 4.1 Fix 1: Verdict Enum Alignment (#1077)

| ID | Description | Type | Expected Result |
|----|-------------|------|-----------------|
| UT-SA-1077-001 | VerdictClean constant equals "aligned" | Unit | `VerdictClean == "aligned"` |
| UT-SA-1077-002 | Audit event with "aligned" verdict passes AlignmentVerdictPayload schema validation | Unit | testutil.ValidateAuditEvent succeeds |
| UT-SA-1077-003 | `kubernaut_alignment_verdict_total{result="aligned"}` increments on clean verdict | Unit | Counter increments by 1 |

### 4.2 Fix 2: Session Manager Panic Recovery (#1078 H-1)

| ID | Description | Type | Expected Result |
|----|-------------|------|-----------------|
| UT-KA-1078-001 | Panicking InvestigateFunc transitions session to StatusFailed | Unit | Session.Status == StatusFailed |
| UT-KA-1078-002 | Panic error message preserved in session result error field | Unit | Error contains panic value |
| UT-KA-1078-003 | Panic with nil recover value transitions to StatusFailed | Unit | Session.Status == StatusFailed |

### 4.3 Fix 3: Two-Tier TTL Eviction (#1078 H-3)

| ID | Description | Type | Expected Result |
|----|-------------|------|-----------------|
| UT-KA-1078-TTL-001 | StatusRunning session NOT evicted when CreatedAt + TTL elapsed | Unit | Session exists after cleanup |
| UT-KA-1078-TTL-002 | StatusRunning session IS evicted when CreatedAt + MaxSessionAge elapsed | Unit | Session deleted, warning logged |
| UT-KA-1078-TTL-003 | StatusCompleted session IS evicted at TTL | Unit | Session deleted (existing behavior) |
| UT-KA-1078-TTL-004 | StatusFailed session IS evicted at TTL | Unit | Session deleted (existing behavior) |
| UT-KA-1078-TTL-005 | MaxSessionAge < TTL config rejected | Unit | Validation error returned |

### 4.4 Fix 4: AA-Side Investigation Timeout (#1078 H-2)

| ID | Description | Type | Expected Result |
|----|-------------|------|-----------------|
| UT-AA-1078-TOUT-001 | Timeout exceeded transitions to PhaseFailed with TransientError | Unit | Phase=Failed, Reason=TransientError |
| UT-AA-1078-TOUT-002 | Timeout message includes configured duration | Unit | Message contains "25 minutes" |
| UT-AA-1078-TOUT-003 | Investigation within duration limit continues polling | Unit | No phase transition |
| UT-AA-1078-TOUT-004 | MaxInvestigationDuration==0 uses default 25 minutes | Unit | Default applied |

### 4.5 Fix 5: LLMProxy Bypass (C-1)

| ID | Description | Type | Expected Result |
|----|-------------|------|-----------------|
| UT-SA-C1-001 | PinDecorator set: pinned client passes through decorator | Unit | Decorator function called |
| UT-SA-C1-002 | PinDecorator nil: falls back to NewInstrumentedClient | Unit | Default behavior |
| IT-SA-C1-001 | Investigation with PinDecorator + alignment produces StepKindLLMReasoning observations | Integration | Observer has LLM reasoning observations |

### 4.6 Fix 6: Circuit Breaker (#1076)

| ID | Description | Type | Expected Result |
|----|-------------|------|-----------------|
| UT-SA-1076-001 | Suspicious in enforce mode -> investigation cancelled, HumanReviewNeeded=true | Unit | Context cancelled, result populated |
| UT-SA-1076-002 | In monitor mode, suspicious does NOT cancel investigation | Unit | Investigation completes normally |
| UT-SA-1076-RACE-001 | Investigation completes before suspicious flag; no panic | Unit | Clean completion, no double-cancel |
| UT-SA-1076-CTX-001 | Parent context cancelled (shutdown) -> not treated as circuit break | Unit | Error propagated, no circuit break |
| UT-SA-1076-NIL-001 | Circuit break with nil inner result -> minimal result constructed | Unit | No nil deref, result has fields |
| UT-SA-1076-PARTIAL-001 | RCA completes, circuit breaks during workflow -> partial RCA preserved | Unit | RCA + shadow findings in result |
| UT-SA-1076-METRIC-001 | `kubernaut_alignment_circuit_breaker_total{mode="enforce"}` increments | Unit | Counter increments by 1 |
| UT-SA-1076-AUDIT-001 | Audit event includes `circuit_breaker: true` | Unit | circuit_breaker field present |

### 4.7 Fix 7: Alignment Verdict Schema

| ID | Description | Type | Expected Result |
|----|-------------|------|-----------------|
| UT-SA-SCHEMA-001 | Clean verdict -> AlignmentVerdict.Result="aligned", no findings | Unit | Correct mapping |
| UT-SA-SCHEMA-002 | Suspicious verdict -> AlignmentVerdict with findings | Unit | Findings populated |
| UT-SA-SCHEMA-003 | Circuit breaker -> CircuitBreakerActivated=true | Unit | Field set correctly |
| UT-SA-SCHEMA-004 | AlignmentVerdict populated for ALL investigations | Unit | Always present when alignment enabled |
| UT-KA-SCHEMA-001 | mapInvestigationResultToResponse maps AlignmentVerdict | Unit | Response field populated |
| UT-AA-SCHEMA-001 | AA response processor maps alignment_verdict to CRD | Unit | CRD status field populated |
| UT-AA-SCHEMA-002 | alignment_verdict: null -> no CRD field, no crash | Unit | AlignmentVerdict is nil |

### 4.8 Fix 8: RO Notification Rendering

| ID | Description | Type | Expected Result |
|----|-------------|------|-----------------|
| UT-RO-NOTIF-001 | Circuit breaker -> body contains "SUSPICIOUS (Circuit Breaker Activated)" | Unit | Shadow findings before RCA |
| UT-RO-NOTIF-002 | Aligned verdict -> body contains "ALIGNED" | Unit | Short positive confirmation |
| UT-RO-NOTIF-003 | AlignmentVerdict=nil -> no alignment section | Unit | Backward compatible |
| UT-RO-NOTIF-004 | SubReason="alignment_check_failed" -> NotificationPriorityCritical | Unit | Priority mapped correctly |
| UT-RO-NOTIF-005 | populateManualReviewContext with AlignmentVerdict -> field populated | Unit | reviewCtx.AlignmentVerdict set |
| UT-RO-NOTIF-006 | buildManualReviewContext maps verdict and circuit breaker to ReviewContext | Unit | CRD fields populated |

---

## 5. Concurrency Tests (all run with `-race`)

| ID | Description | Scope |
|----|-------------|-------|
| UT-SA-1076-RACE-001 | Late onSuspicious after investigation completes | Circuit breaker |
| Existing observer_perf_test.go | 10+ goroutines calling SubmitAsync | Observer |
| PR 2 concurrency | 10+ goroutines calling StartInvestigation | Session manager |
| PR 2 race | Panic recovery vs normal completion race | Session manager |

---

## 6. Prometheus Metrics Changes

| Metric | Change | Breaking |
|--------|--------|----------|
| `kubernaut_alignment_verdict_total` | Label `result="clean"` -> `result="aligned"` | Yes (pre-GA RC) |
| `kubernaut_alignment_step_total` | Label `outcome="clean"` -> `outcome="aligned"` | Yes (pre-GA RC) |
| `kubernaut_alignment_circuit_breaker_total` | New counter with `{mode}` label | No |

---

## 7. Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-05-10 | Initial test plan for #1076, #1077, #1078, C-1 |

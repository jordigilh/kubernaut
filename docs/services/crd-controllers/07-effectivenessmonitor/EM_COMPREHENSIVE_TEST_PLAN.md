# Effectiveness Monitor (EM) Comprehensive Test Plan

**Version**: 1.3.0
**Created**: 2026-02-09
**Status**: Draft
**Service Type**: CRD Controller
**Author**: AI Assistant
**Reviewers**: TBD

---

## Executive Summary

This document defines the comprehensive test plan for the Effectiveness Monitor (EM) CRD controller service. The EM watches `EffectivenessAssessment` (EA) CRDs created by the Remediation Orchestrator, performs 4 assessment checks (health, alert, metrics, spec-hash), emits component audit events to DataStorage, and marks the EA CRD status. DataStorage computes the weighted effectiveness score on demand.

### Defense-in-Depth Strategy

Coverage is measured **per tier against the tier-specific code subset** (see `scripts/coverage/coverage_report.py`). Because we follow strict TDD (RED -> GREEN -> REFACTOR), every business requirement must have corresponding tests. The target is **>=80% per tier** to ensure all business logic is implemented and validated.

| Tier | Code Subset | Coverage Target | Focus | Infrastructure | Prometheus/AlertManager |
|------|-------------|-----------------|-------|---------------|------------------------|
| **Unit** | Unit-testable (pure logic) | **>=80%** | Scoring, config, validators, builders | Interface mocks | Go interface mocks |
| **Integration** | Integration-testable (I/O) | **>=80%** | Reconciler, K8s client, DS writes, Prom/AM queries | envtest + DS bootstrap | `httptest.NewServer` mocks |
| **E2E** | Full service | **>=80%** | Full pipeline, real API contract | Kind cluster + real stack | **Real** Prometheus + AlertManager |
| **All Tiers** | Full service (merged) | **>=80%** | Line-by-line dedup across all tiers | — | — |

#### Code Partitioning for EM (per `coverage_report.py`)

**Unit-testable** (`unit_exclude` filters out I/O code):
- `pkg/effectivenessmonitor/config/` -- config parsing, validation, defaults
- `pkg/effectivenessmonitor/health/` -- health sub-scoring algorithm (pod_running, readiness, restarts, crashes, OOM)
- `pkg/effectivenessmonitor/alert/` -- alert scoring logic (resolved/firing/no-alert decisions)
- `pkg/effectivenessmonitor/metrics/` -- metric comparison logic (improvement/no-change/degradation math)
- `pkg/effectivenessmonitor/hash/` -- spec hash computation (deterministic hashing)
- `pkg/effectivenessmonitor/validity/` -- validity window enforcement (deadline computation, expiry checks)
- `pkg/effectivenessmonitor/audit/` -- audit event payload construction (building structs, NOT sending them)
- `pkg/effectivenessmonitor/phase/` -- phase transition logic
- `pkg/effectivenessmonitor/types/` -- type definitions

**Integration-testable** (`int_include` filters to I/O code):
- `pkg/effectivenessmonitor/client/` -- Prometheus HTTP client, AlertManager HTTP client, DS HTTP client
- `pkg/effectivenessmonitor/status/` -- EA CRD status subresource updaters (K8s API writes)
- `pkg/effectivenessmonitor/reconciler/` -- main Reconcile() loop, requeue logic, component orchestration
- `internal/controller/effectivenessmonitor/` -- controller-runtime wiring, manager setup, startup checks

**Prometheus/AlertManager Mocking Exception**: Tier 2 uses `httptest.NewServer` mocks (approved exception per TESTING_GUIDELINES.md v2.6.0 Section 4a). Real Prometheus/AlertManager are used in E2E to catch API contract mismatches (lesson learned from Mock LLM experience).

### Key References

- [ADR-EM-001](../../../architecture/decisions/ADR-EM-001-effectiveness-monitor-service-integration.md) (v1.3) -- EM integration architecture (v1.3: derived timing in status)
- [DD-017](../../../architecture/decisions/DD-017-effectiveness-monitor-v1.1-deferral.md) (v2.3) -- EM v1.1 deferral and Level 1 scope
- [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md) -- Testing strategy and policies
- [V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md](../../development/testing/V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md) -- Test plan template

---

## Test Scenario Naming Convention

**Format**: `{TIER}-EM-{DOMAIN}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration), `E2E` (End-to-End)
- **EM**: Effectiveness Monitor service abbreviation
- **DOMAIN**: Functional domain prefix (see below)
- **SEQUENCE**: Zero-padded 3-digit sequence

### Domain Prefixes

| Prefix | Domain | Description |
|--------|--------|-------------|
| **RC** | Reconciler | Phase transitions, lifecycle management |
| **HC** | Health Check | Pod health assessment (running, ready, restarts, crashes, OOM) |
| **AR** | Alert Resolution | AlertManager queries, alert status scoring |
| **MC** | Metric Comparison | Prometheus PromQL queries, before/after metric delta |
| **SH** | Spec Hash | Pre/post remediation spec hash comparison |
| **VW** | Validity Window | Assessment freshness enforcement, expiry handling |
| **AE** | Audit Events | Component audit event construction and emission |
| **KE** | Kubernetes Events | EventRecorder emissions for phase transitions |
| **CF** | Configuration | Config parsing, validation, defaults |
| **FF** | Fail-Fast | Startup dependency checks |
| **DT** | Derived Timing | First-reconciliation timing computation (ValidityDeadline, check-after times) |
| **OM** | Operational Metrics | Controller-runtime metrics (reconciliation counts, durations) |
| **RR** | Restart Recovery | Mid-assessment restart and resumption |
| **GS** | Graceful Shutdown | Audit flush, in-flight completion, shutdown timeout (DD-007) |

---

## Business Requirements Coverage

The EM does not have standalone BR-EFFECTIVENESS-xxx requirements (these were archived). The following capabilities are derived from [ADR-EM-001](../../../architecture/decisions/ADR-EM-001-effectiveness-monitor-service-integration.md) and [DD-017](../../../architecture/decisions/DD-017-effectiveness-monitor-v1.1-deferral.md):

| Capability | ADR Section | Testable Behavior |
|------------|-------------|-------------------|
| EA CRD reconciliation | 9.4 | Phase transitions: Pending -> Assessing -> Completed/Failed |
| Health check assessment | 9.2 (event 1) | Pod running, readiness, restart delta, crash loops, OOM detection |
| Alert resolution | 9.2 (event 3) | AlertManager query, resolved vs firing, no-alert handling |
| Metric comparison | 9.2 (event 4) | PromQL before/after, improvement/no-change/degradation scoring |
| Spec hash computation | 9.2 (event 2) | Pre vs post remediation spec hash, changed/unchanged |
| Validity window | Design Principles | Assessment expiry after `validityDeadline`, `expired` reason |
| Derived timing computation | 9.4, Principle 7 (v1.3) | EM computes ValidityDeadline, PrometheusCheckAfter, AlertManagerCheckAfter in status on first reconciliation |
| Assessment scheduled audit | 9.2.0 (v1.3) | `effectiveness.assessment.scheduled` event emitted on first reconciliation with derived timing |
| Audit event emission | 9.2 | 4 component events + 1 scheduling event + 1 lifecycle marker to DataStorage |
| K8s Event emission | DD-EVENT-001 | EventRecorder events for phase transitions; EM always emits Normal EffectivenessAssessed on completion |
| Fail-fast startup | Error Handling | FATAL if enabled dependency unreachable at startup |
| Configuration | 9.5 | validityWindow, scrapeInterval, enable/disable toggles (PrometheusEnabled, AlertManagerEnabled are EM operational config only) |
| Restart recovery | Error Handling | Resume from `status.components` after EM pod restart |
| Graceful shutdown | DD-007 | Audit flush, in-flight EA completion, shutdown within timeout |
| DS scoring (read path) | Scoring Formula | DS computes weighted score on demand from component events |

---

## Test Scenario Matrix (Defense-in-Depth)

### Reconciler Lifecycle (RC)

| ID | Scenario | Unit | Integration | E2E | Notes |
|----|----------|------|-------------|-----|-------|
| UT-EM-RC-001 | EA created -> phase transitions Pending -> Assessing -> Completed | X | | | Happy path |
| UT-EM-RC-002 | EA with expired validity -> phase transitions to Completed (expired) | X | | | |
| UT-EM-RC-003 | EA already Completed -> reconcile is no-op | X | | | Idempotency |
| UT-EM-RC-004 | EA with missing target resource -> phase Failed | X | | | |
| UT-EM-RC-005 | DS unavailable -> requeue with backoff | X | | | Error handling |
| UT-EM-RC-006 | EA for failed RR -> full assessment with unhealthy results | X | | | |
| UT-EM-RC-007 | EA for timed-out RR -> appropriate handling | X | | | |
| IT-EM-RC-001 | Create EA CRD -> reconciler triggered -> status updated -> audit events in DS | | X | | Full reconciler lifecycle |
| IT-EM-RC-002 | Multiple EAs created concurrently -> all processed independently | | X | | Concurrency |
| IT-EM-RC-003 | EA with past validity deadline -> marked expired on first reconcile | | X | | |
| IT-EM-RC-004 | EA for missing target pod -> phase transitions to Failed with message | | X | | envtest: no matching pod |
| IT-EM-RC-005 | EA already Completed -> reconcile is idempotent no-op (no duplicate events) | | X | | Verify DS event count unchanged |
| IT-EM-RC-006 | EA phase transitions Pending -> Assessing -> CRD status updated via K8s API | | X | | Verify status subresource writes |
| IT-EM-RC-007 | EA created with ownerRef -> delete RR -> EA garbage collected | | X | | K8s GC via envtest |
| IT-EM-RC-008 | DS returns transient error -> reconcile requeues with backoff | | X | | DS mock returns 503 temporarily |
| IT-EM-RC-009 | EA for failed RR -> full assessment proceeds, results reflect unhealthy state | | X | | |
| E2E-EM-RC-001 | OOMKill -> full pipeline -> EA created -> EM processes -> all events in DS | | | X | Happy path end-to-end |

### Health Check (HC)

| ID | Scenario | Unit | Integration | E2E | Notes |
|----|----------|------|-------------|-----|-------|
| UT-EM-HC-001 | Pod running + all ready -> health_score = 1.0 | X | | | All sub-checks pass |
| UT-EM-HC-002 | Pod running + not ready -> health_score = 0.5 (readiness_pass=0) | X | | | |
| UT-EM-HC-003 | Pod not running -> health_score = 0.0 | X | | | |
| UT-EM-HC-004 | Pod with restart delta = 1 -> restart_delta sub-score = 0.5 | X | | | |
| UT-EM-HC-005 | Pod with restart delta > 1 -> restart_delta sub-score = 0.0 | X | | | |
| UT-EM-HC-006 | Pod in CrashLoopBackOff -> no_crash_loops sub-score = 0.0 | X | | | |
| UT-EM-HC-007 | Pod OOMKilled since remediation -> no_oom_killed sub-score = 0.0 | X | | | |
| UT-EM-HC-008 | Pod healthy, no restarts, no crashes -> all sub-scores = 1.0 | X | | | Perfect health |
| UT-EM-HC-009 | Target resource not found -> health_score = 0.0, message set | X | | | |
| UT-EM-HC-010 | Health sub-score weighting: pod_running(0.30) + readiness(0.30) + restart(0.15) + crash(0.15) + oom(0.10) | X | | | Weight verification |
| IT-EM-HC-001 | EA for healthy pod -> health event emitted to DS with score 1.0 | | X | | envtest: Running + Ready pod |
| IT-EM-HC-002 | EA for unhealthy pod (not ready) -> health event with score < 1.0 | | X | | envtest: Running + not Ready pod |
| IT-EM-HC-003 | EA for pod not running -> health event with score 0.0 | | X | | envtest: Pending pod |
| IT-EM-HC-004 | EA target pod has restart delta > 0 -> restart_delta sub-score < 1.0 | | X | | envtest: pod with restartCount |
| IT-EM-HC-005 | Health event payload verified in DS (correlation_id, sub-checks, score) | | X | | Query DS API, verify structure |
| IT-EM-HC-006 | Target resource deleted between EA creation and assessment -> health score 0.0 | | X | | envtest: delete pod mid-reconcile |
| E2E-EM-HC-001 | Target pod not running post-remediation -> health score 0.0 in DS | | | X | Unhealthy target |

### Alert Resolution (AR)

| ID | Scenario | Unit | Integration | E2E | Notes |
|----|----------|------|-------------|-----|-------|
| UT-EM-AR-001 | Alert resolved -> alert_score = 1.0 | X | | | |
| UT-EM-AR-002 | Alert still firing -> alert_score = 0.0 | X | | | |
| UT-EM-AR-003 | No alerts found for target -> alert_score = 0.5 | X | | | Ambiguous case |
| UT-EM-AR-004 | AlertManager returns error -> requeue, alert not assessed | X | | | |
| UT-EM-AR-005 | AlertManager disabled in config -> skip alert assessment | X | | | |
| UT-EM-AR-006 | Multiple alerts, some resolved some firing -> score based on primary | X | | | |
| IT-EM-AR-001 | Mock AM returns resolved -> alert event in DS with score 1.0 | | X | | httptest mock |
| IT-EM-AR-002 | Mock AM returns firing -> alert event in DS with score 0.0 | | X | | httptest mock |
| IT-EM-AR-003 | Mock AM returns no alerts for target -> alert event with score 0.5 | | X | | Ambiguous/no-match case |
| IT-EM-AR-004 | Mock AM returns error (503) -> alert not assessed, reconcile requeues | | X | | Error path |
| IT-EM-AR-005 | AM disabled in config -> no alert assessment, no alert event emitted | | X | | Config: alertmanager.enabled=false |
| IT-EM-AR-006 | Alert event payload verified in DS (correlation_id, signal_resolved, score) | | X | | Query DS API, verify structure |
| E2E-EM-AR-001 | Real AM with resolved alert -> alert score 1.0 in DS | | | X | Real AlertManager |
| E2E-EM-AR-002 | Real AM with active alerts -> alert score 0.0 in DS | | | X | Real AlertManager |

### Metric Comparison (MC)

| ID | Scenario | Unit | Integration | E2E | Notes |
|----|----------|------|-------------|-----|-------|
| UT-EM-MC-001 | CPU improved (0.95 -> 0.30) -> metrics_score > 0 | X | | | |
| UT-EM-MC-002 | Memory improved (512Mi -> 128Mi) -> metrics_score > 0 | X | | | |
| UT-EM-MC-003 | No change in metrics -> metrics_score = 0.0 | X | | | |
| UT-EM-MC-004 | Metrics degraded (higher CPU) -> metrics_score = 0.0 (clamped) | X | | | |
| UT-EM-MC-005 | Prometheus returns empty result -> metrics not assessed, requeue | X | | | |
| UT-EM-MC-006 | Prometheus disabled in config -> skip metrics assessment | X | | | |
| UT-EM-MC-007 | Prometheus returns partial data -> use available metrics only | X | | | |
| UT-EM-MC-008 | Multiple metrics, mixed improvement/degradation -> average score | X | | | |
| IT-EM-MC-001 | Mock Prom returns improvement data -> metrics event in DS with score > 0 | | X | | httptest mock |
| IT-EM-MC-002 | Mock Prom returns no-change data -> metrics event with score 0.0 | | X | | httptest mock |
| IT-EM-MC-003 | Mock Prom returns degraded data -> metrics event with score 0.0 (clamped) | | X | | httptest mock |
| IT-EM-MC-004 | Mock Prom returns empty result -> metrics not assessed, reconcile requeues | | X | | Prom has no data yet |
| IT-EM-MC-005 | Mock Prom returns error (503) -> metrics not assessed, reconcile requeues | | X | | Error path |
| IT-EM-MC-006 | Prom disabled in config -> no metrics assessment, no metrics event emitted | | X | | Config: prometheus.enabled=false |
| IT-EM-MC-007 | Mock Prom returns partial data (CPU only, no memory) -> use available metrics | | X | | Partial data handling |
| IT-EM-MC-008 | Metrics event payload verified in DS (correlation_id, before/after, score) | | X | | Query DS API, verify structure |
| E2E-EM-MC-001 | Real Prom with injected improvement data -> metrics score > 0 | | | X | Real Prometheus |
| E2E-EM-MC-002 | Real Prom with same before/after data -> metrics score 0.0 | | | X | Real Prometheus |

### Spec Hash (SH)

| ID | Scenario | Unit | Integration | E2E | Notes |
|----|----------|------|-------------|-----|-------|
| UT-EM-SH-001 | Hash changed (spec modified by remediation) -> spec_changed = true | X | | | |
| UT-EM-SH-002 | Hash unchanged (spec not modified) -> spec_changed = false | X | | | |
| UT-EM-SH-003 | No pre-remediation hash available (legacy RR) -> skip hash comparison | X | | | |
| UT-EM-SH-004 | Hash computation is deterministic for same spec | X | | | |
| IT-EM-SH-001 | Hash computed -> hash event emitted to DS with pre/post hashes | | X | | |
| IT-EM-SH-002 | No pre-remediation hash (legacy) -> hash event with spec_changed=nil, skip | | X | | Missing workflow_created audit event |
| IT-EM-SH-003 | Hash event payload verified in DS (correlation_id, pre/post hash, changed) | | X | | Query DS API, verify structure |
| E2E-EM-SH-001 | Full pipeline -> hash event in DS matches actual spec change | | | X | |

### Validity Window (VW)

| ID | Scenario | Unit | Integration | E2E | Notes |
|----|----------|------|-------------|-----|-------|
| UT-EM-VW-001 | Current time within validity window -> assessment proceeds | X | | | |
| UT-EM-VW-002 | Current time past validity deadline -> EA marked expired, no assessment | X | | | |
| UT-EM-VW-003 | Validity deadline computed correctly from creation + config window (now in status, see DT domain) | X | | | Moved to DT-001 |
| UT-EM-VW-004 | Partial data collected, then validity expires -> complete with partial | X | | | metrics_timed_out reason |
| UT-EM-VW-005 | No data collected before expiry -> complete with expired reason | X | | | |
| IT-EM-VW-001 | EA with past deadline -> marked expired on first reconcile | | X | | |
| IT-EM-VW-002 | EA completes within window -> normal completion | | X | | |
| IT-EM-VW-003 | Partial data collected, then validity expires -> EA Completed with partial reason | | X | | Health done, metrics timeout |
| IT-EM-VW-004 | No data collected before expiry -> EA Completed with expired reason, no component events | | X | | |
| IT-EM-VW-005 | EA with tight validity window (e.g., 1s) -> expires before any check completes | | X | | Edge case: fast expiry |
| E2E-EM-VW-001 | Delayed assessment -> EA marked expired | | | X | |

### Derived Timing (DT) -- ADR-EM-001 v1.3

| ID | Scenario | Unit | Integration | E2E | Notes |
|----|----------|------|-------------|-----|-------|
| UT-EM-DT-001 | ValidityDeadline computed as creationTimestamp + config.ValidityWindow | X | | | Core computation |
| UT-EM-DT-002 | PrometheusCheckAfter computed as creationTimestamp + StabilizationWindow | X | | | Derived from spec |
| UT-EM-DT-003 | AlertManagerCheckAfter computed as creationTimestamp + StabilizationWindow | X | | | Derived from spec |
| UT-EM-DT-004 | All three fields are nil before first reconciliation (Pending EA) | X | | | Pre-condition |
| UT-EM-DT-005 | ValidityDeadline > StabilizationWindow end (guaranteed by config validation) | X | | | Invariant: EM config enforces ValidityWindow > StabilizationWindow |
| UT-EM-DT-006 | Derived times persist in status across reconcile loops (no recomputation) | X | | | Idempotency check |
| IT-EM-DT-001 | First reconciliation sets ValidityDeadline in EA status | | X | | Verify via K8s API |
| IT-EM-DT-002 | First reconciliation sets PrometheusCheckAfter in EA status | | X | | Verify via K8s API |
| IT-EM-DT-003 | First reconciliation sets AlertManagerCheckAfter in EA status | | X | | Verify via K8s API |
| IT-EM-DT-004 | Subsequent reconciliations do not overwrite derived timing fields | | X | | Idempotency: values stable |
| IT-EM-DT-005 | `effectiveness.assessment.scheduled` audit event emitted on first reconciliation | | X | | Verify event in DS |
| IT-EM-DT-006 | Scheduled audit event contains correct validity_deadline, prometheus_check_after, alertmanager_check_after | | X | | Payload correctness |
| IT-EM-DT-007 | Scheduled audit event contains validity_window and stabilization_window durations | | X | | Config observability |
| IT-EM-DT-008 | Reconciler uses status.ValidityDeadline for expiry check (not recomputed) | | X | | Behavioral: status field drives logic |
| IT-EM-DT-009 | Custom ValidityWindow in ReconcilerConfig produces correct ValidityDeadline | | X | | Config override |
| E2E-EM-DT-001 | Full pipeline: EA status shows all 3 derived timing fields after first reconciliation | | | X | kubectl verify |
| E2E-EM-DT-002 | Full pipeline: `effectiveness.assessment.scheduled` event present in DS | | | X | DS API query |

### Audit Events (AE)

| ID | Scenario | Unit | Integration | E2E | Notes |
|----|----------|------|-------------|-----|-------|
| UT-EM-AE-001 | Health event payload: correct fields (health_score, sub-checks, correlation_id) | X | | | |
| UT-EM-AE-002 | Hash event payload: correct fields (pre/post hash, spec_changed) | X | | | |
| UT-EM-AE-003 | Alert event payload: correct fields (signal_resolved, alert_score) | X | | | |
| UT-EM-AE-004 | Metrics event payload: correct fields (cpu_before/after, metrics_score) | X | | | |
| UT-EM-AE-005 | Completed event payload: lifecycle marker, no score, components_assessed list | X | | | |
| UT-EM-AE-006 | Correlation ID matches EA.spec.correlationID (= RR.Name) | X | | | |
| UT-EM-AE-007 | Events emitted incrementally as components complete | X | | | |
| UT-EM-AE-008 | Scheduled event payload: correct fields (validity_deadline, prometheus_check_after, alertmanager_check_after, validity_window, stabilization_window) | X | | | ADR-EM-001 v1.3 |
| IT-EM-AE-001 | All 6 events present in DS after successful assessment (scheduled + 4 components + completed) | | X | | Query DS API |
| IT-EM-AE-002 | Events have correct event_type field values | | X | | |
| IT-EM-AE-003 | Only completed event emitted when validity expired with no data | | X | | |
| IT-EM-AE-004 | Events emitted incrementally (health first, then hash, alert, metrics, completed) | | X | | Verify ordering by timestamp |
| IT-EM-AE-005 | Correlation ID in all events matches EA.spec.correlationID | | X | | Cross-event consistency |
| IT-EM-AE-006 | Partial assessment (Prom disabled): 4 events emitted (health, hash, alert, completed) | | X | | Config: prometheus.enabled=false |
| IT-EM-AE-007 | Partial assessment (AM disabled): 4 events emitted (health, hash, metrics, completed) | | X | | Config: alertmanager.enabled=false |
| IT-EM-AE-008 | DS write failure for component event -> reconcile requeues, retries event emission | | X | | DS mock returns 500 on first attempt |
| E2E-EM-AE-001 | Full pipeline -> all 6 events in DS with correct payloads (scheduled + 4 components + completed) | | | X | |

### Kubernetes Events (KE)

| ID | Scenario | Unit | Integration | E2E | Notes |
|----|----------|------|-------------|-----|-------|
| UT-EM-KE-001 | EffectivenessAssessed event emitted on completion (always Normal) | X | | | No threshold comparison |
| UT-EM-KE-002 | Event emitted for each phase transition | X | | | |
| IT-EM-KE-001 | K8s events recorded via FakeRecorder during reconcile | | X | | record.FakeRecorder |
| IT-EM-KE-002 | EffectivenessAssessed event emitted on successful completion | | X | | Verify event reason + message (always Normal) |
| IT-EM-KE-003 | No K8s events emitted when EA already Completed (idempotency) | | X | | No duplicate events |

### Configuration (CF)

| ID | Scenario | Unit | Integration | E2E | Notes |
|----|----------|------|-------------|-----|-------|
| UT-EM-CF-001 | Valid config parsed successfully | X | | | |
| UT-EM-CF-002 | Missing required fields -> error | X | | | |
| UT-EM-CF-003 | Default values applied when optional fields omitted | X | | | |
| UT-EM-CF-004 | validityWindow default = 30m | X | | | |
| UT-EM-CF-005 | scrapeInterval default = 60s | X | | | |
| UT-EM-CF-006 | prometheus.enabled = false -> metrics assessment skipped | X | | | |
| UT-EM-CF-007 | alertmanager.enabled = false -> alert assessment skipped | X | | | |
| UT-EM-CF-008 | maxConcurrentAssessments sets MaxConcurrentReconciles | X | | | |
| IT-EM-CF-001 | Controller starts with valid config -> reconciler operational | | X | | Full wired-up config |
| IT-EM-CF-002 | Prom disabled + AM disabled -> reconciler runs without external deps | | X | | Health + hash only |
| IT-EM-CF-003 | Custom validityWindow (e.g., 5m) -> EA deadline computed correctly | | X | | Verify status.spec.config |

### Fail-Fast Startup (FF)

| ID | Scenario | Unit | Integration | E2E | Notes |
|----|----------|------|-------------|-----|-------|
| UT-EM-FF-001 | DS reachable, Prom reachable, AM reachable -> startup succeeds | X | | | |
| UT-EM-FF-002 | DS unreachable -> FATAL, startup fails | X | | | |
| UT-EM-FF-003 | Prom enabled but unreachable -> FATAL, startup fails | X | | | |
| UT-EM-FF-004 | AM enabled but unreachable -> FATAL, startup fails | X | | | |
| UT-EM-FF-005 | Prom disabled and unreachable -> startup succeeds (not checked) | X | | | |
| UT-EM-FF-006 | AM disabled and unreachable -> startup succeeds (not checked) | X | | | |
| IT-EM-FF-001 | Controller starts with DS reachable, Prom mock reachable, AM mock reachable -> success | | X | | All startup checks pass |
| IT-EM-FF-002 | Controller start with DS unreachable -> startup fails (FATAL) | | X | | DS health check fails |
| IT-EM-FF-003 | Controller start with Prom enabled but mock unreachable -> startup fails | | X | | Prom mock not started |
| IT-EM-FF-004 | Controller start with AM enabled but mock unreachable -> startup fails | | X | | AM mock not started |
| IT-EM-FF-005 | Controller start with Prom disabled, mock absent -> startup succeeds | | X | | Config override |
| E2E-EM-FF-001 | EM started without Prometheus running -> pod fails to start | | | X | Real failure |

### Operational Metrics (OM)

| ID | Scenario | Unit | Integration | E2E | Notes |
|----|----------|------|-------------|-----|-------|
| UT-EM-OM-001 | Reconciliation counter incremented on each reconcile | X | | | |
| UT-EM-OM-002 | Reconciliation duration histogram observed | X | | | |
| UT-EM-OM-003 | Error counter incremented on reconcile failure | X | | | |
| IT-EM-OM-001 | Reconciliation counter incremented after EA reconcile | | X | | controller-runtime registry |
| IT-EM-OM-002 | Reconciliation duration histogram observed after reconcile | | X | | |
| IT-EM-OM-003 | Error counter incremented when reconcile fails (DS error) | | X | | Verify delta |

### Restart Recovery (RR)

| ID | Scenario | Unit | Integration | E2E | Notes |
|----|----------|------|-------------|-----|-------|
| UT-EM-RR-001 | EA with partial status.components -> skip completed, resume remaining | X | | | |
| UT-EM-RR-002 | EA with all components completed -> emit completed event only | X | | | |
| UT-EM-RR-003 | EA with no components completed -> full assessment | X | | | |
| IT-EM-RR-001 | EM restarts mid-assessment -> resumes from status.components (skip completed) | | X | | Stop/start controller manager |
| IT-EM-RR-002 | EA with all components assessed -> reconcile emits completed event only | | X | | Pre-set status.components |
| IT-EM-RR-003 | EA with partial components (health done, rest pending) -> completes remaining | | X | | Pre-set healthAssessed=true |

### Graceful Shutdown (GS)

| ID | Scenario | Unit | Integration | E2E | Notes |
|----|----------|------|-------------|-----|-------|
| UT-EM-GS-001 | Context cancellation causes reconciler to exit cleanly | X | | | No panic, no goroutine leak |
| UT-EM-GS-002 | Audit store Close() called on context cancellation | X | | | DD-007: audit data preserved |
| UT-EM-GS-003 | Audit store Flush() completes before shutdown continues | X | | | Ordered shutdown |
| UT-EM-GS-004 | Audit store Flush() error during shutdown is handled gracefully | X | | | Error logged, exit non-zero |
| IT-EM-GS-001 | In-flight EA assessment completes before shutdown | | X | | EA reaches Completed/Failed |
| IT-EM-GS-002 | Audit buffer flushed before shutdown (no event loss) | | X | | Verify all events in DS |
| IT-EM-GS-003 | Controller manager stop -> audit flush -> audit close (ordered) | | X | | Verify ordering via log timestamps |
| E2E-EM-GS-001 | SIGTERM handled within shutdown timeout (<=30s) | | | X | Real pod lifecycle |

### Spec Drift Guard (SD) — DD-EM-002 v1.1

| ID | Scenario | Unit | Integration | E2E | Notes |
|----|----------|------|-------------|-----|-------|
| UT-EM-COND-001 | AssessmentComplete and SpecIntegrity condition types defined | X | | | Constants exist |
| UT-EM-COND-002 | All AssessmentComplete reason constants defined | X | | | Full, Partial, Expired, SpecDrift, MetricsTimedOut, NoExecution |
| UT-EM-COND-003 | SpecIntegrity reason constants defined (SpecUnchanged, SpecDrifted) | X | | | |
| UT-EM-COND-004 | SetCondition adds a new condition to EA | X | | | |
| UT-EM-COND-005 | SetCondition updates an existing condition | X | | | |
| UT-EM-COND-006 | GetCondition retrieves an existing condition by type | X | | | |
| UT-EM-COND-007 | GetCondition returns nil for missing condition | X | | | |
| UT-EM-COND-008 | IsConditionTrue returns true for True condition | X | | | |
| UT-EM-COND-009 | IsConditionTrue returns false for False/missing condition | X | | | |
| UT-EM-COND-010 | Multiple conditions on same EA managed independently | X | | | |
| UT-EM-COND-011 | AssessmentReasonSpecDrift constant exists in EA types | X | | | |
| UT-DS-EFF-013 | DS short-circuits to score 0.0 when reason=spec_drift | X | | | DD-EM-002 v1.1 |
| UT-DS-EFF-014 | DS short-circuits even when component scores are high | X | | | Hard override |
| UT-DS-EFF-015 | Non-drift reasons still scored normally | X | | | No false positives |
| IT-EM-SD-001 | Target spec changes after hash computation -> EA completes with spec_drift | | X | | Status patch + watch trigger |
| IT-EM-SD-002 | Target spec unchanged -> EA completes normally (no drift) | | X | | Negative test |
| IT-EM-SD-003 | Target resource missing -> hash from empty spec, no drift | | X | | Graceful degradation |
| E2E-EM-SD-001 | Spec drift detected -> EA completes with spec_drift, DS scores 0.0 | | | X | Full stack: CRD + audit + DS scoring |
| E2E-EM-SD-002 | No drift -> normal completion, DS scores normally | | | X | Negative E2E test |

---

## Coverage Projection

Coverage is measured per-tier against the tier-specific code subset (see Defense-in-Depth Strategy above). Because TDD drives implementation from tests, every business requirement must have corresponding test scenarios. The **>=80% target per tier** ensures comprehensive coverage of all business logic.

### Unit Tests (Tier 1) -- Target: >=80% of unit-testable code

Code under test: `config/`, `health/`, `alert/`, `metrics/`, `hash/`, `validity/`, `audit/` (construction), `phase/`, `types/`

| Domain | Scenarios | Packages Exercised |
|--------|-----------|-------------------|
| Reconciler (RC) | 7 | phase/ (transition logic) |
| Health Check (HC) | 10 | health/ (sub-scoring algorithm, weighting) |
| Alert Resolution (AR) | 6 | alert/ (scoring decisions) |
| Metric Comparison (MC) | 8 | metrics/ (delta math, improvement detection) |
| Spec Hash (SH) | 4 | hash/ (deterministic hashing) |
| Validity Window (VW) | 5 | validity/ (deadline computation, expiry logic) |
| Audit Events (AE) | 7 | audit/ (payload construction, not emission) |
| K8s Events (KE) | 2 | phase/ (event reason mapping) |
| Configuration (CF) | 8 | config/ (parsing, validation, defaults) |
| Fail-Fast (FF) | 6 | config/ (startup check logic) |
| Operational Metrics (OM) | 3 | metrics/ (metric registration, naming) |
| Restart Recovery (RR) | 3 | phase/ (resume decision logic) |
| Graceful Shutdown (GS) | 4 | cmd/ (shutdown sequence, audit flush/close) |
| **Total** | **73** | |

### Integration Tests (Tier 2) -- Target: >=80% of integration-testable code

Code under test: `client/` (Prom/AM/DS HTTP clients), `status/` (EA status updater), `reconciler/` (Reconcile loop), `internal/controller/effectivenessmonitor/` (manager wiring)

| Domain | Scenarios | Integration Code Exercised |
|--------|-----------|---------------------------|
| Reconciler (RC) | 9 | reconciler/ (full Reconcile loop: create, update, error, GC, idempotency) |
| Health Check (HC) | 6 | client/ (K8s pod queries), status/ (health score write) |
| Alert Resolution (AR) | 6 | client/ (AM HTTP client: resolved, firing, error, disabled paths) |
| Metric Comparison (MC) | 8 | client/ (Prom HTTP client: query, error, disabled, partial) |
| Spec Hash (SH) | 3 | client/ (DS audit query for pre-hash), status/ (hash write) |
| Validity Window (VW) | 5 | reconciler/ (deadline check, partial completion, expiry) |
| Audit Events (AE) | 8 | client/ (DS HTTP writes: events, errors, retries) |
| K8s Events (KE) | 3 | reconciler/ (EventRecorder integration) |
| Configuration (CF) | 3 | internal/controller/ (manager setup with config) |
| Fail-Fast (FF) | 5 | internal/controller/ (startup health checks to Prom/AM/DS) |
| Operational Metrics (OM) | 3 | reconciler/ (controller-runtime metrics wiring) |
| Restart Recovery (RR) | 3 | reconciler/ (re-list EAs, status.components check, skip logic) |
| Graceful Shutdown (GS) | 3 | cmd/ + internal/controller/ (shutdown ordering, audit flush to DS) |
| **Total** | **65** | |

### E2E Tests (Tier 3)

| Domain | Scenarios | Focus |
|--------|-----------|-------|
| Reconciler (RC) | 1 | Full pipeline happy path |
| Health Check (HC) | 1 | Unhealthy target |
| Alert Resolution (AR) | 2 | Real AM resolved/active |
| Metric Comparison (MC) | 2 | Real Prom improvement/no-change |
| Spec Hash (SH) | 1 | Real spec change detection |
| Validity Window (VW) | 1 | Delayed assessment expiry |
| Audit Events (AE) | 1 | All events in DS end-to-end |
| Fail-Fast (FF) | 1 | Pod fails without Prometheus |
| Graceful Shutdown (GS) | 1 | SIGTERM within timeout |
| **Total** | **11** | |

### Grand Total: 149 test scenarios

---

## Infrastructure Requirements

### Unit Tests (Tier 1)

| Component | Required | Notes |
|-----------|----------|-------|
| envtest / K8s API | No | Interface mocks for K8s client |
| DataStorage | No | Interface mock |
| Prometheus | No | `PrometheusQuerier` interface mock |
| AlertManager | No | `AlertManagerClient` interface mock |
| PostgreSQL / Redis | No | |

### Integration Tests (Tier 2)

| Component | Required | Source | Port (DD-TEST-001) |
|-----------|----------|--------|---------------------|
| envtest (K8s API) | Yes | In-process | N/A |
| PostgreSQL | Yes | Podman container | 15434 |
| Redis | Yes | Podman container | 16383 |
| DataStorage | Yes | Podman container | 18092 |
| Prometheus mock | Yes | `httptest.NewServer` (in-process) | Ephemeral |
| AlertManager mock | Yes | `httptest.NewServer` (in-process) | Ephemeral |

**Suite Pattern**: `SynchronizedBeforeSuite` -- Phase 1 starts DS bootstrap (PostgreSQL, Redis, DataStorage) + envtest. Phase 2 per-process setup (controller manager, httptest mocks).

### E2E Tests (Tier 3)

| Component | Required | Deployment | Port / Access |
|-----------|----------|------------|---------------|
| Kind cluster | Yes | `kind create cluster` | N/A |
| EM controller | Yes | Inline manifest in Kind | ClusterIP |
| DataStorage | Yes | Deployment in Kind | NodePort 30081 |
| PostgreSQL | Yes | Deployment in Kind | ClusterIP |
| Redis | Yes | Deployment in Kind | ClusterIP |
| Prometheus | Yes | Deployment in Kind | NodePort (TBD, verify collision matrix) |
| AlertManager | Yes | Deployment in Kind | NodePort (TBD, verify collision matrix) |
| Full pipeline services | Yes (for full pipeline) | Existing infrastructure | See DD-TEST-001 |

**Data Injection**:
- Prometheus: remote write API via `--web.enable-remote-write-receiver` flag, using `promwrite` Go library
- AlertManager: POST to `/api/v2/alerts` endpoint

---

## Error Handling and Edge Cases

| Edge Case | Expected Behavior | Test Tier |
|-----------|-------------------|-----------|
| RR deleted before assessment | EA GC'd via ownerRef; emitted events remain in DS | IT |
| DS unavailable during assessment | Requeue with backoff (5m, 10m, 20m, 30m max) | UT, IT |
| Target resource deleted | Health checks fail; score reflects unhealthy state | UT |
| Prometheus metrics not yet available | Requeue after `scrapeInterval`; complete without if validity expires | UT, IT |
| Prometheus returns partial data | Use available metrics; omit missing fields | UT |
| Multiple RRs for same target | Each RR has own EA; assessed independently by correlation_id | UT, IT |
| EM pod restart during assessment | Resume from `status.components`; skip completed components | UT, IT |
| Duplicate reconciles for same EA | If phase == Completed, no-op; DS dedup by correlation_id + event_type | UT |
| No `remediation.workflow_created` event | Skip hash comparison; continue with health, metrics, alert | UT |
| Validity expired with partial data | Complete with `metrics_timed_out` or `partial` reason | UT, IT |
| Validity expired with NO data | Complete with `expired` reason; no component events | UT, IT |
| EM delayed beyond validity (e.g., DS outage) | On first reconcile, check validity; if expired, mark without collecting data | UT |

---

## Implementation Roadmap

### Scenario Implementation Status Audit (2026-02-14)

#### Unit Tests (UT-EM-*): Implemented Scenarios

| Domain | Implemented | Total | Scenario IDs |
|--------|-------------|-------|--------------|
| **AR** (Alert Resolution) | 5 | 6 | UT-EM-AR-001, -002, -003, -004, -005 |
| **AE** (Audit Events) | 5 | 7 | UT-EM-AE-001, -002, -003, -004, -005 |
| **CF** (Configuration) | 8 | 8 | UT-EM-CF-001, -002, -003, -004, -005, -006, -007, -008 |
| **OM** (Operational Metrics) | 3 | 3 | UT-EM-OM-001, -002, -003 |
| **SH** (Spec Hash) | 5 | 5 | UT-EM-SH-001, -002, -003, -004, -005 |
| **HC** (Health Check) | 6 | 10 | UT-EM-HC-001, -002, -003, -004, -005, -006 |
| **MC** (Metric Comparison) | 7 | 8 | UT-EM-MC-001, -002, -003, -004, -005, -007, -008 |
| **PH** (Phase) | 5 | — | UT-EM-PH-001, -002, -003, -004, -005 |
| **VW** (Validity Window) | 3 | 5 | UT-EM-VW-001, -002, -003 |
| **COND** (Conditions) | 11 | 11 | UT-EM-COND-001 through -011 |
| **DS** (DS Scoring) | 3 | 3 | UT-DS-EFF-013, -014, -015 |
| **EA Types** | ✅ | — | EA type constants and helpers |
| **Total** | **~61** | **~75** | |

#### Integration Tests (IT-EM-*): Implemented Scenarios

| Domain | Implemented | Total | Scenario IDs |
|--------|-------------|-------|--------------|
| **RC** (Reconciler) | 9 | 9 | IT-EM-RC-001, -002, -003, -004, -005, -006, -007, -008, -009 |
| **CF** (Configuration) | 3 | 3 | IT-EM-CF-001, -002, -003 |
| **VW** (Validity Window) | 5 | 5 | IT-EM-VW-001, -002, -003, -004, -005 |
| **HC** (Health Check) | 6 | 6 | IT-EM-HC-001, -002, -003, -004, -005, -006 |
| **AR** (Alert Resolution) | 6 | 6 | IT-EM-AR-001, -002, -003, -004, -005, -006 |
| **MC** (Metric Comparison) | 8 | 8 | IT-EM-MC-001, -002, -003, -004, -005, -006, -007, -008 |
| **SH** (Spec Hash) | 3 | 3 | IT-EM-SH-001, -002, -003 |
| **AE** (Audit Events) | 8 | 8 | IT-EM-AE-001, -002, -003, -004, -005, -006, -007, -008 |
| **KE** (K8s Events) | 4 | 4 | IT-EM-KE-001, -002, -003, -004 |
| **FF** (Fail-Fast) | 5 | 5 | IT-EM-FF-001, -002, -003, -004, -005 |
| **OM** (Operational Metrics) | 3 | 3 | IT-EM-OM-001, -002, -003 |
| **RR** (Restart Recovery) | 3 | 3 | IT-EM-RR-001, -002, -003 |
| **GS** (Graceful Shutdown) | 3 | 3 | IT-EM-GS-001, -002, -003 |
| **DT** (Derived Timing) | 8 | 9 | IT-EM-DT-001, -002, -003, -004, -008, -009, -010 + extras |
| **SD** (Spec Drift) | 3 | 3 | IT-EM-SD-001, -002, -003 |
| **Total** | **76** | **~78** | |

Remaining IT gaps (low-ROI, deferred — see Group B analysis in PR discussion):
IT-EM-DT-005 (scheduled event emitted), IT-EM-DT-006 (payload fields), IT-EM-DT-007 (window durations) — redundant with UT audit payload tests.

#### E2E Tests (E2E-EM-*): Implemented Scenarios

| Domain | Implemented | Total | Scenario IDs |
|--------|-------------|-------|--------------|
| **RC** (Reconciler) | 1 | 1 | E2E-EM-RC-001 |
| **HC** (Health Check) | 1 | 1 | E2E-EM-HC-001 |
| **AR** (Alert Resolution) | 2 | 2 | E2E-EM-AR-001, -002 |
| **MC** (Metric Comparison) | 2 | 2 | E2E-EM-MC-001, -002 |
| **SH** (Spec Hash) | 1 | 1 | E2E-EM-SH-001 |
| **VW** (Validity Window) | 1 | 1 | E2E-EM-VW-001 |
| **AE** (Audit Events) | 1 | 1 | E2E-EM-AE-001 |
| **FF** (Fail-Fast) | 1 | 1 | E2E-EM-FF-001 |
| **GS** (Graceful Shutdown) | 1 | 1 | E2E-EM-GS-001 |
| **SD** (Spec Drift) | 2 | 2 | E2E-EM-SD-001, -002 |
| **DT** (Derived Timing) | 2 | 2 | E2E-EM-DT-001, -002 |
| **Total** | **15** | **15** | **100%** |

---

### Phase 1: Unit Tests (RED -> GREEN -> REFACTOR)

| Step | Task | Estimate | Status |
|------|------|----------|--------|
| 1.1 | Define Go interfaces (`PrometheusQuerier`, `AlertManagerClient`, `AuditClient`, `K8sClient`) | 2h | Pending |
| 1.2 | Scaffold `pkg/effectivenessmonitor/` package structure | 1h | Pending |
| 1.3 | Write RC unit tests (7 scenarios) -> implement reconciler skeleton | 4h | Pending |
| 1.4 | Write HC unit tests (10 scenarios) -> implement health check logic | 3h | Pending |
| 1.5 | Write AR unit tests (6 scenarios) -> implement alert resolution | 2h | Pending |
| 1.6 | Write MC unit tests (8 scenarios) -> implement metric comparison | 3h | Pending |
| 1.7 | Write SH unit tests (4 scenarios) -> implement spec hash | 1h | Pending |
| 1.8 | Write VW unit tests (5 scenarios) -> implement validity window | 2h | Pending |
| 1.9 | Write AE unit tests (7 scenarios) -> implement audit event construction | 2h | Pending |
| 1.10 | Write KE, CF, FF, OM, RR unit tests (24 scenarios) | 4h | Pending |
| 1.11 | Write GS unit tests (4 scenarios) -> implement shutdown logic | 2h | Pending |

### Phase 2: Integration Tests (RED -> GREEN -> REFACTOR)

| Step | Task | Estimate | Status |
|------|------|----------|--------|
| 2.1 | Create `httptest` mock Prometheus server (`test/infrastructure/prometheus_mock.go`) | 1h | Completed |
| 2.2 | Create `httptest` mock AlertManager server (`test/infrastructure/alertmanager_mock.go`) | 1h | Completed |
| 2.3 | Scaffold `test/integration/effectivenessmonitor/suite_test.go` (DS bootstrap + envtest + mocks) | 2h | Completed |
| 2.4 | Write IT scenarios (56 remaining of 67) -> wire reconciler with real DS | 16h | In Progress (11/67) |
| 2.5 | Write GS integration tests (3 scenarios) -> verify shutdown ordering with DS | 2h | Pending |

### Phase 3: E2E Tests (RED -> GREEN -> REFACTOR)

| Step | Task | Estimate | Status |
|------|------|----------|--------|
| 3.1 | Deploy Prometheus in Kind (`DeployPrometheus()` in fullpipeline_e2e.go) | 2h | Completed |
| 3.2 | Deploy AlertManager in Kind (`DeployAlertManager()` in fullpipeline_e2e.go) | 1h | Completed |
| 3.3 | Create `InjectMetrics()` and `InjectAlerts()` test helpers | 2h | Completed (OTLP/HTTP JSON for metrics, REST for alerts) |
| 3.4 | Scaffold `test/e2e/effectivenessmonitor/` suite | 1h | Pending |
| 3.5 | Write E2E scenarios (11 tests) | 6h | Pending |

### Phase 4: CI/CD Integration

| Step | Task | Estimate | Status |
|------|------|----------|--------|
| 4.1 | Add EM to `.github/workflows/ci-pipeline.yml` matrixes | 30m | Completed |
| 4.2 | Add `EFFECTIVENESSMONITOR_UNIT_PATTERN` to Makefile | 15m | Completed |
| 4.3 | Update DD-TEST-001 port allocation | 30m | Completed |

### Phase 5: Full Pipeline Integration (Separate Task)

| Step | Task | Estimate | Status |
|------|------|----------|--------|
| 5.1 | Deploy EM in full pipeline E2E | 2h | Deferred |
| 5.2 | Add EM assessment scenarios to `test/e2e/fullpipeline/` | 4h | Deferred |

---

## Success Criteria

### Quantitative

| Metric | Target | Measured Against |
|--------|--------|------------------|
| Unit test count | 73 scenarios | |
| Integration test count | 65 scenarios | |
| E2E test count | 11 scenarios | |
| Unit-testable coverage | **>=80%** | Pure logic: config/, health/, alert/, metrics/, hash/, validity/, audit/, phase/, types/ |
| Integration-testable coverage | **>=80%** | I/O code: client/, status/, reconciler/, internal/controller/ |
| E2E coverage | **>=80%** | Full service code (pkg + internal/controller) |
| All Tiers coverage | **>=80%** | Full service code (line-by-line merge across all tiers) |
| All tests pass in CI | 100% | |
| No `Skip()` or `XIt()` usage | 0 | |

### Qualitative

- All 4 assessment components tested independently and in combination
- Validity window enforcement verified at all tiers
- Audit event payloads verified against ADR-EM-001 Section 9.2 schema
- Real Prometheus/AlertManager API contract validated in E2E
- Restart recovery verified with partial `status.components`
- Fail-fast startup verified for all enabled dependencies
- Graceful shutdown verified: audit flush, in-flight completion, ordered teardown (DD-007)

---

## References

- [ADR-EM-001: Effectiveness Monitor Service Integration](../../../architecture/decisions/ADR-EM-001-effectiveness-monitor-service-integration.md)
- [DD-017: Effectiveness Monitor v1.1 Deferral](../../../architecture/decisions/DD-017-effectiveness-monitor-v1.1-deferral.md)
- [DD-TEST-001: Port Allocation Strategy](../../../architecture/decisions/DD-TEST-001-port-allocation-strategy.md)
- [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md) (v2.6.0)
- [V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md](../../development/testing/V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md)
- [RO_COMPREHENSIVE_TEST_PLAN.md](../05-remediationorchestrator/RO_COMPREHENSIVE_TEST_PLAN.md) (structural reference)

---

## Revision History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.3.0 | 2026-02-14 | AI Assistant | Full implementation status audit refresh: 76 IT (was 11), 15 E2E (was 0); added 5 config-disable INT scenarios (IT-EM-CF-002, AR-005, AE-006, AE-007, FF-005) covering nil-client safety; deferred 3 low-ROI DT audit payload IT scenarios |
| 1.2.0 | 2026-02-14 | AI Assistant | Added Spec Drift Guard (SD) domain: 11 UT (conditions) + 3 UT (DS scoring) + 3 IT = 17 scenarios (DD-EM-002 v1.1, DD-CRD-002-EA); covers conditions infrastructure, DS score=0.0 short-circuit, reconciler drift detection |
| 1.1.1 | 2026-02-13 | AI Assistant | Removed ScoringThreshold: UT-EM-CF-008, IT-EM-CF-004; removed RemediationIneffective: UT-EM-KE-002, IT-EM-KE-003; EM always emits Normal EffectivenessAssessed; DS computes score on demand; grand total 149 scenarios |
| 1.1.0 | 2026-02-12 | AI Assistant | Added Derived Timing (DT) domain: 6 UT + 9 IT + 2 E2E = 17 scenarios (ADR-EM-001 v1.3); updated AE domain with UT-EM-AE-008 (scheduled event payload); updated IT-EM-AE-001 and E2E-EM-AE-001 counts from 5 to 6 events; ValidityDeadline moved from spec to status |
| 1.0.5 | 2026-02-13 | AI Assistant | Added Scenario Implementation Status Audit section; updated Phase 2-4 statuses (infra, CI, Makefile completed; 11/67 IT scenarios implemented); corrected E2E count to 11 |
| 1.0.4 | 2026-02-13 | AI Assistant | Added Graceful Shutdown (GS) domain: 4 UT + 3 IT + 1 E2E = 8 scenarios (DD-007, ADR-032); grand total now 153 scenarios; aligned with AIAnalysis/SignalProcessing/Gateway GS patterns |
| 1.0.3 | 2026-02-09 | AI Assistant | Updated coverage targets to >=80% per tier (TDD mandate: all business requirements must be implemented); updated coverage_report.py quality targets |
| 1.0.2 | 2026-02-09 | AI Assistant | Corrected coverage model: per-tier coverage against tier-specific code subset; added EM code partitioning; added EM to coverage_report.py GO_SERVICE_CONFIG; aligned UNIT_PATTERN with coverage script |
| 1.0.1 | 2026-02-09 | AI Assistant | Expanded integration tier from 17 to 64 scenarios to cover integration-testable code paths |
| 1.0.0 | 2026-02-09 | AI Assistant | Initial test plan with 98 scenarios across UT/IT/E2E |

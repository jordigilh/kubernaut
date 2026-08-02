# Audit Event Catalog — Effectiveness Monitor (EM)

Authoritative reference for all structured audit events emitted by the `effectivenessmonitor` service.

**Source of truth:** `pkg/effectivenessmonitor/types/types.go` (`AuditEventType` const block)
**Payload mapping:** `pkg/effectivenessmonitor/audit/manager.go` (`Record*` methods build typed sub-objects per ADR-EM-001 v1.3); OpenAPI enum `EffectivenessAssessmentAuditPayloadEventType.AllValues()` in `pkg/datastorage/ogen-client/oas_schemas_gen.go` is the schema-level mirror of the same 7 values
**Predecessor doc:** [DD-AUDIT-003](../../../../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md) §"Effectiveness Monitor Service" documents 9 events (7 shipped + 2 aspirational "V1.1 Level 2" placeholders); this catalog is the current, code-verified reference — the 2 V1.1 events are not implemented anywhere in code as of this date (see Known Gaps)

**Schema:** All events are built as `EffectivenessAssessmentAuditPayload` records (typed sub-objects, ADR-EM-001 v1.3 component-level architecture). Common fields on every event: `event_type`, `event_category="effectiveness"` (`CategoryEffectiveness`), `correlation_id` (`ea.Spec.CorrelationID`), `namespace` (`ea.Namespace`), `ea_name`, `component`, `assessed`, `details`, `score`, `cluster_id` (conditionally set from `ea.Spec.ClusterID`, DD-AUDIT-003 v2.2 CC8.1). Actor: `ActorType=Service`, `ActorID=effectivenessmonitor-controller` (`ServiceName`). Resource: `EffectivenessAssessment` / `ea.Name`.

---

## Component Assessment Events (V1.0, Level 1)

| Event Type | Constant | Action | Trigger | Typed Sub-Object / Fields |
|-----------|----------|--------|---------|---------------------------|
| `effectiveness.health.assessed` | `AuditHealthAssessed` | `assessed` | K8s health check component runs and produces a score, outside alert-decay re-probing (`AlertDecayRetries == 0`) | `health_checks`: `pod_running`, `readiness_pass`, `total_replicas`, `ready_replicas`, `restart_delta`, `crash_loops`, `oom_killed`, `pending_count` |
| `effectiveness.alert.assessed` | `AuditAlertAssessed` | `assessed` | Alert component check completes and is **not** judged to be decaying (resolved, or still firing without satisfying decay conditions) | `alert_resolution`: `alert_resolved`, `active_count`, `resolution_time_seconds` (optional) |
| `effectiveness.metrics.assessed` | `AuditMetricsAssessed` | `assessed` | Prometheus enabled and the metrics component successfully assesses (`metricsResult.Component.Assessed == true`) | `metric_deltas`: namespace-scoped (`cpu_before/after` populated; `memory_before/after`, `latency_p95_before/after_ms`, `error_rate_before/after`, `throughput_before/after_rps` reserved for Phase B, currently unpopulated) + cluster-scoped Node/PV fields (Issue #193, DD-EM-005 v1.1): `node_not_ready_before/after`, `node_memory_pressure_before/after`, `node_disk_pressure_before/after`, `pv_phase_failed_before/after`, `pv_phase_pending_before/after`, `pv_usage_ratio_before/after` |
| `effectiveness.hash.computed` | `AuditHashComputed` | `computed` | Hash component runs (every EA lifecycle, subject to DD-EM-004 async-target deferral), unconditionally after `assessHash` returns | Flat fields (DD-EM-002, no sub-object): `pre_remediation_spec_hash`, `post_remediation_spec_hash`, `hash_match` |
| `effectiveness.alert_decay.detected` | `AuditAlertDecayDetected` | `detected` | Alert decay detected via multi-probe cross-validation (resource healthy, alert still firing) — emitted **exactly once** per EA, guarded by `AlertDecayRetries == 0` (Issue #369, BR-EM-012) | `alert_resolution` (hardcoded `alert_resolved=false, active_count=1`); `details` carries health/alert scores + retry count as text |
| `effectiveness.assessment.scheduled` | `AuditAssessmentScheduled` | `scheduled` | **Exactly once** per EA lifecycle, when `ValidityDeadline` is first persisted (Pending→WaitingForPropagation or Pending/WFP→Stabilizing transition); explicitly not emitted on the Assessing transition to avoid duplicates (Issue #573, ADR-EM-001 §9.2.0) | Flat fields: `validity_deadline`, `prometheus_check_after`, `alertmanager_check_after`, `hash_compute_after`/`hash_compute_delay` (#277), `alert_check_delay`, `validity_window`, `stabilization_window` |
| `effectiveness.assessment.completed` | `AuditAssessmentCompleted` | `assessed` | EA transitions to Completed phase, for every completion reason (`full`, `partial`, `expired`, `spec_drift`, `metrics_timed_out`, `no_execution`, `alert_decay_timeout`) | Flat fields (ADR-EM-001 §9.2 "Batch 3"): `reason`, `details`, `signal_name`, `components_assessed` (string array), `completed_at`, `assessment_duration_seconds` |

**Emitted from:** `pkg/effectivenessmonitor/audit/manager.go` (`Record*` methods: `RecordHealthAssessed`, `RecordAlertAssessed`, `RecordMetricsAssessed`, `RecordHashComputed`, `RecordAlertDecayDetected`, `RecordAssessmentScheduled`, `RecordAssessmentCompleted`), called from `internal/controller/effectivenessmonitor/events.go` (`emitHealthEvent`, `emitAlertEvent`, `emitMetricsEvent`, `emitHashEvent`, `emitAlertDecayEvent`, `emitScheduledEventIfFirst`, `emitCompletedAuditEvent`), which are in turn invoked from `internal/controller/effectivenessmonitor/reconcile_components.go` and `reconcile_validity_phase.go`/`completion.go`.

**Note:** `RecordComponentAssessed` (`manager.go`) is a generic/legacy path with a full `componentConfigs` map lookup (including `alert_decay`) but has **zero production callers** — every real emission goes through the 7 specialized `Record*` methods above, which build the typed sub-objects. Similarly, `pkg/effectivenessmonitor/audit/audit.go`'s `Builder` interface (`BuildHealthEvent`, `BuildHashEvent`, etc.) has zero non-test callers — a superseded parallel design pre-dating the `Manager` pattern. Neither is wired into production; flagged here for awareness, not yet removed as dead code (out of scope for this catalog).

---

## Known Gaps (tracked, not fixed by this catalog)

1. **`effectiveness.learning.triggered` and `effectiveness.crd.updated` are documented (DD-AUDIT-003, "V1.1 Level 2") but not implemented.** A full-repo search finds no Go constant, no OpenAPI schema enum value, no `Record*` method, and no call site for either — they are aspirational placeholders for a future release, not a current production gap. Do not treat their absence as a regression.
2. **`metric_deltas`' Phase B fields are documented but unpopulated.** `RecordMetricsAssessed`'s payload comment explicitly states "Phase A (V1.0): Only CPUBefore/CPUAfter are populated" — `memory_before/after`, `latency_p95_before/after_ms`, `error_rate_before/after`, and `throughput_before/after_rps` are reserved fields, not yet wired to real data sources.

---

## Adding New Events

1. Define the `AuditEventType` constant in `pkg/effectivenessmonitor/types/types.go`
2. Add a payload builder / typed sub-object and a `Record*` method in `pkg/effectivenessmonitor/audit/manager.go`
3. Register the OpenAPI discriminator variant for `EffectivenessAssessmentAuditPayload` (`pkg/datastorage/ogen-client`) so `AllValues()` stays in sync
4. Wire the emit call at the production reconciler call site (`internal/controller/effectivenessmonitor/`), never only in a test
5. Update this catalog with the new event's trigger, fields, and NIST/SOC2 control mapping
6. Ensure a UT proves the emission decision and an IT proves it fires through the reconciler entry point (Pyramid Invariant)

---

*Last updated: 2026-07-31 | QE readiness audit follow-up (DD-AUDIT-003 single-source-of-truth migration) | Covers all 7 currently-implemented event types; see Known Gaps for the 2 documented-but-unimplemented V1.1 placeholders*

# DD-017: Effectiveness Monitor Service — V1.0 Level 1 + V1.1 Level 2

**Status**: ✅ APPROVED (v2.5 — Typed Audit Sub-Objects)
**Last Reviewed**: 2026-02-14
**Confidence**: 95%

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 2.6 | 2026-02-14 | Architecture Team | **Post-implementation audit**: Phase B metrics (memory, latency p95, error rate) were implemented during V1.0 — updated Phase A/B sections to reflect this. Health scoring documented as decision-tree algorithm (matches implementation). Throughput queries remain unimplemented. See ADR-EM-001 v1.7 for full gap inventory. |
| 2.5 | 2026-02-14 | Architecture Team | **Typed audit sub-objects**: Added `health_checks`, `metric_deltas`, `alert_resolution` typed sub-objects to `EffectivenessAssessmentAuditPayload` in OpenAPI spec. EM component assessors now emit structured data alongside the human-readable `details` string. Health assessor enhanced with CrashLoopBackOff and OOMKilled detection. Metrics assessor expansion (memory, latency p95, error rate PromQL queries) deferred to Phase B. Coordinated with HAPI team (DD-HAPI-016 v1.1) via issue #82. |
| 2.4 | 2026-02-13 | Architecture Team | EA spec simplification: EAConfig only has StabilizationWindow; ScoringThreshold, PrometheusEnabled, AlertManagerEnabled removed from EA spec (EM operational config only). EM always emits Normal EffectivenessAssessed; RemediationIneffective removed. DS computes weighted score on demand from component audit events. |
| 2.3 | 2026-02-12 | Architecture Team | EA CRD ValidityDeadline moved from spec to status (computed by EM on first reconciliation). Added PrometheusCheckAfter and AlertManagerCheckAfter status fields. New `effectiveness.assessment.scheduled` audit event. See ADR-EM-001 v1.3. |
| 2.2 | 2026-02-09 | Architecture Team | RO-created EffectivenessAssessment CRD pattern (ADR-EM-001 v1.1). EM watches EA CRDs, not RR CRDs. K8s Condition `EffectivenessAssessed` on RR. Async metrics evaluation. Side-effect detection deferred to post-V1.0. Updated scoring formula (3 components). See ADR-EM-001 for full integration architecture. |
| 2.1 | 2026-02-09 | Architecture Team | Design refinements from gap analysis: (1) Correlation ID is RR.Name, not UID. (2) EM reads exclusively from audit traces, never from RR CRD. (3) RO must query API server directly for target resource spec (not cache). (4) No graceful degradation for Prometheus/AlertManager — fail-fast via config toggles. (5) Superseded BR-EFFECTIVENESS-001/002/003 and archived stale docs. (6) DD-EFFECTIVENESS-002 DB-backed idempotency superseded — EM uses audit-event dedup via DS. |
| 2.0 | 2026-02-05 | Architecture Team | Partial reinstatement: Level 1 (automated assessment) moves to V1.0. Level 2 (AI-powered analysis) remains V1.1. Dual spec hash design, audit-trace storage, cooldown alignment, RR immutability via CEL. Supersedes v1.0 full deferral. |
| 1.0 | 2025-12-01 | Architecture Team | Initial DD-017: Full Effectiveness Monitor deferred to V1.1 due to end-of-2025 timeline constraint |

---

## Context & Problem

### Original Deferral (v1.0, December 2025)

The Effectiveness Monitor was originally deferred entirely to V1.1 (DD-017 v1.0) for two reasons:

1. **Timeline constraint**: V1.0 had a hard end-of-2025 deadline. Including the EM would have added 2-3 weeks and risked missing the deadline.
2. **Data dependency**: The EM's AI-powered analysis (Level 2) requires 8+ weeks of remediation data for meaningful pattern recognition and effectiveness assessment at 80%+ confidence.

Both were valid constraints at the time of the decision.

### What Changed (v2.0, February 2026)

1. **Timeline constraint has lapsed**: V1.0 was delivered. The end-of-2025 deadline is no longer a constraint.
2. **Remediation feedback loop identified as critical gap**: During DD-HAPI-016 design work, we identified that without post-remediation state capture, the system cannot:
   - Detect configuration regressions (target resource reverted to a previously problematic state)
   - Inform the LLM about past remediation effectiveness for the same target
   - Prevent the LLM from recommending the same ineffective remediation repeatedly
3. **The spec hash alone justifies V1.0 inclusion**: The dual spec hash (pre/post remediation) is the minimum critical data point for the HAPI remediation history context feature (DD-HAPI-016). Without it, the history context has no way to determine whether past remediations are relevant to the current signal.
4. **Level 1 provides Day-1 value**: Unlike Level 2 (which needs 8+ weeks of data), Level 1 automated assessment (health checks, metric comparison, effectiveness scoring) provides useful output from the first remediation.

### Business Requirements

- **BR-INS-001**: Assess remediation action effectiveness — partially addressed by Level 1
- **BR-INS-002**: Correlate action outcomes with environment improvements — partially addressed by Level 1 metric comparison
- **BR-INS-003**: Track long-term effectiveness trends — V1.1 (Level 2)
- **BR-INS-004**: Identify consistently positive actions — V1.1 (Level 2)
- **BR-INS-005**: Detect adverse side effects — addressed by Level 1 side-effect detection
- **BR-INS-006 to BR-INS-010**: Advanced pattern recognition, comparative analysis, temporal patterns, seasonal variations, continuous improvement — V1.1 (Level 2)

---

## Decision

**APPROVED: Partial Reinstatement** — Level 1 (Automated Assessment) moves to V1.0. Level 2 (AI-Powered Analysis) remains V1.1.

### Rationale

1. **Spec hash capture is a V1.0 dependency**: DD-HAPI-016 (Remediation History Context) requires pre/post remediation spec hashes to function. Without them, HAPI cannot determine whether past remediations are relevant or detect configuration regressions.

2. **Level 1 has no data dependency**: Health checks, metric comparisons, and effectiveness scoring work from Day 1. They don't need historical data to accumulate.

3. **Level 2 still benefits from deferral**: AI-powered analysis (HolmesGPT PostExec) requires historical patterns. By V1.1 (~8 weeks post V1.0), Level 1 will have generated sufficient assessment data for Level 2 to provide high-confidence pattern analysis.

4. **Cooldown alignment**: The EM stabilization window (5 min) aligns with the RO's `RecentlyRemediatedCooldown` (5 min), guaranteeing effectiveness data is available before a new remediation can start for the same target. This is a natural integration point.

---

## V1.0 Scope: Level 1 — Automated Assessment

### Overview

After every workflow completion, the EM waits for a stabilization window, then performs deterministic automated checks to assess whether the remediation improved the situation. All assessment data is emitted as structured audit events — no new database tables required.

### Capabilities

#### 1. Dual Spec Hash Capture

The EM captures two hashes per remediation, enabling configuration regression detection:

- **Pre-remediation hash**: SHA-256 of the target resource's `.spec` BEFORE the workflow modifies it. Captured by the **RO controller** when it creates the WorkflowExecution (Analyzing phase). Emitted as a field in the RO's `remediation.workflow_created` audit event.

- **Post-remediation hash**: SHA-256 of the target resource's `.spec` AFTER the workflow completes and the stabilization window passes. Captured by the **EM service**. Emitted as part of the `effectiveness.assessment.completed` audit event.

The hash comparison logic (three-way matching) is documented in DD-HAPI-016.

#### 2. Health Checks (K8s API)

After stabilization, the EM queries the K8s API for the target resource and emits a typed `health_checks` sub-object in the `effectiveness.health.assessed` audit event:

- **pod_running** (bool): At least one pod found for the target resource
- **readiness_pass** (bool): All desired replicas are ready (`readyReplicas == totalReplicas`)
- **total_replicas** (int): Total desired replicas
- **ready_replicas** (int): Number of ready replicas
- **restart_delta** (int): Total container restart count since remediation
- **crash_loops** (bool): Any container in CrashLoopBackOff waiting state (v2.5)
- **oom_killed** (bool): Any container terminated with OOMKilled reason since remediation (v2.5)
- **pending_count** (int): Number of pods still in Pending phase after stabilization window (v2.5). Non-zero after stabilization indicates scheduling failures, image pull issues, or resource exhaustion.

#### 3. Pre/Post Metric Comparison (Prometheus)

The EM queries Prometheus for key metrics and emits a typed `metric_deltas` sub-object in the `effectiveness.metrics.assessed` audit event. Metrics are compared as pre-remediation (earliest sample) vs. post-remediation (latest sample) within a query range window.

**Implemented in V1.0** (all four metrics):
- CPU utilization (`container_cpu_usage_seconds_total`)
- Memory utilization (`container_memory_working_set_bytes`)
- Request latency p95 (`histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[...]))`)
- Error rate (cluster-specific; default: `rate(http_requests_total{code=~"5.."}[...])`)

**Not yet implemented**:
- Throughput (`throughput_before_rps`, `throughput_after_rps`) — fields exist in OpenAPI schema but no PromQL query is wired.

> **v2.6 Note**: The original Phase A (CPU only) / Phase B (memory, latency, error rate) split was eliminated during implementation. All four PromQL queries were implemented in the V1.0 metrics scorer (`pkg/effectivenessmonitor/metrics/scorer.go`). The `metric_deltas` typed sub-object populates all fields. Throughput remains as a future enhancement.

"Before" = earliest sample in the query range (before EA creation).
"After" = latest sample in the query range (post-stabilization).

#### 4. Alert Resolution Check (AlertManager)

The EM queries AlertManager to determine whether the triggering alert resolved after remediation and emits a typed `alert_resolution` sub-object in the `effectiveness.alert.assessed` audit event:

- **alert_resolved** (bool): Whether the triggering alert is no longer active
- **active_count** (int): Number of matching active alerts in AlertManager
- **resolution_time_seconds** (nullable float): Time from remediation to resolution (null if not resolved)

If AlertManager is unavailable, the check fails with `assessed: false` and the component is excluded from the weighted score.

#### 5. Effectiveness Scoring (V1.0)

A formula-based score from 0.0 (completely ineffective) to 1.0 (fully effective). V1.0 uses three components; side-effect detection is deferred to post-V1.0.

```
score = weighted_average(
    health_check_pass_rate   * 0.40,   // target resource healthy?
    signal_resolved          * 0.35,   // did the triggering alert clear?
    metric_improvement_ratio * 0.25    // did metrics improve? (no change = 0.0)
)
```

> See [ADR-EM-001](ADR-EM-001-effectiveness-monitor-service-integration.md) Section 6 for detailed sub-scoring, weight redistribution tables, and health check sub-component weights.

#### 6. Side-Effect Detection (Deferred to post-V1.0)

> **v2.2 Decision**: Side-effect detection is deferred to post-V1.0. The complexity of determining causality (a remediation could impact workloads in other namespaces or at the node level) and the dependency on multiple AlertManager scrape cycles makes this unsuitable for the V1.0 formula-based approach. To be revisited in ~1 month.

#### 7. K8s Event Emission

The EM always emits a Normal `EffectivenessAssessed` K8s event on assessment completion. There is no threshold-based Warning event; the EM does not compute or compare average scores. DataStorage computes the weighted effectiveness score on demand from component audit events.

### EM Trigger: RO-Created EffectivenessAssessment CRD

> **v2.2 Clarification**: DD-EFFECTIVENESS-003 (Watch RemediationRequest) is **superseded**. The EM no longer watches RR CRDs directly. Instead, the **RO creates an `EffectivenessAssessment` CRD** when the RR reaches a terminal phase (Completed, Failed, TimedOut), following the same lifecycle pattern as AIAnalysis, WorkflowExecution, and NotificationRequest. The EM watches EA CRDs.
>
> This provides: restart recovery via EA CRD `status.components`, `kubectl` observability, assessment deadline enforcement, and consistent lifecycle ownership by the RO.
>
> See [ADR-EM-001](ADR-EM-001-effectiveness-monitor-service-integration.md) Section 9.4 for the full EA CRD definition and Section 3 for the lifecycle sequence diagram.
>
> **v2.3 Clarification**: EA CRD `validityDeadline` is in **status** (not spec), computed by the EM on first reconciliation. Status also includes `prometheusCheckAfter` and `alertManagerCheckAfter` for derived timing. The EM emits an `effectiveness.assessment.scheduled` audit event when scheduling the assessment, capturing these derived timestamps for audit trail correlation.
>
> **EA Spec Simplification**: The EA CRD `spec.config` (EAConfig) now contains only `StabilizationWindow` (set by the RO). `ScoringThreshold`, `PrometheusEnabled`, and `AlertManagerEnabled` have been removed from the EA spec — they are EM operational config only.

### Stabilization Window & Cooldown Alignment

The EM waits a stabilization window (configurable, default 5 minutes) after EA CRD creation before running its assessment. This gives the system time to reflect the change (e.g., new pods starting, traffic rebalancing).

**Critical invariant**: The EM stabilization window MUST be <= the RO's `RecentlyRemediatedCooldown` (currently both 5 min). This guarantees health and alert data are available within the cooldown window:

```
T+0:      Workflow completes → RO creates EA CRD + NotificationRequest (parallel)
T+0:      RO sets RR Condition: EffectivenessAssessed=False
T+0-5m:   RO cooldown active → new RRs for same target BLOCKED
T+5m:     EM stabilization elapsed → health, alert, hash assessed immediately
T+5m-10m: EM waits for Prometheus metrics (if scrape pending, up to maxWaitForMetrics)
T+≤10m:   EM finalizes → audit event emitted, EA phase=Completed
T+≤10m:   RO detects EA completion → updates RR Condition: EffectivenessAssessed=True
T+5m:     Cooldown expires → if signal fires again, new RR allowed through
T+5m+:    HAPI queries DS → health/alert effectiveness data IS AVAILABLE
```

These two values are coupled. If either changes, the other must be reviewed. This constraint must be documented in operational runbooks.

### Data Storage: Audit Traces

All EM assessment data is stored as **structured audit events**, leveraging the existing audit infrastructure:

**RO audit event** (at WFE creation):
```json
{
    "event_type": "remediation.workflow_created",
    "service": "remediationorchestrator",
    "remediation_request_uid": "rr-xxx",
    "target_resource": "Deployment/prod/my-app",
    "pre_remediation_spec_hash": "sha256:aabb1122...",
    "workflow_type": "ScaleUp",
    "timestamp": "2026-02-05T14:00:00Z"
}
```

**EM audit events** (per ADR-EM-001 Principle 5: EM collects, DS scores):

The EM emits **individual component-level audit events**, not a single monolithic event. Each component event carries a typed sub-object with structured assessment data alongside the human-readable `details` string. DS correlates component events by `correlation_id` and computes the weighted score on demand.

- `effectiveness.assessment.scheduled` (v2.3): Emitted on first reconciliation; captures derived timing (`validityDeadline`, `prometheusCheckAfter`, `alertManagerCheckAfter`).

- `effectiveness.health.assessed`: Health check results with typed `health_checks` sub-object:
```json
{
    "event_type": "effectiveness.health.assessed",
    "correlation_id": "rr-xxx",
    "component": "health",
    "assessed": true,
    "score": 1.0,
    "details": "all 3 pods ready, no restarts",
    "health_checks": {
        "pod_running": true,
        "readiness_pass": true,
        "total_replicas": 3,
        "ready_replicas": 3,
        "restart_delta": 0,
        "crash_loops": false,
        "oom_killed": false,
        "pending_count": 0
    }
}
```

- `effectiveness.alert.assessed`: Alert resolution with typed `alert_resolution` sub-object:
```json
{
    "event_type": "effectiveness.alert.assessed",
    "correlation_id": "rr-xxx",
    "component": "alert",
    "assessed": true,
    "score": 0.0,
    "details": "alert \"HighCPULoad\" still active (1 active alerts)",
    "alert_resolution": {
        "alert_resolved": false,
        "active_count": 1,
        "resolution_time_seconds": null
    }
}
```

- `effectiveness.metrics.assessed`: Metric comparison with typed `metric_deltas` sub-object:
```json
{
    "event_type": "effectiveness.metrics.assessed",
    "correlation_id": "rr-xxx",
    "component": "metrics",
    "assessed": true,
    "score": 0.5,
    "details": "1 of 1 metrics improved, average score 0.50",
    "metric_deltas": {
        "cpu_before": 0.95,
        "cpu_after": 0.72,
        "memory_before": null,
        "memory_after": null,
        "latency_p95_before_ms": null,
        "latency_p95_after_ms": null,
        "error_rate_before": null,
        "error_rate_after": null
    }
}
```

> **v2.6 Note — Metrics**: All four metrics (CPU, memory, latency p95, error rate) are populated in V1.0. Throughput fields remain nullable. See ADR-EM-001 v1.7.

- `effectiveness.hash.computed`: Spec hash comparison (DD-EM-002):
```json
{
    "event_type": "effectiveness.hash.computed",
    "correlation_id": "rr-xxx",
    "component": "hash",
    "assessed": true,
    "post_remediation_spec_hash": "sha256:ccdd3344...",
    "pre_remediation_spec_hash": "sha256:aabb1122...",
    "hash_match": false
}
```

- `effectiveness.assessment.completed`: Lifecycle marker with reason:
```json
{
    "event_type": "effectiveness.assessment.completed",
    "correlation_id": "rr-xxx",
    "component": "completed",
    "assessed": true,
    "reason": "full"
}
```

**No new database tables**. DataStorage correlates RO and EM audit events by **`RR.Name`** (the RemediationRequest name, which is the correlation ID across all audit traces) to build the complete picture. DS may add internal indexes on hash columns for query performance — this is a DS implementation detail.

The existing `migrations/v1.1/006_effectiveness_assessment.sql` (`action_assessments`, `effectiveness_results` tables) can be repurposed as DS-internal materialized views for performance optimization, not as a separate write target.

> **v2.1 Clarification**: DD-EFFECTIVENESS-002's DB-backed idempotency design (direct PostgreSQL tables for `effectiveness_results` and `action_assessments`) is superseded. The EM does not have its own database connection. Idempotency is achieved through audit event deduplication in DataStorage (DS checks for existing `effectiveness.assessment.completed` event for the given RR.Name before accepting a new one).

### EM Data Source: Audit Traces Only (via EA CRD Trigger)

> **v2.1 Clarification**: The EM MUST NOT read remediation context from the RemediationRequest CRD directly. The RR is transient — it may be deleted, tampered, or have its status modified after the EM assessment window. The EM reads all required data (correlation ID, signal metadata, workflow details, completion timestamps) from the **audit trace** stored in DataStorage.
>
> **v2.2 Clarification**: The EM is *triggered* by the `EffectivenessAssessment` CRD (created by RO), not by watching the RR. The EA CRD `spec.correlationID` provides the correlation ID for the DS audit trail query. If the RR is deleted while the EA is still in progress, the EA is garbage-collected via ownerReference, but any audit events already emitted to DS survive.
>
> This guarantees:
> - EM can assess remediations even if the RR CRD is deleted before assessment (EA GC is acceptable; partial audit data in DS survives)
> - EM is immune to RR status tampering
> - EM can handle all RR terminal states (Completed, Failed, TimedOut) uniformly
> - The assessment is based on the immutable audit record, not mutable K8s state

### RO Pre-Remediation Hash: Direct API Server Query

> **v2.1 Clarification**: When the RO computes the `pre_remediation_spec_hash`, it MUST query the Kubernetes API server directly (`client.Get()` with uncached reader) for the target resource's current `.spec`. The RO does NOT cache full resource specs — it only caches partial object metadata for scope management. The hash MUST be computed and emitted in the `remediation.workflow_created` audit event BEFORE creating the WorkflowExecution CRD, ensuring the hash captures the true pre-remediation state.

### Dependency Configuration: Fail-Fast, No Fallbacks

> **v2.1 Clarification**: The EM requires **deterministic outcomes**. There is no graceful degradation for Prometheus or AlertManager.

The EM configuration (`config.yaml`) includes individual toggles for each external dependency:

```yaml
# config.yaml
prometheus:
  enabled: true          # Enable/disable Prometheus metric comparison
  url: "http://prometheus:9090"
alertmanager:
  enabled: true          # Enable/disable AlertManager alert resolution check
  url: "http://alertmanager:9093"
```

**Startup behavior**:
- If `prometheus.enabled: true` and Prometheus is unreachable → **EM fails to start** (fail-fast)
- If `alertmanager.enabled: true` and AlertManager is unreachable → **EM fails to start** (fail-fast)
- If either is `enabled: false` → that capability is excluded from the assessment and scoring formula is adjusted accordingly (the weight is redistributed to remaining capabilities)

| Dependency | `enabled: true` + unreachable | `enabled: false` |
|------------|-------------------------------|-------------------|
| K8s API | Assessment fails — retry on next reconcile | N/A (always required) |
| Prometheus | **EM fails to start** | Skip metric comparison, adjust scoring weights |
| AlertManager | **EM fails to start** | Skip alert resolution, adjust scoring weights |
| DataStorage | Buffer audit events locally, retry delivery | N/A (always required) |

### New Service

```
cmd/effectivenessmonitor/main.go
pkg/effectivenessmonitor/
    assessor.go          — assessment orchestrator
    hash.go              — spec hash computation
    health/checker.go    — K8s health checks
    metrics/collector.go — Prometheus metric queries
    metrics/scorer.go    — effectiveness score formula
    alertmanager/client.go — alert resolution check
```

**RBAC**: Read access to pods, deployments, replicasets. Prometheus query access. AlertManager query access. DataStorage audit event write access.

---

## RR Status Immutability (CRD CEL Validation)

As part of the EM integration, we introduce immutability enforcement on RemediationRequest status fields via Kubernetes CEL validation rules (`x-kubernetes-validations`). This ensures hash fields and terminal outcomes cannot be accidentally overwritten.

**No webhook needed** — CEL validation is enforced by the API server at the CRD schema level.

### Immutability Tiers

**Write-once** (immutable after first set):
- `Outcome`, `CompletedAt`, `FailureReason`, `FailurePhase`, `TimeoutPhase`, `TimeoutTime`, `SkipReason`
- CEL rule: `!has(oldSelf.outcome) || self.outcome == oldSelf.outcome`

**Terminal-immutable** (locked once `OverallPhase` reaches a terminal state):
- `OverallPhase`, `Message`
- CEL rule: `!(oldSelf.overallPhase in ['Completed', 'Failed', 'Skipped']) || self.overallPhase == oldSelf.overallPhase`

**Always mutable** (controller/operator-writable throughout lifecycle):
- `Conditions`, `Deduplication`, `TimeoutConfig`, `LastModifiedBy`, `LastModifiedAt`, `ConsecutiveFailureCount`, `NextAllowedExecution`, `BlockedUntil`

**Lifecycle-mutable** (writable during active reconciliation, locked after terminal phase):
- All Ref fields (`SignalProcessingRef`, `AIAnalysisRef`, `WorkflowExecutionRef`, etc.), phase timestamps, counters

The existing mutating webhook for `TimeoutConfig` operator attribution (`LastModifiedBy`, `LastModifiedAt`) remains — CEL can validate but not mutate.

---

## V1.1 Scope: Level 2 — AI-Powered Analysis (Unchanged)

Level 2 remains deferred to V1.1 as originally planned in DD-017 v1.0:

- **HolmesGPT PostExec endpoint** (`/api/v1/postexec/analyze`): Root cause validation, oscillation detection, lesson extraction
- **Decision logic**: When to trigger AI analysis (P0 failures, new action types, suspected oscillations, periodic batch)
- **Pattern learning**: Historical comparison, context-aware effectiveness
- **Batch processing**: Cost-efficient daily/weekly aggregation

### V1.1 Triggers (Unchanged)

1. V1.0 deployed with Level 1 EM operational
2. 8+ weeks of Level 1 assessment data accumulated
3. Sufficient historical patterns for Level 2 to provide high-confidence analysis (80%+)

### V1.1 Integration with DD-HAPI-016

When Level 2 arrives, it enriches the audit events with:
- `root_cause_resolved: true/false` — did the remediation fix the actual cause or just mask it?
- `lessons_learned: [...]` — extracted insights for future investigations
- `oscillation_detected: true/false` — did the fix cause a different problem?

DS reads these richer fields from the same audit traces. HAPI receives richer context through the same endpoint. No architectural change required.

---

## Consequences

### Positive

- V1.0 captures post-remediation state (dual spec hash) — critical for DD-HAPI-016
- Deterministic effectiveness scoring from Day 1 — no data dependency
- HAPI receives pre-computed effectiveness data, reducing LLM investigation burden for past remediations
- Cooldown-EM alignment guarantees data availability before next remediation
- Audit-trace storage avoids new tables and leverages existing infrastructure
- CEL immutability protects terminal status fields without webhook overhead
- V1.1 delta reduced — only Level 2 AI analysis remains

### Negative

- New service in V1.0 (`cmd/effectivenessmonitor/`) adds operational complexity
- RBAC requires Prometheus and AlertManager access — not all clusters may have these
  - **Mitigation**: Configuration toggles (`prometheus.enabled`, `alertmanager.enabled`) allow disabling at deployment time. When enabled, fail-fast ensures deterministic outcomes.
- Formula-based scoring may not capture nuance (e.g., "CPU dropped 3% but crossed below alert threshold" vs. "CPU dropped 3% but still critical")
  - **Mitigation**: Level 2 AI analysis in V1.1 addresses nuanced cases

---

## Related Decisions

- **DD-HAPI-016**: Remediation History Context Enrichment (depends on this DD for effectiveness data)
- **DD-EFFECTIVENESS-001**: Hybrid Automated + AI Analysis approach (Level 1 architecture, Level 2 triggers)
- **DD-EFFECTIVENESS-002**: Restart Recovery Idempotency (DB-backed idempotency — **SUPERSEDED by v2.1**: EM uses audit-event dedup via DS instead of direct DB tables)
- **DD-017 v1.0**: Original full deferral (superseded by this v2.0)
- **DD-CRD-002**: Kubernetes Conditions Standard (conditions infrastructure)
- **DD-CRD-002-EA**: [EffectivenessAssessment Conditions](DD-CRD-002-effectivenessassessment-conditions.md) — `AssessmentComplete` and `SpecIntegrity` condition types
- **DD-EM-002 v1.1**: [Canonical Spec Hashing & Spec Drift Guard](DD-EM-002-canonical-spec-hash.md) — Re-hash on each reconcile, `spec_drift` reason, DS score=0.0 short-circuit
- **DD-EVENT-001**: Controller Event Registry (event reason constants)

---

## Review & Evolution

### When to Revisit

- **MANDATORY**: When V1.1 planning begins — confirm Level 2 scope and triggers
- **MANDATORY**: If Level 1 formula-based scoring proves insufficient — may need to accelerate Level 2
- **OPTIONAL**: If cooldown values change — review EM stabilization window alignment
- **OPTIONAL**: If audit trace query performance degrades — evaluate materialized view approach

---

**Status**: ✅ APPROVED (v2.5) — Level 1 in V1.0 (EA CRD pattern, typed audit sub-objects), Level 2 in V1.1
**Authoritative Integration Architecture**: [ADR-EM-001](ADR-EM-001-effectiveness-monitor-service-integration.md)
**Next Review**: Throughput PromQL query implementation, then V1.1 planning (estimated Q2 2026)
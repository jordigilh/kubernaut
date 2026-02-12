# DD-017: Effectiveness Monitor Service — V1.0 Level 1 + V1.1 Level 2

**Status**: ✅ APPROVED (v2.0 — Partial Reinstatement)
**Last Reviewed**: 2026-02-05
**Confidence**: 95%

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
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

After stabilization, the EM queries the K8s API for the target resource:

- Pod running status
- Readiness probe passing
- Restart count delta (before vs. after remediation)
- CrashLoopBackOff detection
- OOMKilled detection since remediation

#### 3. Pre/Post Metric Comparison (Prometheus)

The EM queries Prometheus for key metrics:

- CPU utilization (before vs. after)
- Memory utilization (before vs. after)
- Request latency — p50, p95, p99 (before vs. after)
- Error rate (before vs. after)
- Request throughput (before vs. after)

"Before" = average over 30-minute window before the triggering signal fired.
"After" = average over the stabilization window after workflow completion.

#### 4. Alert Resolution Check (AlertManager)

The EM queries AlertManager to determine whether the triggering alert resolved after remediation. If AlertManager is unavailable, this check is skipped and marked as `unknown` in the assessment.

#### 5. Effectiveness Scoring

A formula-based score from 0.0 (completely ineffective) to 1.0 (fully effective):

```
score = weighted_average(
    health_check_pass_rate   * 0.3,   // target resource healthy?
    signal_resolved          * 0.3,   // did the triggering alert clear?
    metric_improvement_ratio * 0.2,   // did metrics improve?
    no_side_effects          * 0.2    // no new alerts or degradation?
)
```

#### 6. Side-Effect Detection

The EM checks for new alerts or conditions that appeared on the target resource (or co-located resources) after remediation that were not present before. These indicate the remediation may have caused unintended consequences.

#### 7. K8s Event Emission

When effectiveness is below a threshold (configurable, default 0.5), the EM emits:

```go
recorder.Event(rr, corev1.EventTypeWarning, events.EventReasonRemediationIneffective,
    fmt.Sprintf("Remediation effectiveness %.1f/1.0 — signal unresolved, consider alternative approach", score))
```

### Stabilization Window & Cooldown Alignment

The EM waits a stabilization window (hardcoded at 5 minutes) after workflow completion before running its assessment. This gives the system time to reflect the change (e.g., new pods starting, traffic rebalancing).

**Critical invariant**: The EM stabilization window MUST be <= the RO's `RecentlyRemediatedCooldown` (currently 5 min, hardcoded in `reconciler.go` L168). This guarantees:

```
T+0:    Workflow completes → EM starts stabilization timer
T+0-5m: RO cooldown active → new RRs for same target BLOCKED
T+~5m:  EM finishes assessment → audit event emitted to DataStorage
T+5m:   Cooldown expires → if signal fires again, new RR allowed through
T+5m+:  HAPI queries DS → effectiveness data IS AVAILABLE
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

**EM audit event** (after assessment):
```json
{
    "event_type": "effectiveness.assessment.completed",
    "service": "effectivenessmonitor",
    "remediation_request_uid": "rr-xxx",
    "target_resource": "Deployment/prod/my-app",
    "post_remediation_spec_hash": "sha256:ccdd3344...",
    "effectiveness_score": 0.4,
    "signal_resolved": false,
    "health_checks": {
        "pod_running": true,
        "readiness_pass": true,
        "restart_delta": 0,
        "crash_loops": false,
        "oom_killed": false
    },
    "metric_deltas": {
        "cpu_before": 0.95,
        "cpu_after": 0.92,
        "memory_before": 0.60,
        "memory_after": 0.62,
        "latency_p95_before_ms": 200,
        "latency_p95_after_ms": 195,
        "error_rate_before": 0.02,
        "error_rate_after": 0.019,
        "throughput_before_rps": 1000,
        "throughput_after_rps": 1000
    },
    "side_effects_detected": [],
    "assessed_at": "2026-02-05T14:05:00Z"
}
```

**No new database tables**. DataStorage correlates RO and EM audit events by `remediation_request_uid` to build the complete picture. DS may add internal indexes on hash columns for query performance — this is a DS implementation detail.

The existing `migrations/v1.1/006_effectiveness_assessment.sql` (`action_assessments`, `effectiveness_results` tables) can be repurposed as DS-internal materialized views for performance optimization, not as a separate write target.

### Graceful Degradation

The EM operates on a best-effort basis for external dependencies:

| Dependency | Unavailable Behavior |
|------------|---------------------|
| K8s API | Assessment fails — retry on next reconcile |
| Prometheus | Skip metric comparison, compute score from health checks + signal resolution only |
| AlertManager | Skip alert resolution check, mark as `unknown` |
| DataStorage | Buffer audit events locally, retry delivery |

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
  - **Mitigation**: Graceful degradation (skip unavailable dependencies)
- Formula-based scoring may not capture nuance (e.g., "CPU dropped 3% but crossed below alert threshold" vs. "CPU dropped 3% but still critical")
  - **Mitigation**: Level 2 AI analysis in V1.1 addresses nuanced cases

---

## Related Decisions

- **DD-HAPI-016**: Remediation History Context Enrichment (depends on this DD for effectiveness data)
- **DD-EFFECTIVENESS-001**: Hybrid Automated + AI Analysis approach (Level 1 architecture, Level 2 triggers)
- **DD-EFFECTIVENESS-002**: Restart Recovery Idempotency (DB-backed idempotency for assessments)
- **DD-017 v1.0**: Original full deferral (superseded by this v2.0)
- **DD-CRD-002**: Kubernetes Conditions Standard (conditions infrastructure)
- **DD-EVENT-001**: Controller Event Registry (event reason constants)

---

## Review & Evolution

### When to Revisit

- **MANDATORY**: When V1.1 planning begins — confirm Level 2 scope and triggers
- **MANDATORY**: If Level 1 formula-based scoring proves insufficient — may need to accelerate Level 2
- **OPTIONAL**: If cooldown values change — review EM stabilization window alignment
- **OPTIONAL**: If audit trace query performance degrades — evaluate materialized view approach

---

**Status**: ✅ APPROVED (v2.0) — Level 1 in V1.0, Level 2 in V1.1
**Next Review**: V1.1 planning (estimated Q2 2026)
# DD-HAPI-016: Remediation History Context Enrichment

**Status**: ✅ APPROVED
**Decision Date**: 2026-02-05
**Version**: 1.1
**Confidence**: 95%
**Applies To**: HolmesGPT API (HAPI), DataStorage Service (DS)

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-02-05 | Architecture Team | Initial design: two-tier query, three-way hash comparison, full remediation chain, DS business logic endpoint, prompt reasoning framework |
| 1.1 | 2026-02-14 | Architecture Team | Updated DS internal logic to reflect EM's component-level audit event architecture (per ADR-EM-001 v1.3). DS now uses a two-step query (RO events by target_resource, then EM component events by correlation_id) and reuses exported scoring functions from effectiveness_handler.go. Health/metric/alert data sourced from typed ogen sub-objects (health_checks, metric_deltas, alert_resolution) on EM component events. signalResolved read from alert_resolution.alert_resolved. Coordinated via issue #82. |

---

## Context & Problem

When a signal fires for a target resource that has already been remediated, the LLM investigation starts from zero — it has no visibility into what was previously attempted. This leads to:

1. **Repeating ineffective remediations**: The LLM recommends ScaleUp for HighCPULoad without knowing ScaleUp was already tried twice and failed.
2. **No configuration regression detection**: If someone rolls back a deployment to a previously problematic version, the LLM doesn't know this exact configuration caused issues before.
3. **No awareness of declining effectiveness**: A pattern of ScaleUp attempts with effectiveness 0.4, 0.3, 0.2 clearly shows diminishing returns, but without the history the LLM can't see it.
4. **Wasted investigation time**: The LLM re-investigates questions that the Effectiveness Monitor (DD-017 v2.0, Level 1) has already answered — "did the pod recover?", "did metrics improve?", "did the alert clear?"

### Scope

This DD covers how HAPI acquires and uses remediation history context. It does NOT cover:

- How the EM assesses effectiveness (see DD-017 v2.0)
- How the EM stores data (see DD-017 v2.0, audit traces)
- The effectiveness scoring formula (see DD-017 v2.0)

### Business Requirements

- **BR-INS-001**: Assess remediation action effectiveness — this DD enables the LLM to consume those assessments
- **BR-INS-002**: Correlate action outcomes with environment improvements — DS correlates audit events, HAPI surfaces the correlation

---

## Decision

**APPROVED**: HAPI queries a DataStorage business logic endpoint for structured remediation history context before constructing the LLM investigation prompt. The context includes the full remediation chain for the target resource with effectiveness data, enabling the LLM to make informed decisions about whether to try something new, escalate, or re-apply a known remedy with justification.

---

## Design Principles

### 1. Separation of Responsibilities

| Component | Responsibility |
|-----------|---------------|
| **EM** (DD-017) | Assesses effectiveness, emits structured audit events |
| **DS** | Owns data intelligence: queries audit traces, correlates events, performs hash comparison, classifies tiers, returns structured response |
| **HAPI** | Owns prompt construction: calls one DS endpoint, formats the response into the LLM prompt, adds reasoning guidance |
| **LLM** | Owns investigation and decision-making: uses history context + its own signal investigation to determine the right course of action |

### 2. Reasoning Framework, Not Investigation Checklists

The prompt tells the LLM HOW to reason about repeated remediation, not WHAT specific factors to check. The LLM determines what contextual factors are relevant based on the signal type and source adapter.

**Good** (reasoning framework):
> "If the same remediation was applied without resolving the signal, investigate contextual factors relevant to this signal type and source to determine whether the root cause is external or internal to the workload."

**Bad** (investigation checklist):
> "Check network traffic, memory patterns, and cron job schedules."

The LLM's value is in adapting its investigation to the specific signal and context. Source adapter metadata (Prometheus labels, alert annotations, K8s event reason) naturally guides what the LLM investigates.

### 3. Full Remediation Chain, Not Just Most Recent

Always return the complete chain of remediations for the target resource within the query window. Individual entries are data points; the chain tells the story:

- Pattern of attempts (same remediation repeated vs. different approaches)
- Declining effectiveness (0.4 → 0.3 → 0.2 = diminishing returns)
- Escalation history (chain ended in human review = already triaged)
- Resolution context (what finally worked, if anything)

### 4. LLM-Driven Decision Making, Not Policy-Driven

No Rego rules, no hardcoded escalation thresholds. The LLM receives the history and reasoning guidance, then decides. This preserves the RCA value — the LLM investigates root cause rather than a policy short-circuiting the decision.

---

## Architecture

### Data Flow

```
New Signal → SP → AA Controller → HAPI
                                     │
                                     ▼
                              ┌─────────────┐
                              │ HAPI reads   │
                              │ current      │
                              │ target .spec │
                              │ → computes   │
                              │   hash       │
                              └──────┬───────┘
                                     │
                                     ▼
                              ┌─────────────┐
                              │ HAPI calls   │
                              │ DS endpoint  │
                              │ with target  │
                              │ + hash       │
                              └──────┬───────┘
                                     │
                                     ▼
                              ┌─────────────┐
                              │ DS queries   │
                              │ audit traces │
                              │ correlates   │
                              │ RO + EM      │
                              │ events by    │
                              │ RR UID       │
                              └──────┬───────┘
                                     │
                                     ▼
                              ┌─────────────┐
                              │ DS performs  │
                              │ three-way    │
                              │ hash match   │
                              │ tier classif │
                              └──────┬───────┘
                                     │
                                     ▼
                              ┌─────────────┐
                              │ DS returns   │
                              │ structured   │
                              │ response     │
                              └──────┬───────┘
                                     │
                                     ▼
                              ┌─────────────┐
                              │ HAPI formats │
                              │ prompt with  │
                              │ history +    │
                              │ reasoning    │
                              │ guidance     │
                              └──────┬───────┘
                                     │
                                     ▼
                                   LLM
                              (investigation
                               + decision)
```

### DataStorage Business Logic Endpoint

DS exposes a dedicated endpoint that encapsulates all the complexity of querying, correlating, and classifying remediation history. HAPI calls one endpoint and receives a ready-to-use response.

**Request**:

```
GET /api/v1/remediation-history/context
    ?targetKind=Deployment
    &targetName=my-app
    &targetNamespace=prod
    &currentSpecHash=sha256:aabb1122...
    &tier1Window=24h
    &tier2Window=90d
```

**Response**:

```json
{
    "targetResource": "Deployment/prod/my-app",
    "currentSpecHash": "sha256:aabb1122...",
    "regressionDetected": true,
    "tier1": {
        "window": "24h",
        "chain": [
            {
                "remediationUID": "rr-abc",
                "signalFingerprint": "fp-123",
                "signalType": "HighCPULoad",
                "workflowType": "ScaleUp",
                "outcome": "Success",
                "effectivenessScore": 0.4,
                "signalResolved": false,
                "hashMatch": "postRemediation",
                "preRemediationSpecHash": "sha256:aabb1122...",
                "postRemediationSpecHash": "sha256:ccdd3344...",
                "healthChecks": {
                    "podRunning": true,
                    "readinessPass": true,
                    "restartDelta": 0,
                    "crashLoops": false,
                    "oomKilled": false,
                    "pendingCount": 0
                },
                "metricDeltas": {
                    "cpuBefore": 0.95,
                    "cpuAfter": 0.92,
                    "memoryBefore": 0.60,
                    "memoryAfter": 0.62,
                    "latencyP95BeforeMs": 200,
                    "latencyP95AfterMs": 195,
                    "errorRateBefore": 0.02,
                    "errorRateAfter": 0.019
                },
                "sideEffects": [],
                "completedAt": "2026-02-05T08:00:00Z",
                "assessedAt": "2026-02-05T08:05:00Z"
            },
            {
                "remediationUID": "rr-def",
                "signalFingerprint": "fp-123",
                "signalType": "HighCPULoad",
                "workflowType": "ScaleUp",
                "outcome": "Success",
                "effectivenessScore": 0.3,
                "signalResolved": false,
                "hashMatch": "none",
                "preRemediationSpecHash": "sha256:ccdd3344...",
                "postRemediationSpecHash": "sha256:eeff5566...",
                "healthChecks": {
                    "podRunning": true,
                    "readinessPass": true,
                    "restartDelta": 0,
                    "crashLoops": false,
                    "oomKilled": false,
                    "pendingCount": 0
                },
                "metricDeltas": {
                    "cpuBefore": 0.92,
                    "cpuAfter": 0.90,
                    "memoryBefore": 0.62,
                    "memoryAfter": 0.63,
                    "latencyP95BeforeMs": 195,
                    "latencyP95AfterMs": 193,
                    "errorRateBefore": 0.019,
                    "errorRateAfter": 0.018
                },
                "sideEffects": [],
                "completedAt": "2026-02-05T12:00:00Z",
                "assessedAt": "2026-02-05T12:05:00Z"
            }
        ]
    },
    "tier2": {
        "window": "90d",
        "chain": [
            {
                "remediationUID": "rr-old-001",
                "signalType": "HighCPULoad",
                "workflowType": "ScaleUp",
                "outcome": "Success",
                "effectivenessScore": 0.4,
                "signalResolved": false,
                "hashMatch": "preRemediation",
                "completedAt": "2026-01-15T10:00:00Z"
            },
            {
                "remediationUID": "rr-old-002",
                "signalType": "HighCPULoad",
                "workflowType": "RestartPod",
                "outcome": "Success",
                "effectivenessScore": 0.2,
                "signalResolved": false,
                "hashMatch": "none",
                "completedAt": "2026-01-15T14:00:00Z"
            },
            {
                "remediationUID": "rr-old-003",
                "signalType": "HighCPULoad",
                "workflowType": null,
                "outcome": "Escalated",
                "effectivenessScore": null,
                "signalResolved": true,
                "hashMatch": "none",
                "completedAt": "2026-01-15T16:00:00Z"
            }
        ]
    }
}
```

### DS Internal Logic

DS performs the following when serving this endpoint:

1. **Query Tier 1 — RO events**: Query `remediation.workflow_created` audit events by `target_resource` (JSONB expression index `idx_audit_events_target_resource`) within the Tier 1 time window (default 24h). These events provide the remediation chain skeleton: `correlation_id` (RR name), `pre_remediation_spec_hash`, `workflow_type`, `outcome`, `signal_type`, `signal_fingerprint`.
2. **Query Tier 1 — EM component events**: For each RO event's `correlation_id`, batch-query EM component events (`event_category = 'effectiveness'`). The EM emits component-level audit events per ADR-EM-001 v1.3:
   - `effectiveness.health.assessed` — health score + typed `health_checks` sub-object (`pod_running`, `readiness_pass`, `restart_delta`, `crash_loops`, `oom_killed`, `pending_count`)
   - `effectiveness.alert.assessed` — alert score + typed `alert_resolution` sub-object (`alert_resolved`, `active_count`, `resolution_time_seconds`)
   - `effectiveness.metrics.assessed` — metrics score + typed `metric_deltas` sub-object (`cpu_before/after`, `memory_before/after`, `latency_p95_before/after_ms`, `error_rate_before/after`)
   - `effectiveness.hash.computed` — `pre_remediation_spec_hash`, `post_remediation_spec_hash`, `hash_match` (boolean)
   - `effectiveness.assessment.completed` — lifecycle marker with `reason` ("full", "partial", "expired")
3. **Correlate**: Join RO and EM events by `correlation_id` (the RemediationRequest name). For each RO event, compute the weighted effectiveness score using `ComputeWeightedScore()` from `effectiveness_handler.go` (DD-017 v2.1 formula). Read `signalResolved` from `alert_resolution.alert_resolved`. Read `healthChecks` and `metricDeltas` from the typed ogen sub-objects on the corresponding EM component events.
4. **Query Tier 2**: If `regressionDetected` (any entry's `preRemediationSpecHash` matches `currentSpecHash`), search audit events beyond Tier 1 (up to 90 days) for any `preRemediationSpecHash` matching `currentSpecHash` using expression index `idx_audit_events_pre_remediation_spec_hash`. Return the full chain from that historical window in summary form.
5. **Hash comparison**: For each remediation record, compare `currentSpecHash` against both `preRemediationSpecHash` and `postRemediationSpecHash`. Tag each record with `hashMatch`: `"preRemediation"`, `"postRemediation"`, or `"none"`. The `preRemediationSpecHash` match signals configuration regression.
6. **Regression detection**: Set `regressionDetected: true` if any record's `preRemediationSpecHash` matches `currentSpecHash` — the target resource has been reverted to a configuration that previously caused issues.
7. **Order chronologically**: Both Tier 1 and Tier 2 chains are ordered by `completedAt` ascending.

DS maintains expression indexes on `pre_remediation_spec_hash`, `target_resource` within the audit table for query performance (migration 027).

---

## Two-Tier Query Design

### Tier 1: Recent History (24h, Detailed)

- **Purpose**: Full effectiveness data for recent remediations on the same target
- **Query key**: Target resource (GVK + name + namespace) + 24h window
- **Additional filter**: Signal fingerprint (for exact recurrence detection)
- **Returns**: Full remediation chain with all fields — effectiveness scores, metric deltas, health checks, dual hashes
- **Prompt framing**: Strong signal — "these remediations were attempted recently"

### Tier 2: Historical Hash Lookup (Beyond 24h, Summary)

- **Purpose**: Detect configuration regressions that cross the 24h TTL boundary
- **Query key**: `preRemediationSpecHash` match against `currentSpecHash`, up to 90 days
- **Returns**: Full remediation chain from the matched historical window, in summary form (no metric deltas or health check details)
- **Prompt framing**: Softer lead — "this configuration was previously observed N days ago, here is the complete remediation sequence that followed"
- **Lookback cap**: 90 days (hardcoded, can be made configurable if needed)

### Fallthrough Logic

1. HAPI calls DS with target resource + current spec hash
2. DS runs Tier 1 query → if results found, return them
3. DS runs Tier 2 query → if `preRemediationSpecHash` match found beyond 24h, return that chain
4. If no matches in either tier → return empty response (fresh investigation, no history context injected into prompt)

---

## Three-Way Spec Hash Comparison

For each remediation record in the chain, DS compares `currentSpecHash` against both stored hashes:

| Current Hash Matches | Meaning | Prompt Implication |
|---------------------|---------|-------------------|
| `postRemediationSpecHash` | Config unchanged since our remediation | History is directly relevant — our changes are still active |
| `preRemediationSpecHash` | Configuration regression — resource reverted to the state that caused the original signal | Predictive signal — the same config caused the same problem before, and the remediation applied at that time had a known effectiveness |
| Neither | Config was changed to something we haven't seen | Tier 1: other records in the chain may still match. Tier 2: if no match in either tier, fresh investigation |

The `preRemediationSpecHash` match is the most powerful signal. It tells the LLM: "We've seen this exact configuration before. Here's what happened. The remediation applied was [X] with effectiveness [Y]. Don't repeat what didn't work."

---

## Prompt Construction

### HAPI Prompt Template

HAPI formats the DS response into a prompt section injected before the LLM investigation:

**Tier 1 (recent, detailed)**:

```
## Remediation History for Deployment/prod/my-app (last 24h)

1. [6h ago] ScaleUp (3→5 replicas) - Outcome: Success
   - Effectiveness: 0.4/1.0 (LOW)
   - Health: Pod running, readiness passing, no crashes
   - Metrics: CPU 95% → 92% (minimal improvement), latency unchanged
   - Signal resolved: NO (HighCPULoad still firing 45min later)
   - Side effects: None
   - Target config: UNCHANGED since remediation

2. [2h ago] ScaleUp (5→7 replicas) - Outcome: Success
   - Effectiveness: 0.3/1.0 (LOW, declining)
   - Health: Pod running, readiness passing, no crashes
   - Metrics: CPU 92% → 90% (diminishing returns), latency unchanged
   - Signal resolved: NO (HighCPULoad still firing 30min later)
   - Side effects: None
   - Target config: CHANGED (different from current)

Two ScaleUp remediations with declining effectiveness (0.4 → 0.3) and the
signal unresolved. Investigate contextual factors relevant to this signal type
and source to determine whether the root cause is external or internal to
the workload before recommending the same approach.
```

**Tier 2 (historical, summary)**:

```
## Historical Context: Configuration Previously Observed

WARNING: Current resource configuration (spec hash: aabb1122) matches a
PREVIOUS state observed 21 days ago that triggered the same signal type.

Remediation sequence from that occurrence:
1. [21 days ago] ScaleUp - Effectiveness: 0.4 - Signal NOT resolved
2. [21 days ago] RestartPod - Effectiveness: 0.2 - Signal NOT resolved
3. [21 days ago] Escalated to human review - Signal eventually resolved

The previous remediation sequence for this exact configuration was
ineffective (ScaleUp and RestartPod both failed). The issue required
human intervention. Consider this historical context when investigating,
though note the environment may have changed since then.
```

**Regression warning** (when `regressionDetected: true`):

```
CONFIGURATION REGRESSION DETECTED: The current resource spec matches a
previous pre-remediation state. This configuration has caused issues before.
```

### Reasoning Guidance (Always Included When History Exists)

```
If the same remediation type was applied without resolving the underlying
signal, investigate contextual factors relevant to this signal type and
source to determine whether the root cause is external or internal to the
workload. Use available observability data to correlate before recommending
the same remediation again.
```

---

## V1.1 Enhancement Path

When DD-017 Level 2 (AI-Powered Analysis via HolmesGPT PostExec) arrives in V1.1, the EM audit events gain additional fields:

- `root_cause_resolved: true/false`
- `lessons_learned: [...]`
- `oscillation_detected: true/false`

DS reads these richer fields from the same audit traces. The DS endpoint response gains corresponding fields. HAPI formats them into the prompt:

```
1. [6h ago] ScaleUp - Effectiveness: 0.4
   - Root cause resolved: NO (problem masked, not fixed)
   - Lesson: "Memory increase masked a memory leak. Throughput unchanged
     confirms leak persists."
   - Oscillation: None detected
```

No architectural change to HAPI, DS endpoint contract, or query design. The enhancement is purely additive.

---

## Consequences

### Positive

- LLM makes remediation decisions informed by full history — avoids repeating failures
- Configuration regression detection catches rollback scenarios that would otherwise start from zero
- Full remediation chain provides pattern visibility (declining effectiveness, escalation history)
- Clean separation: DS owns data intelligence, HAPI owns prompt construction, LLM owns decision-making
- Prompt reasoning framework preserves LLM flexibility — no hardcoded investigation paths
- Two-tier design balances detail (24h) with long-term memory (90 days)
- V1.1 enhancement path is purely additive — no architectural changes

### Negative

- DS endpoint adds complexity to the DataStorage service
  - **Mitigation**: DS already handles audit queries; this is a specialized aggregation view
- Prompt grows larger when history exists (additional tokens)
  - **Mitigation**: In practice, chains are short (5-10 entries max per target within 24h given cooldown periods). Token cost is minimal.
- Tier 2 query (90-day hash scan) could be slow at scale
  - **Mitigation**: DS indexes hash columns; capped at 90 days; summary-only response (no metric deltas)

---

## Related Decisions

- **DD-017 v2.0**: Effectiveness Monitor V1.0 Level 1 (provides the audit data this DD consumes)
- **DD-EFFECTIVENESS-001**: Hybrid Automated + AI Analysis approach (Level 1/Level 2 architecture)
- **DD-EVENT-001**: Controller Event Registry (event reason constants alignment)

---

## Review & Evolution

### When to Revisit

- **MANDATORY**: When DD-017 Level 2 is implemented — add root cause / lessons / oscillation fields to prompt template
- **OPTIONAL**: If Tier 2 query performance degrades — evaluate materialized view or dedicated index
- **OPTIONAL**: If prompt token budget becomes a concern — evaluate chain truncation or summarization

---

**Status**: ✅ APPROVED — V1.0
**Next Review**: When DD-017 Level 2 implementation begins (estimated V1.1, Q2 2026)

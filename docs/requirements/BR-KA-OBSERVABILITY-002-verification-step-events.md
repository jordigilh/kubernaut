# BR-KA-OBSERVABILITY-002: Verification Step Events for Console Activity Log

**Business Requirement ID**: BR-KA-OBSERVABILITY-002
**Category**: KA (Kubernaut Agent) / AF (API Frontend)
**Priority**: P1
**Target Version**: V1.6
**Status**: Approved
**Date**: 2026-06-15
**Issue**: [#1427](https://github.com/jordigilh/kubernaut/issues/1427)
**Dependency**: [#1426](https://github.com/jordigilh/kubernaut/issues/1426) (phase-level metadata)

---

## Business Need

### Problem Statement

During the Verifying phase of a remediation lifecycle, the Console UI shows a VerificationTimer card with a progress bar and countdown. However, operators have no visibility into *which* verification sub-checks the Effectiveness Monitor (EM) is performing or whether any check is stuck. The phase-level `Verifying` status is a single opaque state that can last minutes, providing no intermediate feedback.

### Business Impact

| Stakeholder | Gap | Impact |
|---|---|---|
| SRE / Operator | Cannot see if EM is stuck on alert decay | Unnecessary escalation or false confidence |
| Console team | Cannot render activity log inside VerificationTimer | Degraded UX during critical remediation validation |
| Audit / Compliance | No granular audit trail of verification progress | FedRAMP SI-4 observability gap during assessment |

### FedRAMP Control Mapping

| Control | Objective | How This BR Serves It |
|---------|-----------|----------------------|
| **SI-4** | Information System Monitoring | Each EM sub-check (health, alert, hash, metrics) is observable in real-time via typed events |
| **AU-3** | Content of Audit Records | Structured metadata (`step`, `step_status`, `detail`, `elapsed_s`) creates parseable audit records |
| **AU-12** | Audit Generation | Events generated at source (HandleWatch) and flow through EventBridge — proven by IT tests |
| **SI-7** | Software/Information Integrity | `spec_hash_computed` step proves remediated resource spec hasn't drifted |

---

## Requirements

### BR-KA-OBSERVABILITY-002.1: Emit verification_step status-update events

The AF MUST emit `verification_step` status-update events via the A2A EventBridge when the EffectivenessAssessment CRD status changes during the Verifying phase.

Each event MUST contain the following metadata fields:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | mandatory | `"verification_step"` |
| `step` | string | mandatory | Step identifier (see 002.2) |
| `step_status` | string | mandatory | `completed` \| `in_progress` \| `failed` |
| `detail` | string | mandatory | Human-readable context for Console rendering |
| `elapsed_s` | integer | mandatory | Seconds since the Verifying phase started |

RR context fields (`rr_id`, `phase`, `namespace`, `kind`, `target`, `alert_name`) are auto-merged by the EventBridge via `mergeRRContext`.

### BR-KA-OBSERVABILITY-002.2: Step names and mapping

The following step names MUST be emitted based on EA CRD status transitions:

| Step Name | EA Trigger | step_status |
|-----------|------------|-------------|
| `stabilization_elapsed` | Phase: Stabilizing/Pending -> Assessing | `completed` |
| `spec_hash_computed` | `Components.HashComputed` false -> true | `completed` |
| `alert_check` | `Components.AlertDecayRetries` increase (not yet assessed) | `in_progress` |
| `alert_check` | `Components.AlertAssessed` false -> true | `completed` |
| `health_check` | `Components.HealthAssessed` false -> true | `completed` |
| `metrics_check` | `Components.MetricsAssessed` false -> true | `completed` |
| `phase_transition` | Other EA phase changes (e.g., -> Failed) | `completed` or `failed` |

### BR-KA-OBSERVABILITY-002.3: Graceful degradation

If the EA watch cannot be established (RBAC, timing, API unavailability), the AF MUST continue emitting phase-level status updates for the RR. The absence of `verification_step` events MUST NOT block the remediation lifecycle. The failure MUST be logged at V(1) verbosity.

### BR-KA-OBSERVABILITY-002.4: Signal name in alert detail

When `step` is `alert_check` and `step_status` is `in_progress`, the `detail` field MUST include the signal name from `EA.Spec.SignalName` when available (e.g., "Waiting for KubePodCrashLooping to clear (retry 2)").

### BR-KA-OBSERVABILITY-002.5: Spec hash in detail

When `step` is `spec_hash_computed`, the `detail` field MUST include a truncated hash excerpt (first 12 characters) when `PostRemediationSpecHash` is available.

---

## Acceptance Criteria

- [ ] `verification_step` events emitted through A2A EventBridge during Verifying phase
- [ ] All mandatory metadata fields present: `step`, `step_status`, `detail`, `elapsed_s`
- [ ] `stabilization_elapsed` step name used for Stabilizing->Assessing (not generic `phase_transition`)
- [ ] Alert decay retries emit `alert_check` with `step_status: "in_progress"`
- [ ] Signal name included in alert_check detail when available
- [ ] EA watch failure degrades gracefully (RR watch continues)
- [ ] UT tests prove DiffEASteps logic (UT-AF-1427-001..015)
- [ ] IT tests prove HandleWatch -> EA watch -> EventBridge wiring (IT-AF-1427-001..004)

---

## Test Coverage

| Test ID | Tier | FedRAMP | What It Proves |
|---------|------|---------|----------------|
| UT-AF-1427-001 | UT | SI-4 | health_check completion with step_status and detail |
| UT-AF-1427-007 | UT | SI-4, AU-3 | alert_check in_progress with signal name |
| UT-AF-1427-010 | UT | SI-4 | stabilization_elapsed step name mapping |
| UT-AF-1427-012 | UT | SI-4 | Failed phase produces step_status=failed |
| UT-AF-1427-013 | UT | AU-3 | in_progress suppressed when completed fires |
| IT-AF-1427-001 | IT | AU-3, SI-4 | step_status and detail flow through production wiring |
| IT-AF-1427-002 | IT | AU-12 | elapsed_s generated at source and present in metadata |
| IT-AF-1427-003 | IT | SI-4 | stabilization_elapsed emitted (not phase_transition) |
| IT-AF-1427-004 | IT | SI-4, AU-3 | alert decay emits alert_check in_progress with signal name |

---

## Implementation

- **Production code**: `pkg/apifrontend/tools/verification_step.go`, `pkg/apifrontend/tools/crd_tools.go`
- **Event bridge**: `pkg/apifrontend/launcher/event_bridge.go` (`MetaTypeVerificationStep`)
- **EA CRD types**: `api/effectivenessassessment/v1alpha1/effectivenessassessment_types.go`

---

## Related Requirements

- **BR-EM-009** (Derived Timing Observability): `verification_step` extends sub-step granularity beyond phase-level timing
- **BR-KA-OBSERVABILITY-001** (Agent Prometheus Metrics): Complements session/request metrics with event-level verification observability
- **DD-AUDIT-003** (Service Audit Trace): `verification_step` is the AF-side streaming counterpart of EM-side audit events (`effectiveness.health.assessed`, etc.)
- **BR-ORCH-044** (Operational Observability): Phase transition metrics including Verifying — `verification_step` adds sub-phase resolution

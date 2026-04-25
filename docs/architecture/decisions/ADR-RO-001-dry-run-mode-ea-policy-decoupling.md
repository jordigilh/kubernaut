# ADR-RO-001: Dry-Run Mode — EA Policy Decoupling from Remediation Pipeline

**Status**: ACCEPTED
**Date**: 2026-04-24
**Issue**: [#712](https://github.com/jordigilh/kubernaut/issues/712), [#736](https://github.com/jordigilh/kubernaut/issues/736)
**Related**: [#116](https://github.com/jordigilh/kubernaut/issues/116) (v1.5 interactive dry-run)

## Context

After AI analysis completes, the Remediation Orchestrator (RO) pipeline creates WorkflowExecution (WFE) and EffectivenessAssessment (EA) CRDs for execution and verification. Operators evaluating Kubernaut in production need a way to observe the AI's analysis decisions — which workflows it would select, at what confidence level — without actually executing remediation on the cluster.

Issue #736 AC-1 requires documenting the EA policy decoupling from the workflow engine, enabling scenarios where the pipeline stops after AI analysis without entering the Verifying phase.

### Alternatives Considered

1. **Per-workflow dry-run via RAR interactive flow** — Deferred to v1.5 (#116). Requires RAR CRD changes and UI integration.
2. **Non-K8s target detection as skip condition** — Deferred to the milestone where Goose recipe support lands. Today all workflows target K8s resources.
3. **Config-driven EA `Enabled` opt-out** — Rejected. Creates temporal coupling between the skip evaluation site and EA creation. Risks silent misconfiguration where an operator disables EA without realizing the feedback loop is broken.
4. **WFE-level dry-run flag** — Rejected. The WFE CRD has no dry-run concept today. Creating a WFE that doesn't execute is semantically misleading and would require changes to the workflow execution engine.

## Decision

### Global Dry-Run Toggle (v1.4)

A top-level `dryRun: true` flag in the RO's service configuration YAML enables dry-run mode. When enabled, the pipeline stops after AI analysis completes — no WFE, RAR, or EA CRDs are created. The RemediationRequest completes immediately with outcome `DryRun`.

### Intercept Point

Dry-run intercepts in `AnalyzingHandler.handleCompleted`, after the `IsWorkflowNotNeeded` and `NeedsHumanReview` checks but before the approval and direct-execution paths. This ensures:

- If the AI determines no action is needed (`WorkflowNotNeeded`), the RR completes with `NoActionRequired` regardless of dry-run — dry-run does not shadow the AI's determination.
- If the AI determines manual review is needed with no workflow, the RR completes with `ManualReviewRequired` regardless of dry-run.
- Dry-run only intercepts the paths that **would create execution artifacts** (WFE via direct execution, RAR via approval flow).

### Gateway Infinite-Loop Suppression

Dry-run RRs complete immediately, but the triggering alert may still be firing. Without suppression, the Gateway would create a new RR for the same alert, which would also complete immediately — an infinite loop.

The `NextAllowedExecution` field on the terminal RR is set to `now + dryRunHoldPeriod` (default: 1 hour, minimum: 5 minutes, maximum: 7 days). The Gateway's existing `ShouldDeduplicate` / `PhaseBasedDeduplicationChecker` logic reads this field and suppresses new RR creation for the same signal fingerprint until the hold period expires.

### Failure History Preservation

Dry-run does NOT reset `ConsecutiveFailureCount` or clear existing failure-based `NextAllowedExecution` backoff. Dry-run did not prove the remediation works — it only stopped execution. The failure history from real executions must be preserved. If the existing failure backoff sets a `NextAllowedExecution` later than the dry-run hold period, the later value is preserved.

### KA Recurring Detection Exclusion

The `DryRun` outcome is excluded from the Kubernaut Agent's `completedOutcomes` map used for recurring detection. Dry-run RRs must not trigger false "this remediation keeps recurring" escalation warnings, since no remediation was actually executed.

### Metrics

Dry-run completions are tracked via the existing `NoActionNeededTotal` counter with `reason="DryRun"`, alongside the standard `PhaseTransitionsTotal{from_phase="Analyzing", to_phase="Completed"}`. No new metric registration is needed.

### Configuration

```yaml
dryRun: true
dryRunHoldPeriod: 1h  # min: 5m, max: 168h (7d)
```

Configuration is read once at startup via `LoadFromFile`. Toggling dry-run requires a pod restart. `Config.Validate()` enforces `dryRunHoldPeriod >= 5m` when `dryRun` is true.

Future: In OCP deployments, dry-run will be exposed via the Kubernaut CR so the operator reconciles changes through the standard K8s admission pipeline (RBAC, audit, webhooks). A separate issue will track adding hot-reload for the RO service config.

## Consequences

### Positive

- Operators can observe AI decisions in production without risk of unintended remediation
- Zero impact on existing behavior when `dryRun: false` (default)
- Reuses existing Gateway deduplication and metric infrastructure
- The `TransitionCompletedWithoutVerification` type is generic enough for v1.5 reuse (#116)
- `IsDryRun` callback follows the established `AnalyzingCallbacks` pattern — no new interfaces or handler modifications needed beyond the analyzing handler

### Negative

- Global blast radius: `dryRun: true` affects all RRs cluster-wide (no per-namespace granularity in v1.4)
- Effectiveness data gap: dry-run signals have no EA verification data, breaking the feedback loop for those signals
- Pod restart required to toggle (until hot-reload is implemented)

### Risks

- `DryRunHoldPeriod` too short (< 5m) could cause Gateway flooding. Mitigated by config validation with a 5-minute floor.
- Operator forgets dry-run is enabled. Mitigated by startup log message and `NoActionNeededTotal{reason="DryRun"}` metric alerting.

## References

- [ADR-EM-001](ADR-EM-001-effectiveness-monitor-service-integration.md) — EA creation and verification pipeline
- [ADR-062](ADR-062-phase-handler-registry-pattern.md) — Phase handler registry and TransitionIntent pattern
- Issue [#116](https://github.com/jordigilh/kubernaut/issues/116) — v1.5 interactive dry-run via RAR

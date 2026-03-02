# PR: feature/v1.0-remaining-bugs-demos — Scope & PR Description

**Branch**: `feature/v1.0-remaining-bugs-demos`  
**Base**: `main` (rebased on `origin/main`)

**Agreed scope for this branch**: **P0** (security), **P1** (bugs), **P3** (test hygiene + demo validation + VHS). P2 and P4 postponed; v1.0 vs v1.1 for those to be decided later.

---

## Branch focus (order of work)

1. **Fix remaining bugs** — P0 + P1 (see below).
2. **Run all demo scenarios** — Ensure no regressions across existing scenarios.
3. **Record VHS files** — Capture terminal recordings for demo scenarios (see #101).
4. **P2/P4** — Not in this branch; decide later which land in v1.0 vs v1.1.

---

## In-scope issues (tracked in this PR)

*P0 + P1 + P3 only. Close issues when fixed.*

### P0 — Security

| # | Title |
|---|--------|
| [#229](https://github.com/jordigilh/kubernaut/issues/229) | security(rbac): Tighten cluster-wide RBAC + consolidate notification namespace into kubernaut-system |
| [#204](https://github.com/jordigilh/kubernaut/issues/204) | sec(helm): Move hardcoded PostgreSQL/Redis credentials from values.yaml to Kubernetes Secrets |

### P1 — Bugs (fix after P0)

| # | Title |
|---|--------|
| [#240](https://github.com/jordigilh/kubernaut/issues/240) | Guard EA creation to only fire after successful WorkflowExecution |
| [#205](https://github.com/jordigilh/kubernaut/issues/205) | bug(ro): EffectivenessAssessment created for Failed/ManualReviewRequired RR with no WFE |
| [#242](https://github.com/jordigilh/kubernaut/issues/242) | Gateway: enforce exponential backoff cooldown before creating new RRs |
| [#246](https://github.com/jordigilh/kubernaut/issues/246) | bug(ea): EffectivenessAssessment healthScore incorrectly reports non-1.0 when all pods are healthy |
| [#227](https://github.com/jordigilh/kubernaut/issues/227) | Bug: K8s Event adapter fallback fingerprint includes event reason, breaking cross-adapter deduplication |
| [#230](https://github.com/jordigilh/kubernaut/issues/230) | GW-DEDUP-002: FlakeAttempts pollute shared gatewayNamespace with stale RRs |
| [#214](https://github.com/jordigilh/kubernaut/issues/214) | bug(ro): CheckConsecutiveFailures ignores completed-but-ineffective remediations (memory-escalation design gap) |
| [#211](https://github.com/jordigilh/kubernaut/issues/211) | DS effectiveness query non-deterministic ordering causes intermittent spec_drift misclassification |

### P3 — Test hygiene & demos (after bugs)

| # | Title |
|---|--------|
| [#194](https://github.com/jordigilh/kubernaut/issues/194) | Test hygiene: Remove stale Skip/PIt placeholders and implement missing test coverage |
| [#144](https://github.com/jordigilh/kubernaut/issues/144) | test(demo): Validate all 17 demo scenarios end-to-end before recording |
| [#101](https://github.com/jordigilh/kubernaut/issues/101) | feat(demo): Create VHS terminal recordings for all 17 demo scenarios |

---

## Remaining v1.0 issues by priority

***In this branch**: P0, P1, P3. **Postponed** (v1.0 vs v1.1 later): P2, P4.*

### P0 — Security / must-fix for v1.0

| # | Title | Labels |
|---|--------|--------|
| [#229](https://github.com/jordigilh/kubernaut/issues/229) | security(rbac): Tighten cluster-wide RBAC + consolidate notification namespace into kubernaut-system | bug |
| [#204](https://github.com/jordigilh/kubernaut/issues/204) | sec(helm): Move hardcoded PostgreSQL/Redis credentials from values.yaml to Kubernetes Secrets | v1.0 |

### P1 — High-priority bugs / correctness

| # | Title | Labels |
|---|--------|--------|
| [#240](https://github.com/jordigilh/kubernaut/issues/240) | Guard EA creation to only fire after successful WorkflowExecution | bug, team: remediationorchestrator |
| [#205](https://github.com/jordigilh/kubernaut/issues/205) | bug(ro): EffectivenessAssessment created for Failed/ManualReviewRequired RR with no WFE | team: remediationorchestrator |
| [#242](https://github.com/jordigilh/kubernaut/issues/242) | Gateway: enforce exponential backoff cooldown before creating new RRs | bug, team: gateway |
| [#246](https://github.com/jordigilh/kubernaut/issues/246) | bug(ea): EffectivenessAssessment healthScore incorrectly reports non-1.0 when all pods are healthy | bug, team: effectivenessmonitor |
| [#227](https://github.com/jordigilh/kubernaut/issues/227) | Bug: K8s Event adapter fallback fingerprint includes event reason, breaking cross-adapter deduplication | bug, team: gateway |
| [#230](https://github.com/jordigilh/kubernaut/issues/230) | GW-DEDUP-002: FlakeAttempts pollute shared gatewayNamespace with stale RRs | bug |
| [#214](https://github.com/jordigilh/kubernaut/issues/214) | bug(ro): CheckConsecutiveFailures ignores completed-but-ineffective remediations (memory-escalation design gap) | bug |
| [#211](https://github.com/jordigilh/kubernaut/issues/211) | DS effectiveness query non-deterministic ordering causes intermittent spec_drift misclassification | — |

### P2 — priority: high enhancements (v1.0)

| # | Title | Labels |
|---|--------|--------|
| [#190](https://github.com/jordigilh/kubernaut/issues/190) | WE/RO: Skipped/Deduplicated phase with result inheritance from original WE | enhancement, priority: high, v1.0 |
| [#189](https://github.com/jordigilh/kubernaut/issues/189) | RO: Distributed locking for WE creation to prevent concurrent executions on same target | enhancement, priority: high, v1.0 |

### P3 — v1.0 test hygiene & demos

| # | Title | Labels |
|---|--------|--------|
| [#194](https://github.com/jordigilh/kubernaut/issues/194) | Test hygiene: Remove stale Skip/PIt placeholders and implement missing test coverage | team: datastorage, test, v1.0 |
| [#144](https://github.com/jordigilh/kubernaut/issues/144) | test(demo): Validate all 17 demo scenarios end-to-end before recording | test, v1.0, demo |
| [#101](https://github.com/jordigilh/kubernaut/issues/101) | feat(demo): Create VHS terminal recordings for all 17 demo scenarios | documentation, v1.0, demo |

### P4 — v1.0 enhancements (WE RBAC, demo scenarios, release)

| # | Title | Labels |
|---|--------|--------|
| [#187](https://github.com/jordigilh/kubernaut/issues/187) | WE RBAC: Add resourceNames instance-level scoping (depends on #184) | enhancement, v1.0, team: workflowexecution |
| [#186](https://github.com/jordigilh/kubernaut/issues/186) | Implement workflow-scoped RBAC via schema-declared permissions (DD-WE-005) | enhancement, v1.0, team: workflowexecution |
| [#185](https://github.com/jordigilh/kubernaut/issues/185) | Implement scoped RBAC per workflow execution (DD-WE-005) | enhancement, v1.0, team: workflowexecution |
| [#172](https://github.com/jordigilh/kubernaut/issues/172) | feat(demo): Concurrent cross-namespace — multi-tenant label-driven parallel remediation | enhancement, v1.0, demo |
| [#171](https://github.com/jordigilh/kubernaut/issues/171) | feat(demo): Resource quota exhaustion — policy constraint escalation | enhancement, v1.0, demo |
| [#170](https://github.com/jordigilh/kubernaut/issues/170) | feat(demo): Duplicate alert suppression — alert storm deduplication | enhancement, v1.0, demo |
| [#168](https://github.com/jordigilh/kubernaut/issues/168) | feat(demo): Memory escalation — LLM recognizes limits and escalates to human review | enhancement, v1.0, demo |
| [#122](https://github.com/jordigilh/kubernaut/issues/122) | feat(demo): Disk pressure / PVC full scenario with cleanup remediation | enhancement, v1.0, demo |
| [#80](https://github.com/jordigilh/kubernaut/issues/80) | Release: Helm chart creation, multi-arch images, and public publishing | enhancement, v1.0 |
| [#60](https://github.com/jordigilh/kubernaut/issues/60) | feat(notification): implement PagerDuty delivery channel | enhancement, v1.0 |
| [#105](https://github.com/jordigilh/kubernaut/issues/105) | feat(datastorage): OCI 1.1 subject/referrers integration for schema-to-execution-bundle relationship | enhancement, v1.0, team: datastorage |
| [#50](https://github.com/jordigilh/kubernaut/issues/50) | [Technical Debt] Refactor unit tests to Go idiomatic structure (black-box, colocated) | test, technical-debt, v1.0 |

### Open issues (no v1.0 label) — likely v1.1 or backlog

*Not listed above; include only if explicitly brought into v1.0.*

- #247, #245, #244, #243, #239, #238, #237, #236, #235, #233, #232, #231, #228, #226, #221, #210, #193, #184, #179, #177, #175, #162, #160, #157, #156, #155, #154, #153, #151, #150, and others.

---

## Copy-paste PR description (for GitHub)

```markdown
## Summary

Branch for **P0 (security), P1 (bugs), and P3 (test hygiene + demo validation + VHS)**. P2 and P4 postponed; v1.0 vs v1.1 for those later.

## Order of work

1. **P0** — #229 (RBAC), #204 (Helm credentials).
2. **P1** — Bugs (#240, #205, #242, #246, #227, #230, #214, #211).
3. **P3** — #194 (test hygiene), then #144 (validate 17 demos), #101 (VHS recordings).

## In-scope issues (P0 + P1 + P3)

### P0 — Security
- #229 security(rbac): Tighten RBAC + consolidate notification namespace
- #204 sec(helm): Move PostgreSQL/Redis credentials from values.yaml to Kubernetes Secrets

### P1 — Bugs
- #240 Guard EA creation to only fire after successful WorkflowExecution
- #205 EffectivenessAssessment created for Failed/ManualReviewRequired RR with no WFE
- #242 Gateway: enforce exponential backoff cooldown before creating new RRs
- #246 EffectivenessAssessment healthScore incorrect when all pods healthy
- #227 K8s Event adapter fallback fingerprint includes event reason (dedup)
- #230 GW-DEDUP-002: FlakeAttempts pollute shared gatewayNamespace
- #214 CheckConsecutiveFailures ignores completed-but-ineffective remediations
- #211 DS effectiveness query non-deterministic ordering (spec_drift)

### P3 — Test hygiene & demos
- #194 Test hygiene: Remove stale Skip/PIt placeholders and implement missing test coverage
- #144 test(demo): Validate all 17 demo scenarios end-to-end before recording
- #101 feat(demo): Create VHS terminal recordings for all 17 demo scenarios

## Reference

Full priority list (P2/P4 postponed): [PR_V1_REMAINING_BUGS_DEMOS_SCOPE.md](docs/handoff/PR_V1_REMAINING_BUGS_DEMOS_SCOPE.md)
```

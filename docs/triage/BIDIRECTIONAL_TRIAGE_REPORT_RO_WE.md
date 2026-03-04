# Bidirectional Triage Report: Remediation Orchestrator & Workflow Execution

**Date**: 2025-03-04  
**Scope**: Remediation Orchestrator Service, Workflow Execution Service  
**Directions**: Code → Docs, Docs → Code

---

## Remediation Orchestrator Service

### Code → Docs (things in code not documented or documented incorrectly)

| # | Category | Severity | File | Detail | Evidence |
|---|----------|----------|------|--------|----------|
| RO-C2D-1 | INCONSISTENCY | HIGH | `pkg/remediationorchestrator/routing/types.go` | BlockingCondition comment lists 5 BlockReasons but omits `UnmanagedResource` and `IneffectiveChain`. Code has 7 BlockReasons. | Lines 54-56: "Valid values: ConsecutiveFailures, DuplicateInProgress, ResourceBusy, RecentlyRemediated, ExponentialBackoff" |
| RO-C2D-2 | INCONSISTENCY | HIGH | `internal/controller/remediationorchestrator/blocking.go` vs `reconciler.go` | `HandleBlockedPhase` in reconciler.go (lines 2942-2998) contains BR-SCOPE-010 UnmanagedResource re-validation logic but is **never called**. The active `handleBlockedPhase` in blocking.go treats ALL time-based blocks the same (transition to Failed). UnmanagedResource blocks should re-validate scope per BR-SCOPE-010. | blocking.go:152 (used at reconciler.go:637); reconciler.go:2962-2965 (dead code) |
| RO-C2D-3 | INCONSISTENCY | MEDIUM | `pkg/remediationorchestrator/phase/types.go` | ValidTransitions map: `Blocked` → `{Failed}` only. Code also transitions Blocked → Analyzing (ResourceBusy clear) and Blocked → Pending (DuplicateInProgress clear) via `clearEventBasedBlock`. | phase/types.go:96; blocking.go:249, 289 |
| RO-C2D-4 | GAP-IN-DOCS | MEDIUM | `remediation-routing.md` | Blocked phase lifecycle: docs say "Time-based blocks... transition to **Failed** (terminal). Future RRs for the same signal can then proceed." For **RecentlyRemediated** and **ExponentialBackoff**, the design intent is that the *current* RR should **resume** (retry) after cooldown, not transition to Failed. Code treats all time-based blocks the same (→ Failed). Docs vs code alignment unclear. | remediation-routing.md:76-78; blocking.go:169-185 |
| RO-C2D-5 | GAP-IN-DOCS | LOW | `remediation-routing.md` | Blocked phase lifecycle for event-based blocks: docs say "Cleared" but don't specify resume phase. Code: ResourceBusy → Analyzing, DuplicateInProgress → Pending. | remediation-routing.md:79-82; blocking.go:249, 289 |

### Docs → Code (things in docs/BRs planned for v1.0 but not implemented or not wired)

| # | Category | Severity | File | Detail | Evidence |
|---|----------|----------|------|--------|----------|
| RO-D2C-1 | INCONSISTENCY | MEDIUM | `remediation-routing.md`, `crds.md` | Docs list "Rejected" as a terminal phase. Code has no `Rejected` phase; approval rejection transitions RR to **Failed** with rejection reason. | remediation-routing.md:49; phase/types.go (no Rejected); reconciler.go:1271-1306 |
| RO-D2C-2 | GAP-IN-CODE | HIGH | BR-SCOPE-010, ADR-053 | UnmanagedResource block expiry should re-validate scope (resume to Pending if now managed, re-block with backoff if still unmanaged). Correct logic exists in `HandleBlockedPhase` (reconciler.go) but is dead code. Active `handleBlockedPhase` (blocking.go) transitions to Failed. | reconciler.go:2962-2965, 3000-3040; blocking.go:169-185 |
| RO-D2C-3 | GAP-IN-DOCS | LOW | `crds.md` | RemediationRequest spec table shows `targetResource`, `signal`, `scope` with generic types. Actual API uses `ResourceIdentifier`, `SignalName`, `SignalFingerprint`, etc. Spec table is outdated. | crds.md:16-19; remediationrequest_types.go |

### BlockReasons and Check Order (Reference)

**Code order (pre-analysis)**: 1. UnmanagedResource, 2. ConsecutiveFailures, 3. DuplicateInProgress, 4. ExponentialBackoff  
**Code order (post-analysis)**: 1. UnmanagedResource, 2. ConsecutiveFailures, 3. DuplicateInProgress, 4. ResourceBusy, 5. RecentlyRemediated, 6. ExponentialBackoff, 7. IneffectiveChain (last, fail-open)

**All 7 BlockReasons** (api/remediation/v1alpha1/remediationrequest_types.go:144-180): ConsecutiveFailures, DuplicateInProgress, ResourceBusy, RecentlyRemediated, ExponentialBackoff, UnmanagedResource, IneffectiveChain

### Phase Transitions (Reference)

**ValidTransitions** (phase/types.go:89-102): Pending→Processing; Processing→{Analyzing,Failed,TimedOut}; Analyzing→{AwaitingApproval,Executing,Completed,Failed,TimedOut}; AwaitingApproval→{Executing,Failed,TimedOut}; Executing→{Completed,Failed,TimedOut,Skipped}; Blocked→Failed; Failed→Blocked

**Actual additional transitions** (not in map): Blocked→Analyzing (ResourceBusy clear), Blocked→Pending (DuplicateInProgress clear)

### Cooldown Defaults (Reference)

- ConsecutiveFailureCooldown: 1h (config, BR-ORCH-042)
- RecentlyRemediatedCooldown: 5m (config, DD-WE-001)
- Exponential backoff: Base=1m, Max=10m, MaxExponent=4 (DD-WE-004)

---

## Workflow Execution Service

### Code → Docs (things in code not documented or documented incorrectly)

| # | Category | Severity | File | Detail | Evidence |
|---|----------|----------|------|--------|----------|
| WE-C2D-1 | GAP-IN-DOCS | MEDIUM | `workflow-execution.md` | Default execution engine is **Tekton** (not documented). Code: `executionEngineWithDefault` returns "tekton" when empty; API has `+kubebuilder:default=tekton`. | pkg/remediationorchestrator/creator/workflowexecution.go:209-212; api/workflowexecution/v1alpha1/workflowexecution_types.go:177 |
| WE-C2D-2 | GAP-IN-DOCS | LOW | `workflow-execution.md` | Parameter injection table lists `NAMESPACE`, `RESOURCE_NAME`, `ALERT_NAME`. Code injects `TARGET_RESOURCE` (built-in) plus `wfe.Spec.Parameters` (workflow schema). Doc variable names may be from workflow schema examples, not built-ins. | workflow-execution.md:54-60; executor/job.go:284-298; executor/tekton.go:159 |
| WE-C2D-3 | GAP-IN-DOCS | LOW | `workflow-execution.md` | Dependency validation: docs correctly state Secrets and ConfigMaps. Code validates only Secrets and ConfigMaps (NOT ServiceAccounts). DD-WE-006 and BR-WE-014 align. | workflow-execution.md:42-47; pkg/datastorage/validation/dependency_validator.go; internal/controller/workflowexecution:218 |

### Docs → Code (things in docs/BRs planned for v1.0 but not implemented or not wired)

| # | Category | Severity | File | Detail | Evidence |
|---|----------|----------|------|--------|----------|
| WE-D2C-1 | INCONSISTENCY | MEDIUM | `crds.md` | WorkflowExecution status shows `jobRef`, `pipelineRunRef`. Actual API uses unified `executionRef` (LocalObjectReference). | crds.md:124-126; workflowexecution_types.go:256-258 |
| WE-D2C-2 | INCONSISTENCY | LOW | `crds.md` | RemediationRequest phases: "Rejected" listed; code uses Failed. | crds.md:38 |
| WE-D2C-3 | GAP-IN-DOCS | LOW | `workflow-execution.md` | Execution namespace `kubernaut-workflows` is mentioned in workflow schema docs but not explicitly in workflow-execution.md. Config default is documented in config. | pkg/workflowexecution/config/config.go:96; workflow-execution.md |

### Implementation Reference

- **Execution namespace**: `kubernaut-workflows` (config default, DD-WE-002)
- **Deterministic naming**: `wfe-<sha256(targetResource)[:16]>` (DD-WE-003, executor/tekton.go:270-272, executor/job.go:167)
- **Bundle resolver**: Tekton uses `bundles` resolver with OCI bundle (tekton.go:181-187)
- **Dependency validation**: Secrets and ConfigMaps only; validated at DS registration and WE reconcilePending (DD-WE-006)
- **Job vs Tekton**: Both supported; Tekton is default

---

## Helm Values

| Service | Finding | Detail |
|---------|---------|--------|
| remediationorchestrator | values.yaml | `remediationorchestrator.config` has `effectivenessAssessment` and `asyncPropagation` but not `routing`. Routing config is in chart template (remediationorchestrator.yaml) with defaults. |
| workflowexecution | values.yaml | Minimal config (resources only). Execution namespace from config default, not values. |

---

## Summary

| Direction | HIGH | MEDIUM | LOW |
|-----------|------|--------|-----|
| RO Code→Docs | 2 | 2 | 1 |
| RO Docs→Code | 1 | 2 | 1 |
| WE Code→Docs | 0 | 1 | 2 |
| WE Docs→Code | 0 | 1 | 2 |

**Priority fixes**:
1. **RO-D2C-2 / RO-C2D-2**: Wire UnmanagedResource re-validation on block expiry (use HandleBlockedPhase logic or merge into blocking.go).
2. **RO-C2D-1**: Update BlockingCondition comment to list all 7 BlockReasons.
3. **RO-C2D-3**: Add Blocked→{Analyzing, Pending} to ValidTransitions or document as special resume transitions.
4. **WE-C2D-1**: Document Tekton as default execution engine.
5. **crds.md**: Update RemediationRequest and WorkflowExecution spec/status to match API.

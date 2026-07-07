# Implementation Plan: Job Resource Governance and Transient-Failure Tolerance

**Issue**: #1572 (follow-up to #1564)
**Design Decision**: [DD-WE-008](../../architecture/decisions/DD-WE-008-job-resource-governance-transient-failure-tolerance.md)
**Business Requirement**: [BR-WE-019](../../requirements/BR-WE-019-job-resource-governance-transient-failure-tolerance.md)
**Test Plan**: [TP-1572-v1.5](TEST_PLAN.md)
**Branch**: `feature/dd-we-008-job-resources-podfailurepolicy`
**Created**: 2026-07-06

---

## Overview

Three additive mechanisms, all scoped to the Job engine only:

1. Per-workflow `execution.resources` (optional `corev1.ResourceRequirements` shape), resolved
   from the DS catalog into `WFE.Status.Resources`, applied to the `"workflow"` container
   (`JobExecutor.buildJob()`).
2. Unconditional `Job.Spec.PodFailurePolicy` that `Ignore`s OOM-kill (exit 137) and
   node-disruption (`DisruptionTarget`) pod failures, while every other failure keeps today's
   `Count` (fail-fast) behavior against the unchanged `backoffLimit: 0` (`JobExecutor.buildJob()`).
3. Audit-trail retry-count completeness (BR-WE-019 AC10): the durable audit trail records how
   many pod-failure attempts (2) tolerated before a remediation succeeded, closing a
   SOC2 CC8.1/AU-3 completeness gap that mechanism 2 would otherwise introduce
   (`JobExecutor.buildStatusSummary()`, `buildWorkflowExecutionAuditPayload()`).

### Due Diligence Findings (incorporated)

| ID | Finding | Resolution |
|---|---|---|
| F1 | `gopkg.in/yaml.v3` cannot unmarshal `corev1.ResourceRequirements` directly (no `UnmarshalYAML`, only JSON codec) | `resources`' shape is fixed (unlike `EngineConfig`'s genuinely-variable-per-engine shape), so no `interface{}` is needed: `ResourcesSchema` (`map[string]string` fields) is unmarshaled natively by yaml.v3, and `ExtractResources()` parses each quantity string directly via `resource.ParseQuantity()` — verified by spike; simpler than the original JSON-round-trip plan, not just cosmetically different |
| F2 | `requests > limits` is not rejected by individual quantity parsing (`resource.ParseQuantity` validates each value in isolation) | Explicit `Cmp()` check added in `ExtractResources()` |
| F3 | `JobPodFailurePolicy` graduated GA in v1.31, not v1.27 (initial assumption was wrong) | Verified `Default: true` (Beta) on the v1.30 release branch — safe across kubernaut's full v1.30+ range, no feature-gate logic needed |
| F4 | `ServiceAccountName` has a dedicated DB column; `Dependencies`/`EngineConfig` do not | `resources` follows the no-column pattern — smaller blast radius, no migration/OpenAPI/ogen changes |
| F5 | Silent no-op risk: `resources` declared for `engine: tekton` would be structurally valid but never consumed | Explicit registration-time rejection when `resources` is set for any engine other than `job` |
| F6 | `make manifests` auto-syncs CRD YAML into `charts/kubernaut/crds/` and `charts/kubernaut/files/crds/` | Not a "Helm chart change" in the template-logic sense, but these generated files DO change and must be regenerated, not hand-edited |
| F7 | Original plan had zero E2E coverage; envtest cannot exercise the real Job-controller `PodFailurePolicy` evaluation loop | Pyramid Invariant violation caught in review — added a real-cluster E2E tier (Phase 3b below), reusing existing `test-job-intentional-failure` fixture for the regression case and adding one new minimal `job-oomkill` fixture |
| F8 | ~~`WorkflowExecutionAuditPayload` is ogen-generated (no free-form field to carry a retry count without misusing existing field semantics)~~ **Superseded by F14 below** | ~~A durable audit-completeness fix for tolerated retries requires an OpenAPI spec + ogen-client regen — deliberately deferred to a follow-up issue+DD opened after this PR merges; this PR makes zero OpenAPI/ogen changes~~ **Reversed**: the ogen-generated nature of the schema was never the actual blocker (regen is one command); F14 corrects the deferral rationale and brings the fix into this PR as Wiring Point C |
| F9 | `Resources interface{}` (original plan) is an avoidable Go Anti-Pattern Checklist violation — `resources`' shape is fixed, unlike `EngineConfig`'s genuinely-variable shape | Replaced with a concrete `ResourcesSchema` struct (`map[string]string` fields, yaml.v3-native); `ExtractResources()` parses quantities directly via `resource.ParseQuantity()`, no JSON round-trip needed — simpler than the original plan |
| F10 | Investigating whether `go-playground/validator` tags on `ResourcesSchema` would be enforced surfaced that ADR-046 (approved, standard-setting) was never actually implemented for `WorkflowSchema` — `pkg/validation/` doesn't exist, `parser.go` never calls `validator.Struct()`; every `validate:"..."` tag in this file (not just new ones) is currently decorative | Pre-existing, unrelated gap — not caused by or fixed in this PR. `ResourcesSchema` gets `validate` tags for consistency/self-documentation only; the actual enforcement (quantity format, `requests<=limits`) stays hand-written in `ExtractResources`, consistent with how every other non-trivial rule in this file already works. Tracked as [issue #1591](https://github.com/jordigilh/kubernaut/issues/1591) |
| F11 | v1.2 of this plan grouped RED tests by test tier (all UT, then all IT, then all E2E) with GREEN implementing everything at once, then declared GREEN "complete" on UT alone — a direct violation of AGENTS.md's Wiring-First TDD Sequence (`RED: IT→UT`, `GREEN: wire→implement`) and the "UT-Only GREEN" anti-pattern ("GREEN is NOT complete until both UT and IT pass"). Caught in review, not by spike. | Restructured around the two actual wiring points (resources-to-container, PodFailurePolicy), each following genuine `RED(IT first, then UT)` → `GREEN(wire first, then implement logic)` → `REFACTOR(named improvement or explicit N/A)`. For the `PodFailurePolicy` wiring point specifically, envtest structurally cannot exercise the real Job-controller evaluation loop, so E2E — not IT — is treated as that point's actual GREEN-completion gate (Phase 5), consistent with the Pyramid Invariant rather than as a bonus add-on |
| F12 | Preflighting the audit-count question ("does the E2E FP scope account for total audit traces?") surfaced that no test anywhere asserts audit-emission is gated on *terminal phase transition* rather than *reconcile count* — a real, general regression-guard gap this PR's tolerated-retry behavior newly makes exercisable (more reconciles now legitimately happen against a still-`Running` Job than before) | Not a bug — audit emission is already coupled to terminal-phase transition in the existing reconciler code, so this is a forward-looking regression guard, not a fix. Added `IT-WE-019-003` (new) and extended `E2E-WE-019-001` to assert exactly 1 completion audit event, citing **BR-AUDIT-005** (not BR-WE-019 — no BR-WE-019 AC covers this). See "Bundled Audit Work" phase below |
| F13 | Same preflight found 5 pre-existing, unrelated audit assertions using loose `>= 1`/bare-existence checks without an `EventType` filter (3 in `reconciler_test.go`, 2 in `02_observability_test.go`), plus 2 more in `audit_flow_integration_test.go` that are loose *by design* (non-deterministic Tekton PipelineRun timing in envtest) | Per user decision: the 5 mechanical ones are bundled into this PR (low-risk, reuses the proven `countEventsByType` pattern, avoids a second ~20-30min CI cycle for an unrelated one-line-per-assertion fix). The 2 Tekton-timing ones are explicitly **not** bundled — they need a design decision (force determinism vs. leave loose), not a mechanical tightening — and are tracked as a separate follow-up issue |
| F14 | User directly challenged F8's deferral ("is there a problem running the ogen regen?"). Re-investigation found the stated justification ("cross-cutting audit infrastructure shared by every audit-emitting service") false: `WorkflowExecutionAuditPayload` is used exclusively by WFE's own 5 event types (not shared with any other service's payload schema); `make generate-datastorage-client` is a single command; `job.Status.Failed` is already read by `buildStatusSummary()` (just not on the success branch); `ExecutionStatusSummary` already flows durably into `wfe.Status.ExecutionStatus` via the existing `MarkCompleted`/`MarkFailed` path | F8 reversed. Promoted to **Wiring Point C** (BR-WE-019 AC10), now in scope for this PR: `ExecutionStatusSummary.RetryCount` (reuses Phase 2.4's `make generate manifests` step), unconditional capture in `buildStatusSummary()`, one new optional OpenAPI field + `make generate-datastorage-client`, and a corresponding read in `buildWorkflowExecutionAuditPayload()`. Comparable in size to Wiring Point A, not a cross-cutting project. See DD-WE-008 v1.4 Section 9 |

### Key Design Decisions

- **No DB migration, no OpenAPI/ogen regen** — `resources` extracted on-demand from `content`, mirroring `dependencies`/`engineConfig` (DD-WE-008 Scenario 4)
- **`backoffLimit` unchanged** — `PodFailurePolicy` grants retry tolerance for infra-caused failures without weakening fail-fast for genuine script failures
- **Engine-gated** — `resources` valid only for `engine: job`; explicit rejection otherwise (no silent no-op)
- **`ActiveDeadlineSeconds` (already in place, BR-WORKFLOW-008) remains the outer bound** — prevents an under-resourced workflow from retrying indefinitely under the new `Ignore` tolerance
- **Three-tier coverage for AC4/AC5/AC9** — Unit + Integration + a new real-cluster E2E tier, since `PodFailurePolicy` evaluation is Job-controller-loop behavior envtest cannot exercise (Pyramid Invariant)
- **Audit-completeness *content* fix (retry count) is in scope for this PR, as Wiring Point C (F14)** — reversing the original deferral after re-investigation showed the schema is WFE-exclusive, not cross-cutting (DD-WE-008 Section 9). Per-attempt root-cause attribution remains a deliberate, separate non-goal (unchanged scope boundary)
- **Wiring-first phase structure (F11)** — phases below are organized per wiring point (resources-to-container, `PodFailurePolicy`, then audit retry-count), each following AGENTS.md's `RED(IT→UT)` → `GREEN(wire→implement)` sequence, not grouped by test tier. E2E is `PodFailurePolicy`'s actual GREEN-completion gate (envtest can't reach controller-loop behavior), not a bonus phase after GREEN is already declared done
- **Audit-count regression guards (F12/F13) bundled, not treated as a fourth wiring point** — `IT-WE-019-003` and the extended `E2E-WE-019-001` assertion, plus 5 mechanical test-hardening fixes, are added in Phase 5.3 below. They don't follow RED→GREEN because there is no new production code behind them — the invariant they guard already holds today; they are honestly framed as regression guards, not proof of a new guarantee this PR creates (see TEST_PLAN.md Section 4.4)
- **Audit-content fix promoted to Wiring Point C (F14)** — reverses the earlier deferral; follows genuine RED->GREEN in Phase 5.1/5.2, distinct from the no-RED bundled guards in Phase 5.3

---

## Phase 1: TDD RED — Wiring Point A (`resources`: catalog → `WFE.Status` → Job container)

Per AGENTS.md's Wiring-First TDD Sequence: **IT test through the production entry point first**,
then the UT tests for the logic behind it. Both sets MUST fail at this point.

### Phase 1.1: Integration test (written FIRST)

**File**: `test/integration/workflowexecution/job_lifecycle_integration_test.go` (extended)

| Test ID | What it asserts | Why it fails |
|---|---|---|
| IT-WE-019-001 | Through the real reconciler entry point (envtest): a catalog entry declaring `execution.resources` results in a real Job whose `"workflow"` container `Resources` match | The chain doesn't exist yet: no `Resources` field on `WorkflowCatalogMetadata`/`WorkflowExecutionStatus`, no extraction, no consumption in `buildJob` |

To let this test *compile* (not pass), add only the plain data-type field declarations needed
as scaffolding — zero logic, no wiring, no consumption anywhere. This is the narrow carve-out
AGENTS.md's REFACTOR section itself acknowledges ("a milestone that only adds plain data-type
fields with no logic"); it is not GREEN, because nothing yet reads or writes through these
fields in production code:

- `ResourcesSchema` type + `Resources *ResourcesSchema` on `models.WorkflowExecution` (`pkg/datastorage/models/workflow_schema.go`)
- `Resources *corev1.ResourceRequirements` on `WorkflowCatalogMetadata` (`pkg/workflowexecution/client/workflow_querier.go`)
- `Resources *corev1.ResourceRequirements` on `WorkflowExecutionStatus` (`api/workflowexecution/v1alpha1/workflowexecution_types.go`) + `make generate manifests`

IT-WE-019-001 now compiles and **fails at assertion** (fields exist but stay `nil` — nothing
populates or consumes them yet).

### Phase 1.2: Unit tests for the logic behind this wiring point

**Files**: `pkg/datastorage/schema_resources_test.go` (new), `pkg/workflowexecution/workflow_querier_test.go` (extended), `pkg/workflowexecution/executor/job_test.go` (extended)

| Test ID | What it asserts | Why it fails |
|---|---|---|
| UT-DS-008-001 | Quoted/unquoted quantities parse into typed `corev1.ResourceRequirements` | `ExtractResources()` doesn't exist |
| UT-DS-008-002 | Invalid quantity → `SchemaValidationError` on `execution.resources` | Same |
| UT-DS-008-003 | Absent `resources` → `ExtractResources` returns `(nil, nil)` | Same |
| UT-DS-008-004 | `requests`-only or `limits`-only both parse | Same |
| UT-DS-008-005 | `resources` + `engine: tekton` → rejected | Engine-gating check doesn't exist |
| UT-DS-008-006 | `resources` + `engine: ansible` → rejected | Same |
| UT-DS-008-007 | `requests.cpu > limits.cpu` → rejected | `Cmp()` check doesn't exist |
| UT-DS-008-008 | `requests.memory > limits.memory` → rejected (independent of cpu) | Same |
| UT-DS-008-009 | Valid mixed cpu/memory requests+limits → parses | Same |
| UT-WE-1572-001 | `ResolveWorkflowCatalogMetadata` returns `Resources` from schema `content` | `ExtractResources()` not called yet |
| UT-WE-1572-002 | Returns `Resources: nil` when catalog entry declares none | Same |
| UT-WE-1572-003 | Propagates a resources parse error from the schema parser | Same |
| UT-WE-054-JOB-021 | `buildJob()` sets container `Resources` from `wfe.Status.Resources` | `buildJob` doesn't consume the field |
| UT-WE-054-JOB-024 | `wfe.Status.Resources == nil` → container gets zero-value `Resources{}` (unchanged behavior) | `resourcesFor()` doesn't exist |

### Phase 1 Checkpoint

- [ ] IT-WE-019-001 compiles and FAILS at assertion (not a compile error)
- [ ] All 12 Phase 1.2 unit tests compile and FAIL (RED)
- [ ] Zero new lint errors introduced
- [ ] Existing tests still compile (no import breakage)

---

## Phase 2: TDD GREEN — Wiring Point A

Per the Wiring-First sequence: **wire the component first** (until IT-WE-019-001 passes), *then*
implement the full logic (until the Phase 1.2 unit tests pass). GREEN for this wiring point is
not complete until both have passed — not on unit tests alone.

### Phase 2.1: Wire first — minimal plumbing to make IT-WE-019-001 pass

**Files**: `pkg/datastorage/schema/parser.go`, `pkg/workflowexecution/client/workflow_querier.go`, `internal/controller/workflowexecution/workflowexecution_catalog.go`, `pkg/workflowexecution/executor/job.go`

Add the minimal happy-path versions of `ExtractResources`/`parseResourceQuantities` (just enough
to parse a valid `requests`/`limits` map, no edge-case handling yet), wire it through
`ResolveWorkflowCatalogMetadata`:

```go
resources, err := parser.ExtractResources(parsed)
if err != nil {
    return nil, fmt.Errorf("failed to extract resources for workflow %s: %w", workflowID, err)
}
meta.Resources = resources
```

wire the controller assignment in `resolveWorkflowCatalog` (immediately after the existing
`ServiceAccountName` assignment):

```go
wfe.Status.ExecutionEngine = meta.ExecutionEngine
wfe.Status.ServiceAccountName = meta.ServiceAccountName
wfe.Status.Resources = meta.Resources
```

and add the `resourcesFor()` helper + consumption in `buildJob`'s container spec:

```go
// resourcesFor returns the resolved per-workflow resources (BR-WE-019), or
// the zero value (no requests/limits, BestEffort QoS) when the catalog entry
// declares none -- preserving today's behavior exactly.
func resourcesFor(wfe *workflowexecutionv1alpha1.WorkflowExecution) corev1.ResourceRequirements {
    if wfe.Status.Resources == nil {
        return corev1.ResourceRequirements{}
    }
    return *wfe.Status.Resources
}
```

```go
Containers: []corev1.Container{
    {
        Name:            "workflow",
        Image:           wfe.Spec.WorkflowRef.ExecutionBundle,
        Env:             envVars,
        VolumeMounts:    mounts,
        SecurityContext: restrictedContainerSecurityContext(),
        Resources:       resourcesFor(wfe),
    },
},
```

**Test passing after this**: IT-WE-019-001

### Phase 2.2: Implement full logic — until the Phase 1.2 unit tests pass

**File**: `pkg/datastorage/schema/parser.go`

Flesh out `ExtractResources`/`parseResourceQuantities` with the edge cases the unit tests
demand (invalid quantities, `requests <= limits`, absent-resources handling):

```go
func (p *Parser) ExtractResources(schema *models.WorkflowSchema) (*corev1.ResourceRequirements, error) {
    if schema.Execution == nil || schema.Execution.Resources == nil {
        return nil, nil
    }
    requests, err := parseResourceQuantities(schema.Execution.Resources.Requests, "requests")
    if err != nil {
        return nil, err
    }
    limits, err := parseResourceQuantities(schema.Execution.Resources.Limits, "limits")
    if err != nil {
        return nil, err
    }
    for name, limit := range limits {
        if req, ok := requests[name]; ok && req.Cmp(limit) > 0 {
            return nil, models.NewSchemaValidationError("execution.resources",
                fmt.Sprintf("%s request (%s) exceeds limit (%s)", name, req.String(), limit.String()))
        }
    }
    return &corev1.ResourceRequirements{Requests: requests, Limits: limits}, nil
}

func parseResourceQuantities(raw map[string]string, field string) (corev1.ResourceList, error) {
    if len(raw) == 0 {
        return nil, nil
    }
    list := make(corev1.ResourceList, len(raw))
    for name, value := range raw {
        q, err := resource.ParseQuantity(value)
        if err != nil {
            return nil, models.NewSchemaValidationError(
                fmt.Sprintf("execution.resources.%s.%s", field, name),
                fmt.Sprintf("invalid quantity %q: %v", value, err))
        }
        list[corev1.ResourceName(name)] = q
    }
    return list, nil
}
```

Extend `validateWorkflowExecution` (after the existing `if engine == "ansible"` block) for the
engine-gating tests:

```go
if schema.Execution.Resources != nil {
    if engine != "job" {
        return models.NewSchemaValidationError("execution.resources",
            fmt.Sprintf("execution.resources is only supported for engine \"job\", got %q", engine))
    }
    if _, err := (&Parser{}).ExtractResources(schema); err != nil {
        return err
    }
}
```

**Tests passing after this**: UT-DS-008-001 through UT-DS-008-009, UT-WE-1572-001..003, UT-WE-054-JOB-021, UT-WE-054-JOB-024

### Phase 2 Checkpoint

- [ ] IT-WE-019-001 PASSES (wiring proven, not just unit logic)
- [ ] All 12 Phase 1.2 unit tests PASS
- [ ] `go build ./...` succeeds
- [ ] `make generate manifests` succeeds — CRD YAML includes `status.resources`
- [ ] Existing `UT-WE-054-JOB-*`, `UT-DS-006-*`, `UT-WE-650-*`, `IT-WE-014-*` tests still pass (no regressions)

---

## Checkpoint 1: Wiring Point A Due Diligence

- [ ] **F1**: quantities parsed via `resource.ParseQuantity`, both quoted and unquoted forms — confirmed by UT-DS-008-001
- [ ] **F2**: `requests > limits` rejected — confirmed by UT-DS-008-007, UT-DS-008-008
- [ ] **F5**: engine-gating rejects `resources` for non-`job` engines — confirmed by UT-DS-008-005, UT-DS-008-006
- [ ] **Backward compatibility**: absent `resources` → zero-value container `Resources{}`, byte-identical to pre-change Job spec — confirmed by UT-WE-054-JOB-024
- [ ] **Wiring proven end-to-end, not just unit-level**: IT-WE-019-001 passing is the actual gate here, per the Pyramid Invariant ("GREEN is NOT complete until the IT test... passes")
- [ ] **Regression**: `go test ./pkg/datastorage/...`, `./pkg/workflowexecution/...`, `./test/integration/workflowexecution/...` — all pass
- [ ] **Build**: `go build ./...` — zero errors
- [ ] **Lint**: `golangci-lint run --timeout=5m` — no new warnings

---

## Phase 3: TDD RED — Wiring Point B (`PodFailurePolicy`)

Same Wiring-First discipline: IT test through the production entry point first, then UT.

### Phase 3.1: Integration test (written FIRST)

**File**: `test/integration/workflowexecution/job_lifecycle_integration_test.go` (extended)

| Test ID | What it asserts | Why it fails |
|---|---|---|
| IT-WE-019-002 | envtest's real API server accepts a Job with the exact `PodFailurePolicy` shape on creation | `PodFailurePolicy` not set in `buildJob` yet |

Note on what this test can and cannot prove: envtest runs a real API server but not the real
`kube-controller-manager` Job controller, so IT-WE-019-002 proves the *shape* is valid and
accepted, not that `Ignore` semantics are actually evaluated on a failure. That deeper proof is
Phase 5's job, not this one — see the note there on why E2E, not IT, is this wiring point's real
GREEN-completion gate.

### Phase 3.2: Unit tests for the logic behind this wiring point

**File**: `pkg/workflowexecution/executor/job_test.go` (extended)

| Test ID | What it asserts | Why it fails |
|---|---|---|
| UT-WE-054-JOB-022 | `Job.Spec.PodFailurePolicy` has `Ignore` rules for exit-137 and `DisruptionTarget` | `PodFailurePolicy` not set in `buildJob` |
| UT-WE-054-JOB-023 | `Job.Spec.BackoffLimit` remains `0` | Regression guard, written now to lock the contract before touching `buildJob` again |

### Phase 3 Checkpoint

- [ ] IT-WE-019-002 compiles and FAILS
- [ ] UT-WE-054-JOB-022/023 compile and FAIL (022) / already pass by coincidence but are now locked in (023)
- [ ] Zero new lint errors introduced

---

## Phase 4: TDD GREEN — Wiring Point B

**File**: `pkg/workflowexecution/executor/job.go`

Wire `PodFailurePolicy` into `JobSpec` (alongside `BackoffLimit`, `TTLSecondsAfterFinished`,
`ActiveDeadlineSeconds`). This is a static, declarative struct literal with no separate "logic"
step — wiring it in makes the IT and UT tests pass together, which is expected here (the
Wiring-First sequence's wire-then-implement split matters when there's non-trivial logic behind
the wiring; there isn't any for this mechanism):

```go
PodFailurePolicy: &batchv1.PodFailurePolicy{
    Rules: []batchv1.PodFailurePolicyRule{
        {
            Action: batchv1.PodFailurePolicyActionIgnore,
            OnPodConditions: []batchv1.PodFailurePolicyOnPodConditionsPattern{
                {Type: corev1.DisruptionTarget, Status: corev1.ConditionTrue},
            },
        },
        {
            Action: batchv1.PodFailurePolicyActionIgnore,
            OnExitCodes: &batchv1.PodFailurePolicyOnExitCodesRequirement{
                ContainerName: ptr.To("workflow"),
                Operator:      batchv1.PodFailurePolicyOnExitCodesOpIn,
                Values:        []int32{137},
            },
        },
    },
},
```

(Add `ptr "k8s.io/utils/ptr"` import — confirm during implementation whether it's already
vendored; fall back to a local helper if not.)

**Tests passing after this**: IT-WE-019-002, UT-WE-054-JOB-022, UT-WE-054-JOB-023

### Phase 4 Checkpoint

- [ ] IT-WE-019-002 and both unit tests PASS
- [ ] `go build ./...` succeeds
- [ ] Existing `IT-WE-014-*` tests still pass (zero regressions)

---

## Phase 5: TDD RED→GREEN — Wiring Point C (Audit Retry-Count Completeness, BR-WE-019 AC10) + Bundled BR-AUDIT-005 Regression Guards

Wiring Point C reverses the v1.2-v1.4 deferral (F14): re-investigation showed the "cross-cutting
audit infrastructure" justification for deferring was incorrect (DD-WE-008 Section 9), so this
is now a genuine third wiring point, following the same RED(IT→UT)→GREEN(wire→implement)
discipline as A and B. It runs after Phase 4 (both A and B already GREEN) since it reuses the
same `make generate manifests` step Phase 2.4 already exercises, and it is grouped with the
bundled BR-AUDIT-005 regression guards (5.3) because both are audit-correctness work discovered
in the same preflight and are cheapest to validate together before the E2E phase.

### Phase 5.1: RED — Wiring Point C

**File**: `test/integration/workflowexecution/job_lifecycle_integration_test.go` (extended)

| Test ID | What it asserts | Why it fails |
|---|---|---|
| IT-WE-019-004 | A WFE's Job with `Status.Failed: N` before `Status.Succeeded: 1` produces a `workflow.completed` event whose `event_data.retry_count == N` | `ExecutionStatusSummary` has no `RetryCount` field yet; `buildStatusSummary` never reads `Failed` on the success path; `WorkflowExecutionAuditPayload` has no `retry_count` field |

As with Phase 1.1, add only the plain-data-type scaffolding to let this compile (not pass):
`RetryCount int32 `json:"retryCount,omitempty"`` on `ExecutionStatusSummary`
(`api/workflowexecution/v1alpha1/workflowexecution_types.go`) + `make generate manifests`, and
the optional `retry_count` field on `WorkflowExecutionAuditPayload`
(`api/openapi/data-storage-v1.yaml`) + `make generate-datastorage-client`. IT-WE-019-004 now
compiles and fails at assertion (fields exist but stay unset).

**File**: `pkg/workflowexecution/executor/job_test.go` (extended), `pkg/workflowexecution/audit/manager_test.go` (new)

| Test ID | What it asserts | Why it fails |
|---|---|---|
| UT-WE-054-JOB-025 | `buildStatusSummary()` sets `RetryCount == job.Status.Failed` when both `Succeeded > 0` and `Failed > 0` | Success branch never reads `Failed` |
| UT-WE-054-JOB-026 | `buildStatusSummary()` leaves `RetryCount` at `0` when the Job succeeded with zero failures | Same (currently: field doesn't exist) |
| UT-WE-AUDIT-001 | `buildWorkflowExecutionAuditPayload()` sets `payload.RetryCount` from `wfe.Status.ExecutionStatus.RetryCount` when > 0 | `RetryCount` not read in the payload builder yet |
| UT-WE-AUDIT-002 | `buildWorkflowExecutionAuditPayload()` leaves `payload.RetryCount` unset when `ExecutionStatus` is `nil` or `RetryCount == 0` | Same |

### Phase 5.1 Checkpoint

- [ ] IT-WE-019-004 compiles and FAILS at assertion (not a compile error)
- [ ] All 4 Phase 5.1 unit tests compile and FAIL (RED)
- [ ] Zero new lint errors introduced

### Phase 5.2: GREEN — Wiring Point C

**Wire first** (until IT-WE-019-004 passes) — `pkg/workflowexecution/executor/job.go`:

```go
func (j *JobExecutor) buildStatusSummary(job *batchv1.Job) *workflowexecutionv1alpha1.ExecutionStatusSummary {
    summary := &workflowexecutionv1alpha1.ExecutionStatusSummary{
        Status:     corev1.ConditionUnknown,
        TotalTasks: 1,
        RetryCount: job.Status.Failed, // unconditional -- BR-WE-019 AC10
    }
    if job.Status.Succeeded > 0 {
        summary.Status = corev1.ConditionTrue
        summary.Reason = "Succeeded"
        summary.CompletedTasks = 1
    } else if job.Status.Failed > 0 {
        summary.Status = corev1.ConditionFalse
        summary.Reason = "Failed"
        summary.Message = fmt.Sprintf("%d pod(s) failed", job.Status.Failed)
    } else if job.Status.Active > 0 {
        summary.Status = corev1.ConditionUnknown
        summary.Reason = "Running"
        summary.Message = fmt.Sprintf("%d pod(s) active", job.Status.Active)
    }
    return summary
}
```

`pkg/workflowexecution/audit/manager.go` (`buildWorkflowExecutionAuditPayload`, after the
existing `ExecutionRef`/`PipelinerunName` block):

```go
if wfe.Status.ExecutionStatus != nil && wfe.Status.ExecutionStatus.RetryCount > 0 {
    payload.RetryCount.SetTo(int(wfe.Status.ExecutionStatus.RetryCount)) // confirm exact ogen Opt-type during implementation
}
```

**Tests passing after this**: IT-WE-019-004, UT-WE-054-JOB-025, UT-WE-054-JOB-026,
UT-WE-AUDIT-001, UT-WE-AUDIT-002

### Phase 5.2 Checkpoint

- [ ] IT-WE-019-004 PASSES
- [ ] All 4 Phase 5.1 unit tests PASS
- [ ] `go build ./...` succeeds
- [ ] `make generate manifests` and `make generate-datastorage-client` both succeed and their
      diffs are committed (guards R9 in TEST_PLAN.md)
- [ ] Existing `IT-WE-014-*`, `UT-WE-054-JOB-*` tests still pass (zero regressions)

### Phase 5.3: Bundled BR-AUDIT-005 Regression Guards (F12/F13 — no genuine RED phase)

Distinct from Wiring Point C above: no new production code is added here. Per the user's
decision to bundle Category A-mechanical and Category B into this PR rather than raise separate
follow-up PRs (each full E2E run costs ~20-30 min of CI time, and splitting compounds that
cost), this sub-phase adds forward-looking regression guards and tightens pre-existing loose
assertions.

**File**: `test/integration/workflowexecution/job_lifecycle_integration_test.go` (extended)

| Test ID | What it asserts | Expected result |
|---|---|---|
| IT-WE-019-003 | A WFE's Job is observed `Status.Active == 1` across N repeated reconciles before finally transitioning to `Status.Succeeded == 1`; exactly 1 `workflow.completed` audit event is recorded (via `countEventsByType`, filtered by `EventType`) | **Passes immediately** — audit emission is already gated on terminal-phase transition in the existing reconciler, not reconcile count. No genuine RED phase; write it, confirm it passes, and keep it as a permanent regression guard (be explicit about this in the PR description, do not claim it as new-behavior proof) |

**Files**: `test/integration/workflowexecution/reconciler_test.go` (3 assertions),
`test/e2e/workflowexecution/02_observability_test.go` (2 assertions)

Replace each `>= 1`/bare-existence check with an exact count via the existing
`countEventsByType` helper (already proven in `audit_workflow_refs_integration_test.go`),
filtered to the specific `EventType` each assertion is actually about. Purely mechanical —
one-line change per assertion, no new helper code needed.

**Explicitly not touched**: `audit_flow_integration_test.go`'s 2 loose assertions (Tekton
PipelineRun timing in envtest is non-deterministic by the test's own docstring) — tracked as a
separate follow-up issue, not bundled here (needs a design decision, not a mechanical fix).

(`E2E-WE-019-001`'s BR-AUDIT-005 count-assertion extension and its BR-WE-019 AC10
`retry_count`-content extension both ride along on Phase 6's real-cluster run — see below, not
duplicated here.)

### Phase 5 Checkpoint

- [ ] IT-WE-019-004 PASSES (Wiring Point C's real gate, not UT alone)
- [ ] IT-WE-019-003 written and PASSES (confirm it is not accidentally a no-op assertion)
- [ ] All 5 tightened assertions in `reconciler_test.go`/`02_observability_test.go` PASS — if any
      newly fails, that indicates a real pre-existing bug (duplicate audit write), not a broken
      test; investigate before assuming the assertion itself is wrong (R8 in TEST_PLAN.md)
- [ ] PR description explicitly states the Phase 5.3 items are regression guards with no genuine
      RED phase, distinct from Wiring Point C's genuine RED->GREEN (avoid overclaiming coverage)
- [ ] Zero regressions in `reconciler_test.go`/`02_observability_test.go`'s other assertions

---

## Phase 6: TDD RED→GREEN — Real-Cluster Proof of Wiring Points B and C (Pyramid Invariant)

Per the Pyramid Invariant, IT proves wiring — but envtest structurally cannot exercise the real
Job-controller's `PodFailurePolicy` evaluation loop (F7), so for *this specific wiring point*,
**E2E is the actual GREEN-completion gate**, not a bonus phase tacked on after GREEN is already
declared done. This is not a repeat of the F11 anti-pattern: Phase 4 already proved schema-level
wiring via IT-WE-019-002 first; this phase proves the deeper controller-loop semantics IT
cannot reach, which is exactly what "E2E proves the journey" means in the Pyramid Invariant —
building on IT-proven wiring, not substituting for it after the fact. Wiring Point C's
`retry_count` guarantee rides along on the same real-cluster run (Phase 6.2) rather than
requiring its own fixture, since it is measured on the exact same tolerated-retry scenario.

### Phase 6.1: New E2E fixture (`job-oomkill`) — RED prerequisite

**New files**:
- `test/fixtures/job/oomkill/Dockerfile` — copy of `test/fixtures/job/failing/Dockerfile`
  with `exit 1` replaced by `exit 137` (deterministic, unconditional — no stateful
  retry-then-succeed logic needed; see DD-WE-008 Section 8 for why this is sufficient)
- `test/fixtures/job/oomkill/workflow-schema.yaml` — copy of
  `test/fixtures/workflows/job-failing/workflow-schema.yaml`, renamed to
  `test-job-oomkill`, same parameters (`TARGET_RESOURCE`, `FAILURE_MESSAGE`)

**Modified files**:
- `test/fixtures/job/build-and-push.sh`: add `"job-oomkill:${SCRIPT_DIR}/oomkill"` to the
  `IMAGES` array
- `test/infrastructure/workflow_bundles.go`: add
  `{"test-job-oomkill", "v1.0.0", "job-oomkill", "Job backend OOM-kill-simulation workflow for DD-WE-008 E2E testing"}`
  to `bundleWorkflows`

**Blocking manual step** (cannot be automated by this PR, and blocks calling Wiring Point B's
GREEN "complete", not just a footnote): a maintainer with `quay.io/kubernaut-cicd` push
credentials runs `./test/fixtures/job/build-and-push.sh` (or a targeted equivalent) to publish
`job-oomkill:v1.0.0` **before** `E2E-WE-019-001` can pass in CI. Until published, the test fails
with `ImagePullBackOff`, not a false green.

### Phase 6.2: E2E tests (RED, against Phase 4 already applied)

**File**: `test/e2e/workflowexecution/03_job_backend_test.go` (extended)

Because Phase 4's `PodFailurePolicy` wiring already exists by the time this phase starts, prove
genuine RED the same way Phase 3/1's IT tests would if written after their GREEN: temporarily
revert Phase 4's `buildJob` change locally, confirm `E2E-WE-019-001` fails (Job fails
permanently on the first exit-137 pod, matching today's pre-change behavior), then reapply.

| Test ID | What it asserts | Why it fails (pre-Phase-4) |
|---|---|---|
| E2E-WE-019-001 | Real Job controller `Ignore`s exit-137 failures — `job.Status.Failed >= 2` within 90s while `wfe.Status.Phase == Running` | No `PodFailurePolicy` — the first exit-137 pod immediately fails the Job |
| E2E-WE-019-002 | Real Job controller still `Count`s a genuine failure — `job.Status.Failed == 1` exactly, WFE reaches `Failed` | Passes today by coincidence (current `backoffLimit:0` behavior); written now as a regression guard against Phase 4 accidentally loosening it |

**Note**: `E2E-WE-019-001` carries two further assertions added in the same `It()` block, once
the fixture completes and the WFE reaches a terminal phase — neither needs a separate fixture or
cluster setup:
- Bundled BR-AUDIT-005 count guard (Phase 5.3): exactly 1 `workflow.completed` event exists for
  the WFE's `correlation_id`
- Wiring Point C / BR-WE-019 AC10 (Phase 5.2, real-cluster proof): that event's `retry_count` is
  `>= 2`, matching the observed `job.Status.Failed`

### Phase 6 Checkpoint (Wiring Point B GREEN-completion gate)

- [ ] `job-oomkill` fixture builds locally (`--local` flag, no push required for this check)
- [ ] `job-oomkill:v1.0.0` published to `quay.io/kubernaut-cicd/test-workflows` (blocking — Wiring Point B's GREEN is not considered complete until this is confirmed, per the Pyramid Invariant reasoning above)
- [ ] E2E-WE-019-001 FAILS against pre-Phase-4 code (verified per the note above)
- [ ] Both E2E tests PASS once Phase 4 is fully applied and the image is published, including
      the bundled audit-count and `retry_count` assertions
- [ ] All existing `E2E-WE-014-*` tests still pass (zero regressions)

---

## Phase 7: TDD REFACTOR — Named Improvements or Explicit N/A

Per wiring point, per AGENTS.md's "REFACTOR is content, not validation": name the concrete
improvement, or mark `N/A` with a one-line reason if GREEN left nothing to clean up. Running
`go build ./...` / lint / tests is the mandatory safety net proving these are behavior-
preserving — it is not itself a REFACTOR item.

### Phase 7.1: Wiring Point A (`resources`)

- Review `parseResourceQuantities` / `ExtractResources` against `ExtractDependencies` /
  `ExtractEngineConfig` for duplicated error-construction patterns; extract a shared helper if a
  real duplication exists, otherwise mark `N/A` with the specific reason (e.g. "each extractor's
  error shape is different enough — `execution.resources.<field>.<name>` vs
  `execution.dependencies.secrets[i].name` — that a shared helper would add an indirection layer
  without removing real duplication")
- Error string casing/punctuation audit: `execution.resources is only supported for engine...` and friends

### Phase 7.2: Wiring Point B (`PodFailurePolicy`)

- Likely `N/A`: GREEN (Phase 4) added a static struct literal with no branching logic to
  simplify. Confirm during implementation and record the one-line reason rather than filling
  this slot with a build/lint/test confirmation.

### Phase 7.3: Wiring Point C (audit retry-count)

- Review whether `buildWorkflowExecutionAuditPayload`'s growing list of conditional
  `payload.X.SetTo(...)` blocks (started/completed/duration/failure/pipelinerun/parameters, now
  +retry-count) warrants extraction into a smaller set of named helpers, or whether that would
  add indirection without removing real duplication (each block sets a semantically distinct
  field) — record the concrete decision either way, not a bare pass-through
- Confirm the exact ogen `Opt*` type used for `RetryCount` matches the codebase's existing
  numeric-optional-field convention (if any precedent exists in `oas_schemas_gen.go`)

### Phase 7.4: Documentation

- Update `docs/services/crd-controllers/03-workflowexecution/crd-schema.md` with the new
  `Status.Resources` and `Status.ExecutionStatus.RetryCount` fields
- Add `execution.resources` example to the workflow schema reference (wherever `dependencies`/`serviceAccountName` are currently documented for workflow authors)
- Document the new `retry_count` audit payload field wherever `WorkflowExecutionAuditPayload`'s
  other fields are documented for audit consumers (SOC2/FedRAMP evidence reviewers)

### Phase 7 Checkpoint

- [ ] Each REFACTOR item above is either a named, concrete change or an explicit `N/A` + one-line reason — none are just "confirm build/lint/tests pass" (REFACTOR-as-Validation anti-pattern)
- [ ] All tests still PASS (behavior unchanged) — the safety net, not the content
- [ ] `golangci-lint run --timeout=5m` — zero new warnings
- [ ] `go build ./...` — zero errors

---

## Checkpoint 2: Full Due Diligence (Final Gate)

### Correctness Verification

- [ ] **Declared resources reach the Job**: IT-WE-019-001
- [ ] **Absent resources unchanged**: UT-WE-054-JOB-024
- [ ] **Engine-gating enforced**: UT-DS-008-005, UT-DS-008-006
- [ ] **Registration-time fail-fast**: UT-DS-008-002, UT-DS-008-007, UT-DS-008-008
- [ ] **OOM/disruption tolerated**: UT-WE-054-JOB-022, IT-WE-019-002
- [ ] **OOM/disruption tolerated on a real cluster**: E2E-WE-019-001
- [ ] **Genuine failures still fail-fast**: UT-WE-054-JOB-023
- [ ] **Genuine failures still fail-fast on a real cluster**: E2E-WE-019-002
- [ ] **ActiveDeadlineSeconds still the outer bound**: UT-WE-054-JOB-016/017 (pre-existing, re-run as regression guard)
- [ ] **Audit-trail retry-count completeness (BR-WE-019 AC10, Wiring Point C)**: UT-WE-054-JOB-025/026, UT-WE-AUDIT-001/002, IT-WE-019-004, E2E-WE-019-001 (extended)
- [ ] **No duplicate/missing completion audit events despite tolerated retries (BR-AUDIT-005, bundled)**: IT-WE-019-003, E2E-WE-019-001 (extended)

### Regression Verification

- [ ] `go test ./pkg/datastorage/...` — all pass
- [ ] `go test ./pkg/workflowexecution/...` — all pass
- [ ] `go test ./test/integration/workflowexecution/...` — all pass
- [ ] `make test-e2e-workflowexecution` — all pass, including pre-existing `E2E-WE-014-*`

### Compliance / GA Readiness

- [ ] Build: `go build ./...` clean
- [ ] Lint: `golangci-lint run --timeout=5m` clean
- [ ] BDD framework: 100% Ginkgo/Gomega, zero `testing.T`
- [ ] Business requirement: BR-WE-019 all 10 ACs have a passing test (or N/A justification for AC8)
- [ ] Business requirement (bundled): BR-AUDIT-005's count/no-duplication invariant has a passing test at IT and E2E tier (IT-WE-019-003, E2E-WE-019-001 extended) — honestly framed as regression guards, not new-guarantee proof
- [ ] Wiring verification: CHECKPOINT W passes for all 7 Wiring Manifest rows in TEST_PLAN.md Section 14 (confirmed per-wiring-point during Phases 2/4/5/6, not deferred to this final gate)
- [ ] Fail-open safety: engine-gating rejection is loud (registration error), not silent
- [ ] Pyramid Invariant: UT + IT + E2E all present for AC4/AC5/AC9 (controller-loop behavior); two-tier (UT+IT) confirmed sufficient for the remaining, non-controller-loop ACs, with AC10 additionally getting a bonus E2E assertion (reused from AC4's run, not a separate requirement)
- [ ] SOC2/FedRAMP: AC10 (audit retry-count completeness) is implemented and tested in this PR, not deferred (reversing the v1.2-v1.4 deferral — DD-WE-008 Section 9). Remaining out-of-scope item (per-attempt root-cause attribution) is a deliberate, documented scope boundary in BR-WE-019 "Known Limitations", not a silent gap
- [ ] Regression risk (R8): the 5 tightened pre-existing audit assertions (Phase 5.3) pass; any new failure is triaged as a possible real bug, not dismissed as a bad assertion
- [ ] Regression risk (R9): `make generate-datastorage-client`'s diff for the new `retry_count` field is committed; `go build ./...` catches any drift

---

## Confidence Assessment

**Confidence**: 94%
**Justification**: Both original technical unknowns (yaml.v3/corev1 interop, PodFailurePolicy
feature-gate status) were resolved with spikes, not assumptions. The full precedent pattern
(`ServiceAccountName`/`Dependencies`/`EngineConfig`) was traced end-to-end. Three gaps were
caught in review (not by spike) and are now closed in the plan: (1) zero E2E coverage for
controller-loop behavior — closed via Phase 6, reusing an existing fixture for the regression
case and adding one small new deterministic fixture for the retry case; (2) no SOC2/FedRAMP
control mapping — closed by explicit investigation; (3) the v1.2 phase structure grouped
RED/GREEN by test tier and declared GREEN complete on UT alone, violating AGENTS.md's
Wiring-First TDD Sequence and the "UT-Only GREEN" anti-pattern — closed by restructuring around
the actual wiring points (F11), with E2E treated as Wiring Point B's genuine GREEN-completion
gate rather than a bonus phase. A fourth item was bundled after user review of the audit-count
preflight (F12/F13): a new BR-AUDIT-005 regression guard (`IT-WE-019-003`, extended
`E2E-WE-019-001`) and mechanical tightening of 5 pre-existing loose audit assertions — both
honestly framed as regression guards with no genuine RED phase, not new-guarantee proof, and
bounded in effort since they reuse an already-proven `countEventsByType` pattern.

**A fifth, more significant revision (F14)**: the v1.2-v1.4 deferral of the audit-*content* fix
(retry count in the payload) was reversed after direct user challenge exposed its stated
justification ("cross-cutting audit infrastructure shared by every audit-emitting service") as
incorrect — `WorkflowExecutionAuditPayload` is exclusively WFE-owned, ogen regen is a single
command, and the CRD/reconciler plumbing already exists. This is now **Wiring Point C**
(BR-WE-019 AC10), following the same wiring-first RED->GREEN discipline as A and B (Phase 5).
This is a genuine, not cosmetic, scope increase — it adds one new CRD field, one new OpenAPI
field + regen, and ~10 lines of production logic across two files, comparable in size to Wiring
Point A. The confidence rating is held to 94% (down slightly from 95%) not because Wiring Point
C is risky, but to honestly reflect that a plan requiring a second material reversal in the same
session (F11's TDD-ordering fix, now F14's scope-deferral reversal) carries a residual process
risk: due-diligence findings that later prove incomplete or overstated. Residual technical
risks: `onExitCodes: [137]` cannot structurally distinguish OOM-kill from an arbitrary SIGKILL
(accepted, documented, no code-level fix available); `E2E-WE-019-001` has a manual, credentialed
image-publish dependency that is outside this PR's automatable scope (tracked as a blocking
pre-req for Wiring Point B's GREEN-completion, not hidden). The Tekton-timing-related loose
assertions were deliberately excluded from this bundle (needs a design decision, tracked
separately) to keep this PR's scope from creeping into an unrelated, open-ended fix. With AC10
now in scope, BR-WE-019 ships with no deferred acceptance-criterion-level gap — the only
remaining out-of-scope item (per-attempt root-cause attribution) was always a deliberate
non-goal, not a deferral.

---

## Changelog

| Version | Date | Changes |
|---|---|---|
| 1.0 | 2026-07-06 | Initial implementation plan |
| 1.1 | 2026-07-06 | Added Phase 3b (E2E RED: new `job-oomkill` fixture + `E2E-WE-019-001/002`), updated Checkpoint 2 and Wiring Manifest references, documented the deferred audit-completeness gap (F8) and revised confidence to 94% to reflect the two gaps caught in review |
| 1.2 | 2026-07-06 | Replaced `Resources interface{}` with a concrete `ResourcesSchema` struct (F9), removing the JSON round-trip entirely (simpler, not just cosmetic). Surfaced and documented (not fixed) a pre-existing ADR-046 gap: `WorkflowSchema` validation tags are never enforced anywhere in `pkg/datastorage` (F10), tracked as a separate follow-up issue |
| 1.3 | 2026-07-06 | Restructured Phases 1-4 into genuine wiring-first order per AGENTS.md's Wiring-First TDD Sequence (F11) — was grouping RED/GREEN by test tier (all UT, then all IT/E2E) and declaring GREEN complete on UT alone, an "UT-Only GREEN" anti-pattern violation. Now organized per wiring point: Phase 1-2 (resources: RED IT-then-UT, GREEN wire-then-implement), Phase 3-4 (PodFailurePolicy: same sequence), Phase 5 (E2E as Wiring Point B's actual GREEN-completion gate, since envtest can't reach controller-loop behavior), Phase 6 (REFACTOR: named improvements or explicit N/A, per the newer AGENTS.md REFACTOR-is-content-not-validation guidance pulled in from origin/main). Confidence revised to 95% |
| 1.4 | 2026-07-06 | Added F12/F13 findings and new Phase 4.5 (Bundled Audit-Count Regression Guards, cites BR-AUDIT-005): new `IT-WE-019-003`, extended `E2E-WE-019-001`, and mechanical tightening of 5 pre-existing loose audit assertions in `reconciler_test.go`/`02_observability_test.go`. Per user decision, bundled into this PR rather than split into separate follow-up PRs to avoid repeated ~20-30min CI cycles; Tekton-timing-related loose assertions in `audit_flow_integration_test.go` explicitly excluded (design decision needed, tracked as a separate follow-up issue). Updated Checkpoint 2 and Confidence Assessment accordingly |
| 1.5 | 2026-07-06 | **Added F14 and Wiring Point C** (BR-WE-019 AC10, audit retry-count completeness), reversing the v1.2-v1.4 deferral after user challenge showed its justification incorrect. Renumbered: former Phase 4.5 -> Phase 5 (now also holds Wiring Point C's genuine RED->GREEN alongside the pre-existing bundled BR-AUDIT-005 guards), former Phase 5 (E2E of B) -> Phase 6 (extended with the AC10 real-cluster assertion), former Phase 6 (REFACTOR) -> Phase 7 (new 7.3 for Wiring Point C). Updated Checkpoint 2, Wiring Manifest count (6->7 rows), and Confidence Assessment (95% -> 94%, reflecting the process risk of a second material reversal in-session, not new technical risk) |

# DD-WE-008: Job Resource Governance and Transient-Failure Tolerance

**Version**: 1.5
**Date**: 2026-07-06
**Status**: PROPOSED
**Author**: WorkflowExecution Team
**Reviewers**: Platform Team, DataStorage Team

> **Superseded field location (Issue #1661 Change 11f, 2026-07-22)**: `Resources`
> (along with `ExecutionEngine`, `ServiceAccountName`, and `ActionType`) has
> moved from `wfe.Status` to the immutable, CRD-embedded
> `wfe.Spec.WorkflowRef.Resources`, set once by the RemediationOrchestrator
> creator at WFE-creation time. The WE controller's `resolveWorkflowCatalog`
> runtime-resolution step described below (and the `wfe.Status.Resources`
> writes it performed) has been removed entirely -- there is no longer a DS
> catalog round-trip or a Status mirror to keep in sync. The scenario
> analysis and rationale below remain historically accurate for how the
> value is *sourced upstream* (DS workflow catalog `execution.resources`);
> only the CRD field that carries it into the WFE has changed.

---

## Context

`JobExecutor.buildJob()` (`pkg/workflowexecution/executor/job.go`) creates the Kubernetes
`Job` that runs a remediation workflow's `"workflow"` container with two gaps:

1. **No `resources.requests`/`resources.limits`** on the container. Kubernetes classifies
   such a pod `BestEffort` QoS — the first candidate the kubelet evicts under node memory
   pressure, and the container has no CPU/memory floor or ceiling at all.
2. **`backoffLimit: 0`** (hardcoded). Any pod failure — including a transient,
   infrastructure-caused one (OOM-kill, node eviction/preemption) — immediately and
   permanently fails the `WorkflowExecution`. There is no distinction between "the
   remediation script legitimately failed" and "the node evicted the pod."

### Triggering Incident

Issue #1564: a CI run's WFE job was OOM-killed under memory pressure unrelated to the
workflow's own logic. Because `backoffLimit: 0`, the Job (and therefore the WFE, and the
remediation attempt) failed permanently on that single transient event, with no retry.

### Relationship to Existing WE Design Decisions

| Concern | Established precedent | This decision |
|---|---|---|
| Per-workflow `serviceAccountName` | DD-WE-005 v2.0: dedicated DB column, resolved into `WFE.Status` | New per-workflow `resources`: **no dedicated DB column** — extracted from `content` like `dependencies`/`engineConfig` (see Scenario 3 below) |
| Schema-declared infra (Secrets/ConfigMaps) | DD-WE-006: declared in `execution`-adjacent `dependencies` section, resolved on demand from DS `content` | Same on-demand extraction pattern, applied to `execution.resources` |
| Pod/container hardening | BR-WE-018: hardcoded, non-configurable `SecurityContext` (no opt-out, to close an AI-driven-CR-creation attack surface) | `resources` **is** configurable per workflow (unlike `SecurityContext`) because under-provisioning resources has no security blast radius — it only affects the workflow's own reliability, and operators/authors need the ability to size CPU/memory per workload |

---

## Decision

**Two independent, additive mechanisms, both scoped to the Kubernetes Job execution engine only:**

1. **Per-workflow `execution.resources`** (optional, `corev1.ResourceRequirements` shape) in
   `workflow-schema.yaml`, resolved into `WFE.Status.Resources` at runtime and applied to the
   `"workflow"` container in `buildJob()`. Absent → today's behavior (no resources block,
   `BestEffort` QoS) is preserved exactly, so this is fully backward compatible.
2. **`Job.Spec.PodFailurePolicy`**, unconditionally added to every Job `buildJob()` creates
   (not tied to whether `resources` is set): pod failures caused by OOM-kill (container exit
   code 137) or node eviction/preemption (`DisruptionTarget` pod condition) are `Ignore`d —
   i.e. not counted against `backoffLimit` — while every other failure keeps today's `Count`
   (fail-fast) behavior against the existing `backoffLimit: 0`.

`backoffLimit` itself is **not** changed. Increasing it blindly would let a genuinely-failing,
non-idempotent remediation action retry automatically, which is a correctness/safety risk this
decision deliberately avoids. `PodFailurePolicy` gives infra-transient-failure tolerance
without that risk, and without weakening today's "fail fast on a real error" contract.

### Explicitly out of scope (see Alternatives)

- **Tekton engine resources**: `PipelineRunSpec.TaskRunTemplate.PodTemplate` has no
  resources-override field. Tekton Task authors already control container resources via
  `steps[].resources`/`stepTemplate.resources` inside the OCI bundle — this is the existing,
  documented WE/Tekton asymmetry (BR-WE-018).
- **Tekton engine retries**: Tekton has no `PodFailurePolicy`-equivalent for `TaskRun`s.
  Solving this requires the controller itself to inspect *why* the underlying pod died
  (a harder, controller-level problem) and is deferred to a future issue.
- **Ansible engine**: `AnsibleExecutor` never constructs a Pod/Job; it dispatches to AWX/AAP
  over HTTP. Resource/retry tuning is an AWX Job Template concern, outside kubernaut's control.
- **Fleet-wide namespace default** (`LimitRange`/`ResourceQuota` on `kubernaut-workflows`):
  deliberately left to the platform/cluster operator. See "Residual Risk" below.

---

## Spike Findings (Pre-Implementation Validation)

Two technical unknowns were resolved with throwaway Go programs before committing to this design:

### Finding 1: `gopkg.in/yaml.v3` cannot directly unmarshal `corev1.ResourceRequirements`

The workflow schema parser (`pkg/datastorage/schema/parser.go`) unmarshals
`workflow-schema.yaml` with `gopkg.in/yaml.v3`, and `WorkflowSchema` struct fields use `yaml:"..."`
tags. `corev1.ResourceRequirements` (and `resource.Quantity` inside it) only implements the
**JSON** marshal/unmarshal interfaces (`UnmarshalJSON`), not yaml.v3's `UnmarshalYAML(*yaml.Node)`.
Embedding it directly as a typed field would silently fail to parse quantity strings like `500m`
or `512Mi` (yaml.v3 would attempt struct-reflection over `Quantity`'s unexported fields instead
of using its JSON codec).

`EngineConfig` (BR-WE-016) works around an analogous problem by typing itself `interface{}` and
going through a `json.Marshal`/`json.Unmarshal` round-trip, because its shape genuinely varies
per engine (a true discriminated union). **`resources` does not need that pattern**: its shape
is fixed and known (`requests`/`limits`, each a map of resource-name → quantity *string*), and
yaml.v3 unmarshals `map[string]string` natively with no special handling. So instead of
`interface{}` + JSON round-trip, this decision uses a small concrete struct
(`ResourcesSchema`, plain `map[string]string` fields) and parses each quantity string directly
with `resource.ParseQuantity()` — the same parser Kubernetes itself uses for admission — rather
than routing through `encoding/json`. This avoids `interface{}` entirely (AGENTS.md Go
Anti-Pattern Checklist flags bare `any`/`interface{}` usage) and gives per-field error messages
naming the exact offending resource key, not just a generic JSON unmarshal error. Verified via
spike:

```go
// yaml.v3 unmarshals map[string]string natively -- no interface{}, no JSON
// round-trip needed. Quantity strings are parsed directly.
var schema struct {
    Resources struct {
        Requests map[string]string `yaml:"requests"`
        Limits   map[string]string `yaml:"limits"`
    } `yaml:"resources"`
}
yaml.Unmarshal(doc, &schema)                         // Requests["cpu"] == "100m"
q, err := resource.ParseQuantity(schema.Resources.Requests["cpu"])  // succeeds, q.String() == "100m"
```

Confirmed working for both quoted (`cpu: "1"`) and unquoted (`memory: 512Mi`) YAML scalar forms.

### Finding 2: invalid quantities error out; `requests > limits` does not (needs explicit validation)

- An invalid quantity string (e.g. `cpu: not-a-number`) fails `resource.ParseQuantity()` with a
  clear error (`quantities must match the regular expression ...`), which this decision wraps
  into a `SchemaValidationError` naming the exact field (`execution.resources.requests.cpu`) at
  DS registration time.
- A structurally valid but semantically inverted requests/limits pair (e.g. `requests.cpu: 2`,
  `limits.cpu: 1`) does **not** error on parsing — both quantities parse fine individually.
  Kubernetes' API server rejects this at Pod admission, but only when the Job is actually
  created against a live cluster — for fast feedback to workflow authors, this decision adds an
  explicit `requests <= limits` check (via `resource.Quantity.Cmp()`) at DS registration time
  (see Implementation).

### Finding 3: `JobPodFailurePolicy` feature-gate status across kubernaut's supported range

`JobPodFailurePolicy` graduated **alpha in v1.25, beta in v1.26, GA/stable (locked-on) in
v1.31** — not v1.27 as initially assumed. Checked against kubernaut's actual minimum
(`v1.30+`, per DD-CRD-003): confirmed via the upstream `v1.30` release branch
(`pkg/features/kube_features.go`) that the beta gate is `{Default: true, PreRelease: Beta}` —
**enabled by default** on v1.30 clusters, and unconditionally on from v1.31 onward. No
feature-gate handling or version-conditional logic is required.

---

## Scenarios Evaluated

### Scenario 1: Fleet-wide default resources via controller config

A single `DefaultJobResources` value in the WE controller's config (`pkg/workflowexecution/config/config.go`),
applied to every Job regardless of the workflow catalog entry.

**Pros**: Zero schema changes; every workflow gets a floor immediately.
**Cons**: One size does not fit workflows with wildly different footprints (a `kubectl patch`
one-liner vs. a data-migration script); conflates "platform default" with "per-workflow
authoring," which the codebase already separates for `serviceAccountName`/`dependencies`.
**Decision**: Rejected — inconsistent with the established per-workflow-catalog-entry pattern.

### Scenario 2: `resources` inside `WFE.Spec.ExecutionConfig` (per-incident override)

**Pros**: No DataStorage/schema changes; `ExecutionConfig` already exists for `Timeout`.
**Cons**: `ExecutionConfig` is populated per-incident by `RemediationOrchestrator`
(`pkg/remediationorchestrator/creator/workflowexecution.go`), which has no visibility into a
workflow's actual resource footprint — that is workflow-authoring knowledge, not
incident-routing knowledge. Every incident using the same workflow would need to
independently guess the same resource values.
**Decision**: Rejected — wrong architectural layer. Resource sizing is a property of the
*workflow* (like its container image or engine), not of the *incident* being remediated.

### Scenario 3: `execution.resources` as a dedicated DB column (mirrors `serviceAccountName`)

Add a `resources` (`JSONB`) column to `remediation_workflow_catalog`, migration + OpenAPI
schema field + ogen regen, mirroring DD-WE-005's `service_account_name` column.

**Pros**: Symmetric with the existing SA pattern; queryable/filterable in principle.
**Cons**: Unlike `serviceAccountName`, `resources` has no search/discovery/filtering use case
(the LLM never needs to see or filter on resource requests when selecting a workflow) — a
dedicated column would be write-once, read-once-per-WFE-execution, with no other consumer.
This is a materially larger blast radius (migration, OpenAPI spec, ogen-client regen,
repository CRUD, `WorkflowSearchResult` field) for no behavioral benefit over Scenario 4.
**Decision**: Rejected in favor of Scenario 4 — `dependencies` and `engineConfig` already
establish the "extract from `content` on demand, no dedicated column" pattern for exactly this
kind of structured, WFE-only-consumed data.

### Scenario 4: `execution.resources` extracted from `content` on demand (Selected)

Add `Resources *ResourcesSchema` to `models.WorkflowExecution` (the yaml-parsed `execution`
block), with a new `Parser.ExtractResources()` that parses each quantity string via
`resource.ParseQuantity()` (Finding 1) and performs the `requests <= limits` check (Finding 2),
called by `WorkflowQuerier.ResolveWorkflowCatalogMetadata` exactly like
`ExtractDependencies`/`ExtractEngineConfig` already are.

**Pros**: No DB migration, no OpenAPI/ogen-client regen, no repository/CRUD changes — the
entire DataStorage-side change is additive to already-existing `models`/`schema` packages.
Consistent with the two closest structural precedents (`dependencies`, `engineConfig`).
**Cons**: Re-parses the full YAML `content` on every WFE-Pending reconcile (already happens
today for `dependencies`/`engineConfig` — no new I/O pattern introduced).
**Decision**: Selected.

### Scenario 5: `backoffLimit` increase (e.g. `backoffLimit: 3`) instead of `PodFailurePolicy`

**Pros**: Single-line change, no new K8s API surface.
**Cons**: Blindly retries *any* pod failure, including a genuinely-failing, potentially
non-idempotent remediation script — the exact risk this decision must avoid. Also retries
consume the same budget regardless of cause, so tuning it to tolerate infra flakiness without
also tolerating repeated real failures is not possible with this field alone.
**Decision**: Rejected — see "Decision" above.

### Scenario 6: `PodFailurePolicy` with `FailJob` fast-path for known-fatal exit codes

Considered adding an explicit `FailJob` rule for some sentinel "do not retry" exit code
convention that workflow authors could opt into.
**Pros**: Would let workflow authors signal "this failure is definitely not worth retrying"
explicitly.
**Cons**: No existing convention for this in the workflow catalog; would require a new
authoring contract (a reserved exit code) with no current demand. Speculative (YAGNI) per the
Go Anti-Pattern Checklist.
**Decision**: Rejected for this iteration. The default `Count` action already achieves
fail-fast for everything not explicitly `Ignore`d, which is sufficient for the identified
problem (#1564). Can be revisited if a concrete need emerges.

---

## Implementation

### 1. Workflow Schema Extension

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: RemediationWorkflow
metadata:
  name: oomkill-increase-memory-v1
spec:
  execution:
    engine: job
    bundle: quay.io/kubernaut-cicd/test-workflows/oomkill-fix-job@sha256:...
    resources:
      requests:
        cpu: 100m
        memory: 128Mi
      limits:
        cpu: "1"
        memory: 512Mi
```

- `execution.resources` is **only valid when `engine: job`**. If declared for any other engine
  (including the `tekton` default), registration is rejected with a clear error — a silent
  no-op would otherwise leave workflow authors believing a Tekton workflow respects a field it
  cannot use (GA Readiness dimension #12, Fail-Open Safety: no silent failures).
- Absent `resources` → unchanged today's behavior (`BestEffort`, no resources block).

### 2. Go Types (`pkg/datastorage/models/workflow_schema.go`)

```go
// ResourcesSchema declares CPU/memory requests and limits for the "workflow"
// container (Job engine only). Plain string-keyed maps (yaml.v3-native)
// rather than corev1.ResourceRequirements directly -- resource.Quantity only
// implements the JSON codec (DD-WE-008 Finding 1); yaml.v3 would silently
// mis-parse it via struct-reflection over its unexported fields. Quantity
// strings are parsed explicitly in ExtractResources via resource.ParseQuantity.
// validate tags follow the sibling-struct convention in this file (see
// ADR-046) but are not currently enforced -- WorkflowSchema validation does
// not yet call validator.Struct() anywhere (tracked separately, ADR-046
// Phase 1 gap, not addressed by this decision).
type ResourcesSchema struct {
    // +optional
    Requests map[string]string `yaml:"requests,omitempty" json:"requests,omitempty" validate:"omitempty,dive,keys,required,endkeys,required"`
    // +optional
    Limits map[string]string `yaml:"limits,omitempty" json:"limits,omitempty" validate:"omitempty,dive,keys,required,endkeys,required"`
}

type WorkflowExecution struct {
    // ... existing fields (Engine, Bundle, BundleDigest, ServiceAccountName, EngineConfig) ...

    // Resources declares CPU/memory requests and limits for the "workflow"
    // container (Job engine only). Same shape as a Kubernetes container's
    // `resources` block.
    // +optional
    Resources *ResourcesSchema `yaml:"resources,omitempty" json:"resources,omitempty"`
}
```

### 3. Schema Parser (`pkg/datastorage/schema/parser.go`)

```go
// ExtractResources parses the optional execution.resources section into a
// typed corev1.ResourceRequirements. Returns (nil, nil) when absent. Returns
// a SchemaValidationError naming the exact offending resource key for an
// invalid quantity, or naming "execution.resources" for a request exceeding
// its corresponding limit (Finding 2).
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

// parseResourceQuantities converts a yaml-native map[string]string (e.g.
// {"cpu": "100m", "memory": "512Mi"}) into a corev1.ResourceList using
// resource.ParseQuantity -- the same parser Kubernetes itself uses for
// admission. Returns nil (not an empty map) for an empty input, matching
// corev1's own zero-value convention for ResourceList.
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

`validateWorkflowExecution` gains two checks, mirroring the existing `validateAnsibleEngineConfig`
engine-gating pattern:

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

### 4. WorkflowQuerier (`pkg/workflowexecution/client/workflow_querier.go`)

`WorkflowCatalogMetadata` gains `Resources *corev1.ResourceRequirements`, populated in
`ResolveWorkflowCatalogMetadata` alongside the existing `Dependencies`/`EngineConfig` extraction
(same `if wf.Content != ""` block, same parser instance).

### 5. WFE CRD (`api/workflowexecution/v1alpha1/workflowexecution_types.go`)

```go
// Resources declares the resolved CPU/memory requests and limits for the
// Job engine's "workflow" container, from the DS workflow catalog
// (BR-WE-019 / DD-WE-008). Set once during Pending phase via
// ResolveWorkflowCatalogMetadata; immutable thereafter. nil when the
// catalog entry declares no resources (BestEffort QoS, current behavior).
// +optional
Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
```

Added to `WorkflowExecutionStatus` next to `ServiceAccountName` (same resolution phase, same
"resolved once, immutable thereafter" lifecycle). Requires `make generate manifests` to
regenerate `DeepCopy` and the CRD YAML (`config/crd/bases/`, synced to
`charts/kubernaut/{crds,files/crds}/`).

### 6. Controller Wiring (`internal/controller/workflowexecution/workflowexecution_catalog.go`)

```go
wfe.Status.ExecutionEngine = meta.ExecutionEngine
wfe.Status.ServiceAccountName = meta.ServiceAccountName
wfe.Status.Resources = meta.Resources // new
```

Same idempotency guard as today (`resolveWorkflowCatalog` short-circuits once
`wfe.Status.ExecutionEngine != ""`), so this is set exactly once per WFE.

### 7. Job Executor (`pkg/workflowexecution/executor/job.go`)

```go
Containers: []corev1.Container{
    {
        Name:            "workflow",
        Image:           wfe.Spec.WorkflowRef.ExecutionBundle,
        Env:             envVars,
        VolumeMounts:    mounts,
        SecurityContext: restrictedContainerSecurityContext(),
        Resources:       resourcesFor(wfe), // new
    },
},
```

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

`JobSpec` unconditionally gains:

```go
PodFailurePolicy: &batchv1.PodFailurePolicy{
    Rules: []batchv1.PodFailurePolicyRule{
        {
            // Node eviction/preemption/other disruption: not the workflow's fault.
            Action: batchv1.PodFailurePolicyActionIgnore,
            OnPodConditions: []batchv1.PodFailurePolicyOnPodConditionsPattern{
                {Type: corev1.DisruptionTarget, Status: corev1.ConditionTrue},
            },
        },
        {
            // OOM-kill (SIGKILL, exit 137): see "Known Limitation" below.
            Action: batchv1.PodFailurePolicyActionIgnore,
            OnExitCodes: &batchv1.PodFailurePolicyOnExitCodesRequirement{
                ContainerName: ptr.To("workflow"),
                Operator:      batchv1.PodFailurePolicyOnExitCodesOpIn,
                Values:        []int32{137},
            },
        },
        // No explicit default rule: unmatched failures fall through to the
        // Job controller's built-in default, which is Count -- i.e. today's
        // fail-fast behavior against backoffLimit is unchanged for every
        // failure that isn't OOM-kill or a K8s-initiated disruption.
    },
},
```

`RestartPolicy: Never` is a hard K8s requirement for `PodFailurePolicy` (already set today).

**Known limitation** (accepted, not solvable at the K8s API level): `onExitCodes` matches
purely on the numeric exit code. Exit 137 (SIGKILL) is emitted both when the cgroup OOM killer
fires *and* whenever any other process sends the container `SIGKILL` — the Job API has no way
to match on the kubelet's more specific container-status `reason: OOMKilled` field. In practice
exit 137 in a `kubernaut-workflows` Job pod is overwhelmingly caused by the OOM killer (nothing
else in this pod's lifecycle sends arbitrary signals), so this is treated as an acceptable proxy.

**Safety interaction with `ActiveDeadlineSeconds`**: because `Ignore`-classified failures are
excluded from `backoffLimit` accounting, a workflow whose resources are chronically
under-provisioned could in principle retry indefinitely on repeated OOM-kills. The Job's
existing `ActiveDeadlineSeconds` (BR-WORKFLOW-008, defaulting to 30 minutes) remains the outer
wall-clock bound regardless of how many `Ignore`d retries occur, so this cannot hang forever —
it terminates as `JobFailed` (`DeadlineExceeded`) once the deadline elapses.

### 8. E2E Proof of the Retry-Tolerance Journey (Real Cluster)

envtest has no kubelet and does not run the Job controller's pod-failure-policy evaluation
loop, so IT-WE-019-002 can only prove the `PodFailurePolicy` manifest is schema-valid — it
cannot prove that a real cluster actually *applies* `Ignore` semantics and creates a
replacement pod. Per the Pyramid Invariant ("E2E proves the journey" — AGENTS.md), this
decision adds a real-cluster E2E tier rather than treating the real-OOM scenario as simply
untestable:

- **E2E-WE-019-001**: a new minimal test fixture (`job-oomkill`, a UBI-minimal container that
  unconditionally `exit 137`s — deterministic, no stateful retry-then-succeed logic needed)
  proves that within a bounded observation window the real Job controller creates >= 3 pod
  attempts (counted via `SuccessfulCreate` Events on the Job — see Section 9's v1.5 correction;
  **not** `job.Status.Failed`, which this very test run against a real cluster empirically
  disproved as a usable signal, see below) while the WFE remains `Running` (not `Failed`) — the
  concrete, real-cluster proof that `Ignore` is in effect. The test does not wait for the full
  `ActiveDeadlineSeconds` (30 min) to elapse; it observes the tolerated-retry behavior within
  a short window and then deletes the WFE, avoiding a 30-minute CI test.
- **E2E-WE-019-002**: reuses the existing `test-job-intentional-failure` fixture (`exit 1`,
  no new image) to prove `PodFailurePolicy`'s unconditional addition does **not** weaken
  fail-fast behavior for a genuine failure — `job.Status.Failed` reaches exactly `1` and the
  Job/WFE reach `Failed` within the existing SLA window. (Unaffected by the v1.5 correction:
  genuine, non-`Ignore`d failures still use the default `Count` action, which *does* increment
  `job.Status.Failed` normally.)

**v1.5 correction — `job.Status.Failed` does not observe `Ignore`-tolerated failures at all**:
running `E2E-WE-019-001` against a real cluster falsified this section's original assertion.
`k8s.io/api batch/v1`'s `PodFailurePolicyActionIgnore` doc comment states plainly: "the counter
towards `.backoffLimit`, represented by the job's `.status.failed` field, is **not** incremented"
for `Ignore`-action failures. A follow-up spike (a bare Kind cluster, no Kubernaut components)
confirmed this directly: `job.Status.Failed` stayed `0` across 3 confirmed exit-137 pod failures
and remained `0` even after the Job reached a terminal `DeadlineExceeded` condition. This
invalidated not just this test's original assertion, but also Section 9's originally-chosen
`RetryCount` data source — see Section 9's v1.5 correction for the replacement mechanism
(`SuccessfulCreate` Event counting) both now share.

**Operational dependency**: `E2E-WE-019-001` requires a new test container image
(`quay.io/kubernaut-cicd/test-workflows/job-oomkill:v1.0.0`), built and pushed via
`test/fixtures/job/build-and-push.sh` (manual, requires `quay.io/kubernaut-cicd` push
credentials — the same manual step already required for the existing `job-hello-world`/
`job-failing` images). This is a one-time setup action, tracked in the implementation plan,
not an ongoing operational cost.

### 9. Wiring Point C: Audit-Trail Retry-Count Completeness (In Scope, Corrected From v1.2's Deferral)

Investigating SOC2 CC8.1/AU-3 alignment for this decision surfaced a real, pre-existing
audit-completeness gap that this decision **introduces the conditions for** (it did not exist
before, because `backoffLimit: 0` guaranteed at most one pod attempt per Job):

`JobExecutor.buildStatusSummary()` only inspects `job.Status.Failed` on the branch where the
Job has *not yet* succeeded; once `job.Status.Succeeded > 0`, that count is never read. The
audit payload builder (`buildWorkflowExecutionAuditPayload`, `pkg/workflowexecution/audit/manager.go`)
carries no field for it either. Consequence: a remediation that required tolerating N
OOM-kill/disruption retries before succeeding is **indistinguishable in the durable audit
trail** from one that succeeded cleanly on the first attempt — a genuine CC8.1 ("complete
remediation request reconstruction from audit traces alone") / AU-3 (content of audit records)
completeness gap for the *fact* that a retry occurred. Root-cause attribution per attempt
(OOM-kill vs. disruption) remains separately and permanently best-effort only (see `job.go`'s
`enrichFailureMessage`, which already documents that failed Pods are typically GC'd before
`GetStatus` observes the terminal Job — a fully guaranteed causal trail would require
real-time audit emission on pod failure, not point-in-time polling) — **that** narrower gap is
still out of scope for this decision.

**v1.2 of this decision deferred the retry-*count* fix, on the stated grounds that it required
"a cross-cutting change to audit infrastructure shared by every audit-emitting service." On
re-investigation (prompted by direct user challenge), that justification does not hold up**:

- `WorkflowExecutionAuditPayload` (`api/openapi/data-storage-v1.yaml`) is used exclusively by
  WorkflowExecution's own 5 event types (`workflow.started/completed/failed`,
  `selection.completed`, `execution.started`). It is not shared with any other service's
  payload schema — each service (Notification, AIAnalysis, etc.) has its own dedicated payload
  type in the same spec file. Adding an optional field to this schema has zero effect on any
  other service's generated types.
- Regenerating the ogen client is `make generate-datastorage-client` → a single
  `go generate ./pkg/datastorage/ogen-client/...` command — not a multi-step or
  cross-team process.
- The data is already in hand at the point it is needed: (as originally believed) `job.Status.Failed`
  is read by `buildStatusSummary()` on the `job` object already fetched by `GetStatus()`; it is
  simply not captured on the success branch due to an `if Succeeded > 0 { ... } else if Failed >
  0 { ... }` exclusivity. `ExecutionStatusSummary` (the struct `buildStatusSummary` returns)
  already flows durably into `wfe.Status.ExecutionStatus` via `MarkCompleted`/`MarkFailed`
  (`internal/controller/workflowexecution/workflowexecution_status_marking.go`) — the same CRD
  status struct, using the same `make generate manifests` step Wiring Point A (Section 5) is
  already exercising for `Status.Resources`. **(v1.5: this bullet's premise was wrong — see the
  v1.5 correction below. The wiring-simplicity argument for bringing this in scope still holds;
  only the data source changes.)**

**Corrected decision**: this is genuinely comparable in size to Wiring Point A, not a
cross-cutting project. It is now **in scope for this PR**, as **Wiring Point C**:

1. `ExecutionStatusSummary` (`api/workflowexecution/v1alpha1/workflowexecution_types.go`) gains
   `RetryCount int32 `json:"retryCount,omitempty"`` — the number of `PodFailurePolicy`-tolerated
   pod-creation attempts observed before the terminal state, unconditionally captured (not gated
   by success/failure)
2. `buildStatusSummary()` sets `summary.RetryCount` unconditionally from `countPodCreationAttempts()`
   (see v1.5 correction below for what that reads), instead of only inside an
   `else if job.Status.Failed > 0` branch
3. `WorkflowExecutionAuditPayload` (OpenAPI spec) gains an optional `retry_count: {type: integer,
   format: int32}` field; `make generate-datastorage-client` regenerates the ogen client
4. `buildWorkflowExecutionAuditPayload()` sets `payload.RetryCount` from
   `wfe.Status.ExecutionStatus.RetryCount` when greater than zero

**v1.5 correction — `job.Status.Failed` is not a usable `RetryCount` data source**: running
`E2E-WE-019-001` against a real cluster falsified bullet 2's original mechanism
(`summary.RetryCount = job.Status.Failed`). `k8s.io/api batch/v1`'s
`PodFailurePolicyActionIgnore` doc comment states plainly: "the counter towards
`.backoffLimit`, represented by the job's `.status.failed` field, is **not** incremented" for
`Ignore`-action failures — confirmed empirically (a bare Kind cluster, no Kubernaut components):
`job.Status.Failed` stayed `0` across 3 confirmed exit-137 pod failures, including after the Job
reached a terminal condition. Since AC10 exists specifically to capture *tolerated* (i.e.
`Ignore`-action) retries, the original mechanism would have always recorded `RetryCount: 0` for
exactly the case it was built for — a silent, self-defeating no-op, not merely an edge case.

Two alternatives were evaluated and rejected before settling on the corrected mechanism:

- **Watch-based Pod tracking** (incrementally count Pod-creation events as they happen, via a
  live `Watch` on Pods owned by the Job): more precise in principle, but architecturally
  impossible for Fleet-federated remote clusters — `ExecutorClient`
  (`pkg/workflowexecution/executor/client_factory.go`) only exposes `Get`/`List`/`Create`/`Delete`
  for MCP-backed remote execution, no `Watch` verb, since Fleet federation is a request/response
  API (not a persistent streaming channel). This would produce accurate `RetryCount` for local
  execution and silently-always-zero for Fleet — an inconsistency worse than the bug being fixed.
- **Relaxing AC10 to best-effort with a documented compliance caveat**: rejected as a first
  resort — a working alternative existed (see below), so weakening the acceptance criterion was
  unnecessary.

**Corrected mechanism (both Section 8 and this section)**: `countPodCreationAttempts()`
(`pkg/workflowexecution/executor/job.go`) lists `corev1.Event`s in the Job's namespace, sums the
`.Count` field of every Event with `InvolvedObject.Kind == "Job"`, `InvolvedObject.Name ==
<job.Name>`, and `Reason == "SuccessfulCreate"` — the Kubernetes job-controller's Event Reason
for every Pod it creates, initial attempt and every `Ignore`-tolerated replacement alike — then
returns `total - 1`. This works identically for local and Fleet execution (`List` is available on
both), and survives Pod garbage collection because Events have their own TTL (~1h default),
independent of and outlasting individual Pods, mirroring the existing GC-timing rationale already
documented on `enrichFailureMessage`. A dedicated spike (a bare Kind cluster, no Kubernaut
components) confirmed empirically: exactly one `SuccessfulCreate` Event per Pod created (`Count:
1` each, well under Kubernetes' ~10-events/10-minute Event-aggregation threshold), and all Events
remained queryable after the Job transitioned to a terminal (`Failed`/`DeadlineExceeded`)
condition — comfortably inside this Job's 30-minute `ActiveDeadlineSeconds` ceiling. Like the
mechanism it replaces, this remains a best-effort signal, not a mathematical guarantee (see scope
boundary below): it depends on a cluster-operator-controlled `kube-apiserver --event-ttl`
outliving the Job's `ActiveDeadlineSeconds`, and on `SuccessfulCreate` remaining a stable
Kubernetes-internal Reason string (not a versioned API contract). `countPodCreationAttempts()`
logs a warning if a Job that reached a terminal outcome has zero matching Events, so a future
regression in either assumption is observable rather than silently understating the audit trail.

**Scope boundary (unchanged from v1.2)**: this hard-guarantees the retry *count* only.
Root-cause attribution per attempt (OOM-kill vs. disruption) remains best-effort via the
existing Event-scanning pattern; a fully-guaranteed real-time causal audit trail (recording each
attempt's specific failure reason) would be its own, larger DD if ever required — not attempted
here.

**Also bundled in this PR (distinct concern, no genuine RED phase)**: preflighting this
section's SOC2 concern separately raised whether tolerated retries could cause a *duplicate* (or
missing) completion audit *event*, independent of the *content* fix above. They cannot — audit
emission is already gated on terminal-phase transition, not on reconcile count — but this was
previously untested. A forward-looking regression guard for this invariant (`IT-WE-019-003`,
extended `E2E-WE-019-001`, citing BR-AUDIT-005) is bundled alongside Wiring Point C, plus
mechanical tightening of 5 unrelated pre-existing loose audit assertions. See
IMPLEMENTATION_PLAN.md Phase 5 and TEST_PLAN.md Section 4.4 for detail.

### 10. Residual Risk: No Fleet-Wide Namespace Default

kubernaut deliberately does **not** ship a `LimitRange`/`ResourceQuota` for the
`kubernaut-workflows` namespace, from either the Helm chart or `kubernaut-operator`. Rationale:
default sizing varies too much across environments and risks conflicting with a platform
team's existing namespace governance. This is documented as an operator-facing concern
(see companion tracking below), not a code change in this repo.

Without either an operator-provisioned `LimitRange` or a per-workflow `resources` declaration
(this decision, opt-in), a workflow still runs `BestEffort` QoS by default — this decision
closes the "no way to declare resources at all" gap but does not itself force every workflow to
declare them.

---

## Affected Components

| Component | Team | Change |
|---|---|---|
| Workflow schema types | DataStorage | Add `ResourcesSchema` type and `Resources *ResourcesSchema` field to `WorkflowExecution` (no `interface{}`) |
| Schema parser | DataStorage | Add `ExtractResources()`; extend `validateWorkflowExecution` (engine-gating + requests<=limits) |
| WorkflowQuerier | WorkflowExecution | Add `Resources` to `WorkflowCatalogMetadata`; populate in `ResolveWorkflowCatalogMetadata` |
| WFE CRD | WorkflowExecution | Add `Status.Resources *corev1.ResourceRequirements`; `make generate manifests` |
| WFE controller | WorkflowExecution | Set `wfe.Status.Resources` in `resolveWorkflowCatalog` |
| Job executor | WorkflowExecution | Consume `Status.Resources` in `buildJob()`; add `PodFailurePolicy` unconditionally; `buildStatusSummary()` unconditionally captures `RetryCount` (Wiring Point C) |
| WFE CRD (status summary) | WorkflowExecution | Add `ExecutionStatusSummary.RetryCount`; `make generate manifests` (Wiring Point C) |
| Audit payload schema | DataStorage (OpenAPI spec, WFE-owned schema) | Add optional `retry_count` to `WorkflowExecutionAuditPayload`; `make generate-datastorage-client` (Wiring Point C) |
| Audit payload builder | WorkflowExecution | `buildWorkflowExecutionAuditPayload()` sets `RetryCount` from `wfe.Status.ExecutionStatus.RetryCount` (Wiring Point C) |
| Workflow catalog docs | DataStorage / Workflow Authors | Document `execution.resources` in schema reference |
| `kubernaut-docs` (separate repo) | Docs | Operator-facing `LimitRange`/`ResourceQuota` guidance for `kubernaut-workflows` (tracked separately, see below) |
| `kubernaut-operator` (separate repo) | Operator | Awareness: no default `LimitRange`/`ResourceQuota` shipped (tracked separately, see below) |
| E2E test fixtures | WorkflowExecution | New `job-oomkill` test image + fixture; `test/fixtures/job/build-and-push.sh` updated (manual push, one-time) |

**Explicitly NOT touched in this decision**: DB schema/migrations, DataStorage repository/CRUD
layer, Tekton executor, Ansible executor. **OpenAPI spec + ogen-client ARE touched** (revised
from v1.2/v1.3's "not touched" claim), but narrowly: one optional field (`retry_count`) added to
`WorkflowExecutionAuditPayload`, a schema exclusively owned by WorkflowExecution's own event
types (Section 9) — not the broad, cross-cutting audit-schema project v1.2 originally assumed.

**Also not touched (separate, pre-existing gap, not caused by this decision)**: activating
`go-playground/validator` for `WorkflowSchema` validation. Reviewing this decision's own
`ResourcesSchema.validate` tags surfaced that ADR-046 (approved 2025-11-28) diagnosed the
`validate:"..."` tags across `WorkflowSchema` and its siblings as "decorative only" and
approved wiring up `validator.Struct()` to fix it — but that implementation (`pkg/validation/`,
Phase 1 = "Implement with ADR-043 (WorkflowSchema)") was never done; `pkg/validation/` does not
exist and `pkg/datastorage/schema/parser.go` never calls `validator.Struct()`. This predates
DD-WE-008 and affects every field in this file, not just `resources` — tracked separately as
[issue #1591](https://github.com/jordigilh/kubernaut/issues/1591), not fixed here.

---

## Consequences

### Positive

1. Workflow authors can size CPU/memory per workflow, closing the `BestEffort`/OOM-first-kill
   gap identified in #1564.
2. Transient infrastructure failures (OOM-kill, node eviction) no longer permanently fail a
   remediation attempt, without weakening fail-fast behavior for genuine script/logic failures.
3. Minimal DB/OpenAPI/ogen blast radius — no DB migration; the one OpenAPI/ogen touch (Wiring
   Point C, Section 9) is a single additive optional field on a schema WFE already owns
   exclusively.
4. Fully backward compatible: absent `resources` is behaviorally identical to today.
5. `ActiveDeadlineSeconds` (already in place) prevents the new `Ignore` retry tolerance from
   becoming an unbounded retry loop.
6. Closes the audit-completeness *count* gap (Section 9) that this decision's own retry
   tolerance would otherwise introduce — the durable audit trail now distinguishes a
   clean-first-attempt success from one that required tolerating N transient retries.

### Negative

1. `onExitCodes: [137]` cannot distinguish OOM-kill from an arbitrary externally-sent SIGKILL
   — accepted API-level limitation, documented above.
2. Re-parsing full YAML `content` on every WFE-Pending reconcile to extract `resources` (same
   cost already paid for `dependencies`/`engineConfig` — not a new I/O pattern, but also not
   eliminated by this decision).
3. No fleet-wide default: an operator who never declares `execution.resources` on any workflow
   and never sets a `LimitRange` still gets `BestEffort` pods. Mitigation: documented
   operationally (see below), not solved in code.
4. Root-cause attribution per retry attempt (OOM-kill vs. disruption specifically) remains
   best-effort only (Section 9 scope boundary) — the retry *count* is now hard-guaranteed, but
   *why* each individual attempt failed is not part of this decision's guarantee.

### Neutral

1. Tekton and Ansible engines are entirely unaffected by this decision.
2. Existing workflows without `execution.resources` require no migration action.

---

## Related Documents

- [DD-WE-005: Workflow-Scoped RBAC](./DD-WE-005-workflow-scoped-rbac.md) — precedent for per-workflow, status-resolved configuration
- [DD-WE-006: Schema-Declared Infrastructure Dependencies](./DD-WE-006-schema-declared-dependencies.md) — precedent for on-demand `content` extraction (no dedicated DB column)
- [BR-WE-014: Kubernetes Job Execution Backend](../../requirements/BR-WE-014-kubernetes-job-execution-backend.md)
- [BR-WE-016: Engine Config Discriminator](../../requirements/BR-WE-016-engine-config-discriminator.md) — precedent for on-demand `content` extraction (contrast: `EngineConfig` needs `interface{}` + JSON round-trip because its shape genuinely varies per engine; `resources` has a fixed shape and uses a concrete struct instead, see Finding 1)
- [ADR-046: Struct Validation Standard](../ADR-046-struct-validation-standard.md) — `ResourcesSchema`'s `validate` tags follow this standard's convention but are not currently enforced (Phase 1 gap predates this decision, tracked separately as [issue #1591](https://github.com/jordigilh/kubernaut/issues/1591))
- [BR-WE-018: Execution Pod Security Hardening](../../requirements/BR-WE-018-execution-pod-security-hardening.md) — contrast: non-configurable hardening vs. this decision's configurable resources
- [BR-WORKFLOW-008: Runtime Dependency-Failure Observability](../../requirements/BR-WORKFLOW-008-runtime-dependency-failure-observability.md) — `ActiveDeadlineSeconds` safety net this decision relies on
- [BR-WE-019: Job Resource Governance and Transient-Failure Tolerance](../../requirements/BR-WE-019-job-resource-governance-transient-failure-tolerance.md) — this decision's business requirement
- [Issue #1564](https://github.com/jordigilh/kubernaut/issues/1564) — originating issue
- [Issue #1572](https://github.com/jordigilh/kubernaut/issues/1572) — implementation tracking
- [kubernaut-docs#197](https://github.com/jordigilh/kubernaut-docs/issues/197) — operator-facing `LimitRange`/`ResourceQuota` guidance
- [kubernaut-operator#210](https://github.com/jordigilh/kubernaut-operator/issues/210) — cross-repo awareness on the no-default decision
- Follow-up issue: Tekton PipelineRun-timing loose audit assertions (`audit_flow_integration_test.go`) — needs a design decision (force determinism vs. leave loose), not bundled into this PR; **to be opened**
- Follow-up issue: `RetryOnConflict`-wraps-audit-call ordering gap in `AtomicStatusUpdate` (risk of duplicate audit writes on a retried status-update conflict) — pre-existing, WFE-specific architectural gap discovered during Section 9's audit-count preflight; not fixed here, needs its own design review; **to be opened**

---

## Document Maintenance

| Date | Version | Changes |
|---|---|---|
| 2026-07-06 | 1.0 | Initial decision |
| 2026-07-06 | 1.1 | Added Section 8 (E2E retry-tolerance proof on a real cluster, closing a Pyramid Invariant gap) and Section 9 (documented, deferred audit-completeness gap for tolerated retries — SOC2 CC8.1/AU-3) |
| 2026-07-06 | 1.2 | Replaced `Resources interface{}` + JSON round-trip with a concrete `ResourcesSchema` struct (`map[string]string` fields, parsed via `resource.ParseQuantity`) — avoids the Go Anti-Pattern Checklist's `interface{}` flag and is simpler than the original design, not just cosmetically different. Surfaced and documented (not fixed) a separate, pre-existing ADR-046 gap: `WorkflowSchema` validation tags are never actually enforced anywhere in `pkg/datastorage` |
| 2026-07-06 | 1.3 | Clarified Section 9: distinguished the deferred audit *content* gap (retry count not in payload, still deferred) from the audit *count* invariant (no duplicate/missing completion events from tolerated retries), which is now covered by a bundled regression guard in #1572 itself (`IT-WE-019-003`, extended `E2E-WE-019-001`, cites BR-AUDIT-005) — see IMPLEMENTATION_PLAN.md Phase 4.5 |
| 2026-07-06 | 1.4 | **Reversed the v1.2/v1.3 deferral of Section 9's audit-*content* fix** after user challenge exposed that its stated justification ("cross-cutting audit infrastructure shared by every audit-emitting service") did not hold up: `WorkflowExecutionAuditPayload` is WFE-exclusive, ogen regen is a single command, and the CRD/reconciler plumbing already exists via `ExecutionStatusSummary`. Promoted to **Wiring Point C**, now in scope for this PR: `ExecutionStatusSummary.RetryCount`, unconditional capture in `buildStatusSummary()`, optional `retry_count` OpenAPI field, and payload-builder wiring. Updated Affected Components, Consequences, and Explicitly-NOT-touched accordingly |
| 2026-07-07 | 1.5 | **Corrected a design flaw discovered while running `E2E-WE-019-001` against a real cluster**: `job.Status.Failed` is never incremented for `PodFailurePolicyActionIgnore` failures (confirmed via `k8s.io/api` doc comments and an empirical bare-Kind-cluster spike), so it could not serve as AC10's `RetryCount` source — it would have always read `0` for exactly the tolerated-retry case AC10 exists to capture. Evaluated and rejected a Watch-based alternative (architecturally impossible for Fleet-federated remote clusters, whose `ExecutorClient` has no `Watch` verb) before adopting the corrected mechanism: count `SuccessfulCreate` Events on the Job object (`countPodCreationAttempts()`), which works uniformly for local and Fleet execution and was independently spike-validated on a real cluster. Updated Section 8's E2E assertion and Section 9's `RetryCount` mechanism description accordingly; `job.go`, its unit tests, `IT-WE-019-004`'s helper, and `E2E-WE-019-001`'s assertion were all updated to match |

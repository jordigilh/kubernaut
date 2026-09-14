# Implementation Plan: Failed Workflow Execution Resource Retention

> Implementation plan for issue #2392.

**Plan Identifier**: IP-2392-v1
**Feature**: Retain failed workflow execution resources for bounded post-failure diagnosis across Job, Tekton, and Ansible/AWX engines.
**Created**: 2026-09-12
**Author**: Kubernaut maintainers
**Status**: Implementation complete for the current code scope; CI integration/E2E validation pending
**Branch**: `feat/2392-failed-execution-retention`
**Issue**: [#2392](https://github.com/jordigilh/kubernaut/issues/2392)

## 1. Business Requirements and Controls

This change is backed by:

- `BR-WE-014`: execution resources are created, observed, and cleaned up through the WorkflowExecution execution backend.
- `BR-WE-015`: Ansible/AWX execution is a supported WorkflowExecution backend and must preserve its backend-specific lifecycle guarantees.
- `BR-WE-019`: Job execution resource failures and retry information remain observable.
- `BR-WORKFLOW-008`: execution failures must be actionable and observable without leaking dependency values.
- `BR-AUDIT-005`: execution lifecycle evidence must remain reconstructable from the WorkflowExecution and execution-resource metadata.

Applicable control objectives:

- **FedRAMP AU-11**: failed execution evidence is retained for a bounded, configurable period.
- **FedRAMP AU-9**: retained resources remain protected by existing namespace, ownership, and gateway boundaries.
- **FedRAMP AC-6**: retries may replace only a terminal retained resource owned by the prior WorkflowExecution; active resources and unrelated owners are never deleted.
- **FedRAMP AU-3**: retained resources carry sufficient non-secret execution identity metadata for reconstruction.
- **OWASP ASVS V4.1.1/V4.1.3**: resource ownership and least-privilege authorization remain enforced during retention and replacement.
- **OWASP ASVS V5.1.x**: retention configuration is parsed and validated, including positive bounded durations.
- **OWASP ASVS V7.1.1/V7.2.1**: retention, replacement, and cleanup decisions are observable without logging credentials or workflow output.

## 2. Outcome

When enabled, a failed WorkflowExecution remains diagnosable for the configured retention period. The retained resource is removed automatically after that period, or earlier when a new execution for the same target safely replaces the old terminal resource. Successful executions retain the current cleanup behavior.

The public configuration is generic rather than engine-specific:

```yaml
execution:
  retainFailedExecutions: false
  failedExecutionRetentionSeconds: 600
```

The existing defaults remain unchanged when the feature is disabled.

## 3. Preflight Findings

The relevant production paths are:

- `pkg/workflowexecution/config/config.go`: `ExecutionConfig` is the existing YAML configuration seam.
- `cmd/workflowexecution/main.go`: loads config, constructs the executor registry, and passes `ReconcilerOptions`.
- `internal/controller/workflowexecution/workflowexecution_lifecycle.go`: owns cooldown cleanup and deletion finalization.
- `pkg/workflowexecution/executor/executor.go`: shared `Executor` strategy interface.
- `pkg/workflowexecution/executor/job.go`: creates Jobs with a hard-coded `TTLSecondsAfterFinished: 600` and deletes owned Jobs during cleanup.
- `pkg/workflowexecution/executor/tekton.go`: creates and deletes owned PipelineRuns; no backend TTL is configured here.
- `pkg/workflowexecution/executor/ansible.go`: cancels AWX jobs and deletes ephemeral credentials during cleanup.
- `internal/controller/workflowexecution/workflowexecution_collision.go`: Job-only terminal collision replacement exists; Tekton currently treats an existing PipelineRun as a collision.
- `charts/kubernaut/templates/workflowexecution/workflowexecution.yaml` and `charts/kubernaut/values.yaml`: Helm configuration wiring and defaults.
- Existing `DD-WE-004` behavior treats failures during workflow execution as non-retryable (`wasExecutionFailure=true`) and routes them to manual intervention. Retention must not create an automatic retry path.

Baseline validation on this branch passed:

```text
go test ./pkg/workflowexecution/...
go test ./internal/controller/workflowexecution/...
go test ./cmd/workflowexecution/...
```

## 4. Spikes and Decisions

### Spike 1: Retention versus deterministic execution locks

**Question**: Can a failed resource remain available for diagnosis without permanently blocking a later execution for the same target?

**Finding**: Yes, but retention crosses two Kubernetes lifecycles. A WorkflowExecution is controller-owned by its RemediationRequest, so an RR terminal/deletion cascade can put the WFE into deletion while the execution resource is still being retained. The WorkflowExecution finalizer must remain until retention expires or replacement completes. A new execution may replace an expired or terminal retained resource, but never an active or differently-owned resource.

**Decision**: Use option 1. Retain the original terminal resource until a separately authorized/new execution attempt or configured expiry, whichever comes first. Reuse the deterministic name and perform an ownership-checked terminal replacement before creating the new resource. Retention must not bypass `DD-WE-004`'s no-automatic-retry rule.

### Spike 2: Kubernetes Job TTL interaction

**Question**: Is changing executor cleanup sufficient for Job retention?

**Finding**: No. `Job.Spec.TTLSecondsAfterFinished` currently prunes the Job and its pods independently of WorkflowExecution finalization. The TTL countdown begins from Job completion, while the WFE retention timer begins when Kubernaut observes and records failure, so equal values can prune diagnostics early. The current Job builder always sets a hard-coded 600-second TTL at creation, before the controller knows whether the execution will fail.

**Decision**: When failed-execution retention is enabled, omit `TTLSecondsAfterFinished` for Jobs and make the WorkflowExecution controller the cleanup authority for both terminal outcomes. This avoids early diagnostic deletion caused by two clocks starting at different times. Successful Jobs remain subject to existing cooldown cleanup; failed Jobs are removed at the retention deadline. The default-off path preserves the existing 600-second Job TTL.

### Spike 3: Tekton and Ansible semantics

**Question**: Can the same policy cover non-Job engines without pretending they are Kubernetes Jobs?

**Finding**: Yes, but the policy must be expressed in terms of the execution resource. Tekton retention is controlled by delaying PipelineRun cleanup, subject to any cluster-level Tekton pruner. AWX retention is controlled by not cancelling the failed AWX job, subject to AWX/AAP retention policy. AWX ephemeral credential deletion is independent and must continue immediately.

**Decision**: Keep the policy and timer in the shared controller; retain/delete behavior remains engine-specific inside each executor.

### Spike 4: Existing collision handling

**Question**: Can existing collision paths safely replace a retained failed resource?

**Finding**: Job has a terminal collision path, but it currently constructs a cleanup WFE without the existing owner identity, which is incompatible with the Job cleanup ownership check. Tekton has no equivalent terminal replacement path. Ansible launch IDs are external AWX identifiers and do not use the same deterministic Kubernetes collision path. This is a required implementation gap, not a reason to add a second resource naming scheme.

**Decision**: Introduce a shared terminal-resource replacement capability or an equivalent executor operation that receives the verified existing owner identity. Extend Job and Tekton symmetrically. Ansible launch IDs are external AWX identifiers and do not use the same deterministic Kubernetes resource collision path.

### Spike 5: AWX credential lifetime

**Question**: Can failed AWX jobs be retained without retaining credentials created for them?

**Finding**: Not with the current `AnsibleExecutor.Cleanup` boundary. `Cleanup` deletes ephemeral credentials and then cancels the AWX job. Delaying the whole method would keep credentials alive for the retention interval; skipping it would leak them.

**Decision**: Split credential cleanup from execution-resource cancellation. Credential cleanup must run as soon as a terminal AWX failure is observed, while cancellation remains governed by the shared retention policy. User-requested WFE deletion must still cancel active AWX jobs.

### Spike 6: Executor API shape

**Question**: Can the controller trigger AWX-only credential cleanup without weakening the shared executor contract?

**Finding**: The existing `Executor` interface is shared by Job, Tekton, and Ansible, and its `Cleanup` method means execution-resource deletion/cancellation. Making `Cleanup` retention-aware would force every backend to understand AWX credential semantics and would delay credential removal.

**Decision**: Add a narrow optional capability for ephemeral-resource cleanup. The controller invokes it after a terminal AWX failure when implemented; Job and Tekton do not implement it. The existing `Cleanup` method remains the deletion/finalizer operation and keeps active-job cancellation behavior.

### Spike 7: Terminal replacement authorization

**Question**: Can deterministic-name replacement be made safe without creating an automatic retry path?

**Finding**: Job replacement currently checks only whether the deterministic Job is terminal, then constructs a cleanup WFE without the original label owner (`workflowexecution_collision.go:84-89`). Cleanup refuses deletion because the synthetic WFE has an empty name. Tekton collision handling currently classifies a differently-owned PipelineRun as deduplicated and has no terminal replacement path. Gateway terminal phases allow a new RR, but `DD-WE-004` still requires execution failures to remain non-automatic/manual unless a new execution is explicitly authorized.

**Decision**: Before replacement, read and validate the existing resource owner label, fetch the owner WFE, require the owner WFE to be terminal, and delete only that exact owner resource. Pass the verified owner identity to cleanup or delete directly. Extend this symmetrically to Job and Tekton. The path is reached only from creation of a new authorized WFE and does not alter retry/gating decisions.

## 5. Scope

### In scope

- Add validated generic retention fields to `ExecutionConfig`.
- Wire configuration through Helm, startup logging, executor creation, and `ReconcilerOptions`.
- Delay failed-resource cleanup and WFE finalizer removal for the configured bounded period.
- Retain failed Job/Pod diagnostics, Tekton PipelineRun/TaskRun diagnostics, and AWX job output where supported.
- Preserve prompt, independent AWX ephemeral credential cleanup.
- Add ownership-safe terminal replacement for retrying a target with a retained failed resource.
- Add unit, integration, and engine-journey tests tied to the controls above.
- Add operator-facing configuration documentation.

### Out of scope

- Retaining successful execution resources.
- Retaining arbitrary workflow logs outside the backend resource lifecycle.
- Changing execution-resource names or weakening deterministic locking.
- Adding new RBAC permissions beyond the existing Job/PipelineRun access.
- Changing AWX job retention policy outside Kubernaut’s cancellation behavior.
- Guaranteeing retention against an independently configured Tekton pruner or AWX/AAP cleanup policy.

## 6. Risks and Mitigations

| ID | Risk | Mitigation | Tests |
|---|---|---|---|
| R1 | Retained resource blocks a valid retry | Replace only verified terminal resources owned by the prior WFE; keep active resources locked | `UT-WE-2392-RET-003`, `IT-WE-2392-004`, `E2E-WE-2392-001` |
| R2 | Kubernetes TTL deletes failed Job diagnostics early | Disable the Job TTL only when retention is enabled and use controller-owned expiry | `UT-WE-2392-JOB-001`, `IT-WE-2392-JOB-002`, `IT-WE-2392-002` |
| R3 | Unrelated resource is deleted during replacement | Require matching execution-owner label and terminal status | `UT-WE-2392-RET-004`, `IT-WE-2392-005` |
| R4 | AWX credentials remain after retaining the job | Keep credential cleanup independent from job cancellation | `UT-WE-2392-AWX-002`, `IT-WE-2392-006` |
| R5 | Invalid or unbounded retention configuration causes resource leaks | Validate positive duration and preserve secure default-off behavior | `UT-WE-2392-CFG-001`, `UT-WE-2392-CFG-002` |
| R6 | Retention behavior differs by engine | Test the shared lifecycle and each executor’s resource-specific operation | `IT-WE-2392-001`, `IT-WE-2392-002`, `IT-WE-2392-003`, `IT-WE-2392-006` |
| R7 | Diagnostic metadata leaks secrets | Assert labels, events, and logs contain identity/reason only, not Secret values or workflow output | `UT-WE-2392-OBS-001`, `IT-WE-2392-007` |
| R8 | RR deletion cascades into WFE deletion before retention expires | Keep the WFE finalizer until expiry/replacement and test deletion while the parent is terminating | `UT-WE-2392-RET-005`, `IT-WE-2392-008` |
| R9 | External Tekton/AWX pruning defeats retention | Document the guarantee boundary and treat missing external resources as safe terminal state | `UT-WE-2392-EXT-001`, `IT-WE-2392-009` |
| R10 | Retention delays AWX credential cleanup | Invoke the optional ephemeral-resource cleanup capability at terminal failure and assert credentials are absent while the AWX job remains | `UT-WE-2392-AWX-003`, `IT-WE-2392-010` |
| R11 | Retention accidentally enables automatic retry after execution failure | Preserve `wasExecutionFailure` / manual-intervention semantics; only an explicitly authorized new WFE may replace a retained resource | `UT-WE-2392-RETRY-001`, `IT-WE-2392-012`, `E2E-WE-2392-004` |

## 7. Wiring Manifest

| Component | Production entry point | Wiring location | IT proof |
|---|---|---|---|
| Retention configuration | WorkflowExecution startup | `cmd/workflowexecution/main.go`, `pkg/workflowexecution/config/config.go` | `IT-WE-2392-001` |
| Shared retention lifecycle | Terminal and deletion reconciliation | `internal/controller/workflowexecution/workflowexecution_lifecycle.go` | `IT-WE-2392-002` |
| Job retention and replacement | Job executor dispatch | `pkg/workflowexecution/executor/job.go`, `internal/controller/workflowexecution/workflowexecution_collision.go` | `IT-WE-2392-004` |
| Tekton retention and replacement | Tekton executor dispatch | `pkg/workflowexecution/executor/tekton.go`, collision handling | `IT-WE-2392-005` |
| Ansible retention and credential cleanup | Ansible executor dispatch | `pkg/workflowexecution/executor/ansible.go` | `IT-WE-2392-006` |
| Helm configuration | WorkflowExecution ConfigMap rendering | `charts/kubernaut/values.yaml`, `templates/workflowexecution/workflowexecution.yaml` | `IT-WE-2392-003` |

## 8. TDD Sequence

### RED

Write failing Ginkgo/Gomega tests before implementation:

1. Configuration defaults to retention disabled and parses a positive bounded duration.
2. Invalid zero/negative retention values fail validation when retention is enabled.
3. Job creation omits the TTL only when retention is enabled and preserves the existing 600-second TTL when disabled.
4. Shared terminal reconciliation retains failed resources during the window and cleans them at expiry.
5. Deletion finalization does not remove a failed retained resource before expiry.
6. Successful resources are cleaned at the existing cooldown boundary.
7. Job replacement removes only a verified terminal resource owned by the prior WFE and only for a separately authorized/new execution.
8. Tekton replacement has the same terminal ownership guarantees.
9. Active and differently-owned resources remain untouched.
10. Ansible retention skips AWX cancellation while deleting ephemeral credentials immediately after failure through the optional capability.
11. Retention and replacement logs/events contain no credential or workflow-output values.
12. Helm renders the generic settings into the WorkflowExecution ConfigMap and production startup consumes them.
13. Execution failures remain non-retryable unless a separate authorized execution is created.

Every test must identify its BR/control objective. No standard `testing.T`, `Skip`, `XIt`, or `PIt` tests are permitted.

### GREEN

1. Add `retainFailedExecutions` and `failedExecutionRetentionSeconds` to `ExecutionConfig` with secure defaults and validation.
2. Pass a shared retention policy into the reconciler and required executor creation path.
3. Make terminal and deletion reconciliation defer failed-resource cleanup until expiry.
4. Make Job TTL conditional: preserve the default-off TTL and use controller-owned expiry when retention is enabled.
5. Add ownership-safe terminal replacement for Job and Tekton resources.
6. Add the optional ephemeral-resource cleanup capability and split AWX credential cleanup from conditionally suppressed job cancellation.
7. Wire Helm values, `values.schema.json`, generated documentation, and startup logging.

Run CHECKPOINT W immediately after GREEN. Every wiring-manifest row must have a production caller and a passing integration test.

### REFACTOR

- Centralize terminal-retention eligibility and expiry calculations.
- Remove duplicated owner/terminal checks across Job and Tekton replacement paths.
- Make retention logs and events structured and secret-safe.
- Preserve the existing executor interface unless a narrowly scoped replacement capability is required.

## 9. Test Pyramid and Control Matrix

The pyramid invariant is mandatory: unit tests prove retention and ownership logic, integration tests prove production wiring and engine dispatch, and E2E tests prove the operator-visible remediation journey.

| Control / outcome | Unit | Integration | E2E |
|---|---|---|---|
| AU-11 bounded failed-resource retention | `UT-WE-2392-CFG-001`, `UT-WE-2392-RET-001` | `IT-WE-2392-002` | `E2E-WE-2392-001` |
| AU-9 protected retained evidence | `UT-WE-2392-RET-004`, `UT-WE-2392-OBS-001` | `IT-WE-2392-007` | `E2E-WE-2392-001`, `E2E-WE-2392-002` |
| AC-6 ownership and least privilege | `UT-WE-2392-RET-003` | `IT-WE-2392-004`, `IT-WE-2392-005` | `E2E-WE-2392-001` |
| AU-3 reconstructable execution identity | `UT-WE-2392-OBS-001` | `IT-WE-2392-002` | `E2E-WE-2392-001` |
| ASVS V4.1.1/V4.1.3 ownership enforcement | `UT-WE-2392-RET-004` | `IT-WE-2392-004`, `IT-WE-2392-005` | `E2E-WE-2392-002` |
| ASVS V5.1.x configuration validation | `UT-WE-2392-CFG-001`, `UT-WE-2392-CFG-002` | `IT-WE-2392-001`, `IT-WE-2392-003` | N/A; configuration wiring is proven at IT tier |
| ASVS V7.1.1/V7.2.1 safe observability | `UT-WE-2392-OBS-001` | `IT-WE-2392-007` | `E2E-WE-2392-001` |
| Ansible credentials remain ephemeral | `UT-WE-2392-AWX-002` | `IT-WE-2392-006` | `E2E-WE-2392-003` |

## 10. Success Criteria

- `go build ./...` passes.
- `golangci-lint run --timeout=5m` introduces no new findings.
- All affected Ginkgo unit and integration tests pass.
- Job, Tekton, and Ansible failed-resource behavior is proven through production dispatch paths.
- Successful cleanup and existing ownership protections regressions are absent.
- The configured retention duration is bounded and default-off.
- A separately authorized new execution can replace only the prior terminal retained resource and never an active or unrelated resource.
- FedRAMP and OWASP ASVS control objectives in Section 9 have passing tests at the listed tiers.

## 11. Reassessment Findings

The second preflight pass changed the risk assessment:

- Retention must cover both terminal reconciliation and deletion reconciliation because WFE deletion is normally cascaded from the RR owner.
- The current Job terminal replacement helper has an owner-identity handoff defect that must be fixed before option 1 can work safely.
- Existing `DD-WE-004` semantics prohibit automatic retry after an execution failure; retention must not weaken that safety boundary.
- Tekton and AWX retention are best-effort against external pruners and retention policies; the acceptance criteria must not promise stronger guarantees.
- AWX credential cleanup cannot remain coupled to delayed execution-resource cleanup.
- Job TTL cannot simply equal the WFE retention duration because the two clocks start at different observations.
- Helm wiring requires updates to the JSON schema and generated documentation in addition to the ConfigMap template.

Spike resolutions:

- **Job TTL**: conditional behavior. Retention-enabled Jobs have no Kubernetes TTL and are expired by the WFE controller; the default-off path preserves the existing 600-second TTL.
- **AWX credentials**: an optional one-method ephemeral-resource cleanup capability leaves the shared `Executor.Cleanup` contract unchanged.
- **Replacement**: owner-WFE validation plus terminal-state verification will be used for both Job and Tekton. The existing Job owner handoff defect is localized and testable.
- **Retry semantics**: replacement is limited to a separately authorized/new WFE and does not change no-automatic-retry behavior for execution failures.

These findings are sufficiently resolved to proceed to implementation. The RED tests encode the configuration, Job TTL, lifecycle, AWX credential, and terminal PipelineRun boundaries. They remain implementation risks to be validated by the integration tests listed above.

## 12. Confidence

**Preflight confidence: 91%.**

Justification: focused spikes resolved the main design uncertainties and the RED/GREEN implementation now covers configuration, Job TTL behavior, controller retention/finalization timing, AWX credential separation, and owner-checked terminal PipelineRun replacement. Remaining risk is integration-level: controller timer/finalizer interactions, remote-client behavior, and Tekton API replacement semantics require dedicated integration coverage. Confidence remains above the 90% implementation gate.

## 13. Current Implementation Checkpoint

- Configuration defaults and validation are implemented and wired through startup logging, Helm, schema, and generated values documentation.
- Retention-enabled Job resources omit Kubernetes TTL; default-off Jobs preserve the existing 600-second TTL.
- Failed WFE terminal reconciliation and deletion finalization defer execution-resource cleanup until retention expiry.
- AWX ephemeral credentials can be cleaned independently while a failed AWX job remains retained.
- Job collision cleanup now requires an identified owner; Tekton replacement requires an owner WFE in terminal failure state.
- Targeted Go tests, formatting, diff checks, and changed-package lint pass.
- Local `make test-integration-workflowexecution` reached EnvTest CRD bootstrap and controller startup but exceeded the local timeout during suite startup/cleanup; no feature assertion ran. Full integration and end-to-end validation are delegated to CI.

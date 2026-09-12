# Verification Report: Interactive Workflow Snapshot Metadata Parity

> Implementation and verification record for issue #2390.

**Plan Identifier**: IP-2390-v1
**Feature**: Preserve catalog-authoritative workflow execution metadata through interactive KA selection.
**Created**: 2026-09-11
**Author**: Kubernaut maintainers
**Status**: Complete; E2E execution deferred to CI/CD
**Branch**: `fix/issue-2390-workflow-snapshot`
**Issue**: [#2390](https://github.com/jordigilh/kubernaut/issues/2390)

**Business requirements**:

- `BR-WE-016`: engine-specific configuration reaches the executor.
- `BR-FLEET-004`: workflow-declared execution cluster is authoritative and routed through the existing fleet trust boundary.
- `BR-WE-014`: the Job backend creates the correct execution resource.
- `BR-WE-015`: the Ansible backend receives and executes the selected workflow configuration.
- `BR-WE-019`: declared Job resources reach the workflow container.
- `BR-WORKFLOW-008`: dependency failures are observable and fail fast.
- `BR-KA-191`: KA remains the authoritative workflow-parameter validation layer.
- `BR-AUDIT-005` and `BR-AUDIT-021-030`: workflow selection and execution metadata remain reconstructable.

**Authority documents**:

- `DD-WORKFLOW-018`: CRD-embedded workflow execution snapshot.
- `DD-FLEET-008`: workflow-declared execution cluster.
- `DD-WE-006`: schema-declared infrastructure dependencies.
- `DD-KA-001`: workflow response validation and catalog authority.
- `BR-WE-016`: engine configuration discriminator.

## 1. Business Outcome

An interactive workflow selection must produce the same catalog-authoritative execution snapshot as autonomous selection. A workflow selected through the live GitOps-drift path must retain its dependency Secret, execution cluster, service account, resource policy, declared parameter allowlist, bundle metadata, and engine configuration through the following chain:

```text
RemediationWorkflow
  -> KA catalog/cache
  -> interactive CatalogWorkflow
  -> InvestigationResult
  -> selected_workflow wire response
  -> AIAnalysis.status.rcaResult.selectedWorkflow
  -> WorkflowExecution.spec.workflowRef
  -> Job/Tekton/Ansible execution
```

The user-visible outcome is that `git-revert-v2` can authenticate using its declared Secret and execute on its declared cluster instead of failing with a generic Job backoff error.

## 2. Preflight Evidence

The live RC12 cluster was inspected using `~/.kube/kubernaut-demo-config`.

Affected objects:

- RR: `rr-43437acb3f13-a4b4d9ec`
- AIAnalysis: `ai-rr-43437acb3f13-a4b4d9ec`
- WorkflowExecution: `we-rr-43437acb3f13-a4b4d9ec`
- Workflow: `git-revert-v2`

Live catalog metadata included:

- `dependencies.secrets: gitea-repo-creds`
- `dependencies.configMaps: gitea-repo-config`
- `execution.clusterId: hub`
- `execution.engine: job`
- `execution.serviceAccountName: git-revert-v2-runner`

Live AA and WFE snapshots omitted:

- `dependencies`
- `resources`
- `declaredParameterNames`
- `executionClusterId`
- `engineConfig`

The AIAnalysis had an interactive session and the selected-workflow rationale was `User-selected via interactive mode`. All Kubernaut service pods were running RC12 images. The resulting Job failed with `BackoffLimitExceeded`; a manually recreated Job with the Secret mounted succeeded.

Current source inspection shows:

- Autonomous `buildSelectedWorkflowMap()` already emits several affected fields.
- AA and RO already consume most of the shared snapshot fields.
- The interactive `CatalogWorkflow` and `applySelectedWorkflow()` do not carry or apply the complete catalog/schema metadata.
- Dependencies, resources, and declared parameter names are schema-derived and must use the same extraction path as autonomous selection.

The preflight confidence for the root cause is 98%.

## 3. Scope

### In scope

- Interactive catalog metadata parity with autonomous selection.
- Shared extraction of dependencies, resources, declared parameter names, and engine configuration.
- Catalog-authoritative execution cluster and service account propagation.
- AA status mapping parity across successful, partial, and low-confidence paths.
- Integration proof through actual production mapping boundaries.
- E2E proof that the generated Job mounts the declared Secret and executes successfully.
- Sanitized diagnostics when expected execution metadata is missing.

### Out of scope

- New execution engines.
- Dynamic or LLM-selected execution clusters.
- Reimplementation of downstream Job, Tekton, or Ansible behavior already covered by existing tests.
- Raw workflow-log retention. Any such retention requires a separate security and retention decision because workflow logs may contain credentials.
- Replacing the existing fleet gateway authorization boundary.

## 4. Risks and Mitigations

### R1: Interactive and autonomous paths diverge again

Impact: one workflow mode silently loses execution metadata.

Mitigation: one shared catalog-to-snapshot metadata mapper and parity tests for both paths.

Tests: `UT-KA-2390-001`, `UT-KA-2390-002`, `IT-KA-2390-001`, `E2E-FP-2390-001`.

### R2: Schema-derived metadata is extracted differently in interactive mode

Impact: dependencies, resources, or parameter allowlists are incomplete or inconsistent.

Mitigation: reuse the parser/schema extraction implementation used by `buildWorkflowMeta`; do not duplicate YAML interpretation in the MCP adapter.

Tests: `UT-KA-2390-003`, `IT-KA-2390-002`.

### R3: LLM output overrides catalog-authoritative execution fields

Impact: unauthorized dependency, service-account, or cluster selection.

Mitigation: apply catalog values after LLM selection and overwrite conflicting result values unconditionally.

Controls: FedRAMP `AC-4`, `AC-6`; OWASP ASVS `V4.1.1`, `V4.1.3`.

Tests: `UT-KA-2390-004`, `IT-KA-2390-003`, `E2E-FLEET-2390-001`.

### R4: Nil and empty parameter allowlists are conflated

Impact: undeclared parameters reach an execution container.

Mitigation: preserve the existing contract: `nil` means schema metadata unavailable; an empty map means the schema declares no parameters. Fail closed when schema parsing fails.

Controls: FedRAMP `SI-10`; OWASP ASVS V5 input-validation objectives.

Tests: `UT-KA-2390-005`, `UT-WE-2390-001`, `IT-WE-2390-001`.

### R5: Missing metadata diagnostics expose credentials

Impact: debugging output leaks Secret-derived values or workflow logs.

Mitigation: emit only field names, workflow ID, remediation correlation ID, and sanitized Kubernetes Events/termination messages.

Controls: FedRAMP `AU-3`, `AU-9`; OWASP ASVS V5 output/input handling objectives.

Tests: `UT-WE-2390-002`, `IT-WE-2390-002`.

### R6: Downstream behavior is changed unnecessarily

Impact: regressions in existing Job, Tekton, or Ansible execution.

Mitigation: make the smallest upstream change; preserve existing RO and WE implementations unless a regression test proves a downstream defect.

Tests: existing affected package suites and `E2E-WE-006-*`, `E2E-FLEET-2326-001` regressions.

### R7: Job-only E2E coverage misses engine-specific metadata

Impact: a shared snapshot field works for the Job backend but is dropped or misinterpreted
for Tekton or Ansible.

Mitigation: use one shared snapshot assertion helper at the AA and WFE boundaries, then
extend an existing live journey for each execution engine with engine-specific assertions.
Keep actual executor behavior in the existing engine-specific E2E suites.

Tests: `E2E-WE-2390-002`, `E2E-WE-2390-003`, `E2E-WE-2390-004`.

### R8: CRD and workflow-schema contracts drift

Impact: a schema-valid `workflow-schema.yaml` is rejected by Kubernetes before any service
transition is exercised, as occurred with `execution.resources`.

Mitigation: add an admission matrix test using metadata-rich Job, Tekton, and Ansible
fixtures against the generated RemediationWorkflow CRD.

Tests: `IT-AW-2390-001`.

## 5. Control and Business-Outcome Matrix

The pyramid invariant is mandatory: unit tests prove logic, integration tests prove wiring, and E2E proves the remediation journey and control objectives.

- FedRAMP `AC-4`: the declared `hub` execution cluster is used only through the existing registered-cluster gateway path. Unit: `UT-RO-2390-001`. Integration: `IT-RO-2390-001`. E2E: `E2E-FLEET-2390-001`.
- FedRAMP `AC-6`: the LLM can select a workflow but cannot change its service account, dependencies, bundle, or execution cluster. Unit: `UT-KA-2390-004`. Integration: `IT-KA-2390-003`. E2E: `E2E-FLEET-2390-001`.
- FedRAMP `SI-10`: malformed schema or engine metadata does not produce an unsafe executable snapshot. Unit: `UT-KA-2390-003` and `UT-WE-2390-001`. Integration: `IT-KA-2390-002`.
- FedRAMP `SI-10`: every engine fixture is admitted by the RemediationWorkflow CRD before service-level E2E execution. Integration: `IT-AW-2390-001`.
- FedRAMP `AU-3`: workflow ID, version, action, engine, bundle, dependency, and routing metadata remain available for lifecycle reconstruction. Unit: `UT-AA-2390-001`. Integration: `IT-AA-2390-001`. E2E: `E2E-FP-2390-001`.
- OWASP ASVS `V4.1.1`: routing authorization remains enforced at the trusted fleet gateway layer. Integration: `IT-RO-2390-001`. E2E: `E2E-FLEET-2390-001`.
- OWASP ASVS `V4.1.3`: execution remains limited to operator-provisioned clusters and service accounts. Unit: `UT-KA-2390-004`. E2E: `E2E-FLEET-2390-001`.
- OWASP ASVS V5 input-validation objectives: schema-derived parameters and engine configuration are not accepted as uncontrolled LLM input. Unit: `UT-KA-2390-003`, `UT-KA-2390-005`. Integration: `IT-WE-2390-001`.
- `BR-WE-016`: Ansible engine configuration reaches `WorkflowRef` and the executor. Unit: `UT-KA-2390-006`, `UT-WE-2390-003`. Integration: `IT-RO-2390-002`.
- `BR-WE-019`: declared resources reach the Job container. Unit: `UT-WE-2390-001`. E2E: `E2E-FP-2390-001`.
- `BR-WE-015`: Ansible-specific configuration survives the common snapshot path and reaches the Ansible execution boundary. Unit: `UT-WE-2390-003`. E2E: `E2E-FP-2390-004`.
- `BR-WORKFLOW-008`: missing dependency failures are actionable and sanitized. Unit: `UT-WE-2390-002`. Integration: `IT-WE-2390-002`.

## 6. TDD Sequence

### RED

Write failing Ginkgo/Gomega tests before implementation:

1. Catalog adapter derives all schema metadata from a workflow fixture.
2. Interactive `applySelectedWorkflow()` produces a complete catalog-authoritative `InvestigationResult`.
3. Interactive and autonomous selected-workflow wire payloads contain equivalent snapshot fields.
4. Conflicting LLM fields are overwritten by catalog values.
5. AA successful, partial, and low-confidence mappings preserve the same snapshot contract.
6. RO preserves the snapshot and resolves the declared execution cluster.
7. WE Job receives dependencies, resources, service account, and filtered parameters.
8. Ansible receives valid engine configuration and rejects invalid/missing configuration.
9. Missing metadata diagnostics contain no credential values.
10. A CRD admission matrix accepts the complete metadata shape for Job, Tekton, and Ansible fixtures.
11. The common snapshot is identical at AA and WFE boundaries for all three engines.

All tests must assert business outcomes and reference the applicable BR or control objective in the test name or comment. No `testing.T`, `Skip`, `XIt`, or `PIt` is permitted.

### GREEN

Implement the minimum path:

1. Introduce or reuse a single catalog metadata conversion helper.
2. Populate interactive `CatalogWorkflow` from that metadata.
3. Extend `applySelectedWorkflow()` to copy the complete snapshot.
4. Add `EngineConfig` to the missing KA intermediate types and wire representation.
5. Make catalog-authoritative execution fields unconditional.
6. Use one AA mapper for all selected-workflow population paths.
7. Add sanitized missing-field diagnostics.

Run CHECKPOINT W immediately after GREEN. Every wiring item below must have a production caller and a passing integration proof.

### REFACTOR

After GREEN remains green:

- Remove duplicated snapshot field lists.
- Centralize nil/empty allowlist semantics.
- Keep schema parsing in one package/path.
- Make catalog authority explicit in names and comments.
- Wrap and log errors with lowercase contextual messages.

Post-refactor validation is mandatory:

```text
go build ./...
go test ./... -run=^$ -timeout=30s
golangci-lint run --timeout=5m
```

## 7. Wiring Manifest

- Catalog workflow -> shared execution metadata
  - Production entry point: `WorkflowCatalogAdapter.GetWorkflowByID()` and autonomous `buildWorkflowMeta()`.
  - Wiring location: `internal/kubernautagent/mcp/adapters/adapters.go`, `cmd/kubernautagent/toolregistry.go`.
  - IT proof: `IT-KA-2390-001`.

- Shared metadata -> interactive selection result
  - Production entry point: `applySelectedWorkflow()`.
  - Wiring location: `internal/kubernautagent/mcp/tools/select_workflow.go`.
  - IT proof: `IT-KA-2390-002`.

- KA result -> selected-workflow wire response
  - Production entry point: response mapping used by the interactive and autonomous session completion paths.
  - Wiring location: `internal/kubernautagent/agentsession/mapping.go` and interactive completion path.
  - IT proof: `IT-KA-2390-003`.

- KA response -> AA status snapshot
  - Production entry point: `ResponseProcessor.storeSelectedWorkflow()` and shared selected-workflow mapper.
  - Wiring location: `pkg/aianalysis/handlers/response_processor.go`.
  - IT proof: `IT-AA-2390-001`.

- AA status -> WFE snapshot and routing
  - Production entry point: `WorkflowExecutionCreator.Create()`.
  - Wiring location: `pkg/remediationorchestrator/creator/workflowexecution.go`.
  - IT proof: `IT-RO-2390-001` and `IT-RO-2390-002`.

- WFE snapshot -> execution resource
  - Production entry point: Job, Tekton, and Ansible executor dispatch.
  - Wiring location: `pkg/workflowexecution/executor/`.
  - IT proof: `IT-WE-2390-001` and `IT-WE-2390-003`.

CHECKPOINT W fails if a component is only tested through a direct helper call, if interactive production dispatch is not exercised, or if a new metadata type has no production caller.

## 8. Test Scenarios

### Unit tests

- `UT-KA-2390-001`: interactive catalog mapping includes dependency metadata from the registered workflow schema. `BR-WORKFLOW-008`, `AC-6`.
- `UT-KA-2390-002`: interactive catalog mapping includes resources and declared parameter names. `BR-WE-019`, `BR-KA-191`, `SI-10`.
- `UT-KA-2390-003`: malformed schema and engine metadata fail closed. `SI-10`, ASVS V5.
- `UT-KA-2390-004`: catalog execution cluster and service account overwrite conflicting LLM values. `AC-4`, `AC-6`, ASVS `V4.1.3`.
- `UT-KA-2390-005`: nil and empty declared-parameter allowlists retain distinct semantics. `BR-KA-191`, `SI-10`.
- `UT-KA-2390-006`: engine configuration is catalog-authoritative and reaches the KA result. `BR-WE-016`.
- `UT-AA-2390-001`: AA persists the complete snapshot from interactive wire data. `AU-3`.
- `UT-AA-2390-002`: partial and low-confidence paths do not silently drop snapshot metadata. `AU-3`.
- `UT-RO-2390-001`: workflow-declared cluster overrides the signal cluster only through existing routing resolution. `AC-4`, `AC-6`.
- `UT-WE-2390-001`: Job mounts dependencies, applies resources, uses the service account, and filters parameters. `BR-WE-014`, `BR-WE-019`, `AC-6`.
- `UT-WE-2390-002`: missing snapshot metadata produces sanitized diagnostics. `BR-WORKFLOW-008`, `AU-3`.
- `UT-WE-2390-003`: Ansible engine configuration is parsed and used. `BR-WE-016`.

### Integration tests

- `IT-KA-2390-001`: real catalog adapter maps a registered workflow schema into interactive selection metadata.
- `IT-KA-2390-002`: production interactive selection produces a complete `selected_workflow` result.
- `IT-KA-2390-003`: conflicting LLM selection fields cannot override catalog execution metadata.
- `IT-AA-2390-001`: AA controller persists all fields into `status.rcaResult.selectedWorkflow`.
- `IT-RO-2390-001`: RO creates WFE with dependencies, resources, declared parameters, service account, and declared cluster routing.
- `IT-RO-2390-002`: RO passes engine configuration into `WorkflowRef`.
- `IT-WE-2390-001`: WE creates a Job with the declared Secret/ConfigMap mounts, resource requirements, and filtered environment.
- `IT-WE-2390-004`: WE creates a read-only ConfigMap-backed Job volume and mount from the declared ConfigMap dependency.
- `IT-WE-2390-002`: missing dependency or metadata failure is observable without sensitive values.
- `IT-WE-2390-003`: Ansible execution receives the propagated engine configuration.
- `IT-AW-2390-001`: generated RemediationWorkflow CRD admission accepts the metadata-rich Job, Tekton, and Ansible fixtures, including `execution.resources` only for Job.

### E2E tests

- `E2E-FP-2390-001`: GitOps-drift interactive selection preserves `gitea-repo-creds`, creates the correct Secret mount, and completes the Git operation.
- `E2E-FP-2390-002`: the same journey preserves `gitea-repo-config` and creates the correct ConfigMap-backed Job volume and mount.
- `E2E-FP-2390-003`: both dependency mounts are read-only, satisfying the least-privilege verification objective. The existing standalone execution-cluster journey is separately tracked as `E2E-FP-2390-005`.
- `E2E-WE-2390-002`: shared snapshot contract at the WorkflowExecution boundary, with Job-specific resource and ServiceAccount assertions.
- `E2E-WE-2390-003`: shared snapshot contract at the WorkflowExecution boundary, with Tekton dependency workspace and PipelineRun dispatch assertions.
- `E2E-WE-2390-004`: shared snapshot contract at the WorkflowExecution boundary, with Ansible `engineConfig` and AWX execution-reference assertions. Actual AWX completion remains covered by the existing Ansible E2E scenario where AWX is available.
- `E2E-FLEET-2390-001`: a workflow-declared execution cluster remains catalog-authoritative and dispatches through the registered fleet gateway.
- Regression: `E2E-WE-006-*` continues to prove dependency injection behavior.
- Regression: `E2E-FLEET-2326-001` continues to prove execution-cluster propagation.
- Regression: existing Ansible engine tests continue to prove fail-closed behavior for unsupported remote execution.

## 9. Execution Order

1. Add RED unit tests for shared metadata extraction and interactive parity.
2. Add RED integration tests that traverse the interactive production path.
3. Implement the shared catalog metadata mapping.
4. Implement complete interactive result mapping.
5. Run CHECKPOINT W and affected unit/integration suites.
6. Refactor duplicated AA snapshot mapping.

## 10. Verification Status

Implemented in the `fix/issue-2390-workflow-snapshot` branch:

- `IT-WE-2390-001` now verifies dependency mounts, resources, service account, and declared-parameter filtering on a real Job.
- `UT-WE-2390-001` verifies both dependency volume sources and read-only mounts in the Job builder.
- `IT-WE-2390-004` verifies ConfigMap dependency propagation and read-only Job wiring through the controller path.
- `IT-WE-2390-002` and `UT-WE-2390-002` cover observable missing-dependency diagnostics.
- `IT-WE-2390-003` verifies Ansible `engineConfig` survives WFE persistence before dispatch classification.
- `IT-AA-2390-001` verifies workflow identity, version, action type, bundle, engine, and selection timestamp persistence.
- `E2E-FP-2390-001/002/003` adds the dedicated interactive GitOps fixture and verifies both dependency kinds, their Job backing sources, and read-only mounts.
- `E2E-FLEET-2390-001` is wired to the existing workflow-declared-cluster journey.

Validation completed:

- `go build ./...`
- Affected package compile checks and unit suites passed.
- Engram diagnostics report no errors in changed source files.

The full GitOps and fleet E2E suites are intentionally deferred to the CI/CD Kubernetes environment; the source, fixture, registration, and assertions are complete.
7. Add sanitized diagnostics and test sensitive-value exclusion.
8. Add the CRD admission matrix and run it before live E2E suites.
9. Extend existing Job, Tekton, and Ansible E2E journeys with the shared snapshot assertions and engine-specific checks.
10. Run the full GitOps-drift E2E scenario and fleet regression.
11. Run build, lint, unit, integration, and coverage checks.

## 10. Completion Criteria

- All P0/P1 tests pass.
- Interactive and autonomous paths produce equivalent catalog-authoritative snapshots.
- `gitea-repo-creds` and `gitea-repo-config` reach the generated Job as read-only mounts.
- `WorkflowExecution.spec.clusterId` follows the workflow declaration when present and preserves the existing fallback when absent.
- Declared parameters are enforced without conflating nil and empty metadata.
- `engineConfig` reaches Ansible through the complete chain.
- Job, Tekton, and Ansible each have one live scenario proving the common snapshot plus their engine-specific fields.
- The generated RemediationWorkflow CRD admits every valid engine fixture and rejects Job-only resources on non-Job engines through registration validation.
- No LLM-supplied value can override catalog-authoritative execution metadata.
- Sanitized diagnostics identify missing fields without exposing credential contents.
- Every wiring-manifest row has a passing integration test.
- Unit tests prove logic, integration tests prove wiring, and E2E tests prove the live remediation journey.
- `go build ./...`, affected tests, lint, and required project checks pass.
- No standard `testing.T` business tests, skipped tests, or pending tests are introduced.

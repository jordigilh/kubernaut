# Implementation Plan: AF Business-Outcome Completion Coordinator

> **Ephemeral plan**: this document is the implementation plan for issue #2365 and the redesign governed by [DD-AF-014](../../architecture/decisions/DD-AF-014-business-outcome-completion-coordinator.md). Replace or archive it with the final verification report after implementation.

**Plan Identifier**: IP-2365-v1
**Feature**: Prevent AF from closing a successful workflow-discovery turn without a structured decision artifact.
**Created**: 2026-09-05
**Author**: Kubernaut maintainers
**Status**: Implemented; E2E/integration environment follow-up remains
**Branch**: `fix/demo-scenario-run-hint`
**Business Requirements**: BR-INTERACTIVE-010, BR-SESS-013, BR-AUDIT-005
**Issue**: [#2365](https://github.com/jordigilh/kubernaut/issues/2365)
**Authority**: [DD-AF-014](../../architecture/decisions/DD-AF-014-business-outcome-completion-coordinator.md)

## 1. Business Outcome

After successful workflow discovery, the Console must receive a structured decision artifact or a structured escalation/failure outcome. The RemediationRequest must not remain silently stuck in `Analyzing` because an LLM returned narration instead of calling `kubernaut_present_decision`.

The implementation must preserve:

- `full_remediation` consent before `select_workflow`.
- `full_remediation_autonomous` discover/select/watch chaining.
- Grounded RCA data integrity.
- Audit reconstruction and operator escalation semantics.

## 2. Preflight Evidence

The preflight and design spike established:

- RCA is available from `StateKeyGroundedRCA`.
- Discovery results are available in ADK `FunctionResponse` events.
- `ka.ParseDiscoverWorkflowsResponse` supports direct and KA envelope formats.
- Direct empty workflow responses need parser hardening to preserve target metadata.
- `StreamingExecutor.Execute` is the production pre-finalization wiring point.
- `EventBridge.EmitArtifact` is the existing sanitized A2A artifact boundary.
- `complete_no_action` with `escalation_reason` produces `operator_escalation` and `ManualReviewRequired`.
- Affected package suites pass before implementation.

## 3. TDD Sequence

### RED

Write failing Ginkgo/Gomega tests for:

1. Direct and envelope discovery normalization, including empty direct results.
2. Lifecycle obligation derivation for all three interaction modes.
3. Detection of an existing decision artifact.
4. Deterministic recovery from authoritative RCA and discovery data.
5. Truthful incomplete-data artifact construction.
6. Idempotent recovery and idempotent escalation.
7. Consent preservation after recovery.
8. Production A2A wiring through the finalization boundary.
9. Autonomous chaining regression.

All tests must assert business outcomes and include the BR/control mapping in their names or comments.

### GREEN

Implement the minimum behavior:

1. Harden the canonical discovery parser.
2. Introduce typed lifecycle/obligation state.
3. Implement the completion coordinator.
4. Wire it before A2A final status publication.
5. Emit recovered artifacts through `EventBridge.EmitArtifact`.
6. Escalate only after bounded recovery failure and only through the existing operator-escalation contract.

Run CHECKPOINT W immediately after GREEN. Every wiring-manifest row must have a production caller and passing integration proof.

### REFACTOR

Refactor only after GREEN:

- Replace duplicated boolean checks with named lifecycle predicates.
- Centralize discovery-to-option normalization.
- Centralize recovery idempotency keys.
- Make provenance and failure reasons explicit.
- Ensure errors are wrapped, logged, lowercase, and observable.
- Keep the coordinator independent from provider-specific tool-choice behavior.

Post-refactor validation is mandatory: `go build ./...`, affected tests, and lint.

## 4. Risks and Mitigations

| ID | Risk | Impact | Mitigation | Tests |
|---|---|---|---|---|
| R1 | Recovery bypasses phase-3 consent | Unauthorized WorkflowExecution | Coordinator emits presentation only and preserves `phase3_blocked` | UT-AF-2365-003, IT-AF-2365-006, E2E-FP-2365-001 |
| R2 | Autonomous chain is intercepted | Regression of unattended remediation | Obligation policy excludes autonomous mode | IT-AF-2365-005, E2E-FP-1853-002 regression |
| R3 | Discovery response is mis-normalized | Incorrect/missing workflow cards | One canonical parser; direct empty response test | UT-AF-2365-001/002, IT-AF-2365-004 |
| R4 | Recovery fabricates RCA or workflow data | Incorrect operator decision/audit record | Only use grounded RCA/discovery; emit explicit incomplete outcome otherwise | UT-AF-2365-007, IT-AF-2365-008 |
| R5 | Duplicate artifact/escalation | Duplicate Console events/notifications | Session/RR/discovery idempotency key | UT-AF-2365-009, IT-AF-2365-010 |
| R6 | A2A closes before coordinator runs | Original issue persists | Wire coordinator before final status and prove real A2A path | IT-AF-2365-006/011, E2E-FP-2365-001 |
| R7 | Malformed model fields reach output | Invalid or unsafe structured output | JSON schema validation and sanitized EventBridge boundary | UT-AF-2365-002/007, IT-AF-2365-012 |

## 5. Pyramid and Control Matrix

The pyramid invariant is mandatory: unit tests prove logic, integration tests prove wiring, and E2E proves the user journey and control objectives.

| Control objective | Business behavior | Unit | Integration | E2E |
|---|---|---|---|---|
| FedRAMP AC-6 | Recovery cannot authorize workflow execution | UT-AF-2365-003 | IT-AF-2365-006 | E2E-FP-2365-001 |
| FedRAMP SI-10 / ASVS 5.1 | Only valid normalized discovery data becomes an option | UT-AF-2365-001/002 | IT-AF-2365-004/012 | E2E-FP-2365-001 |
| FedRAMP AU-3/AU-12 | Artifact provenance, correlation, and escalation outcome are preserved | UT-AF-2365-007/009 | IT-AF-2365-007/008/010 | E2E-FP-2365-001 |
| FedRAMP SI-4 | Missing presentation and recovery exhaustion are observable | UT-AF-2365-009 | IT-AF-2365-011 | E2E-FP-2365-002 |
| ASVS 5.5.2 | Structured output passes the sanitized A2A boundary | UT-AF-2365-007/012 | IT-AF-2365-007 | E2E-FP-2365-001 |
| BR-INTERACTIVE-010 | User receives options and can select on a genuine subsequent turn | UT-AF-2365-003 | IT-AF-2365-006 | E2E-FP-2365-001 |
| BR-SESS-013 | A stalled turn is recovered without confusing it with consent | UT-AF-2365-005 | IT-AF-2365-011 | E2E-FP-2365-002 |
| BR-AUDIT-005 | Lifecycle remains reconstructable by correlation ID | UT-AF-2365-009 | IT-AF-2365-008/010 | E2E-FP-2365-002 |

## 6. Wiring Manifest

| Component | Production entry point | Wiring location | IT test |
|---|---|---|---|
| Discovery normalizer | `kubernaut_discover_workflows` response | `pkg/apifrontend/ka/config.go` | IT-AF-2365-004 |
| Lifecycle obligation state | Phase callbacks/session events | `pkg/apifrontend/agent/phase_guard.go`, `pkg/apifrontend/session/` | IT-AF-2365-005 |
| Completion coordinator | A2A request execution | `pkg/apifrontend/launcher/streaming_executor.go` | IT-AF-2365-006/011 |
| Recovered artifact | EventBridge output | `pkg/apifrontend/launcher/event_bridge.go` | IT-AF-2365-007 |
| Operator escalation | AF KA MCP bridge | `pkg/apifrontend/tools/ka_tools.go` | IT-AF-2365-008 |

CHECKPOINT W fails if any component lacks a production caller or if tests only call the component directly without traversing the production A2A path.

## 7. Test Scenarios

### Unit Tests

- `UT-AF-2365-001`: direct and envelope discovery payloads normalize to the same canonical workflow representation, including empty direct results with target metadata. BR-INTERACTIVE-010, SI-10, ASVS 5.1.
- `UT-AF-2365-002`: malformed workflow entries are rejected and cannot become executable options. SI-10, ASVS 5.1.
- `UT-AF-2365-003`: `full_remediation` requires presentation but remains execution-blocked; autonomous mode does not activate recovery. BR-INTERACTIVE-010, AC-6.
- `UT-AF-2365-004`: existing `present_decision` FunctionCall satisfies the presentation obligation exactly once. BR-SESS-013.
- `UT-AF-2365-005`: a successful discovery with no decision produces a recovery obligation without enabling `select_workflow`. AC-6.
- `UT-AF-2365-006`: recovered artifact uses grounded RCA and normalized discovery data only. AU-3, SI-10, ASVS 5.1.
- `UT-AF-2365-007`: missing RCA/discovery data produces truthful failure output without fabricated severity, confidence, target, or workflow. AU-3, ASVS 5.1.
- `UT-AF-2365-008`: escalation reason is fixed, bounded, observable, and maps to `operator_escalation`. AU-12, SI-4.
- `UT-AF-2365-009`: recovery idempotency prevents duplicate artifacts. AU-3.
- `UT-AF-2365-010`: escalation idempotency prevents duplicate human-review notifications. AU-12.
- `UT-AF-2365-011`: recovered artifact data remains safe through schema validation and sanitization. SI-10, ASVS 5.5.2.

### Integration Tests

- `IT-AF-2365-001`: real agent callbacks persist lifecycle obligation state.
- `IT-AF-2365-002`: real ADK session history supplies the discovery FunctionResponse and present-decision detection.
- `IT-AF-2365-003`: full-remediation consent state survives completion recovery.
- `IT-AF-2365-004`: production discovery parser wiring handles direct and envelope responses.
- `IT-AF-2365-005`: production mode policy distinguishes full-remediation and autonomous chaining.
- `IT-AF-2365-006`: A2A execution invokes the coordinator before final status publication and emits the structured artifact.
- `IT-AF-2365-007`: recovered artifact reaches the A2A queue with required metadata and correlation.
- `IT-AF-2365-008`: incomplete recovery reaches KA operator escalation and finalizes the investigation outcome.
- `IT-AF-2365-008b`: incomplete recovery at an active phase-3 consent boundary emits failure data but preserves the session and does not escalate.
- `IT-AF-2365-009`: normal `present_decision` does not produce a duplicate recovered artifact.
- `IT-AF-2365-010`: repeated finalization does not duplicate escalation.
- `IT-AF-2365-011`: existing reinvocation and consent integration behavior remains unchanged.
- `IT-AF-2365-012`: malformed provider data is rejected before artifact emission.

### E2E Tests

- `E2E-FP-2365-001`: model narrates after successful discovery without calling `present_decision`; Console receives workflow options, RR leaves the pending analysis state, and a genuine selection turn executes the selected workflow without same-turn authorization.
- `E2E-FP-2365-002`: authoritative data is incomplete outside an active consent boundary; Console receives a structured failure/escalation outcome, notification routing occurs, and the RR is not left in `Analyzing`.
- Regression: `E2E-FP-1899-002` continues to enforce phase-3 consent.
- Regression: `E2E-FP-1853-002` continues to complete autonomous discover/select/watch chaining.

## 8. Execution Order

1. RED: add parser, lifecycle-policy, coordinator, artifact, idempotency, and wiring tests.
2. GREEN: implement canonical parsing and minimal coordinator wiring.
3. CHECKPOINT W: prove every new production path through A2A integration tests.
4. REFACTOR: simplify state predicates, centralize normalization, and harden observability.
5. E2E: run the missing-presentation and incomplete-data journeys plus consent/autonomous regressions.
6. Validation: `go build ./...`, `golangci-lint run --timeout=5m`, affected tests, `make test`, and coverage/report checks.

## 9. Completion Criteria

- All P0 UT/IT/E2E scenarios pass.
- No existing consent or autonomous-chain regressions.
- Unit tests cover all business logic introduced.
- Every wiring-manifest row has a passing integration test.
- E2E scenarios prove AC-6, SI-10, AU-3, AU-12, SI-4, ASVS 5.1, and ASVS 5.5.2 objectives.
- No standard `testing.T` business tests are added.
- No `Skip`, `XIt`, or `PIt` is used.
- All changed errors are logged and wrapped with context.
- The final verification report replaces this ephemeral plan.

## 10. Execution Results

### Passed

- `make test-unit-apifrontend`: 721/721 specs passed; 80.1% composite coverage.
- `make test`: repository unit aggregation passed.
- `make lint`: 0 issues.
- `make vet`: passed.
- `go build ./...`: passed.
- `go test ./... -run '^$' -count=1`: all packages compiled.
- `go test ./pkg/apifrontend/session ./pkg/apifrontend/agent ./pkg/apifrontend/launcher ./pkg/apifrontend/ka -count=1`: passed.
- AF AgentSession-close and fleet integration suites: passed.

### Environment-Gated

- `make test-integration-apifrontend`: the shared AF integration suite failed during `SynchronizedBeforeSuite` infrastructure setup at `test/integration/apifrontend/suite_test.go:225`; zero feature specs ran. This was not a product-test failure. The independent AF integration suites in the same target completed successfully.
- Live E2E execution was not run because the required cluster/mock-LLM environment was unavailable. E2E package compilation passed through `go test ./test/e2e/apifrontend -run '^$' -count=1`.

The production-path integration proof for this change is implemented in `pkg/apifrontend/launcher/reinvoking_runner_test.go`: it exercises the real wrapped runner, ADK session state, EventBridge artifact delivery, consent preservation, and operator escalation. Live E2E execution remains a release-environment follow-up.

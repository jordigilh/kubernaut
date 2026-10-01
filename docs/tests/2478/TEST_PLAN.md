# Test Plan — #2478: Rank Management-Aware Action Types

**IEEE 829-2008 hybrid**
**Test Plan Identifier**: TP-2478-v1
**Feature**: Cross-action workflow discovery ranking with bounded decision evidence
**Version**: 1.0
**Created**: 2026-09-30
**Status**: Active
**Branch**: `feat/2478-management-aware-workflow-ranking`

## 1. Purpose and scope

This plan verifies that KA's Step 1 workflow discovery ranks action families by
the best matching active workflow, including detected-label compatibility, while
keeping generic workflows available for RCA-driven selection. It also verifies
that the ranking basis is reconstructable from bounded audit evidence without
persisting unrestricted private model reasoning.

The implementation is limited to KA's informer-cache-backed discovery
integration and its existing audit path. DataStorage remains the audit sink;
Helm target-parameter metadata in `kubernaut-demo-scenarios#445` and
intentional `result_omitted` schema redaction are separate work.

## 2. Business and control objectives

| Requirement/control | Business behavior verified |
|---|---|
| BR-KA-017-001, BR-KA-017-007 | Three-step discovery receives and uses detected labels for action-family ranking. |
| BR-KA-265 | The LLM receives bounded, structured label-match evidence in Phase 3 context. |
| BR-AUDIT-021-030, BR-AUDIT-005 | Discovery ordering, ranking basis, filters, and correlation ID are reconstructable from audit events. |
| FedRAMP AU-2 | `workflow.catalog.actions_listed` remains emitted for the ranking decision. |
| FedRAMP AU-3 | Audit content includes actor/event identity, correlation, filters, rank, score, and preference reason. |
| FedRAMP SI-10 | Tool pagination and structured discovery fields remain validated and server-generated ranking fields cannot be model-supplied. |
| FedRAMP AC-6 | Ranking is advisory; the existing context/security gate still controls workflow retrieval. |
| SOC 2 CC7.2 | A correlation ID reconstructs the action-family discovery decision and subsequent selection evidence. |
| SOC 2 CC8.1 | The change is implemented through the approved plan, RED/GREEN/REFACTOR, review, and regression gates. |
| OWASP ASVS V4.1.3 | An advisory rank cannot authorize access to a workflow outside the existing catalog security gate. |
| OWASP ASVS V5.1 | Cursor/tool inputs and generated discovery fields retain schema validation and bounded handling. |
| OWASP ASVS V5.5.2 | Structured discovery and audit output crosses the existing typed/sanitized boundary. |
| OWASP ASVS V7.1.1/V7.2.1 | Security-relevant discovery evidence is logged with correlation and bounded redaction. |

### 2.1 Executable control-objective evidence

| Control objective | Business-level assertion | Executable evidence |
|---|---|---|
| FedRAMP AU-2 / AU-3 | A successful discovery decision emits a typed event with event ID, action, outcome, actor, correlation ID, filters, ranking evidence, score, and workflow identity. | IT-KA-2478-003; E2E-KA-2478-001/002 |
| FedRAMP AC-6 / OWASP ASVS V4.1.3 | A preferred rank is advisory; workflow retrieval still applies the existing context gate and does not disclose filtered workflow existence. | IT-KA-2478-004 |
| FedRAMP SI-10 / OWASP ASVS V5.1 | Malformed detected-label input is rejected before catalog dispatch; opaque pagination remains bounded. | UT-KA-2478-006; existing UT-KA-688-101..104 |
| OWASP ASVS V5.5.2 | The model-facing response contains bounded ranking evidence and excludes internal score/workflow identity fields. | UT-KA-2478-004; IT-KA-2478-002; E2E-KA-2478-001 |
| SOC 2 CC7.2 / OWASP ASVS V7.1.1/V7.2.1 | A correlation-ID query reconstructs the four discovery events and their typed payloads without unrestricted private reasoning. | E2E-KA-2478-001/002 |

## 3. Pyramid test scenarios

### Unit tests — business logic

| ID | Scenario | BR/control trace |
|---|---|---|
| UT-KA-2478-001 | An exact `helmManaged=true` workflow ranks `HelmRollback` above generic `RestartPod` using the existing score formula. | BR-KA-017-007, BR-KA-265 |
| UT-KA-2478-002 | Generic action families remain in the response and can remain the valid choice when no management label matches. | BR-KA-017-001, AC-6, ASVS V4.1.3 |
| UT-KA-2478-003 | Action-family ties use deterministic action-type ordering; rank is assigned before pagination. | BR-KA-017-001, SI-10 |
| UT-KA-2478-004 | Response evidence contains rank, preferred status, matched labels, and a deterministic preference reason. | BR-KA-265, ASVS V5.5.2 |
| UT-KA-2478-005 | Step 2 workflow ordering remains `final_score DESC, workflow_id ASC`. | BR-KA-017-001 |
| UT-KA-2478-006 | Malformed detected-label data still fails the catalog call instead of being silently discarded. | SI-10, ASVS V5.1 |

### Integration tests — wiring

| ID | Scenario | BR/control trace |
|---|---|---|
| IT-KA-2478-001 | Envtest informer cache → `Catalog.ListActions` ranks management-aware and generic action types correctly. | BR-KA-017-001/007 |
| IT-KA-2478-002 | Production custom-tool registration → `list_available_actions` returns the ranked structured response. | BR-KA-017-001, ASVS V5.5.2 |
| IT-KA-2478-003 | `workflow.catalog.actions_listed` persists complete bounded ranking evidence through the KA→DS audit path. | AU-2, AU-3, CC7.2, ASVS V7.1.1/V7.2.1 |
| IT-KA-2478-004 | Lower-ranked generic selection remains subject to the existing context/security gate. | AC-6, ASVS V4.1.3 |

### End-to-end tests — business journey

| ID | Scenario | BR/control trace |
|---|---|---|
| E2E-KA-2478-001 | A completed three-step selection persists the complete four-event discovery trace plus global action-family rank, preference, score, and correlation evidence. | BR-KA-017-001, BR-AUDIT-005, AU-2/AU-3, CC7.2, ASVS V7.1.1/V7.2.1 |
| E2E-KA-2478-002 | A generic workflow remains selectable and the complete discovery/selection trace remains reconstructable when no management label matches. | BR-KA-017-001, AC-6, AU-2/AU-3, CC7.2, ASVS V4.1.3 |

## 4. Wiring manifest

| Component | Production entry point | Wiring location | IT/E2E proof |
|---|---|---|---|
| Cross-action aggregate ranking | `Catalog.ListActions` | `internal/kubernautagent/workflowcatalog/discovery_cache.go` | IT-KA-2478-001 |
| LLM-facing ranking evidence | `list_available_actions` | `internal/kubernautagent/tools/custom/tools.go` | IT-KA-2478-002, E2E-KA-2478-001 |
| Production tool registration | Custom discovery tool set | `cmd/kubernautagent/toolregistry.go` | IT-KA-2478-002 |
| Audit evidence persistence | `workflow.catalog.actions_listed` | `internal/kubernautagent/tools/custom/discovery_audit.go` → `internal/kubernautagent/audit/ds_store.go` | IT-KA-2478-003, E2E-KA-2478-001/002 |
| Lower-ranked override rationale | Phase 3 result | `internal/kubernautagent/prompt/templates/phase3_workflow_selection.tmpl` and parser/result audit path | UT-KA-2478-004, E2E-KA-2478-002, existing E2E-KA-017-001-001 |

## 5. TDD execution gates

1. **RED**: add the scenarios above and confirm the new ranking/evidence
   assertions fail against the alphabetical Step 1 implementation.
2. **GREEN**: implement the smallest cache aggregation, response, prompt, and
   audit-schema changes needed for all UT/IT/E2E wiring points to pass.
3. **REFACTOR**: share candidate scoring, keep deterministic ordering, bound
   audit evidence, and preserve existing error/security-gate behavior.
4. **Post-refactor validation**: `go build ./...`, compile-only tests, targeted
   tests, full test suite, lint, and the repository's TDD/business-integration
   checks.

## 6. Exit criteria

- All listed UT, IT, and E2E scenarios pass; no pending/skipped tests.
- Every wiring-manifest row has passing evidence.
- Audit evidence is queryable by correlation ID and contains no unrestricted
  private chain-of-thought.
- Existing Step 2 ordering and security-gate regressions are absent.
- Build, lint, coverage, and repository methodology checks pass.

## 7. Execution record — 2026-09-30

| Make target | Result | Evidence or blocker |
|---|---|---|
| `make test-unit-kubernautagent` | PASS | Default parallelism; 131/131 specs passed; composite coverage 81.8%. |
| `make GINKGO_FOCUS=2478 test-integration-kubernautagent` | PARTIAL | Workflow-catalog focused specs passed 2/2. Custom-tools setup could not build the DataStorage image because Podman/Buildah reported `no space left on device`; its focused specs did not run. |
| `make GINKGO_FOCUS=2478 KA_E2E_PROCS=<host CPUs> test-e2e-kubernautagent` | BLOCKED | E2E setup could not build the mock-LLM image because Podman/Buildah reported `no space left on device`; 0/109 specs ran. |
| `go build ./...` | PASS | Repository build completed successfully. |

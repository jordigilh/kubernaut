# Test Plan: Typed Investigation Outcomes and Decision Ledger

**Test Plan Identifier**: TP-2520-2521-v1.1
**Feature**: Explicit investigation terminal outcomes, target-scoped Phase 3 handoff, bounded Kubernetes tool output, and bounded decision-ledger audit events
**Version**: 1.2
**Created**: 2026-10-16
**Author**: Kubernaut contributors
**Status**: Approved for RED/GREEN/REFACTOR implementation; Issue #2521 scoped gates in progress
**Branch**: `main` (Issue #2521 follow-up)

---

## 1. Introduction

### 1.1 Purpose

This plan verifies that investigation results distinguish valid RCA outcomes from
#2518 MCP/history transport exhaustion, workflow-discovery failures, and genuine
no-workflow results. It also verifies that the new decision-ledger audit event is
bounded, redacted, attributable, correlated, and wired through the production
Investigator, AgentSession, and AIAnalysis paths.

### 1.2 Objectives

1. Prove all typed terminal outcomes and status enums round-trip with lower-snake-case wire values.
2. Prove #2518 retry/reconnect success and exhaustion are consumed without duplicating retry logic or mislabeling transport failure as RCA-incomplete.
3. Prove missing selected workflow never becomes no-match unless discovery explicitly completes with zero candidates.
4. Prove decision-ledger events reconstruct meaningful RCA-to-discovery transitions by logical correlation ID.
5. Prove ledger payloads exclude prompts, hidden reasoning, credentials, raw transport errors, and unrestricted tool arguments/results.
6. Prove every new production component has a passing integration wiring test.
7. Prove Phase 3 receives only structured RCA, authoritative RCA-target Phase 2 enrichment/history, target resolution, repeat context, and workflow catalog data; raw PhaseRCA observations and the full investigation conversation are excluded.
8. Prove Kubernetes bulk list and exact-name lookup responses are bounded at the API source before sanitizer, LLM-context, audit, or memory amplification.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| In-scope scenario coverage | 21/21; minimum 90%, critical scenarios 100% | BR matrix and passing UT/IT assertions |
| Unit test pass rate | 100% | Affected Ginkgo suites |
| Integration wiring pass rate | 100% of manifest rows | KA/AA integration suites |
| Audit redaction | 100% of ledger fields allow-listed and bounded | UT-KA-2520-002 and schema assertions |
| Backward compatibility | 0 regressions | Existing investigator, mapping, handler, and generated-client tests |

**Execution result (2026-10-16)**: all nine planned Tier-1 scenarios pass in the
affected Ginkgo suites. Both integration packages compile with `-run '^$'`; the
runtime AIAnalysis integration bootstrap reached zero specs and then failed while
building the DataStorage image because Podman/Buildah reported `no space left on
device`. No changed-package assertion failure was observed. `go build ./...`,
repository-wide compile-only tests, `git diff --check`, and changed-code lint
(`golangci-lint run --new-from-rev=HEAD`) pass. The repository-wide lint command
still reports pre-existing findings outside the changed paths; no new finding is
reported for this change. The aggregate `make test` unit target also passed
(25 Ginkgo suites, 742 specs in the final aggregate output). The 14/14 target is
therefore not claimed until the integration harness can start.

**Issue #2521 amendment:** the preceding execution record is historical for the
original #2520/#2518 scope. The current matrix defines 21 scenarios, including
target-scoped Phase 3 handoff, cluster-scoped history, source-bounded Kubernetes
lists/JQ/counts, and bounded audit projection. Current validation is recorded by the
implementation session; unrelated repository-wide baseline failures remain
separate from this change.

---

## 2. References

### 2.1 Authority

- BR-AUDIT-005 v2.0 — correlated, attributable, reconstructable audit events
- BR-KA-016 / DD-KA-2496 — target-scoped history and safe handoff
- BR-KA-197 / BR-KA-197.2 / #768 — human review and genuine no-workflow semantics
- BR-KA-261 — enrichment failure classification and safe degradation
- DD-KA-2511 — recurrent RCA continuation and target re-enrichment
- DD-KA-2520-2521 — approved typed outcome and decision-ledger contract
- Issue #2518 — MCP retry/reconnect and evidence transport exhaustion
- Issues #2520 and #2521 — decision ledger and typed terminal outcomes

### 2.2 Control mappings

- FedRAMP AU-2, AU-3, AU-9, AU-11, AC-4, AC-6, SI-10
- SOC 2 CC7.2; CC8.1 only for the controlled implementation process
- OWASP ASVS V5.1, V5.5.2, V7.1.1, V7.2.1

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Missing workflow is inferred as no-match | False negative or unsafe routing | High | UT-KA-2521-001/004, IT-AA-2521-001 | Require explicit completed discovery state |
| R2 | #2518 transport exhaustion is mapped to RCA-incomplete | Misleading operator diagnosis and lost repeat context | Critical | UT-KA-2521-002/003, IT-KA-2521-002 | Consume typed exhaustion and preserve history status |
| R3 | Workflow-discovery/provider failure is confused with no-match | Incorrect terminal route | High | UT-KA-2521-005, IT-AA-2521-001 | Gate outcome on phase-aware discovery status |
| R4 | Retries duplicate or split audit correlation | Incomplete forensic reconstruction | High | UT-KA-2521-006, UT-KA-2520-003, IT-KA-2520-002 | Preserve logical correlation and deterministic ledger identity |
| R5 | Prompt, reasoning, credentials, or raw tool data enters audit | Security/compliance exposure | Critical | UT-KA-2520-002, IT-KA-2520-001 | Central allow-list, size bounds, adversarial redaction cases |
| R6 | Types are implemented but not wired through handlers/generated schemas | Production behavior remains unchanged | High | IT-KA-2520-001, IT-AA-2521-001, wiring audit | Test real production dispatch and generated contract parity |

### 3.1 Risk-to-Test Traceability

All Critical/High risks have at least one mandatory P0 test. No risk is accepted
without a proving UT and, where a production boundary exists, a wiring IT.

---

## 4. Scope

### 4.1 Features to be Tested

- `pkg/shared/types/`: canonical typed outcome and status enums.
- `pkg/kubernautagent/types/InvestigationResult`: server-owned typed state and compatibility projections.
- `internal/kubernautagent/investigator/`: phase-aware classification, #2518 exhaustion consumption, early-return behavior, and decision-ledger emission.
- `internal/kubernautagent/agentsession/`: Investigator-to-AgentSession mapping and terminal status propagation.
- `pkg/aianalysis/handlers/`: AgentSession-to-AIAnalysis routing and no-match guard precedence.
- `internal/kubernautagent/audit/` and DataStorage schemas: typed payload mapping, redaction, bounds, and envelope fidelity.
- `internal/kubernautagent/prompt/` and `internal/kubernautagent/investigator/`: structured Phase 3 handoff and fixed prompt/context budget.
- `pkg/kubernautagent/tools/k8s/`: source-side metadata/count list summaries and server-side exact-name lookup.
- Generated OpenAPI/CRD/client artifacts: schema and conversion parity.

### 4.2 Features Not to be Tested

- MCP reconnect/retry mechanics themselves: owned by #2518; this plan tests typed success/exhaustion consumption only.
- A long-lived live SSE reset: requires the #2518 integration/E2E environment and is not duplicated here.
- Unrelated audit event categories or existing RCA narrative content beyond ensuring the new ledger does not copy it.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Explicit typed state is authoritative | Prevents inference from `SelectedWorkflow == nil`. |
| Existing audit store plus `aiagent.investigation.decision` | Reuses immutable retention/correlation infrastructure without a second persistence model. |
| #2518 owns retry/reconnect | Avoids competing retry policies and preserves one logical LLM tool result. |
| Compatibility projections remain | Existing consumers can continue using legacy reason/review fields while migration proceeds. |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: Ginkgo/Gomega behavior tests for typed values, classifiers, mappers, and redaction; target >=90% of in-scope scenarios and 100% of critical scenarios.
- **Integration**: Ginkgo/Gomega production-path tests for audit dispatch, AgentSession mapping, AIAnalysis routing, and DataStorage schema fidelity; target 100% of wiring rows.
- **E2E**: Deferred for this plan because #2518 owns the live MCP reset journey; the typed exhaustion result is covered at the KA/AA integration boundary.

### 5.2 Two-Tier Minimum

Every business requirement in scope has a UT and an IT proof unless explicitly
identified as a pure wire/schema assertion. Unit tests mock only external LLM,
DataStorage, Kubernetes, or network dependencies; internal business logic is real.

### 5.3 Pass/Fail Criteria

**PASS** requires all P0 tests, all wiring rows, generated schema parity, no
regressions in affected suites, and 14/14 planned scenarios passing. Any false
`no_matching_workflows`, raw ledger secret/reasoning leakage, or missing production
caller is a release-blocking failure.

**FAIL** occurs for any P0 failure, any critical scenario gap, generated-contract
drift, or any terminal path that maps #2518 transport exhaustion to
`rca_incomplete` or `no_matching_workflows`.

### 5.4 Suspension & Resumption

Suspend when the branch cannot compile, the generated contract source is
ambiguous, #2518's typed exhaustion result is unavailable, or more than three
failures share an unisolated infrastructure cause. Resume after the dependency or
fixture is made explicit and the failing cause is recorded.

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File/package | Functions/behavior |
|--------------|--------------------|
| `pkg/shared/types/` | Enum values, validation, JSON round-trip |
| `pkg/kubernautagent/types/` | Result fields and compatibility semantics |
| `internal/kubernautagent/investigator/` | Outcome classification, discovery gate, ledger builder/redaction |
| `internal/kubernautagent/audit/` | Typed payload conversion and bounded allow-list |
| `internal/kubernautagent/agentsession/` | Result mapping and legacy fallback |

### 6.2 Integration-Testable Code

| File/package | Wiring |
|--------------|--------|
| `test/integration/kubernautagent/investigator/` | Production Investigator → audit store and terminal result |
| `internal/kubernautagent/agentsession/` | AgentSession status result propagation |
| `test/integration/aianalysis/` | AgentSession → AIAnalysis handler routing |
| `pkg/datastorage/ogen-client/` and audit OpenAPI | Typed payload encode/decode parity |
| `cmd/kubernautagent/` | Production resolver/metadata-client and Investigator budget wiring |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `feat/2510-2511-recurrent-rca` HEAD | Approved implementation branch |
| #2518 retry contract | Open issue #2518 | Consumed, not reimplemented |
| Schema source | Repository OpenAPI/CRD sources | Generated artifacts must be synchronized |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-2521 | Typed terminal outcome and no-fabricated-no-match semantics | P0 | Unit | UT-KA-2521-001..005 | PASS |
| BR-2521 | #2518 transport exhaustion is distinct and propagated | P0 | Unit/Integration | UT-KA-2521-002, IT-KA-2521-002 | Unit PASS; IT blocked at bootstrap |
| BR-2521 | Successful retry preserves logical correlation | P0 | Unit | UT-KA-2521-006 | PASS |
| BR-AUDIT-005 | Decision checkpoints are attributable and reconstructable | P0 | Unit/Integration | UT-KA-2520-001/003, IT-KA-2520-001/002 | Unit PASS; IT blocked at bootstrap |
| BR-AUDIT-005 | Bounded redacted audit data | P0 | Unit/Integration | UT-KA-2520-002, IT-KA-2520-001 | Unit PASS; IT blocked at bootstrap |
| BR-KA-016 / DD-KA-2496 | Target-scoped history remains safe | P0 | Integration | IT-KA-2521-002 | Blocked at bootstrap |
| BR-KA-197 / BR-KA-197.2 / #768 | Genuine no-match remains a completed discovery result | P0 | Unit/Integration | UT-KA-2521-004, IT-KA-2520-003 | Unit PASS; IT blocked at bootstrap |
| BR-KA-261 / DD-KA-2511 | Enrichment failure is safely degraded and observable | P0 | Unit/Integration | UT-KA-2521-002/003, IT-KA-2520-003 | Unit PASS; IT blocked at bootstrap |
| BR-KA-016 / BR-2521 / DD-KA-017 / ADR-055 | Phase 3 handoff excludes raw PhaseRCA observations and remains target-scoped | P0 | Unit/Integration | UT-KA-2521-007, IT-KA-2521-003/004 | PASS |
| BR-KA-016 / DD-KA-2496 | Cluster-scoped target history remains associated with the canonical target | P0 | Unit | UT-KA-2521-010 | PASS |
| BR-SAFETY-752 / SI-10 / AC-4 / AC-6 | Bulk and exact-name Kubernetes tool output is bounded at source | P0 | Unit/Integration | UT-KA-752-112/113, IT-KA-752-004 | PASS |
| BR-SAFETY-752 / SI-10 / AC-4 / AC-6 | Cluster-scoped metadata lists and metadata-only JQ cannot expose bulk bodies | P0 | Unit | UT-KA-752-114/115 | PASS |
| BR-SAFETY-752 / SI-10 | Count output distinguishes retained page count from known total source count | P0 | Unit | UT-KA-752-116 | PASS |
| BR-SAFETY-752 / SI-10 | Paginated fallback explicitly marks unknown total counts instead of misreporting the retained page as corpus total | P0 | Unit | UT-KA-752-117 | PASS |
| BR-SAFETY-752 / SI-10 | JQ fails closed when the metadata-only source path is unavailable | P0 | Unit | UT-KA-752-118 | PASS |
| BR-SAFETY-752 / SI-10 | Metadata-only find_resource reports truncation instead of a false empty result | P0 | Unit | UT-KA-752-119 | PASS |
| BR-AUDIT-005 / DD-AUDIT-003 | Bulk tool audit projection excludes raw payloads while preserving bounded summary metadata | P0 | Unit | UT-KA-2521-009 | PASS |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-2521-001` | Typed values round-trip; missing workflow ID is not no-match. | PASS |
| `UT-KA-2521-002` | History/MCP transport exhaustion yields typed unavailable outcome and discovery does not start. | PASS |
| `UT-KA-2521-003` | Valid evidence plus explicitly insufficient causal evidence yields `rca_incomplete`, not transport failure. | PASS |
| `UT-KA-2521-004` | Explicit completed discovery with zero candidates yields genuine no-match. | PASS |
| `UT-KA-2521-005` | Discovery parser/catalog/provider/validation failure yields discovery failure, not no-match. | PASS |
| `UT-KA-2521-006` | Successful #2518 retry preserves logical correlation and proceeds to discovery. | PASS |
| `UT-KA-2520-001` | Tool-only turn differs from terminal decision in the ledger. | PASS |
| `UT-KA-2520-002` | Ledger is bounded, allow-listed, redacted, and contains no private reasoning/secrets. | PASS |
| `UT-KA-2520-003` | RCA-to-discovery gate transitions are ordered and reconstructable. | PASS |
| `UT-KA-2521-007` | The frozen Phase 3 packet contains structured RCA and RCA-target enrichment/history but no `current_observations`, raw PhaseRCA tool output, or conversation transcript; signal-target observations cannot become RCA-target workflow evidence. | PASS |
| `UT-KA-2521-008` | Structured-context budget rejects oversized Phase 3 prompt/context while ordinary oversized PhaseRCA tool output remains available to RCA and does not create a typed terminal failure solely because it was large. | PASS |
| `UT-KA-2521-009` | Bulk Kubernetes audit output contains only bounded item counts, truncation status, digest and correlation reference; raw bodies are absent. | PASS |
| `UT-KA-2521-010` | Node/PersistentVolume history in legacy or canonical cluster-scoped forms remains target-scoped and contributes repeat context. | PASS |
| `UT-KA-752-112` | Kubernetes bulk list requests a bounded server-side page and returns metadata/count/truncation summary without ConfigMap/Secret bodies. | PASS |
| `UT-KA-752-113` | Exact-name cluster lookup uses a server-side `metadata.name` selector and bounded limit rather than fetching and filtering an unrestricted list. | PASS |
| `UT-KA-752-114` | Cluster-scoped metadata lists work without a namespace subresource. | PASS |
| `UT-KA-752-115` | JQ bulk queries are metadata-only and cannot retrieve ConfigMap/Secret data. | PASS |
| `UT-KA-752-116` | Count output reports retained and total source items separately when the metadata API provides a safe total. | PASS |
| `UT-KA-752-117` | Paginated fallback omits `total_count` when the API does not provide a safe remaining-item count. | PASS |
| `UT-KA-752-118` | JQ does not fall back to fetching full bulk objects when metadata-only access is unavailable. | PASS |
| `UT-KA-752-119` | Bounded find_resource reports truncation when a match may be outside the retained page. | PASS |

### Tier 2: Integration Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-KA-2520-001` | Production KA investigation dispatch persists the typed ledger payload through the audit store. | BLOCKED: suite bootstrap |
| `IT-AA-2521-001` | AgentSession result reaches AIAnalysis status and the correct terminal handler. | BLOCKED: suite bootstrap |
| `IT-KA-2520-002` | Successful investigation timeline is queryable by one logical correlation ID. | BLOCKED: suite bootstrap |
| `IT-KA-2520-003` | Genuine no-match timeline is queryable without an execution resource. | BLOCKED: suite bootstrap |
| `IT-KA-2521-002` | Transport/history failure timeline shows unavailable history and skipped discovery. | BLOCKED: suite bootstrap |
| `IT-KA-2521-003` | Production RCA completion reaches workflow selection with one target-scoped structured packet and no raw observations. | PASS |
| `IT-KA-2521-004` | Production PhaseRCA tool output path does not capture raw observations into the Phase 3 handoff. | PASS |
| `IT-KA-752-004` | Production K8s tool registration uses the bounded resolver/metadata-client path for list and exact-name lookup. | PASS |
| `IT-KA-2521-005` | `cmd/kubernautagent.registerK8sTools` reaches the bounded list result path without raw bulk bodies. | PASS |

### Tier 3: E2E Tests

Deferred to #2518's long-lived MCP reset journey. This plan consumes a typed
retry-success or retry-exhausted fixture at the integration boundary and does not
duplicate live transport fault injection.

### 8.1 Control and security traceability for Issue #2521

The new scenarios map to the approved objectives: FedRAMP AU-2/AU-3/AU-9/AU-11
for bounded correlated audit metadata, SI-10 for structured input/output bounds,
AC-4/AC-6 for target and access scope; SOC 2 CC7.2 for reconstructable lifecycle
decisions and CC8.1 for the controlled change process; and OWASP ASVS
V5.1/V5.5.2/V7.1.1/V7.2.1 for input validation, data minimization, authorization
boundaries, and safe error handling. Raw bulk ConfigMap/Secret bodies, prompts,
private reasoning, and unrestricted tool arguments/results are not valid test
evidence or persistence formats.

---

## 9. Test Cases

### UT-KA-2521-002: #2518 transport exhaustion classification

**BR**: BR-KA-261, BR-2521
**Priority**: P0
**Type**: Unit
**File**: `internal/kubernautagent/investigator/*_test.go`

**Given** a typed #2518 retry-exhausted result for target-scoped history, with
source and bounded retry metadata, and a valid initial RCA.
**When** the Investigator finalizes the result.
**Then** it preserves the RCA, sets `HistoryStatus=transport_unavailable`, emits
`TerminalOutcome=evidence_transport_unavailable` (or the approved source-specific
projection), sets discovery to `skipped_due_to_failure`, requests review, and does
not emit a no-match result or call workflow discovery.

### UT-KA-2521-004: explicit genuine no-match

**BR**: BR-KA-197, BR-KA-197.2, #768
**Priority**: P0
**Type**: Unit
**File**: `internal/kubernautagent/investigator/*_test.go`

**Given** discovery has started and the discovery parser accepts an explicit
`submit_result_no_workflow` result. **When** the result is finalized. **Then** the
outcome is `no_matching_workflows`, the gate is `completed_no_match`, and the
transport/history statuses remain truthful.

### UT-KA-2520-002: bounded decision-ledger redaction

**BR**: BR-AUDIT-005
**Priority**: P0
**Type**: Unit
**File**: `internal/kubernautagent/audit/*_test.go`

**Given** tool arguments/results, prompts, credentials, raw transport errors, and
reasoning text containing adversarial secrets. **When** the ledger payload is
built. **Then** only allow-listed enums, counts, IDs/digests, and bounded summaries
remain; each field is within its byte limit and no secret, prompt, unrestricted
tool data, or hidden reasoning is present.

### IT-AA-2521-001: production terminal routing

**BR**: BR-2521, BR-AUDIT-005
**Priority**: P0
**Type**: Integration
**File**: `test/integration/aianalysis/`

**Given** AgentSession results for selected workflow, explicit no-match, workflow
discovery failure, and #2518 evidence transport exhaustion. **When** the real
AIAnalysis production handler processes them. **Then** each maps to its intended
status/handler, absent selected workflow never enters the no-match handler, and
typed fields survive the status write.

Remaining scenarios follow the same Given/When/Then contract in the matrix above;
all are P0 and must be implemented rather than skipped or marked pending.

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: external LLM, DataStorage, Kubernetes, and transport-result providers only
- **Location**: existing `internal/kubernautagent/...` Ginkgo suites
- **Resources**: local Go toolchain; no live cluster required

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: zero mocks for production wiring; use real in-process audit store and service boundaries
- **Infrastructure**: existing integration harness, DataStorage test path, generated client, and Kubernetes test fixtures
- **Location**: `test/integration/kubernautagent/investigator/` and `test/integration/aianalysis/`
- **Observed limitation**: runtime bootstrap was blocked before any spec ran because
  Podman/Buildah exhausted its storage while building the DataStorage image
  (`no space left on device`). Package compilation passed.

### 10.3 E2E Tests

- Deferred to #2518's owned long-lived MCP/SSE reset environment.

### 10.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | Repository toolchain | Build and test |
| Ginkgo/Gomega | Repository versions | BDD unit/integration tests |
| golangci-lint | Repository CI version | Static analysis |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| Issue #2518 typed retry/exhaustion result | Code contract | Open/owned by #2518 | Transport classification fixture cannot prove source metadata | Use an explicit test fixture implementing the agreed result boundary; do not implement retry here |
| OpenAPI/CRD generation toolchain | Build | Repository-provided | Generated client/schema parity cannot be verified | None; suspend generation phase |
| Existing integration harness | Infrastructure | Blocked at runtime | Wiring proof blocked | Compile-only package check passed; rerun runtime suites after container storage recovery |

### 11.2 Execution Order

1. **RED**: create failing typed-contract, classifier, redaction, routing, and audit tests.
2. **GREEN**: add minimal typed state, wire it through production paths, consume #2518 result, and persist the ledger event.
3. **REFACTOR**: centralize classification/redaction, preserve compatibility, and remove duplication.
4. **WIRING VERIFICATION**: run every manifest integration proof through production entry points.
5. **E2E**: coordinate only with the #2518 long-lived MCP reset validation.

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/2520-2521/TEST_PLAN.md` | Approved scenario/control strategy |
| Unit tests | Existing investigator/audit/agentsession packages | Typed classification and redaction contracts |
| Integration tests | `test/integration/kubernautagent/` and `test/integration/aianalysis/` | Production wiring and schema fidelity |
| Coverage record | Test execution report | 14/14 requirements-based scenarios and tier results |

---

## 13. Execution

```bash
# Affected unit suites (PASS)
go test ./internal/kubernautagent/investigator/... ./internal/kubernautagent/audit/... ./internal/kubernautagent/agentsession/... ./internal/kubernautagent/enrichment/... ./internal/kubernautagent/parser/... ./pkg/aianalysis/... ./pkg/shared/types -count=1 -timeout=15m

# Affected integration suites (runtime BLOCKED during suite bootstrap)
go test ./test/integration/kubernautagent/investigator/... ./test/integration/aianalysis/...

# Compile-only regression check (PASS)
go test ./... -run=^$ -timeout=30s

# Required repository checks after implementation
go build ./...
golangci-lint run --timeout=5m
make test

# Package-only integration compilation (PASS)
go test ./test/integration/kubernautagent/investigator ./test/integration/aianalysis -run '^$' -count=1 -timeout=5m
```

---

## 14. Wiring Verification

| Code Path | Entry Point | Exit Point | Wiring IT | Status |
|-----------|-------------|------------|-----------|--------|
| Typed Investigator terminal state | KA investigation/session terminal callback | AgentSession result/status | `IT-KA-2521-002` | Production path present; runtime IT blocked |
| Result-to-AgentSession mapping | AgentSession status writer | Typed AgentSession result | `IT-AA-2521-001` | Production path present; runtime IT blocked |
| AgentSession-to-AIAnalysis routing | AIAnalysis reconciliation/response handler | Terminal status/handler | `IT-AA-2521-001` | Production path present; runtime IT blocked |
| Decision-ledger emitter | Investigator turn/state transition | DataStorage audit event | `IT-KA-2520-001` | Production path present; runtime IT blocked |
| Typed DataStorage mapper | Audit store dispatch | Persisted event payload | `IT-KA-2520-001` | Generated/client path compiled; runtime IT blocked |
| Final audit projection | RCA/response completion path | Queryable typed terminal event | `IT-KA-2520-002/003` | Generated/client path compiled; runtime IT blocked |

CHECKPOINT W passes only when every row has a production caller and a passing
integration proof.

---

## 15. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| Investigator enrichment/recurrent RCA tests | History unavailable maps to `rca_incomplete` | Assert typed transport/history outcome plus compatibility projection | #2518 must not be misclassified |
| `internal/kubernautagent/agentsession/mapping_test.go` | Legacy human-review reason drives mapping | Typed state has precedence; legacy projection remains compatible | New authoritative contract |
| AIAnalysis handler tests | Missing workflow selects no-match/failure guard | Typed outcome and discovery gate select the route | Prevent fabricated no-match |
| Audit payload tests | Existing event payload has no decision checkpoint | Add typed ledger payload and terminal projection assertions | BR-AUDIT-005 reconstruction |

---

## 16. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-10-16 | Initial approved plan for issues #2520/#2521; incorporates #2518 transport-failure reassessment. |
| 1.1 | 2026-10-16 | Recorded passing scoped unit/build/compile checks and the Podman storage limitation blocking runtime integration suites. |

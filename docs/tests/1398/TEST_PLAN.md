# Test Plan: Structured Approval Request A2A Events

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1398-v1
**Feature**: Structured approval_request and approval_request_resolved A2A events for console rendering
**Version**: 1.0
**Created**: 2026-06-11
**Author**: AI Agent
**Status**: Draft
**Branch**: `feat/structured-decision-payload`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the structured A2A event emission when a RemediationApprovalRequest (RAR) enters the pending state, and when the approval decision is made or the RAR expires. The console receives machine-parseable JSON events to render a rich approval card with Approve/Decline buttons, confidence badges, evidence panels, and countdown timers.

### 1.2 Objectives

1. **Approval event emission**: When `kubernaut_watch` detects `AwaitingApproval`, GET the RAR and emit a structured `approval_request` event with full spec fields
2. **Resolution event emission**: When the RAR watch channel fires a decision (`Approved`/`Rejected`/`Expired`), emit a structured `approval_request_resolved` event
3. **Graceful degradation**: If RAR GET fails (RBAC, not-yet-created), continue with existing text-only behavior (no regression)
4. **Edge case ordering**: If RAR already has a decision at detection time, emit both events in sequence (ordering guarantee)
5. **Nil-safe emission**: `EmitStructuredMetaSafe` helper is safe when no EventBridge is in context
6. **Console wire contract**: Event JSON matches the payload contract agreed with the demo-console team

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./pkg/apifrontend/tools/... ./pkg/apifrontend/launcher/...` |
| Integration test pass rate | 100% | `go test ./pkg/apifrontend/tools/... -run IT-AF-1398` |
| E2E test pass rate | 100% | `make test-e2e-apifrontend` (approval event focus) |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on logic files |
| Integration-testable code coverage | >=80% | `go test -coverprofile` on wiring files |
| E2E code coverage | >=80% | `E2E_COVERAGE=true` binary coverage collection |
| Backward compatibility | 0 regressions | All existing `UT-AF-106-*` watch tests pass without modification |
| JSON integrity | 0 malformed payloads | All emitted events are valid JSON parseable by `json.Unmarshal` |

---

## 2. References

### 2.1 Authority (governing documents)

- Issue #1398: Emit structured approval_request A2A event for console rendering
- Issue #1396: AF: Emit structured RCA and extended workflow options in present_decision payload (same pattern)
- Issue #1395: EventBridge sanitizeBridgeText truncates structured JSON payloads at 512 runes (infrastructure)
- ADR-040: RAR CRD design (immutable spec, mutable status)
- DD-AUTH-MCP-001 v3.0: Trusted intermediary delegation

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Wiring Verification](../../.cursor/rules/10-wiring-verification.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)
- [100 Go Mistakes](https://github.com/teivah/100-go-mistakes) — refactoring validation
- PR #1397: Structured decision payload implementation (same infrastructure)

### 2.3 FedRAMP Control Objectives

| Control | NIST Intent | Application to This Feature |
|---------|-------------|----------------------------|
| **AU-3** | Audit records contain sufficient detail | Approval event JSON includes all RAR spec fields (confidence, evidence, policy, workflow) |
| **SI-4** | Real-time monitoring | `metadata.type=approval_request` classification enables monitoring/alerting separation |
| **SI-10** | Input validation/sanitization | Control-char strip + secret redaction on structured payloads via `sanitizeStructuredText` |
| **SI-17** | Fail-safe on error | Graceful degradation if RAR GET fails; oversized payloads rejected cleanly; nil-safe helper |
| **SC-7** | Boundary protection | Secrets redacted before crossing AF→client boundary |
| **AC-6** | Least privilege / human-in-the-loop | All evidence/alternatives surfaced; human approval decision preserved; no automated bypass |

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | RAR not yet created when AwaitingApproval fires | No approval card rendered | Medium | IT-AF-1398-001 | Graceful degradation: GET failure logged, text-only path continues |
| R2 | RAR payload exceeds 8KB (many evidence items) | Event rejected by EmitStructuredMeta | Low | UT-AF-1398-010 | Realistic payloads are 1-4KB; 8KB cap is generous |
| R3 | Race: decision fires before initial event emitted | Console sees resolved without request | Low | IT-AF-1398-003 | Edge case handling: emit both events in sequence |
| R4 | Watch test infrastructure lacks EventBridge wiring | ITs cannot capture events | High (confirmed) | IT-AF-1398-001..003 | Add WithEventBridge + fakeQueue to watch test context |
| R5 | Unstructured field path mismatch (spec vs status) | Silent nil extraction, empty JSON fields | Medium | UT-AF-1398-001, UT-AF-1398-008 | Validate against actual CRD type definition field paths |

### 3.1 Risk-to-Test Traceability

- **R1** (Medium): IT-AF-1398-001 (graceful degradation path)
- **R2** (Low): UT-AF-1398-010 (realistic payload size)
- **R3** (Low): IT-AF-1398-003 (both events in sequence)
- **R4** (High): IT-AF-1398-001..003 (all ITs use bridge)
- **R5** (Medium): UT-AF-1398-001, UT-AF-1398-008 (field extraction)

---

## 4. Scope

### 4.1 Features to be Tested

- **Approval payload marshaling** (`pkg/apifrontend/tools/approval_event.go`): Extract RAR spec/status fields from unstructured and produce console-contract JSON
- **Resolution payload marshaling** (same file): Extract decision/expiry fields for resolution event
- **EmitStructuredMetaSafe** (`pkg/apifrontend/launcher/event_bridge.go`): Nil-safe helper for structured emission from tool handlers
- **MetaType constants** (same file): `approval_request` and `approval_request_resolved` event classification
- **HandleWatch wiring** (`pkg/apifrontend/tools/crd_tools.go`): Emit structured events at AwaitingApproval detection and RAR decision change

### 4.2 Features Not to be Tested

- **Console rendering**: Separate repo (`kubernaut-demo-console`); covered by Console integration tests
- **MCP response path**: `kubernaut_approve` already tested in `UT-AF-109-*`, `kubernaut_approval_adversarial_test.go`
- **Auth webhook delegation**: Already tested in `pkg/authwebhook/` test suite
- **RAR CRD creation**: Owned by RO controller; tested in controller test suite

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Separate file `approval_event.go` | `crd_tools.go` is already large (~830 lines); keeps domain logic cohesive |
| `EmitStructuredMetaSafe` public helper | Consistent with existing `EmitStatusSafe`, `EmitOutputSafe` pattern |
| camelCase JSON (not snake_case) | Matches RAR CRD json tags directly; console team confirmed contract |
| Graceful degradation on GET failure | No regression; RAR may not exist yet due to controller timing |
| Both events on fast-decision edge case | Console always sees request before resolution (ordering guarantee) |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of unit-testable code (payload marshaling, nil-safety, constant resolution)
- **Integration**: >=80% of integration-testable code (HandleWatch emission wiring through production code path)
- **E2E**: >=80% of full service code exercised through Kind cluster (approval event journey over SSE)

### 5.2 Two-Tier Minimum

Every business requirement covered by UT + IT minimum. E2E provides journey assurance.

### 5.3 Business Outcome Quality Bar

Tests validate that the **Console receives machine-parseable structured approval data from the RAR CRD spec** — not just that functions are called. Payload assertions verify field presence, JSON validity, and contract compliance.

### 5.4 Pass/Fail Criteria

**PASS**:
1. All P0 tests pass (0 failures)
2. All P1 tests pass or have documented exceptions
3. Per-tier code coverage >=80%
4. No regressions in existing test suites (`UT-AF-106-*`, `UT-AF-109-*`)
5. Approval event JSON validates against console contract for all tested scenarios
6. Existing watch behavior (early return on AwaitingApproval) unchanged

**FAIL**:
1. Any P0 test fails
2. Per-tier coverage below 80%
3. Existing tests regress
4. Structured JSON event contains malformed/empty payload
5. HandleWatch return value changes (side-effect only, not return value)

### 5.5 Suspension & Resumption Criteria

**Suspend**: Build broken; RAR CRD type changes invalidate field paths
**Resume**: Build green; field paths re-validated against CRD types

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/apifrontend/tools/approval_event.go` (NEW) | `MarshalApprovalRequestPayload`, `MarshalApprovalResolvedPayload`, payload types | ~120 |
| `pkg/apifrontend/launcher/event_bridge.go` | `EmitStructuredMetaSafe` (NEW), `MetaTypeApprovalRequest`, `MetaTypeApprovalRequestResolved` | ~15 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/apifrontend/tools/crd_tools.go` | `HandleWatch` — AwaitingApproval emission + RAR channel emission | ~20 |

---

## 7. BR Coverage Matrix

| BR/Issue | Description | Priority | Tier | Test ID | Status |
|----------|-------------|----------|------|---------|--------|
| #1398 | Approval payload with all RAR spec fields | P0 | Unit | UT-AF-1398-001 | Pending |
| #1398 | Payload includes remediationRequestName | P0 | Unit | UT-AF-1398-002 | Pending |
| #1398 | Resolution payload with decision fields | P0 | Unit | UT-AF-1398-003 | Pending |
| #1398 | Resolution omits null workflowOverride | P1 | Unit | UT-AF-1398-004 | Pending |
| #1398 | EmitStructuredMetaSafe nil-safe | P0 | Unit | UT-AF-1398-005 | Pending |
| #1398 | EmitStructuredMetaSafe emits correctly | P0 | Unit | UT-AF-1398-006 | Pending |
| #1398 | MetaType constants correct values | P1 | Unit | UT-AF-1398-007 | Pending |
| #1398 | Optional fields gracefully handled | P1 | Unit | UT-AF-1398-008 | Pending |
| #1398 | Already-decided RAR produces both payloads | P0 | Unit | UT-AF-1398-009 | Pending |
| #1398 | Realistic payload within 8KB | P1 | Unit | UT-AF-1398-010 | Pending |
| #1398 | HandleWatch emits approval_request on AwaitingApproval | P0 | Integration | IT-AF-1398-001 | Pending |
| #1398 | HandleWatch emits approval_request_resolved on decision | P0 | Integration | IT-AF-1398-002 | Pending |
| #1398 | Edge case: both events on fast decision | P0 | Integration | IT-AF-1398-003 | Pending |
| #1398 | Full SSE delivery of approval_request event | P0 | E2E | E2E-AF-1398-001 | Pending |
| #1398 | MCP approve triggers resolution event | P0 | E2E | E2E-AF-1398-002 | Pending |
| #1398 | Expiration produces resolved with Expired | P1 | E2E | E2E-AF-1398-003 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{ISSUE}-{SEQUENCE}`
- **TIER**: `UT` (Unit), `IT` (Integration), `E2E` (End-to-End)
- **SERVICE**: `AF` (ApiFrontend)
- **ISSUE**: `1398`
- **SEQUENCE**: Zero-padded 3-digit

### Tier 1: Unit Tests

**Testable code scope**: Payload marshaling from unstructured, EventBridge nil-safe helper, constant resolution

| ID | Business Outcome Under Test | FedRAMP | Phase |
|----|----------------------------|---------|-------|
| `UT-AF-1398-001` | AU-3: Full RAR spec extraction produces valid JSON with all fields (confidence, reason, workflow, evidence, policy) | AU-3 | A |
| `UT-AF-1398-002` | AC-6: Payload includes `remediationRequestName` for breadcrumb context | AU-3, AC-6 | A |
| `UT-AF-1398-003` | AU-3: Resolution payload marshals decision, decidedBy, decidedAt, workflowOverride | AU-3 | A |
| `UT-AF-1398-004` | SI-10: Resolution payload omits workflowOverride when nil (no `null` in JSON) | SI-10 | A |
| `UT-AF-1398-005` | SI-17: `EmitStructuredMetaSafe` returns nil when no bridge in context (no panic) | SI-17 | B |
| `UT-AF-1398-006` | SI-4: `EmitStructuredMetaSafe` delegates to `EmitStructuredMeta` when bridge present | SI-4 | B |
| `UT-AF-1398-007` | SI-4: `MetaTypeApprovalRequest` = `"approval_request"` and `MetaTypeApprovalRequestResolved` = `"approval_request_resolved"` | SI-4 | B |
| `UT-AF-1398-008` | SI-17: Missing optional fields (policyEvaluation nil, evidenceCollected empty) produces valid JSON | SI-17 | A |
| `UT-AF-1398-009` | SI-17: RAR with existing decision returns request payload including decision fields | SI-17 | A |
| `UT-AF-1398-010` | SC-7: Realistic RAR payload (5 evidence items, 3 actions, 2 alternatives, policy) within 8KB | SC-7 | A |

### Tier 2: Integration Tests

**Testable code scope**: HandleWatch → EventBridge wiring through production code path

| ID | Business Outcome Under Test | FedRAMP | Phase |
|----|----------------------------|---------|-------|
| `IT-AF-1398-001` | AU-3, SI-4: HandleWatch on AwaitingApproval GETs RAR, emits structured `approval_request` event to A2A queue with full payload | AU-3, SI-4 | C |
| `IT-AF-1398-002` | AU-3, SI-4: HandleWatch RAR channel fires decision change — emits `approval_request_resolved` with decision + decidedBy | AU-3, SI-4 | C |
| `IT-AF-1398-003` | SI-17: RAR already decided at AwaitingApproval detection — both events emitted sequentially (ordering guarantee) | SI-17 | C |

### Tier 3: E2E Tests

**Testable code scope**: Full AF stack in Kind — watch → RAR lifecycle → SSE delivery

| ID | Business Outcome Under Test | FedRAMP | Phase |
|----|----------------------------|---------|-------|
| `E2E-AF-1398-001` | AU-3, AC-6: A2A SSE stream delivers `approval_request` event with full RAR spec fields on AwaitingApproval | AU-3, AC-6 | D |
| `E2E-AF-1398-002` | AU-3, SI-4: MCP `kubernaut_approve` call triggers `approval_request_resolved` event on SSE stream | AU-3, SI-4 | D |
| `E2E-AF-1398-003` | SI-17: RAR timeout produces `approval_request_resolved` with `decision: "Expired"` | SI-17 | D |

**Infrastructure**: Existing `test/e2e/apifrontend/` cluster (AF+mock-LLM+DEX), reusing `buildRAR`, `createRR`, `scanSSEFrames`, `fetchDEXTokenForPersona`

---

## 9. Test Cases

### UT-AF-1398-001: Full RAR spec extraction produces valid JSON

**Priority**: P0
**Type**: Unit
**File**: `pkg/apifrontend/tools/approval_event_test.go`

**Test Steps**:
1. **Given**: An unstructured RAR object with all spec fields populated (confidence, confidenceLevel, reason, whyApprovalRequired, recommendedWorkflow, investigationSummary, evidenceCollected, recommendedActions, alternativesConsidered, policyEvaluation, requiredBy)
2. **When**: `MarshalApprovalRequestPayload(rarObj)` is called
3. **Then**: Returns valid JSON string containing all fields with correct camelCase keys and values

**Expected Results**:
1. No error returned
2. JSON unmarshals into `ApprovalRequestEventPayload` successfully
3. All fields match the input RAR spec values
4. `name` and `namespace` populated from metadata

**Acceptance Criteria**:
- **Behavior**: All RAR spec fields projected into event payload
- **Correctness**: camelCase JSON keys match console contract
- **Accuracy**: No data loss or transformation errors

---

### UT-AF-1398-002: Payload includes remediationRequestName

**Priority**: P0
**Type**: Unit
**File**: `pkg/apifrontend/tools/approval_event_test.go`

**Test Steps**:
1. **Given**: An unstructured RAR with `spec.remediationRequestRef.name = "rr-gitops-drift-1"`
2. **When**: `MarshalApprovalRequestPayload(rarObj)` is called
3. **Then**: JSON contains `"remediationRequestName": "rr-gitops-drift-1"`

**Expected Results**:
1. `remediationRequestName` field present in output JSON
2. Value matches the nested ref name

---

### UT-AF-1398-003: Resolution payload with all decision fields

**Priority**: P0
**Type**: Unit
**File**: `pkg/apifrontend/tools/approval_event_test.go`

**Test Steps**:
1. **Given**: An unstructured RAR with `status.decision = "Approved"`, `status.decidedBy = "jane@acme.com"`, `status.decidedAt = "2026-06-11T15:50:00Z"`, `status.decisionMessage = "Reviewed"`, `status.workflowOverride = {workflowName, parameters, rationale}`
2. **When**: `MarshalApprovalResolvedPayload(rarObj)` is called
3. **Then**: Returns JSON with all decision fields + workflowOverride

**Expected Results**:
1. `decision`, `decidedBy`, `decidedAt`, `decisionMessage`, `workflowOverride` all present
2. `workflowOverride.parameters` is a JSON object (map)

---

### UT-AF-1398-005: EmitStructuredMetaSafe nil-safe

**Priority**: P0
**Type**: Unit
**File**: `pkg/apifrontend/launcher/emit_structured_meta_test.go`

**Test Steps**:
1. **Given**: A context without an EventBridge attached
2. **When**: `EmitStructuredMetaSafe(ctx, payload, meta)` is called
3. **Then**: Returns nil (no panic, no error)

**Expected Results**:
1. Function returns nil
2. No panic occurs
3. No events written anywhere

---

### IT-AF-1398-001: HandleWatch emits approval_request on AwaitingApproval

**Priority**: P0
**Type**: Integration
**File**: `pkg/apifrontend/tools/kubernaut_watch_test.go`

**Test Steps**:
1. **Given**: A fake dynamic client with an RR and a detailed RAR (`rar-{rrName}`), plus an EventBridge context with fakeQueue
2. **When**: RR watch fires phase change to `AwaitingApproval`
3. **Then**: The fakeQueue contains a `TaskStatusUpdateEvent` with `metadata.type = "approval_request"` and payload matching RAR spec

**Expected Results**:
1. Event queue has at least one structured event with `approval_request` type
2. Payload JSON contains all RAR spec fields (confidence, reason, evidence, etc.)
3. `HandleWatch` still returns `WatchResult{Status: "awaiting_approval"}` (return value unchanged)

**Preconditions**: EventBridge wired into context via `launcher.WithEventBridge`

---

### IT-AF-1398-002: HandleWatch emits approval_request_resolved on decision

**Priority**: P0
**Type**: Integration
**File**: `pkg/apifrontend/tools/kubernaut_watch_test.go`

**Test Steps**:
1. **Given**: A fake dynamic client with an RR (phase: Analyzing), RAR watch reactor, EventBridge context
2. **When**: RAR watch fires with `status.decision = "Approved"`, `status.decidedBy = "operator@acme.com"`
3. **Then**: The fakeQueue contains a `TaskStatusUpdateEvent` with `metadata.type = "approval_request_resolved"`

**Expected Results**:
1. Resolution event JSON contains `decision: "Approved"` and `decidedBy: "operator@acme.com"`
2. Existing text status message still emitted (no regression)

---

### E2E-AF-1398-001: Approval event delivered over SSE

**Priority**: P0
**Type**: E2E
**File**: `test/e2e/apifrontend/structured_approval_e2e_test.go`

**Test Steps**:
1. **Given**: Kind cluster with AF running, DEX token for `sre` persona
2. **When**: A2A invoke with keyword triggering `kubernaut_watch` on an RR that transitions to AwaitingApproval (with RAR created)
3. **Then**: SSE stream contains a `status-update` frame with `metadata.type = "approval_request"` and full payload

**Expected Results**:
1. HTTP 200 + `Content-Type: text/event-stream`
2. At least one SSE `data:` line parses to structured approval event
3. Payload contains confidence, reason, recommendedWorkflow, evidenceCollected

**Infrastructure**: Kind cluster, mock-LLM with `af_approval_request` scenario, DEX

---

## 10. Environmental Needs

### 10.1 Unit Tests

- Go 1.22+
- Ginkgo v2 / Gomega
- No external dependencies (pure logic testing)

### 10.2 Integration Tests

- Go 1.22+
- `k8s.io/client-go/dynamic/fake` for fake K8s client
- `k8s.io/apimachinery/pkg/watch` for fake watchers
- `a2a-go` event queue interface (fakeQueue)

### 10.3 E2E Tests

- Kind cluster (`apifrontend-e2e`)
- AF binary with coverage instrumentation
- Mock-LLM service with `af_approval_request` scenario
- DEX identity provider
- `kubernaut.ai/v1alpha1` CRDs installed

### 10.4 Tools & Versions

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.22+ | Compiler |
| Ginkgo | v2.x | Test runner |
| Kind | 0.20+ | Local K8s cluster |
| golangci-lint | 1.55+ | Linting |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Status | Impact if Missing |
|------------|--------|-------------------|
| `EmitStructuredMeta` (from #1395) | Merged (PR #1397) | Cannot emit structured events without truncation |
| RAR CRD type definition | Exists | Field paths for extraction |
| `newDetailedFakeRAR` test helper | Exists | Reuse in ITs |
| E2E infrastructure | Exists | Kind cluster, SSE helpers |

### 11.2 TDD Execution Order (Phased)

```
Phase A (RED → GREEN → REFACTOR): Payload Marshaling
  ├── UT-AF-1398-001..004, 008..010
  └── CHECKPOINT A

Phase B (RED → GREEN → REFACTOR): EventBridge Additions
  ├── UT-AF-1398-005..007
  └── CHECKPOINT B

Phase C (RED → GREEN → REFACTOR): HandleWatch Wiring
  ├── IT-AF-1398-001..003
  └── CHECKPOINT W (wiring verification)

Phase D (RED → GREEN → REFACTOR): E2E Journey
  ├── E2E-AF-1398-001..003
  └── CHECKPOINT FINAL (Pyramid Invariant)
```

---

## 12. Test Deliverables

| Deliverable | Location | Format |
|-------------|----------|--------|
| Test plan | `docs/tests/1398/TEST_PLAN.md` | IEEE 829 hybrid |
| Unit tests (marshaling) | `pkg/apifrontend/tools/approval_event_test.go` | Ginkgo/Gomega |
| Unit tests (bridge) | `pkg/apifrontend/launcher/emit_structured_meta_test.go` | Ginkgo/Gomega |
| Integration tests | `pkg/apifrontend/tools/kubernaut_watch_test.go` | Ginkgo/Gomega |
| E2E tests | `test/e2e/apifrontend/structured_approval_e2e_test.go` | Ginkgo/Gomega |
| Mock-LLM scenario | `deploy/apifrontend/overlays/e2e/mock-llm.yaml` | YAML |

---

## 13. Execution

```bash
# Unit tests (Phase A + B)
go test ./pkg/apifrontend/tools/... -run "UT-AF-1398" -v
go test ./pkg/apifrontend/launcher/... -run "UT-AF-1398" -v

# Integration tests (Phase C)
go test ./pkg/apifrontend/tools/... -run "IT-AF-1398" -v

# E2E tests (Phase D)
make test-e2e-apifrontend FOCUS="Structured Approval"

# Coverage
go test ./pkg/apifrontend/tools/... -coverprofile=coverage-tools.out
go test ./pkg/apifrontend/launcher/... -coverprofile=coverage-launcher.out

# Regression check
go test ./pkg/apifrontend/tools/... -run "UT-AF-106" -v
go test ./pkg/apifrontend/tools/... -run "UT-AF-109" -v
```

---

## 14. Go Anti-Pattern Validation

| # | Mistake | Applicable? | Validation |
|---|---------|-------------|------------|
| 4 | Overusing getters | Yes | Payload structs use public fields, not getters |
| 28 | Maps and memory leaks | Yes | No map accumulation in marshaling functions |
| 36 | Unnecessary type conversions | Yes | Direct type assertions from unstructured nested maps |
| 54 | Not using testing utility packages | Yes | Reuse `newDetailedFakeRAR`, `fakeQueue` |
| 60 | Not using table-driven tests | Yes | Payload field assertions use table-driven approach |
| 73 | Not using errgroup | No | No parallel goroutine orchestration needed |
| 78 | JSON marshaling considerations | Yes | Single `json.Marshal` per call; struct tags validated |
| 83 | Not using io.Reader/Writer properly | No | No streaming I/O in this feature |
| 89 | Not closing resources | Yes | No resources opened in marshaling (pure functions) |
| 97 | Not using context correctly | Yes | Context passed through for bridge extraction; no context.Background in production |

---

## 15. Checkpoint Protocol

At each checkpoint (A, B, W, FINAL), perform the following GA readiness audit:

1. **Build validation**: `go build ./...` — zero errors
2. **Test pass rate**: All tests in affected packages pass (100%)
3. **Lint compliance**: `golangci-lint run --timeout=5m` — zero new warnings on changed files
4. **Per-tier coverage**: >=80% on tier-specific code subset
5. **Regression guard**: Existing `UT-AF-106-*` and `UT-AF-109-*` watch/approval tests pass
6. **100-go-mistakes**: Validate against applicable patterns listed in §14
7. **Escalation gate**: Confidence >=95% to proceed; <95% escalate with actionable findings

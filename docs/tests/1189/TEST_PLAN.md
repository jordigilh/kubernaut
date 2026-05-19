# Test Plan: FP AF Integration — AF as Signal Source (Issue #1189)

**IEEE 829 Test Plan — Version 1.0**

| Field | Value |
|---|---|
| Test Plan ID | TP-AF-1189-001 |
| Issue | [#1189](https://github.com/jordigilh/kubernaut/issues/1189) |
| Service | ApiFrontend (AF), Mock-LLM (ML), Full Pipeline (FP) |
| Date | 2026-05-19 |
| Status | Draft |
| Author | AI Agent |
| Business Requirements | BR-API-042 (A2A Protocol), BR-API-043 (MCP Protocol), BR-INTEGRATION-001 (KA Communication), BR-AUDIT-005 (SOC2 AU-2 Compliance) |

---

## 1. Introduction

### 1.1 Purpose

This test plan defines the testing strategy for integrating the API Frontend (AF) into the Full Pipeline (FP) E2E cluster as an alternative signal source (replacing Gateway), covering MCP, A2A autonomous, and A2A interactive 4-phase remediation flows. It also addresses a mock-LLM multi-turn keyword matching defect, CorrelationID propagation fix, and non-happy path coverage for two new AF tools.

### 1.2 Scope

**In scope:**
- Mock-LLM multi-turn keyword matching fix (`LastUserContent` + `match_last_only`)
- AF `CorrelationID` propagation in `HandleCreateRR` audit events
- AF deployment into FP Kind cluster (image, DEX, CRDs, RBAC, TLS, Service)
- Three new FP E2E scenarios: MCP path, A2A autonomous, A2A interactive 4-phase
- Mock-LLM `af_manual` signal scenario for AF-created RRs
- AF E2E/IT non-happy path coverage for `kubernaut_stream_investigation` and `kubernaut_discover_workflows`
- Stale tool count comment fixes

**Out of scope:**
- Gateway-sourced FP scenarios (already covered)
- AF session persistence (separate feature)
- KA workflow parameter validation (tested in #1170)
- DataStorage backend changes

### 1.3 References

| Document | Path |
|---|---|
| Implementation plan | `.cursor/plans/fp_af_integration_a38dc307.plan.md` |
| AF architecture | `docs/services/apifrontend/design/ARCHITECTURE.md` |
| Mock-LLM registry | `test/services/mock-llm/scenarios/registry_default.go` |
| Mock-LLM keyword helpers | `test/services/mock-llm/scenarios/match_helpers.go` |
| FP infrastructure | `test/infrastructure/fullpipeline_e2e.go` |
| AF audit types | `pkg/apifrontend/audit/audit.go` |
| AF audit store adapter | `pkg/apifrontend/audit/store_adapter.go` |
| AF E2E infrastructure | `test/e2e/apifrontend/infrastructure/setup.go` |
| Existing FP lifecycle test | `test/e2e/fullpipeline/01_full_remediation_lifecycle_test.go` |
| ADK executor | `google.golang.org/adk@v1.2.0/server/adka2a/executor.go` |
| ADK runner | `google.golang.org/adk@v1.2.0/runner/runner.go` |

---

## 2. Test Items

### 2.1 Mock-LLM Multi-Turn Fix (Phase 0)

| Item | Component | Type |
|---|---|---|
| TI-01 | `test/services/mock-llm/scenarios/detection.go` | MODIFIED — `LastUserContent` field |
| TI-02 | `test/services/mock-llm/handlers/openai.go` | MODIFIED — populate `LastUserContent` |
| TI-03 | `test/services/mock-llm/scenarios/match_helpers.go` | MODIFIED — `lastUserKeywordScenario` helpers |
| TI-04 | `test/services/mock-llm/config/overrides.go` | MODIFIED — `MatchLastOnly` YAML field |
| TI-05 | `test/services/mock-llm/scenarios/registry_default.go` | MODIFIED — consume `MatchLastOnly` |

### 2.2 AF Pre-Requisite Fixes (Phase 1)

| Item | Component | Type |
|---|---|---|
| TI-06 | `pkg/apifrontend/tools/af_create_rr.go` | MODIFIED — CorrelationID on audit event |
| TI-07 | `deploy/apifrontend/overlays/e2e/mock-llm.yaml` | MODIFIED — new keyword scenarios |
| TI-08 | `pkg/apifrontend/handler/mcp_bridge_test.go` | MODIFIED — stale count comments |
| TI-09 | `test/e2e/apifrontend/a2a_test.go` | MODIFIED — stale counts + new tool tests |

### 2.3 FP Infrastructure (Phase 2)

| Item | Component | Type |
|---|---|---|
| TI-10 | `test/infrastructure/kind-fullpipeline-config.yaml` | MODIFIED — AF/DEX ports |
| TI-11 | `test/infrastructure/fullpipeline_e2e.go` | MODIFIED — AF image, deploy, suite wiring |
| TI-12 | `test/services/mock-llm/scenarios/registry_default.go` | MODIFIED — `af_manual` signal scenario |

### 2.4 FP E2E Scenarios (Phase 3)

| Item | Component | Type |
|---|---|---|
| TI-13 | `test/e2e/fullpipeline/af_helpers_test.go` | NEW — MCP/A2A helpers |
| TI-14 | `test/e2e/fullpipeline/02_af_mcp_remediation_test.go` | NEW — MCP path |
| TI-15 | `test/e2e/fullpipeline/03_af_a2a_autonomous_test.go` | NEW — A2A autonomous |
| TI-16 | `test/e2e/fullpipeline/04_af_a2a_interactive_test.go` | NEW — A2A 4-phase |

---

## 3. Features to Be Tested

### 3.1 Mock-LLM Multi-Turn Keyword Matching

| Feature | Acceptance Criteria |
|---|---|
| F-01: LastUserContent extraction | `DetectionContext.LastUserContent` contains only the last `role:"user"` message content |
| F-02: match_last_only keyword matching | When `MatchLastOnly: true`, keyword matcher uses `LastUserContent` instead of `Content + AllText` |
| F-03: Backward compatibility | When `MatchLastOnly: false` (default), behavior is identical to current implementation |
| F-04: Multi-turn determinism | In a 4-turn conversation, each turn matches the correct keyword scenario |
| F-05: YAML config support | `match_last_only: true` in keyword_scenario YAML is parsed and applied |

### 3.2 AF CorrelationID Propagation

| Feature | Acceptance Criteria |
|---|---|
| F-06: CorrelationID on rr.created | `HandleCreateRR` sets `Event.CorrelationID = res.RRID` before emitting |
| F-07: FP audit queryability | FP can query AF audit events by RR name as CorrelationID |

### 3.3 AF as FP Signal Source — MCP Path

| Feature | Acceptance Criteria |
|---|---|
| F-08: MCP tools/call creates RR | `af_create_rr` via MCP creates RR in `kubernaut-system` namespace |
| F-09: Pipeline completes | RR triggers RO -> SP -> KA -> EA lifecycle |
| F-10: Audit trail includes AF events | DS audit query by CorrelationID returns AF `rr.created` event |
| F-11: Audit total count | Total audit event count includes AF events (no undercount) |

### 3.4 AF as FP Signal Source — A2A Autonomous

| Feature | Acceptance Criteria |
|---|---|
| F-12: A2A autonomous RR creation | Single `message/send` triggers agent to call `af_create_rr` |
| F-13: Pipeline completes | Same as F-09 |
| F-14: Audit trail includes AF events | Same as F-10, plus `triage.started`, `triage.completed` |

### 3.5 AF as FP Signal Source — A2A Interactive 4-Phase

| Feature | Acceptance Criteria |
|---|---|
| F-15: Phase 1 — Investigation | User message triggers `kubernaut_stream_investigation` tool call |
| F-16: Phase 2 — Discovery | User message triggers `kubernaut_discover_workflows` tool call |
| F-17: Phase 3 — Selection | User message triggers `kubernaut_select_workflow` tool call |
| F-18: Phase 4 — RR Creation | User message triggers `af_create_rr` tool call |
| F-19: Pipeline completes | Same as F-09 |
| F-20: Audit trail completeness | All 4 tool events + pipeline events present in DS audit query |

### 3.6 Non-Happy Path Coverage

| Feature | Acceptance Criteria |
|---|---|
| F-21: RBAC denial on stream_investigation | User without `sre` group receives `auth.access_denied` |
| F-22: RBAC denial on discover_workflows | User without `sre` group receives `auth.access_denied` |
| F-23: Rate limiting on new tools | Rate-limited requests receive 429 response |
| F-24: Circuit breaker on new tools | When KA circuit is open, tools return circuit-open error |

---

## 4. Features Not Tested

| Feature | Reason |
|---|---|
| Gateway-sourced FP pipeline | Already fully tested in existing FP suite |
| AF session persistence | Separate feature not related to FP integration |
| KA parameter validation | Covered by #1170 test plan |
| DataStorage ingestion pipeline | Tested by DS's own test suite |
| AF MCP session lifecycle | Covered by existing AF E2E suite |

---

## 5. Approach

### 5.1 Test Pyramid

| Tier | Prefix | Target | Coverage Goal |
|---|---|---|---|
| Unit | UT-ML-1189 | Mock-LLM keyword matching fix, backward compat | >= 80% of changed match_helpers.go |
| Unit | UT-AF-1189 | CorrelationID propagation in HandleCreateRR | >= 80% of af_create_rr.go audit path |
| E2E | E2E-AF-1189 | A2A per-tool tests for stream_investigation, discover_workflows | Tool invocation + RBAC + audit |
| E2E | E2E-FP-1189 | Full pipeline with AF as signal source (3 scenarios) | Behavioral assurance on critical paths |

### 5.2 Testing Framework

- **Ginkgo/Gomega BDD** (mandatory per project rules)
- Table-driven tests for mock-LLM keyword matching (per Go mistake #85)
- `Eventually()` for async assertions (no `time.Sleep`, per Go mistake #86)
- Race flag enabled (`-race`) in CI (per Go mistake #83)
- No `XIt`/pending tests (per project TDD rules)

### 5.3 Mock Strategy

| Dependency | Mock? | Rationale |
|---|---|---|
| LLM (for AF agent) | YES | Mock-LLM with keyword_scenarios |
| LLM (for KA) | YES | Mock-LLM with signal scenarios |
| Kubernetes API | NO (E2E) | Real Kind cluster |
| DataStorage | NO (E2E) | Real DS for audit trail verification |
| DEX (OIDC) | NO (E2E) | Real DEX for JWT issuance |

---

## 6. Test Scenarios

### 6.1 Unit Tests — Mock-LLM Multi-Turn Fix (UT-ML-1189-001..010)

#### 6.1.1 LastUserContent Extraction (UT-ML-1189-001..003)

| Test ID | Description | Pass Criteria |
|---|---|---|
| UT-ML-1189-001 | Single user message populates LastUserContent | `LastUserContent` == user message text |
| UT-ML-1189-002 | Multi-turn: LastUserContent is last user message only | 3 user messages in history; `LastUserContent` == 3rd message |
| UT-ML-1189-003 | No user messages: LastUserContent is empty | System-only messages; `LastUserContent` == "" |

#### 6.1.2 match_last_only Keyword Matching (UT-ML-1189-004..008)

| Test ID | Description | Pass Criteria |
|---|---|---|
| UT-ML-1189-004 | match_last_only=true matches only last user message | Keyword in last message matches; keyword in prior message does not |
| UT-ML-1189-005 | match_last_only=false matches full conversation (backward compat) | Keyword in any prior message matches (existing behavior) |
| UT-ML-1189-006 | Multi-turn 4-phase: each turn matches correct scenario | 4 sequential detection contexts with accumulating history; each matches the expected keyword |
| UT-ML-1189-007 | match_last_only with empty LastUserContent | No match (returns false, 0) |
| UT-ML-1189-008 | YAML MatchLastOnly parsing | `match_last_only: true` in YAML correctly sets `MatchLastOnly` field |

#### 6.1.3 Registry Integration (UT-ML-1189-009..010)

| Test ID | Description | Pass Criteria |
|---|---|---|
| UT-ML-1189-009 | MatchLastOnly keyword scenario registered with lastUser matcher | Registry `Detect` with multi-turn context matches last-user keyword only |
| UT-ML-1189-010 | Mixed registry: some match_last_only=true, some false | Correct scenarios match based on their individual mode |

### 6.2 Unit Tests — AF CorrelationID Fix (UT-AF-1189-001..003)

| Test ID | Description | Pass Criteria |
|---|---|---|
| UT-AF-1189-001 | HandleCreateRR emits rr.created with CorrelationID set to RR name | Spy auditor captures event with `CorrelationID == res.RRID` |
| UT-AF-1189-002 | HandleCreateRR dedup emits rr.deduplicated with CorrelationID | Spy auditor captures event with `CorrelationID == existingRR.Name` |
| UT-AF-1189-003 | StoreAdapter maps CorrelationID to DS AuditEventRequest | Adapter output `CorrelationId` field matches input `Event.CorrelationID` |

### 6.3 E2E Tests — AF New Tool Coverage (E2E-AF-1189-001..008)

These extend the existing AF E2E suite (`test/e2e/apifrontend/`).

#### 6.3.1 A2A Happy Path (E2E-AF-1189-001..002)

| Test ID | Tool | Description | Pass Criteria |
|---|---|---|---|
| E2E-AF-1189-001 | `kubernaut_stream_investigation` | A2A message/send triggers streaming investigation via KA | Task completes with investigation result artifact |
| E2E-AF-1189-002 | `kubernaut_discover_workflows` | A2A message/send triggers workflow discovery via KA | Task completes with workflow list artifact |

#### 6.3.2 RBAC Denial (E2E-AF-1189-003..004)

| Test ID | Tool | Description | Pass Criteria |
|---|---|---|---|
| E2E-AF-1189-003 | `kubernaut_stream_investigation` | User without `sre` group triggers RBAC denial | `auth.access_denied` audit event emitted; task fails with access denied |
| E2E-AF-1189-004 | `kubernaut_discover_workflows` | User without `sre` group triggers RBAC denial | `auth.access_denied` audit event emitted; task fails with access denied |

#### 6.3.3 Rate Limiting (E2E-AF-1189-005..006)

| Test ID | Tool | Description | Pass Criteria |
|---|---|---|---|
| E2E-AF-1189-005 | `kubernaut_stream_investigation` | Exceed rate limit -> 429 | HTTP 429 returned after burst exceeds limit |
| E2E-AF-1189-006 | `kubernaut_discover_workflows` | Exceed rate limit -> 429 | HTTP 429 returned after burst exceeds limit |

#### 6.3.4 Circuit Breaker (E2E-AF-1189-007..008)

| Test ID | Tool | Description | Pass Criteria |
|---|---|---|---|
| E2E-AF-1189-007 | `kubernaut_stream_investigation` | KA circuit open -> tool returns circuit-open error | Error message indicates circuit breaker open |
| E2E-AF-1189-008 | `kubernaut_discover_workflows` | KA circuit open -> tool returns circuit-open error | Error message indicates circuit breaker open |

### 6.4 E2E Tests — Full Pipeline with AF (E2E-FP-1189-001..003)

These are new tests in the FP E2E suite (`test/e2e/fullpipeline/`).

**Shared Prerequisites:**
- AF deployed in FP Kind cluster with DEX, TLS, RBAC
- Mock-LLM configured with AF `keyword_scenarios` (match_last_only: true) and `af_manual` signal scenario
- Valid JWT obtained from DEX for `sre` user

#### 6.4.1 MCP Path (E2E-FP-1189-001)

| Test ID | Description | Infrastructure |
|---|---|---|
| E2E-FP-1189-001 | AF creates RR via MCP `tools/call` -> full pipeline completes -> audit trail verified | FP Kind cluster + AF + DEX + Mock-LLM |

**Detailed Steps:**
1. Initialize MCP session with AF (POST `/mcp/` with `initialize` method)
2. Call `tools/call` with tool `af_create_rr` and args `{namespace: "kubernaut-system", kind: "Deployment", name: "memory-eater", description: "FP MCP test"}`
3. Verify RR created in `kubernaut-system` namespace
4. Wait for RR status progression: Pending -> InvestigationRequested -> Investigating -> ... -> Completed
5. Verify EffectivenessAssessment CR created
6. Query DS audit events by CorrelationID (RR name):
   - AF `rr.created` event present
   - RO, SP, KA, EA events present
   - Total event count >= expected minimum

**Pass Criteria:**
- RR reaches terminal status (Completed or Effective)
- >= 1 AF audit event with `event_type = "apifrontend.rr.created"`
- All backend service audit events present (RO, SP, KA, EA)

#### 6.4.2 A2A Autonomous (E2E-FP-1189-002)

| Test ID | Description | Infrastructure |
|---|---|---|
| E2E-FP-1189-002 | A2A `message/send` autonomously creates RR -> full pipeline completes -> audit trail verified | FP Kind cluster + AF + DEX + Mock-LLM |

**Detailed Steps:**
1. Send A2A `message/send` to AF: `{"message": {"role": "user", "parts": [{"kind": "text", "text": "create a remediation request for deployment memory-eater in kubernaut-system"}]}}`
2. Mock-LLM matches keyword "create a remediation request" -> returns `af_create_rr` tool call
3. ADK agent executes tool -> RR created
4. Verify A2A task reaches `completed` state
5. Wait for pipeline: RR status -> Completed
6. Verify EffectivenessAssessment
7. Query DS audit events by CorrelationID:
   - AF `triage.started` event present
   - AF `rr.created` event present
   - AF `triage.completed` event present
   - Backend events present

**Pass Criteria:**
- A2A task status == `completed`
- RR reaches terminal status
- >= 3 AF audit events (`triage.started`, `rr.created`, `triage.completed`)
- All backend service audit events present

#### 6.4.3 A2A Interactive 4-Phase (E2E-FP-1189-003)

| Test ID | Description | Infrastructure |
|---|---|---|
| E2E-FP-1189-003 | A2A 4-phase interactive journey -> full pipeline completes -> audit trail verified | FP Kind cluster + AF + DEX + Mock-LLM (match_last_only) |

**Detailed Steps:**

**Turn 1 — Investigation (Phase 1):**
1. Send A2A `message/send`: `"start investigation for deployment memory-eater in kubernaut-system"`
2. Mock-LLM matches keyword "start investigation" (last-user-only) -> returns `kubernaut_stream_investigation` tool call
3. ADK agent executes tool -> streams investigation from KA
4. Task status == `completed` with investigation result artifact

**Turn 2 — Discovery (Phase 2):**
5. Send A2A `message/send` (same taskId): `"discover available workflows"`
6. Mock-LLM matches keyword "discover available workflows" (last-user-only; ignores "start investigation" from Turn 1)
7. ADK agent executes `kubernaut_discover_workflows` -> returns workflow list
8. Task status == `completed` with workflow list artifact

**Turn 3 — Selection (Phase 3):**
9. Send A2A `message/send` (same taskId): `"select workflow oomkill-increase-memory"`
10. Mock-LLM matches keyword "select workflow" (last-user-only) -> returns `kubernaut_select_workflow` tool call
11. ADK agent executes tool -> workflow selected
12. Task status == `completed`

**Turn 4 — RR Creation (Phase 4):**
13. Send A2A `message/send` (same taskId): `"create a remediation request"`
14. Mock-LLM matches keyword "create a remediation request" (last-user-only) -> returns `af_create_rr` tool call
15. ADK agent executes tool -> RR created in `kubernaut-system`
16. Task status == `completed`

**Pipeline Verification:**
17. Wait for RR status -> Completed
18. Verify EffectivenessAssessment
19. Query DS audit events by CorrelationID:
    - AF `triage.started` (Turn 1)
    - AF tool events for all 4 tools
    - AF `rr.created` (Turn 4)
    - AF `triage.completed` (Turn 4)
    - Backend events (RO, SP, KA, EA)

**Pass Criteria:**
- All 4 A2A turns complete successfully (no keyword mis-routing)
- RR reaches terminal status
- >= 6 AF audit events across all turns
- All backend service audit events present
- **Critical**: Turn 2-4 must NOT re-trigger Turn 1's tool (validates mock-LLM `match_last_only` fix)

---

## 7. Pass/Fail Criteria

### 7.1 Per-Tier Pass Criteria

| Tier | Criterion |
|---|---|
| Unit (ML) | All UT-ML-1189-* pass; match_helpers.go backward compat verified |
| Unit (AF) | All UT-AF-1189-* pass; CorrelationID propagated correctly |
| E2E (AF) | All E2E-AF-1189-* pass; RBAC + rate limit + circuit breaker covered |
| E2E (FP) | All E2E-FP-1189-* pass; full pipeline completes for all 3 AF paths |

### 7.2 Overall Pass Criteria

- `go build ./...` succeeds with zero errors
- `go vet ./...` clean
- `golangci-lint run` no new warnings
- All existing FP tests still pass (no regression)
- All existing AF tests still pass (no regression)
- No `time.Sleep()` in test code
- No `XIt`/pending tests
- No assertion-free `It` blocks

---

## 8. Test Deliverables

| Deliverable | Path |
|---|---|
| Test plan (this document) | `docs/tests/1189/TEST_PLAN.md` |
| Unit tests — mock-LLM | `test/services/mock-llm/scenarios/match_helpers_test.go` (extended) |
| Unit tests — AF CorrelationID | `pkg/apifrontend/tools/af_create_rr_test.go` (extended) |
| E2E tests — AF new tools | `test/e2e/apifrontend/a2a_test.go` (extended) |
| E2E tests — FP MCP | `test/e2e/fullpipeline/02_af_mcp_remediation_test.go` |
| E2E tests — FP A2A autonomous | `test/e2e/fullpipeline/03_af_a2a_autonomous_test.go` |
| E2E tests — FP A2A interactive | `test/e2e/fullpipeline/04_af_a2a_interactive_test.go` |
| FP AF helpers | `test/e2e/fullpipeline/af_helpers_test.go` |

---

## 9. Test Schedule

| Phase | TDD Stage | Tests |
|---|---|---|
| Phase 0 (Mock-LLM fix) | RED -> GREEN | UT-ML-1189-001..010 |
| Phase 1a (CorrelationID) | RED -> GREEN | UT-AF-1189-001..003 |
| Phase 1d (AF E2E tools) | RED -> GREEN | E2E-AF-1189-001..002 |
| Phase 2 (FP infra) | Infrastructure | No test scenarios; build validation only |
| Phase 3 (FP scenarios) | RED -> GREEN | E2E-FP-1189-001..003 |
| Phase 4 (Non-happy paths) | RED -> GREEN | E2E-AF-1189-003..008 |
| Phase 5 (Validation) | VERIFY | Full regression: existing FP + AF suites |

---

## 10. Risks and Mitigations

| ID | Risk | Severity | Mitigation |
|----|------|----------|------------|
| R1 | Mock-LLM `match_last_only` changes break existing scenarios | HIGH | Default is `false`; all existing scenarios unchanged; unit tests verify backward compat |
| R2 | ADK agent reformulates user message before sending to LLM | MEDIUM | Verified: ADK `toGenAIContent` preserves `TextPart.Text` verbatim (parts.go line 281) |
| R3 | FP Kind cluster memory exceeded with AF + DEX | LOW | Estimated ~6.0GB within 7.2GB budget (1.2GB buffer) |
| R4 | A2A multi-turn same-task semantics differ from expectations | MEDIUM | ADK `Executor.Execute` manages task/session state per taskId; verified in executor.go |
| R5 | `af_manual` signal name mismatch in mock-LLM | MEDIUM | Register explicit `signalScenario` in `registry_default.go` with exact prefix match |
| R6 | Stale keyword from Turn N leaks into Turn N+1 detection | HIGH | Entire Phase 0 fix addresses this; UT-ML-1189-006 validates 4-turn isolation |
| R7 | RR namespace: AF creates in user-specified namespace, FP controllers watch `kubernaut-system` | HIGH | Keyword scenario args hardcode `namespace: "kubernaut-system"` |
| R8 | CorrelationID empty when FP queries audit | HIGH | Phase 1a fix sets `CorrelationID = res.RRID`; UT-AF-1189-001 validates |

---

## 11. Go Testing Anti-Patterns Avoided

| Anti-Pattern | Mitigation |
|---|---|
| `time.Sleep()` in tests (#86) | Use `Eventually()` with timeout/polling |
| Missing race detection (#83) | `-race` flag in CI |
| No table-driven tests (#85) | Table-driven for mock-LLM keyword matching |
| Pending/skipped tests | No `XIt`, `Skip()`, or `Pending()` |
| Assertion-free tests | Every `It` block has at least one `Expect()` |
| Test pollution | `BeforeEach` for fresh state |
| Hardcoded ports | FP port allocation from `DD-TEST-001` |

---

## 12. Dependency on Mock-LLM Fix

E2E-FP-1189-003 (A2A interactive 4-phase) **critically depends** on the Phase 0 mock-LLM fix. Without `match_last_only`, the 4-turn conversation will experience keyword collision:

**Without fix:**
- Turn 1: "start investigation" -> matches `af_start_investigation` (correct)
- Turn 2: "discover available workflows" -> detection context contains BOTH "start investigation" AND "discover available workflows"; first-registered scenario wins -> `af_start_investigation` wins again (WRONG)

**With fix (match_last_only: true):**
- Turn 1: "start investigation" -> `LastUserContent` = "start investigation" -> matches `af_start_investigation` (correct)
- Turn 2: "discover available workflows" -> `LastUserContent` = "discover available workflows" -> matches `af_discover_workflows` (correct)

This is validated by UT-ML-1189-006 before the FP test is executed.

---

## 13. Approvals

| Role | Name | Date | Status |
|---|---|---|---|
| Author | AI Agent | 2026-05-19 | Draft |
| Reviewer | | | Pending |
| Approver | | | Pending |

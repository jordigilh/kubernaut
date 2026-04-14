# Test Plan: Cursor-Based LLM Pagination for Workflow Discovery Tools

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-688-v2
**Feature**: Replace raw offset/limit with opaque cursor-based pagination in LLM-facing workflow discovery tools
**Version**: 2.0
**Created**: 2026-04-06
**Author**: Kubernaut AI Assistant
**Status**: Complete
**Branch**: `fix/684-vertex-ai-claude`

---

## 1. Introduction

> **IEEE 829 §3** — Purpose, objectives, and measurable success criteria for the test effort.

### 1.1 Purpose

The LLM prompt templates instruct the model to paginate workflow discovery results using
`offset`, but the tool schemas do not expose `offset`/`limit` and the Execute methods
ignore them. This plan validates a cursor-based pagination mechanism that aligns the tool
schemas, Execute methods, and prompt instructions, enabling functional LLM-driven
pagination without exposing raw numeric offsets or total counts.

### 1.2 Objectives

1. **Cursor encoding/decoding correctness**: `EncodeCursor` and `DecodeCursor` produce
   round-trippable opaque tokens; malformed input falls back to safe defaults.
2. **TransformPagination fidelity**: DS `PaginationMetadata` (totalCount, offset, limit,
   hasMore) is correctly converted to LLM-facing format (hasNext, nextCursor,
   hasPrevious, previousCursor) with conditional field omission per page position.
3. **Schema alignment**: Both `list_available_actions` and `list_workflows` schemas
   expose `page` and `cursor` properties; `offset` and `limit` are NOT exposed.
4. **Execute wiring**: Both tool Execute methods parse `page`/`cursor`, translate to
   DS `offset`/`limit` via `OptInt`, call DS, and transform the response.
5. **Prompt alignment**: Updated templates reference cursor-based pagination instructions
   instead of raw `offset`.
6. **Backward compatibility**: First call with no args produces the same DS query as
   today (offset=0, limit=10).

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/tools/custom/...` |
| Integration test pass rate | 100% | `go test ./test/integration/kubernautagent/tools/custom/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on `pkg/kubernautagent/tools/custom/tools.go` |
| Backward compatibility | 0 regressions | Existing tests pass without modification (or with documented updates) |
| Schema-prompt-code consistency | 3-surface alignment | All three surfaces (schema, prompt, code) agree on cursor semantics |

---

## 2. References

> **IEEE 829 §2** — Documents that govern or inform this test plan.

### 2.1 Authority (governing documents)

- [DD-WORKFLOW-016 v1.4]: Action-Type Workflow Catalog Indexing — cursor-based pagination amendment
- [DD-HAPI-017]: Three-Step Workflow Discovery Integration — cross-reference
- Issue #688: Conditional pagination stripping / cursor-based LLM pagination

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)

---

## 3. Risks & Mitigations

> **IEEE 829 §5** — Software risk issues that drive test design.

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | LLM sends malformed base64 cursor | Tool returns error instead of graceful fallback | Medium | UT-KA-688-101, UT-KA-688-102 | DecodeCursor returns safe defaults (offset=0, limit=10) on any decode failure |
| R2 | LLM sends cursor with tampered values (negative offset, huge limit) | DS receives illegal params | Low | UT-KA-688-103 | DS ParsePagination clamps: offset<0→0, limit<=0→10, limit>100→100. Agent-side pre-validation mirrors this. |
| R3 | Prompt-schema-code drift: one surface references offset while others use cursor | LLM confusion, tool call failures | High | UT-KA-688-201, UT-KA-688-202, prompt review | All three surfaces updated atomically in same PR. Tests validate schema properties. |
| R4 | StripPaginationIfComplete removal breaks existing integration tests | CI regression | Medium | UT-KA-688-001 through 003 (existing) | StripPaginationIfComplete preserved; TransformPagination calls it internally for single-page case |
| R5 | Base64 cursor exposes implementation details (offset/limit) | LLM may attempt to parse and manipulate cursor contents | Low | UT-KA-688-104 | Cursor treated as opaque; all validation on decode. Future: HMAC signing if needed. |
| R6 | First call (no page/cursor) breaks existing behavior | DS receives unexpected params | Low | UT-KA-688-301, UT-KA-688-302 | When no page/cursor, OptInt fields left unset → DS defaults to offset=0, limit=10 |

### 3.1 Risk-to-Test Traceability

- **R1** (malformed cursor): UT-KA-688-101 (invalid base64), UT-KA-688-102 (non-JSON base64)
- **R2** (tampered values): UT-KA-688-103 (negative offset, huge limit in cursor)
- **R3** (drift): UT-KA-688-201, UT-KA-688-202 (schema assertions)
- **R4** (regression): UT-KA-688-001 through UT-KA-688-003 (preserved existing tests)
- **R5** (opaque cursor): UT-KA-688-104 (cursor is base64-URL string, not raw JSON)
- **R6** (first call): UT-KA-688-301, UT-KA-688-302 (Execute with empty args)

---

## 4. Scope

> **IEEE 829 §6/§7** — Features to be tested and features not to be tested.

### 4.1 Features to be Tested

- **Cursor encoding/decoding** (`pkg/kubernautagent/tools/custom/tools.go`): `EncodeCursor(offset, limit)` and `DecodeCursor(token)` — round-trip correctness, malformed input handling
- **TransformPagination** (`pkg/kubernautagent/tools/custom/tools.go`): Converts DS pagination metadata to LLM-facing cursor format with conditional field omission
- **Tool schemas** (`pkg/kubernautagent/tools/custom/tools.go`): `listAvailableActionsSchema` and `listWorkflowsSchemaJSON` include `page`/`cursor`, exclude `offset`/`limit`
- **Execute methods** (`pkg/kubernautagent/tools/custom/tools.go`): `listActionsTool.Execute` and `listWorkflowsTool.Execute` parse page/cursor and wire to DS

### 4.2 Features Not to be Tested

- **DataStorage server-side pagination** (`pkg/datastorage/server/workflow_handlers.go`): Already covered by existing DS tests. ParsePagination clamping is assumed correct.
- **Ogen client/server codegen** (`pkg/datastorage/ogen-client/`): Generated code, tested upstream.
- **Anomaly detector tool call limits** (`internal/kubernautagent/investigator/anomaly.go`): Unchanged; pagination adds tool calls within existing limits.
- **Golden transcript replay**: Will need separate update if transcripts include pagination; deferred to post-merge validation.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Base64-URL encoding for cursor, not HMAC-signed | DS clamps all values server-side; cursor tampering has no security impact beyond what the LLM can already do by calling the tool directly. Simplicity over complexity. |
| TransformPagination wraps StripPaginationIfComplete | Preserves existing single-page stripping behavior; avoids breaking existing tests. TransformPagination only adds cursor logic for multi-page responses. |
| `page` as enum ("next", "previous"), not free-form | Constrains LLM to valid navigation directions; prevents offset guessing. |
| `totalCount` never exposed to LLM | Per user requirement: prevents LLM from calculating total pages and over-paginating. |
| hasPrevious omitted on first page, hasNext omitted on last page | Per user requirement: prevents LLM from seeing navigation options that don't apply. |

---

## 5. Approach

> **IEEE 829 §8/§9/§10** — Test strategy, pass/fail criteria, and suspension/resumption.

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code in `pkg/kubernautagent/tools/custom/tools.go` (EncodeCursor, DecodeCursor, TransformPagination, schema accessors, Execute args parsing)
- **Integration**: New ITs (IT-KA-688-401, IT-KA-688-402) validate that cursor tokens flow through the real DS HTTP wire, producing correct multi-page and single-page responses. Existing ITs (IT-KA-433-033 through 035) retained for backward compatibility.
- **E2E**: Deferred — cursor pagination does not change the DS wire protocol; existing E2E coverage via Kind cluster validates end-to-end tool execution.

### 5.2 Two-Tier Minimum

- **Unit tests**: Cover all cursor encoding, transformation, schema, and Execute wiring logic via mocked DS client
- **Integration tests**: Validate cursor-based pagination end-to-end through real DataStorage (PostgreSQL + DS server) with seeded workflow data

### 5.3 Business Outcome Quality Bar

Each test validates a specific business outcome: "the LLM can navigate workflow discovery results page by page using opaque cursor tokens, without seeing raw offsets, limits, or total counts."

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:

1. All P0 tests pass (0 failures)
2. All P1 tests pass or have documented exceptions approved by reviewer
3. Per-tier code coverage meets >=80% on unit-testable code in `tools.go`
4. No regressions in existing test suites (`custom_tools_test.go`, integration tests)
5. Both tool schemas contain `page` and `cursor`; neither contains `offset` or `limit`

**FAIL** — any of the following:

1. Any P0 test fails
2. Per-tier coverage falls below 80% on unit-testable functions
3. Existing passing tests regress
4. Prompt templates still reference `offset`

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:

- Build broken: code does not compile after schema or Execute changes
- DD-WORKFLOW-016 v1.4 not yet finalized (cursor format undecided)

**Resume testing when**:

- Build fixed and green on CI
- DD amendment approved

---

## 6. Test Items

> **IEEE 829 §4** — Software items to be tested, with version identification.

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/kubernautagent/tools/custom/tools.go` | `EncodeCursor`, `DecodeCursor` | ~25 |
| `pkg/kubernautagent/tools/custom/tools.go` | `TransformPagination` | ~40 |
| `pkg/kubernautagent/tools/custom/tools.go` | `listAvailableActionsSchema`, `listWorkflowsSchemaJSON` | ~15 |
| `pkg/kubernautagent/tools/custom/tools.go` | `listActionsTool.Execute`, `listWorkflowsTool.Execute` (args parsing portion) | ~30 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/kubernautagent/tools/custom/tools.go` | `listActionsTool.Execute`, `listWorkflowsTool.Execute` (full DS round-trip with cursor) | ~30 |
| `test/integration/kubernautagent/tools/custom/custom_tools_integration_test.go` | IT-KA-433-033 through 035 (existing) + IT-KA-688-401, IT-KA-688-402 (new) | ~130 |
| `test/integration/kubernautagent/tools/custom/suite_test.go` | SynchronizedBeforeSuite: seeding 3 workflows | ~30 |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `fix/684-vertex-ai-claude` HEAD | Active branch for #688 |
| DD-WORKFLOW-016 | v1.4 (complete) | Cursor pagination amendment |

---

## 7. BR Coverage Matrix

> Maps every business requirement to test scenarios across tiers.

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| DD-WORKFLOW-016 §cursor | Cursor encoding produces opaque base64 token | P0 | Unit | UT-KA-688-100 | Pass |
| DD-WORKFLOW-016 §cursor | Cursor decoding recovers offset/limit | P0 | Unit | UT-KA-688-101 | Pass |
| DD-WORKFLOW-016 §cursor | Invalid cursor falls back to defaults | P0 | Unit | UT-KA-688-102, UT-KA-688-103 | Pass |
| DD-WORKFLOW-016 §cursor | Cursor is opaque (base64-URL, not raw JSON) | P1 | Unit | UT-KA-688-104 | Pass |
| DD-WORKFLOW-016 §transform | Single-page response: pagination stripped | P0 | Unit | UT-KA-688-001 (existing) | Pass |
| DD-WORKFLOW-016 §transform | Multi-page first page: hasNext+nextCursor, no hasPrevious | P0 | Unit | UT-KA-688-110 | Pass |
| DD-WORKFLOW-016 §transform | Multi-page middle page: hasNext+hasPrevious+both cursors | P0 | Unit | UT-KA-688-111 | Pass |
| DD-WORKFLOW-016 §transform | Multi-page last page: hasPrevious+previousCursor, no hasNext | P0 | Unit | UT-KA-688-112 | Pass |
| DD-WORKFLOW-016 §transform | Workflow response transformation | P0 | Unit | UT-KA-688-002 (existing, updated) | Pass |
| DD-WORKFLOW-016 §schema | list_workflows exposes page/cursor, not offset/limit | P0 | Unit | UT-KA-688-201 | Pass |
| DD-WORKFLOW-016 §schema | list_available_actions exposes page/cursor, not offset/limit | P0 | Unit | UT-KA-688-202 | Pass |
| DD-WORKFLOW-016 §execute | list_workflows first call (no args): DS defaults | P0 | Unit | UT-KA-688-301 | Pass |
| DD-WORKFLOW-016 §execute | list_workflows page=next with cursor: DS receives decoded offset/limit | P0 | Unit | UT-KA-688-302 | Pass |
| DD-WORKFLOW-016 §execute | list_available_actions first call (no args): DS defaults | P0 | Unit | UT-KA-688-303 | Pass |
| DD-WORKFLOW-016 §execute | list_available_actions page=next with cursor: DS receives decoded offset/limit | P0 | Unit | UT-KA-688-304 | Pass |
| DD-WORKFLOW-016 §execute | Execute with invalid cursor: graceful fallback to first page | P1 | Unit | UT-KA-688-305 | Pass |
| DD-WORKFLOW-016 §cursor | Cursor pagination flows through real DS HTTP wire: forward/backward with limit=1 | P0 | Integration | IT-KA-688-401 | Written |
| DD-WORKFLOW-016 §cursor | Default call (no cursor) returns all matching workflows, pagination stripped | P0 | Integration | IT-KA-688-402 | Written |

### Status Legend

- **Pending**: Specification complete, implementation not started
- **RED**: Failing test written (TDD RED phase)
- **GREEN**: Minimal implementation passes (TDD GREEN phase)
- **REFACTORED**: Code cleaned up (TDD REFACTOR phase)
- **Written**: Test implemented, awaiting execution against real infrastructure
- **Pass**: Implemented and passing

---

## 8. Test Scenarios

> **IEEE 829 Test Design Specification** — How test cases are organized and grouped.

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{BR_NUMBER}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: `KA` (Kubernaut Agent)
- **BR_NUMBER**: `688`
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ... for existing; 100+ for cursor tests)

### Tier 1: Unit Tests

**Testable code scope**: `pkg/kubernautagent/tools/custom/tools.go` — EncodeCursor, DecodeCursor, TransformPagination, schema accessors, Execute args parsing. Target >=80% coverage.

#### Group A: Cursor Encoding/Decoding (UT-KA-688-100 through UT-KA-688-104)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-688-100` | EncodeCursor(10, 10) produces a valid base64 string that DecodeCursor round-trips to (10, 10) | Pass |
| `UT-KA-688-101` | DecodeCursor with invalid base64 returns (0, defaultLimit) — safe fallback | Pass |
| `UT-KA-688-102` | DecodeCursor with valid base64 but non-JSON content returns (0, defaultLimit) | Pass |
| `UT-KA-688-103` | DecodeCursor with tampered values: negative offset clamped to 0, limit>100 clamped to 100, limit<=0 set to default | Pass |
| `UT-KA-688-104` | EncodeCursor output is base64-URL encoded (no padding, URL-safe chars) | Pass |

#### Group B: TransformPagination (UT-KA-688-110 through UT-KA-688-115)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-688-110` | First page (offset=0, hasMore=true): response has hasNext=true + nextCursor, no hasPrevious/previousCursor | Pass |
| `UT-KA-688-111` | Middle page (offset=10, hasMore=true): response has hasNext + nextCursor AND hasPrevious + previousCursor | Pass |
| `UT-KA-688-112` | Last page (offset=20, hasMore=false): response has hasPrevious + previousCursor, no hasNext/nextCursor | Pass |
| `UT-KA-688-113` | Single page (offset=0, hasMore=false): pagination stripped entirely (matches existing UT-KA-688-001 behavior) | Pass |
| `UT-KA-688-114` | TransformPagination never exposes totalCount to LLM | Pass |
| `UT-KA-688-115` | TransformPagination preserves non-pagination fields (actionTypes, workflows, actionType) | Pass |

#### Group C: Schema Validation (UT-KA-688-201 through UT-KA-688-202)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-688-201` | list_workflows schema has `page` (enum: next, previous) and `cursor` properties; does NOT have `offset` or `limit` | Pass |
| `UT-KA-688-202` | list_available_actions schema has `page` (enum: next, previous) and `cursor` properties; does NOT have `offset` or `limit` | Pass |

#### Group D: Execute Wiring (UT-KA-688-301 through UT-KA-688-305)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-688-301` | list_workflows Execute with `{"action_type":"X"}` (no page/cursor): DS called with unset Offset/Limit; response transformed | Pass |
| `UT-KA-688-302` | list_workflows Execute with `{"action_type":"X","page":"next","cursor":"<valid>"}`: DS called with decoded offset/limit | Pass |
| `UT-KA-688-303` | list_available_actions Execute with `{}` (no page/cursor): DS called with unset Offset/Limit; response transformed | Pass |
| `UT-KA-688-304` | list_available_actions Execute with `{"page":"next","cursor":"<valid>"}`: DS called with decoded offset/limit | Pass |
| `UT-KA-688-305` | Execute with `{"action_type":"X","page":"next","cursor":"garbage"}`: falls back to first page (offset=0), no error returned | Pass |

### Tier 2: Integration Tests

**Testable code scope**: `test/integration/kubernautagent/tools/custom/custom_tools_integration_test.go` — validates cursor pagination end-to-end through real DataStorage (PostgreSQL + DS HTTP server) with seeded workflow data.

**Infrastructure**: Real PostgreSQL, Redis, DataStorage server, envtest for K8s auth (DD-AUTH-014). Two `IncreaseMemoryLimits` workflows seeded (`oomkill-increase-memory-v1`, `oom-recovery-aggressive-v1`) that match the hardcoded context filters (severity=critical, component=deployment, environment=production, priority=P0) via wildcard labels.

#### Group E: Integration — Cursor Pagination over Real DS (IT-KA-688-401 through IT-KA-688-402)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-KA-688-401` | list_workflows with cursor limit=1 paginates correctly: page 1 returns 1 workflow with hasNext, page 2 returns 1 workflow with hasPrevious, backward navigation returns to page 1 (idempotent) | Written |
| `IT-KA-688-402` | list_workflows without cursor returns all matching workflows in single page with pagination stripped | Written |

### Tier Skip Rationale

- **E2E**: Cursor pagination does not change the DS wire protocol. E2E validation deferred to post-merge smoke test.

---

## 9. Test Cases

> **IEEE 829 Test Case Specification** — Detailed specification for each test case.

### UT-KA-688-100: EncodeCursor round-trips correctly

**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/custom/custom_tools_test.go`

**Preconditions**:
- `EncodeCursor` and `DecodeCursor` functions exist in `pkg/kubernautagent/tools/custom/tools.go`

**Test Steps**:
1. **Given**: offset=10, limit=10
2. **When**: `token := EncodeCursor(10, 10)`; `gotOffset, gotLimit := DecodeCursor(token)`
3. **Then**: gotOffset == 10, gotLimit == 10

**Expected Results**:
1. Token is a non-empty string
2. Decoded values match input exactly

**Acceptance Criteria**:
- **Behavior**: Round-trip produces identical values
- **Correctness**: Both offset and limit preserved
- **Accuracy**: No data loss in encoding/decoding

---

### UT-KA-688-101: DecodeCursor handles invalid base64

**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/custom/custom_tools_test.go`

**Preconditions**:
- `DecodeCursor` function exists

**Test Steps**:
1. **Given**: cursor = "not-valid-base64!!!"
2. **When**: `offset, limit := DecodeCursor(cursor)`
3. **Then**: offset == 0, limit == 10

**Expected Results**:
1. No panic or error propagation
2. Safe defaults returned

---

### UT-KA-688-103: DecodeCursor clamps tampered values

**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/custom/custom_tools_test.go`

**Preconditions**:
- `DecodeCursor` function exists

**Test Steps** (table-driven):
1. **Given**: cursor encoding `{"o":-5,"l":10}` → offset clamped to 0
2. **Given**: cursor encoding `{"o":0,"l":500}` → limit clamped to 100
3. **Given**: cursor encoding `{"o":0,"l":0}` → limit set to 10 (default)
4. **Given**: cursor encoding `{"o":0,"l":-1}` → limit set to 10 (default)

**Expected Results**:
1. Negative offset → 0
2. Excessive limit → 100
3. Zero/negative limit → 10 (default)

---

### UT-KA-688-110: TransformPagination on first page with more results

**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/custom/custom_tools_test.go`

**Preconditions**:
- `TransformPagination` function exists
- Input is action-type list response with `pagination: {totalCount:16, offset:0, limit:10, hasMore:true}`

**Test Steps**:
1. **Given**: JSON with pagination `{totalCount:16, offset:0, limit:10, hasMore:true}`
2. **When**: `result := TransformPagination(data)`
3. **Then**: result has `pagination.hasNext == true`, `pagination.nextCursor` is non-empty string, no `hasPrevious` key, no `previousCursor` key, no `totalCount` key, no `offset` key, no `limit` key

**Expected Results**:
1. `hasNext: true` present
2. `nextCursor` is base64-encoded token for offset=10, limit=10
3. `hasPrevious` absent (first page)
4. `totalCount`, `offset`, `limit` absent

---

### UT-KA-688-111: TransformPagination on middle page

**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/custom/custom_tools_test.go`

**Preconditions**:
- Input pagination: `{totalCount:30, offset:10, limit:10, hasMore:true}`

**Test Steps**:
1. **Given**: JSON with offset=10, hasMore=true
2. **When**: `result := TransformPagination(data)`
3. **Then**: `hasNext: true` + `nextCursor` (offset=20, limit=10), `hasPrevious: true` + `previousCursor` (offset=0, limit=10)

**Expected Results**:
1. Both navigation directions present
2. Both cursors decode to correct offsets
3. No totalCount/offset/limit exposed

---

### UT-KA-688-112: TransformPagination on last page

**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/custom/custom_tools_test.go`

**Preconditions**:
- Input pagination: `{totalCount:25, offset:20, limit:10, hasMore:false}`

**Test Steps**:
1. **Given**: JSON with offset=20, hasMore=false
2. **When**: `result := TransformPagination(data)`
3. **Then**: `hasPrevious: true` + `previousCursor` (offset=10, limit=10), no `hasNext`, no `nextCursor`

**Expected Results**:
1. Only backward navigation present
2. No forward navigation (last page)
3. No totalCount/offset/limit

---

### UT-KA-688-113: TransformPagination on single page (offset=0, hasMore=false)

**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/custom/custom_tools_test.go`

**Preconditions**:
- Input pagination: `{totalCount:3, offset:0, limit:10, hasMore:false}`

**Test Steps**:
1. **Given**: JSON with offset=0, hasMore=false (single page)
2. **When**: `result := TransformPagination(data)`
3. **Then**: `pagination` key stripped entirely (same as existing UT-KA-688-001)

---

### UT-KA-688-114: TransformPagination never exposes totalCount

**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/custom/custom_tools_test.go`

**Test Steps** (table-driven across all page positions):
1. First page (hasMore=true) → no totalCount
2. Middle page → no totalCount
3. Last page → no totalCount
4. Single page → pagination stripped entirely

---

### UT-KA-688-201: list_workflows schema has page/cursor, not offset/limit

**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/custom/custom_tools_test.go`

**Test Steps**:
1. **Given**: `schema := custom.ListWorkflowsSchema()`
2. **When**: Parse schema JSON
3. **Then**: `properties` contains `page` with `enum: ["next", "previous"]`, `properties` contains `cursor` with `type: "string"`, `properties` does NOT contain `offset` or `limit`, `required` still contains `action_type`

**Note**: Replaces existing UT-KA-433-171 second `It` block.

---

### UT-KA-688-202: list_available_actions schema has page/cursor, not offset/limit

**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/custom/custom_tools_test.go`

**Test Steps**:
1. **Given**: `schema := custom.ListAvailableActionsSchema()`
2. **When**: Parse schema JSON
3. **Then**: `properties` contains `page` and `cursor`, `properties` does NOT contain `offset` or `limit`

---

### UT-KA-688-301: list_workflows Execute with no pagination args

**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/custom/custom_tools_test.go`

**Preconditions**:
- Mock DS client that captures ListWorkflowsByActionType params
- Mock returns response with `hasMore: false`

**Test Steps**:
1. **Given**: args = `{"action_type":"ScaleReplicas"}`
2. **When**: `result, err := tool.Execute(ctx, args)`
3. **Then**: DS called with Offset.Set==false, Limit.Set==false; response has no `pagination` key (single page stripped)

---

### UT-KA-688-302: list_workflows Execute with page=next and cursor

**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/custom/custom_tools_test.go`

**Preconditions**:
- Mock DS client
- cursor encodes offset=10, limit=10

**Test Steps**:
1. **Given**: args = `{"action_type":"ScaleReplicas","page":"next","cursor":"<encoded>"}`
2. **When**: `result, err := tool.Execute(ctx, args)`
3. **Then**: DS called with Offset=10, Limit=10; response transformed with cursor pagination

---

### UT-KA-688-305: Execute with invalid cursor falls back gracefully

**Priority**: P1
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/custom/custom_tools_test.go`

**Test Steps**:
1. **Given**: args = `{"action_type":"ScaleReplicas","page":"next","cursor":"garbage"}`
2. **When**: `result, err := tool.Execute(ctx, args)`
3. **Then**: err is nil; DS called with Offset=0, Limit=10 (fallback defaults)

---

### IT-KA-688-401: Cursor pagination forward/backward through real DS

**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/tools/custom/custom_tools_integration_test.go`

**Preconditions**:
- Real DataStorage server running with PostgreSQL and Redis
- Two `IncreaseMemoryLimits` workflows seeded: `oomkill-increase-memory-v1` and `oom-recovery-aggressive-v1`
- Both match hardcoded context filters via wildcard labels (severity=critical, component=\*, environment=production, priority=\*)

**Test Steps**:
1. **Given**: cursor encoding offset=0, limit=1
2. **When**: Execute `list_workflows` with `{"action_type":"IncreaseMemoryLimits","page":"next","cursor":"<cursor>"}` (Page 1)
3. **Then**: Response has exactly 1 workflow, `pagination.hasNext=true`, `pagination.nextCursor` present, no `hasPrevious`, no `totalCount`
4. **When**: Execute `list_workflows` with `{"page":"next","cursor":"<nextCursor>"}` (Page 2)
5. **Then**: Response has exactly 1 workflow, `pagination.hasPrevious=true`, `pagination.previousCursor` present, no `hasNext`, no `totalCount`
6. **When**: Execute `list_workflows` with `{"page":"previous","cursor":"<previousCursor>"}` (Back to Page 1)
7. **Then**: Response has exactly 1 workflow matching Page 1 result (idempotent navigation), `hasNext=true`, no `hasPrevious`

**Expected Results**:
1. Cursor tokens correctly encode offset/limit through real DS HTTP wire
2. DS returns correct subsets based on offset/limit query parameters
3. TransformPagination correctly transforms DS response to LLM-facing format
4. Backward navigation returns to the same first-page results

**Acceptance Criteria**:
- **Behavior**: Full forward/backward pagination cycle completes without errors
- **Correctness**: Each page returns exactly 1 workflow; navigation is idempotent
- **Security**: `totalCount` never exposed in any response

---

### IT-KA-688-402: Default call without cursor returns all workflows, pagination stripped

**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/tools/custom/custom_tools_integration_test.go`

**Preconditions**:
- Same seeded data as IT-KA-688-401 (2 matching workflows)

**Test Steps**:
1. **Given**: No `page` or `cursor` in args
2. **When**: Execute `list_workflows` with `{"action_type":"IncreaseMemoryLimits"}`
3. **Then**: Response contains >=2 workflows; `pagination` key is absent (single-page stripping by TransformPagination)

**Expected Results**:
1. Default DS limit=10 returns all 2 matching workflows in one page
2. TransformPagination strips pagination entirely for single-page results (offset=0, hasMore=false)

**Acceptance Criteria**:
- **Behavior**: Backward compatible with non-paginated calls
- **Correctness**: All matching workflows returned; no pagination metadata exposed

---

## 10. Environmental Needs

> **IEEE 829 §13** — Hardware, software, tools, and infrastructure required.

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: Mock DS client interface for Execute wiring tests (UT-KA-688-301 through 305)
- **Location**: `test/unit/kubernautagent/tools/custom/`
- **Resources**: Standard Go build environment

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Infrastructure**: Real PostgreSQL (containerized), Redis (containerized), DataStorage server (binary), envtest (K8s API for DD-AUTH-014)
- **Seeded Data**: 3 workflows — `oom-recovery-v1`, `oomkill-increase-memory-v1`, `oom-recovery-aggressive-v1` (all `IncreaseMemoryLimits`)
- **Fixture**: `test/fixtures/workflows/oom-recovery-aggressive/workflow-schema.yaml` (new, for IT-KA-688-401)
- **Location**: `test/integration/kubernautagent/tools/custom/`
- **Resources**: ~30s startup (PostgreSQL + Redis + DS + envtest), ~5s test execution

### 10.3 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.22+ | Build and test |
| Ginkgo CLI | v2.x | Test runner |

---

## 11. Dependencies & Schedule

> **IEEE 829 §12/§16** — Remaining tasks, blocking issues, and execution order.

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| DD-WORKFLOW-016 v1.4 | Design | Complete | Cursor format undecided | Update DD before TDD RED phase |

### 11.2 Execution Order (TDD Phases)

1. **Phase 0 (Due Diligence)**: Rigorous analysis of risks, gaps, security concerns ✅ COMPLETE
   - Identified prompt-tool mismatch, designed cursor-based pagination, formalized in DD-WORKFLOW-016 v1.4
2. **Phase 1 (TDD RED)**: Write all failing tests (Groups A, B, C, D) ✅ COMPLETE
   - 24 unit tests written across cursor encoding, TransformPagination, schema, and Execute wiring
   - **Checkpoint 1**: Adversarial + security audit of RED tests ✅ COMPLETE
3. **Phase 2 (TDD GREEN)**: Implement minimal code to pass all tests ✅ COMPLETE
   - EncodeCursor, DecodeCursor, TransformPagination implemented
   - Schema updates applied (page/cursor added, offset/limit removed)
   - Execute method wiring via WorkflowDiscoveryClient interface
   - All 55 unit tests passing (24 new + 31 existing)
   - **Checkpoint 2**: Adversarial + security audit of GREEN implementation ✅ COMPLETE
4. **Phase 3 (TDD REFACTOR)**: Code quality improvements ✅ COMPLETE
   - Refactored tool structs to accept `WorkflowDiscoveryClient` interface for mockability
   - 4 prompt templates updated (incident_investigation, phase3_workflow_selection, workflow_selection, investigation)
   - DD-HAPI-017 cross-reference updated to v1.4
   - 3-surface consistency audit (schemas, prompts, code) — all aligned
   - **Checkpoint 3**: Final adversarial + security audit, regression gate ✅ COMPLETE
5. **Phase 4 (Integration Tests)**: Cursor pagination over real DataStorage ✅ WRITTEN
   - IT-KA-688-401: Forward/backward pagination with limit=1 cursor
   - IT-KA-688-402: Default call without cursor — single-page stripping
   - New fixture: `oom-recovery-aggressive/workflow-schema.yaml`
   - Suite seeding updated: 3 workflows (2 matching hardcoded filters via wildcard labels)
   - Awaiting execution against real DS infrastructure

---

## 12. Test Deliverables

> **IEEE 829 §11** — What artifacts this test effort produces.

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/688/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `test/unit/kubernautagent/tools/custom/custom_tools_test.go` | Ginkgo BDD test files (24 tests) |
| Integration test suite | `test/integration/kubernautagent/tools/custom/custom_tools_integration_test.go` | Ginkgo BDD IT tests (2 new + 3 existing) |
| Test fixture | `test/fixtures/workflows/oom-recovery-aggressive/workflow-schema.yaml` | Workflow fixture for pagination seeding |
| Coverage report | CI artifact | Per-tier coverage percentages |

---

## 13. Execution

```bash
# Unit tests
go test ./test/unit/kubernautagent/tools/custom/... -ginkgo.v

# Specific test group
go test ./test/unit/kubernautagent/tools/custom/... -ginkgo.focus="UT-KA-688-1"

# Integration tests (requires Docker for PostgreSQL/Redis containers)
go test ./test/integration/kubernautagent/tools/custom/... -ginkgo.v -timeout=120s

# Specific IT test
go test ./test/integration/kubernautagent/tools/custom/... -ginkgo.focus="IT-KA-688-401" -timeout=120s

# Coverage (unit tier)
go test ./test/unit/kubernautagent/tools/custom/... -coverprofile=coverage.out -coverpkg=github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/custom
go tool cover -func=coverage.out
```

---

## 14. Existing Tests Requiring Updates

> When implementation changes behavior that existing tests assert on, document the
> required updates here to prevent surprises during TDD GREEN phase.

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| UT-KA-433-171 (second It block) | `offset` and `limit` NOT in list_workflows schema | Replace with: `page` and `cursor` ARE in schema; `offset`/`limit` still NOT in schema | Schema now includes cursor pagination properties |
| UT-KA-433-170 | list_available_actions schema is `{type:object, properties:{}}` | Update: properties now include `page` and `cursor` | Schema now includes cursor pagination properties |
| UT-KA-688-001 | StripPaginationIfComplete strips when hasMore=false | Keep as-is if StripPaginationIfComplete preserved; otherwise update to test TransformPagination | TransformPagination delegates to StripPaginationIfComplete for single-page case |
| UT-KA-688-002 | StripPaginationIfComplete works for workflow responses | Same as above | Same as above |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-06 | Initial test plan for cursor-based LLM pagination |
| 2.0 | 2026-04-06 | Added Tier 2 integration tests (IT-KA-688-401, IT-KA-688-402): cursor pagination over real DataStorage with seeded workflow data. Added `oom-recovery-aggressive` fixture. Updated scope, coverage policy, and environmental needs. Updated all phase statuses to reflect completion: all unit tests passing, all TDD phases (RED/GREEN/REFACTOR) complete with checkpoints, IT tests written and awaiting execution. |

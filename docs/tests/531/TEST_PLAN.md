# Test Plan: Mock LLM Go Rewrite (#531)

> **Template**: IEEE 829-2008 + Kubernaut Hybrid v2.0

**Test Plan Identifier**: TP-531-v1.0
**Feature**: Rewrite the Mock LLM test infrastructure service from Python to Go, replacing hardcoded conversation logic with a DAG engine, monolithic scenario dict with a self-registering scenario registry, ConfigMap UUID sync with deterministic UUID generation, and adding an HTTP verification API for Go test assertions.
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Draft
**Branch**: `development/v1.3`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates that the Go rewrite of the Mock LLM service (#531) produces a drop-in replacement for the existing Python Mock LLM. The Go Mock LLM must be indistinguishable from a real LLM provider at the HTTP API level and must preserve identical behavior for all 15+ existing test scenarios consumed by KAPI (#433) and the integration/E2E test suites.

The test plan also validates the new architectural capabilities (DAG conversation engine, scenario registry, verification API, deterministic UUIDs) that make the Mock LLM extensible for future AIOps pillars (threat remediation, cost optimization).

### 1.2 Objectives

1. **Backward compatibility**: All 15 Python Mock LLM scenarios produce identical HTTP responses in the Go implementation — existing integration and E2E tests pass without modification (beyond image name change)
2. **DAG engine correctness**: Three conversation modes (legacy, three-step discovery, three-phase RCA) traverse the correct DAG paths and produce the correct response sequences
3. **Scenario detection accuracy**: Keyword, signal name, and proactive detection routes to the correct scenario with the correct priority resolution
4. **UUID consistency**: Deterministic UUIDs computed by Mock LLM match DataStorage-seeded UUIDs without any ConfigMap synchronization
5. **Verification API usability**: Go test helpers can assert on tool calls, DAG paths, and scenario detection via HTTP after each conversation
6. **Container contract preservation**: Go image runs on port 8080, UID 1001, responds to `/health` within 1 second, and is <50MB compressed

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/mockllm/...` |
| Integration test pass rate | 100% | `go test ./test/integration/mockllm/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on `internal/conversation/`, `internal/scenarios/`, `internal/response/`, `internal/config/`, `internal/fault/`, `internal/tracker/` (includes header recorder) |
| Integration-testable code coverage | >=80% | `go test -coverprofile` on `internal/handlers/`, `cmd/mock-llm/` |
| Scenario parity | 15/15 | All Python scenarios produce identical JSON response shapes in Go |
| Existing test regressions | 0 | Existing integration/E2E suites pass with Go Mock LLM image |
| Container image size | <50MB | `docker images` compressed size |
| Startup time | <1s | Time from `docker run` to `/health` returning 200 |

---

## 2. References

### 2.1 Authority (governing documents)

- BR-MOCK-001 through BR-MOCK-093: [Mock LLM Business Requirements](../../services/test-infrastructure/mock-llm/BUSINESS_REQUIREMENTS.md)
- DD-TEST-011 v4.0: [Mock LLM Configuration Pattern — Go rewrite + deterministic UUIDs](../../../architecture/decisions/DD-TEST-011-mock-llm-self-discovery-pattern.md)
- Issue #531: Rewrite Mock LLM service in Go (eliminate Python test dependency)
- Sub-issues: #560 (DAG), #561 (UUIDs), #562 (shared types), #563 (verification API), #564 (scenario registry), #565 (fault injection), #566 (YAML scenarios), #567 (pillar composition), #568 (metrics), #570 (auth headers)

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- DD-HAPI-017: Three-step workflow discovery protocol
- Issue #529: Three-phase RCA architecture
- Issue #548: Deterministic UUIDs in DataStorage
- Issue #417: Support custom authentication headers for LLM proxy endpoints (KAPI auth)
- Issue #433: KAPI Go rewrite (primary consumer of Mock LLM)
- Issue #570: Mock LLM auth header passthrough and verification

---

## 3. Risks & Mitigations

> Risks are placed before test design because they drive which tests are written and at what priority.

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | **#548 not landed** — shared `pkg/shared/uuid` unavailable | UT-MOCK-030/031 cannot import shared function; UUID tests blocked | Medium | UT-MOCK-030-001, UT-MOCK-030-002, UT-MOCK-031-001 | Stub UUID function locally in Mock LLM; replace with shared import when #548 lands. UT-MOCK-033 (YAML override) validates the fallback path. |
| R2 | **Python response shape drift** — Go responses don't match Python exactly | Existing E2E tests fail when Mock LLM image is swapped | High | UT-MOCK-026-*, IT-MOCK-026-* | Table-driven UT-MOCK-026 validates all 15 scenarios against Python fixture snapshots. IT-MOCK-026 validates over real HTTP. |
| R3 | **Strict routing breaks consumers** — `deploy/mock-llm/01-deployment.yaml` probes `/liveness`/`/readiness` which will 404 | Static deployments fail health checks | Certain | IT-MOCK-005-001, IT-MOCK-005-002 | Update static manifests to use `/health` during #531 implementation. IT-MOCK-005 validates strict routing. |
| R4 | **Force-text mode untested in Python** — `MOCK_LLM_FORCE_TEXT` was never read by Python server | No baseline behavior to compare against; Go implementation defines new behavior | Medium | UT-MOCK-004-001, UT-MOCK-004-002, IT-MOCK-004-001 | Define contract from scratch in Go. No regression risk (feature didn't work before). |
| R5 | **Concurrent request safety** — DAG engine or tracker corrupted under concurrent load | Flaky test failures, data leakage between conversations | Medium | UT-MOCK-010-004, UT-MOCK-040-001 | UT-MOCK-010-004 tests concurrent DAG execution. Tracker uses `sync.Mutex` (UT-MOCK-040-001 validates thread safety). |
| R6 | **Keyword alias coverage gap** — `mock_not_reproducible` → `problem_resolved` is undocumented | Alias missed during migration, existing tests fail | Low | UT-MOCK-026-007 | Explicit test case for the alias in the scenario catalog table. |
| R7 | **Invalid UUIDs in low_confidence scenario** — `7cg3` not valid hex | Go UUID validation rejects Python-era test data | Low | UT-MOCK-026-005 | Preserve exact Python strings without format validation (per RISK-MOCK-004 in BR). |
| R8 | **Shared types divergence** — `pkg/shared/types/openai/` shape doesn't match KAPI's expectations | Compile-time errors when KAPI imports shared types | Medium | UT-MOCK-060-001, UT-MOCK-061-001 | Design shared types from OpenAI API spec (stable, well-documented), not KAPI-specific assumptions. Mock LLM is first consumer. |
| R9 | **Auth header recording unbounded memory** — high-traffic tests flood header store | OOM or slow verification API responses | Low | UT-MOCK-006-003 | Record only configured header names (not all headers). Reset between tests via `POST /api/test/reset`. |

### 3.1 Risk-to-Test Traceability

Every High or Certain-probability risk has explicit test coverage:

| Risk | Primary Tests | Secondary Tests |
|------|--------------|-----------------|
| R2 (response shape drift) | UT-MOCK-026-001..010 (all scenarios) | IT-MOCK-026-001..004 (HTTP validation) |
| R3 (strict routing breaks probes) | IT-MOCK-005-001, IT-MOCK-005-002 | E2E-MOCK-093-001 (container contract) |
| R4 (force-text ghost) | UT-MOCK-004-001, UT-MOCK-004-002 | IT-MOCK-004-001 |
| R5 (concurrency) | UT-MOCK-010-004, UT-MOCK-040-001 | — |

---

## 4. Scope

### 4.1 Features to be Tested

- **DAG conversation engine** (`internal/conversation/`): State machine executing conversation flows (legacy, three-step discovery, three-phase RCA) with recorded path traversal
- **Scenario registry** (`internal/scenarios/`): Self-registering scenarios with keyword/signal/proactive detection, priority resolution, and full catalog of 15+ scenarios migrated from Python
- **Response builders** (`internal/response/`): OpenAI and Ollama response construction using shared types, with deterministic fields
- **HTTP handlers** (`internal/handlers/`): OpenAI (`/v1/chat/completions`, `/chat/completions`), Ollama (`/api/chat`, `/api/generate`), model list (`/v1/models`, `/api/tags`), health (`/health`), verification API (`/api/test/*`), strict routing (404 for unknown paths)
- **Configuration** (`internal/config/`): Environment variable parsing, deterministic UUID generation, optional YAML overrides
- **Conversation context** (`internal/conversation/context.go`): Message parsing, tool result extraction, Phase 3 marker detection, resource/owner extraction
- **Verification API** (`internal/handlers/verification.go`): Tool call recording, conversation path query, scenario detection query, auth header recording, state reset
- **Auth header passthrough** (`internal/handlers/headers.go`): Record configurable auth headers per request for verification by KAPI integration tests (#417)
- **Fault injection** (`internal/fault/`): Runtime fault injection API, permanent error keyword, intermittent and timeout modes
- **Container contract**: Image size, startup time, port, UID, probe path

### 4.2 Features Not to be Tested

- **KAPI Go rewrite** (#433): Separate issue, separate test plan. Mock LLM tests validate the mock's behavior, not KAPI's consumption of it. KAPI's test suites will implicitly validate the Go Mock LLM when the image is swapped.
- **Declarative YAML scenarios** (#566): P2 enhancement — architecture accommodates it but implementation deferred. Tested when implemented.
- **Pillar composition** (#567): P2 enhancement — abstraction defined but no concrete security/cost scenarios. Tested when pillars are added.
- **Prometheus metrics** (#568): P2 enhancement — scaffolded but `/metrics` endpoint tested only when implemented.
- **Streaming responses**: Not supported in current Python implementation. Out of scope for v1.0 Go rewrite.
- **Full pipeline E2E** (signal → AA → KAPI → Mock LLM → RO → WE): Validated by existing `test/e2e/aianalysis/` and `test/e2e/fullpipeline/` suites which use Mock LLM as infrastructure. Those tests implicitly validate the Go Mock LLM when the image is swapped.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Unit tests target pure logic (DAG engine, detection, context extraction, response builders) | These components have no I/O and can be tested in isolation at sub-millisecond speed |
| Integration tests use `net/http/httptest` for real HTTP | Mock LLM is an HTTP server — integration tests validate the full request→response cycle over real HTTP |
| E2E tests validate container contract only | Image size, startup time, port, probes — tested via `docker build` + `docker run`. Full pipeline E2E deferred to KAPI/AA test suites which already use Mock LLM as infrastructure |
| Table-driven tests for scenario catalog (BR-MOCK-026) | 15+ scenarios share identical structure; table-driven approach prevents 15 redundant test files |
| Deterministic UUID tests import `pkg/shared/uuid` directly | Validates the shared function produces identical output in both Mock LLM and DataStorage contexts |
| No mocks of internal components in integration tests | All Mock LLM internal components are business logic under `internal/` — only external I/O (HTTP) is real |
| Consumer is KAPI (#433), not HAPI | v1.3 uses the Go-based KAPI as the LLM client. All test assertions validate KAPI compatibility, not Python HAPI |

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (pure logic: DAG engine, scenario detection, context extraction, response builders, config parsing, fault injection logic, UUID generation)
- **Integration**: >=80% of integration-testable code (I/O: HTTP handlers, full conversation flows, verification API, fault injection API, startup wiring)
- **E2E**: Container contract only (port, probes, image size, startup time)

### 5.2 Two-Tier Minimum

Every business requirement is covered by at least 2 test tiers:
- **Unit tests**: Validate algorithm correctness (DAG traversal, scenario detection priority, context extraction, response shapes)
- **Integration tests**: Validate HTTP behavior (correct endpoints, response serialization, conversation flow over HTTP, verification API access)

### 5.3 Business Outcome Quality Bar

Tests validate **business outcomes**:
1. **KAPI receives correct responses** — OpenAI/Ollama wire-compatible, deterministic, scenario-appropriate
2. **Tests can assert conversation behavior** — verification API reports tool calls, DAG paths, matched scenarios
3. **Existing tests don't break** — backward compatibility with all current integration and E2E test contracts
4. **New scenarios are easy to add** — registry pattern enables adding scenarios without modifying existing code

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:

1. All P0 tests pass (0 failures)
2. All P1 tests pass or have documented exceptions approved by reviewer
3. Per-tier code coverage meets >=80% threshold on each tier
4. No regressions in existing test suites that interact with Mock LLM
5. All 15 Python scenarios produce structurally identical JSON responses in Go (validated by UT-MOCK-026 table)
6. Container image size <50MB and `/health` responds within 1 second of startup

**FAIL** — any of the following:

1. Any P0 test fails
2. Per-tier coverage falls below 80% on any tier
3. Any existing integration or E2E test that was passing before the change now fails
4. Any Python scenario produces a structurally different response in Go

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:

- **#548 not merged**: Tests UT-MOCK-030/031 (deterministic UUID) are blocked. Other tests can proceed using stubbed UUIDs.
- **Build broken**: `go build ./cmd/mock-llm/...` fails — all tests blocked.
- **Cascading failures**: >5 integration tests fail for the same root cause (e.g., router misconfiguration) — stop and investigate the root cause before proceeding.
- **Python fixture snapshots missing**: UT-MOCK-026 table-driven tests require reference fixtures from the Python implementation. If not yet captured, snapshot extraction must happen first.

**Resume testing when**:

- #548 merged → unblock UUID tests, replace stubs with shared imports
- Build fixed → resume full test execution
- Root cause fixed → re-run the affected tier
- Fixtures captured → resume scenario catalog tests

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/conversation/dag.go` | `NewDAG`, `Execute`, `Traverse`, `RecordPath` | ~120 |
| `internal/conversation/context.go` | `NewContext`, `ExtractToolResults`, `CountToolResults`, `HasPhase3Markers`, `HasThreeStepTools`, `ExtractResource`, `ExtractRootOwner` | ~150 |
| `internal/conversation/conditions.go` | `HasPhase3Markers`, `HasToolResults`, `HasThreeStepTools`, `IsForceText`, `HasToolsInRequest` | ~80 |
| `internal/conversation/modes.go` | `LegacyDAG`, `ThreeStepDAG`, `ThreePhaseDAG` | ~100 |
| `internal/scenarios/registry.go` | `Register`, `Detect`, `Get`, `List` | ~60 |
| `internal/scenarios/detection.go` | `DetectionContext`, `HasKeyword`, `SignalNameMatches`, `IsProactive` | ~80 |
| `internal/scenarios/*.go` (15+ files) | `Name`, `Match`, `DAG`, `Metadata` per scenario | ~400 |
| `internal/response/openai.go` | `BuildToolCallResponse`, `BuildContentResponse`, `BuildErrorResponse` | ~100 |
| `internal/response/ollama.go` | `BuildOllamaChatResponse`, `BuildOllamaGenerateResponse` | ~60 |
| `internal/config/config.go` | `LoadFromEnv`, `LoadOverrides`, `ParseForceText` | ~50 |
| `internal/config/uuid.go` | (imports `pkg/shared/uuid`) `WorkflowUUID` | ~20 |
| `internal/fault/injector.go` | `Inject`, `Clear`, `ShouldFault`, `IncrementCounter` | ~60 |
| `internal/tracker/tracker.go` | `Record`, `GetCalls`, `GetByConversation`, `Reset` | ~50 |
| `internal/tracker/headers.go` | `RecordHeaders`, `GetHeaders`, `GetByName`, `Reset` | ~60 |
| **Total unit-testable** | | **~1390** |

### 6.2 Integration-Testable Code (I/O, wiring, HTTP)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/handlers/openai.go` | `HandleChatCompletions` | ~80 |
| `internal/handlers/ollama.go` | `HandleOllamaChat`, `HandleOllamaGenerate` | ~60 |
| `internal/handlers/models.go` | `HandleModels`, `HandleTags` | ~30 |
| `internal/handlers/health.go` | `HandleHealth` | ~15 |
| `internal/handlers/verification.go` | `HandleToolCalls`, `HandleConversations`, `HandleScenariosMatched`, `HandleReset` | ~80 |
| `internal/handlers/fault.go` | `HandleInjectFault`, `HandleClearFaults` | ~40 |
| `internal/handlers/headers.go` | `HandleGetHeaders` (with name filter) | ~30 |
| `internal/handlers/router.go` | `NewRouter` (strict routing, 404 for unknown) | ~40 |
| `cmd/mock-llm/main.go` | `main`, server startup, config loading | ~50 |
| **Total integration-testable** | | **~425** |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `development/v1.3` HEAD | Mock LLM Go implementation |
| Dependency: deterministic UUIDs | #548 (Open) | Required for UT-MOCK-030/031. Stubbed until merged. |
| Dependency: shared OpenAI types | #562 (Open) | Created as part of #531. First consumer is Mock LLM. |
| Dependency: KAPI Go rewrite | #433 (In progress) | Mock LLM is upstream of KAPI. Shared types (#562) bridge the two. |
| Reference: Python Mock LLM | `test/services/mock-llm/src/server.py` | Source of truth for response shapes and scenario behavior during migration |

---

## 7. BR Coverage Matrix

### Category 1: HTTP API Compatibility

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-MOCK-001 | OpenAI chat completions — response shape | P0 | Unit | UT-MOCK-001-001..005 | Pending |
| BR-MOCK-001 | OpenAI chat completions — over HTTP | P0 | Integration | IT-MOCK-001-001..003 | Pending |
| BR-MOCK-002 | Ollama API — response shape | P0 | Unit | UT-MOCK-002-001..002 | Pending |
| BR-MOCK-002 | Ollama API — over HTTP | P0 | Integration | IT-MOCK-002-001..003 | Pending |
| BR-MOCK-003 | Health endpoint | P0 | Integration | IT-MOCK-003-001 | Pending |
| BR-MOCK-004 | Force text response mode — logic | P0 | Unit | UT-MOCK-004-001..002 | Pending |
| BR-MOCK-004 | Force text response mode — over HTTP | P0 | Integration | IT-MOCK-004-001 | Pending |
| BR-MOCK-005 | Strict HTTP routing (404 for unknown) | P1 | Integration | IT-MOCK-005-001..002 | Pending |
| BR-MOCK-006 | Auth header passthrough — recording logic | P0 | Unit | UT-MOCK-006-001..003 | Pending |
| BR-MOCK-006 | Auth header passthrough — over HTTP | P0 | Integration | IT-MOCK-006-001..002 | Pending |
| BR-MOCK-007 | Auth header verification API — query and filter | P1 | Integration | IT-MOCK-007-001..003 | Pending |
| BR-MOCK-007 | Auth header Go test helper assertions | P1 | Integration | IT-MOCK-007-004 | Pending |

### Category 2: Conversation Engine

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-MOCK-010 | DAG engine traversal and path recording | P0 | Unit | UT-MOCK-010-001..004 | Pending |
| BR-MOCK-011 | Legacy conversation mode — logic | P0 | Unit | UT-MOCK-011-001..002 | Pending |
| BR-MOCK-011 | Legacy conversation — over HTTP | P0 | Integration | IT-MOCK-011-001 | Pending |
| BR-MOCK-012 | Three-step discovery mode — logic | P0 | Unit | UT-MOCK-012-001..004 | Pending |
| BR-MOCK-012 | Three-step discovery — over HTTP | P0 | Integration | IT-MOCK-012-001 | Pending |
| BR-MOCK-013 | Three-phase RCA mode — logic | P0 | Unit | UT-MOCK-013-001..003 | Pending |
| BR-MOCK-013 | Three-phase RCA — over HTTP | P0 | Integration | IT-MOCK-013-001 | Pending |
| BR-MOCK-014 | Conversation context tracking | P0 | Unit | UT-MOCK-014-001..005 | Pending |
| BR-MOCK-015 | DAG path recording | P1 | Unit | UT-MOCK-010-002 | Pending |
| BR-MOCK-015 | DAG path via verification API | P1 | Integration | IT-MOCK-041-001 | Pending |

### Category 3: Scenario Management

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-MOCK-020 | Scenario registry self-registration | P0 | Unit | UT-MOCK-020-001..003 | Pending |
| BR-MOCK-021 | Keyword-based detection | P0 | Unit | UT-MOCK-021-001..002 | Pending |
| BR-MOCK-022 | Signal name-based detection | P0 | Unit | UT-MOCK-022-001..002 | Pending |
| BR-MOCK-023 | Proactive detection | P0 | Unit | UT-MOCK-023-001..003 | Pending |
| BR-MOCK-024 | Default fallback | P0 | Unit | UT-MOCK-024-001 | Pending |
| BR-MOCK-025 | Priority resolution | P1 | Unit | UT-MOCK-025-001..002 | Pending |
| BR-MOCK-026 | Pre-defined scenario catalog (15+ scenarios) | P0 | Unit | UT-MOCK-026-001..010 | Pending |
| BR-MOCK-026 | Scenario catalog — over HTTP | P0 | Integration | IT-MOCK-026-001..004 | Pending |

### Category 4: Configuration & UUID Management

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-MOCK-030 | Deterministic UUID generation | P0 | Unit | UT-MOCK-030-001..002 | Pending |
| BR-MOCK-031 | Shared UUID function with DataStorage | P0 | Unit | UT-MOCK-031-001 | Pending |
| BR-MOCK-032 | Environment variable configuration — logic | P0 | Unit | UT-MOCK-032-001 | Pending |
| BR-MOCK-032 | Env var config — in HTTP server | P0 | Integration | IT-MOCK-032-001 | Pending |
| BR-MOCK-033 | Optional YAML scenario overrides | P2 | Unit | UT-MOCK-033-001..002 | Pending |

### Category 5: Verification & Assertion API

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-MOCK-040 | Tool call recording and query — logic | P1 | Unit | UT-MOCK-040-001..002 | Pending |
| BR-MOCK-040 | Tool call recording — over HTTP | P1 | Integration | IT-MOCK-040-001 | Pending |
| BR-MOCK-041 | Conversation path query | P1 | Integration | IT-MOCK-041-001 | Pending |
| BR-MOCK-042 | Scenario detection query | P1 | Integration | IT-MOCK-042-001 | Pending |
| BR-MOCK-043 | State reset — logic | P1 | Unit | UT-MOCK-043-001 | Pending |
| BR-MOCK-043 | State reset — over HTTP | P1 | Integration | IT-MOCK-043-001 | Pending |
| BR-MOCK-044 | Go test helper package | P1 | Integration | IT-MOCK-044-001 | Pending |

### Category 6: Fault Injection

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-MOCK-050 | Runtime fault injection API | P2 | Integration | IT-MOCK-050-001 | Pending |
| BR-MOCK-054 | Permanent error (mock_rca_permanent_error) — logic | P0 | Unit | UT-MOCK-054-001 | Pending |
| BR-MOCK-054 | Permanent error — over HTTP | P0 | Integration | IT-MOCK-054-001 | Pending |

### Category 7: Shared Types

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-MOCK-060 | Shared OpenAI types | P1 | Unit | UT-MOCK-060-001 | Pending |
| BR-MOCK-061 | Shared tool definition constants | P1 | Unit | UT-MOCK-061-001 | Pending |
| BR-MOCK-062 | Shared Ollama types | P1 | Unit | UT-MOCK-062-001 | Pending |

### Category 10: Deployment & Container

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-MOCK-090 | Minimal container image | P0 | E2E | E2E-MOCK-090-001 | Pending |
| BR-MOCK-091 | Image size under 50MB | P1 | E2E | E2E-MOCK-091-001 | Pending |
| BR-MOCK-092 | Sub-second startup time | P1 | E2E | E2E-MOCK-092-001 | Pending |
| BR-MOCK-093 | Same container contract | P0 | E2E | E2E-MOCK-093-001 | Pending |

### Status Legend

- **Pending**: Specification complete, implementation not started
- **RED**: Failing test written (TDD RED phase)
- **GREEN**: Minimal implementation passes (TDD GREEN phase)
- **REFACTORED**: Code cleaned up (TDD REFACTOR phase)
- **Pass**: Implemented and passing

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-MOCK-{BR_NUMBER}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration), `E2E` (End-to-End)
- **SERVICE**: `MOCK` (Mock LLM)
- **BR_NUMBER**: Business requirement number (e.g., 001, 010, 026)
- **SEQUENCE**: Zero-padded 3-digit

### Tier 1: Unit Tests

**Testable code scope**: `internal/conversation/`, `internal/scenarios/`, `internal/response/`, `internal/config/`, `internal/fault/`, `internal/tracker/`. Target: >=80%.

#### 8.1.1 HTTP API Response Construction

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-MOCK-001-001` | OpenAI response contains all required fields (id, object, created, model, choices, usage) so KAPI can deserialize it | Pending |
| `UT-MOCK-001-002` | Response `id` follows `chatcmpl-{8hex}` format and `created` is fixed (1701388800) for deterministic assertions | Pending |
| `UT-MOCK-001-003` | Tool call response has correct `finish_reason: tool_calls` and arguments as JSON string | Pending |
| `UT-MOCK-001-004` | Content response has correct `finish_reason: stop` with markdown-embedded JSON | Pending |
| `UT-MOCK-001-005` | Usage tokens match expected values per response type (tool_call: 500/50/550, phase1: 600/150/750, phase3: 900/200/1100) | Pending |
| `UT-MOCK-002-001` | Ollama chat response contains `done: true`, fixed `created_at`, and expected duration fields | Pending |
| `UT-MOCK-002-002` | Ollama generate response contains `done: true` with content from matched scenario | Pending |
| `UT-MOCK-004-001` | Force-text mode wraps tool call data in markdown code fences within `content` field | Pending |
| `UT-MOCK-004-002` | Force-text mode leaves non-tool responses unchanged | Pending |

#### 8.1.2 DAG Conversation Engine

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-MOCK-010-001` | DAG engine transitions between nodes based on condition evaluation, reaching the correct terminal node | Pending |
| `UT-MOCK-010-002` | DAG engine records full traversal path (nodes visited, transitions taken) for verification API | Pending |
| `UT-MOCK-010-003` | DAG engine returns error when no transition matches (prevents silent failure) | Pending |
| `UT-MOCK-010-004` | Multiple concurrent conversations do not leak state between each other | Pending |
| `UT-MOCK-011-001` | Legacy DAG detects `search_workflow_catalog` in tools and returns single tool call on first turn | Pending |
| `UT-MOCK-011-002` | Legacy DAG returns content response with RCA + workflow selection on second turn | Pending |
| `UT-MOCK-012-001` | Three-step DAG returns `list_available_actions` tool call on first turn (no prior tool results) | Pending |
| `UT-MOCK-012-002` | Three-step DAG returns `list_workflows` after first tool result | Pending |
| `UT-MOCK-012-003` | Three-step DAG returns `get_workflow` after second tool result | Pending |
| `UT-MOCK-012-004` | Three-step mode detected when request includes `list_available_actions` tool definition | Pending |
| `UT-MOCK-013-001` | Phase 1 response includes `root_cause_analysis`, `remediation_target`, `contributing_factors` structure | Pending |
| `UT-MOCK-013-002` | Phase 3 detected when messages contain all three markers (`## Enrichment Context`, `## Phase 1 Root Cause Analysis`, `**Root Owner**:`) | Pending |
| `UT-MOCK-013-003` | Phase 3 response includes `selected_workflow` with UUID, parameters, justification, and uses enrichment root owner | Pending |

#### 8.1.3 Conversation Context Extraction

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-MOCK-014-001` | Context correctly counts tool result messages from message history | Pending |
| `UT-MOCK-014-002` | Context identifies presence of three-step tool names in tool result content | Pending |
| `UT-MOCK-014-003` | Context identifies all three Phase 3 markers and returns true only when ALL are present | Pending |
| `UT-MOCK-014-004` | Context extracts resource name and namespace from structured message content | Pending |
| `UT-MOCK-014-005` | Context extracts root owner from HolmesGPT-prefixed JSON tool result (tolerates prefix before JSON) | Pending |

#### 8.1.4 Scenario Registry and Detection

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-MOCK-020-001` | Scenario registered via `init()` is discoverable by `registry.Get(name)` | Pending |
| `UT-MOCK-020-002` | `registry.Detect()` returns highest-confidence match from registered scenarios | Pending |
| `UT-MOCK-020-003` | `registry.List()` returns metadata for all registered scenarios (enables auto-documentation) | Pending |
| `UT-MOCK-021-001` | Keyword `mock_oomkilled` matches OOMKilled scenario case-insensitively (also matches `MOCK_OOMKILLED`) | Pending |
| `UT-MOCK-021-002` | Space variant `mock oomkilled` matches identically to underscore variant | Pending |
| `UT-MOCK-022-001` | Regex `signal name:\s*(\w+)` extracts signal from KAPI prompt content | Pending |
| `UT-MOCK-022-002` | Signal normalization maps `OOMKilled` → `oomkilled`, `CrashLoopBackOff` → `crashloop` | Pending |
| `UT-MOCK-023-001` | Proactive markers (`proactive mode`, `predicted` + `not yet occurred`) detected in content | Pending |
| `UT-MOCK-023-002` | Proactive + OOM signal routes to `oomkilled_proactive` scenario (not `oomkilled`) | Pending |
| `UT-MOCK-023-003` | `mock_predictive_no_action` keyword returns no-action response (no workflow selected) | Pending |
| `UT-MOCK-024-001` | When no keyword/signal/proactive matches, default scenario returns valid OOMKilled-style response | Pending |
| `UT-MOCK-025-001` | Keyword match (confidence 1.0) wins over signal name match (0.9) when both present | Pending |
| `UT-MOCK-025-002` | Detection reason (keyword/signal/proactive/fallback) recorded for verification API | Pending |

#### 8.1.5 Scenario Catalog (table-driven)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-MOCK-026-001` | OOMKilled: keyword `mock_oomkilled` → workflow `oom-recovery`, engine `job`, correct RCA | Pending |
| `UT-MOCK-026-002` | CrashLoop: keyword `mock_crashloop` → workflow `crashloop-config-fix`, engine `job` | Pending |
| `UT-MOCK-026-003` | NodeNotReady: keyword `mock_node_not_ready` → workflow `node-drain-reboot`, engine `ansible` | Pending |
| `UT-MOCK-026-004` | NoWorkflowFound: keyword → no workflow selected, `needs_human_review: true` | Pending |
| `UT-MOCK-026-005` | LowConfidence: keyword → workflow with low confidence score, `needs_human_review: true` | Pending |
| `UT-MOCK-026-006` | ProblemResolved: keyword → no workflow, problem resolved messaging | Pending |
| `UT-MOCK-026-007` | `mock_not_reproducible` alias maps to ProblemResolved scenario | Pending |
| `UT-MOCK-026-008` | MaxRetriesExhausted: keyword → appropriate failure response | Pending |
| `UT-MOCK-026-009` | RCAIncomplete: keyword → incomplete RCA response structure | Pending |
| `UT-MOCK-026-010` | PermanentError: keyword `mock_rca_permanent_error` → HTTP 500 error JSON | Pending |

#### 8.1.6 Configuration & UUIDs

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-MOCK-030-001` | `DeterministicUUID("oom-recovery", "production")` produces same UUID as DataStorage would seed | Pending |
| `UT-MOCK-030-002` | Different workflow name or environment produces different UUID (no collisions) | Pending |
| `UT-MOCK-031-001` | Shared `pkg/shared/uuid` function returns consistent results when called from Mock LLM context | Pending |
| `UT-MOCK-032-001` | `LoadFromEnv()` reads `MOCK_LLM_HOST`, `MOCK_LLM_PORT`, `MOCK_LLM_FORCE_TEXT`, `MOCK_LLM_LOG_LEVEL` with correct defaults | Pending |
| `UT-MOCK-033-001` | YAML override file merges on top of deterministic UUID defaults (override wins) | Pending |
| `UT-MOCK-033-002` | Missing YAML file when `MOCK_LLM_CONFIG_PATH` set → graceful fallback to deterministic defaults with log warning | Pending |

#### 8.1.7 Verification, Tracker, Fault Logic

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-MOCK-040-001` | Tool call tracker records name, arguments, call_id, timestamp, sequence in thread-safe manner | Pending |
| `UT-MOCK-040-002` | `GetCalls()` returns calls sorted by sequence number | Pending |
| `UT-MOCK-043-001` | `Reset()` clears tool calls, conversations, scenario matches, and injected faults | Pending |
| `UT-MOCK-054-001` | `mock_rca_permanent_error` in message content triggers error response before scenario routing | Pending |

#### 8.1.8 Shared Types

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-MOCK-060-001` | `ChatCompletionResponse` struct marshals to JSON matching OpenAI API specification | Pending |
| `UT-MOCK-061-001` | Tool name constants (`SearchWorkflowCatalog`, `ListAvailableActions`, etc.) match Python string values exactly | Pending |
| `UT-MOCK-062-001` | `OllamaChatResponse` struct marshals to JSON matching Ollama API specification | Pending |

#### 8.1.9 Auth Header Recording

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-MOCK-006-001` | Header recorder captures `Authorization`, `x-api-key`, `x-tenant-id` from request headers into thread-safe store | Pending |
| `UT-MOCK-006-002` | Header recorder respects configurable header names via `MOCK_LLM_RECORD_HEADERS` env var (custom names added to defaults) | Pending |
| `UT-MOCK-006-003` | Header recorder ignores headers not in the configured set (no unbounded memory growth) | Pending |

---

### Tier 2: Integration Tests

**Testable code scope**: `internal/handlers/`, `cmd/mock-llm/main.go`, full HTTP request→response cycle. Target: >=80%.

#### 8.2.1 HTTP Endpoints

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-MOCK-001-001` | `POST /v1/chat/completions` with valid OpenAI request returns 200 with deserializable `ChatCompletionResponse` | Pending |
| `IT-MOCK-001-002` | `POST /chat/completions` (no `/v1` prefix) returns identical response to `/v1` path | Pending |
| `IT-MOCK-001-003` | `GET /v1/models` returns model list with `mock-model` entry | Pending |
| `IT-MOCK-002-001` | `POST /api/chat` with Ollama request returns valid Ollama response with `done: true` | Pending |
| `IT-MOCK-002-002` | `POST /api/generate` with Ollama request returns valid Ollama response | Pending |
| `IT-MOCK-002-003` | `GET /api/tags` returns model list matching `/v1/models` content | Pending |
| `IT-MOCK-003-001` | `GET /health` returns `200 OK` with `{"status":"ok"}` | Pending |
| `IT-MOCK-004-001` | Server started with `MOCK_LLM_FORCE_TEXT=true` returns content (no tool_calls) for tool-eligible requests | Pending |
| `IT-MOCK-005-001` | `GET /unknown/path` returns `404 Not Found` with JSON error body | Pending |
| `IT-MOCK-005-002` | `POST /unknown/path` returns `404 Not Found` with JSON error body | Pending |

#### 8.2.2 Full Conversation Flows Over HTTP

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-MOCK-011-001` | Legacy mode: 2-turn conversation (tool call → content response) over HTTP matches Python behavior | Pending |
| `IT-MOCK-012-001` | Three-step discovery: 4-turn conversation (`list_available_actions` → `list_workflows` → `get_workflow` → content) over HTTP | Pending |
| `IT-MOCK-013-001` | Three-phase RCA: Phase 1 content response, then Phase 3 request with markers returns workflow selection over HTTP | Pending |

#### 8.2.3 Scenario Detection Over HTTP

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-MOCK-026-001` | OOMKilled keyword in request messages → response contains oom-recovery workflow UUID | Pending |
| `IT-MOCK-026-002` | CrashLoop keyword → response contains crashloop-config-fix workflow UUID | Pending |
| `IT-MOCK-026-003` | Signal name `OOMKilled` in prompt (no keyword) → OOMKilled scenario detected | Pending |
| `IT-MOCK-026-004` | Proactive markers + OOM → proactive OOM response with predictive copy | Pending |

#### 8.2.4 Verification API Over HTTP

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-MOCK-040-001` | After a chat completion request, `GET /api/test/tool-calls` returns the tool calls from that response | Pending |
| `IT-MOCK-041-001` | After a chat completion request, `GET /api/test/conversations` includes the DAG path traversed | Pending |
| `IT-MOCK-042-001` | After a chat completion request, `GET /api/test/scenarios/matched` shows detected scenario name and method | Pending |
| `IT-MOCK-043-001` | `POST /api/test/reset` clears tool calls, conversations, and matches; subsequent GETs return empty | Pending |
| `IT-MOCK-044-001` | Go helper `MockLLMClient.AssertToolCalled("search_workflow_catalog")` succeeds after legacy conversation | Pending |

#### 8.2.5 Fault Injection Over HTTP

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-MOCK-050-001` | `POST /api/test/inject-fault` with `rate_limit` → next chat completion returns HTTP 429 | Pending |
| `IT-MOCK-054-001` | `mock_rca_permanent_error` in messages → HTTP 500 with OpenAI error JSON | Pending |
| `IT-MOCK-054-002` | Intermittent fault (count=2) → first 2 requests return 500, third succeeds | Pending |

#### 8.2.6 Auth Header Passthrough and Verification

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-MOCK-006-001` | `POST /v1/chat/completions` with `Authorization: Bearer test-token` succeeds and header is recorded | Pending |
| `IT-MOCK-006-002` | `POST /v1/chat/completions` without auth headers succeeds identically (backward compat) | Pending |
| `IT-MOCK-007-001` | `GET /api/test/headers` returns recorded auth headers with request sequence and conversation ID | Pending |
| `IT-MOCK-007-002` | `GET /api/test/headers?name=Authorization` filters to only Authorization header entries | Pending |
| `IT-MOCK-007-003` | `POST /api/test/reset` clears recorded headers; subsequent `GET /api/test/headers` returns empty | Pending |
| `IT-MOCK-007-004` | Go helper `MockLLMClient.AssertHeaderReceived("Authorization", HavePrefix("Bearer "))` succeeds after request with Bearer token | Pending |

#### 8.2.7 Backward Compatibility

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-MOCK-032-001` | Server listens on configured port with expected env vars (`MOCK_LLM_HOST`, `MOCK_LLM_PORT`) | Pending |

---

### Tier 3: E2E Tests

**Testable code scope**: Container build, image properties, deployment contract.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `E2E-MOCK-090-001` | Docker image builds from multi-stage Dockerfile with distroless/scratch runtime | Pending |
| `E2E-MOCK-091-001` | Built image compressed size is under 50MB | Pending |
| `E2E-MOCK-092-001` | Container `/health` endpoint returns 200 within 1 second of `docker run` | Pending |
| `E2E-MOCK-093-001` | Container runs as non-root (UID 1001), listens on port 8080, responds to `/v1/chat/completions` | Pending |

### Tier Skip Rationale

- **E2E (full pipeline)**: Deferred. The full pipeline (signal → AA → KAPI → Mock LLM → RO → WE) is validated by existing `test/e2e/aianalysis/` and `test/e2e/fullpipeline/` suites which use Mock LLM as infrastructure. Those tests will implicitly validate the Go Mock LLM when the image is swapped.
- **Category 8 (Extensibility)**: BR-MOCK-070 through 073 are P2 framework-level abstractions. Architecture tests are covered implicitly by the scenario registry unit tests. Explicit extensibility tests added when concrete security/cost scenarios exist.
- **Category 9 (Observability)**: BR-MOCK-080 through 083 are P2 metrics. Scaffolding tested when `/metrics` endpoint is implemented (#568).

---

## 9. Test Cases

> Detailed specifications for P0 tests. For the complete Test Case Specification format,
> see [TEST_CASE_SPECIFICATION_TEMPLATE.md](../../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md).

### UT-MOCK-010-001: DAG engine traversal

**BR**: BR-MOCK-010
**Priority**: P0
**Type**: Unit
**File**: `test/unit/mockllm/conversation/dag_test.go`

**Preconditions**:
- DAG definition with 3 nodes: `start` → `middle` → `end`
- Transition `start→middle` condition: tool results count == 0
- Transition `middle→end` condition: tool results count >= 1

**Test Steps**:
1. **Given**: A DAG with the above definition
2. **When**: DAG is executed with a context containing 0 tool results
3. **Then**: Engine lands on `middle` node and records path `[start, middle]`

**Expected Results**:
1. Terminal node is `middle` (not `end` — condition for `middle→end` is not met)
2. Recorded path is `["start", "middle"]`
3. No error returned (valid terminal state)

**Acceptance Criteria**:
- **Behavior**: DAG follows the first matching transition from each node
- **Correctness**: Terminal node is `middle` (condition for `middle→end` not met with 0 tool results)
- **Accuracy**: Recorded path matches actual traversal `["start", "middle"]`

**Dependencies**: None (foundational test)

---

### UT-MOCK-026-001: Scenario catalog — table-driven

**BR**: BR-MOCK-026
**Priority**: P0
**Type**: Unit (table-driven)
**File**: `test/unit/mockllm/scenarios/catalog_test.go`

**Preconditions**:
- All 15+ scenarios registered in the global registry via `init()`
- Python response fixtures captured for reference comparison

**Test Steps**:
1. **Given**: A table of `{keyword, expectedScenario, expectedWorkflow, expectedEngine}` entries for all 15+ scenarios
2. **When**: Each keyword is injected into a `DetectionContext` and `registry.Detect()` is called
3. **Then**: The correct scenario is matched with the expected workflow name and engine

**Expected Results**:
1. Every keyword maps to its correct scenario
2. Workflow names match Python: `oom-recovery`, `crashloop-config-fix`, `node-drain-reboot`, etc.
3. `execution_engine` matches Python: `job` for OOM/crashloop, `ansible` for node-drain
4. `mock_not_reproducible` alias resolves to `problem_resolved` scenario

**Test Data** (representative rows):

```go
entries := []struct {
    keyword          string
    expectedScenario string
    expectedWorkflow string
    expectedEngine   string
}{
    {"mock_oomkilled", "oomkilled", "oom-recovery", "job"},
    {"mock_crashloop", "crashloop", "crashloop-config-fix", "job"},
    {"mock_node_not_ready", "node_not_ready", "node-drain-reboot", "ansible"},
    {"mock_no_workflow_found", "no_workflow_found", "", ""},
    {"mock_low_confidence", "low_confidence", "oom-recovery", "job"},
    {"mock_problem_resolved", "problem_resolved", "", ""},
    {"mock_not_reproducible", "problem_resolved", "", ""},
    {"mock_max_retries_exhausted", "max_retries_exhausted", "oom-recovery", "job"},
    {"mock_rca_incomplete", "rca_incomplete", "", ""},
    {"mock_cert_not_ready", "cert_not_ready", "generic-restart", "job"},
    {"mock_test_signal", "test_signal", "generic-restart", "job"},
    {"mock_predictive_no_action", "predictive_no_action", "", ""},
}
```

**Acceptance Criteria**:
- **Behavior**: Every keyword in the Python Mock LLM maps to the correct scenario in Go
- **Correctness**: Workflow names and execution engines match Python exactly
- **Accuracy**: Aliases (`mock_not_reproducible` → `problem_resolved`) are preserved

**Dependencies**: UT-MOCK-020-001 (registry registration must work)

---

### UT-MOCK-030-001: Deterministic UUID matches DataStorage

**BR**: BR-MOCK-030, BR-MOCK-031
**Priority**: P0
**Type**: Unit
**File**: `test/unit/mockllm/config/uuid_test.go`

**Preconditions**:
- `pkg/shared/uuid` package available (or local stub if #548 not landed)

**Test Steps**:
1. **Given**: Workflow name `oom-recovery` and environment `production`
2. **When**: `pkg/shared/uuid.DeterministicUUID("oom-recovery", "production")` is called
3. **Then**: Returned UUID matches the UUID that DataStorage would produce for the same inputs

**Expected Results**:
1. UUID is a valid UUID v5 string
2. Same inputs always produce the same UUID
3. Different inputs (`oom-recovery:staging`) produce a different UUID

**Acceptance Criteria**:
- **Behavior**: Mock LLM and DataStorage independently produce identical UUIDs (no sync needed)
- **Correctness**: UUID is valid UUID v5 format
- **Accuracy**: Deterministic — same inputs → same output across invocations

**Dependencies**: Issue #548 (shared UUID function). If not landed, use local stub.

---

### UT-MOCK-014-003: Context identifies Phase 3 markers

**BR**: BR-MOCK-014, BR-MOCK-013
**Priority**: P0
**Type**: Unit
**File**: `test/unit/mockllm/conversation/context_test.go`

**Preconditions**:
- Message content fixtures with all three, two, one, and zero Phase 3 markers

**Test Steps**:
1. **Given**: Messages containing all three Phase 3 markers: `## Enrichment Context (Phase 2`, `## Phase 1 Root Cause Analysis`, `**Root Owner**:`
2. **When**: `context.HasPhase3Markers()` is evaluated
3. **Then**: Returns `true`

**Expected Results**:
1. All three markers present → `true`
2. Any single marker missing → `false` (3 sub-cases)
3. Zero markers → `false`

**Acceptance Criteria**:
- **Behavior**: Phase 3 detection requires ALL three markers (partial match returns false)
- **Correctness**: Missing any single marker → `false`
- **Accuracy**: Marker strings match Python implementation exactly (including spacing and formatting)

**Dependencies**: None

---

### IT-MOCK-013-001: Three-phase RCA conversation over HTTP

**BR**: BR-MOCK-013, BR-MOCK-010, BR-MOCK-001
**Priority**: P0
**Type**: Integration
**File**: `test/integration/mockllm/threephase_test.go`

**Preconditions**:
- Running Mock LLM `httptest.Server`
- OOMKilled scenario keyword in request messages

**Test Steps**:
1. **Given**: A running Mock LLM server
2. **When**: Client sends Phase 1 request (no Phase 3 markers) → receives RCA response → sends Phase 3 request (with enrichment markers and root owner)
3. **Then**: Phase 1 response contains `root_cause_analysis` structure; Phase 3 response contains `selected_workflow` with deterministic UUID

**Expected Results**:
1. Phase 1 response: has `root_cause_analysis`, does NOT have `selected_workflow`
2. Phase 3 response: has `selected_workflow` with UUID matching `DeterministicUUID("oom-recovery", "production")`, does NOT have `root_cause_analysis` at top level
3. Phase 3 `remediation_target` uses root owner from enrichment context

**Acceptance Criteria**:
- **Behavior**: Two-turn conversation produces correct phase-specific responses
- **Correctness**: Phase 1 does NOT contain `selected_workflow`; Phase 3 does NOT contain `root_cause_analysis` at top level
- **Accuracy**: Workflow UUID in Phase 3 matches deterministic computation

**Dependencies**: UT-MOCK-010-001 (DAG engine), UT-MOCK-013-001 (Phase 1 logic), UT-MOCK-030-001 (UUID)

---

### IT-MOCK-040-001: Verification API — tool call recording

**BR**: BR-MOCK-040, BR-MOCK-043
**Priority**: P1
**Type**: Integration
**File**: `test/integration/mockllm/verification_api_test.go`

**Preconditions**:
- Running Mock LLM `httptest.Server`

**Test Steps**:
1. **Given**: A running Mock LLM server
2. **When**: Client sends a chat completion request that triggers a `search_workflow_catalog` tool call → then calls `GET /api/test/tool-calls`
3. **Then**: Response contains the recorded tool call with name, arguments, call ID, and sequence

**Expected Results**:
1. Tool call entry has `name: "search_workflow_catalog"`, `arguments` as JSON, `call_id`, `timestamp`, `sequence`
2. After `POST /api/test/reset`, `GET /api/test/tool-calls` returns empty list

**Acceptance Criteria**:
- **Behavior**: Test runner can inspect what tool calls Mock LLM generated
- **Correctness**: Tool call fields match the response that was sent to the client
- **Accuracy**: Reset clears all state

**Dependencies**: IT-MOCK-011-001 (legacy conversation generates the tool call)

---

### IT-MOCK-054-001: Permanent error keyword

**BR**: BR-MOCK-054
**Priority**: P0
**Type**: Integration
**File**: `test/integration/mockllm/fault_test.go`

**Preconditions**:
- Running Mock LLM `httptest.Server`

**Test Steps**:
1. **Given**: A running Mock LLM server
2. **When**: Client sends `POST /v1/chat/completions` with `mock_rca_permanent_error` in message content
3. **Then**: Server returns HTTP 500 with OpenAI-compatible error JSON

**Expected Results**:
1. HTTP status code is 500
2. Response body has `error.message`, `error.type`, `error.code` fields
3. Space variant `mock rca permanent error` also triggers HTTP 500

**Acceptance Criteria**:
- **Behavior**: Error is returned BEFORE scenario routing (keyword checked first)
- **Correctness**: HTTP status code is exactly 500
- **Accuracy**: Response body is OpenAI-compatible error JSON

**Dependencies**: None (standalone)

---

### E2E-MOCK-093-001: Container contract

**BR**: BR-MOCK-090, BR-MOCK-091, BR-MOCK-092, BR-MOCK-093
**Priority**: P0
**Type**: E2E
**File**: `test/e2e/mockllm/container_contract_test.go`

**Preconditions**:
- Docker available on the test machine
- Multi-stage Dockerfile built via `docker build`

**Test Steps**:
1. **Given**: Docker image built from the Mock LLM Dockerfile
2. **When**: Container is started with `docker run -p 8080:8080`
3. **Then**: Image properties and runtime behavior match the contract

**Expected Results**:
1. Image size <50MB (`docker images`)
2. `/health` responds with 200 within 1 second of container start
3. Container runs as UID 1001 (`docker inspect`)
4. Port 8080 is exposed and accessible
5. `POST /v1/chat/completions` returns valid response

**Acceptance Criteria**:
- **Behavior**: Container is a drop-in replacement for the Python Mock LLM image
- **Correctness**: UID, port, probe path match the existing contract
- **Accuracy**: Image size and startup time meet performance targets

**Dependencies**: Working Dockerfile, `docker` CLI

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: None — all unit-testable code is pure logic with no I/O
- **Test data**: Scenario keywords, message content fixtures, DAG definitions (in-memory)
- **Location**: `test/unit/mockllm/`
  - `conversation/` — DAG engine, context, conditions
  - `scenarios/` — registry, detection, catalog
  - `response/` — response builders
  - `config/` — env vars, UUIDs, overrides
  - `fault/` — fault injection logic
  - `tracker/` — tool call tracker

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks — real HTTP via `net/http/httptest.NewServer`
- **Infrastructure**: `httptest.Server` wrapping the Mock LLM router (no containers needed)
- **Location**: `test/integration/mockllm/`
  - `endpoints_test.go` — HTTP endpoint tests
  - `conversation_test.go` — multi-turn conversation flows
  - `threephase_test.go` — three-phase RCA flow
  - `verification_api_test.go` — verification endpoints
  - `fault_test.go` — fault injection API
  - `backward_compat_test.go` — Python contract compatibility

### 10.3 E2E Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Infrastructure**: `docker build` + `docker run` (lightweight, no Kind cluster)
- **Location**: `test/e2e/mockllm/`
  - `container_contract_test.go` — image size, startup, port, user
- **Resources**: Docker daemon, ~100MB disk for build cache

### 10.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.22+ | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| Docker | 24.x+ | Container build and E2E tests |
| Python 3.12 | (existing) | Reference implementation for fixture snapshot extraction |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| #548 (Deterministic UUIDs) | Code | Open | UT-MOCK-030/031 blocked; all scenario UUID assertions use hardcoded values | Stub `DeterministicUUID()` locally; replace with shared import when merged |
| #562 (Shared OpenAI types) | Code | Open (part of #531) | UT-MOCK-060/061/062 blocked; response builders use local types | Define types in Mock LLM first, move to shared package when #562 is implemented |
| Python fixture snapshots | Test data | Not started | UT-MOCK-026 table-driven tests have no reference data | Extract snapshots from Python Mock LLM before starting Go implementation |

### 11.2 Execution Order

Tests should be implemented in this order, following TDD and dependency chains:

1. **Phase 1 — Core engine (Unit)**: UT-MOCK-010 (DAG engine), UT-MOCK-014 (context), UT-MOCK-020 (registry), UT-MOCK-032 (config)
2. **Phase 2 — Scenarios (Unit)**: UT-MOCK-021..025 (detection), UT-MOCK-026 (catalog), UT-MOCK-001..002 (response builders)
3. **Phase 3 — HTTP layer (Integration)**: IT-MOCK-001..005 (endpoints), IT-MOCK-011..013 (conversations), IT-MOCK-026 (scenarios over HTTP)
4. **Phase 4 — Verification & fault (Unit + Integration)**: UT-MOCK-040/043, IT-MOCK-040..044, UT-MOCK-054, IT-MOCK-054
5. **Phase 5 — UUIDs (Unit)**: UT-MOCK-030/031 (unblocked when #548 lands)
6. **Phase 6 — Container (E2E)**: E2E-MOCK-090..093 (after working Dockerfile)

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/531/TEST_PLAN.md` | Strategy, risk analysis, and test design |
| Python fixture snapshots | `test/fixtures/mockllm/` | JSON reference responses extracted from Python for scenario comparison |
| Unit test suite | `test/unit/mockllm/` | 56 Ginkgo BDD tests covering DAG, scenarios, responses, config, faults, headers |
| Integration test suite | `test/integration/mockllm/` | 33 Ginkgo BDD tests covering HTTP endpoints, conversations, verification API, auth headers |
| E2E test suite | `test/e2e/mockllm/` | 4 Ginkgo BDD tests covering container contract |
| Coverage report | CI artifact | Per-tier coverage percentages for unit-testable and integration-testable code |

---

## 13. Execution

```bash
# Unit tests
go test ./test/unit/mockllm/... -ginkgo.v

# Unit tests — specific scenario
go test ./test/unit/mockllm/... -ginkgo.focus="UT-MOCK-026"

# Integration tests
go test ./test/integration/mockllm/... -ginkgo.v

# Integration tests — specific conversation flow
go test ./test/integration/mockllm/... -ginkgo.focus="IT-MOCK-013-001"

# E2E tests (requires Docker)
go test ./test/e2e/mockllm/... -ginkgo.v

# All Mock LLM tests (excluding E2E)
go test ./test/unit/mockllm/... ./test/integration/mockllm/... -ginkgo.v

# Coverage — unit tier
go test ./test/unit/mockllm/... -coverprofile=unit_coverage.out
go tool cover -func=unit_coverage.out

# Coverage — integration tier
go test ./test/integration/mockllm/... -coverprofile=int_coverage.out
go tool cover -func=int_coverage.out
```

---

## 14. Test Count Summary

| Tier | Count | Coverage Target |
|------|-------|-----------------|
| Unit | 56 | >=80% of ~1390 lines (DAG, scenarios, response, config, fault, tracker, headers) |
| Integration | 33 | >=80% of ~425 lines (handlers, router, headers, startup wiring) |
| E2E | 4 | Container contract (image, startup, port, user) |
| **Total** | **93** | |

**BR Coverage**: All 37 P0+P1 BRs covered by >=2 tiers. 8 P2 BRs covered by >=1 tier (deferred tier 2 to implementation phase). Categories 8 (Extensibility) and 9 (Observability) deferred per scope exclusion (Section 4.2). Auth header BRs (BR-MOCK-006, BR-MOCK-007) added for #417 integration.

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan (IEEE 829 hybrid format). 84 tests across 3 tiers covering 47 BRs. Risk-first design with 8 risks and explicit traceability to mitigating tests. Pass/fail and suspension/resumption criteria defined. KAPI as primary consumer (not HAPI). Execution order aligned with TDD phasing and #548 dependency. |
| 1.1 | 2026-03-04 | Added BR-MOCK-006 (auth header passthrough) and BR-MOCK-007 (auth header verification API) for #417/#570 integration. +3 unit tests (UT-MOCK-006-001..003), +6 integration tests (IT-MOCK-006-001..002, IT-MOCK-007-001..004). Total: 93 tests, 49 BRs, 9 risks. |

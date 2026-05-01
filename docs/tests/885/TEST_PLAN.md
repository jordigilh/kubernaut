# Test Plan: Migrate kubernaut-agent from log/slog to pkg/log (logr.Logger)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-885-v1
**Feature**: Complete slog-to-logr migration for kubernaut-agent MCP and LLM layers
**Version**: 1.0
**Created**: 2026-05-01
**Author**: AI Assistant
**Status**: Active
**Branch**: `development/v1.5`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the migration of kubernaut-agent's remaining `log/slog`
usage to `logr.Logger` (backed by `pkg/log`/kubelog). The migration eliminates the
`slogBridge` shim in `main.go`, ensuring a single logging path through the zap-backed
logr.Logger for consistent structured logging, atomic level control, and hot-reload.

### 1.2 Objectives

1. **Zero slog imports**: All `"log/slog"` imports removed from agent scope (production + test)
2. **Signature migration**: All MCP constructors accept `logr.Logger` instead of `*slog.Logger`
3. **LLM logger injection**: LLM adapters accept optional `logr.Logger` via `WithLogger` option
4. **Bridge removal**: `slogBridge` removed from `cmd/kubernautagent/main.go`
5. **Behavioral equivalence**: All existing tests pass without modification to test logic

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/...` |
| Integration test pass rate | 100% | `go test ./test/integration/kubernautagent/...` |
| slog imports in agent scope | 0 | `grep -rn '"log/slog"' internal/kubernautagent/ pkg/kubernautagent/ cmd/kubernautagent/ test/unit/kubernautagent/ test/integration/kubernautagent/` |
| Build clean | 0 errors | `go build ./...` |
| Backward compatibility | 0 regressions | Existing tests pass without logic changes |

---

## 2. References

### 2.1 Authority

- Issue #885: Migrate kubernaut-agent from log/slog to pkg/log (zap-backed logr.Logger)
- Issue #875: Standardize log level configuration across all services
- DD-005: Logging standard (logr.Logger via kubelog)
- `docs/architecture/LOGGING_STANDARD.md` v2.0

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Go Coding Standards](../../.cursor/rules/02-go-coding-standards.mdc)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | logr.Logger has no Warn method | Warn-level messages lost in output | Low | UT-KA-885-002, UT-KA-885-003, UT-KA-885-004 | Map slog.Warn to logr.Info (standard convention per LOGGING_STANDARD.md) |
| R2 | logr.Error requires err argument first | Compile failures if signature mismatched | Medium | UT-KA-885-004 | Convert `logger.Error("msg", slog.String(...))` to `logger.Error(nil, "msg", ...)` when no error available |
| R3 | LLM WithLogger option breaks existing callers | API backward incompatibility | Low | UT-KA-885-005, UT-KA-885-006 | Option pattern with logr.Discard() default — existing callers unchanged |
| R4 | Integration tests using logr.FromSlogHandler bridge | Test compilation failure | Medium | IT-KA-885-001, IT-KA-885-002 | Replace bridge with direct logr.Discard() or funcr-based test logger |

---

## 4. Scope

### 4.1 Features to be Tested

- **MCP disconnect_handler** (`internal/kubernautagent/mcp/disconnect_handler.go`): Constructor signature migration and logger usage
- **MCP session_manager** (`internal/kubernautagent/mcp/session_manager.go`): Constructor signature migration and logger usage
- **MCP ds_reconstructor** (`internal/kubernautagent/mcp/ds_reconstructor.go`): Constructor signature migration and logger usage
- **MCP reconstruct** (`internal/kubernautagent/mcp/reconstruct.go`): Constructor signature migration and logger usage
- **LLM vertexanthropic** (`pkg/kubernautagent/llm/vertexanthropic/client.go`): WithLogger option injection
- **LLM langchaingo** (`pkg/kubernautagent/llm/langchaingo/adapter.go`): WithLogger option injection
- **Wiring** (`cmd/kubernautagent/main.go`, `cmd/kubernautagent/llm_builder.go`): Bridge removal and direct logr pass-through

### 4.2 Features Not to be Tested

- **pkg/log (kubelog) internals**: Already tested upstream; not changed by this issue
- **Hot-reload of log level**: Already implemented (#875); only verified not regressed
- **Other services' logging**: Out of scope (gateway, notification, etc.)

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Map `slog.Warn` to `logr.Info` | logr has no Warn level; Info is the standard convention per LOGGING_STANDARD.md |
| `WithLogger` option (not constructor param) for LLM | Backward-compatible; existing callers don't break |
| Default to `logr.Discard()` when no logger provided | Silent by default; no nil pointer risk |
| Use `logr.Discard()` in tests (not funcr) | Tests don't need log output; keeps test code simple |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: Verify constructor signatures accept logr.Logger and logging calls compile
- **Integration**: Verify wiring from main.go through MCP constructors works end-to-end

### 5.2 Two-Tier Minimum

Each component is tested at unit (signature + basic behavior) and integration (wiring) tiers.

### 5.3 Pass/Fail Criteria

**PASS**: All tests pass, zero slog imports remain, build clean.
**FAIL**: Any test fails, or slog imports remain in agent scope.

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/mcp/disconnect_handler.go` | `NewSessionClosedHandler`, `NewSessionJanitor` | ~135 |
| `internal/kubernautagent/mcp/session_manager.go` | `NewLeaseSessionManager`, `NewLeaseSessionManagerConcrete` | ~300 |
| `internal/kubernautagent/mcp/ds_reconstructor.go` | `NewDSContextReconstructor` | ~70 |
| `internal/kubernautagent/mcp/reconstruct.go` | `NewReconstructionSpawner` | ~100 |
| `pkg/kubernautagent/llm/vertexanthropic/client.go` | `New`, `WithLogger` | ~200 |
| `pkg/kubernautagent/llm/langchaingo/adapter.go` | `New`, `WithLogger` | ~200 |

### 6.2 Integration-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `cmd/kubernautagent/main.go` | `buildMCPHandler` | ~120 |
| `cmd/kubernautagent/llm_builder.go` | `buildLLMClientFromConfig` | ~50 |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-PLATFORM-885 | All slog imports removed from agent | P0 | Unit | UT-KA-885-001..006 | Pending |
| BR-PLATFORM-885 | slogBridge removed from main.go | P0 | Integration | IT-KA-885-001 | Pending |
| BR-PLATFORM-885 | LLM adapters accept logr via option | P1 | Integration | IT-KA-885-002 | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-885-001` | MCP disconnect_handler constructors accept logr.Logger and log correctly | Pending |
| `UT-KA-885-002` | MCP session_manager constructors accept logr.Logger and log correctly | Pending |
| `UT-KA-885-003` | MCP ds_reconstructor constructor accepts logr.Logger and logs on failure | Pending |
| `UT-KA-885-004` | MCP reconstruct spawner accepts logr.Logger and logs panic/error/warn | Pending |
| `UT-KA-885-005` | LLM vertexanthropic accepts WithLogger option | Pending |
| `UT-KA-885-006` | LLM langchaingo accepts WithLogger option | Pending |

### Tier 2: Integration Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-KA-885-001` | MCP constructors in buildMCPHandler receive logr.Logger directly (no bridge) | Pending |
| `IT-KA-885-002` | LLM builder passes logr.Logger to adapter constructors | Pending |

### Tier Skip Rationale

- **E2E**: Not applicable — logging migration has no user-facing behavioral change. E2E would test the same code paths as integration tests without additional value.

---

## 9. Test Cases

### UT-KA-885-001: disconnect_handler logr migration

**BR**: BR-PLATFORM-885
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/mcp/logr_migration_test.go`

**Test Steps**:
1. **Given**: logr.Discard() logger
2. **When**: NewSessionClosedHandler(eventStore, onClose, logger) called
3. **Then**: Handler is created without panic; Run() processes events using logr

**Acceptance Criteria**:
- Constructor compiles with logr.Logger parameter
- No slog imports in disconnect_handler.go

### UT-KA-885-002: session_manager logr migration

**BR**: BR-PLATFORM-885
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/mcp/logr_migration_test.go`

**Test Steps**:
1. **Given**: logr.Discard() logger
2. **When**: NewLeaseSessionManagerConcrete(client, namespace, logger) called
3. **Then**: Manager is created; Start/Release operations log via logr

**Acceptance Criteria**:
- Constructor compiles with logr.Logger parameter
- Warn-level messages emit via logr.Info (no data loss)

### UT-KA-885-003: ds_reconstructor logr migration

**BR**: BR-PLATFORM-885
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/mcp/logr_migration_test.go`

**Test Steps**:
1. **Given**: logr.Discard() logger and a failing AuditQuerier
2. **When**: DSContextReconstructor.Reconstruct() encounters an error
3. **Then**: Error is logged via logr.Info (warn equivalent) and empty context returned

**Acceptance Criteria**:
- Constructor compiles with logr.Logger parameter
- Graceful degradation behavior preserved

### UT-KA-885-004: reconstruct spawner logr migration

**BR**: BR-PLATFORM-885
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/mcp/logr_migration_test.go`

**Test Steps**:
1. **Given**: logr.Discard() logger and a panicking ReconRunner
2. **When**: ReconstructionSpawner.SpawnReconstruct() recovers from panic
3. **Then**: Panic is logged via logr.Error(nil, ...) with panic value

**Acceptance Criteria**:
- Constructor compiles with logr.Logger parameter
- Panic recovery still works (critical safety behavior)

### UT-KA-885-005: vertexanthropic WithLogger option

**BR**: BR-PLATFORM-885
**Priority**: P1
**Type**: Unit
**File**: `test/unit/kubernautagent/llm/vertexanthropic/logr_migration_test.go`

**Test Steps**:
1. **Given**: A logr.Logger (test sink)
2. **When**: vertexanthropic.New(..., vertexanthropic.WithLogger(logger)) called
3. **Then**: Client is created with injected logger; malformed tool JSON logs via logr

**Acceptance Criteria**:
- WithLogger option exists and is accepted by New
- Default (no option) uses logr.Discard() — no nil panic

### UT-KA-885-006: langchaingo WithLogger option

**BR**: BR-PLATFORM-885
**Priority**: P1
**Type**: Unit
**File**: `test/unit/kubernautagent/llm/langchaingo/logr_migration_test.go`

**Test Steps**:
1. **Given**: A logr.Logger (test sink)
2. **When**: langchaingo.New(..., langchaingo.WithLogger(logger)) called
3. **Then**: Adapter is created with injected logger; malformed tool JSON logs via logr

**Acceptance Criteria**:
- WithLogger option exists and is accepted by New
- Default (no option) uses logr.Discard() — no nil panic

### IT-KA-885-001: buildMCPHandler wiring (no slog bridge)

**BR**: BR-PLATFORM-885
**Priority**: P0
**Type**: Integration
**File**: Verified via existing integration tests in `test/integration/kubernautagent/mcp/`

**Test Steps**:
1. **Given**: All integration tests updated to pass logr.Logger
2. **When**: Integration test suite runs
3. **Then**: All MCP tests pass without slog imports

**Acceptance Criteria**:
- `grep -rn '"log/slog"' test/integration/kubernautagent/mcp/` returns 0 matches
- All integration tests pass

### IT-KA-885-002: LLM builder logger threading

**BR**: BR-PLATFORM-885
**Priority**: P1
**Type**: Integration
**File**: Verified via build + existing hot-reload tests

**Test Steps**:
1. **Given**: buildLLMClientFromConfig accepts logr.Logger
2. **When**: LLM client is built via hot-reload callback
3. **Then**: Logger is threaded to adapter via WithLogger

**Acceptance Criteria**:
- `cmd/kubernautagent/llm_builder.go` has no slog imports
- Build passes clean

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: None (logr.Discard() as logger)
- **Location**: `test/unit/kubernautagent/mcp/`, `test/unit/kubernautagent/llm/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD
- **Infrastructure**: envtest (for lease tests)
- **Location**: `test/integration/kubernautagent/mcp/`

### 10.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.22+ | Build and test |
| Ginkgo CLI | v2.x | Test runner |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

None. All prerequisite work (kubelog, LoggingConfig, hot-reload) is already in place.

### 11.2 Execution Order

1. **Phase 1 (RED)**: Write UT-KA-885-001..006 as failing tests
2. **Phase 2 (GREEN)**: Migrate production code, update test files
3. **Phase 3 (REFACTOR)**: Clean up imports, verify consistency
4. **Phase 4 (WIRING)**: Verified via existing IT suite (no new IT files needed)

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/885/TEST_PLAN.md` | Strategy and test design |
| Unit test file | `test/unit/kubernautagent/mcp/logr_migration_test.go` | MCP logr tests |
| Verification | Build + grep | Zero slog imports |

---

## 13. Execution

```bash
# Unit tests (MCP)
go test ./test/unit/kubernautagent/mcp/... -ginkgo.v

# Unit tests (LLM)
go test ./test/unit/kubernautagent/llm/... -ginkgo.v

# Integration tests (MCP)
go test ./test/integration/kubernautagent/mcp/... -ginkgo.v

# Verification
grep -rn '"log/slog"' internal/kubernautagent/ pkg/kubernautagent/ cmd/kubernautagent/ test/unit/kubernautagent/ test/integration/kubernautagent/
```

---

## 14. Wiring Verification

| Code Path | Entry Point | Exit Point | Wiring IT | Status |
|-----------|-------------|------------|-----------|--------|
| MCP session manager logging | buildMCPHandler | Lease create/release log | Existing lease_session_mgr_test.go | Pending |
| MCP disconnect handler logging | buildMCPHandler | Disconnect event log | Existing disconnect_handler_test.go | Pending |
| MCP reconstruction logging | buildMCPHandler | Reconstruction log | Existing reconstruct_test.go | Pending |
| LLM logger injection | buildLLMClientFromConfig | Tool schema warn log | Existing LLM IT | Pending |

---

## 15. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| All 15 test files using `slog.Default()` / `slog.New(...)` | Pass `*slog.Logger` to MCP constructors | Pass `logr.Discard()` or `logr.Logger` | Constructor signatures changed |
| `helpers_test.go:119-120` | `logr.FromSlogHandler(slogLogger.Handler())` | Use `logr.Discard()` directly | No more slog bridge needed |
| `interactive_compat_test.go:48-49` | Same bridge pattern | Use `logr.Discard()` directly | No more slog bridge needed |
| `suite_test.go:56,151` | `suiteLogger *slog.Logger` | Remove or convert to `logr.Logger` | Dead variable using slog |

---

## 16. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-05-01 | Initial test plan |

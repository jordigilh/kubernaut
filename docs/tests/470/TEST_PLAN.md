# Test Plan: ConfigManager SDK Config Merge for Accurate Toolset Reporting

**Feature**: ConfigManager merges SDK config so DD-HAPI-004 log accurately reports toolsets
**Version**: 1.0
**Created**: 2026-03-20
**Author**: AI Assistant
**Status**: Executed — All Tests Passing
**Branch**: `development/v1.2`

**Authority**:
- [BR-HAPI-199]: ConfigMap Hot-Reload
- [DD-HAPI-004]: ConfigMap Hot-Reload Design
- Issue #470: HAPI startup reports toolsets=[] despite SDK config containing prometheus/metrics

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Integration/E2E No-Mocks Policy](../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)

---

## 1. Scope

### In Scope

- `src/config/hot_reload.py` (`ConfigManager`): SDK config merge during initial load and hot-reload
- `src/main.py` (`init_config_manager()`): Passing SDK config path to ConfigManager
- DD-HAPI-004 log accuracy: toolsets reported must reflect the full merged config (main + SDK)

### Out of Scope

- SDK config hot-reload (watching SDK file for changes independently) — separate feature
- `src/config/sdk_loader.py`: No changes to merge logic, already tested
- `src/extensions/llm_integration.py`: Toolset loading at query time is unaffected

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Add required `sdk_config_path` param to ConfigManager | No backward compatibility needed. All callers updated. Existing tests pass `""` for cases where SDK config is irrelevant. |
| Graceful degradation on SDK merge failure | During hot-reload, SDK file may be temporarily unavailable (kubelet remount). Log warning and apply main config without SDK merge rather than rejecting the change entirely. Empty path or missing file is handled the same way. |
| SDK config re-merged on every main config reload | Ensures consistency — if main config changes, the latest SDK state is always included |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (ConfigManager SDK merge logic, graceful degradation)
- **Integration**: Not applicable — ConfigManager is pure logic with filesystem I/O via tempfiles (already tested as unit tests with real files)
- **E2E**: Not applicable — startup log reporting is a cosmetic/observability fix

### 2-Tier Minimum

This change is **pure logic** in a single Python class. The existing test suite already uses **real filesystem I/O** (tempfiles) rather than mocks, making the unit tests functionally equivalent to integration tests. A single tier (unit) with real file I/O provides sufficient defense-in-depth.

### Tier Skip Rationale

- **Integration**: ConfigManager tests already use real filesystem via `tempfile.NamedTemporaryFile`. Adding a separate integration tier would duplicate the same approach. The unit tests ARE integration tests in practice.
- **E2E**: The fix is a log message accuracy improvement. E2E validation requires deploying to a cluster and inspecting pod logs, which is disproportionate to the risk level.

### Business Outcome Quality Bar

Each test validates: "Does the operator see accurate toolset information in the DD-HAPI-004 log?" — not just "is the function called?"

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic with filesystem I/O via tempfiles)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `holmesgpt-api/src/config/hot_reload.py` | `ConfigManager.__init__`, `_on_config_change` | ~15 (new/modified) |
| `holmesgpt-api/src/main.py` | `init_config_manager` | ~3 (modified) |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-HAPI-199 | ConfigManager reports accurate toolsets after SDK merge | P1 | Unit | UT-HAPI-470-001 | Pass |
| BR-HAPI-199 | ConfigManager merges SDK toolsets on initial load | P1 | Unit | UT-HAPI-470-002 | Pass |
| BR-HAPI-199 | ConfigManager merges SDK toolsets on hot-reload | P1 | Unit | UT-HAPI-470-003 | Pass |
| BR-HAPI-199 | ConfigManager gracefully degrades when SDK file missing | P1 | Unit | UT-HAPI-470-004 | Pass |
| BR-HAPI-199 | ConfigManager gracefully degrades when SDK file invalid | P1 | Unit | UT-HAPI-470-005 | Pass |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `UT-HAPI-470-{SEQUENCE}`

### Tier 1: Unit Tests

**Testable code scope**: `src/config/hot_reload.py` ConfigManager SDK merge — targeting 100% of new/modified lines.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-HAPI-470-001` | DD-HAPI-004 log reports SDK toolsets (not empty) after startup | Pass |
| `UT-HAPI-470-002` | get_toolsets() returns merged toolsets from both main and SDK configs | Pass |
| `UT-HAPI-470-003` | Hot-reload of main config re-merges SDK toolsets (not lost after reload) | Pass |
| `UT-HAPI-470-004` | Missing/empty SDK path: ConfigManager starts normally, toolsets from main config only, warning logged | Pass |
| `UT-HAPI-470-005` | Invalid SDK file (empty YAML/malformed): ConfigManager starts normally, applies main config, warning logged | Pass |

---

## 6. Test Cases (Detail)

### UT-HAPI-470-001: DD-HAPI-004 log reports SDK toolsets

**BR**: BR-HAPI-199
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_config_manager.py`

**Given**: A main config file with `llm.model: gpt-4` and no toolsets, and an SDK config file with `toolsets: {prometheus/metrics: {enabled: true}}`
**When**: ConfigManager is created with both paths and started
**Then**: The DD-HAPI-004 log message contains `prometheus/metrics` in the toolsets list

**Acceptance Criteria**:
- Log output includes `toolsets=['prometheus/metrics']` (not `toolsets=[]`)
- `get_toolsets()` returns a dict containing `prometheus/metrics`

### UT-HAPI-470-002: get_toolsets() returns merged toolsets

**BR**: BR-HAPI-199
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_config_manager.py`

**Given**: Main config has `toolsets: {kubernetes/core: {}}`, SDK config has `toolsets: {prometheus/metrics: {enabled: true}}`
**When**: ConfigManager starts with both paths
**Then**: `get_toolsets()` returns both `kubernetes/core` and `prometheus/metrics`

**Acceptance Criteria**:
- Both toolsets present in the returned dict
- SDK toolsets do not overwrite main config toolsets (additive merge)

### UT-HAPI-470-003: Hot-reload preserves SDK toolsets

**BR**: BR-HAPI-199
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_config_manager.py`

**Given**: ConfigManager running with SDK toolsets merged
**When**: Main config file is updated (e.g., `llm.model` changes)
**Then**: After hot-reload, `get_toolsets()` still contains SDK toolsets

**Acceptance Criteria**:
- SDK toolsets survive main config reload
- New main config values are applied
- DD-HAPI-004 reload log still reports SDK toolsets

### UT-HAPI-470-004: Missing/empty SDK path — graceful degradation

**BR**: BR-HAPI-199
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_config_manager.py`

**Given**: Main config file exists, SDK config path is empty string or points to non-existent file
**When**: ConfigManager starts
**Then**: ConfigManager starts normally with main config only; a warning is logged

**Acceptance Criteria**:
- No exception raised
- `get_toolsets()` returns toolsets from main config (or empty if main has none)
- Warning log contains the missing/empty SDK path info

### UT-HAPI-470-005: Invalid SDK file — graceful degradation

**BR**: BR-HAPI-199
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_config_manager.py`

**Given**: Main config file exists, SDK config file contains invalid YAML or is empty
**When**: ConfigManager starts
**Then**: ConfigManager starts normally with main config only; a warning is logged

**Acceptance Criteria**:
- No exception raised
- `get_toolsets()` returns toolsets from main config only
- Warning log indicates SDK merge failure

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: pytest (HAPI Python convention, consistent with existing `test_config_manager.py`)
- **Mocks**: None — uses real filesystem via `tempfile.NamedTemporaryFile`
- **Log capture**: pytest `caplog` fixture for asserting log messages
- **Location**: `holmesgpt-api/tests/unit/test_config_manager.py` (append to existing file)

### Anti-Pattern Compliance

- No `time.sleep()` for async waiting — use `wait_for` fixture (already available in conftest)
- No `Skip()` — all tests must pass or fail
- No mocking of business logic — ConfigManager and sdk_loader are real
- Tests validate business outcomes (operator sees accurate toolsets), not implementation details

---

## 8. Execution

```bash
# All HAPI unit tests
cd holmesgpt-api && python3 -m pytest tests/unit/test_config_manager.py -v

# Specific test by pattern
cd holmesgpt-api && python3 -m pytest tests/unit/test_config_manager.py -v -k "470"
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-20 | Initial test plan |
| 1.1 | 2026-03-20 | All 5 tests passing. Committed to development/v1.2, cherry-picked to fix/1.1.0-rc4 |

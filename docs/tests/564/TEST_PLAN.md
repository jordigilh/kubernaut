# Test Plan: Mock LLM Scenario Registry Refactor (#564)

**Test Plan Identifier**: TP-564-v1
**Feature**: Split monolithic scenario registry into individual scenario files
**Version**: 1.0
**Created**: 2026-03-28
**Author**: AI Assistant
**Status**: Active
**Branch**: `development/v1.3`

---

## 1. Introduction

### 1.1 Purpose

Validate that the scenario registry refactoring — splitting `scenarios/default.go` into
individual per-scenario files — introduces zero behavioral changes. All 15 scenarios must
continue to self-register, detect correctly, and produce identical HTTP responses.

### 1.2 Objectives

1. **Behavioral Equivalence**: All existing unit and integration tests pass without modification
2. **Registry Completeness**: `DefaultRegistry()` returns exactly 16 scenarios (15 + fallback)
3. **Override Compatibility**: `DefaultRegistryWithOverrides` continues to apply overrides correctly

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/mockllm/...` |
| Integration test pass rate | 100% | `go test ./test/integration/mockllm/...` |
| Backward compatibility | 0 regressions | All pre-existing tests pass unmodified |
| Scenario count | 16 | `len(DefaultRegistry().List())` |

---

## 2. References

| Document | Relevance |
|----------|-----------|
| BR-MOCK-020: Scenario Registry Architecture | Registry self-registration requirement |
| BR-MOCK-021: Mock Keyword Detection | Keyword scenarios must continue to match |
| BR-MOCK-022: Signal Name Detection | Signal scenarios must continue to match |
| BR-MOCK-023: Proactive Mode Detection | Proactive scenarios must continue to match |
| BR-MOCK-024: Default Fallback | Fallback must continue to work |
| BR-MOCK-025: Detection Priority | Priority ordering must be preserved |
| `docs/tests/531/IMPLEMENTATION_PLAN.md` | Master plan Phase 2 |

---

## 3. Scope

### 3.1 In Scope

- Splitting `scenarios/default.go` (417 lines) into individual scenario files
- Preserving all match functions, config constructors, and detection logic
- Verifying registry ordering and priority resolution

### 3.2 Out of Scope

- New scenario additions (deferred to Phase 6)
- DAG assignment per scenario (already nil; wiring deferred to Phase 1B+)
- `init()` self-registration (conflicts with `DefaultRegistryWithOverrides`)

---

## 4. Test Strategy

This is a **pure structural refactor**. No new tests are needed. The existing test suite
(94 UT + 39 IT = 133 total) serves as the complete behavioral equivalence gate.

### 4.1 Acceptance Gate

| Check | Command | Expected |
|-------|---------|----------|
| Full build | `go build ./...` | exit 0 |
| Unit tests | `go test ./test/unit/mockllm/... -count=1` | 94+ pass |
| Integration tests | `go test ./test/integration/mockllm/... -count=1` | 39 pass |
| No unused imports | `go vet ./test/services/mock-llm/...` | exit 0 |

### 4.2 Risk Mitigation

| Risk | Mitigation |
|------|------------|
| Accidental scenario omission | Existing IT tests cover all 15 scenarios |
| Import cycle from file split | All files in same package (`scenarios`) |
| Broken unexported helper references | Keep helpers in shared file |

---

## 5. File Layout (Target)

```
scenarios/
├── types.go           # MockScenarioConfig, configScenario, ScenarioWithConfig
├── registry.go        # DefaultRegistry, DefaultRegistryWithOverrides, defaultRegistryInternal
├── match_helpers.go   # mockKeywordScenario, signalScenario, extractSignal, isProactive
├── scenario_oomkilled.go
├── scenario_crashloop.go
├── scenario_node_not_ready.go
├── scenario_cert_not_ready.go
├── scenario_test_signal.go
├── scenario_mock_keywords.go   # 7 keyword-based scenarios
├── scenario_proactive.go       # predictive_no_action, oomkilled_predictive
└── scenario_default.go         # default fallback
```

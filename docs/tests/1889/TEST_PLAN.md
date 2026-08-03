# Test Plan: Tool-budget MCPError code + per-investigation AnomalyDetector isolation + Helm value (#1889, #1892) — `main`/v1.6 port

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1889-1892-main-v1.0
**Version**: 1.0
**Created**: 2026-08-03
**Author**: AI Assistant
**Status**: Implemented
**Branch**: `fix/1889-1892-tool-budget-isolation-main` (targets `main`)

---

## 1. Introduction

### 1.1 Purpose

This is the `main`/v1.6 port of the fix originally designed, TDD'd, and CI-validated on
`release/v1.5` in [PR #1895](https://github.com/jordigilh/kubernaut/pull/1895) (branch
`fix/1889-1892-tool-budget-isolation`). Per project policy, `release/v1.5` is fixed first (it
ships to customers); this document tracks the follow-up port to `main` once `main`'s
independently-refactored codebase structure was re-verified against the same design.

The v1.5 PR's design rationale, alternatives analysis, and full test case descriptions are not
repeated here — see `docs/tests/1889/TEST_PLAN.md` on `release/v1.5` (PR #1895) for that. This
document records only what differs in the port: `main`'s file layout, and confirmation that every
call site enumerated in the original plan has a `main`-side equivalent.

Three issues, one PR (same combined-PR rationale as #1895: CI runner capacity, ~30min/PR):

1. **#1889 gap 1**: `ErrCodeToolBudgetExhausted` MCPError code, so an interactive session hitting
   the I7 anomaly detector's tool-call budget surfaces a diagnosable `tool_budget_exhausted` code
   instead of being redacted to a generic `internal_error` by `ErrorBoundary`.
2. **#1892**: Per-investigation `AnomalyDetector` isolation. `main`, like `release/v1.5`, shares one
   pod-wide `AnomalyDetector` (`inv.pipeline.AnomalyDetector`) across every concurrent
   investigation; one investigation's phase-transition `Reset()` silently zeroed every other
   in-flight investigation's tool-call budget on the same KA pod.
3. **Helm value**: expose `maxTotalToolCalls` as `kubernautAgent.ai.safety.anomaly.maxTotalToolCalls`
   (Go-side `cfg.AI.Safety.Anomaly.MaxTotalToolCalls` already existed on `main`; only the Helm
   chart lacked a way to set it — the template hardcoded `investigation.maxTurns: 40` with no
   `safety` block at all).

### 1.2 Codebase-structure differences from the `release/v1.5` port (confirmed by direct inspection)

| Aspect | `release/v1.5` | `main` |
|---|---|---|
| `ExhaustedResult`/`runLLMLoop`/`processToolCalls` location | `investigator.go` (inline) | `investigator.go` (types) + `investigator_loop.go` (loop logic) — refactored into a separate file |
| `ReasonToolBudgetExhausted` | added as a `const` in `investigator.go` | same: added as a `const` next to `ExhaustedResult`'s definition in `investigator.go` |
| `executeTool` signature | took `(ctx, name, args)`, no correlationID (pre-fix) | same starting shape: took `(ctx, name, args)`, no correlationID (pre-fix) — both needed the same `correlationID` parameter added |
| `investigate.go` / `investigate_takeover.go` wrap site | `investigate.go:656` `"interactive turn failed: %w"` | `investigate_takeover.go:131` `"interactive turn failed: %w"` — same wrap text, different filename |
| Helm `values.yaml`/`values.schema.json` structure | `kubernautAgent.llm.*` literal block | `kubernautAgent.llmProfileRef` + `global.llmProfiles` (DD-PLATFORM-007) — chart restructured independently of this fix; the new `kubernautAgent.ai.safety.anomaly.maxTotalToolCalls` leaf is unaffected by this and inserted the same way |
| `docs/tests/`, `docs/architecture/decisions/DD-K8S-001*` | present (from this same work) | did not exist on `main` prior to this port (created fresh here) |

All other call sites (`Investigate()`, `runWorkflowDiscoveryPhase()`,
`RunWorkflowDiscoveryFromRCA()`, `processToolCalls()`, `cmd/kubernautagent/main.go`'s
`store.StartCleanupLoop` neighbor) matched the `release/v1.5` shape exactly, confirmed by direct
read of each file before editing (CHECKPOINT A).

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit/Integration test pass rate | 100% | `make test-unit-kubernautagent` |
| Helm chart test pass rate | 100%, 0 regressions | `make test-helm` (436 tests across 45 suites) |
| Backward compatibility | 0 regressions | Full `make test-unit-kubernautagent` suite (109 specs / 34 Ginkgo suites) |
| Build/lint | Clean | `go build ./...`, `golangci-lint run` on changed packages |

---

## 2. References

- [PR #1895](https://github.com/jordigilh/kubernaut/pull/1895) — original `release/v1.5` implementation (design authority)
- `docs/tests/1889/TEST_PLAN.md` on `release/v1.5` — full design rationale, alternatives, and per-test-case detail
- Issue #1889, Issue #1892

---

## 3. Test Items (`main`-specific file paths)

| File | Change |
|------|--------|
| `internal/kubernautagent/mcp/tools/errors.go` | Added `ErrCodeToolBudgetExhausted` |
| `internal/kubernautagent/mcp/tools/errors_test.go` | **New**. `IT-KA-1889-001` + `ErrorBoundary` unit tests |
| `internal/kubernautagent/mcp/adapters/adapters.go` | `ExtractContent` returns `ErrCodeToolBudgetExhausted` for `ReasonToolBudgetExhausted` |
| `internal/kubernautagent/investigator/investigator.go` | Added `ReasonToolBudgetExhausted` const; added `anomalyScope` field + wiring in `New()`; `Investigate()`/`runWorkflowDiscoveryPhase()` use `anomalyDetectorFor(correlationID)` |
| `internal/kubernautagent/investigator/investigator_loop.go` | `processToolCalls` uses `anomalyDetectorFor(correlationID).TotalExceeded()`; passes `correlationID` to `executeTool`; uses `ReasonToolBudgetExhausted` constant |
| `internal/kubernautagent/investigator/investigator_tools.go` | `executeTool` takes `correlationID`, uses `anomalyDetectorFor(correlationID)` |
| `internal/kubernautagent/investigator/investigator_discovery.go` | `RunWorkflowDiscoveryFromRCA` uses `anomalyDetectorFor(correlationID).Reset()` |
| `internal/kubernautagent/investigator/anomaly.go` | Added `Clone()` |
| `internal/kubernautagent/investigator/investigator_anomaly_scope.go` | **New**. `anomalyScope`, `anomalyDetectorFor`, `pruneAnomalyDetectors`, `StartAnomalyDetectorCleanupLoop`, `DefaultAnomalyDetectorTTL` |
| `internal/kubernautagent/investigator/investigator_anomaly_scope_test.go` | **New**. `UT-KA-1892-002/003`, `IT-KA-1892-001` |
| `cmd/kubernautagent/main.go` | Wired `p.inv.StartAnomalyDetectorCleanupLoop` next to `p.store.StartCleanupLoop` |
| `charts/kubernaut/values.yaml` | Added `kubernautAgent.ai.safety.anomaly.maxTotalToolCalls: 30` |
| `charts/kubernaut/values.schema.json` | Added schema for `kubernautAgent.ai.safety.anomaly.maxTotalToolCalls` |
| `charts/kubernaut/templates/kubernaut-agent/kubernaut-agent.yaml` | Templates `investigation.safety.anomaly.maxTotalToolCalls` from `$v.ai.safety.anomaly.maxTotalToolCalls` |
| `charts/kubernaut/tests/kubernaut_agent_anomaly_config_test.yaml` | **New**. `HT-KA-1892-001a/b` — default + override rendering |

---

## 4. BR Coverage Matrix

| Reference | Description | Priority | Tier | Test ID | Status |
|-----------|--------------|----------|------|---------|--------|
| #1889 | Tool budget exhaustion surfaces as `tool_budget_exhausted`, not `internal_error` | P1 | Unit+Integration | IT-KA-1889-001 | Pass |
| #1892 | Per-correlationID `AnomalyDetector` memoization | P1 | Unit | UT-KA-1892-003 | Pass |
| #1892 | Concurrent investigations never observe each other's `Reset()`/counters | P1 | Integration | IT-KA-1892-001 | Pass |
| #1892 | Idle per-investigation detector entries are pruned by TTL sweep | P2 | Unit | UT-KA-1892-002 | Pass |
| Helm | `maxTotalToolCalls` renders default (30) and operator override | P2 | Helm-unittest | HT-KA-1892-001a/b | Pass |

---

## 5. Execution Evidence

```bash
go build ./...                                        # clean
go vet ./internal/kubernautagent/... ./cmd/kubernautagent/...   # clean
golangci-lint run --timeout=5m \
  ./internal/kubernautagent/investigator/... \
  ./internal/kubernautagent/mcp/... \
  ./cmd/kubernautagent/...                             # 0 issues
make test-unit-kubernautagent                          # 109 Passed, 0 Failed (34 suites)
make test-helm                                         # 436 Passed, 0 Failed (45 suites)
```

---

## 6. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-08-03 | Initial port of PR #1895 to `main`. All tests pass; build/vet/lint clean. Opened as a draft PR pending console-team validation of the `release/v1.5` fix in production. |

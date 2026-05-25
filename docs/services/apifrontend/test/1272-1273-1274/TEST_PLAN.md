# Test Plan: TP-AF-1272-1273-1274-01

## 1. Test Plan Identifier

**TP-AF-1272-1273-1274-01** — Session Controller Resilience, Diagnostics, and Logging Unification

## 2. References

| Document | Link |
|----------|------|
| Issue #1272 | Graceful degradation: session health flag + TTL metrics |
| Issue #1273 | Diagnostic logging: pre-flight CRD discovery + RBAC checks |
| Issue #1274 | slog-to-logr conversion across all AF production code |
| ADR-017 | CRD PII classification |
| DD-TEST-006 | Test plan policy |
| DD-TEST-010 | Multi-process integration test infrastructure |
| NIST AU-11 | 30-day minimum audit record retention |

## 3. Introduction

This test plan covers three related issues that strengthen the AF session controller:

1. **#1274 (slog-to-logr)**: Replace `slog.Default()` with injected `logr.Logger` in 6 production files, eliminating logging blind spots where errors were silently dropped.
2. **#1273 (diagnostic logging)**: Add pre-flight CRD discovery and RBAC checks before starting the session controller manager, providing actionable diagnostic output when the controller fails.
3. **#1272 (graceful degradation)**: Add a session health flag (`atomic.Bool`) driven by `WaitForCacheSync`, and wire `af_session_ttl_actions_total` Prometheus counter into the TTL reconciler.

## 4. Test Items

### 4.1 Production Files Under Test

| File | Issue | Change |
|------|-------|--------|
| `internal/controller/apifrontend/ttl.go` | #1274 | `*slog.Logger` -> `logr.Logger` in struct + constructor |
| `pkg/apifrontend/session/service.go` | #1274 | `*slog.Logger` -> `logr.Logger`, add `WithLogger` option |
| `pkg/apifrontend/session/statemachine.go` | #1274 | Shared logger field type change |
| `pkg/apifrontend/launcher/launcher.go` | #1274 | `A2AConfig.Logger` type change, `buildAfterExecuteCallback` param |
| `pkg/apifrontend/launcher/streaming_executor.go` | #1274 | Constructor param + field type change |
| `pkg/apifrontend/config/hotreload.go` | #1274 | `WithLogger` option param type change |
| `pkg/apifrontend/tools/ka_stream.go` | #1274 | Replace `slog.WarnContext` with `logr.Logger` |
| `pkg/apifrontend/agent/root.go` | #1274 | Wire logger to tool factory |
| `cmd/apifrontend/main.go` | #1272, #1273, #1274 | Wire loggers, health flag, pre-flight checks, TTL metrics |
| `pkg/apifrontend/metrics/metrics.go` | #1272 | Add `SessionTTLActions` counter |

### 4.2 Test Files

| File | Tier | Tests |
|------|------|-------|
| `internal/controller/apifrontend/ttl_test.go` | UT | UT-AF-1274-001, 002; UT-AF-1272-004, 005, 006 |
| `pkg/apifrontend/session/service_test.go` | UT | UT-AF-1274-003, 004 |
| `pkg/apifrontend/launcher/launcher_test.go` | UT | UT-AF-1274-005 |
| `pkg/apifrontend/launcher/streaming_executor_test.go` | UT | UT-AF-1274-006, 007 |
| `pkg/apifrontend/config/hotreload_test.go` | UT | UT-AF-1274-008 |
| `pkg/apifrontend/tools/ka_stream_test.go` | UT | UT-AF-1274-009 |
| `cmd/apifrontend/main_wiring_test.go` | UT | UT-AF-1274-010, 011, 012; UT-AF-1273-001..004; UT-AF-1272-001, 002, 003 |
| `test/integration/apifrontend/session_wiring_test.go` | IT | IT-AF-1272-001, 002; IT-AF-1273-001, 002, 003 |
| `test/integration/apifrontend/logger_wiring_test.go` | IT | IT-AF-1274-001, 002, 003, 004 |
| `test/e2e/apifrontend/session_controller_wiring_test.go` | E2E | E2E-AF-1274-001; E2E-AF-1273-001, 002; E2E-AF-1272-001, 002 |

## 5. Pyramid Invariant

> UT proves logic. IT proves wiring. E2E proves the journey.

| Tier | Count | Location |
|------|-------|----------|
| Unit | 22 | Colocated `*_test.go` + `main_wiring_test.go` |
| Integration | 9 | `test/integration/apifrontend/` |
| E2E | 5 | `test/e2e/apifrontend/` |
| **Total** | **36** | |

## 6. Wiring Manifest

Every production wiring point mapped to its test coverage:

### 6.1 Issue #1274: slog-to-logr

| Component | Production Entry Point | Wiring Location | UT | IT | E2E |
|-----------|----------------------|-----------------|----|----|-----|
| TTL reconciler logr | `buildSessionInfra` | `main.go:1009` | UT-AF-1274-001, 002 | IT-AF-1274-001 | E2E-AF-1274-001 |
| Session service logr | `buildSessionInfra` | `main.go:999` | UT-AF-1274-003, 004 | IT-AF-1274-002 | (covered) |
| Launcher logr | `buildA2AHandler` | `main.go:682` | UT-AF-1274-005 | IT-AF-1274-003 | (covered) |
| Streaming executor logr | Constructor via launcher | `launcher.go:80` | UT-AF-1274-006, 007 | (via IT-1274-003) | (covered) |
| Hotreload logr | `config.NewFileWatcher` | `main.go:186` | UT-AF-1274-008 | IT-AF-1274-004 | (covered) |
| ka_stream logr | Tool factory | `agent/root.go` | UT-AF-1274-009 | (via IT-1274-003) | (covered) |
| main.go wiring | `buildSessionInfra`, `buildA2AHandler` | `main.go` | UT-AF-1274-010, 011, 012 | -- | -- |

### 6.2 Issue #1273: Diagnostic Logging

| Component | Production Entry Point | Wiring Location | UT | IT | E2E |
|-----------|----------------------|-----------------|----|----|-----|
| CRD discovery | `buildSessionInfra` | `main.go` (before `ctrl.NewManager`) | UT-AF-1273-001 | IT-AF-1273-001 | E2E-AF-1273-001 |
| RBAC SSAR check | `buildSessionInfra` | `main.go` (after discovery) | UT-AF-1273-002 | IT-AF-1273-002 | (covered) |
| Manager start log | `buildSessionInfra` | `main.go` (before `mgr.Start`) | UT-AF-1273-003 | (via IT-1273-001) | (covered) |
| ctrl.SetLogger | `run()` | `main.go:82` (already done) | UT-AF-1273-004 | IT-AF-1273-003 | E2E-AF-1273-002 |

### 6.3 Issue #1272: Graceful Degradation

| Component | Production Entry Point | Wiring Location | UT | IT | E2E |
|-----------|----------------------|-----------------|----|----|-----|
| Session health flag | `buildSessionInfra` -> `WaitForCacheSync` | `main.go` | UT-AF-1272-001, 002, 003 | IT-AF-1272-001 | E2E-AF-1272-001 |
| TTL metrics | `NewSessionCleanupReconciler(... metrics ...)` | `main.go:1009` | UT-AF-1272-004, 005, 006 | IT-AF-1272-002 | E2E-AF-1272-002 |

## 7. Business Requirements Mapping

| BR | Description | Tests |
|----|-------------|-------|
| BR-SESS-011 | Session controller degrades gracefully when cache sync fails | UT-AF-1272-001..003, IT-AF-1272-001, E2E-AF-1272-001 |
| BR-SESS-012 | Operators can diagnose session controller failures from logs | UT-AF-1273-001..004, IT-AF-1273-001..003, E2E-AF-1273-001..002 |
| BR-SESS-013 | All AF production code uses unified logr logging pipeline | UT-AF-1274-001..012, IT-AF-1274-001..004, E2E-AF-1274-001 |
| BR-MONITORING-001 | TTL actions are observable via Prometheus metrics | UT-AF-1272-004..006, IT-AF-1272-002, E2E-AF-1272-002 |

## 8. Test Scenarios

### 8.1 Unit Tests (#1274: slog-to-logr)

| ID | Description | Input | Expected | BR |
|----|-------------|-------|----------|-----|
| UT-AF-1274-001 | Reconciler accepts `logr.Logger` and logs reconcile errors through it | Reconcile with error-inducing session | Error logged via injected logger (not slog) | BR-SESS-013 |
| UT-AF-1274-002 | NIST clamp warning uses injected logger | retentionTTL below MinRetentionTTL | Warning logged via injected logger | BR-SESS-013 |
| UT-AF-1274-003 | `WithLogger(logr.Logger)` option wires logger into CRDSessionService | Service created with WithLogger | Logger field set to provided value | BR-SESS-013 |
| UT-AF-1274-004 | Default logger is `logr.Discard()` when no WithLogger option | Service created without WithLogger | Logger is logr.Discard() | BR-SESS-013 |
| UT-AF-1274-005 | `A2AConfig.Logger` accepts `logr.Logger` | Config with logr.Logger | logger() returns provided logger | BR-SESS-013 |
| UT-AF-1274-006 | StreamingExecutor constructor accepts `logr.Logger` | Constructor with logr.Logger | Field stores provided logger | BR-SESS-013 |
| UT-AF-1274-007 | StreamingExecutor logs stream open/close through logr | Execute with mock inner | "a2a stream opened"/"closed" logged | BR-SESS-013 |
| UT-AF-1274-008 | FileWatcher `WithLogger` accepts `logr.Logger` | NewFileWatcher with WithLogger | Logger field set to provided value | BR-SESS-013 |
| UT-AF-1274-009 | ka_stream tool logs bridge failures through logr | emitViaBridge with failing bridge | Error logged via injected logger (not slog.Warn) | BR-SESS-013 |
| UT-AF-1274-010 | `buildSessionInfra` passes logger to reconciler | buildSessionInfra with logger | Reconciler uses provided logger name | BR-SESS-013 |
| UT-AF-1274-011 | `buildA2AHandler` passes logger to A2AConfig | buildA2AHandler with logger | A2AConfig.Logger is set | BR-SESS-013 |
| UT-AF-1274-012 | Config watcher receives logger via WithLogger | cfgWatcher construction | Logger is wired (not slog.Default) | BR-SESS-013 |

### 8.2 Unit Tests (#1273: Diagnostic Logging)

| ID | Description | Input | Expected | BR |
|----|-------------|-------|----------|-----|
| UT-AF-1273-001 | Pre-flight CRD discovery logs GVR and result | Mock discovery client (present/absent) | Log contains "pre-flight CRD discovery", GVR, available=true/false | BR-SESS-012 |
| UT-AF-1273-002 | Pre-flight RBAC check logs permission status | Mock SSAR (allowed/denied) | Log contains "pre-flight RBAC check", allowed=true/false | BR-SESS-012 |
| UT-AF-1273-003 | Manager start log includes namespace, GVR | buildSessionInfra with config | Log contains namespace, GVR | BR-SESS-012 |
| UT-AF-1273-004 | ctrl.SetLogger wires controller-runtime logs | ctrl.SetLogger called | controller-runtime logs flow through zapr | BR-SESS-012 |

### 8.3 Unit Tests (#1272: Graceful Degradation)

| ID | Description | Input | Expected | BR |
|----|-------------|-------|----------|-----|
| UT-AF-1272-001 | `sessionInfra.Healthy` is false before cache sync | New sessionInfra (no sync) | Healthy.Load() == false | BR-SESS-011 |
| UT-AF-1272-002 | `sessionInfra.Healthy` becomes true after sync | Simulate cache sync | Healthy.Load() == true | BR-SESS-011 |
| UT-AF-1272-003 | Fake-client path sets Healthy=true immediately | buildSessionInfra without kubeconfig | Healthy.Load() == true | BR-SESS-011 |
| UT-AF-1272-004 | Cancel action increments `af_session_ttl_actions_total{action="cancel"}` | Reconcile expired disconnect | Counter value == 1.0 | BR-MONITORING-001 |
| UT-AF-1272-005 | Delete action increments `af_session_ttl_actions_total{action="delete"}` | Reconcile expired terminal | Counter value == 1.0 | BR-MONITORING-001 |
| UT-AF-1272-006 | `SessionTTLActions` registered in metrics registry | NewRegistry() | `/metrics` output contains `af_session_ttl_actions_total` | BR-MONITORING-001 |

### 8.4 Integration Tests

| ID | Component | Description | Infrastructure | BR |
|----|-----------|-------------|----------------|-----|
| IT-AF-1274-001 | TTL reconciler | envtest manager with reconciler, trigger reconcile, assert structured log captured | Isolated envtest + log buffer | BR-SESS-013 |
| IT-AF-1274-002 | Session service | Create InvestigationSession via CRDSessionService on envtest, assert log captured | Suite k8sClient + log buffer | BR-SESS-013 |
| IT-AF-1274-003 | Launcher/A2A | HTTP POST through router to A2A endpoint, assert launcher log captured | Suite routerServer + log buffer | BR-SESS-013 |
| IT-AF-1274-004 | Hotreload | Trigger config file change, assert "config reloaded" log captured | Temp file + watcher + log buffer | BR-SESS-013 |
| IT-AF-1273-001 | CRD discovery | Call discovery helper against envtest with CRD installed, assert log | Envtest + log buffer | BR-SESS-012 |
| IT-AF-1273-002 | RBAC SSAR | envtest with SA + RBAC roles, call SSAR helper, assert log | Suite envtest + SA | BR-SESS-012 |
| IT-AF-1273-003 | ctrl.SetLogger | Start envtest manager with ctrl.SetLogger, assert "controller-runtime" prefix | Isolated envtest + log buffer | BR-SESS-012 |
| IT-AF-1272-001 | Session health | Isolated envtest, start full manager, WaitForCacheSync, assert flag true | Isolated envtest | BR-SESS-011 |
| IT-AF-1272-002 | TTL metrics | envtest reconcile triggers TTL cancel, scrape metricsRegistry, assert counter | Suite envtest + metrics | BR-MONITORING-001 |

### 8.5 E2E Tests

| ID | Description | Assertion | Label | BR |
|----|-------------|-----------|-------|-----|
| E2E-AF-1274-001 | kubectl logs show structured logr output, no slog-style `msg=` | Logs have JSON structure, no `msg=` prefix | e2e, phase1, session-wiring | BR-SESS-013 |
| E2E-AF-1273-001 | kubectl logs contain pre-flight diagnostic lines | ContainSubstring("pre-flight CRD discovery"), ContainSubstring("pre-flight RBAC check") | e2e, phase1, session-wiring | BR-SESS-012 |
| E2E-AF-1273-002 | kubectl logs contain controller-runtime prefix | ContainSubstring("controller-runtime") | e2e, phase1, session-wiring | BR-SESS-012 |
| E2E-AF-1272-001 | /readyz returns 200, logs contain "session controller cache synced" | HTTP 200 + log substring | e2e, phase1, session-wiring | BR-SESS-011 |
| E2E-AF-1272-002 | /metrics contains `af_session_ttl_actions_total` | Metric family present (value may be 0) | e2e, phase1, session-wiring | BR-MONITORING-001 |

## 9. Pass/Fail Criteria

| Criterion | Threshold |
|-----------|-----------|
| All UT pass | 22/22 |
| All IT pass | 9/9 |
| All E2E pass | 5/5 |
| No `slog` imports in AF production code | 0 occurrences |
| `go build ./...` clean | Exit 0 |
| `golangci-lint run` clean | Exit 0 |
| UT coverage on changed code | >= 80% |
| IT coverage on changed code | >= 80% |

## 10. Environmental Needs

| Tier | Environment |
|------|-------------|
| UT | `go test` with fake clients, no K8s cluster |
| IT | envtest (kubebuilder) with CRDs from `config/crd/bases/` |
| E2E | Kind cluster via `make test-e2e-apifrontend` |

## 11. Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| `logr.Logger` zero-value panics on use | Runtime crash | Default to `logr.Discard()` in every constructor; audit in Refactor phase |
| Changing `NewSessionCleanupReconciler` signature breaks callers | Build failure | Update all 3 call sites (2 in main.go, all in tests) atomically |
| `slog` removal breaks `logging.NewSlogLogger` bridge | Compile error | Verify bridge is unused (confirmed: never called from main.go) |
| Pre-flight checks add latency to startup | Slower boot | Discovery + SSAR are single RPCs (<100ms each); log-only, non-blocking |
| IT log capture interferes with parallel Ginkgo processes | Flaky tests | Use per-spec log buffer, not global override |

## 12. Schedule

| Phase | Duration | Checkpoint |
|-------|----------|------------|
| P1: Test Plan | Complete | This document |
| P2: TDD Red | ~2 hours | CP-W-Red: all 36 tests compile and fail |
| P3: TDD Green | ~3 hours | CP-W-Green: all 36 tests pass, build clean |
| P4: TDD Refactor | ~1 hour | CP3: lint clean, 100 Go Mistakes audit |

## 13. Approvals

| Role | Name | Date |
|------|------|------|
| Author | AI Agent | 2026-05-24 |
| Reviewer | | |

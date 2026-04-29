# Test Plan: Config Hot-Reload via FileWatcher

> **Template Version**: 2.0 — Hybrid IEEE 829 + Kubernaut

**Test Plan Identifier**: TP-835-v1.0
**Feature**: FileWatcher-based config hot-reload for service configuration (log level)
**Version**: 1.0
**Created**: 2026-04-29
**Author**: AI Assistant
**Status**: Active
**Branch**: `main`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the config hot-reload feature introduced by Issue #835. The feature enables runtime log level changes without pod restarts via a shared `FileWatcher` component that monitors the service config file (mounted ConfigMap) for changes.

### 1.2 Objectives

1. Validate `FileWatcher` detects file changes and invokes the reload callback
2. Validate `ParseAndSetLevel` dynamically updates `zap.AtomicLevel`
3. Validate invalid log level changes are rejected without affecting the current level
4. Validate the feature works end-to-end via ConfigMap update in Kubernetes

### 1.3 Success Metrics

| Metric | Target |
|--------|--------|
| Unit test pass rate | 100% |
| Hot-reloadable fields documented | 100% |

---

## 2. References

### 2.1 Authoritative Documents

- [Issue #835](https://github.com/jordigilh/kubernaut/issues/835) — Enable hot-reload for RO service configuration
- [Issue #875](https://github.com/jordigilh/kubernaut/issues/875) — Configurable log level (KA)
- BR-PLATFORM-875 — Log level hot-reload
- DD-INFRA-001 — ConfigMap hot-reload pattern
- `docs/architecture/LOGGING_STANDARD.md`

### 2.2 Implementation Files

| File | Role |
|------|------|
| `pkg/shared/hotreload/file_watcher.go` | `FileWatcher`, `ReloadCallback` type, `NewFileWatcher` |
| `internal/config/logging.go` | `LoggingConfig`, `NewAtomicLevel()`, `ParseAndSetLevel` |
| `cmd/remediationorchestrator/main.go` | RO FileWatcher wiring (log level callback) |
| `internal/config/remediationorchestrator/config.go` | `Config.Logging` embed, `DryRun` fields |

---

## 3. Scope

### 3.1 Hot-Reloadable Fields

| Field | Service | Mechanism |
|-------|---------|-----------|
| `logging.level` | RO, KA (all services with FileWatcher) | `ParseAndSetLevel` → `zap.AtomicLevel.SetLevel` |

### 3.2 Requires Restart

All other config fields (controller settings, timeouts, DataStorage URL, routing, dry-run, TLS profile) require a pod restart.

### 3.3 Out of Scope

- OCP CR-based config reconciliation (Kubernaut Operator)
- Hot-reload for non-logging fields (future work)

---

## 4. Test Scenarios

### 4.1 Unit Tests — Shared FileWatcher

| ID | Description | BR |
|----|-------------|-----|
| UT-HR-835-001 | FileWatcher invokes callback on file change | BR-PLATFORM-875 |
| UT-HR-835-002 | FileWatcher debounces rapid changes | BR-PLATFORM-875 |
| UT-HR-835-003 | Callback error does not crash watcher | BR-PLATFORM-875 |
| UT-HR-835-004 | FileWatcher stops cleanly on context cancellation | BR-PLATFORM-875 |

### 4.2 Unit Tests — LoggingConfig

| ID | Description | BR |
|----|-------------|-----|
| UT-CFG-875-001 | Default logging config returns `info` level | BR-PLATFORM-875 |
| UT-CFG-875-002 | `ParseAndSetLevel` updates `AtomicLevel` for valid level | BR-PLATFORM-875 |
| UT-CFG-875-003 | `ParseAndSetLevel` rejects invalid level without changing current | BR-PLATFORM-875 |
| UT-CFG-875-004 | `NewAtomicLevel` maps all valid levels correctly | BR-PLATFORM-875 |
| UT-CFG-875-005 | Case-insensitive input accepted (`INFO` → `info`) | BR-PLATFORM-875 |
| UT-CFG-875-006 | `ZapLevel` returns correct `zapcore.Level` for each level | BR-PLATFORM-875 |

### 4.3 Unit Tests — KA Config

| ID | Description | BR |
|----|-------------|-----|
| UT-KA-875-001 | KA config default logging level is `info` | BR-PLATFORM-875 |
| UT-KA-875-002 | KA config parses logging level from YAML | BR-PLATFORM-875 |
| UT-KA-875-003..006 | KA-specific `ZapLevel` and validation | BR-PLATFORM-875 |

---

## 5. Existing Test Coverage

| File | Test IDs | Tier |
|------|----------|------|
| `test/unit/shared/hotreload/file_watcher_test.go` | UT-HR-835-* | Unit |
| `test/unit/config/logging_test.go` | UT-CFG-875-001..006 | Unit |
| `test/unit/kubernautagent/config/logging_test.go` | UT-KA-875-001..006 | Unit |

---

## 6. E2E Considerations

FileWatcher E2E testing in Kind is complex (requires ConfigMap update propagation + race timing). The feature is covered by:
- Unit tests for `FileWatcher` and `ParseAndSetLevel` components
- Integration between components validated via unit tests
- ConfigMap propagation is a Kubernetes-native behavior

---

## 7. Execution

```bash
# FileWatcher unit tests
go test ./pkg/shared/hotreload/... -v

# Logging config unit tests
go test ./test/unit/config/... -v -run "Logging"

# KA logging config tests
go test ./test/unit/kubernautagent/config/... -v -run "Logging"
```

---

## 8. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-29 | Initial test plan — documents existing coverage for QE readiness |

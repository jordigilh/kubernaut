# Test Plan: Configurable KA Log Level

> **Template Version**: 2.0 — Hybrid IEEE 829 + Kubernaut

**Test Plan Identifier**: TP-875-v1.0
**Feature**: Kubernaut Agent log level configurable via config file with hot-reload support
**Version**: 1.0
**Created**: 2026-04-29
**Author**: AI Assistant
**Status**: Active
**Branch**: `main`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the configurable log level feature for the Kubernaut Agent introduced by Issue #875. The KA now reads `logging.level` from its config file at startup and supports dynamic level changes via FileWatcher hot-reload.

### 1.2 Objectives

1. Validate KA respects `logging.level` from config YAML at startup
2. Validate dynamic log level changes via config file update
3. Validate lowercase canonical form with case-insensitive acceptance
4. Validate invalid level rejection without affecting current level

---

## 2. References

- [Issue #875](https://github.com/jordigilh/kubernaut/issues/875) — Make kubernaut-agent log level configurable via config file
- BR-PLATFORM-875 — Log level hot-reload
- `cmd/kubernautagent/main.go` — FileWatcher wiring
- `internal/config/logging.go` — `ParseAndSetLevel`, `NewAtomicLevel`
- TP-835-v1.0 — Config hot-reload (shared FileWatcher infrastructure)

---

## 3. Test Scenarios

### 3.1 Unit Tests — KA Config

| ID | Description | BR |
|----|-------------|-----|
| UT-KA-875-001 | Default KA logging config returns `info` level | BR-PLATFORM-875 |
| UT-KA-875-002 | KA config parses `logging.level: debug` from YAML | BR-PLATFORM-875 |
| UT-KA-875-003 | KA config validates supported levels (debug, info, warn, error) | BR-PLATFORM-875 |
| UT-KA-875-004 | KA config rejects invalid level | BR-PLATFORM-875 |
| UT-KA-875-005 | KA `ZapLevel` maps all levels correctly | BR-PLATFORM-875 |
| UT-KA-875-006 | KA config accepts uppercase input (normalized to lowercase) | BR-PLATFORM-875 |

### 3.2 Shared Config Tests

Covered by TP-835: UT-CFG-875-001..006 in `test/unit/config/logging_test.go`.

---

## 4. Existing Test Coverage

| File | Test IDs | Tier |
|------|----------|------|
| `test/unit/kubernautagent/config/logging_test.go` | UT-KA-875-001..006 | Unit |
| `test/unit/config/logging_test.go` | UT-CFG-875-001..006 | Unit |

---

## 5. Execution

```bash
go test ./test/unit/kubernautagent/config/... -v -run "Logging"
go test ./test/unit/config/... -v -run "Logging"
```

---

## 6. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-29 | Initial test plan — documents existing coverage for QE readiness |

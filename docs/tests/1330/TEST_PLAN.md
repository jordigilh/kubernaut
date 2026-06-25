# Test Plan: KA Structured JSON Logging Format (#1330)

**IEEE 829 Standard** | **Version**: 1.0 | **Status**: Draft

## 1. Test Plan Identifier

TP-CFG-1330-V1

## 2. References

- **Business Requirement**: BR-PLATFORM-1330
- **Issue**: https://github.com/jordigilh/kubernaut/issues/1330
- **Operator Issue**: kubernaut-operator#144
- **Related**: `pkg/log/logger.go` Options.Development, LOG_FORMAT env var

## 3. Introduction

The KA config struct lacks a `Format` field in its `LoggingConfig`. The
operator renders `runtime.logging.format: json` in the KA ConfigMap, but KA
ignores it since the field is not defined. This test plan covers adding
`Format` to the shared `LoggingConfig`, its validation, and wiring into the
KA logger construction.

## 4. FedRAMP Control Mapping

| Control | Test Coverage |
|---|---|
| AU-3 (Content of Audit Records) | UT-CFG-1330-001, UT-CFG-1330-004: structured JSON ensures machine-parseable audit trail |
| CM-6 (Configuration Settings) | UT-KA-1330-005, UT-KA-1330-006: log format is configurable via YAML |
| CM-3 (Configuration Change Control) | UT-CFG-1330-002: format validation prevents misconfiguration |

## 5. Test Items

| Item | Location |
|---|---|
| `Format` field on `LoggingConfig` | `internal/config/logging.go` |
| `ValidFormats` map | `internal/config/logging.go` |
| `IsConsoleFormat()` method | `internal/config/logging.go` |
| `DefaultLoggingConfig()` format default | `internal/config/logging.go` |
| KA logger construction wiring | `cmd/kubernautagent/main.go` line 121 |

## 6. Test Scenarios

### Unit Tests (Logic) -- Shared LoggingConfig

| Test ID | Description | FedRAMP | Pass Criteria |
|---|---|---|---|
| UT-CFG-1330-001 | `DefaultLoggingConfig()` returns `Format: "json"` | AU-3 | `DefaultLoggingConfig().Format == "json"` |
| UT-CFG-1330-002 | `Validate()` accepts "json"/"console", rejects "yaml"/"text" | CM-3 | Valid formats pass, invalid return error containing "log format" |
| UT-CFG-1330-003 | `IsConsoleFormat()` returns correct boolean | AU-3 | `true` for "console"/"CONSOLE", `false` for "json"/"" |
| UT-CFG-1330-004 | Empty format defaults to JSON behavior | AU-3 | `IsConsoleFormat() == false` when `Format == ""` |

### Unit Tests (Logic) -- KA Config Integration

| Test ID | Description | FedRAMP | Pass Criteria |
|---|---|---|---|
| UT-KA-1330-005 | `runtime.logging.format: console` YAML round-trip | CM-6 | `cfg.Runtime.Logging.Format == "console"` after parsing |
| UT-KA-1330-006 | KA `Validate()` delegates format validation | CM-6 | Invalid format causes `Validate()` error |

### Pyramid Invariant

- **UT** proves logic: format parsing, validation, IsConsoleFormat helper
- **Wiring** is verified via CHECKPOINT W: `Development: cfg.Runtime.Logging.IsConsoleFormat()` passed to `Options` in `cmd/kubernautagent/main.go`
- No IT needed: the wiring is a boolean flag passed to the logger factory with no I/O dispatch

## 7. Pass/Fail Criteria

All 6 unit tests must pass. CHECKPOINT W must confirm `IsConsoleFormat()` is called in `cmd/`.

## 8. Environmental Needs

- Go test environment with Ginkgo/Gomega
- No external dependencies (no K8s cluster, no network)

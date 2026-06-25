# Test Plan: KA Configurable Graceful Shutdown Drain Duration (#1329)

**IEEE 829 Standard** | **Version**: 1.0 | **Status**: Draft

## 1. Test Plan Identifier

TP-KA-1329-V1

## 2. References

- **Business Requirement**: BR-PLATFORM-1329
- **Issue**: https://github.com/jordigilh/kubernaut/issues/1329
- **Operator Issue**: kubernaut-operator#142
- **Related**: AF implementation in `pkg/apifrontend/config/config.go` (ShutdownConfig)

## 3. Introduction

The Kubernaut Agent binary uses a hardcoded 10-second shutdown timeout in
`cmd/kubernautagent/main.go`. The operator already renders
`runtime.shutdown.drainSeconds: 30` in the KA ConfigMap, but KA ignores this
value. This test plan covers the addition of a `ShutdownConfig` struct to
the KA config, its validation, default values, and wiring into the shutdown
sequence.

## 4. FedRAMP Control Mapping

| Control | Test Coverage |
|---|---|
| CM-6 (Configuration Settings) | UT-KA-1329-001, UT-KA-1329-002: shutdown duration is configurable via YAML |
| SC-5 (Denial of Service Protection) | UT-KA-1329-003: validation rejects invalid drain values that could cause premature termination |
| CM-3 (Configuration Change Control) | UT-KA-1329-004, UT-KA-1329-005: config-driven shutdown timeout replaces hardcoded value |

## 5. Test Items

| Item | Location |
|---|---|
| `ShutdownConfig` struct | `internal/kubernautagent/config/config.go` |
| `ShutdownConfig` validation | `internal/kubernautagent/config/config.go` `Validate()` |
| `DefaultConfig()` shutdown defaults | `internal/kubernautagent/config/config.go` |
| `shutdownTimeout()` helper | `cmd/kubernautagent/main.go` |
| Shutdown context wiring | `cmd/kubernautagent/main.go` line 648 |

## 6. Test Scenarios

### Unit Tests (Logic)

| Test ID | Description | FedRAMP | Pass Criteria |
|---|---|---|---|
| UT-KA-1329-001 | `runtime.shutdown.drainSeconds` YAML field round-trips correctly | CM-6 | `cfg.Runtime.Shutdown.DrainSeconds == 45` after parsing `drainSeconds: 45` |
| UT-KA-1329-002 | `DefaultConfig()` sets `drainSeconds` to 30 | CM-6 | `DefaultConfig().Runtime.Shutdown.DrainSeconds == 30` |
| UT-KA-1329-003 | `Validate()` rejects `drainSeconds <= 0` | SC-5 | `Validate()` returns error containing `"runtime.shutdown.drainSeconds"` |
| UT-KA-1329-004 | `shutdownTimeout()` returns configured value | CM-3 | `shutdownTimeout(cfg) == 3*time.Second` when `DrainSeconds == 3` |
| UT-KA-1329-005 | `shutdownTimeout()` returns 30s default on zero | CM-3 | `shutdownTimeout(cfg) == 30*time.Second` when `DrainSeconds == 0` |

### Pyramid Invariant

- **UT** proves logic: config parsing, validation, timeout computation
- **Wiring** is verified via CHECKPOINT W: `shutdownTimeout(cfg)` is called in `cmd/kubernautagent/main.go` shutdown sequence
- No IT needed: the wiring is a single `context.WithTimeout` call with no I/O dispatch

## 7. Pass/Fail Criteria

All 5 unit tests must pass. CHECKPOINT W must confirm production caller in `cmd/`.

## 8. Environmental Needs

- Go test environment with Ginkgo/Gomega
- No external dependencies (no K8s cluster, no network)

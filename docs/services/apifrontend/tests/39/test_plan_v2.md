# Test Plan: Configuration Extension and Log-Level Hot-Reload

**Test Plan Identifier:** TP-AF-039-v2
**Issue:** [#39](https://github.com/jordigilh/kubernaut-apifrontend/issues/39)
**Version:** 2.0
**Date:** 2026-05-06
**Status:** Draft
**Supersedes:** TP-AF-039 v1.0 (base config loading — already implemented in PR #80)

---

## 1. Introduction

This test plan extends TP-AF-039 v1.0 to cover the remaining acceptance criteria from issue #39: extended config fields (Auth, Logging, RateLimit, Shutdown), validation of those fields, and log-level hot-reload via fsnotify file watching.

### 1.1 Scope

- Extend `Config` struct with `Auth`, `Logging`, `RateLimit`, `Shutdown` fields
- Extend `Validate()` to enforce: OIDC issuer URL syntax, valid log level, positive rate limit values, drain period constraint
- `internal/config/hotreload.go`: fsnotify-based file watcher adapted from kubernaut DD-INFRA-001 pattern
- Runtime log-level change: ConfigMap update triggers `zap.AtomicLevel.SetLevel()`
- Structured log on level change: `"log level changed" old=INFO new=DEBUG`

### 1.2 References

- Issue #39: Configuration validation and log-level hot-reload (ADR-030, LOGGING_STANDARD)
- kubernaut `pkg/shared/hotreload/file_watcher.go` — DD-INFRA-001 pattern (324 lines)
- `go.mod`: `github.com/fsnotify/fsnotify v1.7.0` (indirect, to be promoted)
- Existing implementation: `internal/config/config.go` (121 lines, from PR #80)
- 100 Go Mistakes (Teiva Harsanyi) — refactoring validation reference

### 1.3 Definitions

| Term | Definition |
|------|-----------|
| AtomicLevel | `zap.AtomicLevel` — thread-safe log level that can be changed at runtime |
| FileWatcher | Component that watches a mounted ConfigMap file via fsnotify and triggers callbacks on change |
| Debounce | Coalescing multiple rapid fsnotify events (WRITE+CHMOD) into a single reload |
| SHA256 hash | Used to detect content-actually-changed vs spurious events |

---

## 2. Test Items

| Item | Package | Source |
|------|---------|--------|
| `AuthConfig` (struct) | `internal/config` | New |
| `LoggingConfig` (struct) | `internal/config` | New |
| `RateLimitConfig` (struct) | `internal/config` | New |
| `ShutdownConfig` (struct) | `internal/config` | New |
| Extended `Validate()` | `internal/config` | Modified |
| `FileWatcher` (struct) | `internal/config` | New |
| `NewFileWatcher()` | `internal/config` | New |
| `(*FileWatcher).Start()` | `internal/config` | New |
| `(*FileWatcher).Stop()` | `internal/config` | New |
| Log-level wiring | `cmd/apifrontend` | Modified |

---

## 3. Features to Be Tested

### 3.1 Business Acceptance Criteria (remaining from Issue #39)

| ID | Criterion | Testable |
|----|-----------|----------|
| BAC-8 | Config struct includes Auth/Logging/RateLimit/Shutdown fields | Yes |
| BAC-9 | Validate() catches invalid OIDC issuer URL | Yes |
| BAC-10 | Validate() catches invalid log level | Yes |
| BAC-11 | Validate() catches non-positive rate limit values | Yes |
| BAC-12 | Log level changeable at runtime via ConfigMap update (no restart) | Yes |
| BAC-13 | File watcher on ConfigMap mount path | Yes |
| BAC-14 | Integration test: ConfigMap change -> log level changes within 30s | Yes |

### 3.2 Features by Tier

#### Tier 1: Extended Config Fields

| ID | Feature | Acceptance Criteria |
|----|---------|-------------------|
| F-39.19 | AuthConfig with IssuerURL, Audience fields | YAML deserializes into struct |
| F-39.20 | LoggingConfig with Level field | Accepts DEBUG/INFO/WARN/ERROR |
| F-39.21 | RateLimitConfig with IPRequestsPerSec, UserRequestsPerSec | Positive integers |
| F-39.22 | ShutdownConfig with DrainSeconds field | Positive integer |
| F-39.23 | Extended DefaultConfig provides sensible defaults | Level=INFO, DrainSeconds=15 |

#### Tier 2: Extended Validation

| ID | Feature | Acceptance Criteria |
|----|---------|-------------------|
| F-39.24 | Validate rejects invalid OIDC issuer URL (no scheme) | Error includes "auth.issuerURL" |
| F-39.25 | Validate rejects unknown log level | Error includes "logging.level" |
| F-39.26 | Validate rejects zero/negative rate limits | Error includes "rateLimit" |
| F-39.27 | Validate rejects zero drain seconds | Error includes "shutdown.drainSeconds" |
| F-39.28 | Validate accepts empty auth (optional in dev) | No error when auth fields empty |

#### Tier 3: FileWatcher (hot-reload)

| ID | Feature | Acceptance Criteria |
|----|---------|-------------------|
| F-39.29 | NewFileWatcher requires non-empty path | Returns error for empty path |
| F-39.30 | NewFileWatcher requires non-nil callback | Returns error for nil callback |
| F-39.31 | Start loads initial file content | Callback invoked with file bytes |
| F-39.32 | Start returns error if file missing | Error wraps os.ErrNotExist |
| F-39.33 | File change triggers callback | Callback invoked with new content |
| F-39.34 | Unchanged content skipped (hash compare) | Callback NOT invoked on same-hash |
| F-39.35 | Callback error keeps old content | GetLastContent returns previous |
| F-39.36 | Stop is graceful | No goroutine leak, doneCh closed |
| F-39.37 | Multiple rapid events debounced | Single callback for burst writes |

#### Tier 4: Log-Level Hot-Reload Integration

| ID | Feature | Acceptance Criteria |
|----|---------|-------------------|
| F-39.38 | Config change updates AtomicLevel | zap level changes after file write |
| F-39.39 | Level change logged structured | Log contains old=X new=Y |
| F-39.40 | Invalid level in update rejected | Old level retained, error logged |
| F-39.41 | Watcher wired in main.go | Start called, Stop deferred on shutdown |

---

## 4. Features Not Tested

| Feature | Reason |
|---------|--------|
| Auth token validation | Covered by `internal/auth` tests |
| Rate limit enforcement | Covered by `internal/ratelimit` tests |
| TLS cert path checks | No TLS in current scope (service mesh handles) |
| Kubernetes ConfigMap update propagation timing | Infrastructure concern (~60s) |
| Full main.go startup (requires network) | Integration test covers config path only |

---

## 5. Approach

### 5.1 Test Methodology

TDD in three phases (Red, Green, Refactor). Tests use standard `testing` package with table-driven patterns.

### 5.2 Test Levels

| Level | Scope | Tools |
|-------|-------|-------|
| Unit | Extended fields, Validate(), FileWatcher | `testing`, `t.TempDir()`, `os.WriteFile` |
| Integration | Log-level change end-to-end | `fsnotify`, `zap.AtomicLevel`, `Eventually` pattern |

### 5.3 FileWatcher Test Strategy

FileWatcher tests use `t.TempDir()` with real files and fsnotify. To avoid flakiness:
- Debounce timer set to 200ms (matching kubernaut pattern)
- Tests use polling with 5s timeout (`deadline := time.Now().Add(5 * time.Second)`)
- No `time.Sleep` — use condition polling loops
- SHA256 comparison prevents spurious reloads

---

## 6. Test Cases

### 6.1 Extended Config Fields (5 tests)

| ID | Description | Priority | BAC |
|----|-------------|----------|-----|
| UT-AF-039-031 | Load parses auth.issuerURL and auth.audience | P0 | BAC-8 |
| UT-AF-039-032 | Load parses logging.level (case-insensitive) | P0 | BAC-8 |
| UT-AF-039-033 | Load parses rateLimit.ipRequestsPerSec and userRequestsPerSec | P0 | BAC-8 |
| UT-AF-039-034 | Load parses shutdown.drainSeconds | P0 | BAC-8 |
| UT-AF-039-035 | Extended DefaultConfig provides INFO level and 15s drain | P0 | BAC-8 |

### 6.2 Extended Validation (7 tests)

| ID | Description | Priority | BAC |
|----|-------------|----------|-----|
| UT-AF-039-036 | Validate rejects auth.issuerURL without scheme | P0 | BAC-9 |
| UT-AF-039-037 | Validate accepts empty auth (dev mode) | P1 | BAC-9 |
| UT-AF-039-038 | Validate rejects invalid logging.level (e.g. "TRACE") | P0 | BAC-10 |
| UT-AF-039-039 | Validate accepts DEBUG, INFO, WARN, ERROR levels | P0 | BAC-10 |
| UT-AF-039-040 | Validate rejects zero ipRequestsPerSec | P0 | BAC-11 |
| UT-AF-039-041 | Validate rejects negative userRequestsPerSec | P0 | BAC-11 |
| UT-AF-039-042 | Validate rejects zero shutdown.drainSeconds | P1 | BAC-11 |

### 6.3 FileWatcher (9 tests)

| ID | Description | Priority | BAC |
|----|-------------|----------|-----|
| UT-AF-039-043 | NewFileWatcher returns error for empty path | P0 | BAC-13 |
| UT-AF-039-044 | NewFileWatcher returns error for nil callback | P0 | BAC-13 |
| UT-AF-039-045 | Start loads initial content and invokes callback | P0 | BAC-13 |
| UT-AF-039-046 | Start returns error when file does not exist | P0 | BAC-13 |
| UT-AF-039-047 | File modification triggers callback with new content | P0 | BAC-12 |
| UT-AF-039-048 | Same content write does not trigger callback (hash match) | P1 | BAC-12 |
| UT-AF-039-049 | Callback error preserves previous content | P0 | BAC-12 |
| UT-AF-039-050 | Stop terminates watch loop without goroutine leak | P0 | BAC-13 |
| UT-AF-039-051 | Rapid successive writes produce single callback (debounce) | P1 | BAC-12 |

### 6.4 Log-Level Integration (4 tests)

| ID | Description | Priority | BAC |
|----|-------------|----------|-----|
| UT-AF-039-052 | Config update changes zap AtomicLevel from INFO to DEBUG | P0 | BAC-12,14 |
| UT-AF-039-053 | Level change produces structured log with old and new | P0 | BAC-12 |
| UT-AF-039-054 | Invalid level in config update retains previous level | P0 | BAC-12 |
| UT-AF-039-055 | FileWatcher started and stopped in main.go lifecycle | P1 | BAC-13 |

---

## 7. Pass/Fail Criteria

### 7.1 Pass Criteria

- All 25 new test cases pass with `-race` flag
- Combined coverage (v1 + v2 tests) >= 80% of `internal/config/*.go`
- `golangci-lint run` reports 0 new errors
- `go mod tidy` produces no changes
- `fsnotify` promoted from indirect to direct dependency
- No goroutine leaks (verified by test framework or `goleak`)

### 7.2 Fail Criteria

- Any test fails or exhibits flakiness (> 1 failure in 10 runs)
- FileWatcher Start() blocks indefinitely
- AtomicLevel not thread-safe under `-race`
- Coverage below 80% on new code

---

## 8. Suspension and Resumption

### 8.1 Suspension

- fsnotify v1.7.0 has known bugs on darwin (unlikely, well-tested)
- zap.AtomicLevel API changes (pinned version)

### 8.2 Resumption

- Upgrade fsnotify or use polling fallback
- Pin zap version in go.mod

---

## 9. Test Deliverables

| Deliverable | Location |
|-------------|----------|
| FileWatcher unit tests | `internal/config/hotreload_test.go` |
| Extended config tests | `internal/config/config_test.go` (appended) |
| Integration test | `internal/config/integration_test.go` |
| This test plan | `docs/tests/39/test_plan_v2.md` |

---

## 10. Environmental Needs

| Requirement | Details |
|-------------|---------|
| Go version | 1.25.6 |
| Dependencies | `github.com/fsnotify/fsnotify v1.7.0` (promote to direct) |
| File system | tmpdir with writable files for fsnotify events |
| CI environment | GitHub Actions (linux/amd64, darwin/arm64 for local dev) |

---

## 11. Refactoring Phase — 100 Go Mistakes Validation

### Applicable to FileWatcher

| # | Mistake | Verification |
|---|---------|-------------|
| 1 | Variable shadowing | No shadowed `err` in watchLoop |
| 2 | Unnecessary nesting | Early returns in Start(), handleFileChange() |
| 3 | No init functions | No init() in hotreload.go |
| 15 | Missing docs | All exported: NewFileWatcher, Start, Stop, GetLastHash |
| 42 | Wrong receiver | FileWatcher methods use pointer receiver |
| 48 | No panics | All errors returned or logged |
| 49 | Error wrapping | fmt.Errorf("context: %w", err) throughout |
| 53 | All errors handled | watcher.Close() error checked |
| 58 | Goroutine leak | doneCh pattern ensures watchLoop exits |
| 61 | Missing channel direction | stopCh is `chan struct{}` (internal only) |
| 77 | Mixing concerns | File I/O separate from callback logic |
| 86 | No sleeping in tests | Polling loop with deadline, not time.Sleep |

---

## 12. Implementation Phases

### Phase 1: TDD Red

Write 25 failing tests in `internal/config/hotreload_test.go` and append extended validation tests to `config_test.go`. Create minimal stubs so tests compile.

### Phase 2: TDD Green

Implement `AuthConfig`, `LoggingConfig`, `RateLimitConfig`, `ShutdownConfig` structs. Extend `Validate()`. Implement `FileWatcher` (adapted from kubernaut pattern). Wire into main.go.

### Phase 3: TDD Refactor

Run 100 Go Mistakes checklist. Verify coverage. Run linter. Check for goroutine leaks.

---

## 13. Traceability Matrix

| BAC | Test Cases | Count |
|-----|-----------|-------|
| BAC-8 | UT-AF-039-031 to 035 | 5 |
| BAC-9 | UT-AF-039-036, 037 | 2 |
| BAC-10 | UT-AF-039-038, 039 | 2 |
| BAC-11 | UT-AF-039-040, 041, 042 | 3 |
| BAC-12 | UT-AF-039-047-054 | 8 |
| BAC-13 | UT-AF-039-043-046, 050, 051, 055 | 7 |
| BAC-14 | UT-AF-039-052 | 1 |

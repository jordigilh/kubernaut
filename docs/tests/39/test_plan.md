# Test Plan: ConfigMap-Based Configuration — Validation, Loading, and envOr Removal

**Test Plan Identifier:** TP-AF-039
**Issue:** [#39](https://github.com/jordigilh/kubernaut-apifrontend/issues/39)
**Version:** 1.0
**Date:** 2026-05-05
**Status:** Draft

---

## 1. Introduction

This test plan validates the ConfigMap-mounted YAML configuration system for the kubernaut API Frontend. It replaces the environment variable (`envOr`) pattern with file-based configuration loaded at startup via a `--config` CLI flag, matching the established kubernaut pattern (DD-INFRA-001). The implementation includes a typed Config struct, validation logic, sensible defaults, and startup wiring in `main.go`.

### 1.1 Scope

- Config struct definition with YAML deserialization (`internal/config`)
- `Load(data []byte)` parser with default merging
- `Validate()` method enforcing required fields and value constraints
- `DefaultConfig()` factory for production defaults
- CLI flag (`--config`) wiring in `cmd/apifrontend/main.go`
- Removal of all `envOr()` calls and `os.Getenv` usage
- Derived field resolution (e.g., AgentCard URL from Server.Port)
- Startup logging of effective configuration
- Error messaging for missing/invalid config files

### 1.2 References

- Issue #39: Configuration validation and log-level hot-reload (ADR-030, LOGGING_STANDARD)
- kubernaut `internal/kubernautagent/config/config.go` — reference implementation
- kubernaut `pkg/shared/hotreload/file_watcher.go` — DD-INFRA-001 pattern
- `internal/auth/config.go` — existing file-based config in this repo
- `internal/ratelimit/config.go` — existing file-based config in this repo
- PR #80 review findings: REL-3 (undocumented env vars), architectural constraint (no env vars)
- 100 Go Mistakes (Teiva Harsanyi) — refactoring validation reference

### 1.3 Definitions

| Term | Definition |
|------|-----------|
| ConfigMap | Kubernetes resource mounted as a file into the container |
| envOr | Helper function `func envOr(key, fallback string) string` being removed |
| Fail-fast | Application exits immediately on invalid configuration at startup |
| Hot-reload | Runtime config update without pod restart (deferred to PR7+) |

---

## 2. Test Items

| Item | Package | Source |
|------|---------|--------|
| `Config` (struct) | `internal/config` | New |
| `ServerConfig` (struct) | `internal/config` | New |
| `AgentConfig` (struct) | `internal/config` | New |
| `MCPConfig` (struct) | `internal/config` | New |
| `AgentCardConfig` (struct) | `internal/config` | New |
| `Load(data []byte)` | `internal/config` | New |
| `DefaultConfig()` | `internal/config` | New |
| `Config.Validate()` | `internal/config` | New |
| `Config.ResolveDefaults()` | `internal/config` | New |
| `run()` (config wiring) | `cmd/apifrontend` | Modified |

---

## 3. Features to Be Tested

### 3.1 Business Acceptance Criteria (from Issue #39)

| ID | Criterion | Testable | Source |
|----|-----------|----------|--------|
| BAC-1 | Config struct defined with all operational parameters | Yes | Issue #39 |
| BAC-2 | `Validate()` catches all invalid configurations before server starts | Yes | Issue #39 |
| BAC-3 | Invalid config at startup produces structured error log + exit(1) | Yes | Issue #39 |
| BAC-4 | Config loaded from YAML file (ConfigMap mount path) | Yes | Issue #39 |
| BAC-5 | Unit tests for config validation (valid/invalid cases) | Yes | Issue #39 |
| BAC-6 | No environment variables used for configuration | Yes | PR #80 architectural constraint |
| BAC-7 | Config file path specified via `--config` CLI flag | Yes | kubernaut pattern |

### 3.2 Features by Tier

#### Tier 1: Config Loading (core parsing)

| ID | Feature | Acceptance Criteria |
|----|---------|-------------------|
| F-39.1 | YAML parsing | Valid YAML bytes are deserialized into Config struct |
| F-39.2 | Default merging | Omitted fields receive `DefaultConfig()` values |
| F-39.3 | Unknown key handling | Unknown YAML keys are silently ignored (forward-compat) |
| F-39.4 | Empty input | Empty YAML input produces DefaultConfig with no error |
| F-39.5 | Malformed YAML | Invalid YAML returns descriptive parse error |

#### Tier 2: Config Validation (fail-fast)

| ID | Feature | Acceptance Criteria |
|----|---------|-------------------|
| F-39.6 | Port range | Port must be 1-65535; out-of-range rejected |
| F-39.7 | Required agent fields | `gcpProject` required in production (non-empty) |
| F-39.8 | URL syntax | `kaBaseURL`, `kaMCPEndpoint`, `dsBaseURL` must be valid URLs |
| F-39.9 | MCP flag | `mcp.enabled` accepts true/false (boolean) |
| F-39.10 | DefaultConfig valid | `DefaultConfig()` passes `Validate()` |
| F-39.11 | Multiple errors | First validation error returned (fail-fast, not accumulated) |

#### Tier 3: Derived Defaults and Resolution

| ID | Feature | Acceptance Criteria |
|----|---------|-------------------|
| F-39.12 | AgentCard URL default | If `agentCard.url` empty, derived from `server.port` |
| F-39.13 | Port as integer | Port stored as int, formatted to string for net listener |
| F-39.14 | filepath.Clean | Config path cleaned before read (path traversal prevention) |

#### Tier 4: Startup Integration

| ID | Feature | Acceptance Criteria |
|----|---------|-------------------|
| F-39.15 | Missing file error | Missing config file returns error with path and flag hint |
| F-39.16 | Unreadable file error | Permission-denied returns wrapped OS error |
| F-39.17 | Config logged at startup | Effective config values logged at INFO (no secrets) |
| F-39.18 | envOr removed | No `os.Getenv` or `envOr` calls remain in codebase |

---

## 4. Features Not Tested

| Feature | Reason |
|---------|--------|
| Log-level hot-reload | Deferred to PR7+ per issue #39 schedule |
| Auth config file loading | Separate concern; `auth.LoadConfigFromFile` already tested |
| Ratelimit config file loading | Separate concern; already tested |
| TLS cert path existence checks | No TLS in current scope (handled by service mesh) |
| Kubernetes ConfigMap controller | Infrastructure concern (K8s handles mount lifecycle) |
| Drain period vs terminationGracePeriod | No graceful drain in current scope |

---

## 5. Approach

### 5.1 Test Methodology

Test-Driven Development (TDD) in three phases:

1. **Red phase:** Write all test cases as failing tests. Tests compile but assertions fail.
2. **Green phase:** Implement minimal production code to make each test pass.
3. **Refactor phase:** Clean code against 100-go-mistakes checklist (Section 11).

### 5.2 Test Levels

| Level | Scope | Tools |
|-------|-------|-------|
| Unit | `Load()`, `Validate()`, `DefaultConfig()`, `ResolveDefaults()` | `testing`, table-driven tests |
| Integration | `run()` with temp config files | `os.CreateTemp`, `flag` reset |
| Static analysis | Codebase-wide envOr removal verification | `go vet`, `golangci-lint`, `grep` |

### 5.3 Test Infrastructure

- **Temp files:** `os.CreateTemp` + `t.Cleanup` for integration tests
- **Table-driven tests:** All validation tests use `[]struct{name, input, wantErr}` pattern (#85)
- **No init functions:** Tests use explicit setup, never `init()` (#3)
- **Filename as input:** `Load()` accepts `[]byte` not filename (#46); file I/O tested separately
- **Error wrapping:** All errors checked with `errors.Is` / contains-string, never `==` (#50, #51)

### 5.4 Anti-Patterns Avoided

| Anti-Pattern | Mitigation |
|--------------|-----------|
| Tautology tests | Every test asserts observable behavior, never struct field assignment |
| Testing implementation | Tests verify behavior (valid config loads, invalid rejected) not internal struct layout |
| Sleeping in tests | No time-dependent tests in this scope (#86) |
| Global state | No package-level vars mutated; `DefaultConfig()` returns fresh value |
| Brittle string matching | Error assertions use `strings.Contains`, not exact match |

---

## 6. Test Cases

### 6.1 Config Loading — `Load()` (8 tests)

| ID | Description | Priority | BAC | Feature |
|----|-------------|----------|-----|---------|
| UT-AF-039-001 | Load parses valid full YAML into Config | P0 | BAC-4 | F-39.1 |
| UT-AF-039-002 | Load applies defaults for omitted fields | P0 | BAC-4 | F-39.2 |
| UT-AF-039-003 | Load returns error for malformed YAML | P0 | BAC-2 | F-39.5 |
| UT-AF-039-004 | Load with empty input returns DefaultConfig | P0 | BAC-4 | F-39.4 |
| UT-AF-039-005 | Load ignores unknown YAML keys | P1 | BAC-4 | F-39.3 |
| UT-AF-039-006 | Load preserves zero-value booleans (mcp.enabled=false) | P0 | BAC-4 | F-39.9 |
| UT-AF-039-007 | Load parses port as integer | P0 | BAC-1 | F-39.13 |
| UT-AF-039-008 | Load with partial YAML merges with defaults | P1 | BAC-4 | F-39.2 |

### 6.2 Config Validation — `Validate()` (12 tests)

| ID | Description | Priority | BAC | Feature |
|----|-------------|----------|-----|---------|
| UT-AF-039-009 | Validate accepts DefaultConfig | P0 | BAC-2 | F-39.10 |
| UT-AF-039-010 | Validate rejects port < 1 | P0 | BAC-2 | F-39.6 |
| UT-AF-039-011 | Validate rejects port > 65535 | P0 | BAC-2 | F-39.6 |
| UT-AF-039-012 | Validate rejects port = 0 | P0 | BAC-2 | F-39.6 |
| UT-AF-039-013 | Validate rejects empty kaBaseURL | P0 | BAC-2 | F-39.8 |
| UT-AF-039-014 | Validate rejects malformed kaBaseURL (no scheme) | P1 | BAC-2 | F-39.8 |
| UT-AF-039-015 | Validate rejects empty kaMCPEndpoint | P0 | BAC-2 | F-39.8 |
| UT-AF-039-016 | Validate rejects empty dsBaseURL | P0 | BAC-2 | F-39.8 |
| UT-AF-039-017 | Validate rejects malformed dsBaseURL | P1 | BAC-2 | F-39.8 |
| UT-AF-039-018 | Validate accepts valid complete config | P0 | BAC-2 | F-39.6-9 |
| UT-AF-039-019 | Validate error message includes field name | P1 | BAC-3 | F-39.11 |
| UT-AF-039-020 | Validate returns first error only (fail-fast) | P1 | BAC-3 | F-39.11 |

### 6.3 Default Resolution — `ResolveDefaults()` (4 tests)

| ID | Description | Priority | BAC | Feature |
|----|-------------|----------|-----|---------|
| UT-AF-039-021 | ResolveDefaults sets agentCard.url from port when empty | P0 | BAC-1 | F-39.12 |
| UT-AF-039-022 | ResolveDefaults preserves explicit agentCard.url | P0 | BAC-1 | F-39.12 |
| UT-AF-039-023 | ResolveDefaults is idempotent | P1 | BAC-1 | F-39.12 |
| UT-AF-039-024 | ResolveDefaults called before Validate | P1 | BAC-2 | F-39.12 |

### 6.4 Startup Integration — `run()` (6 tests)

| ID | Description | Priority | BAC | Feature |
|----|-------------|----------|-----|---------|
| UT-AF-039-025 | run() loads config from --config flag path | P0 | BAC-4,7 | F-39.15 |
| UT-AF-039-026 | run() returns error when config file missing | P0 | BAC-3 | F-39.15 |
| UT-AF-039-027 | run() error includes file path and --config hint | P0 | BAC-3 | F-39.15 |
| UT-AF-039-028 | run() returns error for invalid config content | P0 | BAC-3 | F-39.16 |
| UT-AF-039-029 | run() uses filepath.Clean on config path | P1 | BAC-7 | F-39.14 |
| UT-AF-039-030 | No envOr or os.Getenv in final codebase | P0 | BAC-6 | F-39.18 |

---

## 7. Pass/Fail Criteria

### 7.1 Pass Criteria

- All 30 test cases pass with `-race` flag
- Coverage >= 80% of `internal/config/*.go`
- Coverage >= 80% of config-loading path in `cmd/apifrontend/main.go`
- `golangci-lint run` reports 0 new errors
- No `panic()` in production code paths
- `go mod tidy` produces no changes
- Zero `envOr` or `os.Getenv` calls remain in Go source (grep verification)
- Refactoring checklist (Section 11) passes with zero violations

### 7.2 Fail Criteria

- Any test case fails
- Coverage drops below 80% per package
- `envOr` or `os.Getenv` found in any `.go` file
- Config validation allows invalid port (0, negative, >65535)
- Config validation allows empty required URL fields
- DefaultConfig() fails its own Validate()
- Error messages expose no file path context

---

## 8. Suspension and Resumption Criteria

### 8.1 Suspension

- `gopkg.in/yaml.v3` cannot handle the required struct tags
- `flag` package cannot be reset in tests without build tag hacks
- Breaking change in `internal/agent.AgentConfig` interface

### 8.2 Resumption

- Alternative YAML library identified (e.g., `sigs.k8s.io/yaml`)
- Integration test approach changed to subprocess-based
- AgentConfig interface stabilized

---

## 9. Test Deliverables

| Deliverable | Location |
|-------------|----------|
| Test source — config unit tests | `internal/config/config_test.go` |
| Test source — main integration | `cmd/apifrontend/main_test.go` |
| Coverage report | `go test -coverprofile=coverage.out ./internal/config/...` |
| Lint report | `golangci-lint run ./internal/config/... ./cmd/apifrontend/...` |
| This test plan | `docs/tests/39/test_plan.md` |
| Sample config (test fixture) | `internal/config/testdata/valid.yaml` |
| Sample config (production) | `deploy/configmap.yaml` |

---

## 10. Environmental Needs

| Requirement | Details |
|-------------|---------|
| Go version | 1.23+ |
| Dependencies | `gopkg.in/yaml.v3` (already in go.mod) |
| External services | None (pure unit tests, no network) |
| CI environment | GitHub Actions (linux/amd64) |
| File system | Temp dir for integration tests (`t.TempDir()`) |

---

## 11. Refactoring Phase — 100 Go Mistakes Validation Checklist

The refactoring phase validates that production code is free of the following applicable Go mistakes. Each item must be verified before the PR is marked ready.

### Code and Project Organization

| # | Mistake | Verification | Applies To |
|---|---------|-------------|-----------|
| 1 | Unintended variable shadowing | No shadowed `err` or `cfg` in nested blocks | `config.go`, `main.go` |
| 2 | Unnecessary nested code | Happy path aligned left; early returns for errors | `Load()`, `Validate()` |
| 3 | Misusing init functions | No `init()` in `internal/config`; config loaded explicitly | Package-wide |
| 5 | Interface pollution | No interface defined for Config (concrete struct only) | `config.go` |
| 7 | Returning interfaces | `Load()` returns `*Config`, not an interface | `config.go` |
| 11 | Not using functional options | Not applicable (Config is data-only, no constructor options needed) | N/A |
| 12 | Project misorganization | `internal/config` is a focused, single-purpose package | Package layout |
| 13 | Creating utility packages | Package named `config` (what it provides), not `utils` | Package naming |
| 15 | Missing code documentation | All exported types/functions have GoDoc | `config.go` |
| 16 | Not using linters | `golangci-lint run` passes clean | All files |

### Data Types

| # | Mistake | Verification | Applies To |
|---|---------|-------------|-----------|
| 22 | nil vs empty slice confusion | No slice fields in Config; N/A | N/A |
| 27 | Inefficient map initialization | No maps in Config struct | N/A |
| 29 | Comparing values incorrectly | Error comparisons use `strings.Contains` or `errors.Is` | Tests |

### Functions and Methods

| # | Mistake | Verification | Applies To |
|---|---------|-------------|-----------|
| 42 | Wrong receiver type | `Validate()` and `ResolveDefaults()` use pointer receiver (mutates) | `config.go` |
| 43 | Named result parameters | Used only where it improves GoDoc clarity | `config.go` |
| 46 | Filename as function input | `Load()` accepts `[]byte`, not a file path | `config.go` |
| 47 | Defer argument evaluation | No deferred calls with evaluated args in config code | `main.go` |

### Error Management

| # | Mistake | Verification | Applies To |
|---|---------|-------------|-----------|
| 48 | Panicking | No `panic()` in config code; all errors returned | `config.go`, `main.go` |
| 49 | Ignoring when to wrap errors | All errors wrapped with `fmt.Errorf("context: %w", err)` | `Load()`, `main.go` |
| 50 | Comparing error type inaccurately | Tests use `errors.Is` or string matching, never `==` | Tests |
| 52 | Handling error twice | Errors logged OR returned, never both | `main.go` |
| 53 | Not handling an error | Every `os.ReadFile`, `yaml.Unmarshal` error checked | `config.go` |
| 54 | Not handling defer errors | No defer in config loading (file read is synchronous) | N/A |

### Standard Library

| # | Mistake | Verification | Applies To |
|---|---------|-------------|-----------|
| 75 | Wrong time duration | No time.Duration fields in config (deferred to hot-reload PR) | N/A |
| 77 | JSON handling mistakes | YAML not JSON; `yaml.v3` handles struct tags correctly | `config.go` |
| 79 | Not closing transient resources | `os.ReadFile` returns `[]byte` (no open handle to close) | `main.go` |
| 81 | Default HTTP client/server | `http.Server` configured with explicit timeouts (already done) | `main.go` |
| 100 | Go in Docker/K8s | Config from ConfigMap mount (not env vars); matches K8s patterns | Architecture |

### Testing

| # | Mistake | Verification | Applies To |
|---|---------|-------------|-----------|
| 82 | Not categorizing tests | Tests use `t.Helper()`, subtests, clear naming | Tests |
| 83 | Not enabling race flag | CI runs `go test -race` | CI |
| 85 | Not using table-driven tests | All validation cases use table-driven pattern | `config_test.go` |
| 86 | Sleeping in unit tests | No `time.Sleep` in any test | Tests |
| 88 | Not using testing utility packages | Integration tests use `t.TempDir()` | Tests |

---

## 12. Implementation Phases

Each TDD phase is a distinct implementation step with explicit entry criteria, work items, and exit gates. No phase may begin until the previous phase's gate passes.

---

### Phase 1: TDD Red — Write Failing Tests

**Entry Criteria:** Test plan approved; `feature/pr5-http-mux-wiring` branch checked out.

**Objective:** Write all 30 test cases so they compile but fail. This proves the test harness is correct and the assertions target real behavior before any production code exists.

**Work Items:**

| Step | Action | File |
|------|--------|------|
| 1.1 | Create `internal/config` package with minimal type stubs (empty structs, function signatures returning zero values) so tests compile | `internal/config/config.go` |
| 1.2 | Write test cases UT-AF-039-001 through UT-AF-039-008 (Config Loading tier) using table-driven pattern | `internal/config/config_test.go` |
| 1.3 | Write test cases UT-AF-039-009 through UT-AF-039-020 (Validation tier) using table-driven pattern | `internal/config/config_test.go` |
| 1.4 | Write test cases UT-AF-039-021 through UT-AF-039-024 (Default Resolution tier) | `internal/config/config_test.go` |
| 1.5 | Write test cases UT-AF-039-025 through UT-AF-039-030 (Startup Integration tier) | `cmd/apifrontend/main_test.go` |
| 1.6 | Create test fixture `internal/config/testdata/valid.yaml` | testdata |
| 1.7 | Verify all tests compile: `go test ./internal/config/... ./cmd/apifrontend/... -run .` | CI gate |

**Exit Gate (Checkpoint A):**
- `go build ./...` succeeds (no compilation errors)
- `go test ./internal/config/... -count=1` compiles and ALL 24 unit tests FAIL
- `go test ./cmd/apifrontend/... -count=1` compiles and ALL 6 integration tests FAIL
- No test passes prematurely (would indicate a tautology)

---

### Phase 2: TDD Green — Minimal Passing Implementation

**Entry Criteria:** Phase 1 gate passed (all 30 tests compile and fail).

**Objective:** Write the minimum production code to make every test pass. No optimization, no cleanup — just make tests green.

**Work Items:**

| Step | Action | File |
|------|--------|------|
| 2.1 | Implement `Config`, `ServerConfig`, `AgentConfig`, `MCPConfig`, `AgentCardConfig` structs with YAML tags | `internal/config/config.go` |
| 2.2 | Implement `DefaultConfig()` returning production defaults | `internal/config/config.go` |
| 2.3 | Implement `Load(data []byte) (*Config, error)` — YAML unmarshal with default merging | `internal/config/config.go` |
| 2.4 | Implement `(*Config).Validate() error` — port range, required URLs, URL syntax | `internal/config/config.go` |
| 2.5 | Implement `(*Config).ResolveDefaults()` — derive agentCard.url from port if empty | `internal/config/config.go` |
| 2.6 | Refactor `cmd/apifrontend/main.go`: add `--config` flag, call `config.Load`, remove `envOr()` | `cmd/apifrontend/main.go` |
| 2.7 | Run `go mod tidy` if new dependencies added | `go.mod`, `go.sum` |
| 2.8 | Verify: `go test ./internal/config/... ./cmd/apifrontend/... -race -count=1` | CI gate |

**Exit Gate (Checkpoint B):**
- ALL 30 tests PASS with `-race` flag
- `go build ./...` succeeds
- No `envOr` or `os.Getenv` calls remain: `grep -rn "envOr\|os.Getenv" --include="*.go" .` returns empty
- `go mod tidy` produces no diff

---

### Phase 3: TDD Refactor — Code Quality and 100 Go Mistakes Validation

**Entry Criteria:** Phase 2 gate passed (all 30 tests pass; no env vars remain).

**Objective:** Improve code quality, readability, and correctness without changing behavior (tests must remain green). Validate against the 100 Go Mistakes checklist. Achieve coverage targets.

**Work Items:**

| Step | Action | Validation |
|------|--------|-----------|
| 3.1 | **Variable shadowing (#1):** Scan for shadowed `err`, `cfg`, `data` in nested blocks | `go vet -vettool=$(which shadow) ./...` or manual review |
| 3.2 | **Unnecessary nesting (#2):** Refactor `Validate()` and `Load()` to use early returns; happy path on left | Visual inspection; no `else` after `return` |
| 3.3 | **No init functions (#3):** Confirm no `func init()` in `internal/config` or modified `main.go` | `grep -rn "func init()" internal/config/ cmd/apifrontend/` |
| 3.4 | **No interface pollution (#5):** Config is a concrete struct; no `Configurer` interface exists | Package export review |
| 3.5 | **No returned interfaces (#7):** `Load()` returns `*Config`, not `interface{}` or custom interface | Function signature review |
| 3.6 | **Package organization (#12, #13):** `internal/config` is focused; no `utils`/`common`/`shared` packages created | Directory structure review |
| 3.7 | **Code documentation (#15):** All exported types (`Config`, `ServerConfig`, etc.) and functions (`Load`, `Validate`, `DefaultConfig`, `ResolveDefaults`) have GoDoc comments starting with the identifier name | `go doc ./internal/config` |
| 3.8 | **Linters (#16):** Run `golangci-lint run ./internal/config/... ./cmd/apifrontend/...` — zero errors | Lint output |
| 3.9 | **Receiver type (#42):** `Validate()` and `ResolveDefaults()` use `*Config` (pointer) since they may mutate | Signature review |
| 3.10 | **Filename as input (#46):** `Load()` accepts `[]byte`; file I/O is in `main.go` only | API surface review |
| 3.11 | **No panics (#48):** No `panic()` in `internal/config`; `main.go` uses `os.Exit(1)` only at top level | `grep -rn "panic(" internal/config/` |
| 3.12 | **Error wrapping (#49):** Every error path uses `fmt.Errorf("context: %w", err)` | Manual review of `Load()`, `Validate()` |
| 3.13 | **No double-handling (#52):** Errors are either logged OR returned in `main.go`, never both | Manual review of `run()` |
| 3.14 | **All errors handled (#53):** No `_` assignments for error returns from `os.ReadFile`, `yaml.Unmarshal` | `grep -rn "_ =" internal/config/ cmd/apifrontend/` |
| 3.15 | **filepath.Clean (#79, #100):** Config path sanitized with `filepath.Clean()` before `os.ReadFile` | Code review |
| 3.16 | **HTTP server timeouts (#81):** Confirm `http.Server` still has explicit `ReadHeaderTimeout`, `WriteTimeout`, `IdleTimeout` | `main.go` review |
| 3.17 | **Table-driven tests (#85):** All validation test cases use `[]struct{name string; ...}` pattern | Test code review |
| 3.18 | **No sleeping (#86):** No `time.Sleep` in any test file | `grep -rn "time.Sleep" internal/config/ cmd/apifrontend/` |
| 3.19 | **Test utilities (#88):** Integration tests use `t.TempDir()`, not manual cleanup | Test code review |
| 3.20 | Run coverage: `go test -coverprofile=cover.out ./internal/config/...` and verify >= 80% | Coverage report |
| 3.21 | Run full test suite one final time: `go test ./... -race -count=1` | All pass |

**Exit Gate (Checkpoint C — Final):**
- ALL 30 tests still PASS (no regressions from refactoring)
- `golangci-lint run` reports 0 errors on modified files
- Coverage >= 80% on `internal/config`
- Coverage >= 80% on config-loading path in `cmd/apifrontend/main.go`
- All 21 items in Steps 3.1–3.21 verified (checklist signed off)
- `go mod tidy` clean
- `grep -rn "envOr\|os.Getenv" --include="*.go" .` returns empty
- Code is ready for documentation phase (Phase 4) and commit

---

## 13. Test Case Details (Representative)

### UT-AF-039-001: Load parses valid full YAML into Config

**Input:**
```yaml
server:
  port: 9090
agent:
  gcpProject: "my-project"
  gcpRegion: "us-east1"
  kaBaseURL: "https://ka.example.com"
  kaMCPEndpoint: "https://ka.example.com/api/v1/mcp/"
  dsBaseURL: "https://ds.example.com"
mcp:
  enabled: true
agentCard:
  url: "https://af.example.com"
```

**Expected:** All fields populated with YAML values; no defaults overriding explicit values.

### UT-AF-039-010: Validate rejects port < 1

**Input:** `Config{Server: ServerConfig{Port: -1}, ...}` (other fields valid via helper)

**Expected:** Error containing `"server.port"` and `"1-65535"`.

### UT-AF-039-026: run() returns error when config file missing

**Input:** `--config /nonexistent/path.yaml`

**Expected:** Error containing `/nonexistent/path.yaml` and `"--config"`.

---

## 14. Traceability Matrix

| BAC | Test Cases | Coverage |
|-----|-----------|----------|
| BAC-1 | UT-AF-039-001, 007, 021, 022, 023, 024 | 6 tests |
| BAC-2 | UT-AF-039-003, 009-020 | 13 tests |
| BAC-3 | UT-AF-039-019, 020, 026, 027, 028 | 5 tests |
| BAC-4 | UT-AF-039-001-008, 025 | 9 tests |
| BAC-5 | All UT-AF-039-* | 30 tests |
| BAC-6 | UT-AF-039-030 | 1 test (codebase grep) |
| BAC-7 | UT-AF-039-025, 029 | 2 tests |

---

## 15. Responsibilities

| Role | Responsibility |
|------|---------------|
| Developer | Write tests (Red), implement code (Green), refactor (Section 11) |
| CI | Automated test execution, race detection, coverage, lint |
| Reviewer | Verify no env vars remain, config struct completeness, error quality |

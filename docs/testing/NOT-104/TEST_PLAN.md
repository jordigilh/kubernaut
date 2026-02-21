# Test Plan: Named Credential Store for Per-Receiver Delivery

**Feature**: Decouple notification adapter credentials from delivery using a file-based named credential store (projected volume pattern)
**Version**: 1.1
**Created**: 2026-02-20
**Author**: AI Assistant
**Status**: Executed
**Branch**: `feat/demo-v1.0`

**Authority**:
- [BR-NOT-104](../../requirements/BR-NOT-104-named-credential-store.md): Named Credential Store for Per-Receiver Delivery
- [DD-NOT-104](../../architecture/decisions/DD-NOT-104-credential-decoupling.md): Credential Decoupling via Named Credential Store

**Cross-References**:
- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../INTEGRATION_E2E_NO_MOCKS_POLICY.md)

---

## 1. Scope

### In Scope

- **CredentialResolver** (`pkg/notification/credentials/resolver.go`): File-based credential resolution, caching, reload, fsnotify watching, and multi-ref validation
- **Routing config credentialRef** (`pkg/notification/routing/config.go`): `SlackConfig.CredentialRef` parsing and validation, replacing `APIURL`
- **Per-receiver delivery wiring** (`internal/controller/notification/routing_handler.go`): `receiverToChannels()` returning receiver-qualified names, `rebuildSlackDeliveryServices()` creating per-receiver `SlackDeliveryService` instances
- **Config/main.go wiring** (`pkg/notification/config/config.go`, `cmd/notification/main.go`): Removal of `WebhookURL`/`SLACK_WEBHOOK_URL`, addition of `CredentialsDir`, resolver initialization and lifecycle

### Out of Scope

- `deploy/demo/` manifest changes (deferred to separate task)
- E2E tests requiring full Kind cluster (not applicable for credential resolver; covered by UT + IT)
- PagerDuty, Email, Webhook credentialRef support (future enhancement)

### Design Decisions

- credentialRef is the sole mechanism for Slack webhook URLs; no api_url or env var fallback (breaking change allowed, project unreleased)
- Orchestrator keys change from "slack" to "slack:<receiver-name>" for per-receiver isolation
- CredentialResolver uses fsnotify (same library as pkg/shared/hotreload/file_watcher.go) for real-time file change detection
- Fail-fast: routing config reload rejects configs with unresolvable credentialRefs

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of **unit-testable** code (CredentialResolver pure logic, routing config parsing/validation, receiverToChannels mapping)
- **Integration**: >=80% of **integration-testable** code (fsnotify watcher, routing config reload with credential validation, per-receiver delivery to mock HTTP endpoints)

### 2-Tier Minimum

Every business requirement gap is covered by at least 2 test tiers (UT + IT):
- **Unit tests** catch logic and correctness errors (fast feedback, isolated)
- **Integration tests** catch wiring, data fidelity, and behavior errors across component boundaries (real filesystem, real fsnotify, mock HTTP)

### Business Outcome Quality Bar

Tests validate **business outcomes** -- behavior, correctness, and data accuracy -- not just code path coverage. Each test scenario answers: "what does the operator/system get?" not "what function is called?"

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/notification/credentials/resolver.go` | `NewResolver`, `Resolve`, `Reload`, `ValidateRefs`, `Close` | ~120 |
| `pkg/notification/routing/config.go` | `SlackConfig.CredentialRef` parsing, `Validate` | ~20 (delta) |
| `internal/controller/notification/routing_handler.go` | `receiverToChannels` (qualified names) | ~30 (delta) |

**Total unit-testable**: ~170 new/modified lines

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/notification/credentials/resolver.go` | `StartWatching` (fsnotify loop), `Reload` (filesystem I/O) | ~80 |
| `internal/controller/notification/routing_handler.go` | `rebuildSlackDeliveryServices`, `loadRoutingConfigFromCluster` (credential validation) | ~60 |
| `cmd/notification/main.go` | Startup wiring (resolver init, routing handler injection) | ~30 |

**Total integration-testable**: ~170 new/modified lines

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-NOT-104-001 | Named credential resolution: existing file | P0 | Unit | UT-NOT-104-001 | Pass |
| BR-NOT-104-001 | Named credential resolution: missing file | P0 | Unit | UT-NOT-104-002 | Pass |
| BR-NOT-104-002 | Reload picks up new files | P0 | Unit | UT-NOT-104-003 | Pass |
| BR-NOT-104-002 | Reload picks up changed values | P0 | Unit | UT-NOT-104-004 | Pass |
| BR-NOT-104-002 | Reload removes deleted files | P0 | Unit | UT-NOT-104-005 | Pass |
| BR-NOT-104-001 | Whitespace/newline trimming | P0 | Unit | UT-NOT-104-006 | Pass |
| BR-NOT-104-003 | ValidateRefs: all resolve | P0 | Unit | UT-NOT-104-007 | Pass |
| BR-NOT-104-003 | ValidateRefs: unresolvable refs | P0 | Unit | UT-NOT-104-008 | Pass |
| BR-NOT-104-004 | credentialRef parses from YAML | P0 | Unit | UT-NOT-104-009 | Pass |
| BR-NOT-104-004 | Missing credentialRef fails validation | P0 | Unit | UT-NOT-104-010 | Pass |
| BR-NOT-104-004 | receiverToChannels returns qualified names | P0 | Unit | UT-NOT-104-011 | Pass |
| BR-NOT-104-003 | Unresolvable credentialRef preserves previous config | P0 | Unit | UT-NOT-104-012 | Pass |
| BR-NOT-104-004 | DefaultConfig includes CredentialsDir | P0 | Unit | UT-NOT-104-013 | Pass |
| BR-NOT-104-004 | applyDefaults sets CredentialsDir when empty | P0 | Unit | UT-NOT-104-014 | Pass |
| BR-NOT-104-004 | SlackSettings has no WebhookURL field | P0 | Unit | UT-NOT-104-015 | Pass |
| BR-NOT-104-004 | LoadFromEnv no longer loads SLACK_WEBHOOK_URL | P0 | Unit | UT-NOT-104-016 | Pass |
| BR-NOT-104-004 | Slack without credentialRef uses unqualified channel | P1 | Unit | UT-NOT-104-017 | Pass |
| BR-NOT-104-002 | fsnotify detects file change and updates cache | P0 | Integration | IT-NOT-104-001 | Pass |
| BR-NOT-104-004 | Routing reload creates per-receiver Slack services | P0 | Integration | IT-NOT-104-002 | Pass |
| BR-NOT-104-004 | Multi-receiver delivery routes to correct endpoints | P0 | Integration | IT-NOT-104-003 | Pass |
| BR-NOT-104-003 | Unresolvable credentialRef preserves previous routing | P0 | Integration | IT-NOT-104-004 | Pass |
| BR-NOT-104-005 | Credential rotation updates delivery URL | P0 | Integration | IT-NOT-104-005 | Pass |
| BR-NOT-104-004 | Mixed channel receiver resolves both channels | P1 | Integration | IT-NOT-104-006 | Pass |
| BR-NOT-104-003 | Empty credentials dir rejects all credentialRefs | P0 | Integration | IT-NOT-104-007 | Pass |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-NOT-104-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **NOT**: Notification service
- **104**: Issue number
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

**Testable code scope**: `pkg/notification/credentials/resolver.go` (~120 lines), `pkg/notification/routing/config.go` (~20 lines delta), `internal/controller/notification/routing_handler.go` (~30 lines delta). Target: >=80% of unit-testable code.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-NOT-104-001` | Operator's credential reference resolves to the correct secret value stored in the projected volume file | Pass |
| `UT-NOT-104-002` | System returns a clear error when operator references a credential name that doesn't exist in the credentials directory | Pass |
| `UT-NOT-104-003` | After a new Secret is projected (new file appears), the system can resolve the new credential without restart | Pass |
| `UT-NOT-104-004` | After a Secret value is rotated (file content changes), the system returns the updated value on next resolve | Pass |
| `UT-NOT-104-005` | After a Secret is removed (file deleted), the system no longer resolves that credential and returns an error | Pass |
| `UT-NOT-104-006` | Credential values are clean (no trailing newlines or whitespace) regardless of how the Secret was written | Pass |
| `UT-NOT-104-007` | Routing config with valid credentialRefs passes validation, confirming all receiver credentials are available | Pass |
| `UT-NOT-104-008` | Routing config validation lists ALL unresolvable credentialRefs in a single error (not just the first), so operator can fix all at once | Pass |
| `UT-NOT-104-009` | Routing YAML with credentialRef field parses correctly into SlackConfig struct | Pass |
| `UT-NOT-104-010` | Routing YAML with missing credentialRef fails validation with a clear error message | Pass |
| `UT-NOT-104-011` | Channel resolution returns receiver-qualified names (e.g., "slack:sre-critical") so orchestrator can route to the correct per-receiver service | Pass |
| `UT-NOT-104-012` | When routing config reload fails credential validation, the previous working config is preserved and deliveries continue uninterrupted | Pass |

### Tier 2: Integration Tests

**Testable code scope**: `pkg/notification/credentials/resolver.go` StartWatching/Reload (~80 lines), `internal/controller/notification/routing_handler.go` rebuildSlackDeliveryServices (~60 lines). Target: >=80% of integration-testable code.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-NOT-104-001` | When a new credential file is written to the projected volume, the resolver detects it via fsnotify and makes it available for resolution within seconds | Pass |
| `IT-NOT-104-002` | When routing config is reloaded, each receiver gets a dedicated SlackDeliveryService instance bound to its resolved webhook URL | Pass |
| `IT-NOT-104-003` | Notifications routed to different receivers are delivered to different Slack webhook endpoints (mock HTTP servers), confirming per-receiver isolation | Pass |
| `IT-NOT-104-004` | When routing config reload references an unresolvable credential, the previous valid routing is preserved and subsequent deliveries still work | Pass |
| `IT-NOT-104-005` | After a credential file is rotated (updated content), the next delivery uses the new webhook URL without any restart or manual intervention | Pass |
| `IT-NOT-104-006` | A receiver with both Slack and Console configs correctly resolves both channel types, with Slack using the credentialRef webhook and Console using the default service | Pass |
| `IT-NOT-104-007` | When the credentials directory is empty, all routing configs with credentialRefs are rejected and the system uses the previous valid config | Pass |

### Tier Skip Rationale

- **E2E**: Not applicable. The credential resolver operates at the service level (filesystem + in-process wiring). Full Kind cluster E2E tests would not meaningfully increase coverage beyond IT-level tests with real filesystem and mock HTTP servers. E2E coverage for delivery routing already exists in the notification E2E suite.

---

## 6. Test Cases (Detail)

### UT-NOT-104-001: Resolve existing credential

**BR**: BR-NOT-104-001
**Type**: Unit
**File**: `test/unit/notification/credentials/resolver_test.go`

**Given**: A credentials directory containing a file `slack-sre-critical` with content `https://hooks.slack.com/services/T.../B.../xxx`
**When**: `Resolve("slack-sre-critical")` is called
**Then**: Returns the exact URL string without error

**Acceptance Criteria**:
- Returned value exactly matches file content
- No error returned
- Value is usable as a Slack webhook URL

### UT-NOT-104-002: Resolve non-existent credential

**BR**: BR-NOT-104-001
**Type**: Unit
**File**: `test/unit/notification/credentials/resolver_test.go`

**Given**: A credentials directory that does NOT contain a file named `nonexistent`
**When**: `Resolve("nonexistent")` is called
**Then**: Returns an error containing the credential name for debugging

**Acceptance Criteria**:
- Error is non-nil
- Error message contains `"nonexistent"` for operator debugging
- Returned value is empty string

### UT-NOT-104-003: Reload picks up new credential files

**BR**: BR-NOT-104-002
**Type**: Unit
**File**: `test/unit/notification/credentials/resolver_test.go`

**Given**: A resolver initialized with an empty credentials directory
**When**: A new file `slack-new` is written to the directory AND `Reload()` is called
**Then**: `Resolve("slack-new")` succeeds with the new file's content

**Acceptance Criteria**:
- `Resolve("slack-new")` fails before `Reload()`
- `Resolve("slack-new")` succeeds after `Reload()`
- Returned value matches file content

### UT-NOT-104-004: Reload picks up changed credential values

**BR**: BR-NOT-104-002
**Type**: Unit
**File**: `test/unit/notification/credentials/resolver_test.go`

**Given**: A resolver with cached credential `slack-sre` = `old-url`
**When**: The file content is changed to `new-url` AND `Reload()` is called
**Then**: `Resolve("slack-sre")` returns `new-url`

**Acceptance Criteria**:
- Value before reload is `old-url`
- Value after reload is `new-url`

### UT-NOT-104-005: Reload removes deleted credential files

**BR**: BR-NOT-104-002
**Type**: Unit
**File**: `test/unit/notification/credentials/resolver_test.go`

**Given**: A resolver with cached credential `slack-temp` = `some-url`
**When**: The file `slack-temp` is deleted AND `Reload()` is called
**Then**: `Resolve("slack-temp")` returns an error

**Acceptance Criteria**:
- `Resolve("slack-temp")` succeeds before deletion
- `Resolve("slack-temp")` fails after `Reload()`

### UT-NOT-104-006: Whitespace and newline trimming

**BR**: BR-NOT-104-001
**Type**: Unit
**File**: `test/unit/notification/credentials/resolver_test.go`

**Given**: A credential file with content `"  https://hooks.slack.com/xxx  \n"`
**When**: `Resolve()` is called
**Then**: Returns `"https://hooks.slack.com/xxx"` (trimmed)

**Acceptance Criteria**:
- Leading spaces removed
- Trailing spaces removed
- Trailing newline removed

### UT-NOT-104-007: ValidateRefs -- all resolvable

**BR**: BR-NOT-104-003
**Type**: Unit
**File**: `test/unit/notification/credentials/resolver_test.go`

**Given**: Credentials directory contains files `a`, `b`, `c`
**When**: `ValidateRefs([]string{"a", "b", "c"})` is called
**Then**: Returns `nil` (all valid)

**Acceptance Criteria**:
- No error returned
- All three refs are resolvable

### UT-NOT-104-008: ValidateRefs -- unresolvable refs

**BR**: BR-NOT-104-003
**Type**: Unit
**File**: `test/unit/notification/credentials/resolver_test.go`

**Given**: Credentials directory contains only file `a`
**When**: `ValidateRefs([]string{"a", "missing1", "missing2"})` is called
**Then**: Returns error listing both `missing1` and `missing2`

**Acceptance Criteria**:
- Error is non-nil
- Error message contains `"missing1"`
- Error message contains `"missing2"`
- Error does NOT contain `"a"` (which is valid)

### UT-NOT-104-009: SlackConfig credentialRef YAML parsing

**BR**: BR-NOT-104-004
**Type**: Unit
**File**: `test/unit/notification/routing_credentialRef_test.go`

**Given**: YAML routing config with `credentialRef: slack-sre-critical` under a Slack receiver
**When**: Config is parsed via `ParseConfig()`
**Then**: `SlackConfig.CredentialRef` equals `"slack-sre-critical"`

**Acceptance Criteria**:
- Parsed struct has correct `CredentialRef` value
- No parsing error

### UT-NOT-104-010: SlackConfig missing credentialRef fails validation

**BR**: BR-NOT-104-004
**Type**: Unit
**File**: `test/unit/notification/routing_credentialRef_test.go`

**Given**: YAML routing config with a Slack receiver that has no `credentialRef` field
**When**: Config is parsed and validated
**Then**: Validation error mentioning `credentialRef`

**Acceptance Criteria**:
- Error is non-nil
- Error message references `credentialRef` or the receiver name

### UT-NOT-104-011: receiverToChannels returns qualified names

**BR**: BR-NOT-104-004
**Type**: Unit
**File**: `test/unit/notification/routing_receiver_qualified_test.go`

**Given**: A receiver named `sre-critical` with one `SlackConfig` and one `ConsoleConfig`
**When**: `receiverToChannels(receiver)` is called
**Then**: Returns `["slack:sre-critical", "console"]` (Slack qualified, console unqualified)

**Acceptance Criteria**:
- Slack channel is receiver-qualified: `"slack:sre-critical"`
- Console channel is unqualified: `"console"`
- Both channels present in result

### UT-NOT-104-012: Failed credential validation preserves previous config

**BR**: BR-NOT-104-003
**Type**: Unit
**File**: `test/unit/notification/routing_receiver_qualified_test.go`

**Given**: A router with a valid routing config loaded
**When**: A new config is loaded that references unresolvable `credentialRef`
**Then**: The router continues using the previous valid config

**Acceptance Criteria**:
- Previous config remains active after failed reload
- Error is returned from `LoadConfig()`
- Subsequent routing decisions use previous receivers

### IT-NOT-104-001: fsnotify detects credential file change

**BR**: BR-NOT-104-002
**Type**: Integration
**File**: `test/integration/notification/credential_resolver_test.go`

**Given**: A `CredentialResolver` watching a temp directory via `StartWatching(ctx)`
**When**: A new file `slack-new` is written to the directory
**Then**: Within 5 seconds, `Resolve("slack-new")` returns the file content

**Acceptance Criteria**:
- No manual `Reload()` call needed (fsnotify triggers automatically)
- Resolution succeeds within 5 seconds of file creation
- Correct value returned

### IT-NOT-104-002: Routing reload creates per-receiver services

**BR**: BR-NOT-104-004
**Type**: Integration
**File**: `test/integration/notification/credential_resolver_test.go`

**Given**: Credentials directory with `slack-sre` and `slack-platform` files, each containing different mock webhook URLs
**When**: Routing config is loaded with two receivers, each referencing their credential
**Then**: Orchestrator has two registered Slack services with correct webhook URLs

**Acceptance Criteria**:
- `orchestrator.HasChannel("slack:sre-critical")` is true
- `orchestrator.HasChannel("slack:platform-alerts")` is true
- Each service delivers to the correct mock endpoint

### IT-NOT-104-003: Multi-receiver delivery to different endpoints

**BR**: BR-NOT-104-004
**Type**: Integration
**File**: `test/integration/notification/credential_resolver_test.go`

**Given**: Two mock HTTP servers (one per receiver), credential resolver configured with their URLs
**When**: Notifications are delivered via `"slack:sre-critical"` and `"slack:platform-alerts"` channels
**Then**: Each mock server receives exactly one request

**Acceptance Criteria**:
- Mock server A receives request for sre-critical
- Mock server B receives request for platform-alerts
- No cross-contamination between receivers

### IT-NOT-104-004: Unresolvable credential preserves previous config

**BR**: BR-NOT-104-003
**Type**: Integration
**File**: `test/integration/notification/credential_resolver_test.go`

**Given**: A working routing config with valid credentials
**When**: A new config referencing `nonexistent-cred` is loaded
**Then**: Previous config remains active; deliveries still work

**Acceptance Criteria**:
- Load error returned
- Previous receivers still routable
- Delivery to previous channels succeeds

### IT-NOT-104-005: Credential rotation updates delivery URL

**BR**: BR-NOT-104-005
**Type**: Integration
**File**: `test/integration/notification/credential_resolver_test.go`

**Given**: Credential `slack-sre` pointing to mock server A
**When**: Credential file is updated to point to mock server B, routing is rebuilt
**Then**: Next delivery goes to mock server B

**Acceptance Criteria**:
- First delivery hits mock server A
- After file update + rebuild, second delivery hits mock server B
- No pod restart required

### IT-NOT-104-006: Mixed channel receiver resolves both channels

**BR**: BR-NOT-104-004
**Type**: Integration
**File**: `test/integration/notification/credential_resolver_test.go`

**Given**: Receiver `mixed` with one `SlackConfig` (credentialRef) and one `ConsoleConfig`
**When**: `receiverToChannels()` and delivery are executed
**Then**: Both `"slack:mixed"` and `"console"` channels are resolved and delivered to

**Acceptance Criteria**:
- Slack mock server receives delivery
- Console output is produced
- Both channels present in resolved list

### IT-NOT-104-007: Empty credentials directory rejects all refs

**BR**: BR-NOT-104-003
**Type**: Integration
**File**: `test/integration/notification/credential_resolver_test.go`

**Given**: An empty credentials directory
**When**: Routing config with credentialRefs is loaded
**Then**: All credentialRefs are rejected; error lists all unresolvable refs

**Acceptance Criteria**:
- Error contains all referenced credential names
- No channels registered in orchestrator
- Previous config preserved (if any)

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: Filesystem is mocked via `os.MkdirTemp` + direct file writes (no external dependency)
- **Location**: `test/unit/notification/credentials/resolver_test.go`, `test/unit/notification/routing_credentialRef_test.go`, `test/unit/notification/routing_receiver_qualified_test.go`

### Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks of business code. Mock HTTP servers (`httptest.NewServer`) for Slack webhook endpoints.
- **Infrastructure**: Real filesystem (temp directories), real fsnotify watcher, mock HTTP servers for Slack webhooks
- **Location**: `test/integration/notification/credential_resolver_test.go`

---

## 8. Execution

```bash
# Unit tests
make test-unit-notification

# Integration tests
make test-integration-notification

# Specific test by ID
go test ./test/unit/notification/... -ginkgo.focus="UT-NOT-104-001"
go test ./test/integration/notification/... -ginkgo.focus="IT-NOT-104-001"
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-02-20 | Initial test plan: 12 UTs + 7 ITs, 2-tier coverage |
| 1.1 | 2026-02-21 | All tests pass. Added UT-NOT-104-013 through 017 (config settings + backward compat). Fixed IT file path. Updated statuses to Pass. |

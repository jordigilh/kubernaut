# Implementation Plan: Microsoft Teams Delivery Channel (#593)

**Issue**: #593 — feat(notification): implement Microsoft Teams delivery channel
**Test Plan**: [TEST_PLAN.md](TEST_PLAN.md)
**Created**: 2026-03-04
**Status**: Draft
**Depends on**: #60 (PagerDuty) — shared patterns established first

---

## Execution Strategy

This plan follows **strict TDD RED → GREEN → REFACTOR** with each phase as a discrete implementation step. Checkpoints perform rigorous due diligence between logical groups.

**Total phases**: 33 (11 TDD cycles × 3 phases each) + 4 audit checkpoints = 37 steps

**Ordering rationale**: This plan executes AFTER #60 (PagerDuty) because:
1. PD establishes the raw-HTTP delivery service pattern (constructor, Deliver, error classification)
2. Teams reuses the size guard pattern (generalized in #60 REFACTOR phases)
3. Integration test infrastructure (routing handler rebuild, credential wiring) is proven by PD first

---

## Phase 0: Prerequisites (already completed)

The following shared infrastructure was completed in the Phase 0 session:

- [x] `ChannelTeams` already existed in CRD enum (confirmed)
- [x] `TeamsConfig` struct created with `CredentialRef` (F-3)
- [x] `TeamsConfigs` added to `Receiver` struct (F-3)
- [x] `ValidateCredentialRefs()` extended for Teams (F-6)
- [x] `QualifiedChannels()` updated for Teams per-receiver names (F-4)
- [x] `GetChannels()` updated to include Teams (F-4)
- [x] `collectTeamsCredentialRefs()` added (F-5)
- [x] `registeredTeamsKeys` added to reconciler struct (F-5)
- [x] `teams` added to DS audit schema (OpenAPI, model, validator, migration, ogen) (F-1b)
- [x] All existing tests passing

---

## TDD Cycle 1: Routing Config Validation (UT-NOT-593-013, UT-NOT-593-014)

### Phase 1 — RED: Write failing tests for routing config Teams validation

**Tests**: `test/unit/notification/delivery/teams_test.go` (new file)

| Test ID | Assertion |
|---------|-----------|
| `UT-NOT-593-013` | `QualifiedChannels()` returns `teams:<receiver>` for Teams with CredentialRef |
| `UT-NOT-593-014` | `ValidateCredentialRefs()` fails when TeamsConfig has empty CredentialRef |

**Actions**:
1. Create `test/unit/notification/delivery/teams_test.go` with Ginkgo suite
2. Write test: receiver with `TeamsConfigs: [{CredentialRef: "teams-webhook"}]` → `QualifiedChannels()` contains `teams:<name>`
3. Write test: receiver with `TeamsConfigs: [{CredentialRef: ""}]` → `ValidateCredentialRefs()` returns error containing `credentialRef` and `teamsConfigs`

**Exit criteria**: Tests compile and PASS (Phase 0 already implemented the routing config changes — these formalize the contract as regression guards)

### Phase 2 — GREEN: Verify Phase 0 implementation passes

**Actions**:
1. Run tests → confirm both pass
2. If any fail, fix the Phase 0 implementation

**Exit criteria**: UT-NOT-593-013 and UT-NOT-593-014 pass

### Phase 3 — REFACTOR: Extract test helpers

**Actions**:
1. Extract `newTeamsReceiver(name, credRef string)` test helper
2. Ensure Ginkgo BDD style consistency with PD test file

**Exit criteria**: Tests still pass; helpers extracted

---

## TDD Cycle 2: Adaptive Card Construction — Workflows Format (UT-NOT-593-001)

### Phase 4 — RED: Write failing test for Workflows format compliance

**Tests**: `test/unit/notification/delivery/teams_test.go`

| Test ID | Assertion |
|---------|-----------|
| `UT-NOT-593-001` | Outer `type: "message"`, `attachments[0].contentType = "application/vnd.microsoft.card.adaptive"`, inner card has `type: "AdaptiveCard"`, `version: "1.0"`, `$schema` |

**Actions**:
1. Write test calling `buildTeamsPayload(notification)` (function does not exist yet)
2. Parse JSON → assert outer wrapper structure
3. Assert `attachments` array length == 1
4. Assert inner card schema, type, version
5. Assert this is NOT legacy `@type: MessageCard` format

**Exit criteria**: Test fails to compile (`buildTeamsPayload` undefined)

### Phase 5 — GREEN: Implement `buildTeamsPayload` with Adaptive Card wrapper

**Files**: `pkg/notification/delivery/teams_cards.go` (new)

**Actions**:
1. Create `teams_cards.go` with:
   - `TeamsMessage` struct: `Type string`, `Attachments []TeamsAttachment`
   - `TeamsAttachment` struct: `ContentType string`, `Content AdaptiveCard`
   - `AdaptiveCard` struct: `Schema string`, `Type string`, `Version string`, `Body []CardElement`
   - `CardElement` interface or struct union for TextBlock, FactSet, etc.
2. Implement `buildTeamsPayload(notification) ([]byte, error)`:
   - Outer: `type: "message"`, single attachment with `contentType: "application/vnd.microsoft.card.adaptive"`
   - Inner card: `$schema`, `type: "AdaptiveCard"`, `version: "1.0"`
   - Body: TextBlock with subject, TextBlock with body (minimal for GREEN)
3. Marshal to JSON

**Exit criteria**: UT-NOT-593-001 passes

### Phase 6 — REFACTOR: Improve card type definitions

**Actions**:
1. Define card element types as proper Go structs (TextBlock, FactSet, ActionSet, etc.)
2. Add constants for schema URL, content type, card version
3. Ensure JSON tags produce correct camelCase output (`$schema` needs `json:"$schema"`)

**Exit criteria**: Tests still pass; types are clean and reusable

---

## TDD Cycle 3: Adaptive Card Content — RCA + Affected Resource (UT-NOT-593-002)

### Phase 7 — RED: Write failing test for card content

**Tests**: `test/unit/notification/delivery/teams_test.go`

| Test ID | Assertion |
|---------|-----------|
| `UT-NOT-593-002` | Card body contains TextBlocks with subject, RCA summary, affected resource, confidence score |

**Actions**:
1. Create NR with Context containing Lineage, Analysis, Workflow sub-structs
2. Call `buildTeamsPayload` → parse inner card body
3. Assert body contains TextBlock elements with expected content

**Exit criteria**: Test fails (card body only has minimal subject/body from Phase 5)

### Phase 8 — GREEN: Populate card body with NR context

**Files**: `pkg/notification/delivery/teams_cards.go`

**Actions**:
1. Build card body sections from NR Context:
   - Header: Subject as heading TextBlock (weight: bolder, size: medium)
   - RCA: Body text as TextBlock (wrap: true)
   - Facts: FactSet with affected resource, confidence, workflow ID
2. Handle nil Context sub-structs gracefully

**Exit criteria**: UT-NOT-593-002 passes

### Phase 9 — REFACTOR: Extract card section builders

**Actions**:
1. Extract `buildHeaderSection(subject, priority)`, `buildFactsSection(notification)`, `buildBodySection(body)`
2. Ensure each section builder handles nil inputs without panic
3. Add color indicators based on priority (Attention for critical, Warning for high, etc.)

**Exit criteria**: Tests still pass; card building is modular

---

## TDD Cycle 4: Per-Type Card Layouts (UT-NOT-593-003, UT-NOT-593-004, UT-NOT-593-005, UT-NOT-593-015)

### Phase 10 — RED: Write failing tests for per-type cards

**Tests**: `test/unit/notification/delivery/teams_test.go`

| Test ID | Assertion |
|---------|-----------|
| `UT-NOT-593-003` | Approval card includes kubectl command: `kubectl kubernaut chat rar/{name} -n {namespace}` |
| `UT-NOT-593-004` | Status-update card includes phase transition details and verification context |
| `UT-NOT-593-005` | Escalation card includes urgency indicators (priority emoji, severity color) |
| `UT-NOT-593-015` | Completion card includes verification outcome when present |

**Actions**:
1. Create NR with type=approval + Context.Lineage → assert kubectl command in card body
2. Create NR with type=status-update + Context.Verification → assert verification fields
3. Create NR with type=escalation + priority=critical → assert urgency indicators
4. Create NR with type=completion + Context.Verification.Assessed=true → assert verification outcome

**Exit criteria**: Tests fail (per-type layouts not implemented)

### Phase 11 — GREEN: Implement per-type card layout selection

**Files**: `pkg/notification/delivery/teams_cards.go`

**Actions**:
1. Add `buildTeamsPayloadForType(notification) ([]byte, error)` that dispatches on `notification.Spec.Type`:
   - `approval` → add kubectl command TextBlock and ActionSet with OpenUrl
   - `status-update` → add verification context FactSet
   - `escalation` → add urgency header with color and emoji
   - `completion` → add verification outcome FactSet
   - default → generic card with body text
2. Wire `buildTeamsPayload` to call `buildTeamsPayloadForType`
3. kubectl command: `fmt.Sprintf("kubectl kubernaut chat rar/%s -n %s", rrName, namespace)`

**Exit criteria**: UT-NOT-593-003, 004, 005, 015 pass

### Phase 12 — REFACTOR: Deduplicate card section builders

**Actions**:
1. Extract common sections (header, body, facts) used across all types
2. Each type-specific builder composes common + unique sections
3. Ensure no section builder is >30 lines (split if needed)

**Exit criteria**: Tests still pass; per-type builders are composable

---

## CHECKPOINT 1: Adaptive Card Audit

**Actions**:
1. Verify all 7 card-related unit tests pass (UT-NOT-593-001..005, 013, 014, 015)
2. Run `go vet ./pkg/notification/delivery/...`
3. Validate JSON output against Adaptive Card v1.0 schema (check `$schema`, `type`, `version`)
4. Verify Workflows format: NO `@type: MessageCard`, NO `@context`, NO legacy connector fields
5. Verify kubectl command format is exact: `kubectl kubernaut chat rar/{name} -n {namespace}`
6. Check for hardcoded strings → extract to constants
7. Verify no anti-patterns: no `time.Sleep`, no `Skip()`, no `ToNot(BeNil)` existence-only assertions
8. Measure coverage of `teams_cards.go` → target >=80%

**Finding resolution**: Address all findings before proceeding to error classification.

---

## TDD Cycle 5: Error Classification — Retryable (UT-NOT-593-006)

### Phase 13 — RED: Write failing tests for retryable errors

**Tests**: `test/unit/notification/delivery/teams_test.go`

| Test ID | Assertion |
|---------|-----------|
| `UT-NOT-593-006` | HTTP 500, 502, 503, 429 → `IsRetryableError(err) == true` |

**Actions**:
1. Table-driven test with httptest mock returning each status code
2. Assert `IsRetryableError(err)` for 5xx and 429

**Exit criteria**: Tests fail (`Deliver` method for Teams does not exist yet)

### Phase 14 — GREEN: Implement Teams `Deliver` method

**Files**: `pkg/notification/delivery/teams.go` (new)

**Actions**:
1. Create `TeamsDeliveryService` struct with `webhookURL string`, `httpClient *http.Client`
2. Implement `NewTeamsDeliveryService(webhookURL string, timeout time.Duration) *TeamsDeliveryService`
3. Implement `Deliver(ctx, notification) error`:
   - Call `buildTeamsPayload(notification)`
   - HTTP POST to webhook URL with `Content-Type: application/json`
   - Reuse `isRetryableStatusCode` for error classification
   - Reuse `isTLSError` for TLS detection
4. Follow exact same pattern as PagerDuty `Deliver` (established in #60)

**Exit criteria**: UT-NOT-593-006 passes

### Phase 15 — REFACTOR: Ensure consistency with PD/Slack patterns

**Actions**:
1. Verify error message format matches PD/Slack: `"teams webhook returned %d (retryable/permanent failure): %s"`
2. Verify response body read + close pattern matches
3. Verify `Content-Type: application/json` header is set

**Exit criteria**: Tests still pass; three delivery services have identical HTTP patterns

---

## TDD Cycle 6: Error Classification — Permanent + TLS (UT-NOT-593-007, UT-NOT-593-008)

### Phase 16 — RED: Write failing tests for permanent errors

**Tests**: `test/unit/notification/delivery/teams_test.go`

| Test ID | Assertion |
|---------|-----------|
| `UT-NOT-593-007` | HTTP 400, 401, 403, 404 → permanent error (`!IsRetryableError`) |
| `UT-NOT-593-008` | TLS cert error → permanent error (BR-NOT-058) |

**Actions**:
1. Table-driven 4xx test
2. TLS test with httptest TLS server and invalid cert

**Exit criteria**: Tests may pass from Phase 14 implementation; verify assertions are tight

### Phase 17 — GREEN: Confirm permanent error paths

**Actions**:
1. Verify 4xx and TLS tests pass with tight assertions
2. Fix any gaps in error classification

**Exit criteria**: UT-NOT-593-007 and UT-NOT-593-008 pass

### Phase 18 — REFACTOR: No duplication in error handling

**Actions**:
1. Confirm `isRetryableStatusCode` and `isTLSError` are reused (not duplicated)
2. If any delivery-service-specific error wrapping is needed, add it cleanly

**Exit criteria**: Tests still pass; zero error classification duplication across Slack/PD/Teams

---

## TDD Cycle 7: Payload Size Guard — 28KB (UT-NOT-593-009, UT-NOT-593-010)

### Phase 19 — RED: Write failing tests for 28KB size guard

**Tests**: `test/unit/notification/delivery/teams_test.go`

| Test ID | Assertion |
|---------|-----------|
| `UT-NOT-593-009` | Payload >28KB triggers truncation; result <=28KB |
| `UT-NOT-593-010` | Truncated payload contains `"[truncated — full details in audit trail]"` and correlation ID (NR name); total <=28KB |

**Actions**:
1. Create NR with 40KB body text
2. Call `buildTeamsPayload` → assert `len(jsonBytes) <= 28*1024`
3. Parse truncated card → assert body TextBlock contains truncation marker
4. Assert correlation ID present in card

**Exit criteria**: Tests fail (no truncation logic yet)

### Phase 20 — GREEN: Implement 28KB truncation

**Files**: `pkg/notification/delivery/teams_cards.go`

**Actions**:
1. After building full card, marshal to JSON and check size
2. If >28KB: truncate body TextBlock content with marker
3. Include correlation ID (NR name) in card facts
4. Re-marshal and verify <=28KB

> **Note**: If PD's size guard was extracted as a reusable helper in Phase 18 of the #60 plan, reuse it here with `limit=28*1024`. Otherwise, implement inline.

**Exit criteria**: UT-NOT-593-009 and UT-NOT-593-010 pass

### Phase 21 — REFACTOR: Consolidate size guard with PD if possible

**Actions**:
1. If both PD (512KB) and Teams (28KB) use similar truncation logic, extract `truncatePayloadBody(payload, limit, marker)` to `pkg/notification/delivery/size_guard.go`
2. If too different (PD truncates custom_details, Teams truncates card TextBlock), keep separate but use shared constant pattern
3. Ensure `const teamsPayloadLimit = 28 * 1024` is named

**Exit criteria**: Tests still pass; size guard pattern is clean

---

## TDD Cycle 8: Context Cancellation + Constructor Guard (UT-NOT-593-011, UT-NOT-593-012)

### Phase 22 — RED: Write failing tests for edge cases

**Tests**: `test/unit/notification/delivery/teams_test.go`

| Test ID | Assertion |
|---------|-----------|
| `UT-NOT-593-011` | Cancelled context → error returned immediately |
| `UT-NOT-593-012` | Empty webhook URL → descriptive error |

**Actions**:
1. Cancelled context test (same pattern as PD)
2. Empty URL guard test

**Exit criteria**: Tests may pass from Phase 14; empty URL guard needs explicit check

### Phase 23 — GREEN: Add URL validation guard

**Actions**:
1. In `Deliver`: `if s.webhookURL == "" { return fmt.Errorf("teams webhook URL is empty") }`
2. Context cancellation already handled by `http.NewRequestWithContext`

**Exit criteria**: UT-NOT-593-011 and UT-NOT-593-012 pass

### Phase 24 — REFACTOR: Consistent error messages

**Actions**:
1. Verify all error messages use `"teams ..."` prefix (not `"Teams ..."` or `"TEAMS ..."`)
2. Ensure error wrapping uses `%w`
3. Clean up edge case test descriptions

**Exit criteria**: Tests still pass; errors are descriptive and consistent

---

## CHECKPOINT 2: Full Unit Test Audit

**Actions**:
1. Run ALL 15 unit tests → 100% pass rate
2. Measure coverage:
   - `teams.go` → >=80%
   - `teams_cards.go` → >=80%
3. Run `go vet ./...` and `go build ./...`
4. Verify no anti-patterns:
   - No `time.Sleep()`
   - No `Skip()`
   - No `ToNot(BeNil)` existence-only checks
5. Verify interface compliance: `var _ delivery.Service = (*TeamsDeliveryService)(nil)` compiles
6. Verify Adaptive Card JSON is valid (parse round-trip)
7. Verify Workflows format in ALL per-type cards (no legacy connector fields leaked)
8. Check unused imports, dead code
9. Verify ALL error paths tested

**Finding resolution**: Address all findings. If coverage <80%, add tests.

---

## TDD Cycle 9: Integration — Per-Receiver Credential Flow (IT-NOT-593-001)

### Phase 25 — RED: Write failing integration test for credential resolution

**Tests**: `test/integration/notification/teams_delivery_test.go` (new file)

| Test ID | Assertion |
|---------|-----------|
| `IT-NOT-593-001` | Teams service registered via credential resolver; delivery succeeds through orchestrator |

**Actions**:
1. Create integration test file with Ginkgo suite
2. Set up: temp dir with credential file, credential resolver, delivery orchestrator, routing config with Teams receiver
3. Call `ReloadRoutingFromContent` → trigger `rebuildTeamsDeliveryServices`
4. Deliver through orchestrator to `teams:<receiver>` key → assert httptest mock receives request

**Exit criteria**: Test fails (`rebuildTeamsDeliveryServices` not implemented yet)

### Phase 26 — GREEN: Implement `rebuildTeamsDeliveryServices`

**Files**: `internal/controller/notification/routing_handler.go`

**Actions**:
1. Implement `rebuildTeamsDeliveryServices(ctx, config)` following `rebuildPagerDutyDeliveryServices` pattern:
   - Unregister stale Teams keys from `r.registeredTeamsKeys`
   - For each Teams config: resolve credential → create `TeamsDeliveryService` → register with orchestrator
   - Track new keys in `r.registeredTeamsKeys`
2. Call `rebuildTeamsDeliveryServices` from `ReloadRoutingFromContent` after PD rebuild

**Exit criteria**: IT-NOT-593-001 passes

### Phase 27 — REFACTOR: Consider generic rebuild helper

**Actions**:
1. Now that Slack, PD, and Teams all follow identical rebuild patterns, evaluate extracting:
   ```
   rebuildCredentialDeliveryServices[T Config](ctx, config, configs func(Receiver)[]T, credRef func(T)string, factory func(url)Service, keyPrefix string)
   ```
2. If >80% overlap and the generic is clean, extract. Otherwise keep three separate functions with clear comments noting the duplication is intentional.
3. Verify all three rebuild functions have consistent logging

**Exit criteria**: Tests still pass; rebuild pattern is either generalized or documented as intentional duplication

---

## TDD Cycle 10: Integration — Full Delivery Flow + Config Reload (IT-NOT-593-002, IT-NOT-593-003)

### Phase 28 — RED: Write failing tests for full flow and reload

**Tests**: `test/integration/notification/teams_delivery_test.go`

| Test ID | Assertion |
|---------|-----------|
| `IT-NOT-593-002` | Full flow: routing config → credential resolution → Teams delivery → mock receives valid Workflows webhook payload with Adaptive Card |
| `IT-NOT-593-003` | Config reload: stale Teams keys unregistered, new keys registered |

**Actions**:
1. IT-002: Full pipeline test → assert mock received JSON with `type: "message"` and Adaptive Card attachment
2. IT-003: Load config A → reload config B → verify A unregistered and B registered

**Exit criteria**: Tests fail (wiring not complete) or pass (from Phase 26)

### Phase 29 — GREEN: Wire remaining integration paths

**Actions**:
1. Fix any wiring issues discovered
2. Ensure mock server validates `Content-Type: application/json`

**Exit criteria**: IT-NOT-593-002 and IT-NOT-593-003 pass

### Phase 30 — REFACTOR: Clean up integration test infrastructure

**Actions**:
1. Extract `setupTeamsTestInfra()` helper
2. Consider shared `setupDeliveryTestInfra(channelPrefix)` if PD and Teams setup is >80% similar
3. Ensure httptest servers properly closed

**Exit criteria**: Tests still pass; infrastructure is reusable

---

## TDD Cycle 11: Integration — Workflows Format Validation (IT-NOT-593-004)

### Phase 31 — RED: Write failing test for Workflows format at integration level

**Tests**: `test/integration/notification/teams_delivery_test.go`

| Test ID | Assertion |
|---------|-----------|
| `IT-NOT-593-004` | Mock server validates `Content-Type: application/json` and Workflows payload structure (not legacy MessageCard) |

**Actions**:
1. Set up mock server that inspects request body
2. Deliver notification → capture body at mock
3. Parse JSON → assert `type == "message"`, `attachments[0].contentType == "application/vnd.microsoft.card.adaptive"`
4. Assert NO `@type: "MessageCard"` field in payload

**Exit criteria**: Test should pass if implementation is correct from earlier phases

### Phase 32 — GREEN: Confirm Workflows format in integration

**Actions**:
1. Verify test passes
2. If not, trace payload through delivery pipeline and fix

**Exit criteria**: IT-NOT-593-004 passes

### Phase 33 — REFACTOR: Final integration cleanup

**Actions**:
1. Consolidate any duplicated setup across integration tests
2. Add descriptive `IT-NOT-593-NNN:` test name prefixes
3. Verify all mock servers properly shut down

**Exit criteria**: All 4 integration tests pass; code is clean

---

## CHECKPOINT 3: Full Test Suite Audit

**Actions**:
1. Run ALL Teams unit tests (15): `go test ./test/unit/notification/delivery/... -ginkgo.focus="Teams" -v`
2. Run ALL Teams integration tests (4): `go test ./test/integration/notification/... -ginkgo.focus="Teams" -v`
3. Run ALL PagerDuty tests: verify 0 regressions from Teams work
4. Run ALL existing notification tests (no filter): verify 0 regressions
5. Measure per-tier coverage:
   - Unit: `go test -coverprofile` on `pkg/notification/delivery/teams*.go` → >=80%
   - Integration: `go test -coverprofile` on routing handler Teams paths → >=80%
6. Full build: `go build ./...`
7. Lint: `go vet ./...`
8. Verify `Service` interface compliance: `var _ delivery.Service = (*TeamsDeliveryService)(nil)`
9. Check all tests use `Eventually()` for async (no `time.Sleep`)
10. Verify no `Skip()` or `XIt`
11. Cross-reference test IDs against test plan — ensure 100% coverage of planned scenarios

**Finding resolution**: Address all findings.

---

## CHECKPOINT 4: Final Acceptance Audit (Both #60 and #593)

**Actions**:
1. Walk through EVERY acceptance criterion from Issue #593:
   - [ ] Teams delivery service implements `Service` interface
   - [ ] Payload uses Power Automate Workflows format (NOT legacy MessageCard)
   - [ ] Adaptive Card with RCA summary, affected resource, confidence, status
   - [ ] Approval card includes kubectl command
   - [ ] Distinct card layouts for approval, status-update, escalation, completion
   - [ ] 28KB payload size guard with truncation
   - [ ] `CredentialRef` on `TeamsConfig`
   - [ ] `QualifiedChannels()` treats Teams as credential channel
   - [ ] `ValidateCredentialRefs()` validates Teams refs
   - [ ] `teams` added to DS audit schema
   - [ ] Error classification: 5xx/429 retryable, 4xx permanent
   - [ ] Unit tests >=80% coverage
   - [ ] Integration tests with mock endpoint
   - [ ] Routing config supports Teams receiver type
2. Cross-validate #60 (PagerDuty) still passes all acceptance criteria
3. Verify both channels can coexist in same routing config (multi-channel receiver)
4. Run combined test: routing config with Slack + PD + Teams receiver → all three deliver
5. Verify no new dependencies in `go.mod` (raw HTTP for both PD and Teams)
6. Full `go build ./...` one final time
7. Document known v1.4 limitations (no Teams bot, no dedup, no cross-channel bridge)

**Finding resolution**: Address all findings. Document deferred items for v1.5.

---

## Summary

| Phase | Type | Tests | Description |
|-------|------|-------|-------------|
| 0 | Prereq | — | Shared infrastructure (completed) |
| 1-3 | TDD Cycle 1 | UT-593-013, 014 | Routing config validation |
| 4-6 | TDD Cycle 2 | UT-593-001 | Workflows format compliance |
| 7-9 | TDD Cycle 3 | UT-593-002 | Card content (RCA, resource) |
| 10-12 | TDD Cycle 4 | UT-593-003, 004, 005, 015 | Per-type card layouts |
| CP1 | Checkpoint | — | Adaptive Card audit |
| 13-15 | TDD Cycle 5 | UT-593-006 | Retryable errors + Deliver impl |
| 16-18 | TDD Cycle 6 | UT-593-007, 008 | Permanent + TLS errors |
| 19-21 | TDD Cycle 7 | UT-593-009, 010 | 28KB size guard |
| 22-24 | TDD Cycle 8 | UT-593-011, 012 | Edge cases |
| CP2 | Checkpoint | — | Full unit test audit |
| 25-27 | TDD Cycle 9 | IT-593-001 | Credential flow integration |
| 28-30 | TDD Cycle 10 | IT-593-002, 003 | Full flow + reload |
| 31-33 | TDD Cycle 11 | IT-593-004 | Workflows format integration |
| CP3 | Checkpoint | — | Full test suite audit |
| CP4 | Checkpoint | — | Final acceptance audit (both #60 + #593) |

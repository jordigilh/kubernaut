# Implementation Plan: PagerDuty Delivery Channel (#60)

**Issue**: #60 — feat(notification): implement PagerDuty delivery channel
**Test Plan**: [TEST_PLAN.md](TEST_PLAN.md)
**Created**: 2026-03-04
**Status**: Draft

---

## Execution Strategy

This plan follows **strict TDD RED → GREEN → REFACTOR** with each phase as a discrete implementation step. Checkpoints perform rigorous due diligence between logical groups.

**Total phases**: 30 (10 TDD cycles × 3 phases each) + 4 audit checkpoints = 34 steps

**Test grouping rationale**: Tests are grouped into TDD cycles by functional area. Each cycle produces a failing test (RED), minimal passing implementation (GREEN), and cleanup (REFACTOR) before proceeding.

---

## Phase 0: Prerequisites (already completed)

The following shared infrastructure was completed in the previous session:

- [x] `ChannelPagerDuty` added to CRD enum (F-1)
- [x] `PagerDutyConfig` restructured with `CredentialRef` (F-2)
- [x] `ValidateCredentialRefs()` extended for PD (F-6)
- [x] `QualifiedChannels()` updated for PD per-receiver names (F-4)
- [x] `collectPagerDutyCredentialRefs()` added (F-5)
- [x] `ReloadRoutingFromContent` validates all credential refs (F-5)
- [x] `registeredPagerDutyKeys` added to reconciler struct (F-5)
- [x] All existing tests updated and passing

---

## TDD Cycle 1: Routing Config Validation (UT-NOT-060-013, UT-NOT-060-014)

### Phase 1 — RED: Write failing tests for routing config PD validation

**Tests**: `test/unit/notification/delivery/pagerduty_test.go` (new file)

| Test ID | Assertion |
|---------|-----------|
| `UT-NOT-060-013` | `QualifiedChannels()` returns `pagerduty:<receiver>` for PD with CredentialRef |
| `UT-NOT-060-014` | `ValidateCredentialRefs()` fails when PagerDutyConfig has empty CredentialRef |

**Actions**:
1. Create `test/unit/notification/delivery/pagerduty_test.go` with Ginkgo suite
2. Write test for `QualifiedChannels()` with PD receiver having `CredentialRef: "pd-key"` → expect `pagerduty:<name>`
3. Write test for `ValidateCredentialRefs()` with empty `CredentialRef` → expect error containing `credentialRef`

**Exit criteria**: Tests compile but FAIL (no PD delivery code yet; routing config tests should pass since Phase 0 landed the changes — adjust: these tests validate Phase 0 work and should PASS immediately, confirming the contract)

> **Note**: Since Phase 0 already implemented these routing config changes, these tests serve as **regression guards** and will pass immediately. This is acceptable — they formalize the contract.

### Phase 2 — GREEN: Verify Phase 0 implementation passes

**Actions**:
1. Run tests → confirm both pass (Phase 0 code already satisfies the contract)
2. If any fail, fix the Phase 0 implementation

**Exit criteria**: UT-NOT-060-013 and UT-NOT-060-014 pass

### Phase 3 — REFACTOR: Clean up test helpers

**Actions**:
1. Extract `newPagerDutyReceiver(name, credRef string)` test helper for reuse across PD tests
2. Ensure test descriptions follow Ginkgo BDD style

**Exit criteria**: Tests still pass; helpers extracted for reuse

---

## TDD Cycle 2: Payload Construction — Core Structure (UT-NOT-060-001, UT-NOT-060-005)

### Phase 4 — RED: Write failing tests for Events API v2 payload structure

**Tests**: `test/unit/notification/delivery/pagerduty_test.go`

| Test ID | Assertion |
|---------|-----------|
| `UT-NOT-060-001` | Payload has `routing_key`, `event_action=trigger`, `payload.severity`, `payload.summary`, `payload.source`, `payload.component` |
| `UT-NOT-060-005` | `dedup_key` equals `notification.Name` |

**Actions**:
1. Write test calling `buildPagerDutyPayload(notification, routingKey)` (function does not exist yet)
2. Assert JSON structure matches Events API v2 schema
3. Assert `dedup_key` equals NR name

**Exit criteria**: Tests fail to compile (`buildPagerDutyPayload` undefined)

### Phase 5 — GREEN: Implement `buildPagerDutyPayload`

**Files**: `pkg/notification/delivery/pagerduty_payload.go` (new)

**Actions**:
1. Create `pagerduty_payload.go` with `buildPagerDutyPayload(notification *notificationv1alpha1.NotificationRequest, routingKey string) ([]byte, error)`
2. Define PD Events API v2 payload struct types: `PagerDutyEvent`, `PagerDutyPayload`
3. Map: `routing_key` ← param, `event_action` ← "trigger", `dedup_key` ← `notification.Name`
4. Map: `payload.severity` ← `mapSeverity(notification.Spec.Priority)`, `payload.summary` ← subject, `payload.source` ← namespace
5. Implement `mapSeverity`: critical→critical, high→error, medium→warning, low→info

**Exit criteria**: UT-NOT-060-001 and UT-NOT-060-005 pass

### Phase 6 — REFACTOR: Extract types, improve naming

**Actions**:
1. Move PD struct types to top of file with clear doc comments
2. Ensure `mapSeverity` is a clean switch with default fallback
3. Verify `go vet` and lint clean

**Exit criteria**: Tests still pass; code is clean and well-documented

---

## TDD Cycle 3: Payload Content — custom_details (UT-NOT-060-002, UT-NOT-060-003, UT-NOT-060-004)

### Phase 7 — RED: Write failing tests for payload content details

**Tests**: `test/unit/notification/delivery/pagerduty_test.go`

| Test ID | Assertion |
|---------|-----------|
| `UT-NOT-060-002` | `custom_details` contains RCA summary, confidence score, affected resource |
| `UT-NOT-060-003` | Severity mapping table-driven: critical→critical, high→error, medium→warning, low→info |
| `UT-NOT-060-004` | `custom_details` includes `kubectl kubernaut chat rar/{name} -n {namespace}` |

**Actions**:
1. Write table-driven test for severity mapping (4 entries)
2. Write test asserting `custom_details` contains `rca_summary`, `confidence`, `affected_resource`, `kubectl_command`
3. Assert kubectl command format: `kubectl kubernaut chat rar/<RR-name> -n <namespace>`

**Exit criteria**: Tests fail (custom_details not populated yet)

### Phase 8 — GREEN: Populate custom_details in payload

**Files**: `pkg/notification/delivery/pagerduty_payload.go`

**Actions**:
1. Build `custom_details` map from `notification.Spec.Context` fields
2. Add kubectl command: `fmt.Sprintf("kubectl kubernaut chat rar/%s -n %s", rrName, namespace)`
3. Extract RR name from `notification.Spec.Context.Lineage.RemediationRequest`
4. Add `confidence` from `notification.Spec.Context.Workflow.Confidence`

**Exit criteria**: UT-NOT-060-002, 003, 004 pass

### Phase 9 — REFACTOR: Extract custom_details builder

**Actions**:
1. Extract `buildCustomDetails(notification) map[string]string` helper
2. Handle nil Context sub-structs gracefully (no panics on nil pointers)
3. Add doc comments explaining field sources

**Exit criteria**: Tests still pass; nil-safety verified

---

## CHECKPOINT 1: Unit Payload Audit

**Actions**:
1. Verify all 7 payload-related unit tests pass (UT-NOT-060-001..005, 013, 014)
2. Run `go vet ./pkg/notification/delivery/...`
3. Measure coverage of `pagerduty_payload.go` — target >=80%
4. Validate JSON output against PagerDuty Events API v2 reference schema
5. Check for hardcoded strings that should be constants
6. Verify no anti-patterns: no `time.Sleep`, no `Skip()`, no `ToNot(BeNil)` existence-only assertions

**Finding resolution**: Address all findings before proceeding to error classification.

---

## TDD Cycle 4: Error Classification — Retryable (UT-NOT-060-006, UT-NOT-060-015)

### Phase 10 — RED: Write failing tests for retryable errors

**Tests**: `test/unit/notification/delivery/pagerduty_test.go`

| Test ID | Assertion |
|---------|-----------|
| `UT-NOT-060-006` | HTTP 500, 502, 503, 429 → `IsRetryableError(err) == true` |
| `UT-NOT-060-015` | HTTP 202 → success (nil error) |

**Actions**:
1. Write table-driven test with httptest mock returning each status code
2. Assert `IsRetryableError(err)` for 5xx/429
3. Assert nil error for 200 and 202

**Exit criteria**: Tests fail (`Deliver` method does not exist yet)

### Phase 11 — GREEN: Implement `Deliver` method with HTTP client

**Files**: `pkg/notification/delivery/pagerduty.go` (new)

**Actions**:
1. Create `PagerDutyDeliveryService` struct with `webhookURL string`, `httpClient *http.Client`
2. Implement `NewPagerDutyDeliveryService(webhookURL string, timeout time.Duration) *PagerDutyDeliveryService`
3. Implement `Deliver(ctx, notification) error`:
   - Call `buildPagerDutyPayload(notification, s.routingKey)`
   - HTTP POST to webhook URL with JSON payload
   - Classify response: 2xx = success, 5xx/429 = `RetryableError`, 4xx = permanent error
4. Reuse `isRetryableStatusCode` from `slack.go` (already shared in delivery package)

**Exit criteria**: UT-NOT-060-006 and UT-NOT-060-015 pass

### Phase 12 — REFACTOR: Consolidate HTTP patterns with Slack

**Actions**:
1. Verify error message format is consistent with Slack (`"pagerduty webhook returned %d"`)
2. Ensure response body is read and included in error messages
3. Verify `defer resp.Body.Close()` pattern matches Slack

**Exit criteria**: Tests still pass; code follows established delivery patterns

---

## TDD Cycle 5: Error Classification — Permanent + TLS (UT-NOT-060-007, UT-NOT-060-008)

### Phase 13 — RED: Write failing tests for permanent errors

**Tests**: `test/unit/notification/delivery/pagerduty_test.go`

| Test ID | Assertion |
|---------|-----------|
| `UT-NOT-060-007` | HTTP 400, 401, 403, 404 → `IsRetryableError(err) == false` AND `err != nil` |
| `UT-NOT-060-008` | TLS cert error → permanent error (BR-NOT-058) |

**Actions**:
1. Table-driven test for 4xx status codes
2. TLS test: use httptest TLS server with invalid cert, verify `isTLSError` classification

**Exit criteria**: Tests fail or pass depending on implementation state (Phase 11 may already handle 4xx correctly)

### Phase 14 — GREEN: Ensure permanent error paths work

**Actions**:
1. If 4xx tests already pass from Phase 11, verify assertions are tight (not just `err != nil` but also `!IsRetryableError(err)`)
2. Add TLS error detection reusing `isTLSError` from `slack.go`

**Exit criteria**: UT-NOT-060-007 and UT-NOT-060-008 pass

### Phase 15 — REFACTOR: Extract shared error helpers if needed

**Actions**:
1. Verify `isTLSError` is already shared (it is — in `slack.go`, same package)
2. If not extractable, ensure it's accessible; it's package-level so OK
3. Clean up test descriptions

**Exit criteria**: Tests still pass; no duplication in error classification

---

## TDD Cycle 6: Payload Size Guard (UT-NOT-060-009, UT-NOT-060-010)

### Phase 16 — RED: Write failing tests for 512KB size guard

**Tests**: `test/unit/notification/delivery/pagerduty_test.go`

| Test ID | Assertion |
|---------|-----------|
| `UT-NOT-060-009` | Payload >512KB triggers truncation; result <=512KB |
| `UT-NOT-060-010` | Truncated payload contains `"[truncated — full details in audit trail]"` and `correlation_id` |

**Actions**:
1. Create NR with 600KB body text
2. Call `buildPagerDutyPayload` → assert `len(jsonBytes) <= 512*1024`
3. Parse truncated JSON → assert `custom_details.rca_summary` contains truncation marker
4. Assert `custom_details.correlation_id` is NR name

**Exit criteria**: Tests fail (no truncation logic yet)

### Phase 17 — GREEN: Implement truncation in payload builder

**Files**: `pkg/notification/delivery/pagerduty_payload.go`

**Actions**:
1. After building payload, marshal to JSON and check size
2. If >512KB: truncate `payload.custom_details.rca_summary` with marker
3. Re-marshal and verify size; iterate truncation if needed
4. Always include `correlation_id` in custom_details

**Exit criteria**: UT-NOT-060-009 and UT-NOT-060-010 pass

### Phase 18 — REFACTOR: Extract size guard as reusable helper

**Actions**:
1. Extract `truncatePayloadToLimit(payload []byte, limit int, marker string) ([]byte, error)` if pattern is general enough for Teams reuse
2. Otherwise keep inline with clear constants: `const pagerDutyPayloadLimit = 512 * 1024`
3. Ensure truncation is deterministic (same input → same output)

**Exit criteria**: Tests still pass; size constant is named, not magic number

---

## TDD Cycle 7: Context Cancellation + Constructor Guard (UT-NOT-060-011, UT-NOT-060-012)

### Phase 19 — RED: Write failing tests for edge cases

**Tests**: `test/unit/notification/delivery/pagerduty_test.go`

| Test ID | Assertion |
|---------|-----------|
| `UT-NOT-060-011` | Cancelled context → error returned immediately |
| `UT-NOT-060-012` | Empty webhook URL → `Deliver` returns descriptive error |

**Actions**:
1. Create context with `context.WithCancel`, cancel immediately, call `Deliver` → expect context error
2. Create service with empty URL, call `Deliver` → expect error mentioning "webhook URL"

**Exit criteria**: Tests may partially pass (context cancellation is handled by `http.NewRequestWithContext`); empty URL guard needs implementation

### Phase 20 — GREEN: Add URL validation guard

**Actions**:
1. In `Deliver`, check `s.webhookURL == ""` → return `fmt.Errorf("pagerduty webhook URL is empty")`
2. Context cancellation should already work via `http.NewRequestWithContext`

**Exit criteria**: UT-NOT-060-011 and UT-NOT-060-012 pass

### Phase 21 — REFACTOR: Improve error messages

**Actions**:
1. Ensure all error messages include context (e.g., NR name) for debugging
2. Verify error wrapping uses `%w` for `errors.Is`/`errors.As` compatibility
3. Clean up test helper usage

**Exit criteria**: Tests still pass; errors are descriptive and wrappable

---

## CHECKPOINT 2: Full Unit Test Audit

**Actions**:
1. Run ALL 15 unit tests → 100% pass rate
2. Measure coverage: `go test -coverprofile` on `pkg/notification/delivery/pagerduty*.go` → target >=80%
3. Run `go vet ./...` and `go build ./...`
4. Verify no anti-patterns in tests:
   - No `time.Sleep()`
   - No `Skip()`
   - No `ToNot(BeNil)` existence-only checks (all assertions check concrete values)
5. Verify interface compliance: `var _ delivery.Service = (*PagerDutyDeliveryService)(nil)`
6. Check for unused imports, dead code paths
7. Validate all error paths are tested (no untested error returns)

**Finding resolution**: Address all findings. Reassess coverage gap and add tests if below 80%.

---

## TDD Cycle 8: Integration — Per-Receiver Credential Flow (IT-NOT-060-001)

### Phase 22 — RED: Write failing integration test for credential resolution

**Tests**: `test/integration/notification/pagerduty_delivery_test.go` (new file)

| Test ID | Assertion |
|---------|-----------|
| `IT-NOT-060-001` | PD service registered via credential resolver; delivery succeeds through orchestrator |

**Actions**:
1. Create integration test file with Ginkgo suite
2. Set up: temp dir with credential file, credential resolver, delivery orchestrator, routing config with PD receiver
3. Call `ReloadRoutingFromContent` → trigger `rebuildPagerDutyDeliveryServices`
4. Deliver through orchestrator to `pagerduty:<receiver>` key → assert httptest mock receives request

**Exit criteria**: Test fails (`rebuildPagerDutyDeliveryServices` not implemented yet)

### Phase 23 — GREEN: Implement `rebuildPagerDutyDeliveryServices`

**Files**: `internal/controller/notification/routing_handler.go`

**Actions**:
1. Implement `rebuildPagerDutyDeliveryServices(ctx, config)` following `rebuildSlackDeliveryServices` pattern:
   - Unregister stale PD keys from `r.registeredPagerDutyKeys`
   - For each PD config: resolve credential → create `PagerDutyDeliveryService` → register with orchestrator
   - Track new keys in `r.registeredPagerDutyKeys`
2. Call `rebuildPagerDutyDeliveryServices` from `ReloadRoutingFromContent` after `rebuildSlackDeliveryServices`

**Exit criteria**: IT-NOT-060-001 passes

### Phase 24 — REFACTOR: Align rebuild pattern with Slack

**Actions**:
1. Ensure logging format matches Slack pattern ("Registered per-receiver PagerDuty delivery")
2. Verify circuit breaker wrapping is consistent (if CB is available, wrap PD service)
3. Extract common rebuild logic if >80% overlap with Slack version (consider generic helper)

**Exit criteria**: Tests still pass; logging and CB handling consistent

---

## TDD Cycle 9: Integration — Full Delivery Flow + Config Reload (IT-NOT-060-002, IT-NOT-060-003)

### Phase 25 — RED: Write failing tests for full flow and reload

**Tests**: `test/integration/notification/pagerduty_delivery_test.go`

| Test ID | Assertion |
|---------|-----------|
| `IT-NOT-060-002` | Full flow: routing config → credential resolution → PD delivery → mock receives valid Events API v2 payload |
| `IT-NOT-060-003` | Config reload: stale PD keys unregistered, new keys registered |

**Actions**:
1. IT-002: Set up complete pipeline (routing config with PD receiver, credential file, reconciler with orchestrator) → create NR → deliver → assert mock received valid JSON
2. IT-003: Load config with receiver A → verify key A registered → reload with receiver B → verify key A unregistered AND key B registered

**Exit criteria**: Tests fail (integration wiring not complete)

### Phase 26 — GREEN: Wire remaining integration paths

**Actions**:
1. Ensure `rebuildPagerDutyDeliveryServices` correctly unregisters stale keys
2. Verify orchestrator `UnregisterChannel` is called for old keys
3. Fix any wiring issues discovered during integration

**Exit criteria**: IT-NOT-060-002 and IT-NOT-060-003 pass

### Phase 27 — REFACTOR: Clean up integration test infrastructure

**Actions**:
1. Extract `setupPagerDutyTestInfra()` helper for reuse
2. Ensure httptest servers are properly closed in `AfterEach`
3. Verify no resource leaks (goroutines, file handles)

**Exit criteria**: Tests still pass; infrastructure is reusable

---

## TDD Cycle 10: Integration — Dedup Key (IT-NOT-060-004)

### Phase 28 — RED: Write failing test for dedup_key in delivered payload

**Tests**: `test/integration/notification/pagerduty_delivery_test.go`

| Test ID | Assertion |
|---------|-----------|
| `IT-NOT-060-004` | Mock server receives payload with `dedup_key` matching NR name |

**Actions**:
1. Set up full delivery pipeline
2. Create NR with name "test-nr-123"
3. Deliver → capture request body at mock server
4. Parse JSON → assert `dedup_key == "test-nr-123"`

**Exit criteria**: Test may pass if dedup_key is correctly set from unit implementation

### Phase 29 — GREEN: Confirm dedup_key propagation

**Actions**:
1. If test passes, verify assertion is tight (exact match, not substring)
2. If test fails, trace the dedup_key through the delivery pipeline and fix

**Exit criteria**: IT-NOT-060-004 passes

### Phase 30 — REFACTOR: Final integration cleanup

**Actions**:
1. Consolidate any duplicated setup across integration tests
2. Verify all mock servers validate `Content-Type: application/json`
3. Add descriptive test names following `IT-NOT-060-NNN:` convention

**Exit criteria**: All 4 integration tests pass; code is clean

---

## CHECKPOINT 3: Full Test Suite Audit

**Actions**:
1. Run ALL unit tests (15): `go test ./test/unit/notification/delivery/... -ginkgo.focus="PagerDuty" -v`
2. Run ALL integration tests (4): `go test ./test/integration/notification/... -ginkgo.focus="PagerDuty" -v`
3. Run EXISTING notification tests (no PagerDuty filter): verify 0 regressions
4. Measure per-tier coverage:
   - Unit: `go test -coverprofile` on `pkg/notification/delivery/pagerduty*.go` → >=80%
   - Integration: `go test -coverprofile` on routing handler PD paths → >=80%
5. Full build: `go build ./...`
6. Lint: `go vet ./...`
7. Verify `Service` interface compliance: `var _ delivery.Service = (*PagerDutyDeliveryService)(nil)` compiles
8. Check all tests use `Eventually()` for async operations (no `time.Sleep`)
9. Verify no `Skip()` or `XIt` in any test
10. Cross-reference test IDs against test plan — ensure 100% coverage of planned scenarios

**Finding resolution**: Address all findings. If coverage <80% on any tier, add targeted tests. If regressions found, fix before proceeding.

---

## CHECKPOINT 4: Final Acceptance Audit

**Actions**:
1. Walk through EVERY acceptance criterion from Issue #60:
   - [ ] PagerDuty delivery service implements `Service` interface
   - [ ] Events API v2 payload correctly maps NR fields
   - [ ] `dedup_key` set to NR name
   - [ ] Error classification: 5xx/429 retryable, 4xx permanent
   - [ ] 512KB payload size guard
   - [ ] `CredentialRef` on `PagerDutyConfig`
   - [ ] `QualifiedChannels()` treats PD as credential channel
   - [ ] `ValidateCredentialRefs()` validates PD refs
   - [ ] `ChannelPagerDuty` enum in CRD
   - [ ] DS audit schema includes `pagerduty`
   - [ ] Routing config supports PD receiver type
   - [ ] Unit tests >=80% coverage
   - [ ] Integration tests with mock endpoint
2. Verify kubectl command format in payload: `kubectl kubernaut chat rar/{name} -n {namespace}`
3. Verify no new dependencies added to `go.mod` (raw HTTP, no PD SDK)
4. Run full `go build ./...` one final time
5. Document any known v1.4 limitations (no auto-resolve, no cross-channel bridge)

**Finding resolution**: Address all findings. Document deferred items for v1.5.

---

## Summary

| Phase | Type | Tests | Description |
|-------|------|-------|-------------|
| 0 | Prereq | — | Shared infrastructure (completed) |
| 1-3 | TDD Cycle 1 | UT-060-013, 014 | Routing config validation |
| 4-6 | TDD Cycle 2 | UT-060-001, 005 | Payload core structure |
| 7-9 | TDD Cycle 3 | UT-060-002, 003, 004 | Payload custom_details |
| CP1 | Checkpoint | — | Unit payload audit |
| 10-12 | TDD Cycle 4 | UT-060-006, 015 | Retryable errors |
| 13-15 | TDD Cycle 5 | UT-060-007, 008 | Permanent + TLS errors |
| 16-18 | TDD Cycle 6 | UT-060-009, 010 | Size guard |
| 19-21 | TDD Cycle 7 | UT-060-011, 012 | Edge cases |
| CP2 | Checkpoint | — | Full unit test audit |
| 22-24 | TDD Cycle 8 | IT-060-001 | Credential flow integration |
| 25-27 | TDD Cycle 9 | IT-060-002, 003 | Full flow + reload |
| 28-30 | TDD Cycle 10 | IT-060-004 | Dedup key integration |
| CP3 | Checkpoint | — | Full test suite audit |
| CP4 | Checkpoint | — | Final acceptance audit |

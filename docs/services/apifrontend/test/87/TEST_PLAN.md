# Test Plan: Phase 1 GA Remediation — Production Fixes

**Test Plan Identifier:** TP-AF-GA-P1
**Issue:** #87
**Version:** 1.1
**Date:** 2026-05-13

---

## 1. Introduction

This test plan validates the 8 critical/high production fixes in the GA
Remediation Phase 1 (issue #87). Each fix addresses a verified gap discovered
during the multi-dimensional FedRAMP readiness audit.

### 1.1 Scope

- CRIT-01: RecoverMiddleware router wiring
- CRIT-02: AgentCard RBAC wiring from main.go
- HIGH-01: KA REST client resilience + metrics wiring
- HIGH-02a: SeverityTriage metrics instrumentation
- HIGH-03: JWKS body size limit (MaxBytesReader)
- HIGH-04: JWKS URL HTTPS enforcement
- HIGH-05: DS TLS CA error handling (fail-fast)
- HIGH-06: Unified claim sanitization
- HIGH-07: NetworkPolicy Prometheus egress namespace

### 1.2 Out of Scope

- HIGH-02b (CRDSessionService controller-runtime wiring) — deferred to v1.6
- Phase 2 test quality gaps
- Phase 3 medium findings

### 1.3 References

- Reassessed GA Remediation Plan (`.cursor/plans/reassessed_ga_remediation_606f7754.plan.md`)
- 100 Go Mistakes and How to Avoid Them
- FedRAMP controls: SC-8 (TLS), AU-2 (audit), SI-10 (input validation)

---

## 2. Test Items

| Item | Package | Source Files |
|------|---------|-------------|
| RecoverMiddleware wiring | `internal/handler` | `router.go`, `recover.go` |
| AgentCard RBAC wiring | `cmd/apifrontend` | `main.go` |
| KA resilience config | `cmd/apifrontend` | `main.go` |
| SeverityTriage metrics | `internal/severity` | `triage.go` |
| JWKS body limit | `internal/auth` | `jwks_cache.go` |
| JWKS URL scheme | `internal/auth` | `config.go` |
| DS CA error handling | `cmd/apifrontend` | `main.go` |
| Claim sanitization | `internal/auth` | `jwt.go`, `sanitize.go` |
| NetworkPolicy | `deploy/kustomize` | `06-networkpolicy.yaml` |

---

## 3. Approach

TDD: write failing tests (RED), implement fix (GREEN), refactor.
Each test case asserts observable behavior — not implementation details.

---

## 4. Pass/Fail Criteria

- All tests pass with `go test -race`
- `go vet ./...` reports zero errors
- No regressions in existing test suites

---

## 5. Test Cases

### 5.1 CRIT-01: RecoverMiddleware Router Wiring

**Current behavior:** `NewRouter` returns a handler without panic recovery.
A panic in any middleware or route handler crashes the process.

**Required behavior:** Outermost `RecoverMiddleware` wraps the entire handler
tree. Panics are caught, `af_http_panics_total` is incremented, and RFC 7807
`application/problem+json` 500 is returned.

| TC ID | Description | Input | Expected Result | Type |
|-------|-------------|-------|-----------------|------|
| TC-P1-01a | String panic increments counter | `panic("boom")` | 500 + counter == 1 | Unit |
| TC-P1-01b | Error-typed panic increments counter | `panic(errors.New(...))` | 500 + counter == 1 | Unit |
| TC-P1-01c | Nil panic returns 500 | `panic(nil)` | 500 | Unit |
| TC-P1-01d | Runtime OOB panic increments counter | `s[42]` on empty slice | 500 + counter == 1 | Unit |
| TC-P1-01e | Normal handler passes through | 200 OK handler | 200 + counter == 0 | Unit |
| TC-P1-01f | Late panic after headers written | WriteHeader then panic | counter == 1 (process survives) | Unit |
| TC-P1-01g | 10 concurrent panics all counted | 10 goroutines | counter == 10 | Unit |
| TC-P1-01h | Router-level integration: panic on /mcp | POST /mcp to panicking handler | 500 + counter | Integration |

### 5.2 CRIT-02: AgentCard RBAC Wiring

**Current behavior:** `NewAgentCardHandler` receives no `RBACRoles` or
`GroupMapping`. All callers see the full skill list regardless of identity.

**Required behavior:** RBAC roles and group mapping from config are passed.
Authenticated callers see filtered skills; unauthenticated see shell card.

| TC ID | Description | Input | Expected Result | Type |
|-------|-------------|-------|-----------------|------|
| TC-P1-02a | Unauthenticated request returns shell card | GET without identity | 200 + empty skills | Unit |
| TC-P1-02b | Authenticated SRE gets SRE tools only | Identity with group "sre" | Skills filtered to SRE role | Unit |
| TC-P1-02c | Unknown group returns empty skills | Identity with group "unknown" | 200 + empty skills | Unit |
| TC-P1-02d | Multiple groups merge allowed tools | Identity with groups ["sre","cicd"] | Union of SRE + CICD skills | Unit |

### 5.3 HIGH-01: KA Resilience + Metrics Wiring

**Current behavior:** `ka.NewClient` is called with only `BaseURL` and
`BaseTransport`. Circuit breaker uses defaults; no metrics collectors.

**Required behavior:** `cfg.Resilience.KA` fields are mapped to `ka.Config`.
`ClientMetrics` are passed from the metrics registry.

| TC ID | Description | Input | Expected Result | Type |
|-------|-------------|-------|-----------------|------|
| TC-P1-03a | KA CB triggers after threshold | N+1 failures | `af_circuit_breaker_state{dependency="ka"}` goes to 1 (open) | Unit |
| TC-P1-03b | KA retry counter increments | Retryable 503 from KA | `af_downstream_retry_total{dependency="ka"}` > 0 | Unit |
| TC-P1-03c | KA duration histogram populated | Successful KA call | `af_downstream_request_duration_seconds{dependency="ka"}` has observations | Unit |

### 5.4 HIGH-02a: SeverityTriage Metrics Instrumentation

**Current behavior:** `SeverityTriageTotal`, `SeverityTriageDuration`, and
`SeverityTriageErrorsTotal` are registered but never incremented.

**Required behavior:** On each `Triage()` call, the outcome is recorded:
success increments Total + Duration; error increments Errors.

| TC ID | Description | Input | Expected Result | Type |
|-------|-------------|-------|-----------------|------|
| TC-P1-04a | Successful Tier 1 triage records total + duration | Firing alert match | `Total{tier="firing_alert", severity="critical"}` == 1 | Unit |
| TC-P1-04b | Tier 3 LLM failure records error | LLM returns error | `Errors{tier="llm_triage", error_type="llm_failure"}` == 1 | Unit |
| TC-P1-04c | Disabled triage emits no metrics | `Enabled: false` | All counters == 0 | Unit |
| TC-P1-04d | Tier 2 success records rule_evaluation source | Rule match | `Total{tier="rule_evaluation"}` == 1 | Unit |
| TC-P1-04e | Duration histogram has observations on success | Any successful triage | `Duration` count > 0 | Unit |

### 5.5 HIGH-03: JWKS Body Size Limit

**Current behavior:** `fetchJWKS` decodes the response body with no size cap.
A malicious JWKS endpoint can exhaust memory.

**Required behavior:** `http.MaxBytesReader` limits body to 1 MiB. Oversized
responses fail with a decode error.

| TC ID | Description | Input | Expected Result | Type |
|-------|-------------|-------|-----------------|------|
| TC-P1-05a | Normal JWKS (<1 MiB) decoded successfully | Valid 10-key JWKS | Parsed key set | Unit |
| TC-P1-05b | JWKS at exactly 1 MiB boundary | 1 MiB JSON body | Parsed or error (boundary) | Unit |
| TC-P1-05c | JWKS exceeding 1 MiB rejected | 2 MiB response body | Error containing "decode JWKS" | Unit |
| TC-P1-05d | Empty JWKS body returns decode error | 0-byte response | Error | Unit |

### 5.6 HIGH-04: JWKS URL HTTPS Enforcement

**Current behavior:** JWKS URL accepts `http://` unconditionally, even when
`AllowInsecureIssuers` is false.

**Required behavior:** JWKS URL follows the same HTTPS enforcement as the
issuer URL. `http://` is rejected unless `AllowInsecureIssuers` is true.

| TC ID | Description | Input | Expected Result | Type |
|-------|-------------|-------|-----------------|------|
| TC-P1-06a | HTTPS JWKS URL passes validation | `https://idp/jwks` | No error | Unit |
| TC-P1-06b | HTTP JWKS URL rejected (insecure=false) | `http://idp/jwks` | Error containing "JWKS URL" | Unit |
| TC-P1-06c | HTTP JWKS URL allowed (insecure=true) | `http://idp/jwks` + `AllowInsecureIssuers` | No error | Unit |
| TC-P1-06d | Empty JWKS URL skips validation | `""` | No error | Unit |
| TC-P1-06e | javascript: scheme rejected | `javascript:alert(1)` | Error | Unit |

### 5.7 HIGH-05: DS TLS CA Error Handling

**Current behavior:** `CAReloadableTransport` error is discarded (`_, _, _ :=`).
A broken CA config silently falls back to `http.DefaultTransport`.

**Required behavior:** When `DSBaseURL` is configured and CA transport fails,
startup is refused. Consistent with Prometheus CA transport path.

| TC ID | Description | Input | Expected Result | Type |
|-------|-------------|-------|-----------------|------|
| TC-P1-07a | Valid CA file allows startup | Correct CA path | Audit writer wired to DS | Unit |
| TC-P1-07b | Missing CA file fails startup | Non-existent path + DSBaseURL set | `run()` returns 1 | Unit |
| TC-P1-07c | Empty DSBaseURL skips CA check | `DSBaseURL: ""` | Log-based writer (no error) | Unit |

### 5.8 HIGH-06: Unified Claim Sanitization

**Current behavior:** JWT `buildIdentity` uses `security.SanitizeClaimValue`
(weak: strips control chars only). TokenReview uses `auth.SanitizeClaimValue`
(strong: bidi, UTF-8 repair, truncation).

**Required behavior:** Both paths use `auth.SanitizeClaimValue`. JWT-derived
identities get bidi stripping, UTF-8 repair, and 256-char truncation.

| TC ID | Description | Input | Expected Result | Type |
|-------|-------------|-------|-----------------|------|
| TC-P1-08a | Bidi override stripped from JWT username | Username with U+202E | No bidi chars in identity | Unit |
| TC-P1-08b | Long JWT username truncated to 256 chars | 500-char username | `len(identity.Username) <= 256` | Unit |
| TC-P1-08c | Invalid UTF-8 in JWT group cleaned | Group with `\xff` | Valid UTF-8 in identity.Groups | Unit |
| TC-P1-08d | Normal claims pass through unchanged | "alice" | "alice" | Unit |
| TC-P1-08e | Null bytes removed from JWT claims | "alice\x00admin" | No null bytes | Unit |

### 5.9 HIGH-07: NetworkPolicy Prometheus Egress

**Current behavior:** Egress for port 9090 targets `kubernaut-system` namespace.
`config.yaml` references `prometheus.monitoring:9090` (namespace `monitoring`).

**Required behavior:** Egress namespace matches config: `monitoring`.

| TC ID | Description | Input | Expected Result | Type |
|-------|-------------|-------|-----------------|------|
| TC-P1-09a | NetworkPolicy allows egress to monitoring ns | YAML parse | `namespaceSelector` matches `monitoring` | Manifest |
| TC-P1-09b | Config and policy namespace agree | Cross-reference | Same namespace string | Manifest |

---

## 6. Test Environment

- Go 1.26+, Ginkgo/Gomega, `go test -race`
- No external services required (all mocked)
- Manifest tests use YAML parsing only

---

## 7. Schedule

| Phase | Description |
|-------|-------------|
| RED | Write all failing tests per TCs above |
| GREEN | Implement/verify each fix |
| REFACTOR | `go vet`, `golangci-lint`, `-race`, 100-go-mistakes |

---

## 8. Risks

| Risk | Mitigation |
|------|-----------|
| HIGH-05 watcher Start needs ctx before signal setup | Verify ctx is in scope at call site |
| HIGH-04 may break tests using http:// JWKS URLs | Tests must set `AllowInsecureIssuers: true` |
| HIGH-06 truncation may break tests with long claims | Update test assertions for 256-char max |
| CRIT-01 late panic after headers may corrupt SSE | Accepted tradeoff — process survival > clean response |

---

## 9. Traceability Matrix

Maps each TC to its implementing test file and function/spec name.

| TC ID | Test File | Test Function / Spec Name |
|-------|-----------|--------------------------|
| TC-P1-01a | `internal/handler/panic_recovery_test.go` | TC-C-01a: recovers from string panic |
| TC-P1-01b | `internal/handler/panic_recovery_test.go` | TC-C-01b: recovers from error-typed panic |
| TC-P1-01c | `internal/handler/panic_recovery_test.go` | TC-C-01c: recovers from nil panic |
| TC-P1-01d | `internal/handler/panic_recovery_test.go` | TC-C-01d: runtime OOB panic |
| TC-P1-01e | `internal/handler/panic_recovery_test.go` | TC-C-01e: normal handler passes through |
| TC-P1-01f | `internal/handler/panic_recovery_test.go` | TC-C-01f: late panic after headers written |
| TC-P1-01g | `internal/handler/panic_recovery_test.go` | TC-C-01g: 10 concurrent panics |
| TC-P1-01h | `internal/handler/panic_recovery_test.go` | TC-P1-01h: router-level integration |
| TC-P1-02a | `internal/handler/agentcard_test.go` | Unauthenticated returns empty skills |
| TC-P1-02b | `internal/handler/agentcard_test.go` | SRE group gets SRE tools only |
| TC-P1-02c | `internal/handler/agentcard_test.go` | Unknown group returns empty skills |
| TC-P1-02d | `internal/handler/agentcard_test.go` | Multiple groups merge allowed tools |
| TC-P1-03a | `internal/ka/rest_client_resilience_test.go` | TC-P1-03a: CB state gauge |
| TC-P1-03b | `internal/ka/rest_client_resilience_test.go` | TC-P1-03b: retry counter |
| TC-P1-03c | `internal/ka/rest_client_resilience_test.go` | TC-P1-03c: duration histogram |
| TC-P1-04a | `internal/severity/triage_test.go` | TC-P1-04a: Tier 1 total + duration |
| TC-P1-04b | `internal/severity/triage_test.go` | TC-P1-04b: Tier 3 LLM error counter |
| TC-P1-04c | `internal/severity/triage_test.go` | TC-P1-04c: disabled emits no metrics |
| TC-P1-04d | `internal/severity/triage_test.go` | TC-P1-04d: Tier 2 rule_evaluation |
| TC-P1-04e | `internal/severity/triage_test.go` | TC-P1-04e: duration observations |
| TC-P1-05a | `internal/auth/security_hardening_test.go` | TestJWKSCache_FetchBodySizeLimit_ValidSmall |
| TC-P1-05b | `internal/auth/security_hardening_test.go` | TestJWKSCache_FetchBodySizeLimit_ExactBoundary |
| TC-P1-05c | `internal/auth/security_hardening_test.go` | TestJWKSCache_FetchBodySizeLimit_Oversized |
| TC-P1-05d | `internal/auth/security_hardening_test.go` | TestJWKSCache_FetchBodySizeLimit_Empty |
| TC-P1-06a | `internal/auth/security_hardening_test.go` | TestAuthConfig_JWKSURLScheme_HTTPS |
| TC-P1-06b | `internal/auth/security_hardening_test.go` | TestAuthConfig_JWKSURLScheme_HTTP_Rejected |
| TC-P1-06c | `internal/auth/security_hardening_test.go` | TestAuthConfig_JWKSURLScheme_HTTP_AllowInsecure |
| TC-P1-06d | `internal/auth/config_test.go` | TestConfig_Validate_AcceptsEmptyJWKSURL |
| TC-P1-06e | `internal/auth/security_hardening_test.go` | TestAuthConfig_JWKSURLScheme_JavaScriptRejected |
| TC-P1-07a | `internal/tlswiring/tlswiring_test.go` | TestCAReloadableTransport_ValidCA |
| TC-P1-07b | `internal/tlswiring/tlswiring_test.go` | TestCAReloadableTransport_NonExistentFile |
| TC-P1-07c | `internal/tlswiring/tlswiring_test.go` | TestCAReloadableTransport_EmptyCA |
| TC-P1-08a | `internal/auth/security_hardening_test.go` | TestSanitizeClaimValue_RTLO |
| TC-P1-08b | `internal/auth/security_hardening_test.go` | TestSanitizeClaimValue_Truncation |
| TC-P1-08c | `internal/auth/security_hardening_test.go` | TestSanitizeClaimValue_InvalidUTF8 |
| TC-P1-08d | `internal/auth/security_hardening_test.go` | TestSanitizeClaimValue_Normal |
| TC-P1-08e | `internal/auth/security_hardening_test.go` | TestSanitizeClaimValue_NullByte |
| TC-P1-09a | `internal/handler/manifest_validation_test.go` | TC-P1-09a: Prometheus egress targets monitoring |
| TC-P1-09b | `internal/handler/manifest_validation_test.go` | TC-P1-09b: Prometheus namespace matches config |

---

## 10. TDD Refactor: 100 Go Mistakes Audit

Audit performed against [100 Go Mistakes and How to Avoid Them](https://100go.co/)
on all Phase 1 business code changes.

### 10.1 Findings

| # | Mistake | File | Finding | Resolution |
|---|---------|------|---------|------------|
| #2 | Unnecessary nested code | `cmd/apifrontend/main.go:98-104` | `if err == nil { happy } else { fail }` inverts happy path | **Fixed** — flipped to `if err != nil { return 1 }` |

### 10.2 Verified Clean

| Mistake | Relevant Code | Verdict |
|---------|--------------|---------|
| #1 Variable shadowing | `main.go` scoped `:=` in `if` blocks | Intentional — outer var consumed before shadow |
| #21 Inefficient slice init | `jwt.go:sanitizeGroups` | Uses `make([]string, len(groups))` — correct |
| #39 String concatenation | `recover.go` single `fmt.Sprintf` | Not a loop concat — correct |
| #42 Receiver type | `Triager` pointer receiver | Mutable cache + large struct — pointer correct |
| #48 Panicking | `NewTriager` panics on nil LLM | Programmer error signal at startup — acceptable |
| #49 Error wrapping | `triage.go:299` `%w` | Wraps correctly for `errors.Is` chain |
| #52 Error handled twice | `main.go` log + return 1 | Startup exit path, not error propagation — correct |
| #53 Not handling error | `recover.go` stack trace | Logged via `logr.Logger` — verified |
| #54 Defer errors | `jwks_cache.go:189` | `_ = resp.Body.Close()` with explicit blank identifier |
| #60 Context misuse | `triagePipeline` | Checks `ctx.Err()` at tier boundaries — correct |

---

## 11. Execution Results

| Package | Tests | Status | Duration |
|---------|-------|--------|----------|
| `internal/handler` | 148 specs | PASS | ~5.3s |
| `internal/ka` | 24 specs | PASS | ~3.1s |
| `internal/severity` | 47 specs | PASS | ~3.1s |
| `internal/auth` | all specs | PASS | ~8.7s |
| `cmd/apifrontend` | all tests | PASS | ~2.9s |

All packages pass with `-race`. `go vet ./...` reports zero errors.

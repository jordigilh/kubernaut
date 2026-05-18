# Test Plan: Phase 2–3 GA Remediation — Test Quality & Medium Fixes

**Test Plan Identifier:** TP-AF-GA-P2
**Issue:** #87
**Version:** 1.0
**Date:** 2026-05-13
**Predecessor:** TP-AF-GA-P1 (Phase 1 Production Fixes)

---

## 1. Introduction

This test plan validates the Phase 2 test quality improvements and Phase 3
medium-severity production fixes for the v1.5.0-rc1 GA release (issue #87).
Phase 2 strengthens behavioral test coverage to ≥80% per modified tier. Phase 3
remediates 7 medium findings from the multi-dimensional FedRAMP readiness audit.

### 1.1 Scope

**Phase 2A — Strengthen Weak Cycle A Tests (4 items)**
- TC-A-08: Auth middleware CB metrics scrape (behavioral rewrite)
- TC-A-04: UserLimiter wiring verification (behavioral rewrite)
- TC-A-01e: E2E readiness 503 (defer with skip annotation)
- TC-A-metrics-01: E2E metric label completeness (method + path)

**Phase 2B — Cycle B Security Tests (~20 TCs)**
- JWKS body boundary (1 MiB exact, 1 MiB + 1)
- Issuer adversarial schemes (ftp, data, file, empty)
- nbf edge cases (epoch, negative)
- Replay protection (errors.Is, JTI uniqueness, audit reason)
- Claim sanitization (multi-group, 1 MB input)
- Concurrent JWKS refresh (10 goroutines + race detector)

**Phase 2C — Cycle C Handler/Config Tests (~15 TCs)**
- Panic recovery edge cases (nil, OOB, headers-sent, concurrent)
- Write deadline middleware (SSE exemption)
- Config DefaultConfig validation
- AgentCard RBAC filtering (5 scenarios)
- Shutdown graceful drain + ReplayCache stop

**Phase 3 — Medium Findings (7 production fixes)**
- MED-01: Trusted proxy X-Forwarded-For
- MED-02: JWKS CB label hashing
- MED-03: JWKS health in readiness probe
- MED-04: Generic panic message (no value reflection)
- MED-05: NaN/Inf exp claim rejection
- MED-06: ReplayCache.Stop idempotency (sync.Once)
- MED-10: FIPS GOEXPERIMENT=boringcrypto

### 1.2 Out of Scope

- HIGH-02b (CRDSessionService + TTL reconciler) — separate phase
- E2E 503 readiness strict assertion (no harness control)
- TC-C-04 config watcher default merge (no prod surface)
- TC-C-06 A2A non-stub (handler returns 501)
- TC-C-07 MCP session lifecycle (AcquireSession not called)
- TC-C-09 severity audit emitter (no audit interface)
- Memory-growth stress tests (needs dedicated hardware)

### 1.3 References

- TP-AF-GA-P1 (Phase 1 Test Plan, `docs/test/87/TEST_PLAN.md`)
- Reassessed GA Remediation Plan
- [100 Go Mistakes and How to Avoid Them](https://100go.co/)
- FedRAMP controls: SC-8 (TLS), AU-2 (audit), SI-10 (input validation), AC-7 (lockout)
- OWASP Top 10 (2021)
- IEEE 829-2008 Standard for Software and System Test Documentation

### 1.4 Business Acceptance Criteria

| BAC | Description | Phases |
|-----|-------------|--------|
| BAC-01 | Operators can filter Prometheus dashboards by HTTP method and path | 2A |
| BAC-02 | Auth CB state metric is observable per dependency for alerting | 2A |
| BAC-03 | Per-user rate limiting is wired and enforceable | 2A |
| BAC-04 | JWKS endpoints cannot exhaust AF memory via oversized responses | 2B |
| BAC-05 | Non-HTTPS issuer URLs are rejected in production configuration | 2B |
| BAC-06 | Token replay attacks are detected and produce distinct audit trails | 2B |
| BAC-07 | Claim values from untrusted tokens are sanitized against injection | 2B |
| BAC-08 | Concurrent JWKS fetches are race-free under load | 2B |
| BAC-09 | Panic recovery never leaks internal state to API callers | 2C, 3 |
| BAC-10 | Streaming (SSE) connections are not subject to write deadline | 2C |
| BAC-11 | Graceful shutdown drains in-flight requests and stops all goroutines | 2C |
| BAC-12 | Client IP extraction is accurate behind reverse proxies | 3 |
| BAC-13 | Readiness probe reflects all dependency health (including JWKS) | 3 |
| BAC-14 | FIPS-compliant cryptography for FedRAMP deployment | 3 |

---

## 2. Test Items

| Item | Package | Source Files | Phase |
|------|---------|-------------|-------|
| Auth middleware CB metrics | `cmd/apifrontend` | `main.go`, `main_wiring_test.go` | 2A |
| UserLimiter wiring | `cmd/apifrontend` | `main.go`, `main_wiring_test.go` | 2A |
| E2E readiness 503 | `test/e2e` | `operational_contract_test.go` | 2A |
| E2E metric labels | `test/e2e` | `operational_contract_test.go` | 2A |
| JWKS body boundary | `internal/auth` | `jwks_cache.go`, `security_hardening_test.go` | 2B |
| Issuer URL schemes | `internal/auth` | `config.go`, `security_hardening_test.go` | 2B |
| nbf edge cases | `internal/auth` | `jwt.go`, `jwt_test.go` | 2B |
| Replay protection | `internal/auth` | `jwt.go`, `replay_cache.go`, `middleware.go` | 2B |
| Claim sanitization | `internal/auth` | `sanitize.go`, `jwt.go` | 2B |
| JWKS concurrency | `internal/auth` | `jwks_cache.go` | 2B |
| Panic recovery edges | `internal/handler` | `recover.go`, `panic_recovery_test.go` | 2C |
| Write deadline | `internal/handler` | `router.go` | 2C |
| Config defaults | `internal/config` | `config.go` | 2C |
| AgentCard RBAC | `internal/handler` | `agentcard.go` | 2C |
| Shutdown wiring | `cmd/apifrontend` | `main.go` | 2C |
| Trusted proxy | `internal/httputil` | `clientip.go` | 3 |
| JWKS CB label | `internal/auth` | `jwks_cache.go` | 3 |
| JWKS readiness | `internal/auth`, `internal/handler` | `jwt.go`, `health.go`, `main.go` | 3 |
| Panic message | `internal/handler` | `recover.go` | 3 |
| Expiry NaN/Inf | `internal/auth` | `jwt.go` | 3 |
| ReplayCache.Stop | `internal/auth` | `replay_cache.go` | 3 |
| FIPS boringcrypto | build | `Dockerfile` | 3 |

---

## 3. Approach

### 3.1 TDD Methodology

Each phase follows strict RED → GREEN → REFACTOR:
- **RED**: Write tests expressing desired behavior; they MUST fail before implementation
- **GREEN**: Write minimal production code to pass; no gold-plating
- **REFACTOR**: Improve code quality; validate against 100 Go Mistakes

### 3.2 Test Quality Principles

- **Behavioral assertions**: Tests assert observable outcomes (HTTP responses, metric values, error types), not internal implementation details
- **80%+ coverage per tier**: Each modified package achieves ≥80% line coverage
- **Anti-pattern avoidance**:
  - No `if testing.Testing()` in production code
  - No shared mutable state between tests
  - No `time.Sleep` for timing — use channels, waitgroups, or Gomega `Eventually`
  - No asserting on log output — assert on observable behavior
  - Table-driven tests where >2 inputs vary
  - `t.Helper()` on all test helpers
  - `t.Run` for isolation; `t.Parallel()` where safe
  - `t.Cleanup()` over `defer` (survives `t.Fatal`)

### 3.3 GA Readiness Audit (14 Dimensions)

At each CHECKPOINT, all dimensions scored 1–3 (3=PASS, 2=ACCEPTABLE, 1=FAIL).
Gate: average ≥ 2.5, no dimension at 1. If confidence < 95%, ESCALATE.

| # | Dimension | Criteria |
|---|-----------|----------|
| 1 | Correctness | All tests pass with `-race -count=1`; zero `go vet` errors |
| 2 | Coverage | ≥80% line coverage on modified packages |
| 3 | Lint Compliance | Zero golangci-lint issues |
| 4 | Security (AppSec) | Zero gosec HIGH/MEDIUM; zero govulncheck |
| 5 | API Contract | OpenAPI validates; RFC 7807; AgentCard spec |
| 6 | Observability | All code paths emit metrics or structured logs |
| 7 | Resilience | CB/retry/timeout on external calls; shutdown tested |
| 8 | FedRAMP Alignment | AU-2 audit; SC-8 TLS; no PII in logs |
| 9 | Test Quality (QE) | Behavioral; no anti-patterns; table-driven; race-safe |
| 10 | Test Documentation | IEEE 829 plan current; traceability matrix complete |
| 11 | Regression Safety | No existing test weakened; no `t.Skip` without reason |
| 12 | Product Acceptance | BAC covered by tests; no untested user-facing behavior |
| 13 | UX / DX | Error messages actionable; responses consistent |
| 14 | Product Security | Threat model covered; auth bypass tested; OWASP gaps closed |

---

## 4. Pass/Fail Criteria

- All tests pass with `go test -race -count=1`
- `go vet ./...` reports zero errors
- `golangci-lint run` reports zero issues
- No regressions in existing test suites
- ≥80% line coverage on each modified package
- 14-dimension audit: average ≥ 2.5, no dimension at 1

---

## 5. Test Cases

### 5.1 Phase 2A: Strengthen Weak Cycle A Tests

#### TC-P2A-01: Auth Middleware CB Metrics Scrape (BAC-02)

| TC ID | Description | Input | Expected Result | Type |
|-------|-------------|-------|-----------------|------|
| TC-P2A-01a | buildAuthMiddleware with httptest JWKS produces CB metric | httptest JWKS server + valid config | `af_circuit_breaker_state{dependency=...}` present in registry | Unit |
| TC-P2A-01b | Middleware drives request and reports auth duration | Authenticated request through middleware | `af_auth_duration_seconds` has observations | Unit |

#### TC-P2A-02: UserLimiter Wiring (BAC-03)

| TC ID | Description | Input | Expected Result | Type |
|-------|-------------|-------|-----------------|------|
| TC-P2A-02a | MCPBridgeConfig accepts real UserLimiter | `ratelimit.NewUserLimiter(cfg)` | `bridgeCfg.UserLimiter != nil` | Unit |
| TC-P2A-02b | UserLimiter rate-limits after threshold | N+1 requests from same user | `AllowRequest` returns false | Unit |

#### TC-P2A-03: E2E Readiness 503 (Deferred)

| TC ID | Description | Input | Expected Result | Type |
|-------|-------------|-------|-----------------|------|
| TC-P2A-03a | E2E readiness test skipped with documented reason | N/A | `Skip("DEFERRED: ...")` | E2E |

#### TC-P2A-04: E2E Metric Label Completeness (BAC-01)

| TC ID | Description | Input | Expected Result | Type |
|-------|-------------|-------|-----------------|------|
| TC-P2A-04a | af_http_requests_total includes method label | Scrape /metrics | `method=` present in metric line | E2E |
| TC-P2A-04b | af_http_requests_total includes path label | Scrape /metrics | `path=` present in metric line | E2E |

---

### 5.2 Phase 2B: Cycle B Security Tests

#### TC-P2B-01: JWKS Body Boundary (BAC-04)

| TC ID | Description | Input | Expected Result | Type |
|-------|-------------|-------|-----------------|------|
| TC-P2B-01a | Exactly 1 MiB JWKS body accepted | 1<<20 bytes padded JSON | No error or decode error (boundary) | Unit |
| TC-P2B-01b | 1 MiB + 1 byte JWKS body rejected | (1<<20)+1 bytes | Error containing "decode JWKS" or "http: request body too large" | Unit |

#### TC-P2B-02: Issuer Adversarial Schemes (BAC-05)

| TC ID | Description | Input | Expected Result | Type |
|-------|-------------|-------|-----------------|------|
| TC-P2B-02a | ftp:// scheme rejected | `ftp://evil.com/jwks` | Error containing "unsupported scheme" | Unit |
| TC-P2B-02b | data: scheme rejected | `data:text/html,...` | Error containing "unsupported scheme" | Unit |
| TC-P2B-02c | file:// scheme rejected | `file:///etc/passwd` | Error containing "unsupported scheme" | Unit |
| TC-P2B-02d | Empty string issuer URL rejected | `""` | Error "must not be empty" | Unit |

#### TC-P2B-03: nbf Edge Cases (BAC-05)

| TC ID | Description | Input | Expected Result | Type |
|-------|-------------|-------|-----------------|------|
| TC-P2B-03a | nbf=0 (Unix epoch) is in the past | `"nbf": 0` | No error (epoch is well past) | Unit |
| TC-P2B-03b | nbf=-1 (before epoch) accepted | `"nbf": -1` | No error (before epoch is past) | Unit |
| TC-P2B-03c | nbf=NaN rejected | `"nbf": NaN` | Error wrapping ErrMalformedToken | Unit |
| TC-P2B-03d | nbf=+Inf rejected | `"nbf": +Inf` | Error wrapping ErrMalformedToken | Unit |

#### TC-P2B-04: Replay Protection (BAC-06)

| TC ID | Description | Input | Expected Result | Type |
|-------|-------------|-------|-----------------|------|
| TC-P2B-04a | Replayed JTI returns ErrTokenReplayed | Same JTI twice through Validate | `errors.Is(err, ErrTokenReplayed)` == true | Unit |
| TC-P2B-04b | Different JTI passes after replay check | JTI "a" then JTI "b" | Second validation succeeds | Unit |
| TC-P2B-04c | Replay classified as "token_replayed" in audit | Replay through middleware | audit reason == `"token_replayed"` | Unit |
| TC-P2B-04d | Missing JTI with replay enabled returns error | Token without jti claim | Error about missing JTI | Unit |

#### TC-P2B-05: Claim Sanitization Stress (BAC-07)

| TC ID | Description | Input | Expected Result | Type |
|-------|-------------|-------|-----------------|------|
| TC-P2B-05a | Multi-group with special chars all sanitized | `["\x00admin", "sre\u202e", "valid"]` | All output entries valid UTF-8, no control chars | Unit |
| TC-P2B-05b | 1 MB claim value truncated without OOM | 1<<20 byte string | `len(result) <= 256`; no panic | Unit |
| TC-P2B-05c | Empty string returns empty string | `""` | `""` | Unit |

#### TC-P2B-06: Concurrent JWKS Refresh (BAC-08)

| TC ID | Description | Input | Expected Result | Type |
|-------|-------------|-------|-----------------|------|
| TC-P2B-06a | 10-goroutine concurrent GetKeys with -race | 10 goroutines calling GetKeys | All return valid keyset; no race | Unit |
| TC-P2B-06b | Concurrent GetKeys with failing server | Mixed success/failure responses | No deadlock; errors returned cleanly | Unit |

---

### 5.3 Phase 2C: Cycle C Handler/Config Tests

#### TC-P2C-01: Panic Recovery Edges (BAC-09)

**Note:** TC-C-01a through TC-C-01g already exist from Phase 1. These TCs verify
they cover edge conditions and are not weakened.

| TC ID | Description | Input | Expected Result | Type |
|-------|-------------|-------|-----------------|------|
| TC-P2C-01a | Verify TC-C-01b (error-typed panic) still passes | Existing test | PASS | Regression |
| TC-P2C-01b | Verify TC-C-01d (runtime OOB) still passes | Existing test | PASS | Regression |
| TC-P2C-01c | Verify TC-C-01f (headers-already-sent) still passes | Existing test | PASS | Regression |
| TC-P2C-01d | Verify TC-C-01g (10 concurrent panics) still passes | Existing test | PASS | Regression |

#### TC-P2C-02: Write Deadline Middleware (BAC-10)

| TC ID | Description | Input | Expected Result | Type |
|-------|-------------|-------|-----------------|------|
| TC-P2C-02a | Non-SSE request has write deadline set | Regular POST /mcp | Response within deadline | Unit |
| TC-P2C-02b | SSE connection clears write deadline | SSE upgrade request | No timeout on long-lived connection | Unit |

#### TC-P2C-03: Config Guard (BAC-11)

| TC ID | Description | Input | Expected Result | Type |
|-------|-------------|-------|-----------------|------|
| TC-P2C-03a | DefaultConfig returns valid port | `config.DefaultConfig()` | `cfg.Server.Port == 8443` | Unit |
| TC-P2C-03b | DefaultConfig sets reasonable shutdown drain | `config.DefaultConfig()` | `cfg.Shutdown.DrainSeconds == 15` | Unit |
| TC-P2C-03c | DefaultConfig resilience fields are non-zero | `config.DefaultConfig()` | KA/DS/K8s CB thresholds > 0 | Unit |

#### TC-P2C-04: AgentCard RBAC Filtering (BAC-09)

**Note:** TC-P1-02a through TC-P1-02d exist from Phase 1. These verify
additional edge cases.

| TC ID | Description | Input | Expected Result | Type |
|-------|-------------|-------|-----------------|------|
| TC-P2C-04a | Admin role sees all tools | Identity with group mapped to "admin" | All skills returned | Unit |
| TC-P2C-04b | Skills are sorted by ID for determinism | Any role query | Skills in ID order | Unit |
| TC-P2C-04c | Group mapping resolves external group → internal role | External "platform-sre" → role "sre" | SRE skills returned | Unit |

#### TC-P2C-05: Shutdown / ReplayCache (BAC-11)

| TC ID | Description | Input | Expected Result | Type |
|-------|-------------|-------|-----------------|------|
| TC-P2C-05a | ConnectionTracker.DrainAll waits for in-flight | 1 active SSE + drain | DrainAll returns after SSE closes | Unit |
| TC-P2C-05b | ReplayCache.Stop is called during shutdown | Signal handler path | `done` channel closed | Unit |

---

### 5.4 Phase 3: Medium Findings

#### TC-P3-01: Trusted Proxy X-Forwarded-For (BAC-12)

| TC ID | Description | Input | Expected Result | Type |
|-------|-------------|-------|-----------------|------|
| TC-P3-01a | Single XFF hop returns correct IP | `X-Forwarded-For: 1.2.3.4` | `ExtractClientIP` returns `1.2.3.4` | Unit |
| TC-P3-01b | Multi-hop XFF returns rightmost untrusted | `X-Forwarded-For: 1.1.1.1, 10.0.0.1` with trust `10.0.0.0/8` | Returns `1.1.1.1` | Unit |
| TC-P3-01c | No XFF falls back to RemoteAddr | No header | RemoteAddr (host only) | Unit |
| TC-P3-01d | Spoofed XFF ignored when not from trusted proxy | XFF from untrusted source | RemoteAddr used | Unit |

#### TC-P3-02: JWKS CB Label Hashing (BAC-02)

| TC ID | Description | Input | Expected Result | Type |
|-------|-------------|-------|-----------------|------|
| TC-P3-02a | CB label uses SHA256 prefix, not full URL | JWKS URL `https://long.issuer.example.com/...` | Label `jwks_<first 12 chars of sha256>` | Unit |
| TC-P3-02b | Different URLs produce different labels | Two distinct JWKS URLs | Different label values | Unit |

#### TC-P3-03: JWKS Health in Readiness (BAC-13)

| TC ID | Description | Input | Expected Result | Type |
|-------|-------------|-------|-----------------|------|
| TC-P3-03a | Readyz returns 503 when JWKS CB is open | `validator.Ready() == false` | HTTP 503 | Unit |
| TC-P3-03b | Readyz returns 200 when all deps healthy | All checkers true | HTTP 200 | Unit |

#### TC-P3-04: Generic Panic Message (BAC-09)

| TC ID | Description | Input | Expected Result | Type |
|-------|-------------|-------|-----------------|------|
| TC-P3-04a | Problem+json detail does NOT contain panic value | `panic("secret-internal-state")` | `detail` field does not contain "secret-internal-state" | Unit |
| TC-P3-04b | Problem+json title is generic "Internal Server Error" | Any panic | `title == "Internal Server Error"` | Unit |

#### TC-P3-05: NaN/Inf Expiry Rejection (BAC-05)

| TC ID | Description | Input | Expected Result | Type |
|-------|-------------|-------|-----------------|------|
| TC-P3-05a | NaN exp claim rejected | `"exp": NaN` | Error wrapping ErrMalformedToken | Unit |
| TC-P3-05b | +Inf exp claim rejected | `"exp": +Inf` | Error wrapping ErrMalformedToken | Unit |
| TC-P3-05c | -Inf exp claim rejected | `"exp": -Inf` | Error wrapping ErrMalformedToken | Unit |
| TC-P3-05d | Normal future exp accepted | `"exp": now+1h` | No error | Unit |

#### TC-P3-06: ReplayCache.Stop Idempotency (BAC-11)

| TC ID | Description | Input | Expected Result | Type |
|-------|-------------|-------|-----------------|------|
| TC-P3-06a | Double Stop does not panic | Call `Stop()` twice | No panic | Unit |
| TC-P3-06b | Stop after Seen still returns cleanly | `Seen("jti1")` then `Stop()` | No panic | Unit |

#### TC-P3-07: FIPS boringcrypto (BAC-14)

| TC ID | Description | Input | Expected Result | Type |
|-------|-------------|-------|-----------------|------|
| TC-P3-07a | Dockerfile sets GOEXPERIMENT=boringcrypto | Read Dockerfile | Env var present in build stage | Build |

---

## 6. Test Environment

- Go 1.26+, Ginkgo v2.28/Gomega, `go test -race -count=1`
- No external services required (all mocked via httptest)
- Manifest tests use YAML parsing only
- Dockerfile test uses file inspection

---

## 7. Schedule

| Phase | RED | GREEN | REFACTOR | Checkpoint |
|-------|-----|-------|----------|------------|
| 2A | Write 4 TC groups | Strengthen tests | 100-go-mistakes | CHECKPOINT 1 |
| 2B | Write ~20 security TCs | Implement if gaps found | 100-go-mistakes | CHECKPOINT 2 |
| 2C | Write ~15 handler/config TCs | Implement if gaps found | 100-go-mistakes | CHECKPOINT 3 |
| 3 | Write 7 failing tests | Implement 7 fixes | 100-go-mistakes | CHECKPOINT 4 (FINAL) |

---

## 8. Risks

| Risk | Impact | Mitigation |
|------|--------|-----------|
| TC-P2B-04d (missing JTI) may not be validated today | Test fails RED; needs GREEN fix | Acceptable — TDD working as designed |
| TC-P2C-02b (SSE write deadline) tests unexported middleware | Requires test in same package or exported wrapper | Test via router behavior instead |
| TC-P3-01b (trusted proxy) needs config field that doesn't exist | GREEN phase adds `TrustedProxyCIDRs` config | Verify config struct is extensible |
| TC-P3-02 (CB label hash) changes metric label → dashboard impact | Document migration in CHANGELOG | Operators must update queries |
| TC-P3-07a (FIPS) may require CGO_ENABLED=1 | Changes build dependencies | Test both paths; document decision |

---

## 9. Traceability Matrix

### 9.1 Phase 2A

| TC ID | BAC | Test File | Test Function |
|-------|-----|-----------|---------------|
| TC-P2A-01a | BAC-02 | `cmd/apifrontend/main_wiring_test.go` | TestBuildAuthMiddleware_PassesCBMetrics |
| TC-P2A-01b | BAC-02 | `cmd/apifrontend/main_wiring_test.go` | TestBuildAuthMiddleware_PassesCBMetrics |
| TC-P2A-02a | BAC-03 | `cmd/apifrontend/main_wiring_test.go` | TestBridgeCfg_UserLimiter_IsWired |
| TC-P2A-02b | BAC-03 | `cmd/apifrontend/main_wiring_test.go` | TestBridgeCfg_UserLimiter_IsWired |
| TC-P2A-03a | — | `test/e2e/operational_contract_test.go` | TC-A-01e (Skip) |
| TC-P2A-04a | BAC-01 | `test/e2e/operational_contract_test.go` | TC-A-metrics-01 |
| TC-P2A-04b | BAC-01 | `test/e2e/operational_contract_test.go` | TC-A-metrics-01 |

### 9.2 Phase 2B

| TC ID | BAC | Test File | Test Function |
|-------|-----|-----------|---------------|
| TC-P2B-01a | BAC-04 | `internal/auth/security_hardening_test.go` | TestJWKSBoundary_Exact1MiB |
| TC-P2B-01b | BAC-04 | `internal/auth/security_hardening_test.go` | TestJWKSBoundary_Over1MiB |
| TC-P2B-02a | BAC-05 | `internal/auth/security_hardening_test.go` | TestIssuerURL_FTP |
| TC-P2B-02b | BAC-05 | `internal/auth/security_hardening_test.go` | TestIssuerURL_Data |
| TC-P2B-02c | BAC-05 | `internal/auth/security_hardening_test.go` | TestIssuerURL_File |
| TC-P2B-02d | BAC-05 | `internal/auth/security_hardening_test.go` | TestIssuerURL_Empty |
| TC-P2B-03a | BAC-05 | `internal/auth/jwt_test.go` | TestValidateNotBefore_Epoch |
| TC-P2B-03b | BAC-05 | `internal/auth/jwt_test.go` | TestValidateNotBefore_Negative |
| TC-P2B-03c | BAC-05 | `internal/auth/jwt_test.go` | TestValidateNotBefore_NaN |
| TC-P2B-03d | BAC-05 | `internal/auth/jwt_test.go` | TestValidateNotBefore_Inf |
| TC-P2B-04a | BAC-06 | `internal/auth/jwt_test.go` | TestReplay_ErrorsIs |
| TC-P2B-04b | BAC-06 | `internal/auth/jwt_test.go` | TestReplay_DifferentJTI |
| TC-P2B-04c | BAC-06 | `internal/auth/middleware_test.go` | TestReplay_AuditReason |
| TC-P2B-04d | BAC-06 | `internal/auth/jwt_test.go` | TestReplay_MissingJTI |
| TC-P2B-05a | BAC-07 | `internal/auth/security_hardening_test.go` | TestSanitize_MultiGroup |
| TC-P2B-05b | BAC-07 | `internal/auth/security_hardening_test.go` | TestSanitize_1MBInput |
| TC-P2B-05c | BAC-07 | `internal/auth/security_hardening_test.go` | TestSanitize_Empty |
| TC-P2B-06a | BAC-08 | `internal/auth/jwks_cache_test.go` | TestConcurrentGetKeys |
| TC-P2B-06b | BAC-08 | `internal/auth/jwks_cache_test.go` | TestConcurrentGetKeys_Failing |

### 9.3 Phase 2C

| TC ID | BAC | Test File | Test Function |
|-------|-----|-----------|---------------|
| TC-P2C-01a–d | BAC-09 | `internal/handler/panic_recovery_test.go` | (regression verification) |
| TC-P2C-02a | BAC-10 | `internal/handler/router_test.go` | TestWriteDeadline_NonSSE |
| TC-P2C-02b | BAC-10 | `internal/handler/router_test.go` | TestWriteDeadline_SSE |
| TC-P2C-03a | BAC-11 | `internal/config/config_test.go` | TestDefaultConfig_Port |
| TC-P2C-03b | BAC-11 | `internal/config/config_test.go` | TestDefaultConfig_DrainSeconds |
| TC-P2C-03c | BAC-11 | `internal/config/config_test.go` | TestDefaultConfig_Resilience |
| TC-P2C-04a | BAC-09 | `internal/handler/agentcard_test.go` | TestAgentCard_AdminAllTools |
| TC-P2C-04b | BAC-09 | `internal/handler/agentcard_test.go` | TestAgentCard_SortedByID |
| TC-P2C-04c | BAC-09 | `internal/handler/agentcard_test.go` | TestAgentCard_GroupMapping |
| TC-P2C-05a | BAC-11 | `internal/streaming/tracker_test.go` | TestDrainAll_WaitsForInFlight |
| TC-P2C-05b | BAC-11 | `cmd/apifrontend/main_wiring_test.go` | TestShutdown_ReplayCacheStop |

### 9.4 Phase 3

| TC ID | BAC | Test File | Test Function |
|-------|-----|-----------|---------------|
| TC-P3-01a | BAC-12 | `internal/httputil/clientip_test.go` | TestExtractClientIP_SingleXFF |
| TC-P3-01b | BAC-12 | `internal/httputil/clientip_test.go` | TestExtractClientIP_TrustedProxy |
| TC-P3-01c | BAC-12 | `internal/httputil/clientip_test.go` | TestExtractClientIP_NoXFF |
| TC-P3-01d | BAC-12 | `internal/httputil/clientip_test.go` | TestExtractClientIP_Spoofed |
| TC-P3-02a | BAC-02 | `internal/auth/jwks_cache_test.go` | TestJWKSCBLabel_SHA256 |
| TC-P3-02b | BAC-02 | `internal/auth/jwks_cache_test.go` | TestJWKSCBLabel_Unique |
| TC-P3-03a | BAC-13 | `cmd/apifrontend/main_wiring_test.go` | TestReadyz_JWKSCBOpen |
| TC-P3-03b | BAC-13 | `cmd/apifrontend/main_wiring_test.go` | TestReadyz_AllHealthy |
| TC-P3-04a | BAC-09 | `internal/handler/panic_recovery_test.go` | TestPanicMessage_NoValueReflection |
| TC-P3-04b | BAC-09 | `internal/handler/panic_recovery_test.go` | TestPanicMessage_GenericTitle |
| TC-P3-05a | BAC-05 | `internal/auth/jwt_test.go` | TestValidateExpiry_NaN |
| TC-P3-05b | BAC-05 | `internal/auth/jwt_test.go` | TestValidateExpiry_PosInf |
| TC-P3-05c | BAC-05 | `internal/auth/jwt_test.go` | TestValidateExpiry_NegInf |
| TC-P3-05d | BAC-05 | `internal/auth/jwt_test.go` | TestValidateExpiry_FutureValid |
| TC-P3-06a | BAC-11 | `internal/auth/replay_cache_test.go` | TestReplayCacheStop_Double |
| TC-P3-06b | BAC-11 | `internal/auth/replay_cache_test.go` | TestReplayCacheStop_AfterSeen |
| TC-P3-07a | BAC-14 | `build/dockerfile_test.go` | TestDockerfile_BoringCrypto |

---

## 10. Execution Results

*To be populated after each phase completes.*

| Phase | Package | Tests | Status | Duration | Coverage |
|-------|---------|-------|--------|----------|----------|
| 2A | | | | | |
| 2B | | | | | |
| 2C | | | | | |
| 3 | | | | | |

---

## 11. Checkpoint Audit Results

*To be populated at each checkpoint.*

### Checkpoint 1 (Post-Phase 2A)

| # | Dimension | Score | Notes |
|---|-----------|-------|-------|
| 1–14 | | | |
| **Average** | | | |
| **Confidence** | | | |

### Checkpoint 2 (Post-Phase 2B)

| # | Dimension | Score | Notes |
|---|-----------|-------|-------|
| 1–14 | | | |

### Checkpoint 3 (Post-Phase 2C)

| # | Dimension | Score | Notes |
|---|-----------|-------|-------|
| 1–14 | | | |

### Checkpoint 4 — FINAL (Post-Phase 3)

| # | Dimension | Score | Notes |
|---|-----------|-------|-------|
| 1–14 | | | |

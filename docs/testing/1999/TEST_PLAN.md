# Test Plan: #1999 — Source-Bound jti Replay Detection

## 1. Business Requirement

`BR-SECURITY-1505` (GAP-08, distributed jti replay cache). This fix corrects a functional
regression in the existing control, not a new business requirement: `ReplayCacheStore.Seen(jti)`'s
"single-use-ever" semantics reject any second presentation of the same `jti`, which is exactly how
a legitimate OAuth2 Bearer token is expected to be reused across a session. Confirmed as a live
production incident (diagnosed alongside #1995): with `auth.replayCache` enabled against a real
Valkey backend, every session died silently on its second authenticated call.

## 2. Root Cause

`jti` is fixed for a token's entire lifetime. "Seen once → reject forever (within TTL)" cannot
distinguish "the same legitimate client reusing its own token" from "a stolen token replayed by an
attacker" — it rejects both identically. `DD-PLATFORM-006` Decision Area 16 already reverted the
Helm chart's mandate to opt-in for this exact reason, but deliberately left `jwt.go`'s semantics
unchanged, deferring a real fix as "out of scope" (its Option 3). This plan implements that
deferred fix.

## 3. Fix Design (Option 2 + trusted-proxy hardening)

Bind replay detection to the request's source instead of treating every repeat presentation as a
replay:

- New `jti` → recorded with the current source key, not a replay.
- Existing `jti`, **same** source key → legitimate reuse, **not** a replay.
- Existing `jti`, **different** source key → replay, rejected with `ErrTokenReplayed`.

Source key resolution must itself be spoof-resistant. A new, self-contained
`trustedSourceResolver` (`pkg/apifrontend/auth/trusted_source.go`) mirrors the already-proven
`pkg/gateway/middleware/trusted_realip.go` pattern (DD-AUTH-003, issue #673 L-1): proxy headers
(`True-Client-IP` / `X-Real-IP` / `X-Forwarded-For`) are trusted only when the immediate peer's
`RemoteAddr` falls within a configured trusted-CIDR allowlist; otherwise the source key is always
`RemoteAddr`'s IP. Fail-closed: no CIDRs configured → headers never trusted.

This is deliberately **not** a change to `pkg/apifrontend/httputil.ExtractClientIP` (shared by
rate limiting and audit logging — zero reason to touch either) — the new resolver is local to the
`auth` package and used only for replay-cache source binding.

### API changes

| Component | Change |
|---|---|
| `ReplayCacheStore` interface | `Seen(jti string) bool` → `Seen(jti, sourceKey string) bool` |
| `ReplayCache` (in-memory) | entry value: `time.Time` → `{sourceKey string; expiry time.Time}` |
| `ValkeyReplayCache` | `SetNX(key, sourceKey, ttl)`; on miss, `GET` and compare to `sourceKey` (still one write + at most one read — no new race: the stored value is write-once until TTL eviction) |
| `JWTValidator.Validate(ctx, rawToken)` | signature **unchanged** — reads the source key via new `SourceIPFromContext(ctx)` (defaults to `""` when unset, mirroring the existing `WithUserIdentity`/`UserIdentityFromContext` context-value convention in `pkg/apifrontend/auth/context.go`) |
| `pkg/apifrontend/auth/middleware.go` | constructs one `trustedSourceResolver` (like `authMethod`), computes the per-request source key, calls `ctx = auth.WithSourceIP(ctx, key)` before `Validate` |
| `config.ReplayCacheConfig` | new `TrustedProxyCIDRs []string` (`yaml:"trustedProxyCIDRs,omitempty"`) |
| `cmd/apifrontend/auth_wiring.go` | wires `cfg.Auth.ReplayCache.TrustedProxyCIDRs` into `MiddlewareConfig.TrustedProxyCIDRs` |
| Helm chart | `values.yaml` + `apifrontend.yaml` expose `auth.replayCache.trustedProxyCIDRs` |

## 4. FedRAMP / SOC2 Control Mapping

- **SI-10** (information input validation): the trusted-CIDR check on `RemoteAddr` before
  trusting any proxy header is itself an input-validation control on otherwise attacker-controlled
  headers.
- **AC-6** (least privilege / defense in depth): replay detection remains layered on top of
  signature/expiry/audience/issuer validation, unaffected by this change (BR-SECURITY-1505's own
  "defense in depth, not sole control" design note).
- **AU-3** (content of audit records): `emitAuthFailure`'s existing `token_replayed` classification
  is unchanged; the underlying rejection reason text is clarified ("used from a different source").

## 5. Pyramid Invariant — Test Scenario Inventory

| Tier | Test ID(s) | File | Proves |
|---|---|---|---|
| UT | `UT-AF-1999-001..004` | `pkg/apifrontend/auth/trusted_source_1999_test.go` | `trustedSourceResolver`: fail-closed default, trusted-CIDR header trust, header priority, spoofed header from untrusted peer ignored |
| UT | `TestReplayCache_*` (rewritten) | `pkg/apifrontend/auth/replay_cache_test.go` | in-memory backend: same-source reuse allowed, different-source rejected |
| UT | Ginkgo specs (rewritten) | `pkg/apifrontend/auth/replay_cache_valkey_test.go` | Valkey backend: same-source cross-replica reuse allowed, different-source cross-replica rejected (preserves the original "shared state across replicas" proof) |
| UT | `TestValidate_*` (rewritten) | `pkg/apifrontend/auth/security_hardening_test.go` | `Validate()` end-to-end: same-source reuse allowed, different-source rejected, missing-jti/different-jti behavior unchanged |
| UT | `ADV-017*` (rewritten) | `pkg/apifrontend/auth/adversarial_jwt_test.go` | black-box `Validate()` via exported `auth.WithSourceIP` |
| UT | `TestWithReplayCache_*` (rewritten) | `pkg/apifrontend/auth/validator_coverage_test.go` | business-outcome framing: reused token from same caller succeeds end-to-end |
| IT | `IT-AF-1999-001..003` | `test/integration/apifrontend/auth_middleware_test.go` | full `MiddlewareWithConfig` wiring: `TrustedProxyCIDRs` config → real HTTP request → source-key resolution → `ReplayCacheStore` — same client 2 calls succeed, spoofed-source-from-untrusted-peer has no effect, genuinely different trusted source rejected |
| Wiring | n/a | `cmd/apifrontend/auth_wiring.go`, `cmd/apifrontend/replay_cache_wiring_test.go` | mechanical signature update; `buildAuthMiddleware` threads config through unchanged control flow |
| E2E | `E2E-FP-AF-1999-001` | `test/e2e/fullpipeline/19_af_replay_cache_test.go` | real deployed chart + real Valkey: same client, same DEX-issued Bearer token reused across 3 independent authenticated `GET /a2a/access` requests, none rejected as `401` (the exact production-incident scenario) |

**Correction (post-implementation review):** this plan originally claimed E2E coverage was
unnecessary because "`fullpipeline`'s AF token reuse already covers the happy path." That was
inaccurate — `apifrontend.config.auth.replayCache.enabled` was never set to `true` anywhere in
`test/infrastructure/fullpipeline_e2e_helm.go` (or any other E2E Helm install), so the feature
this fix repairs was never actually exercised against a real cluster before this test plan
revision. Two changes close that gap:

1. `fullpipeline_e2e_helm.go` now sets `apifrontend.config.auth.replayCache.enabled=true` and
   `apifrontend.config.auth.replayCache.tls.enabled=true` for the *entire* FP suite (not just one
   test) — `redisAddr`/`redisDB`/`credentialsPath`/the Valkey volume mount/the NetworkPolicy
   egress rule are all chart-computed once `enabled=true` (confirmed via `helm template`), so no
   further overrides are needed.
2. Because `getAFToken()` (`suite_test.go`) caches one DEX-issued token per Ginkgo process and
   every other AF-touching FP spec reuses it via that cache, turning this on makes **every**
   existing FP AF test an implicit regression check for the same-source-reuse happy path (had the
   old `Seen(jti)`-only semantics still been in effect, the *second* authenticated MCP call
   anywhere in the whole process would 401). `E2E-FP-AF-1999-001` above is the explicit,
   narrowly-named version of that same check, so a regression fails with a clear message instead
   of diffusely across unrelated specs.

   `E2E-FP-AF-1999-001` targets `GET /a2a/access` (#1919, `ConsoleAccessHandler`) rather than a raw
   `/mcp` handshake. Both routes pass through the identical `AuthMiddleware` chain
   (`pkg/apifrontend/handler/router.go`'s `a2aChain`/`mcpChain` both wrap `cfg.AuthMiddleware`), so
   this exercises the exact same `JWTValidator`/`ReplayCacheStore` code path #1999 fixed. An earlier
   revision of this test drove `/mcp` `initialize` directly and intermittently broke
   `06_af_audit_trace_test.go`'s `E2E-FP-AF-001`: `pkg/apifrontend/handler/mcp.go`'s `seenSessions`
   sentinel can only ever emit `apifrontend.mcp.session_init` once per AF process lifetime (it's
   keyed on the *absence* of an `Mcp-Session-Id` header, which every session-less `initialize` call
   shares), so whichever spec's `initialize` call ran first in Ginkgo's execution order "won" that
   one-shot audit event and starved every other direct-`/mcp` spec of it. `/a2a/access` is a
   lightweight SAR-backed check with no MCP session semantics, so it avoids that collision
   entirely. The `mcp.go` sentinel bug itself is pre-existing and out of scope for #1999; tracked
   separately.

The "different source is still rejected" side of the fix is deliberately left to the IT tier
(`IT-AF-1999-002`) rather than E2E: reliably forcing two *genuinely different* apparent source IPs
through a Kind NodePort (SNAT'd to the node's IP by kube-proxy) would require faking
`X-Forwarded-For` through a configured trusted-proxy CIDR, which doesn't reflect this suite's real
ingress topology and would test the trusted-CIDR plumbing, not the deployed system.

## 6. Documentation Updates

- `docs/architecture/decisions/DD-PLATFORM-006-helm-chart-configuration-surface-reduction.md`:
  new Decision Area 18, superseding DA16's Option 3 ("redesign to distinguish legitimate reuse from
  actual replay") from "rejected as out-of-scope" to "implemented," with the live-incident evidence
  that motivated revisiting it.
- `docs/services/apifrontend/test/cycle-b-security-hardening/TEST_PLAN.md`: SEC-05 section updated
  to reflect source-bound semantics.
- `docs/services/apifrontend/operations/runbooks/RB-AF-012.md`: updated `ErrTokenReplayed`
  troubleshooting guidance (now source-bound, not single-use-ever).
- `docs/requirements/BR-SECURITY-1505-distributed-jwt-replay-cache.md`: updated to describe the
  source-bound semantics as the current design.

## 7. Confidence

90% (per interactive design review) — false-positive fix (same-client reuse allowed) is ~92%
confident; the residual gap is IP-roaming mid-session (mobile NAT / proxy rotation), a low-probability
exposure for this product's actual client population (browser console sessions, server-to-server
agents/CLI — not mobile), and strictly smaller than the "every 2nd request, unconditionally"
failure mode already confirmed in production.

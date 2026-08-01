# DD-KA-005: LLM Input Sanitization

**Status**: ✅ **APPROVED** (implemented, Go)
**Date**: December 9, 2025
**Decision Makers**: Kubernaut Agent (KA) Team, Security Team
**Priority**: P0 (CRITICAL)

**Last Updated**: 2026-08-01 — Renamed from `DD-HAPI-005` and rewritten against the actual Go
implementation ([Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806)). The original
document described a proposed Python `llm_sanitizer.py` module and `Tool.invoke()` monkey-patch
that were never implemented — HolmesGPT-API was rewritten in Go before this design shipped. This
version documents the credential-sanitization pipeline that actually exists in
`internal/kubernautagent/` and `pkg/kubernautagent/tools/sanitization/` today, including where the
Go implementation diverges from the original Python design (fail-closed vs. fail-open, no dedicated
JWT/base64 rules, tool-error paths not yet sanitized).

---

## Context

### Problem Statement

Kubernaut Agent (KA) sends data to external LLM providers (OpenAI, Anthropic, Vertex AI, etc.) for
AI-powered Kubernetes investigation. This data flow includes:

1. **Investigation and workflow-selection prompts** built from signal context, error messages, and
   descriptions
2. **Tool call results** from Kubernetes toolsets (logs, pod descriptions, events) and MCP tools
3. **Fleet/remote-cluster tool results** proxied through the MCP Gateway

**Security Risk**: This data may contain credentials that would be leaked to external LLM providers:

| Data Source | Risk Level | Example Credential Exposure |
|-------------|------------|----------------------------|
| `kubectl logs` output | 🔴 HIGH | Database passwords in application logs |
| Tool error messages | 🔴 HIGH | Connection strings in error stack traces |
| `kubectl get pods -o yaml` | 🟡 MEDIUM | Environment variables with secrets |
| K8s `Secret`/`SecretList` objects | 🔴 HIGH | Base64-encoded `data`/`stringData` values |
| `kubectl get events` | 🟡 MEDIUM | Secrets in event messages |

### Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| **FR-1** | All tool output sent to the LLM must be sanitized for credentials | P0 |
| **FR-2** | Sanitization must cover generic credential patterns AND K8s `Secret` object shapes | P0 |
| **FR-3** | Reuse the shared Go sanitization library (`pkg/shared/sanitization/`) for consistency with Gateway/Notification | P1 |
| **FR-4** | Sanitization must not silently corrupt investigation-relevant context | P0 |
| **FR-5** | Sanitization failures must be observable and must not leak unsanitized content | P0 |

---

## Decision

### APPROVED: Multi-Stage Sanitization Pipeline Wrapping Tool Execution

KA wraps every LLM-directed tool call through a generic `Pipeline` of `Stage`s, rather than
patching individual tool methods (the Go equivalent of the originally-proposed Python
`Tool.invoke()` wrapper):

```
┌───────────────────────────────────────────────────────────────────────────┐
│                    Kubernaut Agent (KA) — executeTool()                   │
├───────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  LLM requests tool call                                                  │
│         │                                                                │
│         ▼                                                                │
│  Tool executes (kubectl logs, kubectl get, MCP tool, Fleet remote tool)  │
│         │                                                                │
│         ├─ error ──────────────────────────────▶ raw err.Error() to LLM  │
│         │                                         (NOT sanitized — gap,  │
│         │                                          see "Known Gaps")     │
│         ▼ success                                                       │
│  sanitization.Pipeline.Run(ctx, result)                                  │
│    ├─ Stage: CredentialSanitizer (G4) — pkg/shared/sanitization rules   │
│    ├─ Stage: SecretSanitizer (K8S-SECRET) — redacts Secret data/stringData │
│    └─ Stage: InjectionSanitizer (I1) — prompt-injection phrase stripping │
│         │                                                                │
│         ├─ error ──▶ withhold tool output entirely (fail-closed, SOC2)  │
│         ▼ success                                                       │
│  Sanitized result ──▶ alignment shadow-agent ──▶ summarize/truncate ──▶ LLM │
│                                                                           │
└───────────────────────────────────────────────────────────────────────────┘
```

Each stage is independently toggleable via config. This is a deliberate simplification vs. the
original Python design: Go tool results are pre-flattened to `string` by the tool registry before
`executeTool` runs, so there is no `Any`-dispatching sanitizer branching over `str`/`dict`/`list`/`None`
like the original `sanitize_for_llm(content: Any)` — the one exception is `SecretSanitizer`, which
does narrow structure-aware JSON parsing to redact only `data`/`stringData` fields of K8s `Secret`
objects, then remarshals to a string.

---

## Alternatives Considered

### Alternative 1: Disable High-Risk Tools

**Approach**: Disable log-reading and secret-adjacent tools to prevent credential leakage at the
source.

**Cons**: Logs are essential for root cause analysis; severely degrades investigation quality; does
not address prompt-level leakage.

**Confidence**: 10% — REJECTED (breaks core functionality).

### Alternative 2: RBAC Restriction Only

**Approach**: Rely on the KA ServiceAccount's RBAC to prevent secret access.

**Cons**: Does not protect against secrets embedded in logs, error messages, or ConfigMaps that
*are* readable under the granted RBAC scope.

**Confidence**: 30% — INSUFFICIENT (partial protection only).

### Alternative 3: Comprehensive Sanitization Pipeline (APPROVED, implemented)

**Approach**: Wrap all tool output with a generic, staged sanitization pipeline reusing the shared
Go credential-pattern library.

**Pros**: Complete coverage of tool output; consistent patterns with Gateway/Notification; preserves
full investigation capability; auditable (pipeline stage names logged).

**Cons**: Tool-*error* paths are not yet wrapped (see Known Gaps); fail-closed semantics mean a
sanitizer bug can withhold legitimate tool output rather than degrade gracefully.

**Confidence**: 90% — APPROVED, implemented with known follow-up work.

---

## Implementation

### Component 1: Shared Credential Pattern Library

**Location**: `pkg/shared/sanitization/sanitizer.go`

Assembles `DefaultRules()` from 12 builder functions covering (as of this rewrite) 25 uniquely-named
rules / 28 total `Rule` entries (one builder is intentionally invoked twice to preserve legacy rule
ordering):

| Category | Rule names |
|----------|-----------|
| Container patterns | `generator-url`, `annotations-json` |
| Passwords | `password-json`, `password-plain`, `password-url` |
| API keys | `api-key-json`, `api-key-plain`, `openai-key` |
| Tokens | `bearer-token`, `github-token`, `token-json`, `token-plain` |
| Secrets | `secret-json`, `secret-plain` |
| Authorization | `authorization-header` |
| Cloud credentials | `aws-access-key`, `aws-secret-key` |
| Database URLs | `postgresql-url`, `mysql-url`, `mongodb-url`, `redis-url` |
| Certificates/keys | `pem-certificate`, `private-key` |
| K8s secrets | `k8s-secret-data` |
| PII (`BR-GATEWAY-042`) | `email-address`, `ipv4-address` |

This is a superset of the original 17-category Python spec, but with two notable regressions:
there is **no standalone JWT-token regex** and **no standalone generic base64-secret regex** — JWTs
are only caught incidentally when prefixed with `Bearer ` or a `token:`/`access_token:` key. The
database-URL category, singular in the original spec, is now split into 4 dedicated rules
(`postgresql-url`/`mysql-url`/`mongodb-url`/`redis-url`).

The library also provides `SanitizeWithFallback`/`SafeFallback` (`sanitizer.go`) — a panic-recovering
regex path with a simple keyword-based fallback — used by Gateway (`BR-GATEWAY-042`) and
Notification (`BR-NOT-055`). **KA does not call this fallback path** (see Component 2).

### Component 2: KA Sanitization Pipeline

**Location**: `pkg/kubernautagent/tools/sanitization/` (`pipeline.go`, `credential.go`, `secret.go`,
`injection.go`)

```go
// Pipeline chains sanitization stages in order (G4 -> I1 per DD-KA-019-003).
type Stage interface {
    Name() string
    Sanitize(ctx context.Context, input string) (string, error)
}

type Pipeline struct {
    stages []Stage
}
```

Three stages, each independently configurable:

1. **`CredentialSanitizer` (G4)** — wraps `pkg/shared/sanitization.Sanitizer` for the generic
   pattern set above.
2. **`SecretSanitizer` (K8S-SECRET)** — narrowly parses tool output as `map[string]json.RawMessage`
   to redact `data`/`stringData` fields of K8s `Secret`/`SecretList` shapes.
3. **`InjectionSanitizer` (I1)** — strips prompt-injection phrases (unrelated to credentials; shared
   pipeline slot per `DD-KA-019-003`).

`CredentialSanitizer.Sanitize` calls the underlying library's plain `Sanitize(input)` — **not**
`SanitizeWithFallback` — and always returns a `nil` error from the regex path itself.

### Component 3: Wiring Into the Investigation Loop

**Production wiring**: `cmd/kubernautagent/datastorage.go` (`buildSanitizationPipeline`), attached to
the investigator at `cmd/kubernautagent/bootstrap.go`.

**Call site**: `internal/kubernautagent/investigator/investigator_tools.go`, inside `executeTool`
(the Go equivalent of the Python `Tool.invoke()` wrapper — a single generic interception point for
every tool call dispatched by the LLM, rather than per-tool method patching):

```go
if inv.pipeline.Sanitizer != nil {
    sanitized, sanitizeErr := inv.pipeline.Sanitizer.Run(ctx, result)
    if sanitizeErr != nil {
        inv.logger.Error(sanitizeErr, "sanitization failed, fail-closed for SOC2 compliance",
            "tool", name,
        )
        errResult := toolErrorJSON("sanitization failed: tool output withheld")
        alignment.SubmitToolStep(ctx, name, errResult)
        return errResult
    }
    result = sanitized
}
```

**Fail-closed by design**: if the pipeline itself errors, KA withholds the tool output entirely
rather than falling back to a simpler redaction (a deliberate divergence from the original Python
spec's fail-open/degrade approach, chosen for SOC2 compliance — an unsanitized leak is worse than a
withheld tool result).

### Known Gaps (tracked, not yet closed)

1. **Tool-call error messages are not sanitized.** When a tool call itself fails (not the
   sanitization stage, but the underlying tool execution), the raw `err.Error()` string is JSON-wrapped
   by `toolErrorJSON` and sent to the LLM without passing through the pipeline. Connection strings,
   auth failures, or other credential-bearing error text can reach the LLM unredacted. This directly
   corresponds to the original spec's FR-3 ("sanitize error messages in tool results"), which was
   never carried forward to the error path in Go.
2. **No dedicated JWT or generic base64-secret pattern.** See Component 1.

Both gaps are candidates for a follow-up hardening pass; they are not blocking for V1.0 because the
higher-risk path (successful tool output, which is the common case for logs/describe/get) is covered.

---

## Consequences

### Positive

- ✅ Credentials in successful tool output cannot leak to external LLM providers
- ✅ K8s `Secret` object shapes get structure-aware redaction, not just regex-on-string
- ✅ Reuses the same pattern library as Gateway/Notification (`pkg/shared/sanitization/`)
- ✅ Fail-closed semantics prevent an unsanitized leak on pipeline failure

### Negative

- ⚠️ Tool-error paths bypass sanitization entirely (Known Gap 1)
- ⚠️ Fail-closed means a sanitizer bug degrades investigation availability (withheld tool output)
  rather than degrading gracefully — an explicit trade-off favoring security over availability
- ⚠️ Pattern coverage diverged from the original 17-category spec (see Component 1); some formats
  (standalone JWT, base64-secret) are only caught incidentally, if at all

---

## Validation

### Test Coverage

| Location | Test Count | Coverage |
|----------|-----------|----------|
| `pkg/kubernautagent/tools/sanitization/credential_test.go` | 26 (incl. 17-entry `DescribeTable`) | Passwords, API keys, Bearer/GitHub tokens, JWT-via-Bearer, secrets, auth headers, AWS keys, DB URLs, private keys |
| `pkg/kubernautagent/tools/sanitization/injection_test.go` | 17 | Prompt-injection phrase stripping |
| `pkg/kubernautagent/tools/sanitization/secret_test.go` | 7 | K8s `Secret`/`SecretList` redaction |
| `pkg/kubernautagent/tools/sanitization/pipeline_test.go` | 2 | Stage chaining, fail-closed behavior |
| `pkg/shared/sanitization/gateway_sanitization_test.go` + fallback tests | 33 | Underlying pattern library (shared with Gateway/Notification) |
| `test/integration/kubernautagent/investigator/investigator_sanitization_test.go` (`IT-KA-433W-009/010/011`) | 3 | End-to-end: credential/injection stripped from the actual `role:"tool"` message sent to the mock LLM client; raw passthrough when sanitizer is `nil` |

### Security Verification

```bash
# Verify no credentials in LLM audit events / tool-role messages
grep -r "password=\|secret:\|Bearer " audit_events.json
# Should return: only "[REDACTED]" placeholders in successful-tool-output paths
```

---

## Related Documents

| Document | Relationship |
|----------|--------------|
| [BR-KA-211](../../requirements/BR-KA-211-llm-input-sanitization.md) | Business requirement |
| [DD-KA-019-003](DD-KA-019-go-rewrite-design/DD-KA-019-003-security-architecture.md) | Go rewrite security architecture (G4/I1 pipeline design authority) |
| [DD-KA-003](DD-KA-003-mandatory-openapi-client-usage.md) | Unrelated — OpenAPI client pristine-ness (previously mis-cited alongside this DD in `DD-AUTH-005`, corrected) |
| `pkg/shared/sanitization/` | Shared Go pattern library (also used by Gateway `BR-GATEWAY-042`, Notification `BR-NOT-055`) |
| `pkg/kubernautagent/tools/sanitization/` | KA-specific pipeline stages |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 2.0 | 2026-08-01 | Renamed `DD-HAPI-005` → `DD-KA-005`. Full rewrite against Go implementation: documented the 3-stage `Pipeline` (`CredentialSanitizer`/`SecretSanitizer`/`InjectionSanitizer`), fail-closed semantics, pattern-count divergence (17 → 25 unique/28 total, with JWT/base64-secret regressions), and the tool-error sanitization gap. Removed all Python code samples (`llm_sanitizer.py`, `Tool.invoke()` monkey-patch) — never implemented. |
| 1.1 | 2025-12-09 | Original Python-era design (proposed, never implemented as specified). |

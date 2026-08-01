# BR-KA-211: LLM Input Sanitization

**Business Requirement ID**: BR-KA-211
**Service**: Kubernaut Agent (KA)
**Category**: Security
**Priority**: P0 (CRITICAL)
**Status**: ✅ Implemented (Go), with tracked gaps (see below)
**Created**: December 9, 2025

**Last Updated**: 2026-08-01 — Renamed from `BR-HAPI-211` and rewritten against the actual Go
implementation ([Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806)). The original
document specified a Python `llm_sanitizer.py` module and `Tool.invoke()` monkey-patch that were
never built — KA shipped in Go before this design was implemented as specified. See
[DD-KA-005](../architecture/decisions/DD-KA-005-llm-input-sanitization.md) for full architecture
detail; this document is scoped to the business requirement and acceptance criteria.

---

## Summary

Kubernaut Agent (KA) MUST sanitize data sent to external LLM providers to prevent credential
leakage. Today this is implemented for **successful tool output**; sanitization of **tool error
messages** is a tracked gap (see Acceptance Criteria).

---

## Business Justification

### Problem

KA sends data to external LLM providers (OpenAI, Anthropic, Vertex AI, Gemini, etc.) for AI-powered
investigation. Tool output — `kubectl logs`, pod descriptions, K8s `Secret` objects, MCP/Fleet tool
results — may contain:

- Database passwords in application logs
- API keys in environment variables surfaced via `kubectl describe pod`
- Connection strings in error messages
- Base64-encoded credential material in K8s `Secret`/`SecretList` objects

**Risk**: These credentials would be transmitted to external LLM providers, violating security
policy and potentially exposing sensitive data.

### Business Impact

| Impact Area | Description | Severity |
|-------------|-------------|----------|
| **Security** | Credentials leaked to external providers | 🔴 HIGH |
| **Compliance** | Violation of data protection requirements | 🔴 HIGH |
| **Audit** | Cannot demonstrate credential protection | 🟡 MEDIUM |
| **Trust** | Customer data exposure risk | 🔴 HIGH |

---

## Specification

### Functional Requirements

| ID | Requirement | Priority | Status |
|----|-------------|----------|--------|
| **FR-1** | Sanitize successful tool output before it reaches the LLM | P0 | ✅ Implemented |
| **FR-2** | Redact K8s `Secret`/`SecretList` `data`/`stringData` fields specifically | P0 | ✅ Implemented |
| **FR-3** | Sanitize error messages in tool results | P0 | ❌ **Gap** — raw `err.Error()` reaches the LLM unsanitized on tool failure |
| **FR-4** | Use the shared Go sanitization pattern library for consistency with Gateway/Notification | P1 | ✅ Implemented (`pkg/shared/sanitization`) |
| **FR-5** | Observable sanitization failures | P2 | ✅ Implemented (structured log on pipeline error) |
| **FR-6** | Fail-safe behavior on sanitization errors | P1 | ✅ Implemented — **fail-closed** (withholds tool output), not fail-open/degrade as originally specified |

### Sanitization Coverage

Implemented via `pkg/shared/sanitization` (25 uniquely-named rules / 28 total, spanning passwords,
API keys, bearer/GitHub tokens, AWS credentials, database URLs, private keys, authorization headers,
and PII) plus a K8s-`Secret`-shape-aware stage. See
[DD-KA-005](../architecture/decisions/DD-KA-005-llm-input-sanitization.md#component-1-shared-credential-pattern-library)
for the full rule table and the two coverage regressions versus the original 17-category spec
(no standalone JWT regex, no standalone base64-secret regex).

### Non-Functional Requirements

| ID | Requirement | Target | Status |
|----|-------------|--------|--------|
| **NFR-1** | Sanitization latency | <10ms per call | ✅ Verified by test (`pkg/kubernautagent/tools/sanitization/credential_test.go`) |
| **NFR-2** | Pattern coverage | All shared-library patterns applied to successful tool output | ✅ Implemented |

---

## Architecture

See [DD-KA-005](../architecture/decisions/DD-KA-005-llm-input-sanitization.md) for the full pipeline
diagram and component breakdown. Summary: every LLM-directed tool call is wrapped by
`executeTool` (`internal/kubernautagent/investigator/investigator_tools.go`), which — on successful
tool execution — runs the result through a 3-stage `Pipeline` (`CredentialSanitizer` →
`SecretSanitizer` → `InjectionSanitizer`) before the alignment shadow-agent and LLM ever see it.

---

## Test Coverage

| Test Type | Location | Count |
|-----------|----------|-------|
| Unit (credential patterns) | `pkg/kubernautagent/tools/sanitization/credential_test.go` | 26 |
| Unit (injection stripping) | `pkg/kubernautagent/tools/sanitization/injection_test.go` | 17 |
| Unit (K8s Secret redaction) | `pkg/kubernautagent/tools/sanitization/secret_test.go` | 7 |
| Unit (pipeline chaining/fail-closed) | `pkg/kubernautagent/tools/sanitization/pipeline_test.go` | 2 |
| Unit (shared pattern library) | `pkg/shared/sanitization/gateway_sanitization_test.go` + fallback specs | 33 |
| Integration (end-to-end wiring) | `test/integration/kubernautagent/investigator/investigator_sanitization_test.go` (`IT-KA-433W-009/010/011`) | 3 |

---

## Acceptance Criteria

### Implemented (P0)

- [x] Successful tool output sanitized before reaching the LLM (`kubernetes` tools, MCP tools, Fleet remote tools)
- [x] K8s `Secret`/`SecretList` `data`/`stringData` fields specifically redacted
- [x] Shared Go pattern library reused (consistent with Gateway `BR-GATEWAY-042`, Notification `BR-NOT-055`)
- [x] Fail-closed on sanitization pipeline error (tool output withheld, never leaked unsanitized)
- [x] Integration test proves credentials/injection phrases stripped from the actual message sent to the LLM

### Tracked Gap (P0 — follow-up required)

- [ ] Tool-call **error** messages sanitized before reaching the LLM. Currently, when a tool
  execution fails, `toolErrorJSON` wraps the raw `err.Error()` string and this reaches the LLM without
  passing through the sanitization pipeline (`internal/kubernautagent/investigator/investigator_tools.go`).
  This is the same functional requirement as the original spec's FR-3/AC for `test_tool_invoke_sanitizes_error`,
  never carried forward to the Go error path.

### Should Have (P1)

- [ ] Standalone JWT-token pattern (currently only caught incidentally via `Bearer` prefix)
- [ ] Standalone generic base64-secret pattern (present in the original 17-category spec, dropped in the Go library)

---

## Related Documents

| Document | Relationship |
|----------|--------------|
| [DD-KA-005](../architecture/decisions/DD-KA-005-llm-input-sanitization.md) | Design decision (full architecture, pipeline stages, known gaps) |
| `pkg/shared/sanitization/` | Shared Go pattern library (also used by Gateway, Notification) |
| `pkg/kubernautagent/tools/sanitization/` | KA-specific pipeline stages |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 2.0 | 2026-08-01 | Renamed `BR-HAPI-211` → `BR-KA-211`. Rewrote against Go implementation: marked FR-1/2/4/5/6 implemented, FR-3 (error-message sanitization) as a tracked gap, corrected fail-safe semantics from fail-open/degrade to fail-closed, documented actual test coverage, removed Python code samples. |
| 1.1 | 2025-12-09 | Original Python-era specification (proposed, never implemented as specified). |

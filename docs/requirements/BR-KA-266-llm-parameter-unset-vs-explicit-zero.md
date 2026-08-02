# BR-KA-266: LLM Runtime Parameter "Unset" vs "Explicit Zero" Semantics

**ID**: BR-KA-266
**Title**: Preserve "Not Configured" vs "Explicitly Configured as Zero" for Hot-Reloadable LLM Parameters
**Category**: Kubernaut Agent (KA)
**Priority**: 🟡 MEDIUM
**Version**: V1.0
**Status**: ✅ APPROVED (implemented)
**Date**: 2026-08-01

> **Note**: This BR formalizes a requirement that was previously cited in code comments as
> `BR-HAPI-199` — a shorthand reference with no corresponding formal requirement document.
> Minted here as part of [Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806)
> (HAPI→KA terminology cleanup) to close that documentation gap. The underlying behavior was
> implemented for [Issue #1749](https://github.com/jordigilh/kubernaut/issues/1749).

---

## Business Context

### Problem Statement

Kubernaut Agent (KA) supports hot-reloadable LLM runtime parameters (e.g. `temperature`) via a
watched ConfigMap (`kubernaut-agent-llm-runtime`, see `LLMRuntimeConfig` in
`internal/kubernautagent/config/config_types.go`). Some LLM parameters have a meaningful
"explicitly set to zero" state that is semantically distinct from "operator never configured
this field":

- **Temperature = 0.0 (explicit)**: operator wants fully deterministic output.
- **Temperature unset**: operator has no preference; the parameter should be omitted from the
  wire request entirely.

This distinction matters because **some LLM providers reject an explicit `temperature` parameter
outright with an HTTP 400** (e.g. `claude-opus-4-8`, which only accepts the default). If KA
cannot tell "unset" apart from "explicitly zero", it must pick one of two wrong behaviors:

1. Treat unset as `0` (Go's zero value for `float64`) → forces a temperature parameter onto every
   request, breaking providers/models that reject it.
2. Treat explicit `0` as unset → silently drops the operator's deterministic-output request.

[Issue #1749](https://github.com/jordigilh/kubernaut/issues/1749) tracked the discovery and fix
of exactly this bug: `mergeLLMConfig` used a `if rt.Temperature != 0` guard that conflated the
two states.

### Business Value

| Benefit | Impact |
|---------|--------|
| **Provider Compatibility** | KA works correctly with models that reject an explicit `temperature` field (e.g. `claude-opus-4-8`) |
| **Operator Intent Preserved** | An operator who explicitly configures `temperature: 0` for deterministic output gets that behavior, not a silently-dropped setting |
| **Correctness** | Config merge and LLM wire-request construction do not conflate "not configured" with "configured as the zero value" |

---

## Requirements

### BR-KA-266.1: Nilable Representation

**MUST**: Hot-reloadable LLM parameters that have a meaningful zero value (currently:
`temperature`) SHALL be represented as pointer types (`*float64`) in
`LLMRuntimeConfig` and in the LLM client's per-request `Options`/`RuntimeParams`, so that "key
absent from YAML" (`nil`) is distinguishable from "key present with value `0`".

### BR-KA-266.2: Merge Preservation

**MUST**: `mergeLLMConfig` (`cmd/kubernautagent/llm_builder.go`) SHALL copy the pointer directly
(`merged.Temperature = rt.Temperature`) rather than using a truthiness/non-zero check, so that a
`nil` source produces a `nil` result and an explicit `0.0` source produces an explicit `0.0`
result.

### BR-KA-266.3: Wire-Request Omission

**MUST**: When constructing the outbound LLM request, `pkg/kubernautagent/llm/swappable_client.go`
(and any provider-specific client built on it) SHALL omit the `temperature` field entirely from
the wire request when the configured value is `nil`, and SHALL send it when the value is a
non-nil pointer (including a pointer to `0.0`).

### BR-KA-266.4: Non-Determinism-Sensitive Callers May Opt Out

**MAY**: Callers with no determinism requirement (e.g. `pkg/kubernautagent/tools/summarizer`)
MAY omit `temperature` unconditionally rather than threading the nil/explicit-zero distinction
through, since there is no operator-facing "must be deterministic" expectation for that code path.

---

## Acceptance Criteria

- [x] `LLMRuntimeConfig.Temperature` is `*float64`, not `float64` (`internal/kubernautagent/config/config_types.go`)
- [x] `mergeLLMConfig` preserves `nil` and explicit `0.0` through the merge (`cmd/kubernautagent/llm_builder.go`)
- [x] Unit test proves an explicit `0.0` survives config merge (`UT-KA-1749-008`, `cmd/kubernautagent/llm_builder_1749_test.go`)
- [x] Unit test proves an explicit `0.0` is still sent on the wire request (`UT-KA-1749-002`, `pkg/kubernautagent/llm/chat_with_params_test.go`)

---

## Related Documents

- [Issue #1749](https://github.com/jordigilh/kubernaut/issues/1749): Original bug report and fix (temperature unset vs. explicit-zero conflation)
- [Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806): HAPI→KA documentation cleanup that minted this formal BR from the informal `BR-HAPI-199` code citation
- `internal/kubernautagent/config/config_types.go`: `LLMRuntimeConfig.Temperature` definition
- `cmd/kubernautagent/llm_builder.go`: `mergeLLMConfig` implementation
- `pkg/kubernautagent/llm/swappable_client.go`: wire-request construction

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-08-01 | Initial version. Formalizes the informal `BR-HAPI-199` code citation (no prior doc existed) into a proper BR, per Issue #1806. |

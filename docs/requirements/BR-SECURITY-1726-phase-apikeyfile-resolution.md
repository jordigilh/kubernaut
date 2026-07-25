# BR-SECURITY-1726: Phase-Override `apiKeyFile` Resolution

**Business Requirement ID**: BR-SECURITY-1726
**Category**: Kubernaut Agent — LLM Credential Management
**Priority**: **P1 (HIGH)** — Credential-confusion defect blocking a planned enhancement
**Target Version**: **V1.6**
**Status**: ✅ Implemented
**Date**: July 24, 2026
**GitHub Issues**: [#1726](https://github.com/jordigilh/kubernaut/issues/1726)
**Related**: [BR-AI-1470](../tests/1470/TEST_PLAN.md) (per-phase LLM model routing — the feature whose
implicit contract this closes a gap in), [kubernaut-operator#233](https://github.com/jordigilh/kubernaut-operator/issues/233) (blocked by this BR)

---

## Business Need

### Problem Statement

Kubernaut Agent (KA) supports per-phase LLM overrides (`llmRuntime.phaseModels`) and a dedicated
shadow/alignment-checker LLM (`ai.alignmentCheck.llm`), each of which may specify its own
`apiKeyFile` pointing at a different mounted credential than the base profile's. That `apiKeyFile`
is correctly parsed and threaded through config merging (`LLMRuntimeConfig.EffectivePhaseConfig`,
`AlignmentCheckConfig.EffectiveLLM`, `mergeLLMConfig`), but is **never actually resolved into an
API key**: the merge logic copies the base profile's already-resolved `APIKey` into the override's
output and only ever overwrites `APIKeyFile`, so the mismatch between the two fields is never
corrected before a client is built.

All three production call sites that build a phase/alignment LLM client
(`buildLLMClients`, `buildAlignmentStack` in `cmd/kubernautagent/bootstrap.go`, and
`reloadSinglePhaseClient` in `cmd/kubernautagent/llm_builder.go`) construct the client directly
from this merged config with no resolution step in between. Every phase or shadow client therefore
silently authenticates with the **base profile's** credentials, regardless of what its own
`apiKeyFile` points to — with no error, log line, or validation failure.

### Impact Without This BR

- A phase override that only changes `model`/`endpoint` (the only case any deployment configures
  today, because `kubernaut-operator` currently enforces a same-`credentialsSecretName` guardrail
  across all `phaseModels`) happens to work correctly, since it never actually needs a different key.
- A phase or shadow-agent override intended to use a genuinely different provider and/or credential
  file would silently authenticate against that provider using the *base profile's* key — not a
  clean auth failure, just wrong/undefined behavior, with zero observability.
- This blocks `kubernaut-operator`'s planned enhancement
  ([kubernaut-operator#233](https://github.com/jordigilh/kubernaut-operator/issues/233)) to lift its
  operator-side same-`credentialsSecretName` guardrail on `phaseModels` — that guardrail exists
  specifically to prevent this class of silent misbehavior, and removing it before this fix lands
  would reintroduce the exact failure mode it was designed to catch.

---

## Decision: Reuse the Existing, Already-Unit-Tested `ResolveAPIKey()` at All 3 Call Sites

`types.LLMConfig.ResolveAPIKey()` (`pkg/shared/types/llm.go`) already implements the correct
contract — read the file at `APIKeyFile`, trim it, and set `APIKey`, failing (without mutating
`APIKey`) if the file is missing, unreadable, or empty — and already has unit test coverage
(`UT-SH-1488-003`/`004`). Since `mergeLLMConfig()` already produces the *correct* `APIKeyFile` at
each of the 3 call sites (the phase/alignment override's own file if set, otherwise the inherited
base file), the fix is to call `merged.ResolveAPIKey()` immediately after each `mergeLLMConfig()`
call and before `buildLLMClientFromConfig()`, treating a resolution error as fatal:

1. **Boot call sites** (`buildLLMClients` phase loop, `buildAlignmentStack`): treat a resolution
   failure the same as any other client-construction failure — `os.Exit(1)`, matching the existing
   fail-closed pattern for boot errors in both functions.
2. **Hot-reload call site** (`reloadSinglePhaseClient`): treat a resolution failure the same as any
   other `buildLLMClientFromConfig` failure — log and reject the reload, preserving the existing
   phase client, matching the existing reject-and-keep pattern.
3. **No credentials-dir fallback** on a distinct-but-unreadable phase/alignment `apiKeyFile` —
   falling back to the base profile's credentials-dir scan would simply move today's
   silent-wrong-credentials bug one level down instead of fixing it.

No new type, interface, or helper function is introduced — this is a wiring-only fix (3 call sites
gain a conditional call to an existing exported method), consistent with the finding that the
underlying resolution logic was already correct and already tested; only its invocation was
missing.

---

## Success Criteria

1. A phase override (`llmRuntime.phaseModels.<phase>.apiKeyFile`) with a distinct `apiKeyFile` from
   the base profile builds a client that authenticates with that file's content, at both boot
   (`buildLLMClients`) and hot-reload (`reloadSinglePhaseClient`).
2. A shadow/alignment-checker override (`ai.alignmentCheck.llm.apiKeyFile`) with a distinct
   `apiKeyFile` builds a client that authenticates with that file's content (`buildAlignmentStack`).
3. A phase or alignment override with **no** `apiKeyFile` set continues to authenticate with the
   base profile's key, byte-for-byte unchanged — zero behavior change for every configuration in
   production today.
4. A phase's own `apiKeyFile` set but unreadable/missing fails the affected client build
   observably (boot: process exit with a clear error; hot-reload: reload rejected, existing client
   preserved) — never silently falls back to the credentials-dir scan or the base profile's key.

---

## Related Documents

- [Test Plan — TP-1726-PHASE-APIKEYFILE-RESOLUTION](../tests/1726/TEST_PLAN.md)
- [Test Plan — TP-1470-PER-PHASE-LLM-ROUTING](../tests/1470/TEST_PLAN.md) (the feature this is a
  defect against)

---

**Document Version**: 1.0
**Last Updated**: July 24, 2026
**Maintained By**: Kubernaut Agent Team
